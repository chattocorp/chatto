package graph

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// buildUserPermissionMatrix assembles the full per-user permission matrix:
// the rows (applicable permissions), columns (server + every group + every
// room), and the cell at each intersection (user-level override + effective
// resolver decision). One round-trip's worth of data for the User
// Permissions page.
//
// The cell list is sparse — a (permission, scope) pair is only included
// when the permission applies at that scope's tier. Pure server-scope
// permissions appear in the server column only; channel-room permissions
// appear in the server column AND every group/room column.
func (r *Resolver) buildUserPermissionMatrix(ctx context.Context, userID string) (*model.UserPermissionMatrix, error) {
	allPerms := core.AllPermissions()

	// Permissions that can show up *anywhere* in this matrix — i.e. that
	// are configurable at server, group, or room scope. Excludes nothing
	// in practice, but keeps the contract honest: a permission that's
	// not configurable at any tier doesn't belong here.
	applicable := make([]string, 0, len(allPerms))
	for _, meta := range allPerms {
		if core.PermissionAppliesAtScope(meta.Permission, core.ScopeServer) ||
			core.PermissionAppliesAtScope(meta.Permission, core.ScopeGroup) ||
			core.PermissionAppliesAtScope(meta.Permission, core.ScopeRoom) {
			applicable = append(applicable, string(meta.Permission))
		}
	}

	scopes, err := r.buildMatrixScopes(ctx)
	if err != nil {
		return nil, err
	}

	// Build cells: per (permission, scope) intersection that's applicable
	// at the scope's tier, fetch the explicit user-level override and the
	// effective resolver decision.
	cells := make([]*model.PermissionMatrixCell, 0, len(applicable)*len(scopes))
	for _, permStr := range applicable {
		perm := core.Permission(permStr)
		for _, scope := range scopes {
			cell, ok, err := r.buildPermissionMatrixCell(ctx, userID, perm, scope)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			cells = append(cells, cell)
		}
	}

	return &model.UserPermissionMatrix{
		UserID:                userID,
		ApplicablePermissions: applicable,
		Scopes:                scopes,
		Cells:                 cells,
	}, nil
}

// corevRoomLite is a minimal room snapshot used while assembling the
// matrix's room columns — avoids holding full proto refs in a map.
type corevRoomLite struct {
	ID   string
	Name string
}

// buildPermissionMatrixCell returns one (permission, scope) cell. The
// second return is false when the permission doesn't apply at the
// scope's tier — the caller drops the cell from the sparse list.
func (r *Resolver) buildPermissionMatrixCell(
	ctx context.Context,
	userID string,
	perm core.Permission,
	scope *model.PermissionMatrixScope,
) (*model.PermissionMatrixCell, bool, error) {
	var (
		override  core.DecisionKind
		effective core.DecisionKind
		err       error
	)

	switch scope.Kind {
	case model.PermissionMatrixScopeKindServer:
		if !core.PermissionAppliesAtScope(perm, core.ScopeServer) {
			return nil, false, nil
		}
		override, err = r.core.GetUserExplicitServerOverride(ctx, userID, perm)
		if err != nil {
			return nil, false, err
		}
		effective, err = r.core.PermResolver().Resolve(ctx, userID, core.KindChannel, "", perm)
		if err != nil {
			return nil, false, err
		}

	case model.PermissionMatrixScopeKindGroup:
		if !core.PermissionAppliesAtScope(perm, core.ScopeGroup) {
			return nil, false, nil
		}
		groupID := scopeRefID(scope.ID, "group:")
		override, err = r.core.GetUserExplicitGroupOverride(ctx, groupID, userID, perm)
		if err != nil {
			return nil, false, err
		}
		effective, err = r.core.PermResolver().ResolveGroup(ctx, userID, core.KindChannel, groupID, perm)
		if err != nil {
			return nil, false, err
		}

	case model.PermissionMatrixScopeKindRoom:
		if !core.PermissionAppliesAtScope(perm, core.ScopeRoom) {
			return nil, false, nil
		}
		roomID := scopeRefID(scope.ID, "room:")
		override, err = r.core.GetUserExplicitRoomOverride(ctx, roomID, userID, perm)
		if err != nil {
			return nil, false, err
		}
		effective, err = r.core.PermResolver().Resolve(ctx, userID, core.KindChannel, roomID, perm)
		if err != nil {
			return nil, false, err
		}

	default:
		return nil, false, fmt.Errorf("unknown scope kind: %v", scope.Kind)
	}

	return &model.PermissionMatrixCell{
		Permission: string(perm),
		ScopeID:    scope.ID,
		Override:   decisionToModel(override),
		Effective:  decisionToModel(effective),
	}, true, nil
}

func scopeRefID(scopeID, prefix string) string {
	if len(scopeID) <= len(prefix) {
		return ""
	}
	return scopeID[len(prefix):]
}

func decisionToModel(d core.DecisionKind) model.PermissionMatrixDecision {
	switch d {
	case core.DecisionAllow:
		return model.PermissionMatrixDecisionAllow
	case core.DecisionDeny:
		return model.PermissionMatrixDecisionDeny
	default:
		return model.PermissionMatrixDecisionNone
	}
}
