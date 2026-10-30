package core

import (
	"context"
	"errors"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// HistoricalRoomChange is one channel transition with mapped destination IDs.
// Unknown times are nil; Hidden reconciles state without visible activity.
type HistoricalRoomChange struct {
	RoomID, OperationKey, ActorID, Kind, Value string
	OccurredAt                                 *time.Time
	Hidden                                     bool
}

// HistoricalMessageChange is one mutation on an existing mapped message.
type HistoricalMessageChange struct {
	RoomID, MessageID, OperationKey, ActorID, Kind, Body, UserID, Emoji string
	OccurredAt                                                          *time.Time
	AssetIDs                                                            []string
	Present                                                             bool
}

func historicalEvent(actor string, at *time.Time, hidden bool) (*evtv1.Event, error) {
	e := newEvent(SystemActorID, &evtv1.Event{ImportContext: &evtv1.ImportContext{HiddenTimeline: hidden, UnknownTime: at == nil}})
	if at != nil {
		e.CreatedAt = timestamppb.New(*at)
		if e.CreatedAt.CheckValid() != nil {
			return nil, invalidArgument("invalid historical time")
		}
	}
	if actor != "" {
		e.ActorId = actor
		e.ImportContext.SourceActorId = &actor
	}
	return e, nil
}

// ImportHistoricalRoomChange applies a single historical room transition. Room
// name claims use the complete room boundary; no archive state is kept here.
func (c *ChattoCore) ImportHistoricalRoomChange(ctx context.Context, in HistoricalRoomChange) error {
	_, err := c.ImportHistoricalRoomEvent(ctx, in)
	return err
}

// ImportHistoricalRoomEvent returns the committed event ID, including on retries.
// Receipts written before event IDs were retained return an empty ID; their
// transitions are never repeated merely to supply a reaction target.
func (c *ChattoCore) ImportHistoricalRoomEvent(ctx context.Context, in HistoricalRoomChange) (string, error) {
	var id string
	err := c.importHistoricalRoomChange(ctx, in, &id)
	return id, err
}

func (c *ChattoCore) importHistoricalRoomChange(ctx context.Context, in HistoricalRoomChange, id *string) error {
	wait := func() error {
		pos, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.RoomSubjectFilter())
		if err != nil {
			return err
		}
		return c.roomModel.waitForDirectoryAndTimeline(ctx, pos)
	}
	o, err := c.operatorOperation(ctx, in.OperationKey, "ImportRoomEvent", in)
	if err != nil {
		return err
	}
	for range 8 {
		if err = o.load(ctx); err != nil {
			return err
		}
		if o.resource != "" {
			if o.resource != in.RoomID {
				*id = o.resource
			}
			return wait()
		}
		pos, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.RoomSubjectFilter())
		if err != nil {
			return err
		}
		if err = c.roomModel.waitForDirectoryAndTimeline(ctx, pos); err != nil {
			return err
		}
		room, err := c.GetRoom(ctx, KindChannel, in.RoomID)
		if err != nil {
			return err
		}
		if room.Universal {
			return invalidArgument("historical room changes require a non-universal channel")
		}
		if in.ActorID != "" {
			if _, err = c.GetUser(ctx, in.ActorID); err != nil {
				return err
			}
		}
		e, err := historicalEvent(in.ActorID, in.OccurredAt, in.Hidden)
		if err != nil {
			return err
		}
		switch in.Kind {
		case "joined", "left":
			if _, err = c.GetUser(ctx, in.Value); err != nil {
				return err
			}
			e.ActorId = in.Value
			if in.Kind == "joined" {
				e.Event = &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: in.RoomID}}
			} else {
				e.Event = &evtv1.Event_UserLeftRoom{UserLeftRoom: &evtv1.UserLeftRoomEvent{RoomId: in.RoomID}}
			}
		case "name", "topic", "purpose":
			name, topic, purpose := room.Name, room.Description, room.Purpose
			switch in.Kind {
			case "name":
				if err = ValidateRoomName(in.Value); err != nil {
					return err
				}
				name = normalizeRoomName(in.Value)
				if c.roomModel.nameClaimSnapshot(name, in.RoomID).ConflictingRoomID != "" {
					return ErrRoomNameExists
				}
			case "topic":
				topic = in.Value
			case "purpose":
				purpose = in.Value
			}
			if err = ValidateRoomDescription(topic); err != nil {
				return err
			}
			if err = ValidateRoomDescription(purpose); err != nil {
				return err
			}
			e.Event = &evtv1.Event_RoomUpdated{RoomUpdated: &evtv1.RoomUpdatedEvent{RoomId: in.RoomID, Name: name, Description: topic, Purpose: &purpose}}
		case "archived":
			e.Event = &evtv1.Event_RoomArchived{RoomArchived: &evtv1.RoomArchivedEvent{RoomId: in.RoomID}}
		case "unarchived":
			e.Event = &evtv1.Event_RoomUnarchived{RoomUnarchived: &evtv1.RoomUnarchivedEvent{RoomId: in.RoomID}}
		default:
			return invalidArgument("unsupported historical room change")
		}
		entries := []evtstream.BatchEntry{{Subject: evtstream.RoomAggregate(in.RoomID).SubjectFor(e), Event: e, Expect: events.ExpectFilterSeq(evtstream.RoomSubjectFilter(), pos.Seq)}, o.receipt(e.Id)}
		_, err = c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			*id = e.Id
			return wait()
		}
		if !errors.Is(err, events.ErrConflict) {
			return err
		}
	}
	return events.ErrConflict
}

