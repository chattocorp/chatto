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
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/evtstream"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
	"hmans.de/chatto/internal/publiccursor"
)

// Keep catch-up reads open long enough to observe either the server response or
// its configured timeout response under a loaded test runner.
const realtimeTestCatchUpReadTimeout = realtimeCatchUpDefaultTimeout + 5*time.Second

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

func (env *wsTestEnv) dialRealtimeWithOrigin(t testing.TB, origin string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + realtimePath
	header := http.Header{"Origin": []string{origin}}
	for _, cookie := range env.cookieJar.Cookies(mustParseURL(env.server.URL)) {
		header.Add("Cookie", cookie.String())
	}
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		if response != nil {
			t.Fatalf("Realtime WebSocket dial failed with status %d: %v", response.StatusCode, err)
		}
		t.Fatalf("Realtime WebSocket dial failed: %v", err)
	}
	return conn
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
		frame, ok := readRealtimeServerFrame(t, conn, realtimeTestCatchUpReadTimeout)
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
		frame, ok := readRealtimeServerFrame(t, conn, realtimeTestCatchUpReadTimeout)
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

func TestRealtimeProjectionMapsThreadingModeChangeToRoomAndTimeline(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-threading-mode", "RT Threading Mode", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-threading-mode-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if _, err := env.core.SetRoomThreadingMode(env.ctx, viewer.Id, core.KindChannel, room.Id, corev1.RoomThreadingMode_ROOM_THREADING_MODE_ENCOURAGED); err != nil {
		t.Fatalf("SetRoomThreadingMode: %v", err)
	}
	events, _, err := env.core.EventPublisher.SubjectEvents(
		env.ctx,
		evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomThreadingModeChanged),
	)
	if err != nil {
		t.Fatalf("SubjectEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("threading mode events = %d, want 1", len(events))
	}

	frame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, viewer.Id, core.NewEVTEventEnvelope(events[0]))
	if err != nil {
		t.Fatalf("realtimeProjectionFrameForEvent: %v", err)
	}
	if !handled || frame.GetProjectionEvent() == nil {
		t.Fatalf("threading mode projection frame = %+v, handled=%v", frame, handled)
	}
	var roomUpsert *realtimev1.RealtimeProjectionRoom
	var timelineUpsert *realtimev1.RealtimeProjectionRoomTimelineEventUpsert
	for _, operation := range frame.GetProjectionEvent().GetOperations() {
		if upsert := operation.GetRoomUpsert(); upsert != nil {
			roomUpsert = upsert
		}
		if upsert := operation.GetRoomTimelineEventUpsert(); upsert != nil {
			timelineUpsert = upsert
		}
	}
	if roomUpsert == nil || roomUpsert.GetRoom().GetRoom().GetThreadingMode() != apiv1.RoomThreadingMode_ROOM_THREADING_MODE_ENCOURAGED {
		t.Fatalf("room upsert = %+v, want encouraged threading mode", roomUpsert)
	}
	change := timelineUpsert.GetEvent().GetRoomThreadingModeChanged()
	if timelineUpsert == nil || timelineUpsert.GetRoomId() != room.Id || change == nil || change.GetThreadingMode() != apiv1.RoomThreadingMode_ROOM_THREADING_MODE_ENCOURAGED {
		t.Fatalf("timeline upsert = %+v, want visible encouraged threading mode change", timelineUpsert)
	}
}

func TestRealtimeProjectionOmitsMessageStateWithoutMessageRead(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-no-message-read", "RT No Message Read", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-no-message-read-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, viewer.Id, "hidden after denial", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessageRead); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessageReadInteractions); err != nil {
		t.Fatalf("DenyRoomPermission message.read.interactions: %v", err)
	}

	snapshot, err := env.httpServer.connectAPI.BuildRealtimeProjectionSnapshot(env.ctx, viewer.Id, []string{room.Id})
	if err != nil {
		t.Fatalf("BuildRealtimeProjectionSnapshot: %v", err)
	}
	if len(snapshot.Timelines) != 0 {
		t.Fatalf("snapshot timelines = %d, want 0", len(snapshot.Timelines))
	}

	frame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, viewer.Id, core.NewEVTEventEnvelope(message))
	if err != nil {
		t.Fatalf("realtimeProjectionFrameForEvent: %v", err)
	}
	if !handled || frame.GetProjectionEvent() == nil || len(frame.GetProjectionEvent().GetOperations()) != 0 {
		t.Fatalf("message frame = %+v, handled=%v; want empty cursor-advance projection", frame, handled)
	}

	_, err = env.httpServer.realtimeEventEnvelope(env.ctx, viewer.GetId(), core.NewLiveEventEnvelope(&corev1.LiveEvent{
		ActorId: "typing-author",
		Event: &corev1.LiveEvent_UserTyping{UserTyping: &corev1.UserTypingEvent{
			RoomId: room.GetId(),
		}},
	}))
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Fatalf("typing mapper err = %v, want permission denied", err)
	}
}

func TestRealtimeWebSocketDeliversCallLifecycleTimelineEvents(t *testing.T) {
	env := setupWebSocketTestServer(t)
	env.httpServer.config.LiveKit = config.LiveKitConfig{
		Enabled:   true,
		URL:       "ws://livekit.example.test",
		APIKey:    "test-key",
		APISecret: "test-secret",
	}
	env.httpServer.connectAPI = connectapi.New(env.core, env.httpServer.config, env.httpServer.version)

	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-call-timeline", "RT Call Timeline", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-call-timeline-room", "")
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

	if err := env.core.HandleCallParticipantJoined(env.ctx, room.Id, user.Id); err != nil {
		t.Fatalf("HandleCallParticipantJoined: %v", err)
	}
	started := waitRealtimeProjectionEvent(t, conn, 5*time.Second, func(projection *realtimev1.RealtimeProjectionEvent) bool {
		var hasActiveCalls, hasStarted bool
		for _, operation := range projection.GetOperations() {
			hasActiveCalls = hasActiveCalls || operation.GetActiveCallsReplace() != nil
			upsert := operation.GetRoomTimelineEventUpsert()
			hasStarted = hasStarted || (upsert.GetRoomId() == room.Id && upsert.GetEvent().GetCallStarted().GetCallId() != "")
		}
		return hasActiveCalls && hasStarted
	})
	if started == nil {
		t.Fatal("timed out waiting for active-call replacement and call-started timeline upsert")
	}

	if err := env.core.HandleCallParticipantLeft(env.ctx, room.Id, user.Id); err != nil {
		t.Fatalf("HandleCallParticipantLeft: %v", err)
	}
	ended := waitRealtimeProjectionEvent(t, conn, 5*time.Second, func(projection *realtimev1.RealtimeProjectionEvent) bool {
		var hasActiveCalls, hasEnded bool
		for _, operation := range projection.GetOperations() {
			hasActiveCalls = hasActiveCalls || operation.GetActiveCallsReplace() != nil
			upsert := operation.GetRoomTimelineEventUpsert()
			hasEnded = hasEnded || (upsert.GetRoomId() == room.Id && upsert.GetEvent().GetCallEnded().GetCallId() != "")
		}
		return hasActiveCalls && hasEnded
	})
	if ended == nil {
		t.Fatal("timed out waiting for active-call replacement and call-ended timeline upsert")
	}
}

