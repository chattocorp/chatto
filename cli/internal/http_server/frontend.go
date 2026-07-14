package http_server

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
)

//go:embed all:.client
var embeddedWebUIFS embed.FS

// Cache control headers for different file types
const (
	// HTML files must never be cached to ensure users get the latest version
	cacheControlNoCache = "no-store, no-cache, must-revalidate"
	// Service worker bytes should be revalidated without forcing a full refetch
	cacheControlRevalidate = "no-cache, must-revalidate"
	// Hashed assets (in _app/) are immutable - cache for 1 year
	cacheControlImmutable = "public, max-age=31536000, immutable"
	// Report-only CSP preserves Chatto's multi-server client model while surfacing
	// violations during development/staging before we consider enforcement.
	contentSecurityPolicyReportOnly = "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: http: https:; media-src 'self' blob: http: https:; connect-src 'self' http: https: ws: wss:; frame-src https://www.youtube-nocookie.com; worker-src 'self'; require-trusted-types-for 'script'; trusted-types chatto-markdown-html"
)

type pwaServerIconURLs struct {
	Icon192 string
	Icon512 string
}

func setFrontendSecurityHeaders(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
	c.Header("Content-Security-Policy-Report-Only", contentSecurityPolicyReportOnly)
}

// extractImmutableETag extracts an ETag from a SvelteKit immutable asset path.
// SvelteKit filenames include content hashes, e.g.:
//   - /_app/immutable/entry/start.CxnbWTuF.js → "CxnbWTuF"
//   - /_app/immutable/chunks/Dynhoydm.js → "Dynhoydm"
//
// Returns empty string if no hash can be extracted.
func extractImmutableETag(urlPath string) string {
	// Get the filename without extension
	base := path.Base(urlPath)
	ext := path.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// SvelteKit uses two patterns:
	// 1. name.HASH.ext (e.g., start.CxnbWTuF.js)
	// 2. HASH.ext (e.g., Dynhoydm.js) - the hash IS the name
	if idx := strings.LastIndex(name, "."); idx != -1 {
		// Pattern 1: extract hash after the last dot
		return name[idx+1:]
	}
	// Pattern 2: the entire name is the hash
	return name
}

func serviceWorkerETag(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf(`W/"%x"`, sum)
}

func etagMatches(ifNoneMatch string, etag string) bool {
	for _, part := range strings.Split(ifNoneMatch, ",") {
		if strings.TrimSpace(part) == etag {
			return true
		}
	}
	return false
}

func setServiceWorkerETag(c *gin.Context, content []byte) bool {
	etag := serviceWorkerETag(content)
	c.Header("ETag", etag)
	if etagMatches(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return true
	}
	return false
}

func setFrontendCacheHeaders(c *gin.Context) {
	urlPath := c.Request.URL.Path
	if strings.HasPrefix(urlPath, "/_app/immutable/") {
		c.Header("Cache-Control", cacheControlImmutable)

		// Extract ETag from the content-hashed filename
		if etag := extractImmutableETag(urlPath); etag != "" {
			quotedETag := `"` + etag + `"`
			c.Header("ETag", quotedETag)

			// Check If-None-Match for conditional requests
			if match := c.GetHeader("If-None-Match"); match != "" {
				// Handle both quoted and unquoted ETags, and weak ETags (W/"...")
				if match == quotedETag || match == etag || match == `W/`+quotedETag {
					c.AbortWithStatus(http.StatusNotModified)
					return
				}
			}
		}
	} else if urlPath == "/service-worker.js" {
		c.Header("Cache-Control", cacheControlRevalidate)
	} else {
		// For HTML and other non-hashed files, prevent caching
		c.Header("Cache-Control", cacheControlNoCache)
	}
	c.Next()
}

func (s *HTTPServer) currentServerIconURL(ctx context.Context, size int) string {
	if s.core == nil {
		return ""
	}

	width, height := size, size
	iconURL, err := s.core.GetServerLogoURL(ctx, &width, &height, "cover")
	if err != nil {
		s.logger.Warn("failed to get server logo URL for browser metadata", "error", err, "size", size)
		return ""
	}
	return sameOriginServerAssetURL(iconURL)
}

func (s *HTTPServer) currentPWAIconURLs(ctx context.Context) *pwaServerIconURLs {
	icons := &pwaServerIconURLs{
		Icon192: s.currentServerIconURL(ctx, 192),
		Icon512: s.currentServerIconURL(ctx, 512),
	}
	if icons.Icon192 == "" || icons.Icon512 == "" {
		return nil
	}
	return icons
}

func (s *HTTPServer) currentPWAServerName(ctx context.Context) string {
	if s.core == nil || s.core.ConfigManager() == nil {
		return "Chatto"
	}

	name, err := s.core.ConfigManager().GetEffectiveServerName(ctx)
	if err != nil || strings.TrimSpace(name) == "" {
		return "Chatto"
	}
	return name
}

