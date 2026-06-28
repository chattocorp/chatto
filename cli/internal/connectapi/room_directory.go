package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	appv1 "hmans.de/chatto/internal/pb/chatto/app/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type roomDirectoryService struct {
	api *API
}

func (s *roomDirectoryService) ListRooms(ctx context.Context, req *connect.Request[appv1.ListRoomsRequest]) (*connect.Response[appv1.ListRoomsResponse], error) {
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

	apiRooms := make([]*appv1.DirectoryRoom, 0, len(rooms))
	for _, room := range rooms {
		apiRooms = append(apiRooms, apiDirectoryRoom(room))
	}

	return connect.NewResponse(&appv1.ListRoomsResponse{Rooms: apiRooms}), nil
}

func (s *roomDirectoryService) ListRoomGroups(ctx context.Context, _ *connect.Request[appv1.ListRoomGroupsRequest]) (*connect.Response[appv1.ListRoomGroupsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	groups, err := s.api.core.RoomDirectoryReads().ListRoomGroups(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}

	apiGroups := make([]*appv1.RoomGroup, 0, len(groups))
	for _, group := range groups {
		apiGroups = append(apiGroups, apiRoomGroup(group))
	}
	return connect.NewResponse(&appv1.ListRoomGroupsResponse{Groups: apiGroups}), nil
}

func (s *roomDirectoryService) GetRoom(ctx context.Context, req *connect.Request[appv1.GetRoomRequest]) (*connect.Response[appv1.GetRoomResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	room, err := s.api.core.RoomDirectoryReads().GetRoom(ctx, caller.UserID, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.GetRoomResponse{Room: apiDirectoryRoom(room)}), nil
}

func (s *roomDirectoryService) JoinGroup(ctx context.Context, req *connect.Request[appv1.JoinGroupRequest]) (*connect.Response[appv1.JoinGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	joined, err := s.api.core.RoomDirectoryReads().JoinGroup(ctx, caller.UserID, req.Msg.GetGroupId())
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&appv1.JoinGroupResponse{JoinedRoomIds: joined}), nil
}

func apiDirectoryRoom(room *core.DirectoryRoom) *appv1.DirectoryRoom {
	if room == nil {
		return nil
	}
	return &appv1.DirectoryRoom{
		Room:        apiRoom(room.Room),
		ViewerState: apiRoomViewerState(room.ViewerState),
	}
}

func apiRoomViewerState(state core.DirectoryRoomViewerState) *appv1.RoomViewerState {
	return &appv1.RoomViewerState{
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

func apiRoomGroup(group *core.DirectoryRoomGroup) *appv1.RoomGroup {
	if group == nil || group.Group == nil {
		return nil
	}
	apiGroup := &appv1.RoomGroup{
		Id:          group.Group.GetId(),
		Name:        group.Group.GetName(),
		Description: group.Group.GetDescription(),
	}
	for _, room := range group.Rooms {
		apiGroup.Rooms = append(apiGroup.Rooms, apiDirectoryRoom(room))
	}
	for _, item := range group.Items {
		switch {
		case item.Room != nil:
			apiGroup.Items = append(apiGroup.Items, &appv1.RoomGroupItem{
				Item: &appv1.RoomGroupItem_Room{Room: apiDirectoryRoom(item.Room)},
			})
		case item.SidebarLink != nil:
			apiGroup.Items = append(apiGroup.Items, &appv1.RoomGroupItem{
				Item: &appv1.RoomGroupItem_SidebarLink{SidebarLink: apiSidebarLink(item.SidebarLink)},
			})
		}
	}
	return apiGroup
}

func roomDirectoryScopeIncludesChannels(scope appv1.RoomDirectoryScope) bool {
	return scope == appv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_UNSPECIFIED ||
		scope == appv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_ALL ||
		scope == appv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_CHANNELS
}

func roomDirectoryScopeIncludesDMs(scope appv1.RoomDirectoryScope) bool {
	return scope == appv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_UNSPECIFIED ||
		scope == appv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_ALL ||
		scope == appv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_DMS
}

func apiSidebarLink(link *corev1.SidebarLink) *appv1.SidebarLink {
	if link == nil {
		return nil
	}
	return &appv1.SidebarLink{
		Id:    link.GetId(),
		Label: link.GetLabel(),
		Url:   link.GetUrl(),
	}
}
