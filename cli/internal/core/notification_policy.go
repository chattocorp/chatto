package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

// NotificationPolicy is the complete explicit and effective policy at one
// server or room scope.
type NotificationPolicy struct {
	RoomID    string
	Overrides *corev1.NotificationDeliveryModes
	Effective *corev1.NotificationDeliveryModes
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

func cloneNotificationDeliveryModes(modes *corev1.NotificationDeliveryModes) *corev1.NotificationDeliveryModes {
	if modes == nil {
		return &corev1.NotificationDeliveryModes{}
	}
	return proto.Clone(modes).(*corev1.NotificationDeliveryModes)
}

func notificationDeliveryModesEmpty(modes *corev1.NotificationDeliveryModes) bool {
	if modes == nil {
		return true
	}
	if len(modes.ProtoReflect().GetUnknown()) > 0 {
		return false
	}
	empty := true
	modes.ProtoReflect().Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool {
		empty = false
		return false
	})
	return empty
}

func validNotificationMode(mode corev1.NotificationDeliveryMode) bool {
	return mode >= corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF &&
		mode <= corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT
}

// notificationModeProducesOccurrence accepts only modes this binary knows how
// to materialize. Unknown future enum values fail closed during version skew
// instead of entering the durable worker's retry loop.
func notificationModeProducesOccurrence(mode corev1.NotificationDeliveryMode) bool {
	switch mode {
	case corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT:
		return true
	default:
		return false
	}
}

func validateNotificationDeliveryModes(modes *corev1.NotificationDeliveryModes) error {
	if modes == nil {
		return nil
	}
	var validationErr error
	modes.ProtoReflect().Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.EnumKind || !validNotificationMode(corev1.NotificationDeliveryMode(value.Enum())) {
			validationErr = invalidArgument(fmt.Sprintf("invalid notification delivery mode for %s", field.Name()))
			return false
		}
		return true
	})
	return validationErr
}

func applyNotificationDeliveryModesPatch(current, patch *corev1.NotificationDeliveryModes, mask *fieldmaskpb.FieldMask) (*corev1.NotificationDeliveryModes, error) {
	if mask == nil || len(mask.GetPaths()) == 0 {
		return nil, invalidArgument("notification policy update mask must select at least one field")
	}
	result := cloneNotificationDeliveryModes(current)
	patchMessage := cloneNotificationDeliveryModes(patch).ProtoReflect()
	resultMessage := result.ProtoReflect()
	fields := resultMessage.Descriptor().Fields()
	seen := make(map[string]struct{}, len(mask.GetPaths()))
	for _, path := range mask.GetPaths() {
		if path == "" || strings.Contains(path, ".") {
			return nil, invalidArgument(fmt.Sprintf("unsupported notification policy field %q", path))
		}
		if _, duplicate := seen[path]; duplicate {
			continue
		}
		seen[path] = struct{}{}
		field := fields.ByName(protoreflect.Name(path))
		if field == nil || field.Kind() != protoreflect.EnumKind || !field.HasPresence() {
			return nil, invalidArgument(fmt.Sprintf("unsupported notification policy field %q", path))
		}
		if patchMessage.Has(field) {
			mode := corev1.NotificationDeliveryMode(patchMessage.Get(field).Enum())
			if !validNotificationMode(mode) {
				return nil, invalidArgument(fmt.Sprintf("invalid notification delivery mode for %s", path))
			}
			resultMessage.Set(field, patchMessage.Get(field))
		} else {
			resultMessage.Clear(field)
		}
	}
	return result, nil
}

func (cm *ConfigModel) notificationServerModes(userID string) *corev1.NotificationDeliveryModes {
	if cm == nil || cm.config.Projection() == nil {
		return &corev1.NotificationDeliveryModes{}
	}
	cm.config.Projection().RLock()
	defer cm.config.Projection().RUnlock()
	u := cm.config.Projection().users[userID]
	if u == nil {
		return &corev1.NotificationDeliveryModes{}
	}
	return cloneNotificationDeliveryModes(u.serverModes)
}

