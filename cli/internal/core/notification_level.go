package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Notification Level Operations
//
// Notification levels control how a user receives notifications for a space
// or room. Stored as protobuf blobs in the space's CONFIG KV bucket.
//
// Keys:
//   - "user_preferences.{userId}" → SpaceUserPreferences proto
//   - "room_user_preferences.{userId}.{roomId}" → RoomUserPreferences proto
//
// Inheritance: room-level → space-level → NORMAL (system default).
// ============================================================================

// spaceUserPreferencesKey returns the KV key for a user's space-level preferences.
func spaceUserPreferencesKey(userID string) string {
	return "user_preferences." + userID
}

// roomUserPreferencesKey returns the KV key for a user's room-level preferences.
func roomUserPreferencesKey(userID, roomID string) string {
	return "room_user_preferences." + userID + "." + roomID
}

// GetSpaceNotificationLevel returns the user's notification level for a space.
// Returns NOTIFICATION_LEVEL_DEFAULT if no preference is set.
// Authorization: Caller must verify access (self-only in GraphQL layer).
func (c *ChattoCore) GetSpaceNotificationLevel(ctx context.Context, spaceID, userID string) (corev1.NotificationLevel, error) {
	kv := c.storage.serverConfigKV

	entry, err := kv.Get(ctx, spaceUserPreferencesKey(userID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, nil
		}
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, fmt.Errorf("failed to get space user preferences: %w", err)
	}

	prefs := &corev1.SpaceUserPreferences{}
	if err := proto.Unmarshal(entry.Value(), prefs); err != nil {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, fmt.Errorf("failed to unmarshal space user preferences: %w", err)
	}

	return prefs.NotificationLevel, nil
}

// SetSpaceNotificationLevel sets the user's notification level for a space.
// Pass NOTIFICATION_LEVEL_DEFAULT to clear the override (delete the key).
// Authorization: Caller must verify access (self-only + space membership in GraphQL layer).
func (c *ChattoCore) SetSpaceNotificationLevel(ctx context.Context, spaceID, userID string, level corev1.NotificationLevel) error {
	kv := c.storage.serverConfigKV

	key := spaceUserPreferencesKey(userID)

	if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		// Clear override by deleting the key
		if err := kv.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return fmt.Errorf("failed to delete space user preferences: %w", err)
		}
	} else {
		data, err := proto.Marshal(&corev1.SpaceUserPreferences{NotificationLevel: level})
		if err != nil {
			return fmt.Errorf("failed to marshal space user preferences: %w", err)
		}
		if _, err := kv.Put(ctx, key, data); err != nil {
			return fmt.Errorf("failed to set space user preferences: %w", err)
		}
	}

	c.logger.Info("Set space notification level", "space_id", spaceID, "user_id", userID, "level", level)

	// Publish live event for multi-tab sync
	effectiveLevel := level
	if effectiveLevel == corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		effectiveLevel = corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	}
	c.publishNotificationLevelChangedEvent(ctx, userID, spaceID, "", level, effectiveLevel)

	return nil
}

// GetRoomNotificationLevel returns the user's notification level for a room.
// Returns NOTIFICATION_LEVEL_DEFAULT if no preference is set.
// Authorization: Caller must verify access (self-only in GraphQL layer).
func (c *ChattoCore) GetRoomNotificationLevel(ctx context.Context, spaceID, userID, roomID string) (corev1.NotificationLevel, error) {
	kv := c.storage.serverConfigKV

	entry, err := kv.Get(ctx, roomUserPreferencesKey(userID, roomID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, nil
		}
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, fmt.Errorf("failed to get room user preferences: %w", err)
	}

	prefs := &corev1.RoomUserPreferences{}
	if err := proto.Unmarshal(entry.Value(), prefs); err != nil {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, fmt.Errorf("failed to unmarshal room user preferences: %w", err)
	}

	return prefs.NotificationLevel, nil
}

// SetRoomNotificationLevel sets the user's notification level for a room.
// Pass NOTIFICATION_LEVEL_DEFAULT to clear the override (delete the key).
// Authorization: Caller must verify access (self-only + room membership in GraphQL layer).
func (c *ChattoCore) SetRoomNotificationLevel(ctx context.Context, spaceID, userID, roomID string, level corev1.NotificationLevel) error {
	kv := c.storage.serverConfigKV

	key := roomUserPreferencesKey(userID, roomID)

	if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		// Clear override by deleting the key
		if err := kv.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return fmt.Errorf("failed to delete room user preferences: %w", err)
		}
	} else {
		data, err := proto.Marshal(&corev1.RoomUserPreferences{NotificationLevel: level})
		if err != nil {
			return fmt.Errorf("failed to marshal room user preferences: %w", err)
		}
		if _, err := kv.Put(ctx, key, data); err != nil {
			return fmt.Errorf("failed to set room user preferences: %w", err)
		}
	}

	c.logger.Info("Set room notification level", "space_id", spaceID, "room_id", roomID, "user_id", userID, "level", level)

	// Resolve effective level for the live event
	effectiveLevel, err := c.resolveEffectiveNotificationLevel(ctx, spaceID, userID, level)
	if err != nil {
		// If we can't resolve, use the level itself as effective
		c.logger.Warn("Failed to resolve effective notification level", "error", err)
		effectiveLevel = level
	}
	c.publishNotificationLevelChangedEvent(ctx, userID, spaceID, roomID, level, effectiveLevel)

	return nil
}

