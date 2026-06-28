package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	appv1 "hmans.de/chatto/internal/pb/chatto/app/v1"
)

const (
	defaultFollowedThreadLimit = 20
	maxFollowedThreadLimit     = 100
)

type threadService struct {
	api *API
}

func (s *threadService) ListFollowedThreads(ctx context.Context, req *connect.Request[appv1.ListFollowedThreadsRequest]) (*connect.Response[appv1.ListFollowedThreadsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	limit, offset := apiPagination(req.Msg.GetLimit(), req.Msg.GetOffset(), defaultFollowedThreadLimit, maxFollowedThreadLimit)
	page, err := s.api.core.ThreadFollows().ListFollowedThreads(ctx, caller.UserID, limit, offset)
	if err != nil {
		return nil, connectError(err)
	}

	resp, err := s.followedThreadsResponse(ctx, caller.UserID, page)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (s *threadService) FollowThread(ctx context.Context, req *connect.Request[appv1.FollowThreadRequest]) (*connect.Response[appv1.FollowThreadResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.ThreadFollows().FollowThread(ctx, caller.UserID, req.Msg.RoomId, req.Msg.ThreadRootEventId); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.FollowThreadResponse{Following: true}), nil
}

func (s *threadService) UnfollowThread(ctx context.Context, req *connect.Request[appv1.UnfollowThreadRequest]) (*connect.Response[appv1.UnfollowThreadResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.ThreadFollows().UnfollowThread(ctx, caller.UserID, req.Msg.RoomId, req.Msg.ThreadRootEventId); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.UnfollowThreadResponse{Following: false}), nil
}

func (s *threadService) followedThreadsResponse(ctx context.Context, viewerID string, page *core.FollowedThreadsPage) (*appv1.ListFollowedThreadsResponse, error) {
	if page == nil {
		return &appv1.ListFollowedThreadsResponse{
			Includes: &appv1.RoomTimelineIncludes{Users: map[string]*appv1.RoomTimelineUser{}},
		}, nil
	}

	messageIDs := make([]string, 0, len(page.Threads))
	for _, thread := range page.Threads {
		if thread != nil && thread.ThreadRootEventID != "" {
			messageIDs = append(messageIDs, thread.ThreadRootEventID)
		}
	}
	reactionsByMessageID, err := s.api.core.GetReactionsBatch(ctx, messageIDs)
	if err != nil {
		return nil, err
	}

	h := &timelineHydrator{
		api:                  s.api,
		ctx:                  ctx,
		viewerID:             viewerID,
		kind:                 core.KindChannel,
		reactionsByMessageID: reactionsByMessageID,
		userIDs:              make(map[string]struct{}),
	}

	threads := make([]*appv1.FollowedThread, 0, len(page.Threads))
	for _, thread := range page.Threads {
		if thread == nil {
			continue
		}

		kind := core.RoomKindFromLegacySpaceID(thread.SpaceID)
		room, err := s.api.core.GetRoom(ctx, kind, thread.RoomID)
		if err != nil {
			return nil, err
		}

		event, err := s.api.core.GetRoomEventByEventID(ctx, kind, thread.RoomID, thread.ThreadRootEventID)
		if err != nil {
			return nil, err
		}
		var rootMessage *appv1.RoomTimelineEvent
		if event != nil {
			rootMessage, err = h.event(&core.RoomEvent{Event: event})
			if err != nil {
				return nil, err
			}
		}

		var lastReplyAt *timestamppb.Timestamp
		if thread.LastReplyAt != nil {
			lastReplyAt = timestamppb.New(*thread.LastReplyAt)
		}
		threads = append(threads, &appv1.FollowedThread{
			RoomId:            thread.RoomID,
			RoomName:          room.GetName(),
			ThreadRootEventId: thread.ThreadRootEventID,
			RootMessage:       rootMessage,
			ReplyCount:        int32(thread.ReplyCount),
			LastReplyAt:       lastReplyAt,
			HasUnread:         thread.HasUnread,
		})
	}

	users, err := h.users()
	if err != nil {
		return nil, err
	}

	return &appv1.ListFollowedThreadsResponse{
		Threads:    threads,
		TotalCount: int32(page.TotalCount),
		HasMore:    page.HasMore,
		Includes:   &appv1.RoomTimelineIncludes{Users: users},
	}, nil
}
