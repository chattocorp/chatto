package lease

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/testutil"
)

type captureLeaseLogger struct {
	mu       sync.Mutex
	messages []string
}

func (l *captureLeaseLogger) record(msg interface{}) {
	message, ok := msg.(string)
	if !ok {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, message)
}

func (l *captureLeaseLogger) Debug(msg interface{}, _ ...interface{}) { l.record(msg) }
func (l *captureLeaseLogger) Info(msg interface{}, _ ...interface{})  { l.record(msg) }
func (l *captureLeaseLogger) Warn(msg interface{}, _ ...interface{})  { l.record(msg) }
func (l *captureLeaseLogger) Error(msg interface{}, _ ...interface{}) { l.record(msg) }

func (l *captureLeaseLogger) contains(want string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, message := range l.messages {
		if message == want {
			return true
		}
	}
	return false
}

func setupLeaseTest(t *testing.T) (context.Context, jetstream.JetStream, jetstream.KeyValue) {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:         "MEMORY_CACHE",
		Storage:        jetstream.MemoryStorage,
		History:        1,
		LimitMarkerTTL: 2 * time.Second,
	})
	require.NoError(t, err)
	return ctx, js, kv
}

func newTestLease(t *testing.T, js jetstream.JetStream, kv jetstream.KeyValue, name, owner string) *Lease {
	t.Helper()
	l, err := New(js, kv, Options{
		Name:       name,
		OwnerID:    owner,
		Bucket:     "MEMORY_CACHE",
		TTL:        2 * time.Second,
		RenewEvery: 200 * time.Millisecond,
		RetryEvery: 20 * time.Millisecond,
	})
	require.NoError(t, err)
	return l
}

func TestLeaseTryAcquireExcludesOtherOwnersAndReleaseHandsOff(t *testing.T) {
	ctx, js, kv := setupLeaseTest(t)
	first := newTestLease(t, js, kv, "job", "owner-a")
	second := newTestLease(t, js, kv, "job", "owner-b")

	acquired, err := first.TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)

	acquired, err = second.TryAcquire(ctx)
	require.NoError(t, err)
	require.False(t, acquired)

	require.NoError(t, second.Release(ctx))
	entry, err := kv.Get(ctx, first.Key())
	require.NoError(t, err)
	record, err := DecodeRecord(entry.Value())
	require.NoError(t, err)
	require.Equal(t, first.OwnerID(), record.OwnerID)

	require.NoError(t, first.Release(ctx))
	acquired, err = second.TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
}

func TestLeaseTryRunSkipsHeldLeaseWithoutRunningWork(t *testing.T) {
	ctx, js, kv := setupLeaseTest(t)
	first := newTestLease(t, js, kv, "job", "owner-a")
	second := newTestLease(t, js, kv, "job", "owner-b")

	acquired, err := first.TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)

	var ran atomic.Bool
	run, err := second.TryRun(ctx, func(context.Context) error {
		ran.Store(true)
		return nil
	})
	require.NoError(t, err)
	require.False(t, run)
	require.False(t, ran.Load())
	require.NoError(t, first.Release(ctx))
}

func TestLeaseTryRunReleasesAfterWork(t *testing.T) {
	ctx, js, kv := setupLeaseTest(t)
	first := newTestLease(t, js, kv, "job", "owner-a")
	second := newTestLease(t, js, kv, "job", "owner-b")

	run, err := first.TryRun(ctx, func(runCtx context.Context) error {
		return first.CheckOwnership(runCtx)
	})
	require.NoError(t, err)
	require.True(t, run)

	acquired, err := second.TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, second.Release(ctx))
}

func TestLeaseTryRunWithCooldownRetainsSuccessfulClaim(t *testing.T) {
	ctx, js, kv := setupLeaseTest(t)
	first := newTestLease(t, js, kv, "daily-job", "owner-a")
	second := newTestLease(t, js, kv, "daily-job", "owner-b")

	var runs atomic.Int32
	run, err := first.TryRunWithCooldown(ctx, func(context.Context) error {
		runs.Add(1)
		return nil
	})
	require.NoError(t, err)
	require.True(t, run)

	run, err = first.TryRunWithCooldown(ctx, func(context.Context) error {
		runs.Add(1)
		return nil
	})
	require.NoError(t, err)
	require.False(t, run)
	run, err = second.TryRunWithCooldown(ctx, func(context.Context) error {
		runs.Add(1)
		return nil
	})
	require.NoError(t, err)
	require.False(t, run)
	require.Equal(t, int32(1), runs.Load())
	require.NoError(t, first.Release(ctx))
}

func TestLeaseTryRunWithCooldownReleasesFailedClaim(t *testing.T) {
	ctx, js, kv := setupLeaseTest(t)
	first := newTestLease(t, js, kv, "daily-job", "owner-a")
	second := newTestLease(t, js, kv, "daily-job", "owner-b")
	wantErr := errors.New("work failed")

	run, err := first.TryRunWithCooldown(ctx, func(context.Context) error { return wantErr })
	require.True(t, run)
	require.ErrorIs(t, err, wantErr)

	run, err = second.TryRunWithCooldown(ctx, func(context.Context) error { return nil })
	require.NoError(t, err)
	require.True(t, run)
	require.NoError(t, second.Release(ctx))
}