// sameOriginServerAssetURL keeps browser metadata on the frontend origin. General
// asset URLs may use a configured asset base, but each Chatto frontend serves
// its own public server assets and browsers must be able to fetch metadata
// images from the frontend's origin.
func sameOriginServerAssetURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Opaque != "" || !strings.HasPrefix(parsed.Path, "/assets/server/") {
		return ""
	}

	result := parsed.EscapedPath()
	if parsed.RawQuery != "" {
		result += "?" + parsed.RawQuery
	}
	return result
}

func pwaManifestIcons(icon192, icon512 string) []map[string]string {
	return []map[string]string{
		{"src": icon192, "sizes": "192x192", "type": "image/png"},
		{"src": icon512, "sizes": "512x512", "type": "image/png"},
		{"src": icon192, "sizes": "192x192", "type": "image/png", "purpose": "maskable"},
		{"src": icon512, "sizes": "512x512", "type": "image/png", "purpose": "maskable"},
	}
}

func dynamicPWAManifest(staticManifest []byte, serverName string, icons *pwaServerIconURLs) ([]byte, error) {
	var manifest map[string]any
	if err := json.Unmarshal(staticManifest, &manifest); err != nil {
		return nil, err
	}

	manifest["name"] = serverName
	manifest["short_name"] = serverName
	if icons != nil {
		manifest["icons"] = pwaManifestIcons(icons.Icon192, icons.Icon512)
		if shortcuts, ok := manifest["shortcuts"].([]any); ok {
			for _, shortcut := range shortcuts {
				shortcutMap, ok := shortcut.(map[string]any)
				if !ok {
					continue
				}
				shortcutMap["icons"] = []map[string]string{
					{"src": icons.Icon192, "sizes": "192x192", "type": "image/png"},
				}
			}
		}
	}

	return json.MarshalIndent(manifest, "", "  ")
}

// clientAcceptsEncoding checks if the client accepts a specific encoding.
// It parses the Accept-Encoding header and looks for the encoding name.
func clientAcceptsEncoding(acceptEncoding, encoding string) bool {
	// Simple check - look for the encoding in the header
	// This handles common cases like "gzip, deflate, br" or "br"
	for _, part := range strings.Split(acceptEncoding, ",") {
		part = strings.TrimSpace(part)
		// Strip quality value if present (e.g., "gzip;q=0.8")
		if idx := strings.Index(part, ";"); idx != -1 {
			part = part[:idx]
		}
		if part == encoding {
			return true
		}
	}
	return false
}

// serveSPAFallback serves the 200.html file as a fallback for SPA routing.
// It injects OpenGraph meta tags based on the URL path.
// Returns true if the fallback was served successfully, false if an error occurred.
func (s *HTTPServer) serveSPAFallback(c *gin.Context, clientFS fs.FS) bool {
	content, err := fs.ReadFile(clientFS, "200.html")
	if err != nil {
		log.Error("Failed to read 200.html for SPA fallback", "error", err)
		c.String(http.StatusInternalServerError, "Failed to load application")
		return false
	}

	// Inject OpenGraph tags with a timeout to avoid blocking page loads
	ctx, cancel := context.WithTimeout(c.Request.Context(), 500*time.Millisecond)
	defer cancel()

	content = s.injectOpenGraphTags(ctx, content, c.Request.URL.Path)

	c.Data(http.StatusOK, "text/html; charset=utf-8", content)
	return true
}

func (s *HTTPServer) redirectBrowserIcon(c *gin.Context, size int, fallbackURL string) {
	iconURL := s.currentServerIconURL(c.Request.Context(), size)
	if iconURL == "" {
		iconURL = fallbackURL
	}
	c.Redirect(http.StatusTemporaryRedirect, iconURL)
}

