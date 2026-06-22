package core

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/internal/testutil/fakes3"
	"hmans.de/chatto/pkg/signedurl"
)

// createTestPNG creates a simple PNG image for testing
func createTestPNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Fill with a solid color
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, image.White)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// ============================================================================
// Attachment Upload Tests
// ============================================================================

func TestChattoCore_UploadAttachment(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup: create space and room

	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	t.Run("upload image attachment", func(t *testing.T) {
		imageData := createTestPNG(100, 100)

		attachment, err := core.UploadAttachment(
			ctx,
			SystemActorID,
			room.Id,
			"test-image.png",
			"image/png",
			bytes.NewReader(imageData),
		)

		if err != nil {
			t.Fatalf("Failed to upload attachment: %v", err)
		}

		if attachment == nil {
			t.Fatal("Expected attachment, got nil")
		}

		if attachment.Id == "" {
			t.Error("Attachment ID should not be empty")
		}

		if attachment.Filename != "test-image.png" {
			t.Errorf("Expected filename 'test-image.png', got '%s'", attachment.Filename)
		}

		if attachment.ContentType != "image/png" {
			t.Errorf("Expected content type 'image/png', got '%s'", attachment.ContentType)
		}

		if attachment.RoomId != room.Id {
			t.Errorf("Expected room ID '%s', got '%s'", room.Id, attachment.RoomId)
		}

		// Image dimensions should be extracted
		if attachment.Width == 0 || attachment.Height == 0 {
			t.Error("Expected non-zero dimensions for image attachment")
		}

		if attachment.Width != 100 {
			t.Errorf("Expected width 100, got %d", attachment.Width)
		}

		if attachment.Height != 100 {
			t.Errorf("Expected height 100, got %d", attachment.Height)
		}

		if attachment.Size <= 0 {
			t.Error("Expected positive size for attachment")
		}
	})

	t.Run("upload non-image attachment", func(t *testing.T) {
		textData := []byte("Hello, this is a text file for testing!")

		attachment, err := core.UploadAttachment(
			ctx,
			SystemActorID,
			room.Id,
			"test-file.txt",
			"text/plain",
			bytes.NewReader(textData),
		)

		if err != nil {
			t.Fatalf("Failed to upload attachment: %v", err)
		}

		if attachment == nil {
			t.Fatal("Expected attachment, got nil")
		}

		if attachment.Filename != "test-file.txt" {
			t.Errorf("Expected filename 'test-file.txt', got '%s'", attachment.Filename)
		}

		if attachment.ContentType != "text/plain" {
			t.Errorf("Expected content type 'text/plain', got '%s'", attachment.ContentType)
		}

		// Non-image attachments should have zero dimensions
		if attachment.Width != 0 || attachment.Height != 0 {
			t.Errorf("Expected zero dimensions for non-image, got %dx%d", attachment.Width, attachment.Height)
		}

		if attachment.Size != int64(len(textData)) {
			t.Errorf("Expected size %d, got %d", len(textData), attachment.Size)
		}
	})
}

// ============================================================================
// Attachment Retrieval Tests
// ============================================================================

func TestChattoCore_GetAttachment(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")

	originalData := []byte("This is the original attachment content!")

	// Upload an attachment
	attachment, err := core.UploadAttachment(
		ctx,
		SystemActorID,
		room.Id,
		"test-file.txt",
		"text/plain",
		bytes.NewReader(originalData),
	)
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	t.Run("retrieve existing attachment", func(t *testing.T) {
		reader, info, err := core.GetAttachment(ctx, attachment.Id)
		if err != nil {
			t.Fatalf("Failed to get attachment: %v", err)
		}

		if reader == nil {
			t.Fatal("Expected reader, got nil")
		}

		if info == nil {
			t.Fatal("Expected info, got nil")
		}

		// Read and verify content
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read attachment content: %v", err)
		}

		if !bytes.Equal(content, originalData) {
			t.Errorf("Content mismatch: got '%s', want '%s'", string(content), string(originalData))
		}

		// Verify headers
		if info.Name != attachment.Id {
			t.Errorf("Expected name '%s', got '%s'", attachment.Id, info.Name)
		}
	})

	t.Run("retrieve non-existent attachment", func(t *testing.T) {
		_, _, err := core.GetAttachment(ctx, "nonexistent-attachment-id")
		if err == nil {
			t.Fatal("Expected error for non-existent attachment")
		}
	})
}

// ============================================================================
// Attachment Deletion Tests
// ============================================================================

func TestChattoCore_DeleteAttachment(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")

	// Upload an attachment
	attachment, err := core.UploadAttachment(
		ctx,
		SystemActorID,
		room.Id,
		"test-file.txt",
		"text/plain",
		bytes.NewReader([]byte("Content to delete")),
	)
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	t.Run("delete existing attachment", func(t *testing.T) {
		// Verify it exists first
		_, _, err := core.GetAttachment(ctx, attachment.Id)
		if err != nil {
			t.Fatalf("Attachment should exist before deletion: %v", err)
		}

		// Delete it
		if err := core.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
			t.Fatalf("Failed to delete attachment: %v", err)
		}

		// Verify it no longer exists
		_, _, err = core.GetAttachment(ctx, attachment.Id)
		if err == nil {
			t.Fatal("Expected error after deletion")
		}
	})

	t.Run("delete non-existent attachment", func(t *testing.T) {
		ghost := &corev1.Attachment{
			Id:     "nonexistent-attachment-id",
			RoomId: room.Id,
			Storage: &corev1.DeprecatedAsset{
				Asset: &corev1.DeprecatedAsset_Nats{Nats: &corev1.NATSAsset{Key: "nonexistent-attachment-id"}},
			},
		}
		// Deletion of non-existent item may or may not error depending on implementation
		if err := core.DeleteAttachmentFromStorage(ctx, ghost); err != nil {
			t.Logf("Delete non-existent returned error (acceptable): %v", err)
		}
	})
}

