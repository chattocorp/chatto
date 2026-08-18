package core

import (
	"context"
	"sort"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// notificationRecipientDecision is one exact event-time policy result. A
// recipient can have several decisions for the same source fact because each
// rich notification signal remains independently addressable.
type notificationRecipientDecision struct {
	recipientID string
	kind        corev1.NotificationPolicyKind
	intensity   corev1.NotificationDeliveryIntensity
}

func messageMentionKindForPolicy(kind corev1.NotificationPolicyKind) corev1.MessageMentionKind {
	switch kind {
	case corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MENTION:
		return corev1.MessageMentionKind_MESSAGE_MENTION_KIND_USER
	case corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_ROLE_MENTION:
		return corev1.MessageMentionKind_MESSAGE_MENTION_KIND_ROLE
	case corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_HERE:
		return corev1.MessageMentionKind_MESSAGE_MENTION_KIND_HERE
	case corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_ALL:
		return corev1.MessageMentionKind_MESSAGE_MENTION_KIND_ALL
	default:
		return corev1.MessageMentionKind_MESSAGE_MENTION_KIND_UNSPECIFIED
	}
}

func notificationPolicyKindForMention(kind corev1.MessageMentionKind) corev1.NotificationPolicyKind {
	switch kind {
	case corev1.MessageMentionKind_MESSAGE_MENTION_KIND_USER:
		return corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MENTION
	case corev1.MessageMentionKind_MESSAGE_MENTION_KIND_ROLE:
		return corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_ROLE_MENTION
	case corev1.MessageMentionKind_MESSAGE_MENTION_KIND_HERE:
		return corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_HERE
	case corev1.MessageMentionKind_MESSAGE_MENTION_KIND_ALL:
		return corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_ALL
	default:
		return corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_UNSPECIFIED
	}
}

func resolvedMessageMentions(resolution *RoomMentionResolution) []*corev1.MessageMention {
	if resolution == nil {
		return nil
	}
	result := make([]*corev1.MessageMention, 0)
	for _, userID := range resolution.RecipientIDs {
		for _, policyKind := range resolution.PolicyKindsByUser[userID] {
			kind := messageMentionKindForPolicy(policyKind)
			if kind != corev1.MessageMentionKind_MESSAGE_MENTION_KIND_UNSPECIFIED {
				result = append(result, &corev1.MessageMention{UserId: userID, Kind: kind})
			}
		}
	}
	return result
}

func directMentionRecipients(mentions []*corev1.MessageMention) []string {
	seen := make(map[string]struct{})
	for _, mention := range mentions {
		if mention.GetKind() == corev1.MessageMentionKind_MESSAGE_MENTION_KIND_USER && mention.GetUserId() != "" {
			seen[mention.GetUserId()] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for userID := range seen {
		result = append(result, userID)
	}
	sort.Strings(result)
	return result
}

func (c *ChattoCore) buildMessageNotificationDecisionsAt(
	ctx context.Context,
	snapshot *notificationDecisionSnapshot,
	source *corev1.Event,
) ([]notificationRecipientDecision, error) {
	message := source.GetMessagePosted()
	if message == nil || message.GetEchoOfEventId() != "" {
		return nil, nil
	}
	roomID := message.GetRoomId()
	roomKind, exists := snapshot.roomKind(roomID)
	if !exists {
		return nil, nil
	}
	kindsByRecipient := make(map[string]map[corev1.NotificationPolicyKind]struct{})
	add := func(userID string, policyKind corev1.NotificationPolicyKind) {
		_, active := snapshot.activeUsers[userID]
		if userID == "" || !active || userID == source.GetActorId() || policyKind == corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_UNSPECIFIED || !snapshot.membershipExists(userID, roomID) {
			return
		}
		if kindsByRecipient[userID] == nil {
			kindsByRecipient[userID] = make(map[corev1.NotificationPolicyKind]struct{})
		}
		kindsByRecipient[userID][policyKind] = struct{}{}
	}

	if len(message.GetMentions()) > 0 {
		for _, mention := range message.GetMentions() {
			add(mention.GetUserId(), notificationPolicyKindForMention(mention.GetKind()))
		}
	} else {
		// Writers predating rich provenance flattened direct, role, @here, and
		// @all recipients into mentioned_user_ids. Inferring one cause would
		// apply the wrong policy and persist a false signal, so mixed-version
		// deliveries conservatively omit only the ambiguous mention kind.
	}

	if roomKind == KindDM {
		for _, userID := range snapshot.roomMemberIDs(roomID) {
			add(userID, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MESSAGE)
		}
	} else if message.GetInThread() == "" {
		for _, userID := range snapshot.roomMemberIDs(roomID) {
			add(userID, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_ROOM)
		}
	}

	if parentEventID := message.GetInReplyTo(); parentEventID != "" {
		parent, err := c.GetRoomEventByEventID(ctx, roomKind, roomID, parentEventID)
		if err != nil {
			return nil, err
		}
		if parent != nil {
			add(parent.GetActorId(), corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REPLY)
		}
	}

	if threadRootEventID := message.GetInThread(); threadRootEventID != "" {
		for _, userID := range snapshot.threadFollowerIDs(roomID, threadRootEventID) {
			add(userID, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_THREAD)
		}
		if snapshot.replyCounts[threadRootEventID] == 1 {
			root, err := c.GetRoomEventByEventID(ctx, roomKind, roomID, threadRootEventID)
			if err != nil {
				return nil, err
			}
			if root != nil && snapshot.threadFollowState(root.GetActorId(), roomID, threadRootEventID) == ThreadFollowStateNone {
				add(root.GetActorId(), corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_THREAD)
			}
		}
	}

	recipientIDs := sortedMapKeys(kindsByRecipient)
	decisions := make([]notificationRecipientDecision, 0)
	for _, userID := range recipientIDs {
		policyKinds := make([]corev1.NotificationPolicyKind, 0, len(kindsByRecipient[userID]))
		for policyKind := range kindsByRecipient[userID] {
			policyKinds = append(policyKinds, policyKind)
		}
		sort.Slice(policyKinds, func(i, j int) bool { return policyKinds[i] < policyKinds[j] })
		for _, policyKind := range policyKinds {
			intensity := snapshot.effectiveNotificationIntensity(userID, roomID, policyKind)
			if intensity > corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF {
				decisions = append(decisions, notificationRecipientDecision{recipientID: userID, kind: policyKind, intensity: intensity})
			}
		}
	}
	return decisions, nil
}

func newNotificationOccurrenceInputs(
	source *corev1.Event,
	message *corev1.NotificationMessageReference,
	decisions []notificationRecipientDecision,
) []CreateNotificationOccurrenceInput {
	if source == nil || source.GetCreatedAt() == nil || message == nil {
		return nil
	}
	result := make([]CreateNotificationOccurrenceInput, 0, len(decisions))
	for _, decision := range decisions {
		signal := notificationSignalForPolicyKind(decision.kind, message, "")
		if signal == nil {
			continue
		}
		attention := corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
		if decision.kind == corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REACTION {
			attention = corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT
		}
		result = append(result, CreateNotificationOccurrenceInput{
			RecipientID: decision.recipientID, SourceEventID: source.GetId(), SourceCreated: source.GetCreatedAt().AsTime(),
			ActorID: source.GetActorId(), Signal: signal, Intensity: decision.intensity, AttentionLevel: attention,
			EvaluatedAt: source.GetCreatedAt().AsTime(),
		})
	}
	return result
}
