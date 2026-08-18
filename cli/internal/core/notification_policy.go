package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

var notificationPreferenceCategories = []corev1.NotificationPreferenceCategory{
	corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MESSAGE,
	corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MENTION,
	corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REPLY,
	corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_ROLE_MENTION,
	corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_HERE,
	corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_ALL,
	corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_THREAD,
	corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_ROOM,
	corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION,
}

// NotificationPolicyPreference is one category's override and effective policy
// at the requested server or room scope.
type NotificationPolicyPreference struct {
	RoomID    string
	Category  corev1.NotificationPreferenceCategory
	Override  *corev1.NotificationDeliveryMode
	Effective corev1.NotificationDeliveryMode
}

// NotificationPolicyModel owns authenticated Notifications 2.0 policy reads
// and writes. Legacy notification-level events are replay-decodable only and
// are deliberately not consulted here.
type NotificationPolicyModel struct {
	core *ChattoCore
}

// NotificationPolicy returns the operation-level Notifications 2.0 policy model.
func (c *ChattoCore) NotificationPolicy() *NotificationPolicyModel {
	return c.notificationPolicy
}

func defaultNotificationMode(category corev1.NotificationPreferenceCategory) corev1.NotificationDeliveryMode {
	switch category {
	case corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_THREAD,
		corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION:
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_BADGE
	case corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_ROOM:
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF
	case corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MESSAGE,
		corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MENTION,
		corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REPLY,
		corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_ROLE_MENTION,
		corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_HERE,
		corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_ALL:
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT
	default:
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED
	}
}

func validNotificationPreferenceCategory(category corev1.NotificationPreferenceCategory) bool {
	return defaultNotificationMode(category) != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED
}

func validNotificationMode(mode corev1.NotificationDeliveryMode, allowInherit bool) bool {
	if allowInherit && mode == corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
		return true
	}
	return mode >= corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF &&
		mode <= corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT
}

func (cm *ConfigModel) notificationServerMode(userID string, category corev1.NotificationPreferenceCategory) corev1.NotificationDeliveryMode {
	if cm == nil || cm.config.Projection() == nil {
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED
	}
	cm.config.Projection().RLock()
	defer cm.config.Projection().RUnlock()
	u := cm.config.Projection().users[userID]
	if u == nil {
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED
	}
	return u.serverModeByCategory[category]
}

func (cm *ConfigModel) notificationRoomMode(userID, roomID string, category corev1.NotificationPreferenceCategory) corev1.NotificationDeliveryMode {
	if cm == nil || cm.config.Projection() == nil {
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED
	}
	cm.config.Projection().RLock()
	defer cm.config.Projection().RUnlock()
	u := cm.config.Projection().users[userID]
	if u == nil {
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED
	}
	return u.roomModeByRoomAndCategory[roomID][category]
}

// GetEffectiveNotificationMode resolves room override, then server
// override, then the product default for one signal class.
func (c *ChattoCore) GetEffectiveNotificationMode(userID, roomID string, category corev1.NotificationPreferenceCategory) corev1.NotificationDeliveryMode {
	if roomID != "" {
		if mode := c.configModel.notificationRoomMode(userID, roomID, category); mode != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
			return mode
		}
	}
	if mode := c.configModel.notificationServerMode(userID, category); mode != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
		return mode
	}
	return defaultNotificationMode(category)
}

// waitForCurrentNotificationPolicy makes source-time policy evaluation observe
// every preference fact committed before this attempt captured the config
// boundary. A preference write that overlaps the source command may linearize
// on either side of that capture; every OCC retry captures it again.
func (c *ChattoCore) waitForCurrentNotificationPolicy(ctx context.Context) error {
	position, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.ConfigSubjectFilter())
	if err != nil {
		return fmt.Errorf("capture notification policy boundary: %w", err)
	}
	if position.IsZero() {
		return nil
	}
	if err := c.configModel.waitFor(ctx, position); err != nil {
		return fmt.Errorf("wait for notification policy boundary: %w", err)
	}
	return nil
}

func (c *ChattoCore) setServerNotificationMode(ctx context.Context, userID string, category corev1.NotificationPreferenceCategory, mode corev1.NotificationDeliveryMode) error {
	if !validNotificationPreferenceCategory(category) || !validNotificationMode(mode, true) {
		return invalidArgument("invalid notification preference category or delivery mode")
	}
	return c.configModel.updateSubject(ctx, userID, func(_ evtstream.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		if c.configModel.notificationServerMode(userID, category) == mode {
			return nil, nil
		}
		var override *corev1.NotificationDeliveryMode
		if mode != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
			override = &mode
		}
		return []*corev1.Event{newEvent(userID, &corev1.Event{Event: &corev1.Event_UserNotificationPreferenceChanged{
			UserNotificationPreferenceChanged: &corev1.UserNotificationPreferenceChangedEvent{UserId: userID, Category: category, Override: override},
		}})}, nil
	})
}

