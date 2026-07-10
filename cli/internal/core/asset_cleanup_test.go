package core

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/lease"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestAssetCleanupReplaysDeletionAndIsIdempotent(t *testing.T) {
	core, _ := setupTestCoreWithCache(t)
	ctx := testContext(t)
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "asset-cleanup-replay", "Asset cleanup replay")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	attachment, err := core.media().UploadAttachment(ctx, SystemActorID, room.GetId(), "replay.txt", "text/plain", bytes.NewReader([]byte("replay")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	cacheKey := ImageCacheKey(AttachmentSignResource, attachment.GetId(), 32, 32, "cover")
	if err := core.media().StoreCachedResize(ctx, cacheKey, []byte("cached")); err != nil {
		t.Fatalf("StoreCachedResize: %v", err)
	}
	if err := core.assetLifecycle().RecordAssetDeleted(ctx, SystemActorID, room.GetId(), attachment.GetId()); err != nil {
		t.Fatalf("RecordAssetDeleted: %v", err)
	}

	restarted := NewAssetModel(core)
	if err := restarted.consumeAssetCleanup(ctx); err != nil {
		t.Fatalf("consumeAssetCleanup after restart: %v", err)
	}
	if _, _, err := core.media().GetAttachmentReader(ctx, attachment); err == nil {
		t.Fatal("attachment remained readable after replayed cleanup")
	}
	if got, err := core.media().GetCachedResize(ctx, cacheKey); err != nil || got != nil {
		t.Fatalf("cached resize after replayed cleanup = %q, %v; want nil, nil", got, err)
	}

	secondRestart := NewAssetModel(core)
	if err := secondRestart.consumeAssetCleanup(ctx); err != nil {
		t.Fatalf("idempotent cleanup after second restart: %v", err)
	}
}

func TestAssetCleanupSkipsDeletionWithoutCanonicalCreationFact(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	appendAssetDeletionTestEvent(t, ctx, core, &corev1.AssetDeletedEvent{AssetId: "A-historical"})

	restarted := NewAssetModel(core)
	if err := restarted.consumeAssetCleanup(ctx); err != nil {
		t.Fatalf("consume historical deletion: %v", err)
	}
}

func TestAssetCleanupFailureDoesNotBlockLaterDeletion(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	badAsset := &corev1.AssetRecord{
		Id:      "A-bad-s3",
		Storage: &corev1.AssetRecord_S3{S3: &corev1.S3Asset{Key: "unavailable"}},
	}
	appendAssetCreationTestEvent(t, ctx, core, badAsset)
	appendAssetDeletionTestEvent(t, ctx, core, &corev1.AssetDeletedEvent{AssetId: badAsset.GetId()})

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "asset-cleanup-independent", "Asset cleanup independent")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	attachment, err := core.media().UploadAttachment(ctx, SystemActorID, room.GetId(), "later.txt", "text/plain", bytes.NewReader([]byte("later")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if err := core.assetLifecycle().RecordAssetDeleted(ctx, SystemActorID, room.GetId(), attachment.GetId()); err != nil {
		t.Fatalf("RecordAssetDeleted: %v", err)
	}

	restarted := NewAssetModel(core)
	if err := restarted.consumeAssetCleanup(ctx); err == nil {
		t.Fatal("consumeAssetCleanup returned nil despite unavailable S3 deletion")
	}
	if _, _, err := core.media().GetAttachmentReader(ctx, attachment); err == nil {
		t.Fatal("later attachment remained readable after an earlier permanent failure")
	}
}

func TestAssetCleanupLeaseProcessesNonHolderCommitsAndHandsOver(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	ctx := testContext(t)
	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	}
	first, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("first core: %v", err)
	}
	second, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("second core: %v", err)
	}
	first.assetModel.cleanupLease = newAssetCleanupTestLease(t, first, "first")
	second.assetModel.cleanupLease = newAssetCleanupTestLease(t, second, "second")
	first.assetModel.cleanupPollEvery = 10 * time.Millisecond
	second.assetModel.cleanupPollEvery = 10 * time.Millisecond

	acquired, err := first.assetModel.cleanupLease.TryAcquire(ctx)
	if err != nil || !acquired {
		t.Fatalf("first cleanup lease acquisition = %v, %v; want true, nil", acquired, err)
	}
	acquired, err = second.assetModel.cleanupLease.TryAcquire(ctx)
	if err != nil || acquired {
		t.Fatalf("second cleanup lease acquisition = %v, %v; want false, nil", acquired, err)
	}

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() { firstDone <- first.assetModel.Run(firstCtx) }()
	go func() { secondDone <- second.assetModel.Run(secondCtx) }()
	t.Cleanup(func() {
		cancelFirst()
		cancelSecond()
	})

	store, err := first.GetAttachmentsStore(ctx)
	if err != nil {
		t.Fatalf("GetAttachmentsStore: %v", err)
	}
	appendNATSAssetDeletionTestFacts(t, ctx, second, store, "A-non-holder")
	waitForAssetObjectDeleted(t, ctx, store, "A-non-holder")

	cancelFirst()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first cleanup runner shutdown = %v, want context canceled", err)
	}
	appendNATSAssetDeletionTestFacts(t, ctx, first, store, "A-handover")
	waitForAssetObjectDeleted(t, ctx, store, "A-handover")

	cancelSecond()
	if err := <-secondDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("second cleanup runner shutdown = %v, want context canceled", err)
	}
}

