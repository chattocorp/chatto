package core

import (
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	"sort"

	"google.golang.org/protobuf/proto"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// notificationRecipientDecision is the selected processing-time policy result
// for one recipient and source fact.
type notificationRecipientDecision struct {
	recipientID string
	signal      *notificationv1.NotificationSignal
	mode        evtv1.NotificationDeliveryMode
}

// notificationDeliveryRank orders destinations, not enum values: Badge has
// a larger wire value than Push but must not displace a delivered occurrence.
func notificationDeliveryRank(mode evtv1.NotificationDeliveryMode) int {
	switch mode {
	case evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION:
		return 3
	case evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION:
		return 2
	case evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE:
		return 1
	default:
		return 0
	}
}

// notificationCauseRank resolves equal-delivery overlaps. Specific personal
// signals win over broad subscription activity; message signals retain the
// same exact message and thread navigation reference.
func notificationCauseRank(signal *notificationv1.NotificationSignal) int {
	switch signal.GetKind().(type) {
	case *notificationv1.NotificationSignal_DirectMentionReceived:
		return 10
	case *notificationv1.NotificationSignal_ReplyReceived:
		return 9
	case *notificationv1.NotificationSignal_DirectMessageReceived:
		return 8
	case *notificationv1.NotificationSignal_RoleMentionReceived:
		return 7
	case *notificationv1.NotificationSignal_HereMentionReceived:
		return 6
	case *notificationv1.NotificationSignal_AllMentionReceived:
		return 5
	case *notificationv1.NotificationSignal_FollowedThreadActivity:
		return 4
	case *notificationv1.NotificationSignal_RoomMessageReceived:
		return 3
	case *notificationv1.NotificationSignal_FollowedRoomActivity:
		return 2
	case *notificationv1.NotificationSignal_ReactionReceived:
		return 1
	default:
		return 0
	}
}

func notificationDecisionBetter(candidate, current notificationRecipientDecision) bool {
	if notificationDeliveryRank(candidate.mode) != notificationDeliveryRank(current.mode) {
		return notificationDeliveryRank(candidate.mode) > notificationDeliveryRank(current.mode)
	}
	if notificationCauseRank(candidate.signal) != notificationCauseRank(current.signal) {
		return notificationCauseRank(candidate.signal) > notificationCauseRank(current.signal)
	}
	return notificationSignalIdentity(candidate.signal) < notificationSignalIdentity(current.signal)
}

func notificationSignalForMention(mention *evtv1.MessageMention, message *notificationv1.NotificationMessageReference) *notificationv1.NotificationSignal {
	if mention == nil || message == nil {
		return nil
	}
	cloned := func() *notificationv1.NotificationMessageReference {
		return proto.Clone(message).(*notificationv1.NotificationMessageReference)
	}
	switch mention.GetCause().(type) {
	case *evtv1.MessageMention_Direct:
		return &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &notificationv1.DirectMentionReceived{Message: cloned()}}}
	case *evtv1.MessageMention_Role:
		roleNames := []string(nil)
		if role := mention.GetRole(); role.GetRoleName() != "" {
			roleNames = []string{role.GetRoleName()}
		}
		return &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_RoleMentionReceived{RoleMentionReceived: &notificationv1.RoleMentionReceived{Message: cloned(), RoleNames: roleNames}}}
	case *evtv1.MessageMention_Here:
		return &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_HereMentionReceived{HereMentionReceived: &notificationv1.HereMentionReceived{Message: cloned()}}}
	case *evtv1.MessageMention_All:
		return &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_AllMentionReceived{AllMentionReceived: &notificationv1.AllMentionReceived{Message: cloned()}}}
	default:
		return nil
	}
}

func resolvedMessageMentions(resolution *RoomMentionResolution) []*evtv1.MessageMention {
	if resolution == nil {
		return nil
	}
	return cloneMessageMentions(resolution.Mentions)
}

