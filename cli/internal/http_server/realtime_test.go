package http_server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
	return env.dialRealtimeWithOrigin(t, "")
}

func (env *wsTestEnv) dialRealtimeWithOrigin(t testing.TB, origin string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + realtimePath
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
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

func (env *wsTestEnv) dialCompressedRealtime(t testing.TB) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + realtimePath
	dialer := *websocket.DefaultDialer
	dialer.EnableCompression = true
	conn, response, err := dialer.Dial(wsURL, nil)
	if err != nil {
		if response != nil {
			t.Fatalf("compressed realtime WebSocket dial failed with status %d: %v", response.StatusCode, err)
		}
		t.Fatalf("compressed realtime WebSocket dial failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
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

func subscribeRealtime(
	t testing.TB,
	conn *websocket.Conn,
	token string,
	initialState realtimev1.RealtimeInitialState,
	resumeCursor string,
) *realtimev1.RealtimeSubscribed {
	t.Helper()
	hello := &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion}
	if token != "" {
		hello.BearerToken = proto.String(token)
	}
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{Hello: hello}})
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetHello() == nil {
		t.Fatalf("hello response = %+v, want hello", frame)
	}
	subscribe := &realtimev1.RealtimeSubscribeEvents{InitialState: initialState}
	if resumeCursor != "" {
		subscribe.ResumeCursor = proto.String(resumeCursor)
	}
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_SubscribeEvents{
		SubscribeEvents: subscribe,
	}})
	frame, ok = readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetSubscribed() == nil {
		t.Fatalf("subscribe response = %+v, want subscribed", frame)
	}
	return frame.GetSubscribed()
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
	}
}

func readPublicRealtimeEvent(t testing.TB, conn *websocket.Conn) *realtimev1.RealtimeEvent {
	t.Helper()
	for {
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for public realtime event")
		}
		if event := frame.GetEvent(); event != nil {
			return event
		}
	}
}

func TestRealtimeDurableEventHasPublicShapeAndOpaqueCursor(t *testing.T) {
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
		t.Fatal("realtime event omitted public user_bio_changed fact")
	}
	if bioChanged.GetBioPlaintext() != "new bio" {
		t.Fatalf("bio_plaintext = %q, want decrypted delivery value", bioChanged.GetBioPlaintext())
	}
	if bioChanged.GetEncryptedBio() != nil {
		t.Fatal("realtime event exposed encrypted_bio")
	}
	resumeCursor := frame.GetEvent().GetResumeCursor()
	if resumeCursor == "" || resumeCursor == "42" {
		t.Fatalf("resume cursor = %q, want sealed non-empty value", resumeCursor)
	}
	parts := strings.Split(resumeCursor, ".")
	if len(parts) != 3 {
		t.Fatalf("resume cursor is not a compact JWT: %q", resumeCursor)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode resume cursor payload: %v", err)
	}
	if strings.Contains(string(payload), `"s":42`) || !strings.Contains(string(payload), `"p":`) {
		t.Fatalf("resume cursor payload does not hide its sequence: %s", payload)
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

func TestRealtimeInternalEncryptedEventIsOmittedBeforePlaintextPopulation(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "internal-email", "Internal Email", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	event := &evtv1.Event{
		Id: "internal-email-event",
		Event: &evtv1.Event_UserVerifiedEmailAdded{
			UserVerifiedEmailAdded: &evtv1.UserVerifiedEmailAddedEvent{
				UserId: viewer.GetId(),
				EncryptedEmail: &evtv1.EncryptedUserString{
					EncryptedValue:  []byte("invalid ciphertext"),
					Nonce:           []byte("invalid nonce"),
					ContentKeyEpoch: 1,
				},
			},
		},
	}
	plaintextCandidate := proto.Clone(event).(*evtv1.Event)
	if err := env.core.PopulateEventPlaintext(env.ctx, plaintextCandidate); err == nil {
		t.Fatal("PopulateEventPlaintext() accepted malformed ciphertext; test does not exercise the ordering invariant")
	}

	_, err = env.httpServer.realtimeServerFrameForEvent(
		env.ctx,
		viewer.GetId(),
		core.NewEVTEventEnvelopeWithDeliverySeq(event, 42),
	)
	if !errors.Is(err, errRealtimeEventOmitted) {
		t.Fatalf("realtimeServerFrameForEvent() error = %v, want omission before malformed ciphertext is decrypted", err)
	}
}

func TestRealtimeRBACEventRequestsAuthorizedResourceReconnect(t *testing.T) {
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
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetSubscribed().GetRecoveryMode() != realtimev1.RealtimeRecoveryMode_REALTIME_RECOVERY_MODE_SNAPSHOT {
		t.Fatalf("subscribed = %+v, want snapshot recovery", frame)
	}
	seen := map[string]bool{}
	for {
		frame, ok = readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for snapshot")
		}
		if snapshot := frame.GetSnapshot(); snapshot != nil {
			switch snapshot.GetResource().(type) {
			case *realtimev1.RealtimeSnapshot_Server:
				seen["server"] = true
			case *realtimev1.RealtimeSnapshot_Rooms:
				seen["rooms"] = true
			case *realtimev1.RealtimeSnapshot_RoomGroups:
				seen["room_groups"] = true
			case *realtimev1.RealtimeSnapshot_Users:
				seen["users"] = true
			case *realtimev1.RealtimeSnapshot_ActiveCalls:
				seen["active_calls"] = true
			}
		}
		if caughtUp := frame.GetCaughtUp(); caughtUp != nil {
			if caughtUp.GetCursor() == "" {
				t.Fatal("snapshot caught_up omitted cursor")
			}
			break
		}
	}
	for _, resource := range []string{"server", "rooms", "room_groups", "users", "active_calls"} {
		if !seen[resource] {
			t.Fatalf("snapshot omitted %s resource", resource)
		}
	}
	if err := realtimePingRoundTrip(conn, "nonce"); err != nil {
		t.Fatalf("ping round trip: %v", err)
	}
}

