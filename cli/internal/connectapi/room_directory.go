package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type roomDirectoryService struct {
	api *API
}

func (s *roomDirectoryService) ListRooms(ctx context.Context, req *connect.Request[apiv1.ListRoomsRequest]) (*connect.Response[apiv1.ListRoomsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	rooms, err := s.api.core.RoomDirectoryReads().ListRooms(ctx, caller.UserID, core.RoomDirectoryListOptions{
		IncludeChannels: roomDirectoryScopeIncludesChannels(req.Msg.GetScope()),
		IncludeDMs:      roomDirectoryScopeIncludesDMs(req.Msg.GetScope()),
	})
	if err != nil {
		return nil, connectError(err)
	}

	apiRooms := make([]*apiv1.RoomWithViewerState, 0, len(rooms))
	for _, room := range rooms {
		apiRooms = append(apiRooms, apiRoomWithViewerState(room))
	}

	return connect.NewResponse(&apiv1.ListRoomsResponse{Rooms: apiRooms}), nil
}

func (s *roomDirectoryService) ListRoomGroups(ctx context.Context, req *connect.Request[apiv1.ListRoomGroupsRequest]) (*connect.Response[apiv1.ListRoomGroupsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	groups, err := s.api.core.RoomDirectoryReads().ListRoomGroups(ctx, caller.UserID, core.RoomDirectoryGroupOptions{})
	if err != nil {
		return nil, connectError(err)
	}

	apiGroups := make([]*apiv1.RoomGroup, 0, len(groups))
	for _, group := range groups {
		apiGroups = append(apiGroups, apiRoomGroup(group))
	}
	return connect.NewResponse(&apiv1.ListRoomGroupsResponse{Groups: apiGroups}), nil
}

func (s *roomDirectoryService) GetRoomGroup(ctx context.Context, req *connect.Request[apiv1.GetRoomGroupRequest]) (*connect.Response[apiv1.GetRoomGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	group, err := s.api.core.RoomDirectoryReads().GetRoomGroup(ctx, caller.UserID, req.Msg.GetGroupId(), core.RoomDirectoryGroupOptions{})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetRoomGroupResponse{Group: apiRoomGroup(group)}), nil
}

func (s *roomDirectoryService) BatchGetRoomGroups(ctx context.Context, req *connect.Request[apiv1.BatchGetRoomGroupsRequest]) (*connect.Response[apiv1.BatchGetRoomGroupsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	groups, err := s.api.core.RoomDirectoryReads().BatchGetRoomGroups(ctx, caller.UserID, req.Msg.GetGroupIds(), core.RoomDirectoryGroupOptions{})
	if err != nil {
		return nil, connectError(err)
	}

	apiGroups := make([]*apiv1.RoomGroup, 0, len(groups))
	for _, group := range groups {
		apiGroups = append(apiGroups, apiRoomGroup(group))
	}
	return connect.NewResponse(&apiv1.BatchGetRoomGroupsResponse{Groups: apiGroups}), nil
}

func (s *roomDirectoryService) GetRoom(ctx context.Context, req *connect.Request[apiv1.GetRoomRequest]) (*connect.Response[apiv1.GetRoomResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	room, err := s.api.core.RoomDirectoryReads().GetRoom(ctx, caller.UserID, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetRoomResponse{Room: apiRoomWithViewerState(room)}), nil
}

func (s *roomDirectoryService) BatchGetRooms(ctx context.Context, req *connect.Request[apiv1.BatchGetRoomsRequest]) (*connect.Response[apiv1.BatchGetRoomsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	rooms, err := s.api.core.RoomDirectoryReads().BatchGetRooms(ctx, caller.UserID, req.Msg.GetRoomIds())
	if err != nil {
		return nil, connectError(err)
	}

	apiRooms := make([]*apiv1.RoomWithViewerState, 0, len(rooms))
	for _, room := range rooms {
		apiRooms = append(apiRooms, apiRoomWithViewerState(room))
	}
	return connect.NewResponse(&apiv1.BatchGetRoomsResponse{Rooms: apiRooms}), nil
}

func apiRoomWithViewerState(room *core.DirectoryRoom) *apiv1.RoomWithViewerState {
	if room == nil {
		return nil
	}
	state := room.ViewerState
	return &apiv1.RoomWithViewerState{
		Room:                   apiRoom(room.Room),
		IsMember:               state.IsMember,
		HasUnread:              state.HasUnread,
		CanListRoom:            state.CanListRoom,
		CanJoinRoom:            state.CanJoinRoom,
		CanPostMessage:         state.CanPostMessage,
		CanPostInThread:        state.CanPostInThread,
		CanAttach:              state.CanAttach,
		CanReact:               state.CanReact,
		CanEchoMessage:         state.CanEchoMessage,
		CanManageOthersMessage: state.CanManageOthersMessage,
		CanManageRoom:          state.CanManageRoom,
		CanBanRoomMembers:      state.CanBanRoomMembers,
	}
}

func apiRoomGroup(group *core.DirectoryRoomGroup) *apiv1.RoomGroup {
	if group == nil || group.Group == nil {
		return nil
	}
	apiGroup := &apiv1.RoomGroup{
		Id:          group.Group.GetId(),
		Name:        group.Group.GetName(),
		Description: group.Group.GetDescription(),
		ViewerState: &apiv1.RoomGroupViewerState{
			CanCreateRoom: group.ViewerState.CanCreateRoom,
		},
	}
	for _, item := range group.Items {
		switch {
		case item.Room != nil:
			apiGroup.Items = append(apiGroup.Items, &apiv1.RoomGroupItem{
				Item: &apiv1.RoomGroupItem_Room{Room: apiRoomWithViewerState(item.Room)},
			})
		case item.SidebarLink != nil:
			apiGroup.Items = append(apiGroup.Items, &apiv1.RoomGroupItem{
				Item: &apiv1.RoomGroupItem_SidebarLink{SidebarLink: apiSidebarLink(item.SidebarLink)},
			})
		}
	}
	return apiGroup
}

func roomDirectoryScopeIncludesChannels(scope apiv1.RoomDirectoryScope) bool {
	return scope == apiv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_UNSPECIFIED ||
		scope == apiv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_ALL ||
		scope == apiv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_CHANNELS
}

func roomDirectoryScopeIncludesDMs(scope apiv1.RoomDirectoryScope) bool {
	return scope == apiv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_UNSPECIFIED ||
		scope == apiv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_ALL ||
		scope == apiv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_DMS
}

func apiSidebarLink(link *corev1.SidebarLink) *apiv1.SidebarLink {
	if link == nil {
		return nil
	}
	return &apiv1.SidebarLink{
		Id:    link.GetId(),
		Label: link.GetLabel(),
		Url:   link.GetUrl(),
	}
}
