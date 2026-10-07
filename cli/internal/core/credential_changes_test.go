package core

import (
	"testing"
	"time"

	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/config"
)

// Session changes on another replica must wake only the registered credential
// keys. Watchers retain no credential values and share the process lifecycle.
func TestCredentialChangesAcrossReplicas(t *testing.T) {
	t.Parallel()
	first, nc := setupTestCore(t)
	ctx := testContext(t)
	user, err := first.CreateUser(ctx, SystemActorID, "credential-watch", "Credential Watch", "password123")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewChattoCore(ctx, nc, config.CoreConfig{SecretKey: "test-core-secret", Assets: config.AssetsConfig{SigningSecret: "test-signing-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	startCoreServices(t, second)
	for _, kind := range []authctx.RuntimeCredentialKind{authctx.RuntimeCredentialKindCookieSession, authctx.RuntimeCredentialKindBearerToken} {
		t.Run(string(kind), func(t *testing.T) {
			var handle string
			var set func(bool) (time.Time, error)
			if kind == authctx.RuntimeCredentialKindCookieSession {
				handle, _, err = first.CreateCookieSession(ctx, user.Id, "test")
				set = func(active bool) (time.Time, error) { return second.SetCookiePrivilegedMode(ctx, handle, active) }
			} else {
				var credentials BearerSessionCredentials
				credentials, err = first.CreateBearerSessionWithSource(ctx, user.Id, "test")
				handle = credentials.AccessToken
				set = func(active bool) (time.Time, error) { return second.SetBearerPrivilegedMode(ctx, handle, active) }
			}
			if err != nil {
				t.Fatal(err)
			}
			changed, stop, err := first.WatchRuntimeCredentialChanges(ctx, authctx.RuntimeCredential{Kind: kind, UserID: user.Id, Handle: handle})
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			if _, err := set(true); err != nil {
				t.Fatal(err)
			}
			select {
			case <-changed:
			case <-time.After(2 * time.Second):
				t.Fatal("remote session change was lost")
			}
			first.credentialChanges.mu.Lock()
			for key, listeners := range first.credentialChanges.listeners {
				if len(listeners) != 1 {
					t.Errorf("listener count for credential key = %d", len(listeners))
				}
				if key != first.authTokenKey(handle) && kind == authctx.RuntimeCredentialKindCookieSession {
					t.Error("cookie listener registered another session")
				}
			}
			first.credentialChanges.mu.Unlock()
			stop()
			stop()
			first.credentialChanges.mu.Lock()
			count := len(first.credentialChanges.listeners)
			first.credentialChanges.mu.Unlock()
			if count != 0 {
				t.Fatalf("listener keys retained after stop = %d", count)
			}
		})
	}
}

func TestCredentialChangesDoNotWatchBotKeys(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	changed, stop, err := c.WatchRuntimeCredentialChanges(testContext(t), authctx.RuntimeCredential{Kind: authctx.RuntimeCredentialKindBotAPIKey, UserID: "bot", Handle: "key-id"})
	if err != nil || changed != nil || stop == nil {
		t.Fatalf("bot watcher = %v, %v", changed, err)
	}
	stop()
}
