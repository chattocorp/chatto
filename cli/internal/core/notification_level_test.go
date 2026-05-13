package core

import (
	"testing"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Key Helper Tests
// ============================================================================

func TestSpaceUserPreferencesKey(t *testing.T) {
	tests := []struct {
		userID   string
		expected string
	}{
		{"user123", "user_preferences.user123"},
		{"abc", "user_preferences.abc"},
		{"a1b2c3d4e5f6g7", "user_preferences.a1b2c3d4e5f6g7"},
	}

	for _, tt := range tests {
		t.Run(tt.userID, func(t *testing.T) {
			result := spaceUserPreferencesKey(tt.userID)
			if result != tt.expected {
				t.Errorf("spaceUserPreferencesKey(%q) = %q, want %q", tt.userID, result, tt.expected)
			}
		})
	}
}

func TestRoomUserPreferencesKey(t *testing.T) {
	tests := []struct {
		userID   string
		roomID   string
		expected string
	}{
		{"user123", "room456", "room_user_preferences.user123.room456"},
		{"abc", "xyz", "room_user_preferences.abc.xyz"},
	}

	for _, tt := range tests {
		t.Run(tt.userID+"_"+tt.roomID, func(t *testing.T) {
			result := roomUserPreferencesKey(tt.userID, tt.roomID)
			if result != tt.expected {
				t.Errorf("roomUserPreferencesKey(%q, %q) = %q, want %q", tt.userID, tt.roomID, result, tt.expected)
			}
		})
	}
}

func TestPrefsProtoRoundTrip(t *testing.T) {
	levels := []corev1.NotificationLevel{
		corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT,
		corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
		corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL,
		corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES,
	}

	t.Run("UserPreferences", func(t *testing.T) {
		for _, level := range levels {
			t.Run(level.String(), func(t *testing.T) {
				prefs := &corev1.UserPreferences{NotificationLevel: level}
				data, err := proto.Marshal(prefs)
				if err != nil {
					t.Fatalf("Marshal failed: %v", err)
				}
				got := &corev1.UserPreferences{}
				if err := proto.Unmarshal(data, got); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				if got.NotificationLevel != level {
					t.Errorf("Round trip failed: %v -> %v", level, got.NotificationLevel)
				}
			})
		}
	})

	t.Run("RoomUserPreferences", func(t *testing.T) {
		for _, level := range levels {
			t.Run(level.String(), func(t *testing.T) {
				prefs := &corev1.RoomUserPreferences{NotificationLevel: level}
				data, err := proto.Marshal(prefs)
				if err != nil {
					t.Fatalf("Marshal failed: %v", err)
				}
				got := &corev1.RoomUserPreferences{}
				if err := proto.Unmarshal(data, got); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				if got.NotificationLevel != level {
					t.Errorf("Round trip failed: %v -> %v", level, got.NotificationLevel)
				}
			})
		}
	})
}

// ============================================================================
// Integration Tests: Space-Level
// ============================================================================

func TestChattoCore_GetSpaceNotificationLevel_NoPreference(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space (needed for config bucket)

	level, err := core.GetSpaceNotificationLevel(ctx, "test-user")
	if err != nil {
		t.Fatalf("GetSpaceNotificationLevel failed: %v", err)
	}
	if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		t.Errorf("Expected DEFAULT for no preference, got %v", level)
	}
}

func TestChattoCore_SetSpaceNotificationLevel(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)


	tests := []struct {
		name     string
		level    corev1.NotificationLevel
		expected corev1.NotificationLevel
	}{
		{"set muted", corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED, corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED},
		{"set normal", corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL},
		{"set all_messages", corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES, corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES},
		{"set default (clears)", corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := core.SetSpaceNotificationLevel(ctx, "test-user", tt.level)
			if err != nil {
				t.Fatalf("SetSpaceNotificationLevel failed: %v", err)
			}

			got, err := core.GetSpaceNotificationLevel(ctx, "test-user")
			if err != nil {
				t.Fatalf("GetSpaceNotificationLevel failed: %v", err)
			}
			if got != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestChattoCore_SetSpaceNotificationLevel_DefaultDeletesKey(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Set to MUTED first
	err := core.SetSpaceNotificationLevel(ctx, "test-user", corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED)
	if err != nil {
		t.Fatalf("SetSpaceNotificationLevel failed: %v", err)
	}

	// Verify it's MUTED
	level, err := core.GetSpaceNotificationLevel(ctx, "test-user")
	if err != nil {
		t.Fatalf("GetSpaceNotificationLevel failed: %v", err)
	}
	if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
		t.Fatalf("Expected MUTED, got %v", level)
	}

	// Set to DEFAULT (should delete the key)
	err = core.SetSpaceNotificationLevel(ctx, "test-user", corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT)
	if err != nil {
		t.Fatalf("SetSpaceNotificationLevel (DEFAULT) failed: %v", err)
	}

	// Verify it returns DEFAULT (key was deleted)
	level, err = core.GetSpaceNotificationLevel(ctx, "test-user")
	if err != nil {
		t.Fatalf("GetSpaceNotificationLevel failed: %v", err)
	}
	if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		t.Errorf("Expected DEFAULT after clearing, got %v", level)
	}
}

