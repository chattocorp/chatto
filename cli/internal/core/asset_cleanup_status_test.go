package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestAssetCleanupAdminStatusIsSharedAndLeaseAware(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	ctx := testContext(t)
	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	}
	writer, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("writer core: %v", err)
	}
	reader, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("reader core: %v", err)
	}
	acquired, err := writer.assetModel.cleanupLease.TryAcquire(ctx)
	if err != nil || !acquired {
		t.Fatalf("acquire cleanup lease = %v, %v; want true, nil", acquired, err)
	}

	now := time.Now().UTC()
	writeAssetCleanupStatusTestRecord(t, ctx, writer, assetCleanupStatusRecord{
		OwnerID:              writer.assetModel.cleanupLease.OwnerID(),
		UpdatedAt:            now,
		InitialScanComplete:  true,
		LastPassAt:           now.Add(-time.Second),
		LastSuccessfulPassAt: now.Add(-time.Second),
	})
	status, err := reader.assetModel.AdminCleanupStatus(ctx)
	if err != nil {
		t.Fatalf("AdminCleanupStatus healthy: %v", err)
	}
	if status.Health != AssetCleanupHealthHealthy {
		t.Fatalf("healthy status = %+v", status)
	}
	appendAssetDeletionTestEvent(t, ctx, writer, &corev1.AssetDeletedEvent{AssetId: "A-awaiting-inspection"})
	status, err = reader.assetModel.AdminCleanupStatus(ctx)
	if err != nil {
		t.Fatalf("AdminCleanupStatus awaiting inspection: %v", err)
	}
	if status.Health != AssetCleanupHealthRetrying || status.LatestDeletionSeq == 0 {
		t.Fatalf("awaiting-inspection status = %+v", status)
	}

	oldest := now.Add(-time.Hour)
	writeAssetCleanupStatusTestRecord(t, ctx, writer, assetCleanupStatusRecord{
		OwnerID:             writer.assetModel.cleanupLease.OwnerID(),
		UpdatedAt:           now,
		InitialScanComplete: true,
		PendingCount:        2,
		OldestPendingAt:     oldest,
		LastPassFailed:      true,
	})
	status, err = reader.assetModel.AdminCleanupStatus(ctx)
	if err != nil {
		t.Fatalf("AdminCleanupStatus retrying: %v", err)
	}
	if status.Health != AssetCleanupHealthRetrying || status.PendingCount != 2 || !status.OldestPendingAt.Equal(oldest) {
		t.Fatalf("retrying status = %+v", status)
	}

	writeAssetCleanupStatusTestRecord(t, ctx, writer, assetCleanupStatusRecord{
		OwnerID:             writer.assetModel.cleanupLease.OwnerID(),
		UpdatedAt:           now.Add(-assetCleanupHeartbeatStale - time.Second),
		InitialScanComplete: true,
	})
	status, err = reader.assetModel.AdminCleanupStatus(ctx)
	if err != nil {
		t.Fatalf("AdminCleanupStatus stalled: %v", err)
	}
	if status.Health != AssetCleanupHealthStalled {
		t.Fatalf("stalled health = %v, want %v", status.Health, AssetCleanupHealthStalled)
	}

	writeAssetCleanupStatusTestRecord(t, ctx, writer, assetCleanupStatusRecord{
		OwnerID:   "previous-owner",
		UpdatedAt: now,
	})
	status, err = reader.assetModel.AdminCleanupStatus(ctx)
	if err != nil {
		t.Fatalf("AdminCleanupStatus owner handover: %v", err)
	}
	if status.Health != AssetCleanupHealthInitializing {
		t.Fatalf("owner handover health = %v, want %v", status.Health, AssetCleanupHealthInitializing)
	}
}

func TestAssetCleanupAdminStatusIsInactiveWithoutLease(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	ctx := testContext(t)
	core, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	writeAssetCleanupStatusTestRecord(t, ctx, core, assetCleanupStatusRecord{
		OwnerID:      "previous-owner",
		UpdatedAt:    time.Now().Add(-time.Minute),
		PendingCount: 3,
	})
	status, err := core.assetModel.AdminCleanupStatus(ctx)
	if err != nil {
		t.Fatalf("AdminCleanupStatus: %v", err)
	}
	if status.Health != AssetCleanupHealthInactive || status.PendingCount != 3 {
		t.Fatalf("status = %+v, want inactive with last known pending count", status)
	}
}

func TestAssetCleanupPassPublishesSharedStatus(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	ctx := testContext(t)
	core, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	acquired, err := core.assetModel.cleanupLease.TryAcquire(ctx)
	if err != nil || !acquired {
		t.Fatalf("acquire cleanup lease = %v, %v; want true, nil", acquired, err)
	}

	core.assetModel.runAssetCleanupPass(ctx)
	entry, err := core.storage.memoryCacheKV.Get(ctx, assetCleanupStatusKey)
	if err != nil {
		t.Fatalf("read published status: %v", err)
	}
	var record assetCleanupStatusRecord
	if err := json.Unmarshal(entry.Value(), &record); err != nil {
		t.Fatalf("decode published status: %v", err)
	}
	if !record.InitialScanComplete || record.PassInProgress || record.LastPassFailed {
		t.Fatalf("published status = %+v", record)
	}
	if record.OwnerID != core.assetModel.cleanupLease.OwnerID() || record.LastPassAt.IsZero() || record.LastSuccessfulPassAt.IsZero() {
		t.Fatalf("published identity/timestamps = %+v", record)
	}
}

func writeAssetCleanupStatusTestRecord(t *testing.T, ctx context.Context, core *ChattoCore, record assetCleanupStatusRecord) {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	if _, err := core.storage.memoryCacheKV.Put(ctx, assetCleanupStatusKey, data); err != nil {
		t.Fatalf("write status: %v", err)
	}
}
