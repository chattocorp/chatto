package http_server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	authv1 "hmans.de/chatto/internal/pb/chatto/auth/v1"
	"hmans.de/chatto/internal/pb/chatto/auth/v1/authv1connect"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/internal/testutil"
)

const testOAuthClientID = "https://client.example/oauth/client-metadata.json"

// setupOAuthServer creates a minimal HTTPServer with session middleware and OAuth endpoints.
func setupOAuthServer(t *testing.T) *HTTPServer {
	return setupOAuthServerWithTokenTTL(t, 0)
}

func setupOAuthServerWithTokenTTL(t *testing.T, tokenTTL time.Duration) *HTTPServer {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartSharedNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	chattoCore, err := core.NewChattoCore(ctx, nc, config.CoreConfig{AuthTokenTTL: tokenTTL})
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

	// Create router with session middleware (required for OAuth authorize flow)
	router := gin.New()
	sessionStore := cookie.NewStore([]byte("test-secret-key-32-bytes-long!!"))
	sessionStore.Options(sessions.Options{
		MaxAge:   86400,
		HttpOnly: true,
		Secure:   false,
		Path:     "/",
	})
	router.Use(sessions.Sessions("chatto_session", sessionStore))

	s := &HTTPServer{
		config: config.ChattoConfig{
			Webserver: config.WebserverConfig{
				URL: "https://chatto.example",
			},
			Auth: config.AuthConfig{TokenTTL: config.Duration(tokenTTL)},
		},
		nc:      nc,
		router:  router,
		core:    chattoCore,
		version: "test",
	}
	browserStore := newJetStreamBrowserSessionStore(chattoCore)
	s.browserSessions = newBrowserSessionManager(browserStore, s.config.Auth.TokenTTLOrDefault(), false)
	s.oauthClientResolveHook = func(_ context.Context, clientID string) (OAuthClient, bool, error) {
		if clientID != testOAuthClientID {
			return OAuthClient{}, false, nil
		}
		return OAuthClient{
			ClientID: clientID, ClientName: "Test Client", ClientURI: "https://client.example",
			RedirectURIs: []string{
				"https://chatto.example/servers/callback",
				"https://client.example/callback",
				"https://client.example/servers/callback",
				"https://client.example/servers/callback?mode=popup",
				"https://first.example/servers/callback",
				"https://second.example/servers/callback",
			},
		}, true, nil
	}
	s.setupOAuthRoutes()

	return s
}

func TestInjectUserIntoContextDoesNotRenewCookieCredential(t *testing.T) {
	s := setupOAuthServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	user, err := s.core.CreateUser(ctx, core.SystemActorID, "rotated-context-user", "Rotated Context User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID, created, err := s.core.CreateCookieSession(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}

	s.router.GET("/test/renewed-cookie-context", func(c *gin.Context) {
		request := s.injectUserIntoContext(c)
		credential, ok := authctx.CredentialForContext(request.Context())
		if !ok {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.String(http.StatusOK, credential.Handle)
	})

	req := httptest.NewRequest(http.MethodGet, "/test/renewed-cookie-context", nil)
	req.AddCookie(&http.Cookie{Name: browserSessionCookieName, Value: sessionID})
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("rotation status = %d, want 200: %s", w.Code, w.Body.String())
	}
	renewedSessionID := w.Body.String()
	if renewedSessionID != sessionID {
		t.Fatalf("request context handle = %q, want stable handle %q", renewedSessionID, sessionID)
	}
	renewed, err := s.core.ValidateCookieCredential(ctx, sessionID)
	if err != nil {
		t.Fatalf("renewed cookie session validation: %v", err)
	}
	if !renewed.GetExpiresAt().AsTime().Equal(created.GetExpiresAt().AsTime()) {
		t.Fatalf("validated expiry = %v, want unchanged %v", renewed.GetExpiresAt(), created.GetExpiresAt())
	}
}

func loginOAuthTestUser(t *testing.T, s *HTTPServer, login string) ([]*http.Cookie, *evtv1.User) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	user, err := s.core.CreateUser(ctx, "", login, "OAuth Test User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	s.router.GET("/test/login-"+login, func(c *gin.Context) {
		if err := s.createCookieSession(c, user.Id, "test"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest("GET", "/test/login-"+login, nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("login fixture status = %d, want 204: %s", w.Code, w.Body.String())
	}
	return w.Result().Cookies(), user
}

func addCookies(req *http.Request, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
}

func mergeCookies(cookies []*http.Cookie, updates []*http.Cookie) []*http.Cookie {
	byName := make(map[string]*http.Cookie, len(cookies)+len(updates))
	for _, cookie := range cookies {
		byName[cookie.Name] = cookie
	}
	for _, cookie := range updates {
		byName[cookie.Name] = cookie
	}
	out := make([]*http.Cookie, 0, len(byName))
	for _, cookie := range byName {
		out = append(out, cookie)
	}
	return out
}

func TestOAuthAuthorize_ValidParams(t *testing.T) {
	s := setupOAuthServer(t)

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := core.GenerateCodeChallenge(verifier)

	req := httptest.NewRequest("GET", "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {"https://chatto.example/servers/callback"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"random123"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Should redirect to the login page (307)
	if w.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected 307, got %d", w.Code)
	}

	location := w.Header().Get("Location")
	if !strings.HasPrefix(location, "/login?redirect=") || !strings.Contains(location, "oauth%2Fauthorize") {
		t.Errorf("expected redirect to /login?redirect=...oauth/authorize..., got %q", location)
	}
}

func TestOAuthAuthorize_UsesConfiguredProviderHint(t *testing.T) {
	s := setupOAuthServer(t)
	s.config.Auth.Providers = []config.AuthProviderConfig{
		{ID: "authling", Type: config.AuthProviderTypeOpenIDConnect},
	}

	requestURL := "/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {"https://chatto.example/servers/callback"},
		"code_challenge":        {core.GenerateCodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")},
		"code_challenge_method": {"S256"},
		"state":                 {"random123"},
		"provider_id":           {"authling"},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, requestURL, nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("authorize status = %d, want 307", w.Code)
	}
	if location := w.Header().Get("Location"); location != "/auth/providers/authling?redirect=%2Foauth%2Fauthorize" {
		t.Fatalf("authorize Location = %q, want configured provider start", location)
	}
}

func TestOAuthAuthorize_IgnoresUnknownProviderHint(t *testing.T) {
	s := setupOAuthServer(t)

	requestURL := "/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {"https://chatto.example/servers/callback"},
		"code_challenge":        {core.GenerateCodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")},
		"code_challenge_method": {"S256"},
		"state":                 {"random123"},
		"provider_id":           {"not-configured"},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, requestURL, nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("authorize status = %d, want 307", w.Code)
	}
	if location := w.Header().Get("Location"); !strings.HasPrefix(location, "/login?redirect=") {
		t.Fatalf("authorize Location = %q, want regular login", location)
	}
}

func TestOAuthAuthorize_ReturnsUnavailableWhenCookieValidationStorageFails(t *testing.T) {
	s := setupOAuthServer(t)
	cookies, _ := loginOAuthTestUser(t, s, "oauth-storage-unavailable")

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {"https://chatto.example/servers/callback"},
		"code_challenge":        {core.GenerateCodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")},
		"code_challenge_method": {"S256"},
	}.Encode(), nil)
	addCookies(req, cookies)
	canceled, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(canceled)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("authorize status = %d, want 503: %s", w.Code, w.Body.String())
	}
	if location := w.Header().Get("Location"); location != "" {
		t.Fatalf("authorize redirected during storage failure: %q", location)
	}
}

