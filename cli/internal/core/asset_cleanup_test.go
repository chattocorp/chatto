package core

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
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
	attachment, err := core.mediaModel.UploadAttachment(ctx, SystemActorID, room.GetId(), "replay.txt", "text/plain", bytes.NewReader([]byte("replay")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	cacheKey := ImageCacheKey(AttachmentSignResource, attachment.GetId(), 32, 32, "cover")
	if err := core.mediaModel.StoreCachedResize(ctx, cacheKey, []byte("cached")); err != nil {
		t.Fatalf("StoreCachedResize: %v", err)
	}
	if err := core.assetModel.RecordAssetDeleted(ctx, SystemActorID, room.GetId(), attachment.GetId()); err != nil {
		t.Fatalf("RecordAssetDeleted: %v", err)
	}

	restarted := NewAssetModel(core)
	if err := restarted.consumeAssetCleanup(ctx); err != nil {
		t.Fatalf("consumeAssetCleanup after restart: %v", err)
	}
	if _, _, err := core.mediaModel.GetAttachmentReader(ctx, attachment); err == nil {
		t.Fatal("attachment remained readable after replayed cleanup")
	}
	if got, err := core.mediaModel.GetCachedResize(ctx, cacheKey); err != nil || got != nil {
		t.Fatalf("cached resize after replayed cleanup = %q, %v; want nil, nil", got, err)
	}

	secondRestart := NewAssetModel(core)
	if err := secondRestart.consumeAssetCleanup(ctx); err != nil {
		t.Fatalf("idempotent cleanup after second restart: %v", err)
	}
}

func TestAssetCleanupReconcilesHLSChildrenMissedByOlderReplica(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "asset-cleanup-hls-version-skew", "HLS version skew")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	original, err := core.mediaModel.UploadAttachment(ctx, SystemActorID, room.GetId(), "clip.mp4", "video/mp4", bytes.NewReader([]byte("original")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	uploadDerivative := func(filename, contentType string, role corev1.AssetDerivativeRole) *corev1.Attachment {
		t.Helper()
		attachment, err := core.mediaModel.UploadDerivativeAttachment(
			ctx,
			original.GetId(),
			role,
			room.GetId(),
			filename,
			contentType,
			bytes.NewReader([]byte(filename)),
		)
		if err != nil {
			t.Fatalf("UploadDerivativeAttachment(%s): %v", filename, err)
		}
		return attachment
	}
	segment := uploadDerivative("480p-00000.ts", "video/mp2t", corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_HLS_MEDIA_SEGMENT)
	hls := &corev1.AssetProcessedHLS{
		Renditions: []*corev1.AssetHLSRendition{{
			Segments: []*corev1.AssetHLSSegment{{AssetId: segment.GetId(), DurationMs: 2000}},
		}},
	}
	if err := core.assetModel.RecordAssetProcessedWithHLS(
		ctx,
		SystemActorID,
		room.GetId(),
		"E-message",
		original.GetId(),
		2_000,
		320,
		240,
		nil,
		nil,
		hls,
	); err != nil {
		t.Fatalf("RecordAssetProcessedWithHLS: %v", err)
	}

	// Simulate an HLS-unaware replica: it tombstones only the source because
	// the additive HLS manifest field is unknown to its cleanup code.
	if err := core.assetModel.RecordAssetDeleted(ctx, SystemActorID, room.GetId(), original.GetId()); err != nil {
		t.Fatalf("RecordAssetDeleted source: %v", err)
	}
	for _, derivative := range []*corev1.Attachment{segment} {
		if _, ok := core.Assets.AssetCreation(derivative.GetId()); !ok {
			t.Fatalf("HLS derivative %s was unexpectedly tombstoned before reconciliation", derivative.GetId())
		}
	}

	// A new-version durable cleanup consumer replays the source tombstone,
	// recovers HLS child IDs from the source aggregate, and deletes the children.
	if err := NewAssetModel(core).consumeAssetCleanup(ctx); err != nil {
		t.Fatalf("consumeAssetCleanup reconciliation: %v", err)
	}
	if err := NewAssetModel(core).consumeAssetCleanup(ctx); err != nil {
		t.Fatalf("consumeAssetCleanup child deletion: %v", err)
	}
	for _, derivative := range []*corev1.Attachment{segment} {
		if _, ok := core.Assets.AssetCreation(derivative.GetId()); ok {
			t.Fatalf("HLS derivative %s remained projected after reconciliation", derivative.GetId())
		}
		if _, _, err := core.mediaModel.GetAttachmentReader(ctx, derivative); err == nil {
			t.Fatalf("HLS derivative %s remained readable after reconciliation", derivative.GetId())
		}
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
	attachment, err := core.mediaModel.UploadAttachment(ctx, SystemActorID, room.GetId(), "later.txt", "text/plain", bytes.NewReader([]byte("later")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if err := core.assetModel.RecordAssetDeleted(ctx, SystemActorID, room.GetId(), attachment.GetId()); err != nil {
		t.Fatalf("RecordAssetDeleted: %v", err)
	}

	restarted := NewAssetModel(core)
	if err := restarted.consumeAssetCleanup(ctx); err == nil {
		t.Fatal("consumeAssetCleanup returned nil despite unavailable S3 deletion")
	}
	if _, _, err := core.mediaModel.GetAttachmentReader(ctx, attachment); err == nil {
		t.Fatal("later attachment remained readable after an earlier permanent failure")
	}
}

func TestAssetCleanupDoesNotDeleteUnrelatedAssetOrCache(t *testing.T) {
	core, _ := setupTestCoreWithCache(t)
	ctx := testContext(t)
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "asset-cleanup-isolation", "Asset cleanup isolation")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	deletedAsset, err := core.mediaModel.UploadAttachment(ctx, SystemActorID, room.GetId(), "deleted.txt", "text/plain", bytes.NewReader([]byte("deleted")))
	if err != nil {
		t.Fatalf("UploadAttachment deleted asset: %v", err)
	}
	survivingAsset, err := core.mediaModel.UploadAttachment(ctx, SystemActorID, room.GetId(), "surviving.txt", "text/plain", bytes.NewReader([]byte("surviving")))
	if err != nil {
		t.Fatalf("UploadAttachment surviving asset: %v", err)
	}
	deletedCacheKey := ImageCacheKey(AttachmentSignResource, deletedAsset.GetId(), 32, 32, "cover")
	survivingCacheKey := ImageCacheKey(AttachmentSignResource, survivingAsset.GetId(), 32, 32, "cover")
	if err := core.mediaModel.StoreCachedResize(ctx, deletedCacheKey, []byte("deleted-cache")); err != nil {
		t.Fatalf("StoreCachedResize deleted asset: %v", err)
	}
	if err := core.mediaModel.StoreCachedResize(ctx, survivingCacheKey, []byte("surviving-cache")); err != nil {
		t.Fatalf("StoreCachedResize surviving asset: %v", err)
	}
	if err := core.assetModel.RecordAssetDeleted(ctx, SystemActorID, room.GetId(), deletedAsset.GetId()); err != nil {
		t.Fatalf("RecordAssetDeleted: %v", err)
	}

	if err := NewAssetModel(core).consumeAssetCleanup(ctx); err != nil {
		t.Fatalf("consumeAssetCleanup: %v", err)
	}
	if _, _, err := core.mediaModel.GetAttachmentReader(ctx, deletedAsset); err == nil {
		t.Fatal("deleted asset remained readable")
	}
	if got, err := core.mediaModel.GetCachedResize(ctx, deletedCacheKey); err != nil || got != nil {
		t.Fatalf("deleted asset cache = %q, %v; want nil, nil", got, err)
	}
	if _, _, err := core.mediaModel.GetAttachmentReader(ctx, survivingAsset); err != nil {
		t.Fatalf("unrelated asset was not readable: %v", err)
	}
	if got, err := core.mediaModel.GetCachedResize(ctx, survivingCacheKey); err != nil || string(got) != "surviving-cache" {
		t.Fatalf("unrelated asset cache = %q, %v; want surviving-cache, nil", got, err)
	}
}

func TestAssetCleanupRejectsMismatchedCreationPayload(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	store, err := core.GetAttachmentsStore(ctx)
	if err != nil {
		t.Fatalf("GetAttachmentsStore: %v", err)
	}
	if _, err := store.PutBytes(ctx, "A-victim", []byte("victim")); err != nil {
		t.Fatalf("put victim object: %v", err)
	}
	appendAssetCreationTestEventOnAggregate(t, ctx, core, "A-deleted", &corev1.AssetRecord{
		Id:      "A-victim",
		Storage: &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: "A-victim"}},
	})
	appendAssetDeletionTestEvent(t, ctx, core, &corev1.AssetDeletedEvent{AssetId: "A-deleted"})

	if err := NewAssetModel(core).consumeAssetCleanup(ctx); err == nil {
		t.Fatal("consumeAssetCleanup returned nil for mismatched creation payload")
	}
	if got, err := store.GetBytes(ctx, "A-victim"); err != nil || string(got) != "victim" {
		t.Fatalf("victim object = %q, %v; want victim, nil", got, err)
	}
}

func TestAssetCleanupRejectsMismatchedDeletionSubject(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	store, err := core.GetAttachmentsStore(ctx)
	if err != nil {
		t.Fatalf("GetAttachmentsStore: %v", err)
	}
	if _, err := store.PutBytes(ctx, "A-victim", []byte("victim")); err != nil {
		t.Fatalf("put victim object: %v", err)
	}
	appendAssetCreationTestEvent(t, ctx, core, &corev1.AssetRecord{
		Id:      "A-victim",
		Storage: &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: "A-victim"}},
	})
	appendAssetDeletionTestEventOnAggregate(t, ctx, core, "A-other", &corev1.AssetDeletedEvent{AssetId: "A-victim"})

	if err := NewAssetModel(core).consumeAssetCleanup(ctx); err == nil {
		t.Fatal("consumeAssetCleanup returned nil for mismatched deletion subject")
	}
	if got, err := store.GetBytes(ctx, "A-victim"); err != nil || string(got) != "victim" {
		t.Fatalf("victim object = %q, %v; want victim, nil", got, err)
	}
}

