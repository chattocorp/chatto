package core

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_PostMessage(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Post a message
	messageBody := "Hello, world!"
	roomEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, messageBody, nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Verify returned event metadata
	if roomEvent.ActorId != user.Id {
		t.Errorf("Event ActorId = %s, want %s", roomEvent.ActorId, user.Id)
	}

	// Verify it's a MessagePosted event
	messagePosted := roomEvent.GetMessagePosted()
	if messagePosted == nil {
		t.Fatal("Event should be a MessagePosted event")
	}

	// Verify room_id is set on the concrete event.
	if messagePosted.RoomId != room.Id {
		t.Errorf("MessagePosted.RoomId = %s, want %s", messagePosted.RoomId, room.Id)
	}

	// Body is now lazy-loaded, fetch it separately using messageBodyId
	fetchedBody, err := core.GetMessageBody(ctx, KindChannel, roomEvent.Id)
	if err != nil {
		t.Fatalf("Failed to fetch message body: %v", err)
	}
	if fetchedBody != messageBody {
		t.Errorf("Message body = %s, want %s", fetchedBody, messageBody)
	}
}

func TestChattoCore_PostMessageSchedulesVideoProcessing(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	requests := captureVideoProcessingRequests(t, core)

	roomEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Video", []string{attachment.Id}, "", "", nil, false, WithVideoProcessingAssets(attachment.Id))
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	select {
	case req := <-requests:
		if req.assetID != attachment.Id {
			t.Fatalf("queued asset id = %q, want %q", req.assetID, attachment.Id)
		}
		// The owning message id must ride along on the local work item so the worker
		// can stamp it onto the terminal event without a racy projection lookup.
		if req.messageEventID != roomEvent.Id {
			t.Fatalf("queued message event id = %q, want %q", req.messageEventID, roomEvent.Id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected local video processing request")
	}

	manifest, ok := core.RoomTimeline.VideoAttachmentManifest(attachment.Id)
	if !ok || manifest.Started == nil {
		t.Fatalf("expected AssetProcessingStarted manifest, got %+v", manifest)
	}
}

func TestChattoCore_PostMessage_BodyStoredInKV(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Post a message
	messageBody := "This is a test message for GDPR compliance!"
	roomEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, messageBody, nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Get the message ID from the event
	messagePosted := roomEvent.GetMessagePosted()
	if messagePosted == nil {
		t.Fatal("Event should be a MessagePosted event")
	}

	// Verify the body can be fetched via GetMessageBody using messageBodyId
	fetchedBody, err := core.GetMessageBody(ctx, KindChannel, roomEvent.Id)
	if err != nil {
		t.Fatalf("Failed to fetch message body: %v", err)
	}
	if fetchedBody != messageBody {
		t.Errorf("Message body = %s, want %s", fetchedBody, messageBody)
	}

	// Post-#597 cutover: the body lives embedded in the event payload
	// on the EVT stream, not in a separate SERVER_BODIES KV entry.
	// Recover it through the projection-backed GetFullMessageBody and
	// then verify the encryption envelope by peeking at the body
	// embedded on the MessagePostedEvent we just received back.
	storedBody := roomEvent.GetMessagePosted().GetBody()
	if storedBody == nil {
		t.Fatal("Expected message body to be embedded on the published event")
	}

	// Messages are always encrypted - verify encrypted fields are set
	if len(storedBody.EncryptedBody) == 0 {
		t.Error("Expected encrypted body to be non-empty")
	}
	if len(storedBody.EncryptionNonce) == 0 {
		t.Error("Expected encryption nonce to be non-empty")
	}

	// Verify timestamps are set correctly
	if storedBody.CreatedAt == nil {
		t.Error("CreatedAt should be set")
	}
	// UpdatedAt should be nil for new messages (only set when message is edited)
	if storedBody.UpdatedAt != nil {
		t.Error("UpdatedAt should be nil for new messages")
	}
}

func TestChattoCore_PostMessage_ConcurrentOCC(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Post multiple messages concurrently to test OCC retry logic.
	// 5 concurrent publishes is a realistic stress test - in practice,
	// even this level of concurrency to the exact same subject is rare.
	const numMessages = 5
	errChan := make(chan error, numMessages)
	idChan := make(chan string, numMessages)

	for i := 0; i < numMessages; i++ {
		go func(msgNum int) {
			body := fmt.Sprintf("Concurrent message %d", msgNum)
			roomEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, body, nil, "", "", nil, false)
			if err != nil {
				errChan <- err
				return
			}
			idChan <- roomEvent.Id
		}(i)
	}

	// Collect results
	var errs []error
	eventIDs := make(map[string]bool)
	for i := 0; i < numMessages; i++ {
		select {
		case err := <-errChan:
			errs = append(errs, err)
		case id := <-idChan:
			eventIDs[id] = true
		}
	}

	// All messages should succeed
	if len(errs) > 0 {
		t.Errorf("Expected no errors, got %d: %v", len(errs), errs)
	}

	// All event IDs should be unique (no duplicates from OCC retries)
	if len(eventIDs) != numMessages {
		t.Errorf("Expected %d unique event IDs, got %d", numMessages, len(eventIDs))
	}
}

func TestChattoCore_PostMessage_InvalidRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space

	// Try to post to non-existent room
	_, err := core.PostMessage(ctx, KindChannel, "nonexistent", "user123", "Hello", nil, "", "", nil, false)
	if err == nil {
		t.Error("Expected error when posting to nonexistent room")
	}
}