// ============================================================================
// Integration Tests: Room-Level
// ============================================================================

func TestChattoCore_GetRoomNotificationLevel_NoPreference(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)


	level, err := core.GetRoomNotificationLevel(ctx, "test-user", "room123")
	if err != nil {
		t.Fatalf("GetRoomNotificationLevel failed: %v", err)
	}
	if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		t.Errorf("Expected DEFAULT for no preference, got %v", level)
	}
}

func TestChattoCore_SetRoomNotificationLevel(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)


	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	tests := []struct {
		name     string
		level    corev1.NotificationLevel
		expected corev1.NotificationLevel
	}{
		{"set muted", corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED, corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED},
		{"set all_messages", corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES, corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES},
		{"set default (clears)", corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := core.SetRoomNotificationLevel(ctx, "test-user", room.Id, tt.level)
			if err != nil {
				t.Fatalf("SetRoomNotificationLevel failed: %v", err)
			}

			got, err := core.GetRoomNotificationLevel(ctx, "test-user", room.Id)
			if err != nil {
				t.Fatalf("GetRoomNotificationLevel failed: %v", err)
			}
			if got != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, got)
			}
		})
	}
}

// ============================================================================
// Integration Tests: Effective Level (Inheritance)
// ============================================================================

func TestChattoCore_GetEffectiveNotificationLevel_Inheritance(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)


	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// No preferences set: should return NORMAL (system default)
	t.Run("no preferences returns NORMAL", func(t *testing.T) {
		level, err := core.GetEffectiveNotificationLevel(ctx, "test-user", room.Id)
		if err != nil {
			t.Fatalf("GetEffectiveNotificationLevel failed: %v", err)
		}
		if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL {
			t.Errorf("Expected NORMAL, got %v", level)
		}
	})

	// Set space-level to MUTED: room should inherit
	t.Run("room inherits from space", func(t *testing.T) {
		err := core.SetSpaceNotificationLevel(ctx, "test-user", corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED)
		if err != nil {
			t.Fatalf("SetSpaceNotificationLevel failed: %v", err)
		}

		level, err := core.GetEffectiveNotificationLevel(ctx, "test-user", room.Id)
		if err != nil {
			t.Fatalf("GetEffectiveNotificationLevel failed: %v", err)
		}
		if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			t.Errorf("Expected MUTED (inherited from space), got %v", level)
		}
	})

	// Set room-level to ALL_MESSAGES: should override space-level
	t.Run("room overrides space", func(t *testing.T) {
		err := core.SetRoomNotificationLevel(ctx, "test-user", room.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES)
		if err != nil {
			t.Fatalf("SetRoomNotificationLevel failed: %v", err)
		}

		level, err := core.GetEffectiveNotificationLevel(ctx, "test-user", room.Id)
		if err != nil {
			t.Fatalf("GetEffectiveNotificationLevel failed: %v", err)
		}
		if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES {
			t.Errorf("Expected ALL_MESSAGES (room override), got %v", level)
		}
	})

	// Clear room-level: should fall back to space-level (MUTED)
	t.Run("room cleared falls back to space", func(t *testing.T) {
		err := core.SetRoomNotificationLevel(ctx, "test-user", room.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT)
		if err != nil {
			t.Fatalf("SetRoomNotificationLevel failed: %v", err)
		}

		level, err := core.GetEffectiveNotificationLevel(ctx, "test-user", room.Id)
		if err != nil {
			t.Fatalf("GetEffectiveNotificationLevel failed: %v", err)
		}
		if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			t.Errorf("Expected MUTED (from space after clearing room), got %v", level)
		}
	})

	// Clear space-level: should fall back to NORMAL
	t.Run("all cleared falls back to NORMAL", func(t *testing.T) {
		err := core.SetSpaceNotificationLevel(ctx, "test-user", corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT)
		if err != nil {
			t.Fatalf("SetSpaceNotificationLevel failed: %v", err)
		}

		level, err := core.GetEffectiveNotificationLevel(ctx, "test-user", room.Id)
		if err != nil {
			t.Fatalf("GetEffectiveNotificationLevel failed: %v", err)
		}
		if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL {
			t.Errorf("Expected NORMAL (system default), got %v", level)
		}
	})
}

