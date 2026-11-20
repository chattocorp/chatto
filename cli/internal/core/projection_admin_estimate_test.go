package core

import (
	"context"
	"testing"
	"time"
)

// A blocked projection lock must not block projection diagnostics.
func TestBoundedProjectionEstimateDoesNotWaitForBlockedProjection(t *testing.T) {
	c := &ChattoCore{}
	release := make(chan struct{})
	finished := make(chan struct{})
	blocked := func() (int64, int64, []ProjectionAdminMetric) {
		defer close(finished)
		<-release
		return 1, 2, nil
	}

	start := time.Now()
	if _, ok := c.boundedProjectionEstimate(context.Background(), "blocked", blocked); ok {
		t.Fatal("blocked estimate reported as available")
	}
	if elapsed := time.Since(start); elapsed > 5*projectionEstimateTimeout {
		t.Fatalf("blocked estimate waited %s", elapsed)
	}

	// While the first estimate still waits, a later call starts no second one.
	start = time.Now()
	if _, ok := c.boundedProjectionEstimate(context.Background(), "blocked", func() (int64, int64, []ProjectionAdminMetric) {
		t.Error("second estimate started while the first one still runs")
		return 0, 0, nil
	}); ok {
		t.Fatal("estimate in flight reported as available")
	}
	if elapsed := time.Since(start); elapsed > projectionEstimateTimeout/2 {
		t.Fatalf("call with an estimate in flight waited %s", elapsed)
	}

	close(release)
	<-finished
	// The first goroutine removes its key after it returns.
	deadline := time.Now().Add(time.Second)
	for {
		estimate, ok := c.boundedProjectionEstimate(context.Background(), "blocked", func() (int64, int64, []ProjectionAdminMetric) {
			return 3, 4, nil
		})
		if ok {
			if estimate.entries != 3 || estimate.bytes != 4 {
				t.Fatalf("estimate = %#v", estimate)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("estimate stayed unavailable after the projection lock was released")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
