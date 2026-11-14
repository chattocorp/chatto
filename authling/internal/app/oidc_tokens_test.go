package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chattocorp/chatto/authling/internal/config"
	"github.com/chattocorp/chatto/authling/internal/web"
	jose "github.com/go-jose/go-jose/v4"
)

func TestOIDCResourceJWTAndRefreshLifecycle(t *testing.T) {
	cfg := embeddedTestConfig(t)
	cfg.HTTP = config.HTTPConfig{BindAddress: "127.0.0.1:8080", PublicURL: "http://localhost:8080"}
	const resource = "https://cloud.example.test"
	cfg.OIDC.Clients = []config.OIDCClientConfig{{ID: "test-client", Name: "Test Client", RedirectURIs: []string{"http://localhost:9999/callback"}, Resources: []string{resource}, AccessTokenClaims: map[string][]string{resource: {"email", "profile"}}}}
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	defer func() { stopTestRuntime(t, runtime, cancel, runErrors) }()
	account, err := runtime.Accounts.CreateLocal(t.Context(), "oidc@example.com", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := runtime.Sessions.Create(t.Context(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: "authling_session", Value: session}
	handlerFor := func() http.Handler {
		return web.Handler(web.Dependencies{Accounts: runtime.Accounts, Authentication: runtime.Authentication, Sessions: runtime.Sessions, Authorizations: runtime.Authorizations, OIDC: runtime.OIDC, PublicURL: cfg.HTTP.PublicURL})
	}
	handler := handlerFor()
	verifier := strings.Repeat("v", 43)
	issue := func(resourceValues ...string) oidcTokens {
		code := completeAuthorizationForScopes(t, handler, verifier, cookie, "openid email offline_access", resourceValues...)
		return issueOIDCTokens(t, handler, code, verifier)
	}
	refresh := func(token, client string, resources ...string) *httptest.ResponseRecorder {
		form := url.Values{"grant_type": {"refresh_token"}, "client_id": {client}, "refresh_token": {token}}
		if len(resources) > 0 {
			form["resource"] = resources
		}
		return requestHandler(t, handler, http.MethodPost, "/oauth/token", form.Encode(), nil)
	}
	decode := func(response *httptest.ResponseRecorder) oidcTokens {
		if response.Code != http.StatusOK {
			t.Fatalf("token status %d: %s", response.Code, response.Body.String())
		}
		var tokens oidcTokens
		if err := json.Unmarshal(response.Body.Bytes(), &tokens); err != nil {
			t.Fatal(err)
		}
		if tokens.RefreshToken == "" {
			t.Fatal("missing refresh token")
		}
		return tokens
	}
	// Reject resource widening before code consumption.
	resourceCode := completeAuthorizationForScopes(t, handler, verifier, cookie, "openid", resource)
	badTarget := url.Values{"grant_type": {"authorization_code"}, "client_id": {"test-client"}, "redirect_uri": {"http://localhost:9999/callback"}, "code": {resourceCode}, "code_verifier": {verifier}, "resource": {"https://other.example"}}
	if response := requestHandler(t, handler, http.MethodPost, "/oauth/token", badTarget.Encode(), nil); response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "invalid_target") {
		t.Fatal("code resource widening accepted")
	}
	issueOIDCTokens(t, handler, resourceCode, verifier)
	for _, targets := range [][]string{{"https://unknown.example"}, {resource, resource}, {""}, {resource + "#fragment"}} {
		query := url.Values{"client_id": {"test-client"}, "redirect_uri": {"http://localhost:9999/callback"}, "response_type": {"code"}, "scope": {"openid offline_access"}, "code_challenge": {"7w_YNF9DSfIdPf_pRjSq646_kPr-2-o9NAl16JGghdM"}, "code_challenge_method": {"S256"}, "resource": targets}
		response := requestHandler(t, handler, http.MethodGet, "/oauth/authorize?"+query.Encode(), "", cookie)
		location, _ := url.Parse(response.Header().Get("Location"))
		if location.Query().Get("error") != "invalid_target" {
			t.Fatal("invalid resource was not rejected")
		}
	}
	tokens := issue(resource)
	if tokens.RefreshToken == "" {
		t.Fatal("offline access did not issue refresh token")
	}
	signed, err := jose.ParseSigned(tokens.AccessToken, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil || signed.Signatures[0].Header.ExtraHeaders["typ"] != "at+jwt" {
		t.Fatal("access token type")
	}
	claims := verifyIDToken(t, runtime, tokens.AccessToken)
	if float64(tokens.ExpiresIn) != claims["exp"].(float64)-claims["iat"].(float64) {
		t.Fatal("expires_in does not match signed lifetime")
	}
	for _, name := range []string{"iss", "sub", "aud", "exp", "iat", "jti", "client_id", "scope", "email", "email_verified"} {
		if claims[name] == nil {
			t.Fatalf("missing %s", name)
		}
	}
	if claims["sub"] != account.ID || claims["client_id"] != "test-client" || claims["email"] != "oidc@example.com" || claims["email_verified"] != true {
		t.Fatal("access claims")
	}
	// Profile is configured for this resource but was not granted.
	if claims["preferred_username"] != nil {
		t.Fatal("ungranted profile claim released")
	}
	audiences := claims["aud"].([]any)
	if len(audiences) != 1 || audiences[0] != resource {
		t.Fatal("access audience")
	}
	// An API token cannot read identity data from UserInfo.
	apiUserinfo := httptest.NewRequest(http.MethodGet, "http://localhost:8080/oauth/userinfo", nil)
	apiUserinfo.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	apiResult := httptest.NewRecorder()
	handler.ServeHTTP(apiResult, apiUserinfo)
	if apiResult.Code != http.StatusUnauthorized {
		t.Fatalf("API token accepted at UserInfo: %d", apiResult.Code)
	}
	idClaims := verifyIDToken(t, runtime, tokens.IDToken)
	if len(idClaims["aud"].([]any)) != 1 || idClaims["aud"].([]any)[0] != "test-client" {
		t.Fatal("ID token audience changed")
	}
	if refresh(tokens.RefreshToken, "test-client", "https://other.example").Code == http.StatusOK {
		t.Fatal("resource widening")
	}
	if refresh(tokens.RefreshToken, "unknown-client").Code == http.StatusOK {
		t.Fatal("cross-client refresh")
	}
	// Persisted grants and encrypted families survive cold replay.
	stopTestRuntime(t, runtime, cancel, runErrors)
	runtime, cancel, runErrors = startTestRuntime(t, cfg)
	handler = handlerFor()
	next := decode(refresh(tokens.RefreshToken, "test-client", resource))
	if next.RefreshToken == tokens.RefreshToken {
		t.Fatal("refresh did not rotate")
	}
	// A repeat inside the grace window returns the current credential again.
	if repeat := decode(refresh(tokens.RefreshToken, "test-client")); repeat.RefreshToken != next.RefreshToken || repeat.AccessToken == next.AccessToken {
		t.Fatal("grace repeat did not return the current credential with a new access token")
	}
	// Each rotation permits one repeat. A second repeat is reuse and revokes the family.
	if refresh(tokens.RefreshToken, "test-client").Code == http.StatusOK {
		t.Fatal("second repeat accepted")
	}
	if refresh(next.RefreshToken, "test-client").Code == http.StatusOK {
		t.Fatal("second repeat did not revoke family")
	}
	// A credential two generations old is reuse, also inside the grace window.
	aged := issue(resource)
	second := decode(refresh(aged.RefreshToken, "test-client"))
	third := decode(refresh(second.RefreshToken, "test-client"))
	if refresh(aged.RefreshToken, "test-client").Code == http.StatusOK {
		t.Fatal("old generation accepted")
	}
	if refresh(third.RefreshToken, "test-client").Code == http.StatusOK {
		t.Fatal("reuse did not revoke family")
	}
	// Concurrent use of one credential converges on one replacement and keeps the family.
	concurrent := issue(resource)
	responses := make([]*httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := range responses {
		wg.Add(1)
		go func() { defer wg.Done(); responses[i] = refresh(concurrent.RefreshToken, "test-client") }()
	}
	wg.Wait()
	first, second := decode(responses[0]), decode(responses[1])
	if first.RefreshToken != second.RefreshToken {
		t.Fatal("concurrent refreshes produced different replacements")
	}
	if refresh(first.RefreshToken, "test-client").Code != http.StatusOK {
		t.Fatal("concurrent refresh revoked family")
	}
	// Revocation ends refresh authority, but does not revoke signed access tokens.
	revoked := issue()
	grants, err := runtime.Authorizations.List(t.Context(), account.ID)
	if err != nil || len(grants) != 1 {
		t.Fatal("grant unavailable")
	}
	if err := runtime.Authorizations.Revoke(t.Context(), account.ID, grants[0].ID); err != nil {
		t.Fatal(err)
	}
	if refresh(revoked.RefreshToken, "test-client").Code == http.StatusOK {
		t.Fatal("revoked grant refreshed")
	}
	userinfo := httptest.NewRequest(http.MethodGet, "http://localhost:8080/oauth/userinfo", nil)
	userinfo.Header.Set("Authorization", "Bearer "+revoked.AccessToken)
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, userinfo)
	if result.Code != http.StatusOK {
		t.Fatal("short-lived access token revoked")
	}
	userinfo.Header.Set("Authorization", "Bearer "+revoked.IDToken)
	result = httptest.NewRecorder()
	handler.ServeHTTP(result, userinfo)
	if result.Code == http.StatusOK {
		t.Fatal("ID token accepted as access token")
	}
	defaults := issue()
	claims = verifyIDToken(t, runtime, defaults.AccessToken)
	if len(claims["aud"].([]any)) != 1 || claims["aud"].([]any)[0] != "http://localhost:8080/oauth/userinfo" {
		t.Fatal("default audience")
	}
	// Without configured claims for the resource, access tokens carry no identity claims.
	for _, name := range []string{"email", "email_verified", "preferred_username", "name"} {
		if claims[name] != nil {
			t.Fatalf("unconfigured access token carries %s", name)
		}
	}

	// A correctly signed JWT with the wrong context must still fail at UserInfo.
	key, err := runtime.issuer.SigningKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"issuer", "audience", "expiry", "future", "near-iat", "near-nbf", "future-nbf", "subject", "id", "type", "signature"} {
		candidate := make(map[string]any, len(claims))
		for name, value := range claims {
			candidate[name] = value
		}
		typ := "at+jwt"
		switch mutation {
		case "issuer":
			candidate["iss"] = "https://other.example"
		case "audience":
			candidate["aud"] = []string{resource}
		case "expiry":
			candidate["exp"] = time.Now().Add(-time.Minute).Unix()
		case "future":
			candidate["iat"] = time.Now().Add(time.Hour).Unix()
		case "near-iat":
			candidate["iat"] = time.Now().Add(30 * time.Second).Unix()
		case "near-nbf":
			candidate["nbf"] = time.Now().Add(30 * time.Second).Unix()
		case "future-nbf":
			candidate["nbf"] = time.Now().Add(90 * time.Second).Unix()
		case "subject":
			candidate["sub"] = "acc_other"
		case "id":
			candidate["jti"] = "at_other"
		case "type":
			typ = "JWT"
		}
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: &jose.JSONWebKey{Key: key.Private, KeyID: key.ID}}, (&jose.SignerOptions{}).WithType(jose.ContentType(typ)))
		if err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		signed, err := signer.Sign(payload)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := signed.CompactSerialize()
		if err != nil {
			t.Fatal(err)
		}
		if mutation == "signature" {
			parts := strings.Split(raw, ".")
			parts[2] = "AAAA"
			raw = strings.Join(parts, ".")
		}
		req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/oauth/userinfo", nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if (response.Code == http.StatusOK) != strings.HasPrefix(mutation, "near-") {
			t.Fatalf("userinfo skew/validation failed: %s", mutation)
		}
	}
	// Effective scopes can narrow without changing the replacement refresh authority.
	form := url.Values{"grant_type": {"refresh_token"}, "client_id": {"test-client"}, "refresh_token": {defaults.RefreshToken}, "scope": {"openid offline_access"}}
	narrowed := decode(requestHandler(t, handler, http.MethodPost, "/oauth/token", form.Encode(), nil))
	if verifyIDToken(t, runtime, narrowed.AccessToken)["email"] != nil {
		t.Fatal("reduced scope released email")
	}
	form.Set("refresh_token", narrowed.RefreshToken)
	form.Set("scope", "openid email offline_access")
	restored := decode(requestHandler(t, handler, http.MethodPost, "/oauth/token", form.Encode(), nil))
	// Narrowing away offline_access still returns a replacement with the original scope.
	form.Set("refresh_token", restored.RefreshToken)
	form.Set("scope", "openid")
	withoutOffline := decode(requestHandler(t, handler, http.MethodPost, "/oauth/token", form.Encode(), nil))
	if withoutOffline.Scope != "openid" {
		t.Fatalf("narrowed response scope %q", withoutOffline.Scope)
	}
	form.Set("refresh_token", withoutOffline.RefreshToken)
	form.Set("scope", "openid email offline_access")
	if requestHandler(t, handler, http.MethodPost, "/oauth/token", form.Encode(), nil).Code != http.StatusOK {
		t.Fatal("original refresh scope was not preserved")
	}
}