func TestOAuthAuthorize_MissingParams(t *testing.T) {
	s := setupOAuthServer(t)

	tests := []struct {
		name   string
		params url.Values
		errMsg string
	}{
		{
			"missing response_type",
			url.Values{
				"client_id":             {testOAuthClientID},
				"redirect_uri":          {"https://chatto.example/servers/callback"},
				"code_challenge":        {"challenge"},
				"code_challenge_method": {"S256"},
			},
			"unsupported_response_type",
		},
		{
			"missing client_id",
			url.Values{
				"response_type":         {"code"},
				"redirect_uri":          {"https://chatto.example/servers/callback"},
				"code_challenge":        {"challenge"},
				"code_challenge_method": {"S256"},
			},
			"invalid_request",
		},
		{
			"missing redirect_uri",
			url.Values{
				"response_type":         {"code"},
				"client_id":             {testOAuthClientID},
				"code_challenge":        {"challenge"},
				"code_challenge_method": {"S256"},
			},
			"invalid_request",
		},
		{
			"missing code_challenge",
			url.Values{
				"response_type":         {"code"},
				"client_id":             {testOAuthClientID},
				"redirect_uri":          {"https://chatto.example/servers/callback"},
				"code_challenge_method": {"S256"},
			},
			"invalid_request",
		},
		{
			"wrong code_challenge_method",
			url.Values{
				"response_type":         {"code"},
				"client_id":             {testOAuthClientID},
				"redirect_uri":          {"https://chatto.example/servers/callback"},
				"code_challenge":        {"challenge"},
				"code_challenge_method": {"plain"},
			},
			"invalid_request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/oauth/authorize?"+tt.params.Encode(), nil)
			w := httptest.NewRecorder()
			s.router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}

			var resp map[string]string
			json.Unmarshal(w.Body.Bytes(), &resp)
			if resp["error"] != tt.errMsg {
				t.Errorf("expected error %q, got %q", tt.errMsg, resp["error"])
			}
		})
	}
}

func TestOAuthAuthorize_InvalidRedirectURI(t *testing.T) {
	s := setupOAuthServer(t)

	tests := []struct {
		name        string
		redirectURI string
	}{
		{"plain HTTP", "http://example.com/callback"},
		{"unconfigured HTTPS origin", "https://evil.example/callback"},
		{"no scheme", "example.com/callback"},
		{"ftp scheme", "ftp://example.com/callback"},
		{"fragment", "https://chatto.example/callback#frag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/oauth/authorize?"+url.Values{
				"response_type":         {"code"},
				"client_id":             {testOAuthClientID},
				"redirect_uri":          {tt.redirectURI},
				"code_challenge":        {"challenge"},
				"code_challenge_method": {"S256"},
			}.Encode(), nil)
			w := httptest.NewRecorder()
			s.router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for redirect_uri %q, got %d", tt.redirectURI, w.Code)
			}
		})
	}
}

func TestOAuthAuthorize_AllowsExactCIMDRedirectWithoutOriginConfiguration(t *testing.T) {
	s := setupOAuthServer(t)
	clientID, metadataServer := newOAuthCIMDTestServer(t, "https://client.example/servers/callback?mode=popup")
	resolver, err := newOAuthClientResolver("http://localhost:4000", metadataServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	s.oauthClientResolver = resolver

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {"https://client.example/servers/callback?mode=popup"},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want 307: %s", w.Code, w.Body.String())
	}
}

