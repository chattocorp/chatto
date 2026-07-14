package http_server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/pkg/signedurl"
)

func TestExtractImmutableETag(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "entry with hash",
			path:     "/_app/immutable/entry/start.CxnbWTuF.js",
			expected: "CxnbWTuF",
		},
		{
			name:     "chunk with hash only",
			path:     "/_app/immutable/chunks/Dynhoydm.js",
			expected: "Dynhoydm",
		},
		{
			name:     "CSS with hash",
			path:     "/_app/immutable/assets/app.D2jh4_eq.css",
			expected: "D2jh4_eq",
		},
		{
			name:     "nested path with hash",
			path:     "/_app/immutable/nodes/0.BFpGYTTP.js",
			expected: "BFpGYTTP",
		},
		{
			name:     "entry app with hash",
			path:     "/_app/immutable/entry/app.BR6S17SI.js",
			expected: "BR6S17SI",
		},
		{
			name:     "woff2 font with hash",
			path:     "/_app/immutable/assets/ibm-plex-sans-latin-wght-normal.IvpUnPaZ.woff2",
			expected: "IvpUnPaZ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractImmutableETag(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestImmutableAssetCaching(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a minimal test router that simulates our caching middleware
	router := gin.New()

	router.Use(setFrontendCacheHeaders)

	// Add a simple handler that returns content
	router.GET("/*path", func(c *gin.Context) {
		c.String(http.StatusOK, "file content")
	})

	t.Run("immutable asset returns correct headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_app/immutable/entry/start.CxnbWTuF.js", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, cacheControlImmutable, w.Header().Get("Cache-Control"))
		assert.Equal(t, `"CxnbWTuF"`, w.Header().Get("ETag"))
	})

	t.Run("non-immutable asset returns no-cache", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/index.html", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, cacheControlNoCache, w.Header().Get("Cache-Control"))
		assert.Empty(t, w.Header().Get("ETag"))
	})

	t.Run("conditional request with matching ETag returns 304", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_app/immutable/entry/start.CxnbWTuF.js", nil)
		req.Header.Set("If-None-Match", `"CxnbWTuF"`)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotModified, w.Code)
		assert.Empty(t, w.Body.String()) // 304 should have no body
	})

	t.Run("conditional request with weak ETag returns 304", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_app/immutable/entry/start.CxnbWTuF.js", nil)
		req.Header.Set("If-None-Match", `W/"CxnbWTuF"`)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotModified, w.Code)
	})

	t.Run("conditional request with non-matching ETag returns 200", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_app/immutable/entry/start.CxnbWTuF.js", nil)
		req.Header.Set("If-None-Match", `"different-etag"`)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotEmpty(t, w.Body.String())
	})

	t.Run("_app non-immutable returns no-cache", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_app/version.json", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, cacheControlNoCache, w.Header().Get("Cache-Control"))
		assert.Empty(t, w.Header().Get("ETag"))
	})

	t.Run("service worker returns revalidate cache policy", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/service-worker.js", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, cacheControlRevalidate, w.Header().Get("Cache-Control"))
		assert.Empty(t, w.Header().Get("ETag"))
	})
}

func TestServiceWorkerETag(t *testing.T) {
	gin.SetMode(gin.TestMode)

	content := []byte("self.addEventListener('fetch', () => {});")
	etag := serviceWorkerETag(content)

	router := gin.New()
	router.Use(setFrontendCacheHeaders)
	router.GET("/service-worker.js", func(c *gin.Context) {
		if setServiceWorkerETag(c, content) {
			return
		}
		c.Data(http.StatusOK, "application/javascript", content)
	})

	t.Run("returns etag", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/service-worker.js", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, cacheControlRevalidate, w.Header().Get("Cache-Control"))
		assert.Equal(t, etag, w.Header().Get("ETag"))
		assert.Equal(t, string(content), w.Body.String())
	})

	t.Run("matching if none match returns 304", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/service-worker.js", nil)
		req.Header.Set("If-None-Match", etag)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotModified, w.Code)
		assert.Equal(t, cacheControlRevalidate, w.Header().Get("Cache-Control"))
		assert.Equal(t, etag, w.Header().Get("ETag"))
		assert.Empty(t, w.Body.String())
	})
}