func TestRealtimeWebSocketSnapshotOmitsUnreferencedUserDirectory(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "snapshot-user", "Snapshot User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	unreferenced, err := env.core.CreateUser(env.ctx, core.SystemActorID, "unreferenced-user", "Unreferenced User", "password123")
	if err != nil {
		t.Fatalf("CreateUser unreferenced: %v", err)
	}
	conn := env.dialRealtime(t)
	subscribed := subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT, "")
	if subscribed.GetRecoveryMode() != realtimev1.RealtimeRecoveryMode_REALTIME_RECOVERY_MODE_SNAPSHOT {
		t.Fatalf("recovery mode = %v, want snapshot", subscribed.GetRecoveryMode())
	}
	foundViewer := false
	for {
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for snapshot users")
		}
		if users := frame.GetSnapshot().GetUsers(); users != nil {
			for _, member := range users.GetUsers() {
				if member.GetUser().GetId() == unreferenced.GetId() {
					t.Fatal("snapshot disclosed unreferenced server-directory user")
				}
				if member.GetUser().GetId() == viewer.GetId() {
					foundViewer = true
				}
			}
		}
		if frame.GetCaughtUp() != nil {
			break
		}
	}
	if !foundViewer {
		t.Fatal("snapshot omitted its viewer from referenced users")
	}
}

func TestRealtimeWebSocketSnapshotHandsOffToSubsequentBufferedEvent(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "snapshot-handoff-user", "Snapshot Handoff User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.GetId())
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.dialRealtime(t)
	subscribed := subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT, "")
	if subscribed.GetRecoveryMode() != realtimev1.RealtimeRecoveryMode_REALTIME_RECOVERY_MODE_SNAPSHOT {
		t.Fatalf("recovery mode = %v, want snapshot", subscribed.GetRecoveryMode())
	}

	if _, err := env.core.UpdateUserBio(env.ctx, viewer.GetId(), "after snapshot boundary"); err != nil {
		t.Fatalf("UpdateUserBio: %v", err)
	}
	caughtUp := readRealtimeCaughtUp(t, conn)
	if caughtUp.GetCursor() == "" {
		t.Fatal("snapshot handoff omitted boundary cursor")
	}

	for {
		event := readPublicRealtimeEvent(t, conn)
		bio := event.GetEvent().GetUserBioChanged()
		if bio == nil {
			continue
		}
		if bio.GetBioPlaintext() != "after snapshot boundary" || event.GetResumeCursor() == "" {
			t.Fatalf("buffered event = %+v, want post-snapshot bio with cursor", event)
		}
		return
	}
}

