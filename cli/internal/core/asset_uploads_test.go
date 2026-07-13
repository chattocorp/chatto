package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/assets"
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

func TestAssetUploadAnimatedGIFDoesNotRequestVideoProcessingWhenDisabled(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "disabled-gif-upload", "Disabled GIF Upload", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "disabled-gif-uploads", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	content := testAnimatedGIF(t)
	sum := sha256.Sum256(content)
	upload, err := core.AssetUploads().CreateUpload(ctx, AssetUploadCreateInput{
		ActorID:     user.Id,
		RoomID:      room.Id,
		Filename:    "animated.gif",
		ContentType: "image/gif",
		Size:        int64(len(content)),
		SHA256:      hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	if _, err := core.AssetUploads().UploadChunk(ctx, AssetUploadChunkInput{
		ActorID:     user.Id,
		UploadID:    upload.UploadID,
		Offset:      0,
		Content:     content,
		ChunkSHA256: hex.EncodeToString(sum[:]),
	}); err != nil {
		t.Fatalf("UploadChunk: %v", err)
	}
	_, attachment, err := core.AssetUploads().CompleteUpload(ctx, AssetUploadCompleteInput{
		ActorID:  user.Id,
		UploadID: upload.UploadID,
	})
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	declared, ok := core.Assets.AssetCreation(attachment.GetId())
	if !ok {
		t.Fatalf("AssetCreation(%q) missing", attachment.GetId())
	}
	if declared.GetNeedsVideoProcessing() {
		t.Fatal("animated GIF upload persisted needs_video_processing while video is disabled")
	}

	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "gif", []string{attachment.GetId()}, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if manifest, ok := core.Assets.VideoAttachmentManifest(attachment.GetId()); ok && manifest != nil && manifest.Started != nil {
		t.Fatalf("video processing manifest was started while disabled: %+v", manifest)
	}
}

func TestAssetUploadImportRemoteAnimatedGIFUsesPendingAttachmentLifecycle(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	core.OnVideoProcessingRequested = func(_ context.Context, _, _ string) error { return nil }

	user, err := core.CreateUser(ctx, SystemActorID, "remote-gif-import", "Remote GIF Import", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "remote-gif-imports", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	content := testAnimatedGIF(t)
	existing, reservation, err := core.AssetUploads().BeginRemoteAttachmentImport(ctx, user.Id, room.Id, "https://example.com/linked-image.gif")
	if err != nil {
		t.Fatalf("BeginRemoteAttachmentImport: %v", err)
	}
	if existing != nil || reservation == nil {
		t.Fatalf("initial import reservation = (%+v, %+v)", existing, reservation)
	}
	if _, _, err := core.AssetUploads().BeginRemoteAttachmentImport(ctx, user.Id, room.Id, "https://example.com/linked-image.gif"); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("concurrent import reservation error = %v, want ErrLimitExceeded", err)
	}
	attachment, err := core.AssetUploads().ImportRemoteAttachment(ctx, RemoteAttachmentImportInput{
		ActorID:     user.Id,
		RoomID:      room.Id,
		SourceURL:   "https://example.com/linked-image.gif",
		Filename:    "linked-image.gif",
		ContentType: "image/gif",
		Content:     content,
		Reservation: reservation,
	})
	if err != nil {
		t.Fatalf("ImportRemoteAttachment: %v", err)
	}
	if attachment.GetContentType() != "image/gif" || attachment.GetSize() != int64(len(content)) {
		t.Fatalf("imported attachment = %+v", attachment)
	}
	declared, ok := core.Assets.AssetCreation(attachment.GetId())
	if !ok {
		t.Fatalf("AssetCreation(%q) missing", attachment.GetId())
	}
	if !declared.GetNeedsVideoProcessing() {
		t.Fatal("animated linked GIF did not enter the normal video-processing path")
	}
	if expiresAt := declared.GetPendingExpiresAt(); expiresAt == nil || !expiresAt.AsTime().After(time.Now()) {
		t.Fatalf("pending expiry = %v", expiresAt)
	}

	reader, _, err := core.GetAttachmentReader(ctx, attachment)
	if err != nil {
		t.Fatalf("GetAttachmentReader: %v", err)
	}
	stored, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read imported attachment: %v", err)
	}
	if !bytes.Equal(stored, content) {
		t.Fatal("imported GIF bytes changed before normal processing")
	}

	reused, err := core.AssetUploads().ImportRemoteAttachment(ctx, RemoteAttachmentImportInput{
		ActorID:     user.Id,
		RoomID:      room.Id,
		SourceURL:   "https://example.com/linked-image.gif",
		Filename:    "linked-image.gif",
		ContentType: "image/gif",
		Content:     content,
	})
	if err != nil {
		t.Fatalf("ImportRemoteAttachment repeated import: %v", err)
	}
	if reused.GetId() != attachment.GetId() {
		t.Fatalf("repeated import asset ID = %q, want %q", reused.GetId(), attachment.GetId())
	}
	found, reservation, err := core.AssetUploads().BeginRemoteAttachmentImport(ctx, user.Id, room.Id, "https://example.com/linked-image.gif")
	if err != nil {
		t.Fatalf("BeginRemoteAttachmentImport: %v", err)
	}
	if reservation != nil {
		t.Fatal("BeginRemoteAttachmentImport returned a new reservation for an idempotent import")
	}
	if found.GetId() != attachment.GetId() {
		t.Fatalf("pending import asset ID = %q, want %q", found.GetId(), attachment.GetId())
	}
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "linked GIF", []string{attachment.GetId()}, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage linked image attachment: %v", err)
	}
	found, reservation, err = core.AssetUploads().BeginRemoteAttachmentImport(ctx, user.Id, room.Id, "https://example.com/linked-image.gif")
	if err != nil {
		t.Fatalf("BeginRemoteAttachmentImport after claim: %v", err)
	}
	if found != nil || reservation == nil {
		t.Fatalf("post-claim import lookup = (%+v, %+v), want a fresh reservation", found, reservation)
	}
	core.AssetUploads().CancelRemoteAttachmentImport(ctx, reservation)
}

