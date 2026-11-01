package chatto

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/wire"
)

func TestNotificationCountsInitializeRefreshAndClear(t *testing.T) {
	counts := []NotificationCount{{RoomID: "room", Unread: 7, Important: 2}}
	cursor := "initial"
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Chatto-Realtime-Minimum-Cursor") != cursor {
			t.Error("notification read not fenced at event cursor")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/connect/chatto.api.v1.NotificationService/ListNotificationOccurrences":
			var in struct{ Page struct{ Limit int } }
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.Page.Limit != 1 {
				t.Error("fetched unnecessary occurrence pages")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"roomUnreadCounts": counts, "nextExpiryAt": "2030-01-01T00:00:00Z"})
		case "/api/connect/chatto.api.v1.RoomDirectoryService/GetRoom":
			_, _ = w.Write([]byte(`{"room":{"room":{"id":"room","name":"general"},"viewerState":{"hasUnread":false}}}`))
		default:
			t.Error("unexpected resource request")
		}
	}))
	defer httpServer.Close()
	c, _ := New(httpServer.URL, "token")
	s := &Server{ctx: context.Background(), state: emptyState("connecting", 0), options: ConnectionOptions{IncludeNotifications: true}}
	if err := s.commitRecovery(c, Profile{ID: "viewer"}, initialFrame().GetSnapshot(), nil, cursor); err != nil {
		t.Fatal(err)
	}
	state := s.State()
	if state.Notifications["room"].Unread != 7 || state.Notifications["room"].Important != 2 || state.NotificationExpiry.IsZero() {
		t.Fatal("initial counts missing")
	}
	delete(state.Notifications, "room")
	if s.State().Notifications["room"].Unread != 7 {
		t.Fatal("counts share mutable state")
	}
	counts = []NotificationCount{{RoomID: "room", Unread: 3, Important: 3}}
	cursor = "changed"
	if err := s.applyEvent(c, &wire.Event{Id: "change", Cursor: &cursor, Event: &wire.Event_NotificationOccurrencesChanged{NotificationOccurrencesChanged: &wire.Empty{}}}); err != nil {
		t.Fatal(err)
	}
	if s.State().Notifications["room"].Important != 3 {
		t.Fatal("live attention update lost")
	}
	counts = nil
	cursor = "read"
	if err := s.applyEvent(c, &wire.Event{Id: "read", Cursor: &cursor, Event: &wire.Event_NotificationUnreadStateChanged{NotificationUnreadStateChanged: &wire.RoomChanged{RoomId: "room"}}}); err != nil {
		t.Fatal(err)
	}
	if len(s.State().Notifications) != 0 {
		t.Fatal("read notification badge retained")
	}
	s.reset()
	if len(s.State().Notifications) != 0 {
		t.Fatal("private counts survived reset")
	}
}

func TestUnsupportedNotificationCountsDoNotBlockMessaging(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotImplemented) }))
	defer srv.Close()
	c, _ := New(srv.URL, "token")
	s := &Server{options: ConnectionOptions{IncludeNotifications: true}}
	if err := s.refreshNotifications(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}
