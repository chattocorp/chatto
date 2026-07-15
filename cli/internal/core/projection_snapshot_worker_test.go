package core

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/lease"
	"hmans.de/chatto/internal/projectionsnapshot"
	"hmans.de/chatto/internal/testutil"
)

type fakeSnapshotWorkerLease struct {
	runs   atomic.Int32
	checks atomic.Int32
}

func (f *fakeSnapshotWorkerLease) Run(ctx context.Context, work func(context.Context) error) error {
	f.runs.Add(1)
	return work(ctx)
}

func (f *fakeSnapshotWorkerLease) CheckOwnership(context.Context) error {
	f.checks.Add(1)
	return nil
}

type fakeSnapshotExpirer struct {
	mu      sync.Mutex
	options []projectionsnapshot.ExpireOptions
	results []projectionsnapshot.ExpireResult
	errors  []error
}

func (*fakeSnapshotExpirer) Backend() string { return "s3" }

func (f *fakeSnapshotExpirer) Expire(_ context.Context, options projectionsnapshot.ExpireOptions) (projectionsnapshot.ExpireResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := len(f.options)
	f.options = append(f.options, options)
	var result projectionsnapshot.ExpireResult
	if call < len(f.results) {
		result = f.results[call]
	}
	var err error
	if call < len(f.errors) {
		err = f.errors[call]
	}
	return result, err
}

func (f *fakeSnapshotExpirer) calls() []projectionsnapshot.ExpireOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.options)
}

func TestProjectionSnapshotWorkerChecksImmediatelyThenHourlyWithDailyS3Expiry(t *testing.T) {
	lease := &fakeSnapshotWorkerLease{}
	expirer := &fakeSnapshotExpirer{}
	var waits []time.Duration
	worker := &projectionSnapshotWorker{
		lease: lease, expirer: expirer, retention: 9 * 24 * time.Hour,
		logger: testCoreLogger(), done: make(chan struct{}),
		nextInterval: func() time.Duration { return time.Hour },
		wait: func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			if len(waits) == 1 {
				return nil
			}
			return context.Canceled
		},
	}
	boot := make(chan struct{})
	close(boot)
	if err := worker.Run(context.Background(), boot); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
	if lease.runs.Load() != 1 {
		t.Fatalf("lease runs = %d", lease.runs.Load())
	}
	if len(waits) != 2 || waits[0] > time.Hour || waits[0] < 59*time.Minute {
		t.Fatalf("refresh checks = %v", waits)
	}
	calls := expirer.calls()
	if len(calls) != 1 {
		t.Fatalf("expiry calls = %d", len(calls))
	}
	for _, options := range calls {
		if options.Retention != 9*24*time.Hour || options.MaxDeletes != projectionSnapshotExpiryMaxDeletes || options.MaxDeleteBytes != projectionSnapshotExpiryMaxBytes {
			t.Fatalf("expiry options = %#v", options)
		}
	}
	select {
	case <-worker.done:
	default:
		t.Fatal("first-pass signal was not closed")
	}
}

func TestProjectionSnapshotWorkerExpiryFailureDoesNotStopLaterExpiry(t *testing.T) {
	expirer := &fakeSnapshotExpirer{errors: []error{errors.New("S3 unavailable")}}
	waits := 0
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	worker := &projectionSnapshotWorker{
		lease: &fakeSnapshotWorkerLease{}, expirer: expirer, retention: 7 * 24 * time.Hour,
		logger: testCoreLogger(), nextInterval: func() time.Duration { return time.Hour }, expiryInterval: time.Hour,
		now: func() time.Time { return now },
		wait: func(_ context.Context, _ time.Duration) error {
			waits++
			now = now.Add(time.Hour)
			if waits == 1 {
				return nil
			}
			return context.Canceled
		},
	}
	boot := make(chan struct{})
	close(boot)
	if err := worker.Run(context.Background(), boot); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
	if len(expirer.calls()) != 2 {
		t.Fatalf("expiry failure stopped later pass: calls=%d", len(expirer.calls()))
	}
}

