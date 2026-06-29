package http_server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
)

// setupCORSServer creates a minimal HTTPServer with CORS middleware and a test handler.
func setupCORSServer(t *testing.T, webserverConfig config.WebserverConfig) *HTTPServer {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	s := &HTTPServer{
		config: config.ChattoConfig{
			Webserver: webserverConfig,
		},
		router: router,
	}

	allowedOrigins := s.buildAllowedOrigins()
	router.Use(s.corsMiddleware(allowedOrigins))

	// Add test handlers
	router.GET("/api/connect/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	router.POST("/api/connect/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	router.POST(serverDiscoveryConnectPath, func(c *gin.Context) {
		c.String(http.StatusOK, "instance info")
	})

	return s
}

func TestCORSMiddleware(t *testing.T) {
	t.Run("no Origin header sets no CORS response headers", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL:            "https://chat.example.com",
			AllowedOrigins: []string{"https://other.example.com"},
		})

		req := httptest.NewRequest("GET", "/api/connect/test", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "" {
			t.Errorf("expected no Access-Control-Allow-Origin, got %q", origin)
		}
	})

	t.Run("allowed origin from config URL gets CORS headers", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL: "https://chat.example.com",
		})

		req := httptest.NewRequest("GET", "/api/connect/test", nil)
		req.Header.Set("Origin", "https://chat.example.com")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "https://chat.example.com" {
			t.Errorf("expected Access-Control-Allow-Origin 'https://chat.example.com', got %q", origin)
		}
		if creds := w.Header().Get("Access-Control-Allow-Credentials"); creds != "true" {
			t.Errorf("expected Access-Control-Allow-Credentials 'true', got %q", creds)
		}
		if vary := w.Header().Get("Vary"); vary != "Origin" {
			t.Errorf("expected Vary 'Origin', got %q", vary)
		}
	})

	t.Run("allowed origin from allowed_origins list gets CORS headers", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL:            "https://chat.example.com",
			AllowedOrigins: []string{"https://app.example.com"},
		})

		req := httptest.NewRequest("GET", "/api/connect/test", nil)
		req.Header.Set("Origin", "https://app.example.com")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "https://app.example.com" {
			t.Errorf("expected Access-Control-Allow-Origin 'https://app.example.com', got %q", origin)
		}
	})

	t.Run("disallowed origin gets no CORS headers when allowed_origins is explicit", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL:            "https://chat.example.com",
			AllowedOrigins: []string{"https://only-this.example.com"},
		})

		req := httptest.NewRequest("GET", "/api/connect/test", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "" {
			t.Errorf("expected no Access-Control-Allow-Origin, got %q", origin)
		}
	})

	t.Run("OPTIONS preflight with allowed origin returns 204 with CORS headers", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL: "https://chat.example.com",
		})

		req := httptest.NewRequest("OPTIONS", "/api/connect/test", nil)
		req.Header.Set("Origin", "https://chat.example.com")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "https://chat.example.com" {
			t.Errorf("expected Access-Control-Allow-Origin 'https://chat.example.com', got %q", origin)
		}
		if methods := w.Header().Get("Access-Control-Allow-Methods"); methods != "GET, POST, OPTIONS" {
			t.Errorf("expected Access-Control-Allow-Methods 'GET, POST, OPTIONS', got %q", methods)
		}
		if headers := w.Header().Get("Access-Control-Allow-Headers"); headers != corsAllowedHeaders {
			t.Errorf("expected Access-Control-Allow-Headers %q, got %q", corsAllowedHeaders, headers)
		}
		if maxAge := w.Header().Get("Access-Control-Max-Age"); maxAge != "86400" {
			t.Errorf("expected Access-Control-Max-Age '86400', got %q", maxAge)
		}
	})

	t.Run("ConnectRPC bearer preflight from remote origin allows authorization headers without credentials", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL: "https://chat.example.com",
			// AllowedOrigins not set — remote bearer-token clients match the wildcard.
		})

		req := httptest.NewRequest("OPTIONS", connectAPIPrefix+"/chatto.api.v1.NotificationPreferencesModel/SetRoomNotificationLevel", nil)
		req.Header.Set("Origin", "https://integration.example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "authorization, content-type, connect-protocol-version, connect-timeout-ms")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Errorf("expected Access-Control-Allow-Origin '*', got %q", origin)
		}
		if creds := w.Header().Get("Access-Control-Allow-Credentials"); creds != "" {
			t.Errorf("expected no Access-Control-Allow-Credentials for wildcard bearer-token clients, got %q", creds)
		}
		if methods := w.Header().Get("Access-Control-Allow-Methods"); methods != "GET, POST, OPTIONS" {
			t.Errorf("expected Access-Control-Allow-Methods 'GET, POST, OPTIONS', got %q", methods)
		}
		headers := w.Header().Get("Access-Control-Allow-Headers")
		for _, required := range []string{"Authorization", "Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms"} {
			if !strings.Contains(headers, required) {
				t.Errorf("expected Access-Control-Allow-Headers to include %q, got %q", required, headers)
			}
		}
	})

	t.Run("OPTIONS preflight with disallowed origin returns 204 without CORS headers", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL:            "https://chat.example.com",
			AllowedOrigins: []string{"https://only-this.example.com"},
		})

		req := httptest.NewRequest("OPTIONS", "/api/connect/test", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "" {
			t.Errorf("expected no Access-Control-Allow-Origin, got %q", origin)
		}
	})

	t.Run("wildcard in allowed_origins uses literal * without credentials", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL:            "https://chat.example.com",
			AllowedOrigins: []string{"*"},
		})

		req := httptest.NewRequest("GET", "/api/connect/test", nil)
		req.Header.Set("Origin", "https://anywhere.example.com")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Errorf("expected Access-Control-Allow-Origin '*', got %q", origin)
		}
		if creds := w.Header().Get("Access-Control-Allow-Credentials"); creds != "" {
			t.Errorf("expected no Access-Control-Allow-Credentials for wildcard, got %q", creds)
		}
		if vary := w.Header().Get("Vary"); vary != "" {
			t.Errorf("expected no Vary header for wildcard, got %q", vary)
		}
	})

	t.Run("origin matching is case-insensitive", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL: "https://chat.example.com",
		})

		req := httptest.NewRequest("GET", "/api/connect/test", nil)
		req.Header.Set("Origin", "HTTPS://CHAT.EXAMPLE.COM")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "HTTPS://CHAT.EXAMPLE.COM" {
			t.Errorf("expected Access-Control-Allow-Origin to echo the request origin, got %q", origin)
		}
	})

	t.Run("public server discovery connect path uses wildcard CORS even with explicit allowed origins", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL:            "https://chat.example.com",
			AllowedOrigins: []string{"https://only-this.example.com"},
		})

		req := httptest.NewRequest("POST", serverDiscoveryConnectPath, nil)
		req.Header.Set("Origin", "https://unknown.example.com")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Errorf("expected Access-Control-Allow-Origin '*' from discovery handler, got %q", origin)
		}
		if methods := w.Header().Get("Access-Control-Allow-Methods"); methods != "POST, OPTIONS" {
			t.Errorf("expected Access-Control-Allow-Methods 'POST, OPTIONS', got %q", methods)
		}
		if creds := w.Header().Get("Access-Control-Allow-Credentials"); creds != "" {
			t.Errorf("expected no Access-Control-Allow-Credentials for public discovery, got %q", creds)
		}
	})

	t.Run("public server discovery connect preflight allows JSON Connect headers", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL:            "https://chat.example.com",
			AllowedOrigins: []string{"https://only-this.example.com"},
		})

		req := httptest.NewRequest("OPTIONS", serverDiscoveryConnectPath, nil)
		req.Header.Set("Origin", "https://unknown.example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "content-type, connect-protocol-version")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Errorf("expected Access-Control-Allow-Origin '*' from discovery preflight, got %q", origin)
		}
		headers := w.Header().Get("Access-Control-Allow-Headers")
		for _, required := range []string{"Content-Type", "Connect-Protocol-Version"} {
			if !strings.Contains(headers, required) {
				t.Errorf("expected Access-Control-Allow-Headers to include %q, got %q", required, headers)
			}
		}
	})

	t.Run("empty allowed_origins defaults to wildcard for unknown origins", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL: "https://chat.example.com",
			// AllowedOrigins not set — should default to wildcard
		})

		req := httptest.NewRequest("GET", "/api/connect/test", nil)
		req.Header.Set("Origin", "https://remote-instance.example.com")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Errorf("expected Access-Control-Allow-Origin '*' for default wildcard, got %q", origin)
		}
		if creds := w.Header().Get("Access-Control-Allow-Credentials"); creds != "" {
			t.Errorf("expected no Access-Control-Allow-Credentials for wildcard, got %q", creds)
		}
	})

	t.Run("home origin gets explicit match even with wildcard default", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL: "https://chat.example.com",
			// AllowedOrigins not set — wildcard default active
		})

		req := httptest.NewRequest("GET", "/api/connect/test", nil)
		req.Header.Set("Origin", "https://chat.example.com")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		// Home origin should match Tier 1 (explicit) before the wildcard in Tier 3
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "https://chat.example.com" {
			t.Errorf("expected Access-Control-Allow-Origin 'https://chat.example.com', got %q", origin)
		}
		if creds := w.Header().Get("Access-Control-Allow-Credentials"); creds != "true" {
			t.Errorf("expected Access-Control-Allow-Credentials 'true', got %q", creds)
		}
	})

	t.Run("localhost at listen port is auto-allowed", func(t *testing.T) {
		s := setupCORSServer(t, config.WebserverConfig{
			URL:  "https://chat.example.com",
			Port: 4000,
		})

		req := httptest.NewRequest("GET", "/api/connect/test", nil)
		req.Header.Set("Origin", "http://localhost:4000")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "http://localhost:4000" {
			t.Errorf("expected Access-Control-Allow-Origin 'http://localhost:4000', got %q", origin)
		}
	})
}

