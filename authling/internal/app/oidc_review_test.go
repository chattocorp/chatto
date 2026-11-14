package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/chattocorp/chatto/authling/internal/accounts"

	"github.com/chattocorp/chatto/authling/internal/config"
	"github.com/chattocorp/chatto/authling/internal/keyvault"
	"github.com/chattocorp/chatto/authling/internal/oidcprovider"
	"github.com/chattocorp/chatto/authling/internal/storage"
	"github.com/chattocorp/chatto/authling/internal/web"
	"github.com/nats-io/nats.go/jetstream"
)

// failAccessRecordKV fails precisely the pre-consumption access-record write.
type failAccessRecordKV struct {
	storage.KeyValue
	fail bool
}

func (kv *failAccessRecordKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	if kv.fail && strings.HasPrefix(key, "oidc.token.") {
		return 0, errors.New("injected access record failure")
	}
	return kv.KeyValue.Create(ctx, key, value, opts...)
}

func TestOIDCReviewGrantAndRetryBoundaries(t *testing.T) {
	cfg := embeddedTestConfig(t)
	cfg.HTTP = config.HTTPConfig{BindAddress: "127.0.0.1:8080", PublicURL: "http://localhost:8080"}
	const apiA = "https://a.example"
	const apiB = "https://b.example"
	cfg.OIDC.Clients = []config.OIDCClientConfig{{ID: "test-client", Name: "Test Client", RedirectURIs: []string{"http://localhost:9999/callback"}, Resources: []string{apiA, apiB}}}
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(t.Context(), "oidc@example.com", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := runtime.Sessions.Create(t.Context(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: "authling_session", Value: session}
	js, _, err := storage.Open(t.Context(), runtime.connection.NATS, 1)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(t.Context(), js, 1)
	if err != nil {
		t.Fatal(err)
	}
	vault := keyvault.New(stores.Keys)
	workflowKey, err := vault.WorkflowKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	fault := &failAccessRecordKV{KeyValue: stores.RuntimeState}
	inactive := false
	oidcStore := oidcprovider.NewStorage(fault, workflowKey, oidcprovider.NewResolver(cfg, nil), runtime.issuer, func(ctx context.Context, id string) (string, string, error) {
		p, err := runtime.Accounts.Profile(ctx, id)
		return p.PreferredUsername, p.FullName, err
	}, func(ctx context.Context, id string) error {
		if inactive {
			return accounts.ErrInvalidCredentials
		}
		return runtime.Accounts.RequireActive(ctx, id)
	}, runtime.Accounts.EmailAddress)
	service := oidcprovider.New(cfg, runtime.issuer, oidcStore, runtime.Authorizations, vault)
	if err := service.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	handler := web.Handler(web.Dependencies{Accounts: runtime.Accounts, Authentication: runtime.Authentication, Sessions: runtime.Sessions, Authorizations: runtime.Authorizations, OIDC: service, PublicURL: cfg.HTTP.PublicURL})
	verifier := strings.Repeat("v", 43)
	decode := func(response *httptest.ResponseRecorder) oidcTokens {
		t.Helper()
		if response.Code != http.StatusOK {
			t.Fatalf("token request failed: %d %s", response.Code, response.Body.String())
		}
		var tokens oidcTokens
		if json.Unmarshal(response.Body.Bytes(), &tokens) != nil {
			t.Fatal("invalid token response")
		}
		return tokens
	}
	renew := func(token, scope string) *httptest.ResponseRecorder {
		form := url.Values{"grant_type": {"refresh_token"}, "client_id": {"test-client"}, "refresh_token": {token}}
		if scope != "" {
			form.Set("scope", scope)
		}
		return requestHandler(t, handler, http.MethodPost, "/oauth/token", form.Encode(), nil)
	}
	code := completeAuthorizationForScopes(t, handler, verifier, cookie, "openid email offline_access", apiA)
	fault.fail = true
	if response := redeemCode(t, handler, code, verifier); !strings.Contains(response.Body.String(), "temporarily_unavailable") {
		t.Fatal("access record failure not reported")
	}
	fault.fail = false
	original := decode(redeemCode(t, handler, code, verifier))
	if original.RefreshToken == "" {
		t.Fatal("code was consumed by failed access write")
	}
	fault.fail = true
	if response := renew(original.RefreshToken, ""); !strings.Contains(response.Body.String(), "temporarily_unavailable") {
		t.Fatalf("refresh record failure not reported: %s", response.Body.String())
	}
	fault.fail = false
	current := decode(renew(original.RefreshToken, ""))
	if current.RefreshToken == "" {
		t.Fatal("refresh was consumed by failed access write")
	}
	// Consent to another API neither revokes A nor transfers A's email permission.
	code = completeAuthorizationForScopes(t, handler, verifier, cookie, "openid profile", apiB)
	decode(redeemCode(t, handler, code, verifier))
	if covered, err := runtime.Authorizations.Covers(t.Context(), account.ID, "test-client", []string{"openid", "email"}, apiB); err != nil || covered {
		t.Fatal("scope leaked across resources")
	}
	current = decode(renew(current.RefreshToken, ""))
	// Adding scopes without offline_access keeps the existing offline authorization.
	code = completeAuthorizationForScopes(t, handler, verifier, cookie, "openid profile", apiA)
	decode(redeemCode(t, handler, code, verifier))
	current = decode(renew(current.RefreshToken, "offline_access"))
	if current.IDToken != "" || current.RefreshToken == "" {
		t.Fatal("refresh response ignored effective scopes")
	}
	// Replacement refresh retains original email and openid scope.
	current = decode(renew(current.RefreshToken, "openid email offline_access"))
	if current.IDToken == "" {
		t.Fatal("openid ID token missing")
	}
	// Code exchange must classify an unavailable identity as an invalid grant too.
	code = completeAuthorizationForScopes(t, handler, verifier, cookie, "openid", apiA)
	inactive = true
	if response := redeemCode(t, handler, code, verifier); !strings.Contains(response.Body.String(), "invalid_grant") {
		t.Fatal("inactive code exchange did not return invalid_grant")
	}
	if response := renew(current.RefreshToken, ""); !strings.Contains(response.Body.String(), "invalid_grant") {
		t.Fatal("inactive refresh did not return invalid_grant")
	}
	inactive = false
	// Failing account reads did not consume this token. Omitting offline_access ends renewal.
	ended := decode(renew(current.RefreshToken, "openid"))
	if ended.IDToken == "" || ended.RefreshToken != "" {
		t.Fatal("offline_access omission returned replacement")
	}
	if response := renew(current.RefreshToken, ""); response.Code == http.StatusOK {
		t.Fatal("consumed refresh accepted")
	}
	grants, err := runtime.Authorizations.List(t.Context(), account.ID)
	if err != nil || len(grants) != 1 {
		t.Fatal("grant missing")
	}
	// A distinct family is also invalid after explicit app revocation.
	code = completeAuthorizationForScopes(t, handler, verifier, cookie, "openid offline_access", apiB)
	other := decode(redeemCode(t, handler, code, verifier))
	if err := runtime.Authorizations.Revoke(t.Context(), account.ID, grants[0].ID); err != nil {
		t.Fatal(err)
	}
	if renew(other.RefreshToken, "").Code == http.StatusOK {
		t.Fatal("revocation left family active")
	}
}

func TestOIDCCredentialChangesInvalidateOnlyPasswordRefreshFamilies(t *testing.T) {
	for _, kind := range []string{"password-change", "password-reset", "email-change"} {
		t.Run(kind, func(t *testing.T) {
			cfg := embeddedTestConfig(t)
			cfg.HTTP = config.HTTPConfig{BindAddress: "127.0.0.1:8080", PublicURL: "http://localhost:8080"}
			cfg.OIDC.Clients = []config.OIDCClientConfig{{ID: "test-client", Name: "Test Client", RedirectURIs: []string{"http://localhost:9999/callback"}}}
			runtime, cancel, runErrors := startTestRuntime(t, cfg)
			defer func() { stopTestRuntime(t, runtime, cancel, runErrors) }()
			const password = "a deliberately uncommon password"
			account, err := runtime.Accounts.CreateLocal(t.Context(), "oidc@example.com", password)
			if err != nil {
				t.Fatal(err)
			}
			session, _, err := runtime.Sessions.Create(t.Context(), account.ID)
			if err != nil {
				t.Fatal(err)
			}
			handlerFor := func() http.Handler {
				return web.Handler(web.Dependencies{Accounts: runtime.Accounts, Authentication: runtime.Authentication, Sessions: runtime.Sessions, Authorizations: runtime.Authorizations, OIDC: runtime.OIDC, PublicURL: cfg.HTTP.PublicURL})
			}
			handler := handlerFor()
			verifier := strings.Repeat("v", 43)
			code := completeAuthorizationForScopes(t, handler, verifier, &http.Cookie{Name: "authling_session", Value: session}, "openid email offline_access")
			tokens := issueOIDCTokens(t, handler, code, verifier)
			switch kind {
			case "password-change":
				target, err := runtime.Accounts.PreparePasswordChange(t.Context(), account.ID, password, "another deliberately uncommon password")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := runtime.Accounts.ChangePassword(t.Context(), target); err != nil {
					t.Fatal(err)
				}
			case "password-reset":
				target, found, err := runtime.Accounts.RecordPasswordResetRequested(t.Context(), "oidc@example.com")
				if err != nil || !found {
					t.Fatal("reset target")
				}
				if _, err := runtime.Accounts.ResetPassword(t.Context(), target, "another deliberately uncommon password"); err != nil {
					t.Fatal(err)
				}
			case "email-change":
				target, err := runtime.Accounts.PrepareEmailChange(t.Context(), account.ID, password, "changed@example.com")
				if err != nil {
					t.Fatal(err)
				}
				target, err = runtime.Accounts.RecordEmailChangeRequested(t.Context(), target)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := runtime.Accounts.ChangeEmail(t.Context(), target, "changed@example.com"); err != nil {
					t.Fatal(err)
				}
			}
			stopTestRuntime(t, runtime, cancel, runErrors)
			runtime, cancel, runErrors = startTestRuntime(t, cfg)
			handler = handlerFor()
			response := requestHandler(t, handler, http.MethodPost, "/oauth/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {"test-client"}, "refresh_token": {tokens.RefreshToken}}.Encode(), nil)
			if (response.Code == http.StatusOK) != (kind == "email-change") {
				t.Fatalf("unexpected refresh outcome after %s: %d", kind, response.Code)
			}
			if kind != "email-change" && !strings.Contains(response.Body.String(), "invalid_grant") {
				t.Fatal("password change error")
			}
			if covered, err := runtime.Authorizations.Covers(t.Context(), account.ID, "test-client", []string{"openid", "email", "offline_access"}); err != nil || !covered {
				t.Fatal("credential change removed consent")
			}
			// A new authenticated browser can reuse that explicit consent with prompt=none.
			session, _, err = runtime.Sessions.Create(t.Context(), account.ID)
			if err != nil {
				t.Fatal(err)
			}
			query := url.Values{"client_id": {"test-client"}, "redirect_uri": {"http://localhost:9999/callback"}, "response_type": {"code"}, "scope": {"openid email offline_access"}, "prompt": {"none"}, "code_challenge": {"7w_YNF9DSfIdPf_pRjSq646_kPr-2-o9NAl16JGghdM"}, "code_challenge_method": {"S256"}}
			first := requestHandler(t, handler, http.MethodGet, "/oauth/authorize?"+query.Encode(), "", &http.Cookie{Name: "authling_session", Value: session})
			consent := requestHandler(t, handler, http.MethodGet, first.Header().Get("Location"), "", &http.Cookie{Name: "authling_session", Value: session})
			callback := requestHandler(t, handler, http.MethodGet, consent.Header().Get("Location"), "", &http.Cookie{Name: "authling_session", Value: session})
			location, err := url.Parse(callback.Header().Get("Location"))
			if err != nil || location.Query().Get("code") == "" {
				t.Fatal("silent reauthorization failed")
			}
			issueOIDCTokens(t, handler, location.Query().Get("code"), verifier)
		})
	}
}
