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
	"hmans.de/chatto/internal/events"
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

func TestOIDCStringClaim(t *testing.T) {
	tests := []struct {
		name  string
		raw   json.RawMessage
		state oidcStringClaimState
		want  []string
	}{
		{name: "string", raw: json.RawMessage(`"moderator"`), state: oidcStringClaimPresent, want: []string{"moderator"}},
		{name: "array", raw: json.RawMessage(`["moderator", "admin"]`), state: oidcStringClaimPresent, want: []string{"moderator", "admin"}},
		{name: "null is malformed", raw: json.RawMessage(`null`), state: oidcStringClaimMalformed},
		{name: "object is malformed", raw: json.RawMessage(`{"role":"moderator"}`), state: oidcStringClaimMalformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, roles := oidcStringClaim(map[string]json.RawMessage{"roles": tt.raw}, "roles")
			if state != tt.state || !slices.Equal(roles, tt.want) {
				t.Fatalf("oidcStringClaim() = (%v, %v), want (%v, %v)", state, roles, tt.state, tt.want)
			}
		})
	}
}

func TestMergeOIDCProfileClaimsPreservesIDTokenProfile(t *testing.T) {
	primary := oidcProfileClaims{
		Name: "ID Token Name", PreferredUser: "id-token-user", Picture: "https://id.example/avatar.png",
	}
	fallback := oidcProfileClaims{Email: "user@example.com", EmailVerified: true}
	got := mergeOIDCProfileClaims(primary, fallback)
	if got.Email != fallback.Email || !got.EmailVerified {
		t.Fatalf("merged email = %q verified=%v", got.Email, got.EmailVerified)
	}
	if got.Name != primary.Name || got.PreferredUser != primary.PreferredUser || got.Picture != primary.Picture {
		t.Fatalf("merged profile = %+v, want ID-token profile preserved", got)
	}
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
		s.config.Webserver.OAuthRedirectOrigins = []string{"https://client.example"}
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
		s.setupOAuthRoutes()
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

	if err := chattoCore.GrantOAuthConsent(t.Context(), user.Id, "https://client.example"); err != nil {
		t.Fatalf("GrantOAuthConsent: %v", err)
	}
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	redirectURI := "https://client.example/servers/callback"
	state := "multi-server-provider-state"
	authorizeResp, err := client.Get(ts.URL + "/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {core.GenerateCodeChallenge(verifier)},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}.Encode())
	if err != nil {
		t.Fatalf("start multi-server authorize: %v", err)
	}
	authorizeResp.Body.Close()
	if authorizeResp.StatusCode != http.StatusTemporaryRedirect || !strings.HasPrefix(authorizeResp.Header.Get("Location"), "/login?redirect=") {
		t.Fatalf("multi-server authorize status/location = %d/%q, want login redirect", authorizeResp.StatusCode, authorizeResp.Header.Get("Location"))
	}

	callbackLocation := completeNoEmailOIDCLogin(t, client, ts.URL, "oidc-no-email", "/oauth/authorize")
	callbackURL, err := url.Parse(callbackLocation)
	if err != nil {
		t.Fatalf("parse multi-server callback Location %q: %v", callbackLocation, err)
	}
	if got := callbackURL.Scheme + "://" + callbackURL.Host + callbackURL.Path; got != redirectURI {
		t.Fatalf("multi-server callback URI = %q, want %q", got, redirectURI)
	}
	if callbackURL.Query().Get("state") != state || callbackURL.Query().Get("code") == "" || callbackURL.Query().Has("token") {
		t.Fatalf("multi-server callback query = %q, want code and state without token", callbackURL.RawQuery)
	}

	tokenRequest, err := json.Marshal(oauthTokenRequest{
		GrantType:    "authorization_code",
		Code:         callbackURL.Query().Get("code"),
		CodeVerifier: verifier,
		RedirectURI:  redirectURI,
	})
	if err != nil {
		t.Fatalf("encode multi-server token exchange: %v", err)
	}
	tokenResp, err := client.Post(ts.URL+"/oauth/token", "application/json", strings.NewReader(string(tokenRequest)))
	if err != nil {
		t.Fatalf("exchange multi-server authorization code: %v", err)
	}
	defer tokenResp.Body.Close()
	if tokenResp.StatusCode != http.StatusOK {
		t.Fatalf("multi-server token exchange status = %d", tokenResp.StatusCode)
	}
	var tokenResult struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err != nil {
		t.Fatalf("decode multi-server token exchange: %v", err)
	}
	if tokenResult.AccessToken == "" {
		t.Fatal("multi-server token exchange returned no access token")
	}

	issuanceSubject := events.UserAggregate(user.Id).Subject(events.EventBearerTokenIssued)
	issuedBefore, _, err := chattoCore.EventPublisher.SubjectEvents(t.Context(), issuanceSubject)
	if err != nil {
		t.Fatalf("SubjectEvents before provider login: %v", err)
	}
	loginLocation := completeNoEmailOIDCLogin(t, client, ts.URL, "oidc-no-email", "/chat?view=all")
	if loginLocation != "/chat?view=all" {
		t.Fatalf("matched no-email OIDC login Location = %q, want credential-free redirect with query preserved", loginLocation)
	}
	issuedAfter, _, err := chattoCore.EventPublisher.SubjectEvents(t.Context(), issuanceSubject)
	if err != nil {
		t.Fatalf("SubjectEvents after provider login: %v", err)
	}
	if len(issuedAfter) != len(issuedBefore) {
		t.Fatalf("provider login appended %d bearer issuance facts, want none", len(issuedAfter)-len(issuedBefore))
	}
	serverURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	cookieNames := make(map[string]bool)
	for _, cookie := range client.Jar.Cookies(serverURL) {
		cookieNames[cookie.Name] = true
	}
	if !cookieNames["chatto_session"] || !cookieNames[csrfCookieName] {
		t.Fatalf("provider login cookies = %v, want session and CSRF cookies", cookieNames)
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

func TestOIDCRoleClaimSynchronizesMatchedLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()
	issuer.SetSubject("role-subject")
	issuer.SetRoleClaims([]string{core.RoleModerator}, []string{})

	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.Providers = []config.AuthProviderConfig{{
			ID: "oidc-roles", Type: config.AuthProviderTypeOpenIDConnect, Label: "OIDC Roles",
			IssuerURL: issuer.URL(), ClientID: "client-id", ClientSecret: "client-secret",
			RoleClaim: "roles", RoleClaimAllowedRoles: []string{core.RoleModerator}, RoleClaimMode: config.OIDCRoleClaimModeReconcile,
		}}
		s.setupOIDCRoutes()
	})
	user, err := chattoCore.CreateUser(t.Context(), core.SystemActorID, "oidc-role-login", "OIDC Role Login", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := chattoCore.LinkExternalIdentity(t.Context(), "oidc-roles", "oidc", issuer.URL(), "role-subject", user.Id); err != nil {
		t.Fatalf("LinkExternalIdentity: %v", err)
	}

	location := completeNoEmailOIDCLogin(t, client, ts.URL, "oidc-roles", "/chat")
	if location != "/chat" {
		t.Fatalf("OIDC login Location = %q, want /chat", location)
	}
	if !chattoCore.RBAC.HasRole(user.Id, core.RoleModerator) {
		t.Fatal("matched OIDC login should synchronize moderator role")
	}
}

func TestOIDCRoleClaimMalformedIDTokenPreservesManagedRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()
	issuer.SetSubject("malformed-role-subject")
	issuer.SetRoleClaims(map[string]string{"role": core.RoleModerator}, []string{})
	provider := config.AuthProviderConfig{
		ID: "oidc-roles", Type: config.AuthProviderTypeOpenIDConnect, Label: "OIDC Roles",
		IssuerURL: issuer.URL(), ClientID: "client-id", ClientSecret: "client-secret",
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{core.RoleModerator}, RoleClaimMode: config.OIDCRoleClaimModeReconcile,
	}
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.Providers = []config.AuthProviderConfig{provider}
		s.setupOIDCRoutes()
	})
	user, err := chattoCore.CreateUser(t.Context(), core.SystemActorID, "oidc-malformed-role", "OIDC Malformed Role", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := chattoCore.LinkExternalIdentity(t.Context(), provider.ID, "oidc", issuer.URL(), "malformed-role-subject", user.Id); err != nil {
		t.Fatalf("LinkExternalIdentity: %v", err)
	}
	if err := chattoCore.SyncOIDCRoleClaims(t.Context(), user.Id, provider, true, []string{core.RoleModerator}); err != nil {
		t.Fatalf("initial SyncOIDCRoleClaims: %v", err)
	}

	_ = completeNoEmailOIDCLogin(t, client, ts.URL, provider.ID, "/chat")
	if !chattoCore.RBAC.HasRole(user.Id, core.RoleModerator) {
		t.Fatal("a malformed ID-token claim must preserve managed roles even when UserInfo has an empty claim")
	}
}

