package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"hmans.de/authling/internal/authorizations"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/keyvault"
	"hmans.de/authling/internal/logging"
	"hmans.de/authling/internal/storage"
	"hmans.de/authling/internal/web"
	"hmans.de/chatto/pkg/datacrypto"
	"hmans.de/chatto/pkg/events"
)

func TestOIDCScopeClaimsAndCurrentEmail(t *testing.T) {
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
	created, _ := lastAccountEvent(t, runtime, account.ID)
	if _, err := runtime.Accounts.UpdateProfile(t.Context(), account.ID, "distinct-handle", "Distinct Full Name"); err != nil {
		t.Fatal(err)
	}
	session, _, err := runtime.Sessions.Create(t.Context(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: "authling_session", Value: session}
	handlerFor := func() http.Handler {
		return web.Handler(web.Dependencies{Accounts: runtime.Accounts, Authentication: runtime.Authentication, Sessions: runtime.Sessions, Authorizations: runtime.Authorizations, OIDC: runtime.OIDC, PublicURL: cfg.HTTP.PublicURLOrDefault()})
	}
	handler := handlerFor()
	verifier := strings.Repeat("v", 43)
	tokensByScope := map[string]oidcTokens{}
	assertClaims := func(t *testing.T, claims map[string]any, scope, email string) {
		t.Helper()
		if claims["sub"] != account.ID {
			t.Fatal("incorrect subject")
		}
		for key, want := range map[string]any{"preferred_username": "distinct-handle", "name": "Distinct Full Name", "email": email, "email_verified": true} {
			allowed := strings.Contains(scope, "profile")
			if key == "email" || key == "email_verified" {
				allowed = strings.Contains(scope, "email")
			}
			value, present := claims[key]
			if present != allowed || (allowed && value != want) {
				t.Fatalf("scope %s: claim %s = %v (present %v), want %v (present %v)", scope, key, value, present, want, allowed)
			}
		}
	}
	userinfo := func(token string) *httptest.ResponseRecorder {
		// A caller-supplied scope must never widen the stored token scopes.
		r := httptest.NewRequest("GET", "http://localhost:8080/oauth/userinfo?scope=openid%20profile%20email", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, scope := range []string{"openid", "openid profile", "openid email", "openid profile email"} {
		t.Run(scope, func(t *testing.T) {
			code := completeAuthorizationForScopes(t, handler, verifier, cookie, scope)
			tokens := issueOIDCTokens(t, handler, code, verifier)
			tokensByScope[scope] = tokens
			assertClaims(t, verifyIDToken(t, runtime, tokens.IDToken), scope, "oidc@example.com")
			response := userinfo(tokens.AccessToken)
			if response.Code != http.StatusOK {
				t.Fatalf("userinfo status %d", response.Code)
			}
			var claims map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &claims); err != nil {
				t.Fatal(err)
			}
			assertClaims(t, claims, scope, "oidc@example.com")
		})
	}
	// Explicit consent replaces scopes; a covering grant can serve a subset.
	client := authorizations.Client{ID: "test-client", Name: "Test Client", Host: "configured"}
	if covered, err := runtime.Authorizations.Covers(t.Context(), account.ID, client.ID, []string{"openid", "email"}); err != nil || !covered {
		t.Fatalf("subset not covered: %v", err)
	}
	grant, err := runtime.Authorizations.Authorize(t.Context(), account.ID, client, []string{"openid"})
	if err != nil || !slices.Equal(grant.Scopes, []string{"openid"}) {
		t.Fatalf("replacement scopes: %v, %v", grant.Scopes, err)
	}
	if covered, err := runtime.Authorizations.Covers(t.Context(), account.ID, client.ID, []string{"openid", "email"}); err != nil || covered {
		t.Fatalf("expanded request covered: %v", err)
	}
	target, err := runtime.Accounts.PrepareEmailChange(t.Context(), account.ID, password, "changed@example.com")
	if err != nil {
		t.Fatal(err)
	}
	target, err = runtime.Accounts.RecordEmailChangeRequested(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	account, err = runtime.Accounts.ChangeEmail(t.Context(), target, "changed@example.com")
	if err != nil {
		t.Fatal(err)
	}
	stopTestRuntime(t, runtime, cancel, runErrors)
	runtime, cancel, runErrors = startTestRuntime(t, cfg)
	handler = handlerFor()
	for scope, tokens := range tokensByScope {
		response := userinfo(tokens.AccessToken)
		if response.Code != http.StatusOK {
			t.Fatalf("userinfo after restart status %d", response.Code)
		}
		var claims map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &claims); err != nil {
			t.Fatal(err)
		}
		assertClaims(t, claims, scope, "changed@example.com")
	}
	js, _, err := storage.Open(t.Context(), runtime.connection.NATS, 1)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(t.Context(), js, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := stores.Keys.Purge(t.Context(), created.GetAccountCreated().GetUserKeyRef()); err != nil {
		t.Fatal(err)
	}
	for scope, tokens := range tokensByScope {
		if scope != "openid" && userinfo(tokens.AccessToken).Code == http.StatusOK {
			t.Fatalf("scope %s released claims without keys", scope)
		}
	}
	if err := runtime.Accounts.RequestErasure(t.Context(), account.ID, account.AuthenticationVersion); err != nil {
		t.Fatal(err)
	}
	for scope, tokens := range tokensByScope {
		if userinfo(tokens.AccessToken).Code == http.StatusOK {
			t.Fatalf("scope %s accepted erased account", scope)
		}
	}
}

func TestOIDCLegacyGrantRequiresFreshConsentAfterRestart(t *testing.T) {
	cfg := embeddedTestConfig(t)
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	defer func() { stopTestRuntime(t, runtime, cancel, runErrors) }()
	account, err := runtime.Accounts.CreateLocal(t.Context(), "legacy@example.com", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	client := authorizations.Client{ID: "legacy-client", Name: "Legacy App", Host: "legacy.example"}
	grant, err := runtime.Authorizations.Authorize(t.Context(), account.ID, client, []string{"openid"})
	if err != nil {
		t.Fatal(err)
	}
	legacy, _ := lastAccountEvent(t, runtime, account.ID)
	legacy.Id = "evt_legacy_disclosure"
	payload := legacy.GetOidcGrantAuthorized()
	payload.PriorAuthorizationEventId = grant.AuthorizationEventID
	payload.ConsentVersion = 1
	js, stream, err := storage.Open(t.Context(), runtime.connection.NATS, 1)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(t.Context(), js, 1)
	if err != nil {
		t.Fatal(err)
	}
	key, err := keyvault.New(stores.Keys).ResolveDataKey(t.Context(), payload.CredentialKeyRef, payload.UserKeyRef)
	if err != nil {
		t.Fatal(err)
	}
	// Historical version-1 envelope contract, deliberately independent of
	// the current writer so this fixture cannot silently advance versions.
	aad, err := json.Marshal([]any{"authling:oidc-grant-metadata:v1", legacy.Id, account.ID, payload.GrantId, payload.ClientIdDigest, payload.Scopes, payload.PriorAuthorizationEventId, uint32(1), payload.UserKeyRef, payload.CredentialKeyRef})
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := datacrypto.Seal(key, []byte(`{"Name":"Legacy App","Host":"legacy.example"}`), aad)
	clear(key)
	if err != nil {
		t.Fatal(err)
	}
	payload.MetadataNonce, payload.MetadataCiphertext = sealed.Nonce, sealed.Ciphertext
	publisher := evtstream.NewPublisher(events.NewEncodedEventLog(js, stream, logging.Events{Logger: slog.Default()}))
	tail, err := publisher.AccountTail(t.Context(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.AppendOIDCGrantAuthorized(t.Context(), legacy, tail); err != nil {
		t.Fatal(err)
	}
	stopTestRuntime(t, runtime, cancel, runErrors)
	runtime, cancel, runErrors = startTestRuntime(t, cfg)
	grants, err := runtime.Authorizations.List(t.Context(), account.ID)
	if err != nil || len(grants) != 1 || grants[0].ClientName != "Legacy App" {
		t.Fatalf("historical grant read: %v", err)
	}
	if covered, err := runtime.Authorizations.Covers(t.Context(), account.ID, client.ID, []string{"openid"}); err != nil || covered {
		t.Fatalf("legacy disclosure reused: covered=%v err=%v", covered, err)
	}
	renewed, err := runtime.Authorizations.Authorize(t.Context(), account.ID, client, []string{"openid", "profile"})
	if err != nil || renewed.ID != grant.ID {
		t.Fatalf("renewal: %v", err)
	}
	if covered, err := runtime.Authorizations.Covers(t.Context(), account.ID, client.ID, []string{"openid"}); err != nil || !covered {
		t.Fatalf("new disclosure not reused: %v", err)
	}
}
