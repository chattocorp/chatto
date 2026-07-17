package http_server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

func TestRealtimeAuthenticatedUserPreservesAuthenticationValidationError(t *testing.T) {
	s := &HTTPServer{}
	want := errors.New("storage unavailable")
	ctx := context.WithValue(context.Background(), authenticationValidationErrorKey{}, want)

	_, user, err := s.realtimeAuthenticatedUser(ctx, &realtimev1.RealtimeClientHello{})
	if user != nil {
		t.Fatalf("user = %v, want nil", user)
	}
	if !errors.Is(err, want) {
		t.Fatalf("realtimeAuthenticatedUser err = %v, want %v", err, want)
	}
}

type websocketWireRecorder struct {
	net.Conn
	mu    sync.Mutex
	reads []byte
}

func (r *websocketWireRecorder) Read(p []byte) (int, error) {
	n, err := r.Conn.Read(p)
	r.mu.Lock()
	r.reads = append(r.reads, p[:n]...)
	r.mu.Unlock()
	return n, err
}

func (r *websocketWireRecorder) Reset() {
	r.mu.Lock()
	r.reads = r.reads[:0]
	r.mu.Unlock()
}

func (r *websocketWireRecorder) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]byte(nil), r.reads...)
}

func (env *wsTestEnv) dialRealtime(t testing.TB) *websocket.Conn {
	return env.dialRealtimeWithDialer(t, websocket.DefaultDialer)
}

func (env *wsTestEnv) dialRealtimeWithCompression(t testing.TB) *websocket.Conn {
	dialer := *websocket.DefaultDialer
	dialer.EnableCompression = true
	return env.dialRealtimeWithDialer(t, &dialer)
}

func (env *wsTestEnv) dialRealtimeWithCompressionRecorder(t testing.TB) (*websocket.Conn, *websocketWireRecorder) {
	t.Helper()
	dialer := *websocket.DefaultDialer
	dialer.EnableCompression = true
	var recorder *websocketWireRecorder
	netDialer := &net.Dialer{}
	dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := netDialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		recorder = &websocketWireRecorder{Conn: conn}
		return recorder, nil
	}
	conn := env.dialRealtimeWithDialer(t, &dialer)
	if recorder == nil {
		t.Fatal("realtime WebSocket dial did not create a wire recorder")
	}
	return conn, recorder
}

func (env *wsTestEnv) dialRealtimeWithDialer(t testing.TB, dialer *websocket.Dialer) *websocket.Conn {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + realtimePath
	header := http.Header{}
	for _, c := range env.cookieJar.Cookies(mustParseURL(env.server.URL)) {
		header.Add("Cookie", c.String())
	}

	conn, resp, err := dialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			t.Fatalf("Realtime WebSocket dial failed with status %d: %v", resp.StatusCode, err)
		}
		t.Fatalf("Realtime WebSocket dial failed: %v", err)
	}
	return conn
}

func (env *wsTestEnv) connectRealtime(t testing.TB) *websocket.Conn {
	t.Helper()
	conn := env.dialRealtime(t)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func sendRealtimeClientFrame(t testing.TB, conn *websocket.Conn, frame *realtimev1.RealtimeClientFrame) {
	t.Helper()
	data, err := proto.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal realtime client frame: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("write realtime client frame: %v", err)
	}
}

func readRealtimeServerFrame(t testing.TB, conn *websocket.Conn, timeout time.Duration) (*realtimev1.RealtimeServerFrame, bool) {
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
	var frame realtimev1.RealtimeServerFrame
	if err := proto.Unmarshal(data, &frame); err != nil {
		t.Fatalf("unmarshal realtime server frame: %v", err)
	}
	return &frame, true
}

func realtimePingRoundTrip(conn *websocket.Conn, nonce string) error {
	data, err := proto.Marshal(&realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Ping{
		Ping: &realtimev1.RealtimePing{Nonce: nonce},
	}})
	if err != nil {
		return fmt.Errorf("marshal ping: %w", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("write ping: %w", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("set pong deadline: %w", err)
	}
	messageType, data, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read pong: %w", err)
	}
	if messageType != websocket.BinaryMessage {
		return fmt.Errorf("pong message type = %d, want binary", messageType)
	}
	var frame realtimev1.RealtimeServerFrame
	if err := proto.Unmarshal(data, &frame); err != nil {
		return fmt.Errorf("unmarshal pong: %w", err)
	}
	if pong := frame.GetPong(); pong == nil || pong.Nonce != nonce {
		return fmt.Errorf("pong nonce length = %d, want %d", len(pong.GetNonce()), len(nonce))
	}
	return nil
}