func TestRealtimeWebSocketRejectsUnexpectedControlFrame(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "catch-up-control-user", "Catch Up Control User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.GetId())
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.dialRealtime(t)
	subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
		Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion},
	}})

	for {
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for unexpected control-frame rejection")
		}
		if realtimeErr := frame.GetError(); realtimeErr != nil {
			if realtimeErr.GetCode() != "bad_frame" || !realtimeErr.GetFatal() {
				t.Fatalf("control-frame error = %+v, want fatal bad_frame", realtimeErr)
			}
			return
		}
	}
}

func TestRealtimeWebSocketAuthenticationAndProtocolBoundaries(t *testing.T) {
	t.Run("rejects unauthenticated hello", func(t *testing.T) {
		env := setupWebSocketTestServer(t)
		conn := env.dialRealtime(t)
		sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
			Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion},
		}})
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok || frame.GetError().GetCode() != "authentication_required" || !frame.GetError().GetFatal() {
			t.Fatalf("unauthenticated response = %+v, want fatal authentication_required", frame)
		}
	})

	t.Run("rejects unsupported protocol", func(t *testing.T) {
		env := setupWebSocketTestServer(t)
		conn := env.dialRealtime(t)
		sendRealtimeClientFrame(t, conn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
			Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion + 1},
		}})
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok || frame.GetError().GetCode() != "unsupported_protocol" || !frame.GetError().GetFatal() {
			t.Fatalf("unsupported protocol response = %+v, want fatal unsupported_protocol", frame)
		}
	})

	t.Run("accepts bearer credential", func(t *testing.T) {
		env := setupWebSocketTestServer(t)
		viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-bearer", "RT Bearer", "password123")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		token, err := env.core.CreateAuthToken(env.ctx, viewer.GetId())
		if err != nil {
			t.Fatalf("CreateAuthToken: %v", err)
		}
		conn := env.dialRealtime(t)
		subscribed := subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
		if subscribed.GetRecoveryMode() != realtimev1.RealtimeRecoveryMode_REALTIME_RECOVERY_MODE_LIVE_ONLY {
			t.Fatalf("recovery mode = %v, want live-only", subscribed.GetRecoveryMode())
		}
		readRealtimeCaughtUp(t, conn)
	})

	t.Run("accepts same-origin cookie", func(t *testing.T) {
		env := setupWebSocketTestServer(t)
		if _, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-cookie", "RT Cookie", "password123"); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		env.login(t, "rt-cookie", "password123")
		conn := env.dialRealtime(t)
		subscribeRealtime(t, conn, "", realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
		readRealtimeCaughtUp(t, conn)
	})

	t.Run("requires bearer across origins", func(t *testing.T) {
		env := setupWebSocketTestServer(t)
		viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-cross-origin", "RT Cross Origin", "password123")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		env.login(t, "rt-cross-origin", "password123")

		cookieConn := env.dialRealtimeWithOrigin(t, "https://client.example")
		sendRealtimeClientFrame(t, cookieConn, &realtimev1.RealtimeClientFrame{Frame: &realtimev1.RealtimeClientFrame_Hello{
			Hello: &realtimev1.RealtimeClientHello{ProtocolVersion: realtimeProtocolVersion},
		}})
		frame, ok := readRealtimeServerFrame(t, cookieConn, 5*time.Second)
		if !ok || frame.GetError().GetCode() != "authentication_required" {
			t.Fatalf("cross-origin cookie response = %+v, want authentication_required", frame)
		}

		token, err := env.core.CreateAuthToken(env.ctx, viewer.GetId())
		if err != nil {
			t.Fatalf("CreateAuthToken: %v", err)
		}
		bearerConn := env.dialRealtimeWithOrigin(t, "https://client.example")
		subscribeRealtime(t, bearerConn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
		readRealtimeCaughtUp(t, bearerConn)
	})
}

func TestRealtimeWebSocketClosesAtBearerExpiry(t *testing.T) {
	env := setupWebSocketTestServerWithAccessTokenTTL(t, 2*time.Second)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-expiry", "RT Expiry", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.GetId())
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.dialRealtime(t)
	subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	readRealtimeCaughtUp(t, conn)
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetClose().GetCode() != "authentication_required" || !frame.GetClose().GetReconnect() {
		t.Fatalf("expiry response = %+v, want reconnecting authentication_required", frame)
	}
}

func TestRealtimeWebSocketClosesAfterCookieRevocation(t *testing.T) {
	env := setupWebSocketTestServer(t)
	env.httpServer.realtimeCredentialCheckEvery = 25 * time.Millisecond
	if _, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-cookie-revoke", "RT Cookie Revoke", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	env.login(t, "rt-cookie-revoke", "password123")
	var sessionID string
	for _, cookie := range env.cookieJar.Cookies(mustParseURL(env.server.URL)) {
		if isBrowserSessionCookieName(cookie.Name) {
			sessionID = cookie.Value
			break
		}
	}
	if sessionID == "" {
		t.Fatal("login did not set browser session cookie")
	}
	conn := env.dialRealtime(t)
	subscribeRealtime(t, conn, "", realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	readRealtimeCaughtUp(t, conn)
	if err := env.core.RevokeCookieSession(env.ctx, sessionID); err != nil {
		t.Fatalf("RevokeCookieSession: %v", err)
	}
	frame, ok := readRealtimeServerFrame(t, conn, 2*time.Second)
	if !ok || frame.GetClose().GetCode() != "authentication_required" || frame.GetClose().GetReconnect() {
		t.Fatalf("revocation response = %+v, want terminal authentication_required", frame)
	}
}

func TestRealtimeWebSocketClosesOnlyForRevokedBotAPIKey(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-multi-key-owner", "RT Multi-key Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := env.core.CreateBot(env.ctx, owner.GetId(), "rt_multi_key_bot", "RT Multi-key Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	first, err := env.core.CreateBotAPIKey(env.ctx, owner.GetId(), bot.User.GetId(), "First worker")
	if err != nil {
		t.Fatalf("CreateBotAPIKey first: %v", err)
	}
	second, err := env.core.CreateBotAPIKey(env.ctx, owner.GetId(), bot.User.GetId(), "Second worker")
	if err != nil {
		t.Fatalf("CreateBotAPIKey second: %v", err)
	}

	firstConn := env.dialRealtime(t)
	subscribeRealtime(t, firstConn, first.Credential, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	readRealtimeCaughtUp(t, firstConn)
	secondConn := env.dialRealtime(t)
	subscribeRealtime(t, secondConn, second.Credential, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	readRealtimeCaughtUp(t, secondConn)

	if _, err := env.core.RevokeBotAPIKey(env.ctx, owner.GetId(), bot.User.GetId(), first.KeyID); err != nil {
		t.Fatalf("RevokeBotAPIKey first: %v", err)
	}
	frame, ok := readRealtimeServerFrame(t, firstConn, 5*time.Second)
	if !ok || frame.GetClose().GetCode() != "authentication_required" || frame.GetClose().GetMessage() != "the bot API key is no longer valid" || frame.GetClose().GetReconnect() {
		t.Fatalf("first revoked socket frame = %+v, want terminal authentication_required", frame)
	}
	if err := realtimePingRoundTrip(secondConn, "still-valid"); err != nil {
		t.Fatalf("second bot API key socket after first revocation: %v", err)
	}

	if _, err := env.core.RevokeBotAPIKey(env.ctx, owner.GetId(), bot.User.GetId(), second.KeyID); err != nil {
		t.Fatalf("RevokeBotAPIKey second: %v", err)
	}
	frame, ok = readRealtimeServerFrame(t, secondConn, 5*time.Second)
	if !ok || frame.GetClose().GetCode() != "authentication_required" || frame.GetClose().GetReconnect() {
		t.Fatalf("second revoked socket frame = %+v, want terminal authentication_required", frame)
	}
}

func TestRealtimeWebSocketOmitsUnauthorizedRoomEventAndContinues(t *testing.T) {
	env := setupWebSocketTestServer(t)
	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-member", "RT Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-outsider", "RT Outsider", "password123")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	privateRoom, err := env.core.CreateRoom(env.ctx, member.GetId(), core.KindChannel, "", "rt-private", "")
	if err != nil {
		t.Fatalf("CreateRoom private: %v", err)
	}
	visibleRoom, err := env.core.CreateRoom(env.ctx, outsider.GetId(), core.KindChannel, "", "rt-visible", "")
	if err != nil {
		t.Fatalf("CreateRoom visible: %v", err)
	}
	for userID, roomID := range map[string]string{member.GetId(): privateRoom.GetId(), outsider.GetId(): visibleRoom.GetId()} {
		if _, err := env.core.JoinRoom(env.ctx, userID, core.KindChannel, userID, roomID); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
	}
	token, err := env.core.CreateAuthToken(env.ctx, outsider.GetId())
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.dialRealtime(t)
	subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	readRealtimeCaughtUp(t, conn)

	hidden, err := env.core.PostMessage(env.ctx, core.KindChannel, privateRoom.GetId(), member.GetId(), "hidden", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage hidden: %v", err)
	}
	visible, err := env.core.PostMessage(env.ctx, core.KindChannel, visibleRoom.GetId(), outsider.GetId(), "visible", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage visible: %v", err)
	}
	for {
		delivery := readPublicRealtimeEvent(t, conn)
		event := delivery.GetEvent()
		if event.GetId() == hidden.GetId() {
			t.Fatalf("outsider received unauthorized event: %+v", event)
		}
		if event.GetId() == visible.GetId() {
			if got := event.GetMessagePosted().GetBodyPlaintext(); got != "visible" {
				t.Fatalf("visible body_plaintext = %q, want visible", got)
			}
			if delivery.GetResumeCursor() == "" {
				t.Fatal("authorized durable event omitted resume cursor")
			}
			return
		}
	}
}

func TestRealtimeWebSocketDeliversPublicTransientEvent(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-typing-viewer", "RT Typing Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-typing-actor", "RT Typing Actor", "password123")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, actor.GetId(), core.KindChannel, "", "rt-typing", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{viewer.GetId(), actor.GetId()} {
		if _, err := env.core.JoinRoom(env.ctx, userID, core.KindChannel, userID, room.GetId()); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.GetId())
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.dialRealtime(t)
	subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	readRealtimeCaughtUp(t, conn)
	if err := env.core.PublishTypingIndicator(env.ctx, actor.GetId(), core.KindChannel, room.GetId(), nil); err != nil {
		t.Fatalf("PublishTypingIndicator: %v", err)
	}
	for {
		delivery := readPublicRealtimeEvent(t, conn)
		typing := delivery.GetEvent().GetUserTypingSignal()
		if typing == nil || delivery.GetEvent().GetActorId() != actor.GetId() {
			continue
		}
		if typing.GetRoomId() != room.GetId() {
			t.Fatalf("typing room = %q, want %q", typing.GetRoomId(), room.GetId())
		}
		if delivery.GetResumeCursor() != "" {
			t.Fatal("transient event unexpectedly carried a resume cursor")
		}
		return
	}
}

func TestRealtimeWebSocketExpiredCursorUsesSnapshotFallback(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-expired", "RT Expired", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.GetId())
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	boundaryConn := env.dialRealtime(t)
	subscribeRealtime(t, boundaryConn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	boundary := readRealtimeCaughtUp(t, boundaryConn).GetCursor()
	_ = boundaryConn.Close()

	type cursorClaims struct {
		Position string `json:"p"`
		Version  int    `json:"v"`
		jwt.RegisteredClaims
	}
	claims := cursorClaims{}
	parser := jwt.NewParser()
	if _, _, err := parser.ParseUnverified(boundary, &claims); err != nil {
		t.Fatalf("parse boundary cursor: %v", err)
	}
	issuedAt := time.Now().Add(-16 * time.Minute)
	claims.IssuedAt = jwt.NewNumericDate(issuedAt)
	claims.ExpiresAt = jwt.NewNumericDate(issuedAt.Add(15 * time.Minute))
	mac := hmac.New(sha256.New, []byte("test-core-secret"))
	_, _ = mac.Write([]byte("chatto-realtime-resume-v4\x00jwt"))
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(mac.Sum(nil))
	if err != nil {
		t.Fatalf("sign expired cursor: %v", err)
	}

	conn := env.dialRealtime(t)
	subscribed := subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT, expired)
	if subscribed.GetRecoveryMode() != realtimev1.RealtimeRecoveryMode_REALTIME_RECOVERY_MODE_SNAPSHOT {
		t.Fatalf("recovery mode = %v, want snapshot", subscribed.GetRecoveryMode())
	}
	readRealtimeCaughtUp(t, conn)
}

func TestRealtimeWebSocketNegotiatedCompressionSupportsLargeFrames(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-compression", "RT Compression", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.GetId())
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	conn := env.dialCompressedRealtime(t)
	subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	readRealtimeCaughtUp(t, conn)
	nonce := strings.Repeat("0123456789abcdef", 512)
	if err := realtimePingRoundTrip(conn, nonce); err != nil {
		t.Fatalf("large compressed ping round trip: %v", err)
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
