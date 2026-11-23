package http_server

import (
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/chattocorp/chatto/cli/internal/authctx"
	"github.com/chattocorp/chatto/cli/internal/connectapi"
	"github.com/chattocorp/chatto/cli/internal/core"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	realtimev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/realtime/v1"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
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

	closePrivilegedModeExpired := func() {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "privileged mode expired"),
			time.Now().Add(time.Second),
		)
		_ = conn.Close()
	}
	var privilegedModeDeadlineTimer *time.Timer
	if credentialOK && time.Now().Before(credential.PrivilegedModeExpiresAt) {
		privilegedModeDeadlineTimer = time.NewTimer(time.Until(credential.PrivilegedModeExpiresAt))
	}
	if privilegedModeDeadlineTimer != nil {
		defer privilegedModeDeadlineTimer.Stop()
		privilegedModeWatcherDone := make(chan struct{})
		go func() {
			defer close(privilegedModeWatcherDone)
			select {
			case <-privilegedModeDeadlineTimer.C:
				terminateRealtimeForPrivilegedModeExpiry(cancel, writeFrame, closePrivilegedModeExpired)
			case <-ctx.Done():
			}
		}()
		defer func() {
			cancel()
			<-privilegedModeWatcherDone
		}()
	}

	if credentialOK && (credential.Kind == authctx.RuntimeCredentialKindCookieSession || credential.Kind == authctx.RuntimeCredentialKindBearerToken) {
		credentialCheckDone := make(chan struct{})
		accountAccessInvalidated, stopAccountAccessWatch, err := s.watchRealtimeAccountAccess(ctx, credential.UserID)
		if err != nil {
			if errors.Is(err, core.ErrNotAuthenticated) {
				terminateRealtimeForCredentialRevocation(cancel, writeFrame, func() { _ = conn.Close() })
			} else {
				writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, "authentication service temporarily unavailable", true, time.Second)
			}
			return
		}
		defer stopAccountAccessWatch()
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
				case <-accountAccessInvalidated:
					terminateRealtimeForCredentialRevocation(cancel, writeFrame, func() { _ = conn.Close() })
					return
				case <-ticker.C:
					selectionChanged, err := s.revalidateRealtimeCredential(ctx)
					if err == nil {
						if selectionChanged {
							// Another connection ended or changed this session’s activation.
							// Reconnect so fan-out stops using the privileged state.
							terminateRealtimeForPrivilegedModeExpiry(cancel, writeFrame, closePrivilegedModeExpired)
							return
						}
						continue
					}
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

	selectionChanged, err := s.revalidateRealtimeCredential(ctx)
	if err != nil {
		if errors.Is(err, core.ErrNotAuthenticated) {
			writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED, "authentication required", false, 0)
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authentication required"), time.Now().Add(time.Second))
			return
		}
		writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, "authentication service temporarily unavailable", true, time.Second)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "temporarily unavailable"), time.Now().Add(time.Second))
		return
	}
	if selectionChanged {
		terminateRealtimeForPrivilegedModeExpiry(cancel, writeFrame, closePrivilegedModeExpired)
		return
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return
	}
	resumeCursor := strings.TrimSpace(subscribe.GetResumeCursor())
	rejectCatchUp := func(admissionErr *realtimeCatchUpAdmissionError) {
		s.metrics.realtimeCatchUpRejected(admissionErr.code)
		writeClose(realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, "realtime catch-up capacity is temporarily unavailable", true, admissionErr.retryAfter)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, admissionErr.code), time.Now().Add(time.Second))
	}
	// Every catch-up uses the general budget and concurrency limits. The plan
	// charges the additional replay budget at its exact boundary, before scanning.
	releaseCatchUp, admissionErr := s.realtimeCatchUps.acquire(user.Id)
	if admissionErr != nil {
		rejectCatchUp(admissionErr)
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
			s.logger.Warn("Realtime catch-up timed out", "error", connectapi.LogSafeError(err))
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
				Close: &realtimev1.RealtimeClose{Code: realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, Message: "realtime catch-up exceeded its time budget", Reconnect: true, RetryAfter: durationpb.New(time.Second)},
			}})
			return
		}
		s.logger.Warn(logMessage, "error", connectapi.LogSafeError(err))
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
	replayPlan, err := s.core.PlanRealtimeReplay(catchUpCtx, user.Id, resumeCursor, func() error {
		if admissionErr := s.realtimeCatchUps.consumeReplayToken(user.Id); admissionErr != nil {
			return admissionErr
		}
		return nil
	})
	if err != nil {
		if errors.As(err, &admissionErr) {
			rejectCatchUp(admissionErr)
			return
		}
		if errors.Is(catchUpCtx.Err(), context.DeadlineExceeded) {
			failCatchUp("Realtime replay planning timed out", err)
			return
		}
		code, message := realtimeReplayClose(err)
		writeClose(code, message, code == realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, time.Second)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "replay unavailable"), time.Now().Add(time.Second))
		return
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
	recovery := realtimev1.RealtimeRecovery_REALTIME_RECOVERY_RESUMED
	if replayPlan.Reset {
		recovery = realtimev1.RealtimeRecovery_REALTIME_RECOVERY_LIVE_ONLY
		if snapshotFrame != nil {
			recovery = realtimev1.RealtimeRecovery_REALTIME_RECOVERY_SNAPSHOT
		}
	}
	// Release catch-up admission before the client can observe caught_up and
	// immediately reconnect with its new cursor. The final marker contains no
	// authorization work, and finishCatchUp is idempotent on write failure.
	finishCatchUp()
	if err := writeCatchUpFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_CaughtUp{
		CaughtUp: &realtimev1.RealtimeCaughtUp{Cursor: boundaryCursor, Recovery: recovery},
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
				closeRealtimeForEventStream(ctx, writeFrame)
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
			if privilegedModeDeadlineTimer != nil && !time.Now().Before(credential.PrivilegedModeExpiresAt) {
				// Fan-out evaluated this session with privileged mode active. Never
				// write such an event after the deadline; the deadline watcher sends
				// the reconnecting close frame and cancels ctx.
				<-ctx.Done()
				return
			}
			if heartbeat := frame.GetHeartbeat(); heartbeat != nil {
				cursor, cursorErr := s.core.RealtimeCursorForSequence(user.Id, boundarySequence)
				if cursorErr != nil {
					s.logger.Warn("Failed to refresh realtime heartbeat cursor", "error", cursorErr)
					return
				}
				heartbeat.Cursor = &cursor
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

// closeRealtimeForEventStream reports an unexpected stream failure. Credential
// watchers cancel the context before writing their specific close frame. That
// cancellation also closes the event stream, and select can choose either ready
// channel. Leave the close frame to the watcher when cancellation caused it.
func closeRealtimeForEventStream(ctx context.Context, writeFrame func(*realtimev1.RealtimeServerFrame) error) {
	if ctx.Err() != nil {
		return
	}
	_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
		Close: &realtimev1.RealtimeClose{Code: realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_TEMPORARILY_UNAVAILABLE, Message: "event stream closed", Reconnect: true, RetryAfter: durationpb.New(time.Second)},
	}})
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

