package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type effectivePermissionService struct{ api *API }

func (s *effectivePermissionService) ListEffectivePermissions(ctx context.Context, req *connect.Request[apiv1.ListEffectivePermissionsRequest]) (*connect.Response[apiv1.ListEffectivePermissionsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.api.core.ListEffectivePermissions(ctx, caller.UserID, req.Msg.GetUserId())
	if err != nil {
		return nil, connectError(err)
	}
	result := make([]*apiv1.EffectivePermission, 0, len(entries))
	for _, entry := range entries {
		scope := &apiv1.EffectivePermissionScope{}
		switch entry.Scope.Kind {
		case core.MatrixScopeServer:
			scope.Kind = apiv1.EffectivePermissionScopeKind_EFFECTIVE_PERMISSION_SCOPE_KIND_SERVER
		case core.MatrixScopeDM:
			scope.Kind = apiv1.EffectivePermissionScopeKind_EFFECTIVE_PERMISSION_SCOPE_KIND_DM
		case core.MatrixScopeGroup:
			scope.Kind = apiv1.EffectivePermissionScopeKind_EFFECTIVE_PERMISSION_SCOPE_KIND_GROUP
			scope.Id, scope.Name = entry.Scope.ID[len("group:"):], entry.Scope.Label
		case core.MatrixScopeRoom:
			scope.Kind = apiv1.EffectivePermissionScopeKind_EFFECTIVE_PERMISSION_SCOPE_KIND_ROOM
			scope.Id, scope.Name, scope.ParentGroupId = entry.Scope.ID[len("room:"):], entry.Scope.Label, entry.Scope.ParentGroupID
		}
		result = append(result, &apiv1.EffectivePermission{Permission: string(entry.Permission), Scope: scope, CoversDescendants: entry.CoversDescendants})
	}
	return connect.NewResponse(&apiv1.ListEffectivePermissionsResponse{Permissions: result}), nil
}
