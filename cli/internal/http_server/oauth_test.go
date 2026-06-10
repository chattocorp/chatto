package http_server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

// setupOAuthServer creates a minimal HTTPServer with session middleware and OAuth endpoints.
func setupOAuthServer(t *testing.T) *HTTPServer {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	chattoCore, err := core.NewChattoCore(ctx, nc, config.CoreConfig{})
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
		},
		nc:      nc,
		router:  router,
		core:    chattoCore,
		version: "test",
	}
	s.setupOAuthRoutes()

	return s
}

func TestOAuthAuthorize_ValidParams(t *testing.T) {
	s := setupOAuthServer(t)

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := core.GenerateCodeChallenge(verifier)

	req := httptest.NewRequest("GET", "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
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
				"redirect_uri":          {"https://chatto.example/servers/callback"},
				"code_challenge":        {"challenge"},
				"code_challenge_method": {"S256"},
			},
			"unsupported_response_type",
		},
		{
			"missing redirect_uri",
			url.Values{
				"response_type":         {"code"},
				"code_challenge":        {"challenge"},
				"code_challenge_method": {"S256"},
			},
			"invalid_request",
		},
		{
			"missing code_challenge",
			url.Values{
				"response_type":         {"code"},
				"redirect_uri":          {"https://chatto.example/servers/callback"},
				"code_challenge_method": {"S256"},
			},
			"invalid_request",
		},
		{
			"wrong code_challenge_method",
			url.Values{
				"response_type":         {"code"},
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

func TestOAuthAuthorize_AllowsConfiguredRedirectOrigin(t *testing.T) {
	s := setupOAuthServer(t)
	s.config.Webserver.AllowedOrigins = []string{"https://client.example"}

	req := httptest.NewRequest("GET", "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
		"redirect_uri":          {"https://client.example/servers/callback"},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 for configured redirect origin, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOAuthAuthorize_RejectsUnconfiguredRedirectForAuthenticatedUser(t *testing.T) {
	s := setupOAuthServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	user, err := s.core.CreateUser(ctx, "", "oauth-victim", "OAuth Victim", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	s.router.GET("/test/login-victim", func(c *gin.Context) {
		if err := s.createCookieSession(c, user.Id, "test"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	})

	loginReq := httptest.NewRequest("GET", "/test/login-victim", nil)
	loginW := httptest.NewRecorder()
	s.router.ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusNoContent {
		t.Fatalf("login fixture status = %d, want 204: %s", loginW.Code, loginW.Body.String())
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := core.GenerateCodeChallenge(verifier)
	req := httptest.NewRequest("GET", "/oauth/authorize?"+url.Values{
		"response_type":         {"code"},
		"redirect_uri":          {"https://evil.example/callback"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"attacker-state"},
	}.Encode(), nil)
	for _, cookie := range loginW.Result().Cookies() {
		req.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unconfigured redirect, got %d: location=%q body=%s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	if location := w.Header().Get("Location"); strings.Contains(location, "code=") {
		t.Fatalf("unconfigured redirect minted code in Location %q", location)
	}
}

func TestOAuthToken_InvalidGrant(t *testing.T) {
	s := setupOAuthServer(t)

	body := `{"grant_type":"authorization_code","code":"cht_ACnonexistent12","code_verifier":"verifier","redirect_uri":"https://example.com/callback"}`
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

func TestCookieSessionRotationClearsStaleGeneration(t *testing.T) {
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
	if err := s.core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	s.router.GET("/test/rotate-stale-session", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set(sessionKeyUserID, user.Id)
		session.Set(sessionKeyCookieSessionID, "old-session")
		if err := session.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		staleRecord := &corev1.CookieSession{
			UserId:         user.Id,
			CreatedAt:      timestamppb.New(time.Now().Add(-time.Hour)),
			ExpiresAt:      timestamppb.New(time.Now().Add(time.Hour)),
			Source:         "password_login",
			AuthGeneration: authGeneration,
		}
		s.rotateCookieSessionIfNeeded(c, user.Id, "old-session", staleRecord)

		userID, sessionID, ok := cookieSessionIDs(session)
		if ok || userID != "" || sessionID != "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "session auth was not cleared"})
			return
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest("GET", "/test/rotate-stale-session", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("stale rotation status = %d, want 204: %s", w.Code, w.Body.String())
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
		session.Set(sessionKeyOAuthRedirectURI, "https://example.com/callback")
		session.Set(sessionKeyOAuthCodeChallenge, challenge)
		session.Set(sessionKeyOAuthCodeMethod, "S256")
		session.Set(sessionKeyOAuthState, "state123")
		if err := session.Save(); err != nil {
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

	code, err := s.core.CreateAuthCode(ctx, user.Id, redirectURI, challenge, "S256")
	if err != nil {
		t.Fatalf("Failed to create auth code: %v", err)
	}

	// Exchange via POST /oauth/token
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"code_verifier": verifier,
		"redirect_uri":  redirectURI,
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

func TestIsValidRedirectURI(t *testing.T) {
	s := setupOAuthServer(t)
	s.config.Webserver.AllowedOrigins = []string{"https://client.example"}

	tests := []struct {
		uri  string
		want bool
	}{
		{"https://chatto.example/callback", true},
		{"https://client.example/callback", true},
		{"http://localhost:3000/callback", true},
		{"http://localhost/callback", true},
		{"http://127.0.0.1:5173/callback", true},
		{"http://127.0.0.1/callback", true},
		{"http://[::1]:5173/callback", true},
		{"https://evil.example/callback", false},
		{"http://example.com/callback", false},
		{"ftp://example.com/callback", false},
		{"example.com/callback", false},
		{"/callback", false},
		{"https://chatto.example/callback#fragment", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got := s.isAllowedOAuthRedirectURI(tt.uri)
			if got != tt.want {
				t.Errorf("isAllowedOAuthRedirectURI(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}
