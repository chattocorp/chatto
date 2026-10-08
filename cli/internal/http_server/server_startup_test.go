package http_server

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/config"
)

// startupLogWriter transfers complete records without sharing a buffer between
// the server goroutine and the test.
type startupLogWriter chan []byte

func (w startupLogWriter) Write(p []byte) (int, error) {
	w <- bytes.Clone(p)
	return len(p), nil
}

func TestHTTPServerReadinessAndShutdown(t *testing.T) {
	t.Parallel()
	records := make(startupLogWriter, 16)
	logger := log.New(records)
	logger.SetFormatter(log.JSONFormatter)
	router := gin.New()
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	s := &HTTPServer{addr: "127.0.0.1:0", router: router, logger: logger, version: "test-version", startupStartedAt: time.Now().Add(-time.Second)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	})
	var record map[string]any
	select {
	case data := <-records:
		require.NoError(t, json.Unmarshal(data, &record))
	case <-time.After(5 * time.Second):
		t.Fatal("no readiness record")
	}
	require.Equal(t, "Server ready", record["msg"])
	require.Equal(t, "test-version", record["version"])
	duration, err := time.ParseDuration(record["duration"].(string))
	require.NoError(t, err)
	require.GreaterOrEqual(t, duration, time.Second)
	addr := record["addr"].(string)
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + addr + "/")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	client.CloseIdleConnections()
	cancel()
	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
		}
		return err != nil
	}, 5*time.Second, 10*time.Millisecond)
}

func TestHTTPServerBindFailureDoesNotLogReady(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"http", "metrics", "https"} {
		t.Run(failure, func(t *testing.T) {
			occupied, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer occupied.Close()
			// Reserve an address for the listener opened before the failed one.
			first, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			firstAddr := first.Addr().String()
			firstPort := first.Addr().(*net.TCPAddr).Port
			require.NoError(t, first.Close())
			var output bytes.Buffer
			s := &HTTPServer{addr: firstAddr, router: gin.New(), logger: log.New(&output)}
			switch failure {
			case "http":
				s.addr = occupied.Addr().String()
			case "metrics":
				s.config.Metrics = config.MetricsConfig{Enabled: true, BindAddress: "127.0.0.1", Port: occupied.Addr().(*net.TCPAddr).Port}
			case "https":
				s.addr = occupied.Addr().String()
				s.config.Webserver.TLS = config.TLSConfig{Enabled: true, Domain: "example.invalid", CacheDir: t.TempDir(), HTTPPort: firstPort}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			require.Error(t, s.Run(ctx))
			require.False(t, strings.Contains(output.String(), "Server ready"))
			rebound, err := net.Listen("tcp", firstAddr)
			require.NoError(t, err, "partial startup must release earlier listeners")
			require.NoError(t, rebound.Close())
		})
	}
}