// ============================================================================
// S3 Attachment Deletion Tests
// ============================================================================

func TestChattoCore_UploadAttachment_S3(t *testing.T) {
	core, _, s3Client := setupTestCoreWithS3(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "test.txt", "text/plain", bytes.NewReader([]byte("hello S3")))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	// Verify it's stored with S3 storage metadata
	if attachment.Storage == nil {
		t.Fatal("Attachment should have storage metadata")
	}
	s3Storage := attachment.Storage.GetS3()
	if s3Storage == nil {
		t.Fatal("Attachment storage should be S3")
	}

	// Verify object exists in S3
	_, err = s3Client.StatObject(ctx, s3Storage.Key)
	if err != nil {
		t.Fatalf("Object should exist in S3: %v", err)
	}
}

func TestChattoCore_UploadAttachment_S3PathPrefixKeepsStoredKeyLogical(t *testing.T) {
	core, _, s3Client, rawS3Client, _ := setupTestCoreWithS3PathPrefix(t, "tenant-a/chatto")
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "test.txt", "text/plain", bytes.NewReader([]byte("hello prefixed S3")))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	s3Storage := attachment.Storage.GetS3()
	if s3Storage == nil {
		t.Fatal("Attachment storage should be S3")
	}
	wantLogicalKey := S3KeyAttachment(attachment.Id)
	if s3Storage.Key != wantLogicalKey {
		t.Fatalf("persisted S3 key = %q, want logical key %q", s3Storage.Key, wantLogicalKey)
	}

	info, err := s3Client.StatObject(ctx, s3Storage.Key)
	if err != nil {
		t.Fatalf("logical S3 stat should find object through configured prefix: %v", err)
	}
	if info.Key != wantLogicalKey {
		t.Fatalf("logical stat key = %q, want %q", info.Key, wantLogicalKey)
	}

	if _, err := rawS3Client.StatObject(ctx, s3Storage.Key); err == nil {
		t.Fatal("raw bucket-root stat should not find object at the logical key")
	}
	if _, err := rawS3Client.StatObject(ctx, "tenant-a/chatto/"+s3Storage.Key); err != nil {
		t.Fatalf("raw stat should find physical prefixed object: %v", err)
	}
}

func TestChattoCore_DeleteAttachmentFromStorage_S3(t *testing.T) {
	core, _, s3Client := setupTestCoreWithS3(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "test.txt", "text/plain", bytes.NewReader([]byte("delete me from S3")))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	s3Key := attachment.Storage.GetS3().Key

	// Verify it exists in S3
	_, err = s3Client.StatObject(ctx, s3Key)
	if err != nil {
		t.Fatalf("Object should exist in S3 before deletion: %v", err)
	}

	// Delete via storage-aware method
	err = core.DeleteAttachmentFromStorage(ctx, attachment)
	if err != nil {
		t.Fatalf("Failed to delete attachment from storage: %v", err)
	}

	// Verify it's gone from S3
	_, err = s3Client.StatObject(ctx, s3Key)
	if err == nil {
		t.Error("Object should be deleted from S3")
	}
}

func TestChattoCore_S3PathPrefixCanMoveBasePathWithoutChangingStoredKey(t *testing.T) {
	core, _, oldPrefixClient, rawS3Client, s3Cfg := setupTestCoreWithS3PathPrefix(t, "tenant-a/chatto")
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	content := []byte("move me between prefixes")
	attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "move.txt", "text/plain", bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}
	storedKey := attachment.Storage.GetS3().Key

	if _, err := oldPrefixClient.StatObject(ctx, storedKey); err != nil {
		t.Fatalf("old prefix client should find uploaded object: %v", err)
	}

	// Simulate an operator moving objects in S3 before changing config.
	if _, err := rawS3Client.PutObjectFromBytes(ctx, "tenant-b/chatto/"+storedKey, content, "text/plain"); err != nil {
		t.Fatalf("failed to copy object to new prefix: %v", err)
	}
	if err := rawS3Client.DeleteObject(ctx, "tenant-a/chatto/"+storedKey); err != nil {
		t.Fatalf("failed to remove object from old prefix: %v", err)
	}

	s3Cfg.PathPrefix = "tenant-b/chatto"
	newPrefixClient, err := NewS3Client(s3Cfg)
	if err != nil {
		t.Fatalf("failed to create new-prefix S3 client: %v", err)
	}
	core.s3Client = newPrefixClient

	reader, info, err := core.GetAttachmentReader(ctx, attachment)
	if err != nil {
		t.Fatalf("GetAttachmentReader after prefix change: %v", err)
	}
	defer reader.(io.Closer).Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read moved object: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("moved object content = %q, want %q", got, content)
	}
	if info.ContentType != "text/plain" {
		t.Fatalf("content type = %q, want text/plain", info.ContentType)
	}

	presignedURL, err := core.TryPresignedAttachmentURL(ctx, attachment)
	if err != nil {
		t.Fatalf("TryPresignedAttachmentURL after prefix change: %v", err)
	}
	if !bytes.Contains([]byte(presignedURL), []byte("tenant-b/chatto/"+storedKey)) {
		t.Fatalf("presigned URL %q does not contain new physical prefix", presignedURL)
	}
}

