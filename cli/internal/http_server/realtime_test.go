package http_server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
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
	"hmans.de/chatto/internal/publiccursor"
)

func TestRealtimeAuthenticatedUserPreservesAuthenticationValidationError(t *testing.T) {
	s := &HTTPServer{}
	want := errors.New("storage unavailable")
	ctx := context.WithValue(context.Background(), authenticationValidationErrorKey{}, want)

	_, user, err := s.realtimeAuthenticatedUser(ctx, &realtimev1.RealtimeSubscribe{})
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

func sendRealtimeSubscribe(
	t testing.TB,
	conn *websocket.Conn,
	token string,
	initialState realtimev1.RealtimeInitialState,
	resumeCursor string,
) {
	t.Helper()
	subscribe := &realtimev1.RealtimeSubscribe{
		ProtocolVersion: realtimeProtocolVersion,
		InitialState:    initialState,
	}
	if token != "" {
		subscribe.BearerToken = proto.String(token)
	}
	if resumeCursor != "" {
		subscribe.ResumeCursor = proto.String(resumeCursor)
	}
	sendRealtimeSubscribeMessage(t, conn, subscribe)
}

func sendRealtimeSubscribeMessage(t testing.TB, conn *websocket.Conn, subscribe *realtimev1.RealtimeSubscribe) {
	t.Helper()
	data, err := proto.Marshal(subscribe)
	if err != nil {
		t.Fatalf("marshal realtime subscription: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("write realtime subscription: %v", err)
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

func subscribeRealtime(
	t testing.TB,
	conn *websocket.Conn,
	token string,
	initialState realtimev1.RealtimeInitialState,
	resumeCursor string,
) {
	t.Helper()
	sendRealtimeSubscribe(t, conn, token, initialState, resumeCursor)
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
	profileChanged := frame.GetEvent().GetUserProfileChanged()
	if profileChanged == nil {
		t.Fatal("realtime event omitted public user_profile_changed hint")
	}
	if profileChanged.GetUserId() != viewer.GetId() {
		t.Fatalf("profile user_id = %q, want %q", profileChanged.GetUserId(), viewer.GetId())
	}
	if fields := profileChanged.ProtoReflect().Descriptor().Fields(); fields.Len() != 1 {
		t.Fatalf("user profile hint has %d fields, want only user_id", fields.Len())
	}
	resumeCursor := frame.GetEvent().GetCursor()
	if resumeCursor == "" || resumeCursor == "42" {
		t.Fatalf("resume cursor = %q, want sealed non-empty value", resumeCursor)
	}
	payload, err := base64.RawURLEncoding.DecodeString(resumeCursor)
	if err != nil {
		t.Fatalf("decode resume cursor payload: %v", err)
	}
	if strings.Contains(string(payload), `"s":42`) || strings.Contains(string(payload), viewer.GetId()) {
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

func TestRealtimeRoomGroupEventsDoNotExposeHiddenRoomIDs(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "group-owner", "Group Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, owner.GetId(), core.RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "group-viewer", "Group Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	visibleRoom, err := env.core.CreateRoom(env.ctx, owner.GetId(), core.KindChannel, "", "Visible", "")
	if err != nil {
		t.Fatalf("CreateRoom visible: %v", err)
	}
	hiddenRoom, err := env.core.CreateRoom(env.ctx, owner.GetId(), core.KindChannel, "", "Hidden", "")
	if err != nil {
		t.Fatalf("CreateRoom hidden: %v", err)
	}
	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, hiddenRoom.GetId(), core.RoleEveryone, core.PermRoomList); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}

	hiddenAdded := &evtv1.Event{Event: &evtv1.Event_RoomAddedToGroup{
		RoomAddedToGroup: &evtv1.RoomAddedToGroupEvent{GroupId: "group", RoomId: hiddenRoom.GetId()},
	}}
	hiddenFrame, err := env.httpServer.realtimeServerFrameForEvent(env.ctx, viewer.GetId(), core.NewEVTEventEnvelopeWithDeliverySeq(hiddenAdded, 42))
	if err != nil || hiddenFrame.GetEvent().GetRoomLayoutChanged() == nil {
		t.Fatalf("hidden room-added event should be a data-free layout hint: %v, %v", hiddenFrame, err)
	}

	mixedOrder := &evtv1.Event{Event: &evtv1.Event_SidebarGroupEntriesReordered{
		SidebarGroupEntriesReordered: &evtv1.SidebarGroupEntriesReorderedEvent{
			GroupId: "group",
			Entries: []*evtv1.SidebarGroupEntry{
				{Kind: evtv1.SidebarGroupEntry_ROOM, Id: visibleRoom.GetId()},
				{Kind: evtv1.SidebarGroupEntry_ROOM, Id: hiddenRoom.GetId()},
				{Kind: evtv1.SidebarGroupEntry_SIDEBAR_LINK, Id: "link"},
			},
		},
	}}
	frame, err := env.httpServer.realtimeServerFrameForEvent(env.ctx, viewer.GetId(), core.NewEVTEventEnvelopeWithDeliverySeq(mixedOrder, 43))
	if err != nil {
		t.Fatalf("mixed room-group event: %v", err)
	}
	if frame.GetEvent().GetRoomLayoutChanged() == nil {
		t.Fatalf("expected layout hint, got %v", frame)
	}
	if fields := frame.GetEvent().GetRoomLayoutChanged().ProtoReflect().Descriptor().Fields(); fields.Len() != 0 {
		t.Fatalf("layout hint has %d fields, want no layout details", fields.Len())
	}
}

func TestRealtimeInternalEncryptedEventIsOmitted(t *testing.T) {
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
	_, err = env.httpServer.realtimeServerFrameForEvent(
		env.ctx,
		viewer.GetId(),
		core.NewEVTEventEnvelopeWithDeliverySeq(event, 42),
	)
	if !errors.Is(err, errRealtimeEventOmitted) {
		t.Fatalf("realtimeServerFrameForEvent() error = %v, want internal event omission", err)
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
	if closeFrame == nil || closeFrame.GetCode() != realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_RESYNC_REQUIRED || !closeFrame.GetReconnect() {
		t.Fatalf("realtimeServerFrameForEvent() = %+v, want authorization reconnect", frame)
	}
}

func TestRealtimeWebSocketCompactMentionsAndAssetRoutingSurviveReplay(t *testing.T) {
	env := setupWebSocketTestServer(t)
	ctx := env.ctx
	author, err := env.core.CreateUser(ctx, core.SystemActorID, "compact-author", "Author", "password123")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := env.core.CreateUser(ctx, core.SystemActorID, "compact-viewer", "Viewer", "password123")
	if err != nil {
		t.Fatal(err)
	}
	room, err := env.core.CreateRoom(ctx, author.Id, core.KindChannel, "", "compact-room", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{author.Id, viewer.Id} {
		if _, err := env.core.JoinRoom(ctx, id, core.KindChannel, id, room.Id); err != nil {
			t.Fatal(err)
		}
	}
	original, err := env.core.UploadAttachment(ctx, author.Id, room.Id, "original.bin", "application/octet-stream", bytes.NewReader([]byte("original")))
	if err != nil {
		t.Fatal(err)
	}
	failedAsset, err := env.core.UploadAttachment(ctx, author.Id, room.Id, "failed.bin", "application/octet-stream", bytes.NewReader([]byte("failed")))
	if err != nil {
		t.Fatal(err)
	}
	thumbnail, err := env.core.UploadDerivativeAttachment(ctx, original.Id, evtv1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, room.Id, "thumbnail.bin", "application/octet-stream", bytes.NewReader([]byte("thumbnail")))
	if err != nil {
		t.Fatal(err)
	}
	token, err := env.core.CreateAuthToken(ctx, viewer.Id)
	if err != nil {
		t.Fatal(err)
	}
	conn := env.dialRealtime(t)
	subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	boundary := readRealtimeCaughtUp(t, conn).GetCursor()
	if len(boundary) != 99 {
		t.Fatalf("cursor size = %d", len(boundary))
	}
	message, err := env.core.PostMessage(ctx, core.KindChannel, room.Id, author.Id, "@compact-viewer @all hello", []string{original.Id, failedAsset.Id}, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var posted *realtimev1.RealtimeEvent
	for posted == nil {
		event := readPublicRealtimeEvent(t, conn)
		if event.GetMessagePosted() != nil {
			posted = event
		}
	}
	mentions := posted.GetMessagePosted().GetMentions()
	if len(mentions) != 2 {
		t.Fatalf("mentions = %v, want direct target and one broadcast", mentions)
	}
	for _, mention := range mentions {
		if !mention.GetIncludesViewer() {
			t.Fatal("viewer lost original mention resolution")
		}
		if direct := mention.GetDirect(); direct != nil && direct.GetUserId() != viewer.Id {
			t.Fatal("wrong direct target")
		}
	}
	// Each event is read from the receiver's socket after a real EVT append.
	live := map[string]*realtimev1.RealtimeEvent{posted.Id: posted}
	steps := []struct {
		name    string
		publish func() error
		target  func(*realtimev1.RealtimeEvent) (string, string, string)
	}{
		{"started", func() error {
			return env.core.RecordAssetProcessingStarted(ctx, core.SystemActorID, room.Id, message.Id, original.Id)
		}, func(e *realtimev1.RealtimeEvent) (string, string, string) {
			p := e.GetAssetProcessingStarted()
			return p.GetAssetId(), p.GetRoomId(), p.GetMessageEventId()
		}},
		{"failed", func() error {
			return env.core.RecordAssetProcessingFailed(ctx, core.SystemActorID, room.Id, message.Id, failedAsset.Id, evtv1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED)
		}, func(e *realtimev1.RealtimeEvent) (string, string, string) {
			p := e.GetAssetProcessingFailed()
			return p.GetAssetId(), p.GetRoomId(), p.GetMessageEventId()
		}},
		{"succeeded", func() error {
			return env.core.RecordAssetProcessedWithHLS(ctx, core.SystemActorID, room.Id, message.Id, original.Id, 1000, 640, 360, thumbnail, nil, nil)
		}, func(e *realtimev1.RealtimeEvent) (string, string, string) {
			p := e.GetAssetProcessingSucceeded()
			return p.GetAssetId(), p.GetRoomId(), p.GetMessageEventId()
		}},
		{"deleted derivative", func() error { return env.core.RecordAssetDeleted(ctx, core.SystemActorID, room.Id, thumbnail.Id) }, func(e *realtimev1.RealtimeEvent) (string, string, string) {
			p := e.GetAssetDeleted()
			return p.GetAssetId(), p.GetRoomId(), p.GetMessageEventId()
		}},
	}
	for _, step := range steps {
		if err := step.publish(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		for {
			event := readPublicRealtimeEvent(t, conn)
			assetID, roomID, messageID := step.target(event)
			if assetID == "" {
				continue
			}
			if roomID != room.Id || messageID != message.Id {
				t.Fatalf("%s: wrong asset target", step.name)
			}
			live[event.Id] = event
			break
		}
	}
	_ = conn.Close()
	resumed := env.dialRealtime(t)
	subscribeRealtime(t, resumed, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, boundary)
	for {
		frame, ok := readRealtimeServerFrame(t, resumed, 5*time.Second)
		if !ok {
			t.Fatal("replay timed out")
		}
		if caughtUp := frame.GetCaughtUp(); caughtUp != nil {
			if caughtUp.GetRecovery() != realtimev1.RealtimeRecovery_REALTIME_RECOVERY_RESUMED {
				t.Fatal("replay used fallback")
			}
			break
		}
		event := frame.GetEvent()
		if previous := live[event.GetId()]; previous != nil {
			// Tokens contain fresh nonces; event payloads must remain equal.
			previous.Cursor, event.Cursor = nil, nil
			if !proto.Equal(previous, event) {
				t.Fatalf("replay changed public payload: %v", event)
			}
			delete(live, event.Id)
		}
	}
	if len(live) != 0 {
		t.Fatalf("replay omitted %d expected events", len(live))
	}
}

func TestRealtimeWebSocketSnapshotLifecycle(t *testing.T) {
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
	subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT, "")
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	snapshot := frame.GetSnapshot()
	if !ok || snapshot == nil {
		t.Fatalf("first frame = %+v, want atomic snapshot", frame)
	}
	if snapshot.GetServer() == nil || len(snapshot.GetUsers()) == 0 {
		t.Fatalf("snapshot = %+v, want server and referenced users", snapshot)
	}
	if caughtUp := readRealtimeCaughtUp(t, conn); caughtUp.GetCursor() == "" {
		t.Fatal("snapshot caught_up omitted cursor")
	}
	if got := env.httpServer.metrics.realtimeSnapshotBytes.Load(); got == 0 {
		t.Fatal("snapshot byte measurement was not recorded")
	}
}

func TestRealtimeWebSocketReportsActualRecoveryIncludingEmptyResume(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "recovery-viewer", "Recovery Viewer", "password123")
	if err != nil {
		t.Fatal(err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, viewer.GetId())
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := env.core.PlanRealtimeReplay(env.ctx, viewer.GetId(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, cursor string
		fallback     realtimev1.RealtimeInitialState
		want         realtimev1.RealtimeRecovery
	}{
		{"fresh live", "", realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, realtimev1.RealtimeRecovery_REALTIME_RECOVERY_LIVE_ONLY},
		{"fresh snapshot", "", realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT, realtimev1.RealtimeRecovery_REALTIME_RECOVERY_SNAPSHOT},
		{"empty resume", boundary.BoundaryCursor, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT, realtimev1.RealtimeRecovery_REALTIME_RECOVERY_RESUMED},
		{"bad cursor live fallback", "invalid", realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, realtimev1.RealtimeRecovery_REALTIME_RECOVERY_LIVE_ONLY},
		{"bad cursor snapshot fallback", "invalid", realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT, realtimev1.RealtimeRecovery_REALTIME_RECOVERY_SNAPSHOT},
	} {
		t.Run(tt.name, func(t *testing.T) {
			conn := env.dialRealtime(t)
			subscribeRealtime(t, conn, token, tt.fallback, tt.cursor)
			sawSnapshot := false
			for {
				frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
				if !ok {
					t.Fatal("recovery did not finish")
				}
				if frame.GetSnapshot() != nil {
					sawSnapshot = true
				}
				if frame.GetEvent() != nil {
					t.Fatal("empty recovery unexpectedly delivered an event")
				}
				if caughtUp := frame.GetCaughtUp(); caughtUp != nil {
					if caughtUp.GetRecovery() != tt.want || caughtUp.GetCursor() == "" {
						t.Fatalf("caught_up = %v, want %v and cursor", caughtUp, tt.want)
					}
					if sawSnapshot != (tt.want == realtimev1.RealtimeRecovery_REALTIME_RECOVERY_SNAPSHOT) {
						t.Fatal("snapshot delivery disagrees with recovery outcome")
					}
					break
				}
			}
		})
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
	subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT, "")
	foundViewer := false
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetSnapshot() == nil {
		t.Fatalf("first frame = %+v, want snapshot", frame)
	}
	for _, member := range frame.GetSnapshot().GetUsers() {
		if member.GetUser().GetId() == unreferenced.GetId() {
			t.Fatal("snapshot disclosed unreferenced server-directory user")
		}
		if member.GetUser().GetId() == viewer.GetId() {
			foundViewer = true
		}
	}
	readRealtimeCaughtUp(t, conn)
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
	subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT, "")
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetSnapshot() == nil {
		t.Fatalf("first frame = %+v, want snapshot", frame)
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
		profile := event.GetUserProfileChanged()
		if profile == nil {
			continue
		}
		if profile.GetUserId() != viewer.GetId() || event.GetCursor() == "" {
			t.Fatalf("buffered event = %+v, want post-snapshot profile hint with cursor", event)
		}
		return
	}
}

func TestRealtimeWebSocketDeliversDurableViewerAndPublicPreferenceHints(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "preference-owner", "Preference Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "preference-other", "Preference Other", "password123")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	ownerToken, err := env.core.CreateAuthToken(env.ctx, owner.GetId())
	if err != nil {
		t.Fatalf("CreateAuthToken owner: %v", err)
	}
	otherToken, err := env.core.CreateAuthToken(env.ctx, other.GetId())
	if err != nil {
		t.Fatalf("CreateAuthToken other: %v", err)
	}
	ownerConn := env.dialRealtime(t)
	subscribeRealtime(t, ownerConn, ownerToken, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	readRealtimeCaughtUp(t, ownerConn)
	otherConn := env.dialRealtime(t)
	subscribeRealtime(t, otherConn, otherToken, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	readRealtimeCaughtUp(t, otherConn)

	format := evtv1.TimeFormat_TIME_FORMAT_24H
	if _, err := env.core.UpdateUserSettings(env.ctx, owner.GetId(), core.UserSettingsInput{TimeFormat: &format}); err != nil {
		t.Fatalf("UpdateUserSettings time format: %v", err)
	}
	privateEvent := readPublicRealtimeEvent(t, ownerConn)
	if privateEvent.GetViewerPreferencesChanged() == nil || privateEvent.GetCursor() == "" {
		t.Fatalf("owner event = %+v, want cursor-bearing viewer preference hint", privateEvent)
	}

	share := true
	if _, err := env.core.UpdateUserSettings(env.ctx, owner.GetId(), core.UserSettingsInput{ShareTimezone: &share}); err != nil {
		t.Fatalf("UpdateUserSettings sharing: %v", err)
	}
	ownerEvent := readPublicRealtimeEvent(t, ownerConn)
	if ownerEvent.GetViewerPreferencesChanged() == nil || ownerEvent.GetCursor() == "" {
		t.Fatalf("owner sharing event = %+v, want cursor-bearing viewer hint", ownerEvent)
	}
	publicEvent := readPublicRealtimeEvent(t, otherConn)
	if publicEvent.GetUserProfileChanged().GetUserId() != owner.GetId() || publicEvent.GetCursor() == "" {
		t.Fatalf("other sharing event = %+v, want cursor-bearing public profile hint", publicEvent)
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
	readRealtimeCaughtUp(t, conn)
	sendRealtimeSubscribe(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")

	for {
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for unexpected control-frame rejection")
		}
		if closeFrame := frame.GetClose(); closeFrame != nil {
			if closeFrame.GetCode() != realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_INVALID_REQUEST || closeFrame.GetReconnect() {
				t.Fatalf("control-frame close = %+v, want terminal invalid_request", closeFrame)
			}
			return
		}
	}
}

func TestRealtimeWebSocketAuthenticationAndProtocolBoundaries(t *testing.T) {
	t.Run("rejects unauthenticated subscription", func(t *testing.T) {
		env := setupWebSocketTestServer(t)
		conn := env.dialRealtime(t)
		sendRealtimeSubscribe(t, conn, "", realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok || frame.GetClose().GetCode() != realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED || frame.GetClose().GetReconnect() {
			t.Fatalf("unauthenticated response = %+v, want terminal authentication_required", frame)
		}
	})

	t.Run("rejects unsupported protocol", func(t *testing.T) {
		env := setupWebSocketTestServer(t)
		conn := env.dialRealtime(t)
		sendRealtimeSubscribeMessage(t, conn, &realtimev1.RealtimeSubscribe{
			ProtocolVersion: realtimeProtocolVersion + 1,
			InitialState:    realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY,
		})
		frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
		if !ok || frame.GetClose().GetCode() != realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_UNSUPPORTED_PROTOCOL || frame.GetClose().GetReconnect() {
			t.Fatalf("unsupported protocol response = %+v, want terminal unsupported_protocol", frame)
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
		subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
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
		sendRealtimeSubscribe(t, cookieConn, "", realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
		frame, ok := readRealtimeServerFrame(t, cookieConn, 5*time.Second)
		if !ok || frame.GetClose().GetCode() != realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED {
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
	if !ok || frame.GetClose().GetCode() != realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED || !frame.GetClose().GetReconnect() {
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
	if !ok || frame.GetClose().GetCode() != realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED || frame.GetClose().GetReconnect() {
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
	if !ok || frame.GetClose().GetCode() != realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED || frame.GetClose().GetMessage() != "the bot API key is no longer valid" || frame.GetClose().GetReconnect() {
		t.Fatalf("first revoked socket frame = %+v, want terminal authentication_required", frame)
	}
	if _, err := env.core.UpdateUserBio(env.ctx, bot.User.GetId(), "second key still active"); err != nil {
		t.Fatalf("UpdateUserBio: %v", err)
	}
	for {
		event := readPublicRealtimeEvent(t, secondConn)
		if event.GetUserProfileChanged() == nil {
			continue
		}
		if event.GetUserProfileChanged().GetUserId() != bot.User.GetId() {
			t.Fatalf("second bot API key socket event = %+v, want continued delivery", event)
		}
		break
	}

	if _, err := env.core.RevokeBotAPIKey(env.ctx, owner.GetId(), bot.User.GetId(), second.KeyID); err != nil {
		t.Fatalf("RevokeBotAPIKey second: %v", err)
	}
	for {
		frame, ok = readRealtimeServerFrame(t, secondConn, 5*time.Second)
		if !ok {
			t.Fatal("timed out waiting for second bot API key revocation")
		}
		if frame.GetClose() == nil {
			continue
		}
		if frame.GetClose().GetCode() != realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_AUTHENTICATION_REQUIRED || frame.GetClose().GetReconnect() {
			t.Fatalf("second revoked socket frame = %+v, want terminal authentication_required", frame)
		}
		break
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
		event := delivery
		if event.GetId() == hidden.GetId() {
			t.Fatalf("outsider received unauthorized event: %+v", event)
		}
		if event.GetId() == visible.GetId() {
			if got := event.GetMessagePosted().GetBodyPlaintext(); got != "visible" {
				t.Fatalf("visible body_plaintext = %q, want visible", got)
			}
			if delivery.GetCursor() == "" {
				t.Fatal("authorized durable event omitted resume cursor")
			}
			return
		}
	}
}

func TestRealtimeWebSocketDeliversPublicCursorlessEvent(t *testing.T) {
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
		typing := delivery.GetUserTyping()
		if typing == nil || delivery.GetActorId() != actor.GetId() {
			continue
		}
		if typing.GetRoomId() != room.GetId() {
			t.Fatalf("typing room = %q, want %q", typing.GetRoomId(), room.GetId())
		}
		if delivery.GetCursor() != "" {
			t.Fatal("cursorless event unexpectedly carried a resume cursor")
		}
		return
	}
}

func TestRealtimeWebSocketUsesCloseFrameForSessionTermination(t *testing.T) {
	env := setupWebSocketTestServer(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "rt-terminated", "RT Terminated", "password123")
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

	if err := env.core.PublishSessionTerminated(env.ctx, viewer.GetId(), "admin_boot"); err != nil {
		t.Fatalf("PublishSessionTerminated: %v", err)
	}
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for session-termination close")
	}
	closeFrame := frame.GetClose()
	if closeFrame == nil {
		t.Fatalf("frame = %T, want close", frame.GetFrame())
	}
	if closeFrame.GetCode() != realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_SESSION_TERMINATED {
		t.Fatalf("close code = %v, want SESSION_TERMINATED", closeFrame.GetCode())
	}
	if closeFrame.GetReconnect() {
		t.Fatal("session-termination close unexpectedly permits reconnect")
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

	payload, err := publiccursor.Open("test-core-secret", "chatto-realtime-resume-v4", "all-events\x00"+viewer.GetId(), boundary)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != 33 {
		t.Fatalf("unexpected cursor payload size: %d", len(payload))
	}
	issuedAt := time.Now().Add(-16 * time.Minute)
	// The compact record stores its issue time at bytes 9..16. Lifetime is
	// fixed by the payload version, so there is no separate expiry field.
	binary.BigEndian.PutUint64(payload[9:17], uint64(issuedAt.Unix()))
	expired, err := publiccursor.Seal("test-core-secret", "chatto-realtime-resume-v4", "all-events\x00"+viewer.GetId(), payload)
	if err != nil {
		t.Fatal(err)
	}

	conn := env.dialRealtime(t)
	subscribeRealtime(t, conn, token, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_SNAPSHOT, expired)
	frame, ok := readRealtimeServerFrame(t, conn, 5*time.Second)
	if !ok || frame.GetSnapshot() == nil {
		t.Fatalf("expired cursor response = %+v, want snapshot fallback", frame)
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
	room, err := env.core.CreateRoom(env.ctx, viewer.GetId(), core.KindChannel, "", "compression-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, viewer.GetId(), core.KindChannel, viewer.GetId(), room.GetId()); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	body := strings.Repeat("0123456789abcdef", 256)
	posted, err := env.core.PostMessage(env.ctx, core.KindChannel, room.GetId(), viewer.GetId(), body, nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	for {
		event := readPublicRealtimeEvent(t, conn)
		if event.GetId() != posted.GetId() {
			continue
		}
		if event.GetMessagePosted().GetBodyPlaintext() != body {
			t.Fatalf("large compressed event = %+v, want complete message", event)
		}
		break
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

func TestPrivilegedModeExpiryCancelsAuthorizationAndRequestsReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var written *realtimev1.RealtimeServerFrame
	closed := false

	terminateRealtimeForPrivilegedModeExpiry(
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
		t.Fatal("authorized context remained active at the privileged-mode deadline")
	}
	if !closed {
		t.Fatal("connection remained open at the privileged-mode deadline")
	}
	if written.GetClose().GetCode() != realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_PRIVILEGED_MODE_EXPIRED || !written.GetClose().GetReconnect() {
		t.Fatalf("expiry frame = %+v, want reconnecting privileged_mode_expired", written)
	}
}
