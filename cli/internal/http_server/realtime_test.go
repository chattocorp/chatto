package http_server

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func (env *wsTestEnv) connectRealtime(t *testing.T) *websocket.Conn {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + realtimePath
	header := http.Header{}
	for _, c := range env.cookieJar.Cookies(mustParseURL(env.server.URL)) {
		header.Add("Cookie", c.String())
	}

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			t.Fatalf("Realtime WebSocket dial failed with status %d: %v", resp.StatusCode, err)
		}
		t.Fatalf("Realtime WebSocket dial failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func sendRealtimeClientFrame(t *testing.T, conn *websocket.Conn, frame *apiv1.RealtimeClientFrame) {
	t.Helper()
	data, err := proto.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal realtime client frame: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("write realtime client frame: %v", err)
	}
}

func readRealtimeServerFrame(t *testing.T, conn *websocket.Conn, timeout time.Duration) (*apiv1.RealtimeServerFrame, bool) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("set realtime read deadline: %v", err)
	}
	mt, data, err := conn.ReadMessage()
	if err != nil {
		if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
			return nil, false
		}
		t.Fatalf("read realtime server frame: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("realtime message type = %d, want binary", mt)
	}
	var frame apiv1.RealtimeServerFrame
	if err := proto.Unmarshal(data, &frame); err != nil {
		t.Fatalf("unmarshal realtime server frame: %v", err)
	}
	return &frame, true
}

func subscribeRealtime(t *testing.T, conn *websocket.Conn, token string) {
	t.Helper()
	sendRealtimeClientFrame(t, conn, &apiv1.RealtimeClientFrame{Frame: &apiv1.RealtimeClientFrame_Hello{
		Hello: &apiv1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	hello, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for realtime hello")
	}
	if got := hello.GetHello(); got == nil {
		t.Fatalf("first realtime frame = %T, want hello", hello.GetFrame())
	} else if got.ProtocolVersion != realtimeProtocolVersion || got.ServerVersion == "" {
		t.Fatalf("unexpected realtime hello: %+v", got)
	}

	sendRealtimeClientFrame(t, conn, &apiv1.RealtimeClientFrame{Frame: &apiv1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &apiv1.RealtimeSubscribeEvents{},
	}})
	subscribed, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for realtime subscribed")
	}
	if subscribed.GetSubscribed() == nil {
		t.Fatalf("second realtime frame = %T, want subscribed", subscribed.GetFrame())
	}
}

func waitRealtimeEvent(t *testing.T, conn *websocket.Conn, timeout time.Duration, match func(*apiv1.RealtimeEventEnvelope) bool) *apiv1.RealtimeEventEnvelope {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		frame, ok := readRealtimeServerFrame(t, conn, time.Until(deadline))
		if !ok {
			return nil
		}
		if event := frame.GetEvent(); event != nil && match(event) {
			return event
		}
	}
	return nil
}

func TestRealtimeMapperMapsOfflinePresence(t *testing.T) {
	frame, err := (&HTTPServer{}).realtimeEventEnvelope(context.Background(), "", core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id:      "presence-1",
		ActorId: "U1",
		Event: &corev1.LiveEvent_PresenceChanged{PresenceChanged: &corev1.PresenceChangedEvent{
			Status: core.PresenceStatusOffline,
		}},
	}))
	if err != nil {
		t.Fatalf("realtimeEventEnvelope: %v", err)
	}
	presence := frame.GetPresenceChanged()
	if presence == nil {
		t.Fatalf("event = %T, want presence_changed", frame.GetEvent())
	}
	if presence.Status != apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE {
		t.Fatalf("presence status = %v, want OFFLINE", presence.Status)
	}
}

func TestRealtimeMapperMapsThreadCreated(t *testing.T) {
	frame, err := (&HTTPServer{}).realtimeEventEnvelope(context.Background(), "", core.NewEVTEventEnvelope(&corev1.Event{
		Id:      "thread-created-1",
		ActorId: "U1",
		Event: &corev1.Event_ThreadCreated{ThreadCreated: &corev1.ThreadCreatedEvent{
			RoomId: "R1", ThreadRootEventId: "M1",
		}},
	}))
	if err != nil {
		t.Fatalf("realtimeEventEnvelope: %v", err)
	}
	thread := frame.GetThreadCreated()
	if thread == nil {
		t.Fatalf("event = %T, want thread_created", frame.GetEvent())
	}
	if thread.RoomId != "R1" || thread.ThreadRootEventId != "M1" {
		t.Fatalf("thread_created = %+v, want room R1 root M1", thread)
	}
}

