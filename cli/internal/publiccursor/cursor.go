// SPDX-License-Identifier: AGPL-3.0-or-later

// Package publiccursor seals internal pagination and replay coordinates before
// they cross a public API boundary.
package publiccursor

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	envelopeVersion = byte(1)
	maxTokenBytes   = 4096
)

var ErrInvalid = errors.New("invalid opaque cursor")

// Seal encrypts and authenticates payload with a purpose-separated key. Scope
// is authenticated but not encoded, allowing callers to bind a token to a
// viewer and resource without disclosing either value.
func Seal(secret, purpose, scope string, payload []byte) (string, error) {
	aead, err := newAEAD(secret, purpose)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate cursor nonce: %w", err)
	}
	envelope := make([]byte, 1, 1+len(nonce)+len(payload)+aead.Overhead())
	envelope[0] = envelopeVersion
	envelope = append(envelope, nonce...)
	envelope = aead.Seal(envelope, nonce, payload, additionalData(purpose, scope))
	return base64.RawURLEncoding.EncodeToString(envelope), nil
}

// Open authenticates and decrypts a token created by Seal.
func Open(secret, purpose, scope, token string) ([]byte, error) {
	if token = strings.TrimSpace(token); token == "" || len(token) > maxTokenBytes {
		return nil, ErrInvalid
	}
	envelope, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, ErrInvalid
	}
	aead, err := newAEAD(secret, purpose)
	if err != nil {
		return nil, err
	}
	if len(envelope) < 1+aead.NonceSize()+aead.Overhead() || envelope[0] != envelopeVersion {
		return nil, ErrInvalid
	}
	nonce := envelope[1 : 1+aead.NonceSize()]
	plaintext, err := aead.Open(nil, nonce, envelope[1+aead.NonceSize():], additionalData(purpose, scope))
	if err != nil {
		return nil, ErrInvalid
	}
	return plaintext, nil
}

func newAEAD(secret, purpose string) (cipherAEAD, error) {
	if secret == "" || purpose == "" {
		return nil, errors.New("cursor secret and purpose are required")
	}
	key := make([]byte, chacha20poly1305.KeySize)
	reader := hkdf.New(sha256.New, []byte(secret), nil, []byte("chatto/public-cursor/"+purpose))
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive cursor key: %w", err)
	}
	return chacha20poly1305.NewX(key)
}

// cipherAEAD keeps the helper independent of a concrete AEAD implementation.
type cipherAEAD interface {
	NonceSize() int
	Overhead() int
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}

func additionalData(purpose, scope string) []byte {
	return []byte("chatto/public-cursor\x00" + purpose + "\x00" + scope)
}
