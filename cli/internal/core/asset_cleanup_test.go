package core

import (
	"bytes"
	"context"
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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
