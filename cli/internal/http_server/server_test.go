package http_server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
	"hmans.de/chatto/internal/evtstream"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestEnsureAutocertCacheDir(t *testing.T) {
	t.Run("creates a private directory", func(t *testing.T) {
		cacheDir := filepath.Join(t.TempDir(), "nested", "autocert")
		if err := ensureAutocertCacheDir(cacheDir); err != nil {
			t.Fatalf("ensureAutocertCacheDir() error = %v", err)
		}
		assertPathMode(t, cacheDir, autocertCacheDirMode)
	})

	t.Run("preserves a private directory", func(t *testing.T) {
		cacheDir := filepath.Join(t.TempDir(), "autocert")
		if err := os.Mkdir(cacheDir, autocertCacheDirMode); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		if err := ensureAutocertCacheDir(cacheDir); err != nil {
			t.Fatalf("ensureAutocertCacheDir() error = %v", err)
		}
		assertPathMode(t, cacheDir, autocertCacheDirMode)
	})

	t.Run("repairs a permissive directory", func(t *testing.T) {
		cacheDir := filepath.Join(t.TempDir(), "autocert")
		if err := os.Mkdir(cacheDir, 0o755); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		if err := ensureAutocertCacheDir(cacheDir); err != nil {
			t.Fatalf("ensureAutocertCacheDir() error = %v", err)
		}
		assertPathMode(t, cacheDir, autocertCacheDirMode)
	})

	t.Run("rejects a non-directory path", func(t *testing.T) {
		cacheDir := filepath.Join(t.TempDir(), "autocert")
		if err := os.WriteFile(cacheDir, []byte("not a directory"), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := ensureAutocertCacheDir(cacheDir); err == nil {
			t.Fatal("ensureAutocertCacheDir() error = nil, want non-directory error")
		}
	})

	t.Run("rejects a symlink", func(t *testing.T) {
		parent := t.TempDir()
		target := filepath.Join(parent, "target")
		if err := os.Mkdir(target, autocertCacheDirMode); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		cacheDir := filepath.Join(parent, "autocert")
		if err := os.Symlink(target, cacheDir); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		if err := ensureAutocertCacheDir(cacheDir); err == nil || !strings.Contains(err.Error(), "is not a directory") {
			t.Fatalf("ensureAutocertCacheDir() error = %v, want symlink rejection", err)
		}
	})

	t.Run("rejects a replaceable cache path", func(t *testing.T) {
		parent := t.TempDir()
		cacheDir := filepath.Join(parent, "autocert")
		if err := os.Mkdir(cacheDir, autocertCacheDirMode); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		if err := os.Chmod(parent, 0o777); err != nil {
			t.Fatalf("Chmod() error = %v", err)
		}
		if err := ensureAutocertCacheDir(cacheDir); err == nil || !strings.Contains(err.Error(), "writable by group or other users") {
			t.Fatalf("ensureAutocertCacheDir() error = %v, want unsafe-parent rejection", err)
		}
	})

	t.Run("rejects a directory owned by another user", func(t *testing.T) {
		if os.Geteuid() != 0 {
			t.Skip("changing directory ownership requires root")
		}
		cacheDir := filepath.Join(t.TempDir(), "autocert")
		if err := os.Mkdir(cacheDir, autocertCacheDirMode); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		if err := os.Chown(cacheDir, 1, -1); err != nil {
			t.Fatalf("Chown() error = %v", err)
		}
		if err := ensureAutocertCacheDir(cacheDir); err == nil || !strings.Contains(err.Error(), "is owned by uid") {
			t.Fatalf("ensureAutocertCacheDir() error = %v, want ownership rejection", err)
		}
	})

	t.Run("rejects a parent directory owned by another user", func(t *testing.T) {
		if os.Geteuid() != 0 {
			t.Skip("changing directory ownership requires root")
		}
		parent := t.TempDir()
		cacheDir := filepath.Join(parent, "autocert")
		if err := os.Mkdir(cacheDir, autocertCacheDirMode); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		if err := os.Chown(parent, 1, -1); err != nil {
			t.Fatalf("Chown() error = %v", err)
		}
		if err := ensureAutocertCacheDir(cacheDir); err == nil || !strings.Contains(err.Error(), "parent directory") || !strings.Contains(err.Error(), "is owned by uid") {
			t.Fatalf("ensureAutocertCacheDir() error = %v, want parent ownership rejection", err)
		}
	})
}

func assertPathMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode = %04o, want %04o", got, want)
	}
}

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

func TestLimitLegacyRequestBodyRejectsOversizedBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/legacy", limitLegacyRequestBody(), func(c *gin.Context) {
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/legacy", strings.NewReader(strings.Repeat("x", legacyRequestBodyLimit+1)))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestLimitRequestBodyTimesOutSlowClients(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	completed := make(chan struct{})
	router.POST("/legacy", limitRequestBody(1024, 50*time.Millisecond), func(c *gin.Context) {
		defer close(completed)
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		c.Status(http.StatusNoContent)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := newHTTPServer(listener.Addr().String(), router)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "POST /legacy HTTP/1.1\r\nHost: test\r\nContent-Type: application/json\r\nContent-Length: 10\r\n\r\n{"); err != nil {
		t.Fatalf("write partial request: %v", err)
	}
	select {
	case <-completed:
	case <-time.After(2 * time.Second):
		t.Fatal("handler remained blocked reading a slow request body")
	}
}

func TestShutdownServerForcesCloseAfterTimeout(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			enteredOnce.Do(func() { close(entered) })
			<-release
		}),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			t.Errorf("Serve returned unexpected error: %v", err)
		}
	}()

	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		_, _ = http.Get("http://" + ln.Addr().String())
	}()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler did not receive request")
	}

	shutdownDone := make(chan error, 1)
	testServer := &HTTPServer{logger: log.WithPrefix("test.HTTP")}
	go func() { shutdownDone <- testServer.shutdownServerWithTimeout(srv, 25*time.Millisecond) }()

	select {
	case err := <-shutdownDone:
		if err == nil {
			t.Fatal("shutdownServer returned nil; wanted graceful shutdown timeout")
		}
	case <-time.After(time.Second):
		t.Fatal("shutdownServer did not return after forced close")
	}

	close(release)
	select {
	case <-clientDone:
	case <-time.After(time.Second):
		t.Fatal("client request did not release after forced close")
	}
}

// testHTTPServer creates an HTTPServer for testing with an embedded NATS server.
// Returns the test server, a client with cookie jar, and ChattoCore.
func setupTestHTTPServer(t *testing.T) (*httptest.Server, *http.Client, *core.ChattoCore) {
	return setupTestHTTPServerWithHook(t, nil)
}

func setupTestHTTPServerWithHook(t *testing.T, configure func(*HTTPServer)) (*httptest.Server, *http.Client, *core.ChattoCore) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartSharedNATS(t)

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
	browserStore := newJetStreamBrowserSessionStore(chattoCore)
	s.browserSessions = newBrowserSessionManager(browserStore, s.config.Auth.TokenTTLOrDefault(), false)
	if configure != nil {
		configure(s)
	}

	// Set up auth routes only for focused testing.
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

func postBrowserAuthentication(client *http.Client, endpoint string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(connectapi.BrowserAuthenticationModeHeader, connectapi.BrowserAuthenticationModeCookie)
	req.Header.Set("Origin", req.URL.Scheme+"://"+req.URL.Host)
	if client.Jar != nil {
		for _, cookie := range client.Jar.Cookies(req.URL) {
			if cookie.Name == csrfCookieName {
				req.Header.Set(csrfHeaderName, cookie.Value)
			}
		}
	}
	return client.Do(req)
}

// setupTestHTTPServerWithMailer creates an HTTPServer with MockSender enabled.
// Returns the test server, client, ChattoCore, and the MockSender for inspection.
func setupTestHTTPServerWithMailer(t *testing.T) (*httptest.Server, *http.Client, *core.ChattoCore, *email.MockSender) {
	return setupTestHTTPServerWithMailerConfig(t, config.EmailOTPConfig{})
}

func setupTestHTTPServerWithMailerConfig(t *testing.T, emailOTP config.EmailOTPConfig) (*httptest.Server, *http.Client, *core.ChattoCore, *email.MockSender) {
	return setupTestHTTPServerWithMailerAuthConfig(t, config.AuthConfig{EmailOTP: emailOTP})
}

func setupTestHTTPServerWithMailerAuthConfig(t *testing.T, authConfig config.AuthConfig) (*httptest.Server, *http.Client, *core.ChattoCore, *email.MockSender) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartSharedNATS(t)

	ctx := testContext(t)

	// Create ChattoCore
	coreConfig := config.CoreConfig{EmailOTP: authConfig.EmailOTP}
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
			Auth: authConfig,
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

func setTestServerName(t *testing.T, ctx context.Context, chattoCore *core.ChattoCore, name string) {
	t.Helper()
	if err := chattoCore.ConfigModel().SetServerConfig(ctx, "test", &configv1.ServerConfig{ServerName: name}); err != nil {
		t.Fatalf("Failed to set test server name: %v", err)
	}
}

// ============================================================================
// Auth Route Integration Tests
// ============================================================================

