package core

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestPresenceHub_BasicFanOut(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Hub is already running from setupTestCore — subscribe directly
	sub, err := core.PresenceHub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer core.PresenceHub.Unsubscribe(sub)

	// Set a user's presence
	err = core.SetPresence(ctx, "user-1", PresenceStatusOnline)
	if err != nil {
		t.Fatalf("SetPresence failed: %v", err)
	}

	// Should receive the update
	select {
	case update := <-sub.C:
		if update.UserID != "user-1" {
			t.Errorf("Expected user-1, got %s", update.UserID)
		}
		if update.Status != PresenceStatusOnline {
			t.Errorf("Expected ONLINE, got %s", update.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for presence update")
	}
}

func TestPresenceHub_SubscribeReceivesOnlyFutureTransitions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if err := core.SetPresence(ctx, "existing-user", PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence failed: %v", err)
	}
	waitForPresenceHubStatus(t, core.PresenceHub, "existing-user", PresenceStatusAway)

	sub, err := core.PresenceHub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer core.PresenceHub.Unsubscribe(sub)

	assertNoPresenceUpdate(t, sub, 100*time.Millisecond)

	transitions := []string{
		PresenceStatusDoNotDisturb,
		PresenceStatusOnline,
		PresenceStatusAway,
	}
	for _, status := range transitions {
		if err := core.SetPresence(ctx, "existing-user", status); err != nil {
			t.Fatalf("SetPresence %s failed: %v", status, err)
		}
		expectPresenceUpdate(t, sub, "existing-user", status)
	}

	// Refreshing an unchanged status must remain suppressed by the hub.
	if err := core.SetPresence(ctx, "existing-user", PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence unchanged away failed: %v", err)
	}
	assertNoPresenceUpdate(t, sub, 100*time.Millisecond)

	if err := core.storage.memoryCacheKV.Delete(ctx, presenceKey("existing-user")); err != nil {
		t.Fatalf("Delete presence failed: %v", err)
	}
	expectPresenceUpdate(t, sub, "existing-user", PresenceStatusOffline)
}

func TestPresenceHub_MultipleSubscribers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two subscribers
	sub1, err := core.PresenceHub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe 1 failed: %v", err)
	}
	defer core.PresenceHub.Unsubscribe(sub1)

	sub2, err := core.PresenceHub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe 2 failed: %v", err)
	}
	defer core.PresenceHub.Unsubscribe(sub2)

	// Set presence
	err = core.SetPresence(ctx, "multi-user", PresenceStatusDoNotDisturb)
	if err != nil {
		t.Fatalf("SetPresence failed: %v", err)
	}

	// Both subscribers should receive the update
	for i, sub := range []*PresenceSubscription{sub1, sub2} {
		select {
		case update := <-sub.C:
			if update.UserID != "multi-user" {
				t.Errorf("Sub %d: expected multi-user, got %s", i+1, update.UserID)
			}
			if update.Status != PresenceStatusDoNotDisturb {
				t.Errorf("Sub %d: expected DO_NOT_DISTURB, got %s", i+1, update.Status)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("Sub %d: timeout waiting for update", i+1)
		}
	}
}

func TestPresenceHub_OfflineOnDelete(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	sub, err := core.PresenceHub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer core.PresenceHub.Unsubscribe(sub)

	// Set presence, then delete it
	err = core.SetPresence(ctx, "delete-user", PresenceStatusOnline)
	if err != nil {
		t.Fatalf("SetPresence failed: %v", err)
	}

	// Drain the ONLINE event
	select {
	case <-sub.C:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for ONLINE event")
	}

	// Delete the presence entry
	err = core.storage.memoryCacheKV.Delete(ctx, presenceKey("delete-user"))
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should receive OFFLINE
	select {
	case update := <-sub.C:
		if update.Status != PresenceStatusOffline {
			t.Errorf("Expected OFFLINE on delete, got %s", update.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for OFFLINE event")
	}
}

func TestPresenceHub_UserLevelStatusOverwrites(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	sub, err := core.PresenceHub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer core.PresenceHub.Unsubscribe(sub)

	if err := core.SetPresence(ctx, "overwrite-user", PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence away failed: %v", err)
	}
	expectPresenceUpdate(t, sub, "overwrite-user", PresenceStatusAway)

	if err := core.SetPresence(ctx, "overwrite-user", PresenceStatusOnline); err != nil {
		t.Fatalf("SetPresence online failed: %v", err)
	}
	expectPresenceUpdate(t, sub, "overwrite-user", PresenceStatusOnline)

	if err := core.storage.memoryCacheKV.Delete(ctx, presenceKey("overwrite-user")); err != nil {
		t.Fatalf("Delete presence failed: %v", err)
	}
	expectPresenceUpdate(t, sub, "overwrite-user", PresenceStatusOffline)
}

func expectPresenceUpdate(t *testing.T, sub *PresenceSubscription, userID, status string) {
	t.Helper()
	select {
	case update := <-sub.C:
		if update.UserID != userID {
			t.Fatalf("Expected user %s, got %s", userID, update.UserID)
		}
		if update.Status != status {
			t.Fatalf("Expected status %s, got %s", status, update.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Timeout waiting for %s presence update", status)
	}
}

func assertNoPresenceUpdate(t *testing.T, sub *PresenceSubscription, wait time.Duration) {
	t.Helper()
	select {
	case update := <-sub.C:
		t.Fatalf("Unexpected presence update: %+v", update)
	case <-time.After(wait):
	}
}

func waitForPresenceHubStatus(t *testing.T, hub *PresenceHub, userID, status string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		hub.mu.Lock()
		current := hub.snapshot[userID]
		hub.mu.Unlock()
		if current == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("PresenceHub snapshot for %s did not reach %s", userID, status)
}

func BenchmarkPresenceHubSubscribe(b *testing.B) {
	for _, onlineUsers := range []int{0, 100, 1000} {
		b.Run("online_users_"+strconv.Itoa(onlineUsers), func(b *testing.B) {
			hub := NewPresenceHub(nil, nil)
			for i := range onlineUsers {
				hub.snapshot["user-"+strconv.Itoa(i)] = PresenceStatusOnline
			}
			close(hub.ready)

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				sub, err := hub.Subscribe(context.Background())
				if err != nil {
					b.Fatalf("Subscribe failed: %v", err)
				}
				hub.Unsubscribe(sub)
			}
		})
	}
}
