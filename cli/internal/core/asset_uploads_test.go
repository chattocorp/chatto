package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
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