func TestDynamicPWAManifest(t *testing.T) {
	staticManifest := []byte(`{
  "name": "Chatto",
  "short_name": "Chatto",
  "icons": [
    { "src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "/icons/icon-512.png", "sizes": "512x512", "type": "image/png" }
  ],
  "shortcuts": [
    {
      "name": "Open Chatto",
      "icons": [{ "src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png" }]
    }
  ]
}`)

	t.Run("uses server name without requiring a server logo", func(t *testing.T) {
		got, err := dynamicPWAManifest(staticManifest, "Engineering", nil)
		if err != nil {
			t.Fatalf("dynamicPWAManifest: %v", err)
		}

		var manifest map[string]any
		if err := json.Unmarshal(got, &manifest); err != nil {
			t.Fatalf("unmarshal manifest: %v", err)
		}
		assert.Equal(t, "Engineering", manifest["name"])
		assert.Equal(t, "Engineering", manifest["short_name"])
		assert.Len(t, manifest["icons"], 2)
	})

	t.Run("replaces install and shortcut icons with server logo URLs", func(t *testing.T) {
		got, err := dynamicPWAManifest(staticManifest, "Engineering", &pwaServerIconURLs{
			Icon192: "/assets/server/logo/t/192",
			Icon512: "/assets/server/logo/t/512",
		})
		if err != nil {
			t.Fatalf("dynamicPWAManifest: %v", err)
		}

		var manifest map[string]any
		if err := json.Unmarshal(got, &manifest); err != nil {
			t.Fatalf("unmarshal manifest: %v", err)
		}
		assert.Equal(t, "Engineering", manifest["name"])
		assert.Equal(t, "Engineering", manifest["short_name"])

		icons := manifest["icons"].([]any)
		assert.Len(t, icons, 4)
		assert.Equal(t, "/assets/server/logo/t/192", icons[0].(map[string]any)["src"])
		assert.Equal(t, "192x192", icons[0].(map[string]any)["sizes"])
		assert.Equal(t, "image/png", icons[0].(map[string]any)["type"])
		assert.Equal(t, "/assets/server/logo/t/512", icons[1].(map[string]any)["src"])
		assert.Equal(t, "image/png", icons[1].(map[string]any)["type"])
		assert.Equal(t, "maskable", icons[2].(map[string]any)["purpose"])
		assert.Equal(t, "image/png", icons[2].(map[string]any)["type"])
		assert.Equal(t, "maskable", icons[3].(map[string]any)["purpose"])
		assert.Equal(t, "image/png", icons[3].(map[string]any)["type"])

		shortcuts := manifest["shortcuts"].([]any)
		shortcutIcons := shortcuts[0].(map[string]any)["icons"].([]any)
		assert.Equal(t, "/assets/server/logo/t/192", shortcutIcons[0].(map[string]any)["src"])
		assert.Equal(t, "image/png", shortcutIcons[0].(map[string]any)["type"])
	})
}

func TestSameOriginServerAssetURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "keeps relative server asset URL",
			url:  "/assets/server/logo/t/signed",
			want: "/assets/server/logo/t/signed",
		},
		{
			name: "removes external asset origin",
			url:  "https://assets.example.com/assets/server/logo/t/signed?variant=pwa",
			want: "/assets/server/logo/t/signed?variant=pwa",
		},
		{
			name: "rejects unrelated asset URL",
			url:  "https://assets.example.com/assets/files/private",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sameOriginServerAssetURL(tt.url))
		})
	}
}