func TestAuthRoutes_InviteOnlyRegistration(t *testing.T) {
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailerAuthConfig(t, config.AuthConfig{
		AccountCreationPolicy: config.AccountCreationPolicyInviteOnly,
	})
	ctx := testContext(t)
	admin, err := chattoCore.CreateUser(ctx, core.SystemActorID, "http-invite-admin", "HTTP Invite Admin", "password123")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := chattoCore.AssignAdminRole(ctx, admin.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	maxUses := uint32(1)
	invitation, err := chattoCore.CreateInvitation(ctx, admin.Id, &maxUses, nil)
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	invitePath := chattoCore.InvitationLinkPath(invitation.ID)

	postRegistration := func() (int, string) {
		t.Helper()
		body, _ := json.Marshal(map[string]string{
			"email": "invited@example.test",
		})
		resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST /auth/register: %v", err)
		}
		defer resp.Body.Close()
		responseBody, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(responseBody)
	}

	missingStatus, _ := postRegistration()
	if missingStatus != http.StatusBadRequest {
		t.Fatalf("registration without invite link status = %d, want %d", missingStatus, http.StatusBadRequest)
	}

	invalidResp, err := client.Get(ts.URL + "/invite/not-a-valid-token")
	if err != nil {
		t.Fatalf("GET invalid invite link: %v", err)
	}
	invalidResp.Body.Close()
	if invalidResp.StatusCode != http.StatusSeeOther || invalidResp.Header.Get("Location") != "/register?error=invalid_invitation" {
		t.Fatalf("invalid invite link = %d %q", invalidResp.StatusCode, invalidResp.Header.Get("Location"))
	}

	tamperedPath := invitePath[:len(invitePath)-1] + "A"
	if invitePath[len(invitePath)-1] == 'A' {
		tamperedPath = invitePath[:len(invitePath)-1] + "B"
	}
	tamperedResp, err := client.Get(ts.URL + tamperedPath)
	if err != nil {
		t.Fatalf("GET tampered invite link: %v", err)
	}
	tamperedResp.Body.Close()
	if tamperedResp.StatusCode != http.StatusSeeOther || tamperedResp.Header.Get("Location") != "/register?error=invalid_invitation" {
		t.Fatalf("tampered invite link = %d %q", tamperedResp.StatusCode, tamperedResp.Header.Get("Location"))
	}

	inviteResp, err := client.Get(ts.URL + invitePath)
	if err != nil {
		t.Fatalf("GET valid invite link: %v", err)
	}
	inviteResp.Body.Close()
	if inviteResp.StatusCode != http.StatusSeeOther || inviteResp.Header.Get("Location") != "/register?invited=1" {
		t.Fatalf("valid invite link = %d %q", inviteResp.StatusCode, inviteResp.Header.Get("Location"))
	}
	if inviteResp.Header.Get("Cache-Control") != "no-store" || inviteResp.Header.Get("Referrer-Policy") != "no-referrer" || inviteResp.Header.Get("X-Robots-Tag") != "noindex, nofollow" {
		t.Fatalf("valid invite-link privacy headers = %v", inviteResp.Header)
	}
	if status, body := postRegistration(); status != http.StatusOK {
		t.Fatalf("valid invitation registration status = %d: %s", status, body)
	}

	message := mockMailer.LastMessage()
	if message == nil {
		t.Fatal("valid invitation did not send a registration email")
	}
	oneTimeCode := regexp.MustCompile(`\b\d{6}\b`).FindString(message.Body)
	verifyBody, _ := json.Marshal(map[string]string{"email": "invited@example.test", "code": oneTimeCode})
	verifyResp, err := client.Post(ts.URL+"/auth/register/verify-code", "application/json", bytes.NewReader(verifyBody))
	if err != nil {
		t.Fatalf("verify registration code: %v", err)
	}
	defer verifyResp.Body.Close()
	var verified map[string]string
	if err := json.NewDecoder(verifyResp.Body).Decode(&verified); err != nil {
		t.Fatalf("decode verified registration: %v", err)
	}
	completionBody, _ := json.Marshal(map[string]string{
		"token":                verified["completionToken"],
		"login":                "http-invited",
		"password":             "password123",
		"passwordConfirmation": "password123",
	})
	completionResp, err := client.Post(ts.URL+"/auth/register/complete", "application/json", bytes.NewReader(completionBody))
	if err != nil {
		t.Fatalf("complete invited registration: %v", err)
	}
	defer completionResp.Body.Close()
	if completionResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(completionResp.Body)
		t.Fatalf("complete invited registration status = %d: %s", completionResp.StatusCode, body)
	}
	state, err := chattoCore.GetInvitation(ctx, admin.Id, invitation.ID)
	if err != nil {
		t.Fatalf("GetInvitation: %v", err)
	}
	if state.UseCount != 1 {
		t.Fatalf("invitation use count = %d, want 1", state.UseCount)
	}
}

func TestAuthRoutes_OpenRegistrationIgnoresInviteLinks(t *testing.T) {
	ts, client, _, mockMailer := setupTestHTTPServerWithMailer(t)
	inviteResp, err := client.Get(ts.URL + "/invite/not-a-valid-token")
	if err != nil {
		t.Fatalf("GET invite link: %v", err)
	}
	inviteResp.Body.Close()
	if inviteResp.StatusCode != http.StatusSeeOther || inviteResp.Header.Get("Location") != "/register" {
		t.Fatalf("open-mode invite link = %d %q", inviteResp.StatusCode, inviteResp.Header.Get("Location"))
	}
	body, _ := json.Marshal(map[string]string{
		"email": "open-with-link@example.test",
	})
	resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /auth/register: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || mockMailer.LastMessage() == nil {
		responseBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("open registration status = %d, email = %v: %s", resp.StatusCode, mockMailer.LastMessage() != nil, responseBody)
	}
}

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
	if token, _ := result["token"].(string); !strings.HasPrefix(token, "cht_AT") {
		t.Fatalf("login access token = %q", token)
	}
	if refreshToken, _ := result["refreshToken"].(string); !strings.HasPrefix(refreshToken, "cht_RT_") {
		t.Fatalf("login refresh token = %q", refreshToken)
	}
	if result["expiresIn"].(float64) <= 0 || result["refreshTokenExpiresIn"].(float64) <= 0 {
		t.Fatalf("login lifetimes = %v/%v", result["expiresIn"], result["refreshTokenExpiresIn"])
	}
}

func TestAuthRoutes_BrowserLoginDoesNotIssueBearerCredentials(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)
	if _, err := chattoCore.CreateUser(ctx, "system", "cookieonly", "Cookie Only", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"login":    "cookieonly",
		"password": "password123",
	})
	resp, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
	if err != nil {
		t.Fatalf("POST /auth/browser/login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("login status = %d, want 200: %s", resp.StatusCode, responseBody)
	}
	authCookieCount := 0
	for _, cookie := range resp.Cookies() {
		if isBrowserSessionCookieName(cookie.Name) {
			authCookieCount++
		}
	}
	if authCookieCount != 1 {
		t.Fatalf("browser login set %d authentication cookies, want 1", authCookieCount)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	for _, field := range []string{"token", "refreshToken", "expiresIn", "refreshTokenExpiresIn"} {
		if _, exists := result[field]; exists {
			t.Fatalf("cookie-only login response contains %q", field)
		}
	}
	if cookies := client.Jar.Cookies(resp.Request.URL); len(cookies) == 0 {
		t.Fatal("browser login did not save a browser session")
	}
}

func TestAuthRoutes_BrowserLoginRejectsCrossSiteAndFormRequests(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)
	if _, err := chattoCore.CreateUser(ctx, "system", "login-csrf", "Login CSRF", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"login": "login-csrf", "password": "password123"})

	tests := []struct {
		name        string
		contentType string
		origin      string
		mode        string
		wantStatus  int
	}{
		{name: "plain HTML form", contentType: "text/plain", origin: "https://attacker.example", wantStatus: http.StatusUnsupportedMediaType},
		{name: "cross-origin fetch", contentType: "application/json", origin: "https://attacker.example", mode: connectapi.BrowserAuthenticationModeCookie, wantStatus: http.StatusForbidden},
		{name: "missing browser mode", contentType: "application/json", origin: ts.URL, wantStatus: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/auth/browser/login", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			req.Header.Set("Content-Type", test.contentType)
			req.Header.Set("Origin", test.origin)
			if test.mode != "" {
				req.Header.Set(connectapi.BrowserAuthenticationModeHeader, test.mode)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("browser login: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != test.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, test.wantStatus)
			}
			for _, cookie := range resp.Cookies() {
				if isBrowserSessionCookieName(cookie.Name) {
					t.Fatalf("rejected browser login set %s", browserSessionCookieName)
				}
			}
		})
	}
}

func TestAuthRoutes_ProgrammaticLoginRejectsFormContentType(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)
	if _, err := chattoCore.CreateUser(ctx, "system", "form-login", "Form Login", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	body := []byte(`{"login":"form-login","password":"password123","ignore":"="}`)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/auth/login", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Origin", "https://attacker.example")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("programmatic login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", resp.StatusCode)
	}
	for _, cookie := range resp.Cookies() {
		if isBrowserSessionCookieName(cookie.Name) {
			t.Fatalf("form login set %s", browserSessionCookieName)
		}
	}
}

func TestAuthRoutes_BrowserLoginRotatesHandle(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)
	for _, account := range []struct {
		login string
		name  string
	}{
		{login: "browser-first", name: "Browser First"},
		{login: "browser-second", name: "Browser Second"},
	} {
		if _, err := chattoCore.CreateUser(ctx, "system", account.login, account.name, "password123"); err != nil {
			t.Fatalf("CreateUser %q: %v", account.login, err)
		}
	}

	login := func(identifier string) string {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"login": identifier, "password": "password123"})
		response, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
		if err != nil {
			t.Fatalf("browser login %q: %v", identifier, err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("browser login %q status = %d, want 200", identifier, response.StatusCode)
		}
		for _, cookie := range client.Jar.Cookies(response.Request.URL) {
			if isBrowserSessionCookieName(cookie.Name) {
				return cookie.Value
			}
		}
		t.Fatalf("browser login %q did not set %s", identifier, browserSessionCookieName)
		return ""
	}

	firstHandle := login("browser-first")
	secondHandle := login("browser-second")
	if secondHandle == firstHandle {
		t.Fatalf("browser login reused handle %q across an authentication change", firstHandle)
	}
	if _, err := chattoCore.ValidateCookieCredential(ctx, firstHandle); !errors.Is(err, core.ErrCookieSessionNotFound) {
		t.Fatalf("old browser handle validation error = %v, want ErrCookieSessionNotFound", err)
	}
	record, err := chattoCore.ValidateCookieCredential(ctx, secondHandle)
	if err != nil {
		t.Fatalf("ValidateCookieCredential: %v", err)
	}
	secondUser, err := chattoCore.GetUserByLogin(ctx, "browser-second")
	if err != nil {
		t.Fatalf("GetUserByLogin: %v", err)
	}
	if record.GetUserId() != secondUser.GetId() {
		t.Fatalf("browser handle user = %q, want %q", record.GetUserId(), secondUser.GetId())
	}
}