func TestOAuthAuthorize_RejectsBlockedClient(t *testing.T) {
	s := setupOAuthServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	admin, err := s.core.CreateUser(ctx, core.SystemActorID, "blocked-client-admin", "Blocked Client Admin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.core.AssignAdminRole(ctx, admin.Id); err != nil {
		t.Fatal(err)
	}
	if err := s.core.RecordOAuthClientAuthorization(ctx, admin.Id, testOAuthClientID, "Test Client", "https://client.example", "https://client.example", evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatal(err)
	}
	if _, err := s.core.UpdateOAuthClientPolicy(ctx, admin.Id, testOAuthClientID, evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {"https://client.example/callback"},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"invalid_client"`) {
		t.Fatalf("blocked authorize status/body = %d/%s", w.Code, w.Body.String())
	}
}

func TestOAuthAuthorize_TrustedClientStillRequiresUserConsent(t *testing.T) {
	s := setupOAuthServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	admin, err := s.core.CreateUser(ctx, core.SystemActorID, "trusted-client-admin", "Trusted Client Admin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.core.AssignAdminRole(ctx, admin.Id); err != nil {
		t.Fatal(err)
	}
	if err := s.core.RecordOAuthClientAuthorization(ctx, admin.Id, testOAuthClientID, "Test Client", "https://client.example", "https://client.example", evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatal(err)
	}
	if _, err := s.core.UpdateOAuthClientPolicy(ctx, admin.Id, testOAuthClientID, evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED); err != nil {
		t.Fatal(err)
	}

	cookies, _ := loginOAuthTestUser(t, s, "trusted-client-new-user")
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {"https://client.example/callback"},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
	}.Encode(), nil)
	addCookies(req, cookies)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusTemporaryRedirect || w.Header().Get("Location") != "/oauth/consent" {
		t.Fatalf("trusted-client authorize status/location = %d/%q, want consent: %s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
}

func TestOAuthToken_RejectsClientBlockedAfterCodeIssuance(t *testing.T) {
	s := setupOAuthServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	admin, err := s.core.CreateUser(ctx, core.SystemActorID, "blocked-exchange-admin", "Blocked Exchange Admin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.core.AssignAdminRole(ctx, admin.Id); err != nil {
		t.Fatal(err)
	}
	if err := s.core.RecordOAuthClientAuthorization(ctx, admin.Id, testOAuthClientID, "Test Client", "https://client.example", "https://client.example", evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatal(err)
	}
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	redirectURI := "https://client.example/callback"
	generation, err := s.core.CurrentAuthGeneration(ctx, admin.Id)
	if err != nil {
		t.Fatal(err)
	}
	code, err := s.core.CreateAuthCodeForClientGeneration(ctx, admin.Id, testOAuthClientID, redirectURI, core.GenerateCodeChallenge(verifier), "S256", generation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.core.UpdateOAuthClientPolicy(ctx, admin.Id, testOAuthClientID, evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{
		"grant_type": "authorization_code", "code": code, "code_verifier": verifier,
		"redirect_uri": redirectURI, "client_id": testOAuthClientID,
	})
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"invalid_client"`) {
		t.Fatalf("blocked token exchange status/body = %d/%s", w.Code, w.Body.String())
	}
}