func TestOIDCConfidentialRefreshAuthentication(t *testing.T) {
	cfg := embeddedTestConfig(t)
	cfg.HTTP = config.HTTPConfig{BindAddress: "127.0.0.1:8080", PublicURL: "http://localhost:8080"}
	secret := strings.Repeat("s", 32)
	cfg.OIDC.Clients = []config.OIDCClientConfig{{ID: "test-client", Name: "Test Client", Secret: secret, RedirectURIs: []string{"http://localhost:9999/callback"}}}
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
	handler := web.Handler(web.Dependencies{Accounts: runtime.Accounts, Authentication: runtime.Authentication, Sessions: runtime.Sessions, Authorizations: runtime.Authorizations, OIDC: runtime.OIDC, PublicURL: cfg.HTTP.PublicURL})
	verifier := strings.Repeat("v", 43)
	code := completeAuthorizationForScopes(t, handler, verifier, &http.Cookie{Name: "authling_session", Value: session}, "openid offline_access")
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {"test-client"}, "client_secret": {secret}, "redirect_uri": {"http://localhost:9999/callback"}, "code": {code}, "code_verifier": {verifier}}
	response := requestHandler(t, handler, http.MethodPost, "/oauth/token", form.Encode(), nil)
	var tokens oidcTokens
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &tokens) != nil || tokens.RefreshToken == "" {
		t.Fatal("confidential code exchange")
	}
	for _, method := range []string{"basic", "post"} {
		t.Run(method, func(t *testing.T) {
			for _, credential := range []string{"", "wrong-secret", secret} {
				form := url.Values{"grant_type": {"refresh_token"}, "client_id": {"test-client"}, "refresh_token": {tokens.RefreshToken}}
				if method == "post" {
					form.Set("client_secret", credential)
				}
				req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/oauth/token", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if method == "basic" {
					req.SetBasicAuth("test-client", credential)
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, req)
				if credential != secret {
					if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "invalid_client") || response.Header().Get("WWW-Authenticate") == "" {
						t.Fatalf("bad refresh credentials: status %d: %s", response.Code, response.Body.String())
					}
					continue
				}
				if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &tokens) != nil || tokens.RefreshToken == "" {
					t.Fatal("authenticated refresh failed")
				}
			}
		})
	}
}
