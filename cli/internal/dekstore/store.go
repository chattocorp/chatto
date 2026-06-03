// Package dekstore stores Chatto-owned data-encryption-key records.
package dekstore

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
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	refAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	refLength   = 24
)

var ErrInvalidRef = errors.New("invalid data-encryption-key ref")

type Reader interface {
	Get(ctx context.Context, ref string) (*corev1.UserDataEncryptionKey, error)
}

type Store struct {
	kv     jetstream.KeyValue
	logger *log.Logger
}

func New(kv jetstream.KeyValue, logger *log.Logger) *Store {
	if logger == nil {
		logger = log.WithPrefix("dekstore")
	}
	return &Store{kv: kv, logger: logger}
}

func newRef() (string, error) {
	id, err := gonanoid.Generate(refAlphabet, refLength)
	if err != nil {
		return "", err
	}
	return "dek." + id, nil
}

func IsRef(ref string) bool {
	return strings.HasPrefix(ref, "dek.")
}

func ValidateRef(ref string) error {
	if IsRef(ref) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidRef, ref)
}

func validateUserDataEncryptionKey(dek *corev1.UserDataEncryptionKey) error {
	if dek == nil {
		return fmt.Errorf("invalid user data encryption key")
	}
	if len(dek.GetEncryptedContentKey()) == 0 {
		return fmt.Errorf("invalid user data encryption key: encrypted content key is empty")
	}
	if len(dek.GetContentKeyNonce()) == 0 {
		return fmt.Errorf("invalid user data encryption key: content key nonce is empty")
	}
	if dek.GetWrappingKeyRef() == "" {
		return fmt.Errorf("invalid user data encryption key: wrapping key ref is empty")
	}
	if err := kms.ValidateKeyRef(dek.GetWrappingKeyRef()); err != nil {
		return fmt.Errorf("invalid user data encryption key wrapping key ref: %w", err)
	}
	if dek.GetWrappingAlgorithm() != kms.AlgorithmBuiltinXChaCha20Poly1305V1 {
		if dek.GetWrappingAlgorithm() == "" {
			return fmt.Errorf("invalid user data encryption key: wrapping algorithm is empty")
		}
		return fmt.Errorf("%w: %s", kms.ErrUnsupportedWrappingAlgorithm, dek.GetWrappingAlgorithm())
	}
	return nil
}

func ValidateUserDataEncryptionKeyRecord(ref string, data []byte) error {
	if err := ValidateRef(ref); err != nil {
		return err
	}
	var dek corev1.UserDataEncryptionKey
	if err := proto.Unmarshal(data, &dek); err != nil {
		return fmt.Errorf("failed to decode content key: %w", err)
	}
	return validateUserDataEncryptionKey(&dek)
}

func (s *Store) Create(ctx context.Context, dek *corev1.UserDataEncryptionKey) (string, error) {
	if err := validateUserDataEncryptionKey(dek); err != nil {
		return "", err
	}
	data, err := proto.Marshal(dek)
	if err != nil {
		return "", fmt.Errorf("failed to encode content key: %w", err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		ref, err := newRef()
		if err != nil {
			return "", err
		}
		if _, err := s.kv.Create(ctx, ref, data); err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				continue
			}
			return "", fmt.Errorf("failed to store content key: %w", err)
		}
		s.logger.Info("created content key", "content_key_ref", ref, "wrapping_key_ref", dek.GetWrappingKeyRef())
		return ref, nil
	}
	return "", fmt.Errorf("failed to allocate unique content key ref")
}

func (s *Store) Get(ctx context.Context, ref string) (*corev1.UserDataEncryptionKey, error) {
	if err := ValidateRef(ref); err != nil {
		return nil, err
	}
	entry, err := s.kv.Get(ctx, ref)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, encryption.ErrKeyNotFound
		}
		return nil, fmt.Errorf("failed to get content key: %w", err)
	}
	var dek corev1.UserDataEncryptionKey
	if err := proto.Unmarshal(entry.Value(), &dek); err != nil {
		return nil, fmt.Errorf("failed to decode content key: %w", err)
	}
	if err := validateUserDataEncryptionKey(&dek); err != nil {
		return nil, err
	}
	return &dek, nil
}

func (s *Store) Shred(ctx context.Context, ref string) error {
	if err := ValidateRef(ref); err != nil {
		return err
	}
	err := s.kv.Purge(ctx, ref)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete content key: %w", err)
	}
	s.logger.Info("shredded content key", "content_key_ref", ref)
	return nil
}