func (s *HTTPServer) servePWAWebManifest(c *gin.Context, clientFS fs.FS) {
	content, err := fs.ReadFile(clientFS, "manifest.webmanifest")
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	content, err = dynamicPWAManifest(
		content,
		s.currentPWAServerName(c.Request.Context()),
		s.currentPWAIconURLs(c.Request.Context()),
	)
	if err != nil {
		s.logger.Warn("failed to generate dynamic PWA manifest", "error", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(http.StatusOK, "application/manifest+json", content)
}

// servePrecompressedFile attempts to serve a precompressed version of a file.
// It checks for .br (brotli) and .gz (gzip) variants based on Accept-Encoding.
// Returns true if a compressed file was served, false if the original should be served.
func servePrecompressedFile(c *gin.Context, clientFS fs.FS, filePath string) bool {
	acceptEncoding := c.GetHeader("Accept-Encoding")
	if acceptEncoding == "" {
		return false
	}

	// Try brotli first (better compression), then gzip
	encodings := []struct {
		name      string
		extension string
	}{
		{"br", ".br"},
		{"gzip", ".gz"},
	}

	for _, enc := range encodings {
		if !clientAcceptsEncoding(acceptEncoding, enc.name) {
			continue
		}

		compressedPath := filePath + enc.extension
		content, err := fs.ReadFile(clientFS, compressedPath)
		if err != nil {
			continue // Compressed file doesn't exist, try next
		}

		// Determine content type from the original file extension
		contentType := mime.TypeByExtension(filepath.Ext(filePath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		c.Header("Content-Encoding", enc.name)
		c.Header("Content-Type", contentType)
		// Vary header tells caches that response varies based on Accept-Encoding
		c.Header("Vary", "Accept-Encoding")
		c.Data(http.StatusOK, contentType, content)
		return true
	}

	return false
}

func isReservedNonFrontendPath(urlPath string) bool {
	return hasPathSegmentPrefix(urlPath, "/api") ||
		hasPathSegmentPrefix(urlPath, "/auth") ||
		hasPathSegmentPrefix(urlPath, "/assets")
}

func hasPathSegmentPrefix(urlPath string, prefix string) bool {
	return urlPath == prefix || strings.HasPrefix(urlPath, prefix+"/")
}

func (s *HTTPServer) setupFrontendRoutes() error {
	// Get a sub-filesystem rooted at .client
	clientFS, err := fs.Sub(embeddedWebUIFS, ".client")
	if err != nil {
		return err
	}

	// Security headers middleware - applied to all frontend routes
	s.router.Use(func(c *gin.Context) {
		setFrontendSecurityHeaders(c)
		c.Next()
	})

	// Middleware to set cache headers and ETags based on request path.
	// SvelteKit puts all hashed/immutable assets under /_app/immutable/
	// Other files under /_app/ (like version.json, env.js) are NOT content-hashed
	// and must not be cached immutably.
	s.router.Use(setFrontendCacheHeaders)

	// Browser icon metadata uses stable semantic URLs. Each request resolves the
	// current server logo so changing or removing branding does not require a
	// frontend rebuild. The cache middleware keeps these redirects temporary.
	s.router.Match([]string{"GET", "HEAD"}, "/favicon", func(c *gin.Context) {
		s.redirectBrowserIcon(c, 32, "/icons/favicon.png")
	})
	s.router.Match([]string{"GET", "HEAD"}, "/apple-touch-icon", func(c *gin.Context) {
		s.redirectBrowserIcon(c, 180, "/icons/apple-touch-icon.png")
	})

	// refreshSessionIfAuthenticated validates and rotates authenticated
	// cookie-session records for active SPA browsing. KV TTL is set only when
	// a session is created, so near-expiry sessions are rotated instead of
	// "touched" in place.
	refreshSessionIfAuthenticated := func(c *gin.Context) {
		credential, ok, _ := s.cookiePresentedCredential(c)
		if ok {
			s.rotateCookieSessionIfNeeded(c, credential.auth.UserID, credential.auth.Handle, credential.cookieRecord)
		}
	}

	// Custom static file handler with precompressed file support
	serveStatic := func(c *gin.Context, filePath string) {
		// Clean the path and prevent directory traversal
		filePath = path.Clean("/" + filePath)[1:]
		if filePath == "" {
			filePath = "200.html"
		}

		// Refresh session for all SPA routes to prevent cookie expiration
		refreshSessionIfAuthenticated(c)

		// Check if file exists
		_, err := fs.Stat(clientFS, filePath)
		if err != nil {
			// File not found, serve 200.html for SPA routing
			s.serveSPAFallback(c, clientFS)
			return
		}

		// For 200.html, use serveSPAFallback to inject OpenGraph tags
		if filePath == "200.html" {
			s.serveSPAFallback(c, clientFS)
			return
		}

		if filePath == "service-worker.js" {
			content, err := fs.ReadFile(clientFS, filePath)
			if err != nil {
				c.Status(http.StatusInternalServerError)
				return
			}
			if setServiceWorkerETag(c, content) {
				return
			}
		}

		if filePath == "manifest.webmanifest" {
			s.servePWAWebManifest(c, clientFS)
			return
		}

		// Try to serve precompressed version first
		if servePrecompressedFile(c, clientFS, filePath) {
			return
		}

		// Serve the original file
		content, err := fs.ReadFile(clientFS, filePath)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}

		contentType := mime.TypeByExtension(filepath.Ext(filePath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		c.Data(http.StatusOK, contentType, content)
	}

	// Handle root path explicitly to avoid directory listing
	s.router.Match([]string{"GET", "HEAD"}, "/", func(c *gin.Context) {
		serveStatic(c, "200.html")
	})

	// Serve static files with precompression support
	s.router.Use(func(c *gin.Context) {
		// Only handle GET and HEAD requests
		if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			c.Next()
			return
		}

		// Skip if path starts with /api, /auth, /assets (handled by other routes)
		urlPath := c.Request.URL.Path
		if isReservedNonFrontendPath(urlPath) {
			c.Next()
			return
		}

		serveStatic(c, urlPath)
		c.Abort()
	})

	// Fall back to 200.html for SPA routing (NoRoute handler)
	s.router.NoRoute(func(c *gin.Context) {
		if isReservedNonFrontendPath(c.Request.URL.Path) {
			c.Status(http.StatusNotFound)
			return
		}
		serveStatic(c, "200.html")
	})

	return nil
}