func TestChattoCore_S3PathPrefixAppliesToAllAssetUploadsWithoutPersistingPrefix(t *testing.T) {
	core, _, s3Client, rawS3Client, _ := setupTestCoreWithS3PathPrefix(t, "tenant-a/chatto")
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "prefixed-user", "Prefixed User", "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "prefixed-room", "Prefixed Room")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	avatar, err := core.UploadUserAvatar(ctx, user.Id, bytes.NewReader(createTestPNG(64, 64)))
	if err != nil {
		t.Fatalf("UploadUserAvatar failed: %v", err)
	}
	logo, err := core.UploadServerLogo(ctx, bytes.NewReader(createTestPNG(100, 100)))
	if err != nil {
		t.Fatalf("UploadServerLogo failed: %v", err)
	}
	original, err := core.UploadAttachment(ctx, user.Id, room.Id, "original.png", "image/png", bytes.NewReader(createTestPNG(80, 80)))
	if err != nil {
		t.Fatalf("UploadAttachment failed: %v", err)
	}
	derivative, err := core.UploadDerivativeAttachment(ctx, original.Id, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, room.Id, "thumb.png", "image/png", bytes.NewReader(createTestPNG(32, 32)))
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment failed: %v", err)
	}

	checkServerAsset := func(name string, asset *corev1.AssetRecord) {
		t.Helper()
		key := asset.GetS3().GetKey()
		if bytes.Contains([]byte(key), []byte("tenant-a/chatto")) {
			t.Fatalf("%s persisted key contains prefix: %q", name, key)
		}
		logicalS3Key := S3KeyServerAsset(key)
		if _, err := s3Client.StatObject(ctx, logicalS3Key); err != nil {
			t.Fatalf("%s logical S3 stat failed: %v", name, err)
		}
		if _, err := rawS3Client.StatObject(ctx, "tenant-a/chatto/"+logicalS3Key); err != nil {
			t.Fatalf("%s raw physical S3 stat failed: %v", name, err)
		}
	}
	checkAttachment := func(name string, attachment *corev1.Attachment) {
		t.Helper()
		key := attachment.GetStorage().GetS3().GetKey()
		if key != S3KeyAttachment(attachment.Id) {
			t.Fatalf("%s persisted key = %q, want logical attachment key", name, key)
		}
		if _, err := s3Client.StatObject(ctx, key); err != nil {
			t.Fatalf("%s logical S3 stat failed: %v", name, err)
		}
		if _, err := rawS3Client.StatObject(ctx, "tenant-a/chatto/"+key); err != nil {
			t.Fatalf("%s raw physical S3 stat failed: %v", name, err)
		}
	}

	checkServerAsset("avatar", avatar)
	checkServerAsset("server logo", logo)
	checkAttachment("original attachment", original)
	checkAttachment("derivative attachment", derivative)
}

// ============================================================================
// Attachment URL Generation Tests
// ============================================================================

func bodyLocator(t *testing.T) signedurl.AttachmentLocator {
	t.Helper()
	return signedurl.AttachmentLocator{
		RoomID:       "Rabc",
		BodyKey:      "Uauthor.E1234",
		AttachmentID: "attachment456",
	}
}

// testUserID is the caller's user ID baked into URL signatures by
// GetAttachmentURL / GetTransformedAttachmentURL.
const testUserID = "Uviewer"

