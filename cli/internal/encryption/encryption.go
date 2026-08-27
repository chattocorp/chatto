// Package encryption provides server-side encryption for message bodies,
// including legacy direct-key encryption and the v2 content-key envelope.
package encryption

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
	"hmans.de/chatto/pkg/datacrypto"
)

const (
	// KeySize is the size of ChaCha20-Poly1305 keys (256 bits).
	KeySize = datacrypto.KeySize // 32 bytes

	// NonceSize is the size of the nonce (96 bits).
	NonceSize = chacha20poly1305.NonceSize // 12 bytes

	// XNonceSize is the size of the XChaCha20-Poly1305 nonce (192 bits).
	XNonceSize = datacrypto.NonceSize // 24 bytes

	// EnvelopeVersionV2 identifies the content-key epoch message body format.
	EnvelopeVersionV2 int32 = 2

	// AlgorithmEnvelopeV2 identifies the algorithm implied by v2 envelopes.
	// It is kept as a code-level constant rather than stored per message.
	AlgorithmEnvelopeV2 = "xchacha20-poly1305+content-key-epoch-v1"
)

// EncryptedData holds the result of an encryption operation.
type EncryptedData struct {
	Ciphertext []byte
	Nonce      []byte
}

// WrappedContentKey holds a content key encrypted with a per-user KEK.
type WrappedContentKey struct {
	EncryptedContentKey []byte
	Nonce               []byte
}

// Encrypt encrypts plaintext using ChaCha20-Poly1305 AEAD.
// Returns the ciphertext (with auth tag) and nonce, or an error.
func Encrypt(key, plaintext []byte) (*EncryptedData, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("%w: expected %d, got %d", ErrInvalidKeySize, KeySize, len(key))
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AEAD cipher: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt with AEAD (authenticates ciphertext)
	ciphertext := aead.Seal(nil, nonce, plaintext, nil)

	return &EncryptedData{
		Ciphertext: ciphertext,
		Nonce:      nonce,
	}, nil
}

// Decrypt decrypts ciphertext using ChaCha20-Poly1305 AEAD.
// Returns the plaintext or an error (including authentication failure).
func Decrypt(key, ciphertext, nonce []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("%w: expected %d, got %d", ErrInvalidKeySize, KeySize, len(key))
	}
	if len(nonce) != NonceSize {
		return nil, fmt.Errorf("%w: expected %d, got %d", ErrInvalidNonceSize, NonceSize, len(nonce))
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AEAD cipher: %w", err)
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// WrapContentKey encrypts a content key with a key encryption key.
func WrapContentKey(kek, contentKey, aad []byte) (*WrappedContentKey, error) {
	wrapped, err := datacrypto.WrapKey(kek, contentKey, aadForContentKey(aad))
	if err != nil {
		return nil, err
	}
	return &WrappedContentKey{EncryptedContentKey: wrapped.Ciphertext, Nonce: wrapped.Nonce}, nil
}

// UnwrapContentKey decrypts a content key with a key encryption key.
func UnwrapContentKey(kek, encryptedContentKey, nonce, aad []byte) ([]byte, error) {
	return datacrypto.UnwrapKey(kek, encryptedContentKey, nonce, aadForContentKey(aad))
}

// EncryptXChaCha20Poly1305 encrypts plaintext with XChaCha20-Poly1305.
// aad is authenticated as supplied by the caller.
func EncryptXChaCha20Poly1305(key, plaintext, aad []byte) (*EncryptedData, error) {
	sealed, err := datacrypto.Seal(key, plaintext, aad)
	if err != nil {
		return nil, err
	}
	return &EncryptedData{Ciphertext: sealed.Ciphertext, Nonce: sealed.Nonce}, nil
}

// DecryptXChaCha20Poly1305 decrypts ciphertext with XChaCha20-Poly1305.
// aad must match the encryption context exactly.
func DecryptXChaCha20Poly1305(key, ciphertext, nonce, aad []byte) ([]byte, error) {
	return datacrypto.Open(key, ciphertext, nonce, aad)
}

// EncryptWithContentKey encrypts plaintext with an already-selected content
// key. aad must be supplied unchanged for decryption.
func EncryptWithContentKey(contentKey, plaintext, aad []byte) (*EncryptedData, error) {
	return EncryptXChaCha20Poly1305(contentKey, plaintext, aadForBody(aad))
}

// DecryptWithContentKey decrypts a v2 message body with a content key.
func DecryptWithContentKey(contentKey, ciphertext, nonce, aad []byte) ([]byte, error) {
	return DecryptXChaCha20Poly1305(contentKey, ciphertext, nonce, aadForBody(aad))
}

// GenerateKey generates a cryptographically secure random key.
func GenerateKey() ([]byte, error) {
	key, err := datacrypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	return key, nil
}

func randomBytes(size int) ([]byte, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func aadForBody(aad []byte) []byte {
	return scopedAAD("chatto:message-body:v2", aad)
}

func aadForContentKey(aad []byte) []byte {
	return scopedAAD("chatto:content-key:v2", aad)
}

func scopedAAD(scope string, aad []byte) []byte {
	out := make([]byte, 0, len(scope)+1+len(aad))
	out = append(out, scope...)
	out = append(out, 0)
	out = append(out, aad...)
	return out
}
