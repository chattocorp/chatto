package http_server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	jose "github.com/go-jose/go-jose/v3"
	josejwt "github.com/go-jose/go-jose/v3/jwt"
	"github.com/markbates/goth"
	gothgithub "github.com/markbates/goth/providers/github"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
)

func TestProviderScopesForOIDC(t *testing.T) {
	t.Run("default keeps openid profile", func(t *testing.T) {
		scopes := providerScopes(config.AuthProviderConfig{Type: config.AuthProviderTypeOpenIDConnect})
		want := []string{oidc.ScopeOpenID, "profile"}
		if !slices.Equal(scopes, want) {
			t.Fatalf("providerScopes() = %v, want %v", scopes, want)
		}
	})

	t.Run("request_email true requests openid profile email", func(t *testing.T) {
		requestEmail := true
		scopes := providerScopes(config.AuthProviderConfig{
			Type:         config.AuthProviderTypeOpenIDConnect,
			RequestEmail: &requestEmail,
		})
		want := []string{oidc.ScopeOpenID, "profile", "email"}
		if !slices.Equal(scopes, want) {
			t.Fatalf("providerScopes() = %v, want %v", scopes, want)
		}
	})

	t.Run("custom scopes are honored with openid required", func(t *testing.T) {
		scopes := providerScopes(config.AuthProviderConfig{
			Type:   config.AuthProviderTypeOpenIDConnect,
			Scopes: []string{"groups", "profile"},
		})
		want := []string{oidc.ScopeOpenID, "groups", "profile"}
		if !slices.Equal(scopes, want) {
			t.Fatalf("providerScopes() = %v, want %v", scopes, want)
		}
	})
}

func TestProviderScopesForGoogle(t *testing.T) {
	t.Run("default keeps openid profile", func(t *testing.T) {
		scopes := providerScopes(config.AuthProviderConfig{Type: config.AuthProviderTypeGoogle})
		want := []string{"openid", "profile"}
		if !slices.Equal(scopes, want) {
			t.Fatalf("providerScopes() = %v, want %v", scopes, want)
		}
	})

	t.Run("request_email true requests openid profile email", func(t *testing.T) {
		requestEmail := true
		scopes := providerScopes(config.AuthProviderConfig{
			Type:         config.AuthProviderTypeGoogle,
			RequestEmail: &requestEmail,
		})
		want := []string{"openid", "profile", "email"}
		if !slices.Equal(scopes, want) {
			t.Fatalf("providerScopes() = %v, want %v", scopes, want)
		}
	})
}

func TestVerifiedEmailFromGothUser(t *testing.T) {
	t.Run("discord requires verified flag", func(t *testing.T) {
		runtime := &authProviderRuntime{config: config.AuthProviderConfig{Type: config.AuthProviderTypeDiscord}}
		unverified := runtime.verifiedEmailFromGothUser(t.Context(), goth.User{
			Email:   "User@Example.com",
			RawData: map[string]interface{}{"verified": false},
		})
		if unverified != "" {
			t.Fatalf("unverified discord email = %q, want empty", unverified)
		}
		verified := runtime.verifiedEmailFromGothUser(t.Context(), goth.User{
			Email:   "User@Example.com",
			RawData: map[string]interface{}{"verified": true},
		})
		if verified != "user@example.com" {
			t.Fatalf("verified discord email = %q, want normalized email", verified)
		}
	})

	t.Run("google requires verified email flag", func(t *testing.T) {
		runtime := &authProviderRuntime{config: config.AuthProviderConfig{Type: config.AuthProviderTypeGoogle}}
		unverified := runtime.verifiedEmailFromGothUser(t.Context(), goth.User{
			Email:   "User@Example.com",
			RawData: map[string]interface{}{"verified_email": false},
		})
		if unverified != "" {
			t.Fatalf("unverified google email = %q, want empty", unverified)
		}
		verified := runtime.verifiedEmailFromGothUser(t.Context(), goth.User{
			Email:   "User@Example.com",
			RawData: map[string]interface{}{"verified_email": true},
		})
		if verified != "user@example.com" {
			t.Fatalf("verified google email = %q, want normalized email", verified)
		}
	})

	t.Run("gitlab raw email is only a hint", func(t *testing.T) {
		runtime := &authProviderRuntime{config: config.AuthProviderConfig{Type: config.AuthProviderTypeGitLab}}
		if got := runtime.verifiedEmailFromGothUser(t.Context(), goth.User{Email: "user@example.com"}); got != "" {
			t.Fatalf("gitlab verified email = %q, want empty", got)
		}
	})
}

func TestFetchGitHubVerifiedPrimaryEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token-1" {
			t.Fatalf("Authorization = %q, want bearer token", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"email":"secondary@example.com","primary":false,"verified":true},
			{"email":"Primary@Example.com","primary":true,"verified":true}
		]`))
	}))
	t.Cleanup(server.Close)
	oldEmailURL := gothgithub.EmailURL
	gothgithub.EmailURL = server.URL
	t.Cleanup(func() { gothgithub.EmailURL = oldEmailURL })

	email, err := fetchGitHubVerifiedPrimaryEmail(t.Context(), "token-1")
	if err != nil {
		t.Fatalf("fetchGitHubVerifiedPrimaryEmail: %v", err)
	}
	if email != "primary@example.com" {
		t.Fatalf("email = %q, want normalized primary email", email)
	}
}

func TestOIDCProviderWithoutEmailAutoProvisionLinkAndLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestEmail := false
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()

	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.Providers = []config.AuthProviderConfig{{
			ID:            "oidc-no-email",
			Type:          config.AuthProviderTypeOpenIDConnect,
			Label:         "No Email OIDC",
			IssuerURL:     issuer.URL(),
			ClientID:      "client-id",
			ClientSecret:  "client-secret",
			RequestEmail:  &requestEmail,
			AutoProvision: boolPtr(true),
		}}
		s.setupOIDCRoutes()
	})

	issuer.SetSubject("subject-create")
	createToken := completeNoEmailOIDCHandshake(t, client, ts.URL, "oidc-no-email", "/chat")
	createFlow, err := chattoCore.GetPendingExternalIdentityCreateFlow(t.Context(), createToken)
	if err != nil {
		t.Fatalf("GetPendingExternalIdentityCreateFlow: %v", err)
	}
	if createFlow.VerifiedEmail != "" {
		t.Fatalf("create flow VerifiedEmail = %q, want empty", createFlow.VerifiedEmail)
	}
	if createFlow.Issuer != issuer.URL() || createFlow.Subject != "subject-create" {
		t.Fatalf("create flow identity = %s/%s", createFlow.Issuer, createFlow.Subject)
	}
	if createFlow.RedirectPath != "/chat" {
		t.Fatalf("create flow RedirectPath = %q, want /chat", createFlow.RedirectPath)
	}

	user, err := chattoCore.CreateUserForExternalIdentity(t.Context(), "noemailoidc", "No Email OIDC", createFlow)
	if err != nil {
		t.Fatalf("CreateUserForExternalIdentity: %v", err)
	}
	if err := chattoCore.DeletePendingExternalIdentityFlow(t.Context(), createToken); err != nil {
		t.Fatalf("DeletePendingExternalIdentityFlow: %v", err)
	}
	if hasEmail, err := chattoCore.HasVerifiedEmail(t.Context(), user.Id); err != nil || hasEmail {
		t.Fatalf("HasVerifiedEmail = %v, %v; want false", hasEmail, err)
	}
	if got, err := chattoCore.CountVerifiedAccounts(t.Context()); err != nil || got != 1 {
		t.Fatalf("CountVerifiedAccounts = %d, %v; want 1", got, err)
	}

	loginLocation := completeNoEmailOIDCLogin(t, client, ts.URL, "oidc-no-email", "/chat")
	if !strings.HasPrefix(loginLocation, "/chat?token=") {
		t.Fatalf("matched no-email OIDC login Location = %q, want /chat?token=...", loginLocation)
	}

	resp, err := client.Get(ts.URL + "/auth/providers/oidc-no-email?intent=link")
	if err != nil {
		t.Fatalf("GET provider link without start token: %v", err)
	}
	resp.Body.Close()
	if location := resp.Header.Get("Location"); location != "/login?error=provider_failed" {
		t.Fatalf("provider link without start token Location = %q, want provider failure", location)
	}

	issuer.SetSubject("subject-link")
	linkUser, err := chattoCore.CreateUser(t.Context(), core.SystemActorID, "link-no-email-oidc", "Link No Email OIDC", "password123")
	if err != nil {
		t.Fatalf("CreateUser link target: %v", err)
	}
	linkStart, err := chattoCore.CreatePendingExternalIdentityLinkStart(t.Context(), "oidc-no-email", "/chat/-/settings/account", linkUser.Id)
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityLinkStart: %v", err)
	}
	linkToken := completeNoEmailOIDCHandshakeWithQuery(t, client, ts.URL, "oidc-no-email", url.Values{
		"intent":     {"link"},
		"link_start": {linkStart},
	})
	linkFlow, err := chattoCore.GetPendingExternalIdentityLinkFlow(t.Context(), linkToken, linkUser.Id)
	if err != nil {
		t.Fatalf("GetPendingExternalIdentityLinkFlow: %v", err)
	}
	if linkFlow.VerifiedEmail != "" {
		t.Fatalf("link flow VerifiedEmail = %q, want empty", linkFlow.VerifiedEmail)
	}
	if _, err := chattoCore.ConfirmPendingExternalIdentityLink(t.Context(), linkFlow); err != nil {
		t.Fatalf("ConfirmPendingExternalIdentityLink: %v", err)
	}
	linked, err := chattoCore.GetUserByExternalIdentity(t.Context(), issuer.URL(), "subject-link")
	if err != nil {
		t.Fatalf("GetUserByExternalIdentity link: %v", err)
	}
	if linked == nil || linked.Id != linkUser.Id {
		t.Fatalf("linked no-email OIDC identity = %v, want %s", linked, linkUser.Id)
	}

	issuer.SetSubject("subject-conflict")
	conflictToken, err := chattoCore.CreatePendingExternalIdentityCreateFlow(t.Context(), core.PendingExternalIdentityFlow{
		ProviderID:    "oidc-no-email",
		ProviderType:  config.AuthProviderTypeOpenIDConnect,
		ProviderLabel: "No Email OIDC",
		Issuer:        issuer.URL(),
		Subject:       "subject-conflict",
	})
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityCreateFlow conflict: %v", err)
	}
	conflictFlow, err := chattoCore.GetPendingExternalIdentityCreateFlow(t.Context(), conflictToken)
	if err != nil {
		t.Fatalf("GetPendingExternalIdentityCreateFlow conflict: %v", err)
	}
	conflictUser, err := chattoCore.CreateUserForExternalIdentity(t.Context(), "conflict-no-email-oidc", "Conflict No Email OIDC", conflictFlow)
	if err != nil {
		t.Fatalf("CreateUserForExternalIdentity conflict: %v", err)
	}
	if err := chattoCore.DeletePendingExternalIdentityFlow(t.Context(), conflictToken); err != nil {
		t.Fatalf("DeletePendingExternalIdentityFlow conflict: %v", err)
	}
	conflictTarget, err := chattoCore.CreateUser(t.Context(), core.SystemActorID, "conflict-link-target", "Conflict Link Target", "password123")
	if err != nil {
		t.Fatalf("CreateUser conflict target: %v", err)
	}
	conflictStart, err := chattoCore.CreatePendingExternalIdentityLinkStart(t.Context(), "oidc-no-email", "/chat/-/settings/account", conflictTarget.Id)
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityLinkStart conflict: %v", err)
	}
	conflictStartLocation := startNoEmailOIDC(t, client, ts.URL, "oidc-no-email", url.Values{
		"intent":     {"link"},
		"link_start": {conflictStart},
	})
	conflictLocation := finishNoEmailOIDCCallback(t, client, ts.URL, "oidc-no-email", authStateFromLocation(t, conflictStartLocation))
	if conflictLocation != "/chat/-/settings/account?error=external_identity_conflict" {
		t.Fatalf("conflict Location = %q, want settings conflict error redirect (linked user %s)", conflictLocation, conflictUser.Id)
	}

	if issuer.UserInfoRequests() == 0 {
		t.Fatal("expected userinfo fallback when ID token has no email claim")
	}
}

func TestOIDCProviderWithoutEmailIgnoresUserInfoFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestEmail := false
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	issuer.failUserInfo = true
	defer issuer.Close()

	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.Providers = []config.AuthProviderConfig{{
			ID:            "oidc-no-email",
			Type:          config.AuthProviderTypeOpenIDConnect,
			Label:         "No Email OIDC",
			IssuerURL:     issuer.URL(),
			ClientID:      "client-id",
			ClientSecret:  "client-secret",
			RequestEmail:  &requestEmail,
			AutoProvision: boolPtr(true),
		}}
		s.setupOIDCRoutes()
	})

	issuer.SetSubject("subject-create")
	createToken := completeNoEmailOIDCHandshake(t, client, ts.URL, "oidc-no-email", "/chat")
	createFlow, err := chattoCore.GetPendingExternalIdentityCreateFlow(t.Context(), createToken)
	if err != nil {
		t.Fatalf("GetPendingExternalIdentityCreateFlow: %v", err)
	}
	if createFlow.Issuer != issuer.URL() || createFlow.Subject != "subject-create" {
		t.Fatalf("create flow identity = %s/%s", createFlow.Issuer, createFlow.Subject)
	}
	if issuer.UserInfoRequests() == 0 {
		t.Fatal("expected attempted userinfo fallback")
	}
}

func TestLegacyOIDCRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 issuer.URL,
			"authorization_endpoint": issuer.URL + "/authorize",
			"token_endpoint":         issuer.URL + "/token",
			"jwks_uri":               issuer.URL + "/keys",
			"userinfo_endpoint":      issuer.URL + "/userinfo",
		})
	}))
	t.Cleanup(issuer.Close)

	router := gin.New()
	sessionStore := cookie.NewStore([]byte("test-secret-key-32-bytes-long!!"))
	router.Use(sessions.Sessions("chatto_session", sessionStore))

	s := &HTTPServer{
		config: config.ChattoConfig{
			Webserver: config.WebserverConfig{
				URL: "http://chat.example",
			},
			Auth: config.AuthConfig{
				Providers: []config.AuthProviderConfig{{
					ID:           "hub",
					Type:         config.AuthProviderTypeOpenIDConnect,
					IssuerURL:    issuer.URL,
					ClientID:     "client-id",
					ClientSecret: "client-secret",
				}},
			},
		},
		router: router,
		logger: log.WithPrefix("test.HTTP"),
	}
	s.setupOIDCRoutes()

	t.Run("legacy login route uses legacy callback URI", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/oidc", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusTemporaryRedirect {
			t.Fatalf("GET /auth/oidc status = %d, want %d", w.Code, http.StatusTemporaryRedirect)
		}
		assertRedirectURI(t, w.Header().Get("Location"), "http://chat.example/auth/oidc/callback")
	})

	t.Run("provider login route keeps provider callback URI", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/providers/hub", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusTemporaryRedirect {
			t.Fatalf("GET /auth/providers/hub status = %d, want %d", w.Code, http.StatusTemporaryRedirect)
		}
		assertRedirectURI(t, w.Header().Get("Location"), "http://chat.example/auth/providers/hub/callback")
	})

	t.Run("legacy callback route is served", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/oidc/callback?state=missing", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Fatal("GET /auth/oidc/callback returned 404")
		}
		if w.Code != http.StatusTemporaryRedirect {
			t.Fatalf("GET /auth/oidc/callback status = %d, want %d", w.Code, http.StatusTemporaryRedirect)
		}
		if location := w.Header().Get("Location"); !strings.HasPrefix(location, "/login?error=") {
			t.Fatalf("GET /auth/oidc/callback Location = %q, want login error redirect", location)
		}
	})
}

func completeNoEmailOIDCHandshake(t *testing.T, client *http.Client, baseURL, providerID, redirectPath string) string {
	t.Helper()
	return completeNoEmailOIDCHandshakeWithQuery(t, client, baseURL, providerID, url.Values{"redirect": {redirectPath}})
}

func completeNoEmailOIDCLogin(t *testing.T, client *http.Client, baseURL, providerID, redirectPath string) string {
	t.Helper()
	startLocation := startNoEmailOIDC(t, client, baseURL, providerID, url.Values{"redirect": {redirectPath}})
	state := authStateFromLocation(t, startLocation)
	location := finishNoEmailOIDCCallback(t, client, baseURL, providerID, state)
	return location
}

func completeNoEmailOIDCHandshakeWithQuery(t *testing.T, client *http.Client, baseURL, providerID string, query url.Values) string {
	t.Helper()
	startLocation := startNoEmailOIDC(t, client, baseURL, providerID, query)
	state := authStateFromLocation(t, startLocation)
	location := finishNoEmailOIDCCallback(t, client, baseURL, providerID, state)
	redirectURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("callback Location %q did not parse: %v", location, err)
	}
	if redirectURL.Path != "/sso/confirm" {
		t.Fatalf("callback Location = %q, want /sso/confirm", location)
	}
	token := redirectURL.Query().Get("token")
	if token == "" {
		t.Fatalf("callback Location = %q, missing token", location)
	}
	return token
}

func startNoEmailOIDC(t *testing.T, client *http.Client, baseURL, providerID string, query url.Values) string {
	t.Helper()
	reqURL := baseURL + "/auth/providers/" + providerID
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}
	resp, err := client.Get(reqURL)
	if err != nil {
		t.Fatalf("GET provider start: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("provider start status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
	location := resp.Header.Get("Location")
	authURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("provider start Location %q did not parse: %v", location, err)
	}
	if scope := authURL.Query().Get("scope"); scope != "openid profile" {
		t.Fatalf("provider start scope = %q, want openid profile", scope)
	}
	return location
}

func finishNoEmailOIDCCallback(t *testing.T, client *http.Client, baseURL, providerID, state string) string {
	t.Helper()
	callbackURL := baseURL + "/auth/providers/" + providerID + "/callback?state=" + url.QueryEscape(state) + "&code=test-code"
	resp, err := client.Get(callbackURL)
	if err != nil {
		t.Fatalf("GET provider callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("provider callback status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
	return resp.Header.Get("Location")
}

func authStateFromLocation(t *testing.T, location string) string {
	t.Helper()
	authURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("provider start Location %q did not parse: %v", location, err)
	}
	state := authURL.Query().Get("state")
	if state == "" {
		t.Fatalf("provider start Location = %q, missing state", location)
	}
	return state
}

func assertRedirectURI(t *testing.T, location, want string) {
	t.Helper()
	redirectURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("redirect Location %q did not parse: %v", location, err)
	}
	got := redirectURL.Query().Get("redirect_uri")
	if got != want {
		t.Fatalf("redirect_uri = %q, want %q; Location = %q", got, want, location)
	}
}

type noEmailOIDCIssuer struct {
	server           *httptest.Server
	key              *rsa.PrivateKey
	clientID         string
	subject          string
	failUserInfo     bool
	userInfoRequests int
}

func newNoEmailOIDCIssuer(t *testing.T, clientID string) *noEmailOIDCIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	issuer := &noEmailOIDCIssuer{
		key:      key,
		clientID: clientID,
		subject:  "subject",
	}
	issuer.server = httptest.NewServer(http.HandlerFunc(issuer.ServeHTTP))
	return issuer
}

func (i *noEmailOIDCIssuer) Close() {
	i.server.Close()
}

func (i *noEmailOIDCIssuer) URL() string {
	return i.server.URL
}

func (i *noEmailOIDCIssuer) SetSubject(subject string) {
	i.subject = subject
}

func (i *noEmailOIDCIssuer) UserInfoRequests() int {
	return i.userInfoRequests
}

func (i *noEmailOIDCIssuer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 i.server.URL,
			"authorization_endpoint": i.server.URL + "/authorize",
			"token_endpoint":         i.server.URL + "/token",
			"jwks_uri":               i.server.URL + "/keys",
			"userinfo_endpoint":      i.server.URL + "/userinfo",
		})
	case "/authorize":
		http.Redirect(w, r, r.URL.Query().Get("redirect_uri")+"?state="+url.QueryEscape(r.URL.Query().Get("state"))+"&code=test-code", http.StatusTemporaryRedirect)
	case "/token":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     i.idToken(r.Context()),
		})
	case "/keys":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key:       &i.key.PublicKey,
			KeyID:     "test-key",
			Algorithm: string(jose.RS256),
			Use:       "sig",
		}}})
	case "/userinfo":
		i.userInfoRequests++
		if i.failUserInfo {
			http.Error(w, "userinfo unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"sub":                i.subject,
			"name":               "No Email User",
			"preferred_username": "no-email-user",
		})
	default:
		http.NotFound(w, r)
	}
}

func (i *noEmailOIDCIssuer) idToken(_ context.Context) string {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: i.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key"),
	)
	if err != nil {
		panic(err)
	}
	now := time.Now()
	claims := josejwt.Claims{
		Issuer:   i.server.URL,
		Subject:  i.subject,
		Audience: josejwt.Audience{i.clientID},
		Expiry:   josejwt.NewNumericDate(now.Add(time.Hour)),
		IssuedAt: josejwt.NewNumericDate(now),
	}
	profileClaims := struct {
		Name          string `json:"name"`
		PreferredUser string `json:"preferred_username"`
	}{
		Name:          "No Email User",
		PreferredUser: "no-email-user",
	}
	raw, err := josejwt.Signed(signer).Claims(claims).Claims(profileClaims).CompactSerialize()
	if err != nil {
		panic(err)
	}
	return raw
}

func boolPtr(value bool) *bool {
	return &value
}
