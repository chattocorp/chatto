package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const assetCreationESMigrationKey = "attachment_declarations_es.migrated"

const (
	runtimeMigrationRunning = "running"
	runtimeMigrationDone    = "done"
)

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
func (c *ChattoCore) migrateAssetCreationsToES(ctx context.Context) (retErr error) {
	revision, claimed, err := c.claimRuntimeMigration(ctx, assetCreationESMigrationKey)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	// On any error path, release the sentinel so a subsequent boot retries
	// the migration instead of waiting forever on a stale "running" marker.
	defer func() {
		if retErr != nil {
			c.releaseRuntimeMigrationOnFailure(assetCreationESMigrationKey, revision, retErr)
		}
	}()

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
			// Optimistic on Unknown: only flip OriginalBinaryAvailable=false when
			// storage definitively confirms the binary is gone. Treating an
			// unreachable backend as "missing" would burn that into EVT.
			originalBinaryAvailable := c.attachmentBinaryStatus(ctx, declaredAttachment) != AttachmentBinaryMissing
			declaration := newEvent(SystemActorID, &corev1.Event{
				Event: &corev1.Event_AssetCreated{
					AssetCreated: &corev1.AssetCreatedEvent{
						OriginalBinaryAvailable: originalBinaryAvailable,
						Asset:                   asset,
						RoomId:                  posted.GetRoomId(),
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

	if err := c.completeRuntimeMigration(ctx, assetCreationESMigrationKey, revision); err != nil {
		return err
	}
	if imported > 0 {
		c.logger.Info("Imported message asset creations into EVT", "count", imported)
	}
	return nil
}

func (c *ChattoCore) claimRuntimeMigration(ctx context.Context, key string) (uint64, bool, error) {
	if err := c.adoptLegacyRuntimeMigrationSentinel(ctx, key); err != nil {
		return 0, false, err
	}

	revision, err := c.storage.runtimeStateKV.Create(ctx, key, []byte(runtimeMigrationRunning))
	if err == nil {
		return revision, true, nil
	}
	if !errors.Is(err, jetstream.ErrKeyExists) {
		return 0, false, fmt.Errorf("claim migration sentinel %s: %w", key, err)
	}
	if err := c.waitRuntimeMigrationDone(ctx, key); err != nil {
		return 0, false, err
	}
	return 0, false, nil
}

func (c *ChattoCore) completeRuntimeMigration(ctx context.Context, key string, revision uint64) error {
	if _, err := c.storage.runtimeStateKV.Update(ctx, key, []byte(runtimeMigrationDone), revision); err != nil {
		return fmt.Errorf("complete migration sentinel %s: %w", key, err)
	}
	return nil
}

// releaseRuntimeMigrationOnFailure deletes the "running" sentinel when a
// migration aborts mid-flight, so the next boot can retry instead of timing
// out in waitRuntimeMigrationDone. Best-effort: a failed delete only means
// the operator has to clear the key manually, no worse than the prior state.
func (c *ChattoCore) releaseRuntimeMigrationOnFailure(key string, revision uint64, cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(revision)); err != nil {
		c.logger.Warn("Failed to release migration sentinel after failure",
			"key", key,
			"original_error", cause,
			"release_error", err)
	}
}

func (c *ChattoCore) waitRuntimeMigrationDone(ctx context.Context, key string) error {
	return c.waitRuntimeMigrationDoneIn(ctx, c.storage.runtimeStateKV, key)
}

func (c *ChattoCore) adoptLegacyRuntimeMigrationSentinel(ctx context.Context, key string) error {
	if _, err := c.storage.runtimeStateKV.Get(ctx, key); err == nil {
		return nil
	} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("get migration sentinel %s: %w", key, err)
	}

	if c.storage.serverRuntimeKV == nil {
		return nil
	}
	entry, err := c.storage.serverRuntimeKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("get legacy migration sentinel %s: %w", key, err)
	}
	if string(entry.Value()) == runtimeMigrationRunning {
		if err := c.waitRuntimeMigrationDoneIn(ctx, c.storage.serverRuntimeKV, key); err != nil {
			return err
		}
	}
	if _, err := c.storage.runtimeStateKV.Create(ctx, key, []byte(runtimeMigrationDone)); err != nil && !errors.Is(err, jetstream.ErrKeyExists) {
		return fmt.Errorf("adopt legacy migration sentinel %s: %w", key, err)
	}
	return nil
}

func (c *ChattoCore) waitRuntimeMigrationDoneIn(ctx context.Context, bucket jetstream.KeyValue, key string) error {
	if bucket == nil {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		entry, err := bucket.Get(waitCtx, key)
		if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return fmt.Errorf("wait for migration sentinel %s: %w", key, err)
		}
		if err == nil && string(entry.Value()) != runtimeMigrationRunning {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timed out waiting for migration sentinel %s", key)
		case <-ticker.C:
		}
	}
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
