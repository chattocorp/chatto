package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/chattocorp/chatto/authling/internal/config"
	"github.com/chattocorp/chatto/authling/internal/web"
)

// A configured client on a public HTTPS issuer can complete the authorization
// code flow with a plain-HTTP loopback redirect URI, as a relying party's local
// development server does. The provider still matches the redirect URI
// exactly, so a different port or host is not a valid redirect target.
func TestOIDCConfiguredClientCompletesFlowWithLoopbackHTTPRedirectOnHTTPSIssuer(t *testing.T) {
	const origin = "https://auth.example"
	const redirectURI = "http://localhost:5173/auth/callback"
	const secret = "a-confidential-client-secret-of-sufficient-length"
	cfg := embeddedTestConfig(t)
	cfg.HTTP = config.HTTPConfig{PublicURL: origin}
	cfg.OIDC.Clients = []config.OIDCClientConfig{
		{ID: "public-client", Name: "Public Client", RedirectURIs: []string{redirectURI}},
		{ID: "confidential-client", Name: "Confidential Client", Secret: secret, RedirectURIs: []string{redirectURI}},
	}
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	if _, err := runtime.Accounts.CreateLocal(testContext(t), "oidc@example.com", "a deliberately uncommon password"); err != nil {
		t.Fatal(err)
	}
	handler := web.Handler(web.Dependencies{Accounts: runtime.Accounts, Authentication: runtime.Authentication, Sessions: runtime.Sessions, Authorizations: runtime.Authorizations, OIDC: runtime.OIDC, PublicURL: origin})
	request := func(method, path, body string, cookie *http.Cookie, basicAuth ...string) *httptest.ResponseRecorder {
		t.Helper()
		target := path
		if strings.HasPrefix(path, "/") {
			target = origin + path
		}
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", origin)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if len(basicAuth) == 2 {
			req.SetBasicAuth(basicAuth[0], basicAuth[1])
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	verifier := strings.Repeat("v", 43)
	authorizeQuery := func(clientID, redirect string) string {
		return url.Values{"client_id": {clientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid"}, "state": {"state-value"}, "code_challenge": {"7w_YNF9DSfIdPf_pRjSq646_kPr-2-o9NAl16JGghdM"}, "code_challenge_method": {"S256"}}.Encode()
	}

	var cookie *http.Cookie
	for _, clientID := range []string{"public-client", "confidential-client"} {
		t.Run(clientID, func(t *testing.T) {
			authorize := request(http.MethodGet, "/oauth/authorize?"+authorizeQuery(clientID, redirectURI), "", cookie)
			location := authorize.Header().Get("Location")
			if authorize.Code < 300 || authorize.Code >= 400 || !strings.HasPrefix(location, "/oidc/consent?id=") {
				t.Fatalf("authorize status/location/body = %d %q %s", authorize.Code, location, authorize.Body.String())
			}
			parsed, _ := url.Parse(location)
			requestID := parsed.Query().Get("id")
			if cookie == nil {
				login := request(http.MethodPost, "/login", url.Values{"email": {"oidc@example.com"}, "password": {"a deliberately uncommon password"}, "oidc_request": {requestID}}.Encode(), nil)
				if login.Code != http.StatusSeeOther || len(login.Result().Cookies()) != 1 {
					t.Fatalf("login status/body = %d %s", login.Code, login.Body.String())
				}
				cookie = login.Result().Cookies()[0]
			}
			consent := request(http.MethodPost, "/oidc/consent", url.Values{"id": {requestID}, "decision": {"allow"}, "consent_version": {"3"}}.Encode(), cookie)
			if consent.Code != http.StatusSeeOther {
				t.Fatalf("consent status/body = %d %s", consent.Code, consent.Body.String())
			}
			callback := request(http.MethodGet, consent.Header().Get("Location"), "", cookie)
			redirect, err := url.Parse(callback.Header().Get("Location"))
			if err != nil || redirect.Scheme+"://"+redirect.Host+redirect.Path != redirectURI || redirect.Query().Get("state") != "state-value" || redirect.Query().Get("code") == "" {
				t.Fatalf("callback status/location = %d %q, error = %v", callback.Code, callback.Header().Get("Location"), err)
			}
			form := url.Values{"grant_type": {"authorization_code"}, "redirect_uri": {redirectURI}, "code": {redirect.Query().Get("code")}, "code_verifier": {verifier}}
			var token *httptest.ResponseRecorder
			if clientID == "confidential-client" {
				token = request(http.MethodPost, "/oauth/token", form.Encode(), nil, clientID, secret)
			} else {
				form.Set("client_id", clientID)
				token = request(http.MethodPost, "/oauth/token", form.Encode(), nil)
			}
			if token.Code != http.StatusOK || !strings.Contains(token.Body.String(), `"id_token"`) {
				t.Fatalf("token status/body = %d %s", token.Code, token.Body.String())
			}
		})
	}

	// Exact matching: no RFC 8252 "any port" matching, no other loopback name.
	for _, redirect := range []string{"http://localhost:5174/auth/callback", "http://127.0.0.1:5173/auth/callback", "http://localhost:5173/auth/callback/"} {
		t.Run("unregistered "+redirect, func(t *testing.T) {
			response := request(http.MethodGet, "/oauth/authorize?"+authorizeQuery("public-client", redirect), "", cookie)
			if location := response.Header().Get("Location"); strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "/oidc/consent") {
				t.Fatalf("unregistered redirect accepted: %d %q", response.Code, location)
			}
		})
	}
}
