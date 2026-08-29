package core

import (
	"context"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/live/v1"
	"time"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
	TimeFormat *evtv1.TimeFormat
	// ShareTimezone controls whether the stored time zone is public. nil = no change.
	ShareTimezone *bool
}

// GetUserSettings retrieves a user's settings from the config projection.
// Returns nil, nil if no settings have been saved yet (the user hasn't configured any).
// Authorization: Caller must verify access before calling this helper.
func (c *ChattoCore) GetUserSettings(_ context.Context, userID string) (*evtv1.ServerUserPreferences, error) {
	if c.configModel == nil {
		return nil, nil
	}
	settings, _ := c.configModel.userSettings(userID)
	return settings, nil
}

func (cm *ConfigModel) userSettings(userID string) (*evtv1.ServerUserPreferences, bool) {
	if cm == nil || cm.config.Projection() == nil {
		return nil, false
	}
	cm.config.Projection().RLock()
	defer cm.config.Projection().RUnlock()
	u := cm.config.Projection().users[userID]
	if u == nil || (u.timezone == nil && u.timeFormat == nil && !u.shareTimezone) {
		return nil, false
	}
	prefs := &evtv1.ServerUserPreferences{}
	if u.timezone != nil {
		tz := *u.timezone
		prefs.Timezone = &tz
	}
	if u.timeFormat != nil {
		prefs.TimeFormat = *u.timeFormat
	}
	prefs.ShareTimezone = u.shareTimezone
	return prefs, true
}

// UpdateUserSettings merges the provided fields into the user's existing settings.
// Nil fields in the input are ignored (not cleared).
// To clear the timezone override, pass a pointer to an empty string.
// Authorization: Caller must verify access before calling this helper.
func (c *ChattoCore) UpdateUserSettings(ctx context.Context, userID string, input UserSettingsInput) (*evtv1.ServerUserPreferences, error) {
	if c.configModel == nil {
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
	timezoneChanged := false
	sharingChanged := false
	if err := c.configModel.updateSubject(ctx, userID, func(_ evtstream.Aggregate, _ string, _ uint64) ([]*evtv1.Event, error) {
		changed = false
		timezoneChanged = false
		sharingChanged = false
		current, _ := c.configModel.userSettings(userID)
		var evs []*evtv1.Event
		if input.Timezone != nil {
			tz := *input.Timezone
			if tz == "" {
				if current != nil && current.Timezone != nil {
					evs = append(evs, newEvent(userID, &evtv1.Event{Event: &evtv1.Event_UserTimezoneCleared{
						UserTimezoneCleared: &evtv1.UserTimezoneClearedEvent{UserId: userID},
					}}))
					timezoneChanged = true
				}
			} else if current == nil || current.GetTimezone() != tz {
				evs = append(evs, newEvent(userID, &evtv1.Event{Event: &evtv1.Event_UserTimezoneChanged{
					UserTimezoneChanged: &evtv1.UserTimezoneChangedEvent{UserId: userID, Timezone: tz},
				}}))
				timezoneChanged = true
			}
		}
		if input.TimeFormat != nil && (current == nil || current.GetTimeFormat() != *input.TimeFormat) {
			evs = append(evs, newEvent(userID, &evtv1.Event{Event: &evtv1.Event_UserTimeFormatChanged{
				UserTimeFormatChanged: &evtv1.UserTimeFormatChangedEvent{UserId: userID, TimeFormat: *input.TimeFormat},
			}}))
		}
		currentShareTimezone := false
		if current != nil {
			currentShareTimezone = current.GetShareTimezone()
		}
		if input.ShareTimezone != nil && currentShareTimezone != *input.ShareTimezone {
			evs = append(evs, newEvent(userID, &evtv1.Event{Event: &evtv1.Event_UserTimezoneSharingChanged{
				UserTimezoneSharingChanged: &evtv1.UserTimezoneSharingChangedEvent{
					UserId:        userID,
					ShareTimezone: *input.ShareTimezone,
				},
			}}))
			sharingChanged = true
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
		settings = &evtv1.ServerUserPreferences{}
	}
	if !changed {
		return settings, nil
	}

	c.logger.Info("Updated user settings", "user_id", userID)
	c.publishServerUserPreferencesSync(ctx, userID, settings)
	if sharingChanged || (timezoneChanged && settings.GetShareTimezone()) {
		c.publishUserProfileUpdate(ctx, userID)
	}

	return settings, nil
}

// publishServerUserPreferencesSync publishes a transient signal for the user
// whose preferences changed.
func (c *ChattoCore) publishServerUserPreferencesSync(ctx context.Context, userID string, settings *evtv1.ServerUserPreferences) {
	tz := ""
	if settings.Timezone != nil {
		tz = *settings.Timezone
	}

	event := newLiveEvent(userID, &livev1.LiveEvent{
		Event: &livev1.LiveEvent_ServerUserPreferencesUpdated{
			ServerUserPreferencesUpdated: &livev1.ServerUserPreferencesSyncEvent{
				Timezone:      tz,
				TimeFormat:    livev1.TimeFormat(settings.TimeFormat),
				ShareTimezone: settings.GetShareTimezone(),
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
	if c.configModel == nil {
		return nil
	}
	return c.configModel.updateSubject(ctx, userID, func(_ evtstream.Aggregate, _ string, _ uint64) ([]*evtv1.Event, error) {
		current, _ := c.configModel.userSettings(userID)
		if current == nil {
			return nil, nil
		}
		evs := []*evtv1.Event{
			newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_UserTimezoneCleared{
				UserTimezoneCleared: &evtv1.UserTimezoneClearedEvent{UserId: userID},
			}}),
			newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_UserTimeFormatCleared{
				UserTimeFormatCleared: &evtv1.UserTimeFormatClearedEvent{UserId: userID},
			}}),
		}
		if current.GetShareTimezone() {
			evs = append(evs, newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_UserTimezoneSharingChanged{
				UserTimezoneSharingChanged: &evtv1.UserTimezoneSharingChangedEvent{UserId: userID},
			}}))
		}
		return evs, nil
	})
}