func subscribeRealtime(t testing.TB, conn *websocket.Conn, token string) {
	t.Helper()
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	hello, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for realtime hello")
	}
	if got := hello.GetHello(); got == nil {
		t.Fatalf("first realtime frame = %T, want hello", hello.GetFrame())
	} else if got.ProtocolVersion != realtimeProtocolVersion || got.ServerVersion == "" {
		t.Fatalf("unexpected realtime hello: %+v", got)
	} else if got.HeartbeatIntervalSeconds != uint32(core.MyEventsHeartbeatInterval/time.Second) {
		t.Fatalf("heartbeat interval = %d, want %d", got.HeartbeatIntervalSeconds, core.MyEventsHeartbeatInterval/time.Second)
	} else if want := append(append([]string(nil), realtimeServerCapabilities...), realtimeReplayCapability, realtimeProjectionCapability); !slices.Equal(got.GetCapabilities(), want) {
		t.Fatalf("realtime capabilities = %v, want %v", got.GetCapabilities(), want)
	}

	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{},
	}})
	subscribed, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for realtime subscribed")
	}
	if subscribed.GetSubscribed() == nil {
		t.Fatalf("second realtime frame = %T, want subscribed", subscribed.GetFrame())
	}
	for {
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for realtime caught_up")
		}
		if frame.GetCaughtUp() != nil {
			break
		}
		if frame.GetProjectionEvent() == nil {
			t.Fatalf("realtime bootstrap frame = %T, want projection_event or caught_up", frame.GetFrame())
		}
	}
}

func waitRealtimeEvent(t testing.TB, conn *websocket.Conn, timeout time.Duration, match func(*realtimev1.RealtimeEventEnvelope) bool) *realtimev1.RealtimeEventEnvelope {
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

func waitRealtimeTimelineUpsert(t testing.TB, conn *websocket.Conn, timeout time.Duration, match func(*realtimev1.RealtimeProjectionRoomTimelineEventUpsert) bool) *realtimev1.RealtimeProjectionRoomTimelineEventUpsert {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		frame, ok := readRealtimeServerFrame(t, conn, time.Until(deadline))
		if !ok {
			return nil
		}
		projection := frame.GetProjectionEvent()
		if projection == nil {
			continue
		}
		for _, operation := range projection.GetOperations() {
			if upsert := operation.GetRoomTimelineEventUpsert(); upsert != nil && match(upsert) {
				return upsert
			}
		}
	}
	return nil
}

func waitRealtimeTimelineRemove(t testing.TB, conn *websocket.Conn, timeout time.Duration, match func(*realtimev1.RealtimeProjectionRoomTimelineEventRemove) bool) *realtimev1.RealtimeProjectionRoomTimelineEventRemove {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		frame, ok := readRealtimeServerFrame(t, conn, time.Until(deadline))
		if !ok {
			return nil
		}
		projection := frame.GetProjectionEvent()
		if projection == nil {
			continue
		}
		for _, operation := range projection.GetOperations() {
			if remove := operation.GetRoomTimelineEventRemove(); remove != nil && match(remove) {
				return remove
			}
		}
	}
	return nil
}

func waitRealtimeRoomUpsert(t testing.TB, conn *websocket.Conn, timeout time.Duration, match func(*realtimev1.RealtimeProjectionRoom) bool) *realtimev1.RealtimeProjectionRoom {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		frame, ok := readRealtimeServerFrame(t, conn, time.Until(deadline))
		if !ok {
			return nil
		}
		projection := frame.GetProjectionEvent()
		if projection == nil {
			continue
		}
		for _, operation := range projection.GetOperations() {
			if upsert := operation.GetRoomUpsert(); upsert != nil && match(upsert) {
				return upsert
			}
		}
	}
	return nil
}

func readRealtimeCaughtUp(t testing.TB, conn *websocket.Conn) *realtimev1.RealtimeCaughtUp {
	t.Helper()
	for {
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for realtime caught_up")
		}
		if caughtUp := frame.GetCaughtUp(); caughtUp != nil {
			return caughtUp
		}
		if frame.GetProjectionEvent() == nil {
			t.Fatalf("bootstrap frame = %T, want projection_event or caught_up", frame.GetFrame())
		}
	}
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

