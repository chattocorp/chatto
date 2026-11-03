package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
)

func (s *adminRoomLayoutService) GetRoomLayout(ctx context.Context, req *connect.Request[adminv1.GetRoomLayoutRequest]) (*connect.Response[adminv1.GetRoomLayoutResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	allowed, err := s.api.core.CanManageAnyRoom(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, core.ErrPermissionDenied
	}
	groups, err := s.api.core.ListRoomGroupsOrdered(ctx, core.KindChannel)
	if err != nil {
		return nil, err
	}
	layout, err := s.assembleAdminRoomLayout(ctx, groups)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.GetRoomLayoutResponse{Layout: layout}), nil
}

func (s *adminRoomLayoutService) GetRoomGroupLayout(ctx context.Context, req *connect.Request[adminv1.GetRoomGroupLayoutRequest]) (*connect.Response[adminv1.GetRoomGroupLayoutResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	group, err := s.api.core.RoomLayout().GetAdminRoomGroupLayout(ctx, caller.UserID, req.Msg.GetGroupId())
	if err != nil {
		return nil, err
	}
	layout, err := s.assembleAdminRoomGroupLayout(ctx, group)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.GetRoomGroupLayoutResponse{Layout: layout}), nil
}

func (s *adminRoomLayoutService) GetSidebarLink(ctx context.Context, req *connect.Request[adminv1.GetSidebarLinkRequest]) (*connect.Response[adminv1.GetSidebarLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	link, err := s.api.core.RoomLayout().GetAdminSidebarLink(ctx, caller.UserID, req.Msg.GetLinkId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.GetSidebarLinkResponse{Link: &adminv1.AdminSidebarLink{Link: apiSidebarLink(link.Link), GroupId: link.GroupID}}), nil
}
