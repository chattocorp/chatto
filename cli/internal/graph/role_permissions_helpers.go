package graph

// Helpers used by RolePermissions resolver. Kept in a non-`.resolvers.go`
// file so gqlgen's codegen leaves them alone — gqlgen otherwise quarantines
// helper methods that don't correspond to a GraphQL field, breaking
// compilation.

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// authorizeRolePermissions enforces access for the unified role-permissions
// query: instance scope requires instance admin; space and room scopes
// require role.manage in spaceID or instance admin. At room scope, roomID
// must belong to spaceID.
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

	// Metadata.
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
			// Space role lookups need a space.
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

	// Applicable permissions are determined by the deepest requested scope.
	scope := core.ScopeInstance
	switch {
	case roomID != "":
		scope = core.ScopeRoom
	case spaceID != "":
		scope = core.ScopeSpace
	}
	for _, meta := range core.PermissionsForScope(scope) {
		out.ApplicablePermissions = append(out.ApplicablePermissions, string(meta.Permission))
	}

	// Instance tier — only for instance roles.
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

	// Space tier.
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

	// Room tier.
	if roomID != "" {
		grants, denials, err := r.core.GetRoleRoomPermissions(ctx, spaceID, roomID, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load room overrides: %w", err)
		}
		out.Room = newTierPermissions(grants, denials)
	}

	return out, nil
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
