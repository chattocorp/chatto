package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"

	"github.com/nats-io/nats.go/jetstream"
)

// IsKeyAbsent reports that a key-value operation found no live entry. Reads
// report a removal marker as jetstream.ErrKeyNotFound. The
// jetstream.ErrKeyDeleted check is defensive.
func IsKeyAbsent(err error) bool {
	return errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted)
}

// KeyedDigest returns the HMAC-SHA256 of value under key.
func KeyedDigest(key []byte, value string) []byte {
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write([]byte(value))
	return digest.Sum(nil)
}

// DigestKey returns a runtime-state key for secret or personal input: prefix
// followed by the unpadded base64url KeyedDigest of input. prefix includes its
// trailing dot. Stored keys are persisted contracts; never change the prefix,
// the input format, or the encoding of an existing key family.
func DigestKey(prefix string, key []byte, input string) string {
	return prefix + base64.RawURLEncoding.EncodeToString(KeyedDigest(key, input))
}