// TestGetAttachmentReader_ProbesWhenStorageMissing covers the
// fallback path GetAttachmentReader takes when handed an Attachment
// whose `Storage` field is nil, as older video derivative records can be.
func TestGetAttachmentReader_ProbesWhenStorageMissing(t *testing.T) {
	t.Run("falls back to NATS by attachment ID", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "r", "r")
		attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "x.txt", "text/plain", bytes.NewReader([]byte("nats-binary")))
		if err != nil {
			t.Fatalf("UploadAttachment: %v", err)
		}

		// Older derivative metadata may contain Id + RoomId but no Storage.
		// Verify GetAttachmentReader can still find the binary by probing.
		minimal := &corev1.Attachment{Id: attachment.Id, RoomId: room.Id}
		reader, info, err := core.GetAttachmentReader(ctx, minimal)
		if err != nil {
			t.Fatalf("GetAttachmentReader with Storage-less attachment: %v", err)
		}
		data, _ := io.ReadAll(reader)
		if string(data) != "nats-binary" {
			t.Errorf("expected 'nats-binary', got %q", data)
		}
		if info.ContentType != "text/plain" {
			t.Errorf("expected content type 'text/plain', got %q", info.ContentType)
		}
	})

	t.Run("falls back to S3 across known layouts", func(t *testing.T) {
		core, _, _ := setupTestCoreWithS3(t)
		ctx := testContext(t)

		room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "r", "r")
		attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "y.txt", "text/plain", bytes.NewReader([]byte("s3-binary")))
		if err != nil {
			t.Fatalf("UploadAttachment: %v", err)
		}

		// Same shape as above but the binary now lives in S3.
		minimal := &corev1.Attachment{Id: attachment.Id, RoomId: room.Id}
		reader, _, err := core.GetAttachmentReader(ctx, minimal)
		if err != nil {
			t.Fatalf("GetAttachmentReader with Storage-less S3 attachment: %v", err)
		}
		if closer, ok := reader.(io.Closer); ok {
			defer closer.Close()
		}
		data, _ := io.ReadAll(reader)
		if string(data) != "s3-binary" {
			t.Errorf("expected 's3-binary', got %q", data)
		}
	})

	t.Run("falls back to prefixed S3 legacy layouts", func(t *testing.T) {
		core, _, _, rawS3Client, _ := setupTestCoreWithS3PathPrefix(t, "tenant-a/chatto")
		ctx := testContext(t)

		attachmentID := "att_legacy_prefixed"
		legacyLogicalKey := "spaces/server/attachments/" + attachmentID
		legacyPhysicalKey := "tenant-a/chatto/" + legacyLogicalKey
		if _, err := rawS3Client.PutObjectFromBytes(ctx, legacyPhysicalKey, []byte("prefixed-legacy-s3"), "text/plain"); err != nil {
			t.Fatalf("failed to seed legacy S3 object: %v", err)
		}

		minimal := &corev1.Attachment{Id: attachmentID, RoomId: "room-legacy"}
		reader, info, err := core.GetAttachmentReader(ctx, minimal)
		if err != nil {
			t.Fatalf("GetAttachmentReader with prefixed legacy S3 attachment: %v", err)
		}
		if closer, ok := reader.(io.Closer); ok {
			defer closer.Close()
		}
		data, _ := io.ReadAll(reader)
		if string(data) != "prefixed-legacy-s3" {
			t.Errorf("expected 'prefixed-legacy-s3', got %q", data)
		}
		if info.ContentType != "text/plain" {
			t.Errorf("expected content type 'text/plain', got %q", info.ContentType)
		}
	})

	t.Run("returns error when binary is nowhere", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		minimal := &corev1.Attachment{Id: "Aghost", RoomId: "Rghost"}
		if _, _, err := core.GetAttachmentReader(ctx, minimal); err == nil {
			t.Error("expected error for missing binary, got nil")
		}
	})
}

func TestChattoCore_GetAttachmentURL(t *testing.T) {
	core, _ := setupTestCore(t)

	t.Run("generate basic attachment URL", func(t *testing.T) {
		url := core.GetAttachmentURL(bodyLocator(t), testUserID)
		if !bytes.HasPrefix([]byte(url), []byte("/assets/attachments/")) {
			t.Errorf("Expected URL prefix '/assets/attachments/', got '%s'", url)
		}
		// Locator is a base64-encoded JSON blob plus a dot-separated
		// signature — at minimum a few dozen characters.
		if len(url) < len("/assets/attachments/")+20 {
			t.Errorf("URL suspiciously short, missing signed locator: %s", url)
		}
	})

	t.Run("different attachments produce different URLs", func(t *testing.T) {
		a := bodyLocator(t)
		b := a
		b.AttachmentID = "different-attachment"
		if core.GetAttachmentURL(a, testUserID) == core.GetAttachmentURL(b, testUserID) {
			t.Error("Different attachment IDs should produce different URLs")
		}
	})

	t.Run("different users produce different URLs", func(t *testing.T) {
		loc := bodyLocator(t)
		if core.GetAttachmentURL(loc, "UviewerA") == core.GetAttachmentURL(loc, "UviewerB") {
			t.Error("Different users should produce different signed URLs")
		}
	})
}

func TestChattoCore_GetTransformedAttachmentURL(t *testing.T) {
	core, _ := setupTestCore(t)

	t.Run("generate transform URL with dimensions", func(t *testing.T) {
		url := core.GetTransformedAttachmentURL(bodyLocator(t), testUserID, 200, 150, "contain")
		if !bytes.Contains([]byte(url), []byte("/t/")) {
			t.Errorf("URL missing transform path component: %s", url)
		}
	})

	t.Run("different dimensions produce different URLs", func(t *testing.T) {
		loc := bodyLocator(t)
		url1 := core.GetTransformedAttachmentURL(loc, testUserID, 200, 150, "contain")
		url2 := core.GetTransformedAttachmentURL(loc, testUserID, 400, 300, "contain")
		if url1 == url2 {
			t.Error("Different dimensions should produce different URLs")
		}
	})

	t.Run("different fit modes produce different URLs", func(t *testing.T) {
		loc := bodyLocator(t)
		url1 := core.GetTransformedAttachmentURL(loc, testUserID, 200, 150, "contain")
		url2 := core.GetTransformedAttachmentURL(loc, testUserID, 200, 150, "cover")
		if url1 == url2 {
			t.Error("Different fit modes should produce different URLs")
		}
	})
}

// ============================================================================
// Absolute Asset URL Tests
// ============================================================================

