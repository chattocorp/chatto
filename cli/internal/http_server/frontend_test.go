package http_server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"hmans.de/chatto/internal/config"
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

	// Add the caching middleware (same logic as in setupFrontendRoutes)
	router.Use(func(c *gin.Context) {
		urlPath := c.Request.URL.Path
		if len(urlPath) > len("/_app/immutable/") && urlPath[:len("/_app/immutable/")] == "/_app/immutable/" {
			c.Header("Cache-Control", cacheControlImmutable)

			if etag := extractImmutableETag(urlPath); etag != "" {
				quotedETag := `"` + etag + `"`
				c.Header("ETag", quotedETag)

				if match := c.GetHeader("If-None-Match"); match != "" {
					if match == quotedETag || match == etag || match == `W/`+quotedETag {
						c.AbortWithStatus(http.StatusNotModified)
						return
					}
				}
			}
		} else {
			c.Header("Cache-Control", cacheControlNoCache)
		}
		c.Next()
	})

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
		assert.Contains(t, csp, "trusted-types default")
	})
}
