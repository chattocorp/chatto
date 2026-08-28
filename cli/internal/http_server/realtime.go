package http_server

import (
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/live/v1"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

const (
	realtimePath                    = "/api/realtime"
	realtimeProtocolVersion         = 2
	realtimeReadLimitBytes          = 64 << 10
	realtimeReadBufferBytes         = 256
	realtimeWriteBufferBytes        = 512
	realtimeCompressionMinBytes     = 1024
	realtimeHandshakeTimeout        = 10 * time.Second
	realtimeWriteTimeout            = 10 * time.Second
	realtimeCredentialCheckInterval = time.Minute
	// Bound compacted reset construction as well as the long-lived per-socket
	// projection. Each retained room can carry up to 50 decrypted timeline rows.
	realtimeMaxRetainedRooms         = 64
	realtimeMaxRoomIDBytes           = 256
	realtimeHeartbeatIntervalSeconds = uint32(core.MyEventsHeartbeatInterval / time.Second)
)

var realtimeServerCapabilities = []string{
	"chatto.realtime.events.live.v1",
	"chatto.realtime.heartbeat.v1",
	"chatto.realtime.ping.v1",
	"chatto.realtime.events.resume.v1",
	"chatto.realtime.projection.v1",
}

func (s *HTTPServer) setupRealtimeAPI() {
	if s.metrics == nil {
		s.metrics = newProcessMetrics()
	}
	if s.realtimeCatchUps == nil {
		s.realtimeCatchUps = newRealtimeCatchUpAdmission()
	}

	writeBufferPool := &sync.Pool{}
	upgrader := websocket.Upgrader{
		ReadBufferSize:    realtimeReadBufferBytes,
		WriteBufferSize:   realtimeWriteBufferBytes,
		WriteBufferPool:   writeBufferPool,
		EnableCompression: s.config.Webserver.WebSocketCompressionEnabled(),
		CheckOrigin: func(r *http.Request) bool {
			return s.checkRealtimeWebSocketOrigin(r)
		},
	}

	s.router.GET(realtimePath, func(c *gin.Context) {
		req := s.injectUserIntoContext(c)
		req = req.WithContext(connectapi.WithRequestBaseURL(req.Context(), s.requestBaseURL(req)))
		upgradeHeaders := make(http.Header)
		for _, cookie := range c.Writer.Header().Values("Set-Cookie") {
			upgradeHeaders.Add("Set-Cookie", cookie)
		}
		conn, err := upgrader.Upgrade(c.Writer, req, upgradeHeaders)
		if err != nil {
			s.logger.Warn("Realtime WebSocket upgrade failed", "error", err)
			return
		}
		s.metrics.realtimeWebSocketOpened()
		defer s.metrics.realtimeWebSocketClosed()
		defer conn.Close()
		if upgrader.EnableCompression {
			// Huffman-only DEFLATE preserves negotiated permessage-deflate while
			// avoiding Lempel-Ziv match searching for the larger frames that pass
			// the write-compression threshold below.
			if err := conn.SetCompressionLevel(flate.HuffmanOnly); err != nil {
				s.logger.Warn("Failed to configure realtime WebSocket compression", "error", err)
			}
		}

		s.serveRealtimeWebSocket(req.Context(), conn)
	})
}

func (s *HTTPServer) checkRealtimeWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if _, ok := parseBrowserOrigin(origin); ok {
		return true
	}
	s.logger.Warn("Realtime WebSocket connection rejected: invalid origin")
	return false
}

