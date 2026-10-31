package connectapi

import (
	"context"
	"errors"
	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/notificationcontent"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
	"github.com/chattocorp/chatto/cli/internal/push"

	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/parallel"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
)

type notificationAssembler struct {
	api *API
	// Preview hydration is an explicit read option, never a cost of routine lists.
	includeContent bool
}

func newNotificationAssembler(api *API, includeContent bool) *notificationAssembler {
	return &notificationAssembler{api: api, includeContent: includeContent}
}

func (a *notificationAssembler) actor(ctx context.Context, userID, presence string) (*apiv1.User, error) {
	if userID == "" {
		return nil, nil
	}
	user, err := a.api.core.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return userSummaryWithPresence(ctx, a.api, user, nil, presence)
}

func (a *notificationAssembler) room(ctx context.Context, roomID string) (*apiv1.RoomSummary, error) {
	if roomID == "" {
		return nil, nil
	}
	room, err := a.api.core.FindRoomByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return &apiv1.RoomSummary{Id: roomID}, nil
		}
		return nil, err
	}
	return apiRoomSummary(room), nil
}

func (a *notificationAssembler) occurrence(ctx context.Context, occurrence *notificationv1.NotificationOccurrence) (*apiv1.NotificationOccurrence, error) {
	if occurrence == nil {
		return nil, nil
	}
	presence, err := a.api.core.GetUserPresence(ctx, occurrence.GetActorId())
	if err != nil {
		return nil, err
	}
	result, err := a.occurrenceWithPresentation(ctx, occurrence, presence)
	if err != nil || result == nil || !a.includeContent {
		return result, err
	}
	visible, err := a.api.core.NotificationOccurrences().VisibleOccurrences(ctx, occurrence.GetRecipientId(), []*notificationv1.NotificationOccurrence{occurrence})
	if err != nil || len(visible) == 0 {
		return nil, err
	}
	return result, nil
}

func (a *notificationAssembler) messageReference(ctx context.Context, message *notificationv1.NotificationMessageReference) (*apiv1.NotificationMessageReference, error) {
	if message == nil {
		return nil, nil
	}
	room, err := a.room(ctx, message.GetRoomId())
	if err != nil {
		return nil, err
	}
	return &apiv1.NotificationMessageReference{
		Room:              room,
		EventId:           message.GetEventId(),
		ThreadRootEventId: message.ThreadRootEventId,
	}, nil
}

func (a *notificationAssembler) signal(ctx context.Context, signal *notificationv1.NotificationSignal) (*apiv1.NotificationSignal, error) {
	if signal == nil {
		return nil, nil
	}
	if invitation := signal.GetRoomInvitationReceived(); invitation != nil {
		room, err := a.room(ctx, invitation.GetRoomId())
		if err != nil {
			return nil, err
		}
		return &apiv1.NotificationSignal{Kind: &apiv1.NotificationSignal_RoomInvitationReceived{RoomInvitationReceived: &apiv1.RoomInvitationReceived{Room: room, InvitationId: invitation.GetInvitationId()}}}, nil
	}
	var message *notificationv1.NotificationMessageReference
	switch payload := signal.GetKind().(type) {
	case *notificationv1.NotificationSignal_DirectMessageReceived:
		message = payload.DirectMessageReceived.GetMessage()
	case *notificationv1.NotificationSignal_DirectMentionReceived:
		message = payload.DirectMentionReceived.GetMessage()
	case *notificationv1.NotificationSignal_ReplyReceived:
		message = payload.ReplyReceived.GetMessage()
	case *notificationv1.NotificationSignal_RoleMentionReceived:
		message = payload.RoleMentionReceived.GetMessage()
	case *notificationv1.NotificationSignal_HereMentionReceived:
		message = payload.HereMentionReceived.GetMessage()
	case *notificationv1.NotificationSignal_AllMentionReceived:
		message = payload.AllMentionReceived.GetMessage()
	case *notificationv1.NotificationSignal_FollowedThreadActivity:
		message = payload.FollowedThreadActivity.GetMessage()
	case *notificationv1.NotificationSignal_FollowedRoomActivity:
		message = payload.FollowedRoomActivity.GetMessage()
	case *notificationv1.NotificationSignal_ReactionReceived:
		message = payload.ReactionReceived.GetMessage()
	case *notificationv1.NotificationSignal_RoomMessageReceived:
		message = payload.RoomMessageReceived.GetMessage()
	default:
		return nil, nil
	}
	apiMessage, err := a.messageReference(ctx, message)
	if err != nil {
		return nil, err
	}
	result := &apiv1.NotificationSignal{}
	switch payload := signal.GetKind().(type) {
	case *notificationv1.NotificationSignal_DirectMessageReceived:
		result.Kind = &apiv1.NotificationSignal_DirectMessageReceived{DirectMessageReceived: &apiv1.DirectMessageReceived{Message: apiMessage}}
	case *notificationv1.NotificationSignal_DirectMentionReceived:
		result.Kind = &apiv1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &apiv1.DirectMentionReceived{Message: apiMessage}}
	case *notificationv1.NotificationSignal_ReplyReceived:
		result.Kind = &apiv1.NotificationSignal_ReplyReceived{ReplyReceived: &apiv1.ReplyReceived{Message: apiMessage}}
	case *notificationv1.NotificationSignal_RoleMentionReceived:
		result.Kind = &apiv1.NotificationSignal_RoleMentionReceived{RoleMentionReceived: &apiv1.RoleMentionReceived{
			Message: apiMessage, RoleNames: append([]string(nil), payload.RoleMentionReceived.GetRoleNames()...),
		}}
	case *notificationv1.NotificationSignal_HereMentionReceived:
		result.Kind = &apiv1.NotificationSignal_HereMentionReceived{HereMentionReceived: &apiv1.HereMentionReceived{Message: apiMessage}}
	case *notificationv1.NotificationSignal_AllMentionReceived:
		result.Kind = &apiv1.NotificationSignal_AllMentionReceived{AllMentionReceived: &apiv1.AllMentionReceived{Message: apiMessage}}
	case *notificationv1.NotificationSignal_FollowedThreadActivity:
		result.Kind = &apiv1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &apiv1.FollowedThreadActivity{Message: apiMessage}}
	case *notificationv1.NotificationSignal_FollowedRoomActivity:
		result.Kind = &apiv1.NotificationSignal_FollowedRoomActivity{FollowedRoomActivity: &apiv1.FollowedRoomActivity{Message: apiMessage}}
	case *notificationv1.NotificationSignal_ReactionReceived:
		result.Kind = &apiv1.NotificationSignal_ReactionReceived{ReactionReceived: &apiv1.ReactionReceived{Message: apiMessage, Emoji: payload.ReactionReceived.GetEmoji()}}
	case *notificationv1.NotificationSignal_RoomMessageReceived:
		result.Kind = &apiv1.NotificationSignal_RoomMessageReceived{RoomMessageReceived: &apiv1.RoomMessageReceived{Message: apiMessage}}
	}
	return result, nil
}

