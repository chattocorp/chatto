package http_server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// buildAllowedOrigins constructs the list of origins that are allowed for CORS
// and WebSocket connections. This is computed once at server startup.
//
// Tiers:
//  1. Origin derived from webserver.url config
//  2. Localhost at the listen port (for dev/proxy setups)
//  3. Explicit entries from webserver.allowed_origins
func (s *HTTPServer) buildAllowedOrigins() []string {
	var origins []string

	// Tier 1: Configured public URL
	if parsedURL, err := url.Parse(s.config.Webserver.URL); err == nil {
		origins = append(origins, parsedURL.Scheme+"://"+parsedURL.Host)
	}

	// Tier 2: Localhost at listen port (dev convenience)
	listenOrigin := fmt.Sprintf("http://localhost:%d", s.config.Webserver.EffectivePort())
	origins = append(origins, listenOrigin)

	// Tier 3: Explicit allow list from config, defaulting to wildcard for
	// multi-server support. Remote instances authenticate via Bearer tokens,
	// not cookies, so wildcard is safe. The home origin (Tier 1) still gets
	// explicit matching with credentials.
	if len(s.config.Webserver.AllowedOrigins) > 0 {
		origins = append(origins, s.config.Webserver.AllowedOrigins...)
	} else {
		origins = append(origins, "*")
	}

	return origins
}

// originMatchType indicates how an origin was matched against the allow list.
type originMatchType int

const (
	originNotAllowed  originMatchType = iota
	originExplicit                    // Matched a specific origin in the allow list
	originWildcard                    // Matched because "*" is in the allow list
)

// matchOrigin checks whether an origin is in the allowed list and how it matched.
func (s *HTTPServer) matchOrigin(origin string, allowedOrigins []string) originMatchType {
	for _, allowed := range allowedOrigins {
		if allowed == "*" {
			return originWildcard
		}
		if strings.EqualFold(origin, allowed) {
			return originExplicit
		}
	}
	return originNotAllowed
}

// corsMiddleware returns Gin middleware that sets CORS headers for cross-origin
// requests. It skips the /api/server endpoint, which has its own public CORS
// headers (wildcard origin).
//
// When no Origin header is present (same-origin or non-browser clients), this
// middleware is a no-op.
func (s *HTTPServer) corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// /api/server handles its own CORS (public wildcard)
		if c.Request.URL.Path == "/api/server" {
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		match := s.matchOrigin(origin, allowedOrigins)
		if match != originNotAllowed {
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			c.Header("Access-Control-Max-Age", "86400")

			if match == originWildcard {
				// Wildcard: use literal "*" and omit credentials.
				// Cross-origin clients authenticate via Bearer tokens, not cookies.
				c.Header("Access-Control-Allow-Origin", "*")
			} else {
				// Explicit origin: reflect the origin and allow credentials (cookies).
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Vary", "Origin")
			}
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
