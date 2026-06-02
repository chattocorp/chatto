// Package kms defines Chatto's key-wrapping boundary.
package kms

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/encryption"
)

const (
	// AlgorithmBuiltinXChaCha20Poly1305V1 identifies the built-in in-process
	// wrapper that stores raw per-user KEKs in the ENCRYPTION_KEYS KV bucket.
	AlgorithmBuiltinXChaCha20Poly1305V1 = "builtin-xchacha20-poly1305-v1"
)

var ErrUnsupportedWrappingAlgorithm = errors.New("unsupported content key wrapping algorithm")

// WrappedContentKey is the opaque wrapped-key material returned by a KMS.
type WrappedContentKey struct {
	EncryptedContentKey []byte
	Nonce               []byte
	Algorithm           string
	Metadata            []byte
}

// KeyWrapper is the key-only KMS boundary used by Chatto core.
type KeyWrapper interface {
	CreateUserKey(ctx context.Context, userID string) error
	UserKeyExists(ctx context.Context, userID string) (bool, error)
	WrapContentKey(ctx context.Context, userID string, contentKey, aad []byte) (*WrappedContentKey, error)
	UnwrapContentKey(ctx context.Context, userID string, wrapped WrappedContentKey, aad []byte) ([]byte, error)
	ShredUserKey(ctx context.Context, userID string) error
}

// LegacyKeyProvider exposes raw local KEKs only for decrypting pre-envelope
// message bodies. New code should use KeyWrapper instead.
type LegacyKeyProvider interface {
	LegacyUserKey(ctx context.Context, userID string) ([]byte, error)
}

// Builtin is Chatto's default in-process KMS.
type Builtin struct {
	kv     jetstream.KeyValue
	logger *log.Logger
}

var _ KeyWrapper = (*Builtin)(nil)
var _ LegacyKeyProvider = (*Builtin)(nil)

// NewBuiltin creates a KV-backed KMS. The KV bucket should be ENCRYPTION_KEYS.
func NewBuiltin(kv jetstream.KeyValue, logger *log.Logger) *Builtin {
	if logger == nil {
		logger = log.WithPrefix("kms.Builtin")
	}
	return &Builtin{kv: kv, logger: logger}
}

func userKeyPath(userID string) string {
	return "user." + userID
}

func (b *Builtin) getUserKey(ctx context.Context, userID string) ([]byte, error) {
	entry, err := b.kv.Get(ctx, userKeyPath(userID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user encryption key: %w", err)
	}
	return append([]byte(nil), entry.Value()...), nil
}

// LegacyUserKey returns a raw KEK for legacy direct-key body decrypt only.
func (b *Builtin) LegacyUserKey(ctx context.Context, userID string) ([]byte, error) {
	return b.getUserKey(ctx, userID)
}

// CreateUserKey generates and stores a new per-user KEK.
func (b *Builtin) CreateUserKey(ctx context.Context, userID string) error {
	key, err := encryption.GenerateKey()
	if err != nil {
		return err
	}
	if _, err := b.kv.Create(ctx, userKeyPath(userID), key); err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return nil
		}
		return fmt.Errorf("failed to store user encryption key: %w", err)
	}
	b.logger.Info("created user encryption key", "user_id", userID)
	return nil
}

// UserKeyExists checks if a user has a KEK.
func (b *Builtin) UserKeyExists(ctx context.Context, userID string) (bool, error) {
	_, err := b.kv.Get(ctx, userKeyPath(userID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// WrapContentKey wraps a content key with the user's built-in KEK.
func (b *Builtin) WrapContentKey(ctx context.Context, userID string, contentKey, aad []byte) (*WrappedContentKey, error) {
	kek, err := b.getUserKey(ctx, userID)
	if err != nil {
		return nil, err
	}
	if kek == nil {
		return nil, encryption.ErrKeyNotFound
	}
	wrapped, err := encryption.WrapContentKey(kek, contentKey, aad)
	if err != nil {
		return nil, err
	}
	return &WrappedContentKey{
		EncryptedContentKey: wrapped.EncryptedContentKey,
		Nonce:               wrapped.Nonce,
		Algorithm:           AlgorithmBuiltinXChaCha20Poly1305V1,
	}, nil
}

// UnwrapContentKey unwraps a content key with the user's built-in KEK.
func (b *Builtin) UnwrapContentKey(ctx context.Context, userID string, wrapped WrappedContentKey, aad []byte) ([]byte, error) {
	if wrapped.Algorithm != "" && wrapped.Algorithm != AlgorithmBuiltinXChaCha20Poly1305V1 {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedWrappingAlgorithm, wrapped.Algorithm)
	}
	kek, err := b.getUserKey(ctx, userID)
	if err != nil {
		return nil, err
	}
	if kek == nil {
		return nil, encryption.ErrKeyNotFound
	}
	return encryption.UnwrapContentKey(kek, wrapped.EncryptedContentKey, wrapped.Nonce, aad)
}

// ShredUserKey permanently removes a user's KEK.
func (b *Builtin) ShredUserKey(ctx context.Context, userID string) error {
	err := b.kv.Delete(ctx, userKeyPath(userID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete user encryption key: %w", err)
	}
	b.logger.Info("shredded user encryption key", "user_id", userID)
	return nil
}
