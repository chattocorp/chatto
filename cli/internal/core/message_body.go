package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/encryption"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// DecryptedMessageBody represents a message body with decrypted content.
// Used as the return type for GetFullMessageBody since the proto no longer has a plaintext field.
type DecryptedMessageBody struct {
	AuthorId    string
	Body        string // Decrypted message text
	Attachments []*corev1.Attachment
	LinkPreview *corev1.LinkPreview
	CreatedAt   time.Time
	UpdatedAt   *time.Time // nil if never edited
}

// GetFullMessageBody retrieves the complete message body from the BODIES bucket.
// Used by GraphQL resolvers for lazy-loading message content and attachments.
// The messageBodyKey parameter is the full compound key ({userId}.{bodyId}) stored in the event.
// Returns nil if the body doesn't exist (e.g., deleted for GDPR).
// If the encryption key is missing (crypto-shredded), returns nil (same as deleted)
// which triggers "[Message unavailable]" display in UI.
func (c *ChattoCore) GetFullMessageBody(ctx context.Context, kind RoomKind, messageBodyKey string) (*DecryptedMessageBody, error) {
	bucket := c.storage.serverBodiesKV

	entry, err := bucket.Get(ctx, messageBodyKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil // Return nil for missing bodies (deleted for GDPR)
		}
		return nil, fmt.Errorf("failed to fetch message body: %w", err)
	}

	var messageBody corev1.MessageBody
	if err := proto.Unmarshal(entry.Value(), &messageBody); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message body: %w", err)
	}

	// Decrypt the message body
	decrypted, err := c.decryptMessageBody(ctx, &messageBody)
	if err != nil {
		// Key not found = crypto-shredded, treat as unavailable (same as deleted)
		if errors.Is(err, encryption.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to decrypt message body: %w", err)
	}

	result := &DecryptedMessageBody{
		AuthorId:    messageBody.AuthorId,
		Body:        string(decrypted),
		Attachments: messageBody.Attachments,
		LinkPreview: messageBody.LinkPreview,
		CreatedAt:   messageBody.CreatedAt.AsTime(),
	}
	if messageBody.UpdatedAt != nil {
		t := messageBody.UpdatedAt.AsTime()
		result.UpdatedAt = &t
	}
	return result, nil
}

// decryptMessageBody decrypts an encrypted message body using the author's key.
func (c *ChattoCore) decryptMessageBody(ctx context.Context, msg *corev1.MessageBody) ([]byte, error) {
	key, err := c.encryption.keyManager.GetUserKey(ctx, msg.AuthorId)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}
	if key == nil {
		return nil, encryption.ErrKeyNotFound
	}

	return encryption.Decrypt(key, msg.EncryptedBody, msg.EncryptionNonce)
}

// GetMessageBody retrieves a message body text from the bodies KV bucket.
// The messageBodyKey parameter is the full compound key ({userId}.{bodyId}) stored in the event.
// Returns empty string if the body has been deleted (GDPR), doesn't exist,
// or if the encryption key has been deleted (crypto-shredded).
// Prefer GetFullMessageBody when you need attachments or other metadata.
func (c *ChattoCore) GetMessageBody(ctx context.Context, kind RoomKind, messageBodyKey string) (string, error) {
	body, err := c.GetFullMessageBody(ctx, kind, messageBodyKey)
	if err != nil {
		return "", err
	}
	if body == nil {
		return "", nil
	}
	return body.Body, nil
}

// GetMessageAuthorID retrieves the author ID for a message body.
// Returns empty string if the message doesn't exist (already deleted).
// Used by GraphQL layer to check ownership before calling DeleteMessage.
func (c *ChattoCore) GetMessageAuthorID(ctx context.Context, kind RoomKind, messageBodyID string) (string, error) {
	messageBody, err := c.GetFullMessageBody(ctx, kind, messageBodyID)
	if err != nil {
		return "", err
	}
	if messageBody == nil {
		return "", nil // Message already deleted
	}
	return messageBody.AuthorId, nil
}

// deleteUserMessageBodiesInSpace deletes all message bodies authored by a user in a specific space.
// This is used during account deletion to remove the user's message content entirely.
// Returns the number of message bodies deleted.
// Note: This only removes bodies from spaces the user was a member of. Bodies in spaces they
// left before deletion will still be crypto-shredded when the encryption key is deleted.
//
// The key format is {userId}.{bodyId}, so we can efficiently filter by userId prefix
// to find only this user's message bodies without scanning the entire bucket.
func (c *ChattoCore) deleteUserMessageBodiesInSpace(ctx context.Context, userID string, kind RoomKind) (int, error) {
	bucket := c.storage.serverBodiesKV

	// Use prefix filter to find only this user's message bodies
	// Key format: {userId}.{bodyId} - filter by userID prefix
	lister, err := bucket.ListKeysFiltered(ctx, userID+".")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil // No bodies for this user in this space
		}
		return 0, fmt.Errorf("failed to list message body keys: %w", err)
	}

	// Collect all keys first (iterator becomes invalid after first pass)
	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}

	deleted := 0
	for _, key := range keys {
		// Get the message body to find attachments to delete
		entry, err := bucket.Get(ctx, key)
		if err != nil {
			c.logger.Debug("Failed to get message body during deletion", "key", key, "error", err)
			continue
		}

		var messageBody corev1.MessageBody
		if err := proto.Unmarshal(entry.Value(), &messageBody); err != nil {
			c.logger.Debug("Failed to unmarshal message body during deletion", "key", key, "error", err)
			continue
		}

		// Delete all attachments from storage (supports both NATS and S3)
		for _, attachment := range messageBody.Attachments {
			if err := c.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
				c.logger.Warn("Failed to delete attachment during user deletion",
					"attachment_id", attachment.Id,
					"message_body_key", key,
					"error", err)
				// Continue deleting other attachments
			}
		}

		// Delete the message body
		if err := bucket.Delete(ctx, key); err != nil {
			c.logger.Warn("Failed to delete message body during user deletion", "key", key, "error", err)
			continue
		}

		deleted++
	}

	return deleted, nil
}