func TestOIDCRoleClaimMalformedUserInfoPreservesManagedRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()
	issuer.SetSubject("malformed-userinfo-role-subject")
	issuer.SetRoleClaims(nil, map[string]string{"role": core.RoleModerator})
	provider := config.AuthProviderConfig{
		ID: "oidc-roles", Type: config.AuthProviderTypeOpenIDConnect, Label: "OIDC Roles",
		IssuerURL: issuer.URL(), ClientID: "client-id", ClientSecret: "client-secret",
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{core.RoleModerator}, RoleClaimMode: config.OIDCRoleClaimModeReconcile,
	}
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.Providers = []config.AuthProviderConfig{provider}
		s.setupOIDCRoutes()
	})
	user, err := chattoCore.CreateUser(t.Context(), core.SystemActorID, "oidc-malformed-userinfo-role", "OIDC Malformed UserInfo Role", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := chattoCore.LinkExternalIdentity(t.Context(), provider.ID, "oidc", issuer.URL(), "malformed-userinfo-role-subject", user.Id); err != nil {
		t.Fatalf("LinkExternalIdentity: %v", err)
	}
	if err := chattoCore.SyncOIDCRoleClaims(t.Context(), user.Id, provider, true, []string{core.RoleModerator}); err != nil {
		t.Fatalf("initial SyncOIDCRoleClaims: %v", err)
	}

	_ = completeNoEmailOIDCLogin(t, client, ts.URL, provider.ID, "/chat")
	if !chattoCore.RBAC.HasRole(user.Id, core.RoleModerator) {
		t.Fatal("a malformed UserInfo claim must preserve managed roles")
	}
}

