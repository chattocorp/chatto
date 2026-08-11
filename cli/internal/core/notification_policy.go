package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var notificationPolicyReasons = []corev1.NotificationReason{
	corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MESSAGE,
	corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
	corev1.NotificationReason_NOTIFICATION_REASON_REPLY,
	corev1.NotificationReason_NOTIFICATION_REASON_ROLE_MENTION,
	corev1.NotificationReason_NOTIFICATION_REASON_HERE,
	corev1.NotificationReason_NOTIFICATION_REASON_ALL,
	corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_THREAD,
	corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM,
	corev1.NotificationReason_NOTIFICATION_REASON_REACTION,
	corev1.NotificationReason_NOTIFICATION_REASON_ROOM_INVITATION,
}

// NotificationPolicyPreference is one cause's explicit and effective policy
// at server scope and, when RoomID is non-empty, room scope.
type NotificationPolicyPreference struct {
	RoomID          string
	Reason          corev1.NotificationReason
	ServerIntensity corev1.NotificationDeliveryIntensity
	RoomIntensity   corev1.NotificationDeliveryIntensity
	Effective       corev1.NotificationDeliveryIntensity
}

func defaultNotificationIntensity(reason corev1.NotificationReason) corev1.NotificationDeliveryIntensity {
	switch reason {
	case corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_THREAD,
		corev1.NotificationReason_NOTIFICATION_REASON_REACTION:
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE
	case corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM:
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF
	case corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MESSAGE,
		corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
		corev1.NotificationReason_NOTIFICATION_REASON_REPLY,
		corev1.NotificationReason_NOTIFICATION_REASON_ROLE_MENTION,
		corev1.NotificationReason_NOTIFICATION_REASON_HERE,
		corev1.NotificationReason_NOTIFICATION_REASON_ALL,
		corev1.NotificationReason_NOTIFICATION_REASON_ROOM_INVITATION:
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT
	default:
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
	}
}

func validNotificationReason(reason corev1.NotificationReason) bool {
	return defaultNotificationIntensity(reason) != corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
}

func validNotificationIntensity(intensity corev1.NotificationDeliveryIntensity, allowInherit bool) bool {
	if allowInherit && intensity == corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED {
		return true
	}
	return intensity >= corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF &&
		intensity <= corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT
}

func (cm *ConfigModel) notificationServerIntensity(userID string, reason corev1.NotificationReason) corev1.NotificationDeliveryIntensity {
	if cm == nil || cm.config.Projection() == nil {
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
	}
	cm.config.Projection().RLock()
	defer cm.config.Projection().RUnlock()
	u := cm.config.Projection().users[userID]
	if u == nil {
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
	}
	return u.serverIntensityByReason[reason]
}

func (cm *ConfigModel) notificationRoomIntensity(userID, roomID string, reason corev1.NotificationReason) corev1.NotificationDeliveryIntensity {
	if cm == nil || cm.config.Projection() == nil {
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
	}
	cm.config.Projection().RLock()
	defer cm.config.Projection().RUnlock()
	u := cm.config.Projection().users[userID]
	if u == nil {
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
	}
	return u.roomIntensityByRoomAndCause[roomID][reason]
}

// GetEffectiveNotificationIntensity resolves room override, then server
// override, then the product default for one cause.
func (c *ChattoCore) GetEffectiveNotificationIntensity(userID, roomID string, reason corev1.NotificationReason) corev1.NotificationDeliveryIntensity {
	if roomID != "" {
		if intensity := c.configModel.notificationRoomIntensity(userID, roomID, reason); intensity != corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED {
			return intensity
		}
		if intensity, set := legacyNotificationLevelIntensity(c.configModel.notificationRoomLevel(userID, roomID), reason); set {
			return intensity
		}
	}
	if intensity := c.configModel.notificationServerIntensity(userID, reason); intensity != corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED {
		return intensity
	}
	if intensity, set := legacyNotificationLevelIntensity(c.configModel.notificationServerLevel(userID), reason); set {
		return intensity
	}
	return defaultNotificationIntensity(reason)
}

// legacyNotificationLevelIntensity keeps the old coarse preference UI as a
// deprecated preset over the 2.0 matrix without translating notification
// records. Explicit per-cause values at the same scope always win.
func legacyNotificationLevelIntensity(level corev1.NotificationLevel, reason corev1.NotificationReason) (corev1.NotificationDeliveryIntensity, bool) {
	switch level {
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED:
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF, true
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL:
		return defaultNotificationIntensity(reason), true
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES:
		if reason == corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM {
			return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT, true
		}
		return defaultNotificationIntensity(reason), true
	default:
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED, false
	}
}

