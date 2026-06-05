package http_server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
	"hmans.de/chatto/internal/testutil"
)

// ============================================================================
// Content Type Detection Tests
// ============================================================================

func TestGetContentType(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"webp extension", "image.webp", "image/webp"},
		{"png extension", "photo.png", "image/png"},
		{"jpg extension", "photo.jpg", "image/jpeg"},
		{"jpeg extension", "photo.jpeg", "image/jpeg"},
		{"gif extension", "animation.gif", "image/gif"},
		{"unknown extension", "file.xyz", "application/octet-stream"},
		{"no extension", "file", "application/octet-stream"},
		{"path with directory", "/some/path/image.png", "image/png"},
		{"hidden file with extension", ".hidden.png", "image/png"},
		{"uppercase extension", "IMAGE.PNG", "application/octet-stream"}, // case-sensitive
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getContentType(tt.path)
			if result != tt.expected {
				t.Errorf("getContentType(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsImageContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		expected    bool
	}{
		{"jpeg", "image/jpeg", true},
		{"png", "image/png", true},
		{"gif", "image/gif", true},
		{"webp", "image/webp", true},
		{"text plain", "text/plain", false},
		{"application json", "application/json", false},
		{"video mp4", "video/mp4", false},
		{"empty string", "", false},
		{"image svg", "image/svg+xml", false}, // SVG not supported for transforms
		{"image bmp", "image/bmp", false},     // BMP not supported
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isImageContentType(tt.contentType)
			if result != tt.expected {
				t.Errorf("isImageContentType(%q) = %v, want %v", tt.contentType, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// Test Setup Helpers
// ============================================================================

// testContext returns a context with a reasonable timeout for tests.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestNewHTTPServerAppliesTimeouts(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	srv := newHTTPServer(":4000", handler)

	if srv.Addr != ":4000" {
		t.Fatalf("Addr = %q, want :4000", srv.Addr)
	}
	if srv.Handler == nil {
		t.Fatal("Handler was not set")
	}
	if srv.ReadHeaderTimeout != httpServerReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %s, want %s", srv.ReadHeaderTimeout, httpServerReadHeaderTimeout)
	}
	if srv.IdleTimeout != httpServerIdleTimeout {
		t.Fatalf("IdleTimeout = %s, want %s", srv.IdleTimeout, httpServerIdleTimeout)
	}
	if srv.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout = %s, want 0", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %s, want 0", srv.WriteTimeout)
	}
}

// testHTTPServer creates an HTTPServer for testing with an embedded NATS server.
// Returns the test server, a client with cookie jar, and ChattoCore.
func setupTestHTTPServer(t *testing.T) (*httptest.Server, *http.Client, *core.ChattoCore) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartNATS(t)

	ctx := testContext(t)

	// Create ChattoCore
	coreConfig := config.CoreConfig{}
	chattoCore, err := core.NewChattoCore(ctx, nc, coreConfig)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

	// Create router with session middleware
	router := gin.New()
	router.Use(gin.Recovery())

	sessionStore := cookie.NewStore([]byte("test-secret-key-32-bytes-long!!"))
	sessionStore.Options(sessions.Options{
		MaxAge:   60 * 60 * 24 * 90,
		HttpOnly: true,
		Secure:   false,
		Path:     "/",
	})
	router.Use(sessions.Sessions("chatto_session", sessionStore))

	// Create HTTPServer (minimal for testing)
	s := &HTTPServer{
		config: config.ChattoConfig{
			Auth: config.AuthConfig{},
			Webserver: config.WebserverConfig{
				URL:                 "http://localhost:4000",
				CookieSigningSecret: "test-secret-key-32-bytes-long!!",
			},
		},
		nc:     nc,
		router: router,
		core:   chattoCore,
		mailer: nil, // Not needed for testing
	}

	// Set up auth routes only (skip GraphQL and other routes for focused testing)
	s.setupAuthRoutes()

	// Create test server
	ts := httptest.NewServer(router)
	t.Cleanup(func() { ts.Close() })

	// Create client with cookie jar for session persistence
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	return ts, client, chattoCore
}

// setupTestHTTPServerWithMailer creates an HTTPServer with MockSender enabled.
// Returns the test server, client, ChattoCore, and the MockSender for inspection.
func setupTestHTTPServerWithMailer(t *testing.T) (*httptest.Server, *http.Client, *core.ChattoCore, *email.MockSender) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartNATS(t)

	ctx := testContext(t)

	// Create ChattoCore
	coreConfig := config.CoreConfig{}
	chattoCore, err := core.NewChattoCore(ctx, nc, coreConfig)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

	// Create router with session middleware
	router := gin.New()
	router.Use(gin.Recovery())

	sessionStore := cookie.NewStore([]byte("test-secret-key-32-bytes-long!!"))
	sessionStore.Options(sessions.Options{
		MaxAge:   60 * 60 * 24 * 90,
		HttpOnly: true,
		Secure:   false,
		Path:     "/",
	})
	router.Use(sessions.Sessions("chatto_session", sessionStore))

	// Create MockSender for email capture
	mockMailer := email.NewMockSender(true)

	// Create HTTPServer with mailer enabled
	s := &HTTPServer{
		config: config.ChattoConfig{
			Auth: config.AuthConfig{},
			Webserver: config.WebserverConfig{
				URL:                 "http://localhost:4000",
				CookieSigningSecret: "test-secret-key-32-bytes-long!!",
			},
		},
		nc:         nc,
		router:     router,
		core:       chattoCore,
		mailer:     mockMailer,
		mockMailer: mockMailer,
	}

	// Set up auth routes
	s.setupAuthRoutes()

	// Create test server
	ts := httptest.NewServer(router)
	t.Cleanup(func() { ts.Close() })

	// Create client with cookie jar for session persistence
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	return ts, client, chattoCore, mockMailer
}

