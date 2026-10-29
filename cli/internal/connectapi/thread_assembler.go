package connectapi

import (
	"context"
	"errors"

	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/parallel"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func followedThreadsResponse(ctx context.Context, api *API, viewerID string, page *core.FollowedThreadsPage) (*apiv1.ListFollowedThreadsResponse, error) {
	if page == nil {
		return &apiv1.ListFollowedThreadsResponse{
			Includes: &apiv1.RoomTimelineIncludes{Users: map[string]*apiv1.User{}},
			Page:     apiPageInfo(0, false),
		}, nil
	}

	messageIDs := make([]string, 0, len(page.Threads)*2)
	for _, thread := range page.Threads {
		if thread != nil && thread.ThreadRootEventID != "" {
			messageIDs = append(messageIDs, thread.ThreadRootEventID)
			if thread.LatestReplyEventID != "" {
				messageIDs = append(messageIDs, thread.LatestReplyEventID)
			}
		}
	}
	reactionsByMessageID, err := api.core.GetReactionsBatch(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	h := &timelineHydrator{
		api:                  api,
		ctx:                  ctx,
		viewerID:             viewerID,
		reactionsByMessageID: reactionsByMessageID,
		userIDs:              make(map[string]struct{}),
	}

	threads, err := parallel.MapNonNil(ctx, maxConnectAPIHydrationConcurrency, page.Threads, func(ctx context.Context, _ int, thread *core.FollowedThread) (*apiv1.FollowedThread, error) {
		if thread == nil {
			return nil, nil
		}

		kind := core.RoomKindFromLegacySpaceID(thread.SpaceID)
		threadHydrator := &timelineHydrator{
			api: api, ctx: ctx, viewerID: viewerID, kind: kind,
			reactionsByMessageID: reactionsByMessageID,
			userIDs:              make(map[string]struct{}),
		}
		room, err := api.core.GetRoom(ctx, kind, thread.RoomID)
		if err != nil {
			// List responses omit resources that disappear between the core page
			// snapshot and response hydration instead of failing the whole page.
			if errors.Is(err, core.ErrNotFound) {
				return nil, nil
			}
			return nil, err
		}

		event, err := api.core.GetRoomEventByEventID(ctx, kind, thread.RoomID, thread.ThreadRootEventID)
		if err != nil {
			return nil, err
		}
		var rootMessage *apiv1.Message
		if event != nil {
			apiEvent, err := threadHydrator.event(ctx, &core.RoomEvent{Event: event})
			if err != nil {
				return nil, err
			}
			rootMessage = messageFromTimelineEvent(apiEvent)
		}
		var latestReply *apiv1.Message
		if thread.LatestReplyEventID != "" {
			latestEvent, err := api.core.GetRoomEventByEventID(ctx, kind, thread.RoomID, thread.LatestReplyEventID)
			if err != nil {
				return nil, err
			}
			if latestEvent != nil {
				apiEvent, err := threadHydrator.event(ctx, &core.RoomEvent{Event: latestEvent})
				if err != nil {
					return nil, err
				}
				latestReply = messageFromTimelineEvent(apiEvent)
			}
		}

		var lastReplyAt *timestamppb.Timestamp
		if thread.LastReplyAt != nil {
			lastReplyAt = timestamppb.New(*thread.LastReplyAt)
		}
		participantPreviewUserIDs := firstN(thread.ParticipantIDs, threadParticipantPreviewLimit)
		threadHydrator.addUserIDs(participantPreviewUserIDs)
		var directMessageParticipantUserIDs []string
		if kind == core.KindDM {
			// Deleted participants stay DM members and hydrate as deleted-user
			// references, so the label never turns into a self-DM.
			memberIDs, err := api.core.RoomMemberships().ListRoomMemberIDsForList(ctx, viewerID, thread.RoomID)
			if err != nil {
				return nil, err
			}
			directMessageParticipantUserIDs = make([]string, 0, len(memberIDs))
			for _, memberID := range memberIDs {
				if memberID == "" {
					continue
				}
				directMessageParticipantUserIDs = append(directMessageParticipantUserIDs, memberID)
				threadHydrator.addUserID(memberID)
			}
		}
		for userID := range threadHydrator.userIDs {
			h.addUserID(userID)
		}
		following := true
		viewerState := apiThreadViewerState(following, thread.HasUnreadReplies)
		viewerState.SlowModeNextPostAt = rootMessage.GetViewerState().GetThreadSlowModeNextPostAt()
		return &apiv1.FollowedThread{
			RootMessage:                     rootMessage,
			LatestReply:                     latestReply,
			Room:                            apiRoomSummary(room),
			DirectMessageParticipantUserIds: directMessageParticipantUserIDs,
			Thread: &apiv1.ThreadSummary{
				ThreadRootEventId:         thread.ThreadRootEventID,
				ReplyCount:                int32(thread.ReplyCount),
				LastReplyAt:               lastReplyAt,
				ParticipantPreviewUserIds: participantPreviewUserIDs,
				ParticipantCount:          int32(thread.ParticipantCount),
				ViewerState:               viewerState,
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

func apiThreadViewerState(following, hasUnreadReplies bool) *apiv1.ThreadViewerState {
	return &apiv1.ThreadViewerState{
		IsFollowing:      &following,
		HasUnreadReplies: &hasUnreadReplies,
	}
}
