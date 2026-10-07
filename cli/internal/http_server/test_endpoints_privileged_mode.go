//go:build test_endpoints

package http_server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/core"
)

// registerPrivilegedModeDeadlineEndpoint lets E2E tests use the real server and
// browser expiry paths without waiting 15 minutes. It can only shorten the
// caller's already-active cookie session; it cannot activate or extend authority.
func registerPrivilegedModeDeadlineEndpoint(auth *gin.RouterGroup, s *HTTPServer) {
	auth.POST("test/privileged-mode-deadline", func(c *gin.Context) {
		var request struct {
			RemainingMS int64 `json:"remainingMs"`
		}
		if err := c.ShouldBindJSON(&request); err != nil || request.RemainingMS < 1 || request.RemainingMS > 60_000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "remainingMs must be between 1 and 60000"})
			return
		}
		credential, ok, err := s.cookiePresentedCredential(c)
		if err != nil || !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "A valid cookie session is required"})
			return
		}
		now := time.Now()
		entry, err := s.core.LoadCookieSessionValue(c.Request.Context(), credential.auth.Handle, now)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "A valid cookie session is required"})
			return
		}
		var session core.AuthTokenData
		if err := json.Unmarshal(entry.Value, &session); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not decode the session"})
			return
		}
		deadline := now.Add(time.Duration(request.RemainingMS) * time.Millisecond)
		if !now.Before(session.PrivilegedModeExpiresAt) || !deadline.Before(session.PrivilegedModeExpiresAt) {
			c.JSON(http.StatusConflict, gin.H{"error": "The deadline must shorten active privileged mode"})
			return
		}
		session.PrivilegedModeExpiresAt = deadline
		value, err := json.Marshal(session)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not encode the session"})
			return
		}
		// The observed revision prevents this hook from restoring a concurrent
		// deactivation, renewal or logout. Tests must retry from fresh state.
		if err := s.core.UpdateCookieSessionValue(c.Request.Context(), credential.auth.Handle, value, entry.Revision, now); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "The session changed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"expiresAt": deadline})
	})
}
