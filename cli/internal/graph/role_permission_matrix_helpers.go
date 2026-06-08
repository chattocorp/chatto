package graph

import (
	"context"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// buildRolePermissionMatrix assembles the per-role permission matrix —
// the same shape as the per-user matrix but with role-only resolution.
// `effective` here walks room → group → server for THIS role's own
// grants only (no rank, no user overrides), reflecting what the role
// contributes to the resolver.
func (r *Resolver) buildRolePermissionMatrix(ctx context.Context, roleName string) (*model.RolePermissionMatrix, error) {
	role, err := r.core.GetServerRole(ctx, roleName)
	if err != nil {
		if errors.Is(err, core.ErrRoleNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load role: %w", err)
	}
	if role == nil {
		return nil, nil
	}

	allPerms := core.AllPermissions()
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

	// Load the role's grants/denials at every relevant scope up front so
	// the inner loop doesn't refetch per cell.
	serverGrants, err := r.core.GetServerRolePermissions(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("load server grants: %w", err)
	}
	serverDenials, err := r.core.GetServerRolePermissionDenials(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("load server denials: %w", err)
	}

	groupGrants := make(map[string][]core.Permission)
	groupDenials := make(map[string][]core.Permission)
	roomGrants := make(map[string][]core.Permission)
	roomDenials := make(map[string][]core.Permission)
	roomToGroup := make(map[string]string)

	for _, scope := range scopes {
		switch scope.Kind {
		case model.PermissionMatrixScopeKindGroup:
			gid := scopeRefID(scope.ID, "group:")
			g, d, err := r.core.GetGroupRolePermissions(ctx, gid, roleName)
			if err != nil {
				return nil, fmt.Errorf("load group %s perms: %w", gid, err)
			}
			groupGrants[gid] = g
			groupDenials[gid] = d
		case model.PermissionMatrixScopeKindRoom:
			rid := scopeRefID(scope.ID, "room:")
			g, d, err := r.core.GetRoomRolePermissions(ctx, rid, roleName)
			if err != nil {
				return nil, fmt.Errorf("load room %s perms: %w", rid, err)
			}
			roomGrants[rid] = g
			roomDenials[rid] = d
			roomToGroup[rid] = scope.ParentGroupID
		}
	}

	cells := make([]*model.PermissionMatrixCell, 0, len(applicable)*len(scopes))
	for _, permStr := range applicable {
		perm := core.Permission(permStr)
		for _, scope := range scopes {
			cell, ok := buildRolePermissionCell(
				perm, scope,
				serverGrants, serverDenials,
				groupGrants, groupDenials,
				roomGrants, roomDenials,
				roomToGroup,
			)
			if !ok {
				continue
			}
			cells = append(cells, cell)
		}
	}

	return &model.RolePermissionMatrix{
		RoleName:              roleName,
		ApplicablePermissions: applicable,
		Scopes:                scopes,
		Cells:                 cells,
	}, nil
}

// buildRolePermissionCell computes the override + effective decision for
// one (permission, scope) pair using only this role's own grants. Returns
// false when the permission doesn't apply at the scope's tier — the
// caller drops the cell from the sparse list.
func buildRolePermissionCell(
	perm core.Permission,
	scope *model.PermissionMatrixScope,
	serverGrants, serverDenials []core.Permission,
	groupGrants, groupDenials map[string][]core.Permission,
	roomGrants, roomDenials map[string][]core.Permission,
	roomToGroup map[string]string,
) (*model.PermissionMatrixCell, bool) {
	switch scope.Kind {
	case model.PermissionMatrixScopeKindServer:
		if !core.PermissionAppliesAtScope(perm, core.ScopeServer) {
			return nil, false
		}
		override := decisionFromLists(perm, serverGrants, serverDenials)
		return &model.PermissionMatrixCell{
			Permission: string(perm),
			ScopeID:    scope.ID,
			Override:   override,
			Effective:  override, // no parent scope to inherit from
		}, true

	case model.PermissionMatrixScopeKindGroup:
		if !core.PermissionAppliesAtScope(perm, core.ScopeGroup) {
			return nil, false
		}
		gid := scopeRefID(scope.ID, "group:")
		override := decisionFromLists(perm, groupGrants[gid], groupDenials[gid])
		effective := override
		if effective == model.PermissionMatrixDecisionNone &&
			core.PermissionAppliesAtScope(perm, core.ScopeServer) {
			effective = decisionFromLists(perm, serverGrants, serverDenials)
		}
		return &model.PermissionMatrixCell{
			Permission: string(perm),
			ScopeID:    scope.ID,
			Override:   override,
			Effective:  effective,
		}, true

	case model.PermissionMatrixScopeKindRoom:
		if !core.PermissionAppliesAtScope(perm, core.ScopeRoom) {
			return nil, false
		}
		rid := scopeRefID(scope.ID, "room:")
		override := decisionFromLists(perm, roomGrants[rid], roomDenials[rid])
		effective := override
		if effective == model.PermissionMatrixDecisionNone {
			// Walk group → server, mirroring the user resolver.
			if gid := roomToGroup[rid]; gid != "" && core.PermissionAppliesAtScope(perm, core.ScopeGroup) {
				effective = decisionFromLists(perm, groupGrants[gid], groupDenials[gid])
			}
			if effective == model.PermissionMatrixDecisionNone &&
				core.PermissionAppliesAtScope(perm, core.ScopeServer) {
				effective = decisionFromLists(perm, serverGrants, serverDenials)
			}
		}
		return &model.PermissionMatrixCell{
			Permission: string(perm),
			ScopeID:    scope.ID,
			Override:   override,
			Effective:  effective,
		}, true
	}
	return nil, false
}

func decisionFromLists(perm core.Permission, grants, denials []core.Permission) model.PermissionMatrixDecision {
	for _, p := range grants {
		if p == perm {
			return model.PermissionMatrixDecisionAllow
		}
	}
	for _, p := range denials {
		if p == perm {
			return model.PermissionMatrixDecisionDeny
		}
	}
	return model.PermissionMatrixDecisionNone
}

// buildMatrixScopes assembles the scope columns shared by the user and
// role matrices: server first, every room group next, then every channel
// room with its parent group set so the UI can nest visually.
//
// Extracted from `buildUserPermissionMatrix` so the role matrix doesn't
// have to repeat the layout walk.
func (r *Resolver) buildMatrixScopes(ctx context.Context) ([]*model.PermissionMatrixScope, error) {
	scopes := []*model.PermissionMatrixScope{
		{
			ID:            "server",
			Label:         "Server",
			Kind:          model.PermissionMatrixScopeKindServer,
			ParentGroupID: "",
		},
	}
	groups, err := r.core.ListRoomGroupsOrdered(ctx, core.KindChannel)
	if err != nil {
		return nil, fmt.Errorf("load room groups: %w", err)
	}

	roomsByGroup := make(map[string][]*corevRoomLite, len(groups))
	for _, group := range groups {
		scopes = append(scopes, &model.PermissionMatrixScope{
			ID:            "group:" + group.Id,
			Label:         group.Name,
			Kind:          model.PermissionMatrixScopeKindGroup,
			ParentGroupID: "",
		})
		for _, roomID := range group.RoomIds {
			room, err := r.core.GetRoom(ctx, core.KindChannel, roomID)
			if err != nil || room == nil {
				continue
			}
			roomsByGroup[group.Id] = append(roomsByGroup[group.Id], &corevRoomLite{
				ID:   room.Id,
				Name: room.Name,
			})
		}
	}
	for _, group := range groups {
		for _, room := range roomsByGroup[group.Id] {
			scopes = append(scopes, &model.PermissionMatrixScope{
				ID:            "room:" + room.ID,
				Label:         room.Name,
				Kind:          model.PermissionMatrixScopeKindRoom,
				ParentGroupID: group.Id,
			})
		}
	}
	return scopes, nil
}
