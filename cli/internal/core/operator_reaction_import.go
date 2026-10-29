package core

import (
	"context"
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// HistoricalReactionInput identifies one imported reaction. Source timestamps
// are intentionally absent: Slack exports provide membership, not reaction time.
type HistoricalReactionInput struct {
	RoomID         string
	MessageEventID string
	UserID         string
	Emoji          string
}

// ImportHistoricalReaction records a mapped reactor as a system operation.
// It bypasses membership and interactive count limits, but requires an existing
// user and message (including a tombstone), join, or leave event in an active channel. Room OCC makes concurrent repeats
// idempotent. The event time is the import time, not an invented source time.
func (c *ChattoCore) ImportHistoricalReaction(ctx context.Context, input HistoricalReactionInput) (bool, error) {
	if input.RoomID == "" || input.MessageEventID == "" || input.UserID == "" {
		return false, invalidArgument("room_id, message_event_id, and user_id are required")
	}
	emoji, err := resolveEmojiInput(input.Emoji)
	if err != nil {
		return false, err
	}
	agg := evtstream.RoomAggregate(input.RoomID)
	event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{
		RoomId: input.RoomID, Emoji: emoji, UserId: input.UserID, HistoricalImport: true,
	}}})
	subject := agg.SubjectFor(event)
	result, err := c.reactionModel.executeMutation(ctx, events.AtSubject(agg.AllEventsFilter()), func(ctx context.Context, attempt events.MutationAttempt) ([]evtstream.MutationEntry, error) {
		position := events.SubjectPosition(agg.AllEventsFilter(), attempt.ExpectedSequence)
		if err := c.roomModel.waitForDirectoryAndTimeline(ctx, position); err != nil {
			return nil, err
		}
		if err := c.roomModel.waitForReactions(ctx, position); err != nil {
			return nil, err
		}
		// Account state is a separate aggregate. Check it at request/attempt
		// time, as with other operator imports; room mutations remain OCC-bound.
		userPosition, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.UserAggregate(input.UserID).AllEventsFilter())
		if err != nil {
			return nil, err
		}
		if err := c.userModel.waitForUsers(ctx, userPosition); err != nil {
			return nil, err
		}
		if _, err := c.GetUser(ctx, input.UserID); err != nil {
			return nil, err
		}
		room, err := c.GetRoom(ctx, KindChannel, input.RoomID)
		if err != nil {
			return nil, err
		}
		if room.Archived {
			return nil, ErrRoomArchived
		}
		messageID, err := c.canonicalReactionMessageEventID(input.RoomID, input.MessageEventID)
		if err != nil {
			return nil, err
		}
		target, err := c.GetRoomEventByEventID(ctx, KindChannel, input.RoomID, messageID)
		if err != nil {
			return nil, err
		}
		if target.GetMessagePosted() == nil && target.GetUserJoinedRoom() == nil && target.GetUserLeftRoom() == nil {
			return nil, ErrMessageNotFound
		}
		if c.roomModel.reactionMutationSnapshot(input.RoomID, messageID, emoji, input.UserID).Exists {
			return nil, nil
		}
		event.GetReactionAdded().MessageEventId = messageID
		return []evtstream.MutationEntry{{Subject: subject, Event: event}}, nil
	})
	if err != nil || !result.Committed {
		return false, err
	}
	if len(result.Sequences) == 0 {
		return false, fmt.Errorf("historical reaction committed without a sequence")
	}
	if err := c.roomModel.waitForReactions(ctx, events.SubjectPosition(subject, result.Sequences[0])); err != nil {
		return false, err
	}
	return true, nil
}
