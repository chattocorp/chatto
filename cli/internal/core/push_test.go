package core

import (
	"context"
	"strings"
	"testing"
)

func TestPushSubscriptionKey(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		endpoint string
	}{
		{
			name:     "basic key generation",
			userID:   "user-123",
			endpoint: "https://push.example.com/abc",
		},
		{
			name:     "different endpoints produce different keys",
			userID:   "user-123",
			endpoint: "https://push.example.com/xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := pushSubscriptionKey(tt.userID, tt.endpoint)
			if key == "" {
				t.Error("Expected non-empty key")
			}

			// Key should start with push_subscription.{userID}.
			expectedPrefix := "push_subscription." + tt.userID + "."
			if len(key) <= len(expectedPrefix) {
				t.Errorf("Key too short: %s", key)
			}
			if key[:len(expectedPrefix)] != expectedPrefix {
				t.Errorf("Key should start with %s, got %s", expectedPrefix, key)
			}
		})
	}

	// Verify different endpoints produce different keys
	key1 := pushSubscriptionKey("user-123", "https://push.example.com/abc")
	key2 := pushSubscriptionKey("user-123", "https://push.example.com/xyz")
	if key1 == key2 {
		t.Error("Different endpoints should produce different keys")
	}

	// Verify same endpoint produces same key (idempotent)
	key3 := pushSubscriptionKey("user-123", "https://push.example.com/abc")
	if key1 != key3 {
		t.Error("Same endpoint should produce same key")
	}
}

func TestExtractUserIDFromPushKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "valid key",
			key:      "push_subscription.user-123.abc123",
			expected: "user-123",
		},
		{
			name:     "empty key",
			key:      "",
			expected: "",
		},
		{
			name:     "wrong prefix",
			key:      "other_key.user-123.abc",
			expected: "",
		},
		{
			name:     "too few parts",
			key:      "push_subscription.user-123",
			expected: "",
		},
		{
			name:     "too many parts",
			key:      "push_subscription.user-123.abc.extra",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractUserIDFromPushKey(tt.key)
			if got != tt.expected {
				t.Errorf("extractUserIDFromPushKey(%s) = %s, want %s", tt.key, got, tt.expected)
			}
		})
	}
}

func TestSavePushSubscription(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userID := "push-user-1"
	endpoint := "https://push.example.com/endpoint123"
	p256dh := "test-p256dh-key"
	auth := "test-auth-secret"
	userAgent := "TestBrowser/1.0"

	t.Run("creates new subscription", func(t *testing.T) {
		sub, err := core.SavePushSubscription(ctx, userID, endpoint, p256dh, auth, userAgent)
		if err != nil {
			t.Fatalf("SavePushSubscription error: %v", err)
		}
		if sub == nil {
			t.Fatal("Expected subscription to be non-nil")
		}
		if sub.Endpoint != endpoint {
			t.Errorf("Expected endpoint %s, got %s", endpoint, sub.Endpoint)
		}
		if sub.P256Dh != p256dh {
			t.Errorf("Expected p256dh %s, got %s", p256dh, sub.P256Dh)
		}
		if sub.Auth != auth {
			t.Errorf("Expected auth %s, got %s", auth, sub.Auth)
		}
		if sub.UserAgent != userAgent {
			t.Errorf("Expected userAgent %s, got %s", userAgent, sub.UserAgent)
		}
		if sub.CreatedAt == nil {
			t.Error("Expected CreatedAt to be set")
		}

		key := pushSubscriptionKey(userID, endpoint)
		if _, err := core.storage.runtimeStateKV.Get(ctx, key); err != nil {
			t.Fatalf("expected push subscription in RUNTIME_STATE: %v", err)
		}
		assertLegacyKeyAbsent(t, core.storage.serverKV, key, "legacy INSTANCE push subscription")
	})

	t.Run("updates existing subscription with same endpoint", func(t *testing.T) {
		newAuth := "updated-auth-secret"
		sub, err := core.SavePushSubscription(ctx, userID, endpoint, p256dh, newAuth, userAgent)
		if err != nil {
			t.Fatalf("SavePushSubscription error: %v", err)
		}
		if sub.Auth != newAuth {
			t.Errorf("Expected auth %s, got %s", newAuth, sub.Auth)
		}

		// Should still only have one subscription
		subs, _ := core.GetUserPushSubscriptions(ctx, userID)
		if len(subs) != 1 {
			t.Errorf("Expected 1 subscription after update, got %d", len(subs))
		}
	})
}