func TestRealtimeProjectionMapsPinnedMessageChangeThroughKnownServerStateOperation(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-pin-projection", "RT Pin Projection", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-pin-projection-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	event := core.NewEVTEventEnvelope(&corev1.Event{
		Id: "pin-1", CreatedAt: timestamppb.Now(), ActorId: viewer.Id,
		Event: &corev1.Event_MessagePinned{MessagePinned: &corev1.MessagePinnedEvent{RoomId: room.Id, MessageEventId: "M1"}},
	})
	frame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, viewer.Id, event)
	if err != nil {
		t.Fatalf("realtimeProjectionFrameForEvent: %v", err)
	}
	if !handled || frame.GetProjectionEvent() == nil || len(frame.GetProjectionEvent().GetOperations()) != 1 {
		t.Fatalf("pin projection frame = %+v, handled=%v", frame, handled)
	}
	change := frame.GetProjectionEvent().GetOperations()[0].GetServerStateUpsert().GetPinnedMessageChange()
	if change.GetAction() != realtimev1.RealtimeProjectionPinnedMessageAction_REALTIME_PROJECTION_PINNED_MESSAGE_ACTION_CREATED || change.GetRoomId() != room.Id || change.GetMessageEventId() != "M1" {
		t.Fatalf("pinned message change = %+v", change)
	}

	retraction := core.NewEVTEventEnvelope(&corev1.Event{
		Id: "retract-1", CreatedAt: timestamppb.Now(), ActorId: viewer.Id,
		Event: &corev1.Event_MessageRetracted{MessageRetracted: &corev1.MessageRetractedEvent{RoomId: room.Id, EventId: "M1"}},
	})
	frame, handled, err = env.httpServer.realtimeProjectionFrameForEventWithRooms(env.ctx, viewer.Id, retraction, map[string]struct{}{})
	if err != nil {
		t.Fatalf("realtimeProjectionFrameForEventWithRooms retraction: %v", err)
	}
	if !handled || frame.GetProjectionEvent() == nil || len(frame.GetProjectionEvent().GetOperations()) != 1 {
		t.Fatalf("retraction projection frame = %+v, handled=%v", frame, handled)
	}
	change = frame.GetProjectionEvent().GetOperations()[0].GetServerStateUpsert().GetPinnedMessageChange()
	if change.GetAction() != realtimev1.RealtimeProjectionPinnedMessageAction_REALTIME_PROJECTION_PINNED_MESSAGE_ACTION_DELETED || change.GetRoomId() != room.Id || change.GetMessageEventId() != "M1" {
		t.Fatalf("retraction pinned message change = %+v", change)
	}
}

func TestRealtimeTransientMapperRejectsProjectionOwnedLiveEvents(t *testing.T) {
	tests := []struct {
		name  string
		event *corev1.LiveEvent
	}{
		{"notification occurrences invalidated", &corev1.LiveEvent{Event: &corev1.LiveEvent_NotificationOccurrencesInvalidated{NotificationOccurrencesInvalidated: &corev1.NotificationOccurrencesInvalidatedEvent{}}}},
		{"notification unread changed", &corev1.LiveEvent{Event: &corev1.LiveEvent_NotificationUnreadChanged{NotificationUnreadChanged: &corev1.NotificationUnreadChangedEvent{RoomId: "R1"}}}},
		{"thread follow", &corev1.LiveEvent{Event: &corev1.LiveEvent_ThreadFollowChanged{ThreadFollowChanged: &corev1.ThreadFollowChangedEvent{RoomId: "R1", ThreadRootEventId: "M1"}}}},
		{"room read", &corev1.LiveEvent{Event: &corev1.LiveEvent_RoomMarkedAsRead{RoomMarkedAsRead: &corev1.RoomMarkedAsReadEvent{RoomId: "R1"}}}},
		{"server updated", &corev1.LiveEvent{Event: &corev1.LiveEvent_ServerUpdated{ServerUpdated: &corev1.ServerUpdatedEvent{}}}},
		{"profile updated", &corev1.LiveEvent{Event: &corev1.LiveEvent_UserProfileUpdated{UserProfileUpdated: &corev1.UserProfileSyncEvent{UserId: "U1"}}}},
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

func TestRealtimeWebSocketRequestsReconnectAtBearerAccessExpiry(t *testing.T) {
	env := setupWebSocketTestServerWithAccessTokenTTL(t, 2*time.Second)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-bearer-expiry", "RT Bearer Expiry", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, token)
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetClose().GetCode() != "authentication_required" || !frame.GetClose().GetReconnect() {
		t.Fatalf("expiry frame = %+v, want reconnecting authentication_required", frame)
	}
}

func TestRealtimeWebSocketClosesWhenBotAPIKeyRotates(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-bot-owner", "RT Bot Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := env.core.CreateBot(env.ctx, owner.GetId(), "realtime_bot", "Realtime Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}

	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, bot.APIKey)
	rotated, err := env.core.RotateBotAPIKey(env.ctx, owner.GetId(), bot.User.GetId())
	if err != nil {
		t.Fatalf("RotateBotAPIKey: %v", err)
	}

	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetClose().GetCode() != "authentication_required" || frame.GetClose().GetReconnect() {
		t.Fatalf("rotated bot socket frame = %+v, want terminal authentication_required", frame)
	}

	staleConn := env.connectRealtime(t)
	sendRealtimeClientFrame(t, staleConn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(bot.APIKey)},
	}})
	rejected, ok := readRealtimeServerFrame(t, staleConn, 5*time.Second)
	if !ok || rejected.GetError().GetCode() != "authentication_required" {
		t.Fatalf("rotated bot key reconnect = %+v, want authentication_required", rejected)
	}

	freshConn := env.connectRealtime(t)
	defer freshConn.Close()
	subscribeRealtime(t, freshConn, rotated.APIKey)
}

func TestRealtimeBotReceivesNotificationActivations(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-activation-owner", "RT Activation Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	bot, err := env.core.CreateBot(env.ctx, owner.GetId(), "rt_activation_bot", "RT Activation Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, owner.GetId(), core.KindChannel, "", "rt-activation-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.AddMember(env.ctx, owner.GetId(), core.KindChannel, room.GetId(), bot.User.GetId()); err != nil {
		t.Fatalf("AddMember bot: %v", err)
	}
	if err := env.core.SetUserPermissionState(env.ctx, owner.GetId(), bot.User.GetId(), core.PermissionTargetScope{Kind: core.MatrixScopeRoom, ID: room.GetId()}, core.PermMessageReadInteractions, core.PermissionStateAllow); err != nil {
		t.Fatalf("grant bot message.read.interactions: %v", err)
	}

	conn := env.connectRealtime(t)
	defer conn.Close()
	subscribeRealtime(t, conn, bot.APIKey)

	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.GetId(), owner.GetId(), "Realtime ping @rt_activation_bot", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root mention: %v", err)
	}
	mentionProjection := waitRealtimeProjectionEvent(t, conn, 5*time.Second, func(projection *realtimev1.RealtimeProjectionEvent) bool {
		for _, operation := range projection.GetOperations() {
			for _, occurrence := range operation.GetNotificationOccurrencesReplace().GetOccurrences().GetOccurrences() {
				if occurrence.GetSignal().GetDirectMentionReceived().GetMessage().GetEventId() == root.GetId() {
					return true
				}
			}
		}
		return false
	})
	if mentionProjection == nil {
		t.Fatal("bot realtime stream did not receive the direct-mention activation")
	}

	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.GetId(), owner.GetId(), "Realtime follow-up", nil, root.GetId(), "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage followed reply: %v", err)
	}
	followProjection := waitRealtimeProjectionEvent(t, conn, 5*time.Second, func(projection *realtimev1.RealtimeProjectionEvent) bool {
		for _, operation := range projection.GetOperations() {
			for _, occurrence := range operation.GetNotificationOccurrencesReplace().GetOccurrences().GetOccurrences() {
				if occurrence.GetSignal().GetFollowedThreadActivity().GetMessage().GetEventId() == reply.GetId() {
					return true
				}
			}
		}
		return false
	})
	if followProjection == nil {
		t.Fatal("bot realtime stream did not receive the followed-thread activation")
	}
}

