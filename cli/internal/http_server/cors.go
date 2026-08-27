package http_server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	serverDiscoveryConnectPath = connectAPIPrefix + "/chatto.discovery.v1.ServerDiscoveryService/GetServer"
	corsAllowedHeaders         = "Authorization, Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms, X-CSRF-Token, Range, If-None-Match, If-Modified-Since"
)

// corsMiddleware permits browser clients from any origin to use Chatto's
// public HTTP surface. Cross-origin authentication is bearer-only, so the
// response deliberately never enables credentialed CORS.
func (s *HTTPServer) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Origin") == "" {
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", corsAllowedHeaders)
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
