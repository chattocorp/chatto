package core

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/lease"
	"hmans.de/chatto/internal/projectionsnapshot"
	"hmans.de/chatto/internal/testutil"
)

type fakeProjectionSnapshotSweeper struct {
	mu      sync.Mutex
	results []projectionsnapshot.SweepResult
	errors  []error
	opts    []projectionsnapshot.SweepOptions
	called  chan struct{}
	sweep   func(context.Context, projectionsnapshot.SweepOptions) (projectionsnapshot.SweepResult, error)
}

func (*fakeProjectionSnapshotSweeper) Backend() string { return "fake" }

func (f *fakeProjectionSnapshotSweeper) Sweep(ctx context.Context, opts projectionsnapshot.SweepOptions) (projectionsnapshot.SweepResult, error) {
	if f.sweep != nil {
		return f.sweep(ctx, opts)
	}
	f.mu.Lock()
	call := len(f.opts)
	f.opts = append(f.opts, opts)
	var result projectionsnapshot.SweepResult
	if call < len(f.results) {
		result = f.results[call]
	}
	var err error
	if call < len(f.errors) {
		err = f.errors[call]
	}
	f.mu.Unlock()
	if opts.BeforeDelete != nil {
		if ownershipErr := opts.BeforeDelete(ctx); ownershipErr != nil {
			return result, ownershipErr
		}
	}
	if f.called != nil {
		select {
		case f.called <- struct{}{}:
		default:
		}
	}
	return result, err
}

func (f *fakeProjectionSnapshotSweeper) options() []projectionsnapshot.SweepOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.opts)
}

type fakeProjectionSnapshotCleanupLease struct {
	mu          sync.Mutex
	runs        int
	checks      int
	checkErr    error
	runEntered  chan struct{}
	workStarted chan struct{}
}

func (f *fakeProjectionSnapshotCleanupLease) Run(ctx context.Context, work func(context.Context) error) error {
	f.mu.Lock()
	f.runs++
	f.mu.Unlock()
	if f.runEntered != nil {
		close(f.runEntered)
	}
	if f.workStarted != nil {
		close(f.workStarted)
	}
	return work(ctx)
}

func (f *fakeProjectionSnapshotCleanupLease) CheckOwnership(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks++
	return f.checkErr
}

func (f *fakeProjectionSnapshotCleanupLease) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runs, f.checks
}

func closedCleanupBoot() <-chan struct{} {
	boot := make(chan struct{})
	close(boot)
	return boot
}

func TestProjectionSnapshotCleanupWorkerUsesSuccessCadenceAndSafetyOptions(t *testing.T) {
	repository := &fakeProjectionSnapshotSweeper{results: []projectionsnapshot.SweepResult{{DeletedObjects: 2, DeletedBytes: 42}}}
	cleanupLease := &fakeProjectionSnapshotCleanupLease{}
	var waits []time.Duration
	worker := &projectionSnapshotCleanupWorker{
		repository:   repository,
		lease:        cleanupLease,
		logger:       testCoreLogger(),
		initialDelay: func() time.Duration { return 7 * time.Minute },
		wait: func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			if len(waits) == 1 {
				return nil
			}
			return context.Canceled
		},
	}

	err := worker.Run(context.Background(), closedCleanupBoot())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
	if !slices.Equal(waits, []time.Duration{7 * time.Minute, projectionSnapshotCleanupInterval}) {
		t.Fatalf("waits = %v", waits)
	}
	opts := repository.options()
	if len(opts) != 1 {
		t.Fatalf("sweep calls = %d", len(opts))
	}
	if opts[0].GracePeriod != projectionSnapshotCleanupGracePeriod || opts[0].MaxDeletes != projectionSnapshotCleanupMaxDeletes || opts[0].MaxDeleteBytes != projectionSnapshotCleanupMaxDeleteBytes {
		t.Fatalf("sweep options = %#v", opts[0])
	}
	if runs, checks := cleanupLease.counts(); runs != 1 || checks != 1 {
		t.Fatalf("lease runs/checks = %d/%d", runs, checks)
	}
}

