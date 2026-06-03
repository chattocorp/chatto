// Package kms defines Chatto's key-wrapping boundary.
package kms

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/encryption"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	// AlgorithmBuiltinXChaCha20Poly1305V1 identifies the built-in in-process
	// wrapper used for protobuf UserKeyEncryptionKey records in ENCRYPTION_KEYS.
	AlgorithmBuiltinXChaCha20Poly1305V1 = "builtin-xchacha20-poly1305-v1"
)

var (
	ErrInvalidKeyRef                = errors.New("invalid key ref")
	ErrUnsupportedWrappingAlgorithm = errors.New("unsupported content key wrapping algorithm")
)

const (
	keyRefAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	keyRefLength   = 24
)

// WrappedContentKey is the opaque wrapped-key material returned by a KMS.
type WrappedContentKey struct {
	EncryptedContentKey []byte
	Nonce               []byte
	Algorithm           string
	Metadata            []byte
}

// KeyWrapper is the key-only KMS boundary used by Chatto core.
type KeyWrapper interface {
	CreateKey(ctx context.Context, owner string) (string, error)
	KeyExists(ctx context.Context, keyRef string) (bool, error)
	WrapContentKey(ctx context.Context, keyRef string, contentKey, aad []byte) (*WrappedContentKey, error)
	UnwrapContentKey(ctx context.Context, keyRef string, wrapped WrappedContentKey, aad []byte) ([]byte, error)
	ShredKey(ctx context.Context, keyRef string) error
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

func LegacyUserKeyRef(userID string) string {
	return "user." + userID
}

func IsKeyRef(keyRef string) bool {
	return strings.HasPrefix(keyRef, "kek.") || strings.HasPrefix(keyRef, "user.")
}

func ValidateKeyRef(keyRef string) error {
	if IsKeyRef(keyRef) {
		return nil
	}
	return fmt.Errorf("%w for KMS operation: %s", ErrInvalidKeyRef, keyRef)
}

func keyPath(keyRef string) string {
	return keyRef
}

func encodeUserKeyEncryptionKey(key []byte) ([]byte, error) {
	if len(key) != encryption.KeySize {
		return nil, fmt.Errorf("invalid KEK length: got %d, want %d", len(key), encryption.KeySize)
	}
	data, err := proto.Marshal(&corev1.UserKeyEncryptionKey{
		Key:       key,
		Algorithm: AlgorithmBuiltinXChaCha20Poly1305V1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode encryption key: %w", err)
	}
	return data, nil
}

func DecodeUserKeyEncryptionKeyRecord(keyRef string, data []byte) ([]byte, error) {
	if err := ValidateKeyRef(keyRef); err != nil {
		return nil, err
	}
	if strings.HasPrefix(keyRef, "user.") {
		if len(data) == encryption.KeySize {
			return append([]byte(nil), data...), nil
		}
		return nil, fmt.Errorf("invalid legacy user key record")
	}
	var stored corev1.UserKeyEncryptionKey
	if err := proto.Unmarshal(data, &stored); err == nil &&
		stored.GetAlgorithm() == AlgorithmBuiltinXChaCha20Poly1305V1 &&
		len(stored.GetKey()) == encryption.KeySize {
		return append([]byte(nil), stored.GetKey()...), nil
	}
	if len(data) == encryption.KeySize {
		return append([]byte(nil), data...), nil
	}
	if stored.GetAlgorithm() != "" && stored.GetAlgorithm() != AlgorithmBuiltinXChaCha20Poly1305V1 {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedWrappingAlgorithm, stored.GetAlgorithm())
	}
	if len(stored.GetKey()) > 0 && len(stored.GetKey()) != encryption.KeySize {
		return nil, fmt.Errorf("invalid key-encryption-key length: got %d, want %d", len(stored.GetKey()), encryption.KeySize)
	}
	return nil, fmt.Errorf("invalid key-encryption-key record")
}

func ValidateUserKeyEncryptionKeyRecord(keyRef string, data []byte) error {
	_, err := DecodeUserKeyEncryptionKeyRecord(keyRef, data)
	return err
}

func (b *Builtin) getKey(ctx context.Context, keyRef string) ([]byte, error) {
	if err := ValidateKeyRef(keyRef); err != nil {
		return nil, err
	}
	entry, err := b.kv.Get(ctx, keyPath(keyRef))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}
	key, err := DecodeUserKeyEncryptionKeyRecord(keyRef, entry.Value())
	if err != nil {
		return nil, fmt.Errorf("failed to decode encryption key %s: %w", keyRef, err)
	}
	return key, nil
}

// LegacyUserKey returns a raw KEK for legacy direct-key body decrypt only.
func (b *Builtin) LegacyUserKey(ctx context.Context, userID string) ([]byte, error) {
	return b.getKey(ctx, LegacyUserKeyRef(userID))
}

func newKeyRef() (string, error) {
	id, err := gonanoid.Generate(keyRefAlphabet, keyRefLength)
	if err != nil {
		return "", err
	}
	return "kek." + id, nil
}

// CreateKey generates and stores a new KEK, returning its opaque KMS key ref.
func (b *Builtin) CreateKey(ctx context.Context, owner string) (string, error) {
	key, err := encryption.GenerateKey()
	if err != nil {
		return "", err
	}
	for attempt := 0; attempt < 5; attempt++ {
		keyRef, err := newKeyRef()
		if err != nil {
			return "", err
		}
		data, err := encodeUserKeyEncryptionKey(key)
		if err != nil {
			return "", err
		}
		if _, err := b.kv.Create(ctx, keyPath(keyRef), data); err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				continue
			}
			return "", fmt.Errorf("failed to store encryption key: %w", err)
		}
		b.logger.Info("created encryption key", "key_ref", keyRef, "owner", owner)
		return keyRef, nil
	}
	return "", fmt.Errorf("failed to allocate unique encryption key ref")
}

// KeyExists checks if a KEK exists.
func (b *Builtin) KeyExists(ctx context.Context, keyRef string) (bool, error) {
	if err := ValidateKeyRef(keyRef); err != nil {
		return false, err
	}
	_, err := b.kv.Get(ctx, keyPath(keyRef))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// WrapContentKey wraps a content key with the referenced built-in KEK.
func (b *Builtin) WrapContentKey(ctx context.Context, keyRef string, contentKey, aad []byte) (*WrappedContentKey, error) {
	kek, err := b.getKey(ctx, keyRef)
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

// UnwrapContentKey unwraps a content key with the referenced built-in KEK.
func (b *Builtin) UnwrapContentKey(ctx context.Context, keyRef string, wrapped WrappedContentKey, aad []byte) ([]byte, error) {
	if wrapped.Algorithm != "" && wrapped.Algorithm != AlgorithmBuiltinXChaCha20Poly1305V1 {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedWrappingAlgorithm, wrapped.Algorithm)
	}
	kek, err := b.getKey(ctx, keyRef)
	if err != nil {
		return nil, err
	}
	if kek == nil {
		return nil, encryption.ErrKeyNotFound
	}
	return encryption.UnwrapContentKey(kek, wrapped.EncryptedContentKey, wrapped.Nonce, aad)
}

// ShredKey permanently removes a KEK.
func (b *Builtin) ShredKey(ctx context.Context, keyRef string) error {
	if err := ValidateKeyRef(keyRef); err != nil {
		return err
	}
	err := b.kv.Purge(ctx, keyPath(keyRef))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete encryption key: %w", err)
	}
	b.logger.Info("shredded encryption key", "key_ref", keyRef)
	return nil
}
