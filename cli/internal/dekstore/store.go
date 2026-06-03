// Package dekstore stores Chatto-owned data-encryption-key records.
package dekstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/log"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/encryption"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	refAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	refLength   = 24
)

type Reader interface {
	Get(ctx context.Context, ref string) (*corev1.StoredUserDEK, error)
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

func (s *Store) Create(ctx context.Context, dek *corev1.StoredUserDEK) (string, error) {
	if dek == nil || len(dek.GetEncryptedContentKey()) == 0 || len(dek.GetContentKeyNonce()) == 0 || dek.GetWrappingKeyRef() == "" {
		return "", fmt.Errorf("invalid stored DEK")
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

func (s *Store) Get(ctx context.Context, ref string) (*corev1.StoredUserDEK, error) {
	entry, err := s.kv.Get(ctx, ref)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, encryption.ErrKeyNotFound
		}
		return nil, fmt.Errorf("failed to get content key: %w", err)
	}
	var dek corev1.StoredUserDEK
	if err := proto.Unmarshal(entry.Value(), &dek); err != nil {
		return nil, fmt.Errorf("failed to decode content key: %w", err)
	}
	if dek.GetWrappingKeyRef() == "" {
		return nil, encryption.ErrKeyNotFound
	}
	return &dek, nil
}

func (s *Store) Shred(ctx context.Context, ref string) error {
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
