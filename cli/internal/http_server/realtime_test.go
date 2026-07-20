package http_server

import (
	"context"
	"encoding/json"
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
	"hmans.de/chatto/internal/publiccursor"
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
	for {
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
		pong := frame.GetPong()
		if pong == nil {
			continue
		}
		if pong.Nonce != nonce {
			return fmt.Errorf("pong nonce length = %d, want %d", len(pong.GetNonce()), len(nonce))
		}
		return nil
	}
}

func subscribeRealtime(t testing.TB, conn *websocket.Conn, token string, retainedRoomIDs ...string) string {
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
	} else if want := realtimeServerCapabilities; !slices.Equal(got.GetCapabilities(), want) {
		t.Fatalf("realtime capabilities = %v, want %v", got.GetCapabilities(), want)
	}

	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{RetainedRoomIds: retainedRoomIDs},
	}})
	subscribed, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for realtime subscribed")
	}
	if subscribed.GetSubscribed() == nil {
		t.Fatalf("second realtime frame = %T (%+v), want subscribed", subscribed.GetFrame(), subscribed)
	}
	for {
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for realtime caught_up")
		}
		if frame.GetCaughtUp() != nil {
			return frame.GetCaughtUp().GetCursor()
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

func waitRealtimeProjectionEvent(t testing.TB, conn *websocket.Conn, timeout time.Duration, match func(*realtimev1.RealtimeProjectionEvent) bool) *realtimev1.RealtimeProjectionEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		frame, ok := readRealtimeServerFrame(t, conn, time.Until(deadline))
		if !ok {
			return nil
		}
		if projection := frame.GetProjectionEvent(); projection != nil && match(projection) {
			return projection
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

func TestRealtimeTransientMapperRejectsDurableEvents(t *testing.T) {
	_, err := (&HTTPServer{}).realtimeEventEnvelope(context.Background(), "", core.NewEVTEventEnvelope(&corev1.Event{
		Id: "thread-created-1",
		Event: &corev1.Event_ThreadCreated{ThreadCreated: &corev1.ThreadCreatedEvent{
			RoomId: "R1", ThreadRootEventId: "M1",
		}},
	}))
	if err == nil {
		t.Fatal("durable event was accepted by transient mapper")
	}
}

func TestRealtimeProjectionMapsDurableCallTransition(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-call-projection", "RT Call Projection", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	event := core.NewEVTEventEnvelope(&corev1.Event{
		Id: "call-started-1",
		Event: &corev1.Event_VoiceCallStarted{VoiceCallStarted: &corev1.CallStartedEvent{
			RoomId: "R1", CallId: "call-1",
		}},
	})
	frame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, viewer.Id, event)
	if err != nil {
		t.Fatalf("realtimeProjectionFrameForEvent: %v", err)
	}
	if !handled || frame.GetProjectionEvent() == nil || len(frame.GetProjectionEvent().GetOperations()) != 1 {
		t.Fatalf("call projection frame = %+v, handled=%v", frame, handled)
	}
	if frame.GetProjectionEvent().GetOperations()[0].GetActiveCallsReplace() == nil {
		t.Fatalf("call projection operation = %T, want active_calls_replace", frame.GetProjectionEvent().GetOperations()[0].GetOperation())
	}
}

func TestRealtimeTransientMapperRejectsProjectionOwnedLiveEvents(t *testing.T) {
	tests := []struct {
		name  string
		event *corev1.LiveEvent
	}{
		{"notification created", &corev1.LiveEvent{Event: &corev1.LiveEvent_NotificationCreated{NotificationCreated: &corev1.NotificationCreatedEvent{NotificationId: "N1"}}}},
		{"notification dismissed", &corev1.LiveEvent{Event: &corev1.LiveEvent_NotificationDismissed{NotificationDismissed: &corev1.NotificationDismissedEvent{NotificationId: "N1"}}}},
		{"notification level", &corev1.LiveEvent{Event: &corev1.LiveEvent_NotificationLevelChanged{NotificationLevelChanged: &corev1.NotificationLevelChangedEvent{RoomId: "R1"}}}},
		{"thread follow", &corev1.LiveEvent{Event: &corev1.LiveEvent_ThreadFollowChanged{ThreadFollowChanged: &corev1.ThreadFollowChangedEvent{RoomId: "R1", ThreadRootEventId: "M1"}}}},
		{"room read", &corev1.LiveEvent{Event: &corev1.LiveEvent_RoomMarkedAsRead{RoomMarkedAsRead: &corev1.RoomMarkedAsReadEvent{RoomId: "R1"}}}},
		{"server updated", &corev1.LiveEvent{Event: &corev1.LiveEvent_ServerUpdated{ServerUpdated: &corev1.ServerUpdatedEvent{}}}},
		{"profile updated", &corev1.LiveEvent{Event: &corev1.LiveEvent_UserProfileUpdated{UserProfileUpdated: &corev1.UserProfileUpdatedEvent{UserId: "U1"}}}},
		{"preferences updated", &corev1.LiveEvent{Event: &corev1.LiveEvent_ServerUserPreferencesUpdated{ServerUserPreferencesUpdated: &corev1.ServerUserPreferencesUpdatedEvent{}}}},
		{"room groups updated", &corev1.LiveEvent{Event: &corev1.LiveEvent_RoomGroupsUpdated{RoomGroupsUpdated: &corev1.RoomGroupsUpdatedEvent{}}}},
		{"member deleted", &corev1.LiveEvent{Event: &corev1.LiveEvent_ServerMemberDeleted{ServerMemberDeleted: &corev1.ServerMemberDeletedEvent{UserId: "U1"}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (&HTTPServer{}).realtimeEventEnvelope(context.Background(), "U1", core.NewLiveEventEnvelope(test.event))
			if err == nil {
				t.Fatal("projection-owned live event was accepted by transient mapper")
			}
		})
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

func TestRealtimeWebSocketBoundsWholeCatchUpDuration(t *testing.T) {
	env := setupWebSocketTestServer(t)
	env.httpServer.realtimeCatchUps.timeout = -time.Nanosecond
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-catch-up-timeout", "RT Catch Up Timeout", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	conn := env.connectRealtime(t)
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	if frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatalf("hello response = %+v, want server hello", frame)
	}
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{},
	}})
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetClose().GetCode() != "catch_up_timeout" || !frame.GetClose().GetReconnect() || frame.GetClose().GetRetryAfterMs() == 0 {
		t.Fatalf("catch-up timeout response = %+v, want reconnectable catch_up_timeout", frame)
	}
}

