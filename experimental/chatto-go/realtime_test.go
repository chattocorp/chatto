package chatto

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/wire"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func pingEvent() *wire.Event {
	return &wire.Event{Id: "ping", ActorId: proto.String("human"), Event: &wire.Event_MessagePosted{MessagePosted: &wire.MessagePosted{
		RoomId: "room", ThreadRootEventId: "root", BodyPlaintext: proto.String("hello @bot"),
		Mentions: []*wire.Mention{{IncludesViewer: true, Cause: &wire.Mention_Direct{Direct: &wire.Direct{UserId: "bot"}}}},
	}}}
}

func TestPingFiltering(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*wire.Event)
		want   bool
	}{
		{"direct", func(*wire.Event) {}, true},
		{"root", func(e *wire.Event) { e.GetMessagePosted().ThreadRootEventId = "" }, true},
		{"self", func(e *wire.Event) { e.ActorId = proto.String("bot") }, false},
		{"echo", func(e *wire.Event) { e.GetMessagePosted().EchoOfEventId = "original" }, false},
		{"plain-text-mention", func(e *wire.Event) { e.GetMessagePosted().Mentions = nil }, false},
		{"unmentioned-channel", func(e *wire.Event) {
			e.GetMessagePosted().RoomKind = wire.RoomKind_ROOM_KIND_CHANNEL
			e.GetMessagePosted().Mentions = nil
		}, false},
		{"unmentioned-dm-root", func(e *wire.Event) {
			e.GetMessagePosted().RoomKind = wire.RoomKind_ROOM_KIND_DM
			e.GetMessagePosted().Mentions = nil
			e.GetMessagePosted().ThreadRootEventId = ""
		}, true},
		{"unmentioned-dm-thread", func(e *wire.Event) {
			e.GetMessagePosted().RoomKind = wire.RoomKind_ROOM_KIND_DM
			e.GetMessagePosted().Mentions = nil
		}, true},
		{"dm-self", func(e *wire.Event) {
			e.GetMessagePosted().RoomKind = wire.RoomKind_ROOM_KIND_DM
			e.ActorId = proto.String("bot")
		}, false},
		{"dm-echo", func(e *wire.Event) {
			e.GetMessagePosted().RoomKind = wire.RoomKind_ROOM_KIND_DM
			e.GetMessagePosted().EchoOfEventId = "original"
		}, false},
		{"not-recipient", func(e *wire.Event) { e.GetMessagePosted().Mentions[0].IncludesViewer = false }, false},
		{"other-user", func(e *wire.Event) { e.GetMessagePosted().Mentions[0].GetDirect().UserId = "other" }, false},
		{"role", func(e *wire.Event) {
			e.GetMessagePosted().Mentions[0].Cause = &wire.Mention_Role{Role: &wire.Role{RoleName: "helpers"}}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := pingEvent()
			tc.change(e)
			_, ok := ping(e, "bot")
			if ok != tc.want {
				t.Fatalf("accepted=%v", ok)
			}
		})
	}
}

func TestObserveUnaddressedContextAndInvalidations(t *testing.T) {
	e := pingEvent()
	e.GetMessagePosted().Mentions = nil
	update, ok := messageEvent(e, "bot")
	if !ok || update.Addressed || update.Message.ID != "ping" {
		t.Fatal("ordinary context was not exposed")
	}
	for _, event := range []*wire.Event{
		{Id: "edit", Event: &wire.Event_MessageEdited{MessageEdited: &wire.MessageChanged{RoomId: "room", MessageEventId: "ping"}}},
		{Id: "delete", Event: &wire.Event_MessageRetracted{MessageRetracted: &wire.MessageChanged{RoomId: "room", MessageEventId: "ping"}}},
	} {
		update, ok := messageEvent(event, "bot")
		if !ok || update.InvalidatedRoom != "room" || update.Addressed {
			t.Fatal("history invalidation missing")
		}
	}
	update, ok = messageEvent(&wire.Event{Id: "permissions", Event: &wire.Event_ViewerPermissionsChanged{ViewerPermissionsChanged: &wire.Empty{}}}, "bot")
	if !ok || !update.InvalidateAll {
		t.Fatal("permission change did not invalidate context")
	}
}

func TestRealtimeSubscriptionAndDuplicateSuppression(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/realtime" {
			t.Error("wrong endpoint")
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Error(err)
			return
		}
		var sub wire.Subscribe
		if proto.Unmarshal(data, &sub) != nil || sub.GetBearerToken() != "token" || sub.ProtocolVersion != 4 || sub.InitialState != 1 {
			t.Error("bad subscription")
		}
		frames := []*wire.Frame{
			{Frame: &wire.Frame_CaughtUp{CaughtUp: &wire.CaughtUp{Recovery: 3}}},
			{Frame: &wire.Frame_Event{Event: pingEvent()}},
			{Frame: &wire.Frame_Event{Event: pingEvent()}},
			{Frame: &wire.Frame_Event{Event: pingEvent()}},
		}
		frames[3].GetEvent().Id = "second"
		for _, f := range frames {
			b, _ := proto.Marshal(f)
			if conn.WriteMessage(websocket.BinaryMessage, b) != nil {
				return
			}
		}
		_, _, _ = conn.ReadMessage()
	}))
	defer s.Close()
	c, err := New(s.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	count, ready := 0, false
	err = c.Pings(ctx, "bot", func() { ready = true }, func(_ context.Context, m Message) error {
		count++
		if !ready || m.RoomID != "room" || m.ThreadRootEventID != "root" {
			t.Error("bad delivery")
		}
		if count == 2 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
