package core

import (
	"context"
	"fmt"
	"time"

	"hmans.de/chatto/internal/core/subjects"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
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

// GetUserSettings retrieves a user's settings from the config projection.
// Returns nil, nil if no settings have been saved yet (the user hasn't configured any).
// Authorization: Caller must verify access (self-only in GraphQL layer).
func (c *ChattoCore) GetUserSettings(_ context.Context, userID string) (*corev1.ServerUserPreferences, error) {
	if c.ServerConfig == nil {
		return nil, nil
	}
	settings, _, err := c.ServerConfig.UserSettings(userID)
	return settings, err
}

// UpdateUserSettings merges the provided fields into the user's existing settings.
// Nil fields in the input are ignored (not cleared).
// To clear the timezone override, pass a pointer to an empty string.
// Authorization: Caller must verify access (self-only in GraphQL layer).
func (c *ChattoCore) UpdateUserSettings(ctx context.Context, userID string, input UserSettingsInput) (*corev1.ServerUserPreferences, error) {
	if c.configManager == nil || c.configManager.service == nil {
		return nil, fmt.Errorf("config service not configured")
	}

	settings, err := c.GetUserSettings(ctx, userID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = &corev1.ServerUserPreferences{}
	}

	var writes []configWrite
	var clears []string

	if input.Timezone != nil {
		tz := *input.Timezone
		if tz != "" {
			if _, err := time.LoadLocation(tz); err != nil {
				return nil, fmt.Errorf("invalid timezone %q: %w", tz, err)
			}
			settings.Timezone = &tz
			writes = append(writes, mustConfigWrite(ConfigPathUserTimezone.Name, configStringValue(tz)))
		} else {
			settings.Timezone = nil
			clears = append(clears, ConfigPathUserTimezone.Name)
		}
	}

	if input.TimeFormat != nil {
		settings.TimeFormat = *input.TimeFormat
		writes = append(writes, mustConfigWrite(ConfigPathUserTimeFormat.Name, configIntValue(int64(*input.TimeFormat))))
	}

	if len(writes) > 0 {
		if err := c.configManager.service.setValues(ctx, userID, userID, writes); err != nil {
			return nil, fmt.Errorf("failed to store user settings: %w", err)
		}
	}
	if len(clears) > 0 {
		if err := c.configManager.service.clearPaths(ctx, userID, userID, clears); err != nil {
			return nil, fmt.Errorf("failed to clear user settings: %w", err)
		}
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
	if c.configManager == nil || c.configManager.service == nil {
		return nil
	}
	return c.configManager.service.clearPaths(ctx, SystemActorID, userID, []string{
		ConfigPathUserTimezone.Name,
		ConfigPathUserTimeFormat.Name,
	})
}

func mustConfigWrite(path string, value *configv1.ConfigValue) configWrite {
	return configWrite{path: path, value: value}
}
