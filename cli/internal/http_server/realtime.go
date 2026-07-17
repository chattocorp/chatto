package http_server

import (
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

const (
	realtimePath                     = "/api/realtime"
	realtimeProtocolVersion          = 2
	realtimeReadLimitBytes           = 64 << 10
	realtimeReadBufferBytes          = 256
	realtimeWriteBufferBytes         = 512
	realtimeCompressionMinBytes      = 1024
	realtimeHandshakeTimeout         = 10 * time.Second
	realtimeWriteTimeout             = 10 * time.Second
	realtimeHeartbeatIntervalSeconds = uint32(core.MyEventsHeartbeatInterval / time.Second)
)

var realtimeServerCapabilities = []string{
	"chatto.realtime.events.live.v1",
	"chatto.realtime.heartbeat.v1",
	"chatto.realtime.ping.v1",
	"chatto.realtime.events.resume.v1",
	"chatto.realtime.projection.v1",
}

func (s *HTTPServer) setupRealtimeAPI(allowedOrigins []string) {
	if s.metrics == nil {
		s.metrics = newProcessMetrics()
	}

	writeBufferPool := &sync.Pool{}
	upgrader := websocket.Upgrader{
		ReadBufferSize:    realtimeReadBufferBytes,
		WriteBufferSize:   realtimeWriteBufferBytes,
		WriteBufferPool:   writeBufferPool,
		EnableCompression: s.config.Webserver.WebSocketCompressionEnabled(),
		CheckOrigin: func(r *http.Request) bool {
			return s.checkRealtimeWebSocketOrigin(r, allowedOrigins)
		},
	}

	s.router.GET(realtimePath, func(c *gin.Context) {
		req := s.injectUserIntoContext(c)
		req = req.WithContext(connectapi.WithRequestBaseURL(req.Context(), s.requestBaseURL(req)))
		conn, err := upgrader.Upgrade(c.Writer, req, nil)
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

func (s *HTTPServer) checkRealtimeWebSocketOrigin(r *http.Request, allowedOrigins []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if s.matchOrigin(origin, allowedOrigins) != originNotAllowed {
		return true
	}
	host := r.Host
	if s.trustedProxies.containsRemoteAddr(r.RemoteAddr) {
		if forwarded := forwardedHost(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
			host = forwarded
		}
	}
	if parsedOrigin, err := url.Parse(origin); err == nil {
		if strings.EqualFold(parsedOrigin.Host, host) {
			return true
		}
	}
	s.logger.Warn("Realtime WebSocket connection rejected: origin mismatch",
		"origin", origin, "host", host, "allowed", allowedOrigins)
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
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return
	}

	events, err := s.core.StreamMyEventsWithOptions(ctx, user.Id, core.StreamMyEventsOptions{TouchPresence: false})
	if err != nil {
		writeError("subscribe_failed", "failed to start realtime event stream", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "subscribe failed"), time.Now().Add(time.Second))
		return
	}
	replayPlan, err := s.core.PlanRealtimeReplay(ctx, user.Id, subscribeEvents.GetResumeCursor())
	if err != nil {
		code, message := realtimeReplayError(err)
		writeError(code, message, true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, code), time.Now().Add(time.Second))
		return
	}

	subscribed := &realtimev1.RealtimeSubscribed{StartCursor: &replayPlan.StartCursor}
	if err := writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Subscribed{
		Subscribed: subscribed,
	}}); err != nil {
		return
	}

	go s.readRealtimeControlFrames(ctx, cancel, conn, writeFrame)
	if replayPlan.Reset {
		frames, err := s.realtimeProjectionSnapshotFrames(ctx, user.Id)
		if err != nil {
			s.logger.Warn("Realtime compacted projection replay failed", "error", err)
			writeError("replay_unavailable", "realtime projection replay is temporarily unavailable", true)
			return
		}
		for _, frame := range frames {
			if err := writeFrame(frame); err != nil {
				return
			}
		}
	}
	for _, event := range replayPlan.Events {
		frame, handled, err := s.realtimeProjectionFrameForEvent(ctx, user.Id, event)
		if err != nil {
			s.logger.Warn("Realtime replay mapping failed", "event_id", event.ID(), "error", err)
			writeError("replay_unavailable", "realtime replay is temporarily unavailable", true)
			return
		}
		if !handled {
			s.logger.Warn("Realtime durable event has no projection mapping", "event_id", event.ID())
			writeError("replay_unavailable", "realtime replay is temporarily unavailable", true)
			return
		}
		if err := writeFrame(frame); err != nil {
			return
		}
	}
	if !replayPlan.Reset {
		notifications, err := s.connectAPI.BuildRealtimeProjectionNotifications(ctx, user.Id)
		if err != nil {
			s.logger.Warn("Realtime notification reconciliation failed", "error", err)
			writeError("replay_unavailable", "realtime notification reconciliation is temporarily unavailable", true)
			return
		}
		if err := writeFrame(realtimeProjectionServerFrame(&realtimev1.RealtimeProjectionEvent{
			Id:        core.NewEventID(),
			CreatedAt: timestamppb.Now(),
			Operations: []*realtimev1.RealtimeProjectionOperation{{Operation: &realtimev1.RealtimeProjectionOperation_NotificationsReplace{
				NotificationsReplace: realtimeProjectionNotifications(notifications),
			}}},
		})); err != nil {
			return
		}
	}
	if err := writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_CaughtUp{
		CaughtUp: &realtimev1.RealtimeCaughtUp{Cursor: replayPlan.BoundaryCursor},
	}}); err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
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
			frame, handled, mapErr = s.realtimeProjectionFrameForEvent(ctx, user.Id, event)
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
		default:
			_ = writeFrame(&realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Error{
				Error: &realtimev1.RealtimeError{Code: "bad_frame", Message: "unexpected control frame", Fatal: true},
			}})
			return
		}
	}
}