func TestChattoCore_AssetBaseURL(t *testing.T) {
	core, _ := setupTestCore(t)

	t.Run("GetAttachmentURL returns relative when AssetBaseURL is empty", func(t *testing.T) {
		core.AssetBaseURL = ""
		url := core.GetAttachmentURL(bodyLocator(t), testUserID)
		if !bytes.HasPrefix([]byte(url), []byte("/assets/attachments/")) {
			t.Errorf("Expected relative URL, got '%s'", url)
		}
	})

	t.Run("GetAttachmentURL returns absolute when AssetBaseURL is set", func(t *testing.T) {
		core.AssetBaseURL = "https://chat.example.com"
		defer func() { core.AssetBaseURL = "" }()

		url := core.GetAttachmentURL(bodyLocator(t), testUserID)

		if !bytes.HasPrefix([]byte(url), []byte("https://chat.example.com/assets/attachments/")) {
			t.Errorf("Expected absolute URL with base, got '%s'", url)
		}
	})

	t.Run("GetTransformedAttachmentURL returns absolute when AssetBaseURL is set", func(t *testing.T) {
		core.AssetBaseURL = "https://chat.example.com"
		defer func() { core.AssetBaseURL = "" }()

		url := core.GetTransformedAttachmentURL(bodyLocator(t), testUserID, 200, 150, "contain")

		if !bytes.HasPrefix([]byte(url), []byte("https://chat.example.com/assets/attachments/")) {
			t.Errorf("Expected absolute URL with base, got '%s'", url)
		}
	})

	t.Run("GetTransformedServerAssetURL returns absolute when AssetBaseURL is set", func(t *testing.T) {
		core.AssetBaseURL = "https://chat.example.com"
		defer func() { core.AssetBaseURL = "" }()

		url := core.GetTransformedServerAssetURL("avatar-key", 100, 100, "cover")

		if !bytes.HasPrefix([]byte(url), []byte("https://chat.example.com/assets/server/")) {
			t.Errorf("Expected absolute URL with base, got '%s'", url)
		}
	})
}

// ============================================================================
// Attachment Store Tests
// ============================================================================

func TestChattoCore_GetAttachmentsStore(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space (no longer needed for store access but kept for test setup parity)

	t.Run("get attachments store creates lazily", func(t *testing.T) {
		store, err := core.GetAttachmentsStore(ctx)
		if err != nil {
			t.Fatalf("Failed to get attachments store: %v", err)
		}

		if store == nil {
			t.Fatal("Expected store, got nil")
		}
	})

	t.Run("get attachments store returns cached instance", func(t *testing.T) {
		store1, err := core.GetAttachmentsStore(ctx)
		if err != nil {
			t.Fatalf("Failed to get store first time: %v", err)
		}

		store2, err := core.GetAttachmentsStore(ctx)
		if err != nil {
			t.Fatalf("Failed to get store second time: %v", err)
		}

		// Should return the same instance (cached)
		if store1 != store2 {
			t.Error("Expected same store instance to be returned (cached)")
		}
	})
}

// ============================================================================
// Attachment Integration Tests
// ============================================================================

func TestAttachment_FullLifecycle(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")

	originalContent := []byte("Integration test attachment content!")

	// 1. Upload
	attachment, err := core.UploadAttachment(
		ctx,
		SystemActorID,
		room.Id,
		"lifecycle-test.txt",
		"text/plain",
		bytes.NewReader(originalContent),
	)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// 2. Verify URL generation
	url := core.GetAttachmentURL(signedurl.AttachmentLocator{
		RoomID:       attachment.RoomId,
		BodyKey:      "U.E", // placeholder; URL signing doesn't validate body existence
		AttachmentID: attachment.Id,
	}, testUserID)
	if url == "" {
		t.Error("URL generation failed")
	}

	// 3. Retrieve and verify content
	reader, _, err := core.GetAttachment(ctx, attachment.Id)
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}

	content, _ := io.ReadAll(reader)
	if !bytes.Equal(content, originalContent) {
		t.Error("Retrieved content doesn't match original")
	}

	// 4. Delete
	if err := core.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
		t.Fatalf("Deletion failed: %v", err)
	}

	// 5. Verify deleted
	_, _, err = core.GetAttachment(ctx, attachment.Id)
	if err == nil {
		t.Error("Attachment should not exist after deletion")
	}
}

func TestAttachment_MultipleInSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")

	// Upload multiple attachments
	attachmentCount := 5
	attachments := make([]string, attachmentCount)

	for i := 0; i < attachmentCount; i++ {
		content := []byte("Attachment content " + string(rune('A'+i)))
		att, err := core.UploadAttachment(
			ctx,
			SystemActorID,
			room.Id,
			"attachment"+string(rune('A'+i))+".txt",
			"text/plain",
			bytes.NewReader(content),
		)
		if err != nil {
			t.Fatalf("Failed to upload attachment %d: %v", i, err)
		}
		attachments[i] = att.Id
	}

	// Verify all attachments exist and have unique IDs
	ids := make(map[string]bool)
	for _, id := range attachments {
		if ids[id] {
			t.Errorf("Duplicate attachment ID: %s", id)
		}
		ids[id] = true

		// Verify each can be retrieved
		_, _, err := core.GetAttachment(ctx, id)
		if err != nil {
			t.Errorf("Failed to retrieve attachment %s: %v", id, err)
		}
	}
}

func TestAttachment_ImageDimensions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")

	testCases := []struct {
		name   string
		width  int
		height int
	}{
		{"small square", 50, 50},
		{"medium rectangle", 200, 100},
		{"tall image", 100, 300},
		{"wide image", 400, 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			imageData := createTestPNG(tc.width, tc.height)

			attachment, err := core.UploadAttachment(
				ctx,
				SystemActorID,
				room.Id,
				tc.name+".png",
				"image/png",
				bytes.NewReader(imageData),
			)
			if err != nil {
				t.Fatalf("Failed to upload: %v", err)
			}

			if int(attachment.Width) != tc.width {
				t.Errorf("Expected width %d, got %d", tc.width, attachment.Width)
			}

			if int(attachment.Height) != tc.height {
				t.Errorf("Expected height %d, got %d", tc.height, attachment.Height)
			}
		})
	}
}

