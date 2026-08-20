package core

import (
	"context"
	"sort"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// notificationRecipientDecision is one exact event-time policy result. A
// recipient can have several decisions for the same source fact because each
// rich notification signal remains independently addressable.
type notificationRecipientDecision struct {
	recipientID string
	signal      *corev1.NotificationSignal
	mode        corev1.NotificationDeliveryMode
}

func notificationSignalForMention(mention *corev1.MessageMention, message *corev1.NotificationMessageReference) *corev1.NotificationSignal {
	if mention == nil || message == nil {
		return nil
	}
	cloned := func() *corev1.NotificationMessageReference {
		return proto.Clone(message).(*corev1.NotificationMessageReference)
	}
	switch mention.GetCause().(type) {
	case *corev1.MessageMention_Direct:
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &corev1.DirectMentionReceived{Message: cloned()}}}
	case *corev1.MessageMention_Role:
		roleNames := []string(nil)
		if role := mention.GetRole(); role.GetRoleName() != "" {
			roleNames = []string{role.GetRoleName()}
		}
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_RoleMentionReceived{RoleMentionReceived: &corev1.RoleMentionReceived{Message: cloned(), RoleNames: roleNames}}}
	case *corev1.MessageMention_Here:
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_HereMentionReceived{HereMentionReceived: &corev1.HereMentionReceived{Message: cloned()}}}
	case *corev1.MessageMention_All:
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_AllMentionReceived{AllMentionReceived: &corev1.AllMentionReceived{Message: cloned()}}}
	default:
		return nil
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
	reference := newNotificationMessageReference(roomID, source.GetId())
	if threadRootEventID := message.GetInThread(); threadRootEventID != "" {
		reference.ThreadRootEventId = &threadRootEventID
	}
	signalsByRecipient := make(map[string]map[string]*corev1.NotificationSignal)
	add := func(userID string, signal *corev1.NotificationSignal) {
		_, active := snapshot.activeUsers[userID]
		identity := notificationSignalIdentity(signal)
		if userID == "" || !active || userID == source.GetActorId() || identity == "" || !snapshot.membershipExists(userID, roomID) {
			return
		}
		if signalsByRecipient[userID] == nil {
			signalsByRecipient[userID] = make(map[string]*corev1.NotificationSignal)
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
			add(userID, &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_DirectMessageReceived{DirectMessageReceived: &corev1.DirectMessageReceived{Message: proto.Clone(reference).(*corev1.NotificationMessageReference)}}})
		}
	} else if message.GetInThread() == "" {
		for _, userID := range snapshot.roomMemberIDs(roomID) {
			add(userID, &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_FollowedRoomActivity{FollowedRoomActivity: &corev1.FollowedRoomActivity{Message: proto.Clone(reference).(*corev1.NotificationMessageReference)}}})
		}
	}

	if parentEventID := message.GetInReplyTo(); parentEventID != "" {
		parent, err := c.GetRoomEventByEventID(ctx, roomKind, roomID, parentEventID)
		if err != nil {
			return nil, err
		}
		if parent != nil {
			add(parent.GetActorId(), &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_ReplyReceived{ReplyReceived: &corev1.ReplyReceived{Message: proto.Clone(reference).(*corev1.NotificationMessageReference)}}})
		}
	}

	if threadRootEventID := message.GetInThread(); threadRootEventID != "" {
		for _, userID := range snapshot.threadFollowerIDs(roomID, threadRootEventID) {
			add(userID, &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &corev1.FollowedThreadActivity{Message: proto.Clone(reference).(*corev1.NotificationMessageReference)}}})
		}
		if snapshot.replyCounts[threadRootEventID] == 1 {
			root, err := c.GetRoomEventByEventID(ctx, roomKind, roomID, threadRootEventID)
			if err != nil {
				return nil, err
			}
			if root != nil && snapshot.threadFollowState(root.GetActorId(), roomID, threadRootEventID) == ThreadFollowStateNone {
				add(root.GetActorId(), &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &corev1.FollowedThreadActivity{Message: proto.Clone(reference).(*corev1.NotificationMessageReference)}}})
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
			if notificationModeProducesOccurrence(mode) {
				decisions = append(decisions, notificationRecipientDecision{recipientID: userID, signal: signal, mode: mode})
			}
		}
	}
	return decisions, nil
}

func newNotificationOccurrenceInputs(
	source *corev1.Event,
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
		attention := corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
		if signal.GetReactionReceived() != nil {
			attention = corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT
		}
		result = append(result, CreateNotificationOccurrenceInput{
			RecipientID: decision.recipientID, SourceEventID: source.GetId(), SourceCreated: source.GetCreatedAt().AsTime(),
			ActorID: source.GetActorId(), Signal: signal, Mode: decision.mode, AttentionLevel: attention,
		})
	}
	return result
}