func TestAssetUploadImportRemoteRejectsExcessiveAnimatedGIF(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "excessive-remote-gif", "Excessive Remote GIF", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "excessive-remote-gifs", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	content := testAnimatedGIFWithFrames(t, assets.MaxAnimatedImageFrames+1)
	if _, err := core.AssetUploads().ImportRemoteAttachment(ctx, RemoteAttachmentImportInput{
		ActorID:     user.Id,
		RoomID:      room.Id,
		SourceURL:   "https://example.com/excessive.gif",
		Filename:    "linked-image.gif",
		ContentType: "image/gif",
		Content:     content,
	}); err == nil {
		t.Fatal("ImportRemoteAttachment accepted an excessive animated GIF")
	}
}

func TestAssetUploadImportRemoteLimitsOutstandingLinkedImagesPerActor(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "linked-image-import-limit", "Linked Image Import Limit", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "linked-image-import-limits", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	baseContent := testAnimatedGIF(t)
	for i := 0; i < maxPendingLinkedImageImports; i++ {
		content := append(append([]byte(nil), baseContent...), byte(i))
		if _, err := core.AssetUploads().ImportRemoteAttachment(ctx, RemoteAttachmentImportInput{
			ActorID:     user.Id,
			RoomID:      room.Id,
			SourceURL:   fmt.Sprintf("https://example.com/image-%d.gif", i),
			Filename:    "linked-image.gif",
			ContentType: "image/gif",
			Content:     content,
		}); err != nil {
			t.Fatalf("ImportRemoteAttachment %d: %v", i, err)
		}
	}
	content := append(append([]byte(nil), baseContent...), byte(maxPendingLinkedImageImports))
	if _, err := core.AssetUploads().ImportRemoteAttachment(ctx, RemoteAttachmentImportInput{
		ActorID:     user.Id,
		RoomID:      room.Id,
		SourceURL:   "https://example.com/image-over-limit.gif",
		Filename:    "linked-image.gif",
		ContentType: "image/gif",
		Content:     content,
	}); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("ImportRemoteAttachment over limit error = %v, want ErrLimitExceeded", err)
	}
}

func testAnimatedGIF(t *testing.T) []byte {
	return testAnimatedGIFWithFrames(t, 2)
}

func testAnimatedGIFWithFrames(t *testing.T, frameCount int) []byte {
	t.Helper()
	palette := color.Palette{color.Black, color.White}
	frames := make([]*image.Paletted, frameCount)
	delays := make([]int, frameCount)
	for i := range frames {
		frames[i] = image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
		frames[i].SetColorIndex(i%2, i%2, uint8(i%2))
		delays[i] = 10
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{
		Image: frames,
		Delay: delays,
	}); err != nil {
		t.Fatalf("EncodeAll animated GIF: %v", err)
	}
	return buf.Bytes()
}
