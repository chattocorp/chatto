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
	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/lease"
	"hmans.de/chatto/internal/projectionsnapshot"
	"hmans.de/chatto/internal/testutil"
)

type fakeSnapshotWorkerLease struct {
	attempts atomic.Int32
	runs     atomic.Int32
	checks   atomic.Int32
	held     atomic.Bool
}

type scriptedSnapshotWorkerLease struct {
	mu      sync.Mutex
	results []bool
	errors  []error
}

func (f *scriptedSnapshotWorkerLease) TryRun(ctx context.Context, work func(context.Context) error) (bool, error) {
	f.mu.Lock()
	var err error
	if len(f.errors) > 0 {
		err = f.errors[0]
		f.errors = f.errors[1:]
	}
	acquired := len(f.results) > 0 && f.results[0]
	if len(f.results) > 0 {
		f.results = f.results[1:]
	}
	f.mu.Unlock()
	if err != nil {
		return false, err
	}
	if !acquired {
		return false, nil
	}
	return true, work(ctx)
}

func (*scriptedSnapshotWorkerLease) CheckOwnership(context.Context) error { return nil }

func (f *fakeSnapshotWorkerLease) TryRun(ctx context.Context, work func(context.Context) error) (bool, error) {
	f.attempts.Add(1)
	f.runs.Add(1)
	f.held.Store(true)
	defer f.held.Store(false)
	return true, work(ctx)
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

type fakeSnapshotExpiryLease struct {
	mu       sync.Mutex
	complete bool
	attempts int
}

func (f *fakeSnapshotExpiryLease) TryRunWithCooldown(ctx context.Context, work func(context.Context) error) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts++
	if f.complete {
		return false, nil
	}
	if err := work(ctx); err != nil {
		return true, err
	}
	f.complete = true
	return true, nil
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

type blockingSnapshotExpirer struct {
	started chan struct{}
	release chan struct{}
}

func (*blockingSnapshotExpirer) Backend() string { return "s3" }

func (f *blockingSnapshotExpirer) Expire(ctx context.Context, _ projectionsnapshot.ExpireOptions) (projectionsnapshot.ExpireResult, error) {
	close(f.started)
	select {
	case <-f.release:
		return projectionsnapshot.ExpireResult{}, nil
	case <-ctx.Done():
		return projectionsnapshot.ExpireResult{}, ctx.Err()
	}
}

type observedSnapshotWorkerLease struct {
	lease   *lease.Lease
	results chan bool
}

func (l *observedSnapshotWorkerLease) TryRun(ctx context.Context, work func(context.Context) error) (bool, error) {
	acquired, err := l.lease.TryRun(ctx, work)
	l.results <- acquired
	return acquired, err
}

func (l *observedSnapshotWorkerLease) CheckOwnership(ctx context.Context) error {
	return l.lease.CheckOwnership(ctx)
}

func TestProjectionSnapshotWorkerChecksImmediatelyThenHourlyWithDailyS3Expiry(t *testing.T) {
	lease := &fakeSnapshotWorkerLease{}
	expirer := &fakeSnapshotExpirer{}
	var waits []time.Duration
	worker := &projectionSnapshotWorker{
		lease: lease, expiryLease: &fakeSnapshotExpiryLease{}, expirer: expirer, retention: 9 * 24 * time.Hour,
		logger: testCoreLogger(), done: make(chan struct{}),
		nextInterval: func() time.Duration { return time.Hour },
		wait: func(_ context.Context, delay time.Duration) error {
			if lease.held.Load() {
				t.Fatal("snapshot lease remained held during the refresh wait")
			}
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
	if lease.runs.Load() != 2 {
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
	expiryLease := &fakeSnapshotExpiryLease{}
	waits := 0
	worker := &projectionSnapshotWorker{
		lease: &fakeSnapshotWorkerLease{}, expiryLease: expiryLease, expirer: expirer, retention: 7 * 24 * time.Hour,
		logger: testCoreLogger(), nextInterval: func() time.Duration { return time.Hour },
		wait: func(_ context.Context, _ time.Duration) error {
			waits++
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

func TestProjectionSnapshotWorkerRetriesExpiryAfterPublicationLeaseMiss(t *testing.T) {
	for _, test := range []struct {
		name  string
		lease *scriptedSnapshotWorkerLease
	}{
		{name: "held by another replica", lease: &scriptedSnapshotWorkerLease{results: []bool{false, true}}},
		{name: "transient acquisition error", lease: &scriptedSnapshotWorkerLease{
			results: []bool{false, true}, errors: []error{errors.New("NATS unavailable"), nil},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			expirer := &fakeSnapshotExpirer{}
			waits := 0
			worker := &projectionSnapshotWorker{
				lease: test.lease, expiryLease: &fakeSnapshotExpiryLease{}, expirer: expirer,
				retention: 7 * 24 * time.Hour, logger: testCoreLogger(),
				nextInterval: func() time.Duration { return time.Hour },
				wait: func(_ context.Context, _ time.Duration) error {
					waits++
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
			if len(expirer.calls()) != 1 {
				t.Fatalf("expiry calls after lease miss = %d, want 1", len(expirer.calls()))
			}
		})
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
	if lease.attempts.Load() != 0 {
		t.Fatal("snapshot lease acquired before boot")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
}

func TestProjectionSnapshotWorkersDoNotOverlapPassesAndReleaseLease(t *testing.T) {
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

	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	firstExpirer := &blockingSnapshotExpirer{started: firstStarted, release: firstRelease}
	secondExpirer := &fakeSnapshotExpirer{}
	firstExpiryLease := &fakeSnapshotExpiryLease{}
	secondExpiryLease := &fakeSnapshotExpiryLease{}
	firstLease := newLease("owner-one")
	secondLease := newLease("owner-two")
	observedSecondLease := &observedSnapshotWorkerLease{lease: secondLease, results: make(chan bool, 1)}
	workers := []*projectionSnapshotWorker{
		{lease: firstLease, expiryLease: firstExpiryLease, expirer: firstExpirer, retention: 7 * 24 * time.Hour, logger: testCoreLogger()},
		{lease: observedSecondLease, expiryLease: secondExpiryLease, expirer: secondExpirer, retention: 7 * 24 * time.Hour, logger: testCoreLogger()},
	}
	boot := make(chan struct{})
	close(boot)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, len(workers))
	go func() { done <- workers[0].Run(ctx, boot) }()
	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("first replica did not acquire the snapshot lease")
	}
	go func() { done <- workers[1].Run(ctx, boot) }()
	select {
	case acquired := <-observedSecondLease.results:
		if acquired {
			cancel()
			t.Fatal("second replica acquired the snapshot lease concurrently")
		}
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("second replica did not attempt the snapshot lease")
	}
	if len(secondExpirer.calls()) != 0 {
		cancel()
		t.Fatal("second replica ran a pass while the first held the snapshot lease")
	}
	close(firstRelease)
	require.Eventually(t, func() bool {
		acquired, err := secondLease.TryAcquire(context.Background())
		return err == nil && acquired
	}, time.Second, 10*time.Millisecond, "snapshot lease was not released after the pass")
	require.NoError(t, secondLease.Release(context.Background()))
	cancel()
	for range workers {
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("worker stopped with %v", err)
		}
	}
}

func TestProjectionSnapshotWorkersShareS3ExpiryCooldown(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	kv, err := js.CreateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket: "SNAPSHOT_EXPIRY_COOLDOWN_TEST", Storage: jetstream.MemoryStorage,
		History: 1, LimitMarkerTTL: 2 * time.Second,
	})
	require.NoError(t, err)
	newLease := func(name, owner string) *lease.Lease {
		result, err := lease.New(js, kv, lease.Options{
			Name: name, OwnerID: owner, Bucket: "SNAPSHOT_EXPIRY_COOLDOWN_TEST",
			TTL: 2 * time.Second, RenewEvery: 200 * time.Millisecond,
		})
		require.NoError(t, err)
		return result
	}

	firstExpirer := &fakeSnapshotExpirer{}
	secondExpirer := &fakeSnapshotExpirer{}
	workers := []*projectionSnapshotWorker{
		{
			lease:       newLease("snapshot-pass", "pass-owner-one"),
			expiryLease: newLease("snapshot-expiry", "expiry-owner-one"),
			expirer:     firstExpirer, retention: 7 * 24 * time.Hour, logger: testCoreLogger(),
			wait: func(context.Context, time.Duration) error { return context.Canceled },
		},
		{
			lease:       newLease("snapshot-pass", "pass-owner-two"),
			expiryLease: newLease("snapshot-expiry", "expiry-owner-two"),
			expirer:     secondExpirer, retention: 7 * 24 * time.Hour, logger: testCoreLogger(),
			wait: func(context.Context, time.Duration) error { return context.Canceled },
		},
	}
	boot := make(chan struct{})
	close(boot)
	for _, worker := range workers {
		require.ErrorIs(t, worker.Run(context.Background(), boot), context.Canceled)
	}
	require.Len(t, firstExpirer.calls(), 1)
	require.Empty(t, secondExpirer.calls())
}
