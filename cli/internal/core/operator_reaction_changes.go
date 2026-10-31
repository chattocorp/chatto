package core

import (
	"context"
	"errors"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// MaxHistoricalReactionChanges bounds one atomic historical reaction write.
const MaxHistoricalReactionChanges = 128

// historicalReactionEvent is shared by single, reaction-batch, and message-batch
// imports so source actor, unknown time, and live-delivery flags stay identical.
func historicalReactionEvent(in HistoricalMessageChange) (*evtv1.Event, error) {
	emoji, err := HistoricalReactionName(in.Emoji)
	if err != nil {
		return nil, err
	}
	e, err := historicalEvent(in.ActorID, in.OccurredAt, true)
	if err != nil {
		return nil, err
	}
	if in.Present {
		e.Event = &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: in.RoomID, MessageEventId: in.MessageID, UserId: in.UserID, Emoji: emoji, HistoricalImport: true}}
	} else {
		e.ActorId = in.UserID
		e.Event = &evtv1.Event_ReactionRemoved{ReactionRemoved: &evtv1.ReactionRemovedEvent{RoomId: in.RoomID, MessageEventId: in.MessageID, Emoji: emoji}}
	}
	return e, nil
}

// ImportHistoricalReactionChanges commits new reaction operations in order in
// one room, atomically with their individual receipts. Exact retries (including
// retries through ImportHistoricalMessageChange) do not repeat transitions.
// Previously committed operations are skipped. No new operation commits if
// validation fails. An uncertain commit can be resolved with the same keys.
func (c *ChattoCore) ImportHistoricalReactionChanges(ctx context.Context, changes []HistoricalMessageChange) error {
	if len(changes) == 0 || len(changes) > MaxHistoricalReactionChanges {
		return invalidArgument("expected 1–128 historical reaction changes")
	}
	room := changes[0].RoomID
	for _, in := range changes {
		if in.Kind != "reaction" || in.RoomID != room || room == "" || in.Body != "" || len(in.AssetIDs) != 0 {
			return invalidArgument("reaction changes must target one channel")
		}
	}
	agg := evtstream.RoomAggregate(room)
	wait := func() (events.StreamPosition, error) {
		pos, err := c.EventPublisher.LastSubjectPosition(ctx, agg.AllEventsFilter())
		if err != nil {
			return pos, err
		}
		if err = c.roomModel.waitForDirectoryAndTimeline(ctx, pos); err != nil {
			return pos, err
		}
		return pos, c.roomModel.waitForReactions(ctx, pos)
	}
	for range 8 {
		pos, err := wait()
		if err != nil {
			return err
		}
		entries := []evtstream.BatchEntry{}
		keys, users := map[string]bool{}, map[string]bool{}
		posts := map[string]*evtv1.Event{}
		checkedRoom := false
		checkUser := func(id string) error {
			if users[id] {
				return nil
			}
			if _, err := c.GetUser(ctx, id); err != nil {
				return err
			}
			users[id] = true
			return nil
		}
		for _, in := range changes {
			// Reuse the single-operation fingerprint and receipt namespace.
			o, err := c.operatorOperation(ctx, in.OperationKey, "ImportMessageChange", in)
			if err != nil {
				return err
			}
			if keys[o.subject] {
				return invalidArgument("duplicate operation key")
			}
			keys[o.subject] = true
			if o.resource != "" {
				continue
			}
			if !checkedRoom {
				if _, err := c.GetRoom(ctx, KindChannel, room); err != nil {
					return err
				}
				checkedRoom = true
			}
			if in.ActorID != "" {
				if err := checkUser(in.ActorID); err != nil {
					return err
				}
			}
			if err := checkUser(in.UserID); err != nil {
				return err
			}
			post := posts[in.MessageID]
			if post == nil {
				post, err = c.GetRoomEventByEventID(ctx, KindChannel, room, in.MessageID)
				if err != nil {
					return err
				}
				if post.GetMessagePosted() == nil || post.GetMessagePosted().GetEchoOfEventId() != "" {
					return ErrMessageNotFound
				}
				if !post.GetMessagePosted().GetHistoricalImport() {
					return invalidArgument("historical changes require an imported message")
				}
				posts[in.MessageID] = post
			}
			if in.OccurredAt != nil && in.OccurredAt.Before(post.CreatedAt.AsTime()) {
				return invalidArgument("historical mutation precedes message creation")
			}
			e, err := historicalReactionEvent(in)
			if err != nil {
				return err
			}
			entries = append(entries, evtstream.BatchEntry{Subject: agg.SubjectFor(e), Event: e}, o.receipt(in.MessageID))
		}
		if len(entries) == 0 {
			// Another replica can have committed receipts after the initial wait.
			_, err := wait()
			return err
		}
		entries[0].Expect = events.ExpectFilterSeq(agg.AllEventsFilter(), pos.Seq)
		if _, err = c.EventPublisher.AppendBatch(ctx, entries); err == nil {
			_, err = wait()
			return err
		}
		if !errors.Is(err, events.ErrConflict) {
			return err
		}
	}
	return events.ErrConflict
}