func (c *ChattoCore) setServerNotificationIntensity(ctx context.Context, userID string, reason corev1.NotificationReason, intensity corev1.NotificationDeliveryIntensity) error {
	if !validNotificationReason(reason) || !validNotificationIntensity(intensity, true) {
		return invalidArgument("invalid notification reason or delivery intensity")
	}
	return c.configModel.updateSubject(ctx, userID, func(_ evtstream.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		if c.configModel.notificationServerIntensity(userID, reason) == intensity {
			return nil, nil
		}
		return []*corev1.Event{newEvent(userID, &corev1.Event{Event: &corev1.Event_UserNotificationPreferenceChanged{
			UserNotificationPreferenceChanged: &corev1.UserNotificationPreferenceChangedEvent{UserId: userID, Reason: reason, Intensity: intensity},
		}})}, nil
	})
}

func (c *ChattoCore) setRoomNotificationIntensity(ctx context.Context, userID, roomID string, reason corev1.NotificationReason, intensity corev1.NotificationDeliveryIntensity) error {
	if !validNotificationReason(reason) || !validNotificationIntensity(intensity, true) {
		return invalidArgument("invalid notification reason or delivery intensity")
	}
	return c.configModel.updateSubject(ctx, userID, func(_ evtstream.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		if c.configModel.notificationRoomIntensity(userID, roomID, reason) == intensity {
			return nil, nil
		}
		return []*corev1.Event{newEvent(userID, &corev1.Event{Event: &corev1.Event_UserNotificationPreferenceChanged{
			UserNotificationPreferenceChanged: &corev1.UserNotificationPreferenceChangedEvent{UserId: userID, RoomId: &roomID, Reason: reason, Intensity: intensity},
		}})}, nil
	})
}

// GetNotificationPolicy returns every supported cause with its explicit and
// effective values. If roomID is set, the actor must be a room member.
func (s *NotificationPreferencesModel) GetNotificationPolicy(ctx context.Context, actorID, roomID string) ([]NotificationPolicyPreference, error) {
	if err := s.requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if roomID != "" {
		if err := s.requireRoomMember(ctx, actorID, roomID); err != nil {
			return nil, err
		}
	}
	result := make([]NotificationPolicyPreference, 0, len(notificationPolicyReasons))
	for _, reason := range notificationPolicyReasons {
		result = append(result, NotificationPolicyPreference{
			RoomID:          roomID,
			Reason:          reason,
			ServerIntensity: s.core.configModel.notificationServerIntensity(actorID, reason),
			RoomIntensity:   s.core.configModel.notificationRoomIntensity(actorID, roomID, reason),
			Effective:       s.core.GetEffectiveNotificationIntensity(actorID, roomID, reason),
		})
	}
	return result, nil
}

func (s *NotificationPreferencesModel) SetServerNotificationIntensity(ctx context.Context, actorID string, reason corev1.NotificationReason, intensity corev1.NotificationDeliveryIntensity) ([]NotificationPolicyPreference, error) {
	if err := s.requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.core.setServerNotificationIntensity(ctx, actorID, reason, intensity); err != nil {
		return nil, fmt.Errorf("set server notification preference: %w", err)
	}
	return s.GetNotificationPolicy(ctx, actorID, "")
}

func (s *NotificationPreferencesModel) SetRoomNotificationIntensity(ctx context.Context, actorID, roomID string, reason corev1.NotificationReason, intensity corev1.NotificationDeliveryIntensity) ([]NotificationPolicyPreference, error) {
	if err := s.requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.requireRoomMember(ctx, actorID, roomID); err != nil {
		return nil, err
	}
	if err := s.core.setRoomNotificationIntensity(ctx, actorID, roomID, reason, intensity); err != nil {
		return nil, fmt.Errorf("set room notification preference: %w", err)
	}
	return s.GetNotificationPolicy(ctx, actorID, roomID)
}

func (s *NotificationPreferencesModel) requireRoomMember(ctx context.Context, actorID, roomID string) error {
	room, err := s.core.FindRoomByID(ctx, roomID)
	if err != nil {
		return err
	}
	isMember, err := s.core.RoomMembershipExists(ctx, KindOfRoom(room), actorID, roomID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrPermissionDenied
	}
	return nil
}