func (s *HTTPServer) realtimeAuthenticatedUser(ctx context.Context, hello *realtimev1.RealtimeClientHello) (context.Context, *corev1.User, error) {
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

func (s *HTTPServer) mapRealtimeLive(ctx context.Context, viewerID string, envelope *realtimev1.RealtimeEventEnvelope, event *corev1.LiveEvent) error {
	switch payload := event.GetEvent().(type) {
	case *corev1.LiveEvent_UserTyping:
		typing := payload.UserTyping
		envelope.Event = &realtimev1.RealtimeEventEnvelope_UserTyping{UserTyping: &realtimev1.RealtimeTypingEvent{
			RoomId: typing.GetRoomId(), ThreadRootEventId: optionalRealtimeString(typing.GetThreadRootEventId()),
		}}
	case *corev1.LiveEvent_PresenceChanged:
		envelope.Event = &realtimev1.RealtimeEventEnvelope_PresenceChanged{PresenceChanged: &realtimev1.RealtimePresenceChangedEvent{
			UserId: event.GetActorId(), Status: apiPresenceStatus(payload.PresenceChanged.GetStatus()),
		}}
	case *corev1.LiveEvent_NotificationCreated:
		notification := payload.NotificationCreated
		envelope.Event = &realtimev1.RealtimeEventEnvelope_NotificationCreated{NotificationCreated: &realtimev1.RealtimeNotificationCreatedEvent{
			NotificationId: notification.GetNotificationId(),
			RoomId:         optionalRealtimeString(notification.GetRoomId()),
			EventId:        optionalRealtimeString(notification.GetEventId()),
			InReplyToId:    optionalRealtimeString(notification.GetInReplyToId()),
			Silent:         notification.GetSilent(),
		}}
	case *corev1.LiveEvent_NotificationDismissed:
		envelope.Event = &realtimev1.RealtimeEventEnvelope_NotificationDismissed{NotificationDismissed: &realtimev1.RealtimeNotificationDismissedEvent{
			NotificationId: payload.NotificationDismissed.GetNotificationId(),
		}}
	case *corev1.LiveEvent_NotificationLevelChanged:
		level := payload.NotificationLevelChanged
		envelope.Event = &realtimev1.RealtimeEventEnvelope_NotificationLevelChanged{NotificationLevelChanged: &realtimev1.RealtimeNotificationLevelChangedEvent{
			RoomId: level.GetRoomId(), Level: apiNotificationLevel(level.GetLevel()), EffectiveLevel: apiNotificationLevel(level.GetEffectiveLevel()),
		}}
	case *corev1.LiveEvent_ServerUserPreferencesUpdated:
		prefs := payload.ServerUserPreferencesUpdated
		envelope.Event = &realtimev1.RealtimeEventEnvelope_ServerUserPreferencesUpdated{ServerUserPreferencesUpdated: &realtimev1.RealtimeServerUserPreferencesUpdatedEvent{
			Timezone: optionalRealtimeString(prefs.GetTimezone()), TimeFormat: apiRealtimeTimeFormat(prefs.GetTimeFormat()),
		}}
	case *corev1.LiveEvent_ThreadFollowChanged:
		follow := payload.ThreadFollowChanged
		envelope.Event = &realtimev1.RealtimeEventEnvelope_ThreadFollowChanged{ThreadFollowChanged: &realtimev1.RealtimeThreadFollowChangedEvent{
			RoomId: follow.GetRoomId(), ThreadRootEventId: follow.GetThreadRootEventId(), Following: follow.GetIsFollowing(),
		}}
	case *corev1.LiveEvent_MentionNotification:
		mention := payload.MentionNotification
		envelope.Event = &realtimev1.RealtimeEventEnvelope_MentionNotification{MentionNotification: s.realtimeMentionNotification(ctx, viewerID, mention)}
	case *corev1.LiveEvent_NewDirectMessageNotification:
		dm := payload.NewDirectMessageNotification
		envelope.Event = &realtimev1.RealtimeEventEnvelope_NewDirectMessageNotification{NewDirectMessageNotification: s.realtimeNewDirectMessageNotification(ctx, viewerID, dm)}
	case *corev1.LiveEvent_RoomMarkedAsRead:
		envelope.Event = &realtimev1.RealtimeEventEnvelope_RoomMarkedAsRead{RoomMarkedAsRead: &realtimev1.RealtimeRoomMarkedAsReadEvent{
			RoomId: payload.RoomMarkedAsRead.GetRoomId(),
		}}
	case *corev1.LiveEvent_RoomGroupsUpdated:
		envelope.Event = &realtimev1.RealtimeEventEnvelope_RoomGroupsUpdated{RoomGroupsUpdated: &realtimev1.RealtimeRoomGroupsUpdatedEvent{
			Changed: true,
		}}
	case *corev1.LiveEvent_ServerMemberDeleted:
		envelope.Event = &realtimev1.RealtimeEventEnvelope_ServerMemberDeleted{ServerMemberDeleted: &realtimev1.RealtimeServerMemberDeletedEvent{
			UserId: payload.ServerMemberDeleted.GetUserId(),
		}}
	case *corev1.LiveEvent_ServerUpdated:
		server := payload.ServerUpdated
		envelope.Event = &realtimev1.RealtimeEventEnvelope_ServerUpdated{ServerUpdated: &realtimev1.RealtimeServerUpdatedEvent{
			Name: server.GetName(), Description: server.GetDescription(), LogoUrl: optionalRealtimeString(server.GetLogoUrl()), BannerUrl: optionalRealtimeString(server.GetBannerUrl()),
		}}
	case *corev1.LiveEvent_UserProfileUpdated:
		user := payload.UserProfileUpdated
		envelope.Event = &realtimev1.RealtimeEventEnvelope_UserProfileUpdated{UserProfileUpdated: &realtimev1.RealtimeUserProfileUpdatedEvent{
			UserId: user.GetUserId(), Login: user.GetLogin(), DisplayName: user.GetDisplayName(), AvatarUrl: optionalRealtimeString(user.GetAvatarUrl()),
		}}
	case *corev1.LiveEvent_SessionTerminated:
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

func (s *HTTPServer) realtimeMentionNotification(ctx context.Context, viewerID string, mention *corev1.MentionNotificationEvent) *realtimev1.RealtimeMentionNotificationEvent {
	out := &realtimev1.RealtimeMentionNotificationEvent{
		RoomId:      mention.GetRoomId(),
		ActorUserId: mention.GetMentionedByUserId(),
	}
	if s == nil || s.core == nil {
		return out
	}
	if room, err := s.core.FindRoomByID(ctx, mention.GetRoomId()); err == nil && s.viewerCanReadRealtimeRoomLabel(ctx, viewerID, room) {
		out.RoomName = proto.String(room.GetName())
	}
	if actor, err := s.core.GetUser(ctx, mention.GetMentionedByUserId()); err == nil {
		out.ActorDisplayName = proto.String(actor.GetDisplayName())
	}
	return out
}

func (s *HTTPServer) realtimeNewDirectMessageNotification(ctx context.Context, viewerID string, dm *corev1.NewDirectMessageNotificationEvent) *realtimev1.RealtimeNewDirectMessageNotificationEvent {
	out := &realtimev1.RealtimeNewDirectMessageNotificationEvent{
		RoomId:   dm.GetRoomId(),
		SenderId: dm.GetSenderId(),
	}
	if s == nil || s.core == nil {
		return out
	}
	if ok, err := s.core.RoomMembershipExists(ctx, core.KindDM, viewerID, dm.GetRoomId()); viewerID != "" && (err != nil || !ok) {
		return out
	}
	if sender, err := s.core.GetUser(ctx, dm.GetSenderId()); err == nil {
		out.SenderDisplayName = proto.String(sender.GetDisplayName())
		if avatarURL, err := s.core.GetUserAvatarURL(ctx, sender.GetId(), nil, nil, ""); err == nil {
			out.SenderAvatarUrl = proto.String(avatarURL)
		}
	}
	out.ConversationName = proto.String(s.realtimeDMConversationName(ctx, viewerID, dm.GetRoomId()))
	return out
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

func (s *HTTPServer) viewerCanReadRealtimeRoomLabel(ctx context.Context, viewerID string, room *corev1.Room) bool {
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

func apiNotificationLevel(level corev1.NotificationLevel) apiv1.NotificationLevel {
	switch level {
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES
	default:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT
	}
}

func apiRealtimeTimeFormat(format corev1.TimeFormat) apiv1.TimeFormat {
	switch format {
	case corev1.TimeFormat_TIME_FORMAT_12H:
		return apiv1.TimeFormat_TIME_FORMAT_12_HOUR
	case corev1.TimeFormat_TIME_FORMAT_24H:
		return apiv1.TimeFormat_TIME_FORMAT_24_HOUR
	default:
		return apiv1.TimeFormat_TIME_FORMAT_AUTO
	}
}
