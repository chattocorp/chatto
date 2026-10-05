package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	runtimestatev1 "hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
)

// setPushClock pins the push-subscription clock of core to the value that the
// returned function sets.
func setPushClock(core *ChattoCore, start time.Time) func(time.Time) {
	now := start
	core.pushClock = func() time.Time { return now }
	return func(next time.Time) { now = next }
}

func requirePushKeyPresence(t *testing.T, core *ChattoCore, key string, want bool) {
	t.Helper()
	_, err := core.storage.runtimeStateKV.Get(context.Background(), key)
	switch {
	case err == nil && !want:
		t.Fatalf("key %s exists, want it removed", key)
	case isPushRuntimeStateKeyAbsent(err) && want:
		t.Fatalf("key %s is absent, want it present", key)
	case err != nil && !isPushRuntimeStateKeyAbsent(err):
		t.Fatalf("get key %s: %v", key, err)
	}
}

func TestPushSubscriptionExpiresAfterLifetime(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	setNow := setPushClock(core, start)
	userID := "push-expiry-user"
	endpoint := "https://push.example.com/expiry"

	saved, err := core.SavePushSubscription(ctx, userID, endpoint, "key", "auth", "browser")
	if err != nil {
		t.Fatalf("SavePushSubscription: %v", err)
	}

	setNow(start.Add(PushSubscriptionLifetime - time.Second))
	subs, err := core.GetUserPushSubscriptions(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPushSubscriptions before expiry: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("subscriptions before expiry = %d, want 1", len(subs))
	}

	setNow(start.Add(PushSubscriptionLifetime))
	current, err := core.PushSubscriptionCurrentForUser(ctx, userID, saved)
	if err != nil {
		t.Fatalf("PushSubscriptionCurrentForUser: %v", err)
	}
	if current {
		t.Fatal("expired subscription is current, want it rejected before delivery")
	}

	subs, err = core.GetUserPushSubscriptions(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPushSubscriptions after expiry: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("subscriptions after expiry = %d, want 0", len(subs))
	}
	requirePushKeyPresence(t, core, pushSubscriptionKey(userID, endpoint), false)
	requirePushKeyPresence(t, core, pushEndpointOwnerKey(endpoint), false)
}

func TestPushSubscriptionSaveExtendsExpiry(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	setNow := setPushClock(core, start)
	userID := "push-refresh-user"
	endpoint := "https://push.example.com/refresh"

	if _, err := core.SavePushSubscription(ctx, userID, endpoint, "key", "auth", "browser"); err != nil {
		t.Fatalf("first save: %v", err)
	}
	setNow(start.Add(100 * 24 * time.Hour))
	if _, err := core.SavePushSubscription(ctx, userID, endpoint, "key", "auth", "browser"); err != nil {
		t.Fatalf("refresh save: %v", err)
	}

	setNow(start.Add(PushSubscriptionLifetime + time.Hour))
	subs, err := core.GetUserPushSubscriptions(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPushSubscriptions: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("subscriptions after refresh = %d, want 1", len(subs))
	}
}

func TestExpiredTransferredPushSubscriptionIsRemoved(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	setNow := setPushClock(core, start)
	endpoint := "https://push.example.com/transferred"

	if _, err := core.SavePushSubscription(ctx, "push-old-owner", endpoint, "old-key", "old-auth", "browser"); err != nil {
		t.Fatalf("save for previous account: %v", err)
	}
	setNow(start.Add(30 * 24 * time.Hour))
	if _, err := core.SavePushSubscription(ctx, "push-new-owner", endpoint, "new-key", "new-auth", "browser"); err != nil {
		t.Fatalf("save for current account: %v", err)
	}

	setNow(start.Add(PushSubscriptionLifetime))
	if _, err := core.GetUserPushSubscriptions(ctx, "push-old-owner"); err != nil {
		t.Fatalf("GetUserPushSubscriptions for previous account: %v", err)
	}
	requirePushKeyPresence(t, core, pushSubscriptionKey("push-old-owner", endpoint), false)

	subs, err := core.GetUserPushSubscriptions(ctx, "push-new-owner")
	if err != nil {
		t.Fatalf("GetUserPushSubscriptions for current account: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("current account subscriptions = %d, want 1", len(subs))
	}
	requirePushKeyPresence(t, core, pushEndpointOwnerKey(endpoint), true)
}

func TestExpiredPushSubscriptionsDoNotCountTowardLimit(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	setNow := setPushClock(core, start)
	userID := "push-expiry-limit-user"

	for i := range MaxPushSubscriptionsPerUser {
		endpoint := fmt.Sprintf("https://push.example.com/expiry-limit-%d", i)
		if _, err := core.SavePushSubscription(ctx, userID, endpoint, "key", "auth", "browser"); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	setNow(start.Add(PushSubscriptionLifetime))
	if _, err := core.SavePushSubscription(ctx, userID, "https://push.example.com/expiry-limit-new", "key", "auth", "browser"); err != nil {
		t.Fatalf("save after previous subscriptions expired: %v", err)
	}
}

func TestExpiredPushSubscriptionRemovalKeepsRefreshedRecord(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()
	userID := "push-expiry-race-user"
	endpoint := "https://push.example.com/expiry-race"
	key := pushSubscriptionKey(userID, endpoint)

	if _, err := core.SavePushSubscription(ctx, userID, endpoint, "key", "auth", "browser"); err != nil {
		t.Fatalf("first save: %v", err)
	}
	stale, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get first revision: %v", err)
	}
	if _, err := core.SavePushSubscription(ctx, userID, endpoint, "key", "auth", "browser"); err != nil {
		t.Fatalf("refresh save: %v", err)
	}

	if err := core.deleteExpiredPushSubscription(ctx, userID, key, endpoint, stale.Revision()); err != nil {
		t.Fatalf("deleteExpiredPushSubscription: %v", err)
	}
	requirePushKeyPresence(t, core, key, true)
	subs, err := core.GetUserPushSubscriptions(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPushSubscriptions: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("subscriptions after stale removal = %d, want the refreshed record", len(subs))
	}
}

func TestPushSubscriptionWithoutSaveTimeIsRemoved(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()
	userID := "push-no-save-time-user"
	endpoint := "https://push.example.com/no-save-time"
	key := pushSubscriptionKey(userID, endpoint)
	data, err := proto.Marshal(&runtimestatev1.PushSubscription{Endpoint: endpoint, P256Dh: "key", Auth: "auth"})
	if err != nil {
		t.Fatalf("marshal subscription: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Put(ctx, key, data); err != nil {
		t.Fatalf("store subscription: %v", err)
	}

	subs, err := core.GetUserPushSubscriptions(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPushSubscriptions: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("subscriptions = %d, want 0", len(subs))
	}
	requirePushKeyPresence(t, core, key, false)
}
