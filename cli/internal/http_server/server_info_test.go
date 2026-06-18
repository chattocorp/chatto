package http_server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/testutil"
)

// bannerImageBytes returns an in-memory PNG suitable as a banner upload.
// Banners double as OG link-preview images at 1200x630.
func bannerImageBytes(t *testing.T) io.Reader {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1200, 630))
	for y := 0; y < 630; y++ {
		for x := 0; x < 1200; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

// setupServerInfoServer creates a minimal HTTPServer for instance info endpoint tests.
func setupServerInfoServer(t *testing.T, authConfig config.AuthConfig) *HTTPServer {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartSharedNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	chattoCore, err := core.NewChattoCore(ctx, nc, config.CoreConfig{})
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

	router := gin.New()
	s := &HTTPServer{
		config: config.ChattoConfig{
			Auth: authConfig,
		},
		nc:      nc,
		router:  router,
		core:    chattoCore,
		version: "1.2.3",
	}
	s.setupServerInfoRoutes()

	return s
}

func TestServerInfo(t *testing.T) {
	t.Run("returns correct JSON structure with defaults", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp.Name != "Chatto" {
			t.Errorf("expected name 'Chatto', got %q", resp.Name)
		}
		if resp.Version != "1.2.3" {
			t.Errorf("expected version '1.2.3', got %q", resp.Version)
		}
		if !resp.RegistrationOpen {
			t.Error("expected registrationOpen true by default")
		}
	})

	t.Run("includes password in authMethods when direct registration enabled", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if len(resp.AuthMethods) != 1 || resp.AuthMethods[0] != "password" {
			t.Errorf("expected authMethods [password], got %v", resp.AuthMethods)
		}
	})

	t.Run("includes configured auth provider metadata", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{
			Providers: []config.AuthProviderConfig{
				{ID: "hub", Type: config.AuthProviderTypeOpenIDConnect, Label: "Chatto Hub"},
				{ID: "github-main", Type: config.AuthProviderTypeGitHub},
			},
		})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if got, want := resp.AuthMethods, []string{"password", "oidc", "github"}; strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("authMethods = %v, want %v", got, want)
		}
		if len(resp.AuthProviders) != 2 {
			t.Fatalf("authProviders len = %d, want 2", len(resp.AuthProviders))
		}
		if resp.AuthProviders[0].ID != "hub" || resp.AuthProviders[0].Type != config.AuthProviderTypeOpenIDConnect || resp.AuthProviders[0].Label != "Chatto Hub" || resp.AuthProviders[0].LoginURL != "/auth/providers/hub" {
			t.Fatalf("authProviders[0] = %+v", resp.AuthProviders[0])
		}
		if resp.AuthProviders[1].ID != "github-main" || resp.AuthProviders[1].Type != config.AuthProviderTypeGitHub || resp.AuthProviders[1].Label != "GitHub" || resp.AuthProviders[1].LoginURL != "/auth/providers/github-main" {
			t.Fatalf("authProviders[1] = %+v", resp.AuthProviders[1])
		}
	})

	t.Run("registration disabled hides password and sets registrationOpen false", func(t *testing.T) {
		disabled := false
		s := setupServerInfoServer(t, config.AuthConfig{
			DirectRegistration: &disabled,
		})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp.RegistrationOpen {
			t.Error("expected registrationOpen false")
		}
		// authMethods should be empty (no password, no SSO)
		if len(resp.AuthMethods) != 0 {
			t.Errorf("expected empty authMethods, got %v", resp.AuthMethods)
		}
	})

	t.Run("returns empty array not null for authMethods", func(t *testing.T) {
		disabled := false
		s := setupServerInfoServer(t, config.AuthConfig{
			DirectRegistration: &disabled,
		})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		// Parse raw JSON to check for null vs empty array
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if string(raw["authMethods"]) == "null" {
			t.Error("authMethods should be [] not null")
		}
	})

	t.Run("includes authorizeUrl for OAuth discovery", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp.AuthorizeURL != "/oauth/authorize" {
			t.Errorf("expected authorizeUrl '/oauth/authorize', got %q", resp.AuthorizeURL)
		}
	})

	t.Run("sets CORS headers", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Errorf("expected Access-Control-Allow-Origin *, got %q", origin)
		}
		if methods := w.Header().Get("Access-Control-Allow-Methods"); methods != "GET, OPTIONS" {
			t.Errorf("expected Access-Control-Allow-Methods 'GET, OPTIONS', got %q", methods)
		}
	})

	t.Run("sets Cache-Control header", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=300" {
			t.Errorf("expected Cache-Control 'public, max-age=300', got %q", cc)
		}
	})

	t.Run("absolutizes bannerUrl using request scheme/host when a banner is set", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		// Configure a banner on the instance (simulates an admin upload).
		// The Core helper returns a relative URL when AssetBaseURL is empty
		// (the case in this test), so we exercise the http_server's
		// absolutize path.
		ctx := testContext(t)
		asset, err := s.core.UploadServerBanner(ctx, bannerImageBytes(t))
		if err != nil {
			t.Fatalf("upload banner: %v", err)
		}
		if err := s.core.SetServerBanner(ctx, "test-admin", asset); err != nil {
			t.Fatalf("set banner: %v", err)
		}

		// Request via plain http.
		req := httptest.NewRequest("GET", "/api/server", nil)
		req.Host = "remote.example.com"
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if !strings.HasPrefix(resp.BannerURL, "http://remote.example.com/") {
			t.Errorf("expected absolute http://remote.example.com URL, got %q", resp.BannerURL)
		}
	})

	t.Run("absolutizes bannerUrl as https when X-Forwarded-Proto is https", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		ctx := testContext(t)
		asset, err := s.core.UploadServerBanner(ctx, bannerImageBytes(t))
		if err != nil {
			t.Fatalf("upload banner: %v", err)
		}
		if err := s.core.SetServerBanner(ctx, "test-admin", asset); err != nil {
			t.Fatalf("set banner: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/server", nil)
		req.Host = "remote.example.com"
		req.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if !strings.HasPrefix(resp.BannerURL, "https://remote.example.com/") {
			t.Errorf("expected absolute https://remote.example.com URL, got %q", resp.BannerURL)
		}
	})

	t.Run("preserves already-absolute bannerUrl from AssetBaseURL", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})
		// Mirror what cmd/run.go does when [webserver] url is configured.
		s.core.AssetBaseURL = "https://chat.example.com"

		ctx := testContext(t)
		asset, err := s.core.UploadServerBanner(ctx, bannerImageBytes(t))
		if err != nil {
			t.Fatalf("upload banner: %v", err)
		}
		if err := s.core.SetServerBanner(ctx, "test-admin", asset); err != nil {
			t.Fatalf("set banner: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/server", nil)
		req.Host = "remote.example.com" // different from AssetBaseURL
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if !strings.HasPrefix(resp.BannerURL, "https://chat.example.com/") {
			t.Errorf("expected absolute URL to keep AssetBaseURL host, got %q", resp.BannerURL)
		}
	})

	t.Run("omits bannerUrl when no banner is set", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		// Inspect the raw JSON: the JSON tag is `omitempty`, so when no
		// banner is configured the field must not appear at all (rather
		// than serialize as `"bannerUrl": ""`).
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if _, present := raw["bannerUrl"]; present {
			t.Errorf("expected bannerUrl absent when no banner set, got %s", string(raw["bannerUrl"]))
		}
	})

	t.Run("OPTIONS preflight returns 204 with CORS headers", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("OPTIONS", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Errorf("expected Access-Control-Allow-Origin *, got %q", origin)
		}
		if maxAge := w.Header().Get("Access-Control-Max-Age"); maxAge != "86400" {
			t.Errorf("expected Access-Control-Max-Age '86400', got %q", maxAge)
		}
	})
}
