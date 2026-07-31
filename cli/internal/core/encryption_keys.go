package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
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

	if err := c.userModel.waitForContentKeysCurrent(ctx, userID); err != nil {
		return err
	}

	contentKeyRefs, wrappingKeyRefs, err := c.userModel.keyRefsForShredding(userID)
	if err != nil {
		return err
	}
	keyRefs := make(map[string]struct{})
	keyRefs[kms.LegacyUserKeyRef(userID)] = struct{}{}
	for _, keyRef := range wrappingKeyRefs {
		if keyRef != "" {
			keyRefs[keyRef] = struct{}{}
		}
	}
	for _, contentKeyRef := range contentKeyRefs {
		if c.encryption.contentKeys == nil {
			return fmt.Errorf("content key store is not configured")
		}
		stored, err := c.encryption.contentKeys.Get(ctx, contentKeyRef)
		if err != nil {
			return fmt.Errorf("failed to load DEK %s before shredding: %w", contentKeyRef, err)
		}
		if wrappingKeyRef := stored.GetWrappingKeyRef(); wrappingKeyRef != "" {
			keyRefs[wrappingKeyRef] = struct{}{}
		}
	}

	shredded := false
	for _, contentKeyRef := range contentKeyRefs {
		if err := c.encryption.contentKeys.Shred(ctx, contentKeyRef); err != nil {
			return err
		}
		shredded = true
	}

	for keyRef := range keyRefs {
		exists, err := c.encryption.keyWrapper.KeyExists(ctx, keyRef)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if err := c.encryption.keyWrapper.ShredKey(ctx, keyRef); err != nil {
			return err
		}
		shredded = true
	}
	if !shredded {
		return nil
	}
	forgetDEKRequestCacheUser(ctx, userID)

	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_UserKeyShredded{
			UserKeyShredded: &corev1.UserKeyShreddedEvent{UserId: userID},
		},
	})
	seq, err := c.appendUserEvent(ctx, userID, event, "", nil)
	if err != nil {
		return fmt.Errorf("failed to record user key shred event: %w", err)
	}
	subject := evtstream.UserAggregate(userID).SubjectFor(event)
	return c.roomModel.waitForTimelineAndThreads(ctx, events.SubjectPosition(subject, seq))
}
