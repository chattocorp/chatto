package datacrypto

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// KeySize is the required size of encryption and wrapping keys in bytes.
	KeySize = chacha20poly1305.KeySize

	// NonceSize is the size of an XChaCha20-Poly1305 nonce in bytes.
	NonceSize = chacha20poly1305.NonceSizeX
)

// SealedData contains an authenticated ciphertext and its random nonce.
// Callers must persist both values and reconstruct the same associated data
// when opening the ciphertext.
type SealedData struct {
	Ciphertext []byte
	Nonce      []byte
}

// GenerateKey returns a cryptographically random 256-bit key.
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	return key, nil
}

// Seal encrypts and authenticates plaintext with XChaCha20-Poly1305. The
// caller owns associated-data construction and domain separation.
func Seal(key, plaintext, associatedData []byte) (*SealedData, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("create XChaCha20-Poly1305 cipher: %w", err)
	}
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	return &SealedData{
		Ciphertext: aead.Seal(nil, nonce, plaintext, associatedData),
		Nonce:      nonce,
	}, nil
}

// Open authenticates and decrypts an XChaCha20-Poly1305 ciphertext. The
// associated data must exactly match the bytes supplied to [Seal].
func Open(key, ciphertext, nonce, associatedData []byte) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	if len(nonce) != NonceSize {
		return nil, fmt.Errorf("%w: expected %d, got %d", ErrInvalidNonceSize, NonceSize, len(nonce))
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("create XChaCha20-Poly1305 cipher: %w", err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, associatedData)
	if err != nil {
		return nil, ErrDecryptionFailed
	}
	return plaintext, nil
}

// WrapKey encrypts and authenticates a 256-bit key with a 256-bit wrapping
// key. The caller owns associated-data construction and domain separation.
func WrapKey(wrappingKey, key, associatedData []byte) (*SealedData, error) {
	if err := validateKey(key); err != nil {
		return nil, fmt.Errorf("wrapped key: %w", err)
	}
	return Seal(wrappingKey, key, associatedData)
}

// UnwrapKey authenticates and decrypts a 256-bit wrapped key.
func UnwrapKey(wrappingKey, ciphertext, nonce, associatedData []byte) ([]byte, error) {
	key, err := Open(wrappingKey, ciphertext, nonce, associatedData)
	if err != nil {
		return nil, err
	}
	if err := validateKey(key); err != nil {
		return nil, fmt.Errorf("wrapped key: %w", err)
	}
	return key, nil
}

func validateKey(key []byte) error {
	if len(key) != KeySize {
		return fmt.Errorf("%w: expected %d, got %d", ErrInvalidKeySize, KeySize, len(key))
	}
	return nil
}
