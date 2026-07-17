package http_server

import (
	"testing"
	"time"
)

func TestRealtimeCatchUpAdmissionBoundsUserAndProcessConcurrency(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(1, 3, time.Minute, func() time.Time { return now })

	releaseFirst, err := admission.acquire("user-1")
	if err != nil {
		t.Fatalf("acquire first catch-up: %v", err)
	}
	if _, err := admission.acquire("user-1"); err == nil || err.code != "catch_up_in_progress" {
		t.Fatalf("concurrent same-user acquire error = %+v, want catch_up_in_progress", err)
	}
	if _, err := admission.acquire("user-2"); err == nil || err.code != "catch_up_server_busy" {
		t.Fatalf("global-capacity acquire error = %+v, want catch_up_server_busy", err)
	}

	releaseFirst()
	releaseFirst() // Release is deliberately safe on all return paths.
	releaseSecond, err := admission.acquire("user-2")
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	releaseSecond()
}

func TestRealtimeCatchUpAdmissionRateLimitsReplayAndBootstrapAttempts(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(2, 2, time.Minute, func() time.Time { return now })

	for attempt := 0; attempt < 2; attempt++ {
		release, err := admission.acquire("user-1")
		if err != nil {
			t.Fatalf("acquire attempt %d: %v", attempt+1, err)
		}
		release()
	}
	if _, err := admission.acquire("user-1"); err == nil || err.code != "catch_up_rate_limited" || err.retryAfter != time.Minute {
		t.Fatalf("rate-limited acquire error = %+v, want one-minute retry", err)
	}

	now = now.Add(time.Minute)
	release, err := admission.acquire("user-1")
	if err != nil {
		t.Fatalf("acquire after refill: %v", err)
	}
	release()
}

func TestRealtimeCatchUpAdmissionDoesNotChargeRejectedGlobalAttempt(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(1, 1, time.Hour, func() time.Time { return now })

	releaseFirst, err := admission.acquire("user-1")
	if err != nil {
		t.Fatalf("acquire first catch-up: %v", err)
	}
	if _, err := admission.acquire("user-2"); err == nil || err.code != "catch_up_server_busy" {
		t.Fatalf("global-capacity acquire error = %+v, want catch_up_server_busy", err)
	}
	releaseFirst()

	releaseSecond, err := admission.acquire("user-2")
	if err != nil {
		t.Fatalf("global rejection consumed user token: %v", err)
	}
	releaseSecond()
}
