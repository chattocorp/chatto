package sessions

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
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

func TestSessionRejectsAnOlderAuthenticationVersion(t *testing.T) {
	service, _, cleanup := testService(t)
	defer cleanup()
	version := uint64(3)
	service.authenticationVersion = func(accountID string) (uint64, bool) {
		return version, accountID == "acc_versioned"
	}
	token, created, err := service.Create(t.Context(), "acc_versioned")
	if err != nil {
		t.Fatal(err)
	}
	if created.AuthenticationVersion != version {
		t.Fatalf("authentication version = %d, want %d", created.AuthenticationVersion, version)
	}
	version++
	if _, err := service.Validate(t.Context(), token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("validate older authentication version error = %v, want ErrNotFound", err)
	}
}

func TestGenerationBoundSessionDoesNotUpgradeAcrossCredentialChange(t *testing.T) {
	service, _, cleanup := testService(t)
	defer cleanup()
	version := uint64(4)
	service.authenticationVersion = func(accountID string) (uint64, bool) {
		return version, accountID == "acc_generation_bound"
	}
	if _, created, err := service.CreateAtAuthenticationVersion(t.Context(), "acc_generation_bound", version); err != nil {
		t.Fatal(err)
	} else if created.AuthenticationVersion != version {
		t.Fatalf("authentication version = %d, want %d", created.AuthenticationVersion, version)
	}
	version++
	if _, _, err := service.CreateAtAuthenticationVersion(t.Context(), "acc_generation_bound", version-1); err == nil {
		t.Fatal("generation-bound session silently upgraded across credential change")
	}
}

func TestGenerationBoundSessionRemovesRecordWhenVersionChangesAfterStore(t *testing.T) {
	service, stores, cleanup := testService(t)
	defer cleanup()
	const expected = uint64(7)
	var resolutions int
	service.authenticationVersion = func(accountID string) (uint64, bool) {
		resolutions++
		if resolutions == 1 {
			return expected, accountID == "acc_post_store_race"
		}
		return expected + 1, accountID == "acc_post_store_race"
	}
	if _, _, err := service.CreateAtAuthenticationVersion(t.Context(), "acc_post_store_race", expected); err == nil {
		t.Fatal("generation-bound session survived a post-store credential change")
	}
	keys, err := stores.RuntimeState.Keys(t.Context())
	if err != nil && !errors.Is(err, jetstream.ErrNoKeysFound) {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatalf("runtime session keys after rejected creation = %v, want none", keys)
	}
}

