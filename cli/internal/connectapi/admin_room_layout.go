package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	appv1 "hmans.de/chatto/internal/pb/chatto/app/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type adminRoomLayoutService struct {
	api *API
}

func (s *adminRoomLayoutService) ListAdminRoomLayout(ctx context.Context, _ *connect.Request[appv1.ListAdminRoomLayoutRequest]) (*connect.Response[appv1.ListAdminRoomLayoutResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	groups, err := s.listAdminRoomLayoutGroups(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.ListAdminRoomLayoutResponse{Groups: groups}), nil
}

func (s *adminRoomLayoutService) CreateRoomGroup(ctx context.Context, req *connect.Request[appv1.CreateRoomGroupRequest]) (*connect.Response[appv1.CreateRoomGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	group, err := s.api.core.AdminCreateRoomGroup(ctx, caller.UserID, req.Msg.GetName(), req.Msg.GetDescription())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.CreateRoomGroupResponse{
		Group: apiAdminRoomLayoutGroup(group, nil),
	}), nil
}

func (s *adminRoomLayoutService) UpdateRoomGroup(ctx context.Context, req *connect.Request[appv1.UpdateRoomGroupRequest]) (*connect.Response[appv1.UpdateRoomGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	group, err := s.api.core.AdminUpdateRoomGroup(ctx, caller.UserID, req.Msg.GetGroupId(), req.Msg.GetName(), req.Msg.GetDescription())
	if err != nil {
		return nil, connectError(err)
	}
	roomsByID, err := s.visibleChannelRoomsByID(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.UpdateRoomGroupResponse{
		Group: apiAdminRoomLayoutGroup(group, roomsByID),
	}), nil
}

func (s *adminRoomLayoutService) DeleteRoomGroup(ctx context.Context, req *connect.Request[appv1.DeleteRoomGroupRequest]) (*connect.Response[appv1.DeleteRoomGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.AdminDeleteRoomGroup(ctx, caller.UserID, req.Msg.GetGroupId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.DeleteRoomGroupResponse{Deleted: true}), nil
}

func (s *adminRoomLayoutService) ReorderRoomGroups(ctx context.Context, req *connect.Request[appv1.ReorderRoomGroupsRequest]) (*connect.Response[appv1.ReorderRoomGroupsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.AdminReorderRoomGroups(ctx, caller.UserID, req.Msg.GetOrderedGroupIds()); err != nil {
		return nil, connectError(err)
	}
	groups, err := s.listAdminRoomLayoutGroups(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.ReorderRoomGroupsResponse{Groups: groups}), nil
}

func (s *adminRoomLayoutService) MoveRoomToGroup(ctx context.Context, req *connect.Request[appv1.MoveRoomToGroupRequest]) (*connect.Response[appv1.MoveRoomToGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	moved, err := s.api.core.AdminMoveRoomToGroup(ctx, caller.UserID, req.Msg.GetRoomId(), req.Msg.GetGroupId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.MoveRoomToGroupResponse{Room: apiRoom(moved)}), nil
}

func (s *adminRoomLayoutService) ReorderSidebarItemsInGroup(ctx context.Context, req *connect.Request[appv1.ReorderSidebarItemsInGroupRequest]) (*connect.Response[appv1.ReorderSidebarItemsInGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	group, err := s.api.core.AdminReorderSidebarItemsInGroup(ctx, caller.UserID, req.Msg.GetGroupId(), adminRoomLayoutItemInputsToCore(req.Msg.GetItems()))
	if err != nil {
		return nil, connectError(err)
	}
	roomsByID, err := s.visibleChannelRoomsByID(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.ReorderSidebarItemsInGroupResponse{
		Group: apiAdminRoomLayoutGroup(group, roomsByID),
	}), nil
}

func (s *adminRoomLayoutService) CreateSidebarLink(ctx context.Context, req *connect.Request[appv1.CreateSidebarLinkRequest]) (*connect.Response[appv1.CreateSidebarLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	link, err := s.api.core.AdminCreateSidebarLink(ctx, caller.UserID, req.Msg.GetGroupId(), req.Msg.GetLabel(), req.Msg.GetUrl())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.CreateSidebarLinkResponse{SidebarLink: apiSidebarLink(link)}), nil
}

func (s *adminRoomLayoutService) UpdateSidebarLink(ctx context.Context, req *connect.Request[appv1.UpdateSidebarLinkRequest]) (*connect.Response[appv1.UpdateSidebarLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	link, err := s.api.core.AdminUpdateSidebarLink(ctx, caller.UserID, req.Msg.GetLinkId(), req.Msg.GetLabel(), req.Msg.GetUrl())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.UpdateSidebarLinkResponse{SidebarLink: apiSidebarLink(link)}), nil
}

func (s *adminRoomLayoutService) DeleteSidebarLink(ctx context.Context, req *connect.Request[appv1.DeleteSidebarLinkRequest]) (*connect.Response[appv1.DeleteSidebarLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.AdminDeleteSidebarLink(ctx, caller.UserID, req.Msg.GetLinkId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.DeleteSidebarLinkResponse{Deleted: true}), nil
}

func (s *adminRoomLayoutService) MoveSidebarLinkToGroup(ctx context.Context, req *connect.Request[appv1.MoveSidebarLinkToGroupRequest]) (*connect.Response[appv1.MoveSidebarLinkToGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	link, err := s.api.core.AdminMoveSidebarLinkToGroup(ctx, caller.UserID, req.Msg.GetLinkId(), req.Msg.GetGroupId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.MoveSidebarLinkToGroupResponse{SidebarLink: apiSidebarLink(link)}), nil
}

func (s *adminRoomLayoutService) listAdminRoomLayoutGroups(ctx context.Context, userID string) ([]*appv1.AdminRoomLayoutGroup, error) {
	groups, err := s.api.core.ListRoomGroupsOrdered(ctx, core.KindChannel)
	if err != nil {
		return nil, err
	}
	roomsByID, err := s.visibleChannelRoomsByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	apiGroups := make([]*appv1.AdminRoomLayoutGroup, 0, len(groups))
	for _, group := range groups {
		apiGroups = append(apiGroups, apiAdminRoomLayoutGroup(group, roomsByID))
	}
	return apiGroups, nil
}

func (s *adminRoomLayoutService) visibleChannelRoomsByID(ctx context.Context, userID string) (map[string]*corev1.Room, error) {
	rooms, err := s.api.core.ListRooms(ctx, core.KindChannel)
	if err != nil {
		return nil, err
	}
	roomsByID := make(map[string]*corev1.Room, len(rooms))
	for _, room := range rooms {
		visible, err := s.api.core.CanSeeRoom(ctx, userID, core.KindChannel, room.GetId())
		if err != nil {
			return nil, err
		}
		if visible {
			roomsByID[room.GetId()] = room
		}
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

func apiAdminRoomLayoutGroup(group *corev1.RoomGroup, roomsByID map[string]*corev1.Room) *appv1.AdminRoomLayoutGroup {
	if group == nil {
		return nil
	}
	apiGroup := &appv1.AdminRoomLayoutGroup{
		Id:          group.GetId(),
		Name:        group.GetName(),
		Description: group.GetDescription(),
	}
	sidebarLinksByID := make(map[string]*corev1.SidebarLink, len(group.GetSidebarLinks()))
	for _, link := range group.GetSidebarLinks() {
		sidebarLinksByID[link.GetId()] = link
	}
	for _, roomID := range group.GetRoomIds() {
		room := roomsByID[roomID]
		if roomsByID == nil {
			room = nil
		}
		if room == nil {
			continue
		}
		apiGroup.Rooms = append(apiGroup.Rooms, apiAdminRoomLayoutRoom(room))
	}
	for _, entry := range group.GetEntries() {
		switch entry.GetKind() {
		case corev1.SidebarGroupEntry_ROOM:
			room := roomsByID[entry.GetId()]
			if room == nil {
				continue
			}
			apiGroup.Items = append(apiGroup.Items, &appv1.AdminRoomLayoutItem{
				Item: &appv1.AdminRoomLayoutItem_Room{Room: apiAdminRoomLayoutRoom(room)},
			})
		case corev1.SidebarGroupEntry_SIDEBAR_LINK:
			link := sidebarLinksByID[entry.GetId()]
			if link == nil {
				continue
			}
			apiGroup.Items = append(apiGroup.Items, &appv1.AdminRoomLayoutItem{
				Item: &appv1.AdminRoomLayoutItem_SidebarLink{SidebarLink: apiSidebarLink(link)},
			})
		}
	}
	return apiGroup
}

func apiAdminRoomLayoutRoom(room *corev1.Room) *appv1.AdminRoomLayoutRoom {
	if room == nil {
		return nil
	}
	return &appv1.AdminRoomLayoutRoom{
		Id:          room.GetId(),
		Name:        room.GetName(),
		Description: room.GetDescription(),
		Archived:    room.GetArchived(),
		Universal:   room.GetUniversal(),
	}
}

func adminRoomLayoutItemInputsToCore(items []*appv1.AdminRoomLayoutItemInput) []*corev1.SidebarGroupEntry {
	entries := make([]*corev1.SidebarGroupEntry, 0, len(items))
	for _, item := range items {
		var kind corev1.SidebarGroupEntry_Kind
		switch item.GetKind() {
		case appv1.AdminRoomLayoutItemKind_ADMIN_ROOM_LAYOUT_ITEM_KIND_ROOM:
			kind = corev1.SidebarGroupEntry_ROOM
		case appv1.AdminRoomLayoutItemKind_ADMIN_ROOM_LAYOUT_ITEM_KIND_SIDEBAR_LINK:
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