func TestRealtimeMapperCanonicalizesEchoReactionMessageID(t *testing.T) {
	timeline := core.NewRoomTimelineProjection()
	if err := timeline.Apply(&corev1.Event{
		Id: "M1",
		Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{
			RoomId:   "R1",
			InThread: "ROOT1",
		}},
	}, 1); err != nil {
		t.Fatalf("apply original message: %v", err)
	}
	if err := timeline.Apply(&corev1.Event{
		Id: "ECHO1",
		Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{
			RoomId:        "R1",
			EchoOfEventId: "M1",
		}},
	}, 2); err != nil {
		t.Fatalf("apply echo message: %v", err)
	}

	server := &HTTPServer{core: &core.ChattoCore{RoomTimeline: timeline}}
	frame, err := server.realtimeEventEnvelope(context.Background(), "", core.NewEVTEventEnvelope(&corev1.Event{
		Id:      "reaction-1",
		ActorId: "U1",
		Event: &corev1.Event_ReactionAdded{ReactionAdded: &corev1.ReactionAddedEvent{
			RoomId:         "R1",
			MessageEventId: "ECHO1",
			Emoji:          "thumbsup",
		}},
	}))
	if err != nil {
		t.Fatalf("realtimeEventEnvelope: %v", err)
	}
	reaction := frame.GetReactionAdded()
	if reaction == nil {
		t.Fatalf("event = %T, want reaction_added", frame.GetEvent())
	}
	if reaction.MessageEventId != "M1" {
		t.Fatalf("reaction message_event_id = %q, want M1", reaction.MessageEventId)
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
	if call.Source != realtimev1.RealtimeCallEventSource_REALTIME_CALL_EVENT_SOURCE_LIVEKIT {
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

func TestRealtimeWebSocketPreservesVersionOneLiveOnlyFlow(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-v1", "RT V1", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-v1-room", "")
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
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeLegacyProtocolVersion, BearerToken: proto.String(token)},
	}})
	hello, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || hello.GetHello() == nil || hello.GetHello().GetProtocolVersion() != realtimeLegacyProtocolVersion {
		t.Fatalf("v1 hello = %+v", hello)
	}
	if slices.Contains(hello.GetHello().GetCapabilities(), realtimeReplayCapability) {
		t.Fatalf("v1 capabilities advertise replay: %v", hello.GetHello().GetCapabilities())
	}
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{},
	}})
	if frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second); !ok || frame.GetSubscribed() == nil {
		t.Fatalf("v1 subscribed = %+v", frame)
	}
	// UserAccountCreated is intentionally unsupported by the v1 wire shape.
	// Dropping it must not poison subsequent loop iterations.
	if _, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-v1-later-user", "RT V1 Later", "password123"); err != nil {
		t.Fatalf("CreateUser after subscription: %v", err)
	}
	posted, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "v1 remains live-only", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetEvent() == nil || frame.GetEvent().GetMessagePosted().GetMessageEventId() != posted.Id {
		t.Fatalf("first v1 live frame after subscribed = %+v", frame)
	}
}

func TestRealtimeWebSocketRejectsUnknownProtocolVersion(t *testing.T) {
	env := setupWebSocketTestServer(t)
	conn := env.connectRealtime(t)

	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion + 1},
	}})
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for unsupported protocol error")
	}
	errFrame := frame.GetError()
	if errFrame == nil || errFrame.GetCode() != "unsupported_protocol" || !errFrame.GetFatal() {
		t.Fatalf("unsupported protocol frame = %+v, want fatal unsupported_protocol", frame)
	}
}

func TestRealtimeProjectionSnapshotFramesBeginWithResetAndContainCanonicalResources(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-snapshot", "RT Snapshot", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-snapshot-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, viewer.Id, "snapshot message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	frames, err := env.httpServer.realtimeProjectionSnapshotFrames(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("realtimeProjectionSnapshotFrames: %v", err)
	}
	if len(frames) == 0 {
		t.Fatal("snapshot frames are empty, want reset prefix")
	}
	first := frames[0].GetProjectionEvent()
	if first == nil || len(first.GetOperations()) != 1 || first.GetOperations()[0].GetReset_() == nil {
		t.Fatalf("first snapshot frame = %+v, want reset", frames)
	}

	var hasServer, hasServerState, hasViewer, hasViewerUser, hasRoom bool
	var hasGroups, hasNotifications, hasTimeline bool
	for _, frame := range frames {
		projection := frame.GetProjectionEvent()
		if projection == nil || len(projection.GetOperations()) != 1 {
			t.Fatalf("snapshot frame = %+v, want exactly one projection operation", frame)
		}
		operation := projection.GetOperations()[0]
		hasServer = hasServer || operation.GetServerUpsert() != nil
		hasServerState = hasServerState || operation.GetServerStateUpsert() != nil
		hasViewer = hasViewer || operation.GetViewerUpsert() != nil
		if user := operation.GetUserUpsert(); user.GetUser().GetId() == viewer.Id {
			hasViewerUser = true
		}
		if upsert := operation.GetRoomUpsert(); upsert.GetRoom().GetRoom().GetId() == room.Id {
			hasRoom = slices.Contains(upsert.GetMemberUserIds(), viewer.Id)
		}
		hasGroups = hasGroups || operation.GetRoomGroupsReplace() != nil
		hasNotifications = hasNotifications || operation.GetNotificationsReplace() != nil
		if timeline := operation.GetRoomTimelineReplace(); timeline.GetRoomId() == room.Id {
			hasTimeline = timeline.GetEventCursors()[message.Id] != ""
		}
	}
	if !hasServer || !hasServerState || !hasViewer || !hasViewerUser || !hasRoom || !hasGroups || !hasNotifications || !hasTimeline {
		t.Fatalf("snapshot coverage: server=%v server_state=%v viewer=%v user=%v room=%v groups=%v notifications=%v timeline=%v", hasServer, hasServerState, hasViewer, hasViewerUser, hasRoom, hasGroups, hasNotifications, hasTimeline)
	}
}

