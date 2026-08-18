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
	category    corev1.NotificationPreferenceCategory
	mode        corev1.NotificationDeliveryMode
	roleNames   []string
}

func notificationPreferenceCategoryForMention(mention *corev1.MessageMention) corev1.NotificationPreferenceCategory {
	if mention == nil {
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_UNSPECIFIED
	}
	switch mention.GetCause().(type) {
	case *corev1.MessageMention_Direct:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MENTION
	case *corev1.MessageMention_Role:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_ROLE_MENTION
	case *corev1.MessageMention_Here:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_HERE
	case *corev1.MessageMention_All:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_ALL
	default:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_UNSPECIFIED
	}
}

func resolvedMessageMentions(resolution *RoomMentionResolution) []*corev1.MessageMention {
	if resolution == nil {
		return nil
	}
	return cloneMessageMentions(resolution.Mentions)
}

func directMentionRecipients(mentions []*corev1.MessageMention) []string {
	seen := make(map[string]struct{})
	for _, mention := range mentions {
		if _, direct := mention.GetCause().(*corev1.MessageMention_Direct); direct && mention.GetUserId() != "" {
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
	kindsByRecipient := make(map[string]map[corev1.NotificationPreferenceCategory]struct{})
	roleNamesByRecipient := make(map[string]map[string]struct{})
	add := func(userID string, policyKind corev1.NotificationPreferenceCategory) {
		_, active := snapshot.activeUsers[userID]
		if userID == "" || !active || userID == source.GetActorId() || policyKind == corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_UNSPECIFIED || !snapshot.membershipExists(userID, roomID) {
			return
		}
		if kindsByRecipient[userID] == nil {
			kindsByRecipient[userID] = make(map[corev1.NotificationPreferenceCategory]struct{})
		}
		kindsByRecipient[userID][policyKind] = struct{}{}
	}

	if len(message.GetMentions()) > 0 {
		for _, mention := range message.GetMentions() {
			category := notificationPreferenceCategoryForMention(mention)
			add(mention.GetUserId(), category)
			if role := mention.GetRole(); category == corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_ROLE_MENTION && role.GetRoleName() != "" {
				if roleNamesByRecipient[mention.GetUserId()] == nil {
					roleNamesByRecipient[mention.GetUserId()] = make(map[string]struct{})
				}
				roleNamesByRecipient[mention.GetUserId()][role.GetRoleName()] = struct{}{}
			}
		}
	} else {
		// Writers predating rich provenance flattened direct, role, @here, and
		// @all recipients into mentioned_user_ids. Inferring one cause would
		// apply the wrong policy and persist a false signal, so mixed-version
		// deliveries conservatively omit only the ambiguous mention kind.
	}

	if roomKind == KindDM {
		for _, userID := range snapshot.roomMemberIDs(roomID) {
			add(userID, corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MESSAGE)
		}
	} else if message.GetInThread() == "" {
		for _, userID := range snapshot.roomMemberIDs(roomID) {
			add(userID, corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_ROOM)
		}
	}

	if parentEventID := message.GetInReplyTo(); parentEventID != "" {
		parent, err := c.GetRoomEventByEventID(ctx, roomKind, roomID, parentEventID)
		if err != nil {
			return nil, err
		}
		if parent != nil {
			add(parent.GetActorId(), corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REPLY)
		}
	}

	if threadRootEventID := message.GetInThread(); threadRootEventID != "" {
		for _, userID := range snapshot.threadFollowerIDs(roomID, threadRootEventID) {
			add(userID, corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_THREAD)
		}
		if snapshot.replyCounts[threadRootEventID] == 1 {
			root, err := c.GetRoomEventByEventID(ctx, roomKind, roomID, threadRootEventID)
			if err != nil {
				return nil, err
			}
			if root != nil && snapshot.threadFollowState(root.GetActorId(), roomID, threadRootEventID) == ThreadFollowStateNone {
				add(root.GetActorId(), corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_THREAD)
			}
		}
	}

	recipientIDs := sortedMapKeys(kindsByRecipient)
	decisions := make([]notificationRecipientDecision, 0)
	for _, userID := range recipientIDs {
		policyKinds := make([]corev1.NotificationPreferenceCategory, 0, len(kindsByRecipient[userID]))
		for policyKind := range kindsByRecipient[userID] {
			policyKinds = append(policyKinds, policyKind)
		}
		sort.Slice(policyKinds, func(i, j int) bool { return policyKinds[i] < policyKinds[j] })
		for _, policyKind := range policyKinds {
			mode := snapshot.effectiveNotificationMode(userID, roomID, policyKind)
			if mode > corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF {
				decision := notificationRecipientDecision{recipientID: userID, category: policyKind, mode: mode}
				if policyKind == corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_ROLE_MENTION {
					decision.roleNames = sortedMapKeys(roleNamesByRecipient[userID])
				}
				decisions = append(decisions, decision)
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
		signal := notificationSignalForPreferenceCategory(decision.category, message, "")
		if signal == nil {
			continue
		}
		if role := signal.GetRoleMentionReceived(); role != nil {
			role.RoleNames = append([]string(nil), decision.roleNames...)
		}
		attention := corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
		if decision.category == corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION {
			attention = corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT
		}
		result = append(result, CreateNotificationOccurrenceInput{
			RecipientID: decision.recipientID, SourceEventID: source.GetId(), SourceCreated: source.GetCreatedAt().AsTime(),
			ActorID: source.GetActorId(), Signal: signal, Mode: decision.mode, AttentionLevel: attention,
		})
	}
	return result
}
