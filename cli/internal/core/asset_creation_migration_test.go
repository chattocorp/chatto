package core

import (
	"bytes"
	"testing"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestAssetCreationMigration_BackfillsMessageAttachments(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_ = core.storage.runtimeStateKV.Delete(ctx, assetCreationESMigrationKey)
	_ = core.storage.serverRuntimeKV.Delete(ctx, assetCreationESMigrationKey)
	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "Files", "Files room")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	user, err := core.CreateUser(ctx, "system", "asset-user", "asset-user", "password123")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	// Simulate the legacy state: write the binary without emitting an
	// AssetCreatedEvent (pre-Option-1 PostMessage emitted it inline).
	attachment, err := core.uploadAttachmentBinary(ctx, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("upload attachment: %v", err)
	}

	legacyPost := newEvent(user.Id, &corev1.Event{
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				RoomId: room.Id,
				Body: &corev1.MessageBody{
					AuthorId:    user.Id,
					Attachments: []*corev1.Attachment{attachment},
				},
			},
		},
	})
	legacyPost.GetMessagePosted().MessageBodyId = legacyPost.Id
	legacyPost.GetMessagePosted().EventId = legacyPost.Id
	attachment.MessageBodyId = legacyPost.Id
	if _, err := core.EventPublisher.AppendEventually(ctx, events.RoomAggregate(room.Id).SubjectFor(legacyPost), legacyPost); err != nil {
		t.Fatalf("append legacy message: %v", err)
	}

	before, err := core.verifyAssetCreationsInEVT(ctx)
	if err != nil {
		t.Fatalf("verify before migration: %v", err)
	}
	if before.MissingCreations != 1 {
		t.Fatalf("missing creations before migration = %d, want 1", before.MissingCreations)
	}

	if err := core.migrateAssetCreationsToES(ctx); err != nil {
		t.Fatalf("migrate asset creations: %v", err)
	}
	waitForAssetCreationSubject(t, core, room.Id)

	declared, ok := core.RoomTimeline.AssetCreation(attachment.Id)
	if !ok {
		t.Fatal("expected projected asset creation")
	}
	if assetCreatedRoomID(declared) != room.Id {
		t.Fatalf("asset creation room = %q, want %q", assetCreatedRoomID(declared), room.Id)
	}
	// Message ownership is derived from the posting message, not stored on
	// the AssetCreatedEvent.
	ownerRoom, ownerMessage, owned := core.RoomTimeline.AssetMessageOwner(attachment.Id)
	if !owned || ownerRoom != room.Id || ownerMessage != legacyPost.Id {
		t.Fatalf("asset message owner = (%q, %q, %v), want (%q, %q, true)", ownerRoom, ownerMessage, owned, room.Id, legacyPost.Id)
	}
	if got := declared.GetAsset().GetId(); got != attachment.Id {
		t.Fatalf("created asset id = %q, want %q", got, attachment.Id)
	}

	after, err := core.verifyAssetCreationsInEVT(ctx)
	if err != nil {
		t.Fatalf("verify after migration: %v", err)
	}
	if after.MissingCreations != 0 || after.DanglingProcessingOutcomes != 0 {
		t.Fatalf("verification after migration = %+v, want no inconsistencies", after)
	}
}

func TestRuntimeMigrationClaim_WaitsForAtomicOwner(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	key := "test.migration.claim"
	_ = core.storage.runtimeStateKV.Delete(ctx, key)
	_ = core.storage.serverRuntimeKV.Delete(ctx, key)

	revision, claimed, err := core.claimRuntimeMigration(ctx, key)
	if err != nil {
		t.Fatalf("claim migration: %v", err)
	}
	if !claimed {
		t.Fatal("first claimant did not acquire migration")
	}

	type result struct {
		claimed bool
		err     error
	}
	done := make(chan result, 1)
	go func() {
		_, claimed, err := core.claimRuntimeMigration(ctx, key)
		done <- result{claimed: claimed, err: err}
	}()

	select {
	case got := <-done:
		t.Fatalf("second claimant returned before completion: %+v", got)
	case <-time.After(50 * time.Millisecond):
	}

	if err := core.completeRuntimeMigration(ctx, key, revision); err != nil {
		t.Fatalf("complete migration: %v", err)
	}

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("second claimant error: %v", got.err)
		}
		if got.claimed {
			t.Fatal("second claimant acquired already-completed migration")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second claimant did not observe migration completion")
	}
}

func TestRuntimeMigrationClaim_AdoptsLegacyDoneSentinel(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	key := "test.migration.legacy.done"
	_ = core.storage.runtimeStateKV.Delete(ctx, key)
	_ = core.storage.serverRuntimeKV.Delete(ctx, key)

	if _, err := core.storage.serverRuntimeKV.Put(ctx, key, []byte(runtimeMigrationDone)); err != nil {
		t.Fatalf("put legacy sentinel: %v", err)
	}

	_, claimed, err := core.claimRuntimeMigration(ctx, key)
	if err != nil {
		t.Fatalf("claim migration: %v", err)
	}
	if claimed {
		t.Fatal("claim acquired migration despite legacy done sentinel")
	}

	entry, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get adopted sentinel: %v", err)
	}
	if got := string(entry.Value()); got != runtimeMigrationDone {
		t.Fatalf("adopted sentinel = %q, want %q", got, runtimeMigrationDone)
	}
}

func waitForAssetCreationSubject(t *testing.T, core *ChattoCore, roomID string) {
	t.Helper()
	ctx := testContext(t)

	subject := events.RoomAggregate(roomID).Subject(events.EventAssetCreated)
	published, seq, err := core.EventPublisher.SubjectEvents(ctx, subject)
	if err != nil {
		t.Fatalf("read %s: %v", subject, err)
	}
	if len(published) == 0 {
		t.Fatalf("expected event on %s", subject)
	}
	if err := core.RoomTimelineProjector.WaitForSeq(ctx, seq); err != nil {
		t.Fatalf("wait for room timeline seq %d: %v", seq, err)
	}
}
