package core

import (
	"context"
	"testing"
	"time"

	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestAssetCleanupAdminStatusUsesSharedDurableConsumer(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := testContext(t)
	cfg := config.CoreConfig{SecretKey: "test-core-secret", Assets: config.AssetsConfig{SigningSecret: "test-signing-secret"}}
	workerCore, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("worker core: %v", err)
	}
	readerCore, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("reader core: %v", err)
	}

	appendAssetDeletionTestEvent(t, ctx, workerCore, &corev1.AssetDeletedEvent{AssetId: "A-status"})
	status, err := readerCore.assetModel.AdminCleanupStatus(ctx)
	if err != nil {
		t.Fatalf("AdminCleanupStatus pending: %v", err)
	}
	if status.Health != AssetCleanupHealthRetrying || status.PendingCount != 1 || status.LatestDeletionSeq == 0 {
		t.Fatalf("pending status = %+v", status)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- workerCore.assetModel.Run(workerCtx) }()
	// Start the worker before its projection. The deletion must remain pending
	// until the projection has replayed through the delivery's stream sequence.
	startAssetProjectionForCleanupTest(t, workerCore.assetModel)
	waitForRecoveryTest(t, 5*time.Second, func() bool {
		status, err = readerCore.assetModel.AdminCleanupStatus(ctx)
		return err == nil && status.Health == AssetCleanupHealthHealthy && status.PendingCount == 0
	}, "shared durable asset cleanup consumer to settle")
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("asset cleanup worker shutdown: %v", err)
	}
}

func TestAssetCleanupAdminStatusUnavailableWithoutConsumer(t *testing.T) {
	core, _ := setupTestCore(t)
	core.assetModel.cleanupConsumer = nil
	status, err := core.assetModel.AdminCleanupStatus(testContext(t))
	if err == nil || status.Health != AssetCleanupHealthUnavailable {
		t.Fatalf("status, error = %+v, %v; want unavailable", status, err)
	}
}

func TestAssetCleanupAdminStatusDoesNotInferWorkerLiveness(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := testContext(t)
	core, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}

	status, err := core.assetModel.AdminCleanupStatus(ctx)
	if err != nil {
		t.Fatalf("AdminCleanupStatus: %v", err)
	}
	if status.Health != AssetCleanupHealthHealthy || status.PendingCount != 0 {
		t.Fatalf("queue status = %+v, want healthy empty queue", status)
	}
	if !status.UpdatedAt.IsZero() {
		t.Fatalf("updated time = %v, want unknown without a real heartbeat", status.UpdatedAt)
	}
}
