package http_server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestSetupMCPRoutesHonorsEnabled(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		server := setupOAuthServer(t)
		if err := server.setupMCPRoutes(); err != nil {
			t.Fatalf("setupMCPRoutes: %v", err)
		}
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "https://chatto.example/.well-known/oauth-protected-resource/mcp", nil)
		server.router.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("disabled metadata status = %d, want 404", response.Code)
		}
	})

	t.Run("enabled on public listener", func(t *testing.T) {
		server := setupOAuthServer(t)
		server.config.MCP = config.MCPConfig{Enabled: true}
		if err := server.setupMCPRoutes(); err != nil {
			t.Fatalf("setupMCPRoutes: %v", err)
		}
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "https://chatto.example/.well-known/oauth-protected-resource/mcp", nil)
		server.router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("enabled metadata status = %d, want 200: %s", response.Code, response.Body.String())
		}
	})
}
