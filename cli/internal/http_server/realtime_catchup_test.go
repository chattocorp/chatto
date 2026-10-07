package http_server

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRealtimeAuthorityAdmissionWaitsForSharedCapacity(t *testing.T) {
	a := newRealtimeCatchUpAdmissionWithLimits(1, 3, time.Minute, time.Now)
	release, err := a.acquire("viewer", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	admitted := make(chan func(), 1)
	go func() {
		stop, err := a.acquireAuthority(ctx)
		if err != nil {
			admitted <- nil
			return
		}
		admitted <- stop
	}()
	select {
	case <-admitted:
		t.Fatal("authority handoff bypassed occupied shared capacity")
	default:
	}
	release()
	select {
	case stop := <-admitted:
		if stop == nil {
			t.Fatal("authority handoff did not wait for capacity")
		}
		stop()
		stop()
	case <-ctx.Done():
		t.Fatal("authority handoff stalled after capacity was released")
	}
	stop, err2 := a.acquireAuthority(ctx)
	if err2 != nil {
		t.Fatal(err2)
	}
	defer stop()
	cancelled, cancelWait := context.WithCancel(context.Background())
	cancelWait()
	if _, err := a.acquireAuthority(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled admission = %v", err)
	}
}

func TestRealtimeCatchUpAdmissionBoundsUserAndProcessConcurrency(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(1, 3, time.Minute, func() time.Time { return now })

	releaseFirst, err := admission.acquire("user-1", true)
	if err != nil {
		t.Fatalf("acquire first catch-up: %v", err)
	}
	if _, err := admission.acquire("user-1", true); err == nil || err.code != "catch_up_in_progress" {
		t.Fatalf("concurrent same-user acquire error = %+v, want catch_up_in_progress", err)
	}
	if _, err := admission.acquire("user-2", true); err == nil || err.code != "catch_up_server_busy" {
		t.Fatalf("global-capacity acquire error = %+v, want catch_up_server_busy", err)
	}

	releaseFirst()
	releaseFirst() // Release is deliberately safe on all return paths.
	releaseSecond, err := admission.acquire("user-2", true)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	releaseSecond()
}

func TestRealtimeCatchUpAdmissionPrunesInactiveUsers(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(1, 1, time.Hour, func() time.Time { return now })

	release, err := admission.acquire("inactive-user", false)
	if err != nil {
		t.Fatalf("acquire inactive user: %v", err)
	}
	release()

	now = now.Add(realtimeCatchUpLimiterStateLifetime + time.Second)
	admission.acquisitions = 255
	release, err = admission.acquire("active-user", false)
	if err != nil {
		t.Fatalf("acquire active user after cleanup interval: %v", err)
	}
	release()

	if _, exists := admission.users["inactive-user"]; exists {
		t.Fatal("inactive catch-up user was not pruned")
	}
	if _, exists := admission.users["active-user"]; !exists {
		t.Fatal("current catch-up user was pruned")
	}
}

func TestRealtimeCatchUpAdmissionRateLimitsReplayAndBootstrapAttempts(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(2, 2, time.Minute, func() time.Time { return now })

	for attempt := range 2 {
		release, err := admission.acquire("user-1", true)
		if err != nil {
			t.Fatalf("acquire attempt %d: %v", attempt+1, err)
		}
		release()
	}
	if _, err := admission.acquire("user-1", true); err == nil || err.code != "catch_up_rate_limited" || err.retryAfter != time.Minute {
		t.Fatalf("rate-limited acquire error = %+v, want one-minute retry", err)
	}

	now = now.Add(time.Minute)
	release, err := admission.acquire("user-1", true)
	if err != nil {
		t.Fatalf("acquire after refill: %v", err)
	}
	release()
}

func TestRealtimeCatchUpAdmissionDoesNotChargeRejectedGlobalAttempt(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(1, 1, time.Hour, func() time.Time { return now })

	releaseFirst, err := admission.acquire("user-1", true)
	if err != nil {
		t.Fatalf("acquire first catch-up: %v", err)
	}
	if _, err := admission.acquire("user-2", true); err == nil || err.code != "catch_up_server_busy" {
		t.Fatalf("global-capacity acquire error = %+v, want catch_up_server_busy", err)
	}
	releaseFirst()

	releaseSecond, err := admission.acquire("user-2", true)
	if err != nil {
		t.Fatalf("global rejection consumed user token: %v", err)
	}
	releaseSecond()
}

func TestRealtimeCatchUpAdmissionDoesNotRateLimitCurrentBoundaryReconnect(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(1, 1, time.Hour, func() time.Time { return now })

	release, err := admission.acquire("user-1", true)
	if err != nil {
		t.Fatalf("consume rate token: %v", err)
	}
	release()

	release, err = admission.acquire("user-1", false)
	if err != nil {
		t.Fatalf("unmetered current-boundary reconnect: %v", err)
	}
	if _, err := admission.acquire("user-1", false); err == nil || err.code != "catch_up_in_progress" {
		t.Fatalf("concurrent unmetered acquire error = %+v, want catch_up_in_progress", err)
	}
	release()

	if _, err := admission.acquire("user-1", true); err == nil || err.code != "catch_up_rate_limited" {
		t.Fatalf("metered replay after bypass error = %+v, want catch_up_rate_limited", err)
	}
}

func TestRealtimeCatchUpAdmissionChargesGapDiscoveredAfterBoundaryCheck(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(1, 1, time.Hour, func() time.Time { return now })

	release, err := admission.acquire("user-1", false)
	if err != nil {
		t.Fatalf("unmetered boundary admission: %v", err)
	}
	if err := admission.consumeReplayToken("user-1"); err != nil {
		t.Fatalf("charge newly-discovered gap: %v", err)
	}
	release()

	release, err = admission.acquire("user-1", false)
	if err != nil {
		t.Fatalf("second unmetered boundary admission: %v", err)
	}
	if err := admission.consumeReplayToken("user-1"); err == nil || err.code != "catch_up_rate_limited" {
		t.Fatalf("second gap charge error = %+v, want catch_up_rate_limited", err)
	}
	release()
}

func TestRealtimeCatchUpAdmissionRateLimitsSequentialGeneralCatchUps(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(1, 1, time.Hour, func() time.Time { return now })

	for attempt := range realtimeCatchUpGeneralRateBurst {
		release, err := admission.acquire("user-1", false)
		if err != nil {
			t.Fatalf("general catch-up %d: %v", attempt+1, err)
		}
		release()
	}
	if _, err := admission.acquire("user-1", false); err == nil || err.code != "catch_up_rate_limited" || err.retryAfter != time.Second {
		t.Fatalf("general rate-limit error = %+v, want one-second retry", err)
	}

	now = now.Add(time.Second)
	release, err := admission.acquire("user-1", false)
	if err != nil {
		t.Fatalf("general catch-up after refill: %v", err)
	}
	release()
}
