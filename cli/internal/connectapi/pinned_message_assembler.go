package connectapi

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/parallel"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type pinnedMessageAssembler struct {
	api *API
}

type hydratedPinnedMessage struct {
	message *apiv1.Message
}

func newPinnedMessageAssembler(api *API) *pinnedMessageAssembler {
	return &pinnedMessageAssembler{api: api}
}

// assemble hydrates a renderable pin page without per-pin reaction, user, or
// presence lookups. Message bodies and thread state are still independent
// reads, so those are bounded by the shared Connect hydration limit.
func (a *pinnedMessageAssembler) assemble(ctx context.Context, viewerID string, items []core.PinnedMessageItem) ([]*apiv1.PinnedMessage, error) {
	ctx = core.WithDEKRequestCache(ctx)

	messageIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item.Event != nil {
			messageIDs = append(messageIDs, item.Event.GetId())
		}
	}
	reactionsByMessageID, err := a.api.core.GetReactionsBatch(ctx, messageIDs)
	if err != nil {
		return nil, err
	}

	h := &timelineHydrator{
		api:                  a.api,
		ctx:                  ctx,
		viewerID:             viewerID,
		kind:                 core.KindChannel,
		reactionsByMessageID: reactionsByMessageID,
		userIDs:              make(map[string]struct{}),
		thumbnail:            defaultTimelineAttachmentThumbnail(),
	}
	hydrated, err := parallel.Map(ctx, maxConnectAPIHydrationConcurrency, items, func(ctx context.Context, _ int, item core.PinnedMessageItem) (hydratedPinnedMessage, error) {
		if item.Event == nil {
			return hydratedPinnedMessage{}, fmt.Errorf("pinned message %q has no message event", item.Pin.MessageEventID)
		}
		apiEvent, err := h.event(ctx, &core.RoomEvent{Event: item.Event})
		if err != nil {
			return hydratedPinnedMessage{}, err
		}
		message := messageFromTimelineEvent(apiEvent)
		if message == nil {
			return hydratedPinnedMessage{}, fmt.Errorf("pinned event %q did not hydrate as a message", item.Event.GetId())
		}
		return hydratedPinnedMessage{message: message}, nil
	})
	if err != nil {
		return nil, err
	}

	result := make([]*apiv1.PinnedMessage, len(hydrated))
	for i, item := range hydrated {
		result[i] = &apiv1.PinnedMessage{Message: item.message}
	}
	return result, nil
}
