package core

import (
	"context"
	"sort"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// notificationRecipientDecision is the source-time policy result used to
// prepare an exact occurrence. It is deliberately process-local: the prepared
// NotificationOccurrence, not another domain event, is the temporary durable
// work record.
type notificationRecipientDecision struct {
	recipientID string
	reasons     []*corev1.NotificationReasonMatch
}

// buildMessageNotificationDecisions evaluates every matching cause once and
// returns one deterministic decision per recipient for the source message.
func (c *ChattoCore) buildMessageNotificationDecisions(
	ctx context.Context,
	kind RoomKind,
	roomID, authorID, inThread, inReplyTo string,
	mentions *RoomMentionResolution,
) ([]notificationRecipientDecision, error) {
	reasonsByRecipient := make(map[string]map[corev1.NotificationReason]struct{})
	add := func(userID string, reason corev1.NotificationReason) {
		if userID == "" || userID == authorID {
			return
		}
		if reasonsByRecipient[userID] == nil {
			reasonsByRecipient[userID] = make(map[corev1.NotificationReason]struct{})
		}
		reasonsByRecipient[userID][reason] = struct{}{}
	}

	if mentions != nil {
		for userID, reasons := range mentions.ReasonsByUser {
			for _, reason := range reasons {
				add(userID, reason)
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
				add(member.GetUserId(), corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MESSAGE)
			}
		}
	} else if inThread == "" {
		// Joining a channel establishes its ambient room subscription. The
		// product default is Off, so this creates attention only when the user
		// explicitly raises FOLLOWED_ROOM at server or room scope.
		members, err := c.GetRoomMembersList(ctx, kind, roomID)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			if member != nil {
				add(member.GetUserId(), corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM)
			}
		}
	}

	if inReplyTo != "" {
		original, err := c.GetRoomEventByEventID(ctx, kind, roomID, inReplyTo)
		if err != nil {
			return nil, err
		}
		if original != nil {
			add(original.GetActorId(), corev1.NotificationReason_NOTIFICATION_REASON_REPLY)
		}
	}

	if inThread != "" {
		followers, err := c.GetThreadFollowers(ctx, kind, roomID, inThread)
		if err != nil {
			return nil, err
		}
		for _, followerID := range followers {
			add(followerID, corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_THREAD)
		}

		// The root author is automatically subscribed by the first reply. That
		// follow event is appended after the message, so include the cause while
		// evaluating the source instead of depending on later state.
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
				add(root.GetActorId(), corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_THREAD)
			}
		}
	}

	recipientIDs := make([]string, 0, len(reasonsByRecipient))
	for userID := range reasonsByRecipient {
		recipientIDs = append(recipientIDs, userID)
	}
	sort.Strings(recipientIDs)

	decisions := make([]notificationRecipientDecision, 0, len(recipientIDs))
	for _, userID := range recipientIDs {
		reasons := make([]corev1.NotificationReason, 0, len(reasonsByRecipient[userID]))
		for reason := range reasonsByRecipient[userID] {
			reasons = append(reasons, reason)
		}
		sort.Slice(reasons, func(i, j int) bool { return reasons[i] < reasons[j] })
		matches := make([]*corev1.NotificationReasonMatch, 0, len(reasons))
		strongest := corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
		for _, reason := range reasons {
			intensity := c.GetEffectiveNotificationIntensity(userID, roomID, reason)
			matches = append(matches, &corev1.NotificationReasonMatch{Reason: reason, Intensity: intensity})
			if intensity > strongest {
				strongest = intensity
			}
		}
		if strongest <= corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF {
			continue
		}
		decisions = append(decisions, notificationRecipientDecision{recipientID: userID, reasons: matches})
	}
	return decisions, nil
}

func directMentionRecipients(decisions []notificationRecipientDecision) []string {
	result := make([]string, 0)
	for _, decision := range decisions {
		for _, match := range decision.reasons {
			if match.GetReason() == corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION &&
				match.GetIntensity() > corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF {
				result = append(result, decision.recipientID)
				break
			}
		}
	}
	return result
}

// newNotificationOccurrenceWork prepares exact occurrence values for temporary
// RUNTIME_STATE work keys. Source-time policy is therefore never re-evaluated
// by the asynchronous materializer.
func newNotificationOccurrenceWork(
	source *corev1.Event,
	target *corev1.NotificationTarget,
	decisions []notificationRecipientDecision,
) []*corev1.NotificationOccurrence {
	if source == nil || source.GetCreatedAt() == nil || target == nil || len(decisions) == 0 {
		return nil
	}
	work := make([]*corev1.NotificationOccurrence, 0, len(decisions))
	for _, decision := range decisions {
		reasons := cloneNotificationReasons(decision.reasons)
		work = append(work, &corev1.NotificationOccurrence{
			RecipientId:     decision.recipientID,
			SourceEventId:   source.GetId(),
			SourceCreatedAt: source.GetCreatedAt(),
			ActorId:         source.GetActorId(),
			Target:          proto.Clone(target).(*corev1.NotificationTarget),
			Reasons:         reasons,
			AttentionLevel:  notificationAttentionLevelForReasons(reasons),
			EvaluatedAt:     source.GetCreatedAt(),
		})
	}
	return work
}

func (c *ChattoCore) buildReactionNotificationWork(source, target *corev1.Event, roomID, messageEventID string) []*corev1.NotificationOccurrence {
	if source == nil || target == nil || target.GetActorId() == "" || target.GetActorId() == source.GetActorId() {
		return nil
	}
	intensity := c.GetEffectiveNotificationIntensity(target.GetActorId(), roomID, corev1.NotificationReason_NOTIFICATION_REASON_REACTION)
	if intensity <= corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF {
		return nil
	}
	notificationTarget := newNotificationRoomMessageTarget(roomID, messageEventID)
	roomMessageTarget := notificationTarget.GetRoomMessage()
	if threadRootEventID := target.GetMessagePosted().GetInThread(); threadRootEventID != "" {
		roomMessageTarget.ThreadRootEventId = &threadRootEventID
	}
	work := newNotificationOccurrenceWork(source, notificationTarget, []notificationRecipientDecision{{
		recipientID: target.GetActorId(),
		reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_REACTION,
			Intensity: intensity,
		}},
	}})
	for _, occurrence := range work {
		occurrence.ReactionEmoji = source.GetReactionAdded().GetEmoji()
	}
	return work
}

func newNotificationRevocationWork(trigger *corev1.Event, recipientID, sourceEventID string, reason corev1.NotificationRemovalReason) []*corev1.NotificationOccurrence {
	if trigger == nil || recipientID == "" || sourceEventID == "" {
		return nil
	}
	return []*corev1.NotificationOccurrence{{
		RecipientId:     recipientID,
		SourceEventId:   sourceEventID,
		SourceCreatedAt: trigger.GetCreatedAt(),
		RemovalReason:   reason,
	}}
}

func cloneNotificationReasons(reasons []*corev1.NotificationReasonMatch) []*corev1.NotificationReasonMatch {
	cloned := make([]*corev1.NotificationReasonMatch, 0, len(reasons))
	for _, reason := range reasons {
		if reason != nil {
			cloned = append(cloned, proto.Clone(reason).(*corev1.NotificationReasonMatch))
		}
	}
	return cloned
}