func (s *HTTPServer) serveRealtimeWebSocket(parent context.Context, conn *websocket.Conn) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	conn.SetReadLimit(realtimeReadLimitBytes)
	var writeMu sync.Mutex
	writeFrame := func(frame *realtimev1.RealtimeServerFrame) error {
		data, err := proto.Marshal(frame)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		// Compression setup is disproportionately expensive for the small
		// invalidation and heartbeat frames that dominate this protocol. Keep
		// negotiated compression for larger payloads where it can repay the
		// compressor state.
		conn.EnableWriteCompression(
			shouldCompressRealtimeFrame(s.config.Webserver.WebSocketCompressionEnabled(), len(data)),
		)
		if err := conn.SetWriteDeadline(time.Now().Add(realtimeWriteTimeout)); err != nil {
			return err
		}
		return conn.WriteMessage(websocket.BinaryMessage, data)
	}
	writeError := func(code, message string, fatal bool) {
		_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Error{
			Error: &realtimev1.RealtimeError{Code: code, Message: message, Fatal: fatal},
		}})
	}
	writeRoomError := func(code, message, roomID string, retryAfter time.Duration) {
		realtimeError := &realtimev1.RealtimeError{
			Code: code, Message: message, RoomId: proto.String(roomID),
		}
		if retryAfter > 0 {
			realtimeError.RetryAfterMs = proto.Uint32(uint32(retryAfter.Milliseconds()))
		}
		_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Error{
			Error: realtimeError,
		}})
	}

	hello, err := readRealtimeClientFrame(conn, realtimeHandshakeTimeout)
	if err != nil {
		writeError("bad_hello", "expected binary protobuf hello frame", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "bad hello"), time.Now().Add(time.Second))
		return
	}
	clientHello := hello.GetHello()
	if clientHello == nil {
		writeError("bad_hello", "first frame must be hello", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "bad hello"), time.Now().Add(time.Second))
		return
	}
	if clientHello.ProtocolVersion != realtimeProtocolVersion {
		writeError("unsupported_protocol", "unsupported realtime protocol version", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "unsupported protocol"), time.Now().Add(time.Second))
		return
	}
	ctx, user, err := s.realtimeAuthenticatedUser(ctx, clientHello)
	if err != nil {
		if !errors.Is(err, core.ErrNotAuthenticated) {
			writeError("temporarily_unavailable", "authentication service temporarily unavailable", true)
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "temporarily unavailable"), time.Now().Add(time.Second))
			return
		}
		writeError("authentication_required", "authentication required", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authentication required"), time.Now().Add(time.Second))
		return
	}

	var credentialDeadlineReached <-chan time.Time
	var credentialDeadlineTimer *time.Timer
	credential, credentialOK := authctx.CredentialForContext(ctx)
	credentialDeadline, deadlineOK := realtimeCredentialDeadline(credential, s.config.Auth.TokenTTLOrDefault())
	if credentialOK && deadlineOK {
		remaining := time.Until(credentialDeadline)
		if remaining <= 0 {
			remaining = time.Nanosecond
		}
		credentialDeadlineTimer = time.NewTimer(remaining)
		credentialDeadlineReached = credentialDeadlineTimer.C
	}
	if credentialDeadlineTimer != nil {
		defer credentialDeadlineTimer.Stop()
		credentialDeadlineWatcherDone := make(chan struct{})
		go func() {
			defer close(credentialDeadlineWatcherDone)
			select {
			case <-credentialDeadlineReached:
				terminate := terminateRealtimeForBearerExpiry
				closeCode := websocket.ClosePolicyViolation
				closeReason := "authentication required"
				if credential.Kind == authctx.RuntimeCredentialKindCookieSession {
					terminate = terminateRealtimeForCookieRenewal
					closeCode = websocket.CloseNormalClosure
					closeReason = "credential renewal required"
				}
				terminate(cancel, writeFrame, func() {
					_ = conn.WriteControl(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(closeCode, closeReason),
						time.Now().Add(time.Second),
					)
					_ = conn.Close()
				})
			case <-ctx.Done():
			}
		}()
		defer func() {
			cancel()
			<-credentialDeadlineWatcherDone
		}()
	}

	if credentialOK && (credential.Kind == authctx.RuntimeCredentialKindCookieSession || credential.Kind == authctx.RuntimeCredentialKindBearerToken) {
		credentialCheckDone := make(chan struct{})
		credentialCheckEvery := realtimeCredentialCheckInterval
		if s.realtimeCredentialCheckEvery > 0 {
			credentialCheckEvery = s.realtimeCredentialCheckEvery
		}
		go func() {
			defer close(credentialCheckDone)
			ticker := time.NewTicker(credentialCheckEvery)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					err := s.revalidateRealtimeCredential(ctx)
					if !errors.Is(err, core.ErrNotAuthenticated) {
						// Transient storage failures do not log out a valid user. The
						// next interval retries the independent validation.
						continue
					}
					terminateRealtimeForCredentialRevocation(cancel, writeFrame, func() {
						_ = conn.WriteControl(
							websocket.CloseMessage,
							websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authentication required"),
							time.Now().Add(time.Second),
						)
						_ = conn.Close()
					})
					return
				case <-ctx.Done():
					return
				}
			}
		}()
		defer func() {
			cancel()
			<-credentialCheckDone
		}()
	}

	var oauthClientAccessDenied <-chan struct{}
	stopOAuthClientAccessWatch := func() {}
	if credential, ok := authctx.CredentialForContext(ctx); ok && credential.OAuthClientID != "" {
		oauthClientAccessDenied, stopOAuthClientAccessWatch = s.core.WatchOAuthClientAccessDenied(credential.OAuthClientID)
	}
	defer stopOAuthClientAccessWatch()
	if oauthClientAccessDenied != nil {
		oauthClientBlockWatcherDone := make(chan struct{})
		go func() {
			defer close(oauthClientBlockWatcherDone)
			select {
			case <-oauthClientAccessDenied:
				terminateRealtimeForOAuthClientBlock(cancel, writeFrame, func() {
					_ = conn.WriteControl(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authentication required"),
						time.Now().Add(time.Second),
					)
					_ = conn.Close()
				})
			case <-ctx.Done():
			}
		}()
		defer func() {
			cancel()
			<-oauthClientBlockWatcherDone
		}()
	}

	var botAPIKeyInvalidated <-chan struct{}
	stopBotAPIKeyWatch := func() {}
	if credential, ok := authctx.CredentialForContext(ctx); ok &&
		credential.Kind == authctx.RuntimeCredentialKindBotAPIKey && len(credential.BotAPIKeyVerifier) > 0 {
		botAPIKeyInvalidated, stopBotAPIKeyWatch = s.core.WatchBotAPIKeyInvalidated(credential.UserID, credential.BotAPIKeyVerifier)
	}
	defer stopBotAPIKeyWatch()
	if botAPIKeyInvalidated != nil {
		botAPIKeyWatcherDone := make(chan struct{})
		go func() {
			defer close(botAPIKeyWatcherDone)
			select {
			case <-botAPIKeyInvalidated:
				terminateRealtimeForBotAPIKeyInvalidation(cancel, writeFrame, func() {
					_ = conn.WriteControl(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authentication required"),
						time.Now().Add(time.Second),
					)
					_ = conn.Close()
				})
			case <-ctx.Done():
			}
		}()
		defer func() {
			cancel()
			<-botAPIKeyWatcherDone
		}()
	}

	if err := writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Hello{
		Hello: &realtimev1.RealtimeServerHello{
			ProtocolVersion:          realtimeProtocolVersion,
			ServerVersion:            s.version,
			HeartbeatIntervalSeconds: realtimeHeartbeatIntervalSeconds,
			Capabilities:             append([]string(nil), realtimeServerCapabilities...),
		},
	}}); err != nil {
		return
	}

	subscribe, err := readRealtimeClientFrame(conn, realtimeHandshakeTimeout)
	if err != nil {
		writeError("bad_subscribe", "expected subscribe_events frame", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "bad subscribe"), time.Now().Add(time.Second))
		return
	}
	subscribeEvents := subscribe.GetSubscribeEvents()
	if subscribeEvents == nil {
		writeError("bad_subscribe", "second frame must be subscribe_events", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "bad subscribe"), time.Now().Add(time.Second))
		return
	}
	if err := s.revalidateRealtimeCredential(ctx); err != nil {
		if errors.Is(err, core.ErrNotAuthenticated) {
			writeError("authentication_required", "authentication required", true)
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authentication required"), time.Now().Add(time.Second))
			return
		}
		writeError("temporarily_unavailable", "authentication service temporarily unavailable", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "temporarily unavailable"), time.Now().Add(time.Second))
		return
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return
	}
	retainedRooms, err := realtimeRetainedRoomSet(subscribeEvents.GetRetainedRoomIds())
	if err != nil {
		writeError("bad_subscribe", err.Error(), true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "bad subscribe"), time.Now().Add(time.Second))
		return
	}

	resumeCursor := strings.TrimSpace(subscribeEvents.GetResumeCursor())
	cursorAtBoundary, err := s.core.RealtimeCursorAtCurrentBoundary(ctx, user.Id, resumeCursor)
	if err != nil {
		writeError("replay_unavailable", "realtime replay is temporarily unavailable", true)
		return
	}
	// A cursorless compacted bootstrap cannot request historical events. Bound
	// it by catch-up concurrency and timeout, while reserving the per-user rate
	// budget for explicit stale-cursor replay attempts (including cursor reuse).
	meteredReplay := resumeCursor != "" && !cursorAtBoundary
	releaseCatchUp, admissionErr := s.realtimeCatchUps.acquire(user.Id, meteredReplay)
	if admissionErr != nil {
		s.metrics.realtimeCatchUpRejected(admissionErr.code)
		retryAfterMs := uint32(admissionErr.retryAfter.Milliseconds())
		_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
			Close: &realtimev1.RealtimeClose{Code: admissionErr.code, Message: "realtime catch-up capacity is temporarily unavailable", Reconnect: true, RetryAfterMs: retryAfterMs},
		}})
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, admissionErr.code), time.Now().Add(time.Second))
		return
	}
	s.metrics.realtimeCatchUpStarted()
	var finishCatchUpOnce sync.Once
	finishCatchUp := func() {
		finishCatchUpOnce.Do(func() {
			releaseCatchUp()
			s.metrics.realtimeCatchUpFinished()
		})
	}
	defer finishCatchUp()
	catchUpCtx, cancelCatchUp := context.WithTimeout(ctx, s.realtimeCatchUps.timeout)
	defer cancelCatchUp()
	writeCatchUpFrame := func(frame *realtimev1.RealtimeServerFrame) error {
		if err := catchUpCtx.Err(); err != nil {
			return err
		}
		return writeFrame(frame)
	}
	failCatchUp := func(logMessage string, err error) {
		if errors.Is(catchUpCtx.Err(), context.DeadlineExceeded) {
			s.metrics.realtimeCatchUpTimedOut()
			s.logger.Warn("Realtime catch-up timed out", "error", err)
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
				Close: &realtimev1.RealtimeClose{Code: "catch_up_timeout", Message: "realtime catch-up exceeded its time budget", Reconnect: true, RetryAfterMs: 1000},
			}})
			return
		}
		s.logger.Warn(logMessage, "error", err)
		writeError("replay_unavailable", "realtime projection replay is temporarily unavailable", true)
	}
	handleCatchUpWriteError := func(err error) {
		if errors.Is(catchUpCtx.Err(), context.DeadlineExceeded) {
			failCatchUp("Realtime catch-up delivery timed out", err)
		}
	}

	events, err := s.core.StreamMyEventsWithOptions(ctx, user.Id, core.StreamMyEventsOptions{TouchPresence: false})
	if err != nil {
		writeError("subscribe_failed", "failed to start realtime event stream", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "subscribe failed"), time.Now().Add(time.Second))
		return
	}
	replayPlan, err := s.core.PlanRealtimeReplay(catchUpCtx, user.Id, subscribeEvents.GetResumeCursor())
	if err != nil {
		if errors.Is(catchUpCtx.Err(), context.DeadlineExceeded) {
			failCatchUp("Realtime replay planning timed out", err)
			return
		}
		code, message := realtimeReplayError(err)
		writeError(code, message, true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, code), time.Now().Add(time.Second))
		return
	}
	if resumeCursor != "" && !meteredReplay && replayPlan.HadSequenceGap {
		// EVT advanced after the current-boundary check. Charge the newly-real
		// replay gap before emitting subscribed or projection frames.
		if chargeErr := s.realtimeCatchUps.consumeReplayToken(user.Id); chargeErr != nil {
			s.metrics.realtimeCatchUpRejected(chargeErr.code)
			retryAfterMs := uint32(chargeErr.retryAfter.Milliseconds())
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
				Close: &realtimev1.RealtimeClose{Code: chargeErr.code, Message: "realtime catch-up capacity is temporarily unavailable", Reconnect: true, RetryAfterMs: retryAfterMs},
			}})
			return
		}
	}

	subscribed := &realtimev1.RealtimeSubscribed{StartCursor: &replayPlan.StartCursor}
	if err := writeCatchUpFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Subscribed{
		Subscribed: subscribed,
	}}); err != nil {
		handleCatchUpWriteError(err)
		return
	}

	hydrateRooms := make(chan string, 16)
	go s.readRealtimeControlFrames(ctx, cancel, conn, writeFrame, hydrateRooms)
	var roomMarkerFence *uint64
	if replayPlan.Reset {
		retainedRoomIDs := make([]string, 0, len(retainedRooms))
		for roomID := range retainedRooms {
			retainedRoomIDs = append(retainedRoomIDs, roomID)
		}
		slices.Sort(retainedRoomIDs)
		revision, err := s.writeRealtimeProjectionSnapshot(catchUpCtx, user.Id, retainedRoomIDs, writeCatchUpFrame)
		if err != nil {
			failCatchUp("Realtime compacted projection replay failed", err)
			return
		}
		roomMarkerFence = &revision
	}
	for _, event := range replayPlan.Events {
		frame, handled, err := s.realtimeProjectionFrameForEventWithRooms(catchUpCtx, user.Id, event, retainedRooms)
		if err != nil {
			failCatchUp("Realtime replay mapping failed", err)
			return
		}
		if !handled {
			s.logger.Warn("Realtime durable event has no projection mapping", "event_id", event.ID())
			writeError("replay_unavailable", "realtime replay is temporarily unavailable", true)
			return
		}
		if err := writeCatchUpFrame(frame); err != nil {
			handleCatchUpWriteError(err)
			return
		}
	}
	reconciliation, err := s.realtimeProjectionReconciliationFrame(catchUpCtx, user.Id, roomMarkerFence)
	if err != nil {
		failCatchUp("Realtime latest-value reconciliation failed", err)
		return
	}
	if err := writeCatchUpFrame(reconciliation); err != nil {
		handleCatchUpWriteError(err)
		return
	}
	if err := writeCatchUpFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_CaughtUp{
		CaughtUp: &realtimev1.RealtimeCaughtUp{Cursor: replayPlan.BoundaryCursor},
	}}); err != nil {
		handleCatchUpWriteError(err)
		return
	}
	cancelCatchUp()
	finishCatchUp()

	for {
		select {
		case <-ctx.Done():
			return
		case roomID := <-hydrateRooms:
			if _, retained := retainedRooms[roomID]; retained {
				continue
			}
			if len(retainedRooms) >= realtimeMaxRetainedRooms {
				writeRoomError("too_many_retained_rooms", "too many retained room timelines", roomID, 0)
				continue
			}
			releaseHydration, admissionErr := s.realtimeCatchUps.acquireHydration(user.Id)
			if admissionErr != nil {
				writeRoomError(admissionErr.code, "room hydration capacity is temporarily unavailable", roomID, admissionErr.retryAfter)
				continue
			}
			// Retain the request even if authorization currently fails. If this
			// viewer joins later on the same socket, that membership fact can
			// atomically materialise the room without a second client mechanism.
			retainedRooms[roomID] = struct{}{}
			frame, hydrateErr := s.realtimeProjectionRoomTimelineFrame(ctx, user.Id, roomID)
			releaseHydration()
			if hydrateErr != nil {
				if errors.Is(hydrateErr, core.ErrNotFound) || errors.Is(hydrateErr, core.ErrPermissionDenied) || errors.Is(hydrateErr, core.ErrNotRoomMember) {
					writeRoomError("room_unavailable", "room timeline is unavailable", roomID, 0)
					continue
				}
				s.logger.Warn("Realtime room hydration failed", "error", hydrateErr)
				_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
					Close: &realtimev1.RealtimeClose{Code: "room_hydration_failed", Message: "room timeline hydration failed", Reconnect: true},
				}})
				return
			}
			if err := writeFrame(frame); err != nil {
				return
			}
		case event, ok := <-events:
			if !ok {
				_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
					Close: &realtimev1.RealtimeClose{Code: "stream_closed", Message: "event stream closed", Reconnect: true, RetryAfterMs: 1000},
				}})
				return
			}
			if event.DeliverySeq() > 0 && event.DeliverySeq() <= replayPlan.BoundarySequence {
				continue
			}
			var frame *realtimev1.RealtimeServerFrame
			var handled bool
			var mapErr error
			frame, handled, mapErr = s.realtimeProjectionFrameForEventWithRooms(ctx, user.Id, event, retainedRooms)
			if mapErr == nil && !handled {
				if event.DeliverySeq() > 0 {
					mapErr = errors.New("durable event has no projection mapping")
				} else {
					frame, mapErr = s.realtimeServerFrameForEvent(ctx, user.Id, event)
				}
			}
			if mapErr != nil {
				s.logger.Warn("Dropping unsupported realtime event", "event_id", event.ID(), "error", mapErr)
				if event.DeliverySeq() > 0 {
					_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
						Close: &realtimev1.RealtimeClose{Code: "projection_mapping_failed", Message: "durable projection mapping failed", Reconnect: true},
					}})
					return
				}
				continue
			}
			if err := writeFrame(frame); err != nil {
				return
			}
			if frame.GetClose() != nil {
				return
			}
			if core.EventSessionTerminated(event) != nil {
				return
			}
		}
	}
}

