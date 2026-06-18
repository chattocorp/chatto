package http_server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestMetricsServerExposesPrometheusMetrics(t *testing.T) {
	s := &HTTPServer{
		config: config.ChattoConfig{
			Metrics: config.MetricsConfig{
				Enabled: true,
				Path:    "/internal/metrics",
			},
		},
		version: "test-version",
		metrics: newProcessMetrics(),
	}

	closeWebSocket := s.metrics.openGraphQLWebSocket()
	defer closeWebSocket()

	metricsServer, err := s.newMetricsServer()
	if err != nil {
		t.Fatalf("newMetricsServer() error = %v", err)
	}
	ts := httptest.NewServer(metricsServer.Handler)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/internal/metrics")
	if err != nil {
		t.Fatalf("GET metrics error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET metrics status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	text := string(body)

	for _, want := range []string{
		`chatto_build_info{version="test-version"} 1`,
		`chatto_graphql_websocket_connections 1`,
		`chatto_nats_connected 0`,
		`chatto_ready 0`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics body missing %q\n%s", want, text)
		}
	}
}

func TestMetricsServerUsesProjectionKeys(t *testing.T) {
	var appServer *HTTPServer
	setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Metrics = config.MetricsConfig{Enabled: true}
		s.metrics = newProcessMetrics()
		appServer = s
	})
	if appServer == nil {
		t.Fatal("expected setup hook to capture HTTP server")
	}

	metricsServer, err := appServer.newMetricsServer()
	if err != nil {
		t.Fatalf("newMetricsServer() error = %v", err)
	}
	ts := httptest.NewServer(metricsServer.Handler)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET metrics error = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	text := string(body)

	if !strings.Contains(text, `chatto_projection_lag_events{projection="content_keys"}`) {
		t.Fatalf("metrics body missing content_keys projection label\n%s", text)
	}
	if !strings.Contains(text, `chatto_projection_startup_duration_seconds{projection="content_keys"}`) {
		t.Fatalf("metrics body missing content_keys startup duration metric\n%s", text)
	}
	if strings.Contains(text, `projection="Content Keys"`) {
		t.Fatalf("metrics body used human projection name as label\n%s", text)
	}
	if !strings.Contains(text, `chatto_service_info{service="config_manager"} 1`) {
		t.Fatalf("metrics body missing config_manager service label\n%s", text)
	}
	if strings.Contains(text, `service="Config Manager"`) {
		t.Fatalf("metrics body used human service name as label\n%s", text)
	}
}

func TestProcessMetricsTracksGraphQLWebSockets(t *testing.T) {
	metrics := newProcessMetrics()
	closeA := metrics.openGraphQLWebSocket()
	closeB := metrics.openGraphQLWebSocket()

	if got := metrics.activeWebSockets(); got != 2 {
		t.Fatalf("activeWebSockets() = %d, want 2", got)
	}

	closeA()
	closeA()
	if got := metrics.activeWebSockets(); got != 1 {
		t.Fatalf("activeWebSockets() after idempotent close = %d, want 1", got)
	}

	closeB()
	if got := metrics.activeWebSockets(); got != 0 {
		t.Fatalf("activeWebSockets() after close = %d, want 0", got)
	}
}
