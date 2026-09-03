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
	writeClose := func(code realtimev1.RealtimeCloseCode, message string, reconnect bool, retryAfter time.Duration) {
		close := &realtimev1.RealtimeClose{Code: code, Message: message, Reconnect: reconnect}
		if retryAfter > 0 {
			close.RetryAfter = durationpb.New(retryAfter)
		}
		_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
			Close: close,
		}})
	}
	subscribe, err := readRealtimeSubscribe(conn, realtimeHandshakeTimeout)
	if err != nil {
		writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_INVALID_REQUEST, "expected a binary protobuf subscription", false, 0)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "invalid subscription"), time.Now().Add(time.Second))
		return
	}
	if subscribe.GetProtocolVersion() != realtimeProtocolVersion {
		writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_UNSUPPORTED_PROTOCOL, "unsupported realtime protocol version", false, 0)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "unsupported protocol"), time.Now().Add(time.Second))
		return
	}
	if subscribe.GetInitialState() == realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_UNSPECIFIED ||
		(subscribe.GetInitialState() != realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY &&
			subscribe.GetInitialState() != realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT) {
		writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_INVALID_REQUEST, "initial_state is required", false, 0)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "invalid subscription"), time.Now().Add(time.Second))
		return
	}
	ctx, user, err := s.realtimeAuthenticatedUser(ctx, subscribe)
	if err != nil {
		if !errors.Is(err, core.ErrNotAuthenticated) {
			writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, "authentication service temporarily unavailable", true, time.Second)
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "temporarily unavailable"), time.Now().Add(time.Second))
			return
		}
		writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED, "authentication required", false, 0)
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

	if err := s.revalidateRealtimeCredential(ctx); err != nil {
		if errors.Is(err, core.ErrNotAuthenticated) {
			writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED, "authentication required", false, 0)
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authentication required"), time.Now().Add(time.Second))
			return
		}
		writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, "authentication service temporarily unavailable", true, time.Second)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "temporarily unavailable"), time.Now().Add(time.Second))
		return
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return
	}
	resumeCursor := strings.TrimSpace(subscribe.GetResumeCursor())
	cursorAtBoundary, err := s.core.RealtimeCursorAtCurrentBoundary(ctx, user.Id, resumeCursor)
	if err != nil {
		writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, "realtime replay is temporarily unavailable", true, time.Second)
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
			Close: &realtimev1.RealtimeClose{Code: realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, Message: "realtime catch-up capacity is temporarily unavailable", Reconnect: true, RetryAfter: durationpb.New(admissionErr.retryAfter)},
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
				Close: &realtimev1.RealtimeClose{Code: realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, Message: "realtime catch-up exceeded its time budget", Reconnect: true, RetryAfter: durationpb.New(time.Second)},
			}})
			return
		}
		s.logger.Warn(logMessage, "error", err)
		writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, "realtime recovery is temporarily unavailable", true, time.Second)
	}
	handleCatchUpWriteError := func(err error) {
		if errors.Is(catchUpCtx.Err(), context.DeadlineExceeded) {
			failCatchUp("Realtime catch-up delivery timed out", err)
		}
	}

	events, err := s.core.StreamMyEventsWithOptions(ctx, user.Id, core.StreamMyEventsOptions{TouchPresence: false})
	if err != nil {
		writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, "failed to start realtime event stream", true, time.Second)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "subscribe failed"), time.Now().Add(time.Second))
		return
	}
	replayPlan, err := s.core.PlanRealtimeReplay(catchUpCtx, user.Id, subscribe.GetResumeCursor())
	if err != nil {
		if errors.Is(catchUpCtx.Err(), context.DeadlineExceeded) {
			failCatchUp("Realtime replay planning timed out", err)
			return
		}
		code, message := realtimeReplayClose(err)
		writeClose(code, message, code == realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, time.Second)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "replay unavailable"), time.Now().Add(time.Second))
		return
	}
	if resumeCursor != "" && !meteredReplay && replayPlan.HadSequenceGap {
		// EVT advanced after the current-boundary check. Charge the newly-real
		// replay gap before emitting subscribed, snapshot, or event frames.
		if chargeErr := s.realtimeCatchUps.consumeReplayToken(user.Id); chargeErr != nil {
			s.metrics.realtimeCatchUpRejected(chargeErr.code)
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
				Close: &realtimev1.RealtimeClose{Code: realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, Message: "realtime catch-up capacity is temporarily unavailable", Reconnect: true, RetryAfter: durationpb.New(chargeErr.retryAfter)},
			}})
			return
		}
	}

	boundaryCursor := replayPlan.BoundaryCursor
	boundarySequence := replayPlan.BoundarySequence
	var snapshotFrame *realtimev1.RealtimeServerFrame
	if replayPlan.Reset {
		if subscribe.GetInitialState() == realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT {
			var snapshotErr error
			boundarySequence, snapshotFrame, snapshotErr = s.realtimeSnapshotFrame(catchUpCtx, user.Id)
			if snapshotErr != nil {
				failCatchUp("Realtime snapshot capture failed", snapshotErr)
				return
			}
			boundaryCursor, snapshotErr = s.core.RealtimeCursorForSequence(user.Id, boundarySequence)
			if snapshotErr != nil {
				failCatchUp("Realtime snapshot cursor creation failed", snapshotErr)
				return
			}
		}
	}
	go s.watchRealtimeClient(ctx, cancel, conn, writeFrame)
	if snapshotFrame != nil {
		if err := writeCatchUpFrame(snapshotFrame); err != nil {
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
					Close: &realtimev1.RealtimeClose{Code: realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, Message: "event stream closed", Reconnect: true, RetryAfter: durationpb.New(time.Second)},
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
						Close: &realtimev1.RealtimeClose{Code: realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, Message: "durable realtime event mapping failed", Reconnect: true},
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

func realtimeReplayClose(err error) (code realtimev1.RealtimeCloseCode, message string) {
	switch {
	case errors.Is(err, core.ErrRealtimeCursorInvalid):
		return realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_INVALID_REQUEST, "the realtime resume cursor is invalid for this server history"
	case errors.Is(err, core.ErrRealtimeCursorExpired):
		return realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_INVALID_REQUEST, "the realtime resume cursor is no longer retained"
	default:
		return realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, "realtime replay is temporarily unavailable"
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

func readRealtimeSubscribe(conn *websocket.Conn, timeout time.Duration) (*realtimev1.RealtimeSubscribe, error) {
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
	var subscribe realtimev1.RealtimeSubscribe
	if err := proto.Unmarshal(data, &subscribe); err != nil {
		return nil, err
	}
	return &subscribe, nil
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

// watchRealtimeClient owns the WebSocket read loop after subscription. The
// protocol is server-streaming, so any later application message is invalid.
func (s *HTTPServer) watchRealtimeClient(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, writeFrame func(*realtimev1.RealtimeServerFrame) error) {
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_, _, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
			Close: &realtimev1.RealtimeClose{
				Code:      realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_INVALID_REQUEST,
				Message:   "unexpected client message after subscription",
				Reconnect: false,
			},
		}})
		return
	}
}

func (s *HTTPServer) realtimeAuthenticatedUser(ctx context.Context, subscribe *realtimev1.RealtimeSubscribe) (context.Context, *evtv1.User, error) {
	if token := strings.TrimSpace(subscribe.GetBearerToken()); token != "" {
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
	if event.Heartbeat() {
		return &realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Heartbeat{
			Heartbeat: &realtimev1.RealtimeHeartbeat{},
		}}, nil
	}
	if terminated := core.EventSessionTerminated(event); terminated != nil {
		return &realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
			Close: &realtimev1.RealtimeClose{
				Code:      realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_SESSION_TERMINATED,
				Message:   "session terminated: " + terminated.GetReason(),
				Reconnect: false,
			},
		}}, nil
	}
	if core.IsRBACEvent(event.EVTEvent()) {
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
	durable := event.EVTEvent()
	pubsub := event.PubSubEvent()
	if durable == nil && pubsub == nil {
		return nil, fmt.Errorf("unknown event envelope %T", event.Payload())
	}
	if typing := pubsub.GetUserTyping(); typing != nil {
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
	projected, err := s.projectViewerRealtimeEvent(ctx, viewerID, durable)
	if err != nil {
		return nil, err
	}
	if pubsub != nil {
		projected = projectRealtimePubSubEvent(pubsub)
	}
	if projected == nil {
		return nil, errRealtimeEventOmitted
	}
	visible, err := s.filterRealtimeEventFields(ctx, viewerID, projected)
	if err != nil {
		return nil, fmt.Errorf("authorize realtime event fields: %w", err)
	}
	if !visible {
		return nil, errRealtimeEventOmitted
	}
	if durable != nil && durable.GetMessagePosted() != nil {
		plaintext, err := s.core.ResolveEventPlaintext(ctx, durable)
		if err != nil {
			return nil, fmt.Errorf("resolve realtime event plaintext: %w", err)
		}
		applyRealtimePlaintext(projected, plaintext)
	}
	if sequence := event.DeliverySeq(); sequence > 0 {
		cursor, err := s.core.RealtimeCursorForSequence(viewerID, sequence)
		if err != nil {
			return nil, err
		}
		projected.ResumeCursor = &cursor
	}
	return projected, nil
}

// projectViewerRealtimeEvent maps one durable fact to its public semantic
// event. Viewer-specific cases are handled here because one stored fact can
// invalidate a private viewer resource, a public user resource, or neither.
func (s *HTTPServer) projectViewerRealtimeEvent(ctx context.Context, viewerID string, event *evtv1.Event) (*realtimev1.RealtimeEvent, error) {
	if event == nil || event.GetEvent() == nil {
		return nil, nil
	}
	base := func() *realtimev1.RealtimeEvent {
		result := &realtimev1.RealtimeEvent{Id: event.GetId(), CreatedAt: event.GetCreatedAt()}
		if event.GetActorId() != "" {
			result.ActorId = proto.String(event.GetActorId())
		}
		return result
	}
	viewerPreferences := func() *realtimev1.RealtimeEvent {
		result := base()
		result.Event = &realtimev1.RealtimeEvent_ViewerPreferencesChanged{ViewerPreferencesChanged: &realtimev1.ViewerPreferencesChangedEvent{}}
		return result
	}
	userProfile := func(userID string) *realtimev1.RealtimeEvent {
		result := base()
		result.Event = &realtimev1.RealtimeEvent_UserProfileChanged{UserProfileChanged: &realtimev1.UserProfileChangedEvent{UserId: userID}}
		return result
	}

	switch payload := event.GetEvent().(type) {
	case *evtv1.Event_UserTimezoneChanged:
		userID := payload.UserTimezoneChanged.GetUserId()
		if viewerID == userID {
			return viewerPreferences(), nil
		}
		settings, err := s.core.GetUserSettings(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("read user settings for realtime projection: %w", err)
		}
		if settings != nil && settings.GetShareTimezone() {
			return userProfile(userID), nil
		}
		return nil, nil
	case *evtv1.Event_UserTimezoneCleared:
		userID := payload.UserTimezoneCleared.GetUserId()
		if viewerID == userID {
			return viewerPreferences(), nil
		}
		settings, err := s.core.GetUserSettings(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("read user settings for realtime projection: %w", err)
		}
		if settings != nil && settings.GetShareTimezone() {
			return userProfile(userID), nil
		}
		return nil, nil
	case *evtv1.Event_UserTimezoneSharingChanged:
		userID := payload.UserTimezoneSharingChanged.GetUserId()
		if viewerID == userID {
			return viewerPreferences(), nil
		}
		return userProfile(userID), nil
	case *evtv1.Event_UserTimeFormatChanged:
		if viewerID == payload.UserTimeFormatChanged.GetUserId() {
			return viewerPreferences(), nil
		}
		return nil, nil
	case *evtv1.Event_UserTimeFormatCleared:
		if viewerID == payload.UserTimeFormatCleared.GetUserId() {
			return viewerPreferences(), nil
		}
		return nil, nil
	case *evtv1.Event_UserServerPreferencesChanged:
		userID := payload.UserServerPreferencesChanged.GetUserId()
		if viewerID == userID {
			return viewerPreferences(), nil
		}
		// A historical composite preference fact can either expose or hide the
		// public time zone. Refresh the public user resource in both cases.
		return userProfile(userID), nil
	case *evtv1.Event_RoomMemberUnbanned:
		roomID := payload.RoomMemberUnbanned.GetRoomId()
		userID := payload.RoomMemberUnbanned.GetUserId()
		isMember, err := s.core.RoomMembershipExists(ctx, core.KindChannel, userID, roomID)
		if err != nil {
			return nil, fmt.Errorf("resolve unbanned room membership: %w", err)
		}
		if !isMember {
			return nil, nil
		}
		result := base()
		result.ActorId = proto.String(userID)
		result.Event = &realtimev1.RealtimeEvent_UserJoinedRoom{UserJoinedRoom: &realtimev1.UserJoinedRoomEvent{RoomId: roomID}}
		return result, nil
	case *evtv1.Event_ThreadFollowed:
		if viewerID != payload.ThreadFollowed.GetUserId() {
			return nil, nil
		}
		return projectRealtimeEvent(event), nil
	case *evtv1.Event_ThreadUnfollowed:
		if viewerID != payload.ThreadUnfollowed.GetUserId() {
			return nil, nil
		}
		return projectRealtimeEvent(event), nil
	default:
		return projectRealtimeEvent(event), nil
	}
}

// filterRealtimeEventFields applies viewer-dependent authorization to fields
// in a newly projected public event. It returns false when no authorized
// payload remains. Event-level authorization remains in the event hub.
func (s *HTTPServer) filterRealtimeEventFields(ctx context.Context, viewerID string, event *realtimev1.RealtimeEvent) (bool, error) {
	if event == nil {
		return false, nil
	}
	switch event.GetEvent().(type) {
	case *realtimev1.RealtimeEvent_RoomAddedToGroup,
		*realtimev1.RealtimeEvent_RoomRemovedFromGroup,
		*realtimev1.RealtimeEvent_RoomsInGroupReordered,
		*realtimev1.RealtimeEvent_SidebarGroupEntriesReordered:
		// These payloads can contain room IDs that the source event knows
		// about but the current viewer cannot see. Use the same visibility
		// source as the room and room-group resource APIs.
	default:
		return true, nil
	}

	rooms, err := s.core.RoomDirectoryReads().ListRooms(ctx, viewerID, core.RoomDirectoryListOptions{
		IncludeChannels: true,
	})
	if err != nil {
		return false, err
	}
	visibleRoomIDs := make(map[string]struct{}, len(rooms))
	for _, room := range rooms {
		if room != nil && room.Room != nil && room.Room.GetId() != "" {
			visibleRoomIDs[room.Room.GetId()] = struct{}{}
		}
	}
	isVisible := func(roomID string) bool {
		_, ok := visibleRoomIDs[roomID]
		return ok
	}

	switch payload := event.GetEvent().(type) {
	case *realtimev1.RealtimeEvent_RoomAddedToGroup:
		return isVisible(payload.RoomAddedToGroup.GetRoomId()), nil
	case *realtimev1.RealtimeEvent_RoomRemovedFromGroup:
		return isVisible(payload.RoomRemovedFromGroup.GetRoomId()), nil
	case *realtimev1.RealtimeEvent_RoomsInGroupReordered:
		roomIDs := payload.RoomsInGroupReordered.GetRoomIds()
		visible := make([]string, 0, len(roomIDs))
		for _, roomID := range roomIDs {
			if isVisible(roomID) {
				visible = append(visible, roomID)
			}
		}
		payload.RoomsInGroupReordered.RoomIds = visible
	case *realtimev1.RealtimeEvent_SidebarGroupEntriesReordered:
		entries := payload.SidebarGroupEntriesReordered.GetEntries()
		visible := make([]*realtimev1.SidebarGroupEntryReference, 0, len(entries))
		for _, entry := range entries {
			if entry.GetKind() == realtimev1.SidebarGroupEntryKind_SIDEBAR_GROUP_ENTRY_KIND_ROOM && !isVisible(entry.GetId()) {
				continue
			}
			visible = append(visible, entry)
		}
		payload.SidebarGroupEntriesReordered.Entries = visible
	}
	return true, nil
}
