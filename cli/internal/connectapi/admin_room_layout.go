package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type adminRoomLayoutService struct {
	api *API
}

func (s *adminRoomLayoutService) GetRoom(ctx context.Context, req *connect.Request[adminv1.GetRoomRequest]) (*connect.Response[adminv1.GetRoomResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	details, err := s.api.core.GetAdminRoom(ctx, caller.UserID, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.GetRoomResponse{
		Room:                       apiRoom(details.Room),
		ViewerCanManageRoom:        details.CanManageRoom,
		ViewerCanManagePermissions: details.CanManagePermissions,
	}), nil
}

func (s *adminRoomLayoutService) GetRoomGroup(ctx context.Context, req *connect.Request[adminv1.GetRoomGroupRequest]) (*connect.Response[adminv1.GetRoomGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	details, err := s.api.core.GetAdminRoomGroup(ctx, caller.UserID, req.Msg.GetGroupId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.GetRoomGroupResponse{
		Group: &adminv1.AdminRoomLayoutGroup{
			Id:          details.Group.GetId(),
			Name:        details.Group.GetName(),
			Description: details.Group.GetDescription(),
		},
		ViewerCanManageGroup:       details.CanManageGroup,
		ViewerCanManagePermissions: details.CanManagePermissions,
	}), nil
}

func (s *adminRoomLayoutService) ListRoomGroups(ctx context.Context, req *connect.Request[adminv1.ListRoomGroupsRequest]) (*connect.Response[adminv1.ListRoomGroupsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	canManageRooms, err := s.api.core.CanManageAnyRoom(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	if !canManageRooms {
		return nil, connectError(core.ErrPermissionDenied)
	}

	groups, err := s.getAdminRoomLayoutGroups(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.ListRoomGroupsResponse{Groups: groups}), nil
}

func (s *adminRoomLayoutService) CreateRoomGroup(ctx context.Context, req *connect.Request[adminv1.CreateRoomGroupRequest]) (*connect.Response[adminv1.CreateRoomGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	group, err := s.api.core.AdminCreateRoomGroup(ctx, caller.UserID, req.Msg.GetName(), req.Msg.GetDescription())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.CreateRoomGroupResponse{
		Group: apiAdminRoomLayoutGroup(group, nil),
	}), nil
}

func (s *adminRoomLayoutService) UpdateRoomGroup(ctx context.Context, req *connect.Request[adminv1.UpdateRoomGroupRequest]) (*connect.Response[adminv1.UpdateRoomGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	group, err := s.api.core.AdminUpdateRoomGroup(ctx, caller.UserID, req.Msg.GetGroupId(), req.Msg.Name, req.Msg.Description)
	if err != nil {
		return nil, connectError(err)
	}
	roomsByID, err := s.channelRoomsByID(ctx)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.UpdateRoomGroupResponse{
		Group: apiAdminRoomLayoutGroup(group, roomsByID),
	}), nil
}

func (s *adminRoomLayoutService) DeleteRoomGroup(ctx context.Context, req *connect.Request[adminv1.DeleteRoomGroupRequest]) (*connect.Response[adminv1.DeleteRoomGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.AdminDeleteRoomGroup(ctx, caller.UserID, req.Msg.GetGroupId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.DeleteRoomGroupResponse{Deleted: true}), nil
}

func (s *adminRoomLayoutService) ReorderRoomGroups(ctx context.Context, req *connect.Request[adminv1.ReorderRoomGroupsRequest]) (*connect.Response[adminv1.ReorderRoomGroupsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.AdminReorderRoomGroups(ctx, caller.UserID, req.Msg.GetOrderedGroupIds()); err != nil {
		return nil, connectError(err)
	}
	groups, err := s.getAdminRoomLayoutGroups(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.ReorderRoomGroupsResponse{Groups: groups}), nil
}

func (s *adminRoomLayoutService) MoveRoomToGroup(ctx context.Context, req *connect.Request[adminv1.MoveRoomToGroupRequest]) (*connect.Response[adminv1.MoveRoomToGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	moved, err := s.api.core.AdminMoveRoomToGroup(ctx, caller.UserID, req.Msg.GetRoomId(), req.Msg.GetGroupId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.MoveRoomToGroupResponse{Room: apiRoom(moved)}), nil
}

func (s *adminRoomLayoutService) ReorderSidebarItemsInGroup(ctx context.Context, req *connect.Request[adminv1.ReorderSidebarItemsInGroupRequest]) (*connect.Response[adminv1.ReorderSidebarItemsInGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	group, err := s.api.core.AdminReorderSidebarItemsInGroup(ctx, caller.UserID, req.Msg.GetGroupId(), adminRoomLayoutItemInputsToCore(req.Msg.GetItems()))
	if err != nil {
		return nil, connectError(err)
	}
	roomsByID, err := s.channelRoomsByID(ctx)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.ReorderSidebarItemsInGroupResponse{
		Group: apiAdminRoomLayoutGroup(group, roomsByID),
	}), nil
}

func (s *adminRoomLayoutService) CreateSidebarLink(ctx context.Context, req *connect.Request[adminv1.CreateSidebarLinkRequest]) (*connect.Response[adminv1.CreateSidebarLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	link, err := s.api.core.AdminCreateSidebarLink(ctx, caller.UserID, req.Msg.GetGroupId(), req.Msg.GetLabel(), req.Msg.GetUrl())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.CreateSidebarLinkResponse{SidebarLink: apiSidebarLink(link)}), nil
}

func (s *adminRoomLayoutService) UpdateSidebarLink(ctx context.Context, req *connect.Request[adminv1.UpdateSidebarLinkRequest]) (*connect.Response[adminv1.UpdateSidebarLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	link, err := s.api.core.AdminUpdateSidebarLink(ctx, caller.UserID, req.Msg.GetLinkId(), req.Msg.Label, req.Msg.Url)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.UpdateSidebarLinkResponse{SidebarLink: apiSidebarLink(link)}), nil
}

func (s *adminRoomLayoutService) DeleteSidebarLink(ctx context.Context, req *connect.Request[adminv1.DeleteSidebarLinkRequest]) (*connect.Response[adminv1.DeleteSidebarLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.AdminDeleteSidebarLink(ctx, caller.UserID, req.Msg.GetLinkId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.DeleteSidebarLinkResponse{Deleted: true}), nil
}

func (s *adminRoomLayoutService) MoveSidebarLinkToGroup(ctx context.Context, req *connect.Request[adminv1.MoveSidebarLinkToGroupRequest]) (*connect.Response[adminv1.MoveSidebarLinkToGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	link, err := s.api.core.AdminMoveSidebarLinkToGroup(ctx, caller.UserID, req.Msg.GetLinkId(), req.Msg.GetGroupId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.MoveSidebarLinkToGroupResponse{SidebarLink: apiSidebarLink(link)}), nil
}

func (s *adminRoomLayoutService) getAdminRoomLayoutGroups(ctx context.Context, userID string) ([]*adminv1.AdminRoomLayoutGroup, error) {
	groups, err := s.api.core.ListRoomGroupsOrdered(ctx, core.KindChannel)
	if err != nil {
		return nil, err
	}
	roomsByID, err := s.channelRoomsByID(ctx)
	if err != nil {
		return nil, err
	}
	apiGroups := make([]*adminv1.AdminRoomLayoutGroup, 0, len(groups))
	for _, group := range groups {
		apiGroup := apiAdminRoomLayoutGroup(group, roomsByID)
		canCreateRoom, err := s.api.core.CanCreateRoom(ctx, userID, core.KindChannel, group.GetId())
		if err != nil {
			return nil, err
		}
		apiGroup.CanCreateRoom = canCreateRoom
		apiGroups = append(apiGroups, apiGroup)
	}
	return apiGroups, nil
}

func (s *adminRoomLayoutService) channelRoomsByID(ctx context.Context) (map[string]*corev1.Room, error) {
	rooms, err := s.api.core.ListRooms(ctx, core.KindChannel)
	if err != nil {
		return nil, err
	}
	roomsByID := make(map[string]*corev1.Room, len(rooms))
	for _, room := range rooms {
		roomsByID[room.GetId()] = room
	}
	return roomsByID, nil
}

func (s *adminRoomLayoutService) roomGroup(ctx context.Context, groupID string) (*corev1.RoomGroup, error) {
	groups, err := s.api.core.ListRoomGroupsOrdered(ctx, core.KindChannel)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if group.GetId() == groupID {
			return group, nil
		}
	}
	return nil, core.ErrRoomGroupNotFound
}

func apiAdminRoomLayoutGroup(group *corev1.RoomGroup, roomsByID map[string]*corev1.Room) *adminv1.AdminRoomLayoutGroup {
	if group == nil {
		return nil
	}
	apiGroup := &adminv1.AdminRoomLayoutGroup{
		Id:          group.GetId(),
		Name:        group.GetName(),
		Description: group.GetDescription(),
	}
	sidebarLinksByID := make(map[string]*corev1.SidebarLink, len(group.GetSidebarLinks()))
	for _, link := range group.GetSidebarLinks() {
		sidebarLinksByID[link.GetId()] = link
	}
	for _, entry := range group.GetEntries() {
		switch entry.GetKind() {
		case corev1.SidebarGroupEntry_ROOM:
			room := roomsByID[entry.GetId()]
			if room == nil {
				continue
			}
			apiGroup.Items = append(apiGroup.Items, &adminv1.AdminRoomLayoutItem{
				Item: &adminv1.AdminRoomLayoutItem_Room{Room: apiRoom(room)},
			})
		case corev1.SidebarGroupEntry_SIDEBAR_LINK:
			link := sidebarLinksByID[entry.GetId()]
			if link == nil {
				continue
			}
			apiGroup.Items = append(apiGroup.Items, &adminv1.AdminRoomLayoutItem{
				Item: &adminv1.AdminRoomLayoutItem_SidebarLink{SidebarLink: apiSidebarLink(link)},
			})
		}
	}
	return apiGroup
}

func adminRoomLayoutItemInputsToCore(items []*adminv1.AdminRoomLayoutItemInput) []*corev1.SidebarGroupEntry {
	entries := make([]*corev1.SidebarGroupEntry, 0, len(items))
	for _, item := range items {
		var kind corev1.SidebarGroupEntry_Kind
		switch item.GetKind() {
		case adminv1.AdminRoomLayoutItemKind_ADMIN_ROOM_LAYOUT_ITEM_KIND_ROOM:
			kind = corev1.SidebarGroupEntry_ROOM
		case adminv1.AdminRoomLayoutItemKind_ADMIN_ROOM_LAYOUT_ITEM_KIND_SIDEBAR_LINK:
			kind = corev1.SidebarGroupEntry_SIDEBAR_LINK
		default:
			kind = corev1.SidebarGroupEntry_KIND_UNSPECIFIED
		}
		entries = append(entries, &corev1.SidebarGroupEntry{Kind: kind, Id: item.GetId()})
	}
	return entries
}

func sidebarLinkFromAdminRoomLayoutGroup(group *corev1.RoomGroup, linkID string) *corev1.SidebarLink {
	for _, link := range group.GetSidebarLinks() {
		if link.GetId() == linkID {
			return link
		}
	}
	return nil
}
