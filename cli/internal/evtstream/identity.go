package evtstream

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	// IdentityMetadataKey stores Chatto's durable EVT stream incarnation.
	IdentityMetadataKey = "chatto.evt.incarnation"
	identityPrefix      = "evt-incarnation-v1:"
)

// NewIdentity deterministically derives Chatto's identity for one EVT stream
// incarnation. created is used only when initializing missing metadata.
func NewIdentity(created time.Time) (string, error) {
	if created.IsZero() {
		return "", fmt.Errorf("EVT stream creation time is required")
	}
	sum := sha256.Sum256([]byte("chatto/evt-incarnation/v1\x00" + created.UTC().Format(time.RFC3339Nano)))
	return identityPrefix + hex.EncodeToString(sum[:16]), nil
}

// ValidIdentity reports whether identity has Chatto's versioned EVT
// stream-incarnation format.
func ValidIdentity(identity string) bool {
	if len(identity) != len(identityPrefix)+32 || !strings.HasPrefix(identity, identityPrefix) {
		return false
	}
	_, err := hex.DecodeString(identity[len(identityPrefix):])
	return err == nil
}

// Identity reads the durable Chatto EVT incarnation cached when the stream was
// opened. Unlike StreamInfo.Created, it survives backup and restore.
func Identity(stream jetstream.Stream) (string, error) {
	if stream == nil {
		return "", fmt.Errorf("EVT stream is required")
	}
	return IdentityFromInfo(stream.CachedInfo())
}

// IdentityFromInfo resolves and validates Chatto's EVT incarnation from one
// StreamInfo snapshot so callers can bind it to the same sequence bounds.
func IdentityFromInfo(info *jetstream.StreamInfo) (string, error) {
	if info == nil {
		return "", fmt.Errorf("EVT stream info is unavailable")
	}
	identity := info.Config.Metadata[IdentityMetadataKey]
	if !ValidIdentity(identity) {
		return "", fmt.Errorf("EVT stream identity is missing or invalid")
	}
	return identity, nil
}