// GetNotificationPolicy returns every supported signal class with its explicit and
// effective values. If roomID is set, the actor must be a room member.
func (s *NotificationPolicyModel) GetNotificationPolicy(ctx context.Context, actorID, roomID string) ([]NotificationPolicyPreference, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.core.waitForCurrentNotificationPolicy(ctx); err != nil {
		return nil, err
	}
	if roomID != "" {
		if _, err := s.prepareRoomAccess(ctx, actorID, roomID); err != nil {
			return nil, err
		}
	}
	result := make([]NotificationPolicyPreference, 0, len(notificationPreferenceCategories))
	for _, category := range notificationPreferenceCategories {
		explicit := s.core.configModel.notificationServerMode(actorID, category)
		if roomID != "" {
			explicit = s.core.configModel.notificationRoomMode(actorID, roomID, category)
		}
		var override *corev1.NotificationDeliveryMode
		if explicit != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
			override = &explicit
		}
		result = append(result, NotificationPolicyPreference{
			RoomID: roomID, Category: category, Override: override,
			Effective: s.core.GetEffectiveNotificationMode(actorID, roomID, category),
		})
	}
	return result, nil
}

func (s *NotificationPolicyModel) SetServerNotificationMode(ctx context.Context, actorID string, category corev1.NotificationPreferenceCategory, mode corev1.NotificationDeliveryMode) ([]NotificationPolicyPreference, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.core.setServerNotificationMode(ctx, actorID, category, mode); err != nil {
		return nil, fmt.Errorf("set server notification preference: %w", err)
	}
	return s.GetNotificationPolicy(ctx, actorID, "")
}

func (s *NotificationPolicyModel) SetRoomNotificationMode(ctx context.Context, actorID, roomID string, category corev1.NotificationPreferenceCategory, mode corev1.NotificationDeliveryMode) ([]NotificationPolicyPreference, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if !validNotificationPreferenceCategory(category) || !validNotificationMode(mode, true) {
		return nil, invalidArgument("invalid notification preference category or delivery mode")
	}
	for attempt := 0; attempt < maxConfigUpdateRetries; attempt++ {
		authorizationSeq, err := s.prepareRoomAccess(ctx, actorID, roomID)
		if err != nil {
			return nil, err
		}
		agg, filter, expectedSeq, err := s.core.configModel.prepareSubject(ctx, actorID)
		if err != nil {
			return nil, fmt.Errorf("prepare room notification preference: %w", err)
		}
		if s.core.configModel.notificationRoomMode(actorID, roomID, category) == mode {
			return s.GetNotificationPolicy(ctx, actorID, roomID)
		}
		var override *corev1.NotificationDeliveryMode
		if mode != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
			override = &mode
		}
		event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserNotificationPreferenceChanged{
			UserNotificationPreferenceChanged: &corev1.UserNotificationPreferenceChangedEvent{
				UserId: actorID, RoomId: &roomID, Category: category, Override: override,
			},
		}})
		subject := agg.SubjectFor(event)
		seqs, err := s.core.appendAuthorizationFencedBatch(ctx, actorID, []evtstream.BatchEntry{{
			Subject: subject, Event: event, HasOCC: true, ExpectedSeq: expectedSeq, FilterSubject: filter,
		}}, authorizationSeq)
		if errors.Is(err, events.ErrConflict) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("set room notification preference: %w", err)
		}
		if len(seqs) == 0 {
			return nil, errors.New("room notification preference committed no event")
		}
		if err := s.core.configModel.waitFor(ctx, events.SubjectPosition(subject, seqs[0])); err != nil {
			return nil, fmt.Errorf("wait for room notification preference: %w", err)
		}
		return s.GetNotificationPolicy(ctx, actorID, roomID)
	}
	return nil, ErrConfigConflict
}

// prepareRoomAccess returns the authorization-fence position that must remain
// unchanged through a room-scoped preference write. Capturing it first makes
// the following projection boundaries include every authorization fact it
// represents; a later membership/group/RBAC change conflicts at commit.
func (s *NotificationPolicyModel) prepareRoomAccess(ctx context.Context, actorID, roomID string) (uint64, error) {
	authorizationSeq, err := s.core.authorizationFenceSeq(ctx)
	if err != nil {
		return 0, fmt.Errorf("capture notification policy authorization fence: %w", err)
	}
	roomPosition, err := s.core.EventPublisher.LastSubjectPosition(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		return 0, fmt.Errorf("capture notification policy room boundary: %w", err)
	}
	groupPosition, err := s.core.EventPublisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
	if err != nil {
		return 0, fmt.Errorf("capture notification policy group boundary: %w", err)
	}
	rbacPosition, err := s.core.EventPublisher.LastSubjectPosition(ctx, evtstream.RBACSubjectFilter())
	if err != nil {
		return 0, fmt.Errorf("capture notification policy RBAC boundary: %w", err)
	}
	userPosition, err := s.core.EventPublisher.LastSubjectPosition(ctx, evtstream.UserAggregate(actorID).AllEventsFilter())
	if err != nil {
		return 0, fmt.Errorf("capture notification policy user boundary: %w", err)
	}
	if err := s.core.roomModel.waitForDirectory(ctx, roomPosition); err != nil {
		return 0, fmt.Errorf("wait for notification policy room boundary: %w", err)
	}
	if err := s.core.roomModel.waitForGroupLayout(ctx, groupPosition); err != nil {
		return 0, fmt.Errorf("wait for notification policy group boundary: %w", err)
	}
	if err := s.core.rbacModel.waitFor(ctx, rbacPosition); err != nil {
		return 0, fmt.Errorf("wait for notification policy RBAC boundary: %w", err)
	}
	if err := s.core.userModel.waitForUsers(ctx, userPosition); err != nil {
		return 0, fmt.Errorf("wait for notification policy user boundary: %w", err)
	}
	if err := s.requireRoomMember(ctx, actorID, roomID); err != nil {
		return 0, err
	}
	return authorizationSeq, nil
}

func (s *NotificationPolicyModel) requireRoomMember(ctx context.Context, actorID, roomID string) error {
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
