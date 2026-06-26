package http_server

import (
	"context"
	"encoding/base64"
	"encoding/binary"
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
	"hmans.de/chatto/internal/core"
	graphauth "hmans.de/chatto/internal/graph/auth"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	realtimePath                     = "/api/realtime"
	realtimeProtocolVersion          = 1
	realtimeReadLimitBytes           = 64 << 10
	realtimeHandshakeTimeout         = 10 * time.Second
	realtimeWriteTimeout             = 10 * time.Second
	realtimeHeartbeatIntervalSeconds = 25
)

func (s *HTTPServer) setupRealtimeAPI(allowedOrigins []string) {
	upgrader := websocket.Upgrader{
		EnableCompression: s.config.Webserver.WebSocketCompressionEnabled(),
		CheckOrigin: func(r *http.Request) bool {
			return s.checkRealtimeWebSocketOrigin(r, allowedOrigins)
		},
	}

	s.router.GET(realtimePath, func(c *gin.Context) {
		req := s.injectUserIntoContext(c)
		conn, err := upgrader.Upgrade(c.Writer, req, nil)
		if err != nil {
			s.logger.Warn("Realtime WebSocket upgrade failed", "error", err)
			return
		}
		defer conn.Close()

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
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host = forwarded
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
	writeFrame := func(frame *apiv1.RealtimeServerFrame) error {
		data, err := proto.Marshal(frame)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := conn.SetWriteDeadline(time.Now().Add(realtimeWriteTimeout)); err != nil {
			return err
		}
		return conn.WriteMessage(websocket.BinaryMessage, data)
	}
	writeError := func(code, message string, fatal bool) {
		_ = writeFrame(&apiv1.RealtimeServerFrame{Frame: &apiv1.RealtimeServerFrame_Error{
			Error: &apiv1.RealtimeError{Code: code, Message: message, Fatal: fatal},
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
	if clientHello.ProtocolVersion != 0 && clientHello.ProtocolVersion != realtimeProtocolVersion {
		writeError("unsupported_protocol", "unsupported realtime protocol version", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "unsupported protocol"), time.Now().Add(time.Second))
		return
	}
	if clientHello.ResumeCursor != "" {
		writeError("resume_unavailable", "resume cursors are reserved but not supported by this server version", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "resume unavailable"), time.Now().Add(time.Second))
		return
	}

	user, err := s.realtimeAuthenticatedUser(ctx, clientHello)
	if err != nil {
		writeError("authentication_required", "authentication required", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authentication required"), time.Now().Add(time.Second))
		return
	}

	if err := writeFrame(&apiv1.RealtimeServerFrame{Frame: &apiv1.RealtimeServerFrame_Hello{
		Hello: &apiv1.RealtimeServerHello{
			ProtocolVersion:          realtimeProtocolVersion,
			ServerVersion:            s.version,
			ResumeSupported:          false,
			HeartbeatIntervalSeconds: realtimeHeartbeatIntervalSeconds,
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
	if subscribe.GetSubscribeEvents() == nil {
		writeError("bad_subscribe", "second frame must be subscribe_events", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseProtocolError, "bad subscribe"), time.Now().Add(time.Second))
		return
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return
	}

	events, err := s.core.StreamMyEventsWithOptions(ctx, user.Id, core.StreamMyEventsOptions{ReportPresence: false})
	if err != nil {
		writeError("subscribe_failed", "failed to start realtime event stream", true)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "subscribe failed"), time.Now().Add(time.Second))
		return
	}
	if err := writeFrame(&apiv1.RealtimeServerFrame{Frame: &apiv1.RealtimeServerFrame_Subscribed{
		Subscribed: &apiv1.RealtimeSubscribed{},
	}}); err != nil {
		return
	}

	go s.readRealtimeControlFrames(ctx, cancel, conn, writeFrame)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				_ = writeFrame(&apiv1.RealtimeServerFrame{Frame: &apiv1.RealtimeServerFrame_Close{
					Close: &apiv1.RealtimeClose{Code: "stream_closed", Message: "event stream closed", Reconnect: true, RetryAfterMs: 1000},
				}})
				return
			}
			frame, err := realtimeServerFrameForEvent(event)
			if err != nil {
				s.logger.Warn("Dropping unsupported realtime event", "event_id", event.ID(), "error", err)
				continue
			}
			if err := writeFrame(frame); err != nil {
				return
			}
			if core.EventSessionTerminated(event) != nil {
				return
			}
		}
	}
}

func readRealtimeClientFrame(conn *websocket.Conn, timeout time.Duration) (*apiv1.RealtimeClientFrame, error) {
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
	var frame apiv1.RealtimeClientFrame
	if err := proto.Unmarshal(data, &frame); err != nil {
		return nil, err
	}
	return &frame, nil
}

func (s *HTTPServer) readRealtimeControlFrames(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, writeFrame func(*apiv1.RealtimeServerFrame) error) {
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
			_ = writeFrame(&apiv1.RealtimeServerFrame{Frame: &apiv1.RealtimeServerFrame_Error{
				Error: &apiv1.RealtimeError{Code: "bad_frame", Message: "expected binary protobuf frame", Fatal: true},
			}})
			return
		}
		var frame apiv1.RealtimeClientFrame
		if err := proto.Unmarshal(data, &frame); err != nil {
			_ = writeFrame(&apiv1.RealtimeServerFrame{Frame: &apiv1.RealtimeServerFrame_Error{
				Error: &apiv1.RealtimeError{Code: "bad_frame", Message: "invalid protobuf frame", Fatal: true},
			}})
			return
		}
		switch payload := frame.GetFrame().(type) {
		case *apiv1.RealtimeClientFrame_Ping:
			_ = writeFrame(&apiv1.RealtimeServerFrame{Frame: &apiv1.RealtimeServerFrame_Pong{
				Pong: &apiv1.RealtimePong{Nonce: payload.Ping.GetNonce()},
			}})
		case *apiv1.RealtimeClientFrame_Ack:
			// Reserved for future backpressure/resume accounting.
		default:
			_ = writeFrame(&apiv1.RealtimeServerFrame{Frame: &apiv1.RealtimeServerFrame_Error{
				Error: &apiv1.RealtimeError{Code: "bad_frame", Message: "unexpected control frame", Fatal: true},
			}})
			return
		}
	}
}

func (s *HTTPServer) realtimeAuthenticatedUser(ctx context.Context, hello *apiv1.RealtimeClientHello) (*corev1.User, error) {
	if token := strings.TrimSpace(hello.GetBearerToken()); token != "" {
		userID, err := s.core.ValidateAuthToken(ctx, token)
		if err != nil {
			return nil, err
		}
		return s.core.GetUser(ctx, userID)
	}
	if user := graphauth.ForContext(ctx); user != nil {
		return user, nil
	}
	return nil, core.ErrNotAuthenticated
}

func realtimeServerFrameForEvent(event core.EventEnvelope) (*apiv1.RealtimeServerFrame, error) {
	if event == nil {
		return nil, errors.New("nil event")
	}
	if heartbeat := event.HeartbeatEvent(); heartbeat != nil {
		return &apiv1.RealtimeServerFrame{Frame: &apiv1.RealtimeServerFrame_Heartbeat{
			Heartbeat: &apiv1.RealtimeHeartbeat{Id: event.ID(), CreatedAt: event.CreatedAt()},
		}}, nil
	}
	envelope, err := realtimeEventEnvelope(event)
	if err != nil {
		return nil, err
	}
	return &apiv1.RealtimeServerFrame{Frame: &apiv1.RealtimeServerFrame_Event{Event: envelope}}, nil
}

func realtimeEventEnvelope(event core.EventEnvelope) (*apiv1.RealtimeEventEnvelope, error) {
	envelope := &apiv1.RealtimeEventEnvelope{
		Id:        event.ID(),
		CreatedAt: event.CreatedAt(),
		ActorId:   event.ActorID(),
		Cursor:    realtimeCursor(event.DeliverySeq()),
	}

	if evt := event.EVTEvent(); evt != nil {
		if err := mapRealtimeEVT(envelope, evt); err != nil {
			return nil, err
		}
		return envelope, nil
	}
	if live := event.LiveEvent(); live != nil {
		if err := mapRealtimeLive(envelope, live); err != nil {
			return nil, err
		}
		return envelope, nil
	}
	return nil, fmt.Errorf("unknown event envelope %T", event.Payload())
}

func mapRealtimeEVT(envelope *apiv1.RealtimeEventEnvelope, event *corev1.Event) error {
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		msg := payload.MessagePosted
		envelope.Event = &apiv1.RealtimeEventEnvelope_MessagePosted{MessagePosted: &apiv1.RealtimeMessagePostedEvent{
			RoomId:            msg.GetRoomId(),
			MessageEventId:    event.GetId(),
			ThreadRootEventId: msg.GetInThread(),
		}}
	case *corev1.Event_MessageEdited:
		msg := payload.MessageEdited
		envelope.Event = &apiv1.RealtimeEventEnvelope_MessageEdited{MessageEdited: &apiv1.RealtimeMessageEditedEvent{
			RoomId: msg.GetRoomId(), MessageEventId: msg.GetEventId(),
		}}
	case *corev1.Event_MessageRetracted:
		msg := payload.MessageRetracted
		envelope.Event = &apiv1.RealtimeEventEnvelope_MessageRetracted{MessageRetracted: &apiv1.RealtimeMessageRetractedEvent{
			RoomId: msg.GetRoomId(), MessageEventId: msg.GetEventId(), Reason: msg.GetReason(),
		}}
	case *corev1.Event_ReactionAdded:
		reaction := payload.ReactionAdded
		envelope.Event = &apiv1.RealtimeEventEnvelope_ReactionAdded{ReactionAdded: &apiv1.RealtimeReactionEvent{
			RoomId: reaction.GetRoomId(), MessageEventId: reaction.GetMessageEventId(), Emoji: reaction.GetEmoji(),
		}}
	case *corev1.Event_ReactionRemoved:
		reaction := payload.ReactionRemoved
		envelope.Event = &apiv1.RealtimeEventEnvelope_ReactionRemoved{ReactionRemoved: &apiv1.RealtimeReactionEvent{
			RoomId: reaction.GetRoomId(), MessageEventId: reaction.GetMessageEventId(), Emoji: reaction.GetEmoji(),
		}}
	case *corev1.Event_RoomCreated:
		envelope.Event = &apiv1.RealtimeEventEnvelope_RoomCreated{RoomCreated: realtimeRoomEvent(payload.RoomCreated.GetRoomId())}
	case *corev1.Event_RoomUpdated:
		envelope.Event = &apiv1.RealtimeEventEnvelope_RoomUpdated{RoomUpdated: realtimeRoomEvent(payload.RoomUpdated.GetRoomId())}
	case *corev1.Event_RoomDeleted:
		envelope.Event = &apiv1.RealtimeEventEnvelope_RoomDeleted{RoomDeleted: realtimeRoomEvent(payload.RoomDeleted.GetRoomId())}
	case *corev1.Event_RoomArchived:
		envelope.Event = &apiv1.RealtimeEventEnvelope_RoomArchived{RoomArchived: realtimeRoomEvent(payload.RoomArchived.GetRoomId())}
	case *corev1.Event_RoomUnarchived:
		envelope.Event = &apiv1.RealtimeEventEnvelope_RoomUnarchived{RoomUnarchived: realtimeRoomEvent(payload.RoomUnarchived.GetRoomId())}
	case *corev1.Event_UserJoinedRoom:
		envelope.Event = &apiv1.RealtimeEventEnvelope_UserJoinedRoom{UserJoinedRoom: realtimeRoomEvent(payload.UserJoinedRoom.GetRoomId())}
	case *corev1.Event_UserLeftRoom:
		envelope.Event = &apiv1.RealtimeEventEnvelope_UserLeftRoom{UserLeftRoom: realtimeRoomEvent(payload.UserLeftRoom.GetRoomId())}
	case *corev1.Event_RoomUniversalChanged:
		room := payload.RoomUniversalChanged
		envelope.Event = &apiv1.RealtimeEventEnvelope_RoomUniversalChanged{RoomUniversalChanged: &apiv1.RealtimeRoomUniversalChangedEvent{
			RoomId: room.GetRoomId(), Universal: room.GetUniversal(),
		}}
	default:
		return fmt.Errorf("unsupported EVT event %T", payload)
	}
	return nil
}