// TestChattoCore_PostMessage_BodyTooLong tests that oversized message bodies are rejected.
// This is a security test to prevent DoS via oversized messages.
func TestChattoCore_PostMessage_BodyTooLong(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	t.Run("message at max length succeeds", func(t *testing.T) {
		// Create a message body at exactly the max length
		maxBody := make([]byte, MaxMessageBodyLength)
		for i := range maxBody {
			maxBody[i] = 'a'
		}

		_, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, string(maxBody), nil, "", "", nil, false)
		if err != nil {
			t.Errorf("Expected success for message at max length, got: %v", err)
		}
	})

	t.Run("message over max length fails", func(t *testing.T) {
		// Create a message body over the max length
		oversizedBody := make([]byte, MaxMessageBodyLength+1)
		for i := range oversizedBody {
			oversizedBody[i] = 'a'
		}

		_, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, string(oversizedBody), nil, "", "", nil, false)
		if err == nil {
			t.Error("Expected error for oversized message body")
		}
		if err != ErrMessageTooLong {
			t.Errorf("Expected ErrMessageTooLong, got: %v", err)
		}
	})
}

// TestChattoCore_PostMessage_InvisibleChars tests that messages with only invisible Unicode
// characters are rejected. This prevents blank-looking messages that would confuse users.
func TestChattoCore_PostMessage_InvisibleChars(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	t.Run("zero-width spaces only is rejected", func(t *testing.T) {
		_, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "\u200B\u200B\u200B", nil, "", "", nil, false)
		if err == nil {
			t.Error("Expected error for message with only zero-width spaces")
		}
	})

	t.Run("mixed invisible chars only is rejected", func(t *testing.T) {
		// Mix of: zero-width space, ZWNJ, ZWJ, word joiner, BOM
		_, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "\u200B\u200C\u200D\u2060\uFEFF", nil, "", "", nil, false)
		if err == nil {
			t.Error("Expected error for message with only invisible characters")
		}
	})

	t.Run("whitespace and invisible chars only is rejected", func(t *testing.T) {
		_, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "  \u200B  \t\u200C\n", nil, "", "", nil, false)
		if err == nil {
			t.Error("Expected error for message with only whitespace and invisible chars")
		}
	})

	t.Run("visible text with invisible chars is allowed", func(t *testing.T) {
		_, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "\u200BHello\u200B", nil, "", "", nil, false)
		if err != nil {
			t.Errorf("Expected success for message with visible text, got: %v", err)
		}
	})

	t.Run("emoji only is allowed", func(t *testing.T) {
		_, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "😀", nil, "", "", nil, false)
		if err != nil {
			t.Errorf("Expected success for emoji-only message, got: %v", err)
		}
	})

	t.Run("attachment-only with unknown asset is rejected", func(t *testing.T) {
		_, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "", []string{"missing-asset"}, "", "", nil, false)
		if err == nil {
			t.Error("Expected error for attachment-only message with no resolved attachments")
		}
	})
}