func TestAuthRoutes_BrowserSessionRenewalKeepsHandleAndExtendsWindow(t *testing.T) {
	var server *HTTPServer
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(configured *HTTPServer) {
		server = configured
	})
	ctx := testContext(t)
	if _, err := chattoCore.CreateUser(ctx, "system", "browser-renew", "Browser Renew", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"login": "browser-renew", "password": "password123"})
	loginResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
	if err != nil {
		t.Fatalf("browser login: %v", err)
	}
	loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("browser login status = %d, want 200", loginResponse.StatusCode)
	}

	var sessionID, csrfToken string
	for _, cookie := range client.Jar.Cookies(loginResponse.Request.URL) {
		switch cookie.Name {
		case csrfCookieName:
			csrfToken = cookie.Value
		default:
			if isBrowserSessionCookieName(cookie.Name) {
				sessionID = cookie.Value
			}
		}
	}
	if sessionID == "" || csrfToken == "" {
		t.Fatalf("browser login cookies: session=%t csrf=%t", sessionID != "", csrfToken != "")
	}
	before, err := chattoCore.ValidateCookieCredential(ctx, sessionID)
	if err != nil {
		t.Fatalf("ValidateCookieCredential before renewal: %v", err)
	}
	noOpResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/session/renew", []byte("{}"))
	if err != nil {
		t.Fatalf("early browser renewal: %v", err)
	}
	var noOpBody struct {
		Renewed    bool   `json:"renewed"`
		RenewAfter string `json:"renewAfter"`
	}
	if err := json.NewDecoder(noOpResponse.Body).Decode(&noOpBody); err != nil {
		t.Fatalf("decode early renewal response: %v", err)
	}
	noOpResponse.Body.Close()
	if noOpResponse.StatusCode != http.StatusOK || noOpBody.Renewed || noOpBody.RenewAfter == "" {
		t.Fatalf("early renewal status/body = %d/%+v", noOpResponse.StatusCode, noOpBody)
	}
	var noOpSessionID string
	for _, cookie := range client.Jar.Cookies(noOpResponse.Request.URL) {
		if isBrowserSessionCookieName(cookie.Name) {
			noOpSessionID = cookie.Value
		}
	}
	if noOpSessionID != sessionID {
		t.Fatalf("early renewal changed handle from %q to %q", sessionID, noOpSessionID)
	}
	server.cookieSessionRenewalNow = func() time.Time {
		return before.GetExpiresAt().AsTime().Add(-time.Hour)
	}

	renewResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/session/renew", []byte("{}"))
	if err != nil {
		t.Fatalf("browser renewal: %v", err)
	}
	defer renewResponse.Body.Close()
	if renewResponse.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(renewResponse.Body)
		t.Fatalf("browser renewal status = %d, want 200: %s", renewResponse.StatusCode, responseBody)
	}

	var renewedSessionID string
	for _, cookie := range client.Jar.Cookies(renewResponse.Request.URL) {
		if isBrowserSessionCookieName(cookie.Name) {
			renewedSessionID = cookie.Value
		}
	}
	if renewedSessionID != sessionID {
		t.Fatalf("browser renewal changed handle from %q to %q", sessionID, renewedSessionID)
	}
	after, err := chattoCore.ValidateCookieCredential(ctx, sessionID)
	if err != nil {
		t.Fatalf("ValidateCookieCredential after renewal: %v", err)
	}
	if !after.GetExpiresAt().AsTime().After(before.GetExpiresAt().AsTime()) {
		t.Fatalf("browser renewal expiry = %v, want after %v", after.GetExpiresAt().AsTime(), before.GetExpiresAt().AsTime())
	}
}

func TestAuthRoutes_MigratesPreviousBrowserCookieOnce(t *testing.T) {
	var legacySessionID string
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(server *HTTPServer) {
		server.router.Use(server.csrfMiddleware())
		server.router.GET("/test/set-legacy-browser-cookie", func(c *gin.Context) {
			session := sessions.Default(c)
			session.Set(sessionKeyRuntimeCredentialID, legacySessionID)
			if err := session.Save(); err != nil {
				c.Status(http.StatusInternalServerError)
				return
			}
			c.Status(http.StatusNoContent)
		})
		server.router.GET("/test/browser-user", func(c *gin.Context) {
			credential, ok, err := server.cookiePresentedCredential(c)
			if err != nil {
				c.Status(http.StatusServiceUnavailable)
				return
			}
			if !ok {
				c.Status(http.StatusUnauthorized)
				return
			}
			c.JSON(http.StatusOK, gin.H{"userId": credential.auth.UserID})
		})
		server.router.GET("/test/legacy-browser-cookie", func(c *gin.Context) {
			if _, ok := legacyCookieSessionID(sessions.Default(c)); ok {
				c.Status(http.StatusOK)
				return
			}
			c.Status(http.StatusNoContent)
		})
	})
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, core.SystemActorID, "legacy-browser-cookie", "Legacy Browser Cookie", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	legacySessionID, _, err = chattoCore.CreateCookieSession(ctx, user.GetId(), "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}

	seedResponse, err := client.Get(ts.URL + "/test/set-legacy-browser-cookie")
	if err != nil {
		t.Fatalf("set legacy browser cookie: %v", err)
	}
	seedResponse.Body.Close()
	if seedResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("set legacy browser cookie status = %d, want 204", seedResponse.StatusCode)
	}
	requestURL, err := url.Parse(ts.URL + "/auth/browser/session/migrate")
	if err != nil {
		t.Fatalf("Parse migration URL: %v", err)
	}
	staleCookieName, err := newBrowserSessionCookieName()
	if err != nil {
		t.Fatalf("newBrowserSessionCookieName: %v", err)
	}
	client.Jar.SetCookies(requestURL, []*http.Cookie{{
		Name:  staleCookieName,
		Value: "revoked-handle",
		Path:  "/",
	}})
	beforeResponse, err := client.Get(ts.URL + "/test/browser-user")
	if err != nil {
		t.Fatalf("check legacy browser cookie: %v", err)
	}
	beforeResponse.Body.Close()
	if beforeResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ordinary authentication accepted legacy cookie with status %d", beforeResponse.StatusCode)
	}

	migrateResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/session/migrate", []byte("{}"))
	if err != nil {
		t.Fatalf("migrate legacy browser cookie: %v", err)
	}
	migrateResponse.Body.Close()
	if migrateResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("migrate legacy browser cookie status = %d, want 204", migrateResponse.StatusCode)
	}
	if cacheControl := migrateResponse.Header.Get("Cache-Control"); !strings.Contains(cacheControl, "no-store") {
		t.Fatalf("migration Cache-Control = %q, want no-store", cacheControl)
	}

	var migratedHandle, csrfToken string
	authCookieCount := 0
	for _, cookie := range client.Jar.Cookies(migrateResponse.Request.URL) {
		switch {
		case isBrowserSessionCookieName(cookie.Name):
			authCookieCount++
			migratedHandle = cookie.Value
		case cookie.Name == csrfCookieName:
			csrfToken = cookie.Value
		}
	}
	if authCookieCount != 1 || migratedHandle != legacySessionID || csrfToken == "" {
		t.Fatalf("migrated browser cookies: count=%d handle=%q csrf=%t", authCookieCount, migratedHandle, csrfToken != "")
	}

	afterResponse, err := client.Get(ts.URL + "/test/browser-user")
	if err != nil {
		t.Fatalf("check migrated browser cookie: %v", err)
	}
	defer afterResponse.Body.Close()
	if afterResponse.StatusCode != http.StatusOK {
		t.Fatalf("migrated browser authentication status = %d, want 200", afterResponse.StatusCode)
	}
	var afterBody struct {
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(afterResponse.Body).Decode(&afterBody); err != nil {
		t.Fatalf("decode migrated browser user: %v", err)
	}
	if afterBody.UserID != user.GetId() {
		t.Fatalf("migrated browser user = %q, want %q", afterBody.UserID, user.GetId())
	}

	legacyResponse, err := client.Get(ts.URL + "/test/legacy-browser-cookie")
	if err != nil {
		t.Fatalf("check retired legacy cookie field: %v", err)
	}
	legacyResponse.Body.Close()
	if legacyResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("legacy authentication field remains after migration: status %d", legacyResponse.StatusCode)
	}

	retryResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/session/migrate", []byte("{}"))
	if err != nil {
		t.Fatalf("retry legacy browser migration: %v", err)
	}
	retryResponse.Body.Close()
	if retryResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("retry legacy browser migration status = %d, want 204", retryResponse.StatusCode)
	}
}

func TestAuthRoutes_BrowserSessionMigrationRequiresSameOriginProof(t *testing.T) {
	ts, client, _ := setupTestHTTPServer(t)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/auth/browser/session/migrate", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(connectapi.BrowserAuthenticationModeHeader, connectapi.BrowserAuthenticationModeCookie)
	req.Header.Set("Origin", "https://attacker.example")
	response, err := client.Do(req)
	if err != nil {
		t.Fatalf("cross-origin migration: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin migration status = %d, want 403", response.StatusCode)
	}
	if cookies := response.Header.Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("cross-origin migration Set-Cookie = %v, want none", cookies)
	}
}

