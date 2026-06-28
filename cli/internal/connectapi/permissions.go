package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type permissionService struct {
	api *API
}

func (s *permissionService) GetRolePermissionTierMatrix(ctx context.Context, req *connect.Request[apiv1.GetRolePermissionTierMatrixRequest]) (*connect.Response[apiv1.GetRolePermissionTierMatrixResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	matrix, err := s.api.core.GetRolePermissionTierMatrix(ctx, caller.UserID, req.Msg.GetRoomId(), req.Msg.GetGroupId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetRolePermissionTierMatrixResponse{Matrix: apiTierRoles(matrix)}), nil
}

func (s *permissionService) GetRolePermissionMatrix(ctx context.Context, req *connect.Request[apiv1.GetRolePermissionMatrixRequest]) (*connect.Response[apiv1.GetRolePermissionMatrixResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	matrix, err := s.api.core.GetRolePermissionMatrix(ctx, caller.UserID, req.Msg.GetRoleName())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetRolePermissionMatrixResponse{Matrix: apiRolePermissionMatrix(matrix)}), nil
}

func (s *permissionService) GetUserPermissionMatrix(ctx context.Context, req *connect.Request[apiv1.GetUserPermissionMatrixRequest]) (*connect.Response[apiv1.GetUserPermissionMatrixResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	matrix, err := s.api.core.GetUserPermissionMatrix(ctx, caller.UserID, req.Msg.GetUserId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetUserPermissionMatrixResponse{Matrix: apiUserPermissionMatrix(matrix)}), nil
}

func (s *permissionService) ExplainPermissions(ctx context.Context, req *connect.Request[apiv1.ExplainPermissionsRequest]) (*connect.Response[apiv1.ExplainPermissionsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	explanations, err := s.api.core.ExplainPermissions(ctx, caller.UserID, req.Msg.GetUserId(), req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ExplainPermissionsResponse{Explanations: apiPermissionExplanations(explanations)}), nil
}

func (s *permissionService) SetRolePermission(ctx context.Context, req *connect.Request[apiv1.SetRolePermissionRequest]) (*connect.Response[apiv1.SetRolePermissionResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := corePermissionState(req.Msg.GetDecision())
	if err != nil {
		return nil, err
	}
	scope := corePermissionTargetScope(req.Msg.GetScope())
	if err := s.api.core.SetRolePermissionState(ctx, caller.UserID, req.Msg.GetRoleName(), scope, core.Permission(req.Msg.GetPermission()), state); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.SetRolePermissionResponse{Ok: true}), nil
}

func (s *permissionService) RevokeRolePermissionGrant(ctx context.Context, req *connect.Request[apiv1.RevokeRolePermissionGrantRequest]) (*connect.Response[apiv1.RevokeRolePermissionGrantResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.AdminRevokeRolePermissionGrant(ctx, caller.UserID, req.Msg.GetRoleName(), core.Permission(req.Msg.GetPermission())); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.RevokeRolePermissionGrantResponse{Ok: true}), nil
}

func (s *permissionService) SetUserPermission(ctx context.Context, req *connect.Request[apiv1.SetUserPermissionRequest]) (*connect.Response[apiv1.SetUserPermissionResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := corePermissionState(req.Msg.GetDecision())
	if err != nil {
		return nil, err
	}
	scope := corePermissionTargetScope(req.Msg.GetScope())
	if err := s.api.core.SetUserPermissionState(ctx, caller.UserID, req.Msg.GetUserId(), scope, core.Permission(req.Msg.GetPermission()), state); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.SetUserPermissionResponse{Ok: true}), nil
}

func apiPermissionExplanations(explanations []core.PermissionExplanation) []*apiv1.PermissionExplanation {
	out := make([]*apiv1.PermissionExplanation, 0, len(explanations))
	for i := range explanations {
		out = append(out, apiPermissionExplanation(explanations[i]))
	}
	return out
}

func apiPermissionExplanation(explanation core.PermissionExplanation) *apiv1.PermissionExplanation {
	out := &apiv1.PermissionExplanation{
		Permission:    string(explanation.Permission),
		State:         apiPermissionExplanationDecision(explanation.State),
		DecidedAt:     apiPermissionDecisionLevel(explanation.DecidedAt),
		DecidedByRole: explanation.DecidedByRole,
		Trace:         make([]*apiv1.PermissionTraceEntry, 0, len(explanation.Trace)),
	}
	for i, entry := range explanation.Trace {
		out.Trace = append(out.Trace, &apiv1.PermissionTraceEntry{
			Level:    apiPermissionDecisionLevel(entry.Level),
			RoleName: entry.RoleName,
			Decision: apiPermissionExplanationDecision(entry.Decision),
			Applied:  i == 0,
		})
	}
	return out
}

func apiPermissionExplanationDecision(decision core.DecisionKind) apiv1.PermissionDecision {
	switch decision {
	case core.DecisionAllow:
		return apiv1.PermissionDecision_PERMISSION_DECISION_ALLOW
	case core.DecisionDeny:
		return apiv1.PermissionDecision_PERMISSION_DECISION_DENY
	default:
		return apiv1.PermissionDecision_PERMISSION_DECISION_NONE
	}
}

func apiPermissionDecisionLevel(level core.PermissionLevel) apiv1.PermissionDecisionLevel {
	switch level {
	case core.LevelServer:
		return apiv1.PermissionDecisionLevel_PERMISSION_DECISION_LEVEL_SERVER
	case core.LevelGroup:
		return apiv1.PermissionDecisionLevel_PERMISSION_DECISION_LEVEL_GROUP
	case core.LevelRoom:
		return apiv1.PermissionDecisionLevel_PERMISSION_DECISION_LEVEL_ROOM
	default:
		return apiv1.PermissionDecisionLevel_PERMISSION_DECISION_LEVEL_UNSPECIFIED
	}
}

func apiTierRoles(matrix *core.TierRoles) *apiv1.TierRoles {
	if matrix == nil {
		return nil
	}
	out := &apiv1.TierRoles{
		ApplicablePermissions: append([]string(nil), matrix.ApplicablePermissions...),
		Roles:                 make([]*apiv1.TierRole, 0, len(matrix.Roles)),
	}
	for _, role := range matrix.Roles {
		out.Roles = append(out.Roles, &apiv1.TierRole{
			RoleName:         role.RoleName,
			DisplayName:      role.DisplayName,
			Description:      role.Description,
			IsSystem:         role.IsSystem,
			Position:         role.Position,
			Override:         apiTierPermissions(role.Override),
			InheritedAllows:  append([]string(nil), role.InheritedAllows...),
			InheritedDenials: append([]string(nil), role.InheritedDenials...),
		})
	}
	return out
}

func apiTierPermissions(perms core.TierPermissions) *apiv1.TierPermissions {
	return &apiv1.TierPermissions{
		Permissions:       append([]string(nil), perms.Permissions...),
		PermissionDenials: append([]string(nil), perms.PermissionDenials...),
	}
}

func apiRolePermissionMatrix(matrix *core.RolePermissionMatrix) *apiv1.RolePermissionMatrix {
	if matrix == nil {
		return nil
	}
	return &apiv1.RolePermissionMatrix{
		RoleName:              matrix.RoleName,
		ApplicablePermissions: append([]string(nil), matrix.ApplicablePermissions...),
		Scopes:                apiPermissionMatrixScopes(matrix.Scopes),
		Cells:                 apiPermissionMatrixCells(matrix.Cells),
	}
}

func apiUserPermissionMatrix(matrix *core.UserPermissionMatrix) *apiv1.UserPermissionMatrix {
	if matrix == nil {
		return nil
	}
	return &apiv1.UserPermissionMatrix{
		UserId:                matrix.UserID,
		ApplicablePermissions: append([]string(nil), matrix.ApplicablePermissions...),
		Scopes:                apiPermissionMatrixScopes(matrix.Scopes),
		Cells:                 apiPermissionMatrixCells(matrix.Cells),
	}
}

func apiPermissionMatrixScopes(scopes []core.PermissionMatrixScope) []*apiv1.PermissionMatrixScope {
	out := make([]*apiv1.PermissionMatrixScope, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, &apiv1.PermissionMatrixScope{
			Id:            scope.ID,
			Label:         scope.Label,
			Kind:          apiPermissionScopeKind(scope.Kind),
			ParentGroupId: scope.ParentGroupID,
		})
	}
	return out
}

func apiPermissionMatrixCells(cells []core.PermissionMatrixCell) []*apiv1.PermissionMatrixCell {
	out := make([]*apiv1.PermissionMatrixCell, 0, len(cells))
	for _, cell := range cells {
		out = append(out, &apiv1.PermissionMatrixCell{
			Permission: cell.Permission,
			ScopeId:    cell.ScopeID,
			Override:   apiPermissionDecision(cell.Override),
			Effective:  apiPermissionDecision(cell.Effective),
		})
	}
	return out
}

func apiPermissionScopeKind(kind core.MatrixScopeKind) apiv1.PermissionScopeKind {
	switch kind {
	case core.MatrixScopeGroup:
		return apiv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_GROUP
	case core.MatrixScopeRoom:
		return apiv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM
	default:
		return apiv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER
	}
}

func apiPermissionDecision(decision core.MatrixDecision) apiv1.PermissionDecision {
	switch decision {
	case core.MatrixDecisionAllow:
		return apiv1.PermissionDecision_PERMISSION_DECISION_ALLOW
	case core.MatrixDecisionDeny:
		return apiv1.PermissionDecision_PERMISSION_DECISION_DENY
	default:
		return apiv1.PermissionDecision_PERMISSION_DECISION_NONE
	}
}

func corePermissionState(decision apiv1.PermissionDecision) (core.PermissionState, error) {
	switch decision {
	case apiv1.PermissionDecision_PERMISSION_DECISION_ALLOW:
		return core.PermissionStateAllow, nil
	case apiv1.PermissionDecision_PERMISSION_DECISION_DENY:
		return core.PermissionStateDeny, nil
	case apiv1.PermissionDecision_PERMISSION_DECISION_NONE:
		return core.PermissionStateNone, nil
	default:
		return "", invalidArgument("decision is required")
	}
}

func corePermissionTargetScope(scope *apiv1.PermissionScope) core.PermissionTargetScope {
	if scope == nil {
		return core.PermissionTargetScope{}
	}
	switch scope.GetKind() {
	case apiv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_GROUP:
		return core.PermissionTargetScope{Kind: core.MatrixScopeGroup, ID: scope.GetId()}
	case apiv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM:
		return core.PermissionTargetScope{Kind: core.MatrixScopeRoom, ID: scope.GetId()}
	default:
		return core.PermissionTargetScope{}
	}
}
