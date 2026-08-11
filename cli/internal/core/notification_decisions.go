package core

import (
	"context"
	"sort"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// buildMessageNotificationDecisions evaluates every matching cause once and
// returns one deterministic decision per recipient for the notification plan
// committed atomically beside the source message.
func (c *ChattoCore) buildMessageNotificationDecisions(
	ctx context.Context,
	kind RoomKind,
	roomID, authorID, inThread, inReplyTo string,
	mentions *RoomMentionResolution,
) ([]*corev1.NotificationRecipientDecision, error) {
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
		// product default is Off, so this only creates attention when the user
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
		// follow event is appended after the message, so include the cause here
		// while evaluating the source event rather than depending on later state.
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

	decisions := make([]*corev1.NotificationRecipientDecision, 0, len(recipientIDs))
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
		decisions = append(decisions, &corev1.NotificationRecipientDecision{RecipientId: userID, Reasons: matches})
	}
	return decisions, nil
}

func directMentionRecipients(decisions []*corev1.NotificationRecipientDecision) []string {
	result := make([]string, 0)
	for _, decision := range decisions {
		for _, match := range decision.GetReasons() {
			if match.GetReason() == corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION &&
				match.GetIntensity() > corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF {
				result = append(result, decision.GetRecipientId())
				break
			}
		}
	}
	return result
}

func newNotificationOccurrencePlan(
	source *corev1.Event,
	sourceKind corev1.NotificationSourceKind,
	target *corev1.NotificationTarget,
	decisions []*corev1.NotificationRecipientDecision,
	reactionEmoji string,
) *corev1.Event {
	if source == nil || len(decisions) == 0 {
		return nil
	}
	plan := &corev1.NotificationOccurrencePlannedEvent{
		SourceEventId: source.GetId(),
		SourceKind:    sourceKind,
		Target:        target,
		Recipients:    decisions,
	}
	if reactionEmoji != "" {
		plan.ReactionEmoji = &reactionEmoji
	}
	return newEvent(source.GetActorId(), &corev1.Event{
		CreatedAt: source.GetCreatedAt(),
		Event: &corev1.Event_NotificationOccurrencePlanned{
			NotificationOccurrencePlanned: plan,
		},
	})
}

func (c *ChattoCore) buildReactionNotificationPlan(source, target *corev1.Event, roomID, messageEventID, emoji string) *corev1.Event {
	if source == nil || target == nil || target.GetActorId() == "" || target.GetActorId() == source.GetActorId() {
		return nil
	}
	intensity := c.GetEffectiveNotificationIntensity(target.GetActorId(), roomID, corev1.NotificationReason_NOTIFICATION_REASON_REACTION)
	if intensity <= corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF {
		return nil
	}
	notificationTarget := &corev1.NotificationTarget{RoomId: roomID, EventId: messageEventID}
	if threadRootEventID := target.GetMessagePosted().GetInThread(); threadRootEventID != "" {
		notificationTarget.ThreadRootEventId = &threadRootEventID
	}
	return newNotificationOccurrencePlan(
		source,
		corev1.NotificationSourceKind_NOTIFICATION_SOURCE_KIND_REACTION,
		notificationTarget,
		[]*corev1.NotificationRecipientDecision{{
			RecipientId: target.GetActorId(),
			Reasons: []*corev1.NotificationReasonMatch{{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_REACTION,
				Intensity: intensity,
			}},
		}},
		emoji,
	)
}

func notificationOccurrenceEffectSubject(effect *corev1.Event) (string, bool) {
	if effect == nil {
		return "", false
	}
	sourceEventID := effect.GetNotificationOccurrencePlanned().GetSourceEventId()
	if sourceEventID == "" {
		sourceEventID = effect.GetNotificationOccurrenceRevoked().GetSourceEventId()
	}
	if sourceEventID == "" {
		return "", false
	}
	aggregate := evtstream.NotificationAggregate(sourceEventID)
	return aggregate.SubjectFor(effect), true
}

func appendNotificationOccurrencePlan(entries []evtstream.BatchEntry, planEvent *corev1.Event) []evtstream.BatchEntry {
	subject, ok := notificationOccurrenceEffectSubject(planEvent)
	if !ok {
		return entries
	}
	return append(entries, evtstream.BatchEntry{Subject: subject, Event: planEvent})
}

func newNotificationOccurrenceRevocation(actorID, recipientID, sourceEventID string, reason corev1.NotificationRemovalReason) *corev1.Event {
	if recipientID == "" || sourceEventID == "" {
		return nil
	}
	return newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_NotificationOccurrenceRevoked{
			NotificationOccurrenceRevoked: &corev1.NotificationOccurrenceRevokedEvent{
				RecipientId:   recipientID,
				SourceEventId: sourceEventID,
				Reason:        reason,
			},
		},
	})
}