func TestOAuthAuthorize_RejectsRedirectNotRegisteredByCIMD(t *testing.T) {
	s := setupOAuthServer(t)
	clientID, metadataServer := newOAuthCIMDTestServer(t, "https://client.example/servers/callback?mode=popup")
	resolver, err := newOAuthClientResolver("http://localhost:4000", metadataServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	s.oauthClientResolver = resolver

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {"https://client.example/servers/callback"},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "not registered") {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestOAuthAuthorize_NativeCIMDRedirectReachesConsent(t *testing.T) {
	s := setupOAuthServer(t)
	cookies, _ := loginOAuthTestUser(t, s, "native-oauth-consent")
	const redirectURI = "com.example.chatto:/oauth/callback"
	clientID, metadataServer := newOAuthCIMDTestServerForApplication(t, redirectURI, "native")
	resolver, err := newOAuthClientResolver("http://localhost:4000", metadataServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	s.oauthClientResolver = resolver

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
	}.Encode(), nil)
	addCookies(req, cookies)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusTemporaryRedirect || w.Header().Get("Location") != "/oauth/consent" {
		t.Fatalf("authorize status/location = %d/%q: %s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	cookies = mergeCookies(cookies, w.Result().Cookies())

	consentReq := httptest.NewRequest(http.MethodGet, "/oauth/consent/request", nil)
	addCookies(consentReq, cookies)
	consentW := httptest.NewRecorder()
	s.router.ServeHTTP(consentW, consentReq)
	if consentW.Code != http.StatusOK {
		t.Fatalf("consent status = %d: %s", consentW.Code, consentW.Body.String())
	}
	var response map[string]string
	if err := json.Unmarshal(consentW.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["redirectOrigin"] != "com.example.chatto:" {
		t.Fatalf("redirectOrigin = %q", response["redirectOrigin"])
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/oauth/consent/approve", nil)
	addCookies(approveReq, cookies)
	approveW := httptest.NewRecorder()
	s.router.ServeHTTP(approveW, approveReq)
	if approveW.Code != http.StatusOK {
		t.Fatalf("approve status = %d: %s", approveW.Code, approveW.Body.String())
	}
	var approval map[string]string
	if err := json.Unmarshal(approveW.Body.Bytes(), &approval); err != nil {
		t.Fatal(err)
	}
	redirect, err := url.Parse(approval["redirectUrl"])
	if err != nil {
		t.Fatal(err)
	}
	if redirect.Scheme != "com.example.chatto" || redirect.Path != "/oauth/callback" || redirect.Query().Get("code") == "" {
		t.Fatalf("native approval redirect = %q", approval["redirectUrl"])
	}
}

func TestOAuthAuthorize_LargeValidCIMDMetadataSurvivesConsentRedirect(t *testing.T) {
	s := setupOAuthServer(t)
	cookies, _ := loginOAuthTestUser(t, s, "large-cimd-consent")
	redirectURI := "https://callback.example/" + strings.Repeat("r", 1900)
	var clientID string
	var metadataOrigin string
	metadataServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cimdDocument{
			ClientID: clientID, ClientName: strings.Repeat("N", 100),
			ClientURI:       metadataOrigin + "/products/chatto?account=person@example.com",
			ApplicationType: "web", RedirectURIs: []string{redirectURI}, TokenEndpointAuthMethod: "none",
			GrantTypes: []string{"authorization_code"}, ResponseTypes: []string{"code"},
		})
	}))
	t.Cleanup(metadataServer.Close)
	metadataOrigin = metadataServer.URL
	clientID = metadataServer.URL + "/" + strings.Repeat("c", 1900)
	resolver, err := newOAuthClientResolver("http://localhost:4000", metadataServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	s.oauthClientResolver = resolver

	authorizeReq := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirectURI},
		"code_challenge": {"challenge"}, "code_challenge_method": {"S256"},
	}.Encode(), nil)
	addCookies(authorizeReq, cookies)
	authorizeW := httptest.NewRecorder()
	s.router.ServeHTTP(authorizeW, authorizeReq)
	if authorizeW.Code != http.StatusTemporaryRedirect || authorizeW.Header().Get("Location") != "/oauth/consent" {
		t.Fatalf("authorize status/location = %d/%q: %s", authorizeW.Code, authorizeW.Header().Get("Location"), authorizeW.Body.String())
	}
	cookies = mergeCookies(cookies, authorizeW.Result().Cookies())

	consentReq := httptest.NewRequest(http.MethodGet, "/oauth/consent/request", nil)
	addCookies(consentReq, cookies)
	consentW := httptest.NewRecorder()
	s.router.ServeHTTP(consentW, consentReq)
	if consentW.Code != http.StatusOK {
		t.Fatalf("consent status = %d: %s", consentW.Code, consentW.Body.String())
	}
	var response map[string]string
	if err := json.Unmarshal(consentW.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["clientId"] != clientID || response["redirectUri"] != redirectURI {
		t.Fatalf("consent request lost large validated metadata")
	}
}

func newOAuthCIMDTestServer(t *testing.T, redirectURI string) (string, *httptest.Server) {
	return newOAuthCIMDTestServerForApplication(t, redirectURI, "web")
}

func newOAuthCIMDTestServerForApplication(t *testing.T, redirectURI, applicationType string) (string, *httptest.Server) {
	t.Helper()
	var clientID string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cimdDocument{
			ClientID: clientID, ClientName: "Remote Chatto",
			ApplicationType: applicationType, RedirectURIs: []string{redirectURI}, TokenEndpointAuthMethod: "none",
			GrantTypes: []string{"authorization_code"}, ResponseTypes: []string{"code"},
		})
	}))
	t.Cleanup(server.Close)
	clientID = server.URL + "/oauth/client-metadata.json"
	return clientID, server
}

func TestOAuthAuthorize_RejectsUnconfiguredRedirectForAuthenticatedUser(t *testing.T) {
	s := setupOAuthServer(t)
	cookies, _ := loginOAuthTestUser(t, s, "oauth-victim")

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := core.GenerateCodeChallenge(verifier)
	req := httptest.NewRequest("GET", "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {"https://evil.example/callback"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"attacker-state"},
	}.Encode(), nil)
	addCookies(req, cookies)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unconfigured redirect, got %d: location=%q body=%s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	if location := w.Header().Get("Location"); strings.Contains(location, "code=") {
		t.Fatalf("unconfigured redirect minted code in Location %q", location)
	}
}

func TestOAuthAuthorize_AuthenticatedTrustedRedirectRequiresConsent(t *testing.T) {
	s := setupOAuthServer(t)
	cookies, _ := loginOAuthTestUser(t, s, "oauth-consent-required")

	challenge := core.GenerateCodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	req := httptest.NewRequest("GET", "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {"https://client.example/servers/callback"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"state123"},
	}.Encode(), nil)
	addCookies(req, cookies)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 to consent page, got %d: %s", w.Code, w.Body.String())
	}
	if location := w.Header().Get("Location"); location != "/oauth/consent" {
		t.Fatalf("Location = %q, want /oauth/consent", location)
	}
	if location := w.Header().Get("Location"); strings.Contains(location, "code=") {
		t.Fatalf("consent redirect minted code in Location %q", location)
	}
}

