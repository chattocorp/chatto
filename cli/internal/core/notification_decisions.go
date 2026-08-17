package core

import (
	"context"
	"sort"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// notificationRecipientDecision is one exact source-time policy result. A
// recipient can have several decisions for the same source fact because each
// rich notification signal remains independently addressable.
type notificationRecipientDecision struct {
	recipientID string
	kind        corev1.NotificationPolicyKind
	intensity   corev1.NotificationDeliveryIntensity
}

func (c *ChattoCore) buildMessageNotificationDecisions(
	ctx context.Context,
	kind RoomKind,
	roomID, authorID, inThread, inReplyTo string,
	mentions *RoomMentionResolution,
) ([]notificationRecipientDecision, error) {
	kindsByRecipient := make(map[string]map[corev1.NotificationPolicyKind]struct{})
	add := func(userID string, policyKind corev1.NotificationPolicyKind) {
		if userID == "" || userID == authorID || policyKind == corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_UNSPECIFIED {
			return
		}
		if kindsByRecipient[userID] == nil {
			kindsByRecipient[userID] = make(map[corev1.NotificationPolicyKind]struct{})
		}
		kindsByRecipient[userID][policyKind] = struct{}{}
	}

	if mentions != nil {
		for userID, policyKinds := range mentions.ReasonsByUser {
			for _, policyKind := range policyKinds {
				add(userID, policyKind)
			}
		}
	}

	if kind == KindDM {
		members, err := c.GetRoomMembersList(ctx, kind, roomID)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			if member != nil {
				add(member.GetUserId(), corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MESSAGE)
			}
		}
	} else if inThread == "" {
		members, err := c.GetRoomMembersList(ctx, kind, roomID)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			if member != nil {
				add(member.GetUserId(), corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_ROOM)
			}
		}
	}

	if inReplyTo != "" {
		original, err := c.GetRoomEventByEventID(ctx, kind, roomID, inReplyTo)
		if err != nil {
			return nil, err
		}
		if original != nil {
			add(original.GetActorId(), corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REPLY)
		}
	}

	if inThread != "" {
		followers, err := c.GetThreadFollowers(ctx, kind, roomID, inThread)
		if err != nil {
			return nil, err
		}
		for _, followerID := range followers {
			add(followerID, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_THREAD)
		}

		metadata, err := c.GetThreadMetadata(ctx, kind, roomID, inThread)
		if err != nil {
			return nil, err
		}
		if metadata.ReplyCount == 0 {
			root, err := c.GetRoomEventByEventID(ctx, kind, roomID, inThread)
			if err != nil {
				return nil, err
			}
			if root != nil && c.roomModel.threadFollowState(root.GetActorId(), roomID, inThread) == ThreadFollowStateNone {
				add(root.GetActorId(), corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_THREAD)
			}
		}
	}

	recipientIDs := make([]string, 0, len(kindsByRecipient))
	for userID := range kindsByRecipient {
		recipientIDs = append(recipientIDs, userID)
	}
	sort.Strings(recipientIDs)

	decisions := make([]notificationRecipientDecision, 0)
	for _, userID := range recipientIDs {
		policyKinds := make([]corev1.NotificationPolicyKind, 0, len(kindsByRecipient[userID]))
		for policyKind := range kindsByRecipient[userID] {
			policyKinds = append(policyKinds, policyKind)
		}
		sort.Slice(policyKinds, func(i, j int) bool { return policyKinds[i] < policyKinds[j] })
		for _, policyKind := range policyKinds {
			intensity := c.GetEffectiveNotificationIntensity(userID, roomID, policyKind)
			if intensity <= corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF {
				continue
			}
			decisions = append(decisions, notificationRecipientDecision{
				recipientID: userID,
				kind:        policyKind,
				intensity:   intensity,
			})
		}
	}
	return decisions, nil
}

func directMentionRecipients(decisions []notificationRecipientDecision) []string {
	result := make([]string, 0)
	for _, decision := range decisions {
		if decision.kind == corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MENTION &&
			decision.intensity > corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF {
			result = append(result, decision.recipientID)
		}
	}
	return result
}

// newNotificationOccurrenceWork prepares exact signal values for temporary
// RUNTIME_STATE work. The asynchronous materializer never re-evaluates policy.
func newNotificationOccurrenceWork(
	source *corev1.Event,
	message *corev1.NotificationMessageReference,
	decisions []notificationRecipientDecision,
) *corev1.NotificationMaterializationWork {
	if source == nil || source.GetCreatedAt() == nil || message == nil || len(decisions) == 0 {
		return nil
	}
	work := &corev1.NotificationMaterializationWork{}
	for _, decision := range decisions {
		signal := notificationSignalForPolicyKind(decision.kind, message, "")
		if signal == nil {
			continue
		}
		attention := corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
		if decision.kind == corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REACTION {
			attention = corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT
		}
		work.Notifications = append(work.Notifications, &corev1.NotificationOccurrence{
			RecipientId:     decision.recipientID,
			SourceEventId:   source.GetId(),
			SourceCreatedAt: source.GetCreatedAt(),
			ActorId:         source.GetActorId(),
			Signal:          signal,
			Intensity:       decision.intensity,
			AttentionLevel:  attention,
			EvaluatedAt:     source.GetCreatedAt(),
		})
	}
	if len(work.GetNotifications()) == 0 {
		return nil
	}
	return work
}

func (c *ChattoCore) buildReactionNotificationWork(source, target *corev1.Event, roomID, messageEventID string) *corev1.NotificationMaterializationWork {
	if source == nil || target == nil || target.GetActorId() == "" || target.GetActorId() == source.GetActorId() {
		return nil
	}
	intensity := c.GetEffectiveNotificationIntensity(target.GetActorId(), roomID, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REACTION)
	if intensity <= corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF {
		return nil
	}
	message := newNotificationMessageReference(roomID, messageEventID)
	if threadRootEventID := target.GetMessagePosted().GetInThread(); threadRootEventID != "" {
		message.ThreadRootEventId = &threadRootEventID
	}
	work := newNotificationOccurrenceWork(source, message, []notificationRecipientDecision{{
		recipientID: target.GetActorId(),
		kind:        corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REACTION,
		intensity:   intensity,
	}})
	for _, occurrence := range work.GetNotifications() {
		occurrence.Signal.GetReactionReceived().Emoji = source.GetReactionAdded().GetEmoji()
	}
	return work
}

func newNotificationRevocationWork(trigger *corev1.Event, recipientID, sourceEventID string, reason corev1.NotificationRemovalReason) *corev1.NotificationMaterializationWork {
	if trigger == nil || recipientID == "" || sourceEventID == "" {
		return nil
	}
	return &corev1.NotificationMaterializationWork{Revocations: []*corev1.NotificationMaterializationRevocation{{
		RecipientId:   recipientID,
		SourceEventId: sourceEventID,
		Reason:        reason,
	}}}
}
