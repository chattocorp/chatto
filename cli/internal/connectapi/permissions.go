package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
)

type permissionService struct {
	api *API
}

func (s *permissionService) GetRolePermissionTierMatrix(ctx context.Context, req *connect.Request[adminv1.GetRolePermissionTierMatrixRequest]) (*connect.Response[adminv1.GetRolePermissionTierMatrixResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	roomID, groupID, err := permissionScopeIDs(req.Msg.GetScope())
	if err != nil {
		return nil, err
	}
	matrix, err := s.api.core.GetRolePermissionTierMatrix(ctx, caller.UserID, roomID, groupID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.GetRolePermissionTierMatrixResponse{Matrix: apiTierRoles(matrix)}), nil
}

func (s *permissionService) GetRolePermissionMatrix(ctx context.Context, req *connect.Request[adminv1.GetRolePermissionMatrixRequest]) (*connect.Response[adminv1.GetRolePermissionMatrixResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	matrix, err := s.api.core.GetRolePermissionMatrix(ctx, caller.UserID, req.Msg.GetRoleName())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.GetRolePermissionMatrixResponse{Matrix: apiRolePermissionMatrix(matrix)}), nil
}

func (s *permissionService) ListRolePermissionDecisions(ctx context.Context, req *connect.Request[adminv1.ListRolePermissionDecisionsRequest]) (*connect.Response[adminv1.ListRolePermissionDecisionsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	matrix, err := s.api.core.GetRolePermissionMatrix(ctx, caller.UserID, req.Msg.GetRoleName())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.ListRolePermissionDecisionsResponse{
		RoleName:  matrix.RoleName,
		Decisions: apiPermissionDecisionEntries(matrix.Scopes, matrix.Cells),
	}), nil
}

func (s *permissionService) GetUserPermissionMatrix(ctx context.Context, req *connect.Request[adminv1.GetUserPermissionMatrixRequest]) (*connect.Response[adminv1.GetUserPermissionMatrixResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	matrix, err := s.api.core.GetUserPermissionMatrix(ctx, caller.UserID, req.Msg.GetUserId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.GetUserPermissionMatrixResponse{Matrix: apiUserPermissionMatrix(matrix)}), nil
}

func (s *permissionService) ListUserPermissionDecisions(ctx context.Context, req *connect.Request[adminv1.ListUserPermissionDecisionsRequest]) (*connect.Response[adminv1.ListUserPermissionDecisionsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	matrix, err := s.api.core.GetUserPermissionMatrix(ctx, caller.UserID, req.Msg.GetUserId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.ListUserPermissionDecisionsResponse{
		UserId:    matrix.UserID,
		Decisions: apiPermissionDecisionEntries(matrix.Scopes, matrix.Cells),
	}), nil
}

func (s *permissionService) ExplainPermissions(ctx context.Context, req *connect.Request[adminv1.ExplainPermissionsRequest]) (*connect.Response[adminv1.ExplainPermissionsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	explanations, err := s.api.core.ExplainPermissions(ctx, caller.UserID, req.Msg.GetUserId(), req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.ExplainPermissionsResponse{Explanations: apiPermissionExplanations(explanations)}), nil
}

func (s *permissionService) SetRolePermission(ctx context.Context, req *connect.Request[adminv1.SetRolePermissionRequest]) (*connect.Response[adminv1.SetRolePermissionResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := corePermissionState(req.Msg.GetDecision())
	if err != nil {
		return nil, err
	}
	scope, err := corePermissionTargetScope(req.Msg.GetScope())
	if err != nil {
		return nil, err
	}
	if err := s.api.core.SetRolePermissionState(ctx, caller.UserID, req.Msg.GetRoleName(), scope, core.Permission(req.Msg.GetPermission()), state); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.SetRolePermissionResponse{Ok: true}), nil
}

func (s *permissionService) SetUserPermission(ctx context.Context, req *connect.Request[adminv1.SetUserPermissionRequest]) (*connect.Response[adminv1.SetUserPermissionResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := corePermissionState(req.Msg.GetDecision())
	if err != nil {
		return nil, err
	}
	scope, err := corePermissionTargetScope(req.Msg.GetScope())
	if err != nil {
		return nil, err
	}
	if err := s.api.core.SetUserPermissionState(ctx, caller.UserID, req.Msg.GetUserId(), scope, core.Permission(req.Msg.GetPermission()), state); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.SetUserPermissionResponse{Ok: true}), nil
}

func apiPermissionExplanations(explanations []core.PermissionExplanation) []*adminv1.PermissionExplanation {
	out := make([]*adminv1.PermissionExplanation, 0, len(explanations))
	for i := range explanations {
		out = append(out, apiPermissionExplanation(explanations[i]))
	}
	return out
}

func apiPermissionExplanation(explanation core.PermissionExplanation) *adminv1.PermissionExplanation {
	out := &adminv1.PermissionExplanation{
		Permission:    string(explanation.Permission),
		State:         apiPermissionExplanationDecision(explanation.State),
		DecidedAt:     apiPermissionDecisionLevel(explanation.DecidedAt),
		DecidedByRole: explanation.DecidedByRole,
		Trace:         make([]*adminv1.PermissionTraceEntry, 0, len(explanation.Trace)),
	}
	for i, entry := range winningTraceFirst(explanation) {
		out.Trace = append(out.Trace, &adminv1.PermissionTraceEntry{
			Level:    apiPermissionDecisionLevel(entry.Level),
			RoleName: entry.RoleName,
			Decision: apiPermissionExplanationDecision(entry.Decision),
			Applied:  i == 0 && traceEntryWins(explanation, entry),
		})
	}
	return out
}

func winningTraceFirst(explanation core.PermissionExplanation) []core.TraceEntry {
	trace := append([]core.TraceEntry(nil), explanation.Trace...)
	winningIndex := -1
	for i, entry := range trace {
		if traceEntryWins(explanation, entry) {
			winningIndex = i
			break
		}
	}
	if winningIndex <= 0 {
		return trace
	}
	winning := trace[winningIndex]
	copy(trace[1:winningIndex+1], trace[:winningIndex])
	trace[0] = winning
	return trace
}

func traceEntryWins(explanation core.PermissionExplanation, entry core.TraceEntry) bool {
	return entry.Level == explanation.DecidedAt &&
		entry.RoleName == explanation.DecidedByRole &&
		entry.Decision == explanation.State
}

func apiPermissionExplanationDecision(decision core.DecisionKind) adminv1.PermissionDecision {
	switch decision {
	case core.DecisionAllow:
		return adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW
	case core.DecisionDeny:
		return adminv1.PermissionDecision_PERMISSION_DECISION_DENY
	default:
		return adminv1.PermissionDecision_PERMISSION_DECISION_NONE
	}
}

func apiPermissionDecisionLevel(level core.PermissionLevel) adminv1.PermissionDecisionLevel {
	switch level {
	case core.LevelServer:
		return adminv1.PermissionDecisionLevel_PERMISSION_DECISION_LEVEL_SERVER
	case core.LevelGroup:
		return adminv1.PermissionDecisionLevel_PERMISSION_DECISION_LEVEL_GROUP
	case core.LevelRoom:
		return adminv1.PermissionDecisionLevel_PERMISSION_DECISION_LEVEL_ROOM
	default:
		return adminv1.PermissionDecisionLevel_PERMISSION_DECISION_LEVEL_UNSPECIFIED
	}
}

func apiTierRoles(matrix *core.TierRoles) *adminv1.TierRoles {
	if matrix == nil {
		return nil
	}
	out := &adminv1.TierRoles{
		ApplicablePermissions: append([]string(nil), matrix.ApplicablePermissions...),
		Roles:                 make([]*adminv1.TierRole, 0, len(matrix.Roles)),
	}
	for _, role := range matrix.Roles {
		out.Roles = append(out.Roles, &adminv1.TierRole{
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

func apiTierPermissions(perms core.TierPermissions) *adminv1.TierPermissions {
	return &adminv1.TierPermissions{
		Permissions:       append([]string(nil), perms.Permissions...),
		PermissionDenials: append([]string(nil), perms.PermissionDenials...),
	}
}

func apiRolePermissionMatrix(matrix *core.RolePermissionMatrix) *adminv1.RolePermissionMatrix {
	if matrix == nil {
		return nil
	}
	return &adminv1.RolePermissionMatrix{
		RoleName:              matrix.RoleName,
		ApplicablePermissions: append([]string(nil), matrix.ApplicablePermissions...),
		Scopes:                apiPermissionMatrixScopes(matrix.Scopes),
		Cells:                 apiPermissionMatrixCells(matrix.Cells),
	}
}

func apiUserPermissionMatrix(matrix *core.UserPermissionMatrix) *adminv1.UserPermissionMatrix {
	if matrix == nil {
		return nil
	}
	return &adminv1.UserPermissionMatrix{
		UserId:                matrix.UserID,
		ApplicablePermissions: append([]string(nil), matrix.ApplicablePermissions...),
		Scopes:                apiPermissionMatrixScopes(matrix.Scopes),
		Cells:                 apiPermissionMatrixCells(matrix.Cells),
	}
}

func apiPermissionDecisionEntries(scopes []core.PermissionMatrixScope, cells []core.PermissionMatrixCell) []*adminv1.ScopedPermissionDecision {
	scopesByID := make(map[string]core.PermissionMatrixScope, len(scopes))
	for _, scope := range scopes {
		scopesByID[scope.ID] = scope
	}
	out := make([]*adminv1.ScopedPermissionDecision, 0, len(cells))
	for _, cell := range cells {
		scope, ok := scopesByID[cell.ScopeID]
		if !ok {
			continue
		}
		out = append(out, &adminv1.ScopedPermissionDecision{
			Permission: cell.Permission,
			Scope:      apiPermissionEntryScope(scope),
			Override:   apiPermissionDecision(cell.Override),
			Effective:  apiPermissionDecision(cell.Effective),
		})
	}
	return out
}

func apiPermissionEntryScope(scope core.PermissionMatrixScope) *adminv1.PermissionScope {
	switch scope.Kind {
	case core.MatrixScopeGroup:
		return &adminv1.PermissionScope{
			Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_GROUP,
			Id:   strings.TrimPrefix(scope.ID, "group:"),
		}
	case core.MatrixScopeRoom:
		return &adminv1.PermissionScope{
			Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM,
			Id:   strings.TrimPrefix(scope.ID, "room:"),
		}
	default:
		return &adminv1.PermissionScope{
			Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER,
		}
	}
}

func apiPermissionMatrixScopes(scopes []core.PermissionMatrixScope) []*adminv1.PermissionMatrixScope {
	out := make([]*adminv1.PermissionMatrixScope, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, &adminv1.PermissionMatrixScope{
			Id:            scope.ID,
			Label:         scope.Label,
			Kind:          apiPermissionScopeKind(scope.Kind),
			ParentGroupId: scope.ParentGroupID,
		})
	}
	return out
}

func apiPermissionMatrixCells(cells []core.PermissionMatrixCell) []*adminv1.PermissionMatrixCell {
	out := make([]*adminv1.PermissionMatrixCell, 0, len(cells))
	for _, cell := range cells {
		out = append(out, &adminv1.PermissionMatrixCell{
			Permission: cell.Permission,
			ScopeId:    cell.ScopeID,
			Override:   apiPermissionDecision(cell.Override),
			Effective:  apiPermissionDecision(cell.Effective),
		})
	}
	return out
}

func apiPermissionScopeKind(kind core.MatrixScopeKind) adminv1.PermissionScopeKind {
	switch kind {
	case core.MatrixScopeGroup:
		return adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_GROUP
	case core.MatrixScopeRoom:
		return adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM
	default:
		return adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER
	}
}

func apiPermissionDecision(decision core.MatrixDecision) adminv1.PermissionDecision {
	switch decision {
	case core.MatrixDecisionAllow:
		return adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW
	case core.MatrixDecisionDeny:
		return adminv1.PermissionDecision_PERMISSION_DECISION_DENY
	default:
		return adminv1.PermissionDecision_PERMISSION_DECISION_NONE
	}
}

func corePermissionState(decision adminv1.PermissionDecision) (core.PermissionState, error) {
	switch decision {
	case adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW:
		return core.PermissionStateAllow, nil
	case adminv1.PermissionDecision_PERMISSION_DECISION_DENY:
		return core.PermissionStateDeny, nil
	case adminv1.PermissionDecision_PERMISSION_DECISION_NONE:
		return core.PermissionStateNone, nil
	default:
		return "", invalidArgument("decision is required")
	}
}

func permissionScopeIDs(scope *adminv1.PermissionScope) (roomID string, groupID string, err error) {
	if scope == nil {
		return "", "", nil
	}
	switch scope.GetKind() {
	case adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_UNSPECIFIED:
		if scope.GetId() != "" {
			return "", "", invalidArgument("server scope id must be empty")
		}
		return "", "", nil
	case adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER:
		if scope.GetId() != "" {
			return "", "", invalidArgument("server scope id must be empty")
		}
		return "", "", nil
	case adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_GROUP:
		if scope.GetId() == "" {
			return "", "", invalidArgument("group scope id is required")
		}
		return "", scope.GetId(), nil
	case adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM:
		if scope.GetId() == "" {
			return "", "", invalidArgument("room scope id is required")
		}
		return scope.GetId(), "", nil
	default:
		return "", "", invalidArgument("unsupported permission scope kind")
	}
}

func corePermissionTargetScope(scope *adminv1.PermissionScope) (core.PermissionTargetScope, error) {
	roomID, groupID, err := permissionScopeIDs(scope)
	if err != nil {
		return core.PermissionTargetScope{}, err
	}
	switch {
	case roomID != "":
		return core.PermissionTargetScope{Kind: core.MatrixScopeRoom, ID: roomID}, nil
	case groupID != "":
		return core.PermissionTargetScope{Kind: core.MatrixScopeGroup, ID: groupID}, nil
	default:
		return core.PermissionTargetScope{}, nil
	}
}
