package http_server

import (
	"testing"
	"time"
)

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

func TestRealtimeHydrationAdmissionIsSharedAcrossSocketsAndCatchUp(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(2, 3, time.Minute, func() time.Time { return now })

	release, err := admission.acquireHydration("user-1")
	if err != nil {
		t.Fatalf("acquire hydration: %v", err)
	}
	if _, err := admission.acquireHydration("user-1"); err == nil || err.code != "room_hydration_in_progress" {
		t.Fatalf("parallel socket hydration error = %+v, want room_hydration_in_progress", err)
	}
	if _, err := admission.acquire("user-1", false); err == nil || err.code != "catch_up_in_progress" {
		t.Fatalf("catch-up during hydration error = %+v, want catch_up_in_progress", err)
	}
	release()

	releaseCatchUp, catchUpErr := admission.acquire("user-1", false)
	if catchUpErr != nil {
		t.Fatalf("catch-up after hydration: %v", catchUpErr)
	}
	if _, err := admission.acquireHydration("user-1"); err == nil || err.code != "room_hydration_in_progress" {
		t.Fatalf("hydration during catch-up error = %+v, want room_hydration_in_progress", err)
	}
	releaseCatchUp()
}

func TestRealtimeHydrationAdmissionRateLimitsDistinctRooms(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(1, 1, time.Hour, func() time.Time { return now })

	for attempt := 0; attempt < realtimeHydrationRateBurst; attempt++ {
		release, err := admission.acquireHydration("user-1")
		if err != nil {
			t.Fatalf("hydration %d: %v", attempt+1, err)
		}
		release()
	}
	if _, err := admission.acquireHydration("user-1"); err == nil || err.code != "room_hydration_rate_limited" {
		t.Fatalf("hydration rate-limit error = %+v, want room_hydration_rate_limited", err)
	}

	now = now.Add(time.Second)
	release, err := admission.acquireHydration("user-1")
	if err != nil {
		t.Fatalf("hydration after refill: %v", err)
	}
	release()
}

func TestRealtimeHydrationAdmissionPrunesInactiveUsers(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(1, 1, time.Hour, func() time.Time { return now })

	release, err := admission.acquireHydration("inactive-user")
	if err != nil {
		t.Fatalf("acquire hydration: %v", err)
	}
	release()

	now = now.Add(realtimeCatchUpLimiterStateLifetime + time.Second)
	admission.acquisitions = 255
	release, err = admission.acquireHydration("active-user")
	if err != nil {
		t.Fatalf("acquire hydration after cleanup interval: %v", err)
	}
	release()

	if _, exists := admission.users["inactive-user"]; exists {
		t.Fatal("inactive hydration-only user was not pruned")
	}
	if _, exists := admission.users["active-user"]; !exists {
		t.Fatal("current hydration user was pruned")
	}
}

func TestRealtimeCatchUpAdmissionRateLimitsReplayAndBootstrapAttempts(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	admission := newRealtimeCatchUpAdmissionWithLimits(2, 2, time.Minute, func() time.Time { return now })

	for attempt := 0; attempt < 2; attempt++ {
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

	for attempt := 0; attempt < realtimeCatchUpGeneralRateBurst; attempt++ {
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
