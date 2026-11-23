package http_server

import (
	"context"
	"errors"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/chattocorp/chatto/cli/internal/core"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	realtimev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/realtime/v1"
)

// legacyMessagePostedEvent is the message-post mapping for one viewer as it
// was before the mapping was shared across viewers. The test uses it as the
// reference for the shared path. It omits the resume cursor, which has a
// random nonce.
func legacyMessagePostedEvent(t *testing.T, s *HTTPServer, viewerID string, durable *evtv1.Event) *realtimev1.RealtimeEvent {
	t.Helper()
	ctx := context.Background()
	projected := projectRealtimeEvent(viewerID, durable)
	post, err := s.core.HydrateMessagePost(ctx, durable)
	if err != nil {
		t.Fatalf("HydrateMessagePost: %v", err)
	}
	projected.GetMessagePosted().InReplyTo = post.GetInReplyTo()
	projected.GetMessagePosted().Mentions = realtimeMentions(viewerID, post.GetMentions())
	body, err := s.core.GetFullMessageBody(ctx, durable.GetId())
	if err != nil {
		t.Fatalf("GetFullMessageBody: %v", err)
	}
	kind, err := s.core.FindRoomKind(ctx, durable.GetMessagePosted().GetRoomId())
	if err != nil {
		t.Fatalf("FindRoomKind: %v", err)
	}
	projected.GetMessagePosted().RoomKind = realtimeRoomKind(core.ProtoKindForRoomKind(kind))
	if body != nil {
		projected.GetMessagePosted().BodyPlaintext = new(body.Body)
	}
	return projected
}

func TestRealtimeSharedMessagePostMatchesPerViewerMapping(t *testing.T) {
	env := setupWebSocketTestServer(t)
	ctx := env.ctx
	ids := map[string]string{}
	for _, login := range []string{"shared-author", "shared-mentioned", "shared-other", "shared-third"} {
		user, err := env.core.CreateUser(ctx, core.SystemActorID, login, login, "password123")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		ids[login] = user.Id
	}
	room, err := env.core.CreateRoom(ctx, ids["shared-author"], core.KindChannel, "", "shared-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, id := range ids {
		if _, err := env.core.JoinRoom(ctx, id, core.KindChannel, id, room.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
	}
	first, err := env.core.PostMessage(ctx, core.KindChannel, room.Id, ids["shared-author"], "plain first", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	cases := map[string]string{
		"plain":          "a plain message",
		"direct mention": "@shared-mentioned hello",
		"all mention":    "@all @shared-mentioned hello",
	}
	// Viewers alternate between mentioned and not mentioned, so a mention
	// flag that leaks from one viewer into the shared event shows in the next.
	viewers := []string{ids["shared-mentioned"], ids["shared-other"], ids["shared-mentioned"], ids["shared-third"], ids["shared-author"]}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			durable, err := env.core.PostMessage(ctx, core.KindChannel, room.Id, ids["shared-author"], body, nil, "", first.GetId(), nil, false)
			if err != nil {
				t.Fatalf("PostMessage: %v", err)
			}
			envelope := core.NewEVTEventEnvelopeWithDeliverySeq(durable, 42)
			// Map every viewer before any comparison, so that a change to a
			// shared value by a later viewer shows in an earlier frame.
			frames := make([]*realtimev1.RealtimeServerFrame, len(viewers))
			for i, viewerID := range viewers {
				frame, err := env.httpServer.realtimeServerFrameForEvent(ctx, viewerID, envelope)
				if err != nil {
					t.Fatalf("realtimeServerFrameForEvent: %v", err)
				}
				frames[i] = frame
			}
			for i, viewerID := range viewers {
				// Compare the bytes that a client decodes.
				data, err := proto.Marshal(frames[i])
				if err != nil {
					t.Fatalf("Marshal: %v", err)
				}
				decoded := &realtimev1.RealtimeServerFrame{}
				if err := proto.Unmarshal(data, decoded); err != nil {
					t.Fatalf("Unmarshal: %v", err)
				}
				got := decoded.GetEvent()
				if got.GetCursor() == "" {
					t.Fatal("shared message post lost its resume cursor")
				}
				got.Cursor = nil
				want := legacyMessagePostedEvent(t, env.httpServer, viewerID, durable)
				if !proto.Equal(got, want) {
					t.Fatalf("viewer %s:\n got  %v\n want %v", viewerID, got, want)
				}
			}
		})
	}
}

