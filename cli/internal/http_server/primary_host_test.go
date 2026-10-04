package http_server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/config"
)

func TestFrontendPrimaryHostRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, method, path, host, accept, mode string
		disabled, websocket, forwarded         bool
		redirect                               bool
	}{
		{name: "root navigation", path: "/", redirect: true},
		{name: "chat navigation preserves escaped path and query", path: "/chat/remote.example/room%2Fid?search=a%20b&next=%2Fchat", redirect: true},
		{name: "login navigation", path: "/login?next=%2Fchat", redirect: true},
		{name: "HEAD navigation", method: "HEAD", path: "/chat", redirect: true},
		{name: "explicit browser navigation", path: "/chat", mode: "navigate", redirect: true},
		{name: "disabled by default", path: "/chat", disabled: true},
		{name: "primary host", path: "/chat", host: "primary.example"},
		{name: "canonical primary host", path: "/chat", host: "PRIMARY.EXAMPLE:443"},
		{name: "unknown host is not a configured alias", path: "/chat", host: "unknown.example"},
		{name: "forwarded host cannot select an alias", path: "/chat", host: "primary.example", forwarded: true},
		{name: "forwarded host cannot suppress a redirect", path: "/chat", forwarded: true, redirect: true},
		{name: "JSON request", path: "/chat", accept: "application/json"},
		{name: "wildcard accept is not browser navigation", path: "/chat", accept: "*/*"},
		{name: "HTML explicitly rejected", path: "/chat", accept: "text/html;q=0,application/json"},
		{name: "invalid HTML quality", path: "/chat", accept: "text/html;q=NaN"},
		{name: "HTML fetch is not navigation", path: "/chat", mode: "cors"},
		{name: "POST", method: "POST", path: "/chat"},
		{name: "WebSocket", path: "/chat", websocket: true},
		{name: "frontend OAuth popup callback", path: "/servers/callback?mode=popup&code=test&state=test"},
		{name: "callback with trailing slash", path: "/servers/callback/?mode=popup"},
		{name: "static frontend file", path: "/service-worker.js"},
		{name: "missing static asset", path: "/_app/immutable/missing.js"},
		{name: "missing icon", path: "/icons/missing.png"},
		{name: "API", path: "/api/probe"},
		{name: "OAuth", path: "/oauth/probe"},
		{name: "OIDC callback", path: "/auth/providers/test/callback?code=test"},
		{name: "uploaded asset", path: "/assets/probe"},
		{name: "MCP", path: "/mcp"},
		{name: "discovery", path: "/.well-known/probe"},
		{name: "ACME challenge", path: "/.well-known/acme-challenge/test"},
		{name: "health", path: "/healthz"},
		{name: "readiness", path: "/readyz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			server := &HTTPServer{router: router, config: config.ChattoConfig{Webserver: config.WebserverConfig{
				URL: "https://primary.example", AllowedOrigins: []string{"https://alias.example", "*"},
				RedirectToPrimaryHost: !tc.disabled,
			}}}
			for _, route := range []string{"/api/probe", "/oauth/probe", "/auth/providers/test/callback", "/assets/probe", "/mcp", "/healthz", "/readyz", "/.well-known/probe", "/.well-known/acme-challenge/test"} {
				router.GET(route, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
			}
			require.NoError(t, server.setupFrontendRoutes())
			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, "https://alias.example"+tc.path, nil)
			if tc.host != "" {
				req.Host = tc.host
			}
			accept := tc.accept
			if accept == "" {
				accept = "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8"
			}
			req.Header.Set("Accept", accept)
			req.Header.Set("Sec-Fetch-Mode", tc.mode)
			if tc.websocket {
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
			}
			if tc.forwarded {
				req.Header.Set("X-Forwarded-Host", "alias.example")
				req.Header.Set("Forwarded", "host=primary.example;proto=https")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if tc.redirect {
				require.Equal(t, http.StatusTemporaryRedirect, response.Code)
				require.Equal(t, "https://primary.example"+tc.path, response.Header().Get("Location"))
				require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			} else {
				require.Equal(t, http.StatusOK, response.Code)
				require.Empty(t, response.Header().Get("Location"))
			}
		})
	}
}