func TestChattoCore_DeleteMessage_GDPR(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Post a message
	messageBody := "This message will be deleted for GDPR compliance"
	roomEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, messageBody, nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Get the message ID
	messagePosted := roomEvent.GetMessagePosted()
	if messagePosted == nil {
		t.Fatal("Event should be a MessagePosted event")
	}

	// Pre-deletion: GetMessageBody returns the plaintext.
	bodyText, err := core.GetMessageBody(ctx, KindChannel, roomEvent.Id)
	if err != nil {
		t.Fatalf("Failed to fetch message body before deletion: %v", err)
	}
	if bodyText == "" {
		t.Fatal("Message body should be non-empty before deletion")
	}

	// Delete the message (author can delete own messages).
	err = core.DeleteMessage(ctx, user.Id, KindChannel, room.Id, roomEvent.Id)
	if err != nil {
		t.Fatalf("Failed to delete message: %v", err)
	}

	// Post-deletion: projection tombstones the message, body
	// disappears from GetMessageBody.
	bodyText, err = core.GetMessageBody(ctx, KindChannel, roomEvent.Id)
	if err != nil {
		t.Fatalf("GetMessageBody on retracted message returned error: %v", err)
	}
	if bodyText != "" {
		t.Errorf("Retracted message body should be empty, got %q", bodyText)
	}

	// Idempotent: deleting again is a no-op.
	err = core.DeleteMessage(ctx, "test-user", KindChannel, room.Id, roomEvent.Id)
	if err != nil {
		t.Errorf("Deleting already deleted message should not error: %v", err)
	}
}

func TestChattoCore_DeleteMessage_DeletesAttachments(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Upload an attachment (using createTestPNG from attachments_test.go)
	imageData := createTestPNG(100, 100)
	attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "test.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	// Post a message with the attachment
	roomEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Message with attachment", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	postedMessage := roomEvent.GetMessagePosted()
	if postedMessage == nil {
		t.Fatal("Event should be a MessagePosted event")
	}

	// Verify attachment exists in ObjectStore
	store, err := core.GetAttachmentsStore(ctx)
	if err != nil {
		t.Fatalf("Failed to get attachments store: %v", err)
	}

	_, err = store.Get(ctx, attachment.Id)
	if err != nil {
		t.Fatalf("Attachment should exist before deletion: %v", err)
	}

	// Delete the message
	err = core.DeleteMessage(ctx, "test-user", KindChannel, room.Id, roomEvent.Id)
	if err != nil {
		t.Fatalf("Failed to delete message: %v", err)
	}

	// Verify attachment is also deleted
	_, err = store.Get(ctx, attachment.Id)
	if err == nil {
		t.Error("Attachment should be deleted along with the message")
	}

	// Verify message body is deleted
	body, err := core.GetMessageBody(ctx, KindChannel, roomEvent.Id)
	if err != nil {
		t.Fatalf("Failed to get message body: %v", err)
	}
	if body != "" {
		t.Error("Message body should be empty after deletion")
	}
}

func TestChattoCore_DeleteAttachmentFromMessage(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Upload two attachments
	imageData := createTestPNG(100, 100)
	attachment1, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "test1.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment 1: %v", err)
	}
	attachment2, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "test2.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment 2: %v", err)
	}

	// Post a message with both attachments
	roomEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Message with attachments", []string{attachment1.Id, attachment2.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	postedMessage := roomEvent.GetMessagePosted()
	if postedMessage == nil {
		t.Fatal("Event should be a MessagePosted event")
	}

	// Verify both attachments exist
	store, err := core.GetAttachmentsStore(ctx)
	if err != nil {
		t.Fatalf("Failed to get attachments store: %v", err)
	}
	if _, err := store.Get(ctx, attachment1.Id); err != nil {
		t.Fatalf("Attachment 1 should exist: %v", err)
	}
	if _, err := store.Get(ctx, attachment2.Id); err != nil {
		t.Fatalf("Attachment 2 should exist: %v", err)
	}

	// Delete only attachment 1
	err = core.DeleteAttachmentFromMessage(ctx, user.Id, KindChannel, room.Id, roomEvent.Id, attachment1.Id)
	if err != nil {
		t.Fatalf("Failed to delete attachment: %v", err)
	}

	// Verify attachment 1 is deleted from ObjectStore
	if _, err := store.Get(ctx, attachment1.Id); err == nil {
		t.Error("Attachment 1 should be deleted from ObjectStore")
	}

	// Verify attachment 2 still exists
	if _, err := store.Get(ctx, attachment2.Id); err != nil {
		t.Error("Attachment 2 should still exist")
	}

	// Verify message body still has attachment 2 but not attachment 1
	messageBody, err := core.GetFullMessageBody(ctx, KindChannel, roomEvent.Id)
	if err != nil {
		t.Fatalf("Failed to get message body: %v", err)
	}
	if len(messageBody.Attachments) != 1 {
		t.Errorf("Expected 1 attachment, got %d", len(messageBody.Attachments))
	}
	if messageBody.Attachments[0].Id != attachment2.Id {
		t.Error("Remaining attachment should be attachment 2")
	}
}

