package core

import (
	"context"
	"errors"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MaxHistoricalMessages bounds message bodies and work in one atomic request.
const MaxHistoricalMessages = 32

// HistoricalMessageOperation imports a message and its ordered reaction changes.
// Local references must name earlier message operation keys in the same call.
// They cannot accompany the corresponding destination event ID. Reaction inputs
// omit RoomID, MessageID, and Kind; the parent supplies these fields.
type HistoricalMessageOperation struct {
	Message             HistoricalMessageInput
	ThreadRootOperation string
	ReplyToOperation    string
	Reactions           []HistoricalMessageChange
}

type preparedHistoricalMessage struct {
	input     HistoricalMessageInput
	post      *evtv1.Event
	receipt   *operatorOperation
	reactions []evtstream.BatchEntry
}

// ImportHistoricalMessages commits new file-free messages and reactions in one
// room atomically, with individual permanent receipts. Results follow input
// order. Retrying subsets or using individual RPCs preserves the same identities.
// All validation precedes encryption. Key provisioning can survive a failed
// attempt, but message facts, thread declarations, reactions, and receipts commit
// together. No archive state or source-platform references are stored here.
func (c *ChattoCore) ImportHistoricalMessages(ctx context.Context, operations []HistoricalMessageOperation) ([]*evtv1.Event, error) {
	if len(operations) == 0 || len(operations) > MaxHistoricalMessages {
		return nil, invalidArgument("expected 1–32 historical messages")
	}
	room := operations[0].Message.RoomID
	reactions := 0
	for _, op := range operations {
		reactions += len(op.Reactions)
		if room == "" || op.Message.RoomID != room || op.Message.OperationKey == "" || op.Message.IdempotencyKey != "" || len(op.Message.AttachmentAssetIDs) != 0 {
			return nil, invalidArgument("batch requires keyed messages without attachments in one channel")
		}
	}
	if reactions > MaxHistoricalReactionChanges {
		return nil, invalidArgument("expected at most 128 reaction changes")
	}
	agg := evtstream.RoomAggregate(room)
	wait := func() (events.StreamPosition, error) {
		pos, err := c.EventPublisher.LastSubjectPosition(ctx, agg.AllEventsFilter())
		if err == nil {
			err = c.roomModel.waitForDirectoryAndTimeline(ctx, pos)
		}
		if err == nil {
			err = c.roomModel.waitForThreads(ctx, pos)
		}
		if err == nil {
			err = c.roomModel.waitForReactions(ctx, pos)
		}
		return pos, err
	}
	var lastErr error
	for range 8 {
		pos, err := wait()
		if err != nil {
			return nil, err
		}
		plans, err := c.prepareHistoricalMessages(ctx, operations)
		if err == nil {
			err = c.appendHistoricalMessages(ctx, room, pos, plans)
		}
		if err == nil {
			if _, err := wait(); err != nil {
				return nil, err
			}
			results := make([]*evtv1.Event, len(plans))
			for i, plan := range plans {
				results[i] = plan.post
			}
			return results, nil
		}
		// A competing batch can commit a parent and child between receipt reads.
		// Resolve local references again before treating a fingerprint conflict as
		// changed input. The same room guard also covers thread creation races.
		lastErr = err
		latest, readErr := c.EventPublisher.LastSubjectPosition(ctx, agg.AllEventsFilter())
		if readErr != nil {
			return nil, readErr
		}
		if !errors.Is(err, events.ErrConflict) && latest.Seq == pos.Seq {
			return nil, err
		}
	}
	// Room activity can trigger retries of non-conflict failures too. Preserve
	// the actual cause when those retries cannot make progress.
	return nil, lastErr
}

func (c *ChattoCore) prepareHistoricalMessages(ctx context.Context, operations []HistoricalMessageOperation) ([]preparedHistoricalMessage, error) {
	room := operations[0].Message.RoomID
	if _, err := c.GetRoom(ctx, KindChannel, room); err != nil {
		return nil, err
	}
	keys, users := map[string]bool{}, map[string]bool{}
	local := map[string]string{}
	posts := map[string]*evtv1.Event{}
	checkUser := func(id string) error {
		if !users[id] {
			if _, err := c.GetUser(ctx, id); err != nil {
				return err
			}
			users[id] = true
		}
		return nil
	}
	claim := func(key, method string, input any) (*operatorOperation, error) {
		o, err := c.operatorOperation(ctx, key, method, input)
		if err != nil {
			return nil, err
		}
		if keys[o.subject] {
			return nil, invalidArgument("duplicate operation key")
		}
		keys[o.subject] = true
		return o, nil
	}
	plans := make([]preparedHistoricalMessage, 0, len(operations))
	for _, op := range operations {
		in := op.Message
		for _, ref := range []struct {
			key    string
			target *string
		}{{op.ThreadRootOperation, &in.ThreadRootEventID}, {op.ReplyToOperation, &in.InReplyTo}} {
			if ref.key == "" {
				continue
			}
			if *ref.target != "" || local[ref.key] == "" {
				return nil, invalidArgument("local reference must name an earlier message and cannot accompany an event ID")
			}
			*ref.target = local[ref.key]
		}
		if err := validateHistoricalMessageInput(in); err != nil {
			return nil, err
		}
		if err := checkUser(in.AuthorID); err != nil {
			return nil, err
		}
		if in.SourceActorID != "" {
			if err := checkUser(in.SourceActorID); err != nil {
				return nil, err
			}
		}
		o, err := claim(in.OperationKey, "ImportMessage", in)
		if err != nil {
			return nil, err
		}
		plan := preparedHistoricalMessage{input: in, receipt: o}
		if o.resource != "" {
			plan.post, err = c.GetRoomEventByEventID(ctx, KindChannel, room, o.resource)
			if err != nil {
				return nil, err
			}
		} else {
			for _, id := range []string{in.ThreadRootEventID, in.InReplyTo} {
				if id == "" {
					continue
				}
				target := posts[id]
				if target == nil {
					target, err = c.GetRoomEventByEventID(ctx, KindChannel, room, id)
					if err != nil {
						return nil, err
					}
					posts[id] = target
				}
				post := target.GetMessagePosted()
				if post == nil || post.GetEchoOfEventId() != "" || (id == in.ThreadRootEventID && post.GetInThread() != "") {
					return nil, invalidArgument("invalid reply or thread root target")
				}
			}
			plan.post = newEvent(SystemActorID, &evtv1.Event{Id: NewEventID(), CreatedAt: timestamppb.New(in.CreatedAt.UTC()), ImportContext: &evtv1.ImportContext{}, Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: room, AuthorId: in.AuthorID, InReplyTo: in.InReplyTo, InThread: in.ThreadRootEventID, HistoricalImport: true}}})
			if in.SourceActorID != "" {
				plan.post.ImportContext.SourceActorId = &in.SourceActorID
			}
		}
		local[in.OperationKey], posts[plan.post.Id] = plan.post.Id, plan.post
		for _, r := range op.Reactions {
			if r.RoomID != "" || r.MessageID != "" || r.Kind != "" || r.Body != "" || len(r.AssetIDs) != 0 {
				return nil, invalidArgument("batch reaction target is supplied by its parent message")
			}
			r.RoomID, r.MessageID, r.Kind = room, plan.post.Id, "reaction"
			receipt, err := claim(r.OperationKey, "ImportMessageChange", r)
			if err != nil {
				return nil, err
			}
			if receipt.resource != "" {
				continue
			}
			if err := checkUser(r.UserID); err != nil {
				return nil, err
			}
			if r.ActorID != "" {
				if err := checkUser(r.ActorID); err != nil {
					return nil, err
				}
			}
			if r.OccurredAt != nil && r.OccurredAt.Before(in.CreatedAt) {
				return nil, invalidArgument("historical mutation precedes message creation")
			}
			e, err := historicalReactionEvent(r)
			if err != nil {
				return nil, err
			}
			plan.reactions = append(plan.reactions, evtstream.BatchEntry{Subject: evtstream.RoomAggregate(room).SubjectFor(e), Event: e}, receipt.receipt(plan.post.Id))
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func (c *ChattoCore) appendHistoricalMessages(ctx context.Context, room string, pos events.StreamPosition, plans []preparedHistoricalMessage) error {
	agg := evtstream.RoomAggregate(room)
	entries := []evtstream.BatchEntry{}
	threads := map[string]bool{}
	add := func(e *evtv1.Event) {
		entries = append(entries, evtstream.BatchEntry{Subject: agg.SubjectFor(e), Event: e})
	}
	for _, plan := range plans {
		in, post := plan.input, plan.post
		if plan.receipt.resource == "" {
			root := in.ThreadRootEventID
			if in.CreateThread {
				root = post.Id
			}
			thread := func() {
				if root != "" && !threads[root] && !c.roomModel.threadExists(root) {
					add(newEvent(SystemActorID, &evtv1.Event{CreatedAt: post.CreatedAt, Event: &evtv1.Event_ThreadCreated{ThreadCreated: &evtv1.ThreadCreatedEvent{RoomId: room, ThreadRootEventId: root, HistoricalImport: true}}}))
					threads[root] = true
				}
			}
			if !in.CreateThread {
				thread()
			}
			if !in.Deleted {
				bodyID := NewEventID()
				body := &evtv1.MessageBody{CreatedAt: post.CreatedAt, AuthorId: in.AuthorID, LinkPreview: in.LinkPreview}
				if in.EditedAt != nil {
					body.UpdatedAt = timestamppb.New(in.EditedAt.UTC())
				}
				if err := c.encryptMessageContent(ctx, body, room, post.Id, post.Id, bodyID, in.Body, nil); err != nil {
					return err
				}
				add(newEvent(SystemActorID, &evtv1.Event{Id: bodyID, CreatedAt: post.CreatedAt, ImportContext: &evtv1.ImportContext{}, Event: &evtv1.Event_MessageBody{MessageBody: &evtv1.MessageBodyEvent{RoomId: room, EventId: post.Id, Body: body}}}))
			}
			add(post)
			if in.CreateThread {
				thread()
			}
			if in.Deleted {
				retracted, err := historicalEvent(in.SourceActorID, in.DeletedAt, true)
				if err != nil {
					return err
				}
				// Retractions keep the system actor, as in single-message imports.
				retracted.ActorId = SystemActorID
				retracted.Event = &evtv1.Event_MessageRetracted{MessageRetracted: &evtv1.MessageRetractedEvent{RoomId: room, EventId: post.Id}}
				add(retracted)
			}
			entries = append(entries, plan.receipt.receipt(post.Id))
		}
		entries = append(entries, plan.reactions...)
	}
	if len(entries) == 0 {
		return nil
	}
	entries[0].Expect = events.ExpectFilterSeq(agg.AllEventsFilter(), pos.Seq)
	_, err := c.EventPublisher.AppendBatch(ctx, entries)
	return err
}
