package core

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"hmans.de/chatto/internal/projectionsnapshot"
)

func snapshotMetricsText(t *testing.T, c *ChattoCore) string {
	t.Helper()
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(c.ProjectionSnapshotMetrics())
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for _, family := range families {
		if _, err := expfmt.MetricFamilyToText(&out, family); err != nil {
			t.Fatal(err)
		}
	}
	return out.String()
}

func requireSnapshotMetric(t *testing.T, c *ChattoCore, want string) {
	t.Helper()
	if text := snapshotMetricsText(t, c); !strings.Contains(text, want) {
		t.Fatalf("missing %s in:\n%s", want, text)
	}
}

func TestSnapshotOperationMetricsFailuresAndConcurrency(t *testing.T) {
	c := &ChattoCore{projectionSnapshotWorker: &projectionSnapshotWorker{}}
	m := &c.projectionSnapshotWorker.metrics
	// Failed attempts must not fabricate successful publication timestamps.
	m.publication("threads", "s3", time.Second, time.Unix(100, 0), errors.New("private error"))
	if strings.Contains(snapshotMetricsText(t, c), "last_publication_timestamp_seconds") {
		t.Fatal("failed attempt has success timestamp")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				m.publication("threads", "s3", time.Second, time.Unix(200, 0), nil)
			}
		})
	}
	for range 20 {
		snapshotMetricsText(t, c)
	}
	wg.Wait()
	requireSnapshotMetric(t, c, `chatto_projection_snapshot_publications_total{backend="s3",projection="threads",result="success"} 160`)
	requireSnapshotMetric(t, c, `chatto_projection_snapshot_publications_total{backend="s3",projection="threads",result="error"} 1`)
	requireSnapshotMetric(t, c, `chatto_projection_snapshot_publication_duration_seconds_count{backend="s3",projection="threads"} 161`)
	requireSnapshotMetric(t, c, `chatto_projection_snapshot_last_publication_timestamp_seconds{backend="s3",projection="threads"} 200`)
}

func TestSnapshotCleanupMetricsPreservePartialWork(t *testing.T) {
	expirer := &fakeSnapshotExpirer{errors: []error{nil, errors.New("provider unavailable")}, results: []projectionsnapshot.ExpireResult{
		{DeletedObjects: 2, DeletedBytes: 100}, {DeletedObjects: 1, DeletedBytes: 25},
	}}
	w := &projectionSnapshotWorker{expirer: expirer, logger: testCoreLogger()}
	c := &ChattoCore{projectionSnapshotWorker: w}
	if err := w.expire(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := w.metrics.cleanup.lastSuccess
	if err := w.expire(context.Background()); err == nil {
		t.Fatal("expected expiry error")
	}
	if w.metrics.cleanup.lastSuccess != before {
		t.Fatal("failed cleanup advanced success timestamp")
	}
	requireSnapshotMetric(t, c, `chatto_projection_snapshot_cleanup_runs_total{backend="s3",result="error"} 1`)
	requireSnapshotMetric(t, c, `chatto_projection_snapshot_cleanup_runs_total{backend="s3",result="success"} 1`)
	requireSnapshotMetric(t, c, `chatto_projection_snapshot_cleanup_deleted_objects_total{backend="s3"} 3`)
	requireSnapshotMetric(t, c, `chatto_projection_snapshot_cleanup_deleted_bytes_total{backend="s3"} 125`)
}

func TestSnapshotRestoreMetricsUseFinalProjectorDecision(t *testing.T) {
	c, _ := setupTestCore(t)
	// This core cold replayed. Simulate a repository load that succeeded but
	// whose payload was subsequently rejected by the projector.
	c.config.ProjectionSnapshots = true
	for i := range c.projections {
		registration := &c.projections[i]
		if registration.snapshotPolicy != sharedSnapshots {
			continue
		}
		registration.snapshotOutcome = &snapshotRestoreObservation{source: "current", reason: "none"}
		requireSnapshotMetric(t, c, `chatto_projection_snapshot_restore_info{projection="`+registration.key+`",reason="rejected",source="cold"} 1`)
		registration.snapshotOutcome = nil
		requireSnapshotMetric(t, c, `chatto_projection_snapshot_restore_info{projection="`+registration.key+`",reason="unavailable",source="cold"} 1`)
		break
	}
}

func TestSnapshotPublicationMetricsRecordLeaseLoss(t *testing.T) {
	c, _ := setupTestCore(t)
	// Lease loss after capture prevents storage work but still counts as an
	// attempted generation. Use a real repository to retain its backend label.
	repository, err := projectionsnapshot.NewRepository(natsSnapshotBlobStore{}, projectionsnapshot.RepositoryOptions{
		Pointers:  natsSnapshotPointerStore{kv: c.storage.runtimeStateKV},
		SecretHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
	})
	if err != nil {
		t.Fatal(err)
	}
	w := &projectionSnapshotWorker{logger: testCoreLogger(), lease: &snapshotMetricsLostLease{}}
	job := projectionSnapshotJob{projector: registeredProjector(t, c, "server_content_view"), projectionKey: "server_content_view", repository: repository}
	if err := w.generateJob(context.Background(), job, true); err == nil {
		t.Fatal("publication succeeded after lease loss")
	}
	requireSnapshotMetric(t, &ChattoCore{projectionSnapshotWorker: w}, `chatto_projection_snapshot_publications_total{backend="nats",projection="server_content_view",result="error"} 1`)
}

type snapshotMetricsLostLease struct{ fakeSnapshotWorkerLease }

func (*snapshotMetricsLostLease) CheckOwnership(context.Context) error {
	return errors.New("lease lost")
}
