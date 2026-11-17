package bleve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/chattocorp/chatto/cli/internal/config"
)

// Health is the process-local state of a standalone search provider. The
// unit fills it while it starts; the health listener reads it. Handlers read
// only atomics and small, separately locked copies, never the projection
// lock, so they answer during a blocked commit and during a rebuild.
//
// A nil *Health is valid and ignores updates.
type Health struct {
	mu         sync.Mutex
	nc         *nats.Conn
	provider   *Provider
	projection *Projection
}

// NewHealth returns empty health state for one standalone provider process.
func NewHealth() *Health { return &Health{} }

func (h *Health) setNATS(nc *nats.Conn) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nc = nc
}

func (h *Health) setProvider(provider *Provider, projection *Projection) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.provider = provider
	h.projection = projection
}

func (h *Health) state() (*nats.Conn, *Provider, *Projection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.nc, h.provider, h.projection
}

// Live reports process health. It stays true while the provider replays or
// rebuilds its index and while index work blocks, so that Kubernetes keeps
// the process for diagnostics. It is false only when the NATS connection is
// permanently closed.
func (h *Health) Live() bool {
	nc, _, _ := h.state()
	return nc == nil || !nc.IsClosed()
}

// Ready reports whether this process serves queries: it is connected to
// NATS and subscribed to the shared query queue. See Unit.serve.
func (h *Health) Ready() bool {
	nc, provider, _ := h.state()
	return nc != nil && nc.IsConnected() && provider != nil && provider.serving.Load()
}

// healthDiagnostics is the JSON body of /debug/search. It holds states,
// sequences, counts, and times, never message data, query terms, or IDs.
type healthDiagnostics struct {
	Live       bool                      `json:"live"`
	Ready      bool                      `json:"ready"`
	Serving    bool                      `json:"serving"`
	Projector  *projectorDiagnostics     `json:"projector,omitempty"`
	QueryProbe *queryProbeDiagnostics    `json:"query_probe,omitempty"`
	Index      *IndexDiagnosticsSnapshot `json:"index,omitempty"`
}

type projectorDiagnostics struct {
	Started          bool   `json:"started"`
	StartupComplete  bool   `json:"startup_complete"`
	Failed           bool   `json:"failed"`
	LastSeq          uint64 `json:"last_seq"`
	StartupTargetSeq uint64 `json:"startup_target_seq"`
	StartupMessages  uint64 `json:"startup_messages"`
}

type queryProbeDiagnostics struct {
	LastResult          string     `json:"last_result"`
	LastCompletedAt     *time.Time `json:"last_completed_at,omitempty"`
	LastSucceededAt     *time.Time `json:"last_succeeded_at,omitempty"`
	LastDurationSeconds float64    `json:"last_duration_seconds"`
	ConsecutiveFailures uint32     `json:"consecutive_failures"`
	InFlightSince       *time.Time `json:"in_flight_since,omitempty"`
	Healthy             bool       `json:"healthy"`
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (h *Health) diagnostics(now time.Time) healthDiagnostics {
	_, provider, projection := h.state()
	result := healthDiagnostics{Live: h.Live(), Ready: h.Ready()}
	if projection != nil {
		index := projection.diagnostics.Snapshot(now)
		result.Index = &index
	}
	if provider == nil {
		return result
	}
	result.Serving = provider.serving.Load()
	if projector := provider.projection.Projector(); projector != nil {
		status := projector.Status()
		result.Projector = &projectorDiagnostics{
			Started:          status.Started,
			StartupComplete:  status.StartupComplete,
			Failed:           status.Err != nil,
			LastSeq:          status.LastSeq,
			StartupTargetSeq: status.StartupTargetSeq,
			StartupMessages:  status.StartupMessages,
		}
	}
	if snapshot, started := provider.probe.Snapshot(); started {
		result.QueryProbe = &queryProbeDiagnostics{
			LastResult:          snapshot.LastResult.String(),
			LastCompletedAt:     optionalTime(snapshot.LastCompletedAt),
			LastSucceededAt:     optionalTime(snapshot.LastSucceededAt),
			LastDurationSeconds: snapshot.LastDuration.Seconds(),
			ConsecutiveFailures: snapshot.ConsecutiveFailures,
			InFlightSince:       optionalTime(snapshot.InFlightSince),
			Healthy:             snapshot.Healthy(now),
		}
	}
	return result
}

// Handler returns the health listener routes:
//
//   - /livez: process liveness; see Live.
//   - /readyz: query-serving readiness; see Ready.
//   - /metrics: Prometheus metrics for the process and the query probe.
//   - /debug/pprof/ and /debug/search: Go profiles, goroutine stacks, and
//     index diagnostics, only when diagnostics is true, and only for
//     loopback clients.
func (h *Health) Handler(diagnostics bool) http.Handler {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		newHealthCollector(h),
	)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		writeHealthStatus(w, h.Live(), "ok", "not live")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeHealthStatus(w, h.Ready(), "ready", "not ready")
	})
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	if diagnostics {
		mux.Handle("/debug/pprof/", loopbackOnly(http.HandlerFunc(pprof.Index)))
		mux.Handle("/debug/pprof/cmdline", loopbackOnly(http.HandlerFunc(pprof.Cmdline)))
		mux.Handle("/debug/pprof/profile", loopbackOnly(http.HandlerFunc(pprof.Profile)))
		mux.Handle("/debug/pprof/symbol", loopbackOnly(http.HandlerFunc(pprof.Symbol)))
		mux.Handle("/debug/pprof/trace", loopbackOnly(http.HandlerFunc(pprof.Trace)))
		mux.Handle("GET /debug/search", loopbackOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(h.diagnostics(time.Now()))
		})))
	}
	return mux
}

