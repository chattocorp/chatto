package http_server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/mcpserver"
)

const (
	mcpPath                  = "/mcp"
	mcpProtectedResourcePath = "/.well-known/oauth-protected-resource/mcp"
)

// setupMCPRoutes mounts the optional MCP integration on Chatto's public HTTP
// server. The routes share the public listener and canonical webserver origin.
func (s *HTTPServer) setupMCPRoutes() error {
	if !s.config.MCP.Enabled {
		notFound := func(c *gin.Context) { c.Status(http.StatusNotFound) }
		s.router.Any(mcpPath, notFound)
		s.router.Any(mcpProtectedResourcePath, notFound)
		return nil
	}
	handler, err := mcpserver.NewHandler(s.core, s.config, s.version)
	if err != nil {
		return fmt.Errorf("configure MCP routes: %w", err)
	}
	wrapped := gin.WrapH(handler)
	s.router.Any(mcpPath, wrapped)
	s.router.Any(mcpProtectedResourcePath, wrapped)
	return nil
}
