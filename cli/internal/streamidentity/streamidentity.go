// Package streamidentity provides Chatto's versioned stream-incarnation
// identity format, shared by the durable EVT stream and other incarnation-bound
// streams such as NOTIFICATIONS.
//
// The identity is a prefixed, hex-encoded SHA-256 digest derived from the
// stream's creation time. It is persisted in stream metadata so it survives
// backup and restore, unlike StreamInfo.Created. Each stream declares its own
// metadata key, prefix, and domain string; this package owns only the common
// derivation, format validation, and lookup mechanics.
package streamidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Identity describes one stream's incarnation scheme: where the identity is
// cached in JetStream metadata, how the encoded value is recognized, and which
// domain string feeds the digest. Values are immutable after definition.
type Identity struct {
	// MetadataKey is the stream-config metadata key that caches the identity.
	MetadataKey string
	// Prefix marks the identity format generation inside the encoded value.
	Prefix string
	// Domain names the identity inside the digest input, e.g. "chatto/evt-incarnation/v1".
	Domain string
	// Label is used verbatim in error messages, e.g. "EVT stream".
	Label string
}

// New deterministically derives the stream identity for one incarnation.
// created is used only when initializing missing metadata.
func (i Identity) New(created time.Time) (string, error) {
	if created.IsZero() {
		return "", fmt.Errorf("%s creation time is required", i.Label)
	}
	sum := sha256.Sum256([]byte(i.Domain + "\x00" + created.UTC().Format(time.RFC3339Nano)))
	return i.Prefix + hex.EncodeToString(sum[:16]), nil
}

// Valid reports whether identity has the expected versioned identity format.
func (i Identity) Valid(identity string) bool {
	if len(identity) != len(i.Prefix)+32 || !strings.HasPrefix(identity, i.Prefix) {
		return false
	}
	_, err := hex.DecodeString(identity[len(i.Prefix):])
	return err == nil
}

// FromInfo resolves and validates the identity from one StreamInfo snapshot so
// callers can bind it to the same sequence bounds.
func (i Identity) FromInfo(info *jetstream.StreamInfo) (string, error) {
	if info == nil {
		return "", fmt.Errorf("%s info is unavailable", i.Label)
	}
	identity := info.Config.Metadata[i.MetadataKey]
	if !i.Valid(identity) {
		return "", fmt.Errorf("%s identity is missing or invalid", i.Label)
	}
	return identity, nil
}