func TestRealtimeSelfAuthoredBotPermissionAdvancesWithoutProjectionReset(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-permission-owner", "RT Permission Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	bot, err := env.core.CreateBot(env.ctx, owner.GetId(), "rt_permission_bot", "RT Permission Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	event := &corev1.Event{
		Id:      core.NewEventID(),
		ActorId: owner.GetId(),
		Event: &corev1.Event_RbacPermissionGranted{RbacPermissionGranted: &corev1.RbacPermissionGrantedEvent{
			Permission: string(core.PermMessagePost),
			Subject: &corev1.RbacPermissionSubject{
				Kind: corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER,
				Id:   bot.User.GetId(),
			},
		}},
	}

	frame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, owner.GetId(), core.NewEVTEventEnvelope(event))
	if err != nil || !handled {
		t.Fatalf("self-authored bot permission frame = %+v, %v, %v", frame, handled, err)
	}
	if frame.GetProjectionEvent() == nil || len(frame.GetProjectionEvent().GetOperations()) != 0 {
		t.Fatalf("self-authored bot permission frame = %+v, want empty projection event", frame)
	}

	frame, handled, err = env.httpServer.realtimeProjectionFrameForEvent(env.ctx, core.SystemActorID, core.NewEVTEventEnvelope(event))
	if err != nil || !handled || frame.GetClose().GetCode() != "projection_reset_required" {
		t.Fatalf("other viewer bot permission frame = %+v, %v, %v", frame, handled, err)
	}

	human, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-permission-human", "RT Permission Human", "password123")
	if err != nil {
		t.Fatalf("CreateUser human target: %v", err)
	}
	event.GetRbacPermissionGranted().Subject.Id = human.GetId()
	frame, handled, err = env.httpServer.realtimeProjectionFrameForEvent(env.ctx, owner.GetId(), core.NewEVTEventEnvelope(event))
	if err != nil || !handled || frame.GetClose().GetCode() != "projection_reset_required" {
		t.Fatalf("self-authored human permission frame = %+v, %v, %v", frame, handled, err)
	}
}

func TestRealtimeWebSocketClosesOnlyBlockedOAuthClientConnections(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-oauth-block", "RT OAuth Block", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	const (
		clientID      = "https://realtime-client.example/oauth/client-metadata.json"
		otherClientID = "https://other-client.example/oauth/client-metadata.json"
	)
	if err := env.core.RecordOAuthClientAuthorization(
		env.ctx,
		user.Id,
		clientID,
		"Realtime Client",
		"https://realtime-client.example",
		"https://realtime-client.example",
		corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD,
	); err != nil {
		t.Fatalf("RecordOAuthClientAuthorization: %v", err)
	}
	if err := env.core.RecordOAuthClientAuthorization(
		env.ctx,
		user.Id,
		otherClientID,
		"Other Client",
		"https://other-client.example",
		"https://other-client.example",
		corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD,
	); err != nil {
		t.Fatalf("RecordOAuthClientAuthorization(other): %v", err)
	}
	authGeneration, err := env.core.CurrentAuthGeneration(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}
	oauthToken, err := env.core.CreateOAuthAccessTokenForClient(env.ctx, user.Id, clientID, authGeneration)
	if err != nil {
		t.Fatalf("CreateOAuthAccessTokenForClient: %v", err)
	}
	otherOAuthToken, err := env.core.CreateOAuthAccessTokenForClient(env.ctx, user.Id, otherClientID, authGeneration)
	if err != nil {
		t.Fatalf("CreateOAuthAccessTokenForClient(other): %v", err)
	}
	firstPartyToken, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	oauthConn := env.connectRealtime(t)
	subscribeRealtime(t, oauthConn, oauthToken)
	firstPartyConn := env.connectRealtime(t)
	subscribeRealtime(t, firstPartyConn, firstPartyToken)
	otherOAuthConn := env.connectRealtime(t)
	subscribeRealtime(t, otherOAuthConn, otherOAuthToken)

	// Commit only the policy fact so socket termination cannot accidentally
	// depend on the best-effort runtime-token cleanup performed by the admin
	// operation.
	aggregate := evtstream.OAuthClientAggregate(clientID)
	sequence, err := env.core.EventPublisher.LastSubjectSeq(env.ctx, aggregate.AllEventsFilter())
	if err != nil {
		t.Fatalf("read OAuth client aggregate sequence: %v", err)
	}
	blocked := &corev1.Event{
		Id:        core.NewEventID(),
		ActorId:   user.Id,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_OauthClientPolicyChanged{
			OauthClientPolicyChanged: &corev1.OAuthClientPolicyChangedEvent{
				ClientId: clientID,
				Policy:   corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED,
			},
		},
	}
	if _, err := env.core.EventPublisher.AppendAtFilter(
		env.ctx,
		aggregate.SubjectFor(blocked),
		blocked,
		aggregate.AllEventsFilter(),
		sequence,
	); err != nil {
		t.Fatalf("publish OAuth client block: %v", err)
	}

	frame, ok := readRealtimeServerFrame(t, oauthConn, 5*time.Second)
	if !ok || frame.GetClose().GetCode() != "authentication_required" || frame.GetClose().GetReconnect() {
		t.Fatalf("blocked OAuth socket frame = %+v, want terminal authentication_required", frame)
	}
	if err := realtimePingRoundTrip(firstPartyConn, "still-authorized"); err != nil {
		t.Fatalf("first-party socket was affected by OAuth client block: %v", err)
	}
	if err := realtimePingRoundTrip(otherOAuthConn, "other-client-still-authorized"); err != nil {
		t.Fatalf("other OAuth client's socket was affected by block: %v", err)
	}

	reconnect := env.connectRealtime(t)
	sendRealtimeClientFrame(t, reconnect, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{
			ProtocolVersion: realtimeProtocolVersion,
			BearerToken:     proto.String(oauthToken),
		},
	}})
	rejected, ok := readRealtimeServerFrame(t, reconnect, 5*time.Second)
	if !ok || rejected.GetError().GetCode() != "authentication_required" {
		t.Fatalf("blocked OAuth reconnect = %+v, want authentication_required", rejected)
	}
}

func TestOAuthClientBlockCancelsAuthorizationBeforeCloseWriteCompletes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	writeStarted := make(chan *realtimev1.RealtimeServerFrame, 1)
	releaseWrite := make(chan struct{})
	connectionClosed := make(chan struct{})
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		terminateRealtimeForOAuthClientBlock(
			cancel,
			func(frame *realtimev1.RealtimeServerFrame) error {
				writeStarted <- frame
				<-releaseWrite
				return nil
			},
			func() { close(connectionClosed) },
		)
	}()

	frame := <-writeStarted
	select {
	case <-ctx.Done():
	default:
		t.Fatal("authorized context remained active while close-frame write was blocked")
	}
	if frame.GetClose().GetCode() != "authentication_required" || frame.GetClose().GetReconnect() {
		t.Fatalf("terminal frame = %+v, want non-reconnecting authentication_required", frame)
	}
	select {
	case <-connectionClosed:
		t.Fatal("connection closed before the best-effort terminal frame completed")
	default:
	}

	close(releaseWrite)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("blocked OAuth client termination did not finish after releasing writer")
	}
	select {
	case <-connectionClosed:
	default:
		t.Fatal("connection remained open after terminal-frame write completed")
	}
}

func TestBearerExpiryCancelsAuthorizationAndRequestsReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var written *realtimev1.RealtimeServerFrame
	closed := false

	terminateRealtimeForBearerExpiry(
		cancel,
		func(frame *realtimev1.RealtimeServerFrame) error {
			written = frame
			return nil
		},
		func() { closed = true },
	)

	select {
	case <-ctx.Done():
	default:
		t.Fatal("authorized context remained active after bearer expiry")
	}
	if !closed {
		t.Fatal("connection remained open after bearer expiry")
	}
	if written.GetClose().GetCode() != "authentication_required" || !written.GetClose().GetReconnect() {
		t.Fatalf("expiry frame = %+v, want reconnecting authentication_required", written)
	}
}

