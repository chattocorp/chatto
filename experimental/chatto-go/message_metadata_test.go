package chatto

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/wire"
)

func TestMessageMetadataDecodeAndDetachedCopies(t *testing.T) {
	var m Message
	if err := json.Unmarshal([]byte(`{"id":"m","updatedAt":"edited","pinned":true,"thread":{"replyCount":2,"participantPreviewUserIds":["a"],"viewerState":{"isFollowing":true,"hasUnreadReplies":true}},"reactions":[{"emoji":"heart","count":2,"hasReacted":true,"previewUserIds":["a"]}]}`), &m); err != nil {
		t.Fatal(err)
	}
	p := TimelinePage{Messages: []Message{m}}
	copy := clonePage(p)
	copy.Messages[0].Thread.ParticipantPreviewUserIDs[0] = "other"
	*copy.Messages[0].Thread.ViewerState.IsFollowing = false
	*copy.Messages[0].Thread.ViewerState.HasUnreadReplies = false
	copy.Messages[0].Reactions[0].PreviewUserIDs[0] = "other"
	if m.Thread.ParticipantPreviewUserIDs[0] != "a" || !*m.Thread.ViewerState.IsFollowing || !*m.Thread.ViewerState.HasUnreadReplies || m.Reactions[0].PreviewUserIDs[0] != "a" {
		t.Fatal("metadata shares mutable store memory")
	}
	if m.UpdatedAt != "edited" || !m.Pinned || m.Thread.ReplyCount != 2 || !m.Reactions[0].HasReacted {
		t.Fatal("metadata decode failed")
	}
}

func TestMetadataDoesNotLoadOldMessagesAndRemovesMissingResources(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmtJSON(w, `{"messages":[]}`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "token")
	s := &Server{ctx: context.Background(), state: emptyState("connected", 1)}
	key := TimelineKey{Room: "room"}
	s.state.Timelines[key] = TimelinePage{Messages: []Message{{ID: "retained"}}}
	for _, id := range []string{"unloaded", "retained"} {
		if err := s.applyEvent(c, &wire.Event{Id: "event-" + id, Event: &wire.Event_ReactionAdded{ReactionAdded: &wire.MessageChanged{RoomId: "room", MessageEventId: id}}}); err != nil {
			t.Fatal(err)
		}
		if id == "unloaded" && (requests != 0 || len(s.State().Timelines[key].Messages) != 1) {
			t.Fatal("metadata fetched or altered unloaded history")
		}
	}
	if requests != 1 || len(s.State().Timelines[key].Messages) != 0 {
		t.Fatal("missing resource retained after authoritative refresh")
	}
}

func TestRealtimeMessageMetadataAndThreadSummaries(t *testing.T) {
	events := []*wire.Event{
		{Event: &wire.Event_ReactionAdded{ReactionAdded: &wire.MessageChanged{RoomId: "room", MessageEventId: "root"}}},
		{Event: &wire.Event_ReactionRemoved{ReactionRemoved: &wire.MessageChanged{RoomId: "room", MessageEventId: "root"}}},
		{Event: &wire.Event_MessagePinned{MessagePinned: &wire.MessageChanged{RoomId: "room", MessageEventId: "root"}}},
		{Event: &wire.Event_MessageUnpinned{MessageUnpinned: &wire.MessageChanged{RoomId: "room", MessageEventId: "root"}}},
		{Event: &wire.Event_ThreadCreated{ThreadCreated: &wire.ThreadChanged{RoomId: "room", ThreadRootEventId: "root"}}},
		{Event: &wire.Event_ThreadViewerStateChanged{ThreadViewerStateChanged: &wire.ThreadChanged{RoomId: "room", ThreadRootEventId: "root"}}},
		{Event: &wire.Event_MessagePosted{MessagePosted: &wire.MessagePosted{RoomId: "room", ThreadRootEventId: "root"}}},
		{Event: &wire.Event_MessageRetracted{MessageRetracted: &wire.MessageChanged{RoomId: "room", MessageEventId: "reply"}}},
	}
	for _, event := range events {
		t.Run(string(event.ProtoReflect().WhichOneof(event.ProtoReflect().Descriptor().Oneofs().ByName("event")).Name()), func(t *testing.T) {
			fresh := Message{ID: "root", RoomID: "room", ActorID: "author", Pinned: true, UpdatedAt: "edited", Thread: &ThreadSummary{ReplyCount: 4, ParticipantPreviewUserIDs: []string{"participant"}}, Reactions: []MessageReaction{{Emoji: "heart", Count: 2}}}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Chatto-Realtime-Minimum-Cursor") != "boundary" {
					t.Error("metadata read not fenced")
				}
				switch r.URL.Path {
				case "/api/connect/chatto.api.v1.MessageService/GetMessage":
					var in map[string]string
					_ = json.NewDecoder(r.Body).Decode(&in)
					message := fresh
					if in["eventId"] == "event" {
						message = Message{ID: "event", RoomID: "room", ThreadRootEventID: "root"}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"message": message})
				case "/api/connect/chatto.api.v1.MessageService/BatchGetMessages":
					_ = json.NewEncoder(w).Encode(map[string]any{"messages": []Message{fresh}})
				case "/api/connect/chatto.api.v1.UserService/BatchGetUsers":
					fmtJSON(w, `{"users":[{"user":{"id":"participant","login":"participant","avatarUrl":"/avatar"}}]}`)
				default:
					t.Error("unexpected metadata request")
				}
			}))
			defer srv.Close()
			c, _ := New(srv.URL, "token")
			s := &Server{ctx: context.Background(), state: emptyState("connected", 1)}
			s.state.Users["author"] = Profile{ID: "author"}
			key := TimelineKey{Room: "room"}
			s.state.Timelines[key] = TimelinePage{Messages: []Message{{ID: "root", Thread: &ThreadSummary{ReplyCount: 1}}}}
			event.Id, event.Cursor = "event", new("boundary")
			if err := s.applyEvent(c, event); err != nil {
				t.Fatal(err)
			}
			m := s.State().Timelines[key].Messages[0]
			if m.Thread.ReplyCount != 4 || !m.Pinned || len(m.Reactions) != 1 || m.UpdatedAt != "edited" || s.State().Users["participant"].AvatarURL != "/avatar" {
				t.Fatal("realtime metadata stale")
			}
		})
	}
}