func realtimeReplayError(err error) (code, message string) {
	switch {
	case errors.Is(err, core.ErrRealtimeCursorInvalid):
		return "invalid_cursor", "the realtime resume cursor is invalid for this server history"
	case errors.Is(err, core.ErrRealtimeCursorExpired):
		return "cursor_expired", "the realtime resume cursor is no longer retained"
	case errors.Is(err, core.ErrRealtimeReplayLimitExceeded):
		return "replay_limit_exceeded", "the realtime gap is too large to replay; refresh projected state"
	default:
		return "replay_unavailable", "realtime replay is temporarily unavailable"
	}
}

func shouldCompressRealtimeFrame(compressionEnabled bool, payloadBytes int) bool {
	return compressionEnabled && payloadBytes >= realtimeCompressionMinBytes
}

func realtimeCredentialDeadline(credential authctx.RuntimeCredential, cookieTTL time.Duration) (time.Time, bool) {
	if credential.ExpiresAt.IsZero() {
		return time.Time{}, false
	}
	switch credential.Kind {
	case authctx.RuntimeCredentialKindBearerToken:
		return credential.ExpiresAt, true
	case authctx.RuntimeCredentialKindCookieSession:
		if cookieTTL <= 0 {
			return credential.ExpiresAt, true
		}
		return credential.ExpiresAt.Add(-cookieTTL / 4), true
	default:
		return time.Time{}, false
	}
}

