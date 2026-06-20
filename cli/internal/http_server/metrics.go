package http_server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type processMetrics struct {
	activeGraphQLWebSockets atomic.Int64
	activeClientLiveSockets atomic.Int64
	clientLiveOpened        atomic.Uint64
	clientLiveClosed        atomic.Uint64
	clientLiveErrors        metricCounterSet
	clientLiveRequests      metricCounterSet
	clientLiveRequestTime   metricFloatSet
}

func newProcessMetrics() *processMetrics {
	return &processMetrics{}
}

func (m *processMetrics) openGraphQLWebSocket() func() {
	m.activeGraphQLWebSockets.Add(1)
	var closed atomic.Bool
	return func() {
		if closed.CompareAndSwap(false, true) {
			m.activeGraphQLWebSockets.Add(-1)
		}
	}
}

func (m *processMetrics) activeWebSockets() int64 {
	if m == nil {
		return 0
	}
	return m.activeGraphQLWebSockets.Load()
}

func (m *processMetrics) openClientLiveSocket() func() {
	if m == nil {
		return func() {}
	}
	m.activeClientLiveSockets.Add(1)
	m.clientLiveOpened.Add(1)
	var closed atomic.Bool
	return func() {
		if closed.CompareAndSwap(false, true) {
			m.activeClientLiveSockets.Add(-1)
			m.clientLiveClosed.Add(1)
		}
	}
}

func (m *processMetrics) activeClientLiveWebSockets() int64 {
	if m == nil {
		return 0
	}
	return m.activeClientLiveSockets.Load()
}

func (m *processMetrics) recordClientLiveError(code string) {
	if m == nil {
		return
	}
	m.clientLiveErrors.Add(safeMetricLabel(code))
}

func (m *processMetrics) recordClientLiveRequest(requestType, outcome string, duration time.Duration) {
	if m == nil {
		return
	}
	key := safeMetricLabel(requestType) + "\xff" + safeMetricLabel(outcome)
	m.clientLiveRequests.Add(key)
	m.clientLiveRequestTime.Add(key, duration.Seconds())
}

func (m *processMetrics) clientLiveOpenedTotal() uint64 {
	if m == nil {
		return 0
	}
	return m.clientLiveOpened.Load()
}

func (m *processMetrics) clientLiveClosedTotal() uint64 {
	if m == nil {
		return 0
	}
	return m.clientLiveClosed.Load()
}

type metricCounterSet struct {
	mu     sync.Mutex
	values map[string]uint64
}

func (s *metricCounterSet) Add(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string]uint64)
	}
	s.values[key]++
}

func (s *metricCounterSet) Snapshot() map[string]uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := make(map[string]uint64, len(s.values))
	for key, value := range s.values {
		snapshot[key] = value
	}
	return snapshot
}

type metricFloatSet struct {
	mu     sync.Mutex
	values map[string]float64
}

func (s *metricFloatSet) Add(key string, value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string]float64)
	}
	s.values[key] += value
}

func (s *metricFloatSet) Snapshot() map[string]float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := make(map[string]float64, len(s.values))
	for key, value := range s.values {
		snapshot[key] = value
	}
	return snapshot
}