func TestAuthRoutes_DelayedRenewalCannotOverwriteNewLogin(t *testing.T) {
	var server *HTTPServer
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(configured *HTTPServer) {
		server = configured
	})
	ctx := testContext(t)
	for _, account := range []struct {
		login string
		name  string
	}{
		{login: "renewal-old-login", name: "Renewal Old Login"},
		{login: "renewal-new-login", name: "Renewal New Login"},
	} {
		if _, err := chattoCore.CreateUser(ctx, "system", account.login, account.name, "password123"); err != nil {
			t.Fatalf("CreateUser %q: %v", account.login, err)
		}
	}

	login := func(identifier string) {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"login": identifier, "password": "password123"})
		response, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
		if err != nil {
			t.Fatalf("browser login %q: %v", identifier, err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			responseBody, _ := io.ReadAll(response.Body)
			t.Fatalf("browser login %q status = %d: %s", identifier, response.StatusCode, responseBody)
		}
	}

	login("renewal-old-login")
	requestURL, err := url.Parse(ts.URL + "/auth/browser/session/renew")
	if err != nil {
		t.Fatalf("Parse renewal URL: %v", err)
	}
	var oldHandle string
	for _, cookie := range client.Jar.Cookies(requestURL) {
		if isBrowserSessionCookieName(cookie.Name) {
			oldHandle = cookie.Value
		}
	}
	oldRecord, err := chattoCore.ValidateCookieCredential(ctx, oldHandle)
	if err != nil {
		t.Fatalf("ValidateCookieCredential old login: %v", err)
	}
	server.cookieSessionRenewalNow = func() time.Time {
		return oldRecord.GetExpiresAt().AsTime().Add(-time.Hour)
	}

	// Execute renewal without a cookie jar so its Set-Cookie fields can be
	// applied after the later login, reproducing browser response reordering.
	renewRequest, err := http.NewRequest(http.MethodPost, requestURL.String(), strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("NewRequest renewal: %v", err)
	}
	renewRequest.Header.Set("Content-Type", "application/json")
	renewRequest.Header.Set(connectapi.BrowserAuthenticationModeHeader, connectapi.BrowserAuthenticationModeCookie)
	renewRequest.Header.Set("Origin", requestURL.Scheme+"://"+requestURL.Host)
	for _, cookie := range client.Jar.Cookies(requestURL) {
		renewRequest.AddCookie(cookie)
		if cookie.Name == csrfCookieName {
			renewRequest.Header.Set(csrfHeaderName, cookie.Value)
		}
	}
	delayedRenewal, err := ts.Client().Do(renewRequest)
	if err != nil {
		t.Fatalf("delayed renewal: %v", err)
	}
	defer delayedRenewal.Body.Close()
	if delayedRenewal.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(delayedRenewal.Body)
		t.Fatalf("delayed renewal status = %d: %s", delayedRenewal.StatusCode, responseBody)
	}
	if got := len(delayedRenewal.Cookies()); got > 2 {
		t.Fatalf("renewal Set-Cookie count = %d, want at most 2", got)
	}

	login("renewal-new-login")
	client.Jar.SetCookies(requestURL, delayedRenewal.Cookies())

	checkRequest := httptest.NewRequest(http.MethodGet, requestURL.String(), nil)
	for _, cookie := range client.Jar.Cookies(requestURL) {
		checkRequest.AddCookie(cookie)
	}
	checkContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	checkContext.Request = checkRequest
	credential, ok, err := server.cookiePresentedCredential(checkContext)
	if err != nil || !ok {
		t.Fatalf("cookie authentication after reordered responses = %t, %v", ok, err)
	}
	newUser, err := chattoCore.GetUserByLogin(ctx, "renewal-new-login")
	if err != nil {
		t.Fatalf("GetUserByLogin new login: %v", err)
	}
	if credential.auth.UserID != newUser.GetId() {
		t.Fatalf("reordered response selected user %q, want %q", credential.auth.UserID, newUser.GetId())
	}
	if _, err := chattoCore.ValidateCookieCredential(ctx, oldHandle); !errors.Is(err, core.ErrCookieSessionNotFound) {
		t.Fatalf("old renewed handle validation error = %v, want ErrCookieSessionNotFound", err)
	}

	// The next maintenance request removes the orphaned stale slot and keeps a
	// single cookie carrying the current handle.
	maintenance, err := postBrowserAuthentication(client, requestURL.String(), []byte("{}"))
	if err != nil {
		t.Fatalf("maintenance after reordered responses: %v", err)
	}
	defer maintenance.Body.Close()
	if maintenance.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(maintenance.Body)
		t.Fatalf("maintenance status = %d: %s", maintenance.StatusCode, responseBody)
	}
	authCookies := 0
	for _, cookie := range client.Jar.Cookies(requestURL) {
		if isBrowserSessionCookieName(cookie.Name) {
			authCookies++
		}
	}
	if authCookies != 1 {
		t.Fatalf("authentication cookie count after maintenance = %d, want 1", authCookies)
	}
}

func TestAuthRoutes_ConcurrentRenewalSlotsConverge(t *testing.T) {
	var server *HTTPServer
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(configured *HTTPServer) {
		server = configured
	})
	ctx := testContext(t)
	if _, err := chattoCore.CreateUser(ctx, "system", "concurrent-renewal", "Concurrent Renewal", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"login": "concurrent-renewal", "password": "password123"})
	loginResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
	if err != nil {
		t.Fatalf("browser login: %v", err)
	}
	loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("browser login status = %d, want 200", loginResponse.StatusCode)
	}

	requestURL, err := url.Parse(ts.URL + "/auth/browser/session/renew")
	if err != nil {
		t.Fatalf("Parse renewal URL: %v", err)
	}
	requestCookies := client.Jar.Cookies(requestURL)
	var handle string
	var csrfToken string
	for _, cookie := range requestCookies {
		if isBrowserSessionCookieName(cookie.Name) {
			handle = cookie.Value
		}
		if cookie.Name == csrfCookieName {
			csrfToken = cookie.Value
		}
	}
	record, err := chattoCore.ValidateCookieCredential(ctx, handle)
	if err != nil {
		t.Fatalf("ValidateCookieCredential: %v", err)
	}
	server.cookieSessionRenewalNow = func() time.Time {
		return record.GetExpiresAt().AsTime().Add(-time.Hour)
	}

	const responseCount = browserSessionCookieLimit + 1
	delayedCookies := make([][]*http.Cookie, 0, responseCount)
	for range responseCount {
		request, err := http.NewRequest(http.MethodPost, requestURL.String(), strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("NewRequest renewal: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(connectapi.BrowserAuthenticationModeHeader, connectapi.BrowserAuthenticationModeCookie)
		request.Header.Set("Origin", requestURL.Scheme+"://"+requestURL.Host)
		request.Header.Set(csrfHeaderName, csrfToken)
		for _, cookie := range requestCookies {
			request.AddCookie(cookie)
		}
		response, err := ts.Client().Do(request)
		if err != nil {
			t.Fatalf("concurrent renewal: %v", err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("concurrent renewal status = %d, want 200", response.StatusCode)
		}
		delayedCookies = append(delayedCookies, response.Cookies())
	}
	for index := len(delayedCookies) - 1; index >= 0; index-- {
		client.Jar.SetCookies(requestURL, delayedCookies[index])
	}
	authCookies := 0
	for _, cookie := range client.Jar.Cookies(requestURL) {
		if isBrowserSessionCookieName(cookie.Name) {
			authCookies++
		}
	}
	if authCookies != responseCount {
		t.Fatalf("concurrent authentication cookie count = %d, want %d", authCookies, responseCount)
	}

	checkRequest := httptest.NewRequest(http.MethodGet, requestURL.String(), nil)
	for _, cookie := range client.Jar.Cookies(requestURL) {
		checkRequest.AddCookie(cookie)
	}
	checkContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	checkContext.Request = checkRequest
	credential, ok, err := server.cookiePresentedCredential(checkContext)
	if err != nil || !ok || credential.auth.Handle != handle {
		t.Fatalf("cookie authentication with concurrent slots = %t, %q, %v", ok, credential.auth.Handle, err)
	}

	maintenance, err := postBrowserAuthentication(client, requestURL.String(), []byte("{}"))
	if err != nil {
		t.Fatalf("maintenance after concurrent renewal: %v", err)
	}
	defer maintenance.Body.Close()
	if maintenance.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(maintenance.Body)
		t.Fatalf("maintenance status = %d: %s", maintenance.StatusCode, responseBody)
	}
	authCookies = 0
	for _, cookie := range client.Jar.Cookies(requestURL) {
		if isBrowserSessionCookieName(cookie.Name) {
			authCookies++
		}
	}
	if authCookies != 1 {
		t.Fatalf("authentication cookie count after convergence = %d, want 1", authCookies)
	}
}

func TestAuthRoutes_BrowserRenewalRevokesParallelSessionHandles(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)
	if _, err := chattoCore.CreateUser(ctx, "system", "parallel-browser-login", "Parallel Browser Login", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	secondJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	secondClient := &http.Client{Jar: secondJar, CheckRedirect: client.CheckRedirect}
	body, _ := json.Marshal(map[string]string{"login": "parallel-browser-login", "password": "password123"})
	for _, loginClient := range []*http.Client{client, secondClient} {
		response, err := postBrowserAuthentication(loginClient, ts.URL+"/auth/browser/login", body)
		if err != nil {
			t.Fatalf("browser login: %v", err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("browser login status = %d, want 200", response.StatusCode)
		}
	}

	requestURL, err := url.Parse(ts.URL + "/auth/browser/session/renew")
	if err != nil {
		t.Fatalf("Parse renewal URL: %v", err)
	}
	cookieURL, err := url.Parse(ts.URL + "/")
	if err != nil {
		t.Fatalf("Parse cookie URL: %v", err)
	}
	handles := make([]string, 0, 2)
	for _, loginClient := range []*http.Client{client, secondClient} {
		for _, cookie := range loginClient.Jar.Cookies(requestURL) {
			if !isBrowserSessionCookieName(cookie.Name) {
				continue
			}
			handles = append(handles, cookie.Value)
			if loginClient == secondClient {
				client.Jar.SetCookies(cookieURL, []*http.Cookie{cookie})
			}
		}
	}
	if len(handles) != 2 {
		t.Fatalf("parallel browser handle count = %d, want 2", len(handles))
	}

	renewal, err := postBrowserAuthentication(client, requestURL.String(), []byte("{}"))
	if err != nil {
		t.Fatalf("browser renewal: %v", err)
	}
	defer renewal.Body.Close()
	if renewal.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(renewal.Body)
		t.Fatalf("browser renewal status = %d: %s", renewal.StatusCode, responseBody)
	}

	validHandles := 0
	for _, handle := range handles {
		if _, err := chattoCore.ValidateCookieCredential(ctx, handle); err == nil {
			validHandles++
		} else if !errors.Is(err, core.ErrCookieSessionNotFound) {
			t.Fatalf("ValidateCookieCredential: %v", err)
		}
	}
	if validHandles != 1 {
		t.Fatalf("valid parallel browser handles after renewal = %d, want 1", validHandles)
	}
	authCookies := 0
	for _, cookie := range client.Jar.Cookies(requestURL) {
		if isBrowserSessionCookieName(cookie.Name) {
			authCookies++
		}
	}
	if authCookies != 1 {
		t.Fatalf("authentication cookie count after parallel cleanup = %d, want 1", authCookies)
	}
}

func TestAuthenticatedPublicDiscoveryDoesNotSetCookies(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(server *HTTPServer) {
		server.router.Use(server.csrfMiddleware())
		server.setupConnectAPI()
	})
	ctx := testContext(t)
	if _, err := chattoCore.CreateUser(ctx, "system", "discovery-cookie", "Discovery Cookie", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"login": "discovery-cookie", "password": "password123"})
	loginResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
	if err != nil {
		t.Fatalf("browser login: %v", err)
	}
	loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("browser login status = %d, want 200", loginResponse.StatusCode)
	}

	discoveryResponse, err := client.Get(ts.URL + serverDiscoveryConnectPath + "?connect=v1&encoding=json&message=%7B%7D")
	if err != nil {
		t.Fatalf("authenticated discovery GET: %v", err)
	}
	defer discoveryResponse.Body.Close()
	if discoveryResponse.StatusCode != http.StatusOK {
		t.Fatalf("authenticated discovery status = %d, want 200", discoveryResponse.StatusCode)
	}
	if got := discoveryResponse.Header.Get("Cache-Control"); got != "public, no-cache" {
		t.Fatalf("authenticated discovery Cache-Control = %q, want public, no-cache", got)
	}
	if cookies := discoveryResponse.Header.Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("authenticated discovery Set-Cookie = %v, want none", cookies)
	}
}

