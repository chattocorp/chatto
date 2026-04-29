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
// tierRoles queries: instance scope requires instance admin; space and room
// scopes require role.manage in spaceID or instance admin. At room scope,
// roomID must belong to spaceID.
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
// Instance tier is included for instance roles only. Space and room tiers are
// included when their scope IDs are non-empty.
func (r *Resolver) buildRoleAcrossTiers(
	ctx context.Context,
	roleName string,
	isInstanceRole bool,
	spaceID, roomID string,
) (*model.RoleAcrossTiers, error) {
	out := &model.RoleAcrossTiers{
		RoleName:       roleName,
		IsInstanceRole: isInstanceRole,
	}

	if isInstanceRole {
		role, err := r.core.GetInstanceRole(ctx, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load instance role: %w", err)
		}
		if role == nil {
			return nil, nil
		}
		out.DisplayName = role.DisplayName
		out.Description = role.Description
		out.IsSystem = role.IsSystem
		out.Position = role.Position
	} else {
		if spaceID == "" {
			return nil, fmt.Errorf("spaceId required for space role lookup")
		}
		role, err := r.core.GetRole(ctx, spaceID, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load space role: %w", err)
		}
		if role == nil {
			return nil, nil
		}
		out.DisplayName = role.DisplayName
		out.Description = role.Description
		out.IsSystem = core.IsSpaceSystemRole(role.Name)
		out.Position = role.Position
	}

	for _, meta := range core.PermissionsForScope(tierScope(spaceID, roomID)) {
		out.ApplicablePermissions = append(out.ApplicablePermissions, string(meta.Permission))
	}

	if isInstanceRole {
		grants, err := r.core.GetInstanceRolePermissions(ctx, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load instance grants: %w", err)
		}
		denials, err := r.core.GetInstanceRolePermissionDenials(ctx, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load instance denials: %w", err)
		}
		out.Instance = newTierPermissions(grants, denials)
	}

	if spaceID != "" {
		var (
			grants  []core.Permission
			denials []core.Permission
			err     error
		)
		if isInstanceRole {
			grants, denials, err = r.core.GetInstanceRoleSpacePermissions(ctx, spaceID, roleName)
			if err != nil {
				return nil, fmt.Errorf("failed to load instance role space permissions: %w", err)
			}
		} else {
			grants, err = r.core.GetRolePermissions(ctx, spaceID, roleName)
			if err != nil {
				return nil, fmt.Errorf("failed to load space role grants: %w", err)
			}
			denials, err = r.core.GetRolePermissionDenials(ctx, spaceID, roleName)
			if err != nil {
				return nil, fmt.Errorf("failed to load space role denials: %w", err)
			}
		}
		out.Space = newTierPermissions(grants, denials)
	}

	if roomID != "" {
		grants, denials, err := r.core.GetRoleRoomPermissions(ctx, spaceID, roomID, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load room overrides: %w", err)
		}
		out.Room = newTierPermissions(grants, denials)
	}

	return out, nil
}

// buildTierRoles assembles the per-tier permission matrix: every applicable
// role at the requested scope, with override + inherited baseline, plus the
// list of permissions configurable at this scope.
func (r *Resolver) buildTierRoles(ctx context.Context, spaceID, roomID string) (*model.TierRoles, error) {
	scope := tierScope(spaceID, roomID)

	out := &model.TierRoles{}
	for _, meta := range core.PermissionsForScope(scope) {
		out.ApplicablePermissions = append(out.ApplicablePermissions, string(meta.Permission))
	}

	if scope == core.ScopeInstance {
		instanceRoles, err := r.core.ListInstanceRoles(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list instance roles: %w", err)
		}
		sort.SliceStable(instanceRoles, func(i, j int) bool {
			return instanceRoles[i].Position < instanceRoles[j].Position
		})
		for _, role := range instanceRoles {
			tr, err := r.buildTierRoleForInstanceRole(ctx, role, scope, spaceID, roomID)
			if err != nil {
				return nil, err
			}
			out.Roles = append(out.Roles, tr)
		}
		return out, nil
	}

	// Space and room scope: space roles first by position, then instance roles
	// (excluding universal-at-space) by position.
	spaceRoles, err := r.core.ListRoles(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list space roles: %w", err)
	}
	sort.SliceStable(spaceRoles, func(i, j int) bool {
		return spaceRoles[i].Position < spaceRoles[j].Position
	})
	for _, role := range spaceRoles {
		tr, err := r.buildTierRoleForSpaceRole(ctx, role.Name, role.DisplayName, role.Description, role.Position, scope, spaceID, roomID)
		if err != nil {
			return nil, err
		}
		out.Roles = append(out.Roles, tr)
	}

	instanceRoles, err := r.core.ListInstanceRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list instance roles: %w", err)
	}
	sort.SliceStable(instanceRoles, func(i, j int) bool {
		return instanceRoles[i].Position < instanceRoles[j].Position
	})
	for _, role := range instanceRoles {
		if core.IsSpaceUniversalRole(role.Name) {
			continue
		}
		tr, err := r.buildTierRoleForInstanceRole(ctx, role, scope, spaceID, roomID)
		if err != nil {
			return nil, err
		}
		out.Roles = append(out.Roles, tr)
	}
	return out, nil
}

