package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func (s *botService) ListBotPermissions(ctx context.Context, req *connect.Request[apiv1.ListBotPermissionsRequest]) (*connect.Response[apiv1.ListBotPermissionsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.api.core.ListBotPermissions(ctx, caller.UserID, req.Msg.GetBotUserId())
	if err != nil {
		return nil, connectError(err)
	}
	limit, offset := apiPagination(req.Msg.GetPage(), 20, 100)
	page, total, more := apiSlicePage(entries, limit, offset)
	permissions := make([]*apiv1.BotPermission, 0, len(page))
	for _, entry := range page {
		value := &apiv1.BotPermission{Permission: string(entry.Permission), Active: entry.Active}
		switch entry.Scope.Kind {
		case core.MatrixScopeServer:
			value.Scope = apiv1.BotPermissionScope_BOT_PERMISSION_SCOPE_SERVER
		case core.MatrixScopeDM:
			value.Scope = apiv1.BotPermissionScope_BOT_PERMISSION_SCOPE_DM
		case core.MatrixScopeGroup:
			value.Scope = apiv1.BotPermissionScope_BOT_PERMISSION_SCOPE_GROUP
			value.ScopeId = entry.Scope.ID[len("group:"):]
			value.ScopeName = entry.Scope.Label
		case core.MatrixScopeRoom:
			value.Scope = apiv1.BotPermissionScope_BOT_PERMISSION_SCOPE_ROOM
			value.ScopeId = entry.Scope.ID[len("room:"):]
			value.ScopeName = entry.Scope.Label
		}
		permissions = append(permissions, value)
	}
	return connect.NewResponse(&apiv1.ListBotPermissionsResponse{Permissions: permissions, Page: apiPageInfo(total, more)}), nil
}