func TestOAuthAuthorize_FreshRequestOverwritesPendingConsent(t *testing.T) {
	s := setupOAuthServer(t)
	cookies, _ := loginOAuthTestUser(t, s, "oauth-consent-overwrite")

	challenge := core.GenerateCodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	firstParams := url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {"https://first.example/servers/callback"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"first-state"},
	}
	firstReq := httptest.NewRequest("GET", "/oauth/authorize?"+firstParams.Encode(), nil)
	addCookies(firstReq, cookies)
	firstW := httptest.NewRecorder()
	s.router.ServeHTTP(firstW, firstReq)
	if firstW.Code != http.StatusTemporaryRedirect || firstW.Header().Get("Location") != "/oauth/consent" {
		t.Fatalf("first authorize status/location = %d/%q", firstW.Code, firstW.Header().Get("Location"))
	}
	cookies = mergeCookies(cookies, firstW.Result().Cookies())

	secondParams := url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {"https://second.example/servers/callback"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"second-state"},
	}
	secondReq := httptest.NewRequest("GET", "/oauth/authorize?"+secondParams.Encode(), nil)
	addCookies(secondReq, cookies)
	secondW := httptest.NewRecorder()
	s.router.ServeHTTP(secondW, secondReq)
	if secondW.Code != http.StatusTemporaryRedirect || secondW.Header().Get("Location") != "/oauth/consent" {
		t.Fatalf("second authorize status/location = %d/%q", secondW.Code, secondW.Header().Get("Location"))
	}
	cookies = mergeCookies(cookies, secondW.Result().Cookies())

	requestReq := httptest.NewRequest("GET", "/oauth/consent/request", nil)
	addCookies(requestReq, cookies)
	requestW := httptest.NewRecorder()
	s.router.ServeHTTP(requestW, requestReq)
	if requestW.Code != http.StatusOK {
		t.Fatalf("consent request status = %d: %s", requestW.Code, requestW.Body.String())
	}
	var requestResp map[string]string
	if err := json.Unmarshal(requestW.Body.Bytes(), &requestResp); err != nil {
		t.Fatalf("decode consent request: %v", err)
	}
	if requestResp["redirectOrigin"] != "https://second.example" {
		t.Fatalf("redirectOrigin = %q, want second origin", requestResp["redirectOrigin"])
	}
	if requestResp["redirectUri"] != "https://second.example/servers/callback" {
		t.Fatalf("redirectUri = %q, want second callback", requestResp["redirectUri"])
	}

	approveReq := httptest.NewRequest("POST", "/oauth/consent/approve", nil)
	addCookies(approveReq, cookies)
	approveW := httptest.NewRecorder()
	s.router.ServeHTTP(approveW, approveReq)
	if approveW.Code != http.StatusOK {
		t.Fatalf("approve status = %d: %s", approveW.Code, approveW.Body.String())
	}
	var approveResp map[string]string
	if err := json.Unmarshal(approveW.Body.Bytes(), &approveResp); err != nil {
		t.Fatalf("decode approve response: %v", err)
	}
	redirectURL := approveResp["redirectUrl"]
	if !strings.HasPrefix(redirectURL, "https://second.example/servers/callback?") ||
		!strings.Contains(redirectURL, "code=") ||
		!strings.Contains(redirectURL, "state=second-state") ||
		strings.Contains(redirectURL, "first.example") ||
		strings.Contains(redirectURL, "first-state") {
		t.Fatalf("fresh authorize did not replace stale pending request, redirectUrl=%q", redirectURL)
	}
}

func TestOAuthAuthorizeExternalIdentityCreateEstablishesCookieSession(t *testing.T) {
	s := setupOAuthServer(t)
	s.setupConnectAPI()
	ts := httptest.NewServer(s.router)
	t.Cleanup(ts.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := ts.Client()
	client.Jar = jar
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := core.GenerateCodeChallenge(verifier)
	redirectURI := "https://client.example/callback"
	state := "sso-create-oauth-state"
	authorizeURL := ts.URL + "/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}.Encode()

	authorizeResp, err := client.Get(authorizeURL)
	if err != nil {
		t.Fatalf("initial authorize: %v", err)
	}
	_ = authorizeResp.Body.Close()
	if authorizeResp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("initial authorize status = %d, want 307", authorizeResp.StatusCode)
	}
	if location := authorizeResp.Header.Get("Location"); !strings.HasPrefix(location, "/login?redirect=") {
		t.Fatalf("initial authorize Location = %q, want login redirect", location)
	}

	ctx := context.Background()
	createToken, err := s.core.CreatePendingExternalIdentityCreateFlow(ctx, core.PendingExternalIdentityFlow{
		ProviderID:      "github-main",
		ProviderType:    config.AuthProviderTypeGitHub,
		ProviderLabel:   "GitHub",
		Issuer:          "github-main",
		Subject:         "sso-oauth-create-subject",
		LoginHint:       "sso-oauth-created",
		DisplayNameHint: "SSO OAuth Created",
	})
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityCreateFlow: %v", err)
	}

	authClient := authv1connect.NewExternalIdentityAuthServiceClient(client, ts.URL+connectAPIPrefix)
	createRequest := connect.NewRequest(&authv1.CreateExternalIdentityAccountRequest{
		Token: createToken,
		Login: "sso-oauth-created",
	})
	createRequest.Header().Set(connectapi.BrowserAuthenticationModeHeader, connectapi.BrowserAuthenticationModeCookie)
	created, err := authClient.CreateExternalIdentityAccount(ctx, createRequest)
	if err != nil {
		t.Fatalf("CreateExternalIdentityAccount: %v", err)
	}
	if created.Msg.GetToken() != "" || created.Msg.GetRefreshToken() != "" {
		t.Fatal("browser external-identity account creation returned bearer credentials")
	}
	if err := s.core.GrantOAuthClientConsent(ctx, created.Msg.GetUserId(), testOAuthClientID, "Test Client", "https://client.example", "https://client.example"); err != nil {
		t.Fatalf("GrantOAuthClientConsent: %v", err)
	}

	resumeResp, err := client.Get(ts.URL + "/oauth/authorize")
	if err != nil {
		t.Fatalf("resume authorize: %v", err)
	}
	_ = resumeResp.Body.Close()
	if resumeResp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("resume authorize status = %d, want 307", resumeResp.StatusCode)
	}
	resumeLocation := resumeResp.Header.Get("Location")
	if strings.HasPrefix(resumeLocation, "/login") {
		t.Fatalf("resume authorize redirected to login: %q", resumeLocation)
	}
	callbackURL, err := url.Parse(resumeLocation)
	if err != nil {
		t.Fatalf("parse resume Location %q: %v", resumeLocation, err)
	}
	if got := callbackURL.Scheme + "://" + callbackURL.Host + callbackURL.Path; got != redirectURI {
		t.Fatalf("resume redirect URI = %q, want %q", got, redirectURI)
	}
	if callbackURL.Query().Get("state") != state {
		t.Fatalf("resume state = %q, want %q", callbackURL.Query().Get("state"), state)
	}
	if callbackURL.Query().Get("code") == "" {
		t.Fatalf("resume Location %q did not include code", resumeLocation)
	}
}