// ============================================================================
// Image Cache Key Tests
// ============================================================================

func TestImageCacheKey(t *testing.T) {
	t.Run("generates consistent keys", func(t *testing.T) {
		key1 := ImageCacheKey("space123", "attach456", 200, 150, "contain")
		key2 := ImageCacheKey("space123", "attach456", 200, 150, "contain")

		if key1 != key2 {
			t.Errorf("Same params should produce same key: %s vs %s", key1, key2)
		}
	})

	t.Run("uses NATS subject notation with dots", func(t *testing.T) {
		key := ImageCacheKey("space123", "attach456", 200, 150, "contain")

		// Should have 3 dot-separated parts
		parts := bytes.Split([]byte(key), []byte("."))
		if len(parts) != 3 {
			t.Errorf("Key should have 3 dot-separated parts, got %d: %s", len(parts), key)
		}

		// First two parts should be space and attachment IDs
		if string(parts[0]) != "space123" {
			t.Errorf("First part should be spaceId: %s", string(parts[0]))
		}
		if string(parts[1]) != "attach456" {
			t.Errorf("Second part should be attachmentId: %s", string(parts[1]))
		}
	})

	t.Run("different dimensions produce different keys", func(t *testing.T) {
		key1 := ImageCacheKey("space", "attach", 200, 150, "contain")
		key2 := ImageCacheKey("space", "attach", 400, 300, "contain")

		if key1 == key2 {
			t.Error("Different dimensions should produce different keys")
		}
	})

	t.Run("different fit modes produce different keys", func(t *testing.T) {
		key1 := ImageCacheKey("space", "attach", 200, 150, "contain")
		key2 := ImageCacheKey("space", "attach", 200, 150, "cover")

		if key1 == key2 {
			t.Error("Different fit modes should produce different keys")
		}
	})

	t.Run("hash is 16 hex characters", func(t *testing.T) {
		key := ImageCacheKey("space", "attach", 200, 150, "contain")
		parts := bytes.Split([]byte(key), []byte("."))

		// Third part is the hash (8 bytes = 16 hex chars)
		hash := string(parts[2])
		if len(hash) != 16 {
			t.Errorf("Hash should be 16 hex chars, got %d: %s", len(hash), hash)
		}
	})
}

// ============================================================================
// Cache Cleanup Tests
// ============================================================================

// setupTestCoreWithS3 creates a ChattoCore backed by a fake in-memory S3 server.
// Returns the core, NATS connection, and S3 client for verification.
func setupTestCoreWithS3(t *testing.T) (*ChattoCore, *nats.Conn, *S3Client) {
	core, nc, s3Client, _, _ := setupTestCoreWithS3PathPrefix(t, "")
	return core, nc, s3Client
}

// setupTestCoreWithS3PathPrefix creates a ChattoCore backed by a fake
// in-memory S3 server. It returns both a prefix-aware verification client and a
// raw bucket-root client for assertions about physical object keys.
func setupTestCoreWithS3PathPrefix(t *testing.T, pathPrefix string) (*ChattoCore, *nats.Conn, *S3Client, *S3Client, config.S3Config) {
	t.Helper()

	s3Server := fakes3.NewServer(t)
	endpointHost := s3Server.EndpointHost()

	_, nc := testutil.StartSharedNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	useSSL := false
	pathStyle := true

	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret:  "test-signing-secret",
			StorageBackend: config.StorageBackendS3,
			S3: config.S3Config{
				Endpoint:        endpointHost,
				Bucket:          "test-bucket",
				PathPrefix:      pathPrefix,
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
				UseSSL:          &useSSL,
				PathStyle:       &pathStyle,
			},
		},
	}
	core, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore with S3: %v", err)
	}

	startCoreServices(t, core)

	// Create a separate S3 client for test verification (to check if objects exist)
	verifyClient, err := NewS3Client(cfg.Assets.S3)
	if err != nil {
		t.Fatalf("Failed to create verification S3 client: %v", err)
	}

	rawCfg := cfg.Assets.S3
	rawCfg.PathPrefix = ""
	rawVerifyClient, err := NewS3Client(rawCfg)
	if err != nil {
		t.Fatalf("Failed to create raw verification S3 client: %v", err)
	}

	return core, nc, verifyClient, rawVerifyClient, cfg.Assets.S3
}

// setupTestCoreWithCache creates a ChattoCore with asset caching enabled
func setupTestCoreWithCache(t *testing.T) (*ChattoCore, *nats.Conn) {
	t.Helper()

	_, nc := testutil.StartSharedNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	// Create ChattoCore with caching enabled
	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
			Cache: config.AssetsCacheConfig{
				Enabled: true,
				TTL:     config.Duration(7 * 24 * time.Hour),
			},
		},
	}
	core, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}

	startCoreServices(t, core)

	return core, nc
}