// writeHealthStatus returns only a fixed status word. Probe callers are
// unauthenticated, so reasons stay in /debug/search.
func writeHealthStatus(w http.ResponseWriter, ok bool, okStatus, failedStatus string) {
	w.Header().Set("Content-Type", "application/json")
	status, code := okStatus, http.StatusOK
	if !ok {
		status, code = failedStatus, http.StatusServiceUnavailable
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": status})
}

// loopbackOnly rejects clients that do not connect from a loopback address.
// Operators reach these routes through kubectl port-forward or a shell in
// the container, also when the listener binds a pod address for probes.
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// HealthServer is a running health listener.
type HealthServer struct {
	server *http.Server
	done   chan error
}

// StartHealthServer binds the health listener from cfg and serves h on it.
func StartHealthServer(cfg config.SearchProviderHealthConfig, h *Health, logger *log.Logger) (*HealthServer, error) {
	address := net.JoinHostPort(cfg.BindAddressOrDefault(), fmt.Sprint(cfg.PortOrDefault()))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen for search provider health on %s: %w", address, err)
	}
	server := &http.Server{
		Handler:           h.Handler(cfg.Diagnostics),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	s := &HealthServer{server: server, done: make(chan error, 1)}
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		s.done <- err
	}()
	logger.Info("Search provider health listener started", "address", listener.Addr().String(), "diagnostics", cfg.Diagnostics)
	return s, nil
}

// Close stops the listener. It waits at most 5 seconds for open requests.
func (s *HealthServer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.server.Shutdown(ctx)
	if err != nil {
		err = errors.Join(err, s.server.Close())
	}
	return errors.Join(err, <-s.done)
}

// healthCollector exports query probe and serving metrics. Labels have
// fixed values only.
type healthCollector struct {
	health              *Health
	serving             *prometheus.Desc
	probeRuns           *prometheus.Desc
	probeInFlight       *prometheus.Desc
	probeLastDuration   *prometheus.Desc
	probeLastSuccess    *prometheus.Desc
	probeConsecutiveErr *prometheus.Desc
	indexWriteAge       *prometheus.Desc
}

func newHealthCollector(h *Health) *healthCollector {
	return &healthCollector{
		health: h,
		serving: prometheus.NewDesc("chatto_search_provider_serving",
			"Whether this search provider is subscribed to the shared query queue.", nil, nil),
		probeRuns: prometheus.NewDesc("chatto_search_query_probe_runs_total",
			"Completed search query probes by result.", []string{"result"}, nil),
		probeInFlight: prometheus.NewDesc("chatto_search_query_probe_in_flight_seconds",
			"Age of the running search query probe, or 0 when no probe runs.", nil, nil),
		probeLastDuration: prometheus.NewDesc("chatto_search_query_probe_last_duration_seconds",
			"Duration of the most recent completed search query probe.", nil, nil),
		probeLastSuccess: prometheus.NewDesc("chatto_search_query_probe_last_success_timestamp_seconds",
			"Unix time of the most recent successful search query probe.", nil, nil),
		probeConsecutiveErr: prometheus.NewDesc("chatto_search_query_probe_consecutive_failures",
			"Completed search query probes that failed or timed out since the last success.", nil, nil),
		indexWriteAge: prometheus.NewDesc("chatto_search_index_write_age_seconds",
			"Age of the running search index write, including its lock wait, or 0 when idle.", nil, nil),
	}
}

func (c *healthCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.serving
	ch <- c.probeRuns
	ch <- c.probeInFlight
	ch <- c.probeLastDuration
	ch <- c.probeLastSuccess
	ch <- c.probeConsecutiveErr
	ch <- c.indexWriteAge
}

func (c *healthCollector) Collect(ch chan<- prometheus.Metric) {
	now := time.Now()
	_, provider, projection := c.health.state()
	if projection != nil {
		ch <- prometheus.MustNewConstMetric(c.indexWriteAge, prometheus.GaugeValue, projection.diagnostics.Snapshot(now).WriteElapsedSeconds)
	}
	if provider == nil {
		ch <- prometheus.MustNewConstMetric(c.serving, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.serving, prometheus.GaugeValue, boolGauge(provider.serving.Load()))
	snapshot, _ := provider.probe.Snapshot()
	ch <- prometheus.MustNewConstMetric(c.probeRuns, prometheus.CounterValue, float64(snapshot.Successes), "succeeded")
	ch <- prometheus.MustNewConstMetric(c.probeRuns, prometheus.CounterValue, float64(snapshot.Failures), "failed")
	ch <- prometheus.MustNewConstMetric(c.probeRuns, prometheus.CounterValue, float64(snapshot.Timeouts), "timed_out")
	var inFlight float64
	if !snapshot.InFlightSince.IsZero() {
		inFlight = now.Sub(snapshot.InFlightSince).Seconds()
	}
	ch <- prometheus.MustNewConstMetric(c.probeInFlight, prometheus.GaugeValue, inFlight)
	ch <- prometheus.MustNewConstMetric(c.probeLastDuration, prometheus.GaugeValue, snapshot.LastDuration.Seconds())
	if !snapshot.LastSucceededAt.IsZero() {
		ch <- prometheus.MustNewConstMetric(c.probeLastSuccess, prometheus.GaugeValue, float64(snapshot.LastSucceededAt.Unix()))
	}
	ch <- prometheus.MustNewConstMetric(c.probeConsecutiveErr, prometheus.GaugeValue, float64(snapshot.ConsecutiveFailures))
}

func boolGauge(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
