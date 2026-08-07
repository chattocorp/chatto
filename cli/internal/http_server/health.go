package http_server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// setupHealthRoutes registers health check endpoints for Kubernetes probes.
func (s *HTTPServer) setupHealthRoutes() {
	// Liveness remains healthy through a recoverable reconnect, but fails once
	// the shared NATS client is permanently closed or recovery exceeds its grace
	// period. This lets Kubernetes restart a replica that can no longer make
	// progress while avoiding churn during cluster failover or a short outage.
	s.router.GET("/healthz", func(c *gin.Context) {
		if s.nc == nil || s.nc.IsClosed() {
			s.logger.Error("healthz: NATS connection permanently closed")
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not live"})
			return
		}
		if s.core != nil {
			if err := s.core.NATSRecoveryLivenessError(time.Now()); err != nil {
				s.logger.Error("healthz: NATS recovery stalled", "error", err)
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not live"})
				return
			}
		}
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