func TestRealtimeWebSocketRateLimitsStaleCursorReuse(t *testing.T) {
	env := setupWebSocketTestServer(t)
	env.httpServer.realtimeCatchUps = newRealtimeCatchUpAdmissionWithLimits(2, 1, time.Hour, time.Now)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-catch-up-rate", "RT Catch Up Rate", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	first := env.connectRealtime(t)
	staleCursor := subscribeRealtime(t, first, token)
	if err := first.Close(); err != nil {
		t.Fatalf("close first realtime connection: %v", err)
	}
	if _, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-catch-up-rate-event", "RT Catch Up Rate Event", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	replay := env.connectRealtime(t)
	sendRealtimeClientFrame(t, replay, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	if frame, ok := readRealtimeServerFrame(t, replay, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatalf("replay hello response = %+v, want server hello", frame)
	}
	sendRealtimeClientFrame(t, replay, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{ResumeCursor: &staleCursor},
	}})
	for {
		frame, ok := readRealtimeServerFrame(t, replay, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for stale-cursor replay caught_up")
		}
		if frame.GetCaughtUp() != nil {
			break
		}
	}
	if err := replay.Close(); err != nil {
		t.Fatalf("close replay connection: %v", err)
	}

	limited := env.connectRealtime(t)
	sendRealtimeClientFrame(t, limited, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	if frame, ok := readRealtimeServerFrame(t, limited, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatalf("hello response = %+v, want server hello", frame)
	}
	sendRealtimeClientFrame(t, limited, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{ResumeCursor: &staleCursor},
	}})
	frame, ok := readRealtimeServerFrame(t, limited, 5*time.Second)
	if !ok || frame.GetClose().GetCode() != "catch_up_rate_limited" || !frame.GetClose().GetReconnect() || frame.GetClose().GetRetryAfterMs() == 0 {
		t.Fatalf("stale-cursor reuse response = %+v, want reconnectable catch_up_rate_limited", frame)
	}
}

func TestRealtimeWebSocketAllowsCurrentBoundaryReconnectAfterRateLimitBurst(t *testing.T) {
	env := setupWebSocketTestServer(t)
	env.httpServer.realtimeCatchUps = newRealtimeCatchUpAdmissionWithLimits(2, 1, time.Hour, time.Now)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-current-reconnect", "RT Current Reconnect", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	first := env.connectRealtime(t)
	resumeCursor := subscribeRealtime(t, first, token)
	if err := first.Close(); err != nil {
		t.Fatalf("close first realtime connection: %v", err)
	}
	release, admissionErr := env.httpServer.realtimeCatchUps.acquire(user.Id, true)
	if admissionErr != nil {
		t.Fatalf("consume replay rate token: %+v", admissionErr)
	}
	release()

	reconnected := env.connectRealtime(t)
	sendRealtimeClientFrame(t, reconnected, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	if frame, ok := readRealtimeServerFrame(t, reconnected, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatalf("hello response = %+v, want server hello", frame)
	}
	sendRealtimeClientFrame(t, reconnected, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{ResumeCursor: &resumeCursor},
	}})
	if frame, ok := readRealtimeServerFrame(t, reconnected, 5*time.Second); !ok || frame.GetSubscribed() == nil {
		t.Fatalf("current-boundary reconnect response = %+v, want subscribed", frame)
	}
	for {
		frame, ok := readRealtimeServerFrame(t, reconnected, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for current-boundary reconnect caught_up")
		}
		if frame.GetCaughtUp() != nil {
			break
		}
		if frame.GetProjectionEvent() == nil {
			t.Fatalf("reconnect frame = %T, want projection_event or caught_up", frame.GetFrame())
		}
	}
}

func TestRealtimeWebSocketRejectsVersionOne(t *testing.T) {
	env := setupWebSocketTestServer(t)
	conn := env.connectRealtime(t)
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: 1},
	}})
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetError().GetCode() != "unsupported_protocol" || !frame.GetError().GetFatal() {
		t.Fatalf("v1 response = %+v, want fatal unsupported_protocol", frame)
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

	frames, err := env.httpServer.realtimeProjectionSnapshotFrames(env.ctx, viewer.Id, []string{room.Id})
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
		if notifications := operation.GetNotificationsReplace(); notifications != nil {
			hasNotifications = true
			if notifications.GetChange() != nil {
				t.Fatalf("snapshot notification replacement carried live transition metadata: %+v", notifications.GetChange())
			}
		}
		if timeline := operation.GetRoomTimelineReplace(); timeline.GetRoomId() == room.Id {
			hasTimeline = timeline.GetEventCursors()[message.Id] != ""
		}
	}
	if !hasServer || !hasServerState || !hasViewer || !hasViewerUser || !hasRoom || !hasGroups || !hasNotifications || !hasTimeline {
		t.Fatalf("snapshot coverage: server=%v server_state=%v viewer=%v user=%v room=%v groups=%v notifications=%v timeline=%v", hasServer, hasServerState, hasViewer, hasViewerUser, hasRoom, hasGroups, hasNotifications, hasTimeline)
	}
}

func TestRealtimeProjectionSnapshotFramesKeepTimelinesAndChannelMembershipLazy(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-lazy-snapshot", "RT Lazy Snapshot", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-lazy-snapshot-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, viewer.Id, "lazy snapshot message", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	frames, err := env.httpServer.realtimeProjectionSnapshotFrames(env.ctx, viewer.Id, nil)
	if err != nil {
		t.Fatalf("realtimeProjectionSnapshotFrames: %v", err)
	}
	for _, frame := range frames {
		for _, operation := range frame.GetProjectionEvent().GetOperations() {
			if timeline := operation.GetRoomTimelineReplace(); timeline != nil {
				t.Fatalf("cold snapshot unexpectedly hydrated timeline %q", timeline.GetRoomId())
			}
			if projectedRoom := operation.GetRoomUpsert(); projectedRoom.GetRoom().GetRoom().GetId() == room.Id && len(projectedRoom.GetMemberUserIds()) != 0 {
				t.Fatalf("cold channel membership = %v, want lazy empty membership", projectedRoom.GetMemberUserIds())
			}
		}
	}
}