func TestRealtimeProjectionRoomReadReplacesOnlyThatRoomViewerState(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-read-viewer", "RT Read Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-read-author", "RT Read Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-read-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{viewer.Id, author.Id} {
		if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %q: %v", userID, err)
		}
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, author.Id, "read me", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if _, err := env.core.CreateNotification(env.ctx, viewer.Id, author.Id, &corev1.Notification{
		Notification: &corev1.Notification_Mention{Mention: &corev1.MentionNotification{
			RoomId: room.Id, EventId: message.Id,
		}},
	}); err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}
	if _, err := env.core.ReadState().MarkRoomAsRead(env.ctx, viewer.Id, room.Id, message.Id); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}

	frame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, viewer.Id, core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id:      "room-read-1",
		ActorId: viewer.Id,
		Event: &corev1.LiveEvent_RoomMarkedAsRead{RoomMarkedAsRead: &corev1.RoomMarkedAsReadEvent{
			RoomId: room.Id,
		}},
	}))
	if err != nil {
		t.Fatalf("realtimeProjectionFrameForEvent: %v", err)
	}
	if !handled {
		t.Fatal("room-read event was not handled as a projection mutation")
	}
	operations := frame.GetProjectionEvent().GetOperations()
	if len(operations) != 2 {
		t.Fatalf("room-read operations = %d, want viewer-state and notification replacements", len(operations))
	}
	replacement := operations[0].GetRoomViewerStateReplace()
	if replacement.GetRoomId() != room.Id || replacement.GetViewerState().GetHasUnread() {
		t.Fatalf("room-read replacement = %+v, want room %q with has_unread=false", replacement, room.Id)
	}
	if notifications := operations[1].GetNotificationsReplace(); notifications == nil {
		t.Fatal("room-read event did not replace current notification state")
	} else if len(notifications.GetPage().GetNotifications()) != 0 || len(notifications.GetRoomCounts()) != 0 {
		t.Fatalf("room-read notifications = %+v, want no pending notification state", notifications)
	}
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

func TestRealtimeWebSocketRejectsCookieHandleAsBearerHello(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-cookie-as-bearer", "RT Cookie As Bearer", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID, _, err := env.core.CreateCookieSession(env.ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}

	conn := env.connectRealtime(t)
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(sessionID)},
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

func TestRealtimeWebSocketRejectsUnauthenticatedHello(t *testing.T) {
	env := setupWebSocketTestServer(t)
	conn := env.connectRealtime(t)

	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion},
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

	var upsert *realtimev1.RealtimeProjectionRoomTimelineEventUpsert
	deadline := time.Now().Add(5 * time.Second)
	for upsert == nil && time.Now().Before(deadline) {
		frame, ok := readRealtimeServerFrame(t, conn, time.Until(deadline))
		if !ok {
			break
		}
		projection := frame.GetProjectionEvent()
		if projection == nil || projection.GetId() != posted.Id {
			continue
		}
		for _, operation := range projection.GetOperations() {
			if operation.GetRoomUpsert() != nil {
				t.Fatal("message projection redundantly included a full room upsert")
			}
			if candidate := operation.GetRoomTimelineEventUpsert(); candidate.GetEvent().GetId() == posted.Id {
				upsert = candidate
			}
		}
	}
	if upsert == nil {
		t.Fatal("member did not receive realtime timeline upsert")
	}
	if upsert.GetRoomId() != room.Id || upsert.GetEvent().GetMessagePosted() == nil {
		t.Fatalf("timeline upsert = %+v, want room %q message %q", upsert, room.Id, posted.Id)
	}
}

func TestRealtimeWebSocketConvergesDirectoryRoomsAndAdministrativeMembership(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-directory-owner", "Directory Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-directory-viewer", "Directory Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, owner.Id, core.KindChannel, "", "directory-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, owner.Id, core.KindChannel, owner.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom owner: %v", err)
	}
	viewerToken, err := env.core.CreateAuthToken(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken viewer: %v", err)
	}
	viewerConn := env.connectRealtime(t)
	subscribeRealtime(t, viewerConn, viewerToken)

	if _, err := env.core.UpdateRoom(env.ctx, owner.Id, core.KindChannel, room.Id, "directory-room-renamed", ""); err != nil {
		t.Fatalf("UpdateRoom: %v", err)
	}
	visible := waitRealtimeRoomUpsert(t, viewerConn, 5*time.Second, func(upsert *realtimev1.RealtimeProjectionRoom) bool {
		return upsert.GetRoom().GetRoom().GetId() == room.Id && upsert.GetRoom().GetRoom().GetName() == "directory-room-renamed"
	})
	if visible == nil {
		t.Fatal("directory-visible nonmember did not receive room metadata update")
	}

	ownerToken, err := env.core.CreateAuthToken(env.ctx, owner.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken owner: %v", err)
	}
	ownerConn := env.connectRealtime(t)
	subscribeRealtime(t, ownerConn, ownerToken)
	if _, err := env.core.AddMember(env.ctx, owner.Id, core.KindChannel, room.Id, viewer.Id); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	membership := waitRealtimeRoomUpsert(t, ownerConn, 5*time.Second, func(upsert *realtimev1.RealtimeProjectionRoom) bool {
		return slices.Contains(upsert.GetMemberUserIds(), viewer.Id)
	})
	if membership == nil {
		t.Fatal("existing member did not receive complete administrative membership update")
	}
}

