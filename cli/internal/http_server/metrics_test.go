package http_server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
		`chatto_realtime_websocket_connections 0`,
		`chatto_realtime_catch_ups 0`,
		`chatto_realtime_catch_ups_started_total 0`,
		`chatto_realtime_catch_ups_timed_out_total 0`,
		`chatto_realtime_catch_ups_rejected_total{reason="rate_limited"} 0`,
		`chatto_realtime_catch_ups_rejected_total{reason="user_busy"} 0`,
		`chatto_realtime_catch_ups_rejected_total{reason="server_busy"} 0`,
		`chatto_nats_connected 0`,
		`chatto_ready 0`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics body missing %q\n%s", want, text)
		}
	}
}

func TestMetricsServerPprofDisabledByDefault(t *testing.T) {
	s := &HTTPServer{
		config: config.ChattoConfig{
			Metrics: config.MetricsConfig{
				Enabled: true,
			},
		},
		metrics: newProcessMetrics(),
	}

	metricsServer, err := s.newMetricsServer()
	if err != nil {
		t.Fatalf("newMetricsServer() error = %v", err)
	}
	ts := httptest.NewServer(metricsServer.Handler)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/debug/pprof/")
	if err != nil {
		t.Fatalf("GET pprof error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET pprof status = %d, want 404", resp.StatusCode)
	}
}

func TestMetricsServerPprofCanBeEnabled(t *testing.T) {
	s := &HTTPServer{
		config: config.ChattoConfig{
			Metrics: config.MetricsConfig{
				Enabled: true,
				Pprof:   true,
			},
		},
		metrics: newProcessMetrics(),
	}

	metricsServer, err := s.newMetricsServer()
	if err != nil {
		t.Fatalf("newMetricsServer() error = %v", err)
	}
	ts := httptest.NewServer(metricsServer.Handler)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/debug/pprof/")
	if err != nil {
		t.Fatalf("GET pprof error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET pprof status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read pprof body: %v", err)
	}
	if !strings.Contains(string(body), "Types of profiles available") {
		t.Fatalf("pprof index did not look like pprof output:\n%s", string(body))
	}
}

func TestMetricsServerUsesProjectionAndModelKeys(t *testing.T) {
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
	if !strings.Contains(text, `chatto_projection_startup_messages{projection="content_keys"}`) {
		t.Fatalf("metrics body missing content_keys startup messages metric\n%s", text)
	}
	if strings.Contains(text, `projection="Content Keys"`) {
		t.Fatalf("metrics body used human projection name as label\n%s", text)
	}
	if !strings.Contains(text, `chatto_model_info{model="config_model"} 1`) {
		t.Fatalf("metrics body missing config_model model label\n%s", text)
	}
	if !strings.Contains(text, `chatto_model_info{model="message_model"} 1`) {
		t.Fatalf("metrics body missing message_model model label\n%s", text)
	}
	if strings.Contains(text, "chatto_service_info") {
		t.Fatalf("metrics body contains retired chatto_service_info metric\n%s", text)
	}
	if strings.Contains(text, `model="Message Model"`) {
		t.Fatalf("metrics body used human model name as label\n%s", text)
	}
}

func TestMetricsServerTracksRealtimeWebSocketConnections(t *testing.T) {
	env := setupWebSocketTestServer(t)

	metricsServer, err := env.httpServer.newMetricsServer()
	if err != nil {
		t.Fatalf("newMetricsServer() error = %v", err)
	}
	ts := httptest.NewServer(metricsServer.Handler)
	t.Cleanup(ts.Close)

	assertMetricsContainsEventually(t, ts.URL+"/metrics", `chatto_realtime_websocket_connections 0`)

	conn := env.connectRealtime(t)
	assertMetricsContainsEventually(t, ts.URL+"/metrics", `chatto_realtime_websocket_connections 1`)

	if err := conn.Close(); err != nil {
		t.Fatalf("close realtime websocket: %v", err)
	}
	assertMetricsContainsEventually(t, ts.URL+"/metrics", `chatto_realtime_websocket_connections 0`)
}

func assertMetricsContainsEventually(t *testing.T, url, want string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	var text string
	for time.Now().Before(deadline) {
		text = scrapeMetricsText(t, url)
		if strings.Contains(text, want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("metrics body missing %q\n%s", want, text)
}

func scrapeMetricsText(t *testing.T, url string) string {
	t.Helper()

	resp, err := http.Get(url)
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
	return string(body)
}