func TestRealtimeMapperMapsCallEventSource(t *testing.T) {
	frame, err := (&HTTPServer{}).realtimeEventEnvelope(context.Background(), "", core.NewEVTEventEnvelope(&corev1.Event{
		Id:      "call-joined-1",
		ActorId: "U1",
		Event: &corev1.Event_VoiceCallParticipantJoined{VoiceCallParticipantJoined: &corev1.CallParticipantJoinedEvent{
			RoomId: "R1",
			CallId: "call-1",
			Source: corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_LIVEKIT,
		}},
	}))
	if err != nil {
		t.Fatalf("realtimeEventEnvelope: %v", err)
	}
	call := frame.GetCallParticipantJoined()
	if call == nil {
		t.Fatalf("event = %T, want call_participant_joined", frame.GetEvent())
	}
	if call.Source != apiv1.RealtimeCallEventSource_REALTIME_CALL_EVENT_SOURCE_LIVEKIT {
		t.Fatalf("call source = %v, want LIVEKIT", call.Source)
	}
}

func TestRealtimeMapperMapsUnspecifiedNotificationLevelToDefault(t *testing.T) {
	frame, err := (&HTTPServer{}).realtimeEventEnvelope(context.Background(), "", core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id:      "notification-level-1",
		ActorId: "U1",
		Event: &corev1.LiveEvent_NotificationLevelChanged{NotificationLevelChanged: &corev1.NotificationLevelChangedEvent{
			RoomId:         "R1",
			Level:          corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED,
			EffectiveLevel: corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED,
		}},
	}))
	if err != nil {
		t.Fatalf("realtimeEventEnvelope: %v", err)
	}
	notificationLevel := frame.GetNotificationLevelChanged()
	if notificationLevel == nil {
		t.Fatalf("event = %T, want notification_level_changed", frame.GetEvent())
	}
	if notificationLevel.Level != apiv1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		t.Fatalf("level = %v, want DEFAULT", notificationLevel.Level)
	}
	if notificationLevel.EffectiveLevel != apiv1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		t.Fatalf("effective level = %v, want DEFAULT", notificationLevel.EffectiveLevel)
	}
}

func TestRealtimeMapperOmitsAbsentNotificationNavigationFields(t *testing.T) {
	frame, err := (&HTTPServer{}).realtimeEventEnvelope(context.Background(), "", core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id:      "notification-created-1",
		ActorId: "U1",
		Event: &corev1.LiveEvent_NotificationCreated{NotificationCreated: &corev1.NotificationCreatedEvent{
			NotificationId: "N1",
		}},
	}))
	if err != nil {
		t.Fatalf("realtimeEventEnvelope: %v", err)
	}
	if frame.ActorId == nil || frame.GetActorId() != "U1" {
		t.Fatalf("actor_id = %q, present=%v; want U1 present", frame.GetActorId(), frame.ActorId != nil)
	}
	notification := frame.GetNotificationCreated()
	if notification == nil {
		t.Fatalf("event = %T, want notification_created", frame.GetEvent())
	}
	if notification.RoomId != nil || notification.EventId != nil || notification.InReplyToId != nil {
		t.Fatalf("navigation fields present: room=%v event=%v reply=%v; want all absent", notification.RoomId, notification.EventId, notification.InReplyToId)
	}
}

func TestRealtimeMapperHydratesMentionNotificationDisplayData(t *testing.T) {
	env := setupWebSocketTestServer(t)
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-mention-actor", "RT Mention Actor", "password123")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-mention-viewer", "RT Mention Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, actor.Id, core.KindChannel, "", "rt-mention-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, actor.Id, core.KindChannel, actor.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom actor: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}

	frame, err := env.httpServer.realtimeEventEnvelope(env.ctx, viewer.Id, core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id:      "mention-display-1",
		ActorId: actor.Id,
		Event: &corev1.LiveEvent_MentionNotification{MentionNotification: &corev1.MentionNotificationEvent{
			RoomId:            room.Id,
			MentionedByUserId: actor.Id,
		}},
	}))
	if err != nil {
		t.Fatalf("realtimeEventEnvelope: %v", err)
	}
	mention := frame.GetMentionNotification()
	if mention == nil {
		t.Fatalf("event = %T, want mention_notification", frame.GetEvent())
	}
	if mention.RoomName == nil {
		t.Fatal("room name is absent, want hydrated room name")
	}
	if mention.GetRoomName() != room.Name {
		t.Fatalf("room name = %q, want %q", mention.GetRoomName(), room.Name)
	}
	if mention.ActorDisplayName == nil {
		t.Fatal("actor display name is absent, want hydrated actor display name")
	}
	if mention.GetActorDisplayName() != actor.DisplayName {
		t.Fatalf("actor display name = %q, want %q", mention.GetActorDisplayName(), actor.DisplayName)
	}
}