func TestRealtimeWebSocketThreadReplyUpdatesRootSummary(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-thread-member", "RT Thread Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-thread-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, token)

	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "reply", nil, root.Id, root.Id, nil, false)
	if err != nil {
		t.Fatalf("PostMessage reply: %v", err)
	}

	upsert := waitRealtimeTimelineUpsert(t, conn, 5*time.Second, func(upsert *realtimev1.RealtimeProjectionRoomTimelineEventUpsert) bool {
		return upsert.GetEvent().GetId() == root.Id
	})
	if upsert == nil {
		t.Fatal("did not receive root summary upsert")
	}
	if got := upsert.GetEvent().GetMessagePosted().GetMessage().GetThread().GetReplyCount(); got != 1 {
		t.Fatalf("root reply count = %d, want 1 (reply %q)", got, reply.Id)
	}
}

func TestRealtimeWebSocketMessageRetractionUpsertsDeletedRow(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-delete-member", "RT Delete Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-delete-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "delete me", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, token)

	if err := env.core.DeleteMessage(env.ctx, user.Id, core.KindChannel, room.Id, message.Id); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	upsert := waitRealtimeTimelineUpsert(t, conn, 5*time.Second, func(upsert *realtimev1.RealtimeProjectionRoomTimelineEventUpsert) bool {
		return upsert.GetEvent().GetId() == message.Id
	})
	if upsert == nil {
		t.Fatal("did not receive retracted message upsert")
	}
	deleted := upsert.GetEvent().GetMessagePosted().GetMessage()
	if deleted.GetDeletedAt() == nil || deleted.GetBody() != "" {
		t.Fatalf("retracted message = %+v, want deleted tombstone", deleted)
	}
}

func TestRealtimeWebSocketMirrorsChannelEchoReactionsAndRemoval(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-echo-member", "RT Echo Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-echo-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "reply", nil, root.Id, "", nil, true)
	if err != nil {
		t.Fatalf("PostMessage reply: %v", err)
	}
	echoID, ok := env.core.ChannelEchoEventID(reply.Id)
	if !ok {
		t.Fatal("expected channel echo")
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, token)

	if added, err := env.core.AddReaction(env.ctx, core.KindChannel, room.Id, reply.Id, "thumbsup", user.Id); err != nil || !added {
		t.Fatalf("AddReaction: added=%v err=%v", added, err)
	}
	echoUpsert := waitRealtimeTimelineUpsert(t, conn, 5*time.Second, func(upsert *realtimev1.RealtimeProjectionRoomTimelineEventUpsert) bool {
		return upsert.GetEvent().GetId() == echoID
	})
	if echoUpsert == nil || len(echoUpsert.GetEvent().GetMessagePosted().GetMessage().GetReactions()) != 1 {
		t.Fatalf("echo reaction upsert = %+v, want one reaction", echoUpsert)
	}

	if err := env.core.EditMessage(env.ctx, user.Id, core.KindChannel, room.Id, reply.Id, "reply without echo", core.WithMessageChannelEcho(false)); err != nil {
		t.Fatalf("EditMessage remove echo: %v", err)
	}
	removed := waitRealtimeTimelineRemove(t, conn, 5*time.Second, func(remove *realtimev1.RealtimeProjectionRoomTimelineEventRemove) bool {
		return remove.GetRoomId() == room.Id && remove.GetEventId() == echoID
	})
	if removed == nil {
		t.Fatal("did not receive channel echo timeline removal")
	}

	if err := env.core.EditMessage(env.ctx, user.Id, core.KindChannel, room.Id, reply.Id, "reply echoed again", core.WithMessageChannelEcho(true)); err != nil {
		t.Fatalf("EditMessage restore echo: %v", err)
	}
	restoredEchoID, ok := env.core.ChannelEchoEventID(reply.Id)
	if !ok || restoredEchoID == echoID {
		t.Fatalf("restored echo = %q, want a new visible echo", restoredEchoID)
	}
	if restored := waitRealtimeTimelineUpsert(t, conn, 5*time.Second, func(upsert *realtimev1.RealtimeProjectionRoomTimelineEventUpsert) bool {
		return upsert.GetEvent().GetId() == restoredEchoID
	}); restored == nil {
		t.Fatal("did not receive restored channel echo upsert")
	}

	if err := env.core.DeleteMessage(env.ctx, user.Id, core.KindChannel, room.Id, reply.Id); err != nil {
		t.Fatalf("DeleteMessage canonical reply: %v", err)
	}
	tombstone := waitRealtimeTimelineUpsert(t, conn, 5*time.Second, func(upsert *realtimev1.RealtimeProjectionRoomTimelineEventUpsert) bool {
		return upsert.GetEvent().GetId() == restoredEchoID
	})
	if tombstone == nil || tombstone.GetEvent().GetMessagePosted().GetMessage().GetDeletedAt() == nil {
		t.Fatalf("echo tombstone upsert = %+v, want deleted row", tombstone)
	}
}