func TestCookieRenewalCancelsAuthorizationAndRequestsReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var written *realtimev1.RealtimeServerFrame
	closed := false

	terminateRealtimeForCookieRenewal(
		cancel,
		func(frame *realtimev1.RealtimeServerFrame) error {
			written = frame
			return nil
		},
		func() { closed = true },
	)

	select {
	case <-ctx.Done():
	default:
		t.Fatal("authorized context remained active at the cookie renewal boundary")
	}
	if !closed {
		t.Fatal("connection remained open at the cookie renewal boundary")
	}
	if written.GetClose().GetCode() != "session_renewal_required" || !written.GetClose().GetReconnect() {
		t.Fatalf("renewal frame = %+v, want reconnecting session_renewal_required", written)
	}
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
		frame, ok := readRealtimeServerFrame(t, replay, realtimeTestCatchUpReadTimeout)
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
		frame, ok := readRealtimeServerFrame(t, reconnected, realtimeTestCatchUpReadTimeout)
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
		if notifications := operation.GetNotificationOccurrencesReplace(); notifications != nil {
			hasNotifications = true
			if notifications.GetPlayNotificationSound() {
				t.Fatal("snapshot notification replacement requested notification sound")
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

func TestRealtimeProjectionCompactedReconciliationRepairsOnlyRoomMarkersChangedDuringSnapshot(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-reset-marker-fence", "RT Reset Marker Fence", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-reset-marker-author", "RT Reset Marker Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	rooms := make([]*corev1.Room, 0, 2)
	messages := make([]*corev1.Event, 0, 2)
	for i := range 2 {
		room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", fmt.Sprintf("rt-reset-marker-fence-%d", i), "")
		if err != nil {
			t.Fatalf("CreateRoom %d: %v", i, err)
		}
		if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom %d: %v", i, err)
		}
		if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, author.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom author %d: %v", i, err)
		}
		message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, author.Id, fmt.Sprintf("marker fence %d", i), nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage %d: %v", i, err)
		}
		rooms = append(rooms, room)
		messages = append(messages, message)
	}

	snapshot, err := env.httpServer.connectAPI.BuildRealtimeProjectionSnapshot(env.ctx, viewer.Id, nil)
	if err != nil {
		t.Fatalf("BuildRealtimeProjectionSnapshot: %v", err)
	}
	if _, err := env.core.ReadState().MarkRoomAsRead(env.ctx, viewer.Id, rooms[0].Id, messages[0].Id); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}

	frame, err := env.httpServer.realtimeProjectionReconciliationFrame(env.ctx, viewer.Id, &snapshot.RoomMarkerFence)
	if err != nil {
		t.Fatalf("realtimeProjectionReconciliationFrame: %v", err)
	}
	var replacements []*realtimev1.RealtimeProjectionRoomViewerStateReplace
	for _, operation := range frame.GetProjectionEvent().GetOperations() {
		if replacement := operation.GetRoomViewerStateReplace(); replacement != nil {
			replacements = append(replacements, replacement)
		}
	}
	if len(replacements) != 1 || replacements[0].GetRoomId() != rooms[0].Id || replacements[0].GetViewerState().GetHasUnread() {
		t.Fatalf("changed room replacements = %+v, want only current state for %q", replacements, rooms[0].Id)
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
	// Resume at the current boundary because this test covers post-bootstrap
	// hydration admission. Other tests cover compacted snapshot delivery.
	boundary, err := env.core.PlanRealtimeReplay(env.ctx, viewer.Id, "")
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	resumeCursor := boundary.BoundaryCursor

	conn := env.dialRealtime(t)
	t.Cleanup(func() { conn.Close() })
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	if frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatal("did not receive realtime hello")
	}
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{ResumeCursor: &resumeCursor},
	}})
	if frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second); !ok || frame.GetSubscribed() == nil {
		t.Fatal("did not receive realtime subscribed")
	}
	readRealtimeCaughtUp(t, conn)

	// The server releases bootstrap admission immediately after writing
	// caught_up, but the client can receive that frame before the serving
	// goroutine gets scheduled to perform the release. Wait for that handoff
	// before reserving the hydration slot this test needs to hold.
	var release func()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var admissionErr *realtimeCatchUpAdmissionError
		release, admissionErr = env.httpServer.realtimeCatchUps.acquireHydration(viewer.Id)
		if admissionErr == nil {
			break
		}
		if admissionErr.code != "room_hydration_in_progress" || time.Now().After(deadline) {
			t.Fatalf("reserve hydration admission: %+v", admissionErr)
		}
		time.Sleep(time.Millisecond)
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

