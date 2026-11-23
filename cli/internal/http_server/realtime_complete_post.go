package http_server

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	realtimev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/realtime/v1"
)

// markCompleteRealtimePost sets `complete` and `viewer_state` on a public post
// when the event holds every value of the viewer's message resource. Clients
// then show the post without a `BatchGetMessages` read.
//
// Only an unedited room-level post qualifies. It must have its body, no
// attachments, link preview, reactions, pin, thread, or channel echo. Each
// later change to these values has its own event, which makes clients read the
// message. The conditions and the viewer state follow the room timeline
// assembler in connectapi (`timelineHydrator.messagePosted`). Keep them equal.
func (s *HTTPServer) markCompleteRealtimePost(ctx context.Context, viewerID string, kind core.RoomKind, event *evtv1.Event, body *core.DecryptedMessageBody, posted *realtimev1.MessagePostedEvent) error {
	post := event.GetMessagePosted()
	if body == nil || post == nil || post.GetInThread() != "" || post.GetEchoOfEventId() != "" ||
		core.MessageAuthorID(event) != event.GetActorId() || body.MessageEventID != event.GetId() ||
		len(body.Attachments) > 0 || body.LinkPreview != nil || body.UpdatedAt != nil {
		return nil
	}
	state, err := s.core.RoomTimelineReads().MessageHydrationState(event.GetId())
	if err != nil {
		return err
	}
	if state.HasDeletedAt || state.Pinned || state.ChannelEchoEventID != "" {
		return nil
	}
	if metadata := s.core.GetThreadMetadata(event.GetId()); metadata != nil && metadata.Exists {
		return nil
	}
	reactions, err := s.core.GetReactionsBatch(ctx, []string{event.GetId()})
	if err != nil {
		return err
	}
	if len(reactions[event.GetId()]) > 0 {
		return nil
	}
	roomID := post.GetRoomId()
	if rootID, ok := s.core.MessageEventThreadRoot(roomID, event); ok {
		canReply, err := s.core.CanReplyInThread(ctx, viewerID, kind, roomID, rootID)
		if err != nil {
			return err
		}
		viewerState := &apiv1.MessageViewerState{CanReplyInThread: &canReply}
		if kind == core.KindChannel {
			next, err := s.core.Messages().ThreadSlowModeNextPostAt(ctx, roomID, rootID, viewerID)
			if err != nil {
				return err
			}
			if !next.IsZero() {
				viewerState.ThreadSlowModeNextPostAt = timestamppb.New(next)
			}
		}
		posted.ViewerState = viewerState
	}
	posted.Complete = true
	return nil
}