func (a *notificationAssembler) occurrenceWithPresentation(ctx context.Context, occurrence *notificationv1.NotificationOccurrence, presence string) (*apiv1.NotificationOccurrence, error) {
	if occurrence == nil {
		return nil, nil
	}
	signal, err := a.signal(ctx, occurrence.GetSignal())
	if err != nil {
		return nil, err
	}
	attention := apiNotificationAttentionLevel(occurrence.GetAttentionLevel())
	result := &apiv1.NotificationOccurrence{
		Id: occurrence.GetId(), CreatedAt: occurrence.GetSourceCreatedAt(), Signal: signal,
		AttentionLevel: attention, Unread: !occurrence.GetRead(), ExpiresAt: occurrence.GetExpiresAt(),
	}
	if signal == nil {
		if core.NotificationOccurrenceHasUnsupportedSignal(occurrence) {
			return result, nil
		}
		return nil, nil
	}
	actor, err := a.actor(ctx, occurrence.GetActorId(), presence)
	if err != nil {
		return nil, err
	}
	result.Actor = actor
	if !a.includeContent {
		return result, nil
	}
	actorName := actor.GetDisplayName()
	if actorName == "" {
		actorName = actor.GetLogin()
	}
	if actorName == "" {
		actorName = "Someone"
	}
	payloadContext := notificationcontent.PayloadContext(ctx, a.api.core, occurrence, log.Default())
	// Only display text crosses the resource API. Routing remains in the typed signal.
	payload := push.BuildPayloadFromOccurrence(occurrence, actorName, "", payloadContext)
	result.Content = &apiv1.NotificationContent{Title: payload.Title, Body: payload.Body}
	return result, nil
}

func apiNotificationAttentionLevel(level notificationv1.NotificationAttentionLevel) apiv1.NotificationAttentionLevel {
	switch level {
	case notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT:
		return apiv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT
	case notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT:
		return apiv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
	default:
		// Future importance tiers must never silently reduce attention when an
		// older server assembles a retained occurrence.
		return apiv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
	}
}

func (a *notificationAssembler) occurrences(ctx context.Context, occurrences []*notificationv1.NotificationOccurrence) ([]*apiv1.NotificationOccurrence, error) {
	actorIDs := make([]string, 0)
	for _, occurrence := range occurrences {
		if actorID := occurrence.GetActorId(); actorID != "" {
			actorIDs = append(actorIDs, actorID)
		}
	}
	presences, err := a.api.core.GetUserPresences(ctx, actorIDs)
	if err != nil {
		return nil, err
	}
	hydrated, err := parallel.MapNonNil(ctx, maxConnectAPIHydrationConcurrency, occurrences, func(ctx context.Context, _ int, occurrence *notificationv1.NotificationOccurrence) (*apiv1.NotificationOccurrence, error) {
		return a.occurrenceWithPresentation(ctx, occurrence, presences[occurrence.GetActorId()])
	})
	if err != nil || len(occurrences) == 0 || !a.includeContent {
		return hydrated, err
	}
	// Loading previews can race with deletion or lost access. Recheck the complete
	// batch after hydration, using the same visibility gate as notification reads.
	visible, err := a.api.core.NotificationOccurrences().VisibleOccurrences(ctx, occurrences[0].GetRecipientId(), occurrences)
	if err != nil {
		return nil, err
	}
	visibleIDs := make(map[string]bool, len(visible))
	for _, occurrence := range visible {
		visibleIDs[occurrence.GetId()] = true
	}
	result := make([]*apiv1.NotificationOccurrence, 0, len(hydrated))
	for _, occurrence := range hydrated {
		if visibleIDs[occurrence.GetId()] {
			result = append(result, occurrence)
		}
	}
	return result, nil
}
