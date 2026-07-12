package http_server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
)

func TestShieldsDisabledReturnsNotFound(t *testing.T) {
	server := setupShieldTestServer(t, false)

	for _, path := range []string{shieldRoutePrefix + "/online.json", shieldRoutePrefix + "/online.png"} {
		t.Run(path, func(t *testing.T) {
			w := performShieldRequest(server.router, "GET", path, "")
			if w.Code != http.StatusNotFound {
				t.Fatalf("disabled shield status = %d, want %d", w.Code, http.StatusNotFound)
			}
		})
	}
}

func TestShieldsUnknownMetricReturnsNotFound(t *testing.T) {
	server := setupShieldTestServer(t, true)

	for _, path := range []string{shieldRoutePrefix + "/unknown.json", shieldRoutePrefix + "/unknown.png"} {
		t.Run(path, func(t *testing.T) {
			w := performShieldRequest(server.router, "GET", path, "")
			if w.Code != http.StatusNotFound {
				t.Fatalf("unknown shield status = %d, want %d", w.Code, http.StatusNotFound)
			}
		})
	}
}

func TestShieldsServeOnlineAndRegisteredEndpointJSON(t *testing.T) {
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
		path  string
		etag  string
		label string
		msg   string
		color string
	}{
		{path: shieldRoutePrefix + "/online.json", etag: `"chatto-shield-online-3"`, label: "online", msg: "3", color: shieldOnlineColor},
		{path: shieldRoutePrefix + "/registered.json", etag: `"chatto-shield-registered-1"`, label: "registered", msg: "1", color: shieldRegisteredColor},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := performShieldRequest(server.router, "GET", tt.path, "")
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
			}
			if got := w.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
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
			var body shieldEndpointResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not Shields endpoint JSON: %v", err)
			}
			if body.SchemaVersion != 1 || body.Label != tt.label || body.Message != tt.msg || body.Color != tt.color || body.LabelColor != shieldLabelColor {
				t.Fatalf("JSON body = %+v, want schemaVersion=1 label=%q message=%q color=%q labelColor=%q", body, tt.label, tt.msg, tt.color, shieldLabelColor)
			}
		})
	}
}

func TestShieldsPNGRedirectsToShieldsIOEndpoint(t *testing.T) {
	server := setupShieldTestServer(t, true)

	w := performShieldRequest(server.router, "GET", shieldRoutePrefix+"/online.png", "")
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if got := w.Header().Get("Cache-Control"); got != shieldCacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, shieldCacheControl)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	location := w.Header().Get("Location")
	u, err := url.Parse(location)
	if err != nil {
		t.Fatalf("redirect Location is not a URL: %q", location)
	}
	if got := u.Scheme + "://" + u.Host + u.Path; got != shieldsIOEndpointURL {
		t.Fatalf("redirect endpoint = %q, want %q", got, shieldsIOEndpointURL)
	}
	if got := u.Query().Get("url"); got != "http://example.com"+shieldRoutePrefix+"/online.json" {
		t.Fatalf("redirect endpoint url query = %q", got)
	}
}

func TestShieldsETagConditionalRequest(t *testing.T) {
	server := setupShieldTestServer(t, true)

	w := performShieldRequest(server.router, "GET", shieldRoutePrefix+"/registered.json", `"chatto-shield-registered-0"`)
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
	server.config.Webserver.Shields.Enabled = enabled
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