func TestChattoCore_DeleteAttachmentFromMessage_DeletesVideoDerivatives(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	original, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "original.mp4", "video/mp4", bytes.NewReader([]byte("original")))
	if err != nil {
		t.Fatalf("Failed to upload original: %v", err)
	}
	thumb, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "thumb.png", "image/png", bytes.NewReader(createTestPNG(32, 18)))
	if err != nil {
		t.Fatalf("Failed to upload thumbnail: %v", err)
	}
	variantAttachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "video-720p.mp4", "video/mp4", bytes.NewReader([]byte("variant")))
	if err != nil {
		t.Fatalf("Failed to upload variant: %v", err)
	}

	roomEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Video", []string{original.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	if err := core.RecordAssetProcessed(ctx, SystemActorID, KindChannel, room.Id, roomEvent.Id, original.Id, 1234, 640, 360, thumb, []*corev1.VideoVariant{
		{
			AttachmentId: variantAttachment.Id,
			Quality:      "720p",
			Width:        640,
			Height:       360,
			Size:         variantAttachment.Size,
			Attachment:   variantAttachment,
		},
	}); err != nil {
		t.Fatalf("Failed to record processed video manifest: %v", err)
	}

	store, err := core.GetAttachmentsStore(ctx)
	if err != nil {
		t.Fatalf("Failed to get attachments store: %v", err)
	}
	for _, attachment := range []*corev1.Attachment{original, thumb, variantAttachment} {
		if _, err := store.Get(ctx, attachment.Id); err != nil {
			t.Fatalf("Attachment %s should exist before deletion: %v", attachment.Id, err)
		}
	}

	if err := core.DeleteAttachmentFromMessage(ctx, user.Id, KindChannel, room.Id, roomEvent.Id, original.Id); err != nil {
		t.Fatalf("Failed to delete video attachment: %v", err)
	}

	for _, attachment := range []*corev1.Attachment{original, thumb, variantAttachment} {
		if _, err := store.Get(ctx, attachment.Id); err == nil {
			t.Fatalf("Attachment %s should be deleted", attachment.Id)
		}
	}
}

func TestChattoCore_DeleteAttachmentFromMessage_NotAuthor(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and two users
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	author, _ := core.CreateUser(ctx, "system", "author", "author", "password123")
	otherUser, _ := core.CreateUser(ctx, "system", "other", "other", "password123")

	// Both users join space and room
	core.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id)
	core.JoinRoom(ctx, otherUser.Id, KindChannel, otherUser.Id, room.Id)

	// Upload attachment and post message as author
	imageData := createTestPNG(100, 100)
	attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "test.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	roomEvent, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "Message with attachment", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Try to delete attachment as other user - should fail
	err = core.DeleteAttachmentFromMessage(ctx, otherUser.Id, KindChannel, room.Id, roomEvent.Id, attachment.Id)
	if err == nil {
		t.Error("Expected error when non-author tries to delete attachment")
	}
	if err != ErrNotMessageAuthor {
		t.Errorf("Expected ErrNotMessageAuthor, got: %v", err)
	}

	// Verify attachment still exists
	store, _ := core.GetAttachmentsStore(ctx)
	if _, err := store.Get(ctx, attachment.Id); err != nil {
		t.Error("Attachment should still exist after failed deletion")
	}
}

// S3 Attachment Deletion Integration Tests
// ============================================================================