func readRealtimeClientFrame(conn *websocket.Conn, timeout time.Duration) (*realtimev1.RealtimeClientFrame, error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	mt, data, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if mt != websocket.BinaryMessage {
		return nil, errors.New("expected binary message")
	}
	var frame realtimev1.RealtimeClientFrame
	if err := proto.Unmarshal(data, &frame); err != nil {
		return nil, err
	}
	return &frame, nil
}

// terminateRealtimeForOAuthClientBlock cancels authorized work before any
// potentially blocking transport write. The established authentication close
// code preserves safe behaviour for clients that predate OAuth-client policy.
func terminateRealtimeForOAuthClientBlock(
	cancel context.CancelFunc,
	writeFrame func(*realtimev1.RealtimeServerFrame) error,
	closeConnection func(),
) {
	cancel()
	_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
		Close: &realtimev1.RealtimeClose{
			Code:      "authentication_required",
			Message:   "the OAuth client has been blocked",
			Reconnect: false,
		},
	}})
	closeConnection()
}

// terminateRealtimeForBearerExpiry asks a human client to rotate its access
// token and reconnect while preserving its durable resume cursor.
func terminateRealtimeForBearerExpiry(
	cancel context.CancelFunc,
	writeFrame func(*realtimev1.RealtimeServerFrame) error,
	closeConnection func(),
) {
	cancel()
	_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
		Close: &realtimev1.RealtimeClose{
			Code:      "authentication_required",
			Message:   "the access token has expired",
			Reconnect: true,
		},
	}})
	closeConnection()
}

