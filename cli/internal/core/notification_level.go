package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Notification Level Operations
//
// Notification levels control how a user receives notifications for the server
// or for a specific room. They are stored as semantic user config events on the
// user's config aggregate.
//
// Legacy SERVER_CONFIG keys are imported by MigrateNotificationPreferencesToES:
//   - "user_preferences.{userId}" → UserPreferences proto
//   - "room_user_preferences.{userId}.{roomId}" → RoomUserPreferences proto
//
// Inheritance: room-level → server-level → NORMAL (system default).
// ============================================================================

// spaceUserPreferencesKey returns the legacy KV key for a user's server-level
// notification preference. Kept for tests and the boot migration.
func spaceUserPreferencesKey(userID string) string {
	return "user_preferences." + userID
}

// roomUserPreferencesKey returns the legacy KV key for a user's room-level
// notification preference. Kept for tests and the boot migration.
func roomUserPreferencesKey(userID, roomID string) string {
	return "room_user_preferences." + userID + "." + roomID
}

// GetSpaceNotificationLevel returns the user's server-wide notification level.
// Returns NOTIFICATION_LEVEL_UNSPECIFIED if no preference is set.
// Authorization: Caller must verify access (self-only in GraphQL layer).
func (c *ChattoCore) GetSpaceNotificationLevel(_ context.Context, userID string) (corev1.NotificationLevel, error) {
	if c.ServerConfig == nil {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED, nil
	}
	return c.ServerConfig.NotificationServerLevel(userID), nil
}

// SetSpaceNotificationLevel sets the user's server-wide notification level.
// Pass NOTIFICATION_LEVEL_UNSPECIFIED to clear the override.
// Authorization: Caller must verify access (self-only in GraphQL layer).
func (c *ChattoCore) SetSpaceNotificationLevel(ctx context.Context, userID string, level corev1.NotificationLevel) error {
	if c.configManager == nil || c.configManager.service == nil || c.ServerConfig == nil {
		return fmt.Errorf("config service not configured")
	}

	changed := false
	if err := c.configManager.service.updateSubject(ctx, userID, func(_ events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		current := c.ServerConfig.NotificationServerLevel(userID)
		if current == level || (current == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED && level == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED) {
			changed = false
			return nil, nil
		}
		changed = true
		if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
			return []*corev1.Event{newEvent(userID, &corev1.Event{Event: &corev1.Event_UserServerNotificationLevelCleared{
				UserServerNotificationLevelCleared: &corev1.UserServerNotificationLevelClearedEvent{UserId: userID},
			}})}, nil
		}
		return []*corev1.Event{newEvent(userID, &corev1.Event{Event: &corev1.Event_UserServerNotificationLevelSet{
			UserServerNotificationLevelSet: &corev1.UserServerNotificationLevelSetEvent{UserId: userID, Level: level},
		}})}, nil
	}); err != nil {
		return fmt.Errorf("failed to set server notification level: %w", err)
	}
	if !changed {
		return nil
	}

	c.logger.Info("Set server notification level", "user_id", userID, "level", level)

	effectiveLevel := level
	if effectiveLevel == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
		effectiveLevel = corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	}
	c.publishNotificationLevelChangedEvent(ctx, userID, "", level, effectiveLevel)

	return nil
}

// GetRoomNotificationLevel returns the user's notification level for a room.
// Returns NOTIFICATION_LEVEL_UNSPECIFIED if no preference is set.
// Authorization: Caller must verify access (self-only in GraphQL layer).
func (c *ChattoCore) GetRoomNotificationLevel(_ context.Context, userID, roomID string) (corev1.NotificationLevel, error) {
	if c.ServerConfig == nil {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED, nil
	}
	return c.ServerConfig.NotificationRoomLevel(userID, roomID), nil
}

// SetRoomNotificationLevel sets the user's notification level for a room.
// Pass NOTIFICATION_LEVEL_UNSPECIFIED to clear the override.
// Authorization: Caller must verify access (self-only + room membership in GraphQL layer).
func (c *ChattoCore) SetRoomNotificationLevel(ctx context.Context, userID, roomID string, level corev1.NotificationLevel) error {
	if c.configManager == nil || c.configManager.service == nil || c.ServerConfig == nil {
		return fmt.Errorf("config service not configured")
	}

	changed := false
	if err := c.configManager.service.updateSubject(ctx, userID, func(_ events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		current := c.ServerConfig.NotificationRoomLevel(userID, roomID)
		if current == level || (current == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED && level == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED) {
			changed = false
			return nil, nil
		}
		changed = true
		if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
			return []*corev1.Event{newEvent(userID, &corev1.Event{Event: &corev1.Event_UserRoomNotificationLevelCleared{
				UserRoomNotificationLevelCleared: &corev1.UserRoomNotificationLevelClearedEvent{UserId: userID, RoomId: roomID},
			}})}, nil
		}
		return []*corev1.Event{newEvent(userID, &corev1.Event{Event: &corev1.Event_UserRoomNotificationLevelSet{
			UserRoomNotificationLevelSet: &corev1.UserRoomNotificationLevelSetEvent{UserId: userID, RoomId: roomID, Level: level},
		}})}, nil
	}); err != nil {
		return fmt.Errorf("failed to set room notification level: %w", err)
	}
	if !changed {
		return nil
	}

	c.logger.Info("Set room notification level", "room_id", roomID, "user_id", userID, "level", level)

	effectiveLevel, err := c.resolveEffectiveNotificationLevel(ctx, userID, level)
	if err != nil {
		c.logger.Warn("Failed to resolve effective notification level", "error", err)
		effectiveLevel = level
	}
	c.publishNotificationLevelChangedEvent(ctx, userID, roomID, level, effectiveLevel)

	return nil
}

