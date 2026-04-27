package http_server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// setupHealthRoutes registers health check endpoints for Kubernetes probes.
func (s *HTTPServer) setupHealthRoutes() {
	// Liveness probe - is the server process alive?
	// Returns 200 if the HTTP server is running.
	s.router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Readiness probe - is the server ready to accept traffic?
	// Checks NATS connectivity and JetStream initialization.
	//
	// The `reason` is logged but not returned in the response body. Returning
	// internal startup state to anonymous callers leaks fingerprintable
	// information about NATS/JetStream phases during outages.
	s.router.GET("/readyz", func(c *gin.Context) {
		if s.nc == nil || !s.nc.IsConnected() {
			s.logger.Warn("readyz: NATS not connected")
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
			return
		}

		if s.core != nil {
			if err := s.core.Ready(c.Request.Context()); err != nil {
				s.logger.Warn("readyz: core not ready", "error", err)
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
}
