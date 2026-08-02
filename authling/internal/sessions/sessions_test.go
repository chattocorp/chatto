package sessions

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/natsruntime"
	"hmans.de/authling/internal/storage"
	"hmans.de/chatto/pkg/datacrypto"
)

func TestSessionLifecycleAndEncryptedStorage(t *testing.T) {
	service, stores, cleanup := testService(t)
	defer cleanup()
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	token, created, err := service.Create(t.Context(), "acc_example")
	if err != nil {
		t.Fatal(err)
	}
	if !validToken(token) || created.AccountID != "acc_example" || created.ExpiresAt.Sub(created.CreatedAt) != AbsoluteLifetime {
		t.Fatalf("created session = %+v, token valid = %v", created, validToken(token))
	}
	entry, err := stores.RuntimeState.Get(t.Context(), service.sessionKey(token))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(entry.Value(), []byte("acc_example")) || bytes.Contains(entry.Value(), []byte(token)) {
		t.Fatal("runtime session record exposes account or bearer plaintext")
	}

	now = now.Add(10 * time.Minute)
	validated, err := service.Validate(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if !validated.LastSeenAt.Equal(now) {
		t.Fatalf("last seen = %v, want %v", validated.LastSeenAt, now)
	}

	if err := service.Revoke(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Validate(t.Context(), token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("validate revoked session error = %v, want ErrNotFound", err)
	}
	if err := service.Revoke(t.Context(), token); err != nil {
		t.Fatalf("second revoke: %v", err)
	}
}

func TestSessionEnforcesIdleAndAbsoluteLifetimes(t *testing.T) {
	service, _, cleanup := testService(t)
	defer cleanup()
	started := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	now := started
	service.now = func() time.Time { return now }

	idleToken, _, err := service.Create(t.Context(), "acc_idle")
	if err != nil {
		t.Fatal(err)
	}
	now = started.Add(InactivityLifetime)
	if _, err := service.Validate(t.Context(), idleToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("idle validation error = %v, want ErrNotFound", err)
	}

	now = started
	absoluteToken, _, err := service.Create(t.Context(), "acc_absolute")
	if err != nil {
		t.Fatal(err)
	}
	for now = started.Add(50 * time.Minute); now.Before(started.Add(AbsoluteLifetime)); now = now.Add(50 * time.Minute) {
		if _, err := service.Validate(t.Context(), absoluteToken); err != nil {
			t.Fatalf("validate active session at %v: %v", now, err)
		}
	}
	now = started.Add(AbsoluteLifetime)
	if _, err := service.Validate(t.Context(), absoluteToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absolute validation error = %v, want ErrNotFound", err)
	}
}

func TestInspectDoesNotExtendSessionActivity(t *testing.T) {
	service, _, cleanup := testService(t)
	defer cleanup()
	started := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	now := started
	service.now = func() time.Time { return now }
	token, _, err := service.Create(t.Context(), "acc_passive")
	if err != nil {
		t.Fatal(err)
	}
	now = started.Add(30 * time.Minute)
	inspected, err := service.Inspect(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if !inspected.LastSeenAt.Equal(started) {
		t.Fatalf("passive inspection changed last seen to %v", inspected.LastSeenAt)
	}
	now = started.Add(InactivityLifetime)
	if _, err := service.Inspect(t.Context(), token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("idle passive inspection error = %v, want ErrNotFound", err)
	}
}

func TestSessionRejectsForgedTokensWithoutKVLookup(t *testing.T) {
	service, _, cleanup := testService(t)
	defer cleanup()
	for _, token := range []string{"", "not-base64!", "c2hvcnQ"} {
		if _, err := service.Validate(t.Context(), token); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Validate(%q) error = %v, want ErrNotFound", token, err)
		}
	}
}

func testService(t *testing.T) (*Service, storage.Stores, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	connection, err := natsruntime.Open(ctx, config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	js, err := jetstream.New(connection.NATS)
	if err != nil {
		cancel()
		connection.Close()
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(ctx, js, 1)
	if err != nil {
		cancel()
		connection.Close()
		t.Fatal(err)
	}
	key, err := datacrypto.GenerateKey()
	if err != nil {
		cancel()
		connection.Close()
		t.Fatal(err)
	}
	service := New(stores.RuntimeState, js, key)
	clear(key)
	return service, stores, func() {
		cancel()
		if err := connection.Close(); err != nil {
			t.Errorf("close NATS: %v", err)
		}
	}
}
