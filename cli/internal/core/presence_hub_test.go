package core

import (
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
	err = core.SetPresence(ctx, "user-1", "session-1", PresenceStatusOnline)
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

func TestPresenceHub_SnapshotIncludesExisting(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Set presence — hub is already running, so this gets picked up live
	err := core.SetPresence(ctx, "existing-user", "session-1", PresenceStatusAway)
	if err != nil {
		t.Fatalf("SetPresence failed: %v", err)
	}

	// Brief wait for the hub to process the KV update
	time.Sleep(100 * time.Millisecond)

	// Subscribe — snapshot should include the existing presence
	sub, err := core.PresenceHub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer core.PresenceHub.Unsubscribe(sub)

	status, exists := sub.Snapshot["existing-user"]
	if !exists {
		t.Fatal("Expected existing-user in snapshot")
	}
	if status != PresenceStatusAway {
		t.Errorf("Expected AWAY in snapshot, got %s", status)
	}
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
	err = core.SetPresence(ctx, "multi-user", "session-1", PresenceStatusDoNotDisturb)
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
	err = core.SetPresence(ctx, "delete-user", "session-1", PresenceStatusOnline)
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
	err = core.storage.memoryCacheKV.Delete(ctx, presenceSessionKey("delete-user", "session-1"))
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

func TestPresenceHub_DerivesEffectiveStatusAcrossSessions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	sub, err := core.PresenceHub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer core.PresenceHub.Unsubscribe(sub)

	if err := core.SetPresence(ctx, "multi-session-user", "tab-1", PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence tab-1 failed: %v", err)
	}
	expectPresenceUpdate(t, sub, "multi-session-user", PresenceStatusAway)

	if err := core.SetPresence(ctx, "multi-session-user", "tab-2", PresenceStatusOnline); err != nil {
		t.Fatalf("SetPresence tab-2 failed: %v", err)
	}
	expectPresenceUpdate(t, sub, "multi-session-user", PresenceStatusOnline)

	if err := core.storage.memoryCacheKV.Delete(ctx, presenceSessionKey("multi-session-user", "tab-1")); err != nil {
		t.Fatalf("Delete tab-1 failed: %v", err)
	}
	select {
	case update := <-sub.C:
		t.Fatalf("Unexpected update after deleting lower-precedence session: %+v", update)
	case <-time.After(200 * time.Millisecond):
	}

	if err := core.storage.memoryCacheKV.Delete(ctx, presenceSessionKey("multi-session-user", "tab-2")); err != nil {
		t.Fatalf("Delete tab-2 failed: %v", err)
	}
	expectPresenceUpdate(t, sub, "multi-session-user", PresenceStatusOffline)
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
