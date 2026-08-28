package core

import (
	"hmans.de/chatto/internal/pb/chatto/core/live/v1"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// ============================================================================
// Key Helper Tests
// ============================================================================

func TestUserPreferencesKey(t *testing.T) {
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
			result := userPreferencesKey(tt.userID)
			if result != tt.expected {
				t.Errorf("userPreferencesKey(%q) = %q, want %q", tt.userID, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestChattoCore_GetUserSettings_NoSettings(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Getting settings for a user with no saved settings should return nil
	settings, err := core.GetUserSettings(ctx, "nonexistent-user")
	if err != nil {
		t.Fatalf("GetUserSettings failed: %v", err)
	}
	if settings != nil {
		t.Errorf("Expected nil for user with no settings, got %+v", settings)
	}
}

func TestChattoCore_UpdateUserSettings_SetTimezone(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-tz"
	tz := "America/New_York"

	// Set timezone
	settings, err := core.UpdateUserSettings(ctx, userID, UserSettingsInput{
		Timezone: &tz,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings failed: %v", err)
	}
	if settings.Timezone == nil || *settings.Timezone != tz {
		t.Errorf("Expected timezone %q, got %v", tz, settings.Timezone)
	}

	// Verify it persisted
	settings, err = core.GetUserSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserSettings failed: %v", err)
	}
	if settings == nil {
		t.Fatal("Expected settings, got nil")
	}
	if settings.Timezone == nil || *settings.Timezone != tz {
		t.Errorf("Expected persisted timezone %q, got %v", tz, settings.Timezone)
	}
}

func TestChattoCore_UpdateUserSettings_PublishesProfileOnlyForTimezoneChanges(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "timezone-live", "Timezone Live", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	sub, err := nc.SubscribeSync(subjects.LiveSyncUserEvent(user.GetId(), "profile_updated"))
	if err != nil {
		t.Fatalf("SubscribeSync profile update: %v", err)
	}
	defer sub.Unsubscribe()
	if err := nc.Flush(); err != nil {
		t.Fatalf("Flush subscription: %v", err)
	}

	tz := "Europe/Berlin"
	if _, err := core.UpdateUserSettings(ctx, user.GetId(), UserSettingsInput{Timezone: &tz}); err != nil {
		t.Fatalf("UpdateUserSettings timezone: %v", err)
	}
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("waiting for profile update: %v", err)
	}
	var live livev1.LiveEvent
	if err := proto.Unmarshal(msg.Data, &live); err != nil {
		t.Fatalf("unmarshal profile update: %v", err)
	}
	if got := live.GetUserProfileUpdated().GetTimezone(); got != tz {
		t.Fatalf("profile timezone = %q, want %q", got, tz)
	}

	format := evtv1.TimeFormat_TIME_FORMAT_24H
	if _, err := core.UpdateUserSettings(ctx, user.GetId(), UserSettingsInput{TimeFormat: &format}); err != nil {
		t.Fatalf("UpdateUserSettings time format: %v", err)
	}
	if _, err := core.UpdateUserSettings(ctx, user.GetId(), UserSettingsInput{Timezone: &tz}); err != nil {
		t.Fatalf("UpdateUserSettings timezone no-op: %v", err)
	}
	if msg, err := sub.NextMsg(200 * time.Millisecond); err == nil {
		t.Fatalf("unexpected profile update for private or no-op setting: %s", msg.Subject)
	}
}

func TestChattoCore_UpdateUserSettings_SetTimeFormat(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-tf"

	tests := []struct {
		name     string
		format   evtv1.TimeFormat
		expected evtv1.TimeFormat
	}{
		{"12-hour", evtv1.TimeFormat_TIME_FORMAT_12H, evtv1.TimeFormat_TIME_FORMAT_12H},
		{"24-hour", evtv1.TimeFormat_TIME_FORMAT_24H, evtv1.TimeFormat_TIME_FORMAT_24H},
		{"unspecified", evtv1.TimeFormat_TIME_FORMAT_UNSPECIFIED, evtv1.TimeFormat_TIME_FORMAT_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := core.UpdateUserSettings(ctx, userID, UserSettingsInput{
				TimeFormat: &tt.format,
			})
			if err != nil {
				t.Fatalf("UpdateUserSettings failed: %v", err)
			}
			if settings.TimeFormat != tt.expected {
				t.Errorf("Expected time format %v, got %v", tt.expected, settings.TimeFormat)
			}
		})
	}
}

func TestChattoCore_UpdateUserSettings_PartialUpdate(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-partial"
	tz := "Europe/Berlin"
	format := evtv1.TimeFormat_TIME_FORMAT_24H

	// Set both timezone and time format
	_, err := core.UpdateUserSettings(ctx, userID, UserSettingsInput{
		Timezone:   &tz,
		TimeFormat: &format,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings failed: %v", err)
	}

	// Update only timezone - time format should be preserved
	newTZ := "Asia/Tokyo"
	settings, err := core.UpdateUserSettings(ctx, userID, UserSettingsInput{
		Timezone: &newTZ,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings failed: %v", err)
	}

	if settings.Timezone == nil || *settings.Timezone != newTZ {
		t.Errorf("Expected timezone %q, got %v", newTZ, settings.Timezone)
	}
	if settings.TimeFormat != evtv1.TimeFormat_TIME_FORMAT_24H {
		t.Errorf("Expected time format to be preserved as 24H, got %v", settings.TimeFormat)
	}

	// Update only time format - timezone should be preserved
	newFormat := evtv1.TimeFormat_TIME_FORMAT_12H
	settings, err = core.UpdateUserSettings(ctx, userID, UserSettingsInput{
		TimeFormat: &newFormat,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings failed: %v", err)
	}

	if settings.Timezone == nil || *settings.Timezone != newTZ {
		t.Errorf("Expected timezone to be preserved as %q, got %v", newTZ, settings.Timezone)
	}
	if settings.TimeFormat != evtv1.TimeFormat_TIME_FORMAT_12H {
		t.Errorf("Expected time format 12H, got %v", settings.TimeFormat)
	}
}

func TestChattoCore_UpdateUserSettings_ClearTimezone(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-clear"
	tz := "America/Chicago"

	// Set timezone
	_, err := core.UpdateUserSettings(ctx, userID, UserSettingsInput{
		Timezone: &tz,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings failed: %v", err)
	}

	// Clear timezone by setting to empty string
	empty := ""
	settings, err := core.UpdateUserSettings(ctx, userID, UserSettingsInput{
		Timezone: &empty,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings failed: %v", err)
	}

	if settings.Timezone != nil {
		t.Errorf("Expected nil timezone after clearing, got %v", *settings.Timezone)
	}
}

func TestChattoCore_UpdateUserSettings_InvalidTimezone(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-invalid"
	invalidTZ := "Not/A/Real/Timezone"

	_, err := core.UpdateUserSettings(ctx, userID, UserSettingsInput{
		Timezone: &invalidTZ,
	})
	if err == nil {
		t.Fatal("Expected error for invalid timezone, got nil")
	}
}

func TestChattoCore_DeleteUser_CleansUpSettings(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user with settings
	user, err := core.CreateUser(ctx, "system", "settingsuser", "Settings User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	tz := "Europe/London"
	_, err = core.UpdateUserSettings(ctx, user.Id, UserSettingsInput{
		Timezone: &tz,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings failed: %v", err)
	}

	// Verify settings exist
	settings, err := core.GetUserSettings(ctx, user.Id)
	if err != nil {
		t.Fatalf("GetUserSettings failed: %v", err)
	}
	if settings == nil {
		t.Fatal("Expected settings to exist before deletion")
	}

	// Delete the user (actorID = userID for self-deletion)
	err = core.DeleteUser(ctx, user.Id, user.Id)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// Verify settings are cleaned up
	settings, err = core.GetUserSettings(ctx, user.Id)
	if err != nil {
		t.Fatalf("GetUserSettings failed after deletion: %v", err)
	}
	if settings != nil {
		t.Error("Expected settings to be nil after user deletion")
	}
}