func TestRealtimeRetainedRoomSetIsBoundedAndValidated(t *testing.T) {
	rooms, err := realtimeRetainedRoomSet([]string{"R1", "R1", " R2 "})
	if err != nil {
		t.Fatalf("realtimeRetainedRoomSet: %v", err)
	}
	if len(rooms) != 2 {
		t.Fatalf("deduplicated retained rooms = %v, want R1 and R2", rooms)
	}
	if _, ok := rooms["R2"]; !ok {
		t.Fatalf("trimmed retained rooms = %v, want R2", rooms)
	}
	if _, err := realtimeRetainedRoomSet([]string{""}); err == nil {
		t.Fatal("empty retained room ID was accepted")
	}
	if _, err := realtimeRetainedRoomSet(make([]string, realtimeMaxRetainedRooms+1)); err == nil {
		t.Fatal("oversized retained room set was accepted")
	}
}

func TestRealtimeWebSocketHydrationRejectionIdentifiesRoomAndRetryDelay(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-hydration-retry", "RT Hydration Retry", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-hydration-retry-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	conn := env.dialRealtime(t)
	t.Cleanup(func() { conn.Close() })
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	if frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatal("did not receive realtime hello")
	}
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{},
	}})
	if frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second); !ok || frame.GetSubscribed() == nil {
		t.Fatal("did not receive realtime subscribed")
	}
	readRealtimeCaughtUp(t, conn)

	release, admissionErr := env.httpServer.realtimeCatchUps.acquireHydration(viewer.Id)
	if admissionErr != nil {
		t.Fatalf("reserve hydration admission: %v", admissionErr)
	}
	t.Cleanup(release)
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_HydrateRoom{
		HydrateRoom: &realtimev1.RealtimeHydrateRoom{RoomId: room.Id},
	}})

	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetError() == nil {
		t.Fatalf("hydration rejection frame = %+v", frame)
	}
	realtimeError := frame.GetError()
	if realtimeError.GetCode() != "room_hydration_in_progress" {
		t.Fatalf("hydration rejection code = %q", realtimeError.GetCode())
	}
	if realtimeError.GetRoomId() != room.Id {
		t.Fatalf("hydration rejection room = %q, want %q", realtimeError.GetRoomId(), room.Id)
	}
	if realtimeError.GetRetryAfterMs() == 0 {
		t.Fatal("hydration rejection omitted retry delay")
	}
}

func TestRealtimeWebSocketHydratesRoomLazilyAndFiltersOtherTimelines(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-lazy-room", "RT Lazy Room", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	retainedRoom, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-lazy-retained", "")
	if err != nil {
		t.Fatalf("CreateRoom retained: %v", err)
	}
	otherRoom, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-lazy-other", "")
	if err != nil {
		t.Fatalf("CreateRoom other: %v", err)
	}
	for _, room := range []*corev1.Room{retainedRoom, otherRoom} {
		if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", room.Id, err)
		}
	}
	beforeHydration, err := env.core.PostMessage(env.ctx, core.KindChannel, retainedRoom.Id, viewer.Id, "before hydration", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage before hydration: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	conn := env.dialRealtime(t)
	t.Cleanup(func() { conn.Close() })
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	if frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatal("did not receive realtime hello")
	}
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{},
	}})
	if frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second); !ok || frame.GetSubscribed() == nil {
		t.Fatal("did not receive realtime subscribed")
	}
	readRealtimeCaughtUp(t, conn)

	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_HydrateRoom{
		HydrateRoom: &realtimev1.RealtimeHydrateRoom{RoomId: retainedRoom.Id},
	}})
	hydratedFrame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || hydratedFrame.GetProjectionEvent() == nil {
		t.Fatalf("room hydration frame = %+v", hydratedFrame)
	}
	var hydratedTimeline *realtimev1.RealtimeProjectionRoomTimelineReplace
	var hydratedRoom *realtimev1.RealtimeProjectionRoom
	for _, operation := range hydratedFrame.GetProjectionEvent().GetOperations() {
		if operation.GetRoomTimelineReplace().GetRoomId() == retainedRoom.Id {
			hydratedTimeline = operation.GetRoomTimelineReplace()
		}
		if operation.GetRoomUpsert().GetRoom().GetRoom().GetId() == retainedRoom.Id {
			hydratedRoom = operation.GetRoomUpsert()
		}
	}
	hasBeforeHydration := false
	for _, event := range hydratedTimeline.GetPage().GetEvents() {
		hasBeforeHydration = hasBeforeHydration || event.GetId() == beforeHydration.Id
	}
	if hydratedTimeline == nil || !hasBeforeHydration {
		t.Fatalf("hydrated timeline = %+v, want message %q", hydratedTimeline, beforeHydration.Id)
	}
	if hydratedRoom == nil || !slices.Contains(hydratedRoom.GetMemberUserIds(), viewer.Id) {
		t.Fatalf("hydrated room membership = %+v, want viewer", hydratedRoom)
	}

	afterHydration, err := env.core.PostMessage(env.ctx, core.KindChannel, retainedRoom.Id, viewer.Id, "after hydration", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage after hydration: %v", err)
	}
	for {
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for retained room update")
		}
		projection := frame.GetProjectionEvent()
		if projection == nil {
			continue
		}
		found := false
		for _, operation := range projection.GetOperations() {
			upsert := operation.GetRoomTimelineEventUpsert()
			found = found || (upsert.GetRoomId() == retainedRoom.Id && upsert.GetEvent().GetId() == afterHydration.Id)
		}
		if found {
			break
		}
	}

	if _, err := env.core.PostMessage(env.ctx, core.KindChannel, otherRoom.Id, viewer.Id, "unretained update", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage unretained: %v", err)
	}
	for {
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for unretained cursor advance")
		}
		projection := frame.GetProjectionEvent()
		if projection == nil || projection.GetResumeCursor() == "" {
			continue
		}
		foundRoomSummary := false
		foundRoomActivity := false
		for _, operation := range projection.GetOperations() {
			if operation.GetRoomTimelineEventUpsert() != nil || operation.GetRoomTimelineEventRemove() != nil || operation.GetRoomTimelineReplace() != nil {
				t.Fatalf("unretained projection leaked timeline operation: %+v", operation)
			}
			room := operation.GetRoomUpsert().GetRoom().GetRoom()
			foundRoomSummary = foundRoomSummary || room.GetId() == otherRoom.Id
			foundRoomActivity = foundRoomActivity || operation.GetRoomActivity().GetRoomId() == otherRoom.Id
		}
		if !foundRoomSummary {
			t.Fatal("unretained message did not refresh its lightweight room summary")
		}
		if !foundRoomActivity {
			t.Fatal("unretained root message did not emit lightweight room activity")
		}
		break
	}

	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_HydrateRoom{
		HydrateRoom: &realtimev1.RealtimeHydrateRoom{RoomId: retainedRoom.Id},
	}})
	if duplicate, ok := readRealtimeServerFrame(t, conn, 200*time.Millisecond); ok {
		t.Fatalf("duplicate hydration unexpectedly rebuilt the room: %+v", duplicate)
	}
}

