package graph

import (
	"context"

	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// spaceRoleToGraphQL converts a corev1.Role to core.RoleWithPermissions for GraphQL.
func (r *Resolver) spaceRoleToGraphQL(ctx context.Context, spaceID string, role *corev1.Role) *core.RoleWithPermissions {
	if role == nil {
		return nil
	}

	// Fetch permissions and denials (log errors but don't fail - role metadata is still useful)
	perms, err := r.core.GetRolePermissions(ctx, spaceID, role.Name)
	if err != nil {
		r.logger.Warn("Failed to fetch role permissions", "space_id", spaceID, "role", role.Name, "error", err)
	}
	denials, err := r.core.GetRolePermissionDenials(ctx, spaceID, role.Name)
	if err != nil {
		r.logger.Warn("Failed to fetch role permission denials", "space_id", spaceID, "role", role.Name, "error", err)
	}

	return &core.RoleWithPermissions{
		Name:              role.Name,
		DisplayName:       role.DisplayName,
		Description:       role.Description,
		Permissions:       perms,
		PermissionDenials: denials,
		IsSystem:          core.IsSystemRole(role.Name),
		Position:          role.Position,
	}
}

// spaceRolesToGraphQL converts a slice of corev1.Role to []*core.RoleWithPermissions for GraphQL.
func (r *Resolver) spaceRolesToGraphQL(ctx context.Context, spaceID string, roles []*corev1.Role) []*core.RoleWithPermissions {
	result := make([]*core.RoleWithPermissions, len(roles))
	for i, role := range roles {
		result[i] = r.spaceRoleToGraphQL(ctx, spaceID, role)
	}
	return result
}
