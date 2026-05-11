package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var (
	// ErrOIDCSubjectAlreadyClaimed is returned when an OIDC subject is already linked to a different user.
	ErrOIDCSubjectAlreadyClaimed = errors.New("OIDC subject is already linked to another account")
)

// userByOIDCSubjectKey returns the KV key for the OIDC subject-to-user index.
// Uses SHA256 hash of "issuer:subject" to ensure valid NATS subject characters.
func userByOIDCSubjectKey(issuer, subject string) string {
	hash := sha256.Sum256([]byte(issuer + ":" + subject))
	return fmt.Sprintf("user_by_oidc.%s", hex.EncodeToString(hash[:]))
}

// GetUserByOIDCSubject looks up a user by their OIDC issuer and subject.
func (c *ChattoCore) GetUserByOIDCSubject(ctx context.Context, issuer, subject string) (*corev1.User, error) {
	key := userByOIDCSubjectKey(issuer, subject)

	entry, err := c.storage.serverKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to lookup user by OIDC subject: %w", err)
	}

	userID := string(entry.Value())
	return c.GetUser(ctx, userID)
}

// LinkOIDCSubject links an OIDC subject to a Chatto user. Uses atomic create
// to prevent race conditions. Idempotent if already linked to the same user.
func (c *ChattoCore) LinkOIDCSubject(ctx context.Context, issuer, subject, userID string) error {
	key := userByOIDCSubjectKey(issuer, subject)

	_, err := c.storage.serverKV.Create(ctx, key, []byte(userID))
	if err != nil {
		// Already claimed — check if it's by the same user (idempotent)
		entry, getErr := c.storage.serverKV.Get(ctx, key)
		if getErr == nil && string(entry.Value()) == userID {
			return nil // Already linked to this user
		}
		return ErrOIDCSubjectAlreadyClaimed
	}

	return nil
}
