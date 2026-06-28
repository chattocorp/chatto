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
	if !strings.Contains(text, `chatto_model_info{model="config_manager"} 1`) {
		t.Fatalf("metrics body missing config_manager model label\n%s", text)
	}
	if !strings.Contains(text, `chatto_model_info{model="message_model"} 1`) {
		t.Fatalf("metrics body missing message_model model label\n%s", text)
	}
	if !strings.Contains(text, `chatto_service_info{service="message_service"} 1`) {
		t.Fatalf("metrics body missing deprecated message_service alias\n%s", text)
	}
	if strings.Contains(text, `service="Config Manager"`) {
		t.Fatalf("metrics body used human service name in deprecated label\n%s", text)
	}
	if strings.Contains(text, `service="message_model"`) {
		t.Fatalf("metrics body used model key in deprecated service label\n%s", text)
	}
	if strings.Contains(text, `model="Message Model"`) {
		t.Fatalf("metrics body used human model name as label\n%s", text)
	}
}