func TestRealtimeProjectionReplayAdvancesPastAlreadyHiddenEchoCreation(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-hidden-echo-replay", "Hidden Echo Replay", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-hidden-echo-replay", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	before, err := env.core.PlanRealtimeReplay(env.ctx, user.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "reply", nil, root.Id, "", nil, true)
	if err != nil {
		t.Fatalf("PostMessage reply: %v", err)
	}
	echoID, ok := env.core.ChannelEchoEventID(reply.Id)
	if !ok {
		t.Fatal("expected channel echo")
	}
	if err := env.core.DeleteMessage(env.ctx, user.Id, core.KindChannel, room.Id, echoID); err != nil {
		t.Fatalf("DeleteMessage echo: %v", err)
	}

	replay, err := env.core.PlanRealtimeReplay(env.ctx, user.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	seenHiddenCreation := false
	seenRemoval := false
	for _, event := range replay.Events {
		if posted := event.EVTEvent().GetMessagePosted(); posted != nil && event.ID() == echoID {
			seenHiddenCreation = true
		}
		frame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, user.Id, event)
		if err != nil {
			t.Fatalf("map replay event %q (%T): %v", event.ID(), event.EVTEvent().GetEvent(), err)
		}
		if !handled || frame.GetProjectionEvent() == nil {
			continue
		}
		for _, operation := range frame.GetProjectionEvent().GetOperations() {
			remove := operation.GetRoomTimelineEventRemove()
			if remove.GetRoomId() == room.Id && remove.GetEventId() == echoID {
				seenRemoval = true
			}
		}
	}
	if !seenHiddenCreation || !seenRemoval {
		t.Fatalf("hidden echo replay creation/removal = %v/%v, want both", seenHiddenCreation, seenRemoval)
	}
}

func TestRealtimeProjectionReplayMapsAssetLifecycleToCurrentMessage(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-asset-replay", "Asset Replay", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-asset-replay", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	attachment, err := env.core.UploadAttachment(env.ctx, user.Id, room.Id, "replay.txt", "text/plain", strings.NewReader("asset"))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "asset lifecycle", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	before, err := env.core.PlanRealtimeReplay(env.ctx, user.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	if err := env.core.RecordAssetProcessingStarted(env.ctx, core.SystemActorID, core.KindChannel, room.Id, message.Id, attachment.Id); err != nil {
		t.Fatalf("RecordAssetProcessingStarted: %v", err)
	}
	if err := env.core.RecordAssetProcessingFailed(env.ctx, core.SystemActorID, core.KindChannel, room.Id, message.Id, attachment.Id, corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED); err != nil {
		t.Fatalf("RecordAssetProcessingFailed: %v", err)
	}
	if err := env.core.RecordAssetDeleted(env.ctx, core.SystemActorID, core.KindChannel, room.Id, attachment.Id); err != nil {
		t.Fatalf("RecordAssetDeleted: %v", err)
	}

	replay, err := env.core.PlanRealtimeReplay(env.ctx, user.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if len(replay.Events) != 3 {
		t.Fatalf("asset replay events = %d, want 3", len(replay.Events))
	}
	for _, event := range replay.Events {
		frame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, user.Id, event)
		if err != nil {
			t.Fatalf("map asset replay event %q (%T): %v", event.ID(), event.EVTEvent().GetEvent(), err)
		}
		if !handled || frame.GetProjectionEvent() == nil {
			t.Fatalf("asset replay event %q (%T) was not projected", event.ID(), event.EVTEvent().GetEvent())
		}
		operations := frame.GetProjectionEvent().GetOperations()
		if len(operations) != 1 || operations[0].GetRoomTimelineEventUpsert().GetEvent().GetId() != message.Id {
			t.Fatalf("asset replay operations = %+v, want message %q upsert", operations, message.Id)
		}
		if frame.GetProjectionEvent().GetResumeCursor() == "" {
			t.Fatal("asset replay projection has no resume cursor")
		}
	}
}

func TestRealtimeWebSocketReplaysReactionAfterDisconnect(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-replay-member", "RT Replay Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-replay-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "react while disconnected", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	boundaryConn := env.dialRealtime(t)
	sendRealtimeClientFrame(t, boundaryConn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	if frame, ok := readRealtimeServerFrame(t, boundaryConn, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatal("did not receive replay hello")
	}
	sendRealtimeClientFrame(t, boundaryConn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{},
	}})
	if frame, ok := readRealtimeServerFrame(t, boundaryConn, 5*time.Second); !ok || frame.GetSubscribed() == nil {
		t.Fatal("did not receive replay subscribed")
	}
	boundary := readRealtimeCaughtUp(t, boundaryConn)
	if boundary.GetCursor() == "" {
		t.Fatal("boundary caught_up has no cursor")
	}
	resumeCursor := boundary.GetCursor()
	boundaryConn.Close()

	if added, err := env.core.AddReaction(env.ctx, core.KindChannel, room.Id, message.Id, "thumbsup", user.Id); err != nil || !added {
		t.Fatalf("AddReaction = %v, %v", added, err)
	}

	resumed := env.dialRealtime(t)
	t.Cleanup(func() { resumed.Close() })
	sendRealtimeClientFrame(t, resumed, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	if frame, ok := readRealtimeServerFrame(t, resumed, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatal("did not receive resumed hello")
	}
	sendRealtimeClientFrame(t, resumed, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{ResumeCursor: &resumeCursor},
	}})
	subscribed, ok := readRealtimeServerFrame(t, resumed, 5*time.Second)
	if !ok || subscribed.GetSubscribed() == nil || subscribed.GetSubscribed().GetStartCursor() != resumeCursor {
		t.Fatalf("resumed subscribed = %+v", subscribed)
	}
	replayed, ok := readRealtimeServerFrame(t, resumed, 5*time.Second)
	if !ok || replayed.GetProjectionEvent() == nil || len(replayed.GetProjectionEvent().GetOperations()) != 1 {
		t.Fatalf("replayed frame = %+v, want one projection operation", replayed)
	}
	upsert := replayed.GetProjectionEvent().GetOperations()[0].GetRoomTimelineEventUpsert()
	reaction := upsert.GetReactionChange()
	if upsert.GetRoomId() != room.Id || reaction.GetMessageEventId() != message.Id || reaction.GetEmoji() != "thumbsup" || reaction.GetAction() != realtimev1.RealtimeProjectionReactionAction_REALTIME_PROJECTION_REACTION_ACTION_ADDED {
		t.Fatalf("replayed reaction = %+v", reaction)
	}
	if replayed.GetProjectionEvent().GetResumeCursor() == "" {
		t.Fatal("replayed reaction has no resume cursor")
	}
	notifications, ok := readRealtimeServerFrame(t, resumed, 5*time.Second)
	if !ok || notifications.GetProjectionEvent() == nil || len(notifications.GetProjectionEvent().GetOperations()) != 1 || notifications.GetProjectionEvent().GetOperations()[0].GetNotificationsReplace() == nil {
		t.Fatalf("post-replay frame = %+v, want notification reconciliation", notifications)
	}
	readRealtimeCaughtUp(t, resumed)
}

