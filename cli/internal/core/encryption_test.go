package core

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/pkg/signedurl"
)

// setupTestCoreWithEncryption creates a ChattoCore for encryption tests.
func setupTestCoreWithEncryption(t *testing.T) *ChattoCore {
	t.Helper()

	_, nc := testutil.StartNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}

	core, err := NewChattoCore(ctx, nc, cfg)
	require.NoError(t, err)

	startCoreServices(t, core)

	return core
}

func TestPostMessage_EncryptsMessageBody(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	// Setup: create user, space, room
	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	require.NoError(t, err)

	require.NoError(t, err)

	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)

	// Post a message
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Secret message content", nil, "", "", nil, false)
	require.NoError(t, err)
	require.NotNil(t, event)

	// Post-#597 cutover: the body is embedded in the MessagePostedEvent
	// payload, not stored separately in SERVER_BODIES. Inspect the
	// embedded MessageBody directly.
	stored := event.GetMessagePosted().GetBody()
	require.NotNil(t, stored, "expected embedded body on the published event")
	require.NotEmpty(t, stored.EncryptedBody, "encrypted body should not be empty")
	require.NotEmpty(t, stored.EncryptionNonce, "nonce should not be empty")

	// Verify we can read the message back (decrypted)
	body, err := core.GetMessageBody(ctx, KindChannel, event.Id)
	require.NoError(t, err)
	require.Equal(t, "Secret message content", body, "decrypted message should match original")
}

func TestGetMessageBody_CryptoShredding(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	// Setup: create user, space, room
	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	require.NoError(t, err)

	require.NoError(t, err)

	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)

	// Post an encrypted message
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Secret message content", nil, "", "", nil, false)
	require.NoError(t, err)

	// Verify message can be read before key deletion
	body, err := core.GetMessageBody(ctx, KindChannel, event.Id)
	require.NoError(t, err)
	require.Equal(t, "Secret message content", body)

	// Delete the user's encryption key (crypto-shredding)
	err = core.encryption.keyManager.DeleteUserKey(ctx, user.Id)
	require.NoError(t, err)

	// Verify message now returns empty string (crypto-shredded)
	body, err = core.GetMessageBody(ctx, KindChannel, event.Id)
	require.NoError(t, err)
	require.Empty(t, body, "message should be empty after crypto-shredding")

	// Also test GetFullMessageBody - returns nil for crypto-shredded (same as deleted)
	fullBody, err := core.GetFullMessageBody(ctx, KindChannel, event.Id)
	require.NoError(t, err)
	require.Nil(t, fullBody, "message should be nil after crypto-shredding (treated same as deleted)")
}

func TestDeleteUser_CryptoShredEventTombstonesMessagesAndDeletesAssetGraph(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	author, err := core.CreateUser(ctx, "system", "shredauthor", "Shred Author", "password123")
	require.NoError(t, err)
	viewer, err := core.CreateUser(ctx, "system", "shredviewer", "Shred Viewer", "password123")
	require.NoError(t, err)

	room, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)
	_, err = core.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id)
	require.NoError(t, err)

	original, err := core.UploadAttachment(ctx, author.Id, room.Id, "secret.txt", "text/plain", bytes.NewReader([]byte("secret original")))
	require.NoError(t, err)
	thumbnail, err := core.UploadDerivativeAttachment(ctx, original.Id, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, room.Id, "thumb.png", "image/png", bytes.NewReader(createTestPNG(16, 16)))
	require.NoError(t, err)
	variant, err := core.UploadDerivativeAttachment(ctx, original.Id, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_VIDEO_VARIANT, room.Id, "variant.mp4", "video/mp4", bytes.NewReader([]byte("secret variant")))
	require.NoError(t, err)

	event, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "message with secret asset", []string{original.Id}, "", "", nil, false)
	require.NoError(t, err)

	for _, att := range []*corev1.Attachment{original, thumbnail, variant} {
		_, _, err := core.GetAttachmentReader(ctx, att)
		require.NoError(t, err, "precondition: %s should be readable before shred", att.Id)
	}

	require.NoError(t, core.DeleteUser(ctx, author.Id, author.Id))

	shredEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(author.Id).Subject(events.EventUserKeyShredded))
	require.NoError(t, err)
	require.Len(t, shredEvents, 1)
	require.Equal(t, author.Id, shredEvents[0].GetActorId())
	require.Equal(t, author.Id, shredEvents[0].GetUserKeyShredded().GetUserId())

	fullBody, err := core.GetFullMessageBodyByEventID(ctx, event.Id)
	require.NoError(t, err)
	require.Nil(t, fullBody, "message body should be tombstoned by UserKeyShreddedEvent before decrypt")

	deletedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.RoomAggregate(room.Id).Subject(events.EventAssetDeleted))
	require.NoError(t, err)
	require.Len(t, deletedEvents, 3)
	deletedIDs := map[string]bool{}
	for _, e := range deletedEvents {
		deletedIDs[e.GetAssetDeleted().GetAssetId()] = true
	}
	require.True(t, deletedIDs[original.Id], "source asset should get AssetDeletedEvent")
	require.True(t, deletedIDs[thumbnail.Id], "thumbnail derivative should get AssetDeletedEvent")
	require.True(t, deletedIDs[variant.Id], "variant derivative should get AssetDeletedEvent")

	for _, att := range []*corev1.Attachment{original, thumbnail, variant} {
		got, err := core.LookupAttachment(ctx, signedurl.AttachmentLocator{
			RoomID:       room.Id,
			AttachmentID: att.Id,
			UserID:       viewer.Id,
			ExpiresAt:    time.Now().Add(time.Minute).Unix(),
		})
		require.NoError(t, err)
		require.Nil(t, got, "deleted asset %s should no longer resolve through the serving projection", att.Id)

		_, _, err = core.GetAttachmentReader(ctx, att)
		require.Error(t, err, "backing bytes for %s should be deleted", att.Id)
	}
}

