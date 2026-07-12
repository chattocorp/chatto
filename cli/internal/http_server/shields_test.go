package http_server

import (
	"context"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
)

func TestShieldsDisabledReturnsNotFound(t *testing.T) {
	server := setupShieldTestServer(t, false)

	w := performShieldRequest(server.router, "GET", "/shields/online.png", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled shield status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestShieldsUnknownMetricReturnsNotFound(t *testing.T) {
	server := setupShieldTestServer(t, true)

	w := performShieldRequest(server.router, "GET", "/shields/unknown.png", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown shield status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestShieldsServeOnlineAndRegisteredPNGs(t *testing.T) {
	server := setupShieldTestServer(t, true)
	ctx := testShieldContext(t)

	if err := server.core.SetPresence(ctx, "U-online-shield", core.PresenceStatusOnline); err != nil {
		t.Fatalf("SetPresence online: %v", err)
	}
	if err := server.core.SetPresence(ctx, "U-away-shield", core.PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence away: %v", err)
	}
	if err := server.core.SetPresence(ctx, "U-dnd-shield", core.PresenceStatusDoNotDisturb); err != nil {
		t.Fatalf("SetPresence dnd: %v", err)
	}
	waitForShieldLivePresenceCount(t, ctx, server.core, 3)

	verified, err := server.core.CreateUser(ctx, core.SystemActorID, "shieldverified", "Shield Verified", "password123")
	if err != nil {
		t.Fatalf("CreateUser verified: %v", err)
	}
	if err := server.core.AddVerifiedEmailDirect(ctx, verified.Id, "shieldverified@example.test"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect: %v", err)
	}
	if _, err := server.core.CreateUser(ctx, core.SystemActorID, "shieldunverified", "Shield Unverified", "password123"); err != nil {
		t.Fatalf("CreateUser unverified: %v", err)
	}

	tests := []struct {
		path string
		etag string
	}{
		{path: "/shields/online.png", etag: `"chatto-shield-online-3"`},
		{path: "/shields/registered.png", etag: `"chatto-shield-registered-1"`},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := performShieldRequest(server.router, "GET", tt.path, "")
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
			}
			if got := w.Header().Get("Content-Type"); got != "image/png" {
				t.Fatalf("Content-Type = %q, want image/png", got)
			}
			if got := w.Header().Get("Cache-Control"); got != shieldCacheControl {
				t.Fatalf("Cache-Control = %q, want %q", got, shieldCacheControl)
			}
			if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
			}
			if got := w.Header().Get("ETag"); got != tt.etag {
				t.Fatalf("ETag = %q, want %q", got, tt.etag)
			}
			if _, err := png.Decode(w.Body); err != nil {
				t.Fatalf("response is not a PNG: %v", err)
			}
		})
	}
}

func TestShieldsETagConditionalRequest(t *testing.T) {
	server := setupShieldTestServer(t, true)

	w := performShieldRequest(server.router, "GET", "/shields/registered.png", `"chatto-shield-registered-0"`)
	if w.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d, want %d", w.Code, http.StatusNotModified)
	}
	if got := w.Header().Get("ETag"); got != `"chatto-shield-registered-0"` {
		t.Fatalf("ETag = %q, want registered zero ETag", got)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("304 body length = %d, want 0", w.Body.Len())
	}
}

func setupShieldTestServer(t *testing.T, enabled bool) *HTTPServer {
	t.Helper()
	gin.SetMode(gin.TestMode)
	server := setupHTTPServerTestServer(t, config.AuthConfig{})
	server.config.Shields.Enabled = enabled
	server.setupShieldRoutes()
	return server
}

func performShieldRequest(router *gin.Engine, method, path, ifNoneMatch string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func testShieldContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func waitForShieldLivePresenceCount(t *testing.T, ctx context.Context, c *core.ChattoCore, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var got int
	var err error
	for time.Now().Before(deadline) {
		got, err = c.LivePresenceCount(ctx)
		if err == nil && got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("LivePresenceCount = %d, %v; want %d", got, err, want)
}
