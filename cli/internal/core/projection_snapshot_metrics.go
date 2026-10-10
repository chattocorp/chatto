package core

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"hmans.de/chatto/internal/projectionsnapshot"
	"hmans.de/chatto/pkg/events"
)

// snapshotRestoreObservation records repository selection separately from the
// projector's final restore decision. A successful load can still be rejected.
type snapshotRestoreObservation struct {
	mu             sync.Mutex
	source, reason string
}

func (o *snapshotRestoreObservation) record(source, reason string, err error) {
	if o == nil {
		return
	}
	if err != nil {
		source, reason = "cold", projectionsnapshot.RestoreFailureReason(err)
	}
	o.mu.Lock()
	o.source, o.reason = source, reason
	o.mu.Unlock()
}

func snapshotOutcomeFromOptions(opts events.ProjectorOptions) *snapshotRestoreObservation {
	if opts.Snapshots == nil {
		return nil
	}
	switch source := opts.Snapshots.Source.(type) {
	case projectionSnapshotSource:
		return source.outcome
	case projectionSnapshotCohortSource:
		return source.outcome
	}
	return nil
}

// snapshotOperationMetrics holds process-local completed work. A skipped pass
// is not an operation. Timestamps refer only to this replica's successes.
type snapshotOperationMetrics struct {
	mu           sync.Mutex
	publications map[string]*snapshotPublicationMetrics
	duration     *prometheus.HistogramVec
	cleanup      snapshotCleanupMetrics
}

type snapshotPublicationMetrics struct {
	projection, backend string
	success, failures   uint64
	lastSuccess         float64
	duration            prometheus.Observer
}

type snapshotCleanupMetrics struct {
	backend                      string
	success, failures            uint64
	deletedObjects, deletedBytes uint64
	lastSuccess                  float64
}

func (m *snapshotOperationMetrics) publication(projection, backend string, elapsed time.Duration, at time.Time, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.publications == nil {
		m.publications = make(map[string]*snapshotPublicationMetrics)
	}
	p := m.publications[projection]
	if p == nil {
		if m.duration == nil {
			m.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Name:    "chatto_projection_snapshot_publication_duration_seconds",
				Help:    "Seconds spent capturing, encoding, and publishing a snapshot, including failed attempts; excludes skips.",
				Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 15, 30, 60, 120},
			}, []string{"projection", "backend"})
		}
		p = &snapshotPublicationMetrics{projection: projection, backend: backend, duration: m.duration.WithLabelValues(projection, backend)}
		m.publications[projection] = p
	}
	p.duration.Observe(elapsed.Seconds())
	if err != nil {
		p.failures++
	} else {
		p.success++
		p.lastSuccess = float64(at.UnixNano()) / 1e9
	}
}

func (m *snapshotOperationMetrics) expiry(backend string, result projectionsnapshot.ExpireResult, at time.Time, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanup.backend = backend
	m.cleanup.deletedObjects += uint64(result.DeletedObjects)
	m.cleanup.deletedBytes += uint64(result.DeletedBytes)
	if err != nil {
		m.cleanup.failures++
	} else {
		m.cleanup.success++
		m.cleanup.lastSuccess = float64(at.UnixNano()) / 1e9
	}
}

// ProjectionSnapshotMetrics returns a collector for local snapshot operations.
// Collection reads memory only and remains independent of readiness and storage.
// Register once per process registry. No snapshot payload or locator is exposed.
func (c *ChattoCore) ProjectionSnapshotMetrics() prometheus.Collector {
	return &projectionSnapshotCollector{core: c}
}

type projectionSnapshotCollector struct{ core *ChattoCore }

