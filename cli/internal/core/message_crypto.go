package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/encryption"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func messageBodyAAD(eventID, roomID, authorID string) []byte {
	return []byte(fmt.Sprintf("chatto:message-body-context:v2\x00event_type=message_body\x00event_id=%s\x00room_id=%s\x00author_id=%s", eventID, roomID, authorID))
}

func (c *ChattoCore) encryptMessageBody(ctx context.Context, body *corev1.MessageBody, roomID, eventID, plaintext string) error {
	if body == nil {
		return fmt.Errorf("message body is nil")
	}
	authorID := body.GetAuthorId()
	if authorID == "" {
		return fmt.Errorf("message body author is empty")
	}
	key, err := c.encryption.keyManager.GetUserKey(ctx, authorID)
	if err != nil {
		return fmt.Errorf("failed to get encryption key: %w", err)
	}
	if key == nil {
		return fmt.Errorf("encryption key not found for user %s", authorID)
	}

	encrypted, err := encryption.EncryptEnvelope(key, []byte(plaintext), messageBodyAAD(eventID, roomID, authorID))
	if err != nil {
		return fmt.Errorf("failed to encrypt message body: %w", err)
	}
	body.EncryptedBody = encrypted.Ciphertext
	body.EncryptionNonce = encrypted.Nonce
	body.EncryptionVersion = encrypted.Version
	body.EncryptedDataKey = encrypted.EncryptedDataKey
	body.DataKeyNonce = encrypted.DataKeyNonce
	return nil
}
