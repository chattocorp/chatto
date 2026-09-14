//go:build test_endpoints

package http_server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/core"
)

func registerSeedEndpoint(auth *gin.RouterGroup, s *HTTPServer) {
	// A real cookie session for an existing test account, without password
	// hashing or a simulated registration flow. Never compiled into releases.
	auth.POST("test/create-session", func(c *gin.Context) {
		var request struct {
			UserID string `json:"userId" binding:"required"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "userId is required"})
			return
		}
		user, err := s.core.GetUser(c.Request.Context(), request.UserID)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, core.ErrNotFound) {
				status = http.StatusNotFound
			}
			c.JSON(status, gin.H{"error": "test account is unavailable"})
			return
		}
		if user.GetIsBot() || user.GetDeleted() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "test session requires an active human account"})
			return
		}
		if err := s.createCookieSession(c, user.Id, "test_seed_session"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create test session"})
			return
		}
		if err := s.ensureCSRFToken(c); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create CSRF token"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"userId": user.Id})
	})
	auth.POST("test/seed", func(c *gin.Context) {
		var options core.SeedOptions
		if err := c.ShouldBindJSON(&options); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid seed request"})
			return
		}
		if err := options.Validate(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := s.core.SeedData(c.Request.Context(), options)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, core.ErrLoginAlreadyTaken) || errors.Is(err, core.ErrRoomNameExists) {
				status = http.StatusConflict
			}
			if errors.Is(err, core.ErrInvalidArgument) {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error(), "partial": result})
			return
		}
		c.JSON(http.StatusOK, result)
	})
}
