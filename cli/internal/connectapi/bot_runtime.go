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
	defaultBotDMListLimit = 50
	maxBotDMListLimit     = 100
)

// botRuntimeService is the only ConnectRPC surface mounted for bot API keys.
// Its methods apply application capability, owner-authority, and resource-
// context checks independently of the ordinary user API.
type botRuntimeService struct {
	api *API
}

func (s *botRuntimeService) ListBotDirectMessages(ctx context.Context, req *connect.Request[apiv1.ListBotDirectMessagesRequest]) (*connect.Response[apiv1.ListBotDirectMessagesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.api.core.AuthorizeBotCapability(ctx, caller.UserID, core.ApplicationCapabilityDMMessageRead); err != nil {
		return nil, connectError(err)
	}
	rooms, err := s.api.core.ListMemberRooms(ctx, core.KindDM, caller.UserID, core.MemberRoomListOptions{})
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
	return connect.NewResponse(&apiv1.ListBotDirectMessagesResponse{
		Rooms: out,
		Page:  apiPageInfo(total, hasMore),
	}), nil
}

func (s *botRuntimeService) GetBotDirectMessageEvents(ctx context.Context, req *connect.Request[apiv1.GetBotDirectMessageEventsRequest]) (*connect.Response[apiv1.GetBotDirectMessageEventsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.api.core.AuthorizeBotCapability(ctx, caller.UserID, core.ApplicationCapabilityDMMessageRead); err != nil {
		return nil, connectError(err)
	}
	afterSeq, beforeSeq, err := s.api.roomTimelineCursorBounds(caller.UserID, req.Msg.GetRoomId(), "", req.Msg.Cursor)
	if err != nil {
		return nil, err
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
	return connect.NewResponse(&apiv1.GetBotDirectMessageEventsResponse{Page: responsePage}), nil
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