func TestRealtimeWebSocketMaterializesRetainedRoomAfterViewerGainsAccess(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-lazy-owner", "RT Lazy Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-lazy-future-member", "RT Lazy Future Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, owner.Id, core.KindChannel, "", "rt-lazy-future-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, owner.Id, core.KindChannel, owner.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom owner: %v", err)
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, owner.Id, "visible after joining", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	viewerToken, err := env.core.CreateAuthToken(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, viewerToken)
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_HydrateRoom{
		HydrateRoom: &realtimev1.RealtimeHydrateRoom{RoomId: room.Id},
	}})
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetError().GetCode() != "room_unavailable" || frame.GetError().GetFatal() {
		t.Fatalf("pre-membership hydration frame = %+v, want non-fatal room_unavailable", frame)
	}

	if _, err := env.core.AddMember(env.ctx, owner.Id, core.KindChannel, room.Id, viewer.Id); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame, ok := readRealtimeServerFrame(t, conn, time.Until(deadline))
		if !ok {
			break
		}
		projection := frame.GetProjectionEvent()
		if projection == nil {
			continue
		}
		var hasRoom, hasMessage bool
		for _, operation := range projection.GetOperations() {
			if upsert := operation.GetRoomUpsert(); upsert.GetRoom().GetRoom().GetId() == room.Id {
				hasRoom = slices.Contains(upsert.GetMemberUserIds(), viewer.Id)
			}
			if timeline := operation.GetRoomTimelineReplace(); timeline.GetRoomId() == room.Id {
				for _, event := range timeline.GetPage().GetEvents() {
					hasMessage = hasMessage || event.GetId() == message.Id
				}
			}
		}
		if hasRoom && hasMessage {
			return
		}
	}
	t.Fatal("retained room did not materialize after the viewer gained access")
}

