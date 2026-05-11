package graph

// Helper methods for the rolePermissions and tierRoles resolvers. Lives
// outside the resolvers file so gqlgen's regenerator doesn't move it into a
// "code that was going to be deleted" comment block.

import (
	"context"
	"fmt"
	"sort"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// authorizeRolePermissions enforces access for both the rolePermissions and
// tierRoles queries: server scope requires server admin; room scope requires
// role.manage on the server space or server admin.
func (r *Resolver) authorizeRolePermissions(ctx context.Context, viewerID, spaceID, roomID string) error {
	if spaceID == "" {
		return r.requireInstanceAdminOrErr(ctx, viewerID)
	}
	if err := r.requireInstanceAdminOrErr(ctx, viewerID); err != nil {
		hasRolesManage, hpErr := r.core.PermResolver().HasSpacePermission(ctx, viewerID, spaceID, core.PermRoleManage)
		if hpErr != nil {
			return fmt.Errorf("failed to check role.manage: %w", hpErr)
		}
		if !hasRolesManage {
			return core.ErrPermissionDenied
		}
	}
	return r.requireRoomBelongsToSpace(ctx, spaceID, roomID)
}

// buildRoleAcrossTiers gathers metadata + per-tier grants/denials for the role.
// Server tier is always populated; room tier is populated when roomID is
// non-empty.
func (r *Resolver) buildRoleAcrossTiers(
	ctx context.Context,
	roleName string,
	spaceID, roomID string,
) (*model.RoleAcrossTiers, error) {
	role, err := r.core.GetInstanceRole(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to load role: %w", err)
	}
	if role == nil {
		return nil, nil
	}

	out := &model.RoleAcrossTiers{
		RoleName:    roleName,
		DisplayName: role.DisplayName,
		Description: role.Description,
		IsSystem:    role.IsSystem,
		Position:    role.Position,
	}

	for _, meta := range core.PermissionsForScope(tierScope(spaceID, roomID)) {
		out.ApplicablePermissions = append(out.ApplicablePermissions, string(meta.Permission))
	}

	grants, err := r.core.GetInstanceRolePermissions(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to load grants: %w", err)
	}
	denials, err := r.core.GetInstanceRolePermissionDenials(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to load denials: %w", err)
	}
	out.Server = newTierPermissions(grants, denials)

	if roomID != "" {
		grants, denials, err := r.core.GetRoomRolePermissions(ctx, roomID, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load room overrides: %w", err)
		}
		out.Room = newTierPermissions(grants, denials)
	}

	return out, nil
}

// buildTierRoles assembles the per-tier permission matrix: every role at the
// requested scope, with override + inherited baseline, plus the list of
// permissions configurable at this scope.
func (r *Resolver) buildTierRoles(ctx context.Context, spaceID, roomID string) (*model.TierRoles, error) {
	scope := tierScope(spaceID, roomID)

	out := &model.TierRoles{}
	for _, meta := range core.PermissionsForScope(scope) {
		out.ApplicablePermissions = append(out.ApplicablePermissions, string(meta.Permission))
	}

	roles, err := r.core.ListInstanceRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	sort.SliceStable(roles, func(i, j int) bool {
		return roles[i].Position < roles[j].Position
	})
	for _, role := range roles {
		// At room scope the everyone role is hidden — its grants/denials are the
		// space-default baseline that other roles inherit, so showing it as a
		// peer column would just reprint the inheritance.
		if scope == core.ScopeRoom && role.Name == core.RoleEveryone {
			continue
		}
		tr, err := r.buildTierRole(ctx, role, scope, spaceID, roomID)
		if err != nil {
			return nil, err
		}
		out.Roles = append(out.Roles, tr)
	}
	return out, nil
}

// buildTierRole computes the override + inherited baseline for a role at the
// requested scope. Server scope has no inheritance; room scope inherits from
// the role's server-level state.
func (r *Resolver) buildTierRole(
	ctx context.Context,
	role core.RoleWithPermissions,
	scope core.PermissionScope,
	spaceID, roomID string,
) (*model.TierRole, error) {
	out := &model.TierRole{
		RoleName:    role.Name,
		DisplayName: role.DisplayName,
		Description: role.Description,
		IsSystem:    role.IsSystem,
		Position:    role.Position,
	}

	serverGrants, err := r.core.GetInstanceRolePermissions(ctx, role.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to load server grants: %w", err)
	}
	serverDenials, err := r.core.GetInstanceRolePermissionDenials(ctx, role.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to load server denials: %w", err)
	}

	switch scope {
	case core.ScopeServer, core.ScopeSpace:
		// Server scope (or the legacy "space" alias). The role's server-level
		// state is the override; nothing is inherited.
		out.Override = newTierPermissions(serverGrants, serverDenials)
	case core.ScopeRoom:
		grants, denials, err := r.core.GetRoomRolePermissions(ctx, roomID, role.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load room overrides: %w", err)
		}
		out.Override = newTierPermissions(grants, denials)
		out.InheritedAllows = permsToStrings(serverGrants)
		out.InheritedDenials = permsToStrings(serverDenials)
	}

	if out.Override == nil {
		out.Override = &model.TierPermissions{}
	}
	return out, nil
}

// mergeInheritedDecisions resolves the effective allow/deny baseline for a
// single role across two tiers (override tier + parent tier). Per permission
// the override tier wins: an entry on the override tier's allow or deny list
// suppresses the parent tier's entries.
func mergeInheritedDecisions(overrideAllow, overrideDeny, parentAllow, parentDeny []core.Permission) ([]string, []string) {
	overridden := make(map[core.Permission]struct{}, len(overrideAllow)+len(overrideDeny))
	for _, p := range overrideAllow {
		overridden[p] = struct{}{}
	}
	for _, p := range overrideDeny {
		overridden[p] = struct{}{}
	}

	allow := make([]string, 0, len(overrideAllow)+len(parentAllow))
	for _, p := range overrideAllow {
		allow = append(allow, string(p))
	}
	for _, p := range parentAllow {
		if _, blocked := overridden[p]; blocked {
			continue
		}
		allow = append(allow, string(p))
	}

	deny := make([]string, 0, len(overrideDeny)+len(parentDeny))
	for _, p := range overrideDeny {
		deny = append(deny, string(p))
	}
	for _, p := range parentDeny {
		if _, blocked := overridden[p]; blocked {
			continue
		}
		deny = append(deny, string(p))
	}
	return allow, deny
}

func tierScope(spaceID, roomID string) core.PermissionScope {
	switch {
	case roomID != "":
		return core.ScopeRoom
	case spaceID != "":
		return core.ScopeSpace
	default:
		return core.ScopeServer
	}
}

func newTierPermissions(grants, denials []core.Permission) *model.TierPermissions {
	out := &model.TierPermissions{
		Permissions:       make([]string, len(grants)),
		PermissionDenials: make([]string, len(denials)),
	}
	for i, g := range grants {
		out.Permissions[i] = string(g)
	}
	for i, d := range denials {
		out.PermissionDenials[i] = string(d)
	}
	return out
}

func permsToStrings(perms []core.Permission) []string {
	out := make([]string, len(perms))
	for i, p := range perms {
		out[i] = string(p)
	}
	return out
}