func TestChattoCore_DeleteAttachment_CleansUpCache(t *testing.T) {
	core, _ := setupTestCoreWithCache(t)
	ctx := testContext(t)

	// Verify caching is enabled
	if !core.ImageCacheEnabled() {
		t.Fatal("Image cache should be enabled for this test")
	}

	// Setup: create space, room, and attachment

	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	imageData := createTestPNG(100, 100)
	attachment, err := core.UploadAttachment(
		ctx,
		SystemActorID,
		room.Id,
		"test-image.png",
		"image/png",
		bytes.NewReader(imageData),
	)
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	// Simulate cached resizes by storing them directly. The HTTP handler
	// signs transform URLs with AttachmentSignResource and the cache uses
	// that same prefix.
	cacheKey1 := ImageCacheKey(AttachmentSignResource, attachment.Id, 200, 150, "contain")
	cacheKey2 := ImageCacheKey(AttachmentSignResource, attachment.Id, 400, 300, "cover")
	cacheKey3 := ImageCacheKey(AttachmentSignResource, attachment.Id, 100, 100, "contain")

	// Store fake cached data
	fakeWebP := []byte("fake webp data")
	if err := core.StoreCachedResize(ctx, cacheKey1, fakeWebP); err != nil {
		t.Fatalf("Failed to store cached resize 1: %v", err)
	}
	if err := core.StoreCachedResize(ctx, cacheKey2, fakeWebP); err != nil {
		t.Fatalf("Failed to store cached resize 2: %v", err)
	}
	if err := core.StoreCachedResize(ctx, cacheKey3, fakeWebP); err != nil {
		t.Fatalf("Failed to store cached resize 3: %v", err)
	}

	// Verify cache entries exist
	for _, key := range []string{cacheKey1, cacheKey2, cacheKey3} {
		data, err := core.GetCachedResize(ctx, key)
		if err != nil {
			t.Fatalf("Failed to get cached resize %s: %v", key, err)
		}
		if data == nil {
			t.Fatalf("Cache entry %s should exist before deletion", key)
		}
	}

	// Delete the attachment (should also clean up cache)
	if err := core.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
		t.Fatalf("Failed to delete attachment: %v", err)
	}

	// Verify all cache entries are deleted
	for _, key := range []string{cacheKey1, cacheKey2, cacheKey3} {
		data, err := core.GetCachedResize(ctx, key)
		if err != nil {
			t.Fatalf("Unexpected error getting cached resize %s: %v", key, err)
		}
		if data != nil {
			t.Errorf("Cache entry %s should be deleted after attachment deletion", key)
		}
	}
}

func TestChattoCore_DeleteAttachment_DoesNotAffectOtherAttachmentCache(t *testing.T) {
	core, _ := setupTestCoreWithCache(t)
	ctx := testContext(t)

	// Setup
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")

	// Create two attachments
	imageData := createTestPNG(100, 100)
	attachment1, _ := core.UploadAttachment(ctx, SystemActorID, room.Id, "image1.png", "image/png", bytes.NewReader(imageData))
	attachment2, _ := core.UploadAttachment(ctx, SystemActorID, room.Id, "image2.png", "image/png", bytes.NewReader(imageData))

	// Cache entries for both attachments
	key1 := ImageCacheKey(AttachmentSignResource, attachment1.Id, 200, 150, "contain")
	key2 := ImageCacheKey(AttachmentSignResource, attachment2.Id, 200, 150, "contain")

	fakeWebP := []byte("fake webp data")
	core.StoreCachedResize(ctx, key1, fakeWebP)
	core.StoreCachedResize(ctx, key2, fakeWebP)

	// Delete attachment1
	if err := core.DeleteAttachmentFromStorage(ctx, attachment1); err != nil {
		t.Fatalf("Failed to delete attachment1: %v", err)
	}

	// attachment1's cache should be deleted
	data1, _ := core.GetCachedResize(ctx, key1)
	if data1 != nil {
		t.Error("Deleted attachment's cache entry should be gone")
	}

	// attachment2's cache should still exist
	data2, _ := core.GetCachedResize(ctx, key2)
	if data2 == nil {
		t.Error("Other attachment's cache entry should still exist")
	}
}

func TestChattoCore_DeleteAttachment_CleansUpCacheWithoutStorageMetadata(t *testing.T) {
	core, _ := setupTestCoreWithCache(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "image.png", "image/png", bytes.NewReader(createTestPNG(100, 100)))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	cacheKey := ImageCacheKey(AttachmentSignResource, attachment.Id, 200, 150, "contain")
	if err := core.StoreCachedResize(ctx, cacheKey, []byte("fake webp data")); err != nil {
		t.Fatalf("Failed to store cached resize: %v", err)
	}

	storageLess := &corev1.Attachment{Id: attachment.Id, RoomId: room.Id}
	if err := core.DeleteAttachmentFromStorage(ctx, storageLess); err != nil {
		t.Fatalf("Failed to delete storage-less attachment: %v", err)
	}

	data, err := core.GetCachedResize(ctx, cacheKey)
	if err != nil {
		t.Fatalf("Unexpected error getting cached resize: %v", err)
	}
	if data != nil {
		t.Fatal("Cache entry should be deleted for storage-less attachment")
	}

	if _, _, err := core.GetAttachment(ctx, attachment.Id); err == nil {
		t.Fatal("Expected backing attachment binary to be deleted")
	}
}