func TestProjectionSnapshotCleanupWorkerRetriesFailureOnBackoff(t *testing.T) {
	repository := &fakeProjectionSnapshotSweeper{errors: []error{errors.New("storage unavailable")}}
	cleanupLease := &fakeProjectionSnapshotCleanupLease{}
	var waits []time.Duration
	worker := &projectionSnapshotCleanupWorker{
		repository:   repository,
		lease:        cleanupLease,
		logger:       testCoreLogger(),
		initialDelay: func() time.Duration { return projectionSnapshotCleanupInitialMin },
		wait: func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			if len(waits) == 1 {
				return nil
			}
			return context.Canceled
		},
	}

	if err := worker.Run(context.Background(), closedCleanupBoot()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
	if !slices.Equal(waits, []time.Duration{projectionSnapshotCleanupInitialMin, projectionSnapshotCleanupFailureInterval}) {
		t.Fatalf("waits = %v", waits)
	}
}

func TestProjectionSnapshotCleanupWorkerUsesCatchUpCadenceWhenLimitIsHit(t *testing.T) {
	repository := &fakeProjectionSnapshotSweeper{results: []projectionsnapshot.SweepResult{{DeleteLimitHit: true}}}
	var waits []time.Duration
	worker := &projectionSnapshotCleanupWorker{
		repository:   repository,
		lease:        &fakeProjectionSnapshotCleanupLease{},
		logger:       testCoreLogger(),
		initialDelay: func() time.Duration { return projectionSnapshotCleanupInitialMin },
		wait: func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			if len(waits) == 1 {
				return nil
			}
			return context.Canceled
		},
	}

	if err := worker.Run(context.Background(), closedCleanupBoot()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
	if !slices.Equal(waits, []time.Duration{projectionSnapshotCleanupInitialMin, projectionSnapshotCleanupFailureInterval}) {
		t.Fatalf("waits = %v", waits)
	}
}

func TestProjectionSnapshotCleanupWorkerSharesDeleteBudgetAcrossNamespaces(t *testing.T) {
	first := &fakeProjectionSnapshotSweeper{results: []projectionsnapshot.SweepResult{{DeletedObjects: 60, DeletedBytes: 600}}}
	second := &fakeProjectionSnapshotSweeper{results: []projectionsnapshot.SweepResult{{DeletedObjects: 40, DeletedBytes: 400}}}
	worker := &projectionSnapshotCleanupWorker{
		repositories: []projectionSnapshotSweeper{first, second},
		lease:        &fakeProjectionSnapshotCleanupLease{},
		sweepTimeout: time.Second,
	}
	result, err := worker.sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedObjects != projectionSnapshotCleanupMaxDeletes || result.DeletedBytes != 1000 {
		t.Fatalf("combined result = %#v", result)
	}
	firstOpts, secondOpts := first.options(), second.options()
	if len(firstOpts) != 1 || firstOpts[0].MaxDeletes != projectionSnapshotCleanupMaxDeletes {
		t.Fatalf("first options = %#v", firstOpts)
	}
	if len(secondOpts) != 1 || secondOpts[0].MaxDeletes != 40 || secondOpts[0].MaxDeleteBytes != projectionSnapshotCleanupMaxDeleteBytes-600 {
		t.Fatalf("second options = %#v", secondOpts)
	}
}

func TestProjectionSnapshotCleanupWorkerDoesNotAcquireBeforeBoot(t *testing.T) {
	repository := &fakeProjectionSnapshotSweeper{}
	cleanupLease := &fakeProjectionSnapshotCleanupLease{runEntered: make(chan struct{})}
	worker := &projectionSnapshotCleanupWorker{
		repository:   repository,
		lease:        cleanupLease,
		logger:       testCoreLogger(),
		initialDelay: func() time.Duration { return 0 },
		wait:         func(ctx context.Context, _ time.Duration) error { <-ctx.Done(); return ctx.Err() },
	}
	boot := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx, boot) }()

	select {
	case <-cleanupLease.runEntered:
		t.Fatal("cleanup lease acquired before boot")
	default:
	}
	close(boot)
	select {
	case <-cleanupLease.runEntered:
	case <-time.After(time.Second):
		t.Fatal("cleanup lease was not acquired after boot")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup worker ignored cancellation")
	}
}

