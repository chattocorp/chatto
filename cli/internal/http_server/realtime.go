package http_server

import (
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

const (
	realtimePath                    = "/api/realtime"
	realtimeProtocolVersion         = 4
	realtimeReadLimitBytes          = 64 << 10
	realtimeReadBufferBytes         = 256
	realtimeWriteBufferBytes        = 512
	realtimeCompressionMinBytes     = 1024
	realtimeHandshakeTimeout        = 10 * time.Second
	realtimeWriteTimeout            = 10 * time.Second
	realtimeCredentialCheckInterval = time.Minute
)

var errRealtimeEventOmitted = errors.New("realtime event is not public")

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
			Error: &realtimev1.RealtimeError{Code: realtimeErrorCode(code), Message: message, Fatal: fatal},
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
			ProtocolVersion:   realtimeProtocolVersion,
			ServerVersion:     s.version,
			HeartbeatInterval: durationpb.New(core.MyEventsHeartbeatInterval),
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
	if subscribeEvents.GetInitialState() == realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_UNSPECIFIED {
		writeError("bad_subscribe", "initial_state is required", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "bad subscribe"), time.Now().Add(time.Second))
		return
	}
	if subscribeEvents.GetInitialState() != realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY &&
		subscribeEvents.GetInitialState() != realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT {
		writeError("bad_subscribe", "unsupported initial_state", true)
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
	resumeCursor := strings.TrimSpace(subscribeEvents.GetResumeCursor())
	cursorAtBoundary, err := s.core.RealtimeCursorAtCurrentBoundary(ctx, user.Id, resumeCursor)
	if err != nil {
		writeError("replay_unavailable", "realtime replay is temporarily unavailable", true)
		return
	}
	// A cursorless snapshot or live-only start cannot request history.
	// Bound it by catch-up concurrency and timeout. Reserve the per-user rate
	// budget for explicit stale-cursor replay attempts, including cursor reuse.
	meteredReplay := resumeCursor != "" && !cursorAtBoundary
	releaseCatchUp, admissionErr := s.realtimeCatchUps.acquire(user.Id, meteredReplay)
	if admissionErr != nil {
		s.metrics.realtimeCatchUpRejected(admissionErr.code)
		_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
			Close: &realtimev1.RealtimeClose{Code: realtimeCloseCode(admissionErr.code), Message: "realtime catch-up capacity is temporarily unavailable", Reconnect: true, RetryAfter: durationpb.New(admissionErr.retryAfter)},
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
				Close: &realtimev1.RealtimeClose{Code: realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_CATCH_UP_TIMEOUT, Message: "realtime catch-up exceeded its time budget", Reconnect: true, RetryAfter: durationpb.New(time.Second)},
			}})
			return
		}
		s.logger.Warn(logMessage, "error", err)
		writeError("replay_unavailable", "realtime recovery is temporarily unavailable", true)
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
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
				Close: &realtimev1.RealtimeClose{Code: realtimeCloseCode(chargeErr.code), Message: "realtime catch-up capacity is temporarily unavailable", Reconnect: true, RetryAfter: durationpb.New(chargeErr.retryAfter)},
			}})
			return
		}
	}

	recoveryMode := realtimev1.RealtimeRecoveryMode_REALTIME_RECOVERY_MODE_RESUME
	boundaryCursor := replayPlan.BoundaryCursor
	boundarySequence := replayPlan.BoundarySequence
	var snapshotFrames []*realtimev1.RealtimeServerFrame
	if replayPlan.Reset {
		if subscribeEvents.GetInitialState() == realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT {
			recoveryMode = realtimev1.RealtimeRecoveryMode_REALTIME_RECOVERY_MODE_SNAPSHOT
			var snapshotErr error
			boundarySequence, snapshotFrames, snapshotErr = s.realtimeSnapshotFrames(catchUpCtx, user.Id)
			if snapshotErr != nil {
				failCatchUp("Realtime snapshot capture failed", snapshotErr)
				return
			}
			boundaryCursor, snapshotErr = s.core.RealtimeCursorForSequence(user.Id, boundarySequence)
			if snapshotErr != nil {
				failCatchUp("Realtime snapshot cursor creation failed", snapshotErr)
				return
			}
		} else {
			recoveryMode = realtimev1.RealtimeRecoveryMode_REALTIME_RECOVERY_MODE_LIVE_ONLY
		}
	}
	subscribed := &realtimev1.RealtimeSubscribed{RecoveryMode: recoveryMode}
	if err := writeCatchUpFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Subscribed{
		Subscribed: subscribed,
	}}); err != nil {
		handleCatchUpWriteError(err)
		return
	}

	go s.readRealtimeControlFrames(ctx, cancel, conn, writeFrame)
	for _, frame := range snapshotFrames {
		if err := writeCatchUpFrame(frame); err != nil {
			handleCatchUpWriteError(err)
			return
		}
	}
	for _, event := range replayPlan.Events {
		frame, err := s.realtimeServerFrameForEvent(catchUpCtx, user.Id, event)
		if err != nil {
			if errors.Is(err, errRealtimeEventOmitted) {
				continue
			}
			failCatchUp("Realtime replay projection failed", err)
			return
		}
		if err := writeCatchUpFrame(frame); err != nil {
			handleCatchUpWriteError(err)
			return
		}
	}
	// Release catch-up admission before the client can observe caught_up and
	// immediately reconnect with its new cursor. The final marker contains no
	// authorization work, and finishCatchUp is idempotent on write failure.
	finishCatchUp()
	if err := writeCatchUpFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_CaughtUp{
		CaughtUp: &realtimev1.RealtimeCaughtUp{Cursor: boundaryCursor},
	}}); err != nil {
		handleCatchUpWriteError(err)
		return
	}
	cancelCatchUp()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
					Close: &realtimev1.RealtimeClose{Code: realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_STREAM_CLOSED, Message: "event stream closed", Reconnect: true, RetryAfter: durationpb.New(time.Second)},
				}})
				return
			}
			if event.DeliverySeq() > 0 && event.DeliverySeq() <= boundarySequence {
				continue
			}
			frame, mapErr := s.realtimeServerFrameForEvent(ctx, user.Id, event)
			if mapErr != nil {
				if errors.Is(mapErr, errRealtimeEventOmitted) {
					// The viewer has safely processed this global boundary even
					// though it produced no public frame. Let a later heartbeat
					// move resume past the omitted fact.
					if event.DeliverySeq() > boundarySequence {
						boundarySequence = event.DeliverySeq()
					}
					continue
				}
				s.logger.Warn("Dropping unsupported realtime event", "event_id", event.ID(), "error", mapErr)
				if event.DeliverySeq() > 0 {
					_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
						Close: &realtimev1.RealtimeClose{Code: realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_EVENT_MAPPING_FAILED, Message: "durable realtime event mapping failed", Reconnect: true},
					}})
					return
				}
				continue
			}
			if heartbeat := frame.GetHeartbeat(); heartbeat != nil {
				cursor, cursorErr := s.core.RealtimeCursorForSequence(user.Id, boundarySequence)
				if cursorErr != nil {
					s.logger.Warn("Failed to refresh realtime heartbeat cursor", "error", cursorErr)
					return
				}
				heartbeat.ResumeCursor = &cursor
			}
			if err := writeFrame(frame); err != nil {
				return
			}
			if event.DeliverySeq() > boundarySequence {
				boundarySequence = event.DeliverySeq()
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
	default:
		return "replay_unavailable", "realtime replay is temporarily unavailable"
	}
}