func TestChattoCore_DeleteMessageOwnedAssetsForUser_CleansUpDerivativeCaches(t *testing.T) {
	core, _ := setupTestCoreWithCache(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "assetcleanup", "Asset Cleanup", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "test-room", "Test room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	original, err := core.UploadAttachment(ctx, user.Id, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("fake video bytes")))
	if err != nil {
		t.Fatalf("Failed to upload original video: %v", err)
	}
	thumbnail, err := core.UploadDerivativeAttachment(ctx, original.Id, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, room.Id, "thumb.png", "image/png", bytes.NewReader(createTestPNG(64, 64)))
	if err != nil {
		t.Fatalf("Failed to upload derivative thumbnail: %v", err)
	}
	inheritedRoomDerivativeID := "A-inherited-room-derivative"
	if err := core.Assets.Apply(&corev1.Event{
		Id: "E-inherited-room-derivative-created",
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				OriginalBinaryAvailable: true,
				Asset: &corev1.AssetRecord{
					Id:          inheritedRoomDerivativeID,
					Filename:    "inherited-thumb.png",
					ContentType: "image/png",
				},
				ParentAssetId:  original.Id,
				DerivativeRole: corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL,
			},
		},
	}, 999); err != nil {
		t.Fatalf("Failed to project inherited-room derivative: %v", err)
	}

	thumbnailCacheKey := ImageCacheKey(AttachmentSignResource, thumbnail.Id, 128, 128, "cover")
	if err := core.StoreCachedResize(ctx, thumbnailCacheKey, []byte("fake webp data")); err != nil {
		t.Fatalf("Failed to store thumbnail cached resize: %v", err)
	}

	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "video", []string{original.Id}, "", "", nil, false); err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	if deleted := core.DeleteMessageOwnedAssetsForUser(ctx, user.Id, user.Id); deleted != 3 {
		t.Fatalf("Expected original and derivative assets to be deleted, got %d", deleted)
	}

	if roomID, ok := core.Assets.AssetRoomID(inheritedRoomDerivativeID); !ok || roomID != room.Id {
		t.Fatalf("deleted inherited-room derivative room = %q, %v; want %q, true", roomID, ok, room.Id)
	}

	data, err := core.GetCachedResize(ctx, thumbnailCacheKey)
	if err != nil {
		t.Fatalf("Unexpected error getting thumbnail cached resize: %v", err)
	}
	if data != nil {
		t.Fatal("Derivative thumbnail cache entry should be deleted")
	}
}

func TestChattoCore_DeleteCachedResizesForAttachment_NoCacheEnabled(t *testing.T) {
	// Use standard setup (no cache)
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Should not error when cache is disabled
	deleted, err := core.DeleteCachedResizesForAttachment(ctx, "attachment")
	if err != nil {
		t.Errorf("Should not error when cache is disabled: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Should return 0 deleted when cache is disabled, got %d", deleted)
	}
}

func TestChattoCore_DeleteCachedResizesForAttachment_EmptyCache(t *testing.T) {
	core, _ := setupTestCoreWithCache(t)
	ctx := testContext(t)

	// Should handle empty cache gracefully
	deleted, err := core.DeleteCachedResizesForAttachment(ctx, "attachment")
	if err != nil {
		t.Errorf("Should not error on empty cache: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Should return 0 deleted on empty cache, got %d", deleted)
	}
}

// ============================================================================
// Attachment Lookup Tests
// ============================================================================

// TestFindBodyAttachment_RoundTrip posts a message with an attachment
// and verifies FindBodyAttachment returns the same proto from the body.
func TestFindBodyAttachment_RoundTrip(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "alice", "Alice", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "test-room", "Test room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "test.txt", "text/plain", bytes.NewReader([]byte("hello")))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "with attachment", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage failed: %v", err)
	}
	bodyKey := event.Id

	got, err := core.FindBodyAttachment(ctx, bodyKey, attachment.Id)
	if err != nil {
		t.Fatalf("FindBodyAttachment error: %v", err)
	}
	if got == nil {
		t.Fatal("Expected attachment, got nil")
	}
	if got.Id != attachment.Id {
		t.Errorf("Expected attachment id %q, got %q", attachment.Id, got.Id)
	}
	if got.MessageBodyId != bodyKey {
		t.Errorf("Expected message_body_id %q, got %q", bodyKey, got.MessageBodyId)
	}
	if got.Filename != "test.txt" {
		t.Errorf("Expected filename test.txt, got %q", got.Filename)
	}
}

// TestFindBodyAttachment_MissingReturnsNil verifies (nil, nil) for
// unknown body keys and unknown attachment IDs.
func TestFindBodyAttachment_MissingReturnsNil(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	got, err := core.FindBodyAttachment(ctx, "Uphantom.Ephantom", "ghost")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("Expected nil for missing body, got %+v", got)
	}
}

// TestLookupAttachment_BodyDispatch covers the body branch of the
// locator dispatcher.
func TestLookupAttachment_BodyDispatch(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "alice", "Alice", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "test-room", "Test room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "x.txt", "text/plain", bytes.NewReader([]byte("x")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "with attachment", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage failed: %v", err)
	}

	loc := signedurl.AttachmentLocator{
		RoomID:       room.Id,
		BodyKey:      event.Id,
		AttachmentID: attachment.Id,
		UserID:       user.Id,
		ExpiresAt:    1,
	}
	got, err := core.LookupAttachment(ctx, loc)
	if err != nil {
		t.Fatalf("LookupAttachment error: %v", err)
	}
	if got == nil || got.Id != attachment.Id {
		t.Errorf("Expected attachment %q, got %+v", attachment.Id, got)
	}
}
