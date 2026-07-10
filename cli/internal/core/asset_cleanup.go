package core

import (
	"context"
	"fmt"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Run starts recoverable physical cleanup for message-owned asset deletion
// facts. Lease handover is safe because storage deletion is idempotent and a
// fresh consumer replays the durable event history.
func (s *AssetModel) Run(ctx context.Context) error {
	if s == nil || s.cleanupLease == nil {
		return fmt.Errorf("asset cleanup lease is not configured")
	}
	return s.cleanupLease.Run(ctx, s.runCleanupLoop)
}

func (s *AssetModel) runCleanupLoop(ctx context.Context) error {
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(assetCleanupHeartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.writeAssetCleanupStatus(ctx); err != nil && ctx.Err() == nil {
					s.logger.Warn("Failed to publish asset cleanup heartbeat", "error", err)
				}
			}
		}
	}()
	defer func() { <-heartbeatDone }()

	s.runAssetCleanupPass(ctx)
	ticker := time.NewTicker(s.cleanupPollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.runAssetCleanupPass(ctx)
		}
	}
}

func (s *AssetModel) runAssetCleanupPass(ctx context.Context) {
	s.setAssetCleanupPassStarted()
	if err := s.writeAssetCleanupStatus(ctx); err != nil && ctx.Err() == nil {
		s.logger.Warn("Failed to publish asset cleanup pass start", "error", err)
	}

	err := s.consumeAssetCleanup(ctx)
	s.setAssetCleanupPassFinished(err)
	if writeErr := s.writeAssetCleanupStatus(ctx); writeErr != nil && ctx.Err() == nil {
		s.logger.Warn("Failed to publish asset cleanup pass result", "error", writeErr)
	}
	if err != nil && ctx.Err() == nil {
		s.logger.Warn("Asset cleanup pass failed", "error", err)
	}
}

func (s *AssetModel) consumeAssetCleanup(ctx context.Context) error {
	if s == nil || s.cleanupConsumer == nil {
		return fmt.Errorf("asset cleanup consumer is not configured")
	}
	return s.cleanupConsumer.Consume(ctx)
}

func (s *AssetModel) cleanupDeletedAsset(ctx context.Context, subjectEvent *events.SubjectEvent) error {
	event := subjectEvent.Event
	deleted := event.GetAssetDeleted()
	if deleted == nil || deleted.GetAssetId() == "" {
		return nil
	}
	aggregateAssetID, ok := events.ParseAssetSubject(subjectEvent.Subject)
	if !ok || aggregateAssetID != deleted.GetAssetId() {
		return fmt.Errorf(
			"asset deletion subject %q does not match payload id %q",
			subjectEvent.Subject,
			deleted.GetAssetId(),
		)
	}
	createdEvents, _, err := s.EventPublisher.SubjectEvents(
		ctx,
		events.AssetAggregate(deleted.GetAssetId()).Subject(events.EventAssetCreated),
	)
	if err != nil {
		return fmt.Errorf("read creation fact for asset %s: %w", deleted.GetAssetId(), err)
	}
	if len(createdEvents) == 0 {
		// Beta room-scoped histories cannot be located from the asset ID alone.
		return nil
	}
	created := createdEvents[len(createdEvents)-1].GetAssetCreated()
	if created.GetAsset().GetId() != deleted.GetAssetId() {
		return fmt.Errorf(
			"asset creation id %q does not match deletion aggregate %q",
			created.GetAsset().GetId(),
			deleted.GetAssetId(),
		)
	}
	if err := s.validateCleanupStorage(deleted.GetAssetId(), created.GetAsset()); err != nil {
		return err
	}
	attachment := attachmentFromAsset(created.GetAsset())
	if attachment == nil {
		return fmt.Errorf("asset creation %s has invalid storage metadata", deleted.GetAssetId())
	}
	if err := s.media().DeleteAttachmentFromStorage(ctx, attachment); err != nil {
		return fmt.Errorf("delete asset %s from storage: %w", deleted.GetAssetId(), err)
	}
	return nil
}

func (s *AssetModel) validateCleanupStorage(assetID string, asset *corev1.AssetRecord) error {
	switch {
	case asset.GetNats() != nil:
		if asset.GetNats().GetKey() != assetID {
			return fmt.Errorf("asset %s has non-canonical NATS key %q", assetID, asset.GetNats().GetKey())
		}
	case asset.GetS3() != nil:
		if s.s3Client == nil {
			return fmt.Errorf("asset %s uses S3 but no S3 client is configured", assetID)
		}
		validKey := false
		for _, candidate := range legacyAttachmentS3KeyCandidates(assetID) {
			if asset.GetS3().GetKey() == candidate {
				validKey = true
				break
			}
		}
		if !validKey {
			return fmt.Errorf("asset %s has non-canonical S3 key %q", assetID, asset.GetS3().GetKey())
		}
		if asset.GetS3().GetBucket() != "" && asset.GetS3().GetBucket() != s.s3Client.Bucket() {
			return fmt.Errorf("asset %s has unexpected S3 bucket %q", assetID, asset.GetS3().GetBucket())
		}
	}
	return nil
}