func terminateRealtimeForCredentialRevocation(
	cancel context.CancelFunc,
	writeFrame func(*realtimev1.RealtimeServerFrame) error,
	closeConnection func(),
) {
	cancel()
	_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
		Close: &realtimev1.RealtimeClose{
			Code:      "authentication_required",
			Message:   "the session is no longer valid",
			Reconnect: false,
		},
	}})
	closeConnection()
}

// terminateRealtimeForCookieRenewal reconnects a cookie-authenticated browser
// before the credential expires. The bundled client first calls the explicit
// HTTP renewal endpoint, then opens a replacement socket with the same stable
// handle and a renewed cookie lifetime.
func terminateRealtimeForCookieRenewal(
	cancel context.CancelFunc,
	writeFrame func(*realtimev1.RealtimeServerFrame) error,
	closeConnection func(),
) {
	cancel()
	_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
		Close: &realtimev1.RealtimeClose{
			Code:      "session_renewal_required",
			Message:   "the browser session is ready for renewal",
			Reconnect: true,
		},
	}})
	closeConnection()
}

// terminateRealtimeForBotAPIKeyInvalidation cancels authorized work before
// writing a terminal frame. The key generation is watched through the durable
// user-auth projection, so rotation reaches sockets on every replica.
func terminateRealtimeForBotAPIKeyInvalidation(
	cancel context.CancelFunc,
	writeFrame func(*realtimev1.RealtimeServerFrame) error,
	closeConnection func(),
) {
	cancel()
	_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
		Close: &realtimev1.RealtimeClose{
			Code:      "authentication_required",
			Message:   "the bot API key has been rotated",
			Reconnect: false,
		},
	}})
	closeConnection()
}

