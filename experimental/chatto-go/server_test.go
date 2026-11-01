package chatto

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/wire"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

type acceptedStream struct {
	conn *websocket.Conn
	sub  *wire.Subscribe
}

func testServerConnection(t *testing.T, handler http.HandlerFunc) (*Server, <-chan acceptedStream) {
	t.Helper()
	accepted := make(chan acceptedStream, 8)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/realtime" {
			conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var sub wire.Subscribe
			if proto.Unmarshal(data, &sub) != nil {
				t.Error("invalid subscribe")
				return
			}
			accepted <- acceptedStream{conn, &sub}
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/connect/chatto.api.v1.ViewerService/GetViewer" {
			_, _ = w.Write([]byte(`{"user":{"profile":{"id":"viewer","login":"me"}}}`))
			return
		}
		handler(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	c, _ := New(httpServer.URL, "token")
	s := Connect(ctx, ConnectionOptions{Client: func(context.Context, bool) (*Client, error) { return c, nil }})
	t.Cleanup(func() { cancel(); s.Close(); httpServer.Close() })
	return s, accepted
}
func nextStream(t *testing.T, ch <-chan acceptedStream) acceptedStream {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("no realtime connection")
		return acceptedStream{}
	}
}
func initialFrame() *wire.Frame {
	return &wire.Frame{Frame: &wire.Frame_Snapshot{Snapshot: &wire.Snapshot{
		Server:     &wire.ServerProfile{Name: "Community"},
		Rooms:      []*wire.RoomEntry{{Room: &wire.Room{Id: "room", Name: "general", Kind: wire.RoomKind_ROOM_KIND_CHANNEL}, ViewerState: &wire.RoomViewerState{IsMember: true}}},
		RoomGroups: []*wire.RoomGroup{{Id: "group", Name: "Rooms", Items: []*wire.RoomGroupItem{{Item: &wire.RoomGroupItem_Room{Room: &wire.RoomEntry{Room: &wire.Room{Id: "room"}}}}}}},
		Users:      []*wire.DirectoryMember{{User: &wire.User{Id: "author", Login: "alice"}}},
	}}}
}
func caughtFrame(cursor string, recovery uint32) *wire.Frame {
	return &wire.Frame{Frame: &wire.Frame_CaughtUp{CaughtUp: &wire.CaughtUp{Cursor: cursor, Recovery: recovery}}}
}
func waitServer(t *testing.T, s *Server, condition func(ServerState) bool) ServerState {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond * 5)
	defer ticker.Stop()
	for {
		state := s.State()
		if condition(state) {
			return state
		}
		select {
		case <-deadline.C:
			t.Fatalf("state condition not reached; status=%s", state.Status)
			return state
		case <-ticker.C:
		}
	}
}

