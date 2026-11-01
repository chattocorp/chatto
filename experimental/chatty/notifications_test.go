package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestNotificationBadgeHitAreas(t *testing.T) {
	for _, tc := range []struct {
		x, total, important int
		wantImportant, hit  bool
	}{
		{19, 2, 1, true, true}, {17, 2, 1, true, true},
		{16, 2, 1, false, false}, {15, 2, 1, false, true},
		{13, 2, 1, false, true}, {12, 2, 1, false, false},
		{20, 2, 1, false, false}, {19, 1, 0, false, true},
		{15, 100, 100, true, true}, {14, 100, 100, false, false},
	} {
		important, hit := badgeAt(tc.x, 20, tc.total, tc.important)
		if hit != tc.hit || hit && important != tc.wantImportant {
			t.Fatalf("incorrect badge hit: %+v", tc)
		}
	}
	m := exampleModel()
	m.notifications = map[string]chatto.NotificationCount{"room": {Unread: 1, Important: 1}}
	left, _, _ := m.columns()
	_, cmd := m.Update(tea.MouseClickMsg{X: left - 3, Y: 3, Button: tea.MouseLeft})
	if cmd == nil || m.status != "Finding notification…" || !m.busy {
		t.Fatal("badge click did not start lookup")
	}
}

func TestNotificationJumpLoadsExactRoomOrThreadWindow(t *testing.T) {
	for _, root := range []string{"", "thread"} {
		t.Run("root="+root, func(t *testing.T) {
			var offsets []int
			aroundCalls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/realtime":
					serveTestSnapshot(t, w, r, []chatto.RoomEntry{{Room: chatto.Room{ID: "room"}}})
					return
				case "/api/connect/chatto.api.v1.ViewerService/GetViewer":
					fmt.Fprint(w, `{"user":{"profile":{"id":"viewer"}}}`)
				case "/api/connect/chatto.api.v1.NotificationService/ListNotificationOccurrences":
					var in struct{ Page struct{ Offset, Limit int } }
					_ = json.NewDecoder(r.Body).Decode(&in)
					if in.Page.Limit == 1 {
						fmt.Fprint(w, `{}`)
						return
					}
					offsets = append(offsets, in.Page.Offset)
					if in.Page.Offset == 0 {
						fmt.Fprint(w, `{"occurrences":[{"id":"ordinary","unread":true,"signal":{"roomMessageReceived":{"message":{"room":{"id":"room"},"eventId":"wrong"}}}}],"page":{"hasMore":true}}`)
					} else {
						fmt.Fprintf(w, `{"occurrences":[{"id":"important","unread":true,"attentionLevel":"NOTIFICATION_ATTENTION_LEVEL_IMPORTANT","signal":{"directMentionReceived":{"message":{"room":{"id":"room"},"eventId":"anchor","threadRootEventId":%q}}}}]}`, root)
					}
				case "/api/connect/chatto.api.v1.RoomService/GetRoomEventsAround", "/api/connect/chatto.api.v1.ThreadService/GetThreadEventsAround":
					aroundCalls++
					var in map[string]any
					_ = json.NewDecoder(r.Body).Decode(&in)
					if in["eventId"] != "anchor" || in["roomId"] != "room" || (root != "" && in["threadRootEventId"] != root) {
						t.Error("wrong anchor scope")
					}
					if (root == "") != (r.URL.Path == "/api/connect/chatto.api.v1.RoomService/GetRoomEventsAround") {
						t.Error("wrong timeline API")
					}
					fmt.Fprint(w, `{"page":{"events":[{"messagePosted":{"message":{"id":"anchor","body":"source"}}},{"messagePosted":{"message":{"id":"after","body":"context"}}}],"hasNewer":true}}`)
				case "/api/connect/chatto.api.v1.RoomService/GetRoomEvents":
					fmt.Fprint(w, `{"page":{"events":[]}}`)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
				}
			}))
			defer srv.Close()
			e := &engine{ctx: context.Background(), path: filepath.Join(t.TempDir(), "session.json"), state: diskState{Session: chatto.HumanSession{Origin: srv.URL, AccessToken: "token", AccessExpiresAt: time.Now().Add(time.Hour)}}}
			defer e.close()
			target, state, err := e.notificationDestination(context.Background(), "room", true)
			if err != nil || target.EventID != "anchor" || len(offsets) != 2 || offsets[1] != 1 || aroundCalls != 1 {
				t.Fatal("notification lookup failed", err)
			}
			m := exampleModel()
			m.e = e
			m.compose.SetValue("keep room draft")
			m.acceptNotificationJump(notificationJump{generation: m.generation, target: target, snapshot: state})
			p := m.timeline(root != "")
			if m.root != root || p.page.Messages[*p.selected].ID != "anchor" || !m.history {
				t.Fatal("source was not selected")
			}
			if root != "" && m.focus != 2 || root == "" && m.focus != 1 {
				t.Fatal("wrong pane focused")
			}
			m.setFocus(1)
			if m.compose.Value() != "keep room draft" {
				t.Fatal("navigation lost draft")
			}
			e.releaseHistory("room", root)
			if len(e.server.ReadState(chatto.TimelineKey{Room: "room", Root: root}).Timelines) != 0 {
				t.Fatal("return to live reused old anchor window")
			}
			m.room, m.root = "other", ""
			m.acceptNotificationJump(notificationJump{generation: m.generation, target: target, snapshot: state})
			if m.room != "other" || m.status != "Notification message is no longer available" {
				t.Fatal("late notification restored a removed timeline")
			}
		})
	}
}

func TestStaleNotificationJumpDoesNotNavigate(t *testing.T) {
	m := exampleModel()
	m.busy = true
	m.generation = 4
	m.acceptNotificationJump(notificationJump{generation: 3, target: chatto.NotificationMessageReference{Room: chatto.Room{ID: "other"}, EventID: "old"}})
	if m.room != "room" || m.root != "" {
		t.Fatal("late badge result changed conversation")
	}
}
