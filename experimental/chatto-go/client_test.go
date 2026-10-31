package chatto

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConnectRequests(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Connect-Protocol-Version") != "1" {
			t.Error("invalid request headers")
		}
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/api/connect/chatto.api.v1.RoomService/RefreshTypingIndicator":
			if input["roomId"] != "room" || input["threadRootEventId"] != "root" {
				t.Error("incorrect typing destination")
			}
			fmtJSON(w, `{}`)
		case "/api/connect/chatto.api.v1.ViewerService/GetViewer":
			fmtJSON(w, `{"user":{"profile":{"id":"bot","login":"test-bot","displayName":"Test Bot"}}}`)
		case "/api/connect/chatto.api.v1.ThreadService/GetThreadEvents":
			if input["after"] != "opaque" || input["roomId"] != "room" || input["threadRootEventId"] != "root" {
				t.Error("incorrect thread request")
			}
			fmtJSON(w, `{"page":{"events":[{"messagePosted":{"message":{"id":"reply","roomId":"room","body":"hello","actorId":"user"}}},{"messagePosted":{"message":{"id":"deleted","deletedAt":"2026-01-01T00:00:00Z"}}}],"endCursor":"next","hasOlder":true}}`)
		case "/api/connect/chatto.api.v1.MessageService/CreateMessage":
			if input["inReplyTo"] != "reply" || input["idempotencyKey"] != "key" || input["body"] != "answer" {
				t.Error("incorrect reply request")
			}
			fmtJSON(w, `{"message":{"id":"created"}}`)
		default:
			t.Error("unexpected route")
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	c, err := New(s.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.RefreshTypingIndicator(ctx, "room", "root"); err != nil {
		t.Fatal(err)
	}
	profile, err := c.Viewer(ctx)
	if err != nil || profile.ID != "bot" || profile.Login != "test-bot" || profile.DisplayName != "Test Bot" {
		t.Fatal("authenticated profile was not decoded")
	}
	if id, err := c.ViewerID(ctx); err != nil || id != "bot" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	p, err := c.Thread(ctx, "room", "root", "opaque")
	if err != nil || len(p.Messages) != 1 || p.EndCursor != "next" || !p.HasOlder {
		t.Fatalf("page=%+v err=%v", p, err)
	}
	if err := c.Reply(ctx, "room", "root", "reply", "answer", "key"); err != nil {
		t.Fatal(err)
	}
}

func fmtJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func TestRejectUnsafeOrigins(t *testing.T) {
	for _, origin := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com/path", "https://example.com?token=secret"} {
		if _, err := New(origin, "key"); err == nil {
			t.Errorf("accepted %s", origin)
		}
	}
}
