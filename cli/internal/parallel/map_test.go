package parallel

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestMapPreservesOrderAndLimit(t *testing.T) {
	var inFlight int32
	var maxInFlight int32

	got, err := Map(context.Background(), 2, []int{1, 2, 3, 4}, func(ctx context.Context, _ int, n int) (int, error) {
		current := atomic.AddInt32(&inFlight, 1)
		for {
			previous := atomic.LoadInt32(&maxInFlight)
			if current <= previous || atomic.CompareAndSwapInt32(&maxInFlight, previous, current) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
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
