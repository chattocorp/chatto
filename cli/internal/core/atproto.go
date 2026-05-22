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

// ErrATProtoDIDAlreadyClaimed is returned when an AT Protocol DID is already linked to a different user.
var ErrATProtoDIDAlreadyClaimed = errors.New("AT Protocol DID is already linked to another account")

// userByATProtoDIDKey returns the KV key for the AT Protocol DID-to-user index.
// Hashing keeps the key shape regular and avoids embedding raw DID characters
// (which may include `:` and other non-NATS-friendly characters) in the subject.
func userByATProtoDIDKey(did string) string {
	hash := sha256.Sum256([]byte(did))
	return fmt.Sprintf("user_by_atproto.%s", hex.EncodeToString(hash[:]))
}

// GetUserByATProtoDID looks up a user by their AT Protocol DID.
func (c *ChattoCore) GetUserByATProtoDID(ctx context.Context, did string) (*corev1.User, error) {
	entry, err := c.storage.serverKV.Get(ctx, userByATProtoDIDKey(did))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to lookup user by ATProto DID: %w", err)
	}

	return c.GetUser(ctx, string(entry.Value()))
}

// LinkATProtoDID links an AT Protocol DID to a Chatto user. Atomic; idempotent
// when the same DID is re-linked to the same user.
func (c *ChattoCore) LinkATProtoDID(ctx context.Context, did, userID string) error {
	key := userByATProtoDIDKey(did)

	if _, err := c.storage.serverKV.Create(ctx, key, []byte(userID)); err != nil {
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("failed to link ATProto DID: %w", err)
		}
		entry, getErr := c.storage.serverKV.Get(ctx, key)
		if getErr == nil && string(entry.Value()) == userID {
			return nil
		}
		return ErrATProtoDIDAlreadyClaimed
	}

	return nil
}