func TestBuildAllowedOrigins(t *testing.T) {
	t.Run("includes origin from config URL", func(t *testing.T) {
		s := &HTTPServer{
			config: config.ChattoConfig{
				Webserver: config.WebserverConfig{
					URL:  "https://chat.example.com/some/path",
					Port: 443,
				},
			},
		}

		origins := s.buildAllowedOrigins()

		found := false
		for _, o := range origins {
			if o == "https://chat.example.com" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'https://chat.example.com' in origins, got %v", origins)
		}
	})

	t.Run("includes localhost at listen port", func(t *testing.T) {
		s := &HTTPServer{
			config: config.ChattoConfig{
				Webserver: config.WebserverConfig{
					URL:  "https://chat.example.com",
					Port: 4000,
				},
			},
		}

		origins := s.buildAllowedOrigins()

		found := false
		for _, o := range origins {
			if o == "http://localhost:4000" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'http://localhost:4000' in origins, got %v", origins)
		}
	})

	t.Run("defaults to wildcard when allowed_origins is empty", func(t *testing.T) {
		s := &HTTPServer{
			config: config.ChattoConfig{
				Webserver: config.WebserverConfig{
					URL: "https://chat.example.com",
				},
			},
		}

		origins := s.buildAllowedOrigins()

		found := false
		for _, o := range origins {
			if o == "*" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected '*' in origins when allowed_origins is empty, got %v", origins)
		}
	})

	t.Run("does not add wildcard when allowed_origins is explicit", func(t *testing.T) {
		s := &HTTPServer{
			config: config.ChattoConfig{
				Webserver: config.WebserverConfig{
					URL:            "https://chat.example.com",
					AllowedOrigins: []string{"https://app.example.com"},
				},
			},
		}

		origins := s.buildAllowedOrigins()

		for _, o := range origins {
			if o == "*" {
				t.Errorf("expected no '*' in origins when allowed_origins is explicit, got %v", origins)
				break
			}
		}
	})

	t.Run("includes explicit allowed_origins", func(t *testing.T) {
		s := &HTTPServer{
			config: config.ChattoConfig{
				Webserver: config.WebserverConfig{
					URL:            "https://chat.example.com",
					AllowedOrigins: []string{"https://app.example.com", "https://dev.example.com"},
				},
			},
		}

		origins := s.buildAllowedOrigins()

		for _, expected := range []string{"https://app.example.com", "https://dev.example.com"} {
			found := false
			for _, o := range origins {
				if o == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %q in origins, got %v", expected, origins)
			}
		}
	})
}

func TestMatchOrigin(t *testing.T) {
	s := &HTTPServer{}

	t.Run("matches exact origin", func(t *testing.T) {
		if s.matchOrigin("https://chat.example.com", []string{"https://chat.example.com"}) != originExplicit {
			t.Error("expected originExplicit match")
		}
	})

	t.Run("rejects non-matching origin", func(t *testing.T) {
		if s.matchOrigin("https://evil.com", []string{"https://chat.example.com"}) != originNotAllowed {
			t.Error("expected originNotAllowed")
		}
	})

	t.Run("wildcard returns originWildcard", func(t *testing.T) {
		if s.matchOrigin("https://anything.com", []string{"*"}) != originWildcard {
			t.Error("expected originWildcard match")
		}
	})

	t.Run("case-insensitive matching", func(t *testing.T) {
		if s.matchOrigin("HTTPS://CHAT.EXAMPLE.COM", []string{"https://chat.example.com"}) != originExplicit {
			t.Error("expected originExplicit match")
		}
	})

	t.Run("empty list rejects all origins", func(t *testing.T) {
		if s.matchOrigin("https://chat.example.com", nil) != originNotAllowed {
			t.Error("expected originNotAllowed")
		}
	})
}