func TestAuthRoutes_BrowserMigrationRevokesBearerAuthorityAndKeepsCookie(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, "system", "browser-migrate", "Browser Migrate", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"login": "browser-migrate", "password": "password123"})
	loginResponse, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResponse.StatusCode)
	}
	var credentials struct {
		AccessToken  string `json:"token"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(loginResponse.Body).Decode(&credentials); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if got, err := chattoCore.ValidateAuthToken(ctx, credentials.AccessToken); err != nil || got != user.Id {
		t.Fatalf("ValidateAuthToken before migration = %q, %v", got, err)
	}
	browserLoginResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
	if err != nil {
		t.Fatalf("browser login: %v", err)
	}
	browserLoginResponse.Body.Close()
	if browserLoginResponse.StatusCode != http.StatusOK {
		t.Fatalf("browser login status = %d, want 200", browserLoginResponse.StatusCode)
	}

	var sessionID string
	for _, cookie := range client.Jar.Cookies(browserLoginResponse.Request.URL) {
		if isBrowserSessionCookieName(cookie.Name) {
			sessionID = cookie.Value
		}
	}
	if sessionID == "" {
		t.Fatal("login did not set the browser session cookie")
	}

	revokeBody, _ := json.Marshal(map[string]string{
		"accessToken":  credentials.AccessToken,
		"refreshToken": credentials.RefreshToken,
	})
	revokeResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/revoke-bearer-session", revokeBody)
	if err != nil {
		t.Fatalf("revoke bearer session: %v", err)
	}
	defer revokeResponse.Body.Close()
	if revokeResponse.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(revokeResponse.Body)
		t.Fatalf("revoke bearer status = %d, want 200: %s", revokeResponse.StatusCode, responseBody)
	}
	if _, err := chattoCore.ValidateAuthToken(ctx, credentials.AccessToken); !errors.Is(err, core.ErrAuthTokenNotFound) {
		t.Fatalf("ValidateAuthToken after migration err = %v, want ErrAuthTokenNotFound", err)
	}
	if record, err := chattoCore.ValidateCookieCredential(ctx, sessionID); err != nil || record.GetUserId() != user.Id {
		t.Fatalf("ValidateCookieCredential after migration = %v, %v", record, err)
	}
}

func TestAuthRoutes_Login_DisabledReturns403BeforeCredentialValidation(t *testing.T) {
	disabled := false
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(server *HTTPServer) {
		server.config.Auth.DirectLogin = &disabled
	})
	ctx := testContext(t)
	if _, err := chattoCore.CreateUser(ctx, "system", "disabledlogin", "Disabled Login", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"login":    "disabledlogin",
		"password": "definitely-wrong",
	})
	resp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /auth/login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	var responseBody map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if responseBody["error"] != "Password login is disabled" {
		t.Fatalf("error = %q, want password-login-disabled response", responseBody["error"])
	}
	if cookies := client.Jar.Cookies(resp.Request.URL); len(cookies) != 0 {
		t.Fatalf("disabled login created cookies: %+v", cookies)
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
	user, err := chattoCore.CreateUser(ctx, "system", login, "Test User", password)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Login first
	loginBody := map[string]string{
		"login":    login,
		"password": password,
	}
	body, _ := json.Marshal(loginBody)

	loginResp, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed with status %d", loginResp.StatusCode)
	}

	deleted, err := chattoCore.RevokeCookieSessionsForUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to inspect cookie session after login: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("Login created %d cookie sessions, want 1", deleted)
	}

	loginResp, err = postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
	if err != nil {
		t.Fatalf("Failed to login again: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("Second login failed with status %d", loginResp.StatusCode)
	}

	// Now logout
	logoutResp, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/logout", []byte("{}"))
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

	deleted, err = chattoCore.RevokeCookieSessionsForUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to inspect cookie session after logout: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("Logout left %d cookie sessions behind, want 0", deleted)
	}
}

func TestAuthRoutes_LogoutReportsAuthoritativeRevocationFailure(t *testing.T) {
	var server *HTTPServer
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(configured *HTTPServer) {
		server = configured
	})
	ctx := testContext(t)
	if _, err := chattoCore.CreateUser(ctx, "system", "logout-unavailable", "Logout Unavailable", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"login": "logout-unavailable", "password": "password123"})
	loginResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResponse.StatusCode)
	}

	server.nc.Close()
	logoutResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/logout", []byte("{}"))
	if err != nil {
		t.Fatalf("logout request: %v", err)
	}
	defer logoutResponse.Body.Close()
	if logoutResponse.StatusCode != http.StatusServiceUnavailable {
		responseBody, _ := io.ReadAll(logoutResponse.Body)
		t.Fatalf("logout status = %d, want 503: %s", logoutResponse.StatusCode, responseBody)
	}
	if cookies := client.Jar.Cookies(loginResponse.Request.URL); len(cookies) == 0 {
		t.Fatal("failed logout cleared the browser session")
	}
}

func TestAuthRoutes_CrossSiteLogoutCannotClearBrowserAuthentication(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, core.SystemActorID, "logout-csrf", "Logout CSRF", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"login": user.GetLogin(), "password": "password123"})
	loginResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
	if err != nil {
		t.Fatalf("browser login: %v", err)
	}
	loginResponse.Body.Close()
	var sessionID, csrfToken string
	for _, cookie := range client.Jar.Cookies(loginResponse.Request.URL) {
		switch cookie.Name {
		case csrfCookieName:
			csrfToken = cookie.Value
		default:
			if isBrowserSessionCookieName(cookie.Name) {
				sessionID = cookie.Value
			}
		}
	}
	if sessionID == "" || csrfToken == "" {
		t.Fatal("browser login did not establish session and CSRF cookies")
	}

	// SameSite=Lax withholds cookies on a cross-site POST, but a browser still
	// applies Set-Cookie from the response. The programmatic route must not emit
	// deletion cookies when an empty HTML form reaches it.
	formRequest, err := http.NewRequest(http.MethodPost, ts.URL+"/auth/logout", strings.NewReader(""))
	if err != nil {
		t.Fatalf("NewRequest form logout: %v", err)
	}
	formRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	formRequest.Header.Set("Origin", "https://attacker.example")
	formResponse, err := ts.Client().Do(formRequest)
	if err != nil {
		t.Fatalf("form logout: %v", err)
	}
	formResponse.Body.Close()
	if cookies := formResponse.Header.Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("programmatic form logout Set-Cookie = %v, want none", cookies)
	}

	crossSiteRequest, err := http.NewRequest(http.MethodPost, ts.URL+"/auth/browser/logout", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("NewRequest browser logout: %v", err)
	}
	crossSiteRequest.Header.Set("Content-Type", "application/json")
	crossSiteRequest.Header.Set(connectapi.BrowserAuthenticationModeHeader, connectapi.BrowserAuthenticationModeCookie)
	crossSiteRequest.Header.Set(csrfHeaderName, csrfToken)
	crossSiteRequest.Header.Set("Origin", "https://attacker.example")
	for _, cookie := range client.Jar.Cookies(loginResponse.Request.URL) {
		crossSiteRequest.AddCookie(cookie)
	}
	crossSiteResponse, err := ts.Client().Do(crossSiteRequest)
	if err != nil {
		t.Fatalf("cross-site browser logout: %v", err)
	}
	crossSiteResponse.Body.Close()
	if crossSiteResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site browser logout status = %d, want 403", crossSiteResponse.StatusCode)
	}
	if cookies := crossSiteResponse.Header.Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("rejected browser logout Set-Cookie = %v, want none", cookies)
	}
	if _, err := chattoCore.ValidateCookieCredential(ctx, sessionID); err != nil {
		t.Fatalf("cross-site logout revoked browser session: %v", err)
	}
}

func TestAuthRoutes_StaleBrowserSessionCanLogoutOnlyFromSameOrigin(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(server *HTTPServer) {
		server.router.Use(server.csrfMiddleware())
	})
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, core.SystemActorID, "stale-logout", "Stale Logout", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"login": user.GetLogin(), "password": "password123"})
	loginResponse, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
	if err != nil {
		t.Fatalf("browser login: %v", err)
	}
	loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("browser login status = %d, want 200", loginResponse.StatusCode)
	}

	requestURL, err := url.Parse(ts.URL + "/auth/browser/logout")
	if err != nil {
		t.Fatalf("Parse logout URL: %v", err)
	}
	presentedCookies := client.Jar.Cookies(requestURL)
	var sessionID, csrfToken string
	for _, cookie := range presentedCookies {
		switch cookie.Name {
		case csrfCookieName:
			csrfToken = cookie.Value
		default:
			if isBrowserSessionCookieName(cookie.Name) {
				sessionID = cookie.Value
			}
		}
	}
	if sessionID == "" || csrfToken == "" {
		t.Fatal("browser login did not establish session and CSRF cookies")
	}

	// The browser-route proof does not replace the signed CSRF proof while the
	// cookie still carries valid ambient authority.
	liveRequest, err := http.NewRequest(http.MethodPost, requestURL.String(), strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("NewRequest live logout: %v", err)
	}
	liveRequest.Header.Set("Content-Type", "application/json")
	liveRequest.Header.Set(connectapi.BrowserAuthenticationModeHeader, connectapi.BrowserAuthenticationModeCookie)
	liveRequest.Header.Set("Origin", requestURL.Scheme+"://"+requestURL.Host)
	liveResponse, err := client.Do(liveRequest)
	if err != nil {
		t.Fatalf("live logout without CSRF proof: %v", err)
	}
	liveResponse.Body.Close()
	if liveResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("live logout without CSRF proof status = %d, want 403", liveResponse.StatusCode)
	}
	if cookies := liveResponse.Header.Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("rejected live logout Set-Cookie = %v, want none", cookies)
	}
	if _, err := chattoCore.ValidateCookieCredential(ctx, sessionID); err != nil {
		t.Fatalf("rejected live logout changed session authority: %v", err)
	}
	if err := chattoCore.RevokeCookieSession(ctx, sessionID); err != nil {
		t.Fatalf("RevokeCookieSession: %v", err)
	}

	// An invalid session has no remaining authority, but its cookies must not
	// make the browser logout route vulnerable to cross-site cookie clearing.
	crossSiteRequest, err := http.NewRequest(http.MethodPost, requestURL.String(), strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("NewRequest cross-site logout: %v", err)
	}
	crossSiteRequest.Header.Set("Content-Type", "application/json")
	crossSiteRequest.Header.Set(connectapi.BrowserAuthenticationModeHeader, connectapi.BrowserAuthenticationModeCookie)
	crossSiteRequest.Header.Set(csrfHeaderName, csrfToken)
	crossSiteRequest.Header.Set("Origin", "https://attacker.example")
	for _, cookie := range presentedCookies {
		crossSiteRequest.AddCookie(cookie)
	}
	crossSiteResponse, err := ts.Client().Do(crossSiteRequest)
	if err != nil {
		t.Fatalf("cross-site stale logout: %v", err)
	}
	crossSiteResponse.Body.Close()
	if crossSiteResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site stale logout status = %d, want 403", crossSiteResponse.StatusCode)
	}
	if cookies := crossSiteResponse.Header.Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("cross-site stale logout Set-Cookie = %v, want none", cookies)
	}

	logoutRequest, err := http.NewRequest(http.MethodPost, requestURL.String(), strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("NewRequest same-origin stale logout: %v", err)
	}
	logoutRequest.Header.Set("Content-Type", "application/json")
	logoutRequest.Header.Set(connectapi.BrowserAuthenticationModeHeader, connectapi.BrowserAuthenticationModeCookie)
	logoutRequest.Header.Set("Origin", requestURL.Scheme+"://"+requestURL.Host)
	logoutResponse, err := client.Do(logoutRequest)
	if err != nil {
		t.Fatalf("same-origin stale logout: %v", err)
	}
	defer logoutResponse.Body.Close()
	if logoutResponse.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(logoutResponse.Body)
		t.Fatalf("same-origin stale logout status = %d, want 200: %s", logoutResponse.StatusCode, responseBody)
	}
	for _, cookie := range client.Jar.Cookies(requestURL) {
		if isBrowserSessionCookieName(cookie.Name) || cookie.Name == csrfCookieName {
			t.Fatalf("same-origin stale logout retained cookie %q", cookie.Name)
		}
	}
}

func TestAuthRoutes_LogoutWithBearerTokenRevokesAndAudits(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
	ctx := testContext(t)

	user, err := chattoCore.CreateUser(ctx, "system", "logoutbearer", "Logout Bearer", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := chattoCore.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/auth/logout", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("logout request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("logout status = %d, want 200: %s", resp.StatusCode, string(body))
	}
	if _, err := chattoCore.ValidateAuthToken(ctx, token); !errors.Is(err, core.ErrAuthTokenNotFound) {
		t.Fatalf("ValidateAuthToken after logout err = %v, want ErrAuthTokenNotFound", err)
	}

	logoutEvents, _, err := chattoCore.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventLogoutSucceeded))
	if err != nil {
		t.Fatalf("SubjectEvents logout: %v", err)
	}
	if len(logoutEvents) != 1 {
		t.Fatalf("logout events = %d, want 1", len(logoutEvents))
	}
}

func TestAuthRoutes_Register_SendsRegistrationEmail(t *testing.T) {
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)
	setTestServerName(t, ctx, chattoCore, "Engineering")

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
	if !ok || !strings.Contains(msg, "registration code") {
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
	if email.Subject != "Complete your registration for Engineering" {
		t.Errorf("Expected subject 'Complete your registration for Engineering', got %s", email.Subject)
	}
	if !strings.Contains(email.Body, "Welcome to Engineering!") {
		t.Errorf("Expected email body to include server name welcome, got: %s", email.Body)
	}
	if !strings.Contains(email.Body, "finish creating your account on Engineering") {
		t.Errorf("Expected email body to describe account creation on server, got: %s", email.Body)
	}
	if regexp.MustCompile(`\b\d{6}\b`).FindString(email.Body) == "" {
		t.Errorf("Expected email body to contain six-digit registration code, got: %s", email.Body)
	}
	if strings.Contains(email.Body, "/register/complete") {
		t.Errorf("Expected email body not to contain completion URL, got: %s", email.Body)
	}
}

func TestAuthRoutes_Register_EmailUsesConfiguredOTPExpiration(t *testing.T) {
	ts, client, _, mockMailer := setupTestHTTPServerWithMailerConfig(t, config.EmailOTPConfig{
		TTL: config.Duration(30 * time.Minute),
	})

	body, _ := json.Marshal(map[string]string{"email": "custom-ttl@example.com"})
	resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	msg := mockMailer.LastMessage()
	if msg == nil {
		t.Fatal("Expected registration email to be sent")
	}
	if !strings.Contains(msg.Body, "This code will expire in 30 minutes.") {
		t.Fatalf("Expected email body to mention configured 30-minute expiration, got: %s", msg.Body)
	}
	if strings.Contains(msg.Body, "15 minutes") {
		t.Fatalf("Expected email body not to mention default expiration, got: %s", msg.Body)
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

func TestAuthRoutes_Register_SendFailureDoesNotConsumeThrottle(t *testing.T) {
	ts, client, _, mockMailer := setupTestHTTPServerWithMailer(t)
	mockMailer.SendError = errors.New("smtp unavailable")

	body, _ := json.Marshal(map[string]string{"email": "delivery-debug@example.com"})
	for i := 0; i < 10; i++ {
		resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to send register request %d: %v", i+1, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("failed send %d status = %d, want 500", i+1, resp.StatusCode)
		}
	}

	if msg := mockMailer.LastMessage(); msg != nil {
		t.Fatalf("failed sends should not capture email, got %#v", msg)
	}

	mockMailer.SendError = nil
	resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register request after SMTP recovery: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200 after SMTP recovery, got %d: %s", resp.StatusCode, string(respBody))
	}
	if msg := mockMailer.LastMessage(); msg == nil {
		t.Fatal("expected registration email after SMTP recovery")
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

func TestAuthRoutes_RegisterVerifyCode_Success(t *testing.T) {
	ts, client, _, mockMailer := setupTestHTTPServerWithMailer(t)

	body, _ := json.Marshal(map[string]string{"email": "codeuser@example.com"})
	resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register request: %v", err)
	}
	resp.Body.Close()

	msg := mockMailer.LastMessage()
	if msg == nil {
		t.Fatal("Expected registration email to be sent")
	}
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(msg.Body)
	if code == "" {
		t.Fatalf("Could not extract code from email body: %s", msg.Body)
	}

	verifyBody, _ := json.Marshal(map[string]string{"email": "codeuser@example.com", "code": code})
	verifyResp, err := client.Post(ts.URL+"/auth/register/verify-code", "application/json", bytes.NewReader(verifyBody))
	if err != nil {
		t.Fatalf("Failed to verify registration code: %v", err)
	}
	defer verifyResp.Body.Close()

	if verifyResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(verifyResp.Body)
		t.Fatalf("Expected status 200, got %d: %s", verifyResp.StatusCode, string(respBody))
	}
	var result map[string]string
	if err := json.NewDecoder(verifyResp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if result["completionToken"] == "" {
		t.Fatalf("Expected completionToken, got %#v", result)
	}
}

func TestAuthRoutes_RegisterVerifyCode_ExhaustedAttempts(t *testing.T) {
	ts, client, _, mockMailer := setupTestHTTPServerWithMailer(t)

	body, _ := json.Marshal(map[string]string{"email": "bruteforce@example.com"})
	resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register request: %v", err)
	}
	resp.Body.Close()

	msg := mockMailer.LastMessage()
	if msg == nil {
		t.Fatal("Expected registration email to be sent")
	}
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(msg.Body)
	if code == "" {
		t.Fatalf("Could not extract code from email body: %s", msg.Body)
	}
	wrongCode := "000000"
	if code == wrongCode {
		wrongCode = "111111"
	}

	verifyBody, _ := json.Marshal(map[string]string{"email": "bruteforce@example.com", "code": wrongCode})
	for i := 0; i < 5; i++ {
		verifyResp, err := client.Post(ts.URL+"/auth/register/verify-code", "application/json", bytes.NewReader(verifyBody))
		if err != nil {
			t.Fatalf("Failed to verify registration code attempt %d: %v", i+1, err)
		}
		verifyResp.Body.Close()
		if verifyResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("attempt %d status = %d, want 400", i+1, verifyResp.StatusCode)
		}
	}

	validBody, _ := json.Marshal(map[string]string{"email": "bruteforce@example.com", "code": code})
	validResp, err := client.Post(ts.URL+"/auth/register/verify-code", "application/json", bytes.NewReader(validBody))
	if err != nil {
		t.Fatalf("Failed to verify exhausted registration code: %v", err)
	}
	defer validResp.Body.Close()
	if validResp.StatusCode != http.StatusBadRequest {
		respBody, _ := io.ReadAll(validResp.Body)
		t.Fatalf("Expected exhausted valid code to return 400, got %d: %s", validResp.StatusCode, string(respBody))
	}
}

func TestAuthRoutes_RegisterComplete_Success(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	// Create a registration completion token
	token, err := chattoCore.CreateRegistrationToken(ctx, "complete@example.com")
	if err != nil {
		t.Fatalf("Failed to create registration completion token: %v", err)
	}

	// Complete registration
	reqBody := map[string]string{
		"token":                token,
		"login":                "newuser",
		"password":             "password123",
		"passwordConfirmation": "password123",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/register/complete", body)
	if err != nil {
		t.Fatalf("Failed to send browser register/complete request: %v", err)
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
	for _, field := range []string{"token", "refreshToken", "expiresIn", "refreshTokenExpiresIn"} {
		if _, exists := result[field]; exists {
			t.Fatalf("cookie-only registration response contains %q", field)
		}
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

	// Create registration completion token
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

	for i, login := range []string{"a", "alice."} {
		t.Run(login, func(t *testing.T) {
			token, err := chattoCore.CreateRegistrationToken(ctx, fmt.Sprintf("invalid-%d@example.com", i))
			if err != nil {
				t.Fatalf("CreateRegistrationToken: %v", err)
			}

			reqBody := map[string]string{
				"token":                token,
				"login":                login,
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
		})
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

	// Create a registration completion token for the same email
	// (simulating someone verifying a code before the email was claimed)
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

	_, nc := testutil.StartSharedNATS(t)

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
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)
	setTestServerName(t, ctx, chattoCore, "Engineering")

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

	loginBody, _ := json.Marshal(map[string]string{"login": "verifyuser", "password": "password123"})
	loginResp, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", loginBody)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed with status %d", loginResp.StatusCode)
	}

	requestBody, _ := json.Marshal(map[string]string{"email": "verify@example.com"})
	requestResp, err := client.Post(ts.URL+"/auth/verify-email/request-code", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("Failed to request verification code: %v", err)
	}
	requestResp.Body.Close()
	if requestResp.StatusCode != http.StatusOK {
		t.Fatalf("Verification code request failed with status %d", requestResp.StatusCode)
	}

	msg := mockMailer.LastMessage()
	if msg == nil {
		t.Fatal("Expected verification email to be sent")
	}
	if msg.Subject != "Verify your email for Engineering" {
		t.Errorf("Expected subject 'Verify your email for Engineering', got %s", msg.Subject)
	}
	if !strings.Contains(msg.Body, "add this email address to your Engineering account") {
		t.Errorf("Expected email body to mention Engineering account, got: %s", msg.Body)
	}
	if !strings.Contains(msg.Body, "15 minutes") {
		t.Errorf("Expected email body to mention 15-minute expiration, got: %s", msg.Body)
	}
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(msg.Body)
	if code == "" {
		t.Fatalf("Could not extract verification code from email body: %s", msg.Body)
	}

	verifyBody, _ := json.Marshal(map[string]string{"email": "verify@example.com", "code": code})
	verifyResp, err := client.Post(ts.URL+"/auth/verify-email/confirm-code", "application/json", bytes.NewReader(verifyBody))
	if err != nil {
		t.Fatalf("Failed to send verify request: %v", err)
	}
	defer verifyResp.Body.Close()

	if verifyResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(verifyResp.Body)
		t.Fatalf("Expected OK, got %d: %s", verifyResp.StatusCode, string(respBody))
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

func TestAuthRoutes_EmailVerification_EmailUsesConfiguredOTPExpiration(t *testing.T) {
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailerConfig(t, config.EmailOTPConfig{
		TTL: config.Duration(2 * time.Hour),
	})
	ctx := testContext(t)

	user, err := chattoCore.CreateUser(ctx, "system", "verify-custom-ttl", "Verify Custom TTL", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	loginBody, _ := json.Marshal(map[string]string{"login": user.Login, "password": "password123"})
	loginResp, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", loginBody)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed with status %d", loginResp.StatusCode)
	}

	requestBody, _ := json.Marshal(map[string]string{"email": "verify-custom-ttl@example.com"})
	requestResp, err := client.Post(ts.URL+"/auth/verify-email/request-code", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("Failed to request verification code: %v", err)
	}
	requestResp.Body.Close()
	if requestResp.StatusCode != http.StatusOK {
		t.Fatalf("Verification code request failed with status %d", requestResp.StatusCode)
	}

	msg := mockMailer.LastMessage()
	if msg == nil {
		t.Fatal("Expected verification email to be sent")
	}
	if !strings.Contains(msg.Body, "This code will expire in 2 hours.") {
		t.Fatalf("Expected email body to mention configured 2-hour expiration, got: %s", msg.Body)
	}
	if strings.Contains(msg.Body, "15 minutes") {
		t.Fatalf("Expected email body not to mention default expiration, got: %s", msg.Body)
	}
}

func TestAuthRoutes_EmailVerification_RequestCodeLimit(t *testing.T) {
	ts, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	user, err := chattoCore.CreateUser(ctx, "system", "verify-limit-user", "Verify Limit User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	loginBody, _ := json.Marshal(map[string]string{"login": "verify-limit-user", "password": "password123"})
	loginResp, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", loginBody)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed with status %d", loginResp.StatusCode)
	}

	body, _ := json.Marshal(map[string]string{"email": "limit@example.com"})
	for i := 0; i < 10; i++ {
		resp, err := client.Post(ts.URL+"/auth/verify-email/request-code", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to request verification code %d: %v", i+1, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i+1, resp.StatusCode)
		}
	}

	resp, err := client.Post(ts.URL+"/auth/verify-email/request-code", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to request limited verification code: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 429, got %d: %s", resp.StatusCode, string(respBody))
	}

	if verified, err := chattoCore.HasVerifiedEmail(ctx, user.Id); err != nil {
		t.Fatalf("HasVerifiedEmail: %v", err)
	} else if verified {
		t.Fatal("request-code limit should not verify email")
	}
}

func TestAuthRoutes_EmailVerification_SendFailureDoesNotConsumeThrottle(t *testing.T) {
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)

	user, err := chattoCore.CreateUser(ctx, "system", "verify-delivery-debug", "Verify Delivery Debug", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	loginBody, _ := json.Marshal(map[string]string{"login": user.Login, "password": "password123"})
	loginResp, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", loginBody)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed with status %d", loginResp.StatusCode)
	}

	mockMailer.SendError = errors.New("smtp unavailable")
	body, _ := json.Marshal(map[string]string{"email": "verify-delivery-debug@example.com"})
	for i := 0; i < 10; i++ {
		resp, err := client.Post(ts.URL+"/auth/verify-email/request-code", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to request verification code %d: %v", i+1, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("failed send %d status = %d, want 500", i+1, resp.StatusCode)
		}
	}

	if msg := mockMailer.LastMessage(); msg != nil {
		t.Fatalf("failed sends should not capture email, got %#v", msg)
	}

	mockMailer.SendError = nil
	resp, err := client.Post(ts.URL+"/auth/verify-email/request-code", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to request verification code after SMTP recovery: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200 after SMTP recovery, got %d: %s", resp.StatusCode, string(respBody))
	}
	if msg := mockMailer.LastMessage(); msg == nil {
		t.Fatal("expected verification email after SMTP recovery")
	}
}

func TestAuthRoutes_EmailVerification_DuplicateEmail(t *testing.T) {
	ts, client, chattoCore := setupTestHTTPServer(t)
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

	loginBody, _ := json.Marshal(map[string]string{"login": "user2", "password": "password123"})
	loginResp, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", loginBody)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed with status %d", loginResp.StatusCode)
	}

	code, err := chattoCore.CreateEmailVerificationCode(ctx, user2.Id, "shared@example.com")
	if err != nil {
		t.Fatalf("Failed to create verification code: %v", err)
	}

	verifyBody, _ := json.Marshal(map[string]string{"email": "shared@example.com", "code": code})
	verifyResp, err := client.Post(ts.URL+"/auth/verify-email/confirm-code", "application/json", bytes.NewReader(verifyBody))
	if err != nil {
		t.Fatalf("Failed to send verify request: %v", err)
	}
	defer verifyResp.Body.Close()

	if verifyResp.StatusCode != http.StatusConflict {
		respBody, _ := io.ReadAll(verifyResp.Body)
		t.Errorf("Expected status 409, got %d: %s", verifyResp.StatusCode, string(respBody))
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

func TestAuthRoutes_Register_EmailContainsValidCode(t *testing.T) {
	ts, client, _, mockMailer := setupTestHTTPServerWithMailer(t)
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

	code := regexp.MustCompile(`\b\d{6}\b`).FindString(msg.Body)
	if code == "" {
		t.Fatalf("Could not extract code from email body: %s", msg.Body)
	}

	verifyBody, _ := json.Marshal(map[string]string{"email": "tokentest@example.com", "code": code})
	verifyResp, err := client.Post(ts.URL+"/auth/register/verify-code", "application/json", bytes.NewReader(verifyBody))
	if err != nil {
		t.Fatalf("Failed to verify registration code: %v", err)
	}
	defer verifyResp.Body.Close()
	if verifyResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(verifyResp.Body)
		t.Fatalf("Expected status 200, got %d: %s", verifyResp.StatusCode, string(respBody))
	}

	// Verify email content
	if !strings.Contains(msg.Body, "Welcome to Chatto!") {
		t.Error("Expected welcome message in email body")
	}
	if !strings.Contains(msg.Body, "15 minutes") {
		t.Error("Expected 15-minute expiration mention in email body")
	}
	if strings.Contains(msg.Body, "/register/complete") {
		t.Error("Expected no completion URL in email body")
	}
}

// ============================================================================
// Password Reset Tests
// ============================================================================

func TestAuthRoutes_ForgotPassword_SendsEmail(t *testing.T) {
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)
	setTestServerName(t, ctx, chattoCore, "Engineering")

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
	if email.Subject != "Reset your Engineering password" {
		t.Errorf("Expected subject 'Reset your Engineering password', got %s", email.Subject)
	}
	if !strings.Contains(email.Body, "reset the password for your Engineering account") {
		t.Errorf("Expected email body to mention Engineering account, got: %s", email.Body)
	}
	if !strings.Contains(email.Body, "/reset-password?token=PR") {
		t.Errorf("Expected email body to contain reset link with PR token, got: %s", email.Body)
	}
	if !strings.Contains(email.Body, "1 hour") {
		t.Errorf("Expected email body to mention 1-hour expiration")
	}
	if strings.Contains(email.Body, "The Chatto Team") {
		t.Errorf("Expected password reset email not to use generic Chatto signoff, got: %s", email.Body)
	}
}

func TestAuthRoutes_ForgotPassword_ThrottlesRepeatedDelivery(t *testing.T) {
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, core.SystemActorID, "forgot-throttle", "Forgot Throttle", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := chattoCore.AddVerifiedEmailDirect(ctx, user.Id, "forgot-throttle@example.com"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"email": "forgot-throttle@example.com"})
	for i := 0; i < 2; i++ {
		resp, err := client.Post(ts.URL+"/auth/forgot-password", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("forgot-password request %d: %v", i+1, err)
		}
		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			t.Fatalf("decode forgot-password response %d: %v", i+1, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || !strings.Contains(result["message"], "If that email is registered") {
			t.Fatalf("request %d status/message = %d/%q, want generic success", i+1, resp.StatusCode, result["message"])
		}
	}
	if messages := mockMailer.Messages(); len(messages) != 1 {
		t.Fatalf("delivered password-reset emails = %d, want 1", len(messages))
	}
}

func TestAuthRoutes_ForgotPassword_SendFailureDoesNotConsumeThrottle(t *testing.T) {
	ts, client, chattoCore, mockMailer := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, core.SystemActorID, "forgot-send-failure", "Forgot Send Failure", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := chattoCore.AddVerifiedEmailDirect(ctx, user.Id, "forgot-send-failure@example.com"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"email": "forgot-send-failure@example.com"})
	mockMailer.SendError = errors.New("smtp unavailable")
	resp, err := client.Post(ts.URL+"/auth/forgot-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed forgot-password delivery: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("failed delivery status = %d, want generic 200", resp.StatusCode)
	}

	mockMailer.SendError = nil
	resp, err = client.Post(ts.URL+"/auth/forgot-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("forgot-password after SMTP recovery: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recovered delivery status = %d, want 200", resp.StatusCode)
	}
	if messages := mockMailer.Messages(); len(messages) != 1 {
		t.Fatalf("delivered password-reset emails after recovery = %d, want 1", len(messages))
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
	if refreshToken, _ := result["refreshToken"].(string); !strings.HasPrefix(refreshToken, "cht_RT_") {
		t.Fatalf("register refresh token = %q", refreshToken)
	}
	if result["expiresIn"].(float64) <= 0 || result["refreshTokenExpiresIn"].(float64) <= 0 {
		t.Fatalf("register lifetimes = %v/%v", result["expiresIn"], result["refreshTokenExpiresIn"])
	}
	for _, cookie := range resp.Cookies() {
		if isBrowserSessionCookieName(cookie.Name) || cookie.Name == csrfCookieName {
			t.Fatalf("programmatic login set browser cookie %q", cookie.Name)
		}
	}
}

func TestAuthRoutes_LoginStaleCredentialErrorIsInvalidCredentials(t *testing.T) {
	if !isStaleLoginCredentialError(core.ErrCookieSessionNotFound) {
		t.Fatal("stale cookie-session creation should be treated as invalid credentials")
	}
	if !isStaleLoginCredentialError(core.ErrAuthTokenNotFound) {
		t.Fatal("stale bearer-token creation should be treated as invalid credentials")
	}
	if isStaleLoginCredentialError(errors.New("other error")) {
		t.Fatal("unrelated credential creation errors should not be treated as invalid credentials")
	}
}

func TestAuthRoutes_LoginStaleBearerTokenIssuanceIsInvalidCredentials(t *testing.T) {
	var capture struct {
		sync.Mutex
		userID string
		err    error
	}

	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.passwordLoginSessionCreatedHook = func(c *gin.Context, userID string, _ uint64) {
			capture.Lock()
			defer capture.Unlock()
			capture.userID = userID
			capture.err = s.core.SetPasswordHash(c.Request.Context(), userID, "newpassword456")
		}
	})
	ctx := testContext(t)

	if _, err := chattoCore.CreateUser(ctx, "", "staleissuer", "Stale Issuer", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	loginBody := map[string]string{"login": "staleissuer", "password": "password123"}
	body, _ := json.Marshal(loginBody)
	resp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}
	defer resp.Body.Close()

	capture.Lock()
	hookErr := capture.err
	capturedUserID := capture.userID
	capture.Unlock()

	if hookErr != nil {
		t.Fatalf("password-login hook failed: %v", hookErr)
	}
	if capturedUserID == "" {
		t.Fatal("password-login hook did not run")
	}

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Login status = %d, want 401", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Decode login response: %v", err)
	}
	if result["error"] != "Invalid credentials" {
		t.Fatalf("Login error = %v, want Invalid credentials", result["error"])
	}
	if _, ok := result["token"]; ok {
		t.Fatal("Stale login response should not include a bearer token")
	}

	if deleted, err := chattoCore.RevokeCookieSessionsForUser(ctx, capturedUserID); err != nil || deleted != 0 {
		t.Fatalf("programmatic stale login cookie sessions = %d, %v; want 0", deleted, err)
	}
}

func TestAuthRoutes_BrowserLoginRechecksAuthGeneration(t *testing.T) {
	var capturedSessionID string
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.passwordLoginSessionCreatedHook = func(c *gin.Context, userID string, _ uint64) {
			capturedSessionID, _ = s.browserSessionID(c)
			if err := s.core.SetPasswordHash(c.Request.Context(), userID, "newpassword456"); err != nil {
				t.Errorf("SetPasswordHash: %v", err)
			}
		}
	})
	ctx := testContext(t)
	if _, err := chattoCore.CreateUser(ctx, "", "stale-cookie-only", "Stale Cookie Only", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"login": "stale-cookie-only", "password": "password123"})
	resp, err := postBrowserAuthentication(client, ts.URL+"/auth/browser/login", body)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		responseBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("login status = %d, want 401: %s", resp.StatusCode, responseBody)
	}
	if capturedSessionID == "" {
		t.Fatal("password-login hook did not capture a cookie session")
	}
	if _, err := chattoCore.ValidateCookieCredential(ctx, capturedSessionID); !errors.Is(err, core.ErrCookieSessionNotFound) {
		t.Fatalf("ValidateCookieCredential err = %v, want ErrCookieSessionNotFound", err)
	}
}

func TestAuthRoutes_LoginBearerTokenFailureDoesNotCreateCookieSession(t *testing.T) {
	var capture struct {
		sync.Mutex
		userID string
	}

	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.passwordLoginSessionCreatedHook = func(c *gin.Context, userID string, _ uint64) {
			capture.Lock()
			defer capture.Unlock()
			capture.userID = userID
			s.core.EventPublisher = nil
		}
	})
	ctx := testContext(t)

	if _, err := chattoCore.CreateUser(ctx, "", "tokenfailure", "Token Failure", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	loginBody := map[string]string{"login": "tokenfailure", "password": "password123"}
	body, _ := json.Marshal(loginBody)
	resp, err := client.Post(ts.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}
	defer resp.Body.Close()

	capture.Lock()
	capturedUserID := capture.userID
	capture.Unlock()

	if capturedUserID == "" {
		t.Fatal("password-login hook did not run")
	}

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("Login status = %d, want 500", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Decode login response: %v", err)
	}
	if result["error"] != "Failed to create session" {
		t.Fatalf("Login error = %v, want Failed to create session", result["error"])
	}
	if _, ok := result["token"]; ok {
		t.Fatal("Failed login response should not include a bearer token")
	}

	if deleted, err := chattoCore.RevokeCookieSessionsForUser(ctx, capturedUserID); err != nil || deleted != 0 {
		t.Fatalf("programmatic failed login cookie sessions = %d, %v; want 0", deleted, err)
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

	// Create a registration completion token directly
	regToken, err := chattoCore.CreateRegistrationToken(ctx, "newuser@example.com")
	if err != nil {
		t.Fatalf("Failed to create registration completion token: %v", err)
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
	for _, cookie := range resp.Cookies() {
		if isBrowserSessionCookieName(cookie.Name) || cookie.Name == csrfCookieName {
			t.Fatalf("programmatic registration set browser cookie %q", cookie.Name)
		}
	}
}
