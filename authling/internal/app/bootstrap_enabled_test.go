//go:build bootstrap

package app

import (
	"errors"
	"testing"

	"github.com/chattocorp/chatto/authling/internal/accounts"
)

func TestDevelopmentBootstrapPreservesAccountsAcrossRestart(t *testing.T) {
	cfg := embeddedTestConfig(t)
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	defer func() { stopTestRuntime(t, runtime, cancel, runErrors) }()
	if err := bootstrapDevelopment(t.Context(), cfg, runtime); err != nil {
		t.Fatal(err)
	}
	if runtime.Accounts.Count() != 0 {
		t.Fatal("disabled bootstrap created accounts")
	}
	cfg.Bootstrap.Enabled = true
	if err := bootstrapDevelopment(t.Context(), cfg, runtime); err != nil {
		t.Fatal(err)
	}
	if runtime.Accounts.Count() != 2 {
		t.Fatal("bootstrap must create exactly two accounts")
	}
	alice, err := runtime.Accounts.AuthenticateLocal(t.Context(), "alice@alice.com", "foobar123")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := runtime.Accounts.AuthenticateLocal(t.Context(), "bob@bob.com", "foobar123")
	if err != nil {
		t.Fatal(err)
	}
	access, err := runtime.Accounts.Administrator(t.Context(), alice.ID, []string{"alice@alice.com"})
	if err != nil || !access.Configured || access.Stored {
		t.Fatalf("Alice override: %v", err)
	}
	access, err = runtime.Accounts.Administrator(t.Context(), bob.ID, []string{"alice@alice.com"})
	if err != nil || access.Allowed() {
		t.Fatalf("Bob must be ordinary: %v", err)
	}
	if _, err := runtime.Accounts.CreateLocal(t.Context(), "ordinary@example.invalid", "foobar123"); !errors.Is(err, accounts.ErrInvalidPassword) {
		t.Fatal("bootstrap weakened normal password policy")
	}
	if _, err := runtime.Authentication.ChangePassword(t.Context(), alice.ID, "foobar123", "a changed development password"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Accounts.UpdateProfile(t.Context(), bob.ID, "edited-bob", "Edited Bob"); err != nil {
		t.Fatal(err)
	}
	before := eventCount(t, runtime)
	if err := bootstrapDevelopment(t.Context(), cfg, runtime); err != nil {
		t.Fatal(err)
	}
	if eventCount(t, runtime) != before {
		t.Fatal("repeat bootstrap appended events")
	}
	stopTestRuntime(t, runtime, cancel, runErrors)
	runtime, cancel, runErrors = startTestRuntime(t, cfg)
	if err := bootstrapDevelopment(t.Context(), cfg, runtime); err != nil {
		t.Fatal(err)
	}
	if eventCount(t, runtime) != before {
		t.Fatal("restart bootstrap appended events")
	}
	current, err := runtime.Accounts.AuthenticateLocal(t.Context(), "alice@alice.com", "a changed development password")
	if err != nil || current.ID != alice.ID {
		t.Fatalf("Alice was replaced: %v", err)
	}
	if _, err := runtime.Accounts.AuthenticateLocal(t.Context(), "alice@alice.com", "foobar123"); !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatal("bootstrap reset Alice password")
	}
	current, err = runtime.Accounts.AuthenticateLocal(t.Context(), "bob@bob.com", "foobar123")
	if err != nil || current.ID != bob.ID {
		t.Fatalf("Bob was replaced: %v", err)
	}
	profile, err := runtime.Accounts.Profile(t.Context(), bob.ID)
	if err != nil || profile.PreferredUsername != "edited-bob" || profile.FullName != "Edited Bob" {
		t.Fatalf("profile was replaced: %v", err)
	}
}

func TestDevelopmentBootstrapPreservesPreexistingAccount(t *testing.T) {
	cfg := embeddedTestConfig(t)
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	existing, err := runtime.Accounts.CreateLocal(t.Context(), "alice@alice.com", "an existing account password")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Bootstrap.Enabled = true
	if err := bootstrapDevelopment(t.Context(), cfg, runtime); err != nil {
		t.Fatal(err)
	}
	current, err := runtime.Accounts.AuthenticateLocal(t.Context(), "alice@alice.com", "an existing account password")
	if err != nil || current.ID != existing.ID || runtime.Accounts.Count() != 2 {
		t.Fatalf("existing account changed: %v", err)
	}
}