// terminateRealtimeForPrivilegedModeExpiry reconnects the projection at the
// exact server-side privilege deadline. The replacement connection receives
// viewer and room permissions with privileged mode inactive.
func terminateRealtimeForPrivilegedModeExpiry(
	cancel context.CancelFunc,
	writeFrame func(*realtimev1.RealtimeServerFrame) error,
	closeConnection func(),
) {
	cancel()
	_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
		Close: &realtimev1.RealtimeClose{
			Code:      realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_PRIVILEGED_MODE_EXPIRED,
			Message:   "privileged mode expired",
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

func (s *HTTPServer) realtimeAuthenticatedUser(ctx context.Context, subscribe *realtimev1.RealtimeSubscribe) (context.Context, *readmodel.User, error) {
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

// watchRealtimeAccountAccess registers before validating the credential again.
// A completed deactivate/reactivate cycle before registration must still reject
// the old credential; a transition after registration closes the watch.
func (s *HTTPServer) watchRealtimeAccountAccess(ctx context.Context, userID string) (<-chan struct{}, func(), error) {
	invalidated, stop := s.core.WatchAccountAccessInvalidated(userID)
	if _, err := s.revalidateRealtimeCredential(ctx); err != nil {
		stop()
		return nil, nil, err
	}
	return invalidated, stop, nil
}

// revalidateRealtimeCredential checks the credential that authorized the socket and
// reports whether the socket must reconnect to discard stale elevated authority.
//
// It always uses the strict auth generation check (ADR-127 section 5). With the
// fast check, a socket on a replica that is behind could stay open for one
// timer interval plus one auth generation check window.
func (s *HTTPServer) revalidateRealtimeCredential(ctx context.Context) (bool, error) {
	credential, ok := authctx.CredentialForContext(ctx)
	if !ok {
		return false, core.ErrNotAuthenticated
	}
	ctx = core.WithStrictAuthGenerationCheck(ctx)
	// An inactive socket has no elevated authority to withdraw. Selections are
	// sorted at activation; the deadline alone can be shared by two activations
	// that both end at session expiry.
	changed := func(deadline time.Time, roles []string) bool {
		return time.Now().Before(credential.PrivilegedModeExpiresAt) &&
			(!credential.PrivilegedModeExpiresAt.Equal(deadline) || !slices.Equal(credential.PrivilegedRoles, roles))
	}
	switch credential.Kind {
	case authctx.RuntimeCredentialKindCookieSession:
		record, err := s.core.ValidateCookieCredential(ctx, credential.Handle)
		if err != nil {
			if errors.Is(err, core.ErrCookieSessionNotFound) {
				return false, core.ErrNotAuthenticated
			}
			return false, err
		}
		if record.GetUserId() != credential.UserID {
			return false, core.ErrNotAuthenticated
		}
		if record.GetPrivilegedModeExpiresAt() == nil {
			return changed(time.Time{}, nil), nil
		}
		return changed(record.GetPrivilegedModeExpiresAt().AsTime(), record.PrivilegedRoles), nil
	case authctx.RuntimeCredentialKindBearerToken:
		validated, err := s.core.ValidatePublicBearerCredential(ctx, credential.Handle)
		if err != nil {
			if errors.Is(err, core.ErrAuthTokenNotFound) {
				return false, core.ErrNotAuthenticated
			}
			return false, err
		}
		if validated.UserID != credential.UserID {
			return false, core.ErrNotAuthenticated
		}
		return changed(validated.PrivilegedModeExpiresAt, validated.PrivilegedRoles), nil
	}
	return false, nil
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
	var projected *realtimev1.RealtimeEvent
	var err error
	if durable.GetMessagePosted() != nil {
		projected, err = s.viewerMessagePostedEvent(ctx, viewerID, event)
	} else {
		projected, err = s.projectViewerRealtimeEvent(ctx, viewerID, durable)
	}
	if err != nil {
		return nil, err
	}
	if pubsub != nil {
		projected = projectRealtimePubSubEvent(pubsub)
	}
	if projected == nil {
		return nil, errRealtimeEventOmitted
	}
	// Use the same retained ownership lookup as asset-event authorization.
	// Unattached or unresolved assets must not cause a global client refresh.
	switch durable.GetEvent().(type) {
	case *evtv1.Event_AssetProcessingStarted, *evtv1.Event_AssetProcessingSucceeded,
		*evtv1.Event_AssetProcessingFailed, *evtv1.Event_AssetDeleted:
		roomID, messageID, ok := s.core.AssetEventTimelineTarget(durable)
		if !ok {
			return nil, errRealtimeEventOmitted
		}
		switch value := projected.GetEvent().(type) {
		case *realtimev1.RealtimeEvent_AssetProcessingStarted:
			value.AssetProcessingStarted.RoomId = roomID
			value.AssetProcessingStarted.MessageEventId = messageID
		case *realtimev1.RealtimeEvent_AssetProcessingSucceeded:
			value.AssetProcessingSucceeded.RoomId = roomID
			value.AssetProcessingSucceeded.MessageEventId = messageID
		case *realtimev1.RealtimeEvent_AssetProcessingFailed:
			value.AssetProcessingFailed.RoomId = roomID
			value.AssetProcessingFailed.MessageEventId = messageID
		case *realtimev1.RealtimeEvent_AssetDeleted:
			value.AssetDeleted.RoomId = roomID
			value.AssetDeleted.MessageEventId = messageID
		}
	}
	if sequence := event.DeliverySeq(); sequence > 0 {
		cursor, err := s.core.RealtimeCursorForSequence(viewerID, sequence)
		if err != nil {
			return nil, err
		}
		projected.Cursor = &cursor
	}
	return projected, nil
}

// sharedMessagePost is the viewer-independent public mapping of one message
// post. It is computed once per envelope and shared by all connections that
// get the envelope. Do not change it or the values it points to after it is
// stored.
type sharedMessagePost struct {
	// event is the complete public event without per-viewer values: it has no
	// resume cursor, and its mentions have includes_viewer set to false.
	event *realtimev1.RealtimeEvent
	// mentions are the hydrated stored mention rows. Each viewer gets its own
	// public mentions from these rows.
	mentions []*evtv1.MessageMention
	// kind and body are the room kind and the decrypted body that the shared
	// event uses. A per-viewer step reads them instead of looking them up
	// again. body is nil when the message has no readable body.
	kind core.RoomKind
	body *core.DecryptedMessageBody
	// bodyVersion is the body version read before the body lookup. A viewer
	// whose mapping runs after an edit, a retraction, or a key-shredding
	// request does not use the shared body.
	bodyVersion core.MessageBodyVersion
}

// viewerMessagePostedEvent returns the public message-post event for one
// viewer. The hub already authorized the viewer for the event. The body
// lookup, echo hydration, and room-kind lookup do not depend on the viewer,
// so they run once per envelope. The viewer gets a copy of the shared event.
// Set each per-viewer value of the payload on that copy here, as for the
// mention flags. The caller adds the resume cursor.
func (s *HTTPServer) viewerMessagePostedEvent(ctx context.Context, viewerID string, event core.EventEnvelope) (*realtimev1.RealtimeEvent, error) {
	value, err := event.SharedRealtimeMapping(func() (any, error) {
		return s.mapSharedMessagePost(ctx, event.EVTEvent())
	})
	if err != nil {
		return nil, err
	}
	shared := value.(*sharedMessagePost)
	if shared != nil && s.core.CurrentMessageBodyVersion(event.EVTEvent().GetId()) != shared.bodyVersion {
		// The body changed after the shared mapping. Map this viewer again,
		// so that it never gets a body that was retracted or shredded.
		if shared, err = s.mapSharedMessagePost(ctx, event.EVTEvent()); err != nil {
			return nil, err
		}
	}
	if shared == nil {
		return nil, nil
	}
	projected := proto.CloneOf(shared.event)
	// Per-viewer values. They must never be set on shared.event.
	projected.GetMessagePosted().Mentions = realtimeMentions(viewerID, shared.mentions)
	return projected, nil
}

// mapSharedMessagePost maps a message post without a viewer. Put each new
// viewer-independent field of the public message post here. A field that
// depends on the viewer or on the viewer's session, or that is derived from
// such a value, must not go into the shared event; set it in
// viewerMessagePostedEvent.
func (s *HTTPServer) mapSharedMessagePost(ctx context.Context, durable *evtv1.Event) (*sharedMessagePost, error) {
	projected := projectRealtimeEvent("", durable)
	if projected == nil {
		return nil, nil
	}
	post, err := s.core.HydrateMessagePost(ctx, durable)
	if err != nil {
		return nil, err
	}
	message := projected.GetMessagePosted()
	message.InReplyTo = post.GetInReplyTo()
	message.Mentions = realtimeMentions("", post.GetMentions())
	bodyVersion := s.core.CurrentMessageBodyVersion(durable.GetId())
	body, err := s.core.GetFullMessageBody(ctx, durable.GetId())
	if err != nil {
		return nil, fmt.Errorf("resolve realtime event plaintext: %w", err)
	}
	kind, err := s.core.FindRoomKind(ctx, durable.GetMessagePosted().GetRoomId())
	if err != nil {
		return nil, fmt.Errorf("resolve realtime room kind: %w", err)
	}
	message.RoomKind = realtimeRoomKind(core.ProtoKindForRoomKind(kind))
	if body != nil {
		message.BodyPlaintext = new(body.Body)
	}
	return &sharedMessagePost{event: projected, mentions: post.GetMentions(), kind: kind, body: body, bodyVersion: bodyVersion}, nil
}

// projectViewerRealtimeEvent maps one durable fact to its public semantic
// event. Viewer-specific cases are handled here because one stored fact can
// invalidate a private viewer resource, a public user resource, or neither.
func (s *HTTPServer) projectViewerRealtimeEvent(ctx context.Context, viewerID string, event *evtv1.Event) (*realtimev1.RealtimeEvent, error) {
	if event == nil || event.GetEvent() == nil {
		return nil, nil
	}
	// Bots have an additional authority boundary: their human owner's current
	// permissions. That dependency is private and cannot be inferred by a
	// public client from the bot's own role list.
	if core.IsRBACEvent(event) {
		viewer, err := s.core.GetUser(ctx, viewerID)
		if err != nil && !errors.Is(err, core.ErrNotFound) {
			return nil, err
		}
		if viewer.GetIsBot() && rbacMayChangeBotAuthority(event, viewer.GetBotOwnerUserId(), viewerID) {
			return &realtimev1.RealtimeEvent{
				Id: event.GetId(), CreatedAt: event.GetCreatedAt(),
				Event: &realtimev1.RealtimeEvent_ViewerPermissionsChanged{ViewerPermissionsChanged: &realtimev1.ViewerPermissionsChangedEvent{}},
			}, nil
		}
	}
	base := func() *realtimev1.RealtimeEvent {
		result := &realtimev1.RealtimeEvent{Id: event.GetId(), CreatedAt: event.GetCreatedAt()}
		if event.GetActorId() != "" {
			result.ActorId = new(event.GetActorId())
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
		result.ActorId = new(userID)
		result.Event = &realtimev1.RealtimeEvent_UserJoinedRoom{UserJoinedRoom: &realtimev1.UserJoinedRoomEvent{RoomId: roomID}}
		return result, nil
	case *evtv1.Event_ThreadFollowed:
		if viewerID != payload.ThreadFollowed.GetUserId() {
			return nil, nil
		}
		return projectRealtimeEvent(viewerID, event), nil
	case *evtv1.Event_ThreadUnfollowed:
		if viewerID != payload.ThreadUnfollowed.GetUserId() {
			return nil, nil
		}
		return projectRealtimeEvent(viewerID, event), nil
	default:
		return projectRealtimeEvent(viewerID, event), nil
	}
}
