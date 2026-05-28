package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const assetCreationESMigrationKey = "attachment_declarations_es.migrated"

type assetCreationVerification struct {
	MessageAttachmentCount     int
	CreatedAssetCount          int
	MissingCreations           int
	DanglingProcessingOutcomes int
}

// migrateAssetCreationsToES backfills first-class asset creation events for
// legacy MessagePostedEvent.body.attachments. The old message payloads remain
// unchanged; the new events provide the asset identity/owner records that
// processing outcomes can reference by asset id.
func (c *ChattoCore) migrateAssetCreationsToES(ctx context.Context) error {
	if entry, err := c.storage.serverRuntimeKV.Get(ctx, assetCreationESMigrationKey); err == nil && entry != nil {
		return nil
	} else if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("get sentinel: %w", err)
	}

	existing, err := c.indexAssetCreationsFromEVT(ctx)
	if err != nil {
		return err
	}

	imported := 0
	var appendErr error
	if err := c.scanEVT(ctx, []string{"evt.room.*.message_posted"}, func(event *corev1.Event) {
		if appendErr != nil {
			return
		}
		posted := event.GetMessagePosted()
		if posted == nil || posted.GetRoomId() == "" || posted.GetBody() == nil {
			return
		}
		messageEventID := posted.GetEventId()
		if messageEventID == "" {
			messageEventID = event.GetId()
		}
		if messageEventID == "" {
			return
		}
		for _, att := range posted.GetBody().GetAttachments() {
			if att == nil || att.GetId() == "" || existing[att.GetId()] {
				continue
			}
			declaredAttachment := proto.Clone(att).(*corev1.Attachment)
			if declaredAttachment.MessageBodyId == "" {
				declaredAttachment.MessageBodyId = messageEventID
			}
			asset := assetFromAttachment(declaredAttachment)
			asset.Parent = &corev1.Asset_Message{
				Message: &corev1.MessageAssetParent{
					RoomId:         posted.GetRoomId(),
					MessageEventId: messageEventID,
				},
			}
			declaration := newEvent("", &corev1.Event{
				Event: &corev1.Event_AssetCreated{
					AssetCreated: &corev1.AssetCreatedEvent{
						SourceAvailable: c.attachmentBinaryAvailable(ctx, declaredAttachment),
						Asset:           asset,
					},
				},
			})
			if _, err := c.EventPublisher.AppendEventually(ctx, events.RoomAggregate(posted.GetRoomId()).SubjectFor(declaration), declaration); err != nil {
				appendErr = fmt.Errorf("append asset creation %s: %w", att.GetId(), err)
				return
			}
			existing[att.GetId()] = true
			imported++
		}
	}); err != nil {
		return err
	}
	if appendErr != nil {
		return appendErr
	}

	verification, err := c.verifyAssetCreationsInEVT(ctx)
	if err != nil {
		return err
	}
	if verification.MissingCreations > 0 || verification.DanglingProcessingOutcomes > 0 {
		c.logger.Warn("asset creation verifier found inconsistent EVT references",
			"message_attachments", verification.MessageAttachmentCount,
			"created_assets", verification.CreatedAssetCount,
			"missing_creations", verification.MissingCreations,
			"dangling_processing_outcomes", verification.DanglingProcessingOutcomes)
	}

	if _, err := c.storage.serverRuntimeKV.Put(ctx, assetCreationESMigrationKey, []byte("1")); err != nil {
		return fmt.Errorf("set sentinel: %w", err)
	}
	if imported > 0 {
		c.logger.Info("Imported message asset creations into EVT", "count", imported)
	}
	return nil
}

func (c *ChattoCore) indexAssetCreationsFromEVT(ctx context.Context) (map[string]bool, error) {
	out := make(map[string]bool)
	if err := c.scanEVT(ctx, []string{"evt.room.*.asset_created"}, func(event *corev1.Event) {
		declared := event.GetAssetCreated()
		if declared == nil || declared.GetAsset().GetId() == "" {
			return
		}
		out[declared.GetAsset().GetId()] = true
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *ChattoCore) verifyAssetCreationsInEVT(ctx context.Context) (*assetCreationVerification, error) {
	messageAttachments := make(map[string]struct{})
	declarations := make(map[string]struct{})
	processingRefs := make(map[string]int)

	if err := c.scanEVT(ctx, []string{"evt.room.*.message_posted"}, func(event *corev1.Event) {
		posted := event.GetMessagePosted()
		if posted == nil || posted.GetBody() == nil {
			return
		}
		for _, att := range posted.GetBody().GetAttachments() {
			if att == nil || att.GetId() == "" {
				continue
			}
			messageAttachments[att.GetId()] = struct{}{}
		}
	}); err != nil {
		return nil, err
	}
	if err := c.scanEVT(ctx, []string{"evt.room.*.asset_created"}, func(event *corev1.Event) {
		declared := event.GetAssetCreated()
		if declared == nil || declared.GetAsset().GetId() == "" {
			return
		}
		declarations[declared.GetAsset().GetId()] = struct{}{}
	}); err != nil {
		return nil, err
	}
	if err := c.scanEVT(ctx, []string{"evt.room.*.asset_processing_succeeded", "evt.room.*.asset_processing_failed"}, func(event *corev1.Event) {
		if succeeded := event.GetAssetProcessingSucceeded(); succeeded != nil && succeeded.GetAssetId() != "" {
			processingRefs[succeeded.GetAssetId()]++
		}
		if failed := event.GetAssetProcessingFailed(); failed != nil && failed.GetAssetId() != "" {
			processingRefs[failed.GetAssetId()]++
		}
	}); err != nil {
		return nil, err
	}

	result := &assetCreationVerification{
		MessageAttachmentCount: len(messageAttachments),
		CreatedAssetCount:      len(declarations),
	}
	for attachmentID := range messageAttachments {
		if _, ok := declarations[attachmentID]; !ok {
			result.MissingCreations++
		}
	}
	for attachmentID, count := range processingRefs {
		if _, ok := declarations[attachmentID]; !ok {
			result.DanglingProcessingOutcomes += count
		}
	}
	return result, nil
}