func TestSessionInventoryListsAndRevokesOnlyAuthorizedOtherSession(t *testing.T) {
	service, _, _, _, cleanup := testInventoryService(t)
	defer cleanup()
	stopInventory := startTestInventory(t, service)
	defer stopInventory()

	currentToken, _, err := service.Create(t.Context(), "acc_owner")
	if err != nil {
		t.Fatal(err)
	}
	otherToken, _, err := service.Create(t.Context(), "acc_owner")
	if err != nil {
		t.Fatal(err)
	}
	foreignToken, _, err := service.Create(t.Context(), "acc_foreign")
	if err != nil {
		t.Fatal(err)
	}

	ownerSessions, err := service.List(t.Context(), "acc_owner", currentToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(ownerSessions) != 2 || !ownerSessions[0].Current || ownerSessions[1].Current {
		t.Fatalf("owner sessions = %+v, want current first and one other", ownerSessions)
	}
	if !validSessionID(ownerSessions[0].ID) || ownerSessions[0].ID == ownerSessions[1].ID || strings.Contains(ownerSessions[0].ID, service.sessionKey(currentToken)) {
		t.Fatalf("opaque session IDs = %q, %q", ownerSessions[0].ID, ownerSessions[1].ID)
	}

	foreignSessions, err := service.List(t.Context(), "acc_foreign", foreignToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeSession(t.Context(), "acc_owner", foreignSessions[0].ID, currentToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-account revocation error = %v, want ErrNotFound", err)
	}
	if _, err := service.Validate(t.Context(), foreignToken); err != nil {
		t.Fatalf("foreign session was revoked: %v", err)
	}
	if err := service.RevokeSession(t.Context(), "acc_owner", ownerSessions[0].ID, currentToken); !errors.Is(err, ErrCurrentSession) {
		t.Fatalf("current-session revocation error = %v, want ErrCurrentSession", err)
	}
	if err := service.RevokeSession(t.Context(), "acc_owner", ownerSessions[1].ID, currentToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Validate(t.Context(), otherToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked other session validation error = %v, want ErrNotFound", err)
	}
	if _, err := service.Validate(t.Context(), currentToken); err != nil {
		t.Fatalf("current session was revoked: %v", err)
	}
	if sessions, err := service.List(t.Context(), "acc_owner", currentToken); err != nil || len(sessions) != 1 || !sessions[0].Current {
		t.Fatalf("sessions after revocation = %+v, %v", sessions, err)
	}
}

func TestSessionInventoryRevokesAllOthersAndRebuildsAfterRestart(t *testing.T) {
	first, stores, js, key, cleanup := testInventoryService(t)
	defer cleanup()
	stopFirst := startTestInventory(t, first)

	currentToken, _, err := first.Create(t.Context(), "acc_restart")
	if err != nil {
		t.Fatal(err)
	}
	otherTokens := make([]string, 2)
	for i := range otherTokens {
		otherTokens[i], _, err = first.Create(t.Context(), "acc_restart")
		if err != nil {
			t.Fatal(err)
		}
	}
	beforeRestart, err := first.List(t.Context(), "acc_restart", currentToken)
	if err != nil {
		t.Fatal(err)
	}
	stopFirst()

	restarted := New(stores.RuntimeState, js, key, nil)
	stopRestarted := startTestInventory(t, restarted)
	defer stopRestarted()
	afterRestart, err := restarted.List(t.Context(), "acc_restart", currentToken)
	if err != nil || len(afterRestart) != 3 {
		t.Fatalf("restarted inventory sessions = %+v, %v, want three", afterRestart, err)
	}
	beforeIDs := make(map[string]struct{}, len(beforeRestart))
	for _, session := range beforeRestart {
		beforeIDs[session.ID] = struct{}{}
	}
	for _, session := range afterRestart {
		if _, ok := beforeIDs[session.ID]; !ok {
			t.Fatalf("session ID %q changed across restart", session.ID)
		}
	}
	revoked, err := restarted.RevokeOtherSessions(t.Context(), "acc_restart", currentToken)
	if err != nil || revoked != 2 {
		t.Fatalf("revoke others = %d, %v, want 2, nil", revoked, err)
	}
	for _, token := range otherTokens {
		if _, err := restarted.Validate(t.Context(), token); !errors.Is(err, ErrNotFound) {
			t.Fatalf("other session survived bulk revocation: %v", err)
		}
	}
	if sessions, err := restarted.List(t.Context(), "acc_restart", currentToken); err != nil || len(sessions) != 1 || !sessions[0].Current {
		t.Fatalf("sessions after bulk revocation = %+v, %v", sessions, err)
	}
}

func TestSessionInventoryOmitsMalformedAndStaleAuthenticationVersions(t *testing.T) {
	service, stores, _, _, cleanup := testInventoryService(t)
	defer cleanup()
	version := uint64(1)
	service.authenticationVersion = func(accountID string) (uint64, bool) {
		return version, accountID == "acc_filtered"
	}
	if _, err := stores.RuntimeState.Put(t.Context(), "session.malformed", []byte("not encrypted session state")); err != nil {
		t.Fatal(err)
	}
	stopInventory := startTestInventory(t, service)
	defer stopInventory()

	staleToken, _, err := service.Create(t.Context(), "acc_filtered")
	if err != nil {
		t.Fatal(err)
	}
	version++
	currentToken, _, err := service.Create(t.Context(), "acc_filtered")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := service.List(t.Context(), "acc_filtered", currentToken)
	if err != nil || len(sessions) != 1 || !sessions[0].Current {
		t.Fatalf("filtered sessions = %+v, %v, want only current generation", sessions, err)
	}
	if _, err := service.Validate(t.Context(), staleToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale session validation error = %v, want ErrNotFound", err)
	}
}

func TestSessionInventoryRemovesRuntimeTTLExpirations(t *testing.T) {
	service, stores, _, _, cleanup := testInventoryService(t)
	defer cleanup()
	stopInventory := startTestInventory(t, service)
	defer stopInventory()

	currentToken, current, err := service.Create(t.Context(), "acc_expiry")
	if err != nil {
		t.Fatal(err)
	}
	expiringToken := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, tokenBytes))
	expiringKey := service.sessionKey(expiringToken)
	expiringState := current
	expiringState.LastSeenAt = current.CreatedAt.Add(time.Nanosecond)
	data, err := service.seal(expiringKey, expiringState)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stores.RuntimeState.Create(t.Context(), expiringKey, data, jetstream.KeyTTL(time.Second)); err != nil {
		t.Fatal(err)
	}

	deadline, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	waitForCount := func(want int) {
		t.Helper()
		for {
			sessions, listErr := service.List(deadline, "acc_expiry", currentToken)
			if listErr == nil && len(sessions) == want {
				return
			}
			select {
			case <-deadline.Done():
				t.Fatalf("session count did not reach %d: sessions=%+v error=%v", want, sessions, listErr)
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	waitForCount(2)
	waitForCount(1)
}

func TestSessionInventoryReflectsWritesFromAnotherService(t *testing.T) {
	indexed, stores, js, key, cleanup := testInventoryService(t)
	defer cleanup()
	stopInventory := startTestInventory(t, indexed)
	defer stopInventory()
	writer := New(stores.RuntimeState, js, key, nil)

	currentToken, _, err := writer.Create(t.Context(), "acc_replicated")
	if err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		sessions, listErr := indexed.List(deadline, "acc_replicated", currentToken)
		if listErr == nil && len(sessions) == 1 {
			break
		}
		select {
		case <-deadline.Done():
			t.Fatalf("replicated create did not reach inventory: %v", deadline.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := writer.Revoke(t.Context(), currentToken); err != nil {
		t.Fatal(err)
	}
	for {
		sessions, listErr := indexed.List(deadline, "acc_replicated", currentToken)
		if errors.Is(listErr, ErrNotFound) && len(sessions) == 0 {
			break
		}
		select {
		case <-deadline.Done():
			t.Fatalf("replicated revoke did not reach inventory: %v", deadline.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func testInventoryService(t *testing.T) (*Service, storage.Stores, jetstream.JetStream, []byte, func()) {
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
	service := New(stores.RuntimeState, js, key, nil)
	return service, stores, js, key, func() {
		clear(key)
		cancel()
		if err := connection.Close(); err != nil {
			t.Errorf("close NATS: %v", err)
		}
	}
}

func startTestInventory(t *testing.T, service *Service) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	runErrors := make(chan error, 1)
	go func() { runErrors <- service.RunInventory(ctx) }()
	readyContext, readyCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer readyCancel()
	if err := service.WaitForInventoryStartup(readyContext); err != nil {
		cancel()
		<-runErrors
		t.Fatalf("wait for inventory startup: %v", err)
	}
	stopped := false
	return func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		select {
		case err := <-runErrors:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("inventory shutdown error = %v, want context cancellation", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("session inventory did not stop")
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
	service := New(stores.RuntimeState, js, key, nil)
	clear(key)
	return service, stores, func() {
		cancel()
		if err := connection.Close(); err != nil {
			t.Errorf("close NATS: %v", err)
		}
	}
}
