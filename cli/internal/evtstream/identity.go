package evtstream

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/streamidentity"
)

const (
	// IdentityMetadataKey stores Chatto's durable EVT stream incarnation.
	IdentityMetadataKey = "chatto.evt.incarnation"
)

// identityScheme owns the EVT stream's incarnation format: metadata key,
// versioned prefix, digest domain, and error labels.
var identityScheme = streamidentity.Identity{
	MetadataKey: IdentityMetadataKey,
	Prefix:      "evt-incarnation-v1:",
	Domain:      "chatto/evt-incarnation/v1",
	Label:       "EVT stream",
}

// NewIdentity deterministically derives Chatto's identity for one EVT stream
// incarnation. created is used only when initializing missing metadata.
func NewIdentity(created time.Time) (string, error) {
	return identityScheme.New(created)
}

// ValidIdentity reports whether identity has Chatto's versioned EVT
// stream-incarnation format.
func ValidIdentity(identity string) bool {
	return identityScheme.Valid(identity)
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
	return identityScheme.FromInfo(info)
}
