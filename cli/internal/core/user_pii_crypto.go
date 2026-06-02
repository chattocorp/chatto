package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/encryption"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func userPIIAAD(eventID, userID, eventType, purpose string, epoch int32) []byte {
	return []byte(fmt.Sprintf("chatto:user-pii-context:v1\x00event_id=%s\x00user_id=%s\x00event_type=%s\x00field=%s\x00content_key_epoch=%d", eventID, userID, eventType, purpose, epoch))
}

func encryptUserPIIStringWithDEK(dek *userDEK, eventID, userID, eventType, purpose, plaintext string) (*corev1.EncryptedUserString, error) {
	if dek == nil || dek.epoch <= 0 || len(dek.key) == 0 {
		return nil, fmt.Errorf("DEK is missing")
	}
	encrypted, err := encryption.EncryptXChaCha20Poly1305(dek.key, []byte(plaintext), userPIIAAD(eventID, userID, eventType, purpose, dek.epoch))
	if err != nil {
		return nil, err
	}
	return &corev1.EncryptedUserString{
		EncryptedValue:  encrypted.Ciphertext,
		Nonce:           encrypted.Nonce,
		ContentKeyEpoch: dek.epoch,
	}, nil
}

func encryptUserPIIStringWithContentKey(contentKey *messageContentKey, eventID, userID, eventType, purpose, plaintext string) (*corev1.EncryptedUserString, error) {
	return encryptUserPIIStringWithDEK(contentKey, eventID, userID, eventType, purpose, plaintext)
}

func (c *ChattoCore) encryptUserPIIString(ctx context.Context, eventID, userID, eventType, purpose, plaintext string) (*corev1.EncryptedUserString, error) {
	dek, err := c.ensureActiveUserPIIDEK(ctx, userID)
	if err != nil {
		return nil, err
	}
	return encryptUserPIIStringWithDEK(dek, eventID, userID, eventType, purpose, plaintext)
}

func decryptUserPIIString(contentKey []byte, eventID, userID, eventType, purpose string, encrypted *corev1.EncryptedUserString) (string, error) {
	if encrypted == nil {
		return "", fmt.Errorf("encrypted user string is nil")
	}
	epoch := encrypted.GetContentKeyEpoch()
	if epoch <= 0 {
		return "", fmt.Errorf("encrypted user string content key epoch is missing")
	}
	plaintext, err := encryption.DecryptXChaCha20Poly1305(
		contentKey,
		encrypted.GetEncryptedValue(),
		encrypted.GetNonce(),
		userPIIAAD(eventID, userID, eventType, purpose, epoch),
	)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
