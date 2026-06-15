package http_server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type processMetrics struct {
	activeGraphQLWebSockets atomic.Int64
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
	ch <- prometheus.MustNewConstMetric(c.webSockets, prometheus.GaugeValue, float64(c.server.metrics.activeWebSockets()))
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