func (s *HTTPServer) readRealtimeControlFrames(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, writeFrame func(*realtimev1.RealtimeServerFrame) error, hydrateRooms chan<- string) {
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if mt != websocket.BinaryMessage {
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Error{
				Error: &realtimev1.RealtimeError{Code: "bad_frame", Message: "expected binary protobuf frame", Fatal: true},
			}})
			return
		}
		var frame realtimev1.RealtimeClientFrame
		if err := proto.Unmarshal(data, &frame); err != nil {
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Error{
				Error: &realtimev1.RealtimeError{Code: "bad_frame", Message: "invalid protobuf frame", Fatal: true},
			}})
			return
		}
		switch payload := frame.GetFrame().(type) {
		case *realtimev1.RealtimeClientFrame_Ping:
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Pong{
				Pong: &realtimev1.RealtimePong{Nonce: payload.Ping.GetNonce()},
			}})
		case *realtimev1.RealtimeClientFrame_HydrateRoom:
			roomID := strings.TrimSpace(payload.HydrateRoom.GetRoomId())
			if roomID == "" || len(roomID) > realtimeMaxRoomIDBytes {
				_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Error{
					Error: &realtimev1.RealtimeError{Code: "bad_frame", Message: "invalid room hydration request", Fatal: true},
				}})
				return
			}
			select {
			case hydrateRooms <- roomID:
			case <-ctx.Done():
				return
			}
		default:
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Error{
				Error: &realtimev1.RealtimeError{Code: "bad_frame", Message: "unexpected control frame", Fatal: true},
			}})
			return
		}
	}
}

