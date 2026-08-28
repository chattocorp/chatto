package core

import (
	"context"

	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/kms"
)

// encryptionManager handles message body encryption/decryption.
type encryptionManager struct {
	keyWrapper  kms.KeyWrapper
	legacyKeys  kms.LegacyKeyProvider
	callKeys    kms.CallKeyStore
	contentKeys *dekstore.Store
}

// KeyWrapper returns the key-only KMS boundary used by encryption operations.
func (c *ChattoCore) KeyWrapper() kms.KeyWrapper {
	return c.encryption.keyWrapper
}

// DeleteUserEncryptionKey permanently deletes a user's encryption key (crypto-shredding).
// All messages encrypted with this key become permanently unreadable.
// This is used for GDPR-compliant user deletion.
func (c *ChattoCore) DeleteUserEncryptionKey(ctx context.Context, userID string) error {
	return c.DeleteUserEncryptionKeyAs(ctx, userID, userID)
}

func (c *ChattoCore) deleteEncryptionKeyOnly(ctx context.Context, keyRef string) error {
	if c.encryption.keyWrapper == nil {
		return nil
	}
	return c.encryption.keyWrapper.ShredKey(ctx, keyRef)
}

func (c *ChattoCore) DeleteUserEncryptionKeyAs(ctx context.Context, actorID, userID string) error {
	if c.encryption.keyWrapper == nil {
		return nil // Encryption not configured
	}
	return c.keyShredding.Request(ctx, actorID, userID)
}
