package connectapi

import (
	"context"

	"connectrpc.com/connect"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
)

func (s *roomDirectoryService) GetSidebarLink(ctx context.Context, req *connect.Request[apiv1.GetSidebarLinkRequest]) (*connect.Response[apiv1.GetSidebarLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	link, err := s.api.core.RoomDirectoryReads().GetSidebarLink(ctx, caller.UserID, req.Msg.GetLinkId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetSidebarLinkResponse{Link: apiSidebarLink(link.Link, link.GroupID)}), nil
}

func (s *roomDirectoryService) BatchGetSidebarLinks(ctx context.Context, req *connect.Request[apiv1.BatchGetSidebarLinksRequest]) (*connect.Response[apiv1.BatchGetSidebarLinksResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	links, err := s.api.core.RoomDirectoryReads().BatchGetSidebarLinks(ctx, caller.UserID, req.Msg.GetLinkIds())
	if err != nil {
		return nil, err
	}
	response := &apiv1.BatchGetSidebarLinksResponse{Links: make([]*apiv1.SidebarLink, 0, len(links))}
	for _, link := range links {
		response.Links = append(response.Links, apiSidebarLink(link.Link, link.GroupID))
	}
	return connect.NewResponse(response), nil
}