func TestRealtimeWebSocketDeliversPresenceUpdateToOtherUser(t *testing.T) {
	env := setupWebSocketTestServer(t)
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-presence-actor", "RT Presence Actor", "password123")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-presence-viewer", "RT Presence Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken viewer: %v", err)
	}

	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, token)

	if err := env.core.SetPresenceWithOptions(env.ctx, actor.Id, core.PresenceStatusAway, true); err != nil {
		t.Fatalf("SetPresenceWithOptions actor: %v", err)
	}

	event := waitRealtimeEvent(t, conn, 5*time.Second, func(event *realtimev1.RealtimeEventEnvelope) bool {
		presence := event.GetPresenceChanged()
		return presence != nil && presence.UserId == actor.Id
	})
	if event == nil {
		t.Fatal("viewer did not receive actor presence_changed event")
	}
	if event.GetActorId() != actor.Id {
		t.Fatalf("presence envelope actor_id = %q, want %q", event.GetActorId(), actor.Id)
	}
	presence := event.GetPresenceChanged()
	if presence.Status != apiv1.PresenceStatus_PRESENCE_STATUS_AWAY {
		t.Fatalf("presence status = %v, want AWAY", presence.Status)
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

	upsert := waitRealtimeTimelineUpsert(t, conn, 5*time.Second, func(upsert *realtimev1.RealtimeProjectionRoomTimelineEventUpsert) bool {
		if upsert.GetEvent().GetId() == posted.Id {
			t.Fatalf("outsider received unauthorized realtime timeline upsert: %+v", upsert)
		}
		return upsert.GetEvent().GetId() == visible.Id
	})
	if upsert == nil {
		t.Fatal("outsider did not receive its own authorized realtime timeline upsert")
	}
}

func TestRealtimeWebSocketNegotiatedCompressionSupportsLargeFrames(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-ping", "RT Ping", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	conn := env.dialRealtimeWithCompression(t)
	t.Cleanup(func() { conn.Close() })
	subscribeRealtime(t, conn, token)

	time.Sleep(realtimeHandshakeTimeout + 200*time.Millisecond)

	// The payload is much larger than either transport buffer but remains well
	// below the 64 KiB message limit. Buffer sizing must not limit frame size.
	nonce := strings.Repeat("0123456789abcdef", 512)
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Ping{
		Ping: &realtimev1.RealtimePing{Nonce: nonce},
	}})

	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for pong")
	}
	if got := frame.GetPong(); got == nil || got.Nonce != nonce {
		t.Fatalf("pong nonce length = %d, want %d", len(got.GetNonce()), len(nonce))
	}
}