func mapRealtimeLive(envelope *apiv1.RealtimeEventEnvelope, event *corev1.LiveEvent) error {
	switch payload := event.GetEvent().(type) {
	case *corev1.LiveEvent_UserTyping:
		typing := payload.UserTyping
		envelope.Event = &apiv1.RealtimeEventEnvelope_UserTyping{UserTyping: &apiv1.RealtimeTypingEvent{
			RoomId: typing.GetRoomId(), ThreadRootEventId: typing.GetThreadRootEventId(),
		}}
	case *corev1.LiveEvent_PresenceChanged:
		envelope.Event = &apiv1.RealtimeEventEnvelope_PresenceChanged{PresenceChanged: &apiv1.RealtimePresenceChangedEvent{
			UserId: event.GetActorId(), Status: apiPresenceStatus(payload.PresenceChanged.GetStatus()),
		}}
	case *corev1.LiveEvent_NotificationCreated:
		notification := payload.NotificationCreated
		envelope.Event = &apiv1.RealtimeEventEnvelope_NotificationCreated{NotificationCreated: &apiv1.RealtimeNotificationCreatedEvent{
			NotificationId: notification.GetNotificationId(),
			RoomId:         notification.GetRoomId(),
			EventId:        notification.GetEventId(),
			InReplyToId:    notification.GetInReplyToId(),
			Silent:         notification.GetSilent(),
		}}
	case *corev1.LiveEvent_NotificationDismissed:
		envelope.Event = &apiv1.RealtimeEventEnvelope_NotificationDismissed{NotificationDismissed: &apiv1.RealtimeNotificationDismissedEvent{
			NotificationId: payload.NotificationDismissed.GetNotificationId(),
		}}
	case *corev1.LiveEvent_NotificationLevelChanged:
		level := payload.NotificationLevelChanged
		envelope.Event = &apiv1.RealtimeEventEnvelope_NotificationLevelChanged{NotificationLevelChanged: &apiv1.RealtimeNotificationLevelChangedEvent{
			RoomId: level.GetRoomId(), Level: apiNotificationLevel(level.GetLevel()), EffectiveLevel: apiNotificationLevel(level.GetEffectiveLevel()),
		}}
	case *corev1.LiveEvent_ThreadFollowChanged:
		follow := payload.ThreadFollowChanged
		envelope.Event = &apiv1.RealtimeEventEnvelope_ThreadFollowChanged{ThreadFollowChanged: &apiv1.RealtimeThreadFollowChangedEvent{
			RoomId: follow.GetRoomId(), ThreadRootEventId: follow.GetThreadRootEventId(), Following: follow.GetIsFollowing(),
		}}
	case *corev1.LiveEvent_RoomMarkedAsRead:
		envelope.Event = &apiv1.RealtimeEventEnvelope_RoomMarkedAsRead{RoomMarkedAsRead: &apiv1.RealtimeRoomMarkedAsReadEvent{
			RoomId: payload.RoomMarkedAsRead.GetRoomId(),
		}}
	case *corev1.LiveEvent_ServerUpdated:
		server := payload.ServerUpdated
		envelope.Event = &apiv1.RealtimeEventEnvelope_ServerUpdated{ServerUpdated: &apiv1.RealtimeServerUpdatedEvent{
			Name: server.GetName(), Description: server.GetDescription(), LogoUrl: server.GetLogoUrl(), BannerUrl: server.GetBannerUrl(),
		}}
	case *corev1.LiveEvent_UserProfileUpdated:
		user := payload.UserProfileUpdated
		envelope.Event = &apiv1.RealtimeEventEnvelope_UserProfileUpdated{UserProfileUpdated: &apiv1.RealtimeUserProfileUpdatedEvent{
			UserId: user.GetUserId(), Login: user.GetLogin(), DisplayName: user.GetDisplayName(), AvatarUrl: user.GetAvatarUrl(),
		}}
	case *corev1.LiveEvent_SessionTerminated:
		envelope.Event = &apiv1.RealtimeEventEnvelope_SessionTerminated{SessionTerminated: &apiv1.RealtimeSessionTerminatedEvent{
			Reason: payload.SessionTerminated.GetReason(),
		}}
	default:
		return fmt.Errorf("unsupported live event %T", payload)
	}
	return nil
}

func realtimeRoomEvent(roomID string) *apiv1.RealtimeRoomEvent {
	return &apiv1.RealtimeRoomEvent{RoomId: roomID}
}

func realtimeCursor(seq uint64) string {
	if seq == 0 {
		return ""
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], seq)
	return "rt1:" + base64.RawURLEncoding.EncodeToString(buf[:])
}

func apiPresenceStatus(status string) apiv1.PresenceStatus {
	switch status {
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
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES
	default:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED
	}
}
