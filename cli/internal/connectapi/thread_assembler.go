package connectapi

import (
	"context"
	"errors"

	"github.com/nats-io/nats.go/jetstream"
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
			// List responses omit resources that disappear between the core page
			// snapshot and response hydration instead of failing the whole page.
			if errors.Is(err, core.ErrNotFound) || errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
				return nil, nil
			}
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
		participantPreviewUserIDs := firstN(thread.ParticipantIDs, 5)
		h.addUserIDs(participantPreviewUserIDs)
		following := true
		hasUnread := thread.HasUnread
		return &apiv1.FollowedThread{
			RootMessage: rootMessage,
			Room:        apiRoomSummary(room),
			Thread: &apiv1.ThreadSummary{
				ThreadRootEventId:         thread.ThreadRootEventID,
				ReplyCount:                int32(thread.ReplyCount),
				LastReplyAt:               lastReplyAt,
				ParticipantPreviewUserIds: participantPreviewUserIDs,
				ParticipantCount:          int32(len(thread.ParticipantIDs)),
				ViewerState: &apiv1.ThreadViewerState{
					IsFollowing: &following,
					HasUnread:   &hasUnread,
				},
			},
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