func TestChattoCore_DeleteMessage_DeletesS3Attachments(t *testing.T) {
	core, _, s3Client := setupTestCoreWithS3(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Upload attachment (stored in S3)
	imageData := createTestPNG(100, 100)
	attachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "test.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	s3Key := attachment.Storage.GetS3().Key

	// Post message with attachment
	roomEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Message with S3 attachment", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Verify S3 object exists
	_, err = s3Client.StatObject(ctx, s3Key)
	if err != nil {
		t.Fatalf("S3 object should exist before deletion: %v", err)
	}

	// Delete the message
	err = core.DeleteMessage(ctx, user.Id, KindChannel, room.Id, roomEvent.Id)
	if err != nil {
		t.Fatalf("Failed to delete message: %v", err)
	}

	// Verify S3 object is also deleted
	_, err = s3Client.StatObject(ctx, s3Key)
	if err == nil {
		t.Error("S3 object should be deleted along with the message")
	}
}

func TestChattoCore_DeleteAttachmentFromMessage_S3(t *testing.T) {
	core, _, s3Client := setupTestCoreWithS3(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Upload two attachments (stored in S3)
	imageData := createTestPNG(100, 100)
	attachment1, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "test1.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment 1: %v", err)
	}
	attachment2, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "test2.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment 2: %v", err)
	}

	s3Key1 := attachment1.Storage.GetS3().Key
	s3Key2 := attachment2.Storage.GetS3().Key

	// Post message with both attachments
	roomEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Message with S3 attachments", []string{attachment1.Id, attachment2.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Delete only attachment 1
	err = core.DeleteAttachmentFromMessage(ctx, user.Id, KindChannel, room.Id, roomEvent.Id, attachment1.Id)
	if err != nil {
		t.Fatalf("Failed to delete attachment from message: %v", err)
	}

	// Verify attachment 1 is deleted from S3
	_, err = s3Client.StatObject(ctx, s3Key1)
	if err == nil {
		t.Error("Attachment 1 should be deleted from S3")
	}

	// Verify attachment 2 still exists in S3
	_, err = s3Client.StatObject(ctx, s3Key2)
	if err != nil {
		t.Error("Attachment 2 should still exist in S3")
	}

	// Verify message body still has attachment 2 but not attachment 1
	messageBody, err := core.GetFullMessageBody(ctx, KindChannel, roomEvent.Id)
	if err != nil {
		t.Fatalf("Failed to get message body: %v", err)
	}
	if len(messageBody.Attachments) != 1 {
		t.Errorf("Expected 1 attachment remaining, got %d", len(messageBody.Attachments))
	}
	if messageBody.Attachments[0].Id != attachment2.Id {
		t.Error("Remaining attachment should be attachment 2")
	}
}

// ============================================================================
// Archive blocks writes
// ============================================================================

func TestChattoCore_ArchiveRoom_BlocksWrites(t *testing.T) {
	t.Run("cannot post in archived room", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		user, _ := core.CreateUser(ctx, "system", "poster", "poster", "password123")
		room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "general", "")
		core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
		if _, err := core.ArchiveRoom(ctx, user.Id, KindChannel, room.Id); err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		_, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "hello", nil, "", "", nil, false)
		if !errors.Is(err, ErrRoomArchived) {
			t.Errorf("Expected ErrRoomArchived posting to archived room, got: %v", err)
		}
	})

	t.Run("cannot edit message in archived room", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		user, _ := core.CreateUser(ctx, "system", "editor", "editor", "password123")
		room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "general", "")
		core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
		event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "hello", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage failed: %v", err)
		}
		msgBodyKey := event.Id

		if _, err := core.ArchiveRoom(ctx, user.Id, KindChannel, room.Id); err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		err = core.EditMessage(ctx, user.Id, KindChannel, room.Id, msgBodyKey, "edited")
		if !errors.Is(err, ErrRoomArchived) {
			t.Errorf("Expected ErrRoomArchived editing in archived room, got: %v", err)
		}
	})

	t.Run("cannot react in archived room", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		user, _ := core.CreateUser(ctx, "system", "reactor", "reactor", "password123")
		room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "general", "")
		core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
		event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "hello", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage failed: %v", err)
		}
		eventID := event.Id

		if _, err := core.ArchiveRoom(ctx, user.Id, KindChannel, room.Id); err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		_, err = core.AddReaction(ctx, KindChannel, room.Id, eventID, "thumbsup", user.Id)
		if !errors.Is(err, ErrRoomArchived) {
			t.Errorf("Expected ErrRoomArchived reacting in archived room, got: %v", err)
		}
	})
}
