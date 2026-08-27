package evtstream

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestNewIdentityPreservesPersistedFormat(t *testing.T) {
	created := time.Date(2026, time.July, 30, 12, 34, 56, 789, time.UTC)

	first, err := NewIdentity(created)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewIdentity(created)
	if err != nil {
		t.Fatal(err)
	}

	const want = "evt-incarnation-v1:157fc22df9796d187e95310191fc0ebe"
	if first != want || second != want {
		t.Fatalf("NewIdentity() = %q, %q; want persisted-compatible %q", first, second, want)
	}
	if !ValidIdentity(first) {
		t.Fatalf("generated identity %q is invalid", first)
	}
}

func TestValidIdentityRejectsMalformedValues(t *testing.T) {
	for _, identity := range []string{
		"",
		"application-defined",
		"evt-incarnation-v1:",
		"evt-incarnation-v1:gggggggggggggggggggggggggggggggg",
		"evt-incarnation-v2:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	} {
		if ValidIdentity(identity) {
			t.Errorf("ValidIdentity(%q) = true", identity)
		}
	}
}

func TestIdentityFromInfoReadsChattoMetadata(t *testing.T) {
	const want = "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	info := &jetstream.StreamInfo{
		Config: jetstream.StreamConfig{
			Metadata: map[string]string{IdentityMetadataKey: want},
		},
	}

	got, err := IdentityFromInfo(info)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("IdentityFromInfo() = %q, want %q", got, want)
	}
}