func TestOAuthConsentApproveMintsCodeAndSkipsFuturePrompts(t *testing.T) {
	s := setupOAuthServer(t)
	cookies, user := loginOAuthTestUser(t, s, "oauth-consent-approve")

	challenge := core.GenerateCodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {"https://client.example/servers/callback"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"state123"},
	}
	req := httptest.NewRequest("GET", "/oauth/authorize?"+params.Encode(), nil)
	addCookies(req, cookies)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusTemporaryRedirect || w.Header().Get("Location") != "/oauth/consent" {
		t.Fatalf("authorize status/location = %d/%q", w.Code, w.Header().Get("Location"))
	}
	cookies = mergeCookies(cookies, w.Result().Cookies())

	requestReq := httptest.NewRequest("GET", "/oauth/consent/request", nil)
	addCookies(requestReq, cookies)
	requestW := httptest.NewRecorder()
	s.router.ServeHTTP(requestW, requestReq)
	if requestW.Code != http.StatusOK {
		t.Fatalf("consent request status = %d: %s", requestW.Code, requestW.Body.String())
	}
	var requestResp map[string]string
	if err := json.Unmarshal(requestW.Body.Bytes(), &requestResp); err != nil {
		t.Fatalf("decode consent request: %v", err)
	}
	if requestResp["redirectUri"] != "https://client.example/servers/callback" {
		t.Fatalf("redirectUri = %q", requestResp["redirectUri"])
	}
	if requestResp["redirectOrigin"] != "https://client.example" {
		t.Fatalf("redirectOrigin = %q", requestResp["redirectOrigin"])
	}

	approveReq := httptest.NewRequest("POST", "/oauth/consent/approve", nil)
	addCookies(approveReq, cookies)
	approveW := httptest.NewRecorder()
	s.router.ServeHTTP(approveW, approveReq)
	if approveW.Code != http.StatusOK {
		t.Fatalf("approve status = %d: %s", approveW.Code, approveW.Body.String())
	}
	var approveResp map[string]string
	if err := json.Unmarshal(approveW.Body.Bytes(), &approveResp); err != nil {
		t.Fatalf("decode approve response: %v", err)
	}
	if redirectURL := approveResp["redirectUrl"]; !strings.HasPrefix(redirectURL, "https://client.example/servers/callback?") || !strings.Contains(redirectURL, "code=") || !strings.Contains(redirectURL, "state=state123") {
		t.Fatalf("unexpected approve redirectUrl %q", redirectURL)
	}
	cookies = mergeCookies(cookies, approveW.Result().Cookies())

	consented, err := s.core.HasOAuthClientConsent(context.Background(), user.Id, testOAuthClientID, "https://client.example")
	if err != nil {
		t.Fatalf("HasOAuthClientConsent: %v", err)
	}
	if !consented {
		t.Fatalf("expected consent to be remembered")
	}

	secondReq := httptest.NewRequest("GET", "/oauth/authorize?"+params.Encode(), nil)
	addCookies(secondReq, cookies)
	secondW := httptest.NewRecorder()
	s.router.ServeHTTP(secondW, secondReq)
	if secondW.Code != http.StatusTemporaryRedirect {
		t.Fatalf("second authorize status = %d: %s", secondW.Code, secondW.Body.String())
	}
	if location := secondW.Header().Get("Location"); !strings.HasPrefix(location, "https://client.example/servers/callback?") || !strings.Contains(location, "code=") {
		t.Fatalf("second authorize did not mint code directly, Location=%q", location)
	}
}

