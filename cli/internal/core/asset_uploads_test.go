package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestAssetUploadCleanupDeletesExpiredUnclaimedPendingAsset(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "expired-pending-asset", "Expired Pending Asset", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "expired-pending-assets", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	content := []byte("pending asset content")
	attachment, err := core.uploadAttachmentBinary(ctx, room.Id, "pending.txt", "text/plain", bytes.NewReader(content))
	if err != nil {
		t.Fatalf("uploadAttachmentBinary: %v", err)
	}
	sum := sha256.Sum256(content)
	if err := core.assetLifecycle().RecordUploadedPendingAttachmentAsset(ctx, user.Id, room.Id, attachment, hex.EncodeToString(sum[:]), time.Now().Add(-time.Minute), false); err != nil {
		t.Fatalf("RecordUploadedPendingAttachmentAsset: %v", err)
	}

	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "", []string{attachment.Id}, "", "", nil, false); err == nil {
		t.Fatal("PostMessage with expired pending asset succeeded")
	}
	if err := core.AssetUploads().CleanupExpired(ctx); err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if _, ok := core.Assets.AssetCreation(attachment.Id); ok {
		t.Fatal("expired pending asset still projected after cleanup")
	}
	if _, _, err := core.GetAttachmentReader(ctx, attachment); err == nil {
		t.Fatal("expired pending attachment binary still readable after cleanup")
	}
}

func TestAssetUploadStaleChunkUpdateDoesNotDeleteCommittedChunk(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "stale-upload-chunk", "Stale Upload Chunk", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "stale-upload-chunks", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	content := []byte("chunk content")
	sum := sha256.Sum256(content)
	upload, err := core.AssetUploads().CreateUpload(ctx, AssetUploadCreateInput{
		ActorID:     user.Id,
		RoomID:      room.Id,
		Filename:    "chunk.txt",
		ContentType: "text/plain",
		Size:        int64(len(content)),
		SHA256:      hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	staleSession, staleRevision, err := core.AssetUploads().loadUpload(ctx, upload.UploadID)
	if err != nil {
		t.Fatalf("loadUpload: %v", err)
	}

	committed, err := core.AssetUploads().UploadChunk(ctx, AssetUploadChunkInput{
		ActorID:     user.Id,
		UploadID:    upload.UploadID,
		Offset:      0,
		Content:     content,
		ChunkSHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatalf("UploadChunk: %v", err)
	}
	if len(committed.ChunkKeys) != 1 {
		t.Fatalf("committed chunk key count = %d, want 1", len(committed.ChunkKeys))
	}

	loserKey := assetUploadTempObjectKey(upload.UploadID, 0)
	if loserKey == committed.ChunkKeys[0] {
		t.Fatal("upload chunk temp keys are deterministic across attempts")
	}
	if _, err := core.storage.serverAssets.Put(ctx, jetstream.ObjectMeta{Name: loserKey}, bytes.NewReader(content)); err != nil {
		t.Fatalf("store loser chunk: %v", err)
	}
	staleSession.ChunkKeys = append(staleSession.ChunkKeys, loserKey)
	staleSession.CommittedOffset = int64(len(content))
	if err := core.AssetUploads().updateUpload(ctx, staleSession, staleRevision); err == nil {
		t.Fatal("stale upload update succeeded")
	}
	if err := core.storage.serverAssets.Delete(ctx, loserKey); err != nil && !errors.Is(err, jetstream.ErrObjectNotFound) {
		t.Fatalf("delete loser chunk: %v", err)
	}

	obj, err := core.storage.serverAssets.Get(ctx, committed.ChunkKeys[0])
	if err != nil {
		t.Fatalf("committed chunk was deleted by stale retry cleanup: %v", err)
	}
	if err := obj.Close(); err != nil {
		t.Fatalf("close committed chunk: %v", err)
	}

	completed, attachment, err := core.AssetUploads().CompleteUpload(ctx, AssetUploadCompleteInput{
		ActorID:  user.Id,
		UploadID: upload.UploadID,
	})
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	if completed.Status != AssetUploadStatusCompleted {
		t.Fatalf("completed status = %q, want %q", completed.Status, AssetUploadStatusCompleted)
	}
	if attachment == nil || attachment.GetId() == "" {
		t.Fatal("CompleteUpload did not return an attachment")
	}
}
