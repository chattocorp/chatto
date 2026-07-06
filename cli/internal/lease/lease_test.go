package lease

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/testutil"
)

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