// ============================================================================
// Integration Tests: User Isolation
// ============================================================================

func TestChattoCore_NotificationLevel_UserIsolation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Set userA's space level to MUTED
	err := core.SetSpaceNotificationLevel(ctx, "userA", corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED)
	if err != nil {
		t.Fatalf("SetSpaceNotificationLevel failed: %v", err)
	}

	// userB's space level should still be DEFAULT
	level, err := core.GetSpaceNotificationLevel(ctx, "userB")
	if err != nil {
		t.Fatalf("GetSpaceNotificationLevel failed: %v", err)
	}
	if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		t.Errorf("Expected DEFAULT for userB (isolated from userA), got %v", level)
	}
}

// ============================================================================
// Integration Tests: Cleanup
// ============================================================================

func TestChattoCore_DeleteUserNotificationLevels(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)


	room1, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-1", "")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	room2, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-2", "")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// Set space-level and room-level preferences
	err = core.SetSpaceNotificationLevel(ctx, "test-user", corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED)
	if err != nil {
		t.Fatalf("SetSpaceNotificationLevel failed: %v", err)
	}
	err = core.SetRoomNotificationLevel(ctx, "test-user", room1.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES)
	if err != nil {
		t.Fatalf("SetRoomNotificationLevel failed: %v", err)
	}
	err = core.SetRoomNotificationLevel(ctx, "test-user", room2.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL)
	if err != nil {
		t.Fatalf("SetRoomNotificationLevel failed: %v", err)
	}

	// Delete all notification levels
	err = core.deleteUserNotificationLevels(ctx, "test-user")
	if err != nil {
		t.Fatalf("deleteUserNotificationLevels failed: %v", err)
	}

	// Verify all levels are DEFAULT
	level, err := core.GetSpaceNotificationLevel(ctx, "test-user")
	if err != nil {
		t.Fatalf("GetSpaceNotificationLevel failed: %v", err)
	}
	if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		t.Errorf("Expected DEFAULT for space after cleanup, got %v", level)
	}

	level, err = core.GetRoomNotificationLevel(ctx, "test-user", room1.Id)
	if err != nil {
		t.Fatalf("GetRoomNotificationLevel failed: %v", err)
	}
	if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		t.Errorf("Expected DEFAULT for room1 after cleanup, got %v", level)
	}

	level, err = core.GetRoomNotificationLevel(ctx, "test-user", room2.Id)
	if err != nil {
		t.Fatalf("GetRoomNotificationLevel failed: %v", err)
	}
	if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		t.Errorf("Expected DEFAULT for room2 after cleanup, got %v", level)
	}
}

// ============================================================================
// Integration Tests: HasUnread respects mute
// ============================================================================

func TestChattoCore_HasUnread_MutedRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user, space, room, and join
	user, err := core.CreateUser(ctx, "system", "muteduser", "Muted User", "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// Join the room (CreateRoom does NOT auto-join the creator)
	_, err = core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}

	// Create a second user who will post a message
	poster, err := core.CreateUser(ctx, "system", "poster", "Poster User", "password123")
	if err != nil {
		t.Fatalf("CreateUser (poster) failed: %v", err)
	}

	_, err = core.JoinRoom(ctx, poster.Id, KindChannel, poster.Id, room.Id)
	if err != nil {
		t.Fatalf("JoinRoom (poster) failed: %v", err)
	}

	// Post a message from the poster (spaceID, roomID, userID, body, attachments, inThread, inReplyTo)
	_, err = core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "Hello!", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage failed: %v", err)
	}

	// Verify room has unread messages normally
	hasUnread, err := core.HasUnread(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("HasUnread failed: %v", err)
	}
	if !hasUnread {
		t.Error("Expected HasUnread=true before muting")
	}

	// Mute the room
	err = core.SetRoomNotificationLevel(ctx, user.Id, room.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED)
	if err != nil {
		t.Fatalf("SetRoomNotificationLevel failed: %v", err)
	}

	// HasUnread should now return false for muted room
	hasUnread, err = core.HasUnread(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("HasUnread failed: %v", err)
	}
	if hasUnread {
		t.Error("Expected HasUnread=false for muted room")
	}

	// Unmute the room
	err = core.SetRoomNotificationLevel(ctx, user.Id, room.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT)
	if err != nil {
		t.Fatalf("SetRoomNotificationLevel failed: %v", err)
	}

	// HasUnread should return true again
	hasUnread, err = core.HasUnread(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("HasUnread failed: %v", err)
	}
	if !hasUnread {
		t.Error("Expected HasUnread=true after unmuting")
	}
}
