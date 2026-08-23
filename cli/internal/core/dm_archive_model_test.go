package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestDMArchiveModelEffectiveStateAndAutomaticRestore(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	firstUser := mustCreateDMArchiveUser(t, chatto, ctx, "dm-archive-first")
	secondUser := mustCreateDMArchiveUser(t, chatto, ctx, "dm-archive-second")
	room, _, err := chatto.FindOrCreateDM(ctx, firstUser.Id, []string{secondUser.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	firstMessage, err := chatto.PostMessage(ctx, KindDM, room.Id, firstUser.Id, "first", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage first: %v", err)
	}

	if err := chatto.DMArchive().Archive(ctx, firstUser.Id, room.Id); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	assertDMArchived(t, ctx, chatto, firstUser.Id, room.Id, true)
	entry, err := chatto.storage.runtimeStateKV.Get(ctx, dmArchiveMarkerKey(firstUser.Id, room.Id))
	if err != nil {
		t.Fatalf("Get archive marker: %v", err)
	}
	if got := string(entry.Value()); got != firstMessage.Id {
		t.Fatalf("archive marker = %q, want latest root event %q", got, firstMessage.Id)
	}

	if _, err := chatto.PostMessage(ctx, KindDM, room.Id, secondUser.Id, "new root", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage new root: %v", err)
	}
	assertDMArchived(t, ctx, chatto, firstUser.Id, room.Id, false)

	// Retracting the message that made the marker stale does not make the old
	// marker current again: root-message identity remains part of the timeline.
	latestEventID, _, _, err := chatto.GetRoomLastEvent(ctx, KindDM, room.Id)
	if err != nil {
		t.Fatalf("GetRoomLastEvent: %v", err)
	}
	if err := chatto.DeleteMessage(ctx, secondUser.Id, KindDM, room.Id, latestEventID); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	assertDMArchived(t, ctx, chatto, firstUser.Id, room.Id, false)

	if err := chatto.DMArchive().Archive(ctx, firstUser.Id, room.Id); err != nil {
		t.Fatalf("Archive current root: %v", err)
	}
	assertDMArchived(t, ctx, chatto, firstUser.Id, room.Id, true)
	if err := chatto.DMArchive().Unarchive(ctx, firstUser.Id, room.Id); err != nil {
		t.Fatalf("Unarchive: %v", err)
	}
	if err := chatto.DMArchive().Unarchive(ctx, firstUser.Id, room.Id); err != nil {
		t.Fatalf("idempotent Unarchive: %v", err)
	}
	assertDMArchived(t, ctx, chatto, firstUser.Id, room.Id, false)
}

func TestDMArchiveModelValidatesRoomKindMembershipAndHistory(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	firstUser := mustCreateDMArchiveUser(t, chatto, ctx, "dm-archive-validate-first")
	secondUser := mustCreateDMArchiveUser(t, chatto, ctx, "dm-archive-validate-second")
	emptyDM, _, err := chatto.FindOrCreateDM(ctx, firstUser.Id, []string{secondUser.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if err := chatto.DMArchive().Archive(ctx, firstUser.Id, emptyDM.Id); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("archive empty DM error = %v, want ErrInvalidArgument", err)
	}
	if err := chatto.DMArchive().Archive(ctx, "not-a-member", emptyDM.Id); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("archive as non-member error = %v, want ErrNotRoomMember", err)
	}

	channel, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "dm-archive-channel", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chatto.JoinRoom(ctx, firstUser.Id, KindChannel, firstUser.Id, channel.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if err := chatto.DMArchive().Archive(ctx, firstUser.Id, channel.Id); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("archive channel error = %v, want ErrInvalidArgument", err)
	}
	if err := chatto.DMArchive().Unarchive(ctx, firstUser.Id, channel.Id); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("unarchive channel error = %v, want ErrInvalidArgument", err)
	}
}

func TestDMArchiveModelPublishesUserScopedInvalidation(t *testing.T) {
	chatto, nc := setupTestCore(t)
	ctx := testContext(t)
	firstUser := mustCreateDMArchiveUser(t, chatto, ctx, "dm-archive-live-first")
	secondUser := mustCreateDMArchiveUser(t, chatto, ctx, "dm-archive-live-second")
	room, _, err := chatto.FindOrCreateDM(ctx, firstUser.Id, []string{secondUser.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if _, err := chatto.PostMessage(ctx, KindDM, room.Id, firstUser.Id, "hello", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	sub, err := nc.SubscribeSync(subjects.LiveSyncUserEvent(firstUser.Id, "dm_archive"))
	if err != nil {
		t.Fatalf("SubscribeSync: %v", err)
	}
	defer sub.Unsubscribe()
	if err := nc.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := chatto.DMArchive().Archive(ctx, firstUser.Id, room.Id); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("NextMsg: %v", err)
	}
	var event corev1.LiveEvent
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := event.GetDmArchiveChanged().GetRoomId(); got != room.Id {
		t.Fatalf("DM archive invalidation room = %q, want %q", got, room.Id)
	}
}

func TestDMArchiveIndexInitialSnapshotReplicaUpdatesAndShutdown(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	key := dmArchiveMarkerKey("Uarchive-index", "Rarchive-index")
	seedRevision, err := chatto.storage.runtimeStateKV.Put(ctx, key, []byte("Efirst"))
	if err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	indexes := []*DMArchiveIndex{
		NewDMArchiveIndex(chatto.storage.runtimeStateKV, testCoreLogger()),
		NewDMArchiveIndex(chatto.storage.runtimeStateKV, testCoreLogger()),
	}
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, len(indexes))
	for _, index := range indexes {
		index := index
		go func() { done <- index.Run(runCtx) }()
		if err := index.WaitReady(ctx); err != nil {
			t.Fatalf("WaitReady: %v", err)
		}
		if err := index.waitForRevision(ctx, key, seedRevision); err != nil {
			t.Fatalf("wait for seeded revision: %v", err)
		}
		assertDMArchiveMarker(t, ctx, index, "Uarchive-index", "Rarchive-index", "Efirst")
	}

	updateRevision, err := chatto.storage.runtimeStateKV.Put(ctx, key, []byte("Esecond"))
	if err != nil {
		t.Fatalf("update marker: %v", err)
	}
	for _, index := range indexes {
		if err := index.waitForRevision(ctx, key, updateRevision); err != nil {
			t.Fatalf("wait for replica update: %v", err)
		}
		assertDMArchiveMarker(t, ctx, index, "Uarchive-index", "Rarchive-index", "Esecond")
	}

	if err := chatto.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(updateRevision)); err != nil {
		t.Fatalf("delete marker: %v", err)
	}
	for _, index := range indexes {
		if err := index.waitForRevisionAfter(ctx, key, updateRevision); err != nil {
			t.Fatalf("wait for replica delete: %v", err)
		}
		if _, exists, err := index.marker(ctx, "Uarchive-index", "Rarchive-index"); err != nil || exists {
			t.Fatalf("deleted marker = (exists %v, error %v), want absent", exists, err)
		}
	}

	cancel()
	for range indexes {
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("index shutdown error = %v, want context.Canceled", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("DM archive index did not stop within timeout")
		}
	}
}

func TestDMArchiveMarkersAreRemovedWithAccount(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	firstUser := mustCreateDMArchiveUser(t, chatto, ctx, "dm-archive-delete-first")
	secondUser := mustCreateDMArchiveUser(t, chatto, ctx, "dm-archive-delete-second")
	room, _, err := chatto.FindOrCreateDM(ctx, firstUser.Id, []string{secondUser.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if _, err := chatto.PostMessage(ctx, KindDM, room.Id, firstUser.Id, "hello", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := chatto.DMArchive().Archive(ctx, firstUser.Id, room.Id); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if err := chatto.DeleteUser(ctx, SystemActorID, firstUser.Id); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Get(ctx, dmArchiveMarkerKey(firstUser.Id, room.Id)); !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		t.Fatalf("archive marker after account deletion error = %v, want absent", err)
	}
}

func mustCreateDMArchiveUser(t *testing.T, chatto *ChattoCore, ctx context.Context, login string) *corev1.User {
	t.Helper()
	user, err := chatto.CreateUser(ctx, SystemActorID, login, login, "password123")
	if err != nil {
		t.Fatalf("CreateUser %q: %v", login, err)
	}
	return user
}

func assertDMArchived(t *testing.T, ctx context.Context, chatto *ChattoCore, userID, roomID string, want bool) {
	t.Helper()
	got, err := chatto.DMArchive().IsArchived(ctx, userID, roomID)
	if err != nil {
		t.Fatalf("IsArchived: %v", err)
	}
	if got != want {
		t.Fatalf("IsArchived = %v, want %v", got, want)
	}
}

func assertDMArchiveMarker(t *testing.T, ctx context.Context, index *DMArchiveIndex, userID, roomID, want string) {
	t.Helper()
	entry, exists, err := index.marker(ctx, userID, roomID)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if !exists || string(entry.value) != want {
		t.Fatalf("marker = (%q, %v), want (%q, true)", entry.value, exists, want)
	}
}
