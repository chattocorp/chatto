package chatto

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNotificationSignalTargets(t *testing.T) {
	for _, kind := range []string{"directMessageReceived", "directMentionReceived", "replyReceived", "roleMentionReceived", "hereMentionReceived", "allMentionReceived", "followedThreadActivity", "followedRoomActivity", "reactionReceived", "roomMessageReceived", "roomInvitationReceived", "unknownFuture"} {
		n := NotificationOccurrence{Signal: map[string]json.RawMessage{kind: json.RawMessage(`{"message":{"room":{"id":"room"},"eventId":"event","threadRootEventId":"root"}}`)}}
		target, ok := n.MessageTarget()
		want := kind != "roomInvitationReceived" && kind != "unknownFuture"
		if ok != want || ok && (target.EventID != "event" || target.ThreadRootEventID != "root") {
			t.Fatalf("incorrect destination for %s", kind)
		}
	}
}

func TestLivePostsDoNotBridgeAnAnchoredHistoryGap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmtJSON(w, `{"message":{"id":"new","roomId":"room","actorId":"author","body":"newer"}}`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "token")
	s := &Server{ctx: context.Background(), state: emptyState("connected", 1)}
	s.state.Users["author"] = Profile{ID: "author"}
	key := TimelineKey{Room: "room"}
	s.state.Timelines[key] = TimelinePage{Messages: []Message{{ID: "old"}}, HasNewer: true}
	if err := s.refreshMessage(context.Background(), c, "room", "new", false); err != nil {
		t.Fatal(err)
	}
	if len(s.State().Timelines[key].Messages) != 1 {
		t.Fatal("live post bridged unloaded history")
	}
	page := s.state.Timelines[key]
	page.Messages = append(page.Messages, Message{ID: "new", Body: "before edit"})
	s.state.Timelines[key] = page
	if err := s.refreshMessage(context.Background(), c, "room", "new", true); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range s.State().Timelines[key].Messages {
		if m.ID == "new" && m.Body == "newer" {
			found = true
		}
	}
	if !found {
		t.Fatal("anchored history lost a visible edit")
	}
}

func TestAnchoredLoadReplacesCachedWindow(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/connect/chatto.api.v1.RoomService/GetRoomEvents" {
			close(started)
			select {
			case <-finish:
			case <-r.Context().Done():
				return
			}
			fmtJSON(w, `{"page":{"events":[{"messagePosted":{"message":{"id":"stale"}}}]}}`)
			return
		}
		if r.URL.Path != "/api/connect/chatto.api.v1.RoomService/GetRoomEventsAround" {
			t.Error("wrong timeline endpoint")
		}
		fmtJSON(w, `{"page":{"events":[{"messagePosted":{"message":{"id":"anchor"}}}],"hasNewer":true,"endCursor":"forward"}}`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "token")
	s := &Server{ctx: context.Background(), state: emptyState("connected", 1), options: ConnectionOptions{Client: func(context.Context, bool) (*Client, error) { return c, nil }}}
	s.state.Ready = true
	key := TimelineKey{Room: "room"}
	s.state.Timelines[key] = TimelinePage{Messages: []Message{{ID: "latest"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := s.LoadTimeline(ctx, "room", "", "older-cursor"); result <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("paging request did not start")
	}
	p, err := s.LoadTimelineAround(context.Background(), "room", "", "anchor", 50)
	close(finish)
	if err := <-result; err == nil {
		t.Fatal("old paging request crossed anchor replacement")
	}
	if err != nil || len(p.Messages) != 1 || p.Messages[0].ID != "anchor" || !p.HasNewer || p.EndCursor != "forward" {
		t.Fatal("anchor reused or merged disconnected cached window", err)
	}
	if got := s.State().Timelines[key]; len(got.Messages) != 1 || got.Messages[0].ID != "anchor" {
		t.Fatal("anchored window not stored")
	}
}