func realtimeErrorCode(code string) realtimev1.RealtimeErrorCode {
	switch code {
	case "bad_hello":
		return realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_BAD_HELLO
	case "unsupported_protocol":
		return realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_UNSUPPORTED_PROTOCOL
	case "temporarily_unavailable":
		return realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_TEMPORARILY_UNAVAILABLE
	case "authentication_required":
		return realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_AUTHENTICATION_REQUIRED
	case "bad_subscribe":
		return realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_BAD_SUBSCRIBE
	case "replay_unavailable":
		return realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_REPLAY_UNAVAILABLE
	case "invalid_cursor":
		return realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_INVALID_CURSOR
	case "cursor_expired":
		return realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_CURSOR_EXPIRED
	case "subscribe_failed":
		return realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_SUBSCRIBE_FAILED
	case "bad_frame":
		return realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_BAD_FRAME
	default:
		return realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_UNSPECIFIED
	}
}

func realtimeCloseCode(code string) realtimev1.RealtimeCloseCode {
	switch code {
	case "catch_up_in_progress":
		return realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_CATCH_UP_IN_PROGRESS
	case "catch_up_rate_limited":
		return realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_CATCH_UP_RATE_LIMITED
	case "catch_up_server_busy":
		return realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_CATCH_UP_SERVER_BUSY
	default:
		return realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_UNSPECIFIED
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
			Code:      realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED,
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
			Code:      realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED,
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
			Code:      realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED,
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
			Code:      realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_SESSION_RENEWAL_REQUIRED,
			Message:   "the browser session is ready for renewal",
			Reconnect: true,
		},
	}})
	closeConnection()
}

