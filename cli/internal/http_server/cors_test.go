package http_server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupCORSServer(t *testing.T) *HTTPServer {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	server := &HTTPServer{router: router}
	router.Use(server.corsMiddleware())
	router.GET("/api/connect/test", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	router.POST(serverDiscoveryConnectPath, func(c *gin.Context) { c.String(http.StatusOK, "server info") })
	return server
}

func TestCORSMiddleware(t *testing.T) {
	t.Run("request without Origin has no CORS headers", func(t *testing.T) {
		server := setupCORSServer(t)
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/connect/test", nil))
		if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("status/origin = %d/%q", response.Code, response.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("any browser origin receives bearer-only wildcard CORS", func(t *testing.T) {
		server := setupCORSServer(t)
		request := httptest.NewRequest(http.MethodGet, "/api/connect/test", nil)
		request.Header.Set("Origin", "https://client.example")
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		if got := response.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Fatalf("Access-Control-Allow-Origin = %q", got)
		}
		if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Fatalf("Access-Control-Allow-Credentials = %q", got)
		}
	})

	t.Run("preflight permits bearer and ConnectRPC headers without credentials", func(t *testing.T) {
		server := setupCORSServer(t)
		request := httptest.NewRequest(http.MethodOptions, "/api/connect/test", nil)
		request.Header.Set("Origin", "chatto://desktop")
		request.Header.Set("Access-Control-Request-Method", "POST")
		request.Header.Set("Access-Control-Request-Headers", "authorization, content-type, connect-protocol-version, connect-timeout-ms")
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("status/origin = %d/%q", response.Code, response.Header().Get("Access-Control-Allow-Origin"))
		}
		for _, required := range []string{"Authorization", "Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms"} {
			if !strings.Contains(response.Header().Get("Access-Control-Allow-Headers"), required) {
				t.Fatalf("allowed headers omit %q: %q", required, response.Header().Get("Access-Control-Allow-Headers"))
			}
		}
	})

	t.Run("discovery uses the same public wildcard policy", func(t *testing.T) {
		server := setupCORSServer(t)
		request := httptest.NewRequest(http.MethodPost, serverDiscoveryConnectPath, nil)
		request.Header.Set("Origin", "https://unknown.example")
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("status/origin = %d/%q", response.Code, response.Header().Get("Access-Control-Allow-Origin"))
		}
	})
}