// GetEffectiveNotificationLevel resolves the effective notification level for a user
// in a room. Resolution order: room-level → space-level → NORMAL (system default).
// Authorization: Caller must verify access.
func (c *ChattoCore) GetEffectiveNotificationLevel(ctx context.Context, spaceID, userID, roomID string) (corev1.NotificationLevel, error) {
	// Check room-level first
	roomLevel, err := c.GetRoomNotificationLevel(ctx, spaceID, userID, roomID)
	if err != nil {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, fmt.Errorf("failed to get room notification level: %w", err)
	}
	if roomLevel != corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		return roomLevel, nil
	}

	// Fall back to space-level
	spaceLevel, err := c.GetSpaceNotificationLevel(ctx, spaceID, userID)
	if err != nil {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, fmt.Errorf("failed to get space notification level: %w", err)
	}
	if spaceLevel != corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		return spaceLevel, nil
	}

	// System default
	return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, nil
}

// resolveEffectiveNotificationLevel resolves the effective notification level
// when the room-level is given. Used after setting a room-level preference
// to compute the effective level for the live event.
func (c *ChattoCore) resolveEffectiveNotificationLevel(ctx context.Context, spaceID, userID string, roomLevel corev1.NotificationLevel) (corev1.NotificationLevel, error) {
	if roomLevel != corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		return roomLevel, nil
	}

	// Room level is DEFAULT, fall back to space level
	spaceLevel, err := c.GetSpaceNotificationLevel(ctx, spaceID, userID)
	if err != nil {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, err
	}
	if spaceLevel != corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		return spaceLevel, nil
	}

	return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, nil
}

// RoomNotificationPreference holds a resolved notification preference for a single room.
type RoomNotificationPreference struct {
	SpaceID        string
	RoomID         string
	Level          corev1.NotificationLevel
	EffectiveLevel corev1.NotificationLevel
}

// GetAllRoomNotificationPreferences returns notification preferences for all rooms the user
// has joined across the given spaces. For each room, both the explicit level and the
// effective level (resolved through space/system defaults) are returned.
// Authorization: Caller must verify self-only access.
func (c *ChattoCore) GetAllRoomNotificationPreferences(ctx context.Context, userID string, spaceIDs []string) ([]RoomNotificationPreference, error) {
	var result []RoomNotificationPreference

	for _, spaceID := range spaceIDs {
		// Get all room memberships for this user in this space
		memberships, err := c.GetUserRoomMemberships(ctx, spaceID, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to get room memberships for space %s: %w", spaceID, err)
		}

		if len(memberships) == 0 {
			continue
		}

		// Get the space-level notification preference once (shared across all rooms)
		spaceLevel, err := c.GetSpaceNotificationLevel(ctx, spaceID, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to get space notification level for space %s: %w", spaceID, err)
		}

		// Get the config bucket once for all room lookups in this space
		kv := c.storage.serverConfigKV

		for _, m := range memberships {
			// Get room-level preference directly from KV (avoids re-opening the bucket)
			roomLevel := corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT
			entry, err := kv.Get(ctx, roomUserPreferencesKey(userID, m.RoomId))
			if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
				return nil, fmt.Errorf("failed to get room preference for room %s: %w", m.RoomId, err)
			}
			if err == nil {
				prefs := &corev1.RoomUserPreferences{}
				if err := proto.Unmarshal(entry.Value(), prefs); err != nil {
					return nil, fmt.Errorf("failed to unmarshal room preference for room %s: %w", m.RoomId, err)
				}
				roomLevel = prefs.NotificationLevel
			}

			// Resolve effective level: room → space → NORMAL
			effectiveLevel := roomLevel
			if effectiveLevel == corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
				effectiveLevel = spaceLevel
			}
			if effectiveLevel == corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
				effectiveLevel = corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
			}

			result = append(result, RoomNotificationPreference{
				SpaceID:        spaceID,
				RoomID:         m.RoomId,
				Level:          roomLevel,
				EffectiveLevel: effectiveLevel,
			})
		}
	}

	return result, nil
}

// deleteUserNotificationLevels removes all notification level preferences for a user
// in a space. Called during space leave or account deletion. Best-effort.
func (c *ChattoCore) deleteUserNotificationLevels(ctx context.Context, spaceID, userID string) error {
	kv := c.storage.serverConfigKV

	// List all room-level preference keys for this user
	prefix := "room_user_preferences." + userID
	keyLister, err := kv.ListKeysFiltered(ctx, prefix+".>")
	if err != nil && !errors.Is(err, jetstream.ErrNoKeysFound) {
		return fmt.Errorf("failed to list room user preferences keys: %w", err)
	}

	// Delete room-level keys
	if keyLister != nil {
		for key := range keyLister.Keys() {
			if err := kv.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
				c.logger.Warn("Failed to delete room user preferences key", "key", key, "error", err)
			}
		}
	}

	// Delete the space-level key
	if err := kv.Delete(ctx, spaceUserPreferencesKey(userID)); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		c.logger.Warn("Failed to delete space user preferences key", "user_id", userID, "space_id", spaceID, "error", err)
	}

	return nil
}

// publishNotificationLevelChangedEvent publishes a live event when a notification level changes.
// User-scoped: only delivered to the user who changed their preference.
func (c *ChattoCore) publishNotificationLevelChangedEvent(ctx context.Context, userID, spaceID, roomID string, level, effectiveLevel corev1.NotificationLevel) {
	event := newInstanceEvent(userID, &corev1.InstanceEvent{
		Event: &corev1.InstanceEvent_NotificationLevelChanged{
			NotificationLevelChanged: &corev1.NotificationLevelChangedEvent{
				SpaceId:        spaceID,
				RoomId:         roomID,
				Level:          level,
				EffectiveLevel: effectiveLevel,
			},
		},
	})

	subject := subjects.LiveInstanceUserEvent(userID, "notification_level_changed")
	if err := c.publishInstanceEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish notification level changed event", "error", err, "user_id", userID, "space_id", spaceID)
	}
}
