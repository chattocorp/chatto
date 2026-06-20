package http_server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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
	closeClientLive := s.metrics.openClientLiveSocket()
	defer closeClientLive()
	s.metrics.recordClientLiveRequest("room.events", "ok", 25)
	s.metrics.recordClientLiveRequest("room.events", "forbidden", 50)
	s.metrics.recordClientLiveError("forbidden")

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
		`chatto_client_live_websocket_connections 1`,
		`chatto_client_live_websocket_opened_total 1`,
		`chatto_client_live_requests_total{outcome="ok",type="room.events"} 1`,
		`chatto_client_live_requests_total{outcome="forbidden",type="room.events"} 1`,
		`chatto_client_live_errors_total{code="forbidden"} 1`,
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
	if !strings.Contains(text, `chatto_projection_startup_messages{projection="content_keys"}`) {
		t.Fatalf("metrics body missing content_keys startup messages metric\n%s", text)
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

func TestProcessMetricsTracksClientLiveWebSockets(t *testing.T) {
	metrics := newProcessMetrics()
	closeA := metrics.openClientLiveSocket()
	closeB := metrics.openClientLiveSocket()

	if got := metrics.activeClientLiveWebSockets(); got != 2 {
		t.Fatalf("activeClientLiveWebSockets() = %d, want 2", got)
	}
	if got := metrics.clientLiveOpenedTotal(); got != 2 {
		t.Fatalf("clientLiveOpenedTotal() = %d, want 2", got)
	}

	closeA()
	closeA()
	if got := metrics.activeClientLiveWebSockets(); got != 1 {
		t.Fatalf("activeClientLiveWebSockets() after idempotent close = %d, want 1", got)
	}
	if got := metrics.clientLiveClosedTotal(); got != 1 {
		t.Fatalf("clientLiveClosedTotal() = %d, want 1", got)
	}

	closeB()
	if got := metrics.activeClientLiveWebSockets(); got != 0 {
		t.Fatalf("activeClientLiveWebSockets() after close = %d, want 0", got)
	}
}

func TestClientLiveRequestMetricsClampUnknownRequestTypes(t *testing.T) {
	metrics := newProcessMetrics()
	session := newClientLiveSession(&HTTPServer{metrics: metrics}, nil, "user", func() {})

	session.handleClientRequest(t.Context(), 1, &corev1.ClientLiveRequest{Type: "tenant/user/input"})

	requests := metrics.clientLiveRequests.Snapshot()
	if got := requests["unknown\xffunknown_request"]; got != 1 {
		t.Fatalf("unknown request metric = %d, want 1; snapshot=%v", got, requests)
	}
	if _, ok := requests["tenant_user_input\xffunknown_request"]; ok {
		t.Fatalf("unexpected high-cardinality request metric in snapshot: %v", requests)
	}
}