func directMentionRecipients(mentions []*evtv1.MessageMention) []string {
	seen := make(map[string]struct{})
	for _, mention := range mentions {
		if _, direct := mention.GetCause().(*evtv1.MessageMention_Direct); direct && mention.GetUserId() != "" {
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

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func buildMessageNotificationDecisions(
	snapshot *notificationDecisionSnapshot,
	source *evtv1.Event,
	parentActorID string,
	threadRootActorID string,
) []notificationRecipientDecision {
	message := source.GetMessagePosted()
	if message == nil || message.GetEchoOfEventId() != "" {
		return nil
	}
	roomID := message.GetRoomId()
	roomKind, exists := snapshot.roomKind(roomID)
	if !exists {
		return nil
	}
	reference := newNotificationMessageReference(roomID, source.GetId())
	if threadRootEventID := message.GetInThread(); threadRootEventID != "" {
		reference.ThreadRootEventId = &threadRootEventID
	}
	signalsByRecipient := make(map[string]map[string]*notificationv1.NotificationSignal)
	add := func(userID string, signal *notificationv1.NotificationSignal) {
		_, active := snapshot.activeUsers[userID]
		identity := notificationSignalIdentity(signal)
		if userID == "" || !active || userID == source.GetActorId() || identity == "" || !snapshot.notificationVisibilityExistsForSignal(userID, roomID, signal) {
			return
		}
		if signalsByRecipient[userID] == nil {
			signalsByRecipient[userID] = make(map[string]*notificationv1.NotificationSignal)
		}
		if existing := signalsByRecipient[userID][identity]; existing != nil {
			if role := existing.GetRoleMentionReceived(); role != nil {
				incoming := signal.GetRoleMentionReceived()
				role.RoleNames = sortedUniqueStrings(append(role.RoleNames, incoming.GetRoleNames()...))
			}
			return
		}
		signalsByRecipient[userID][identity] = signal
	}

	if len(message.GetMentions()) > 0 {
		for _, mention := range message.GetMentions() {
			add(mention.GetUserId(), notificationSignalForMention(mention, reference))
		}
	} else {
		// Writers predating rich provenance flattened direct, role, @here, and
		// @all recipients into mentioned_user_ids. Inferring one cause would
		// apply the wrong policy and persist a false signal, so mixed-version
		// deliveries conservatively omit only the ambiguous mention kind.
	}

	// Root activity has one room-kind base cause. Thread replies use followed
	// thread activity in both channel rooms and DMs.
	if roomKind == KindDM && message.GetInThread() == "" {
		for _, userID := range snapshot.roomMemberIDs(roomID) {
			add(userID, &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_DirectMessageReceived{DirectMessageReceived: &notificationv1.DirectMessageReceived{Message: proto.Clone(reference).(*notificationv1.NotificationMessageReference)}}})
		}
	} else if message.GetInThread() == "" {
		for _, userID := range snapshot.roomMemberIDs(roomID) {
			add(userID, &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_RoomMessageReceived{RoomMessageReceived: &notificationv1.RoomMessageReceived{Message: proto.Clone(reference).(*notificationv1.NotificationMessageReference)}}})
		}
		// FollowedRoomActivity is a deprecated compatibility branch. Root room
		// activity uses RoomMessageReceived and its per-room delivery policy.
	}

	if parentEventID := message.GetInReplyTo(); parentEventID != "" {
		add(parentActorID, &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_ReplyReceived{ReplyReceived: &notificationv1.ReplyReceived{Message: proto.Clone(reference).(*notificationv1.NotificationMessageReference)}}})
	}

	if threadRootEventID := message.GetInThread(); threadRootEventID != "" {
		for _, userID := range snapshot.threadFollowerIDs(roomID, threadRootEventID) {
			add(userID, &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &notificationv1.FollowedThreadActivity{Message: proto.Clone(reference).(*notificationv1.NotificationMessageReference)}}})
		}
		if snapshot.replyCounts[threadRootEventID] == 1 {
			if threadRootActorID != "" && snapshot.threadFollowState(threadRootActorID, roomID, threadRootEventID) == ThreadFollowStateNone {
				add(threadRootActorID, &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &notificationv1.FollowedThreadActivity{Message: proto.Clone(reference).(*notificationv1.NotificationMessageReference)}}})
			}
		}
	}

	recipientIDs := sortedMapKeys(signalsByRecipient)
	decisions := make([]notificationRecipientDecision, 0, len(recipientIDs))
	for _, userID := range recipientIDs {
		var selected notificationRecipientDecision
		for _, signal := range signalsByRecipient[userID] {
			mode := snapshot.effectiveNotificationMode(userID, roomID, signal)
			candidate := notificationRecipientDecision{recipientID: userID, signal: signal, mode: mode}
			if notificationModeProducesAttention(mode) && (selected.signal == nil || notificationDecisionBetter(candidate, selected)) {
				selected = candidate
			}
		}
		if selected.signal != nil {
			decisions = append(decisions, selected)
		}
	}
	return decisions
}

func newNotificationOccurrenceInputs(
	source *evtv1.Event,
	decisions []notificationRecipientDecision,
) []CreateNotificationOccurrenceInput {
	if source == nil || source.GetCreatedAt() == nil {
		return nil
	}
	result := make([]CreateNotificationOccurrenceInput, 0, len(decisions))
	for _, decision := range decisions {
		signal := decision.signal
		if signal == nil {
			continue
		}
		attention := notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
		if signal.GetReactionReceived() != nil {
			attention = notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT
		}
		result = append(result, CreateNotificationOccurrenceInput{
			RecipientID: decision.recipientID, SourceEventID: source.GetId(), SourceCreated: source.GetCreatedAt().AsTime(),
			ActorID: source.GetActorId(), Signal: signal, Mode: decision.mode, AttentionLevel: attention,
		})
	}
	return result
}