func TestRealtimeMapperHydratesDMNotificationDisplayData(t *testing.T) {
	env := setupWebSocketTestServer(t)
	sender, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-dm-sender", "RT DM Sender", "password123")
	if err != nil {
		t.Fatalf("CreateUser sender: %v", err)
	}
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-dm-viewer", "RT DM Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, _, err := env.core.FindOrCreateDM(env.ctx, sender.Id, []string{viewer.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}

	frame, err := env.httpServer.realtimeEventEnvelope(env.ctx, viewer.Id, core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id:      "dm-display-1",
		ActorId: sender.Id,
		Event: &corev1.LiveEvent_NewDirectMessageNotification{NewDirectMessageNotification: &corev1.NewDirectMessageNotificationEvent{
			RoomId:   room.Id,
			SenderId: sender.Id,
		}},
	}))
	if err != nil {
		t.Fatalf("realtimeEventEnvelope: %v", err)
	}
	dm := frame.GetNewDirectMessageNotification()
	if dm == nil {
		t.Fatalf("event = %T, want new_direct_message_notification", frame.GetEvent())
	}
	if dm.SenderDisplayName == nil {
		t.Fatal("sender display name is absent, want hydrated sender display name")
	}
	if dm.GetSenderDisplayName() != sender.DisplayName {
		t.Fatalf("sender display name = %q, want %q", dm.GetSenderDisplayName(), sender.DisplayName)
	}
	if dm.ConversationName == nil {
		t.Fatal("conversation name is absent, want hydrated conversation name")
	}
	if dm.GetConversationName() != sender.DisplayName {
		t.Fatalf("conversation name = %q, want %q", dm.GetConversationName(), sender.DisplayName)
	}
	if dm.SenderAvatarUrl == nil {
		t.Fatal("sender avatar URL is absent, want hydrated empty avatar URL")
	}
	if dm.GetSenderAvatarUrl() != "" {
		t.Fatalf("sender avatar URL = %q, want empty", dm.GetSenderAvatarUrl())
	}
}

func TestRealtimeWebSocketAuthenticatesWithBearerHello(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-bearer", "RT Bearer", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, token)
}

func TestRealtimeWebSocketAuthenticatesWithCookie(t *testing.T) {
	env := setupWebSocketTestServer(t)
	if _, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-cookie", "RT Cookie", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	env.login(t, "rt-cookie", "password123")

	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, "")
}

func TestRealtimeWebSocketRejectsUnauthenticatedHello(t *testing.T) {
	env := setupWebSocketTestServer(t)
	conn := env.connectRealtime(t)

	sendRealtimeClientFrame(t, conn, &apiv1.RealtimeClientFrame{Frame: &apiv1.RealtimeClientFrame_Hello{
		Hello: &apiv1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion},
	}})
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for realtime auth error")
	}
	errFrame := frame.GetError()
	if errFrame == nil {
		t.Fatalf("frame = %T, want error", frame.GetFrame())
	}
	if errFrame.Code != "authentication_required" || !errFrame.Fatal {
		t.Fatalf("error = %+v, want fatal authentication_required", errFrame)
	}
}

func TestRealtimeWebSocketDeliversRoomMessageToMember(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-member", "RT Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, token)

	posted, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "hello realtime", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	event := waitRealtimeEvent(t, conn, 5*time.Second, func(event *apiv1.RealtimeEventEnvelope) bool {
		msg := event.GetMessagePosted()
		return msg != nil && msg.MessageEventId == posted.Id
	})
	if event == nil {
		t.Fatal("member did not receive realtime message_posted event")
	}
	msg := event.GetMessagePosted()
	if msg.RoomId != room.Id || msg.MessageEventId != posted.Id {
		t.Fatalf("message_posted = %+v, want room %q event %q", msg, room.Id, posted.Id)
	}
}

func TestRealtimeWebSocketDoesNotDeliverRoomMessageToOutsider(t *testing.T) {
	env := setupWebSocketTestServer(t)
	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-visible-member", "RT Visible Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-outsider", "RT Outsider", "password123")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, member.Id, core.KindChannel, "", "rt-private-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, member.Id, core.KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	outsiderRoom, err := env.core.CreateRoom(env.ctx, outsider.Id, core.KindChannel, "", "rt-outsider-room", "")
	if err != nil {
		t.Fatalf("CreateRoom outsider: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, outsider.Id, core.KindChannel, outsider.Id, outsiderRoom.Id); err != nil {
		t.Fatalf("JoinRoom outsider: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, outsider.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, token)

	posted, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, member.Id, "hidden from outsider", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	visible, err := env.core.PostMessage(env.ctx, core.KindChannel, outsiderRoom.Id, outsider.Id, "visible to outsider", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage visible: %v", err)
	}

	event := waitRealtimeEvent(t, conn, 5*time.Second, func(event *apiv1.RealtimeEventEnvelope) bool {
		msg := event.GetMessagePosted()
		if msg == nil {
			return false
		}
		if msg.MessageEventId == posted.Id {
			t.Fatalf("outsider received unauthorized realtime message event: %+v", event)
		}
		return msg.MessageEventId == visible.Id
	})
	if event == nil {
		t.Fatal("outsider did not receive its own authorized realtime message event")
	}
}

func TestRealtimeWebSocketRespondsToPing(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-ping", "RT Ping", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, token)

	time.Sleep(realtimeHandshakeTimeout + 200*time.Millisecond)

	sendRealtimeClientFrame(t, conn, &apiv1.RealtimeClientFrame{Frame: &apiv1.RealtimeClientFrame_Ping{
		Ping: &apiv1.RealtimePing{Nonce: "abc123"},
	}})

	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for pong")
	}
	if got := frame.GetPong(); got == nil || got.Nonce != "abc123" {
		t.Fatalf("pong = %+v, want nonce abc123", got)
	}
}