func TestAssetCleanupRejectsNATSPointerToAnotherAsset(t *testing.T) {
	core, _ := setupTestCoreWithCache(t)
	ctx := testContext(t)
	store, err := core.GetAttachmentsStore(ctx)
	if err != nil {
		t.Fatalf("GetAttachmentsStore: %v", err)
	}
	if _, err := store.PutBytes(ctx, "A-victim", []byte("victim")); err != nil {
		t.Fatalf("put victim object: %v", err)
	}
	victimCacheKey := ImageCacheKey(AttachmentSignResource, "A-victim", 32, 32, "cover")
	if err := core.mediaModel.StoreCachedResize(ctx, victimCacheKey, []byte("victim-cache")); err != nil {
		t.Fatalf("StoreCachedResize victim: %v", err)
	}
	appendAssetCreationTestEvent(t, ctx, core, &corev1.AssetRecord{
		Id:      "A-attacker",
		Storage: &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: "A-victim"}},
	})
	appendAssetDeletionTestEvent(t, ctx, core, &corev1.AssetDeletedEvent{AssetId: "A-attacker"})

	if err := NewAssetModel(core).consumeAssetCleanup(ctx); err == nil {
		t.Fatal("consumeAssetCleanup returned nil for cross-asset NATS pointer")
	}
	if got, err := store.GetBytes(ctx, "A-victim"); err != nil || string(got) != "victim" {
		t.Fatalf("victim object = %q, %v; want victim, nil", got, err)
	}
	if got, err := core.mediaModel.GetCachedResize(ctx, victimCacheKey); err != nil || string(got) != "victim-cache" {
		t.Fatalf("victim cache = %q, %v; want victim-cache, nil", got, err)
	}
}

