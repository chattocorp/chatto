package http_server

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/gin-gonic/gin"
)

// A provider re-verification marks the browser's current cookie session
// fresh. It creates no session and rejects an identity of another account,
// a provider sign-in that did not happen during the attempt, and a token
// from another provider type.
func TestOIDCFreshAuthReverification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestEmail := false
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()

	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.Providers = []config.AuthProviderConfig{{
			ID:           "sso",
			Type:         config.AuthProviderTypeOpenIDConnect,
			Label:        "SSO",
			IssuerURL:    issuer.URL(),
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			RequestEmail: &requestEmail,
		}}
		s.setupOIDCRoutes()
	})
	ctx := t.Context()

	owner, err := chattoCore.CreateUser(ctx, core.SystemActorID, "reverify-owner", "Owner", "")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	other, err := chattoCore.CreateUser(ctx, core.SystemActorID, "reverify-other", "Other", "")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	if err := chattoCore.LinkExternalIdentity(ctx, "sso", config.AuthProviderTypeOpenIDConnect, issuer.URL(), "subject-owner", owner.Id); err != nil {
		t.Fatalf("LinkExternalIdentity owner: %v", err)
	}
	if err := chattoCore.LinkExternalIdentity(ctx, "sso", config.AuthProviderTypeOpenIDConnect, issuer.URL(), "subject-other", other.Id); err != nil {
		t.Fatalf("LinkExternalIdentity other: %v", err)
	}

	issuer.SetSubject("subject-owner")
	if location := completeNoEmailOIDCLogin(t, client, ts.URL, "sso", "/chat"); location != "/chat" {
		t.Fatalf("login Location = %q, want /chat", location)
	}
	serverURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	cookieHandle := ""
	for _, cookie := range client.Jar.Cookies(serverURL) {
		if isBrowserSessionCookieName(cookie.Name) {
			cookieHandle = cookie.Value
		}
	}
	if cookieHandle == "" {
		t.Fatal("login did not set a browser session cookie")
	}
	freshAuthSource := func() string {
		t.Helper()
		credential, err := chattoCore.ValidatePresentedRuntimeCredential(ctx, cookieHandle, core.AuthTokenPresentationCookie)
		if err != nil {
			t.Fatalf("ValidatePresentedRuntimeCredential: %v", err)
		}
		return credential.FreshAuthSource
	}
	loginSource := freshAuthSource()

	const settingsPath = "/chat/-/settings/account?link_provider=other"
	reverify := func(t *testing.T, subject string, authTime any) string {
		t.Helper()
		token, err := chattoCore.CreateFreshAuthReverificationStart(ctx, "sso", settingsPath, cookieHandle, core.AuthTokenPresentationCookie)
		if err != nil {
			t.Fatalf("CreateFreshAuthReverificationStart: %v", err)
		}
		issuer.SetSubject(subject)
		issuer.tokenClaims = map[string]any{"name": "Owner", "preferred_username": "owner"}
		if authTime != nil {
			issuer.tokenClaims["auth_time"] = authTime
		}
		location := startNoEmailOIDC(t, client, ts.URL, "sso", url.Values{"intent": {"reverify"}, "reverify_start": {token}})
		authURL, err := url.Parse(location)
		if err != nil {
			t.Fatal(err)
		}
		if got := authURL.Query(); got.Get("prompt") != "login" || got.Get("max_age") != "0" {
			t.Fatalf("re-verification authorization query = %q, want prompt=login and max_age=0", authURL.RawQuery)
		}
		return finishNoEmailOIDCCallback(t, client, ts.URL, "sso", authStateFromLocation(t, location))
	}
	assertReturn := func(t *testing.T, location, param, value string) {
		t.Helper()
		got, err := url.Parse(location)
		if err != nil {
			t.Fatal(err)
		}
		if got.Path != "/chat/-/settings/account" || got.Query().Get("link_provider") != "other" || got.Query().Get(param) != value {
			t.Fatalf("callback Location = %q, want settings path with %s=%s", location, param, value)
		}
	}

	t.Run("identity of another account", func(t *testing.T) {
		location := reverify(t, "subject-other", time.Now().Unix())
		assertReturn(t, location, "error", "reverification_mismatch")
		if got := freshAuthSource(); got != loginSource {
			t.Fatalf("FreshAuthSource = %q, want unchanged %q", got, loginSource)
		}
	})

	t.Run("earlier provider sign-in", func(t *testing.T) {
		location := reverify(t, "subject-owner", time.Now().Add(-time.Hour).Unix())
		assertReturn(t, location, "error", "reverification_failed")
		if got := freshAuthSource(); got != loginSource {
			t.Fatalf("FreshAuthSource = %q, want unchanged %q", got, loginSource)
		}
	})

	t.Run("missing auth_time", func(t *testing.T) {
		location := reverify(t, "subject-owner", nil)
		assertReturn(t, location, "error", "reverification_failed")
	})

	t.Run("same account", func(t *testing.T) {
		location := reverify(t, "subject-owner", time.Now().Unix())
		assertReturn(t, location, "reverified", "1")
		if got := freshAuthSource(); got != "external_identity_reverification" {
			t.Fatalf("FreshAuthSource = %q, want external_identity_reverification", got)
		}
		for _, cookie := range client.Jar.Cookies(serverURL) {
			if isBrowserSessionCookieName(cookie.Name) && cookie.Value != cookieHandle {
				t.Fatalf("re-verification replaced the browser session cookie %s", cookie.Name)
			}
		}
	})

	t.Run("token is single use", func(t *testing.T) {
		token, err := chattoCore.CreateFreshAuthReverificationStart(ctx, "sso", settingsPath, cookieHandle, core.AuthTokenPresentationCookie)
		if err != nil {
			t.Fatalf("CreateFreshAuthReverificationStart: %v", err)
		}
		if _, err := chattoCore.ConsumeFreshAuthReverificationStart(ctx, token); err != nil {
			t.Fatalf("ConsumeFreshAuthReverificationStart: %v", err)
		}
		resp, err := client.Get(ts.URL + "/auth/providers/sso?" + url.Values{"intent": {"reverify"}, "reverify_start": {token}}.Encode())
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusTemporaryRedirect || resp.Header.Get("Location") != "/login?error=provider_failed" {
			t.Fatalf("consumed token start = %d %q, want provider failure", resp.StatusCode, resp.Header.Get("Location"))
		}
	})
}