func TestRealtimeWebSocketCompressionThresholdOnWire(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-wire-compression", "RT Wire Compression", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	tests := []struct {
		name           string
		nonce          string
		wantCompressed bool
	}{
		{name: "small frame", nonce: "small", wantCompressed: false},
		{name: "large frame", nonce: strings.Repeat("0123456789abcdef", 128), wantCompressed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn, recorder := env.dialRealtimeWithCompressionRecorder(t)
			t.Cleanup(func() { conn.Close() })
			subscribeRealtime(t, conn, token)
			recorder.Reset()

			if err := realtimePingRoundTrip(conn, test.nonce); err != nil {
				t.Fatal(err)
			}
			wire := recorder.Bytes()
			if len(wire) == 0 {
				t.Fatal("recorded no server WebSocket frame bytes")
			}
			if compressed := wire[0]&0x40 != 0; compressed != test.wantCompressed {
				t.Fatalf("RSV1 compressed = %v, want %v (first byte %#x)", compressed, test.wantCompressed, wire[0])
			}
		})
	}
}

func TestRealtimeWebSocketConcurrentSmallFramesStayUncompressed(t *testing.T) {
	const connectionCount = 16

	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-concurrent-compression", "RT Concurrent Compression", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	connections := make([]*websocket.Conn, 0, connectionCount)
	recorders := make([]*websocketWireRecorder, 0, connectionCount)
	for range connectionCount {
		conn, recorder := env.dialRealtimeWithCompressionRecorder(t)
		subscribeRealtime(t, conn, token)
		recorder.Reset()
		connections = append(connections, conn)
		recorders = append(recorders, recorder)
	}
	t.Cleanup(func() {
		for _, conn := range connections {
			conn.Close()
		}
	})

	start := make(chan struct{})
	results := make(chan error, connectionCount)
	for i, conn := range connections {
		go func(i int, conn *websocket.Conn) {
			<-start
			if err := realtimePingRoundTrip(conn, "small"); err != nil {
				results <- fmt.Errorf("connection %d: %w", i, err)
				return
			}
			wire := recorders[i].Bytes()
			if len(wire) == 0 {
				results <- fmt.Errorf("connection %d: recorded no server frame bytes", i)
				return
			}
			if wire[0]&0x40 != 0 {
				results <- fmt.Errorf("connection %d: small frame has RSV1 set (first byte %#x)", i, wire[0])
				return
			}
			results <- nil
		}(i, conn)
	}
	close(start)
	for range connectionCount {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
}

func TestShouldCompressRealtimeFrame(t *testing.T) {
	tests := []struct {
		name               string
		compressionEnabled bool
		payloadBytes       int
		want               bool
	}{
		{name: "disabled large frame", compressionEnabled: false, payloadBytes: realtimeCompressionMinBytes * 2, want: false},
		{name: "empty frame", compressionEnabled: true, payloadBytes: 0, want: false},
		{name: "below threshold", compressionEnabled: true, payloadBytes: realtimeCompressionMinBytes - 1, want: false},
		{name: "at threshold", compressionEnabled: true, payloadBytes: realtimeCompressionMinBytes, want: true},
		{name: "above threshold", compressionEnabled: true, payloadBytes: realtimeCompressionMinBytes + 1, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldCompressRealtimeFrame(test.compressionEnabled, test.payloadBytes); got != test.want {
				t.Fatalf("shouldCompressRealtimeFrame(%v, %d) = %v, want %v", test.compressionEnabled, test.payloadBytes, got, test.want)
			}
		})
	}
}

func BenchmarkRealtimeWebSocketIdleConnections(b *testing.B) {
	// This is a bounded regression benchmark for connection-scaled Go
	// allocations in the in-process test harness, not a production RSS model.
	// Real server-only RSS and heap measurements use an external load generator.
	if b.N > 500 {
		b.Skip("run with -benchtime=500x; this benchmark retains every socket until measurement")
	}

	env := setupWebSocketTestServer(b)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-benchmark", "RT Benchmark", "password123")
	if err != nil {
		b.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		b.Fatalf("CreateAuthToken: %v", err)
	}

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	connections := make([]*websocket.Conn, 0, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		conn := env.dialRealtimeWithCompression(b)
		subscribeRealtime(b, conn, token)
		connections = append(connections, conn)
	}
	b.StopTimer()

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if b.N > 0 {
		if after.HeapAlloc > before.HeapAlloc {
			b.ReportMetric(float64(after.HeapAlloc-before.HeapAlloc)/float64(b.N), "retained-heap-B/conn")
		}
		if after.HeapSys > before.HeapSys {
			b.ReportMetric(float64(after.HeapSys-before.HeapSys)/float64(b.N), "heap-sys-B/conn")
		}
		if after.Sys > before.Sys {
			b.ReportMetric(float64(after.Sys-before.Sys)/float64(b.N), "runtime-sys-B/conn")
		}
		if after.StackInuse > before.StackInuse {
			b.ReportMetric(float64(after.StackInuse-before.StackInuse)/float64(b.N), "stack-B/conn")
		}
	}

	for _, conn := range connections {
		if err := conn.Close(); err != nil {
			b.Errorf("close realtime connection: %v", err)
		}
	}
}