// terminateRealtimeForBotAPIKeyInvalidation cancels authorized work before
// writing a terminal frame. The key generation is watched through the durable
// user-auth projection, so revocation reaches sockets on every replica.
func terminateRealtimeForBotAPIKeyInvalidation(
	cancel context.CancelFunc,
	writeFrame func(*realtimev1.RealtimeServerFrame) error,
	closeConnection func(),
) {
	cancel()
	_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
		Close: &realtimev1.RealtimeClose{
			Code:      realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED,
			Message:   "the bot API key is no longer valid",
			Reconnect: false,
		},
	}})
	closeConnection()
}

func (s *HTTPServer) readRealtimeControlFrames(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, writeFrame func(*realtimev1.RealtimeServerFrame) error) {
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
				Error: &realtimev1.RealtimeError{Code: realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_BAD_FRAME, Message: "expected binary protobuf frame", Fatal: true},
			}})
			return
		}
		var frame realtimev1.RealtimeClientFrame
		if err := proto.Unmarshal(data, &frame); err != nil {
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Error{
				Error: &realtimev1.RealtimeError{Code: realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_BAD_FRAME, Message: "invalid protobuf frame", Fatal: true},
			}})
			return
		}
		switch payload := frame.GetFrame().(type) {
		case *realtimev1.RealtimeClientFrame_Ping:
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Pong{
				Pong: &realtimev1.RealtimePong{Nonce: payload.Ping.GetNonce()},
			}})
		default:
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Error{
				Error: &realtimev1.RealtimeError{Code: realtimev1.RealtimeErrorCode_REALTIME_ERROR_CODE_BAD_FRAME, Message: "unexpected control frame", Fatal: true},
			}})
			return
		}
	}
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
		validated, err := s.core.ValidatePublicBearerCredential(ctx, credential.Handle)
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
	if core.IsRBACEvent(event.CanonicalEvent()) {
		// RBAC payloads are internal, but an RBAC fact can invalidate any
		// authorization-dependent resource that the caller retained. Ask the
		// client to reconnect without exposing that fact. Its cursor remains
		// before this durable sequence, so replay selects the authorized fallback.
		return &realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
			Close: &realtimev1.RealtimeClose{
				Code:      realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_PROJECTION_RESET_REQUIRED,
				Message:   "realtime authorization changed",
				Reconnect: true,
			},
		}}, nil
	}
	publicEvent, err := s.publicRealtimeEvent(ctx, viewerID, event)
	if err != nil {
		return nil, err
	}
	return &realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Event{Event: publicEvent}}, nil
}

func (s *HTTPServer) publicRealtimeEvent(ctx context.Context, viewerID string, event core.EventEnvelope) (*realtimev1.RealtimeEvent, error) {
	canonical := event.CanonicalEvent()
	if canonical == nil {
		return nil, fmt.Errorf("unknown event envelope %T", event.Payload())
	}
	if typing := canonical.GetUserTypingSignal(); typing != nil {
		kind, err := s.core.FindRoomKind(ctx, typing.GetRoomId())
		if err != nil {
			return nil, err
		}
		isMember, err := s.core.RoomMembershipExists(ctx, kind, viewerID, typing.GetRoomId())
		if err != nil {
			return nil, err
		}
		if !isMember {
			return nil, core.ErrPermissionDenied
		}
		var canRead bool
		if typing.GetThreadRootEventId() != "" {
			canRead, err = s.core.CanReadThreadMessages(ctx, viewerID, kind, typing.GetRoomId(), typing.GetThreadRootEventId())
		} else {
			canRead, err = s.core.CanReadMessages(ctx, viewerID, kind, typing.GetRoomId())
		}
		if err != nil {
			return nil, err
		}
		if !canRead {
			return nil, core.ErrPermissionDenied
		}
	}
	projected := projectRealtimeEvent(canonical)
	if projected == nil {
		return nil, errRealtimeEventOmitted
	}
	plaintext, err := s.core.ResolveEventPlaintext(ctx, canonical)
	if err != nil {
		return nil, fmt.Errorf("resolve realtime event plaintext: %w", err)
	}
	applyRealtimePlaintext(projected, plaintext)
	if sequence := event.DeliverySeq(); sequence > 0 {
		cursor, err := s.core.RealtimeCursorForSequence(viewerID, sequence)
		if err != nil {
			return nil, err
		}
		projected.ResumeCursor = &cursor
	}
	return projected, nil
}