func TestEditMessage_PreservesEncryptionState(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	// Setup: create user, space, room
	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	require.NoError(t, err)

	require.NoError(t, err)

	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)

	// Post an encrypted message
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Original content", nil, "", "", nil, false)
	require.NoError(t, err)
	messageBodyID := event.Id

	// Edit the message
	err = core.EditMessage(ctx, user.Id, KindChannel, room.Id, messageBodyID, "Edited content")
	require.NoError(t, err)

	// Post-#597 cutover: the edited body rides on a MessageEditedEvent
	// in the EVT stream, surfaced via the projection's LatestBody.
	stored, retracted, ok := core.RoomTimeline.LatestBody(event.Id)
	require.True(t, ok, "expected the edited message to still be projected")
	require.False(t, retracted, "message should not be retracted by an edit")
	require.NotNil(t, stored, "expected a body after edit")
	require.NotEmpty(t, stored.EncryptedBody, "encrypted body should not be empty after edit")
	require.NotEmpty(t, stored.EncryptionNonce, "nonce should not be empty after edit")

	// Verify we can read the edited content
	body, err := core.GetMessageBody(ctx, KindChannel, messageBodyID)
	require.NoError(t, err)
	require.Equal(t, "Edited content", body)
}

func TestCrossUserDecryption(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	// Create two users
	userA, err := core.CreateUser(ctx, "system", "userA", "User A", "password123")
	require.NoError(t, err)

	userB, err := core.CreateUser(ctx, "system", "userB", "User B", "password123")
	require.NoError(t, err)

	// User A creates a space and room
	require.NoError(t, err)

	room, err := core.CreateRoom(ctx, userA.Id, KindChannel, "", "General", "General chat")
	require.NoError(t, err)

	// User A posts a message
	eventA, err := core.PostMessage(ctx, KindChannel, room.Id, userA.Id, "Message from User A", nil, "", "", nil, false)
	require.NoError(t, err)

	// User B should be able to read User A's message (decrypted)
	bodyA, err := core.GetMessageBody(ctx, KindChannel, eventA.Id)
	require.NoError(t, err)
	require.Equal(t, "Message from User A", bodyA, "User B should be able to read User A's decrypted message")

	// User B posts a message
	eventB, err := core.PostMessage(ctx, KindChannel, room.Id, userB.Id, "Message from User B", nil, "", "", nil, false)
	require.NoError(t, err)

	// User A should be able to read User B's message (decrypted)
	bodyB, err := core.GetMessageBody(ctx, KindChannel, eventB.Id)
	require.NoError(t, err)
	require.Equal(t, "Message from User B", bodyB, "User A should be able to read User B's decrypted message")
}
