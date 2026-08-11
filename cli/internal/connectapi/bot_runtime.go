package connectapi

import (
	"context"
	"errors"
	"sort"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

const (
	defaultBotDMListLimit     = 50
	maxBotDMListLimit         = 100
	defaultBotThreadListLimit = 50
	maxBotThreadListLimit     = 100
	maxBotContentReadAttempts = 5
)

// botRuntimeService is the only ConnectRPC surface mounted for bot API keys.
// Its methods apply application capability, owner-authority, and resource-
// context checks independently of the ordinary user API.
type botRuntimeService struct {
	api *API
}

func (s *botRuntimeService) ListBotThreads(ctx context.Context, req *connect.Request[apiv1.ListBotThreadsRequest]) (*connect.Response[apiv1.ListBotThreadsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < maxBotContentReadAttempts; attempt++ {
		contexts, boundary, err := s.api.core.ListBotThreadContextsAtBoundary(ctx, caller.UserID)
		if err != nil {
			return nil, connectError(err)
		}
		limit, offset := apiPagination(req.Msg.GetPage(), defaultBotThreadListLimit, maxBotThreadListLimit)
		page, total, hasMore := apiSlicePage(contexts, limit, offset)
		threads := make([]*apiv1.BotThread, 0, len(page))
		for _, thread := range page {
			threads = append(threads, &apiv1.BotThread{
				RoomId: thread.RoomID, ThreadRootEventId: thread.ThreadRootEventID,
			})
		}
		unchanged, err := s.api.core.BotContentListBoundaryUnchanged(ctx, boundary)
		if err != nil {
			return nil, connectError(err)
		}
		if unchanged {
			return connect.NewResponse(&apiv1.ListBotThreadsResponse{
				Threads: threads,
				Page:    apiPageInfo(total, hasMore),
			}), nil
		}
	}
	return nil, connect.NewError(connect.CodeAborted, errors.New("bot thread list authorization changed during response assembly"))
}

func (s *botRuntimeService) GetBotThreadEvents(ctx context.Context, req *connect.Request[apiv1.GetBotThreadEventsRequest]) (*connect.Response[apiv1.GetBotThreadEventsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	afterSeq, beforeSeq, err := s.api.roomTimelineCursorBounds(caller.UserID, req.Msg.GetRoomId(), req.Msg.GetThreadRootEventId(), req.Msg.Cursor)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < maxBotContentReadAttempts; attempt++ {
		_, _, _, boundary, err := s.api.core.AuthorizeBotThreadContextAtBoundary(
			ctx, caller.UserID, req.Msg.GetRoomId(), req.Msg.GetThreadRootEventId(), core.ApplicationCapabilityThreadRead,
		)
		if err != nil {
			return nil, connectError(err)
		}
		result, err := s.api.core.RoomTimelineReads().GetThreadEvents(ctx, core.ThreadTimelineEventsInput{
			ActorID: caller.UserID, RoomID: req.Msg.GetRoomId(), ThreadRootEventID: req.Msg.GetThreadRootEventId(),
			Limit: int(req.Msg.GetLimit()), AfterSeq: afterSeq, BeforeSeq: beforeSeq,
		})
		if err != nil {
			return nil, connectError(err)
		}
		page, err := newRoomTimelineAssembler(s.api).buildThreadPage(ctx, caller.UserID, req.Msg.GetRoomId(), req.Msg.GetThreadRootEventId(), result.Kind, result.Root, result.Replies, result.IncludeRoot)
		if err != nil {
			return nil, connectError(err)
		}
		unchanged, err := s.api.core.BotContentReadBoundaryUnchanged(ctx, boundary)
		if err != nil {
			return nil, connectError(err)
		}
		if unchanged {
			return connect.NewResponse(&apiv1.GetBotThreadEventsResponse{Page: page}), nil
		}
	}
	return nil, connect.NewError(connect.CodeAborted, errors.New("bot thread read authorization changed during response assembly"))
}

func (s *botRuntimeService) CreateBotThreadMessage(ctx context.Context, req *connect.Request[apiv1.CreateBotThreadMessageRequest]) (*connect.Response[apiv1.CreateBotThreadMessageResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.api.core.Messages().PostBotThreadMessage(ctx, core.MessagePostInput{
		ActorID: caller.UserID, RoomID: req.Msg.GetRoomId(), ThreadRootEventID: req.Msg.GetThreadRootEventId(),
		Body: req.Msg.GetBody(), InReplyTo: req.Msg.GetInReplyTo(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	if result == nil || result.Event == nil {
		return nil, connectInternalError(errors.New("bot thread message create returned no event"))
	}
	events, _, err := newRoomTimelineAssembler(s.api).hydrateEvents(ctx, caller.UserID, core.KindChannel, []*core.RoomEvent{{Event: result.Event}})
	if err != nil {
		return nil, connectError(err)
	}
	if len(events) != 1 {
		return nil, connectInternalError(errors.New("bot thread message create returned no visible message"))
	}
	return connect.NewResponse(&apiv1.CreateBotThreadMessageResponse{Message: messageFromTimelineEvent(events[0])}), nil
}

func (s *botRuntimeService) ListBotDirectMessages(ctx context.Context, req *connect.Request[apiv1.ListBotDirectMessagesRequest]) (*connect.Response[apiv1.ListBotDirectMessagesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < maxBotContentReadAttempts; attempt++ {
		rooms, boundary, err := s.api.core.ListBotDirectMessageRoomsAtBoundary(ctx, caller.UserID)
		if err != nil {
			return nil, connectError(err)
		}
		sort.Slice(rooms, func(i, j int) bool {
			return rooms[i].GetId() < rooms[j].GetId()
		})
		limit, offset := apiPagination(req.Msg.GetPage(), defaultBotDMListLimit, maxBotDMListLimit)
		page, total, hasMore := apiSlicePage(rooms, limit, offset)
		out := make([]*apiv1.Room, 0, len(page))
		for _, room := range page {
			out = append(out, apiRoom(room))
		}
		unchanged, err := s.api.core.BotContentListBoundaryUnchanged(ctx, boundary)
		if err != nil {
			return nil, connectError(err)
		}
		if unchanged {
			return connect.NewResponse(&apiv1.ListBotDirectMessagesResponse{
				Rooms: out,
				Page:  apiPageInfo(total, hasMore),
			}), nil
		}
	}
	return nil, connect.NewError(connect.CodeAborted, errors.New("bot DM list authorization changed during response assembly"))
}

func (s *botRuntimeService) GetBotDirectMessageEvents(ctx context.Context, req *connect.Request[apiv1.GetBotDirectMessageEventsRequest]) (*connect.Response[apiv1.GetBotDirectMessageEventsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	afterSeq, beforeSeq, err := s.api.roomTimelineCursorBounds(caller.UserID, req.Msg.GetRoomId(), "", req.Msg.Cursor)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < maxBotContentReadAttempts; attempt++ {
		boundary, err := s.api.core.AuthorizeBotDirectMessageReadAtBoundary(ctx, caller.UserID, req.Msg.GetRoomId())
		if err != nil {
			return nil, connectError(err)
		}
		result, err := s.api.core.RoomTimelineReads().GetRoomEvents(ctx, core.RoomTimelineEventsInput{
			ActorID: caller.UserID, RoomID: req.Msg.GetRoomId(), Limit: int(req.Msg.GetLimit()), AfterSeq: afterSeq, BeforeSeq: beforeSeq,
		})
		if err != nil {
			return nil, connectError(err)
		}
		if result.Kind != core.KindDM {
			return nil, connectError(core.ErrPermissionDenied)
		}
		page := result.Page
		responsePage, err := newRoomTimelineAssembler(s.api).buildPage(ctx, caller.UserID, result.Kind, page.Events, page.HasOlder, page.HasNewer)
		if err != nil {
			return nil, connectError(err)
		}
		responsePage.StartCursor, err = s.api.formatRoomTimelineCursor(caller.UserID, req.Msg.GetRoomId(), "", page.StartCursorSeq)
		if err != nil {
			return nil, connectError(err)
		}
		responsePage.EndCursor, err = s.api.formatRoomTimelineCursor(caller.UserID, req.Msg.GetRoomId(), "", page.EndCursorSeq)
		if err != nil {
			return nil, connectError(err)
		}
		unchanged, err := s.api.core.BotContentReadBoundaryUnchanged(ctx, boundary)
		if err != nil {
			return nil, connectError(err)
		}
		if unchanged {
			return connect.NewResponse(&apiv1.GetBotDirectMessageEventsResponse{Page: responsePage}), nil
		}
	}
	return nil, connect.NewError(connect.CodeAborted, errors.New("bot DM read authorization changed during response assembly"))
}

func (s *botRuntimeService) CreateBotDirectMessage(ctx context.Context, req *connect.Request[apiv1.CreateBotDirectMessageRequest]) (*connect.Response[apiv1.CreateBotDirectMessageResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.api.core.Messages().PostBotDirectMessage(ctx, core.MessagePostInput{
		ActorID: caller.UserID, RoomID: req.Msg.GetRoomId(), Body: req.Msg.GetBody(), InReplyTo: req.Msg.GetInReplyTo(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	if result == nil || result.Event == nil {
		return nil, connectInternalError(errors.New("bot message create returned no event"))
	}
	events, _, err := newRoomTimelineAssembler(s.api).hydrateEvents(ctx, caller.UserID, core.KindDM, []*core.RoomEvent{{Event: result.Event}})
	if err != nil {
		return nil, connectError(err)
	}
	if len(events) != 1 {
		return nil, connectInternalError(errors.New("bot message create returned no visible message"))
	}
	return connect.NewResponse(&apiv1.CreateBotDirectMessageResponse{Message: messageFromTimelineEvent(events[0])}), nil
}