func TestClientAcceptsEncoding(t *testing.T) {
	tests := []struct {
		name           string
		acceptEncoding string
		encoding       string
		expected       bool
	}{
		{
			name:           "accepts brotli in list",
			acceptEncoding: "gzip, deflate, br",
			expected:       true,
			encoding:       "br",
		},
		{
			name:           "accepts gzip in list",
			acceptEncoding: "gzip, deflate, br",
			expected:       true,
			encoding:       "gzip",
		},
		{
			name:           "single encoding",
			acceptEncoding: "br",
			expected:       true,
			encoding:       "br",
		},
		{
			name:           "not in list",
			acceptEncoding: "gzip, deflate",
			expected:       false,
			encoding:       "br",
		},
		{
			name:           "empty header",
			acceptEncoding: "",
			expected:       false,
			encoding:       "br",
		},
		{
			name:           "with quality values",
			acceptEncoding: "gzip;q=1.0, br;q=0.8, *;q=0.1",
			expected:       true,
			encoding:       "br",
		},
		{
			name:           "no spaces",
			acceptEncoding: "gzip,deflate,br",
			expected:       true,
			encoding:       "br",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clientAcceptsEncoding(tt.acceptEncoding, tt.encoding)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestServeSPAFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a minimal HTTPServer for testing
	newTestServer := func() *HTTPServer {
		return &HTTPServer{
			config: config.ChattoConfig{Webserver: config.WebserverConfig{URL: "https://example.com"}},
		}
	}

	t.Run("returns 200 with content when 200.html exists", func(t *testing.T) {
		mockFS := fstest.MapFS{
			"200.html": &fstest.MapFile{
				Data: []byte("<!DOCTYPE html><html><head><!-- OG_META_PLACEHOLDER --></head><body>SPA</body></html>"),
			},
		}

		server := newTestServer()
		router := gin.New()
		router.GET("/test", func(c *gin.Context) {
			server.serveSPAFallback(c, mockFS)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "SPA")
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	})

	t.Run("injects OpenGraph tags", func(t *testing.T) {
		mockFS := fstest.MapFS{
			"200.html": &fstest.MapFile{
				Data: []byte("<!DOCTYPE html><html><head><!-- OG_META_PLACEHOLDER --></head><body>SPA</body></html>"),
			},
		}

		server := newTestServer()
		router := gin.New()
		router.GET("/test", func(c *gin.Context) {
			server.serveSPAFallback(c, mockFS)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		// Should contain OpenGraph tags
		assert.Contains(t, w.Body.String(), `og:title`)
		assert.Contains(t, w.Body.String(), `og:description`)
		assert.Contains(t, w.Body.String(), `twitter:card`)
		// Placeholder should be replaced
		assert.NotContains(t, w.Body.String(), "OG_META_PLACEHOLDER")
	})

	t.Run("returns 500 when 200.html is missing", func(t *testing.T) {
		// Empty filesystem - no 200.html
		mockFS := fstest.MapFS{}

		server := newTestServer()
		router := gin.New()
		router.GET("/test", func(c *gin.Context) {
			server.serveSPAFallback(c, mockFS)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "Failed to load application", w.Body.String())
	})
}

func TestBrowserIconRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newServer := func(t *testing.T, chattoCore *core.ChattoCore) *HTTPServer {
		t.Helper()
		server := &HTTPServer{
			config: config.ChattoConfig{Webserver: config.WebserverConfig{URL: "https://example.com"}},
			core:   chattoCore,
			router: gin.New(),
		}
		if err := server.setupFrontendRoutes(); err != nil {
			t.Fatalf("setupFrontendRoutes: %v", err)
		}
		return server
	}

	t.Run("redirects to distinct same-origin server logo transforms", func(t *testing.T) {
		chattoCore := setupFrontendTestCoreWithLogo(t)
		chattoCore.AssetBaseURL = "https://assets.example.com"
		server := newServer(t, chattoCore)

		expectedSizes := map[string]int{
			"/favicon":          32,
			"/apple-touch-icon": 180,
		}
		locations := make(map[string]string)
		for iconPath, expectedSize := range expectedSizes {
			req := httptest.NewRequest(http.MethodGet, iconPath, nil)
			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
			assert.Equal(t, cacheControlNoCache, w.Header().Get("Cache-Control"))
			location := w.Header().Get("Location")
			assert.True(t, strings.HasPrefix(location, "/assets/server/logo-asset/t/"))
			assert.NotContains(t, location, "assets.example.com")

			signedPath := strings.TrimPrefix(location, "/assets/server/logo-asset/t/")
			params, err := signedurl.ParseSignedTransformPath(
				"test-signing-secret",
				core.ServerAssetSignResource,
				"logo-asset",
				signedPath,
			)
			if err != nil {
				t.Fatalf("parse transform for %s: %v", iconPath, err)
			}
			assert.Equal(t, expectedSize, params.Width)
			assert.Equal(t, expectedSize, params.Height)
			assert.Equal(t, "cover", params.Fit)
			locations[iconPath] = location
		}
		assert.NotEqual(t, locations["/favicon"], locations["/apple-touch-icon"])
	})

	t.Run("redirects to embedded icons when no server logo exists", func(t *testing.T) {
		server := newServer(t, nil)
		tests := map[string]string{
			"/favicon":          "/icons/favicon.png",
			"/apple-touch-icon": "/icons/apple-touch-icon.png",
		}
		for iconPath, fallbackPath := range tests {
			req := httptest.NewRequest(http.MethodGet, iconPath, nil)
			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
			assert.Equal(t, fallbackPath, w.Header().Get("Location"))
		}
	})
}

func TestServePWAWebManifestUsesServerLogoWhenAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockFS := fstest.MapFS{
		"manifest.webmanifest": &fstest.MapFile{
			Data: []byte(`{
  "name": "Chatto",
  "icons": [{ "src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png" }],
  "shortcuts": [
    { "name": "Open Chatto", "icons": [{ "src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png" }] }
  ]
}`),
		},
	}
	chattoCore := setupFrontendTestCoreWithLogo(t)
	setTestServerName(t, context.Background(), chattoCore, "Engineering")
	chattoCore.AssetBaseURL = "https://assets.example.com"
	server := &HTTPServer{
		config: config.ChattoConfig{Webserver: config.WebserverConfig{URL: "https://example.com"}},
		core:   chattoCore,
		router: gin.New(),
	}
	server.router.GET("/manifest.webmanifest", func(c *gin.Context) {
		server.servePWAWebManifest(c, mockFS)
	})

	req := httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/manifest+json", w.Header().Get("Content-Type"))

	var manifest map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	assert.Equal(t, "Engineering", manifest["name"])
	assert.Equal(t, "Engineering", manifest["short_name"])
	icons := manifest["icons"].([]any)
	assert.True(t, strings.HasPrefix(icons[0].(map[string]any)["src"].(string), "/assets/server/logo-asset/t/"))
	assert.NotContains(t, icons[0].(map[string]any)["src"], "assets.example.com")
	assert.Equal(t, "192x192", icons[0].(map[string]any)["sizes"])
	assert.Equal(t, "image/png", icons[0].(map[string]any)["type"])
	assert.Equal(t, "maskable", icons[2].(map[string]any)["purpose"])
	assert.Equal(t, "image/png", icons[2].(map[string]any)["type"])
}

func TestFrontendFallbackDoesNotServeReservedBackendPrefixes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := &HTTPServer{
		config: config.ChattoConfig{Webserver: config.WebserverConfig{URL: "https://example.com"}},
		router: gin.New(),
	}
	sessionStore := cookie.NewStore([]byte("test-secret-key-32-bytes-long!!"))
	server.router.Use(sessions.Sessions("chatto_session", sessionStore))
	if err := server.setupFrontendRoutes(); err != nil {
		t.Fatalf("setupFrontendRoutes: %v", err)
	}

	tests := []string{
		"/api/unknown",
		"/auth/unknown",
		"/assets/unknown",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusNotFound, w.Code)
			assert.NotContains(t, w.Body.String(), "<!DOCTYPE html>")
		})
	}
}