func TestRealtimeWebSocketUniversalMembershipTransitionsScrubAndRestoreOnlyRetainedTimelines(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-universal-owner", "RT Universal Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	implicitViewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-universal-implicit", "RT Universal Implicit", "password123")
	if err != nil {
		t.Fatalf("CreateUser implicit viewer: %v", err)
	}
	explicitViewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-universal-explicit", "RT Universal Explicit", "password123")
	if err != nil {
		t.Fatalf("CreateUser explicit viewer: %v", err)
	}
	lazyViewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-universal-lazy", "RT Universal Lazy", "password123")
	if err != nil {
		t.Fatalf("CreateUser lazy viewer: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, owner.Id, core.KindChannel, "", "rt-universal-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, owner.Id, core.KindChannel, owner.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom owner: %v", err)
	}
	if _, err := env.core.AddMember(env.ctx, owner.Id, core.KindChannel, room.Id, explicitViewer.Id); err != nil {
		t.Fatalf("AddMember explicit viewer: %v", err)
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, owner.Id, "universal retained plaintext", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if _, err := env.core.SetRoomUniversal(env.ctx, owner.Id, core.KindChannel, room.Id, true); err != nil {
		t.Fatalf("SetRoomUniversal true: %v", err)
	}

	tokenFor := func(userID string) string {
		t.Helper()
		token, err := env.core.CreateAuthToken(env.ctx, userID)
		if err != nil {
			t.Fatalf("CreateAuthToken %s: %v", userID, err)
		}
		return token
	}
	implicitConn := env.connectRealtime(t)
	explicitConn := env.connectRealtime(t)
	lazyConn := env.connectRealtime(t)
	subscribeRealtime(t, implicitConn, tokenFor(implicitViewer.Id), room.Id)
	subscribeRealtime(t, explicitConn, tokenFor(explicitViewer.Id), room.Id)
	subscribeRealtime(t, lazyConn, tokenFor(lazyViewer.Id))

	roomTransition := func(conn *websocket.Conn) *realtimev1.RealtimeProjectionEvent {
		t.Helper()
		return waitRealtimeProjectionEvent(t, conn, 5*time.Second, func(projection *realtimev1.RealtimeProjectionEvent) bool {
			for _, operation := range projection.GetOperations() {
				projectedRoom := operation.GetRoomUpsert().GetRoom().GetRoom()
				if projectedRoom.GetId() == room.Id {
					return true
				}
			}
			return false
		})
	}
	assertTransition := func(name string, projection *realtimev1.RealtimeProjectionEvent, wantMember, wantTimeline, wantMessage bool) {
		t.Helper()
		if projection == nil {
			t.Fatalf("%s: timed out waiting for universal projection", name)
		}
		var gotRoom, gotTimeline, gotMessage, gotCalls, gotNotifications bool
		for _, operation := range projection.GetOperations() {
			if upsert := operation.GetRoomUpsert(); upsert.GetRoom().GetRoom().GetId() == room.Id {
				gotRoom = true
				if got := upsert.GetRoom().GetViewerState().GetIsMember(); got != wantMember {
					t.Fatalf("%s: viewer membership = %t, want %t", name, got, wantMember)
				}
			}
			if timeline := operation.GetRoomTimelineReplace(); timeline.GetRoomId() == room.Id {
				gotTimeline = true
				for _, event := range timeline.GetPage().GetEvents() {
					gotMessage = gotMessage || event.GetId() == message.Id
				}
			}
			gotCalls = gotCalls || operation.GetActiveCallsReplace() != nil
			gotNotifications = gotNotifications || operation.GetNotificationsReplace() != nil
		}
		if !gotRoom || gotTimeline != wantTimeline || gotMessage != wantMessage {
			t.Fatalf("%s: room=%t timeline=%t message=%t, want room=true timeline=%t message=%t; operations=%+v", name, gotRoom, gotTimeline, gotMessage, wantTimeline, wantMessage, projection.GetOperations())
		}
		if !wantMember && (!gotCalls || !gotNotifications) {
			t.Fatalf("%s: revocation omitted active-call/notification replacements; operations=%+v", name, projection.GetOperations())
		}
	}

	if _, err := env.core.SetRoomUniversal(env.ctx, owner.Id, core.KindChannel, room.Id, false); err != nil {
		t.Fatalf("SetRoomUniversal false: %v", err)
	}
	assertTransition("implicit revocation", roomTransition(implicitConn), false, true, false)
	assertTransition("explicit membership survives", roomTransition(explicitConn), true, true, true)
	assertTransition("unretained revocation stays lazy", roomTransition(lazyConn), false, false, false)

	if _, err := env.core.SetRoomUniversal(env.ctx, owner.Id, core.KindChannel, room.Id, true); err != nil {
		t.Fatalf("SetRoomUniversal restore: %v", err)
	}
	assertTransition("implicit retained restoration", roomTransition(implicitConn), true, true, true)
	assertTransition("explicit retained restoration", roomTransition(explicitConn), true, true, true)
	assertTransition("unretained restoration stays lazy", roomTransition(lazyConn), true, false, false)
}

func TestRealtimeWebSocketResumeResetsRetainedTimelineAfterUniversalMembershipRevocation(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-universal-replay-owner", "RT Universal Replay Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-universal-replay-viewer", "RT Universal Replay Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, owner.Id, core.KindChannel, "", "rt-universal-replay-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, owner.Id, core.KindChannel, owner.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom owner: %v", err)
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, owner.Id, "replayed revocation plaintext", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if _, err := env.core.SetRoomUniversal(env.ctx, owner.Id, core.KindChannel, room.Id, true); err != nil {
		t.Fatalf("SetRoomUniversal true: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	boundaryConn := env.connectRealtime(t)
	resumeCursor := subscribeRealtime(t, boundaryConn, token, room.Id)
	if resumeCursor == "" {
		t.Fatal("initial retained projection returned no resume cursor")
	}
	if err := boundaryConn.Close(); err != nil {
		t.Fatalf("close boundary connection: %v", err)
	}
	if _, err := env.core.SetRoomUniversal(env.ctx, owner.Id, core.KindChannel, room.Id, false); err != nil {
		t.Fatalf("SetRoomUniversal false: %v", err)
	}

	resumed := env.connectRealtime(t)
	sendRealtimeClientFrame(t, resumed, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	if frame, ok := readRealtimeServerFrame(t, resumed, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatal("did not receive replay hello")
	}
	sendRealtimeClientFrame(t, resumed, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{ResumeCursor: &resumeCursor, RetainedRoomIds: []string{room.Id}},
	}})
	if frame, ok := readRealtimeServerFrame(t, resumed, 5*time.Second); !ok || frame.GetSubscribed() == nil || frame.GetSubscribed().GetStartCursor() == resumeCursor {
		t.Fatalf("authorization-sensitive resume did not select a new compacted boundary: %+v", frame)
	}

	var foundReset, foundRevokedRoom bool
	for {
		frame, ok := readRealtimeServerFrame(t, resumed, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for compacted revocation reset")
		}
		if caughtUp := frame.GetCaughtUp(); caughtUp != nil {
			if !foundReset || !foundRevokedRoom || caughtUp.GetCursor() == "" {
				t.Fatalf("revocation reset incomplete before caught_up: reset=%t revoked_room=%t caught_up=%+v", foundReset, foundRevokedRoom, caughtUp)
			}
			break
		}
		projection := frame.GetProjectionEvent()
		if projection == nil {
			continue
		}
		for _, operation := range projection.GetOperations() {
			foundReset = foundReset || operation.GetReset_() != nil
			if upsert := operation.GetRoomUpsert(); upsert.GetRoom().GetRoom().GetId() == room.Id {
				foundRevokedRoom = !upsert.GetRoom().GetViewerState().GetIsMember()
			}
			if timeline := operation.GetRoomTimelineReplace(); timeline.GetRoomId() == room.Id {
				if len(timeline.GetPage().GetEvents()) != 0 {
					t.Fatalf("revocation reset leaked retained timeline: %+v", timeline)
				}
			}
		}
	}
}

func TestRealtimeWebSocketRestoresRetainedTimelineAfterUnarchive(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-retained-unarchive", "RT Retained Unarchive", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-retained-unarchive-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, viewer.Id, "survives archive", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, token, room.Id)

	if _, err := env.core.ArchiveRoom(env.ctx, viewer.Id, core.KindChannel, room.Id); err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	removed := false
	for !removed && time.Now().Before(deadline) {
		frame, ok := readRealtimeServerFrame(t, conn, time.Until(deadline))
		if !ok {
			break
		}
		for _, operation := range frame.GetProjectionEvent().GetOperations() {
			removed = removed || operation.GetRoomRemove().GetRoomId() == room.Id
		}
	}
	if !removed {
		t.Fatal("archive did not remove retained room state")
	}

	if _, err := env.core.UnarchiveRoom(env.ctx, viewer.Id, core.KindChannel, room.Id); err != nil {
		t.Fatalf("UnarchiveRoom: %v", err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame, ok := readRealtimeServerFrame(t, conn, time.Until(deadline))
		if !ok {
			break
		}
		for _, operation := range frame.GetProjectionEvent().GetOperations() {
			timeline := operation.GetRoomTimelineReplace()
			if timeline.GetRoomId() != room.Id {
				continue
			}
			for _, event := range timeline.GetPage().GetEvents() {
				if event.GetId() == message.Id {
					return
				}
			}
		}
	}
	t.Fatal("unarchive did not restore the retained room timeline")
}

func TestRealtimeWebSocketAdvancesPastRetainedUnarchiveForNonMember(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-unarchive-owner", "RT Unarchive Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-unarchive-nonmember", "RT Unarchive Nonmember", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, owner.Id, core.KindChannel, "", "rt-unarchive-nonmember-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{owner.Id, viewer.Id} {
		if _, err := env.core.JoinRoom(env.ctx, userID, core.KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, token, room.Id)

	if err := env.core.LeaveRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	left := waitRealtimeRoomUpsert(t, conn, 5*time.Second, func(upsert *realtimev1.RealtimeProjectionRoom) bool {
		return upsert.GetRoom().GetRoom().GetId() == room.Id && !upsert.GetRoom().GetViewerState().GetIsMember()
	})
	if left == nil {
		t.Fatal("viewer did not receive non-member room state after leaving")
	}
	if _, err := env.core.ArchiveRoom(env.ctx, owner.Id, core.KindChannel, room.Id); err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame, ok := readRealtimeServerFrame(t, conn, time.Until(deadline))
		if !ok {
			break
		}
		removed := false
		for _, operation := range frame.GetProjectionEvent().GetOperations() {
			removed = removed || operation.GetRoomRemove().GetRoomId() == room.Id
		}
		if removed {
			break
		}
	}
	if _, err := env.core.UnarchiveRoom(env.ctx, owner.Id, core.KindChannel, room.Id); err != nil {
		t.Fatalf("UnarchiveRoom: %v", err)
	}
	unarchived := waitRealtimeRoomUpsert(t, conn, 5*time.Second, func(upsert *realtimev1.RealtimeProjectionRoom) bool {
		return upsert.GetRoom().GetRoom().GetId() == room.Id && !upsert.GetRoom().GetRoom().GetArchived() && !upsert.GetRoom().GetViewerState().GetIsMember()
	})
	if unarchived == nil {
		t.Fatal("non-member did not receive unarchived room summary")
	}
	if err := realtimePingRoundTrip(conn, "after-nonmember-unarchive"); err != nil {
		t.Fatalf("realtime stream did not continue after non-member unarchive: %v", err)
	}
}

func TestRealtimeProjectionNotificationChangesReplaceStateAndCarryLiveTransitions(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-notification-viewer", "RT Notification Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-notification-author", "RT Notification Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-notification-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{viewer.Id, author.Id} {
		if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %q: %v", userID, err)
		}
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, author.Id, "notify me", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := env.core.FollowThread(env.ctx, core.KindChannel, viewer.Id, room.Id, message.Id); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}
	notification, err := env.core.CreateNotification(env.ctx, viewer.Id, author.Id, &corev1.Notification{
		Notification: &corev1.Notification_Reply{Reply: &corev1.ReplyNotification{
			RoomId: room.Id, EventId: message.Id, InReplyToId: message.Id, InThread: message.Id,
		}},
	})
	if err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}

	frame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, viewer.Id, core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id: "notification-created-1", ActorId: author.Id,
		Event: &corev1.LiveEvent_NotificationCreated{NotificationCreated: &corev1.NotificationCreatedEvent{
			NotificationId: notification.Id, RoomId: room.Id, EventId: message.Id, InReplyToId: message.Id, Silent: true,
		}},
	}))
	if err != nil {
		t.Fatalf("realtimeProjectionFrameForEvent: %v", err)
	}
	if !handled {
		t.Fatal("notification-created event was not handled as a projection mutation")
	}
	replacement := frame.GetProjectionEvent().GetOperations()[0].GetNotificationsReplace()
	if replacement == nil || len(replacement.GetPage().GetNotifications()) != 1 {
		t.Fatalf("notification replacement = %+v, want authoritative one-row page", replacement)
	}
	change := replacement.GetChange()
	if change.GetAction() != realtimev1.RealtimeProjectionNotificationAction_REALTIME_PROJECTION_NOTIFICATION_ACTION_CREATED || change.GetNotificationId() != notification.Id || !change.GetSilent() {
		t.Fatalf("notification transition = %+v", change)
	}
	threadStates := frame.GetProjectionEvent().GetOperations()[1].GetThreadViewerStatesReplace()
	if threadStates == nil || len(threadStates.GetStates()) != 1 || threadStates.GetStates()[0].GetThreadRootEventId() != message.Id {
		t.Fatalf("reply notification thread-state replacement = %+v, want followed thread %q", threadStates, message.Id)
	}

	dismissed, err := env.core.DismissNotification(env.ctx, viewer.Id, notification.Id)
	if err != nil || !dismissed {
		t.Fatalf("DismissNotification: dismissed=%v err=%v", dismissed, err)
	}
	dismissFrame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, viewer.Id, core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id: "notification-dismissed-1", ActorId: viewer.Id,
		Event: &corev1.LiveEvent_NotificationDismissed{NotificationDismissed: &corev1.NotificationDismissedEvent{
			NotificationId: notification.Id,
		}},
	}))
	if err != nil {
		t.Fatalf("realtimeProjectionFrameForEvent dismissed: %v", err)
	}
	if !handled {
		t.Fatal("notification-dismissed event was not handled as a projection mutation")
	}
	dismissReplacement := dismissFrame.GetProjectionEvent().GetOperations()[0].GetNotificationsReplace()
	if dismissReplacement == nil || len(dismissReplacement.GetPage().GetNotifications()) != 0 {
		t.Fatalf("dismissed notification replacement = %+v, want authoritative empty page", dismissReplacement)
	}
	dismissChange := dismissReplacement.GetChange()
	if dismissChange.GetAction() != realtimev1.RealtimeProjectionNotificationAction_REALTIME_PROJECTION_NOTIFICATION_ACTION_DISMISSED || dismissChange.GetNotificationId() != notification.Id || dismissChange.GetSilent() {
		t.Fatalf("dismissed notification transition = %+v", dismissChange)
	}
}

