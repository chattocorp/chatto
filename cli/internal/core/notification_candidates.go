package core

import (
	"context"
	"sort"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// buildMessageNotificationCandidates evaluates every matching cause once and
// returns one deterministic candidate per recipient. The caller embeds the
// result in MessagePostedEvent before committing the source fact.
func (c *ChattoCore) buildMessageNotificationCandidates(
	ctx context.Context,
	kind RoomKind,
	roomID, authorID, inThread, inReplyTo string,
	mentions *RoomMentionResolution,
) ([]*corev1.NotificationCandidate, error) {
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

	candidates := make([]*corev1.NotificationCandidate, 0, len(recipientIDs))
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
		candidates = append(candidates, &corev1.NotificationCandidate{RecipientId: userID, Reasons: matches})
	}
	return candidates, nil
}

func directMentionRecipients(candidates []*corev1.NotificationCandidate) []string {
	result := make([]string, 0)
	for _, candidate := range candidates {
		for _, match := range candidate.GetReasons() {
			if match.GetReason() == corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION &&
				match.GetIntensity() > corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF {
				result = append(result, candidate.GetRecipientId())
				break
			}
		}
	}
	return result
}
