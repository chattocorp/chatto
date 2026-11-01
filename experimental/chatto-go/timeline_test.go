package chatto

import (
	"context"
	"encoding/json"
	"github.com/chattocorp/chatto/experimental/chatto-go/internal/wire"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHumanTimelineContracts(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch r.URL.Path {
		case "/api/connect/chatto.api.v1.RoomDirectoryService/ListRooms":
			if in["page"].(map[string]any)["offset"] != float64(100) {
				t.Error("offset lost")
			}
			fmtJSON(w, `{"rooms":[{"room":{"id":"r","kind":"ROOM_KIND_DM"},"memberUserIds":["u"],"viewerState":{"isMember":true,"hasUnread":true}}],"page":{"hasMore":true}}`)
		case "/api/connect/chatto.api.v1.RoomService/GetRoomEvents", "/api/connect/chatto.api.v1.ThreadService/GetThreadEvents":
			if in["roomId"] != "r" || in["before"] != "opaque" {
				t.Error("timeline scope lost")
			}
			fmtJSON(w, `{"page":{"events":[{"createdAt":"2026-11-01T12:00:00Z","messagePosted":{"message":{"id":"m","actorId":"u","body":"hello"}}},{"messagePosted":{"message":{"id":"deleted","deletedAt":"now"}}},{"roomCreated":{"roomId":"r"}}],"startCursor":"older","hasOlder":true,"includes":{"users":{"u":{"id":"u","login":"tester"}}}}}`)
		case "/api/connect/chatto.api.v1.UserService/BatchGetUsers":
			fmtJSON(w, `{"users":[{"user":{"id":"u","login":"tester"}}]}`)
		case "/api/connect/chatto.api.v1.RoomDirectoryService/ListRoomGroups":
			fmtJSON(w, `{"groups":[{"id":"second","name":"First on screen","items":[{"sidebarLink":{"id":"link","label":"Docs","url":"/docs"}},{"room":{"room":{"id":"r"}}}]},{"id":"first","name":"Second on screen"}]}`)
		default:
			t.Error("unexpected route")
		}
	}))
	defer s.Close()
	c, _ := New(s.URL, "token")
	p, err := c.Rooms(context.Background(), 100)
	if err != nil || len(p.Rooms) != 1 || !p.Page.HasMore || !p.Rooms[0].ViewerState.HasUnread {
		t.Fatal("room page decoding", err)
	}
	for _, root := range []string{"", "root"} {
		p, err := c.Timeline(context.Background(), "r", root, "opaque")
		if err != nil || len(p.Messages) != 1 || p.Messages[0].CreatedAt == "" || p.Users["u"].Login != "tester" || p.StartCursor != "older" {
			t.Fatal("timeline decoding", err)
		}
	}
	u, err := c.Profiles(context.Background(), []string{"u"})
	if err != nil || u["u"].Login != "tester" {
		t.Fatal("profile decoding", err)
	}
	groups, err := c.RoomGroups(context.Background())
	if err != nil || len(groups) != 2 || groups[0].ID != "second" || groups[0].Items[0].SidebarLink.Label != "Docs" || groups[0].Items[1].Room.Room.ID != "r" {
		t.Fatal("group order or item decoding lost", err)
	}
}

func TestTimelineIncludesSelfEchoAndNonTextWithoutChangingBotObservers(t *testing.T) {
	actor := "self"
	e := &wire.Event{Id: "m", ActorId: &actor, Event: &wire.Event_MessagePosted{MessagePosted: &wire.MessagePosted{RoomId: "r", EchoOfEventId: "original"}}}
	if _, ok := decodeMessageEvent(e, "self"); ok {
		t.Fatal("bot filter changed")
	}
	if got, ok := decodeVisibleEvent(e, "self", true); !ok || got.Message.ID != "m" {
		t.Fatal("timeline omitted own non-text echo")
	}
}

func TestObserveTimelineDeliversOwnPostOverWebSocket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Error(err)
			return
		}
		sendFrame(t, conn, &wire.Frame{Frame: &wire.Frame_CaughtUp{CaughtUp: &wire.CaughtUp{Cursor: "ready", Recovery: 3}}})
		actor := "self"
		body := "my post"
		sendFrame(t, conn, &wire.Frame{Frame: &wire.Frame_Event{Event: &wire.Event{Id: "own", ActorId: &actor, Event: &wire.Event_MessagePosted{MessagePosted: &wire.MessagePosted{RoomId: "room", BodyPlaintext: &body}}}}})
		<-ctx.Done()
	}))
	defer s.Close()
	c, _ := New(s.URL, "access")
	received := false
	_ = c.ObserveTimeline(ctx, "self", RecoveryOptions{}, nil, func(_ context.Context, e MessageEvent) error {
		received = e.ID == "own" && e.Message.Body == "my post"
		cancel()
		return nil
	})
	if !received {
		t.Fatal("human observer suppressed its own post")
	}
}
