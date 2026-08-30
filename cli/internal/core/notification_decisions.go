package core

import (
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	"sort"

	"google.golang.org/protobuf/proto"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// notificationRecipientDecision is one processing-time policy result. A
// recipient can have several decisions for the same source fact because each
// rich notification signal remains independently addressable.
type notificationRecipientDecision struct {
	recipientID string
	signal      *notificationv1.NotificationSignal
	mode        evtv1.NotificationDeliveryMode
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

	if roomKind == KindDM {
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
	decisions := make([]notificationRecipientDecision, 0)
	for _, userID := range recipientIDs {
		identities := make([]string, 0, len(signalsByRecipient[userID]))
		for identity := range signalsByRecipient[userID] {
			identities = append(identities, identity)
		}
		sort.Strings(identities)
		for _, identity := range identities {
			signal := signalsByRecipient[userID][identity]
			mode := snapshot.effectiveNotificationMode(userID, roomID, signal)
			if notificationModeProducesAttention(mode) {
				decisions = append(decisions, notificationRecipientDecision{recipientID: userID, signal: signal, mode: mode})
			}
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
