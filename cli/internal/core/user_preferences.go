package core

import (
	"context"
	"fmt"
	"time"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/events"
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
// Authorization: Caller must verify access before calling this helper.
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
// Authorization: Caller must verify access before calling this helper.
func (c *ChattoCore) UpdateUserSettings(ctx context.Context, userID string, input UserSettingsInput) (*corev1.ServerUserPreferences, error) {
	if c.configManager == nil || c.configManager.model == nil {
		return nil, fmt.Errorf("config model not configured")
	}

	if input.Timezone != nil {
		tz := *input.Timezone
		if tz != "" {
			if _, err := time.LoadLocation(tz); err != nil {
				return nil, fmt.Errorf("invalid timezone %q: %w", tz, err)
			}
		}
	}

	changed := false
	if err := c.configManager.model.updateSubject(ctx, userID, func(_ events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		current, _, err := c.ServerConfig.UserSettings(userID)
		if err != nil {
			return nil, err
		}
		var evs []*corev1.Event
		if input.Timezone != nil {
			tz := *input.Timezone
			if tz == "" {
				if current != nil && current.Timezone != nil {
					evs = append(evs, newEvent(userID, &corev1.Event{Event: &corev1.Event_UserTimezoneCleared{
						UserTimezoneCleared: &corev1.UserTimezoneClearedEvent{UserId: userID},
					}}))
				}
			} else if current == nil || current.GetTimezone() != tz {
				evs = append(evs, newEvent(userID, &corev1.Event{Event: &corev1.Event_UserTimezoneChanged{
					UserTimezoneChanged: &corev1.UserTimezoneChangedEvent{UserId: userID, Timezone: tz},
				}}))
			}
		}
		if input.TimeFormat != nil && (current == nil || current.GetTimeFormat() != *input.TimeFormat) {
			evs = append(evs, newEvent(userID, &corev1.Event{Event: &corev1.Event_UserTimeFormatChanged{
				UserTimeFormatChanged: &corev1.UserTimeFormatChangedEvent{UserId: userID, TimeFormat: *input.TimeFormat},
			}}))
		}
		changed = len(evs) > 0
		return evs, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to store user settings: %w", err)
	}

	settings, err := c.GetUserSettings(ctx, userID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = &corev1.ServerUserPreferences{}
	}
	if !changed {
		return settings, nil
	}

	c.logger.Info("Updated user settings", "user_id", userID)
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

	event := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_ServerUserPreferencesUpdated{
			ServerUserPreferencesUpdated: &corev1.ServerUserPreferencesUpdatedEvent{
				Timezone:   tz,
				TimeFormat: settings.TimeFormat,
			},
		},
	})

	subject := subjects.LiveSyncUserEvent(userID, "settings_updated")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("failed to publish user settings updated event", "error", err, "user_id", userID)
	}
}

// deleteUserSettings removes a user's settings. Called during account deletion.
func (c *ChattoCore) deleteUserSettings(ctx context.Context, userID string) error {
	if c.configManager == nil || c.configManager.model == nil || c.ServerConfig == nil {
		return nil
	}
	return c.configManager.model.updateSubject(ctx, userID, func(_ events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		current, _, err := c.ServerConfig.UserSettings(userID)
		if err != nil || current == nil {
			return nil, err
		}
		evs := []*corev1.Event{
			newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_UserTimezoneCleared{
				UserTimezoneCleared: &corev1.UserTimezoneClearedEvent{UserId: userID},
			}}),
			newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_UserTimeFormatCleared{
				UserTimeFormatCleared: &corev1.UserTimeFormatClearedEvent{UserId: userID},
			}}),
		}
		return evs, nil
	})
}
