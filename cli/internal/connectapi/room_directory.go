package connectapi

import (
	"context"
	"fmt"

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

	rooms := []*apiv1.DirectoryRoom{}
	if roomDirectoryScopeIncludesChannels(req.Msg.GetScope()) {
		channelRooms, err := s.visibleChannelRooms(ctx, caller.UserID)
		if err != nil {
			return nil, connectError(err)
		}
		rooms = append(rooms, channelRooms...)
	}
	if roomDirectoryScopeIncludesDMs(req.Msg.GetScope()) {
		dmRooms, err := s.visibleDMRooms(ctx, caller.UserID)
		if err != nil {
			return nil, connectError(err)
		}
		rooms = append(rooms, dmRooms...)
	}

	return connect.NewResponse(&apiv1.ListRoomsResponse{Rooms: rooms}), nil
}

func (s *roomDirectoryService) ListRoomGroups(ctx context.Context, _ *connect.Request[apiv1.ListRoomGroupsRequest]) (*connect.Response[apiv1.ListRoomGroupsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	visibleRooms, err := s.visibleChannelRoomMap(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	groups, err := s.api.core.ListRoomGroupsOrdered(ctx, core.KindChannel)
	if err != nil {
		return nil, connectError(err)
	}

	apiGroups := make([]*apiv1.RoomGroup, 0, len(groups))
	for _, group := range groups {
		apiGroup, err := s.apiRoomGroup(ctx, caller.UserID, group, visibleRooms)
		if err != nil {
			return nil, connectError(err)
		}
		apiGroups = append(apiGroups, apiGroup)
	}
	return connect.NewResponse(&apiv1.ListRoomGroupsResponse{Groups: apiGroups}), nil
}

func (s *roomDirectoryService) GetRoom(ctx context.Context, req *connect.Request[apiv1.GetRoomRequest]) (*connect.Response[apiv1.GetRoomResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	room, err := s.api.core.FindRoomByID(ctx, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	kind := core.KindOfRoom(room)
	visible, err := s.canSeeRoom(ctx, caller.UserID, kind, room.Id)
	if err != nil {
		return nil, connectError(err)
	}
	if !visible {
		return nil, connectError(core.ErrPermissionDenied)
	}
	apiRoom, err := s.directoryRoom(ctx, caller.UserID, room)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetRoomResponse{Room: apiRoom}), nil
}

func (s *roomDirectoryService) JoinGroup(ctx context.Context, req *connect.Request[apiv1.JoinGroupRequest]) (*connect.Response[apiv1.JoinGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	group, err := s.api.core.GetRoomGroup(ctx, req.Msg.GetGroupId())
	if err != nil {
		return nil, connectError(err)
	}

	joined := make([]string, 0, len(group.GetRoomIds()))
	for _, roomID := range group.GetRoomIds() {
		room, err := s.api.core.GetRoom(ctx, core.KindChannel, roomID)
		if err != nil {
			return nil, connectError(err)
		}
		if room.GetArchived() {
			continue
		}
		alreadyMember, err := s.api.core.RoomMembershipExists(ctx, core.KindChannel, caller.UserID, roomID)
		if err != nil {
			return nil, connectError(err)
		}
		if alreadyMember {
			continue
		}
		canJoin, err := s.api.core.CanJoinRoomAt(ctx, caller.UserID, core.KindChannel, roomID)
		if err != nil {
			return nil, connectError(err)
		}
		if !canJoin {
			continue
		}
		if _, err := s.api.core.JoinRoom(ctx, caller.UserID, core.KindChannel, caller.UserID, roomID); err != nil {
			return nil, connectError(fmt.Errorf("join %s: %w", roomID, err))
		}
		joined = append(joined, roomID)
	}

	return connect.NewResponse(&apiv1.JoinGroupResponse{JoinedRoomIds: joined}), nil
}

func (s *roomDirectoryService) visibleChannelRooms(ctx context.Context, userID string) ([]*apiv1.DirectoryRoom, error) {
	rooms, err := s.api.core.ListRooms(ctx, core.KindChannel)
	if err != nil {
		return nil, err
	}
	result := make([]*apiv1.DirectoryRoom, 0, len(rooms))
	for _, room := range rooms {
		if room.GetArchived() {
			continue
		}
		visible, err := s.api.core.CanSeeRoom(ctx, userID, core.KindChannel, room.Id)
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		apiRoom, err := s.directoryRoom(ctx, userID, room)
		if err != nil {
			return nil, err
		}
		result = append(result, apiRoom)
	}
	return result, nil
}

func (s *roomDirectoryService) visibleDMRooms(ctx context.Context, userID string) ([]*apiv1.DirectoryRoom, error) {
	rooms, err := s.api.core.ListMemberRooms(ctx, core.KindDM, userID, core.MemberRoomListOptions{
		RequireLastMessage:    true,
		SortByLastMessageDesc: true,
	})
	if err != nil {
		return nil, err
	}
	result := make([]*apiv1.DirectoryRoom, 0, len(rooms))
	for _, room := range rooms {
		apiRoom, err := s.directoryRoom(ctx, userID, room)
		if err != nil {
			return nil, err
		}
		result = append(result, apiRoom)
	}
	return result, nil
}

func (s *roomDirectoryService) visibleChannelRoomMap(ctx context.Context, userID string) (map[string]*corev1.Room, error) {
	rooms, err := s.api.core.ListRooms(ctx, core.KindChannel)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*corev1.Room, len(rooms))
	for _, room := range rooms {
		if room.GetArchived() {
			continue
		}
		visible, err := s.api.core.CanSeeRoom(ctx, userID, core.KindChannel, room.Id)
		if err != nil {
			return nil, err
		}
		if visible {
			result[room.Id] = room
		}
	}
	return result, nil
}

func (s *roomDirectoryService) apiRoomGroup(ctx context.Context, userID string, group *corev1.RoomGroup, visibleRooms map[string]*corev1.Room) (*apiv1.RoomGroup, error) {
	apiGroup := &apiv1.RoomGroup{
		Id:          group.GetId(),
		Name:        group.GetName(),
		Description: group.GetDescription(),
	}

	for _, roomID := range group.GetRoomIds() {
		room := visibleRooms[roomID]
		if room == nil {
			continue
		}
		apiRoom, err := s.directoryRoom(ctx, userID, room)
		if err != nil {
			return nil, err
		}
		apiGroup.Rooms = append(apiGroup.Rooms, apiRoom)
	}

	sidebarLinks := make(map[string]*corev1.SidebarLink, len(group.GetSidebarLinks()))
	for _, link := range group.GetSidebarLinks() {
		sidebarLinks[link.GetId()] = link
	}
	for _, entry := range group.GetEntries() {
		switch entry.GetKind() {
		case corev1.SidebarGroupEntry_ROOM:
			room := visibleRooms[entry.GetId()]
			if room == nil {
				continue
			}
			apiRoom, err := s.directoryRoom(ctx, userID, room)
			if err != nil {
				return nil, err
			}
			apiGroup.Items = append(apiGroup.Items, &apiv1.RoomGroupItem{
				Item: &apiv1.RoomGroupItem_Room{Room: apiRoom},
			})
		case corev1.SidebarGroupEntry_SIDEBAR_LINK:
			link := sidebarLinks[entry.GetId()]
			if link == nil {
				continue
			}
			apiGroup.Items = append(apiGroup.Items, &apiv1.RoomGroupItem{
				Item: &apiv1.RoomGroupItem_SidebarLink{SidebarLink: apiSidebarLink(link)},
			})
		}
	}
	return apiGroup, nil
}

func (s *roomDirectoryService) directoryRoom(ctx context.Context, userID string, room *corev1.Room) (*apiv1.DirectoryRoom, error) {
	state, err := s.roomViewerState(ctx, userID, room)
	if err != nil {
		return nil, err
	}
	return &apiv1.DirectoryRoom{
		Room:        apiRoom(room),
		ViewerState: state,
	}, nil
}

func (s *roomDirectoryService) roomViewerState(ctx context.Context, userID string, room *corev1.Room) (*apiv1.RoomViewerState, error) {
	kind := core.KindOfRoom(room)
	isMember, err := s.api.core.RoomMembershipExists(ctx, kind, userID, room.Id)
	if err != nil {
		return nil, err
	}
	hasUnread := false
	if isMember {
		hasUnread, err = s.api.core.HasUnread(ctx, kind, userID, room.Id)
		if err != nil {
			return nil, err
		}
	}
	canList, err := s.canSeeRoom(ctx, userID, kind, room.Id)
	if err != nil {
		return nil, err
	}
	canJoin, err := s.api.core.CanJoinRoomAt(ctx, userID, kind, room.Id)
	if err != nil {
		return nil, err
	}
	canPostMessage, err := s.api.core.CanPostMessage(ctx, userID, kind, room.Id)
	if err != nil {
		return nil, err
	}
	canPostInThread, err := s.api.core.CanPostInThread(ctx, userID, kind, room.Id)
	if err != nil {
		return nil, err
	}
	canAttach, err := s.api.core.CanAttachFiles(ctx, userID, kind, room.Id)
	if err != nil {
		return nil, err
	}
	canReact, err := s.api.core.CanReactToMessage(ctx, userID, kind, room.Id)
	if err != nil {
		return nil, err
	}
	canEcho, err := s.api.core.CanEchoMessage(ctx, userID, kind, room.Id)
	if err != nil {
		return nil, err
	}
	canManageOthersMessage, err := s.api.core.CanManageOthersMessage(ctx, userID, kind, room.Id)
	if err != nil {
		return nil, err
	}
	canManageRoom, err := s.api.core.PermResolver().HasRoomPermission(ctx, userID, kind, room.Id, core.PermRoomManage)
	if err != nil {
		return nil, err
	}
	canBanRoomMembers, err := s.api.core.PermResolver().HasRoomPermission(ctx, userID, kind, room.Id, core.PermRoomMemberBan)
	if err != nil {
		return nil, err
	}
	messageActionsEnabled := isMember && !room.GetArchived()
	memberActionsEnabled := isMember
	canJoin = canJoin && !isMember && kind == core.KindChannel && !room.GetArchived()
	canPostMessage = canPostMessage && messageActionsEnabled
	canPostInThread = canPostInThread && messageActionsEnabled
	canAttach = canAttach && messageActionsEnabled
	canReact = canReact && messageActionsEnabled
	canEcho = canEcho && messageActionsEnabled
	canManageOthersMessage = canManageOthersMessage && memberActionsEnabled
	if kind == core.KindDM {
		canManageRoom = false
		canBanRoomMembers = false
	}

	return &apiv1.RoomViewerState{
		IsMember:               isMember,
		HasUnread:              hasUnread,
		CanListRoom:            canList,
		CanJoinRoom:            canJoin,
		CanPostMessage:         canPostMessage,
		CanPostInThread:        canPostInThread,
		CanAttach:              canAttach,
		CanReact:               canReact,
		CanEchoMessage:         canEcho,
		CanManageOthersMessage: canManageOthersMessage,
		CanManageRoom:          canManageRoom,
		CanBanRoomMembers:      canBanRoomMembers,
	}, nil
}

func (s *roomDirectoryService) canSeeRoom(ctx context.Context, userID string, kind core.RoomKind, roomID string) (bool, error) {
	if kind == core.KindDM {
		return s.api.core.RoomMembershipExists(ctx, kind, userID, roomID)
	}
	return s.api.core.CanSeeRoom(ctx, userID, kind, roomID)
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
