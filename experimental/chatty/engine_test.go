package main

import (
	"context"
	"encoding/json"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protowire"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Synthetic public wire snapshot; no dependency on the SDK's private wire types.
func serveTestSnapshot(t *testing.T, w http.ResponseWriter, r *http.Request, rooms []chatto.RoomEntry) {
	t.Helper()
	conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
	if err != nil {
		t.Error(err)
		return
	}
	defer conn.Close()
	if _, _, err := conn.ReadMessage(); err != nil {
		return
	}
	field := func(n protowire.Number, b []byte) []byte {
		return protowire.AppendBytes(protowire.AppendTag(nil, n, protowire.BytesType), b)
	}
	snapshot := field(1, field(1, []byte("Test community")))
	for _, entry := range rooms {
		room := append(field(1, []byte(entry.Room.ID)), field(3, []byte(entry.Room.Name))...)
		snapshot = append(snapshot, field(2, field(1, room))...)
	}
	group := append(field(1, []byte("group")), field(2, []byte("Team"))...)
	snapshot = append(snapshot, field(3, group)...)
	if err := conn.WriteMessage(websocket.BinaryMessage, field(5, snapshot)); err != nil {
		return
	}
	caught := append(field(1, []byte("cursor")), protowire.AppendVarint(protowire.AppendTag(nil, 2, protowire.VarintType), 2)...)
	if err := conn.WriteMessage(websocket.BinaryMessage, field(4, caught)); err != nil {
		return
	}
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func TestUncertainSendSurvivesRestartAndRetriesExactly(t *testing.T) {
	var requests []map[string]string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		requests = append(requests, in)
		w.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{"message":{"id":"sent"}}`))
	}))
	defer s.Close()
	e := &engine{ctx: context.Background(), path: filepath.Join(t.TempDir(), "session.json"), state: diskState{Session: chatto.HumanSession{Origin: s.URL, AccessToken: "token", AccessExpiresAt: time.Now().Add(time.Hour)}}}
	if result := e.send("room", "thread", "hello", false); result.Err == nil {
		t.Fatal("uncertain send appeared successful")
	}
	data, err := os.ReadFile(e.path)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(e.path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("session is not private")
	}
	restored := &engine{ctx: context.Background(), path: e.path}
	if err = json.Unmarshal(data, &restored.state); err != nil {
		t.Fatal(err)
	}
	if result := restored.send("other", "", "different", false); result.Err == nil || len(requests) != 1 {
		t.Fatal("pending send overwritten")
	}
	if result := restored.send("", "", "", true); result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(requests) != 2 || requests[0]["idempotencyKey"] != requests[1]["idempotencyKey"] || requests[1]["body"] != "hello" || requests[1]["threadRootEventId"] != "thread" {
		t.Fatal("retry changed send identity")
	}
	data, _ = os.ReadFile(e.path)
	var saved diskState
	_ = json.Unmarshal(data, &saved)
	if saved.Pending != nil {
		t.Fatal("confirmed send retained")
	}
}

func TestLastRoomSurvivesRestartAndUnavailableRoomFallsBack(t *testing.T) {
	var removed, failed atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/realtime":
			rooms := []chatto.RoomEntry{{Room: chatto.Room{ID: "first"}}}
			if !removed.Load() {
				rooms = append(rooms, chatto.RoomEntry{Room: chatto.Room{ID: "second"}})
			}
			serveTestSnapshot(t, w, r, rooms)
		case "/api/connect/chatto.api.v1.ServerService/GetServerProfile":
			_, _ = w.Write([]byte(`{"profile":{"name":"Test community"}}`))
		case "/api/connect/chatto.api.v1.ViewerService/GetViewer":
			_, _ = w.Write([]byte(`{"user":{"profile":{"id":"user"}}}`))
		case "/api/connect/chatto.api.v1.RoomDirectoryService/ListRooms":
			rooms := []chatto.RoomEntry{{Room: chatto.Room{ID: "first"}}}
			if !removed.Load() {
				rooms = append(rooms, chatto.RoomEntry{Room: chatto.Room{ID: "second"}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"rooms": rooms})
		case "/api/connect/chatto.api.v1.RoomDirectoryService/ListRoomGroups", "/api/connect/chatto.api.v1.NotificationService/ListNotificationOccurrences":
			_, _ = w.Write([]byte(`{}`))
		case "/api/connect/chatto.api.v1.RoomService/GetRoomEvents":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if removed.Load() && in["roomId"] == "second" {
				t.Error("requested inaccessible saved room")
			}
			if failed.Load() {
				w.WriteHeader(503)
				return
			}
			_, _ = w.Write([]byte(`{"page":{}}`))
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	}))
	defer s.Close()
	e := &engine{ctx: context.Background(), path: filepath.Join(t.TempDir(), "session.json"), state: diskState{Session: chatto.HumanSession{Origin: s.URL, AccessToken: "token", AccessExpiresAt: time.Now().Add(time.Hour)}}}
	defer e.close()
	if r := e.load("second", "", ""); r.Err != nil || r.Room != "second" {
		t.Fatal("select room", r.Err)
	}
	failed.Store(true)
	if r := e.load("first", "", ""); r.Err == nil {
		t.Fatal("expected read failure")
	}
	failed.Store(false)
	data, err := os.ReadFile(e.path)
	if err != nil {
		t.Fatal(err)
	}
	restored := &engine{ctx: context.Background(), path: e.path}
	defer restored.close()
	if err := json.Unmarshal(data, &restored.state); err != nil {
		t.Fatal(err)
	}
	if r := restored.load("", "", ""); r.Err != nil || r.Room != "second" {
		t.Fatal("restart did not restore last successful room", r.Err)
	}
	removed.Store(true)
	restored.close()
	reopened := &engine{ctx: context.Background(), path: e.path, state: restored.state}
	defer reopened.close()
	if r := reopened.load("", "", ""); r.Err != nil || r.Room != "first" {
		t.Fatal("missing room did not fall back", r.Err)
	}
}

func TestCheckpointFailurePreventsSend(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer s.Close()
	e := &engine{ctx: context.Background(), path: filepath.Join(t.TempDir(), "missing", "session.json"), state: diskState{Session: chatto.HumanSession{Origin: s.URL, AccessToken: "token", AccessExpiresAt: time.Now().Add(time.Hour)}}}
	if e.send("room", "", "hello", false).Err == nil || calls != 0 {
		t.Fatal("sent without durable checkpoint")
	}
}

func TestScrollbackOnlyRequestsTheSelectedTimeline(t *testing.T) {
	for _, root := range []string{"", "thread"} {
		t.Run("root="+root, func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/realtime" {
					serveTestSnapshot(t, w, r, []chatto.RoomEntry{{Room: chatto.Room{ID: "room"}}})
					return
				}
				if r.URL.Path == "/api/connect/chatto.api.v1.ViewerService/GetViewer" {
					_, _ = w.Write([]byte(`{"user":{"profile":{"id":"viewer"}}}`))
					return
				}
				if r.URL.Path == "/api/connect/chatto.api.v1.NotificationService/ListNotificationOccurrences" {
					_, _ = w.Write([]byte(`{}`))
					return
				}
				calls++
				want := "/api/connect/chatto.api.v1.RoomService/GetRoomEvents"
				if root != "" {
					want = "/api/connect/chatto.api.v1.ThreadService/GetThreadEvents"
				}
				if r.URL.Path != want && r.URL.Path != "/api/connect/chatto.api.v1.RoomService/GetRoomEvents" {
					t.Errorf("unnecessary scrollback request: %s", r.URL.Path)
				}
				var in map[string]any
				_ = json.NewDecoder(r.Body).Decode(&in)
				if in["roomId"] != "room" {
					t.Error("lost pagination scope")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"page":{"includes":{"users":{"author":{"id":"author","login":"tester"}}}}}`))
			}))
			defer s.Close()
			e := &engine{ctx: context.Background(), path: filepath.Join(t.TempDir(), "session.json"), state: diskState{Session: chatto.HumanSession{Origin: s.URL, AccessToken: "token", AccessExpiresAt: time.Now().Add(time.Hour)}}}
			defer e.close()
			if r := e.load("room", root, ""); r.Err != nil {
				t.Fatal(r.Err)
			}
			initialCalls := calls
			r := e.load("room", root, "cursor")
			if r.Err != nil || calls != initialCalls+1 || r.Users["author"].Login != "tester" {
				t.Fatal("scrollback did not use one hydrated timeline response", r.Err, calls)
			}
		})
	}
}