func TestRealtimeSharedMessagePostConcurrentViewers(t *testing.T) {
	env := setupWebSocketTestServer(t)
	ctx := env.ctx
	author, err := env.core.CreateUser(ctx, core.SystemActorID, "race-author", "Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	mentioned, err := env.core.CreateUser(ctx, core.SystemActorID, "race-mentioned", "Mentioned", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(ctx, author.Id, core.KindChannel, "", "race-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, id := range []string{author.Id, mentioned.Id} {
		if _, err := env.core.JoinRoom(ctx, id, core.KindChannel, id, room.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
	}
	durable, err := env.core.PostMessage(ctx, core.KindChannel, room.Id, author.Id, "@race-mentioned hello", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	envelope := core.NewEVTEventEnvelopeWithDeliverySeq(durable, 42)
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := range 40 {
		viewerID, wantIncluded := author.Id, false
		if i%2 == 0 {
			viewerID, wantIncluded = mentioned.Id, true
		}
		wg.Go(func() {
			frame, err := env.httpServer.realtimeServerFrameForEvent(ctx, viewerID, envelope)
			if err != nil {
				errs <- err
				return
			}
			if _, err := proto.Marshal(frame); err != nil {
				errs <- err
				return
			}
			if got := frame.GetEvent().GetMessagePosted().GetMentions()[0].GetIncludesViewer(); got != wantIncluded {
				errs <- errors.New("mention flag of another viewer")
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestRealtimeSharedMessagePostDropsBodyAfterRetraction(t *testing.T) {
	env := setupWebSocketTestServer(t)
	ctx := env.ctx
	author, err := env.core.CreateUser(ctx, core.SystemActorID, "retract-author", "Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	viewer, err := env.core.CreateUser(ctx, core.SystemActorID, "retract-viewer", "Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(ctx, author.Id, core.KindChannel, "", "retract-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, id := range []string{author.Id, viewer.Id} {
		if _, err := env.core.JoinRoom(ctx, id, core.KindChannel, id, room.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
	}
	for name, change := range map[string]func(eventID string) error{
		"retraction": func(eventID string) error {
			return env.core.DeleteMessage(ctx, author.Id, core.KindChannel, room.Id, eventID)
		},
		"edit": func(eventID string) error {
			return env.core.EditMessage(ctx, author.Id, core.KindChannel, room.Id, eventID, "edited body")
		},
	} {
		t.Run(name, func(t *testing.T) {
			durable, err := env.core.PostMessage(ctx, core.KindChannel, room.Id, author.Id, "original body", nil, "", "", nil, false)
			if err != nil {
				t.Fatalf("PostMessage: %v", err)
			}
			envelope := core.NewEVTEventEnvelopeWithDeliverySeq(durable, 42)
			first, err := env.httpServer.realtimeServerFrameForEvent(ctx, author.Id, envelope)
			if err != nil {
				t.Fatalf("realtimeServerFrameForEvent: %v", err)
			}
			if got := first.GetEvent().GetMessagePosted().GetBodyPlaintext(); got != "original body" {
				t.Fatalf("first viewer body = %q", got)
			}
			if err := change(durable.GetId()); err != nil {
				t.Fatalf("change message: %v", err)
			}
			later, err := env.httpServer.realtimeServerFrameForEvent(ctx, viewer.Id, envelope)
			if err != nil {
				t.Fatalf("realtimeServerFrameForEvent: %v", err)
			}
			want := legacyMessagePostedEvent(t, env.httpServer, viewer.Id, durable)
			got := later.GetEvent()
			got.Cursor = nil
			if !proto.Equal(got, want) {
				t.Fatalf("later viewer:\n got  %v\n want %v", got, want)
			}
			if got.GetMessagePosted().GetBodyPlaintext() == "original body" {
				t.Fatal("later viewer got the body from before the change")
			}
		})
	}
}
