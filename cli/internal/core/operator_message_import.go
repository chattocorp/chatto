package core

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// HistoricalMessageInput is one operator import. A UUID enables the same
// bounded retry window as interactive sends, scoped separately to imports.
type HistoricalMessageInput struct {
	RoomID             string
	AuthorID           string
	CreatedAt          time.Time
	EditedAt           *time.Time
	InReplyTo          string
	AttachmentAssetIDs []string
	Body               string
	LinkPreview        *evtv1.LinkPreview
	// ThreadRootEventID places a reply explicitly; InReplyTo stays attribution only.
	ThreadRootEventID string
	// CreateThread establishes an empty thread on the new root without a follow.
	CreateThread bool
	// IdempotencyKey reserves one import identity for MessageIdempotencyWindow.
	IdempotencyKey string
}

const messageImportClaimPrefix = "message_import."

// claimHistoricalMessage uses the shared send reservation without granting
// interactive posting privileges to the historical author.
func (c *ChattoCore) claimHistoricalMessage(ctx context.Context, input HistoricalMessageInput, authorize func(context.Context) error) (*messagePostClaim, error) {
	if input.IdempotencyKey == "" {
		return nil, nil
	}
	key, err := messageClaimKey(messageImportClaimPrefix, SystemActorID, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	input.IdempotencyKey = ""
	input.CreatedAt = input.CreatedAt.UTC()
	if input.EditedAt != nil {
		edited := input.EditedAt.UTC()
		input.EditedAt = &edited
	}
	if len(input.AttachmentAssetIDs) == 0 {
		input.AttachmentAssetIDs = nil
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode historical message intent: %w", err)
	}
	fingerprint, err := hex.DecodeString(c.runtimeTokenHash("message_import_request_v1", string(data)))
	if err != nil {
		return nil, err
	}
	return c.claimMessageIdentity(ctx, key, fingerprint, authorize)
}

// lookupHistoricalMessage runs at a captured room cut or inside room OCC.
// Operator access is root-equivalent; a committed result survives retraction
// and archival without rechecking interactive read or posting permissions.
func (c *ChattoCore) lookupHistoricalMessage(ctx context.Context, input HistoricalMessageInput, claim *messagePostClaim) error {
	entry, exists := c.roomModel.timelineEntry(claim.messageID)
	if !exists {
		return nil
	}
	if !entry.IsMessagePost() || entry.RoomID != input.RoomID {
		return fmt.Errorf("historical claim points to an invalid identity")
	}
	stored, err := c.eventReader.EventAt(ctx, entry.StreamSeq)
	if err != nil {
		return err
	}
	if stored.Event.GetActorId() != SystemActorID || !stored.Event.GetMessagePosted().GetHistoricalImport() || messageAuthorID(stored.Event) != input.AuthorID {
		return fmt.Errorf("historical claim points to an invalid author or operation")
	}
	claim.committed = stored
	return nil
}

func (c *ChattoCore) readHistoricalClaim(ctx context.Context, input HistoricalMessageInput, claim *messagePostClaim) error {
	position, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.RoomAggregate(input.RoomID).AllEventsFilter())
	if err != nil {
		return err
	}
	if err := c.roomModel.waitForDirectoryAndTimeline(ctx, position); err != nil {
		return err
	}
	return c.lookupHistoricalMessage(ctx, input, claim)
}

// ImportHistoricalMessage appends a source-time message without user posting
// policy or live posting effects. EVT records the system actor, while the
// message fact and encrypted body retain the mapped historical author.
func (c *ChattoCore) ImportHistoricalMessage(ctx context.Context, input HistoricalMessageInput) (result *evtv1.Event, resultErr error) {
	if input.IdempotencyKey != "" {
		if _, err := messageClaimKey(messageImportClaimPrefix, SystemActorID, input.IdempotencyKey); err != nil {
			return nil, err
		}
	}
	if input.RoomID == "" || input.AuthorID == "" || input.CreatedAt.IsZero() {
		return nil, invalidArgument("room_id, author_id, and created_at are required")
	}
	if input.CreateThread && input.ThreadRootEventID != "" {
		return nil, invalidArgument("create_thread and thread_root_event_id are mutually exclusive")
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
	if input.LinkPreview != nil {
		parsed, err := url.Parse(input.LinkPreview.GetUrl())
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, invalidArgument("preview_url must be an HTTP or HTTPS URL with a host")
		}
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
		if input.ThreadRootEventID != "" {
			root, err := c.GetRoomEventByEventID(attemptCtx, KindChannel, input.RoomID, input.ThreadRootEventID)
			if err != nil {
				return err
			}
			posted := root.GetMessagePosted()
			if posted == nil || posted.GetInThread() != "" || posted.GetEchoOfEventId() != "" || c.roomModel.messageTombstoned(input.ThreadRootEventID) {
				return invalidArgument("thread_root_event_id must identify a root message in the same room")
			}
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
	claim, err := c.claimHistoricalMessage(ctx, input, validateRoomAndReply)
	if err != nil {
		return nil, err
	}
	if claim != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, claim.expiresAt)
		defer cancel()
		if err := c.readHistoricalClaim(ctx, input, claim); err != nil {
			return nil, err
		}
		if claim.committed != nil {
			return claim.committed.Event, nil
		}
		// A concurrent commit can attach assets or change room state while this
		// request prepares. Resolve that result before returning a write error.
		defer func() {
			if resultErr != nil {
				if err := c.readHistoricalClaim(ctx, input, claim); err == nil && claim.committed != nil {
					result, resultErr = claim.committed.Event, nil
				}
			}
		}()
	}
	authorize := func(ctx context.Context) error {
		if claim != nil {
			if err := c.lookupHistoricalMessage(ctx, input, claim); err != nil {
				return err
			}
			if claim.committed != nil {
				return nil
			}
		}
		return validateRoomAndReply(ctx)
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
	if claim != nil {
		messageID = claim.messageID
	}
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
			RoomId: input.RoomID, InReplyTo: input.InReplyTo, InThread: input.ThreadRootEventID, AuthorId: input.AuthorID, HistoricalImport: true,
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
	var threadCreated *evtv1.Event
	if input.CreateThread || input.ThreadRootEventID != "" {
		rootID := input.ThreadRootEventID
		if input.CreateThread {
			rootID = messageID
		}
		threadCreated = newEvent(SystemActorID, &evtv1.Event{CreatedAt: createdAt,
			Event: &evtv1.Event_ThreadCreated{ThreadCreated: &evtv1.ThreadCreatedEvent{
				RoomId: input.RoomID, ThreadRootEventId: rootID, HistoricalImport: true,
			}},
		})
	}
	if input.CreateThread {
		_, err = c.appendRootMessageWithThread(ctx, evtstream.RoomAggregate(input.RoomID), bodyEvent, postedEvent,
			threadCreated, nil, attachedEvents, processingEvents, authorize, nil, claim)
	} else {
		_, err = c.appendMessageWithOptionalThreadCreated(ctx, evtstream.RoomAggregate(input.RoomID), bodyEvent, postedEvent,
			threadCreated, input.ThreadRootEventID, false, attachedEvents, processingEvents, authorize, nil, claim)
	}
	if err != nil {
		return nil, fmt.Errorf("import historical message: %w", err)
	}
	if claim != nil && claim.committed != nil {
		return claim.committed.Event, nil
	}
	return postedEvent, nil
}
