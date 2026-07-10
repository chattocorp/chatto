package core

import (
	"context"
	"fmt"
	"time"

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
	ticker := time.NewTicker(assetCleanupPollEvery)
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
	if deleted == nil || deleted.GetAsset() == nil {
		// Historical deletion facts contain only an ID. Guessing old storage
		// locations is unsafe, so compatibility events remain a no-op.
		return nil
	}
	attachment := attachmentFromAsset(deleted.GetAsset())
	if attachment == nil {
		return fmt.Errorf("asset deletion %s has invalid storage metadata", deleted.GetAssetId())
	}
	if err := s.media().DeleteAttachmentFromStorage(ctx, attachment); err != nil {
		return fmt.Errorf("delete asset %s from storage: %w", deleted.GetAssetId(), err)
	}
	return nil
}