func setupFrontendTestCoreWithLogo(t *testing.T) *core.ChattoCore {
	t.Helper()

	_, nc := testutil.StartSharedNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	chattoCore, err := core.NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

	logo := &corev1.AssetRecord{
		Id:          "logo-asset",
		Filename:    "logo.webp",
		ContentType: "image/webp",
		Storage:     &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: "logo-asset"}},
	}
	if err := chattoCore.SetServerLogo(ctx, core.SystemActorID, logo); err != nil {
		t.Fatalf("SetServerLogo: %v", err)
	}
	return chattoCore
}

func TestFrontendFallbackAllowsRoutesWithReservedPrefixNames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := &HTTPServer{
		config: config.ChattoConfig{Webserver: config.WebserverConfig{URL: "https://example.com"}},
		router: gin.New(),
	}
	sessionStore := cookie.NewStore([]byte("test-secret-key-32-bytes-long!!"))
	server.router.Use(sessions.Sessions("chatto_session", sessionStore))
	if err := server.setupFrontendRoutes(); err != nil {
		t.Fatalf("setupFrontendRoutes: %v", err)
	}

	tests := []string{
		"/apiary",
		"/author",
		"/assets-gallery",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, strings.ToLower(w.Body.String()), "<!doctype html>")
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a minimal test router that simulates our security headers middleware
	router := gin.New()

	// Add the security headers middleware (same as in setupFrontendRoutes)
	router.Use(func(c *gin.Context) {
		setFrontendSecurityHeaders(c)
		c.Next()
	})

	router.GET("/*path", func(c *gin.Context) {
		c.String(http.StatusOK, "content")
	})

	t.Run("security headers are set", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/index.html", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
		assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
		csp := w.Header().Get("Content-Security-Policy-Report-Only")
		assert.NotEmpty(t, csp)
		assert.Contains(t, csp, "default-src 'self'")
		assert.Contains(t, csp, "connect-src 'self' http: https: ws: wss:")
		assert.Contains(t, csp, "img-src 'self' data: blob: http: https:")
		assert.Contains(t, csp, "media-src 'self' blob: http: https:")
		assert.Contains(t, csp, "frame-src https://www.youtube-nocookie.com")
		assert.Contains(t, csp, "require-trusted-types-for 'script'")
		assert.Contains(t, csp, "trusted-types chatto-markdown-html")
		assert.NotContains(t, csp, "trusted-types default")
	})
}