func (s *HTTPServer) newMetricsServer() (*http.Server, error) {
	if s.metrics == nil {
		s.metrics = newProcessMetrics()
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		newChattoCollector(s),
	)

	mux := http.NewServeMux()
	mux.Handle(s.config.Metrics.PathOrDefault(), promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	addr := net.JoinHostPort(s.config.Metrics.BindAddressOrDefault(), fmt.Sprint(s.config.Metrics.PortOrDefault()))
	return newHTTPServer(addr, mux), nil
}

type chattoCollector struct {
	server *HTTPServer

	buildInfo               *prometheus.Desc
	ready                   *prometheus.Desc
	webSockets              *prometheus.Desc
	clientLiveWebSockets    *prometheus.Desc
	clientLiveOpened        *prometheus.Desc
	clientLiveClosed        *prometheus.Desc
	clientLiveRequests      *prometheus.Desc
	clientLiveRequestTime   *prometheus.Desc
	clientLiveErrors        *prometheus.Desc
	myEventsActive          *prometheus.Desc
	myEventsDelivered       *prometheus.Desc
	myEventsSlowDisconnects *prometheus.Desc
	presenceRefreshes       *prometheus.Desc
	presenceFailures        *prometheus.Desc
	serviceInfo             *prometheus.Desc
	natsConnected           *prometheus.Desc
	natsRTT                 *prometheus.Desc
	natsMessages            *prometheus.Desc
	natsBytes               *prometheus.Desc
	natsReconnects          *prometheus.Desc
	projectionStarted       *prometheus.Desc
	projectionStartup       *prometheus.Desc
	projectionStartupMsgs   *prometheus.Desc
	projectionFailed        *prometheus.Desc
	projectionLastApplied   *prometheus.Desc
	projectionTarget        *prometheus.Desc
	projectionLag           *prometheus.Desc
	projectionEntries       *prometheus.Desc
	projectionBytes         *prometheus.Desc
	scrapeError             *prometheus.Desc
}

func newChattoCollector(server *HTTPServer) *chattoCollector {
	return &chattoCollector{
		server: server,

		buildInfo: prometheus.NewDesc(
			"chatto_build_info",
			"Build information for this Chatto process.",
			[]string{"version"},
			nil,
		),
		ready: prometheus.NewDesc(
			"chatto_ready",
			"Whether this Chatto process is ready to serve application traffic.",
			nil,
			nil,
		),
		webSockets: prometheus.NewDesc(
			"chatto_graphql_websocket_connections",
			"Active GraphQL WebSocket connections in this process.",
			nil,
			nil,
		),
		clientLiveWebSockets: prometheus.NewDesc(
			"chatto_client_live_websocket_connections",
			"Active protobuf client live WebSocket connections in this process.",
			nil,
			nil,
		),
		clientLiveOpened: prometheus.NewDesc(
			"chatto_client_live_websocket_opened_total",
			"Total protobuf client live WebSocket connections opened by this process.",
			nil,
			nil,
		),
		clientLiveClosed: prometheus.NewDesc(
			"chatto_client_live_websocket_closed_total",
			"Total protobuf client live WebSocket connections closed by this process.",
			nil,
			nil,
		),
		clientLiveRequests: prometheus.NewDesc(
			"chatto_client_live_requests_total",
			"Total protobuf client live request/response calls handled by this process.",
			[]string{"type", "outcome"},
			nil,
		),
		clientLiveRequestTime: prometheus.NewDesc(
			"chatto_client_live_request_duration_seconds_sum",
			"Cumulative protobuf client live request/response handling time in seconds.",
			[]string{"type", "outcome"},
			nil,
		),
		clientLiveErrors: prometheus.NewDesc(
			"chatto_client_live_errors_total",
			"Total protobuf client live protocol errors emitted by this process.",
			[]string{"code"},
			nil,
		),
		myEventsActive: prometheus.NewDesc(
			"chatto_my_events_streams",
			"Active GraphQL myEvents subscription streams in this process.",
			nil,
			nil,
		),
		myEventsDelivered: prometheus.NewDesc(
			"chatto_my_events_delivered_total",
			"Total GraphQL myEvents envelopes delivered by this process.",
			nil,
			nil,
		),
		myEventsSlowDisconnects: prometheus.NewDesc(
			"chatto_my_events_slow_consumer_disconnects_total",
			"Total myEvents streams closed because their NATS live-event subscription was a slow consumer.",
			nil,
			nil,
		),
		presenceRefreshes: prometheus.NewDesc(
			"chatto_presence_refreshes_total",
			"Total successful presence TTL refreshes from myEvents streams in this process.",
			nil,
			nil,
		),
		presenceFailures: prometheus.NewDesc(
			"chatto_presence_refresh_failures_total",
			"Total failed presence TTL refreshes from myEvents streams in this process.",
			nil,
			nil,
		),
		serviceInfo: prometheus.NewDesc(
			"chatto_service_info",
			"Registered core runtime service in this Chatto process.",
			[]string{"service"},
			nil,
		),
		natsConnected: prometheus.NewDesc(
			"chatto_nats_connected",
			"Whether this process is currently connected to NATS.",
			nil,
			nil,
		),
		natsRTT: prometheus.NewDesc(
			"chatto_nats_rtt_seconds",
			"Current NATS round-trip time in seconds.",
			nil,
			nil,
		),
		natsMessages: prometheus.NewDesc(
			"chatto_nats_messages_total",
			"Total NATS messages sent or received by this process.",
			[]string{"direction"},
			nil,
		),
		natsBytes: prometheus.NewDesc(
			"chatto_nats_bytes_total",
			"Total NATS bytes sent or received by this process.",
			[]string{"direction"},
			nil,
		),
		natsReconnects: prometheus.NewDesc(
			"chatto_nats_reconnects_total",
			"Total NATS reconnects observed by this process.",
			nil,
			nil,
		),
		projectionStarted: prometheus.NewDesc(
			"chatto_projection_started",
			"Whether a process-local projection has started.",
			[]string{"projection"},
			nil,
		),
		projectionStartup: prometheus.NewDesc(
			"chatto_projection_startup_duration_seconds",
			"Seconds from process-local projection start until its initial replay completed.",
			[]string{"projection"},
			nil,
		),
		projectionStartupMsgs: prometheus.NewDesc(
			"chatto_projection_startup_messages",
			"Number of matching EVT messages applied by a process-local projection during initial replay.",
			[]string{"projection"},
			nil,
		),
		projectionFailed: prometheus.NewDesc(
			"chatto_projection_failed",
			"Whether a process-local projection has failed.",
			[]string{"projection"},
			nil,
		),
		projectionLastApplied: prometheus.NewDesc(
			"chatto_projection_last_applied_sequence",
			"Last EVT stream sequence applied by a process-local projection.",
			[]string{"projection"},
			nil,
		),
		projectionTarget: prometheus.NewDesc(
			"chatto_projection_target_sequence",
			"Current matching EVT stream target sequence for a process-local projection.",
			[]string{"projection"},
			nil,
		),
		projectionLag: prometheus.NewDesc(
			"chatto_projection_lag_events",
			"Number of matching EVT stream events not yet applied by a process-local projection.",
			[]string{"projection"},
			nil,
		),
		projectionEntries: prometheus.NewDesc(
			"chatto_projection_entries",
			"Estimated number of entries held by a process-local projection.",
			[]string{"projection"},
			nil,
		),
		projectionBytes: prometheus.NewDesc(
			"chatto_projection_estimated_bytes",
			"Estimated heap bytes held by a process-local projection.",
			[]string{"projection"},
			nil,
		),
		scrapeError: prometheus.NewDesc(
			"chatto_metrics_scrape_error",
			"Whether a Chatto metrics collector failed during this scrape.",
			[]string{"collector"},
			nil,
		),
	}
}

func (c *chattoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.buildInfo
	ch <- c.ready
	ch <- c.webSockets
	ch <- c.clientLiveWebSockets
	ch <- c.clientLiveOpened
	ch <- c.clientLiveClosed
	ch <- c.clientLiveRequests
	ch <- c.clientLiveRequestTime
	ch <- c.clientLiveErrors
	ch <- c.myEventsActive
	ch <- c.myEventsDelivered
	ch <- c.myEventsSlowDisconnects
	ch <- c.presenceRefreshes
	ch <- c.presenceFailures
	ch <- c.serviceInfo
	ch <- c.natsConnected
	ch <- c.natsRTT
	ch <- c.natsMessages
	ch <- c.natsBytes
	ch <- c.natsReconnects
	ch <- c.projectionStarted
	ch <- c.projectionStartup
	ch <- c.projectionFailed
	ch <- c.projectionLastApplied
	ch <- c.projectionTarget
	ch <- c.projectionLag
	ch <- c.projectionEntries
	ch <- c.projectionBytes
	ch <- c.scrapeError
}

func (c *chattoCollector) Collect(ch chan<- prometheus.Metric) {
	version := c.server.version
	if version == "" {
		version = "unknown"
	}
	ch <- prometheus.MustNewConstMetric(c.buildInfo, prometheus.GaugeValue, 1, version)

	c.collectProcessMetrics(ch)
	c.collectNATSMetrics(ch)
	c.collectCoreMetrics(ch)
}

func (c *chattoCollector) collectProcessMetrics(ch chan<- prometheus.Metric) {
	metrics := c.server.metrics
	ch <- prometheus.MustNewConstMetric(c.webSockets, prometheus.GaugeValue, float64(metrics.activeWebSockets()))
	ch <- prometheus.MustNewConstMetric(c.clientLiveWebSockets, prometheus.GaugeValue, float64(metrics.activeClientLiveWebSockets()))
	ch <- prometheus.MustNewConstMetric(c.clientLiveOpened, prometheus.CounterValue, float64(metrics.clientLiveOpenedTotal()))
	ch <- prometheus.MustNewConstMetric(c.clientLiveClosed, prometheus.CounterValue, float64(metrics.clientLiveClosedTotal()))
	for key, value := range metrics.clientLiveRequests.Snapshot() {
		requestType, outcome := splitMetricPair(key)
		ch <- prometheus.MustNewConstMetric(c.clientLiveRequests, prometheus.CounterValue, float64(value), requestType, outcome)
	}
	for key, value := range metrics.clientLiveRequestTime.Snapshot() {
		requestType, outcome := splitMetricPair(key)
		ch <- prometheus.MustNewConstMetric(c.clientLiveRequestTime, prometheus.CounterValue, value, requestType, outcome)
	}
	for code, value := range metrics.clientLiveErrors.Snapshot() {
		ch <- prometheus.MustNewConstMetric(c.clientLiveErrors, prometheus.CounterValue, float64(value), code)
	}
}

func (c *chattoCollector) collectNATSMetrics(ch chan<- prometheus.Metric) {
	if c.server.nc == nil {
		ch <- prometheus.MustNewConstMetric(c.natsConnected, prometheus.GaugeValue, 0)
		return
	}

	connected := 0.0
	if c.server.nc.IsConnected() {
		connected = 1
		if rtt, err := c.server.nc.RTT(); err == nil {
			ch <- prometheus.MustNewConstMetric(c.natsRTT, prometheus.GaugeValue, rtt.Seconds())
		}
	}
	ch <- prometheus.MustNewConstMetric(c.natsConnected, prometheus.GaugeValue, connected)

	stats := c.server.nc.Stats()
	ch <- prometheus.MustNewConstMetric(c.natsMessages, prometheus.CounterValue, float64(stats.InMsgs), "in")
	ch <- prometheus.MustNewConstMetric(c.natsMessages, prometheus.CounterValue, float64(stats.OutMsgs), "out")
	ch <- prometheus.MustNewConstMetric(c.natsBytes, prometheus.CounterValue, float64(stats.InBytes), "in")
	ch <- prometheus.MustNewConstMetric(c.natsBytes, prometheus.CounterValue, float64(stats.OutBytes), "out")
	ch <- prometheus.MustNewConstMetric(c.natsReconnects, prometheus.CounterValue, float64(stats.Reconnects))
}

func (c *chattoCollector) collectCoreMetrics(ch chan<- prometheus.Metric) {
	if c.server.core == nil {
		ch <- prometheus.MustNewConstMetric(c.ready, prometheus.GaugeValue, 0)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), httpServerReadHeaderTimeout)
	defer cancel()

	ready := 1.0
	if err := c.server.core.Ready(ctx); err != nil {
		ready = 0
	}
	ch <- prometheus.MustNewConstMetric(c.ready, prometheus.GaugeValue, ready)

	myEvents := c.server.core.MyEventsMetrics()
	ch <- prometheus.MustNewConstMetric(c.myEventsActive, prometheus.GaugeValue, float64(myEvents.ActiveStreams))
	ch <- prometheus.MustNewConstMetric(c.myEventsDelivered, prometheus.CounterValue, float64(myEvents.DeliveredEvents))
	ch <- prometheus.MustNewConstMetric(c.myEventsSlowDisconnects, prometheus.CounterValue, float64(myEvents.SlowDisconnects))
	ch <- prometheus.MustNewConstMetric(c.presenceRefreshes, prometheus.CounterValue, float64(myEvents.PresenceRefreshes))
	ch <- prometheus.MustNewConstMetric(c.presenceFailures, prometheus.CounterValue, float64(myEvents.PresenceFailures))
	for _, service := range c.server.core.ServiceMetadata() {
		ch <- prometheus.MustNewConstMetric(c.serviceInfo, prometheus.GaugeValue, 1, service.Key)
	}

	projections, err := c.server.core.ProjectionAdminStates(ctx)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.scrapeError, prometheus.GaugeValue, 1, "projections")
		return
	}
	ch <- prometheus.MustNewConstMetric(c.scrapeError, prometheus.GaugeValue, 0, "projections")
	for _, projection := range projections {
		started := boolMetric(projection.Started)
		failed := boolMetric(projection.Failed)
		ch <- prometheus.MustNewConstMetric(c.projectionStarted, prometheus.GaugeValue, started, projection.Key)
		if projection.StartupComplete {
			ch <- prometheus.MustNewConstMetric(c.projectionStartup, prometheus.GaugeValue, projection.StartupDuration, projection.Key)
			ch <- prometheus.MustNewConstMetric(c.projectionStartupMsgs, prometheus.GaugeValue, float64(projection.StartupMessages), projection.Key)
		}
		ch <- prometheus.MustNewConstMetric(c.projectionFailed, prometheus.GaugeValue, failed, projection.Key)
		ch <- prometheus.MustNewConstMetric(c.projectionLastApplied, prometheus.GaugeValue, float64(projection.LastAppliedSeq), projection.Key)
		ch <- prometheus.MustNewConstMetric(c.projectionTarget, prometheus.GaugeValue, float64(projection.MatchingStreamSeq), projection.Key)
		ch <- prometheus.MustNewConstMetric(c.projectionLag, prometheus.GaugeValue, float64(projection.Lag), projection.Key)
		ch <- prometheus.MustNewConstMetric(c.projectionEntries, prometheus.GaugeValue, float64(projection.EntryCount), projection.Key)
		ch <- prometheus.MustNewConstMetric(c.projectionBytes, prometheus.GaugeValue, float64(projection.EstimatedBytes), projection.Key)
	}
}

func boolMetric(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func splitMetricPair(key string) (string, string) {
	left, right, ok := strings.Cut(key, "\xff")
	if !ok {
		return safeMetricLabel(key), "unknown"
	}
	return left, right
}

func safeMetricLabel(value string) string {
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}
