package http_server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	jose "github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	"github.com/markbates/goth"
	gothgithub "github.com/markbates/goth/providers/github"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
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
			RawData: map[string]any{"verified": false},
		})
		if unverified != "" {
			t.Fatalf("unverified discord email = %q, want empty", unverified)
		}
		verified := runtime.verifiedEmailFromGothUser(t.Context(), goth.User{
			Email:   "User@Example.com",
			RawData: map[string]any{"verified": true},
		})
		if verified != "user@example.com" {
			t.Fatalf("verified discord email = %q, want normalized email", verified)
		}
	})

	t.Run("google requires verified email flag", func(t *testing.T) {
		runtime := &authProviderRuntime{config: config.AuthProviderConfig{Type: config.AuthProviderTypeGoogle}}
		unverified := runtime.verifiedEmailFromGothUser(t.Context(), goth.User{
			Email:   "User@Example.com",
			RawData: map[string]any{"verified_email": false},
		})
		if unverified != "" {
			t.Fatalf("unverified google email = %q, want empty", unverified)
		}
		verified := runtime.verifiedEmailFromGothUser(t.Context(), goth.User{
			Email:   "User@Example.com",
			RawData: map[string]any{"verified_email": true},
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

func TestManagedOAuthUsernameFields(t *testing.T) {
	for _, tc := range []struct {
		name, providerType, profile, want string
	}{
		{"github login", "github", `{"id":1,"login":"github-user","name":"Display Name","email":"other@example.com"}`, "github-user"},
		{"gitlab username", "gitlab", `{"id":1,"username":"gitlab-user","name":"Display Name","email":"other@example.com"}`, "gitlab-user"},
		{"discord username", "discord", `{"id":"1","username":"discord-user","global_name":"Display Name","email":"other@example.com"}`, "discord-user"},
		{"google verified email", "google", `{"id":"1","name":"Alice","email":"Account.Name@example.com","verified_email":true}`, "account.name"},
		{"google unverified email", "google", `{"id":"1","name":"Alice","email":"account@example.com","verified_email":false}`, ""},
		{"google missing email", "google", `{"id":"1","name":"Alice"}`, ""},
		{"google malformed email", "google", `{"id":"1","name":"Alice","email":"account","verified_email":true}`, ""},
		{"github missing login", "github", `{"id":1,"name":"Alice","email":"other@example.com"}`, ""},
		{"gitlab missing username", "gitlab", `{"id":1,"name":"Alice","email":"other@example.com"}`, ""},
		{"discord missing username", "discord", `{"id":"1","global_name":"Alice","email":"other@example.com"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime, err := newAuthProviderRuntime(config.AuthProviderConfig{
				ID: tc.providerType, Type: tc.providerType, ClientID: "id", ClientSecret: "secret", ManageUsernames: true,
			}, "http://chat.example/callback")
			require.NoError(t, err)
			// Exercise Goth's actual JSON mappings; its common field names differ
			// between providers and are not a username contract on their own.
			runtime.httpClient.Transport = handlerRoundTripper{handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(req.URL.Path, "/emails") {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_, _ = w.Write([]byte(tc.profile))
			})}
			session, err := runtime.goth.UnmarshalSession(`{"AccessToken":"test-access-token"}`)
			require.NoError(t, err)
			user, err := runtime.goth.FetchUser(session)
			require.NoError(t, err)
			verifiedEmail := runtime.verifiedEmailFromGothUser(t.Context(), user)
			require.Equal(t, tc.want, runtime.managedUsernameFromGothUser(user, verifiedEmail))
			runtime.config.ManageUsernames = false
			require.Empty(t, runtime.managedUsernameFromGothUser(user, verifiedEmail))
		})
	}
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

	email, err := fetchGitHubVerifiedPrimaryEmail(t.Context(), newProviderHTTPClient(), "token-1")
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
		s.oauthClientResolveHook = func(_ context.Context, clientID string) (OAuthClient, bool, error) {
			if clientID != testOAuthClientID {
				return OAuthClient{}, false, nil
			}
			return OAuthClient{
				ClientID: clientID, ClientName: "Test Client", ClientURI: "https://client.example",
				RedirectURIs: []string{"https://client.example/servers/callback"},
			}, true, nil
		}
		s.config.Auth.Providers = []config.AuthProviderConfig{{
			ID:            "oidc-no-email",
			Type:          config.AuthProviderTypeOpenIDConnect,
			Label:         "No Email OIDC",
			IssuerURL:     issuer.URL(),
			ClientID:      "client-id",
			ClientSecret:  "client-secret",
			RequestEmail:  &requestEmail,
			AutoProvision: new(true),
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

	if err := chattoCore.GrantOAuthClientConsent(t.Context(), user.Id, testOAuthClientID, "Test Client", "https://client.example", "https://client.example"); err != nil {
		t.Fatalf("GrantOAuthClientConsent: %v", err)
	}
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	redirectURI := "https://client.example/servers/callback"
	state := "multi-server-provider-state"
	authorizeResp, err := client.Get(ts.URL + "/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
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
		ClientID:     testOAuthClientID,
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

	issuanceSubject := evtstream.UserAggregate(user.Id).Subject(evtstream.EventBearerTokenIssued)
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
	linkLocation := startNoEmailOIDC(t, client, ts.URL, "oidc-no-email", url.Values{
		"intent":     {"link"},
		"link_start": {linkStart},
	})
	linkState := authStateFromLocation(t, linkLocation)
	loginCookies := make(map[string]string)
	for _, cookie := range client.Jar.Cookies(serverURL) {
		if isBrowserSessionCookieName(cookie.Name) {
			loginCookies[cookie.Name] = cookie.Value
		}
	}
	if len(loginCookies) == 0 {
		t.Fatal("expected an existing browser login before linking")
	}
	if location := finishNoEmailOIDCCallback(t, client, ts.URL, "oidc-no-email", linkState); location != "/chat/-/settings/account" {
		t.Fatalf("link callback Location = %q, want account settings without confirmation", location)
	}
	for _, cookie := range client.Jar.Cookies(serverURL) {
		if isBrowserSessionCookieName(cookie.Name) && cookie.Value != loginCookies[cookie.Name] {
			t.Fatal("link callback replaced the existing browser login")
		}
	}
	if location := finishNoEmailOIDCCallback(t, client, ts.URL, "oidc-no-email", linkState); location != "/login?error=provider_failed" {
		t.Fatalf("replayed link callback Location = %q, want provider failure", location)
	}
	linked, err := chattoCore.GetUserByExternalIdentity(t.Context(), issuer.URL(), "subject-link")
	if err != nil {
		t.Fatalf("GetUserByExternalIdentity link: %v", err)
	}
	if linked == nil || linked.Id != linkUser.Id {
		t.Fatalf("linked no-email OIDC identity = %v, want %s", linked, linkUser.Id)
	}

	issuer.SetSubject("subject-conflict")
	// Failed account-link authorization must notify the original popup owner.
	const popupReturn = "/servers/callback?mode=provider&state=popup-state"
	for _, tc := range []struct{ providerError, wantError string }{
		{"access_denied", "provider_denied"},
		{"", "provider_failed"},
	} {
		failedStart, err := chattoCore.CreatePendingExternalIdentityLinkStart(t.Context(), "oidc-no-email", popupReturn, linkUser.Id)
		if err != nil {
			t.Fatalf("create failed link start: %v", err)
		}
		failedLocation := startNoEmailOIDC(t, client, ts.URL, "oidc-no-email", url.Values{
			"intent": {"link"}, "link_start": {failedStart},
		})
		issuer.tokenCheck = func(*http.Request) bool { return false }
		failedResponse, err := client.Get(ts.URL + "/auth/providers/oidc-no-email/callback?" + url.Values{
			"state": {authStateFromLocation(t, failedLocation)}, "error": {tc.providerError}, "code": {"test-code"},
		}.Encode())
		if err != nil {
			t.Fatalf("fail provider authorization: %v", err)
		}
		failedResponse.Body.Close()
		issuer.tokenCheck = nil
		failedReturn, err := url.Parse(failedResponse.Header.Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		if failedReturn.Path != "/servers/callback" || failedReturn.Query().Get("state") != "popup-state" || failedReturn.Query().Get("mode") != "provider" || failedReturn.Query().Get("error") != tc.wantError {
			t.Fatalf("failed link return = %q, want popup callback with %s", failedReturn, tc.wantError)
		}
	}

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

	if issuer.UserInfoRequests() != 0 {
		t.Fatal("unexpected userinfo fallback when names are present and email was not requested")
	}

	// A valid provider response cannot restore an account deleted during the flow.
	issuer.SetSubject("subject-deleted-target")
	deletedStart, err := chattoCore.CreatePendingExternalIdentityLinkStart(t.Context(), "oidc-no-email", "/chat/-/settings/account", linkUser.Id)
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityLinkStart deleted target: %v", err)
	}
	deletedLocation := startNoEmailOIDC(t, client, ts.URL, "oidc-no-email", url.Values{
		"intent":     {"link"},
		"link_start": {deletedStart},
	})
	if err := chattoCore.DeleteUser(t.Context(), linkUser.Id, linkUser.Id); err != nil {
		t.Fatalf("DeleteUser link target: %v", err)
	}
	if location := finishNoEmailOIDCCallback(t, client, ts.URL, "oidc-no-email", authStateFromLocation(t, deletedLocation)); location != "/chat/-/settings/account?error=provider_failed" {
		t.Fatalf("deleted target callback Location = %q, want provider failure", location)
	}
	if linked, err := chattoCore.GetUserByExternalIdentity(t.Context(), issuer.URL(), "subject-deleted-target"); err != nil || linked != nil {
		t.Fatalf("deleted target identity = %v, %v; want no link", linked, err)
	}
}

func TestOIDCAutoProvisionRequiresAndRedeemsInvitation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestEmail := false
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()

	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.AccountCreationPolicy = config.AccountCreationPolicyInviteOnly
		s.config.Auth.Providers = []config.AuthProviderConfig{{
			ID:            "oidc-invited",
			Type:          config.AuthProviderTypeOpenIDConnect,
			Label:         "Invited OIDC",
			IssuerURL:     issuer.URL(),
			ClientID:      "client-id",
			ClientSecret:  "client-secret",
			RequestEmail:  &requestEmail,
			AutoProvision: new(true),
		}}
		s.setupOIDCRoutes()
	})

	admin, err := chattoCore.CreateUser(t.Context(), core.SystemActorID, "oidc-invite-admin", "OIDC Invite Admin", "password123")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := chattoCore.AssignAdminRole(t.Context(), admin.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	maxUses := uint32(1)
	invitation, err := chattoCore.CreateInvitation(t.Context(), admin.Id, &maxUses, nil)
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	bindResp, err := client.Get(ts.URL + chattoCore.InvitationLinkPath(invitation.ID))
	if err != nil {
		t.Fatalf("GET invite link: %v", err)
	}
	bindResp.Body.Close()
	if bindResp.StatusCode != http.StatusSeeOther || bindResp.Header.Get("Location") != "/register?invited=1" {
		t.Fatalf("GET invite link = %d %q", bindResp.StatusCode, bindResp.Header.Get("Location"))
	}

	issuer.SetSubject("subject-invited-create")
	createToken := completeNoEmailOIDCHandshake(t, client, ts.URL, "oidc-invited", "/chat")
	flow, err := chattoCore.GetPendingExternalIdentityCreateFlow(t.Context(), createToken)
	if err != nil {
		t.Fatalf("GetPendingExternalIdentityCreateFlow: %v", err)
	}
	if flow.InvitationID != invitation.ID {
		t.Fatalf("flow invitation = %q, want %q", flow.InvitationID, invitation.ID)
	}
	if _, err := chattoCore.CreateUserForExternalIdentity(t.Context(), "oidc-invited-user", "OIDC Invited User", flow); err != nil {
		t.Fatalf("CreateUserForExternalIdentity: %v", err)
	}
	state, err := chattoCore.GetInvitation(t.Context(), admin.Id, invitation.ID)
	if err != nil || state.UseCount != 1 {
		t.Fatalf("invitation after SSO signup = %+v, %v; want one use", state, err)
	}

	issuer.SetSubject("subject-missing-invite")
	startLocation := startNoEmailOIDC(t, client, ts.URL, "oidc-invited", url.Values{"redirect": {"/chat"}})
	location := finishNoEmailOIDCCallback(t, client, ts.URL, "oidc-invited", authStateFromLocation(t, startLocation))
	if location != "/login?error=invalid_invitation" {
		t.Fatalf("uninvited auto-provision Location = %q, want invalid invitation", location)
	}
}

func TestOIDCProviderWithoutEmailIgnoresUserInfoFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestEmail := false
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	issuer.failUserInfo = true
	issuer.tokenClaims = map[string]any{"preferred_username": "no-email-user"}
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
			AutoProvision: new(true),
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

// The group restriction applies to new identities, to linked accounts at each
// sign-in, and to identity links.
func TestProviderGroupRestrictionAppliesToEverySignInAndLink(t *testing.T) {
	gin.SetMode(gin.TestMode)
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.Providers = []config.AuthProviderConfig{{
			ID: "restricted", Type: config.AuthProviderTypeOpenIDConnect, IssuerURL: issuer.URL(),
			ClientID: "client-id", ClientSecret: "client-secret", AutoProvision: new(true),
			AllowedGroups: []string{"chat-users"},
		}}
		s.setupOIDCRoutes()
	})
	inGroup := func(group string) {
		issuer.tokenClaims = map[string]any{"name": "Restricted User", "preferred_username": "restricted", "groups": []any{group}}
	}
	const denied = "/login?error=provider_access_denied"

	issuer.SetSubject("restricted-new")
	inGroup("other")
	if location := completeNoEmailOIDCLogin(t, client, ts.URL, "restricted", "/chat"); location != denied {
		t.Fatalf("new identity outside group Location = %q, want %q", location, denied)
	}
	inGroup("chat-users")
	if location := completeNoEmailOIDCLogin(t, client, ts.URL, "restricted", "/chat"); !strings.HasPrefix(location, "/sso/confirm?") {
		t.Fatalf("new identity in group Location = %q, want account confirmation", location)
	}

	linked, err := chattoCore.CreateUser(t.Context(), core.SystemActorID, "restricted-linked", "Restricted Linked", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := chattoCore.LinkExternalIdentity(t.Context(), "restricted", config.AuthProviderTypeOpenIDConnect, issuer.URL(), "restricted-linked", linked.Id); err != nil {
		t.Fatal(err)
	}
	issuer.SetSubject("restricted-linked")
	inGroup("other")
	if location := completeNoEmailOIDCLogin(t, client, ts.URL, "restricted", "/chat"); location != denied {
		t.Fatalf("linked account outside group Location = %q, want %q", location, denied)
	}
	inGroup("chat-users")
	if location := completeNoEmailOIDCLogin(t, client, ts.URL, "restricted", "/chat"); location != "/chat" {
		t.Fatalf("linked account in group Location = %q, want /chat", location)
	}

	target, err := chattoCore.CreateUser(t.Context(), core.SystemActorID, "restricted-target", "Restricted Target", "password123")
	if err != nil {
		t.Fatal(err)
	}
	linkStart, err := chattoCore.CreatePendingExternalIdentityLinkStart(t.Context(), "restricted", "/chat/-/settings/account", target.Id)
	if err != nil {
		t.Fatal(err)
	}
	issuer.SetSubject("restricted-link")
	inGroup("other")
	linkLocation := startNoEmailOIDC(t, client, ts.URL, "restricted", url.Values{"intent": {"link"}, "link_start": {linkStart}})
	if location := finishNoEmailOIDCCallback(t, client, ts.URL, "restricted", authStateFromLocation(t, linkLocation)); location != "/chat/-/settings/account?error=provider_access_denied" {
		t.Fatalf("link outside group Location = %q, want account settings error", location)
	}
	if identities, err := chattoCore.ExternalIdentitiesForUser(t.Context(), target.Id); err != nil || len(identities) != 0 {
		t.Fatalf("ExternalIdentitiesForUser = %v, %v; want no link", identities, err)
	}
}

// Hash-only links become usable alternatives after a verified sign-in, without
// requiring a groups claim or group-role mappings.
func TestOIDCLoginCompletesLegacyLinkWithoutGroupRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Auth.DirectLogin = new(false)
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.Providers = []config.AuthProviderConfig{{
			ID: "company", Type: config.AuthProviderTypeOpenIDConnect, IssuerURL: issuer.URL(),
			ClientID: "client-id", ClientSecret: "client-secret",
		}}
		s.setupOIDCRoutes()
	})
	ctx := t.Context()
	user, err := chattoCore.CreateUser(ctx, core.SystemActorID, "legacy-login", "Legacy Login", "")
	require.NoError(t, err)
	issuer.SetSubject("legacy-subject")
	digest := sha256.Sum256([]byte(issuer.URL() + ":legacy-subject"))
	hash := hex.EncodeToString(digest[:])
	legacy := &evtv1.Event{
		Id: core.NewEventID(), ActorId: core.SystemActorID, CreatedAt: timestamppb.Now(),
		Event: &evtv1.Event_UserOidcSubjectLinked{UserOidcSubjectLinked: &evtv1.UserOIDCSubjectLinkedEvent{
			UserId: user.Id, SubjectHash: hash,
		}},
	}
	_, err = chattoCore.EventPublisher.AppendEventually(ctx, evtstream.UserAggregate(user.Id).SubjectFor(legacy), legacy)
	require.NoError(t, err)
	// The next guarded user mutation catches up the legacy event before returning.
	github, err := chattoCore.LinkExternalIdentityAs(ctx, core.SystemActorID, "github", "github", "github", "github-subject", user.Id)
	require.NoError(t, err)
	policy := core.ExternalIdentitySignInPolicy{Issuers: []string{issuer.URL(), "github"}}
	require.ErrorIs(t, chattoCore.DisconnectExternalIdentityAs(ctx, user.Id, user.Id, github.SubjectHash, policy), core.ErrExternalIdentityLastMethod)
	for range 2 {
		require.Equal(t, "/chat", completeNoEmailOIDCLogin(t, client, ts.URL, "company", "/chat"))
	}
	identities, err := chattoCore.ExternalIdentitiesForUser(ctx, user.Id)
	require.NoError(t, err)
	index := slices.IndexFunc(identities, func(identity core.ExternalIdentity) bool { return identity.SubjectHash == hash })
	require.NotEqual(t, -1, index)
	require.Equal(t, issuer.URL(), identities[index].Issuer)
	require.Equal(t, "company", identities[index].ProviderID)
	require.True(t, policy.CanDisconnect(github.SubjectHash, identities, false))
	require.NoError(t, chattoCore.DisconnectExternalIdentityAs(ctx, user.Id, user.Id, github.SubjectHash, policy))
	linkedEvents, _, err := chattoCore.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventUserExternalIdentityLinked))
	require.NoError(t, err)
	require.Len(t, linkedEvents, 2, "one GitHub link and one idempotent OIDC completion")
}

// group_roles applies at sign-in, at identity links, and carries the groups
// to account creation. Only provider assignments are revoked.
func TestProviderGroupRolesSyncAtSignInAndLink(t *testing.T) {
	gin.SetMode(gin.TestMode)
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.Providers = []config.AuthProviderConfig{{
			ID: "company", Type: config.AuthProviderTypeOpenIDConnect, IssuerURL: issuer.URL(),
			ClientID: "client-id", ClientSecret: "client-secret", AutoProvision: new(true),
			GroupRoles: "chat-mods:moderator",
		}}
		s.setupOIDCRoutes()
	})
	inGroups := func(groups ...any) {
		// A present, empty list differs from a missing claim.
		issuer.tokenClaims = map[string]any{"name": "Group User", "preferred_username": "group-user", "groups": append([]any{}, groups...)}
	}
	hasModerator := func(userID string) bool {
		t.Helper()
		roles, err := chattoCore.GetUserRoles(t.Context(), userID)
		if err != nil {
			t.Fatal(err)
		}
		return slices.Contains(roles, core.RoleModerator)
	}

	// A new identity carries its mapped groups, and no others, to account creation.
	issuer.SetSubject("group-new")
	inGroups("chat-mods", "unmapped-team")
	location := completeNoEmailOIDCLogin(t, client, ts.URL, "company", "/chat")
	confirmURL, err := url.Parse(location)
	if err != nil || confirmURL.Path != "/sso/confirm" {
		t.Fatalf("new identity Location = %q, want account confirmation", location)
	}
	flow, err := chattoCore.GetPendingExternalIdentityCreateFlow(t.Context(), confirmURL.Query().Get("token"))
	if err != nil || !flow.GroupsKnown || !slices.Equal(flow.Groups, []string{"chat-mods"}) {
		t.Fatalf("pending flow groups = %v (known %v), %v", flow.Groups, flow.GroupsKnown, err)
	}

	// Sign-in grants and later revokes the provider assignment.
	user, err := chattoCore.CreateUser(t.Context(), core.SystemActorID, "group-linked", "Group Linked", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := chattoCore.LinkExternalIdentity(t.Context(), "company", config.AuthProviderTypeOpenIDConnect, issuer.URL(), "group-linked", user.Id); err != nil {
		t.Fatal(err)
	}
	issuer.SetSubject("group-linked")
	if location := completeNoEmailOIDCLogin(t, client, ts.URL, "company", "/chat"); location != "/chat" || !hasModerator(user.Id) {
		t.Fatalf("sign-in in group Location = %q, moderator = %v", location, hasModerator(user.Id))
	}
	inGroups()
	if location := completeNoEmailOIDCLogin(t, client, ts.URL, "company", "/chat"); location != "/chat" || hasModerator(user.Id) {
		t.Fatalf("sign-in without group Location = %q, moderator = %v", location, hasModerator(user.Id))
	}

	// Linking the provider grants the role to the linked account.
	target, err := chattoCore.CreateUser(t.Context(), core.SystemActorID, "group-target", "Group Target", "password123")
	if err != nil {
		t.Fatal(err)
	}
	linkStart, err := chattoCore.CreatePendingExternalIdentityLinkStart(t.Context(), "company", "/chat/-/settings/account", target.Id)
	if err != nil {
		t.Fatal(err)
	}
	issuer.SetSubject("group-link")
	inGroups("chat-mods")
	linkLocation := startNoEmailOIDC(t, client, ts.URL, "company", url.Values{"intent": {"link"}, "link_start": {linkStart}})
	if location := finishNoEmailOIDCCallback(t, client, ts.URL, "company", authStateFromLocation(t, linkLocation)); location != "/chat/-/settings/account" || !hasModerator(target.Id) {
		t.Fatalf("link Location = %q, moderator = %v", location, hasModerator(target.Id))
	}
}

// A denied sign-in revokes the roles that the provider gave and grants none.
func TestProviderGroupRolesRevokedWhenSignInDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.Providers = []config.AuthProviderConfig{{
			ID: "company", Type: config.AuthProviderTypeOpenIDConnect, IssuerURL: issuer.URL(),
			ClientID: "client-id", ClientSecret: "client-secret",
			AllowedGroups: []string{"chat-users"}, GroupRoles: "chat-mods:moderator",
		}}
		s.setupOIDCRoutes()
	})
	user, err := chattoCore.CreateUser(t.Context(), core.SystemActorID, "denied-member", "Denied Member", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := chattoCore.LinkExternalIdentity(t.Context(), "company", config.AuthProviderTypeOpenIDConnect, issuer.URL(), "denied-member", user.Id); err != nil {
		t.Fatal(err)
	}
	issuer.SetSubject("denied-member")
	hasModerator := func() bool {
		t.Helper()
		roles, err := chattoCore.GetUserRoles(t.Context(), user.Id)
		if err != nil {
			t.Fatal(err)
		}
		return slices.Contains(roles, core.RoleModerator)
	}
	issuer.tokenClaims = map[string]any{"name": "Denied", "preferred_username": "denied", "groups": []any{"chat-users", "chat-mods"}}
	if location := completeNoEmailOIDCLogin(t, client, ts.URL, "company", "/chat"); location != "/chat" || !hasModerator() {
		t.Fatalf("allowed sign-in Location = %q, moderator = %v", location, hasModerator())
	}
	// Still in the mapped group, but no longer allowed to sign in.
	issuer.tokenClaims = map[string]any{"name": "Denied", "preferred_username": "denied", "groups": []any{"chat-mods"}}
	if location := completeNoEmailOIDCLogin(t, client, ts.URL, "company", "/chat"); location != "/login?error=provider_access_denied" || hasModerator() {
		t.Fatalf("denied sign-in Location = %q, moderator = %v", location, hasModerator())
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
	if authURL.Query().Get("nonce") == "" {
		t.Fatalf("provider start Location = %q, missing nonce", location)
	}
	// Visit the issuer as the browser would, so it binds the nonce to the code.
	authorizeResp, err := client.Get(location)
	if err != nil {
		t.Fatalf("GET issuer authorization: %v", err)
	}
	authorizeResp.Body.Close()
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
	tokenClaims      map[string]any
	userInfoClaims   map[string]any
	methods          json.RawMessage
	tokenRequests    int
	tokenCheck       func(*http.Request) bool
	extraKeys        []any
	omitSigningKey   bool
	// nonce is the ID-token nonce. Authorization requests replace it.
	nonce string
	// blockPath names an endpoint that does not answer before Close.
	blockPath       string
	blockedRequests atomic.Int32
	release         chan struct{}
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
		nonce:    "test-nonce",
		release:  make(chan struct{}),
	}
	issuer.server = httptest.NewServer(http.HandlerFunc(issuer.ServeHTTP))
	return issuer
}

func (i *noEmailOIDCIssuer) Close() {
	close(i.release)
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
	if i.blockPath != "" && r.URL.Path == i.blockPath {
		i.blockedRequests.Add(1)
		<-i.release
		return
	}
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		w.Header().Set("Content-Type", "application/json")
		metadata := map[string]any{
			"issuer":                 i.server.URL,
			"authorization_endpoint": i.server.URL + "/authorize",
			"token_endpoint":         i.server.URL + "/token",
			"jwks_uri":               i.server.URL + "/keys",
			"userinfo_endpoint":      i.server.URL + "/userinfo",
		}
		if i.methods != nil {
			metadata["token_endpoint_auth_methods_supported"] = i.methods
		}
		_ = json.NewEncoder(w).Encode(metadata)
	case "/authorize":
		i.nonce = r.URL.Query().Get("nonce")
		http.Redirect(w, r, r.URL.Query().Get("redirect_uri")+"?state="+url.QueryEscape(r.URL.Query().Get("state"))+"&code=test-code", http.StatusTemporaryRedirect)
	case "/token":
		i.tokenRequests++
		if i.tokenCheck != nil && !i.tokenCheck(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"private provider response"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     i.idToken(r.Context()),
		})
	case "/keys":
		w.Header().Set("Content-Type", "application/json")
		keys := append([]any(nil), i.extraKeys...)
		if !i.omitSigningKey {
			keys = append(keys, jose.JSONWebKey{
				Key:       &i.key.PublicKey,
				KeyID:     "test-key",
				Algorithm: string(jose.RS256),
				Use:       "sig",
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	case "/userinfo":
		i.userInfoRequests++
		if i.failUserInfo {
			http.Error(w, "userinfo unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if i.userInfoClaims != nil {
			_ = json.NewEncoder(w).Encode(i.userInfoClaims)
			return
		}
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
	profileClaims := i.tokenClaims
	if profileClaims == nil {
		profileClaims = map[string]any{"name": "No Email User", "preferred_username": "no-email-user"}
	}
	builder := josejwt.Signed(signer).Claims(claims).Claims(profileClaims)
	if i.nonce != "" {
		builder = builder.Claims(map[string]any{"nonce": i.nonce})
	}
	raw, err := builder.Serialize()
	if err != nil {
		panic(err)
	}
	return raw
}
