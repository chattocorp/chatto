package http_server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
)

func TestRequestLoggerUsesConfiguredFormatter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	logger := log.New(&buf)
	logger.SetFormatter(log.JSONFormatter)
	logger.SetPrefix("server.HTTP")

	router := gin.New()
	router.Use(requestLogger(logger))
	router.GET("/missing", func(c *gin.Context) {
		c.String(http.StatusNotFound, "missing")
	})

	req := httptest.NewRequest(http.MethodGet, "/missing?asset=avatar", nil)
	req.Header.Set("User-Agent", "chatto-test")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("request log should be JSON, got %q: %v", buf.String(), err)
	}

	assertLogField(t, line, "level", "warn")
	assertLogField(t, line, "prefix", "server.HTTP")
	assertLogField(t, line, "msg", "HTTP request")
	assertLogField(t, line, "method", http.MethodGet)
	assertLogField(t, line, "path", "/missing")
	if got := line["query_present"]; got != true {
		t.Fatalf("query_present field = %v, want true", got)
	}
	if _, ok := line["query"]; ok {
		t.Fatalf("request log should not include raw query string: %v", line["query"])
	}
	if got := line["client_ip_present"]; got != true {
		t.Fatalf("client_ip_present field = %v, want true", got)
	}
	if _, ok := line["client_ip"]; ok {
		t.Fatalf("request log should not include raw client IP: %v", line["client_ip"])
	}
	assertLogField(t, line, "user_agent", "chatto-test")

	if got := line["status"]; got != float64(http.StatusNotFound) {
		t.Fatalf("status field = %v, want %d", got, http.StatusNotFound)
	}
	if _, ok := line["latency"].(string); !ok {
		t.Fatalf("latency field = %T, want string", line["latency"])
	}
}

func TestRequestLoggerLogsSuccessfulRequestsAtDebug(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	logger := log.New(&buf)
	logger.SetFormatter(log.JSONFormatter)
	logger.SetLevel(log.DebugLevel)

	router := gin.New()
	router.Use(requestLogger(logger))
	router.GET("/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("request log should be JSON, got %q: %v", buf.String(), err)
	}

	assertLogField(t, line, "level", "debug")
	assertLogField(t, line, "msg", "HTTP request")
	assertLogField(t, line, "method", http.MethodGet)
	assertLogField(t, line, "path", "/ok")
	if got := line["status"]; got != float64(http.StatusOK) {
		t.Fatalf("status field = %v, want %d", got, http.StatusOK)
	}
}

func assertLogField(t *testing.T, line map[string]any, key string, want string) {
	t.Helper()

	if got := line[key]; got != want {
		t.Fatalf("%s field = %v, want %q", key, got, want)
	}
}