func TestOAuthConsentDenyRedirectsAccessDenied(t *testing.T) {
	s := setupOAuthServer(t)
	cookies, user := loginOAuthTestUser(t, s, "oauth-consent-deny")

	challenge := core.GenerateCodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	req := httptest.NewRequest("GET", "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {testOAuthClientID},
		"redirect_uri":          {"https://client.example/servers/callback"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"deny-state"},
	}.Encode(), nil)
	addCookies(req, cookies)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusTemporaryRedirect || w.Header().Get("Location") != "/oauth/consent" {
		t.Fatalf("authorize status/location = %d/%q", w.Code, w.Header().Get("Location"))
	}
	cookies = mergeCookies(cookies, w.Result().Cookies())

	denyReq := httptest.NewRequest("POST", "/oauth/consent/deny", nil)
	addCookies(denyReq, cookies)
	denyW := httptest.NewRecorder()
	s.router.ServeHTTP(denyW, denyReq)
	if denyW.Code != http.StatusOK {
		t.Fatalf("deny status = %d: %s", denyW.Code, denyW.Body.String())
	}
	var denyResp map[string]string
	if err := json.Unmarshal(denyW.Body.Bytes(), &denyResp); err != nil {
		t.Fatalf("decode deny response: %v", err)
	}
	redirectURL := denyResp["redirectUrl"]
	if !strings.HasPrefix(redirectURL, "https://client.example/servers/callback?") || !strings.Contains(redirectURL, "error=access_denied") || !strings.Contains(redirectURL, "state=deny-state") || strings.Contains(redirectURL, "code=") {
		t.Fatalf("unexpected deny redirectUrl %q", redirectURL)
	}
	consented, err := s.core.HasOAuthClientConsent(context.Background(), user.Id, testOAuthClientID, "https://client.example")
	if err != nil {
		t.Fatalf("HasOAuthClientConsent: %v", err)
	}
	if consented {
		t.Fatalf("denial should not grant consent")
	}
}

func TestOAuthToken_InvalidGrant(t *testing.T) {
	s := setupOAuthServer(t)

	body := `{"grant_type":"authorization_code","code":"cht_ACnonexistent12","code_verifier":"verifier","redirect_uri":"https://example.com/callback","client_id":"https://client.example/oauth/client-metadata.json"}`
	req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "invalid_grant" {
		t.Errorf("expected error 'invalid_grant', got %q", resp["error"])
	}
}

func TestOAuthToken_MissingParams(t *testing.T) {
	s := setupOAuthServer(t)

	body := `{"grant_type":"authorization_code","code":"","code_verifier":"","redirect_uri":""}`
	req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestOAuthToken_UnsupportedGrantType(t *testing.T) {
	s := setupOAuthServer(t)

	body := `{"grant_type":"client_credentials","code":"abc","code_verifier":"def","redirect_uri":"https://example.com"}`
	req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "unsupported_grant_type" {
		t.Errorf("expected error 'unsupported_grant_type', got %q", resp["error"])
	}
}

func TestOAuthToken_FormEncoded(t *testing.T) {
	s := setupOAuthServer(t)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"cht_ACnonexistent12"},
		"code_verifier": {"verifier"},
		"redirect_uri":  {"https://example.com/callback"},
		"client_id":     {testOAuthClientID},
	}
	req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Should return invalid_grant (not a parse error)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "invalid_grant" {
		t.Errorf("expected error 'invalid_grant', got %q", resp["error"])
	}
}

func TestOAuthToken_RefreshGrantRotatesAndRecoversRetry(t *testing.T) {
	s := setupOAuthServer(t)
	ctx := context.Background()
	user, err := s.core.CreateUser(ctx, core.SystemActorID, "refresh-grant-user", "Refresh Grant User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := s.core.CreateBearerSessionWithSource(ctx, user.GetId(), "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}

	exchange := func(requestID string) (int, map[string]any, http.Header) {
		t.Helper()
		body, err := json.Marshal(map[string]string{
			"grant_type":         "refresh_token",
			"refresh_token":      initial.RefreshToken,
			"refresh_request_id": requestID,
		})
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		req := httptest.NewRequest("POST", "/oauth/token", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		s.router.ServeHTTP(response, req)
		var result map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return response.Code, result, response.Header()
	}

	status, first, headers := exchange("00000000-0000-4000-8000-000000000001")
	if status != http.StatusOK {
		t.Fatalf("first refresh status = %d, body = %v", status, first)
	}
	if first["access_token"] == "" || first["refresh_token"] == "" || first["expires_in"].(float64) <= 0 || first["refresh_token_expires_in"].(float64) <= 0 {
		t.Fatalf("first refresh response = %v", first)
	}
	if headers.Get("Cache-Control") != "no-store" || headers.Get("Pragma") != "no-cache" {
		t.Fatalf("token response cache headers = %v", headers)
	}

	status, retry, _ := exchange("00000000-0000-4000-8000-000000000001")
	if status != http.StatusOK || retry["access_token"] != first["access_token"] || retry["refresh_token"] != first["refresh_token"] {
		t.Fatalf("same-request retry status/body = %d, %v; first = %v", status, retry, first)
	}

	status, reused, _ := exchange("00000000-0000-4000-8000-000000000002")
	if status != http.StatusBadRequest || reused["error"] != "invalid_grant" {
		t.Fatalf("reuse status/body = %d, %v", status, reused)
	}
}

func TestOAuthToken_RefreshGrantRenewsActiveSessionWindow(t *testing.T) {
	s := setupOAuthServerWithTokenTTL(t, 2*time.Second)
	ctx := context.Background()
	user, err := s.core.CreateUser(ctx, core.SystemActorID, "refresh-window-user", "Refresh Window User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := s.core.CreateBearerSessionWithSource(ctx, user.GetId(), "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}

	time.Sleep(1600 * time.Millisecond)
	body, err := json.Marshal(map[string]string{
		"grant_type":         "refresh_token",
		"refresh_token":      initial.RefreshToken,
		"refresh_request_id": "00000000-0000-4000-8000-000000000001",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest("POST", "/oauth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	s.router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body = %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, _ := result["refresh_token_expires_in"].(float64); got < 2 {
		t.Fatalf("renewed session lifetime = %v, want at least 2 seconds", got)
	}
}

func TestOAuthToken_CORS(t *testing.T) {
	s := setupOAuthServer(t)

	// OPTIONS preflight
	req := httptest.NewRequest("OPTIONS", "/oauth/token", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("expected CORS origin *, got %q", origin)
	}

	// POST also includes CORS headers
	body := `{"grant_type":"authorization_code","code":"abc","code_verifier":"def","redirect_uri":"https://example.com"}`
	req = httptest.NewRequest("POST", "/oauth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("expected CORS origin * on POST, got %q", origin)
	}
}

func TestCookieSessionAuthenticationRejectsStaleGenerationWithoutMutatingCookie(t *testing.T) {
	s := setupOAuthServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	user, err := s.core.CreateUser(ctx, "", "rotate-stale-user", "Rotate Stale User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	authGeneration, err := s.core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}
	oldSessionID, _, err := s.core.CreateCookieSessionForGeneration(ctx, user.Id, "password_login", authGeneration)
	if err != nil {
		t.Fatalf("CreateCookieSessionForGeneration: %v", err)
	}
	if err := s.core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	s.router.GET("/test/rotate-stale-session", func(c *gin.Context) {
		if _, ok, err := s.cookiePresentedCredential(c); err != nil || ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "stale session authenticated"})
			return
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest("GET", "/test/rotate-stale-session", nil)
	req.AddCookie(&http.Cookie{Name: browserSessionCookieName, Value: oldSessionID})
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("stale rotation status = %d, want 204: %s", w.Code, w.Body.String())
	}
	if cookies := w.Header().Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("stale validation Set-Cookie = %v, want none", cookies)
	}
}

func TestCookiePresentedCredentialRejectsRetiredSignedSessionFields(t *testing.T) {
	s := setupOAuthServer(t)

	s.router.GET("/test/retired-cookie-session", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set(retiredSessionKeyUserID, "Ulegacy")
		session.Set(retiredSessionKeyCredentialID, "cht_CSlegacy")
		if err := session.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if _, ok, err := s.cookiePresentedCredential(c); err != nil || ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "retired signed-session fields authenticated"})
			return
		}
		if session.Get(retiredSessionKeyUserID) == nil || session.Get(retiredSessionKeyCredentialID) == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "validation mutated retired signed-session fields"})
			return
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/test/retired-cookie-session", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("retired cookie-session status = %d, want 204: %s", w.Code, w.Body.String())
	}
}

