package http_server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

func TestRealtimeAuthenticatedUserPreservesAuthenticationValidationError(t *testing.T) {
	s := &HTTPServer{}
	want := errors.New("storage unavailable")
	ctx := context.WithValue(context.Background(), authenticationValidationErrorKey{}, want)

	_, user, err := s.realtimeAuthenticatedUser(ctx, &realtimev1.RealtimeClientHello{})
	if user != nil || !errors.Is(err, want) {
		t.Fatalf("realtimeAuthenticatedUser = (%v, %v), want (nil, %v)", user, err, want)
	}
}

func (env *wsTestEnv) dialRealtime(t testing.TB) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + realtimePath
	header := http.Header{}
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
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func (env *wsTestEnv) connectRealtime(t testing.TB) *websocket.Conn {
	return env.dialRealtime(t)
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
	messageType, data, err := conn.ReadMessage()
	if err != nil {
		if timeoutError, ok := err.(interface{ Timeout() bool }); ok && timeoutError.Timeout() {
			return nil, false
		}
		t.Fatalf("read realtime server frame: %v", err)
	}
	if messageType != websocket.BinaryMessage {
		t.Fatalf("realtime message type = %d, want binary", messageType)
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
	for {
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var frame realtimev1.RealtimeServerFrame
		if err := proto.Unmarshal(data, &frame); err != nil {
			return err
		}
		if pong := frame.GetPong(); pong != nil {
			if pong.GetNonce() != nonce {
				return fmt.Errorf("pong nonce = %q, want %q", pong.GetNonce(), nonce)
			}
			return nil
		}
	}
}

func TestRealtimeSnapshotFramesReuseCanonicalAPIResponses(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "snapshot-user", "Snapshot User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	frames, err := env.httpServer.realtimeSnapshotFrames(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("realtimeSnapshotFrames: %v", err)
	}
	if len(frames) != 9 {
		t.Fatalf("snapshot frame count = %d, want 9", len(frames))
	}
	for index, frame := range frames {
		if frame.GetSnapshot() == nil {
			t.Fatalf("frame %d = %T, want snapshot", index, frame.GetFrame())
		}
	}
	if frames[4].GetSnapshot().GetUsers() == nil || frames[5].GetSnapshot().GetRooms() == nil {
		t.Fatal("snapshot did not reuse user and room directory responses")
	}
}

func TestRealtimeDurableEventHasCanonicalShapeAndOpaqueCursor(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "cursor-user", "Cursor User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := env.core.UpdateUserBio(env.ctx, viewer.Id, "new bio"); err != nil {
		t.Fatalf("UpdateUserBio: %v", err)
	}
	events, _, err := env.core.EventPublisher.SubjectEvents(
		env.ctx,
		evtstream.UserAggregate(viewer.Id).Subject(evtstream.EventUserBioChanged),
	)
	if err != nil || len(events) != 1 {
		t.Fatalf("SubjectEvents = (%d, %v), want one", len(events), err)
	}
	envelope := core.NewEVTEventEnvelopeWithDeliverySeq(events[0], 42)
	frame, err := env.httpServer.realtimeServerFrameForEvent(env.ctx, viewer.Id, envelope)
	if err != nil {
		t.Fatalf("realtimeServerFrameForEvent: %v", err)
	}
	bioChanged := frame.GetEvent().GetEvent().GetUserBioChanged()
	if bioChanged == nil {
		t.Fatal("realtime event omitted canonical user_bio_changed fact")
	}
	if bioChanged.GetBioPlaintext() != "new bio" {
		t.Fatalf("bio_plaintext = %q, want decrypted delivery value", bioChanged.GetBioPlaintext())
	}
	if bioChanged.GetEncryptedBio() != nil {
		t.Fatal("realtime event exposed encrypted_bio")
	}
	if frame.GetEvent().GetResumeCursor() == "" || strings.Contains(frame.GetEvent().GetResumeCursor(), "42") {
		t.Fatalf("resume cursor = %q, want opaque non-empty value", frame.GetEvent().GetResumeCursor())
	}
}

func TestRealtimeInternalDurableEventIsOmitted(t *testing.T) {
	env := setupWebSocketTestServer(t)
	event := &evtv1.Event{Id: "internal-body",
		Event: &evtv1.Event_MessageBody{MessageBody: &evtv1.MessageBodyEvent{RoomId: "room", EventId: "message"}},
	}
	_, err := env.httpServer.realtimeServerFrameForEvent(
		env.ctx,
		"viewer",
		core.NewEVTEventEnvelopeWithDeliverySeq(event, 42),
	)
	if !errors.Is(err, errRealtimeEventOmitted) {
		t.Fatalf("realtimeServerFrameForEvent() error = %v, want intentional omission", err)
	}
}

func TestRealtimeRBACEventRequestsAuthorizedSnapshotReconnect(t *testing.T) {
	env := setupWebSocketTestServer(t)
	event := &evtv1.Event{
		Id: "rbac-change",
		Event: &evtv1.Event_RbacPermissionDenied{
			RbacPermissionDenied: &evtv1.RbacPermissionDeniedEvent{},
		},
	}
	frame, err := env.httpServer.realtimeServerFrameForEvent(
		env.ctx,
		"viewer",
		core.NewEVTEventEnvelopeWithDeliverySeq(event, 42),
	)
	if err != nil {
		t.Fatalf("realtimeServerFrameForEvent() error = %v", err)
	}
	closeFrame := frame.GetClose()
	if closeFrame == nil || closeFrame.GetCode() != "projection_reset_required" || !closeFrame.GetReconnect() {
		t.Fatalf("realtimeServerFrameForEvent() = %+v, want authorization reconnect", frame)
	}
}

func TestRealtimeWebSocketSnapshotLifecycleAndPing(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "socket-user", "Socket User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.dialRealtime(t)
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion, BearerToken: proto.String(token)},
	}})
	if frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second); !ok || frame.GetHello() == nil {
		t.Fatalf("hello response = %+v", frame)
	}
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: &realtimev1.RealtimeSubscribeEvents{InitialState: realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT},
	}})
	seenSnapshot := false
	for {
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok {
			t.Fatal("snapshot catch-up timed out")
		}
		if frame.GetSnapshot() != nil {
			seenSnapshot = true
		}
		if frame.GetCaughtUp() != nil {
			break
		}
	}
	if !seenSnapshot {
		t.Fatal("snapshot lifecycle omitted resource chunks")
	}
	if err := realtimePingRoundTrip(conn, "nonce"); err != nil {
		t.Fatalf("ping round trip: %v", err)
	}
}

func TestShouldCompressRealtimeFrame(t *testing.T) {
	if shouldCompressRealtimeFrame(false, realtimeCompressionMinBytes) {
		t.Fatal("disabled compression was selected")
	}
	if shouldCompressRealtimeFrame(true, realtimeCompressionMinBytes-1) {
		t.Fatal("small frame was selected for compression")
	}
	if !shouldCompressRealtimeFrame(true, realtimeCompressionMinBytes) {
		t.Fatal("large frame did not select compression")
	}
}