func TestRealtimeWebSocketReplacesActiveCallsWhenViewerGainsRoomAccess(t *testing.T) {
	env := setupWebSocketTestServer(t)
	env.httpServer.config.LiveKit = config.LiveKitConfig{
		Enabled:   true,
		URL:       "ws://livekit.example.test",
		APIKey:    "test-key",
		APISecret: "test-secret",
	}
	env.httpServer.connectAPI = connectapi.New(env.core, env.httpServer.config, env.httpServer.version)

	alice, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-call-alice", "Alice", "password123")
	if err != nil {
		t.Fatalf("CreateUser Alice: %v", err)
	}
	bob, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-call-bob", "Bob", "password123")
	if err != nil {
		t.Fatalf("CreateUser Bob: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, alice.Id, core.KindChannel, "", "rt-call-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, alice.Id, core.KindChannel, alice.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom Alice: %v", err)
	}
	if err := env.core.HandleCallParticipantJoined(env.ctx, room.Id, alice.Id); err != nil {
		t.Fatalf("HandleCallParticipantJoined Alice: %v", err)
	}

	snapshot, err := env.httpServer.connectAPI.BuildRealtimeProjectionSnapshot(env.ctx, bob.Id, nil)
	if err != nil {
		t.Fatalf("BuildRealtimeProjectionSnapshot Bob: %v", err)
	}
	if len(snapshot.ActiveCalls) != 0 {
		t.Fatalf("pre-membership active calls = %+v, want none", snapshot.ActiveCalls)
	}

	bobToken, err := env.core.CreateAuthToken(env.ctx, bob.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken Bob: %v", err)
	}
	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, bobToken)
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_HydrateRoom{
		HydrateRoom: &realtimev1.RealtimeHydrateRoom{RoomId: room.Id},
	}})
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetError().GetCode() != "room_unavailable" || frame.GetError().GetFatal() {
		t.Fatalf("pre-membership hydration frame = %+v, want non-fatal room_unavailable", frame)
	}

	if _, err := env.core.AddMember(env.ctx, alice.Id, core.KindChannel, room.Id, bob.Id); err != nil {
		t.Fatalf("AddMember Bob: %v", err)
	}
	projection := waitRealtimeProjectionEvent(t, conn, 5*time.Second, func(projection *realtimev1.RealtimeProjectionEvent) bool {
		var hasRoom, hasCalls bool
		for _, operation := range projection.GetOperations() {
			upsert := operation.GetRoomUpsert()
			hasRoom = hasRoom || upsert.GetRoom().GetRoom().GetId() == room.Id
			hasCalls = hasCalls || operation.GetActiveCallsReplace() != nil
		}
		return hasRoom && hasCalls
	})
	if projection == nil {
		t.Fatal("timed out waiting for room access and active-call replacement")
	}

	var gotRoom, gotTimeline, gotNotifications bool
	var calls []*apiv1.ActiveCall
	for _, operation := range projection.GetOperations() {
		if upsert := operation.GetRoomUpsert(); upsert.GetRoom().GetRoom().GetId() == room.Id {
			gotRoom = upsert.GetRoom().GetViewerState().GetIsMember() && slices.Contains(upsert.GetMemberUserIds(), bob.Id)
		}
		if timeline := operation.GetRoomTimelineReplace(); timeline.GetRoomId() == room.Id {
			gotTimeline = true
		}
		if replacement := operation.GetActiveCallsReplace(); replacement != nil {
			calls = replacement.GetCalls()
		}
		gotNotifications = gotNotifications || operation.GetNotificationOccurrencesReplace() != nil
	}
	if !gotRoom || !gotTimeline || !gotNotifications {
		t.Fatalf("room access projection room=%t timeline=%t notifications=%t; operations=%+v", gotRoom, gotTimeline, gotNotifications, projection.GetOperations())
	}
	if len(calls) != 1 || calls[0].GetRoom().GetId() != room.Id || len(calls[0].GetParticipants()) != 1 || calls[0].GetParticipants()[0].GetUser().GetId() != alice.Id {
		t.Fatalf("post-membership active calls = %+v, want Alice in room %q", calls, room.Id)
	}
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
			gotNotifications = gotNotifications || operation.GetNotificationOccurrencesReplace() != nil
		}
		if !gotRoom || gotTimeline != wantTimeline || gotMessage != wantMessage {
			t.Fatalf("%s: room=%t timeline=%t message=%t, want room=true timeline=%t message=%t; operations=%+v", name, gotRoom, gotTimeline, gotMessage, wantTimeline, wantMessage, projection.GetOperations())
		}
		if !gotCalls || !gotNotifications {
			t.Fatalf("%s: access transition omitted active-call/notification replacements; operations=%+v", name, projection.GetOperations())
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
	projection := waitRealtimeProjectionEvent(t, conn, 5*time.Second, func(projection *realtimev1.RealtimeProjectionEvent) bool {
		var gotMessage, gotCalls, gotNotifications bool
		for _, operation := range projection.GetOperations() {
			if timeline := operation.GetRoomTimelineReplace(); timeline.GetRoomId() == room.Id {
				for _, event := range timeline.GetPage().GetEvents() {
					gotMessage = gotMessage || event.GetId() == message.Id
				}
			}
			gotCalls = gotCalls || operation.GetActiveCallsReplace() != nil
			gotNotifications = gotNotifications || operation.GetNotificationOccurrencesReplace() != nil
		}
		return gotMessage && gotCalls && gotNotifications
	})
	if projection == nil {
		t.Fatal("unarchive did not restore retained timeline and viewer-sensitive resources")
	}
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

func TestRealtimeProjectionNotificationOccurrenceChangesReplaceOccurrences(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-notification-v2-viewer", "RT Notification V2 Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-notification-v2-author", "RT Notification V2 Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-notification-v2-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{viewer.Id, author.Id} {
		if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %q: %v", userID, err)
		}
	}
	eventCtx, cancelEvents := context.WithCancel(env.ctx)
	defer cancelEvents()
	eventStream, err := env.core.StreamMyEvents(eventCtx, viewer.Id)
	if err != nil {
		t.Fatalf("StreamMyEvents: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, viewer.Id, "thread root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	_, err = env.core.PostMessage(env.ctx, core.KindChannel, room.Id, author.Id, "hello", nil, root.Id, "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	occurrences, err := env.core.NotificationOccurrences().List(env.ctx, viewer.Id)
	if err != nil || len(occurrences) != 1 {
		t.Fatalf("List occurrences = %+v, %v, want one", occurrences, err)
	}
	occurrence := occurrences[0]
	if occurrence.GetAttentionLevel() != corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT {
		t.Fatalf("followed-thread occurrence attention = %v, want important", occurrence.GetAttentionLevel())
	}
	var createdInvalidation *corev1.NotificationOccurrencesInvalidatedEvent
	deadline := time.After(5 * time.Second)
	for createdInvalidation == nil {
		select {
		case envelope := <-eventStream:
			if envelope == nil || envelope.LiveEvent() == nil {
				continue
			}
			if invalidation := envelope.LiveEvent().GetNotificationOccurrencesInvalidated(); invalidation != nil {
				createdInvalidation = proto.Clone(invalidation).(*corev1.NotificationOccurrencesInvalidatedEvent)
			}
		case <-deadline:
			t.Fatal("timed out waiting for notification invalidation")
		}
	}
	if createdInvalidation.GetSoundCandidateNotificationId() != occurrence.GetId() {
		t.Fatalf("sound candidate = %q, want %q", createdInvalidation.GetSoundCandidateNotificationId(), occurrence.GetId())
	}
	if createdInvalidation.GetAlertCandidateNotificationId() != "" {
		t.Fatalf("legacy push candidate = %q for an in-app-only notification", createdInvalidation.GetAlertCandidateNotificationId())
	}
	frame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, viewer.Id, core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id:      "notification-v2-created",
		ActorId: author.Id,
		Event:   &corev1.LiveEvent_NotificationOccurrencesInvalidated{NotificationOccurrencesInvalidated: createdInvalidation},
	}))
	if err != nil || !handled {
		t.Fatalf("created projection frame = %+v, handled=%v, err=%v", frame, handled, err)
	}
	replacement := frame.GetProjectionEvent().GetOperations()[0].GetNotificationOccurrencesReplace()
	if replacement == nil || len(replacement.GetOccurrences().GetOccurrences()) != 1 || replacement.GetOccurrences().GetUnreadCount() != 1 {
		t.Fatalf("created replacement = %+v, want one unread occurrence", replacement)
	}
	if counts := replacement.GetOccurrences().GetRoomUnreadCounts(); len(counts) != 1 || counts[0].GetRoomId() != room.Id || counts[0].GetUnreadCount() != 1 {
		t.Fatalf("created room unread-occurrence counts = %+v, want one group for %s", counts, room.Id)
	}
	if !replacement.GetPlayNotificationSound() {
		t.Fatal("in-app followed-thread notification did not request sound")
	}
	operations := frame.GetProjectionEvent().GetOperations()
	if len(operations) != 1 {
		t.Fatalf("created notification operations = %+v, want one notification replacement", operations)
	}

	if _, err := env.core.NotificationOccurrences().MarkRead(env.ctx, viewer.Id, occurrence.GetId()); err != nil {
		t.Fatalf("mark occurrence read: %v", err)
	}
	frame, handled, err = env.httpServer.realtimeProjectionFrameForEvent(env.ctx, viewer.Id, core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id:      "notification-v2-updated",
		ActorId: viewer.Id,
		Event:   &corev1.LiveEvent_NotificationOccurrencesInvalidated{NotificationOccurrencesInvalidated: &corev1.NotificationOccurrencesInvalidatedEvent{}},
	}))
	if err != nil || !handled {
		t.Fatalf("updated projection frame = %+v, handled=%v, err=%v", frame, handled, err)
	}
	replacement = frame.GetProjectionEvent().GetOperations()[0].GetNotificationOccurrencesReplace()
	if replacement == nil || len(replacement.GetOccurrences().GetOccurrences()) != 1 || replacement.GetOccurrences().GetOccurrences()[0].GetUnread() || replacement.GetOccurrences().GetUnreadCount() != 0 {
		t.Fatalf("updated replacement = %+v, want one read occurrence", replacement)
	}
	if replacement.GetPlayNotificationSound() {
		t.Fatal("read notification replacement requested sound")
	}

	frame, handled, err = env.httpServer.realtimeProjectionFrameForEvent(env.ctx, viewer.Id, core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id:      "notification-v2-stale-created-after-read",
		ActorId: author.Id,
		Event: &corev1.LiveEvent_NotificationOccurrencesInvalidated{NotificationOccurrencesInvalidated: &corev1.NotificationOccurrencesInvalidatedEvent{
			AlertCandidateNotificationId: proto.String(occurrence.GetId()),
		}},
	}))
	if err != nil || !handled {
		t.Fatalf("stale created-after-read frame = %+v, handled=%v, err=%v", frame, handled, err)
	}
	replacement = frame.GetProjectionEvent().GetOperations()[0].GetNotificationOccurrencesReplace()
	if replacement.GetPlayNotificationSound() {
		t.Fatal("stale alert candidate played sound after the occurrence was read")
	}

	if deleted, err := env.core.NotificationOccurrences().Delete(env.ctx, viewer.Id, occurrence.GetId()); err != nil || !deleted {
		t.Fatalf("Delete occurrence = (%v, %v), want true, nil", deleted, err)
	}
	frame, handled, err = env.httpServer.realtimeProjectionFrameForEvent(env.ctx, viewer.Id, core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id:      "notification-v2-stale-created-after-delete",
		ActorId: author.Id,
		Event: &corev1.LiveEvent_NotificationOccurrencesInvalidated{NotificationOccurrencesInvalidated: &corev1.NotificationOccurrencesInvalidatedEvent{
			AlertCandidateNotificationId: proto.String(occurrence.GetId()),
		}},
	}))
	if err != nil || !handled {
		t.Fatalf("stale created-after-delete frame = %+v, handled=%v, err=%v", frame, handled, err)
	}
	replacement = frame.GetProjectionEvent().GetOperations()[0].GetNotificationOccurrencesReplace()
	if len(replacement.GetOccurrences().GetOccurrences()) != 0 {
		t.Fatalf("stale created-after-delete occurrences = %+v, want empty", replacement.GetOccurrences().GetOccurrences())
	}
	if replacement.GetPlayNotificationSound() {
		t.Fatal("stale alert candidate played sound after the occurrence was deleted")
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

func TestRealtimeProjectionBadgeReplacesRoomThreadAndRetainedRoot(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-badge-viewer", "RT Badge Viewer", "password123")
	if err != nil {
		t.Fatal(err)
	}
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-badge-author", "RT Badge Author", "password123")
	if err != nil {
		t.Fatal(err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-badge-room", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{viewer.Id, author.Id} {
		if _, err := env.core.JoinRoom(env.ctx, userID, core.KindChannel, userID, room.Id); err != nil {
			t.Fatal(err)
		}
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, viewer.Id, "thread root", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.core.ReadState().MarkRoomAsRead(env.ctx, viewer.Id, room.Id, root.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := env.core.NotificationPolicy().UpdateNotificationPolicy(
		env.ctx,
		viewer.Id,
		room.Id,
		&corev1.NotificationDeliveryModes{FollowedThreads: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE.Enum()},
		&fieldmaskpb.FieldMask{Paths: []string{"followed_threads"}},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, author.Id, "Badge reply", nil, root.Id, "", nil, false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		unread, err := env.core.HasUnread(env.ctx, core.KindChannel, viewer.Id, room.Id)
		if err != nil {
			t.Fatal(err)
		}
		if unread {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Badge marker did not become visible")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if occurrences, err := env.core.NotificationOccurrences().List(env.ctx, viewer.Id); err != nil || len(occurrences) != 0 {
		t.Fatalf("Badge occurrences = (%+v, %v), want none", occurrences, err)
	}

	frame, handled, err := env.httpServer.realtimeProjectionFrameForEventWithRooms(env.ctx, viewer.Id, core.NewLiveEventEnvelope(&corev1.LiveEvent{
		Id: "badge-unread-1", ActorId: author.Id,
		Event: &corev1.LiveEvent_NotificationUnreadChanged{NotificationUnreadChanged: &corev1.NotificationUnreadChangedEvent{
			RoomId: room.Id, ThreadRootEventId: root.Id,
		}},
	}), map[string]struct{}{room.Id: {}})
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("Badge unread invalidation was not handled")
	}
	var roomUnread, threadUnread, rootUnread bool
	for _, operation := range frame.GetProjectionEvent().GetOperations() {
		if replacement := operation.GetRoomViewerStateReplace(); replacement != nil {
			roomUnread = replacement.GetViewerState().GetHasUnread()
		}
		if replacement := operation.GetThreadViewerStatesReplace(); replacement != nil {
			for _, state := range replacement.GetStates() {
				threadUnread = threadUnread || state.GetThreadRootEventId() == root.Id && state.GetViewerState().GetHasUnread()
			}
		}
		if upsert := operation.GetRoomTimelineEventUpsert(); upsert != nil && upsert.GetEvent().GetId() == root.Id {
			rootUnread = upsert.GetEvent().GetMessagePosted().GetMessage().GetThread().GetViewerState().GetHasUnread()
		}
	}
	if !roomUnread || !threadUnread || !rootUnread {
		t.Fatalf("Badge projection room/thread/root unread = %v/%v/%v; frame=%+v", roomUnread, threadUnread, rootUnread, frame)
	}
}

func TestRealtimeProjectionRefreshesSearchForEveryEditedOrRetractedMessage(t *testing.T) {
	env := setupWebSocketTestServer(t)
	env.httpServer.config.Search.Enabled = true
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-search-viewer", "RT Search Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, viewer.Id, core.KindChannel, "", "rt-search-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, viewer.Id, "original body", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := env.core.EditMessage(env.ctx, viewer.Id, core.KindChannel, room.Id, message.Id, "edited body"); err != nil {
		t.Fatalf("EditMessage: %v", err)
	}
	editEvent := core.NewEVTEventEnvelope(&corev1.Event{
		Id: "edit-1", ActorId: viewer.Id,
		Event: &corev1.Event_MessageEdited{MessageEdited: &corev1.MessageEditedEvent{
			RoomId: room.Id, EventId: message.Id,
		}},
	})

	for name, retainedRooms := range map[string]map[string]struct{}{
		"unretained room": {},
		"retained room":   {room.Id: {}},
	} {
		t.Run(name, func(t *testing.T) {
			frame, handled, err := env.httpServer.realtimeProjectionFrameForEventWithRooms(env.ctx, viewer.Id, editEvent, retainedRooms)
			if err != nil {
				t.Fatalf("realtimeProjectionFrameForEventWithRooms: %v", err)
			}
			refresh := frame.GetProjectionEvent().GetOperations()[0].GetServerStateUpsert()
			if !handled || refresh == nil {
				t.Fatalf("search refresh fence = %+v, handled=%v", refresh, handled)
			}
		})
	}

	if err := env.core.DeleteMessage(env.ctx, viewer.Id, core.KindChannel, room.Id, message.Id); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	retractEvent := core.NewEVTEventEnvelope(&corev1.Event{
		Id: "retract-1", ActorId: viewer.Id,
		Event: &corev1.Event_MessageRetracted{MessageRetracted: &corev1.MessageRetractedEvent{
			RoomId: room.Id, EventId: message.Id,
		}},
	})
	frame, handled, err := env.httpServer.realtimeProjectionFrameForEventWithRooms(env.ctx, viewer.Id, retractEvent, map[string]struct{}{room.Id: {}})
	if err != nil {
		t.Fatalf("retracted realtimeProjectionFrameForEventWithRooms: %v", err)
	}
	if refresh := frame.GetProjectionEvent().GetOperations()[0].GetServerStateUpsert(); !handled || refresh == nil {
		t.Fatalf("retracted search refresh fence = %+v, handled=%v", refresh, handled)
	}

	env.httpServer.config.Search.Enabled = false
	frame, handled, err = env.httpServer.realtimeProjectionFrameForEventWithRooms(env.ctx, viewer.Id, retractEvent, map[string]struct{}{room.Id: {}})
	if err != nil {
		t.Fatalf("disabled realtimeProjectionFrameForEventWithRooms: %v", err)
	}
	for _, operation := range frame.GetProjectionEvent().GetOperations() {
		if state := operation.GetServerStateUpsert(); state != nil && state.GetPinnedMessageChange() == nil {
			t.Fatal("search-disabled server emitted a search refresh fence")
		}
	}
	if !handled {
		t.Fatal("durable edit cursor was not handled with Search disabled")
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
	if notifications := operations[1].GetNotificationOccurrencesReplace(); notifications == nil {
		t.Fatal("room-read event did not replace current notification state")
	} else if len(notifications.GetOccurrences().GetOccurrences()) != 0 {
		t.Fatalf("room-read notifications = %+v, want no unread notification state", notifications)
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

func TestRealtimeWebSocketRevalidatesCookieBeforeSubscription(t *testing.T) {
	env := setupWebSocketTestServer(t)
	if _, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-cookie-subscribe-revoke", "RT Cookie Subscribe Revoke", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	env.login(t, "rt-cookie-subscribe-revoke", "password123")
	var sessionID string
	for _, cookie := range env.cookieJar.Cookies(mustParseURL(env.server.URL)) {
		if isBrowserSessionCookieName(cookie.Name) {
			sessionID = cookie.Value
		}
	}
	if sessionID == "" {
		t.Fatal("login did not set the browser session cookie")
	}

	conn := env.connectRealtime(t)
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion},
	}})
	if frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatalf("hello frame = %+v", frame)
	}
	if err := env.core.RevokeCookieSession(env.ctx, sessionID); err != nil {
		t.Fatalf("RevokeCookieSession: %v", err)
	}

	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{},
	}})
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetError().GetCode() != "authentication_required" || !frame.GetError().GetFatal() {
		t.Fatalf("post-revocation subscribe frame = %+v, want fatal authentication_required", frame)
	}
}

