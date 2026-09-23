package core

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// HistoricalMessageInput is one operator import. The source script owns
// deduplication and retry decisions; every successful call creates a message.
type HistoricalMessageInput struct {
	RoomID             string
	AuthorID           string
	CreatedAt          time.Time
	EditedAt           *time.Time
	InReplyTo          string
	AttachmentAssetIDs []string
	Body               string
	LinkPreview        *evtv1.LinkPreview
}

// ImportHistoricalMessage appends a source-time message without user posting
// policy or live posting effects. EVT records the system actor, while the
// message fact and encrypted body retain the mapped historical author.
func (c *ChattoCore) ImportHistoricalMessage(ctx context.Context, input HistoricalMessageInput) (*evtv1.Event, error) {
	if input.RoomID == "" || input.AuthorID == "" || input.CreatedAt.IsZero() {
		return nil, invalidArgument("room_id, author_id, and created_at are required")
	}
	if err := timestamppb.New(input.CreatedAt).CheckValid(); err != nil {
		return nil, invalidArgument("created_at is invalid")
	}
	if input.EditedAt != nil {
		if input.EditedAt.Before(input.CreatedAt) || timestamppb.New(*input.EditedAt).CheckValid() != nil {
			return nil, invalidArgument("edited_at must be valid and no earlier than created_at")
		}
	}
	if err := validateMessageAttachmentAssetIDs(input.AttachmentAssetIDs); err != nil {
		return nil, err
	}
	if len(input.Body) > MaxMessageBodyLength {
		return nil, ErrMessageTooLong
	}
	if !HasVisibleContent(input.Body) && len(input.AttachmentAssetIDs) == 0 {
		return nil, invalidArgument("message must have either body or attachments")
	}
	if err := validateLinkPreview(input.LinkPreview); err != nil {
		return nil, err
	}
	if _, err := c.GetUser(ctx, input.AuthorID); err != nil {
		return nil, err
	}
	validateRoomAndReply := func(attemptCtx context.Context) error {
		if _, err := c.GetUser(attemptCtx, input.AuthorID); err != nil {
			return err
		}
		room, err := c.GetRoom(attemptCtx, KindChannel, input.RoomID)
		if err != nil {
			return err
		}
		if room.Archived {
			return ErrRoomArchived
		}
		if input.InReplyTo != "" {
			target, err := c.GetRoomEventByEventID(attemptCtx, KindChannel, input.RoomID, input.InReplyTo)
			if err != nil {
				return err
			}
			if target == nil || target.GetMessagePosted() == nil || target.GetMessagePosted().GetEchoOfEventId() != "" {
				return invalidArgument("in_reply_to must identify a message in the same room")
			}
		}
		return nil
	}
	if err := validateRoomAndReply(ctx); err != nil {
		return nil, err
	}
	assetIDs := make([]string, 0, len(input.AttachmentAssetIDs))
	seen := make(map[string]struct{}, len(input.AttachmentAssetIDs))
	for _, assetID := range input.AttachmentAssetIDs {
		if _, exists := seen[assetID]; exists {
			return nil, invalidArgument("attachment asset IDs must be unique")
		}
		seen[assetID] = struct{}{}
		if err := c.assetModel.validateAssetAttachment(assetID, input.AuthorID, input.RoomID, "", time.Now()); err != nil {
			return nil, err
		}
		assetIDs = append(assetIDs, assetID)
	}
	messageID, bodyEventID := NewEventID(), NewEventID()
	createdAt := timestamppb.New(input.CreatedAt.UTC())
	messageBody := &evtv1.MessageBody{CreatedAt: createdAt, AuthorId: input.AuthorID, AssetIds: assetIDs, LinkPreview: input.LinkPreview}
	if input.EditedAt != nil {
		messageBody.UpdatedAt = timestamppb.New(input.EditedAt.UTC())
	}
	if err := c.encryptMessageContent(ctx, messageBody, input.RoomID, messageID, messageID, bodyEventID, input.Body, nil); err != nil {
		return nil, err
	}
	bodyEvent := newEvent(SystemActorID, &evtv1.Event{Id: bodyEventID, CreatedAt: createdAt,
		Event: &evtv1.Event_MessageBody{MessageBody: &evtv1.MessageBodyEvent{RoomId: input.RoomID, EventId: messageID, Body: messageBody}},
	})
	postedEvent := newEvent(SystemActorID, &evtv1.Event{Id: messageID, CreatedAt: createdAt,
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{
			RoomId: input.RoomID, InReplyTo: input.InReplyTo, AuthorId: input.AuthorID, HistoricalImport: true,
		}},
	})
	attachedEvents := make([]*evtv1.Event, 0, len(assetIDs))
	processingEvents := make([]*evtv1.Event, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		attachedEvents = append(attachedEvents, newEvent(SystemActorID, &evtv1.Event{
			Event: &evtv1.Event_AssetAttached{AssetAttached: &evtv1.AssetAttachedEvent{
				AssetId: assetID, RoomId: input.RoomID, MessageEventId: messageID, UserId: input.AuthorID,
			}},
		}))
		if c.VideoUploadsEnabled {
			declared, _ := c.assetModel.AssetCreation(assetID)
			if declared != nil && declared.GetNeedsVideoProcessing() {
				processingEvents = append(processingEvents, newEvent(SystemActorID, &evtv1.Event{
					Event: &evtv1.Event_AssetProcessingStarted{AssetProcessingStarted: &evtv1.AssetProcessingStartedEvent{AssetId: assetID, MessageEventId: messageID}},
				}))
			}
		}
	}
	_, err := c.appendMessageWithOptionalThreadCreated(ctx, evtstream.RoomAggregate(input.RoomID), bodyEvent, postedEvent,
		nil, "", false, attachedEvents, processingEvents, validateRoomAndReply, nil)
	if err != nil {
		return nil, fmt.Errorf("import historical message: %w", err)
	}
	return postedEvent, nil
}