// ============================================================================
// Auth Route Integration Tests
// ============================================================================

func TestAuthRoutes_Login_Success(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	// Create a test user
	login := "loginuser"
	password := "password123"
	_, err := chattoCore.CreateUser(ctx, "system", login, "Test User", password)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Test login
	loginBody := map[string]string{
		"login":    login,
		"password": password,
	}
	body, _ := json.Marshal(loginBody)

	resp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send login request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["success"] != true {
		t.Error("Expected success: true in response")
	}

	user, ok := result["user"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected user object in response")
	}

	if user["login"] != login {
		t.Errorf("Expected login %s, got %v", login, user["login"])
	}
}

func TestAuthRoutes_Login_WithIdentifier(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	// Create a test user
	login := "identifierlogin"
	password := "password123"
	_, err := chattoCore.CreateUser(ctx, "system", login, "Test User", password)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Test login with login name
	loginBody := map[string]string{
		"login":    login,
		"password": password,
	}
	body, _ := json.Marshal(loginBody)

	resp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send login request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["success"] != true {
		t.Error("Expected success: true in response")
	}

	user, ok := result["user"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected user object in response")
	}

	if user["login"] != login {
		t.Errorf("Expected login %s, got %v", login, user["login"])
	}
}

func TestAuthRoutes_Login_InvalidCredentials(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	// Create a test user
	login := "invaliduser"
	_, err := chattoCore.CreateUser(ctx, "system", login, "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Test login with wrong password
	loginBody := map[string]string{
		"login":    login,
		"password": "wrongpassword",
	}
	body, _ := json.Marshal(loginBody)

	resp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send login request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_Login_NonexistentUser(t *testing.T) {
	ts, client, _ := setupTestHTTPServer(t)

	// Test login with non-existent user
	loginBody := map[string]string{
		"login":    "nonexistent",
		"password": "password123",
	}
	body, _ := json.Marshal(loginBody)

	resp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send login request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_Login_MissingFields(t *testing.T) {
	ts, client, _ := setupTestHTTPServer(t)

	// Test login with missing password
	loginBody := map[string]string{
		"email": "test@test.com",
	}
	body, _ := json.Marshal(loginBody)

	resp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send login request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_Login_IdentifierTooLong(t *testing.T) {
	ts, client, _ := setupTestHTTPServer(t)

	// Test login with identifier exceeding max length (254 chars)
	longIdentifier := strings.Repeat("a", 255)
	loginBody := map[string]string{
		"identifier": longIdentifier,
		"password":   "password123",
	}
	body, _ := json.Marshal(loginBody)

	resp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send login request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for too-long identifier, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_Logout(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	// Create and login a test user
	login := "logoutuser"
	password := "password123"
	_, err := chattoCore.CreateUser(ctx, "system", login, "Test User", password)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Login first
	loginBody := map[string]string{
		"login":    login,
		"password": password,
	}
	body, _ := json.Marshal(loginBody)

	loginResp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed with status %d", loginResp.StatusCode)
	}

	// Now logout
	logoutResp, err := client.Post(ts.URL+"/auth/logout", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to logout: %v", err)
	}
	defer logoutResp.Body.Close()

	if logoutResp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", logoutResp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(logoutResp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["success"] != true {
		t.Error("Expected success: true in response")
	}
}

func TestAuthRoutes_Register_SendsRegistrationEmail(t *testing.T) {
	ts, client, _, mockMailer := setupTestHTTPServerWithMailer(t)

	// Step 1: POST /auth/register with email only
	reqBody := map[string]string{"email": "newuser@example.com"}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should return generic message (no email enumeration)
	msg, ok := result["message"].(string)
	if !ok || !strings.Contains(msg, "registration link") {
		t.Errorf("Expected generic registration message, got: %v", result["message"])
	}

	// Verify registration email was sent
	email := mockMailer.LastMessage()
	if email == nil {
		t.Fatal("Expected registration email to be sent")
	}
	if email.To != "newuser@example.com" {
		t.Errorf("Expected email to newuser@example.com, got %s", email.To)
	}
	if email.Subject != "Complete your Chatto registration" {
		t.Errorf("Expected subject 'Complete your Chatto registration', got %s", email.Subject)
	}
	if !strings.Contains(email.Body, "/register/complete?token=RG") {
		t.Errorf("Expected email body to contain registration link with RG token, got: %s", email.Body)
	}
}

func TestAuthRoutes_Register_RequiresMailer(t *testing.T) {
	ts, client, _ := setupTestHTTPServer(t) // No mailer

	reqBody := map[string]string{"email": "newuser@example.com"}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 when mailer not configured, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_Register_EmailEnumeration(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Create a user with verified email
	user, _ := chattoCore.CreateUser(ctx, "system", "existing", "Existing", "password123")
	chattoCore.AddVerifiedEmailDirect(ctx, user.Id, "taken@example.com")

	// Request registration for taken email — should return 200 (same as available email)
	reqBody := map[string]string{"email": "taken@example.com"}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 even for taken email, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_RegisterComplete_Success(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Create a registration token
	token, err := chattoCore.CreateRegistrationToken(ctx, "complete@example.com")
	if err != nil {
		t.Fatalf("Failed to create registration token: %v", err)
	}

	// Complete registration
	reqBody := map[string]string{
		"token":                token,
		"login":                "newuser",
		"password":             "password123",
		"passwordConfirmation": "password123",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register/complete request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["success"] != true {
		t.Error("Expected success: true")
	}

	user, ok := result["user"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected user object in response")
	}
	if user["login"] != "newuser" {
		t.Errorf("Expected login newuser, got %v", user["login"])
	}

	// Verify user was created
	createdUser, err := chattoCore.GetUserByLogin(ctx, "newuser")
	if err != nil {
		t.Fatalf("User was not created: %v", err)
	}
	if createdUser.Login != "newuser" {
		t.Errorf("Expected login newuser, got %s", createdUser.Login)
	}

	// Verify email was auto-verified
	hasVerified, err := chattoCore.HasVerifiedEmail(ctx, createdUser.Id)
	if err != nil {
		t.Fatalf("Failed to check verified email: %v", err)
	}
	if !hasVerified {
		t.Error("Expected email to be auto-verified after registration")
	}

	// Verify token was consumed (can't reuse)
	_, err = chattoCore.GetRegistrationToken(ctx, token)
	if err != core.ErrRegistrationTokenNotFound {
		t.Errorf("Expected token to be consumed, got error: %v", err)
	}
}

func TestAuthRoutes_RegisterComplete_DuplicateLogin(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Create existing user
	_, err := chattoCore.CreateUser(ctx, "system", "existinglogin", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create registration token
	token, err := chattoCore.CreateRegistrationToken(ctx, "different@example.com")
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	// Try to complete registration with taken login
	reqBody := map[string]string{
		"token":                token,
		"login":                "existinglogin",
		"password":             "password123",
		"passwordConfirmation": "password123",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("Expected status 409, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_RegisterComplete_InvalidLogin(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	token, _ := chattoCore.CreateRegistrationToken(ctx, "invalid@example.com")

	// Test with invalid login (too short)
	reqBody := map[string]string{
		"token":                token,
		"login":                "a",
		"password":             "password123",
		"passwordConfirmation": "password123",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_RegisterComplete_ShortPassword(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	token, _ := chattoCore.CreateRegistrationToken(ctx, "short@example.com")

	reqBody := map[string]string{
		"token":                token,
		"login":                "validlogin",
		"password":             "short",
		"passwordConfirmation": "short",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_RegisterComplete_BlockedUsername(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Default blocked usernames include: root, admin, superuser, op, operator, support
	blockedNames := []string{"admin", "root", "superuser", "op", "operator", "support", "ADMIN", "Admin"}

	for _, name := range blockedNames {
		t.Run(name, func(t *testing.T) {
			token, _ := chattoCore.CreateRegistrationToken(ctx, name+"@example.com")

			reqBody := map[string]string{
				"token":                token,
				"login":                name,
				"password":             "password123",
				"passwordConfirmation": "password123",
			}
			body, _ := json.Marshal(reqBody)

			resp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("Expected status 400 for blocked username '%s', got %d", name, resp.StatusCode)
			}

			var respBody map[string]string
			json.NewDecoder(resp.Body).Decode(&respBody)
			if respBody["error"] != "This username is not available" {
				t.Errorf("Expected error 'This username is not available', got '%s'", respBody["error"])
			}
		})
	}
}

func TestAuthRoutes_RegisterComplete_DuplicateEmail(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Create a user and verify their email
	user, _ := chattoCore.CreateUser(ctx, "system", "existinguser", "Existing User", "password123")
	chattoCore.AddVerifiedEmailDirect(ctx, user.Id, "taken@example.com")

	// Create a registration token for the same email
	// (simulating someone getting a token before the email was claimed)
	token, _ := chattoCore.CreateRegistrationToken(ctx, "taken@example.com")

	reqBody := map[string]string{
		"token":                token,
		"login":                "newuser",
		"password":             "password123",
		"passwordConfirmation": "password123",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		respBody, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 409 Conflict, got %d: %s", resp.StatusCode, string(respBody))
	}
}

func TestAuthRoutes_RegisterComplete_PasswordMismatch(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	token, _ := chattoCore.CreateRegistrationToken(ctx, "mismatch@example.com")

	reqBody := map[string]string{
		"token":                token,
		"login":                "mismatchuser",
		"password":             "password123",
		"passwordConfirmation": "different456",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_RegisterComplete_InvalidToken(t *testing.T) {
	ts, client, _, _ := setupTestHTTPServerWithMailer(t)

	reqBody := map[string]string{
		"token":                "nonexistent-token",
		"login":                "newuser",
		"password":             "password123",
		"passwordConfirmation": "password123",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_RegisterComplete_TokenNotConsumedOnFailure(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Create existing user to cause duplicate login
	chattoCore.CreateUser(ctx, "system", "takenlogin", "Taken", "password123")

	token, _ := chattoCore.CreateRegistrationToken(ctx, "retry@example.com")

	// First attempt: fails due to duplicate login
	reqBody := map[string]string{
		"token":                token,
		"login":                "takenlogin",
		"password":             "password123",
		"passwordConfirmation": "password123",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("Expected status 409, got %d", resp.StatusCode)
	}

	// Second attempt: should succeed with different login (token not consumed)
	reqBody["login"] = "differentlogin"
	body, _ = json.Marshal(reqBody)

	resp, err = client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 200 on retry, got %d: %s", resp.StatusCode, string(respBody))
	}
}

// setupTestHTTPServerWithRegistrationDisabled creates an HTTPServer with mailer enabled
// but registration explicitly disabled via config.
func setupTestHTTPServerWithRegistrationDisabled(t *testing.T) (*httptest.Server, *http.Client, *core.ChattoCore) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartNATS(t)

	ctx := testContext(t)

	coreConfig := config.CoreConfig{}
	chattoCore, err := core.NewChattoCore(ctx, nc, coreConfig)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

	router := gin.New()
	router.Use(gin.Recovery())

	sessionStore := cookie.NewStore([]byte("test-secret-key-32-bytes-long!!"))
	sessionStore.Options(sessions.Options{
		MaxAge:   60 * 60 * 24 * 90,
		HttpOnly: true,
		Secure:   false,
		Path:     "/",
	})
	router.Use(sessions.Sessions("chatto_session", sessionStore))

	mockMailer := email.NewMockSender(true)
	directRegistrationDisabled := false

	s := &HTTPServer{
		config: config.ChattoConfig{
			Auth: config.AuthConfig{
				DirectRegistration: &directRegistrationDisabled,
			},
			Webserver: config.WebserverConfig{
				URL:                 "http://localhost:4000",
				CookieSigningSecret: "test-secret-key-32-bytes-long!!",
			},
		},
		nc:     nc,
		router: router,
		core:   chattoCore,
		mailer: mockMailer,
	}

	s.setupAuthRoutes()

	ts := httptest.NewServer(router)
	t.Cleanup(func() { ts.Close() })

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return ts, client, chattoCore
}

func TestAuthRoutes_Register_DisabledReturns403(t *testing.T) {
	ts, client, _ := setupTestHTTPServerWithRegistrationDisabled(t)

	body, _ := json.Marshal(map[string]string{"email": "new@example.com"})
	resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["error"] != "Registration is disabled" {
		t.Errorf("Expected error 'Registration is disabled', got '%s'", respBody["error"])
	}
}

func TestAuthRoutes_RegisterComplete_DisabledReturns403(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServerWithRegistrationDisabled(t)
	ctx := testContext(t)

	// Create a token (simulating one created before registration was disabled)
	token, _ := chattoCore.CreateRegistrationToken(ctx, "disabled@example.com")

	body, _ := json.Marshal(map[string]string{
		"token":                token,
		"login":                "disableduser",
		"password":             "password123",
		"passwordConfirmation": "password123",
	})

	resp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register/complete request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["error"] != "Registration is disabled" {
		t.Errorf("Expected error 'Registration is disabled', got '%s'", respBody["error"])
	}
}

func TestAuthRoutes_EmailVerification_Success(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	// Create a user directly
	user, err := chattoCore.CreateUser(ctx, "system", "verifyuser", "Verify User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Verify the user does NOT have a verified email yet
	hasVerified, err := chattoCore.HasVerifiedEmail(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to check verified email: %v", err)
	}
	if hasVerified {
		t.Error("Expected hasVerifiedEmail to be false before verification")
	}

	// Create a verification token
	token, err := chattoCore.CreateEmailVerificationToken(ctx, user.Id, "verify@example.com")
	if err != nil {
		t.Fatalf("Failed to create verification token: %v", err)
	}

	// Call the verify-email endpoint
	verifyResp, err := client.Get(ts.URL + "/auth/verify-email?token=" + token)
	if err != nil {
		t.Fatalf("Failed to send verify request: %v", err)
	}
	defer verifyResp.Body.Close()

	// Should redirect (307) on success
	if verifyResp.StatusCode != http.StatusOK && verifyResp.StatusCode != http.StatusTemporaryRedirect {
		respBody, _ := io.ReadAll(verifyResp.Body)
		t.Fatalf("Expected redirect or OK, got %d: %s", verifyResp.StatusCode, string(respBody))
	}

	// Verify the user NOW has a verified email
	hasVerified, err = chattoCore.HasVerifiedEmail(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to check verified email after verification: %v", err)
	}
	if !hasVerified {
		t.Error("Expected hasVerifiedEmail to be true after verification")
	}

	// Check verified emails list
	verifiedEmails, err := chattoCore.GetVerifiedEmails(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get verified emails: %v", err)
	}
	if len(verifiedEmails) != 1 {
		t.Errorf("Expected 1 verified email, got %d", len(verifiedEmails))
	}
	if len(verifiedEmails) > 0 && verifiedEmails[0].Email != "verify@example.com" {
		t.Errorf("Expected verified email verify@example.com, got %s", verifiedEmails[0].Email)
	}
}

func TestAuthRoutes_EmailVerification_DuplicateEmail(t *testing.T) {
	ts, _, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	// Create first user with verified email
	user1, err := chattoCore.CreateUser(ctx, "system", "user1", "User 1", "password123")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}
	if err := chattoCore.AddVerifiedEmailDirect(ctx, user1.Id, "shared@example.com"); err != nil {
		t.Fatalf("Failed to verify email for user1: %v", err)
	}

	// Create second user
	user2, err := chattoCore.CreateUser(ctx, "system", "user2", "User 2", "password123")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// Create verification token for user2 with the same email
	token, err := chattoCore.CreateEmailVerificationToken(ctx, user2.Id, "shared@example.com")
	if err != nil {
		t.Fatalf("Failed to create verification token: %v", err)
	}

	// Try to verify - should fail because email is already claimed
	// Use a client that doesn't follow redirects
	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	verifyResp, err := noRedirectClient.Get(ts.URL + "/auth/verify-email?token=" + token)
	if err != nil {
		t.Fatalf("Failed to send verify request: %v", err)
	}
	defer verifyResp.Body.Close()

	// Should redirect to error page with email_taken
	if verifyResp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("Expected redirect (307), got %d", verifyResp.StatusCode)
	}
	location := verifyResp.Header.Get("Location")
	if !strings.Contains(location, "email_taken") {
		t.Errorf("Expected redirect to email_taken error, got Location: %s", location)
	}

	// Verify user2 still doesn't have a verified email
	hasVerified, err := chattoCore.HasVerifiedEmail(ctx, user2.Id)
	if err != nil {
		t.Fatalf("Failed to check verified email: %v", err)
	}
	if hasVerified {
		t.Error("Expected user2 to NOT have verified email")
	}
}

func TestAuthRoutes_RegisterComplete_ThenLogin(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Register via two-step flow
	token, _ := chattoCore.CreateRegistrationToken(ctx, "logintest@example.com")
	reqBody := map[string]string{
		"token":                token,
		"login":                "logintest",
		"password":             "password123",
		"passwordConfirmation": "password123",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to complete registration: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Registration failed with status %d", resp.StatusCode)
	}

	// Log out
	logoutResp, err := client.Post(ts.URL+"/auth/logout", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to logout: %v", err)
	}
	logoutResp.Body.Close()

	// Log in with the same credentials
	loginBody := map[string]string{
		"login":    "logintest",
		"password": "password123",
	}
	body, _ = json.Marshal(loginBody)

	loginResp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send login request: %v", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(loginResp.Body)
		t.Fatalf("Login failed with status %d: %s", loginResp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(loginResp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["success"] != true {
		t.Error("Expected success: true")
	}

	user, ok := result["user"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected user object")
	}
	if user["login"] != "logintest" {
		t.Errorf("Expected login logintest, got %v", user["login"])
	}
}

// TestAuthRoutes_Login_WithIdentifierField tests that the login endpoint
// accepts the "identifier" field name that the frontend uses.
func TestAuthRoutes_Login_WithIdentifierField(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	// Create user directly
	_, err := chattoCore.CreateUser(ctx, "system", "identifiertest", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Log in using "identifier" field (as frontend does)
	loginBody := map[string]string{
		"identifier": "identifiertest",
		"password":   "password123",
	}
	body, _ := json.Marshal(loginBody)

	loginResp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send login request: %v", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(loginResp.Body)
		t.Fatalf("Login with identifier field failed with status %d: %s", loginResp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(loginResp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["success"] != true {
		t.Error("Expected success: true")
	}

	user, ok := result["user"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected user object")
	}
	if user["login"] != "identifiertest" {
		t.Errorf("Expected login identifiertest, got %v", user["login"])
	}
}

// ============================================================================
// OAuth Auto-Verify Tests
//
// These tests verify the behavior that the OAuth callback relies on:
// 1. Creating a user without a password (OAuth users)
// 2. Auto-verifying the email from the OAuth provider
// 3. Finding users by verified email for subsequent logins
// ============================================================================

func TestOAuthFlow_NewUserAutoVerifiesEmail(t *testing.T) {
	_, _, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	// Simulate OAuth callback creating a new user
	// OAuth users are created with empty password
	login := "oauthuser"
	oauthEmail := "oauth@google.com"

	user, err := chattoCore.CreateUser(ctx, "system", login, "OAuth User", "")
	if err != nil {
		t.Fatalf("Failed to create OAuth user: %v", err)
	}

	// Verify user doesn't have verified email yet
	hasVerified, err := chattoCore.HasVerifiedEmail(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to check verified email: %v", err)
	}
	if hasVerified {
		t.Error("Expected hasVerifiedEmail to be false before auto-verify")
	}

	// Simulate auto-verify (what OAuth callback does after user creation)
	err = chattoCore.AddVerifiedEmailDirect(ctx, user.Id, oauthEmail)
	if err != nil {
		t.Fatalf("Failed to auto-verify OAuth email: %v", err)
	}

	// Verify user now has verified email
	hasVerified, err = chattoCore.HasVerifiedEmail(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to check verified email after auto-verify: %v", err)
	}
	if !hasVerified {
		t.Error("Expected hasVerifiedEmail to be true after auto-verify")
	}

	// Verify the specific email is in the verified list
	verifiedEmails, err := chattoCore.GetVerifiedEmails(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get verified emails: %v", err)
	}
	if len(verifiedEmails) != 1 {
		t.Errorf("Expected 1 verified email, got %d", len(verifiedEmails))
	}
	if len(verifiedEmails) > 0 && verifiedEmails[0].Email != oauthEmail {
		t.Errorf("Expected verified email %s, got %s", oauthEmail, verifiedEmails[0].Email)
	}
}

func TestOAuthFlow_ExistingUserFoundByVerifiedEmail(t *testing.T) {
	_, _, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	// Create a user with verified email (simulating previous OAuth registration)
	login := "existingoauth"
	oauthEmail := "existing@google.com"

	user, err := chattoCore.CreateUser(ctx, "system", login, "Existing OAuth User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	err = chattoCore.AddVerifiedEmailDirect(ctx, user.Id, oauthEmail)
	if err != nil {
		t.Fatalf("Failed to verify email: %v", err)
	}

	// Simulate OAuth callback looking up user by email
	foundUser, err := chattoCore.GetUserByVerifiedEmail(ctx, oauthEmail)
	if err != nil {
		t.Fatalf("Failed to find user by verified email: %v", err)
	}

	if foundUser.Id != user.Id {
		t.Errorf("Expected to find user %s, got %s", user.Id, foundUser.Id)
	}
	if foundUser.Login != login {
		t.Errorf("Expected login %s, got %s", login, foundUser.Login)
	}
}

func TestOAuthFlow_EmailLookupIsCaseInsensitive(t *testing.T) {
	_, _, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	// Create user with mixed-case email
	user, err := chattoCore.CreateUser(ctx, "system", "caseuser", "Case User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	err = chattoCore.AddVerifiedEmailDirect(ctx, user.Id, "CaseTest@Google.COM")
	if err != nil {
		t.Fatalf("Failed to verify email: %v", err)
	}

	// OAuth provider may return email in different case
	foundUser, err := chattoCore.GetUserByVerifiedEmail(ctx, "casetest@google.com")
	if err != nil {
		t.Fatalf("Failed to find user with lowercase email: %v", err)
	}

	if foundUser.Id != user.Id {
		t.Errorf("Expected to find user %s, got %s", user.Id, foundUser.Id)
	}
}

func TestOAuthFlow_EmailAlreadyClaimedByAnotherUser(t *testing.T) {
	_, _, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	// Create first user with verified email
	user1, err := chattoCore.CreateUser(ctx, "system", "oauthuser1", "OAuth User 1", "")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}

	err = chattoCore.AddVerifiedEmailDirect(ctx, user1.Id, "claimed@google.com")
	if err != nil {
		t.Fatalf("Failed to verify email for user1: %v", err)
	}

	// Create second user (simulating different OAuth account trying to claim same email)
	user2, err := chattoCore.CreateUser(ctx, "system", "oauthuser2", "OAuth User 2", "")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// Try to auto-verify same email for second user - should fail
	err = chattoCore.AddVerifiedEmailDirect(ctx, user2.Id, "claimed@google.com")
	if err == nil {
		t.Error("Expected error when trying to claim already-verified email")
	}

	// User2 should not have any verified email
	hasVerified, err := chattoCore.HasVerifiedEmail(ctx, user2.Id)
	if err != nil {
		t.Fatalf("Failed to check verified email for user2: %v", err)
	}
	if hasVerified {
		t.Error("Expected user2 to NOT have verified email")
	}
}

// ============================================================================
// Registration Email Tests
//
// These tests verify that the registration endpoint sends registration emails
// with correct content using MockSender.
// ============================================================================

func TestAuthRoutes_Register_EmailContainsValidToken(t *testing.T) {
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Register with email
	reqBody := map[string]string{"email": "tokentest@example.com"}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register request: %v", err)
	}
	resp.Body.Close()

	// Get the captured email
	msg := mockMailer.LastMessage()
	if msg == nil {
		t.Fatal("Expected email to be sent")
	}

	// Extract token from email body
	tokenRegex := regexp.MustCompile(`token=([a-zA-Z0-9_-]+)`)
	matches := tokenRegex.FindStringSubmatch(msg.Body)
	if len(matches) < 2 {
		t.Fatalf("Could not extract token from email body: %s", msg.Body)
	}
	token := matches[1]

	// Token should be valid and usable for registration
	tokenData, err := chattoCore.GetRegistrationToken(ctx, token)
	if err != nil {
		t.Fatalf("Token from email should be valid: %v", err)
	}
	if tokenData.Email != "tokentest@example.com" {
		t.Errorf("Expected email tokentest@example.com, got %s", tokenData.Email)
	}

	// Verify email content
	if !strings.Contains(msg.Body, "Welcome to Chatto!") {
		t.Error("Expected welcome message in email body")
	}
	if !strings.Contains(msg.Body, "24 hours") {
		t.Error("Expected 24-hour expiration mention in email body")
	}
	if !strings.Contains(msg.Body, "/register/complete?token=RG") {
		t.Error("Expected registration link with RG token prefix in email body")
	}
}

func TestAuthRoutes_TestEmailEndpoint(t *testing.T) {
	ts, client, _, mockMailer := setupTestHTTPServerWithMailer(t)

	// Trigger a registration email
	reqBody := map[string]string{"email": "testendpoint@example.com"}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register request: %v", err)
	}
	resp.Body.Close()

	// Verify email was captured
	if mockMailer.LastMessage() == nil {
		t.Fatal("Expected email to be captured")
	}

	// Test the /auth/test/last-email endpoint
	emailResp, err := client.Get(ts.URL + "/auth/test/last-email")
	if err != nil {
		t.Fatalf("Failed to get last email: %v", err)
	}
	defer emailResp.Body.Close()

	if emailResp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", emailResp.StatusCode)
	}

	var emailResult map[string]interface{}
	if err := json.NewDecoder(emailResp.Body).Decode(&emailResult); err != nil {
		t.Fatalf("Failed to decode email response: %v", err)
	}

	if emailResult["to"] != "testendpoint@example.com" {
		t.Errorf("Expected to: testendpoint@example.com, got %v", emailResult["to"])
	}
	if emailResult["subject"] != "Complete your Chatto registration" {
		t.Errorf("Expected subject: 'Complete your Chatto registration', got %v", emailResult["subject"])
	}

	// Test DELETE /auth/test/emails
	req, _ := http.NewRequest("DELETE", ts.URL+"/auth/test/emails", nil)
	deleteResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to delete emails: %v", err)
	}
	deleteResp.Body.Close()

	if deleteResp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", deleteResp.StatusCode)
	}

	if mockMailer.LastMessage() != nil {
		t.Error("Expected emails to be cleared")
	}
}

// ============================================================================
// Password Reset Tests
// ============================================================================

func TestAuthRoutes_ForgotPassword_SendsEmail(t *testing.T) {
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Create a user with verified email
	user, err := chattoCore.CreateUser(ctx, "system", "forgotuser", "Forgot User", "oldpassword")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	err = chattoCore.AddVerifiedEmailDirect(ctx, user.Id, "forgot@example.com")
	if err != nil {
		t.Fatalf("Failed to verify email: %v", err)
	}

	// Request password reset
	reqBody := map[string]string{"email": "forgot@example.com"}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/forgot-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send forgot-password request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response message (should be generic)
	msg, ok := result["message"].(string)
	if !ok || !strings.Contains(msg, "If that email is registered") {
		t.Errorf("Expected generic message, got: %v", result["message"])
	}

	// Verify email was sent
	email := mockMailer.LastMessage()
	if email == nil {
		t.Fatal("Expected password reset email to be sent")
	}

	if email.To != "forgot@example.com" {
		t.Errorf("Expected email to forgot@example.com, got %s", email.To)
	}
	if email.Subject != "Reset your Chatto password" {
		t.Errorf("Expected subject 'Reset your Chatto password', got %s", email.Subject)
	}
	if !strings.Contains(email.Body, "/reset-password?token=PR") {
		t.Errorf("Expected email body to contain reset link with PR token, got: %s", email.Body)
	}
	if !strings.Contains(email.Body, "1 hour") {
		t.Errorf("Expected email body to mention 1-hour expiration")
	}
}

func TestAuthRoutes_ForgotPassword_NoEnumeration(t *testing.T) {
	ts, client, _, mockMailer := setupTestHTTPServerWithMailer(t)

	// Request password reset for non-existent email
	reqBody := map[string]string{"email": "nonexistent@example.com"}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/forgot-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send forgot-password request: %v", err)
	}
	defer resp.Body.Close()

	// Should still return 200 to prevent email enumeration
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should return same message as when email exists
	msg, ok := result["message"].(string)
	if !ok || !strings.Contains(msg, "If that email is registered") {
		t.Errorf("Expected generic message, got: %v", result["message"])
	}

	// No email should be sent for non-existent address
	if mockMailer.LastMessage() != nil {
		t.Error("Should not send email for non-existent address")
	}
}

func TestAuthRoutes_ForgotPassword_InvalidEmail(t *testing.T) {
	ts, client, _, _ := setupTestHTTPServerWithMailer(t)

	// Request password reset with invalid email format
	reqBody := map[string]string{"email": "not-an-email"}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/forgot-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send forgot-password request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_ResetPassword_Success(t *testing.T) {
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Create a user with verified email
	user, err := chattoCore.CreateUser(ctx, "system", "resetuser", "Reset User", "oldpassword123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	err = chattoCore.AddVerifiedEmailDirect(ctx, user.Id, "reset@example.com")
	if err != nil {
		t.Fatalf("Failed to verify email: %v", err)
	}

	// Request password reset
	forgotBody := map[string]string{"email": "reset@example.com"}
	body, _ := json.Marshal(forgotBody)
	resp, err := client.Post(ts.URL+"/auth/forgot-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send forgot-password request: %v", err)
	}
	resp.Body.Close()

	// Extract token from email
	email := mockMailer.LastMessage()
	if email == nil {
		t.Fatal("Expected password reset email to be sent")
	}

	tokenRegex := regexp.MustCompile(`token=([a-zA-Z0-9_-]+)`)
	matches := tokenRegex.FindStringSubmatch(email.Body)
	if len(matches) < 2 {
		t.Fatalf("Could not extract token from email body: %s", email.Body)
	}
	token := matches[1]

	// Reset password
	resetBody := map[string]string{
		"token":    token,
		"password": "newpassword456",
	}
	body, _ = json.Marshal(resetBody)
	resetResp, err := client.Post(ts.URL+"/auth/reset-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send reset-password request: %v", err)
	}
	defer resetResp.Body.Close()

	if resetResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resetResp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resetResp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resetResp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !strings.Contains(result["message"].(string), "Password has been reset") {
		t.Errorf("Expected success message, got: %v", result["message"])
	}

	// Verify new password works
	_, err = chattoCore.VerifyPassword(ctx, "resetuser", "newpassword456")
	if err != nil {
		t.Errorf("New password should work: %v", err)
	}

	// Verify old password no longer works
	_, err = chattoCore.VerifyPassword(ctx, "resetuser", "oldpassword123")
	if err == nil {
		t.Error("Old password should not work")
	}
}

func TestAuthRoutes_ResetPassword_InvalidToken(t *testing.T) {
	ts, client, _, _ := setupTestHTTPServerWithMailer(t)

	resetBody := map[string]string{
		"token":    "PRinvalidtoken123456",
		"password": "newpassword456",
	}
	body, _ := json.Marshal(resetBody)

	resp, err := client.Post(ts.URL+"/auth/reset-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send reset-password request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !strings.Contains(result["error"].(string), "Invalid or expired") {
		t.Errorf("Expected 'Invalid or expired' error, got: %v", result["error"])
	}
}

func TestAuthRoutes_ResetPassword_TokenCanOnlyBeUsedOnce(t *testing.T) {
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Create user
	user, err := chattoCore.CreateUser(ctx, "system", "singleuseuser", "Single Use User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	chattoCore.AddVerifiedEmailDirect(ctx, user.Id, "singleuse@example.com")

	// Request reset
	forgotBody := map[string]string{"email": "singleuse@example.com"}
	body, _ := json.Marshal(forgotBody)
	client.Post(ts.URL+"/auth/forgot-password", "application/json", bytes.NewReader(body))

	// Extract token
	email := mockMailer.LastMessage()
	tokenRegex := regexp.MustCompile(`token=([a-zA-Z0-9_-]+)`)
	matches := tokenRegex.FindStringSubmatch(email.Body)
	token := matches[1]

	// First reset succeeds
	resetBody := map[string]string{"token": token, "password": "newpass1234"}
	body, _ = json.Marshal(resetBody)
	resp1, _ := client.Post(ts.URL+"/auth/reset-password", "application/json", bytes.NewReader(body))
	resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("First reset should succeed, got %d", resp1.StatusCode)
	}

	// Second reset with same token fails
	resetBody2 := map[string]string{"token": token, "password": "newpass5678"}
	body, _ = json.Marshal(resetBody2)
	resp2, err := client.Post(ts.URL+"/auth/reset-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Second reset request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("Second reset should fail, got %d", resp2.StatusCode)
	}
}

func TestAuthRoutes_ResetPassword_ShortPassword(t *testing.T) {
	ts, client, _, _ := setupTestHTTPServerWithMailer(t)

	resetBody := map[string]string{
		"token":    "PRsomevalidtoken123",
		"password": "short", // Less than 8 characters
	}
	body, _ := json.Marshal(resetBody)

	resp, err := client.Post(ts.URL+"/auth/reset-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send reset-password request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for short password, got %d", resp.StatusCode)
	}
}

func TestAuthRoutes_CompletePasswordResetFlow(t *testing.T) {
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// 1. Create user with verified email
	user, _ := chattoCore.CreateUser(ctx, "system", "flowuser", "Flow User", "originalpass")
	chattoCore.AddVerifiedEmailDirect(ctx, user.Id, "flow@example.com")

	// 2. Login with original password works
	loginBody := map[string]string{"login": "flowuser", "password": "originalpass"}
	body, _ := json.Marshal(loginBody)
	loginResp, _ := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatal("Original login should work")
	}

	// Clear session
	client.Post(ts.URL+"/auth/logout", "application/json", nil)

	// 3. Request password reset
	forgotBody := map[string]string{"email": "flow@example.com"}
	body, _ = json.Marshal(forgotBody)
	client.Post(ts.URL+"/auth/forgot-password", "application/json", bytes.NewReader(body))

	// 4. Extract token and reset password
	email := mockMailer.LastMessage()
	tokenRegex := regexp.MustCompile(`token=([a-zA-Z0-9_-]+)`)
	matches := tokenRegex.FindStringSubmatch(email.Body)
	token := matches[1]

	resetBody := map[string]string{"token": token, "password": "brandnewpass"}
	body, _ = json.Marshal(resetBody)
	resetResp, _ := client.Post(ts.URL+"/auth/reset-password", "application/json", bytes.NewReader(body))
	resetResp.Body.Close()
	if resetResp.StatusCode != http.StatusOK {
		t.Fatal("Reset should succeed")
	}

	// 5. Login with new password works
	newLoginBody := map[string]string{"login": "flowuser", "password": "brandnewpass"}
	body, _ = json.Marshal(newLoginBody)
	newLoginResp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Login with new password failed: %v", err)
	}
	defer newLoginResp.Body.Close()
	if newLoginResp.StatusCode != http.StatusOK {
		t.Error("Login with new password should work")
	}

	// 6. Login with old password fails
	oldLoginBody := map[string]string{"login": "flowuser", "password": "originalpass"}
	body, _ = json.Marshal(oldLoginBody)
	oldLoginResp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Login with old password failed: %v", err)
	}
	defer oldLoginResp.Body.Close()
	if oldLoginResp.StatusCode != http.StatusUnauthorized {
		t.Error("Login with old password should fail")
	}
}

// ============================================================================
// Bearer Token Auth Tests
// ============================================================================

func TestAuthRoutes_Login_ReturnsToken(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	// Create a user
	chattoCore.CreateUser(ctx, "", "tokenuser", "Token User", "password123")

	// Login
	loginBody := map[string]string{"login": "tokenuser", "password": "password123"}
	body, _ := json.Marshal(loginBody)
	resp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Login status = %d, want 200", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	token, ok := result["token"].(string)
	if !ok || token == "" {
		t.Fatal("Login response should include a non-empty 'token' field")
	}

	if !strings.HasPrefix(token, "cht_AT") {
		t.Errorf("Token %q should start with 'cht_AT'", token)
	}
}

func TestAuthRoutes_RevokeToken(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	chattoCore.CreateUser(ctx, "", "revokeuser", "Revoke User", "password123")

	// Login to get a token
	loginBody := map[string]string{"login": "revokeuser", "password": "password123"}
	body, _ := json.Marshal(loginBody)
	resp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}
	defer resp.Body.Close()

	var loginResult map[string]any
	json.NewDecoder(resp.Body).Decode(&loginResult)
	token := loginResult["token"].(string)

	// Revoke the token
	revokeBody := map[string]string{"token": token}
	body, _ = json.Marshal(revokeBody)
	revokeResp, err := client.Post(ts.URL+"/auth/revoke-token", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Revoke request failed: %v", err)
	}
	defer revokeResp.Body.Close()

	if revokeResp.StatusCode != http.StatusOK {
		t.Fatalf("Revoke status = %d, want 200", revokeResp.StatusCode)
	}

	// Verify token is no longer valid
	_, err = chattoCore.ValidateAuthToken(ctx, token)
	if err != core.ErrAuthTokenNotFound {
		t.Errorf("Token should be invalid after revocation, got err: %v", err)
	}
}

func TestAuthRoutes_RegisterComplete_ReturnsToken(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Create a registration token directly
	regToken, err := chattoCore.CreateRegistrationToken(ctx, "newuser@example.com")
	if err != nil {
		t.Fatalf("Failed to create registration token: %v", err)
	}

	// Complete registration
	regBody := map[string]string{
		"token":                regToken,
		"login":                "newuser",
		"password":             "password123",
		"passwordConfirmation": "password123",
	}
	body, _ := json.Marshal(regBody)
	resp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Register complete request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Register complete status = %d, want 200, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	token, ok := result["token"].(string)
	if !ok || token == "" {
		t.Fatal("Register complete response should include a non-empty 'token' field")
	}

	if !strings.HasPrefix(token, "cht_AT") {
		t.Errorf("Token %q should start with 'cht_AT'", token)
	}
}