func (cm *ConfigModel) notificationRoomModes(userID, roomID string) *corev1.NotificationDeliveryModes {
	if cm == nil || cm.config.Projection() == nil {
		return &corev1.NotificationDeliveryModes{}
	}
	cm.config.Projection().RLock()
	defer cm.config.Projection().RUnlock()
	u := cm.config.Projection().users[userID]
	if u == nil {
		return &corev1.NotificationDeliveryModes{}
	}
	return cloneNotificationDeliveryModes(u.roomModesByRoom[roomID])
}

func resolvedNotificationMode(room, server *corev1.NotificationDeliveryMode, fallback corev1.NotificationDeliveryMode) corev1.NotificationDeliveryMode {
	if room != nil {
		return *room
	}
	if server != nil {
		return *server
	}
	return fallback
}

func effectiveNotificationDeliveryModes(server, room *corev1.NotificationDeliveryModes) *corev1.NotificationDeliveryModes {
	server = cloneNotificationDeliveryModes(server)
	room = cloneNotificationDeliveryModes(room)
	return &corev1.NotificationDeliveryModes{
		DirectMessages:  resolvedNotificationMode(room.DirectMessages, server.DirectMessages, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT).Enum(),
		DirectMentions:  resolvedNotificationMode(room.DirectMentions, server.DirectMentions, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT).Enum(),
		Replies:         resolvedNotificationMode(room.Replies, server.Replies, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT).Enum(),
		RoleMentions:    resolvedNotificationMode(room.RoleMentions, server.RoleMentions, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT).Enum(),
		HereMentions:    resolvedNotificationMode(room.HereMentions, server.HereMentions, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT).Enum(),
		AllMentions:     resolvedNotificationMode(room.AllMentions, server.AllMentions, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT).Enum(),
		FollowedThreads: resolvedNotificationMode(room.FollowedThreads, server.FollowedThreads, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT).Enum(),
		FollowedRooms:   resolvedNotificationMode(room.FollowedRooms, server.FollowedRooms, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF).Enum(),
		Reactions:       resolvedNotificationMode(room.Reactions, server.Reactions, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT).Enum(),
	}
}

func notificationModeForSignal(modes *corev1.NotificationDeliveryModes, signal *corev1.NotificationSignal) corev1.NotificationDeliveryMode {
	if modes == nil || signal == nil {
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED
	}
	switch signal.GetKind().(type) {
	case *corev1.NotificationSignal_DirectMessageReceived:
		return modes.GetDirectMessages()
	case *corev1.NotificationSignal_DirectMentionReceived:
		return modes.GetDirectMentions()
	case *corev1.NotificationSignal_ReplyReceived:
		return modes.GetReplies()
	case *corev1.NotificationSignal_RoleMentionReceived:
		return modes.GetRoleMentions()
	case *corev1.NotificationSignal_HereMentionReceived:
		return modes.GetHereMentions()
	case *corev1.NotificationSignal_AllMentionReceived:
		return modes.GetAllMentions()
	case *corev1.NotificationSignal_FollowedThreadActivity:
		return modes.GetFollowedThreads()
	case *corev1.NotificationSignal_FollowedRoomActivity:
		return modes.GetFollowedRooms()
	case *corev1.NotificationSignal_ReactionReceived:
		return modes.GetReactions()
	default:
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED
	}
}

// GetEffectiveNotificationModeForSignal resolves room override, then server
// override, then the product default for the rich signal variant.
func (c *ChattoCore) GetEffectiveNotificationModeForSignal(userID, roomID string, signal *corev1.NotificationSignal) corev1.NotificationDeliveryMode {
	return notificationModeForSignal(effectiveNotificationDeliveryModes(
		c.configModel.notificationServerModes(userID),
		c.configModel.notificationRoomModes(userID, roomID),
	), signal)
}

// waitForCurrentNotificationPolicy makes source-time policy evaluation observe
// every policy fact committed before this attempt captured the config boundary.
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