var (
	snapshotRestoreDesc         = prometheus.NewDesc("chatto_projection_snapshot_restore_info", "Completed startup snapshot source and reason in this process; value is 1.", []string{"projection", "source", "reason"}, nil)
	snapshotPublicationsDesc    = prometheus.NewDesc("chatto_projection_snapshot_publications_total", "Completed snapshot publication attempts in this process, excluding skips.", []string{"projection", "backend", "result"}, nil)
	snapshotPublicationTimeDesc = prometheus.NewDesc("chatto_projection_snapshot_last_publication_timestamp_seconds", "Unix time of this process's last successful snapshot publication; absent before success.", []string{"projection", "backend"}, nil)
	snapshotCleanupDesc         = prometheus.NewDesc("chatto_projection_snapshot_cleanup_runs_total", "Completed snapshot cleanup runs in this process.", []string{"backend", "result"}, nil)
	snapshotDeletedObjectsDesc  = prometheus.NewDesc("chatto_projection_snapshot_cleanup_deleted_objects_total", "Snapshot objects deleted by this process, including partial failed cleanup runs.", []string{"backend"}, nil)
	snapshotDeletedBytesDesc    = prometheus.NewDesc("chatto_projection_snapshot_cleanup_deleted_bytes_total", "Snapshot bytes deleted by this process, including partial failed cleanup runs.", []string{"backend"}, nil)
	snapshotCleanupTimeDesc     = prometheus.NewDesc("chatto_projection_snapshot_cleanup_last_success_timestamp_seconds", "Unix time of this process's last successful snapshot cleanup; absent before success.", []string{"backend"}, nil)
	snapshotDurationDesc        = prometheus.NewDesc("chatto_projection_snapshot_publication_duration_seconds", "Seconds spent capturing, encoding, and publishing a snapshot, including failed attempts; excludes skips.", []string{"projection", "backend"}, nil)
)

func (*projectionSnapshotCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{snapshotRestoreDesc, snapshotPublicationsDesc, snapshotPublicationTimeDesc, snapshotCleanupDesc, snapshotDeletedObjectsDesc, snapshotDeletedBytesDesc, snapshotCleanupTimeDesc, snapshotDurationDesc} {
		ch <- d
	}
}

func (m *projectionSnapshotCollector) Collect(ch chan<- prometheus.Metric) {
	for _, registration := range m.core.projections {
		status := registration.projector.Status()
		if !status.StartupComplete {
			continue
		}
		source, reason := "cold", "disabled"
		if registration.snapshotPolicy == sharedSnapshots && m.core.config.ProjectionSnapshots {
			reason = "unavailable"
			if o := registration.snapshotOutcome; o != nil {
				o.mu.Lock()
				if o.source != "" {
					source, reason = o.source, o.reason
				}
				o.mu.Unlock()
			}
			if !status.SnapshotRestored && source != "cold" {
				source, reason = "cold", "rejected"
			}
		}
		ch <- prometheus.MustNewConstMetric(snapshotRestoreDesc, prometheus.GaugeValue, 1, registration.key, source, reason)
	}
	w := m.core.projectionSnapshotWorker
	if w == nil {
		return
	}
	w.metrics.mu.Lock()
	publications := make([]snapshotPublicationMetrics, 0, len(w.metrics.publications))
	for _, p := range w.metrics.publications {
		publications = append(publications, *p)
	}
	cleanup := w.metrics.cleanup
	duration := w.metrics.duration
	w.metrics.mu.Unlock()
	for _, p := range publications {
		ch <- prometheus.MustNewConstMetric(snapshotPublicationsDesc, prometheus.CounterValue, float64(p.success), p.projection, p.backend, "success")
		ch <- prometheus.MustNewConstMetric(snapshotPublicationsDesc, prometheus.CounterValue, float64(p.failures), p.projection, p.backend, "error")
		if p.lastSuccess != 0 {
			ch <- prometheus.MustNewConstMetric(snapshotPublicationTimeDesc, prometheus.GaugeValue, p.lastSuccess, p.projection, p.backend)
		}
	}
	if duration != nil {
		duration.Collect(ch)
	}
	if cleanup.backend == "" {
		return
	}
	ch <- prometheus.MustNewConstMetric(snapshotCleanupDesc, prometheus.CounterValue, float64(cleanup.success), cleanup.backend, "success")
	ch <- prometheus.MustNewConstMetric(snapshotCleanupDesc, prometheus.CounterValue, float64(cleanup.failures), cleanup.backend, "error")
	ch <- prometheus.MustNewConstMetric(snapshotDeletedObjectsDesc, prometheus.CounterValue, float64(cleanup.deletedObjects), cleanup.backend)
	ch <- prometheus.MustNewConstMetric(snapshotDeletedBytesDesc, prometheus.CounterValue, float64(cleanup.deletedBytes), cleanup.backend)
	if cleanup.lastSuccess != 0 {
		ch <- prometheus.MustNewConstMetric(snapshotCleanupTimeDesc, prometheus.GaugeValue, cleanup.lastSuccess, cleanup.backend)
	}
}