func TestAssetCleanupRejectsS3PointerToAnotherAsset(t *testing.T) {
	core, _, s3Client := setupTestCoreWithS3(t)
	ctx := testContext(t)
	victimKey := S3KeyAttachment("A-victim")
	if _, err := s3Client.PutObjectFromBytes(ctx, victimKey, []byte("victim"), "text/plain"); err != nil {
		t.Fatalf("put victim S3 object: %v", err)
	}
	appendAssetCreationTestEvent(t, ctx, core, &corev1.AssetRecord{
		Id: "A-attacker",
		Storage: &corev1.AssetRecord_S3{S3: &corev1.S3Asset{
			Key:    victimKey,
			Bucket: proto.String(s3Client.Bucket()),
		}},
	})
	appendAssetDeletionTestEvent(t, ctx, core, &corev1.AssetDeletedEvent{AssetId: "A-attacker"})

	if err := NewAssetModel(core).consumeAssetCleanup(ctx); err == nil {
		t.Fatal("consumeAssetCleanup returned nil for cross-asset S3 pointer")
	}
	if _, err := s3Client.StatObject(ctx, victimKey); err != nil {
		t.Fatalf("victim S3 object was removed: %v", err)
	}
}

func TestAssetCleanupDeletesS3ObjectFromDurableFacts(t *testing.T) {
	core, _, s3Client := setupTestCoreWithS3(t)
	ctx := testContext(t)
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "asset-cleanup-s3", "Asset cleanup S3")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	attachment, err := core.mediaModel.UploadAttachment(ctx, SystemActorID, room.GetId(), "s3.txt", "text/plain", bytes.NewReader([]byte("s3")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	s3Key := attachment.GetStorage().GetS3().GetKey()
	if _, err := s3Client.StatObject(ctx, s3Key); err != nil {
		t.Fatalf("StatObject before cleanup: %v", err)
	}
	if err := core.assetModel.RecordAssetDeleted(ctx, SystemActorID, room.GetId(), attachment.GetId()); err != nil {
		t.Fatalf("RecordAssetDeleted: %v", err)
	}

	if err := NewAssetModel(core).consumeAssetCleanup(ctx); err != nil {
		t.Fatalf("consumeAssetCleanup: %v", err)
	}
	if _, err := s3Client.StatObject(ctx, s3Key); !IsNoSuchKeyError(err) {
		t.Fatalf("StatObject after cleanup = %v, want no-such-key", err)
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
	if _, err := store.PutBytes(ctx, "A-created-only", []byte("survivor")); err != nil {
		t.Fatalf("put created-only object: %v", err)
	}
	appendAssetCreationTestEvent(t, ctx, second, &corev1.AssetRecord{
		Id:      "A-created-only",
		Storage: &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: "A-created-only"}},
	})
	appendNATSAssetDeletionTestFacts(t, ctx, second, store, "A-non-holder")
	waitForAssetObjectDeleted(t, ctx, store, "A-non-holder")

	cancelFirst()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first cleanup runner shutdown = %v, want context canceled", err)
	}
	appendNATSAssetDeletionTestFacts(t, ctx, first, store, "A-handover")
	waitForAssetObjectDeleted(t, ctx, store, "A-handover")
	if got, err := store.GetBytes(ctx, "A-created-only"); err != nil || string(got) != "survivor" {
		t.Fatalf("created-only object after restart and handover = %q, %v; want survivor, nil", got, err)
	}

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
	appendAssetCreationTestEventOnAggregate(t, ctx, core, asset.GetId(), asset)
}

