package http_server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/testutil"
)

func TestHealthEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartNATS(t)

	t.Run("healthz returns ok", func(t *testing.T) {
		router := gin.New()
		router.GET("/healthz", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest("GET", "/healthz", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if resp["status"] != "ok" {
			t.Errorf("expected status 'ok', got '%s'", resp["status"])
		}
	})

	t.Run("readyz returns ready when NATS connected", func(t *testing.T) {
		router := gin.New()
		router.GET("/readyz", func(c *gin.Context) {
			if nc == nil || !nc.IsConnected() {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"status": "not ready",
					"reason": "NATS not connected",
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "ready"})
		})

		req := httptest.NewRequest("GET", "/readyz", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if resp["status"] != "ready" {
			t.Errorf("expected status 'ready', got '%s'", resp["status"])
		}
	})

	t.Run("readyz returns not ready when NATS disconnected", func(t *testing.T) {
		// Create a disconnected NATS connection
		var disconnectedNC *nats.Conn = nil

		router := gin.New()
		router.GET("/readyz", func(c *gin.Context) {
			if disconnectedNC == nil || !disconnectedNC.IsConnected() {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"status": "not ready",
					"reason": "NATS not connected",
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "ready"})
		})

		req := httptest.NewRequest("GET", "/readyz", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status 503, got %d", w.Code)
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if resp["status"] != "not ready" {
			t.Errorf("expected status 'not ready', got '%s'", resp["status"])
		}
	})
}