func TestRealtimeWebSocketPeriodicallyRevalidatesCookie(t *testing.T) {
	env := setupWebSocketTestServer(t)
	env.httpServer.realtimeCredentialCheckEvery = 25 * time.Millisecond
	if _, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-cookie-periodic-revoke", "RT Cookie Periodic Revoke", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	env.login(t, "rt-cookie-periodic-revoke", "password123")
	var sessionID string
	for _, cookie := range env.cookieJar.Cookies(mustParseURL(env.server.URL)) {
		if isBrowserSessionCookieName(cookie.Name) {
			sessionID = cookie.Value
		}
	}
	if sessionID == "" {
		t.Fatal("login did not set the browser session cookie")
	}

	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, "")
	if err := env.core.RevokeCookieSession(env.ctx, sessionID); err != nil {
		t.Fatalf("RevokeCookieSession: %v", err)
	}

	frame, ok := readRealtimeServerFrame(t, conn, 2*time.Second)
	if !ok || frame.GetClose().GetCode() != "authentication_required" || frame.GetClose().GetReconnect() {
		t.Fatalf("periodic revocation frame = %+v, want terminal authentication_required", frame)
	}
}

func TestRealtimeWebSocketDoesNotRenewCookieOnUpgrade(t *testing.T) {
	env := setupWebSocketTestServer(t)
	if _, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-cookie-renew", "RT Cookie Renew", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	env.login(t, "rt-cookie-renew", "password123")
	env.httpServer.cookieSessionRenewalNow = func() time.Time { return time.Now().Add(89 * 24 * time.Hour) }
	var sessionID string
	for _, cookie := range env.cookieJar.Cookies(mustParseURL(env.server.URL)) {
		if isBrowserSessionCookieName(cookie.Name) {
			sessionID = cookie.Value
			break
		}
	}
	if sessionID == "" {
		t.Fatal("login did not set the browser session cookie")
	}
	before, err := env.core.ValidateCookieCredential(env.ctx, sessionID)
	if err != nil {
		t.Fatalf("validate cookie before upgrade: %v", err)
	}

	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + realtimePath
	header := http.Header{}
	for _, c := range env.cookieJar.Cookies(mustParseURL(env.server.URL)) {
		header.Add("Cookie", c.String())
	}
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Realtime WebSocket dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if response == nil {
		t.Fatal("WebSocket upgrade did not return an HTTP response")
	}
	for _, cookie := range response.Cookies() {
		if isBrowserSessionCookieName(cookie.Name) || cookie.Name == "chatto_session" {
			t.Fatalf("WebSocket upgrade unexpectedly rewrote cookie %q", cookie.Name)
		}
	}
	after, err := env.core.ValidateCookieCredential(env.ctx, sessionID)
	if err != nil {
		t.Fatalf("validate cookie after upgrade: %v", err)
	}
	if !after.GetExpiresAt().AsTime().Equal(before.GetExpiresAt().AsTime()) {
		t.Fatalf("WebSocket upgrade changed expiry from %v to %v", before.GetExpiresAt().AsTime(), after.GetExpiresAt().AsTime())
	}
	subscribeRealtime(t, conn, "")
}

