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
	if err := s.consumeAssetCleanup(ctx); err != nil {
		s.logger.Warn("Asset cleanup pass failed", "error", err)
	}
	ticker := time.NewTicker(s.cleanupPollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.consumeAssetCleanup(ctx); err != nil {
				s.logger.Warn("Asset cleanup pass failed", "error", err)
			}
		}
	}
}

func (s *AssetModel) consumeAssetCleanup(ctx context.Context) error {
	if s == nil || s.cleanupConsumer == nil {
		return fmt.Errorf("asset cleanup consumer is not configured")
	}
	return s.cleanupConsumer.Consume(ctx)
}

func (s *AssetModel) cleanupDeletedAsset(ctx context.Context, event *corev1.Event) error {
	deleted := event.GetAssetDeleted()
	if deleted == nil || deleted.GetAssetId() == "" {
		return nil
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
	attachment := attachmentFromAsset(created.GetAsset())
	if attachment == nil {
		return fmt.Errorf("asset creation %s has invalid storage metadata", deleted.GetAssetId())
	}
	if err := s.media().DeleteAttachmentFromStorage(ctx, attachment); err != nil {
		return fmt.Errorf("delete asset %s from storage: %w", deleted.GetAssetId(), err)
	}
	return nil
}
