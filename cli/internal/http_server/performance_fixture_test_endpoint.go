//go:build test_endpoints

package http_server

import (
	"net/http"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/core"
)

const (
	defaultPerformanceFixtureUsers    = 2048
	defaultPerformanceFixtureMessages = 50_000
	maxPerformanceFixtureUsers        = 5_000
	maxPerformanceFixtureMessages     = 100_000
)

func registerPerformanceFixtureEndpoint(auth *gin.RouterGroup, s *HTTPServer) {
	auth.POST("test/seed-performance", func(c *gin.Context) {
		var request struct {
			Users     int `json:"users"`
			Messages  int `json:"messages"`
			BatchSize int `json:"batchSize"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if request.Users == 0 {
			request.Users = defaultPerformanceFixtureUsers
		}
		if request.Messages == 0 {
			request.Messages = defaultPerformanceFixtureMessages
		}
		if request.Users < 1 || request.Users > maxPerformanceFixtureUsers {
			c.JSON(http.StatusBadRequest, gin.H{"error": "users must be between 1 and 5000"})
			return
		}
		if request.Messages < 1 || request.Messages > maxPerformanceFixtureMessages {
			c.JSON(http.StatusBadRequest, gin.H{"error": "messages must be between 1 and 100000"})
			return
		}

		startedAt := time.Now()
		result, err := s.core.SeedPerformanceFixture(c.Request.Context(), core.PerformanceFixtureOptions{
			Users:     request.Users,
			Messages:  request.Messages,
			BatchSize: request.BatchSize,
		})
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		elapsed := time.Since(startedAt)
		log.Info("Generated E2E performance fixture",
			"version", result.Version,
			"users", result.SyntheticUsers,
			"messages", result.Messages,
			"elapsed", elapsed)
		c.JSON(http.StatusOK, gin.H{
			"version":         result.Version,
			"syntheticUsers":  result.SyntheticUsers,
			"messages":        result.Messages,
			"roomId":          result.RoomID,
			"roomName":        result.RoomName,
			"firstUserLogin":  result.FirstUserLogin,
			"lastUserLogin":   result.LastUserLogin,
			"lastMessageId":   result.LastMessageID,
			"lastMessageBody": result.LastMessageBody,
			"seedDurationMs":  elapsed.Milliseconds(),
		})
	})
}