func TestLeaseTryRunWithCooldownAllowsWorkAfterTTL(t *testing.T) {
	ctx, js, kv := setupLeaseTest(t)
	newCooldown := func(owner string) *Lease {
		result, err := New(js, kv, Options{
			Name: "short-cooldown", OwnerID: owner, Bucket: "MEMORY_CACHE",
			TTL: time.Second, RenewEvery: 100 * time.Millisecond,
		})
		require.NoError(t, err)
		return result
	}
	first := newCooldown("owner-a")
	second := newCooldown("owner-b")
	run, err := first.TryRunWithCooldown(ctx, func(context.Context) error { return nil })
	require.NoError(t, err)
	require.True(t, run)

	require.Eventually(t, func() bool {
		run, err = second.TryRunWithCooldown(ctx, func(context.Context) error { return nil })
		return err == nil && run
	}, 3*time.Second, 50*time.Millisecond)
	require.NoError(t, second.Release(ctx))
}

func TestLeaseRenewRefreshesOwnedRecord(t *testing.T) {
	ctx, js, kv := setupLeaseTest(t)
	l := newTestLease(t, js, kv, "job", "owner-a")

	acquired, err := l.TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	beforeEntry, err := kv.Get(ctx, l.Key())
	require.NoError(t, err)
	before, err := DecodeRecord(beforeEntry.Value())
	require.NoError(t, err)

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, l.Renew(ctx))

	afterEntry, err := kv.Get(ctx, l.Key())
	require.NoError(t, err)
	after, err := DecodeRecord(afterEntry.Value())
	require.NoError(t, err)
	require.Equal(t, before.AcquiredAt, after.AcquiredAt)
	require.True(t, after.RenewedAt.After(before.RenewedAt))
	require.True(t, after.ExpiresAt.After(before.ExpiresAt))
}

func TestLeaseRenewDoesNotLogRoutineSuccess(t *testing.T) {
	ctx, js, kv := setupLeaseTest(t)
	logger := &captureLeaseLogger{}
	l, err := New(js, kv, Options{
		Name: "quiet-renewal", OwnerID: "owner-a", Bucket: "MEMORY_CACHE",
		TTL: 2 * time.Second, RenewEvery: 200 * time.Millisecond, RetryEvery: 20 * time.Millisecond,
		Logger: logger,
	})
	require.NoError(t, err)

	acquired, err := l.TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, l.Renew(ctx))
	require.False(t, logger.contains("lease renewed"))
	require.True(t, logger.contains("lease acquired"))
	require.NoError(t, l.Release(ctx))
}

func TestLeaseRenewFailsAfterAnotherOwnerTakesOver(t *testing.T) {
	ctx, js, kv := setupLeaseTest(t)
	first := newTestLease(t, js, kv, "job", "owner-a")
	second := newTestLease(t, js, kv, "job", "owner-b")

	acquired, err := first.TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, first.Release(ctx))
	acquired, err = second.TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)

	require.ErrorIs(t, first.Renew(ctx), ErrLost)
	require.ErrorIs(t, first.CheckOwnership(ctx), ErrLost)
	require.NoError(t, second.CheckOwnership(ctx))
}

func TestLeaseRunAllowsOneLeaderAndThenHandsOff(t *testing.T) {
	_, js, kv := setupLeaseTest(t)
	first := newTestLease(t, js, kv, "job", "owner-a")
	second := newTestLease(t, js, kv, "job", "owner-b")
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	t.Cleanup(cancelSecond)

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)

	go func() {
		firstDone <- first.Run(firstCtx, func(ctx context.Context) error {
			close(firstStarted)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	require.Eventually(t, func() bool {
		select {
		case <-firstStarted:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	go func() {
		secondDone <- second.Run(secondCtx, func(ctx context.Context) error {
			close(secondStarted)
			<-ctx.Done()
			return ctx.Err()
		})
	}()

	select {
	case <-secondStarted:
		t.Fatal("second lease owner started while first owner held the lease")
	case <-time.After(75 * time.Millisecond):
	}

	cancelFirst()
	require.Eventually(t, func() bool {
		select {
		case <-secondStarted:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	require.ErrorIs(t, <-firstDone, context.Canceled)
	cancelSecond()
	require.ErrorIs(t, <-secondDone, context.Canceled)
}

func TestLeaseRunContinuesAfterYield(t *testing.T) {
	_, js, kv := setupLeaseTest(t)
	l := newTestLease(t, js, kv, "job", "owner-a")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var attempts atomic.Int32
	attemptCh := make(chan int32, 2)
	done := make(chan error, 1)

	go func() {
		done <- l.Run(ctx, func(ctx context.Context) error {
			attempt := attempts.Add(1)
			attemptCh <- attempt
			if attempt == 1 {
				return ErrYield
			}
			cancel()
			return context.Canceled
		})
	}()

	require.Equal(t, int32(1), <-attemptCh)
	require.Eventually(t, func() bool {
		return attempts.Load() >= 2
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, int32(2), <-attemptCh)
	require.ErrorIs(t, <-done, context.Canceled)
}