func TestOAuthAuthorizeDoesNotMintCodeForStaleGeneration(t *testing.T) {
	s := setupOAuthServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	user, err := s.core.CreateUser(ctx, "", "oauth-stale-user", "OAuth Stale User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	authGeneration, err := s.core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}
	if err := s.core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := core.GenerateCodeChallenge(verifier)
	s.router.GET("/test/complete-stale-oauth", func(c *gin.Context) {
		session := sessions.Default(c)
		if err := s.storePendingOAuthAuthorize(c.Request.Context(), session, pendingOAuthAuthorize{
			RedirectURI:         "https://example.com/callback",
			CodeChallenge:       challenge,
			CodeChallengeMethod: "S256",
			State:               "state123",
			ClientID:            testOAuthClientID,
			ClientName:          "Test Client",
			ClientURI:           "https://client.example",
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		s.completeOAuthAuthorize(c, user.Id, authGeneration)
	})

	req := httptest.NewRequest("GET", "/test/complete-stale-oauth", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code < http.StatusBadRequest {
		t.Fatalf("stale OAuth authorize status = %d, want error response", w.Code)
	}
	if location := w.Header().Get("Location"); strings.Contains(location, "code=") {
		t.Fatalf("stale OAuth authorize minted code in Location %q", w.Header().Get("Location"))
	}
}

func TestOAuthToken_FullExchange(t *testing.T) {
	s := setupOAuthServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	// Create a user and an auth code directly (simulating a completed authorize flow)
	user, err := s.core.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := core.GenerateCodeChallenge(verifier)
	redirectURI := "https://example.com/callback"

	authGeneration, err := s.core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}
	code, err := s.core.CreateAuthCodeForClientGeneration(ctx, user.Id, testOAuthClientID, redirectURI, challenge, "S256", authGeneration)
	if err != nil {
		t.Fatalf("Failed to create auth code: %v", err)
	}

	// Exchange via POST /oauth/token
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"code_verifier": verifier,
		"redirect_uri":  redirectURI,
		"client_id":     testOAuthClientID,
	})
	req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["token_type"] != "Bearer" {
		t.Errorf("expected token_type 'Bearer', got %q", resp["token_type"])
	}

	accessToken, ok := resp["access_token"].(string)
	if !ok || !strings.HasPrefix(accessToken, "cht_AT") {
		t.Errorf("expected access_token with cht_AT prefix, got %q", resp["access_token"])
	}

	// Verify the returned token is valid
	validatedUserID, err := s.core.ValidateAuthToken(ctx, accessToken)
	if err != nil {
		t.Fatalf("Token validation failed: %v", err)
	}
	if validatedUserID != user.Id {
		t.Errorf("Token maps to user %q, want %q", validatedUserID, user.Id)
	}

	// Verify user info is included
	userInfo, ok := resp["user"].(map[string]any)
	if !ok {
		t.Fatal("expected user object in response")
	}
	if userInfo["id"] != user.Id {
		t.Errorf("user.id = %q, want %q", userInfo["id"], user.Id)
	}
	if userInfo["login"] != "testuser" {
		t.Errorf("user.login = %q, want 'testuser'", userInfo["login"])
	}
}