func TestServerSnapshotStoresLiveUpdatesAndReplay(t *testing.T) {
	var timelineCalls, messageCalls atomic.Int32
	s, streams := testServerConnection(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/connect/chatto.api.v1.RoomService/GetRoomEvents":
			timelineCalls.Add(1)
			_, _ = w.Write([]byte(`{"page":{"events":[{"messagePosted":{"message":{"id":"old","roomId":"room","actorId":"author","body":"old","createdAt":"2026-11-01T10:00:00Z"}}}],"startCursor":"older","hasOlder":true}}`))
		case "/api/connect/chatto.api.v1.MessageService/GetMessage":
			messageCalls.Add(1)
			if r.Header.Get("Chatto-Realtime-Minimum-Cursor") != "event-cursor" {
				t.Error("resource read was not cursor-fenced")
			}
			_, _ = w.Write([]byte(`{"message":{"id":"new","roomId":"room","actorId":"author","body":"full **Markdown**","createdAt":"2026-11-01T11:00:00Z"}}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	stream := nextStream(t, streams)
	if stream.sub.InitialState != 2 || stream.sub.GetResumeCursor() != "" {
		t.Fatal("not snapshot-first")
	}
	sendFrame(t, stream.conn, initialFrame())
	if s.State().Ready {
		t.Fatal("snapshot published before caught_up")
	}
	sendFrame(t, stream.conn, caughtFrame("initial-cursor", 2))
	state := waitServer(t, s, func(s ServerState) bool { return s.Ready })
	if state.Profile.Name != "Community" || len(state.Rooms) != 1 || len(state.Groups) != 1 || state.Users["author"].Login != "alice" {
		t.Fatal("snapshot families not hydrated")
	}
	state.Groups[0].Items[0].Room.Room.Name = "tampered"
	state.Users["author"] = Profile{}
	if s.State().Groups[0].Items[0].Room.Room.Name == "tampered" || s.State().Users["author"].Login == "" {
		t.Fatal("state copy aliases stores")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.LoadTimeline(ctx, "room", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadTimeline(ctx, "room", "", ""); err != nil {
		t.Fatal(err)
	}
	if timelineCalls.Load() != 1 {
		t.Fatal("cached timeline refetched")
	}
	cursor := "event-cursor"
	actor := "author"
	event := &wire.Frame{Frame: &wire.Frame_Event{Event: &wire.Event{Id: "new", ActorId: &actor, Cursor: &cursor, Event: &wire.Event_MessagePosted{MessagePosted: &wire.MessagePosted{RoomId: "room"}}}}}
	sendFrame(t, stream.conn, event)
	state = waitServer(t, s, func(s ServerState) bool { return len(s.Timelines[TimelineKey{Room: "room"}].Messages) == 2 })
	if state.Timelines[TimelineKey{Room: "room"}].Messages[1].Body != "full **Markdown**" || timelineCalls.Load() != 1 {
		t.Fatal("live update did not merge exact message resource")
	}
	stream.conn.Close()
	resumed := nextStream(t, streams)
	if resumed.sub.GetResumeCursor() != cursor {
		t.Fatal("resume cursor not retained")
	}
	sendFrame(t, resumed.conn, event) // replay duplicates must not duplicate store rows
	sendFrame(t, resumed.conn, caughtFrame("resumed-cursor", 1))
	waitServer(t, s, func(s ServerState) bool { return s.Status == "connected" })
	if len(s.State().Timelines[TimelineKey{Room: "room"}].Messages) != 2 || timelineCalls.Load() != 1 {
		t.Fatal("resume lost cached history")
	}
	if messageCalls.Load() < 1 {
		t.Fatal("missing hydration")
	}
	privacyCursor := "privacy"
	sendFrame(t, resumed.conn, &wire.Frame{Frame: &wire.Frame_Event{Event: &wire.Event{Id: "permissions", Cursor: &privacyCursor, Event: &wire.Event_ViewerPermissionsChanged{ViewerPermissionsChanged: &wire.Empty{}}}}})
	replacement := nextStream(t, streams)
	if replacement.sub.GetResumeCursor() != "" || len(s.State().Timelines) != 0 {
		t.Fatal("privacy reset retained private state/cursor")
	}
	empty := initialFrame()
	empty.GetSnapshot().Rooms = nil
	empty.GetSnapshot().RoomGroups = nil
	sendFrame(t, replacement.conn, empty)
	sendFrame(t, replacement.conn, caughtFrame("replacement", 2))
	waitServer(t, s, func(s ServerState) bool { return s.Ready && len(s.Rooms) == 0 })
}

func TestServerDiscardsUncommittedSnapshot(t *testing.T) {
	s, streams := testServerConnection(t, func(w http.ResponseWriter, r *http.Request) { t.Errorf("unexpected request %s", r.URL.Path) })
	first := nextStream(t, streams)
	sendFrame(t, first.conn, initialFrame())
	first.conn.Close()
	second := nextStream(t, streams)
	if second.sub.GetResumeCursor() != "" || s.State().Ready || len(s.State().Rooms) != 0 {
		t.Fatal("failed snapshot leaked into stores")
	}
	sendFrame(t, second.conn, initialFrame())
	sendFrame(t, second.conn, caughtFrame("ready", 2))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestServerResourceEventsUpdateExistingStores(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Chatto-Realtime-Minimum-Cursor") != "boundary" {
			t.Error("resource read lacks event boundary")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/connect/chatto.api.v1.MessageService/GetMessage":
			_, _ = w.Write([]byte(`{"message":{"id":"root","roomId":"room","actorId":"author","body":"edited","createdAt":"2026-11-01T10:00:00Z"}}`))
		case "/api/connect/chatto.api.v1.UserService/BatchGetUsers":
			_, _ = w.Write([]byte(`{"users":[{"user":{"id":"author","login":"renamed"}}]}`))
		case "/api/connect/chatto.api.v1.RoomDirectoryService/GetRoom":
			_, _ = w.Write([]byte(`{"room":{"room":{"id":"room","name":"renamed room"},"viewerState":{"hasUnread":true}}}`))
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	}))
	defer httpServer.Close()
	c, _ := New(httpServer.URL, "token")
	state, err := snapshotState(initialFrame().GetSnapshot(), Profile{ID: "viewer"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	state.Timelines[TimelineKey{Room: "room"}] = TimelinePage{Messages: []Message{{ID: "root", RoomID: "room", ActorID: "author", Body: "old"}}}
	state.Timelines[TimelineKey{Room: "room", Root: "root"}] = clonePage(state.Timelines[TimelineKey{Room: "room"}])
	s := &Server{ctx: context.Background(), state: state}
	cursor := "boundary"
	events := []*wire.Event{
		{Id: "edit", Cursor: &cursor, Event: &wire.Event_MessageEdited{MessageEdited: &wire.MessageChanged{RoomId: "room", MessageEventId: "root"}}},
		{Id: "profile", Cursor: &cursor, Event: &wire.Event_UserProfileChanged{UserProfileChanged: &wire.UserChanged{UserId: "author"}}},
		{Id: "room", Cursor: &cursor, Event: &wire.Event_RoomUpdated{RoomUpdated: &wire.RoomChanged{RoomId: "room"}}},
	}
	for _, event := range events {
		if err := s.applyEvent(c, event); err != nil {
			t.Fatal(err)
		}
	}
	updated := s.State()
	for _, page := range updated.Timelines {
		if page.Messages[0].Body != "edited" {
			t.Fatal("edit missed a loaded timeline")
		}
	}
	if updated.Users["author"].Login != "renamed" || updated.Rooms[0].Room.Name != "renamed room" || updated.Groups[0].Items[0].Room.Room.Name != "renamed room" {
		t.Fatal("resource stores drifted")
	}
	if err := s.applyEvent(c, &wire.Event{Id: "retract", Event: &wire.Event_MessageRetracted{MessageRetracted: &wire.MessageChanged{RoomId: "room", MessageEventId: "root"}}}); err != nil {
		t.Fatal(err)
	}
	for _, page := range s.State().Timelines {
		if len(page.Messages) != 0 {
			t.Fatal("retracted message retained")
		}
	}
	if updated.Timelines[TimelineKey{Room: "room"}].Messages[0].Body != "edited" {
		t.Fatal("published snapshot mutated")
	}
}

func TestTimelineMergeOrdersFractionalTimestampsAndKeepsCursors(t *testing.T) {
	old := TimelinePage{EndCursor: "newest", Messages: []Message{{ID: "root", CreatedAt: "2026-11-01T10:00:00Z"}, {ID: "latest", CreatedAt: "2026-11-01T10:00:00.2Z"}}}
	older := TimelinePage{StartCursor: "oldest", HasOlder: true, Messages: []Message{{ID: "middle", CreatedAt: "2026-11-01T10:00:00.1Z"}}}
	page := mergeTimeline(old, older, true)
	if page.Messages[0].ID != "root" || page.Messages[1].ID != "middle" || page.Messages[2].ID != "latest" || page.EndCursor != "newest" || page.StartCursor != "oldest" {
		t.Fatal("timeline order/cursors incorrect")
	}
}

func TestServerRejectsHistoryReadAcrossPrivacyReset(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	s, streams := testServerConnection(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/connect/chatto.api.v1.RoomService/GetRoomEvents" {
			t.Errorf("unexpected request %s", r.URL.Path)
			return
		}
		close(started)
		<-release
		_ = json.NewEncoder(w).Encode(map[string]any{"page": map[string]any{"events": []any{map[string]any{"messagePosted": map[string]any{"message": Message{ID: "private", RoomID: "room"}}}}}})
	})
	stream := nextStream(t, streams)
	sendFrame(t, stream.conn, initialFrame())
	sendFrame(t, stream.conn, caughtFrame("ready", 2))
	waitServer(t, s, func(s ServerState) bool { return s.Ready })
	result := make(chan error, 1)
	go func() { _, err := s.LoadTimeline(context.Background(), "room", "", ""); result <- err }()
	<-started
	sendFrame(t, stream.conn, &wire.Frame{Frame: &wire.Frame_Event{Event: &wire.Event{Id: "privacy", Event: &wire.Event_ViewerPermissionsChanged{ViewerPermissionsChanged: &wire.Empty{}}}}})
	nextStream(t, streams)
	close(release)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("stale read accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("read did not finish")
	}
	if len(s.State().Timelines) != 0 {
		t.Fatal("private data restored after reset")
	}
}
