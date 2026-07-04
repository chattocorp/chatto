package connectapi

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/parallel"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type threadAssembler struct {
	api *API
}

func newThreadAssembler(api *API) *threadAssembler {
	return &threadAssembler{api: api}
}

func (a *threadAssembler) followedThreadsResponse(ctx context.Context, viewerID string, page *core.FollowedThreadsPage) (*apiv1.ListFollowedThreadsResponse, error) {
	if page == nil {
		return &apiv1.ListFollowedThreadsResponse{
			Includes: &apiv1.RoomTimelineIncludes{Users: map[string]*apiv1.User{}},
			Page:     apiPageInfo(0, false),
		}, nil
	}

	messageIDs := make([]string, 0, len(page.Threads))
	for _, thread := range page.Threads {
		if thread != nil && thread.ThreadRootEventID != "" {
			messageIDs = append(messageIDs, thread.ThreadRootEventID)
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
	}

	threads, err := parallel.MapNonNil(ctx, maxConnectAPIHydrationConcurrency, page.Threads, func(ctx context.Context, _ int, thread *core.FollowedThread) (*apiv1.FollowedThread, error) {
		if thread == nil {
			return nil, nil
		}

		kind := core.RoomKindFromLegacySpaceID(thread.SpaceID)
		room, err := a.api.core.GetRoom(ctx, kind, thread.RoomID)
		if err != nil {
			return nil, err
		}

		event, err := a.api.core.GetRoomEventByEventID(ctx, kind, thread.RoomID, thread.ThreadRootEventID)
		if err != nil {
			return nil, err
		}
		var rootMessage *apiv1.Message
		if event != nil {
			apiEvent, err := h.event(ctx, &core.RoomEvent{Event: event})
			if err != nil {
				return nil, err
			}
			rootMessage = messageFromTimelineEvent(apiEvent)
		}

		var lastReplyAt *timestamppb.Timestamp
		if thread.LastReplyAt != nil {
			lastReplyAt = timestamppb.New(*thread.LastReplyAt)
		}
		return &apiv1.FollowedThread{
			RoomId:            thread.RoomID,
			RoomName:          room.GetName(),
			ThreadRootEventId: thread.ThreadRootEventID,
			RootMessage:       rootMessage,
			ReplyCount:        int32(thread.ReplyCount),
			LastReplyAt:       lastReplyAt,
			HasUnread:         thread.HasUnread,
		}, nil
	})
	if err != nil {
		return nil, err
	}

	users, err := h.users()
	if err != nil {
		return nil, err
	}

	return &apiv1.ListFollowedThreadsResponse{
		Threads:  threads,
		Includes: &apiv1.RoomTimelineIncludes{Users: users},
		Page:     apiPageInfo(page.TotalCount, page.HasMore),
	}, nil
}