func TestRealtimeWebSocketCookieSessionStopsAtRenewalBoundary(t *testing.T) {
	env := setupWebSocketTestServerWithTTLs(t, 0, 2*time.Second)
	if _, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-cookie-deadline", "RT Cookie Deadline", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	env.login(t, "rt-cookie-deadline", "password123")

	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, "")
	frame, ok := readRealtimeServerFrame(t, conn, 4*time.Second)
	if !ok || frame.GetClose().GetCode() != "session_renewal_required" || !frame.GetClose().GetReconnect() {
		t.Fatalf("cookie deadline frame = %+v, want reconnecting session_renewal_required", frame)
	}
}

func TestRealtimeWebSocketRequiresBearerAcrossOrigins(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-cross-origin", "RT Cross Origin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	env.login(t, "rt-cross-origin", "password123")

	cookieOnly := env.dialRealtimeWithOrigin(t, "https://client.example")
	t.Cleanup(func() { _ = cookieOnly.Close() })
	sendRealtimeClientFrame(t, cookieOnly, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion},
	}})
	frame, ok := readRealtimeServerFrame(t, cookieOnly, 5*time.Second)
	if !ok || frame.GetError().GetCode() != "authentication_required" {
		t.Fatalf("cookie-only cross-origin frame = %+v", frame)
	}

	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatal(err)
	}
	bearer := env.dialRealtimeWithOrigin(t, "https://client.example")
	t.Cleanup(func() { _ = bearer.Close() })
	subscribeRealtime(t, bearer, token)
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
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-thread-author", "RT Thread Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-thread-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	if err := env.core.FollowThread(env.ctx, core.KindChannel, user.Id, room.Id, root.Id); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}
	following, err := env.core.IsFollowingThread(env.ctx, core.KindChannel, user.Id, room.Id, root.Id)
	if err != nil || !following {
		t.Fatalf("IsFollowingThread after FollowThread = %v, %v, want true", following, err)
	}
	if _, err := env.core.NotificationPolicy().UpdateNotificationPolicy(
		env.ctx,
		user.Id,
		room.Id,
		&corev1.NotificationDeliveryModes{
			FollowedThreads: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum(),
		},
		&fieldmaskpb.FieldMask{Paths: []string{"followed_threads"}},
	); err != nil {
		t.Fatalf("UpdateNotificationPolicy: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.connectRealtime(t)
	subscribeRealtime(t, conn, token, room.Id)

	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, author.Id, "reply", nil, root.Id, "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage reply: %v", err)
	}

	var upsert *realtimev1.RealtimeProjectionRoomTimelineEventUpsert
	var states []*realtimev1.RealtimeProjectionThreadViewerState
	for upsert == nil || len(states) == 0 {
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for thread reply projection")
		}
		for _, operation := range frame.GetProjectionEvent().GetOperations() {
			candidate := operation.GetRoomTimelineEventUpsert()
			if candidate.GetEvent().GetId() == root.Id {
				upsert = candidate
			}
			if replacement := operation.GetThreadViewerStatesReplace(); replacement != nil {
				states = replacement.GetStates()
			}
		}
	}
	if upsert == nil {
		t.Fatal("did not receive root summary upsert")
	}
	if got := upsert.GetEvent().GetMessagePosted().GetMessage().GetThread().GetReplyCount(); got != 1 {
		t.Fatalf("root reply count = %d, want 1 (reply %q)", got, reply.Id)
	}
	if len(states) != 1 || states[0].GetThreadRootEventId() != root.Id || !states[0].GetViewerState().GetHasUnread() {
		t.Fatalf("thread viewer states = %+v, want followed root unread", states)
	}
	occurrences, err := env.core.NotificationOccurrences().List(env.ctx, user.Id)
	if err != nil || len(occurrences) != 0 {
		t.Fatalf("off-policy notification occurrences = %+v, %v, want none", occurrences, err)
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

	if added, err := env.core.ReactionModel().AddReaction(env.ctx, core.ReactionMutationInput{
		ActorID: user.Id, RoomID: room.Id, MessageEventID: reply.Id, Emoji: "thumbsup",
	}); err != nil || !added {
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
	if err := env.core.RecordAssetProcessingStarted(env.ctx, core.SystemActorID, room.Id, message.Id, attachment.Id); err != nil {
		t.Fatalf("RecordAssetProcessingStarted: %v", err)
	}
	if err := env.core.RecordAssetProcessingFailed(env.ctx, core.SystemActorID, room.Id, message.Id, attachment.Id, corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED); err != nil {
		t.Fatalf("RecordAssetProcessingFailed: %v", err)
	}
	if err := env.core.RecordAssetDeleted(env.ctx, core.SystemActorID, room.Id, attachment.Id); err != nil {
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

func TestRealtimeProjectionReplayAdvancesPastDeletedAttachmentAssetLifecycle(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-deleted-asset-replay", "Deleted Asset Replay", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-deleted-asset-replay", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	attachment, err := env.core.UploadAttachment(env.ctx, user.Id, room.Id, "delete-during-processing.mp4", "video/mp4", strings.NewReader("video"))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "delete during processing", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := env.core.RecordAssetProcessingStarted(env.ctx, core.SystemActorID, room.Id, message.Id, attachment.Id); err != nil {
		t.Fatalf("RecordAssetProcessingStarted: %v", err)
	}
	partialDerivative, err := env.core.UploadDerivativeAttachment(
		env.ctx,
		attachment.Id,
		corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_HLS_MEDIA_SEGMENT,
		room.Id,
		"partial-segment.ts",
		"video/mp2t",
		strings.NewReader("segment"),
	)
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment: %v", err)
	}
	before, err := env.core.PlanRealtimeReplay(env.ctx, user.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	if err := env.core.DeleteAttachmentFromMessage(env.ctx, user.Id, core.KindChannel, room.Id, message.Id, attachment.Id); err != nil {
		t.Fatalf("DeleteAttachmentFromMessage: %v", err)
	}
	if err := env.core.RecordAssetDeleted(env.ctx, core.SystemActorID, room.Id, partialDerivative.Id); err != nil {
		t.Fatalf("RecordAssetDeleted partial derivative: %v", err)
	}

	replay, err := env.core.PlanRealtimeReplay(env.ctx, user.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	foundDeletion := false
	for _, event := range replay.Events {
		if event.EVTEvent().GetAssetDeleted().GetAssetId() != partialDerivative.Id {
			continue
		}
		foundDeletion = true
		frame, handled, err := env.httpServer.realtimeProjectionFrameForEvent(env.ctx, user.Id, event)
		if err != nil {
			t.Fatalf("map asset deletion event: %v", err)
		}
		if !handled || frame.GetProjectionEvent() == nil {
			t.Fatal("asset deletion was not projected")
		}
		if len(frame.GetProjectionEvent().GetOperations()) != 0 {
			t.Fatalf("asset deletion operations = %+v, want cursor-only projection", frame.GetProjectionEvent().GetOperations())
		}
		if frame.GetProjectionEvent().GetResumeCursor() == "" {
			t.Fatal("asset deletion projection has no resume cursor")
		}
	}
	if !foundDeletion {
		t.Fatal("replay did not contain asset deletion")
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

	if added, err := env.core.ReactionModel().AddReaction(env.ctx, core.ReactionMutationInput{
		ActorID: user.Id, RoomID: room.Id, MessageEventID: message.Id, Emoji: "thumbsup",
	}); err != nil || !added {
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
			foundNotifications = foundNotifications || operation.GetNotificationOccurrencesReplace() != nil
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
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "rt-expired-resume-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "retained reset thread", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := env.core.FollowThread(env.ctx, core.KindChannel, user.Id, room.Id, message.Id); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}
	if _, err := env.core.ReadState().MarkRoomAsRead(env.ctx, user.Id, room.Id, message.Id); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
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
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{ResumeCursor: &expiredCursor, RetainedRoomIds: []string{room.Id}},
	}})
	subscribed, ok := readRealtimeServerFrame(t, resumed, 5*time.Second)
	if !ok || subscribed.GetSubscribed() == nil {
		t.Fatalf("expired resume subscribed = %+v", subscribed)
	}
	if subscribed.GetSubscribed().GetStartCursor() == expiredCursor {
		t.Fatal("expired resume retained the unusable cursor")
	}
	firstProjection, ok := readRealtimeServerFrame(t, resumed, realtimeTestCatchUpReadTimeout)
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

	var foundRoom, foundTimeline, foundThreadStates, foundNotifications, foundViewer, foundPresence bool
	for {
		frame, ok := readRealtimeServerFrame(t, resumed, realtimeTestCatchUpReadTimeout)
		if !ok {
			t.Fatal("timed out waiting for expired-resume caught_up")
		}
		if caughtUp := frame.GetCaughtUp(); caughtUp != nil {
			if caughtUp.GetCursor() == "" {
				t.Fatal("expired resume reset did not reach a new caught_up cursor")
			}
			break
		}
		projection := frame.GetProjectionEvent()
		if projection == nil {
			t.Fatalf("expired-resume frame = %T, want projection_event or caught_up", frame.GetFrame())
		}
		var frameHasThreadStates, frameHasNotifications, frameHasViewer bool
		var framePresences *realtimev1.RealtimeProjectionPresencesReplace
		for _, operation := range projection.GetOperations() {
			if replacement := operation.GetRoomViewerStateReplace(); replacement != nil {
				t.Fatalf("compacted reset repeated room viewer state for %q", replacement.GetRoomId())
			}
			if upsert := operation.GetRoomUpsert(); upsert.GetRoom().GetRoom().GetId() == room.Id {
				viewerState := upsert.GetRoom().GetViewerState()
				foundRoom = viewerState.GetIsMember() && !viewerState.GetHasUnread() && len(viewerState.GetPermissions()) > 0 && slices.Contains(upsert.GetMemberUserIds(), user.Id)
			}
			if timeline := operation.GetRoomTimelineReplace(); timeline.GetRoomId() == room.Id {
				foundTimeline = timeline.GetEventCursors()[message.Id] != ""
			}
			if threads := operation.GetThreadViewerStatesReplace(); threads != nil {
				for _, state := range threads.GetStates() {
					frameHasThreadStates = frameHasThreadStates || state.GetRoomId() == room.Id && state.GetThreadRootEventId() == message.Id && state.GetViewerState().GetIsFollowing()
				}
			}
			frameHasNotifications = frameHasNotifications || operation.GetNotificationOccurrencesReplace() != nil
			frameHasViewer = frameHasViewer || operation.GetViewerUpsert() != nil
			if presences := operation.GetPresencesReplace(); presences != nil {
				framePresences = presences
			}
		}
		if framePresences != nil {
			_, foundPresence = framePresences.GetStatuses()[user.Id]
			foundThreadStates = frameHasThreadStates
			foundNotifications = frameHasNotifications
			foundViewer = frameHasViewer
		}
	}
	if !foundRoom || !foundTimeline || !foundThreadStates || !foundNotifications || !foundViewer || !foundPresence {
		t.Fatalf("compacted reset coverage: room=%v timeline=%v thread_states=%v notifications=%v viewer=%v presence=%v", foundRoom, foundTimeline, foundThreadStates, foundNotifications, foundViewer, foundPresence)
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
	if err := env.core.RecordAssetProcessingStarted(env.ctx, core.SystemActorID, room.Id, assetMessage.Id, attachment.Id); err != nil {
		t.Fatalf("RecordAssetProcessingStarted: %v", err)
	}
	if err := env.core.RecordAssetProcessingFailed(env.ctx, core.SystemActorID, room.Id, assetMessage.Id, attachment.Id, corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED); err != nil {
		t.Fatalf("RecordAssetProcessingFailed: %v", err)
	}
	if err := env.core.RecordAssetDeleted(env.ctx, core.SystemActorID, room.Id, attachment.Id); err != nil {
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
		frame, ok := readRealtimeServerFrame(t, resumed, realtimeTestCatchUpReadTimeout)
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
			if operation.GetNotificationOccurrencesReplace() != nil {
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
	if replyUpserts != 1 || echoRemovals != 2 || assetUpserts != 3 || notificationReconciliations != 1 || presenceReconciliations != 1 || viewerReconciliations != 1 || roomViewerReconciliations == 0 || threadViewerReconciliations == 0 {
		t.Fatalf("replay reply/echo/asset/notifications/presence/viewer/room-viewer/thread-viewer = %d/%d/%d/%d/%d/%d/%d/%d, want 1/2/3/1/1/1/>0/>0", replyUpserts, echoRemovals, assetUpserts, notificationReconciliations, presenceReconciliations, viewerReconciliations, roomViewerReconciliations, threadViewerReconciliations)
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
