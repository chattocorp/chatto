package migrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

// setupTestKV stands up an embedded NATS server with JetStream and
// returns a single KV bucket plus a logger for the migration to work
// against. Mirrors the pattern in cli/internal/core/core_test.go but
// strips out everything except the KV.
func setupTestKV(t *testing.T) (context.Context, jetstream.KeyValue) {
	t.Helper()

	_, nc := testutil.StartNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "TEST",
		Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("create KV: %v", err)
	}

	return ctx, kv
}

// emailHash mirrors the production helper for test setup.
func testEmailHash(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(email)))
	return hex.EncodeToString(sum[:])
}

func TestMigrateVerifiedEmailsToProto_ConvertsLegacyJSONBlob(t *testing.T) {
	ctx, kv := setupTestKV(t)

	type legacy struct {
		Email      string    `json:"email"`
		VerifiedAt time.Time `json:"verified_at"`
	}
	verifiedAt := time.Now().UTC().Truncate(time.Second)
	blob, _ := json.Marshal([]legacy{
		{Email: "Alice@Example.com", VerifiedAt: verifiedAt},
		{Email: "alice-alt@example.com", VerifiedAt: verifiedAt},
	})
	const userID = "user123"
	if _, err := kv.Put(ctx, fmt.Sprintf("user.%s.verified_emails", userID), blob); err != nil {
		t.Fatalf("seed legacy key: %v", err)
	}

	if err := MigrateVerifiedEmailsToProto(ctx, kv, log.New(nil)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Legacy key gone.
	if _, err := kv.Get(ctx, fmt.Sprintf("user.%s.verified_emails", userID)); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Errorf("expected legacy key to be deleted, got err=%v", err)
	}

	// New per-email keys exist with correct proto content. Hash uses
	// lowercase email, so the mixed-case input must resolve correctly.
	for _, email := range []string{"Alice@Example.com", "alice-alt@example.com"} {
		key := fmt.Sprintf("verified_emails.%s.%s", userID, testEmailHash(email))
		entry, err := kv.Get(ctx, key)
		if err != nil {
			t.Errorf("expected new key %s, got err=%v", key, err)
			continue
		}
		ve := &corev1.VerifiedEmail{}
		if err := proto.Unmarshal(entry.Value(), ve); err != nil {
			t.Errorf("unmarshal %s: %v", email, err)
			continue
		}
		if ve.Email != email {
			t.Errorf("email round-trip: want %q, got %q", email, ve.Email)
		}
		if !ve.VerifiedAt.AsTime().Equal(verifiedAt) {
			t.Errorf("verified_at round-trip for %q: want %v, got %v", email, verifiedAt, ve.VerifiedAt.AsTime())
		}
	}
}

func TestMigrateVerifiedEmailsToProto_NoLegacyKeys(t *testing.T) {
	ctx, kv := setupTestKV(t)
	// Empty bucket — must be a no-op without errors.
	if err := MigrateVerifiedEmailsToProto(ctx, kv, log.New(nil)); err != nil {
		t.Fatalf("expected no-op, got %v", err)
	}
}

func TestMigrateVerifiedEmailsToProto_Idempotent(t *testing.T) {
	ctx, kv := setupTestKV(t)

	type legacy struct {
		Email      string    `json:"email"`
		VerifiedAt time.Time `json:"verified_at"`
	}
	blob, _ := json.Marshal([]legacy{
		{Email: "bob@example.com", VerifiedAt: time.Now()},
	})
	if _, err := kv.Put(ctx, "user.bobid.verified_emails", blob); err != nil {
		t.Fatalf("seed: %v", err)
	}

	logger := log.New(nil)

	// First run: converts.
	if err := MigrateVerifiedEmailsToProto(ctx, kv, logger); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Second run: must be a no-op (legacy key already drained).
	if err := MigrateVerifiedEmailsToProto(ctx, kv, logger); err != nil {
		t.Fatalf("second run: %v", err)
	}

	// Per-email entry still present.
	key := fmt.Sprintf("verified_emails.bobid.%s", testEmailHash("bob@example.com"))
	if _, err := kv.Get(ctx, key); err != nil {
		t.Errorf("expected per-email key intact, got err=%v", err)
	}
}