// GetEffectiveNotificationLevel resolves the effective notification level for a
// user in a room. Resolution order: room-level → server-level → NORMAL.
// Authorization: Caller must verify access.
func (c *ChattoCore) GetEffectiveNotificationLevel(ctx context.Context, userID, roomID string) (corev1.NotificationLevel, error) {
	roomLevel, err := c.GetRoomNotificationLevel(ctx, userID, roomID)
	if err != nil {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, fmt.Errorf("failed to get room notification level: %w", err)
	}
	if roomLevel != corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
		return roomLevel, nil
	}

	serverLevel, err := c.GetSpaceNotificationLevel(ctx, userID)
	if err != nil {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, fmt.Errorf("failed to get server notification level: %w", err)
	}
	if serverLevel != corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
		return serverLevel, nil
	}

	return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, nil
}

// resolveEffectiveNotificationLevel resolves the effective notification level
// when the room-level value is known.
func (c *ChattoCore) resolveEffectiveNotificationLevel(ctx context.Context, userID string, roomLevel corev1.NotificationLevel) (corev1.NotificationLevel, error) {
	if roomLevel != corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
		return roomLevel, nil
	}

	serverLevel, err := c.GetSpaceNotificationLevel(ctx, userID)
	if err != nil {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, err
	}
	if serverLevel != corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
		return serverLevel, nil
	}

	return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, nil
}

// RoomNotificationPreference holds a resolved notification preference for a
// single room.
type RoomNotificationPreference struct {
	SpaceID        string
	RoomID         string
	Level          corev1.NotificationLevel
	EffectiveLevel corev1.NotificationLevel
}

// GetAllRoomNotificationPreferences returns notification preferences for all
// rooms the user has joined. For each room, both the explicit level and the
// effective level are returned.
//
// Authorization: Caller must verify self-only access.
func (c *ChattoCore) GetAllRoomNotificationPreferences(ctx context.Context, userID string) ([]RoomNotificationPreference, error) {
	memberships, err := c.GetAllUserRoomMemberships(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room memberships: %w", err)
	}
	if len(memberships) == 0 {
		return nil, nil
	}

	serverLevel, err := c.GetSpaceNotificationLevel(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get server notification level: %w", err)
	}

	result := make([]RoomNotificationPreference, 0, len(memberships))
	for _, m := range memberships {
		roomLevel, err := c.GetRoomNotificationLevel(ctx, userID, m.RoomId)
		if err != nil {
			return nil, fmt.Errorf("failed to get room preference for room %s: %w", m.RoomId, err)
		}

		effectiveLevel := roomLevel
		if effectiveLevel == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
			effectiveLevel = serverLevel
		}
		if effectiveLevel == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
			effectiveLevel = corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
		}

		result = append(result, RoomNotificationPreference{
			RoomID:         m.RoomId,
			Level:          roomLevel,
			EffectiveLevel: effectiveLevel,
		})
	}

	return result, nil
}

// deleteUserNotificationLevels removes all notification level preferences for a
// user. Called during account deletion. Best-effort.
func (c *ChattoCore) deleteUserNotificationLevels(ctx context.Context, userID string) error {
	if c.configManager == nil || c.configManager.service == nil || c.ServerConfig == nil {
		return nil
	}
	return c.configManager.service.updateSubject(ctx, userID, func(_ events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		var evs []*corev1.Event
		if c.ServerConfig.NotificationServerLevel(userID) != corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
			evs = append(evs, newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_UserServerNotificationLevelCleared{
				UserServerNotificationLevelCleared: &corev1.UserServerNotificationLevelClearedEvent{UserId: userID},
			}}))
		}
		for _, roomID := range c.ServerConfig.NotificationRoomIDs(userID) {
			evs = append(evs, newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_UserRoomNotificationLevelCleared{
				UserRoomNotificationLevelCleared: &corev1.UserRoomNotificationLevelClearedEvent{UserId: userID, RoomId: roomID},
			}}))
		}
		return evs, nil
	})
}

// publishNotificationLevelChangedEvent publishes a live event when a
// notification level changes. User-scoped: only delivered to the user who
// changed their preference.
func (c *ChattoCore) publishNotificationLevelChangedEvent(ctx context.Context, userID, roomID string, level, effectiveLevel corev1.NotificationLevel) {
	event := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_NotificationLevelChanged{
			NotificationLevelChanged: &corev1.NotificationLevelChangedEvent{
				RoomId:         roomID,
				Level:          level,
				EffectiveLevel: effectiveLevel,
			},
		},
	})

	subject := subjects.LiveSyncUserEvent(userID, "notification_level_changed")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish notification level changed event", "error", err, "user_id", userID)
	}
}