func appendAssetCreationTestEventOnAggregate(t *testing.T, ctx context.Context, core *ChattoCore, aggregateID string, asset *corev1.AssetRecord) {
	t.Helper()
	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_AssetCreated{AssetCreated: &corev1.AssetCreatedEvent{Asset: asset}},
	})
	if _, err := core.EventPublisher.AppendEventually(ctx, events.AssetAggregate(aggregateID).SubjectFor(event), event); err != nil {
		t.Fatalf("append asset creation event: %v", err)
	}
}

func appendAssetDeletionTestEvent(t *testing.T, ctx context.Context, core *ChattoCore, deleted *corev1.AssetDeletedEvent) {
	t.Helper()
	appendAssetDeletionTestEventOnAggregate(t, ctx, core, deleted.GetAssetId(), deleted)
}

func appendAssetDeletionTestEventOnAggregate(t *testing.T, ctx context.Context, core *ChattoCore, aggregateID string, deleted *corev1.AssetDeletedEvent) {
	t.Helper()
	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_AssetDeleted{AssetDeleted: deleted},
	})
	if _, err := core.EventPublisher.AppendEventually(ctx, events.AssetAggregate(aggregateID).SubjectFor(event), event); err != nil {
		t.Fatalf("append asset deletion event: %v", err)
	}
}
