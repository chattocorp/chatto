package http_server

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireGetAdminRoomLayout(ctx context.Context, userID, requestID string) (*apiv1.GetAdminRoomLayoutResponse, *wirev1.WireError) {
	groups, err := c.adminRoomLayoutView(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.GetAdminRoomLayoutResponse{Groups: groups}, nil
}

func (c *wireConn) handleWireCreateAdminRoomGroup(ctx context.Context, userID, requestID string, body *apiv1.CreateAdminRoomGroupRequest) (*apiv1.CreateAdminRoomGroupResponse, *wirev1.WireError) {
	if err := c.requireWireRoleManage(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	group, err := c.server.core.CreateRoomGroup(ctx, userID, body.GetName(), body.GetDescription())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.CreateAdminRoomGroupResponse{Group: adminRoomGroupView(group, nil)}, nil
}

func (c *wireConn) handleWireUpdateAdminRoomGroup(ctx context.Context, userID, requestID string, body *apiv1.UpdateAdminRoomGroupRequest) (*apiv1.UpdateAdminRoomGroupResponse, *wirev1.WireError) {
	if body.GetGroupId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: group_id is required", errWireInvalidArgument))
	}
	if err := c.requireWireRoleManage(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	group, err := c.server.core.UpdateRoomGroup(ctx, userID, body.GetGroupId(), body.GetName(), body.GetDescription())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.UpdateAdminRoomGroupResponse{Group: adminRoomGroupView(group, nil)}, nil
}

func (c *wireConn) handleWireDeleteAdminRoomGroup(ctx context.Context, userID, requestID string, body *apiv1.DeleteAdminRoomGroupRequest) (*apiv1.DeleteAdminRoomGroupResponse, *wirev1.WireError) {
	if body.GetGroupId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: group_id is required", errWireInvalidArgument))
	}
	if err := c.requireWireRoleManage(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.DeleteRoomGroup(ctx, userID, body.GetGroupId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.DeleteAdminRoomGroupResponse{Deleted: true}, nil
}

func (c *wireConn) handleWireReorderAdminRoomGroups(ctx context.Context, userID, requestID string, body *apiv1.ReorderAdminRoomGroupsRequest) (*apiv1.ReorderAdminRoomGroupsResponse, *wirev1.WireError) {
	if err := c.requireWireRoleManage(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.ReorderRoomGroups(ctx, userID, body.GetOrderedGroupIds()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	groups, err := c.adminRoomLayoutView(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.ReorderAdminRoomGroupsResponse{Groups: groups}, nil
}

func (c *wireConn) handleWireMoveAdminRoomToGroup(ctx context.Context, userID, requestID string, body *apiv1.MoveAdminRoomToGroupRequest) (*apiv1.MoveAdminRoomToGroupResponse, *wirev1.WireError) {
	if body.GetRoomId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: room_id is required", errWireInvalidArgument))
	}
	if body.GetGroupId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: group_id is required", errWireInvalidArgument))
	}
	if err := c.requireWireRoleManage(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.MoveRoomToGroup(ctx, userID, body.GetRoomId(), body.GetGroupId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	room, err := c.server.core.GetRoom(ctx, core.KindChannel, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.MoveAdminRoomToGroupResponse{Room: adminRoomInfoView(room)}, nil
}

func (c *wireConn) handleWireReorderAdminRoomsInGroup(ctx context.Context, userID, requestID string, body *apiv1.ReorderAdminRoomsInGroupRequest) (*apiv1.ReorderAdminRoomsInGroupResponse, *wirev1.WireError) {
	if body.GetGroupId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: group_id is required", errWireInvalidArgument))
	}
	if err := c.requireWireRoleManage(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.ReorderRoomsInGroup(ctx, userID, body.GetGroupId(), body.GetOrderedRoomIds()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	group, err := c.server.core.GetRoomGroup(ctx, body.GetGroupId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	visibleRooms, err := c.visibleAdminRoomMap(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.ReorderAdminRoomsInGroupResponse{Group: adminRoomGroupView(group, visibleRooms)}, nil
}

func (c *wireConn) handleWireUpdateAdminRoom(ctx context.Context, userID, requestID string, body *apiv1.UpdateAdminRoomRequest) (*apiv1.UpdateAdminRoomResponse, *wirev1.WireError) {
	room, kind, wireErr := c.adminRoomAndKind(ctx, requestID, body.GetRoomId())
	if wireErr != nil {
		return nil, wireErr
	}
	if err := c.requireWireRoomManage(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	updated, err := c.server.core.UpdateRoom(ctx, userID, kind, room.GetId(), body.GetName(), body.GetDescription())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.UpdateAdminRoomResponse{Room: adminRoomInfoView(updated)}, nil
}

func (c *wireConn) handleWireArchiveAdminRoom(ctx context.Context, userID, requestID string, body *apiv1.ArchiveAdminRoomRequest) (*apiv1.ArchiveAdminRoomResponse, *wirev1.WireError) {
	room, kind, wireErr := c.adminRoomAndKind(ctx, requestID, body.GetRoomId())
	if wireErr != nil {
		return nil, wireErr
	}
	if err := c.requireWireRoomManage(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	updated, err := c.server.core.ArchiveRoom(ctx, userID, kind, room.GetId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.ArchiveAdminRoomResponse{Room: adminRoomInfoView(updated)}, nil
}

func (c *wireConn) handleWireUnarchiveAdminRoom(ctx context.Context, userID, requestID string, body *apiv1.UnarchiveAdminRoomRequest) (*apiv1.UnarchiveAdminRoomResponse, *wirev1.WireError) {
	room, kind, wireErr := c.adminRoomAndKind(ctx, requestID, body.GetRoomId())
	if wireErr != nil {
		return nil, wireErr
	}
	if err := c.requireWireRoomManage(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	updated, err := c.server.core.UnarchiveRoom(ctx, userID, kind, room.GetId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.UnarchiveAdminRoomResponse{Room: adminRoomInfoView(updated)}, nil
}

func (c *wireConn) adminRoomLayoutView(ctx context.Context, userID string) ([]*apiv1.AdminRoomGroupView, error) {
	groups, err := c.server.core.ListRoomGroupsOrdered(ctx, core.KindChannel)
	if err != nil {
		return nil, err
	}
	visibleRooms, err := c.visibleAdminRoomMap(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make([]*apiv1.AdminRoomGroupView, 0, len(groups))
	for _, group := range groups {
		out = append(out, adminRoomGroupView(group, visibleRooms))
	}
	return out, nil
}

func (c *wireConn) visibleAdminRoomMap(ctx context.Context, userID string) (map[string]*corev1.Room, error) {
	rooms, err := c.server.core.ListRooms(ctx, core.KindChannel)
	if err != nil {
		return nil, err
	}
	visibleRooms := make(map[string]*corev1.Room, len(rooms))
	for _, room := range rooms {
		visible, err := c.server.core.CanSeeRoom(ctx, userID, core.KindChannel, room.GetId())
		if err != nil {
			return nil, err
		}
		if visible {
			visibleRooms[room.GetId()] = room
		}
	}
	return visibleRooms, nil
}

func adminRoomGroupView(group *corev1.RoomGroup, rooms map[string]*corev1.Room) *apiv1.AdminRoomGroupView {
	if group == nil {
		return nil
	}
	view := &apiv1.AdminRoomGroupView{
		Id:   group.GetId(),
		Name: group.GetName(),
	}
	if rooms == nil {
		return view
	}
	view.Rooms = make([]*apiv1.AdminRoomInfoView, 0, len(group.GetRoomIds()))
	for _, roomID := range group.GetRoomIds() {
		if room := rooms[roomID]; room != nil {
			view.Rooms = append(view.Rooms, adminRoomInfoView(room))
		}
	}
	return view
}

func adminRoomInfoView(room *corev1.Room) *apiv1.AdminRoomInfoView {
	if room == nil {
		return nil
	}
	return &apiv1.AdminRoomInfoView{
		Id:          room.GetId(),
		Name:        room.GetName(),
		Description: room.GetDescription(),
		Archived:    room.GetArchived(),
	}
}

func (c *wireConn) adminRoomAndKind(ctx context.Context, requestID, roomID string) (*corev1.Room, core.RoomKind, *wirev1.WireError) {
	if roomID == "" {
		return nil, "", c.errorFromRequestErr(requestID, fmt.Errorf("%w: room_id is required", errWireInvalidArgument))
	}
	room, err := c.server.core.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, "", c.errorFromRequestErr(requestID, err)
	}
	return room, core.KindOfRoom(room), nil
}

func (c *wireConn) requireWireRoleManage(ctx context.Context, userID string) error {
	can, err := c.server.core.CanManageRoles(ctx, userID)
	if err != nil {
		return fmt.Errorf("check role.manage: %w", err)
	}
	if !can {
		return core.ErrPermissionDenied
	}
	return nil
}

func (c *wireConn) requireWireRoomManage(ctx context.Context, userID string) error {
	can, err := c.server.core.CanManageAnyRoom(ctx, userID)
	if err != nil {
		return fmt.Errorf("check room.manage: %w", err)
	}
	if !can {
		return core.ErrPermissionDenied
	}
	return nil
}