func TestProjectionSnapshotRefreshDue(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		status events.ProjectorStatus
		want   bool
	}{
		{name: "cold replay", status: events.ProjectorStatus{LastSeq: 10}, want: true},
		{name: "fresh unchanged restore", status: events.ProjectorStatus{LastSeq: 10, LatestSnapshotSeq: 10, LatestSnapshotAt: now.Add(-time.Hour)}},
		{name: "stale unchanged restore", status: events.ProjectorStatus{LastSeq: 10, LatestSnapshotSeq: 10, LatestSnapshotAt: now.Add(-projectionSnapshotRefreshAge)}, want: true},
		{name: "fresh restore with boot delta", status: events.ProjectorStatus{LastSeq: 11, LatestSnapshotSeq: 10, LatestSnapshotAt: now.Add(-time.Hour)}, want: true},
		{name: "future timestamp beyond tolerance", status: events.ProjectorStatus{LastSeq: 10, LatestSnapshotSeq: 10, LatestSnapshotAt: now.Add(6 * time.Minute)}, want: true},
		{name: "clock skew within tolerance", status: events.ProjectorStatus{LastSeq: 10, LatestSnapshotSeq: 10, LatestSnapshotAt: now.Add(time.Minute)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := projectionSnapshotRefreshDue(test.status, now, true); got != test.want {
				t.Fatalf("refresh due = %t, want %t", got, test.want)
			}
		})
	}
	status := events.ProjectorStatus{LastSeq: 11, LatestSnapshotSeq: 10, LatestSnapshotAt: now.Add(-time.Hour)}
	if projectionSnapshotRefreshDue(status, now, false) {
		t.Fatal("fresh live delta triggered maintenance publication")
	}
}

func TestProjectionSnapshotWorkerDoesNotAcquireLeaseBeforeBoot(t *testing.T) {
	lease := &fakeSnapshotWorkerLease{}
	worker := &projectionSnapshotWorker{lease: lease, logger: testCoreLogger()}
	boot := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx, boot) }()
	time.Sleep(20 * time.Millisecond)
	if lease.runs.Load() != 0 {
		t.Fatal("snapshot lease acquired before boot")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
}

func TestProjectionSnapshotWorkersUseOneReplicaForDailyPass(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	kv, err := js.CreateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket: "SNAPSHOT_WORKER_LEASE_TEST", Storage: jetstream.MemoryStorage,
		History: 1, LimitMarkerTTL: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	newLease := func(owner string) *lease.Lease {
		result, err := lease.New(js, kv, lease.Options{
			Name: "snapshot-worker-test", OwnerID: owner, Bucket: "SNAPSHOT_WORKER_LEASE_TEST",
			TTL: 2 * time.Second, RenewEvery: 200 * time.Millisecond, RetryEvery: 10 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	firstExpirer := &fakeSnapshotExpirer{}
	secondExpirer := &fakeSnapshotExpirer{}
	workers := []*projectionSnapshotWorker{
		{lease: newLease("owner-one"), expirer: firstExpirer, retention: 7 * 24 * time.Hour, logger: testCoreLogger()},
		{lease: newLease("owner-two"), expirer: secondExpirer, retention: 7 * 24 * time.Hour, logger: testCoreLogger()},
	}
	boot := make(chan struct{})
	close(boot)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, len(workers))
	go func() { done <- workers[0].Run(ctx, boot) }()
	deadline := time.Now().Add(2 * time.Second)
	for len(firstExpirer.calls()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(firstExpirer.calls()) != 1 {
		cancel()
		t.Fatal("first replica did not acquire the snapshot lease")
	}
	go func() { done <- workers[1].Run(ctx, boot) }()
	time.Sleep(150 * time.Millisecond)
	if len(secondExpirer.calls()) != 0 {
		cancel()
		t.Fatal("second replica ran a pass while the first held the snapshot lease")
	}
	cancel()
	for range workers {
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("worker stopped with %v", err)
		}
	}
}