func TestProjectionSnapshotCleanupWorkerStopsWhenOwnershipIsLost(t *testing.T) {
	repository := &fakeProjectionSnapshotSweeper{}
	cleanupLease := &fakeProjectionSnapshotCleanupLease{checkErr: errors.New("lease lost")}
	var waits int
	worker := &projectionSnapshotCleanupWorker{
		repository:   repository,
		lease:        cleanupLease,
		logger:       testCoreLogger(),
		initialDelay: func() time.Duration { return 0 },
		wait: func(_ context.Context, _ time.Duration) error {
			waits++
			if waits == 1 {
				return nil
			}
			return context.Canceled
		},
	}

	if err := worker.Run(context.Background(), closedCleanupBoot()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
	if waits != 2 {
		t.Fatalf("wait calls = %d", waits)
	}
	if runs, checks := cleanupLease.counts(); runs != 1 || checks != 1 {
		t.Fatalf("lease runs/checks = %d/%d", runs, checks)
	}
}

func TestProjectionSnapshotCleanupInitialDelayIsWithinWindow(t *testing.T) {
	for range 1000 {
		delay := projectionSnapshotCleanupInitialDelay()
		if delay < projectionSnapshotCleanupInitialMin || delay > projectionSnapshotCleanupInitialMin+projectionSnapshotCleanupInitialJitter {
			t.Fatalf("initial delay outside window: %s", delay)
		}
	}
}

func TestProjectionSnapshotCleanupWorkerBoundsSweepDuration(t *testing.T) {
	repository := &fakeProjectionSnapshotSweeper{
		sweep: func(ctx context.Context, _ projectionsnapshot.SweepOptions) (projectionsnapshot.SweepResult, error) {
			<-ctx.Done()
			return projectionsnapshot.SweepResult{}, ctx.Err()
		},
	}
	worker := &projectionSnapshotCleanupWorker{
		repository:   repository,
		lease:        &fakeProjectionSnapshotCleanupLease{},
		logger:       testCoreLogger(),
		sweepTimeout: 10 * time.Millisecond,
	}

	started := time.Now()
	_, err := worker.sweep(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("sweep error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timed-out sweep took %s", elapsed)
	}
}

func TestProjectionSnapshotCleanupWorkerElectsOneReplicaAndHandsOff(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "SNAPSHOT_CLEANUP_LEASE_TEST", Storage: jetstream.MemoryStorage, History: 1, LimitMarkerTTL: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	newCleanupLease := func(owner string) *lease.Lease {
		cleanupLease, err := lease.New(js, kv, lease.Options{
			Name: "snapshot-cleanup-test", Bucket: "SNAPSHOT_CLEANUP_LEASE_TEST", OwnerID: owner,
			TTL: 2 * time.Second, RenewEvery: 200 * time.Millisecond, RetryEvery: 20 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		return cleanupLease
	}
	blockingWait := func(ctx context.Context, delay time.Duration) error {
		if delay == 0 {
			return nil
		}
		<-ctx.Done()
		return ctx.Err()
	}
	firstRepository := &fakeProjectionSnapshotSweeper{called: make(chan struct{}, 1)}
	secondRepository := &fakeProjectionSnapshotSweeper{called: make(chan struct{}, 1)}
	first := &projectionSnapshotCleanupWorker{
		repository: firstRepository, lease: newCleanupLease("first"),
		logger: testCoreLogger(), initialDelay: func() time.Duration { return 0 }, wait: blockingWait,
	}
	second := &projectionSnapshotCleanupWorker{
		repository: secondRepository, lease: newCleanupLease("second"),
		logger: testCoreLogger(), initialDelay: func() time.Duration { return 0 }, wait: blockingWait,
	}
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() { firstDone <- first.Run(firstCtx, closedCleanupBoot()) }()
	select {
	case <-firstRepository.called:
	case <-time.After(time.Second):
		cancelFirst()
		t.Fatal("first cleanup worker did not acquire lease")
	}

	secondCtx, cancelSecond := context.WithCancel(context.Background())
	defer cancelSecond()
	secondDone := make(chan error, 1)
	go func() { secondDone <- second.Run(secondCtx, closedCleanupBoot()) }()
	select {
	case <-secondRepository.called:
		cancelFirst()
		t.Fatal("second replica swept while first held cleanup lease")
	case <-time.After(150 * time.Millisecond):
	}

	cancelFirst()
	select {
	case err := <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first Run error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first cleanup worker did not release lease")
	}
	select {
	case <-secondRepository.called:
	case <-time.After(time.Second):
		t.Fatal("second cleanup worker did not take over lease")
	}
	cancelSecond()
	select {
	case err := <-secondDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("second Run error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second cleanup worker did not stop")
	}
}