func TestSavePushSubscription_StringLengthLimits(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()
	userID := "push-user-limits"

	t.Run("accepts values at max length", func(t *testing.T) {
		_, err := core.SavePushSubscription(
			ctx,
			userID,
			strings.Repeat("e", MaxPushEndpointLength),
			strings.Repeat("p", MaxPushKeyLength),
			strings.Repeat("a", MaxPushAuthLength),
			strings.Repeat("u", MaxPushUserAgentLength),
		)
		if err != nil {
			t.Fatalf("SavePushSubscription at max lengths: %v", err)
		}
	})

	tests := []struct {
		name      string
		endpoint  string
		p256dh    string
		auth      string
		userAgent string
		field     string
		max       int
	}{
		{
			name:     "endpoint",
			endpoint: strings.Repeat("e", MaxPushEndpointLength+1),
			p256dh:   "key",
			auth:     "auth",
			field:    "push endpoint",
			max:      MaxPushEndpointLength,
		},
		{
			name:     "p256dh",
			endpoint: "https://push.example.com/limits-p256dh",
			p256dh:   strings.Repeat("p", MaxPushKeyLength+1),
			auth:     "auth",
			field:    "push p256dh key",
			max:      MaxPushKeyLength,
		},
		{
			name:     "auth",
			endpoint: "https://push.example.com/limits-auth",
			p256dh:   "key",
			auth:     strings.Repeat("a", MaxPushAuthLength+1),
			field:    "push auth secret",
			max:      MaxPushAuthLength,
		},
		{
			name:      "user agent",
			endpoint:  "https://push.example.com/limits-user-agent",
			p256dh:    "key",
			auth:      "auth",
			userAgent: strings.Repeat("u", MaxPushUserAgentLength+1),
			field:     "push user agent",
			max:       MaxPushUserAgentLength,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := core.SavePushSubscription(ctx, userID, tt.endpoint, tt.p256dh, tt.auth, tt.userAgent)
			assertStringLengthError(t, err, tt.field, tt.max)
		})
	}
}

func TestGetAllPushSubscriptions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	_, err := core.SavePushSubscription(ctx, "push-user-all-a", "https://push.example.com/all-a", "key", "auth", "browser-a")
	if err != nil {
		t.Fatalf("SavePushSubscription user A error: %v", err)
	}
	_, err = core.SavePushSubscription(ctx, "push-user-all-b", "https://push.example.com/all-b", "key", "auth", "browser-b")
	if err != nil {
		t.Fatalf("SavePushSubscription user B error: %v", err)
	}

	subs, err := core.GetAllPushSubscriptions(ctx)
	if err != nil {
		t.Fatalf("GetAllPushSubscriptions error: %v", err)
	}

	seen := map[string]bool{}
	for _, sub := range subs {
		seen[sub.UserID] = true
	}
	if !seen["push-user-all-a"] || !seen["push-user-all-b"] {
		t.Fatalf("GetAllPushSubscriptions missing users; got %#v", seen)
	}
}

func TestGetUserPushSubscriptions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userID := "push-user-2"

	t.Run("returns empty list when no subscriptions", func(t *testing.T) {
		subs, err := core.GetUserPushSubscriptions(ctx, userID)
		if err != nil {
			t.Fatalf("GetUserPushSubscriptions error: %v", err)
		}
		if len(subs) != 0 {
			t.Errorf("Expected 0 subscriptions, got %d", len(subs))
		}
	})

	t.Run("returns multiple subscriptions for same user", func(t *testing.T) {
		// Create subscriptions for different devices
		endpoints := []string{
			"https://push.example.com/device1",
			"https://push.example.com/device2",
			"https://push.example.com/device3",
		}

		for _, endpoint := range endpoints {
			_, err := core.SavePushSubscription(ctx, userID, endpoint, "key", "auth", "browser")
			if err != nil {
				t.Fatalf("SavePushSubscription error: %v", err)
			}
		}

		subs, err := core.GetUserPushSubscriptions(ctx, userID)
		if err != nil {
			t.Fatalf("GetUserPushSubscriptions error: %v", err)
		}
		if len(subs) != 3 {
			t.Errorf("Expected 3 subscriptions, got %d", len(subs))
		}
	})
}