func TestOIDCRoleClaimPendingFlowStoresOnlyAcceptedRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()
	issuer.SetRoles([]string{core.RoleModerator, "provider-only-role"})
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.Providers = []config.AuthProviderConfig{{
			ID: "oidc-roles", Type: config.AuthProviderTypeOpenIDConnect, Label: "OIDC Roles",
			IssuerURL: issuer.URL(), ClientID: "client-id", ClientSecret: "client-secret", AutoProvision: boolPtr(true),
			RoleClaim: "roles", RoleClaimAllowedRoles: []string{core.RoleModerator},
		}}
		s.setupOIDCRoutes()
	})

	token := completeNoEmailOIDCHandshake(t, client, ts.URL, "oidc-roles", "/chat")
	flow, err := chattoCore.GetPendingExternalIdentityCreateFlow(t.Context(), token)
	if err != nil {
		t.Fatalf("GetPendingExternalIdentityCreateFlow: %v", err)
	}
	if !slices.Equal(flow.OIDCRoles, []string{core.RoleModerator}) {
		t.Fatalf("pending OIDC roles = %v, want only %q", flow.OIDCRoles, core.RoleModerator)
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
	roles            []string
	idTokenRoles     any
	userInfoRoles    any
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

func (i *noEmailOIDCIssuer) SetRoles(roles []string) {
	i.roles = append([]string(nil), roles...)
	i.idTokenRoles = i.roles
	i.userInfoRoles = i.roles
}

func (i *noEmailOIDCIssuer) SetRoleClaims(idTokenRoles, userInfoRoles any) {
	i.idTokenRoles = idTokenRoles
	i.userInfoRoles = userInfoRoles
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":                i.subject,
			"name":               "No Email User",
			"preferred_username": "no-email-user",
			"roles":              i.userInfoRoles,
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
		Roles         any    `json:"roles,omitempty"`
	}{
		Name:          "No Email User",
		PreferredUser: "no-email-user",
		Roles:         i.idTokenRoles,
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
