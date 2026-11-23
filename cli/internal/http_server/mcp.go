package http_server

import (
	"fmt"
	"net/http"

	"github.com/chattocorp/chatto/cli/internal/mcpserver"
	"github.com/gin-gonic/gin"
)

const (
	mcpPath                  = "/mcp"
	mcpProtectedResourcePath = "/.well-known/oauth-protected-resource/mcp"
)

// setupMCPRoutes mounts the optional MCP integration on Chatto's public HTTP
// server. The routes accept the canonical origin and exact configured aliases.
func (s *HTTPServer) setupMCPRoutes() error {
	if !s.config.MCP.Enabled {
		notFound := func(c *gin.Context) { c.Status(http.StatusNotFound) }
		s.router.Any(mcpPath, notFound)
		s.router.Any(mcpProtectedResourcePath, notFound)
		return nil
	}
	handler, err := mcpserver.NewHandler(s.core, s.config, s.version, mcpserver.WithMessageSearch(s.connectAPI))
	if err != nil {
		return fmt.Errorf("configure MCP routes: %w", err)
	}
	// Pass the client IP that gin resolved through webserver.trusted_proxies,
	// so that admission limits key callers behind a proxy correctly.
	wrapped := func(c *gin.Context) {
		handler.ServeHTTP(c.Writer, mcpserver.WithClientAddress(c.Request, c.ClientIP()))
	}
	s.router.Any(mcpPath, wrapped)
	s.router.Any(mcpProtectedResourcePath, wrapped)
	return nil
}
