package core

import (
	"strings"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Status Conversion Tests
// ============================================================================

func TestPresenceStatusFromString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected corev1.UserPresenceStatus
	}{
		{
			name:     "ONLINE status",
			input:    PresenceStatusOnline,
			expected: corev1.UserPresenceStatus_USER_PRESENCE_STATUS_ONLINE,
		},
		{
			name:     "AWAY status",
			input:    PresenceStatusAway,
			expected: corev1.UserPresenceStatus_USER_PRESENCE_STATUS_AWAY,
		},
		{
			name:     "DO_NOT_DISTURB status",
			input:    PresenceStatusDoNotDisturb,
			expected: corev1.UserPresenceStatus_USER_PRESENCE_STATUS_DO_NOT_DISTURB,
		},
		{
			name:     "unknown status defaults to ONLINE",
			input:    "UNKNOWN",
			expected: corev1.UserPresenceStatus_USER_PRESENCE_STATUS_ONLINE,
		},
		{
			name:     "empty string defaults to ONLINE",
			input:    "",
			expected: corev1.UserPresenceStatus_USER_PRESENCE_STATUS_ONLINE,
		},
		{
			name:     "OFFLINE defaults to ONLINE (should not be stored)",
			input:    PresenceStatusOffline,
			expected: corev1.UserPresenceStatus_USER_PRESENCE_STATUS_ONLINE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := presenceStatusFromString(tt.input)
			if result != tt.expected {
				t.Errorf("presenceStatusFromString(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPresenceStatusToString(t *testing.T) {
	tests := []struct {
		name     string
		input    corev1.UserPresenceStatus
		expected string
	}{
		{
			name:     "ONLINE status",
			input:    corev1.UserPresenceStatus_USER_PRESENCE_STATUS_ONLINE,
			expected: PresenceStatusOnline,
		},
		{
			name:     "AWAY status",
			input:    corev1.UserPresenceStatus_USER_PRESENCE_STATUS_AWAY,
			expected: PresenceStatusAway,
		},
		{
			name:     "DO_NOT_DISTURB status",
			input:    corev1.UserPresenceStatus_USER_PRESENCE_STATUS_DO_NOT_DISTURB,
			expected: PresenceStatusDoNotDisturb,
		},
		{
			name:     "unknown enum value defaults to ONLINE",
			input:    corev1.UserPresenceStatus(999),
			expected: PresenceStatusOnline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := presenceStatusToString(tt.input)
			if result != tt.expected {
				t.Errorf("presenceStatusToString(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPresenceStatusRoundTrip(t *testing.T) {
	// Verify that converting to proto and back yields the same string
	statuses := []string{PresenceStatusOnline, PresenceStatusAway, PresenceStatusDoNotDisturb}

	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			proto := presenceStatusFromString(status)
			result := presenceStatusToString(proto)
			if result != status {
				t.Errorf("Round trip failed: %q -> %v -> %q", status, proto, result)
			}
		})
	}
}

// ============================================================================
// Key Helper Tests
// ============================================================================

func TestPresenceSessionKey(t *testing.T) {
	tests := []struct {
		userID    string
		sessionID string
		expected  string
	}{
		{"user123", "tab-1", "presence_session.user123.tab-1"},
		{"abc", "device_2", "presence_session.abc.device_2"},
		{"a1b2c3d4e5f6g7", "uuid-123", "presence_session.a1b2c3d4e5f6g7.uuid-123"},
	}

	for _, tt := range tests {
		t.Run(tt.userID, func(t *testing.T) {
			result := presenceSessionKey(tt.userID, tt.sessionID)
			if result != tt.expected {
				t.Errorf("presenceSessionKey(%q, %q) = %q, want %q", tt.userID, tt.sessionID, result, tt.expected)
			}
		})
	}
}

func TestParsePresenceSessionKey(t *testing.T) {
	tests := []struct {
		key       string
		userID    string
		sessionID string
		ok        bool
	}{
		{"presence_session.user123.tab-1", "user123", "tab-1", true},
		{"presence_session.abc.device_2", "abc", "device_2", true},
		{"presence.user123", "", "", false},
		{"presence_session.user123", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			userID, sessionID, ok := parsePresenceSessionKey(tt.key)
			if ok != tt.ok || userID != tt.userID || sessionID != tt.sessionID {
				t.Errorf("parsePresenceSessionKey(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.key, userID, sessionID, ok, tt.userID, tt.sessionID, tt.ok)
			}
		})
	}
}

func TestValidatePresenceSessionID(t *testing.T) {
	valid := []string{"tab-1", "device_2", "ABC123", "550e8400-e29b-41d4-a716-446655440000"}
	for _, id := range valid {
		if err := ValidatePresenceSessionID(id); err != nil {
			t.Errorf("ValidatePresenceSessionID(%q) returned error: %v", id, err)
		}
	}

	invalid := []string{"", "has.dot", "has/slash", "has space", strings.Repeat("a", 129)}
	for _, id := range invalid {
		if err := ValidatePresenceSessionID(id); err == nil {
			t.Errorf("ValidatePresenceSessionID(%q) returned nil error", id)
		}
	}
}

func TestPresenceSessionKeyRoundTrip(t *testing.T) {
	userIDs := []string{"user123", "abc", "a1b2c3d4e5f6g7"}

	for _, userID := range userIDs {
		t.Run(userID, func(t *testing.T) {
			key := presenceSessionKey(userID, "session-1")
			resultUserID, resultSessionID, ok := parsePresenceSessionKey(key)
			if !ok || resultUserID != userID || resultSessionID != "session-1" {
				t.Errorf("Round trip failed: %q -> %q -> (%q, %q, %v)", userID, key, resultUserID, resultSessionID, ok)
			}
		})
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestChattoCore_GetUserPresence_Offline(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// User with no presence entry should be OFFLINE
	status, err := core.GetUserPresence(ctx, "nonexistent-user")
	if err != nil {
		t.Fatalf("GetUserPresence failed: %v", err)
	}
	if status != PresenceStatusOffline {
		t.Errorf("Expected OFFLINE for non-existent user, got %q", status)
	}
}

func TestChattoCore_SetAndGetPresence(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-123"
	sessionID := "session-1"

	// Set presence to ONLINE
	err := core.SetPresence(ctx, userID, sessionID, PresenceStatusOnline)
	if err != nil {
		t.Fatalf("setPresence failed: %v", err)
	}

	// Verify presence is ONLINE
	status, err := core.GetUserPresence(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPresence failed: %v", err)
	}
	if status != PresenceStatusOnline {
		t.Errorf("Expected ONLINE, got %q", status)
	}

	// Change to AWAY
	err = core.SetPresence(ctx, userID, sessionID, PresenceStatusAway)
	if err != nil {
		t.Fatalf("setPresence failed: %v", err)
	}

	status, err = core.GetUserPresence(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPresence failed: %v", err)
	}
	if status != PresenceStatusAway {
		t.Errorf("Expected AWAY, got %q", status)
	}

	// Change to DO_NOT_DISTURB
	err = core.SetPresence(ctx, userID, sessionID, PresenceStatusDoNotDisturb)
	if err != nil {
		t.Fatalf("setPresence failed: %v", err)
	}

	status, err = core.GetUserPresence(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPresence failed: %v", err)
	}
	if status != PresenceStatusDoNotDisturb {
		t.Errorf("Expected DO_NOT_DISTURB, got %q", status)
	}
}

func TestChattoCore_PresenceDelete(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-delete"
	sessionID := "session-1"

	// Set presence
	err := core.SetPresence(ctx, userID, sessionID, PresenceStatusOnline)
	if err != nil {
		t.Fatalf("setPresence failed: %v", err)
	}

	// Verify it's set
	status, _ := core.GetUserPresence(ctx, userID)
	if status != PresenceStatusOnline {
		t.Fatalf("Expected ONLINE before delete, got %q", status)
	}

	// Delete the presence entry
	err = core.storage.memoryCacheKV.Delete(ctx, presenceSessionKey(userID, sessionID))
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should be OFFLINE now
	status, err = core.GetUserPresence(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPresence failed: %v", err)
	}
	if status != PresenceStatusOffline {
		t.Errorf("Expected OFFLINE after delete, got %q", status)
	}
}

func TestChattoCore_RefreshPresence(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-refresh"
	sessionID := "session-1"

	// Set presence to AWAY
	err := core.SetPresence(ctx, userID, sessionID, PresenceStatusAway)
	if err != nil {
		t.Fatalf("SetPresence failed: %v", err)
	}

	// Refresh should preserve the AWAY status
	err = core.refreshPresence(ctx, userID, sessionID)
	if err != nil {
		t.Fatalf("refreshPresence failed: %v", err)
	}

	status, err := core.GetUserPresence(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPresence failed: %v", err)
	}
	if status != PresenceStatusAway {
		t.Errorf("Expected AWAY after refresh, got %q", status)
	}
}

func TestChattoCore_RefreshPresenceRenewsKeyTTL(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-refresh-ttl"
	sessionID := "session-1"
	key := presenceSessionKey(userID, sessionID)

	if err := core.SetPresence(ctx, userID, sessionID, PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence failed: %v", err)
	}

	stream, err := core.js.Stream(ctx, "KV_MEMORY_CACHE")
	if err != nil {
		t.Fatalf("open MEMORY_CACHE stream: %v", err)
	}
	before, err := stream.GetLastMsgForSubject(ctx, "$KV.MEMORY_CACHE."+key)
	if err != nil {
		t.Fatalf("get initial presence message: %v", err)
	}

	if err := core.refreshPresence(ctx, userID, sessionID); err != nil {
		t.Fatalf("refreshPresence failed: %v", err)
	}

	after, err := stream.GetLastMsgForSubject(ctx, "$KV.MEMORY_CACHE."+key)
	if err != nil {
		t.Fatalf("get refreshed presence message: %v", err)
	}
	if after.Sequence <= before.Sequence {
		t.Fatalf("expected refresh to rewrite presence key, before seq=%d after seq=%d", before.Sequence, after.Sequence)
	}
	if got := after.Header.Get(jetstream.MsgTTLHeader); got != PresenceTTL.String() {
		t.Fatalf("refreshed presence TTL header = %q, want %q", got, PresenceTTL.String())
	}
}

func TestChattoCore_RefreshPresence_Expired(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-refresh-expired"
	sessionID := "session-1"

	// Don't set any presence — key doesn't exist
	// refreshPresence should fall back to ONLINE
	err := core.refreshPresence(ctx, userID, sessionID)
	if err != nil {
		t.Fatalf("refreshPresence failed: %v", err)
	}

	status, err := core.GetUserPresence(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPresence failed: %v", err)
	}
	if status != PresenceStatusOnline {
		t.Errorf("Expected ONLINE as fallback, got %q", status)
	}
}

func TestChattoCore_MultiplePresenceSessionsAggregate(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-multi-session"
	if err := core.SetPresence(ctx, userID, "tab-1", PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence tab-1: %v", err)
	}
	if err := core.SetPresence(ctx, userID, "tab-2", PresenceStatusOnline); err != nil {
		t.Fatalf("SetPresence tab-2: %v", err)
	}

	status, err := core.GetUserPresence(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPresence failed: %v", err)
	}
	if status != PresenceStatusOnline {
		t.Fatalf("Expected ONLINE with AWAY+ONLINE sessions, got %q", status)
	}

	if err := core.SetPresence(ctx, userID, "tab-3", PresenceStatusDoNotDisturb); err != nil {
		t.Fatalf("SetPresence tab-3: %v", err)
	}
	status, err = core.GetUserPresence(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPresence failed: %v", err)
	}
	if status != PresenceStatusDoNotDisturb {
		t.Fatalf("Expected DO_NOT_DISTURB with DND session, got %q", status)
	}

	if err := core.storage.memoryCacheKV.Delete(ctx, presenceSessionKey(userID, "tab-3")); err != nil {
		t.Fatalf("Delete tab-3: %v", err)
	}
	status, err = core.GetUserPresence(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPresence failed: %v", err)
	}
	if status != PresenceStatusOnline {
		t.Fatalf("Expected ONLINE after deleting DND session, got %q", status)
	}
}

func TestChattoCore_MultipleUsersPresence(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create multiple users
	users := make([]string, 5)
	for i := 0; i < 5; i++ {
		user, err := core.CreateUser(ctx, "system",
			"multiuser"+string(rune('0'+i)),
			"Multi User "+string(rune('0'+i)),
			"password123")
		if err != nil {
			t.Fatalf("Failed to create user %d: %v", i, err)
		}
		users[i] = user.Id
	}

	// Set different presence statuses
	statuses := []string{
		PresenceStatusOnline,
		PresenceStatusAway,
		PresenceStatusDoNotDisturb,
		PresenceStatusOnline,
		PresenceStatusAway,
	}

	for i, userID := range users {
		err := core.SetPresence(ctx, userID, "session-1", statuses[i])
		if err != nil {
			t.Fatalf("Failed to set presence for user %d: %v", i, err)
		}
	}

	// Verify all statuses are correct
	for i, userID := range users {
		status, err := core.GetUserPresence(ctx, userID)
		if err != nil {
			t.Fatalf("Failed to get presence for user %d: %v", i, err)
		}
		if status != statuses[i] {
			t.Errorf("User %d: expected %q, got %q", i, statuses[i], status)
		}
	}
}
