package core

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// setupTestCoreWithEncryption creates a ChattoCore for encryption tests.
func setupTestCoreWithEncryption(t *testing.T) *ChattoCore {
	t.Helper()

	// Start embedded NATS server
	opts := &server.Options{
		JetStream: true,
		Port:      -1,
		StoreDir:  t.TempDir(),
	}

	ns, err := server.NewServer(opts)
	require.NoError(t, err)

	go ns.Start()
	require.True(t, ns.ReadyForConnections(5*time.Second))

	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)

	t.Cleanup(func() {
		nc.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	cfg := config.CoreConfig{
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}

	core, err := NewChattoCore(ctx, nc, cfg)
	require.NoError(t, err)

	return core
}

func TestPostMessage_EncryptsMessageBody(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	// Setup: create user, space, room
	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	require.NoError(t, err)

	require.NoError(t, err)

	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "General", "General chat")
	require.NoError(t, err)

	// Post a message
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Secret message content", nil, "", "", nil, false)
	require.NoError(t, err)
	require.NotNil(t, event)

	// Verify message is encrypted in storage (read raw from bucket)
	bucket := core.storage.serverBodiesKV

	// MessageBodyId now contains the full compound key ({userId}.{bodyId})
	entry, err := bucket.Get(ctx, event.GetMessagePosted().MessageBodyId)
	require.NoError(t, err)

	var stored corev1.MessageBody
	err = proto.Unmarshal(entry.Value(), &stored)
	require.NoError(t, err)

	// Verify encrypted body is non-empty
	require.NotEmpty(t, stored.EncryptedBody, "encrypted body should not be empty")
	require.NotEmpty(t, stored.EncryptionNonce, "nonce should not be empty")

	// Verify we can read the message back (decrypted)
	body, err := core.GetMessageBody(ctx, KindChannel, event.GetMessagePosted().MessageBodyId)
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

	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "General", "General chat")
	require.NoError(t, err)

	// Post an encrypted message
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Secret message content", nil, "", "", nil, false)
	require.NoError(t, err)

	// Verify message can be read before key deletion
	body, err := core.GetMessageBody(ctx, KindChannel, event.GetMessagePosted().MessageBodyId)
	require.NoError(t, err)
	require.Equal(t, "Secret message content", body)

	// Delete the user's encryption key (crypto-shredding)
	err = core.encryption.keyManager.DeleteUserKey(ctx, user.Id)
	require.NoError(t, err)

	// Verify message now returns empty string (crypto-shredded)
	body, err = core.GetMessageBody(ctx, KindChannel, event.GetMessagePosted().MessageBodyId)
	require.NoError(t, err)
	require.Empty(t, body, "message should be empty after crypto-shredding")

	// Also test GetFullMessageBody - returns nil for crypto-shredded (same as deleted)
	fullBody, err := core.GetFullMessageBody(ctx, KindChannel, event.GetMessagePosted().MessageBodyId)
	require.NoError(t, err)
	require.Nil(t, fullBody, "message should be nil after crypto-shredding (treated same as deleted)")
}

func TestEditMessage_PreservesEncryptionState(t *testing.T) {
	core := setupTestCoreWithEncryption(t)
	ctx := testContext(t)

	// Setup: create user, space, room
	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	require.NoError(t, err)

	require.NoError(t, err)

	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "General", "General chat")
	require.NoError(t, err)

	// Post an encrypted message
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Original content", nil, "", "", nil, false)
	require.NoError(t, err)
	messageBodyID := event.GetMessagePosted().MessageBodyId

	// Edit the message
	err = core.EditMessage(ctx, user.Id, KindChannel, room.Id, messageBodyID, "Edited content")
	require.NoError(t, err)

	// Verify the edited message is still encrypted in storage
	bucket := core.storage.serverBodiesKV

	// messageBodyID is the full compound key ({userId}.{bodyId})
	entry, err := bucket.Get(ctx, messageBodyID)
	require.NoError(t, err)

	var stored corev1.MessageBody
	err = proto.Unmarshal(entry.Value(), &stored)
	require.NoError(t, err)

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

	room, err := core.CreateRoom(ctx, userA.Id, KindChannel, "General", "General chat")
	require.NoError(t, err)

	// User A posts a message
	eventA, err := core.PostMessage(ctx, KindChannel, room.Id, userA.Id, "Message from User A", nil, "", "", nil, false)
	require.NoError(t, err)

	// User B should be able to read User A's message (decrypted)
	bodyA, err := core.GetMessageBody(ctx, KindChannel, eventA.GetMessagePosted().MessageBodyId)
	require.NoError(t, err)
	require.Equal(t, "Message from User A", bodyA, "User B should be able to read User A's decrypted message")

	// User B posts a message
	eventB, err := core.PostMessage(ctx, KindChannel, room.Id, userB.Id, "Message from User B", nil, "", "", nil, false)
	require.NoError(t, err)

	// User A should be able to read User B's message (decrypted)
	bodyB, err := core.GetMessageBody(ctx, KindChannel, eventB.GetMessagePosted().MessageBodyId)
	require.NoError(t, err)
	require.Equal(t, "Message from User B", bodyB, "User A should be able to read User B's decrypted message")
}
