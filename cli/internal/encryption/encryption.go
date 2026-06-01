// Package encryption provides server-side encryption for message bodies,
// including legacy direct-key encryption and the v2 per-message DEK envelope.
package encryption

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// KeySize is the size of ChaCha20-Poly1305 keys (256 bits).
	KeySize = chacha20poly1305.KeySize // 32 bytes

	// NonceSize is the size of the nonce (96 bits).
	NonceSize = chacha20poly1305.NonceSize // 12 bytes

	// XNonceSize is the size of the XChaCha20-Poly1305 nonce (192 bits).
	XNonceSize = chacha20poly1305.NonceSizeX // 24 bytes

	// EnvelopeVersionV2 identifies the per-message DEK envelope format.
	EnvelopeVersionV2 int32 = 2

	// AlgorithmEnvelopeV2 identifies the algorithm implied by v2 envelopes.
	// It is kept as a code-level constant rather than stored per message.
	AlgorithmEnvelopeV2 = "xchacha20-poly1305+dek-wrap-v1"
)

// EncryptedData holds the result of an encryption operation.
type EncryptedData struct {
	Ciphertext []byte
	Nonce      []byte
}

// Envelope holds a v2 encrypted message body and its wrapped per-message key.
type Envelope struct {
	Version          int32
	Ciphertext       []byte
	Nonce            []byte
	EncryptedDataKey []byte
	DataKeyNonce     []byte
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

// EncryptEnvelope encrypts plaintext with a fresh per-message data encryption
// key, then wraps that DEK with the caller's key encryption key. aad is bound
// to both layers and must be supplied unchanged for decryption.
func EncryptEnvelope(kek, plaintext, aad []byte) (*Envelope, error) {
	if len(kek) != KeySize {
		return nil, fmt.Errorf("%w: expected %d, got %d", ErrInvalidKeySize, KeySize, len(kek))
	}

	dek, err := GenerateKey()
	if err != nil {
		return nil, err
	}

	bodyAEAD, err := chacha20poly1305.NewX(dek)
	if err != nil {
		return nil, fmt.Errorf("failed to create body AEAD cipher: %w", err)
	}
	bodyNonce, err := randomBytes(XNonceSize)
	if err != nil {
		return nil, fmt.Errorf("failed to generate body nonce: %w", err)
	}
	ciphertext := bodyAEAD.Seal(nil, bodyNonce, plaintext, aadForBody(aad))

	wrapAEAD, err := chacha20poly1305.NewX(kek)
	if err != nil {
		return nil, fmt.Errorf("failed to create key wrap AEAD cipher: %w", err)
	}
	dataKeyNonce, err := randomBytes(XNonceSize)
	if err != nil {
		return nil, fmt.Errorf("failed to generate data key nonce: %w", err)
	}
	encryptedDataKey := wrapAEAD.Seal(nil, dataKeyNonce, dek, aadForKeyWrap(aad))

	return &Envelope{
		Version:          EnvelopeVersionV2,
		Ciphertext:       ciphertext,
		Nonce:            bodyNonce,
		EncryptedDataKey: encryptedDataKey,
		DataKeyNonce:     dataKeyNonce,
	}, nil
}

// DecryptEnvelope unwraps the per-message DEK with kek, then decrypts the
// message body. aad must match the encryption context exactly.
func DecryptEnvelope(kek, ciphertext, nonce, encryptedDataKey, dataKeyNonce, aad []byte) ([]byte, error) {
	if len(kek) != KeySize {
		return nil, fmt.Errorf("%w: expected %d, got %d", ErrInvalidKeySize, KeySize, len(kek))
	}
	if len(nonce) != XNonceSize || len(dataKeyNonce) != XNonceSize {
		return nil, fmt.Errorf("%w: expected %d-byte XChaCha nonces", ErrInvalidNonceSize, XNonceSize)
	}

	wrapAEAD, err := chacha20poly1305.NewX(kek)
	if err != nil {
		return nil, fmt.Errorf("failed to create key wrap AEAD cipher: %w", err)
	}
	dek, err := wrapAEAD.Open(nil, dataKeyNonce, encryptedDataKey, aadForKeyWrap(aad))
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	bodyAEAD, err := chacha20poly1305.NewX(dek)
	if err != nil {
		return nil, fmt.Errorf("failed to create body AEAD cipher: %w", err)
	}
	plaintext, err := bodyAEAD.Open(nil, nonce, ciphertext, aadForBody(aad))
	if err != nil {
		return nil, ErrDecryptionFailed
	}
	return plaintext, nil
}

// GenerateKey generates a cryptographically secure random key.
func GenerateKey() ([]byte, error) {
	key, err := randomBytes(KeySize)
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

func aadForKeyWrap(aad []byte) []byte {
	return scopedAAD("chatto:message-dek:v2", aad)
}

func scopedAAD(scope string, aad []byte) []byte {
	out := make([]byte, 0, len(scope)+1+len(aad))
	out = append(out, scope...)
	out = append(out, 0)
	out = append(out, aad...)
	return out
}