// buildTierRoleForSpaceRole computes the override + inherited baseline for a
// space role at the requested scope. Space roles never have an instance tier.
func (r *Resolver) buildTierRoleForSpaceRole(
	ctx context.Context,
	roleName, displayName, description string,
	position int32,
	scope core.PermissionScope,
	spaceID, roomID string,
) (*model.TierRole, error) {
	out := &model.TierRole{
		RoleName:       roleName,
		DisplayName:    displayName,
		Description:    description,
		IsInstanceRole: false,
		IsSystem:       core.IsSpaceSystemRole(roleName),
		Position:       position,
	}

	switch scope {
	case core.ScopeSpace:
		grants, err := r.core.GetRolePermissions(ctx, spaceID, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load space role grants: %w", err)
		}
		denials, err := r.core.GetRolePermissionDenials(ctx, spaceID, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load space role denials: %w", err)
		}
		out.Override = newTierPermissions(grants, denials)
	case core.ScopeRoom:
		grants, denials, err := r.core.GetRoleRoomPermissions(ctx, spaceID, roomID, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load room overrides: %w", err)
		}
		out.Override = newTierPermissions(grants, denials)
		spaceGrants, err := r.core.GetRolePermissions(ctx, spaceID, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load space role grants: %w", err)
		}
		spaceDenials, err := r.core.GetRolePermissionDenials(ctx, spaceID, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load space role denials: %w", err)
		}
		out.InheritedAllows = permsToStrings(spaceGrants)
		out.InheritedDenials = permsToStrings(spaceDenials)
	default:
		return nil, fmt.Errorf("space role at instance scope is not meaningful")
	}

	if out.Override == nil {
		out.Override = &model.TierPermissions{}
	}
	return out, nil
}

// buildTierRoleForInstanceRole computes the override + inherited baseline
// for an instance role at the requested scope.
func (r *Resolver) buildTierRoleForInstanceRole(
	ctx context.Context,
	role core.RoleWithPermissions,
	scope core.PermissionScope,
	spaceID, roomID string,
) (*model.TierRole, error) {
	out := &model.TierRole{
		RoleName:       role.Name,
		DisplayName:    role.DisplayName,
		Description:    role.Description,
		IsInstanceRole: true,
		IsSystem:       role.IsSystem,
		Position:       role.Position,
	}

	instGrants, err := r.core.GetInstanceRolePermissions(ctx, role.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to load instance grants: %w", err)
	}
	instDenials, err := r.core.GetInstanceRolePermissionDenials(ctx, role.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to load instance denials: %w", err)
	}

	switch scope {
	case core.ScopeInstance:
		out.Override = newTierPermissions(instGrants, instDenials)
	case core.ScopeSpace:
		grants, denials, err := r.core.GetInstanceRoleSpacePermissions(ctx, spaceID, role.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load instance role space permissions: %w", err)
		}
		out.Override = newTierPermissions(grants, denials)
		out.InheritedAllows = permsToStrings(instGrants)
		out.InheritedDenials = permsToStrings(instDenials)
	case core.ScopeRoom:
		grants, denials, err := r.core.GetRoleRoomPermissions(ctx, spaceID, roomID, role.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load room overrides: %w", err)
		}
		out.Override = newTierPermissions(grants, denials)
		spaceGrants, spaceDenials, err := r.core.GetInstanceRoleSpacePermissions(ctx, spaceID, role.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load instance role space permissions: %w", err)
		}
		allows, denyList := mergeInheritedDecisions(spaceGrants, spaceDenials, instGrants, instDenials)
		out.InheritedAllows = allows
		out.InheritedDenials = denyList
	}

	if out.Override == nil {
		out.Override = &model.TierPermissions{}
	}
	return out, nil
}

// mergeInheritedDecisions resolves the effective allow/deny baseline for a
// single role across two tiers (override tier + parent tier). Per permission
// the override tier wins: an entry on the override tier's allow or deny list
// suppresses the parent tier's entries. Used to compute the inherited
// baseline shown faded behind a room-scope override for an instance role,
// where the room cell's "what would happen without me" is the resolved
// space+instance state.
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
		return core.ScopeInstance
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