// ImportHistoricalMessageChange preserves source-time mutations without live
// effects. Each operation and its permanent retry receipt commit atomically.
func (c *ChattoCore) ImportHistoricalMessageChange(ctx context.Context, in HistoricalMessageChange) error {
	if in.Kind == "reaction" {
		return c.ImportHistoricalReactionChanges(ctx, []HistoricalMessageChange{in})
	}
	o, err := c.operatorOperation(ctx, in.OperationKey, "ImportMessageChange", in)
	if err != nil {
		return err
	}
	agg := evtstream.RoomAggregate(in.RoomID)
	wait := func() error {
		pos, err := c.EventPublisher.LastSubjectPosition(ctx, agg.AllEventsFilter())
		if err != nil {
			return err
		}
		if err = c.roomModel.waitForDirectoryAndTimeline(ctx, pos); err != nil {
			return err
		}
		if err = c.roomModel.waitForThreads(ctx, pos); err != nil {
			return err
		}
		if err = c.roomModel.waitForReactions(ctx, pos); err != nil {
			return err
		}
		if in.Kind == "deleted" {
			if _, retracted, _ := c.roomModel.latestBodyReference(in.MessageID); retracted {
				c.secureDeleteAllMessageBodyEvents(ctx, in.MessageID)
			}
		}
		return nil
	}
	for range 8 {
		if err = o.load(ctx); err != nil {
			return err
		}
		if o.resource != "" {
			return wait()
		}
		pos, err := c.EventPublisher.LastSubjectPosition(ctx, agg.AllEventsFilter())
		if err != nil {
			return err
		}
		if err = wait(); err != nil {
			return err
		}
		if _, err = c.GetRoom(ctx, KindChannel, in.RoomID); err != nil {
			return err
		}
		if in.ActorID != "" {
			if _, err = c.GetUser(ctx, in.ActorID); err != nil {
				return err
			}
		}
		post, err := c.GetRoomEventByEventID(ctx, KindChannel, in.RoomID, in.MessageID)
		if err != nil {
			return err
		}
		if post.GetMessagePosted() == nil || post.GetMessagePosted().GetEchoOfEventId() != "" {
			return ErrMessageNotFound
		}
		if !post.GetMessagePosted().GetHistoricalImport() {
			return invalidArgument("historical changes require an imported message")
		}
		if in.OccurredAt != nil && in.OccurredAt.Before(post.CreatedAt.AsTime()) {
			return invalidArgument("historical mutation precedes message creation")
		}
		var entries []evtstream.BatchEntry
		var deletedAssets []evtstream.BatchEntry
		add := func(e *evtv1.Event) {
			entries = append(entries, evtstream.BatchEntry{Subject: agg.SubjectFor(e), Event: e})
		}
		e, err := historicalEvent(in.ActorID, in.OccurredAt, true)
		if err != nil {
			return err
		}
		switch in.Kind {
		case "edit":
			if _, retracted, _ := c.roomModel.latestBodyReference(in.MessageID); retracted {
				return invalidArgument("cannot edit a deleted message")
			}
			if len(in.Body) > MaxMessageBodyLength {
				return ErrMessageTooLong
			}
			if !HasVisibleContent(in.Body) && len(in.AssetIDs) == 0 {
				return invalidArgument("empty historical edit")
			}
			if err = validateMessageAttachmentAssetIDs(in.AssetIDs); err != nil {
				return err
			}
			bodyID := NewEventID()
			body := &evtv1.MessageBody{CreatedAt: post.CreatedAt, AuthorId: messageAuthorID(post), AssetIds: in.AssetIDs}
			if in.OccurredAt != nil {
				body.UpdatedAt = e.CreatedAt
			}
			if err = c.encryptMessageContent(ctx, body, in.RoomID, in.MessageID, in.MessageID, bodyID, in.Body, nil); err != nil {
				return err
			}
			bodyEvent, _ := historicalEvent(in.ActorID, in.OccurredAt, true)
			bodyEvent.Id = bodyID
			bodyEvent.Event = &evtv1.Event_MessageBody{MessageBody: &evtv1.MessageBodyEvent{RoomId: in.RoomID, EventId: in.MessageID, Body: body}}
			add(bodyEvent)
			e.Event = &evtv1.Event_MessageEdited{MessageEdited: &evtv1.MessageEditedEvent{RoomId: in.RoomID, EventId: in.MessageID}}
			var attached []*evtv1.Event
			var processing []*evtv1.Event
			for _, id := range in.AssetIDs {
				if c.VideoUploadsEnabled {
					if declared, _ := c.assetModel.AssetCreation(id); declared != nil && declared.GetNeedsVideoProcessing() {
						processing = append(processing, newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_AssetProcessingStarted{AssetProcessingStarted: &evtv1.AssetProcessingStartedEvent{AssetId: id, MessageEventId: in.MessageID}}}))
					}
				}
				attached = append(attached, newEvent(SystemActorID, &evtv1.Event{ImportContext: &evtv1.ImportContext{HiddenTimeline: true}, Event: &evtv1.Event_AssetAttached{AssetAttached: &evtv1.AssetAttachedEvent{AssetId: id, RoomId: in.RoomID, MessageEventId: in.MessageID, UserId: body.AuthorId}}}))
			}
			entries, err = c.prepareMessageAssetBatchEntries(ctx, entries, attached, processing)
			if err != nil {
				return err
			}
		case "deleted":
			e.Event = &evtv1.Event_MessageRetracted{MessageRetracted: &evtv1.MessageRetractedEvent{RoomId: in.RoomID, EventId: in.MessageID}}
			if _, retracted, _ := c.roomModel.latestBodyReference(in.MessageID); !retracted {
				body, err := c.currentMessageBody(ctx, in.MessageID)
				if err != nil {
					return err
				}
				for _, id := range body.GetAssetIds() {
					aa := evtstream.AssetAggregate(id)
					tail, err := c.EventPublisher.LastSubjectPosition(ctx, aa.AllEventsFilter())
					if err != nil {
						return err
					}
					if err = c.assetModel.waitForAssets(ctx, tail); err != nil {
						return err
					}
					owned, err := c.assetModel.MessageOwnsAsset(ctx, in.RoomID, in.MessageID, id)
					if err != nil {
						return err
					}
					if !owned {
						continue
					}
					deleted := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_AssetDeleted{AssetDeleted: &evtv1.AssetDeletedEvent{AssetId: id}}})
					deletedAssets = append(deletedAssets, evtstream.BatchEntry{Subject: aa.SubjectFor(deleted), Event: deleted, Expect: events.ExpectFilterSeq(aa.AllEventsFilter(), tail.Seq)})
				}
			}
		case "thread":
			if post.GetMessagePosted().GetInThread() != "" {
				return invalidArgument("thread root must be a channel message")
			}
			if !c.roomModel.threadExists(in.MessageID) {
				e.Event = &evtv1.Event_ThreadCreated{ThreadCreated: &evtv1.ThreadCreatedEvent{RoomId: in.RoomID, ThreadRootEventId: in.MessageID, HistoricalImport: true}}
			}
		default:
			return invalidArgument("unsupported historical message change")
		}
		if e.Event != nil {
			add(e)
		}
		entries = append(entries, deletedAssets...)
		entries = append(entries, o.receipt(in.MessageID))
		if len(entries) > 1 {
			entries[0].Expect = events.ExpectFilterSeq(agg.AllEventsFilter(), pos.Seq)
		}
		_, err = c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			if err = c.assetModel.waitForAssetsCurrent(ctx); err != nil {
				return err
			}
			return wait()
		}
		if !errors.Is(err, events.ErrConflict) {
			return err
		}
	}
	return events.ErrConflict
}
