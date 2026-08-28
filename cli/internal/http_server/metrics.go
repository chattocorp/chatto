package http_server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"hmans.de/chatto/internal/core"
)

type processMetrics struct {
	realtimeWebSocketConnections atomic.Int64
	realtimeCatchUps             atomic.Int64
	realtimeCatchUpsStarted      atomic.Uint64
	realtimeCatchUpsTimedOut     atomic.Uint64
	realtimeCatchUpsRateLimited  atomic.Uint64
	realtimeCatchUpsUserBusy     atomic.Uint64
	realtimeCatchUpsServerBusy   atomic.Uint64
}

func (m *processMetrics) realtimeCatchUpStarted() {
	m.realtimeCatchUps.Add(1)
	m.realtimeCatchUpsStarted.Add(1)
}

func (m *processMetrics) realtimeCatchUpFinished() {
	m.realtimeCatchUps.Add(-1)
}

func (m *processMetrics) realtimeCatchUpTimedOut() {
	m.realtimeCatchUpsTimedOut.Add(1)
}

func (m *processMetrics) realtimeCatchUpRejected(code string) {
	switch code {
	case "catch_up_rate_limited":
		m.realtimeCatchUpsRateLimited.Add(1)
	case "catch_up_in_progress":
		m.realtimeCatchUpsUserBusy.Add(1)
	case "catch_up_server_busy":
		m.realtimeCatchUpsServerBusy.Add(1)
	}
}

func newProcessMetrics() *processMetrics {
	return &processMetrics{}
}

func (m *processMetrics) realtimeWebSocketOpened() {
	m.realtimeWebSocketConnections.Add(1)
}

func (m *processMetrics) realtimeWebSocketClosed() {
	m.realtimeWebSocketConnections.Add(-1)
}

func (m *processMetrics) realtimeWebSocketConnectionCount() int64 {
	return m.realtimeWebSocketConnections.Load()
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
	if s.config.Metrics.Pprof {
		registerPprofHandlers(mux)
	}

	addr := net.JoinHostPort(s.config.Metrics.BindAddressOrDefault(), fmt.Sprint(s.config.Metrics.PortOrDefault()))
	return newHTTPServer(addr, mux), nil
}

func registerPprofHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

type chattoCollector struct {
	server *HTTPServer

	buildInfo                *prometheus.Desc
	ready                    *prometheus.Desc
	realtimeWebSockets       *prometheus.Desc
	realtimeCatchUps         *prometheus.Desc
	realtimeCatchUpsStarted  *prometheus.Desc
	realtimeCatchUpsTimedOut *prometheus.Desc
	realtimeCatchUpsRejected *prometheus.Desc
	myEventsActive           *prometheus.Desc
	myEventsDelivered        *prometheus.Desc
	myEventsSlowDisconnects  *prometheus.Desc
	presenceRefreshes        *prometheus.Desc
	presenceFailures         *prometheus.Desc
	modelInfo                *prometheus.Desc
	natsConnected            *prometheus.Desc
	natsRTT                  *prometheus.Desc
	natsMessages             *prometheus.Desc
	natsBytes                *prometheus.Desc
	natsReconnects           *prometheus.Desc
	projectionStarted        *prometheus.Desc
	projectionStartup        *prometheus.Desc
	projectionStartupMsgs    *prometheus.Desc
	projectionFailed         *prometheus.Desc
	projectionLastApplied    *prometheus.Desc
	projectionTarget         *prometheus.Desc
	projectionLag            *prometheus.Desc
	projectionEntries        *prometheus.Desc
	projectionBytes          *prometheus.Desc
	scrapeError              *prometheus.Desc
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
		realtimeWebSockets: prometheus.NewDesc(
			"chatto_realtime_websocket_connections",
			"Current realtime WebSocket connections in this process.",
			nil,
			nil,
		),
		realtimeCatchUps: prometheus.NewDesc(
			"chatto_realtime_catch_ups",
			"Current realtime replay or compacted-bootstrap catch-ups in this process.",
			nil,
			nil,
		),
		realtimeCatchUpsStarted: prometheus.NewDesc(
			"chatto_realtime_catch_ups_started_total",
			"Total realtime catch-ups admitted by this process.",
			nil,
			nil,
		),
		realtimeCatchUpsTimedOut: prometheus.NewDesc(
			"chatto_realtime_catch_ups_timed_out_total",
			"Total realtime catch-ups that exhausted their whole-operation time budget.",
			nil,
			nil,
		),
		realtimeCatchUpsRejected: prometheus.NewDesc(
			"chatto_realtime_catch_ups_rejected_total",
			"Total realtime catch-ups rejected by the process-local capacity guard.",
			[]string{"reason"},
			nil,
		),
		myEventsActive: prometheus.NewDesc(
			"chatto_my_events_streams",
			"Active live event streams in this process.",
			nil,
			nil,
		),
		myEventsDelivered: prometheus.NewDesc(
			"chatto_my_events_delivered_total",
			"Total live event envelopes delivered by this process.",
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
		modelInfo: prometheus.NewDesc(
			"chatto_model_info",
			"Registered core model in this Chatto process.",
			[]string{"model"},
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
	ch <- c.realtimeWebSockets
	ch <- c.realtimeCatchUps
	ch <- c.realtimeCatchUpsStarted
	ch <- c.realtimeCatchUpsTimedOut
	ch <- c.realtimeCatchUpsRejected
	ch <- c.myEventsActive
	ch <- c.myEventsDelivered
	ch <- c.myEventsSlowDisconnects
	ch <- c.presenceRefreshes
	ch <- c.presenceFailures
	ch <- c.modelInfo
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
	ch <- prometheus.MustNewConstMetric(c.realtimeWebSockets, prometheus.GaugeValue, float64(c.server.metrics.realtimeWebSocketConnectionCount()))
	ch <- prometheus.MustNewConstMetric(c.realtimeCatchUps, prometheus.GaugeValue, float64(c.server.metrics.realtimeCatchUps.Load()))
	ch <- prometheus.MustNewConstMetric(c.realtimeCatchUpsStarted, prometheus.CounterValue, float64(c.server.metrics.realtimeCatchUpsStarted.Load()))
	ch <- prometheus.MustNewConstMetric(c.realtimeCatchUpsTimedOut, prometheus.CounterValue, float64(c.server.metrics.realtimeCatchUpsTimedOut.Load()))
	ch <- prometheus.MustNewConstMetric(c.realtimeCatchUpsRejected, prometheus.CounterValue, float64(c.server.metrics.realtimeCatchUpsRateLimited.Load()), "rate_limited")
	ch <- prometheus.MustNewConstMetric(c.realtimeCatchUpsRejected, prometheus.CounterValue, float64(c.server.metrics.realtimeCatchUpsUserBusy.Load()), "user_busy")
	ch <- prometheus.MustNewConstMetric(c.realtimeCatchUpsRejected, prometheus.CounterValue, float64(c.server.metrics.realtimeCatchUpsServerBusy.Load()), "server_busy")

	c.collectNATSMetrics(ch)
	c.collectCoreMetrics(ch)
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
	for _, modelKey := range core.ModelKeys() {
		ch <- prometheus.MustNewConstMetric(c.modelInfo, prometheus.GaugeValue, 1, modelKey)
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