func TestDeletePushSubscription(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userID := "push-user-3"
	endpoint := "https://push.example.com/to-delete"

	t.Run("returns nil error when deleting non-existent subscription", func(t *testing.T) {
		err := core.DeletePushSubscription(ctx, userID, "non-existent-endpoint")
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("deletes existing subscription", func(t *testing.T) {
		// Create subscription
		_, err := core.SavePushSubscription(ctx, userID, endpoint, "key", "auth", "browser")
		if err != nil {
			t.Fatalf("SavePushSubscription error: %v", err)
		}

		// Verify it exists
		subs, _ := core.GetUserPushSubscriptions(ctx, userID)
		initialCount := len(subs)

		// Delete it
		err = core.DeletePushSubscription(ctx, userID, endpoint)
		if err != nil {
			t.Fatalf("DeletePushSubscription error: %v", err)
		}

		// Verify it's gone
		subs, _ = core.GetUserPushSubscriptions(ctx, userID)
		if len(subs) != initialCount-1 {
			t.Errorf("Expected %d subscriptions after delete, got %d", initialCount-1, len(subs))
		}
	})
}

func TestDeleteAllUserPushSubscriptions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userID := "push-user-4"

	t.Run("returns 0 when no subscriptions", func(t *testing.T) {
		count, err := core.DeleteAllUserPushSubscriptions(ctx, userID)
		if err != nil {
			t.Fatalf("DeleteAllUserPushSubscriptions error: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0, got %d", count)
		}
	})

	t.Run("deletes all subscriptions for user", func(t *testing.T) {
		// Create multiple subscriptions
		for i := 0; i < 3; i++ {
			endpoint := "https://push.example.com/device" + string(rune('a'+i))
			_, _ = core.SavePushSubscription(ctx, userID, endpoint, "key", "auth", "browser")
		}

		count, err := core.DeleteAllUserPushSubscriptions(ctx, userID)
		if err != nil {
			t.Fatalf("DeleteAllUserPushSubscriptions error: %v", err)
		}
		if count != 3 {
			t.Errorf("Expected 3 deleted, got %d", count)
		}

		// Verify all are gone
		subs, _ := core.GetUserPushSubscriptions(ctx, userID)
		if len(subs) != 0 {
			t.Errorf("Expected 0 remaining, got %d", len(subs))
		}
	})
}

func TestPushSubscriptionIsolation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userA := "push-user-a"
	userB := "push-user-b"

	t.Run("user cannot see other user's subscriptions", func(t *testing.T) {
		// Create subscription for userA
		_, _ = core.SavePushSubscription(ctx, userA, "https://push.example.com/a", "key", "auth", "browser")

		// userB should not see userA's subscription
		userBSubs, _ := core.GetUserPushSubscriptions(ctx, userB)
		if len(userBSubs) != 0 {
			t.Error("userB should not see userA's subscriptions")
		}

		// userA should see their subscription
		userASubs, _ := core.GetUserPushSubscriptions(ctx, userA)
		if len(userASubs) != 1 {
			t.Errorf("userA should have 1 subscription, got %d", len(userASubs))
		}
	})

	t.Run("deleting does not affect other user's subscriptions", func(t *testing.T) {
		// Clear and set up fresh
		core.DeleteAllUserPushSubscriptions(ctx, userA)
		core.DeleteAllUserPushSubscriptions(ctx, userB)

		// Create subscriptions for both users
		_, _ = core.SavePushSubscription(ctx, userA, "https://push.example.com/a2", "key", "auth", "browser")
		_, _ = core.SavePushSubscription(ctx, userB, "https://push.example.com/b2", "key", "auth", "browser")

		// Delete userA's subscriptions
		core.DeleteAllUserPushSubscriptions(ctx, userA)

		// userB should still have their subscription
		userBSubs, _ := core.GetUserPushSubscriptions(ctx, userB)
		if len(userBSubs) != 1 {
			t.Errorf("userB should still have 1 subscription after userA deletes, got %d", len(userBSubs))
		}
	})
}