func TestRealtimeProjectionNotificationLevelChangedReplacesViewer(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-notification-level-viewer", "RT Notification Level Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	frame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, viewer.Id, core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id: "notification-level-1", ActorId: viewer.Id,
		Event: &corev1.LiveEvent_NotificationLevelChanged{NotificationLevelChanged: &corev1.NotificationLevelChangedEvent{}},
	}))
	if err != nil {
		t.Fatalf("realtimeProjectionFrameForEvent: %v", err)
	}
	if !handled || frame.GetProjectionEvent().GetOperations()[0].GetViewerUpsert() == nil {
		t.Fatalf("notification-level projection = %+v, handled=%v; want viewer_upsert", frame, handled)
	}
}

func TestRealtimeProjectionThreadFollowReplacesStateForUnretainedRoom(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-thread-follow-viewer", "RT Thread Follow Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-thread-follow-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, viewer.Id, "thread root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := env.core.FollowThread(env.ctx, core.KindChannel, viewer.Id, room.Id, root.Id); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}

	frame, handled, err := env.httpServer.realtimeProjectionFrameForEventWithRooms(env.ctx, viewer.Id, core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id: "thread-follow-1", ActorId: viewer.Id,
		Event: &corev1.LiveEvent_ThreadFollowChanged{ThreadFollowChanged: &corev1.ThreadFollowChangedEvent{
			RoomId: room.Id, ThreadRootEventId: root.Id, IsFollowing: true,
		}},
	}), map[string]struct{}{})
	if err != nil {
		t.Fatalf("realtimeProjectionFrameForEventWithRooms: %v", err)
	}
	operations := frame.GetProjectionEvent().GetOperations()
	if !handled || len(operations) != 1 || operations[0].GetThreadViewerStatesReplace() == nil {
		t.Fatalf("unretained thread-follow projection = %+v, handled=%v", frame, handled)
	}
	states := operations[0].GetThreadViewerStatesReplace().GetStates()
	if len(states) != 1 || states[0].GetRoomId() != room.Id || states[0].GetThreadRootEventId() != root.Id || !states[0].GetViewerState().GetIsFollowing() {
		t.Fatalf("thread viewer states = %+v", states)
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

func TestRealtimeThreadReadMarkerPublishesProjectionUpdate(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-thread-read-viewer", "RT Thread Read Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-thread-read-author", "RT Thread Read Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-thread-read-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{viewer.Id, author.Id} {
		if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %q: %v", userID, err)
		}
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, viewer.Id, "thread root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, author.Id, "unread reply", nil, root.Id, "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage reply: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.dialRealtime(t)
	t.Cleanup(func() { conn.Close() })
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	if frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatal("did not receive hello")
	}
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{RetainedRoomIds: []string{room.Id}},
	}})
	if frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second); !ok || frame.GetSubscribed() == nil {
		t.Fatal("did not receive subscribed")
	}
	readRealtimeCaughtUp(t, conn)

	if _, err := env.core.SetThreadLastReadEventID(env.ctx, core.KindChannel, viewer.Id, room.Id, root.Id, reply.Id); err != nil {
		t.Fatalf("SetThreadLastReadEventID: %v", err)
	}
	upsert := waitRealtimeTimelineUpsert(t, conn, 5*time.Second, func(upsert *realtimev1.RealtimeProjectionRoomTimelineEventUpsert) bool {
		return upsert.GetRoomId() == room.Id && upsert.GetEvent().GetId() == root.Id
	})
	thread := upsert.GetEvent().GetMessagePosted().GetMessage().GetThread()
	if !thread.GetViewerState().GetIsFollowing() || thread.GetViewerState().GetHasUnread() {
		t.Fatalf("thread viewer state after marker advance = %+v, want following and read", thread.GetViewerState())
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
	subscribeRealtime(t, conn, token, room.Id)

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
	subscribeRealtime(t, ownerConn, ownerToken, room.Id)
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
	subscribeRealtime(t, conn, token, room.Id)

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
	subscribeRealtime(t, conn, token, room.Id)

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
	subscribeRealtime(t, conn, token, room.Id)

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
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{ResumeCursor: &resumeCursor, RetainedRoomIds: []string{room.Id}},
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
	reconciliation, ok := readRealtimeServerFrame(t, resumed, 5*time.Second)
	foundNotifications := false
	if ok && reconciliation.GetProjectionEvent() != nil {
		for _, operation := range reconciliation.GetProjectionEvent().GetOperations() {
			foundNotifications = foundNotifications || operation.GetNotificationsReplace() != nil
		}
	}
	if !foundNotifications {
		t.Fatalf("post-replay frame = %+v, want latest-value reconciliation", reconciliation)
	}
	readRealtimeCaughtUp(t, resumed)
}