// GetNotificationPolicy returns the explicit and effective modes at one scope.
// If roomID is set, the actor must be a current room member.
func (s *NotificationPolicyModel) GetNotificationPolicy(ctx context.Context, actorID, roomID string) (*NotificationPolicy, error) {
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
	server := s.core.configModel.notificationServerModes(actorID)
	overrides := server
	room := &corev1.NotificationDeliveryModes{}
	if roomID != "" {
		room = s.core.configModel.notificationRoomModes(actorID, roomID)
		overrides = room
	}
	return &NotificationPolicy{
		RoomID: roomID, Overrides: overrides,
		Effective: effectiveNotificationDeliveryModes(server, room),
	}, nil
}

// UpdateNotificationPolicy sparsely sets or clears server- or room-scoped
// overrides. The complete resulting scope is committed as one OCC-protected
// domain fact so multi-field updates are atomic.
func (s *NotificationPolicyModel) UpdateNotificationPolicy(ctx context.Context, actorID, roomID string, patch *corev1.NotificationDeliveryModes, mask *fieldmaskpb.FieldMask) (*NotificationPolicy, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := validateNotificationDeliveryModes(patch); err != nil {
		return nil, err
	}
	if _, err := applyNotificationDeliveryModesPatch(nil, patch, mask); err != nil {
		return nil, err
	}
	if roomID == "" {
		err := s.core.configModel.updateSubject(ctx, actorID, func(_ evtstream.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
			current := s.core.configModel.notificationServerModes(actorID)
			next, err := applyNotificationDeliveryModesPatch(current, patch, mask)
			if err != nil {
				return nil, err
			}
			if proto.Equal(current, next) {
				return nil, nil
			}
			return []*corev1.Event{newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserNotificationPolicyChanged{
				UserNotificationPolicyChanged: &corev1.UserNotificationPolicyChangedEvent{UserId: actorID, Overrides: next},
			}})}, nil
		})
		if err != nil {
			return nil, fmt.Errorf("update server notification policy: %w", err)
		}
		return s.GetNotificationPolicy(ctx, actorID, "")
	}

	for attempt := 0; attempt < maxConfigUpdateRetries; attempt++ {
		authorizationSeq, err := s.prepareRoomAccess(ctx, actorID, roomID)
		if err != nil {
			return nil, err
		}
		agg, filter, expectedSeq, err := s.core.configModel.prepareSubject(ctx, actorID)
		if err != nil {
			return nil, fmt.Errorf("prepare room notification policy: %w", err)
		}
		current := s.core.configModel.notificationRoomModes(actorID, roomID)
		next, err := applyNotificationDeliveryModesPatch(current, patch, mask)
		if err != nil {
			return nil, err
		}
		if proto.Equal(current, next) {
			unchanged, err := s.core.configModel.subjectSequenceUnchanged(ctx, filter, expectedSeq)
			if err != nil {
				return nil, fmt.Errorf("revalidate room notification policy: %w", err)
			}
			if unchanged {
				// Recheck access at the no-op linearization boundary. A no-op has
				// no authorization-fenced append to perform this validation for us.
				if _, err := s.prepareRoomAccess(ctx, actorID, roomID); err != nil {
					return nil, err
				}
				return s.GetNotificationPolicy(ctx, actorID, roomID)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
			}
			continue
		}
		event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserNotificationPolicyChanged{
			UserNotificationPolicyChanged: &corev1.UserNotificationPolicyChangedEvent{
				UserId: actorID, RoomId: &roomID, Overrides: next,
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
			return nil, fmt.Errorf("update room notification policy: %w", err)
		}
		if len(seqs) == 0 {
			return nil, errors.New("room notification policy committed no event")
		}
		if err := s.core.configModel.waitFor(ctx, events.SubjectPosition(subject, seqs[0])); err != nil {
			return nil, fmt.Errorf("wait for room notification policy: %w", err)
		}
		return s.GetNotificationPolicy(ctx, actorID, roomID)
	}
	return nil, ErrConfigConflict
}

// prepareRoomAccess returns the authorization-fence position that must remain
// unchanged through a room-scoped policy write.
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