func realtimeRetainedRoomSet(roomIDs []string) (map[string]struct{}, error) {
	if len(roomIDs) > realtimeMaxRetainedRooms {
		return nil, errors.New("too many retained room timelines")
	}
	rooms := make(map[string]struct{}, len(roomIDs))
	for _, rawRoomID := range roomIDs {
		roomID := strings.TrimSpace(rawRoomID)
		if roomID == "" || len(roomID) > realtimeMaxRoomIDBytes {
			return nil, errors.New("invalid retained room ID")
		}
		rooms[roomID] = struct{}{}
	}
	return rooms, nil
}

func (s *HTTPServer) realtimeAuthenticatedUser(ctx context.Context, hello *realtimev1.RealtimeClientHello) (context.Context, *evtv1.User, error) {
	if token := strings.TrimSpace(hello.GetBearerToken()); token != "" {
		credential, ok, err := s.bearerPresentedCredential(ctx, token)
		if err != nil {
			return ctx, nil, err
		}
		if !ok {
			return ctx, nil, core.ErrNotAuthenticated
		}
		ctx = authctx.WithUser(ctx, credential.user)
		ctx = authctx.WithCredential(ctx, credential.auth)
		return ctx, credential.user, nil
	}
	if user := authctx.ForContext(ctx); user != nil {
		return ctx, user, nil
	}
	if err := authenticationValidationError(ctx); err != nil {
		return ctx, nil, err
	}
	return ctx, nil, core.ErrNotAuthenticated
}

// revalidateRealtimeCredential checks the exact runtime credential that
// authorized the socket. It closes the upgrade-to-subscribe gap and bounds
// access when a live revocation signal is lost.
func (s *HTTPServer) revalidateRealtimeCredential(ctx context.Context) error {
	credential, ok := authctx.CredentialForContext(ctx)
	if !ok {
		return core.ErrNotAuthenticated
	}
	switch credential.Kind {
	case authctx.RuntimeCredentialKindCookieSession:
		record, err := s.core.ValidateCookieCredential(ctx, credential.Handle)
		if err != nil {
			if errors.Is(err, core.ErrCookieSessionNotFound) {
				return core.ErrNotAuthenticated
			}
			return err
		}
		if record.GetUserId() != credential.UserID {
			return core.ErrNotAuthenticated
		}
	case authctx.RuntimeCredentialKindBearerToken:
		validated, err := s.core.ValidatePresentedRuntimeCredential(ctx, credential.Handle, core.AuthTokenPresentationBearer)
		if err != nil {
			if errors.Is(err, core.ErrAuthTokenNotFound) {
				return core.ErrNotAuthenticated
			}
			return err
		}
		if validated.UserID != credential.UserID {
			return core.ErrNotAuthenticated
		}
	}
	return nil
}

func (s *HTTPServer) realtimeServerFrameForEvent(ctx context.Context, viewerID string, event core.EventEnvelope) (*realtimev1.RealtimeServerFrame, error) {
	if event == nil {
		return nil, errors.New("nil event")
	}
	if heartbeat := event.HeartbeatEvent(); heartbeat != nil {
		return &realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Heartbeat{
			Heartbeat: &realtimev1.RealtimeHeartbeat{Id: event.ID(), CreatedAt: event.CreatedAt()},
		}}, nil
	}
	envelope, err := s.realtimeEventEnvelope(ctx, viewerID, event)
	if err != nil {
		return nil, err
	}
	return &realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Event{Event: envelope}}, nil
}

