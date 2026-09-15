package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/keyvault"
	"hmans.de/authling/internal/storage"
	"hmans.de/chatto/pkg/datacrypto"
)

// This test uses the pre-codec envelope and primitive directly. It checks both
// directions at actual service boundaries, including each caller's AAD prefix.
func TestRuntimeServicesReadLegacyJSONEnvelopes(t *testing.T) {
	for _, tc := range []struct {
		name, prefix, domain string
		capitalized          bool
	}{
		{"signup", "signup.", "authling:runtime:v1", true},
		{"reset", "password-reset.", "authling:password-reset-runtime:v1", true},
		{"email", "email-change.", "authling:email-change-runtime:v1", true},
		{"session", "session.", "authling:runtime-session:v1", false},
		{"oidc", "oidc.request.", "authling:oidc-runtime:v1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sender := &capturingSender{}
			cfg := betaOIDCConfig(t)
			runtime, cancel, runErrors := startTestRuntime(t, cfg, sender)
			defer stopTestRuntime(t, runtime, cancel, runErrors)
			const password = "a deliberately uncommon legacy envelope password"
			account, err := runtime.Accounts.CreateLocal(t.Context(), "legacy@example.invalid", password)
			if err != nil {
				t.Fatal(err)
			}
			var verify func() error
			switch tc.name {
			case "signup", "reset", "email":
				var token string
				switch tc.name {
				case "signup":
					token, err = runtime.Registration.Start(t.Context(), "new@example.invalid")
				case "reset":
					token, err = runtime.PasswordReset.Start(t.Context(), "legacy@example.invalid")
				case "email":
					token, err = runtime.EmailChange.Start(t.Context(), account.ID, password, "new@example.invalid")
				}
				if err != nil {
					t.Fatal(err)
				}
				code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
				verify = func() error {
					switch tc.name {
					case "signup":
						return runtime.Registration.Verify(t.Context(), token, code)
					case "reset":
						return runtime.PasswordReset.Verify(t.Context(), token, code)
					default:
						return runtime.EmailChange.Verify(t.Context(), account.ID, token, code)
					}
				}
			case "session":
				token, _, err := runtime.Sessions.Create(t.Context(), account.ID)
				if err != nil {
					t.Fatal(err)
				}
				verify = func() error { _, err := runtime.Sessions.Validate(t.Context(), token); return err }
			case "oidc":
				response := httptest.NewRecorder()
				betaHandler(runtime, cfg).ServeHTTP(response, httptest.NewRequest(http.MethodGet, betaAuthorizationURL(), nil))
				location, err := url.Parse(response.Header().Get("Location"))
				if err != nil || location.Query().Get("id") == "" {
					t.Fatal("authorization did not create a request")
				}
				verify = func() error { _, err := runtime.OIDC.Consent(t.Context(), location.Query().Get("id")); return err }
			}
			js, err := jetstream.New(runtime.connection.NATS)
			if err != nil {
				t.Fatal(err)
			}
			stores, err := storage.OpenStores(t.Context(), js, 1)
			if err != nil {
				t.Fatal(err)
			}
			key, err := keyvault.New(stores.Keys).WorkflowKey(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer clear(key)
			keys, err := stores.RuntimeState.Keys(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			rewritten := 0
			for _, storageKey := range keys {
				if !strings.HasPrefix(storageKey, tc.prefix) {
					continue
				}
				entry, err := stores.RuntimeState.Get(t.Context(), storageKey)
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(entry.Value(), &fields); err != nil {
					t.Fatal(err)
				}
				nonceName, ciphertextName := "nonce", "ciphertext"
				if tc.capitalized {
					nonceName, ciphertextName = "Nonce", "Ciphertext"
				}
				if len(fields) != 3 || string(fields["version"]) != "1" || fields[nonceName] == nil || fields[ciphertextName] == nil {
					t.Fatal("caller changed envelope format")
				}
				var nonce, ciphertext []byte
				if err := json.Unmarshal(fields[nonceName], &nonce); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(fields[ciphertextName], &ciphertext); err != nil {
					t.Fatal(err)
				}
				aad := []byte(tc.domain + "\x00" + storageKey)
				plain, err := datacrypto.Open(key, ciphertext, nonce, aad)
				if err != nil {
					t.Fatalf("legacy reader could not decrypt new record: %v", err)
				}
				defer clear(plain)
				sealed, err := datacrypto.Seal(key, plain, aad)
				if err != nil {
					t.Fatal(err)
				}
				legacy, err := json.Marshal(map[string]any{"version": 1, nonceName: sealed.Nonce, ciphertextName: sealed.Ciphertext})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := storage.UpdateKeyWithTTL(t.Context(), js, storage.RuntimeStateBucket, storageKey, legacy, entry.Revision(), time.Minute); err != nil {
					t.Fatal(err)
				}
				rewritten++
			}
			if rewritten != 1 {
				t.Fatalf("rewrote %d records, want 1", rewritten)
			}
			if err := verify(); err != nil {
				t.Fatalf("service rejected legacy record: %v", err)
			}
		})
	}
}
