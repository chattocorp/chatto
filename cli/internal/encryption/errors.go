package encryption

import (
	"errors"

	"hmans.de/chatto/pkg/datacrypto"
)

var (
	// ErrDecryptionFailed indicates the ciphertext couldn't be decrypted
	// (wrong key, corrupted data, or tampered ciphertext).
	ErrDecryptionFailed = datacrypto.ErrDecryptionFailed

	// ErrKeyNotFound indicates no encryption key exists for the requested entity.
	ErrKeyNotFound = errors.New("encryption key not found")

	// ErrInvalidKeySize indicates the provided key has an incorrect size.
	ErrInvalidKeySize = datacrypto.ErrInvalidKeySize

	// ErrInvalidNonceSize indicates the provided nonce has an incorrect size.
	ErrInvalidNonceSize = datacrypto.ErrInvalidNonceSize
)
