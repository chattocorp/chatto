package core

import (
	"context"
	"fmt"
	"time"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// User Settings Operations
// ============================================================================

// userPreferencesKey returns the KV key for a user's server-level preferences.
func userPreferencesKey(userID string) string {
	return fmt.Sprintf("user_preferences.%s", userID)
}

// UserSettingsInput represents a partial update to user settings.
// Pointer fields: nil = don't change, non-nil = set to this value.
type UserSettingsInput struct {
	// Timezone is an IANA timezone name. nil = no change, pointer to "" = clear override.
	Timezone *string
	// TimeFormat preference. nil = no change.
	TimeFormat *corev1.TimeFormat
}

// GetUserSettings retrieves a user's settings from the user projection.
// Returns nil, nil if no settings have been saved yet (the user hasn't configured any).
// Authorization: Caller must verify access (self-only in GraphQL layer).
func (c *ChattoCore) GetUserSettings(ctx context.Context, userID string) (*corev1.ServerUserPreferences, error) {
	if settings, ok := c.Users.Preferences(userID); ok {
		return settings, nil
	}
	return nil, nil
}

// UpdateUserSettings merges the provided fields into the user's existing settings.
// Nil fields in the input are ignored (not cleared).
// To clear the timezone override, pass a pointer to an empty string.
// Authorization: Caller must verify access (self-only in GraphQL layer).
func (c *ChattoCore) UpdateUserSettings(ctx context.Context, userID string, input UserSettingsInput) (*corev1.ServerUserPreferences, error) {
	// Get current settings (or start fresh if none exist)
	settings, err := c.GetUserSettings(ctx, userID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = &corev1.ServerUserPreferences{}
	}

	// Apply non-nil fields
	if input.Timezone != nil {
		tz := *input.Timezone
		if tz != "" {
			// Validate IANA timezone name
			if _, err := time.LoadLocation(tz); err != nil {
				return nil, fmt.Errorf("invalid timezone %q: %w", tz, err)
			}
			settings.Timezone = &tz
		} else {
			// Clear the timezone override
			settings.Timezone = nil
		}
	}

	if input.TimeFormat != nil {
		settings.TimeFormat = *input.TimeFormat
	}

	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserServerPreferencesChanged{
		UserServerPreferencesChanged: &corev1.UserServerPreferencesChangedEvent{
			UserId:      userID,
			Preferences: settings,
		},
	}})
	if _, err := c.appendUserEvent(ctx, userID, event, "", nil); err != nil {
		return nil, fmt.Errorf("failed to store user settings: %w", err)
	}

	c.logger.Info("Updated user settings", "user_id", userID)

	// Publish live event for multi-tab/multi-device sync
	c.publishServerUserPreferencesUpdatedEvent(ctx, userID, settings)

	return settings, nil
}

// publishServerUserPreferencesUpdatedEvent publishes a live event when preferences change.
// User-scoped: only delivered to the user who changed their preferences.
func (c *ChattoCore) publishServerUserPreferencesUpdatedEvent(ctx context.Context, userID string, settings *corev1.ServerUserPreferences) {
	tz := ""
	if settings.Timezone != nil {
		tz = *settings.Timezone
	}

	event := newEvent(userID, &corev1.Event{
		Event: &corev1.Event_ServerUserPreferencesUpdated{
			ServerUserPreferencesUpdated: &corev1.ServerUserPreferencesUpdatedEvent{
				Timezone:   tz,
				TimeFormat: settings.TimeFormat,
			},
		},
	})

	subject := subjects.LiveUserEvent(userID, "settings_updated")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("failed to publish user settings updated event", "error", err, "user_id", userID)
	}
}

// deleteUserSettings removes a user's settings. Called during account deletion.
func (c *ChattoCore) deleteUserSettings(ctx context.Context, userID string) error {
	return nil
}
