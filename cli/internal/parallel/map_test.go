package parallel

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestMapPreservesOrderAndLimit(t *testing.T) {
	var inFlight atomic.Int32
	var maxInFlight int32

	got, err := Map(context.Background(), 2, []int{1, 2, 3, 4}, func(ctx context.Context, _ int, n int) (int, error) {
		current := inFlight.Add(1)
		for {
			previous := atomic.LoadInt32(&maxInFlight)
			if current <= previous || atomic.CompareAndSwapInt32(&maxInFlight, previous, current) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		inFlight.Add(-1)
		return n * 10, nil
	})
	if err != nil {
		t.Fatalf("Map returned error: %v", err)
	}
	if maxInFlight > 2 {
		t.Fatalf("max in-flight = %d, want <= 2", maxInFlight)
	}
	for i, want := range []int{10, 20, 30, 40} {
		if got[i] != want {
			t.Fatalf("got[%d] = %d, want %d", i, got[i], want)
		}
	}
}

func TestMapReturnsFirstError(t *testing.T) {
	wantErr := errors.New("boom")

	_, err := Map(context.Background(), 2, []int{1, 2, 3}, func(ctx context.Context, _ int, n int) (int, error) {
		if n == 2 {
			return 0, wantErr
		}
		return n, nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Map error = %v, want %v", err, wantErr)
	}
}

func TestMapSkipsPendingItemsAfterError(t *testing.T) {
	wantErr := errors.New("load failed")
	var calls int

	_, err := Map(context.Background(), 1, []int{1, 2, 3}, func(context.Context, int, int) (int, error) {
		calls++
		return 0, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Map error = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Fatalf("loader calls = %d, want 1", calls)
	}
}

func TestMapSkipsItemsWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32

	_, err := Map(ctx, 2, []int{1, 2, 3}, func(context.Context, int, int) (int, error) {
		calls.Add(1)
		return 0, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Map error = %v, want %v", err, context.Canceled)
	}
	if calls.Load() != 0 {
		t.Fatalf("loader calls = %d, want 0", calls.Load())
	}
}

func TestMapSkipsPendingItemsAfterParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls int

	_, err := Map(ctx, 1, []int{1, 2, 3}, func(context.Context, int, int) (int, error) {
		calls++
		cancel()
		return 1, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Map error = %v, want %v", err, context.Canceled)
	}
	if calls != 1 {
		t.Fatalf("loader calls = %d, want 1", calls)
	}
}

func TestMapNonNilDropsNilResultsAndPreservesOrder(t *testing.T) {
	got, err := MapNonNil(context.Background(), 2, []int{1, 2, 3, 4}, func(ctx context.Context, _ int, n int) (*int, error) {
		if n%2 == 0 {
			return nil, nil
		}
		result := n * 10
		return &result, nil
	})
	if err != nil {
		t.Fatalf("MapNonNil returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	for i, want := range []int{10, 30} {
		if *got[i] != want {
			t.Fatalf("got[%d] = %d, want %d", i, *got[i], want)
		}
	}
}