func TestRealtimeWebSocketExpiredCursorFallsBackToCompactedReset(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-expired-resume", "Expired Resume", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
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
		t.Fatal("did not receive boundary hello")
	}
	sendRealtimeClientFrame(t, boundaryConn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{},
	}})
	if frame, ok := readRealtimeServerFrame(t, boundaryConn, 5*time.Second); !ok || frame.GetSubscribed() == nil {
		t.Fatal("did not receive boundary subscribed")
	}
	boundary := readRealtimeCaughtUp(t, boundaryConn)
	if err := boundaryConn.Close(); err != nil {
		t.Fatalf("close boundary connection: %v", err)
	}

	type cursorPayload struct {
		Version        int    `json:"v"`
		StreamIdentity string `json:"i"`
		Sequence       uint64 `json:"s"`
		UserID         string `json:"u"`
		IssuedAtUnix   int64  `json:"t"`
	}
	payloadJSON, err := publiccursor.Open("test-core-secret", "realtime-resume-v2", user.Id, boundary.GetCursor())
	if err != nil {
		t.Fatalf("open boundary cursor: %v", err)
	}
	var payload cursorPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatalf("decode boundary cursor: %v", err)
	}
	payload.IssuedAtUnix = time.Now().Add(-25 * time.Hour).Unix()
	payloadJSON, err = json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode expired cursor payload: %v", err)
	}
	expiredCursor, err := publiccursor.Seal("test-core-secret", "realtime-resume-v2", user.Id, payloadJSON)
	if err != nil {
		t.Fatalf("seal expired cursor: %v", err)
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
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{ResumeCursor: &expiredCursor},
	}})
	subscribed, ok := readRealtimeServerFrame(t, resumed, 5*time.Second)
	if !ok || subscribed.GetSubscribed() == nil {
		t.Fatalf("expired resume subscribed = %+v", subscribed)
	}
	if subscribed.GetSubscribed().GetStartCursor() == expiredCursor {
		t.Fatal("expired resume retained the unusable cursor")
	}
	firstProjection, ok := readRealtimeServerFrame(t, resumed, 5*time.Second)
	if !ok || firstProjection.GetProjectionEvent() == nil {
		t.Fatalf("expired resume first projection frame = %+v", firstProjection)
	}
	operations := firstProjection.GetProjectionEvent().GetOperations()
	if len(operations) != 1 || operations[0].GetReset_() == nil {
		t.Fatalf("expired resume first operations = %+v, want reset", operations)
	}
	if firstProjection.GetProjectionEvent().GetResumeCursor() != "" {
		t.Fatal("expired resume reset exposed a cursor before the replacement snapshot completed")
	}
	if caughtUp := readRealtimeCaughtUp(t, resumed); caughtUp.GetCursor() == "" {
		t.Fatal("expired resume reset did not reach a new caught_up cursor")
	}
}