func (s *HTTPServer) realtimeEventEnvelope(ctx context.Context, viewerID string, event core.EventEnvelope) (*realtimev1.RealtimeEventEnvelope, error) {
	envelope := &realtimev1.RealtimeEventEnvelope{
		Id:        event.ID(),
		CreatedAt: event.CreatedAt(),
		ActorId:   optionalRealtimeString(event.ActorID()),
	}

	if event.EVTEvent() != nil {
		return nil, errors.New("durable events must use projection operations")
	}
	if live := event.LiveEvent(); live != nil {
		if err := s.mapRealtimeLive(ctx, viewerID, envelope, live); err != nil {
			return nil, err
		}
		return envelope, nil
	}
	return nil, fmt.Errorf("unknown event envelope %T", event.Payload())
}

func (s *HTTPServer) mapRealtimeLive(ctx context.Context, viewerID string, envelope *realtimev1.RealtimeEventEnvelope, event *livev1.LiveEvent) error {
	switch payload := event.GetEvent().(type) {
	case *livev1.LiveEvent_UserTyping:
		typing := payload.UserTyping
		kind, err := s.core.FindRoomKind(ctx, typing.GetRoomId())
		if err != nil {
			return err
		}
		isMember, err := s.core.RoomMembershipExists(ctx, kind, viewerID, typing.GetRoomId())
		if err != nil {
			return err
		}
		if !isMember {
			return core.ErrPermissionDenied
		}
		var canRead bool
		if typing.GetThreadRootEventId() != "" {
			canRead, err = s.core.CanReadThreadMessages(ctx, viewerID, kind, typing.GetRoomId(), typing.GetThreadRootEventId())
		} else {
			canRead, err = s.core.CanReadMessages(ctx, viewerID, kind, typing.GetRoomId())
		}
		if err != nil {
			return err
		}
		if !canRead {
			return core.ErrPermissionDenied
		}
		envelope.Event = &realtimev1.RealtimeEventEnvelope_UserTyping{UserTyping: &realtimev1.RealtimeTypingEvent{
			RoomId: typing.GetRoomId(), ThreadRootEventId: optionalRealtimeString(typing.GetThreadRootEventId()),
		}}
	case *livev1.LiveEvent_PresenceChanged:
		envelope.Event = &realtimev1.RealtimeEventEnvelope_PresenceChanged{PresenceChanged: &realtimev1.RealtimePresenceChangedEvent{
			UserId: event.GetActorId(), Status: apiPresenceStatus(payload.PresenceChanged.GetStatus()),
		}}
	case *livev1.LiveEvent_SessionTerminated:
		envelope.Event = &realtimev1.RealtimeEventEnvelope_SessionTerminated{SessionTerminated: &realtimev1.RealtimeSessionTerminatedEvent{
			Reason: payload.SessionTerminated.GetReason(),
		}}
	default:
		return fmt.Errorf("unsupported live event %T", payload)
	}
	return nil
}

func optionalRealtimeString(value string) *string {
	if value == "" {
		return nil
	}
	return proto.String(value)
}

func (s *HTTPServer) realtimeDMConversationName(ctx context.Context, viewerID, roomID string) string {
	participants, err := s.core.GetRoomMembersList(ctx, core.KindDM, roomID)
	if err != nil {
		return "Direct Message"
	}

	names := make([]string, 0, len(participants))
	for _, participant := range participants {
		userID := participant.GetUserId()
		if userID == "" || userID == viewerID {
			continue
		}
		user, err := s.core.GetUser(ctx, userID)
		if err != nil {
			continue
		}
		if user.GetDisplayName() != "" {
			names = append(names, user.GetDisplayName())
		} else if user.GetLogin() != "" {
			names = append(names, user.GetLogin())
		}
	}
	if len(names) == 0 {
		return "Direct Message"
	}
	return strings.Join(names, ", ")
}

func (s *HTTPServer) viewerCanReadRealtimeRoomLabel(ctx context.Context, viewerID string, room *evtv1.Room) bool {
	if s == nil || s.core == nil || viewerID == "" || room == nil {
		return false
	}
	kind := core.KindOfRoom(room)
	if kind == core.KindDM {
		ok, err := s.core.RoomMembershipExists(ctx, core.KindDM, viewerID, room.GetId())
		return err == nil && ok
	}
	ok, err := s.core.CanSeeRoom(ctx, viewerID, kind, room.GetId())
	return err == nil && ok
}

func apiPresenceStatus(status string) apiv1.PresenceStatus {
	switch status {
	case core.PresenceStatusOffline:
		return apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE
	case core.PresenceStatusOnline:
		return apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE
	case core.PresenceStatusAway:
		return apiv1.PresenceStatus_PRESENCE_STATUS_AWAY
	case core.PresenceStatusDoNotDisturb:
		return apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB
	default:
		return apiv1.PresenceStatus_PRESENCE_STATUS_UNSPECIFIED
	}
}
