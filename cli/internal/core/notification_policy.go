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

var notificationPolicyKinds = []corev1.NotificationPolicyKind{
	corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MESSAGE,
	corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MENTION,
	corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REPLY,
	corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_ROLE_MENTION,
	corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_HERE,
	corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_ALL,
	corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_THREAD,
	corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_ROOM,
	corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REACTION,
}

// NotificationPolicyPreference is one signal kind's explicit and effective policy
// at server scope and, when RoomID is non-empty, room scope.
type NotificationPolicyPreference struct {
	RoomID          string
	Kind            corev1.NotificationPolicyKind
	ServerIntensity corev1.NotificationDeliveryIntensity
	RoomIntensity   corev1.NotificationDeliveryIntensity
	Effective       corev1.NotificationDeliveryIntensity
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

func defaultNotificationIntensity(kind corev1.NotificationPolicyKind) corev1.NotificationDeliveryIntensity {
	switch kind {
	case corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_THREAD,
		corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REACTION:
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE
	case corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_ROOM:
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF
	case corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MESSAGE,
		corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MENTION,
		corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REPLY,
		corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_ROLE_MENTION,
		corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_HERE,
		corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_ALL:
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT
	default:
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
	}
}

func validNotificationPolicyKind(kind corev1.NotificationPolicyKind) bool {
	return defaultNotificationIntensity(kind) != corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
}

func validNotificationIntensity(intensity corev1.NotificationDeliveryIntensity, allowInherit bool) bool {
	if allowInherit && intensity == corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED {
		return true
	}
	return intensity >= corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF &&
		intensity <= corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT
}

func (cm *ConfigModel) notificationServerIntensity(userID string, kind corev1.NotificationPolicyKind) corev1.NotificationDeliveryIntensity {
	if cm == nil || cm.config.Projection() == nil {
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
	}
	cm.config.Projection().RLock()
	defer cm.config.Projection().RUnlock()
	u := cm.config.Projection().users[userID]
	if u == nil {
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
	}
	return u.serverIntensityByKind[kind]
}

func (cm *ConfigModel) notificationRoomIntensity(userID, roomID string, kind corev1.NotificationPolicyKind) corev1.NotificationDeliveryIntensity {
	if cm == nil || cm.config.Projection() == nil {
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
	}
	cm.config.Projection().RLock()
	defer cm.config.Projection().RUnlock()
	u := cm.config.Projection().users[userID]
	if u == nil {
		return corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
	}
	return u.roomIntensityByRoomAndKind[roomID][kind]
}

// GetEffectiveNotificationIntensity resolves room override, then server
// override, then the product default for one signal class.
func (c *ChattoCore) GetEffectiveNotificationIntensity(userID, roomID string, kind corev1.NotificationPolicyKind) corev1.NotificationDeliveryIntensity {
	if roomID != "" {
		if intensity := c.configModel.notificationRoomIntensity(userID, roomID, kind); intensity != corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED {
			return intensity
		}
	}
	if intensity := c.configModel.notificationServerIntensity(userID, kind); intensity != corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED {
		return intensity
	}
	return defaultNotificationIntensity(kind)
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

func (c *ChattoCore) setServerNotificationIntensity(ctx context.Context, userID string, kind corev1.NotificationPolicyKind, intensity corev1.NotificationDeliveryIntensity) error {
	if !validNotificationPolicyKind(kind) || !validNotificationIntensity(intensity, true) {
		return invalidArgument("invalid notification policy kind or delivery intensity")
	}
	return c.configModel.updateSubject(ctx, userID, func(_ evtstream.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		if c.configModel.notificationServerIntensity(userID, kind) == intensity {
			return nil, nil
		}
		return []*corev1.Event{newEvent(userID, &corev1.Event{Event: &corev1.Event_UserNotificationPreferenceChanged{
			UserNotificationPreferenceChanged: &corev1.UserNotificationPreferenceChangedEvent{UserId: userID, Kind: kind, Intensity: intensity},
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
	result := make([]NotificationPolicyPreference, 0, len(notificationPolicyKinds))
	for _, kind := range notificationPolicyKinds {
		result = append(result, NotificationPolicyPreference{
			RoomID:          roomID,
			Kind:            kind,
			ServerIntensity: s.core.configModel.notificationServerIntensity(actorID, kind),
			RoomIntensity:   s.core.configModel.notificationRoomIntensity(actorID, roomID, kind),
			Effective:       s.core.GetEffectiveNotificationIntensity(actorID, roomID, kind),
		})
	}
	return result, nil
}

func (s *NotificationPolicyModel) SetServerNotificationIntensity(ctx context.Context, actorID string, kind corev1.NotificationPolicyKind, intensity corev1.NotificationDeliveryIntensity) ([]NotificationPolicyPreference, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.core.setServerNotificationIntensity(ctx, actorID, kind, intensity); err != nil {
		return nil, fmt.Errorf("set server notification preference: %w", err)
	}
	return s.GetNotificationPolicy(ctx, actorID, "")
}

func (s *NotificationPolicyModel) SetRoomNotificationIntensity(ctx context.Context, actorID, roomID string, kind corev1.NotificationPolicyKind, intensity corev1.NotificationDeliveryIntensity) ([]NotificationPolicyPreference, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if !validNotificationPolicyKind(kind) || !validNotificationIntensity(intensity, true) {
		return nil, invalidArgument("invalid notification policy kind or delivery intensity")
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
		if s.core.configModel.notificationRoomIntensity(actorID, roomID, kind) == intensity {
			return s.GetNotificationPolicy(ctx, actorID, roomID)
		}
		event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserNotificationPreferenceChanged{
			UserNotificationPreferenceChanged: &corev1.UserNotificationPreferenceChangedEvent{
				UserId: actorID, RoomId: &roomID, Kind: kind, Intensity: intensity,
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
