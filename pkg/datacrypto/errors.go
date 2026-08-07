// Package datacrypto provides application-neutral authenticated-encryption
// primitives for protected application data and key wrapping.
package datacrypto

import "errors"

var (
	// ErrDecryptionFailed indicates authentication failed because the key,
	// nonce, ciphertext, or associated data did not match.
	ErrDecryptionFailed = errors.New("decryption failed: invalid key or corrupted data")

	// ErrInvalidKeySize indicates a key was not exactly [KeySize] bytes.
	ErrInvalidKeySize = errors.New("invalid key size")

	// ErrInvalidNonceSize indicates a nonce was not exactly [NonceSize] bytes.
	ErrInvalidNonceSize = errors.New("invalid nonce size")
)
