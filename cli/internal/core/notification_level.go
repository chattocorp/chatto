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
// Inheritance: room-level → server-level → NORMAL (system default).
//
// Transitional layering note: the low-level ChattoCore helpers in this file
// predate the ConnectRPC API and mostly perform projected reads, config writes,
// effective-level resolution, and live-event publishing. New user-facing
// transports should enter through NotificationPreferencesModel instead, so
// operation authZ and response shaping do not drift between GraphQL, ConnectRPC,
// and future transports.
// ============================================================================

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
	if c.configManager == nil || c.configManager.model == nil || c.ServerConfig == nil {
		return fmt.Errorf("config model not configured")
	}

	changed := false
	if err := c.configManager.model.updateSubject(ctx, userID, func(_ events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
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

// SetRoomNotificationLevel sets the user's notification level for a room and
// publishes the live invalidation event. Pass NOTIFICATION_LEVEL_UNSPECIFIED to
// clear the override.
//
// This is intentionally a lower-level write helper. It does not verify room
// membership; callers that serve user requests should use
// NotificationPreferencesModel.SetRoomNotificationLevel instead.
func (c *ChattoCore) SetRoomNotificationLevel(ctx context.Context, userID, roomID string, level corev1.NotificationLevel) error {
	if c.configManager == nil || c.configManager.model == nil || c.ServerConfig == nil {
		return fmt.Errorf("config model not configured")
	}

	changed := false
	if err := c.configManager.model.updateSubject(ctx, userID, func(_ events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
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

// NotificationPreferences returns the operation-level model for notification
// preference reads and writes. Transports should use this model rather than
// calling the lower-level ChattoCore helpers directly so authorization and
// response semantics stay shared. NewChattoCore initializes this eagerly so
// concurrent request handlers do not race on first use.
func (c *ChattoCore) NotificationPreferences() *NotificationPreferencesModel {
	return c.notificationPrefs
}

// NotificationPreferencesModel owns user-facing notification preference
// operations. It is intentionally thin for now: the low-level config
// reads/writes already lived on ChattoCore, while membership authZ and response
// shaping used to live in the GraphQL resolver. This model centralizes that
// operation policy so ConnectRPC and GraphQL share it during the transition
// toward model-owned operations.
type NotificationPreferencesModel struct {
	core *ChattoCore
}

// SetRoomNotificationLevel sets the authenticated actor's notification
// preference for a channel room and returns the resolved preference after the
// write. Authorization: actor must be a member of the room.
func (s *NotificationPreferencesModel) SetRoomNotificationLevel(ctx context.Context, actorID, roomID string, level corev1.NotificationLevel) (*RoomNotificationPreference, error) {
	if err := s.requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.requireChannelRoomMember(ctx, actorID, roomID); err != nil {
		return nil, err
	}
	if err := s.core.SetRoomNotificationLevel(ctx, actorID, roomID, level); err != nil {
		return nil, err
	}
	return s.GetRoomNotificationPreference(ctx, actorID, roomID)
}

// GetRoomNotificationPreference returns the authenticated actor's explicit and
// effective preference for a channel room. Authorization: actor must be a
// member of the room.
func (s *NotificationPreferencesModel) GetRoomNotificationPreference(ctx context.Context, actorID, roomID string) (*RoomNotificationPreference, error) {
	if err := s.requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.requireChannelRoomMember(ctx, actorID, roomID); err != nil {
		return nil, err
	}
	level, err := s.core.GetRoomNotificationLevel(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	effectiveLevel, err := s.core.GetEffectiveNotificationLevel(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	return &RoomNotificationPreference{
		RoomID:         roomID,
		Level:          level,
		EffectiveLevel: effectiveLevel,
	}, nil
}

func (s *NotificationPreferencesModel) requireAuthenticatedActor(actorID string) error {
	if actorID == "" {
		return ErrNotAuthenticated
	}
	return nil
}

func (s *NotificationPreferencesModel) requireChannelRoomMember(ctx context.Context, actorID, roomID string) error {
	isMember, err := s.core.RoomMembershipExists(ctx, KindChannel, actorID, roomID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrPermissionDenied
	}
	return nil
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
	if c.configManager == nil || c.configManager.model == nil || c.ServerConfig == nil {
		return nil
	}
	return c.configManager.model.updateSubject(ctx, userID, func(_ events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
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