func newAssetCleanupTestLease(t *testing.T, core *ChattoCore, ownerID string) *lease.Lease {
	t.Helper()
	l, err := lease.New(core.js, core.storage.memoryCacheKV, lease.Options{
		Name:       assetCleanupLeaseName,
		OwnerID:    ownerID,
		Bucket:     "MEMORY_CACHE",
		TTL:        time.Second,
		RenewEvery: 200 * time.Millisecond,
		RetryEvery: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new asset cleanup lease: %v", err)
	}
	return l
}

func appendNATSAssetDeletionTestFacts(t *testing.T, ctx context.Context, core *ChattoCore, store jetstream.ObjectStore, assetID string) {
	t.Helper()
	if _, err := store.PutBytes(ctx, assetID, []byte(assetID)); err != nil {
		t.Fatalf("put asset object: %v", err)
	}
	appendAssetCreationTestEvent(t, ctx, core, &corev1.AssetRecord{
		Id:      assetID,
		Storage: &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: assetID}},
	})
	appendAssetDeletionTestEvent(t, ctx, core, &corev1.AssetDeletedEvent{AssetId: assetID})
}

func waitForAssetObjectDeleted(t *testing.T, ctx context.Context, store jetstream.ObjectStore, assetID string) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := store.GetBytes(ctx, assetID); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for asset %s deletion: %v", assetID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func appendAssetCreationTestEvent(t *testing.T, ctx context.Context, core *ChattoCore, asset *corev1.AssetRecord) {
	t.Helper()
	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_AssetCreated{AssetCreated: &corev1.AssetCreatedEvent{Asset: asset}},
	})
	if _, err := core.EventPublisher.AppendEventually(ctx, events.AssetAggregate(asset.GetId()).SubjectFor(event), event); err != nil {
		t.Fatalf("append asset creation event: %v", err)
	}
}

func appendAssetDeletionTestEvent(t *testing.T, ctx context.Context, core *ChattoCore, deleted *corev1.AssetDeletedEvent) {
	t.Helper()
	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_AssetDeleted{AssetDeleted: deleted},
	})
	if _, err := core.EventPublisher.AppendEventually(ctx, events.AssetAggregate(deleted.GetAssetId()).SubjectFor(event), event); err != nil {
		t.Fatalf("append asset deletion event: %v", err)
	}
}