func TestHumanLoginRenewalBrowseAndLogout(t *testing.T) {
	file := filepath.Join(t.TempDir(), "session.json")
	refreshed := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch r.URL.Path {
		case "/api/realtime":
			serveTestSnapshot(t, w, r, []chatto.RoomEntry{{Room: chatto.Room{ID: "room", Name: "general"}}})
		case "/api/connect/chatto.api.v1.ServerService/GetServerProfile":
			_, _ = w.Write([]byte(`{"profile":{"name":"Test community"}}`))
		case "/api/connect/chatto.api.v1.NotificationService/ListNotificationOccurrences":
			_, _ = w.Write([]byte(`{}`))
		case "/auth/login":
			_, _ = w.Write([]byte(`{"token":"initial","refreshToken":"renew","expiresIn":1,"refreshTokenExpiresIn":3600}`))
		case "/oauth/token":
			data, err := os.ReadFile(file)
			if err != nil {
				t.Error(err)
			}
			var saved diskState
			_ = json.Unmarshal(data, &saved)
			if saved.Session.RefreshRequestID == "" || saved.Session.RefreshRequestID != in["refresh_request_id"] {
				t.Error("refresh identity was not saved before request")
			}
			refreshed = true
			_, _ = w.Write([]byte(`{"access_token":"fresh","refresh_token":"rotated","expires_in":300,"refresh_token_expires_in":3500}`))
		case "/api/connect/chatto.api.v1.ViewerService/GetViewer":
			if r.Header.Get("Authorization") != "Bearer fresh" {
				t.Error("expired token used")
			}
			_, _ = w.Write([]byte(`{"user":{"profile":{"id":"user","login":"tester"}}}`))
		case "/api/connect/chatto.api.v1.RoomDirectoryService/ListRooms":
			_, _ = w.Write([]byte(`{"rooms":[{"room":{"id":"room","name":"general"}}]}`))
		case "/api/connect/chatto.api.v1.RoomDirectoryService/ListRoomGroups":
			_, _ = w.Write([]byte(`{"groups":[{"id":"group","name":"Team","items":[{"room":{"room":{"id":"room","name":"general"}}}]}]}`))
		case "/api/connect/chatto.api.v1.RoomService/GetRoomEvents":
			_, _ = w.Write([]byte(`{"page":{"events":[{"messagePosted":{"message":{"id":"msg","actorId":"user","body":"hello"}}}],"includes":{"users":{"user":{"id":"user","login":"tester"}}}}}`))
		case "/auth/logout":
			if in["refreshToken"] != "rotated" {
				t.Error("logout lost refresh credential")
			}
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	}))
	defer s.Close()
	e := &engine{ctx: context.Background(), path: file}
	defer e.close()
	if result := e.login(context.Background(), s.URL, "tester", "password", false, nil); result.Err != nil {
		t.Fatal(result.Err)
	}
	r := e.load("", "", "")
	if r.Err != nil || r.ServerName != "Test community" || r.Room != "room" || len(r.Page.Messages) != 1 || len(r.Groups) != 1 || !refreshed {
		t.Fatal("browse after renewal failed", r.Err)
	}
	if result := e.logout(); result.Err != nil {
		t.Fatal(result.Err)
	}
	data, _ := os.ReadFile(file)
	var saved diskState
	_ = json.Unmarshal(data, &saved)
	if saved.Session.AccessToken != "" || saved.LastRoom != "" {
		t.Fatal("logout retained credentials or saved room")
	}
}