func TestRealtimeWebSocketResumesAssetAndHiddenEchoGapThenContinuesLive(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-complete-replay", "Complete Replay", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-complete-replay", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "thread root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	attachment, err := env.core.UploadAttachment(env.ctx, user.Id, room.Id, "replay.txt", "text/plain", strings.NewReader("asset"))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	assetMessage, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "asset lifecycle", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage asset: %v", err)
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
		t.Fatal("did not receive boundary hello")
	}
	sendRealtimeClientFrame(t, boundaryConn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{},
	}})
	if frame, ok := readRealtimeServerFrame(t, boundaryConn, 5*time.Second); !ok || frame.GetSubscribed() == nil {
		t.Fatal("did not receive boundary subscribed")
	}
	boundary := readRealtimeCaughtUp(t, boundaryConn)
	resumeCursor := boundary.GetCursor()
	if resumeCursor == "" {
		t.Fatal("boundary caught_up has no cursor")
	}
	if err := boundaryConn.Close(); err != nil {
		t.Fatalf("close boundary connection: %v", err)
	}

	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "hidden echo reply", nil, root.Id, "", nil, true)
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
	if err := env.core.RecordAssetProcessingStarted(env.ctx, core.SystemActorID, core.KindChannel, room.Id, assetMessage.Id, attachment.Id); err != nil {
		t.Fatalf("RecordAssetProcessingStarted: %v", err)
	}
	if err := env.core.RecordAssetProcessingFailed(env.ctx, core.SystemActorID, core.KindChannel, room.Id, assetMessage.Id, attachment.Id, corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED); err != nil {
		t.Fatalf("RecordAssetProcessingFailed: %v", err)
	}
	if err := env.core.RecordAssetDeleted(env.ctx, core.SystemActorID, core.KindChannel, room.Id, attachment.Id); err != nil {
		t.Fatalf("RecordAssetDeleted: %v", err)
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
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{ResumeCursor: &resumeCursor, RetainedRoomIds: []string{room.Id}},
	}})
	subscribed, ok := readRealtimeServerFrame(t, resumed, 5*time.Second)
	if !ok || subscribed.GetSubscribed() == nil || subscribed.GetSubscribed().GetStartCursor() != resumeCursor {
		t.Fatalf("resumed subscribed = %+v", subscribed)
	}

	assetUpserts := 0
	echoRemovals := 0
	replyUpserts := 0
	notificationReconciliations := 0
	presenceReconciliations := 0
	viewerReconciliations := 0
	roomViewerReconciliations := 0
	threadViewerReconciliations := 0
	var caughtUpCursor string
	for caughtUpCursor == "" {
		frame, ok := readRealtimeServerFrame(t, resumed, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for resumed caught_up")
		}
		if caughtUp := frame.GetCaughtUp(); caughtUp != nil {
			caughtUpCursor = caughtUp.GetCursor()
			break
		}
		projection := frame.GetProjectionEvent()
		if projection == nil {
			t.Fatalf("replay frame = %T, want projection_event or caught_up", frame.GetFrame())
		}
		for _, operation := range projection.GetOperations() {
			if operation.GetReset_() != nil {
				t.Fatal("valid resume unexpectedly emitted a compacted reset")
			}
			if remove := operation.GetRoomTimelineEventRemove(); remove != nil && remove.GetRoomId() == room.Id && remove.GetEventId() == echoID {
				echoRemovals++
				if projection.GetResumeCursor() == "" {
					t.Fatal("replayed echo removal has no resume cursor")
				}
			}
			if upsert := operation.GetRoomTimelineEventUpsert(); upsert != nil && upsert.GetRoomId() == room.Id {
				switch upsert.GetEvent().GetId() {
				case reply.Id:
					replyUpserts++
					if projection.GetResumeCursor() == "" {
						t.Fatal("replayed reply upsert has no resume cursor")
					}
				case assetMessage.Id:
					assetUpserts++
					if projection.GetResumeCursor() == "" {
						t.Fatal("replayed asset upsert has no resume cursor")
					}
					if attachments := upsert.GetEvent().GetMessagePosted().GetMessage().GetAttachments(); len(attachments) != 0 {
						t.Fatalf("replayed asset message attachments = %d, want current deleted state", len(attachments))
					}
				}
			}
			if operation.GetNotificationsReplace() != nil {
				notificationReconciliations++
			}
			if operation.GetPresencesReplace() != nil {
				presenceReconciliations++
			}
			if operation.GetViewerUpsert() != nil {
				viewerReconciliations++
			}
			if operation.GetRoomViewerStateReplace() != nil {
				roomViewerReconciliations++
			}
			if operation.GetThreadViewerStatesReplace() != nil {
				threadViewerReconciliations++
			}
		}
	}
	if caughtUpCursor == "" {
		t.Fatal("resumed caught_up has no cursor")
	}
	if caughtUpCursor == resumeCursor {
		t.Fatal("caught_up cursor did not advance across durable replay gap")
	}
	if replyUpserts != 1 || echoRemovals != 2 || assetUpserts != 3 || notificationReconciliations != 1 || presenceReconciliations != 1 || viewerReconciliations != 1 || roomViewerReconciliations == 0 || threadViewerReconciliations != 1 {
		t.Fatalf("replay reply/echo/asset/notifications/presence/viewer/room-viewer/thread-viewer = %d/%d/%d/%d/%d/%d/%d/%d, want 1/2/3/1/1/1/>0/1", replyUpserts, echoRemovals, assetUpserts, notificationReconciliations, presenceReconciliations, viewerReconciliations, roomViewerReconciliations, threadViewerReconciliations)
	}

	liveMessage, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "after caught up", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage live: %v", err)
	}
	if upsert := waitRealtimeTimelineUpsert(t, resumed, 5*time.Second, func(upsert *realtimev1.RealtimeProjectionRoomTimelineEventUpsert) bool {
		return upsert.GetRoomId() == room.Id && upsert.GetEvent().GetId() == liveMessage.Id
	}); upsert == nil {
		t.Fatal("resumed socket did not continue with live delivery after caught_up")
	}
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
	subscribeRealtime(t, conn, token, outsiderRoom.Id)

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
	env.httpServer.realtimeCatchUps = newRealtimeCatchUpAdmissionWithLimits(connectionCount, connectionCount, time.Minute, time.Now)
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
