// Package graph contains the GraphQL resolver implementations.
//
// Authorization helpers (authz.go) enforce access control rules for GraphQL operations.
// Authentication is handled by the auth package (middleware extracts user from JWT/session
// and injects into context). These helpers verify the authenticated user has permission
// to perform the requested operation.
//
// Helpers return the authenticated user on success, allowing callers to use it for
// subsequent operations. On failure, they return sentinel errors for consistent
// error handling and testability.
//
// For queries/fields that should return null instead of error when unauthorized
// (e.g., Me, ViewerIsMember), use auth.ForContext directly.
package graph

import (
	"context"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/auth"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Authorization errors for consistent error handling across resolvers.
// ErrNotSpaceMember and ErrNotRoomMember are aliases for core errors to ensure
// consistency between GraphQL resolvers and NATS services.
var (
	ErrNotAuthenticated = errors.New("authentication required")
	ErrNotSpaceMember   = core.ErrNotSpaceMember
	ErrNotRoomMember    = core.ErrNotRoomMember
	ErrNotSelf          = errors.New("access denied: cannot access other users' data")
	ErrNotInstanceAdmin = errors.New("access denied: instance admin required")
)

// requireAuth extracts the authenticated user from context.
// Returns ErrNotAuthenticated if no user is authenticated.
func requireAuth(ctx context.Context) (*corev1.User, error) {
	user := auth.ForContext(ctx)
	if user == nil {
		return nil, ErrNotAuthenticated
	}
	return user, nil
}

// requireSelf verifies that the authenticated user is accessing their own data.
// Returns ErrNotSelf if the caller is not the target user.
func requireSelf(ctx context.Context, targetUserID string) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if user.Id != targetUserID {
		return nil, ErrNotSelf
	}
	return user, nil
}

// requireSpaceMember verifies that the authenticated user is a member of the space.
// Returns ErrNotSpaceMember if the user is not a member.
//
// For the DM system space, users have room memberships but not space memberships.
// In this case, we verify the user has at least one room membership in the DM space.
func requireSpaceMember(ctx context.Context, c *core.ChattoCore, spaceID string) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Special handling for DM space: check dm.view permission instead of membership
	if core.IsDMSpace(spaceID) {
		can, err := c.HasInstancePermission(ctx, user.Id, core.PermDMView)
		if err != nil {
			return nil, fmt.Errorf("failed to check DM permission: %w", err)
		}
		if !can {
			return nil, core.ErrPermissionDenied
		}
		return user, nil
	}

	isMember, err := c.SpaceMembershipExists(ctx, user.Id, spaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify space membership: %w", err)
	}
	if !isMember {
		return nil, ErrNotSpaceMember
	}

	return user, nil
}

// requireRoomMember verifies that the authenticated user is a member of the room.
// Returns ErrNotRoomMember if the user is not a member.
func requireRoomMember(ctx context.Context, c *core.ChattoCore, spaceID, roomID string) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	isMember, err := c.RoomMembershipExists(ctx, spaceID, user.Id, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify room membership: %w", err)
	}
	if !isMember {
		return nil, ErrNotRoomMember
	}

	return user, nil
}

// requireInstanceAdmin verifies that the authenticated user has owner or admin
// role in the unified server RBAC. Owner-by-config (owners.emails) is
// materialised as a real role assignment by `chatto reset rbac` and by
// `addVerifiedEmail` at email-verification time, so the dual-path check the
// pre-Phase-5 helper used to do collapses to a single role lookup.
func requireInstanceAdmin(ctx context.Context, c *core.ChattoCore) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	isOwner, err := c.IsInstanceOwner(ctx, user.Id)
	if err != nil {
		return nil, fmt.Errorf("check owner status: %w", err)
	}
	if isOwner {
		return user, nil
	}

	isAdmin, err := c.IsInstanceAdmin(ctx, user.Id)
	if err != nil {
		return nil, fmt.Errorf("check admin status: %w", err)
	}
	if isAdmin {
		return user, nil
	}

	return nil, ErrNotInstanceAdmin
}

// canManageInstanceRoles checks the admin.manage-roles permission.
func (r *Resolver) canManageInstanceRoles(ctx context.Context, userID string) (bool, error) {
	return r.core.HasInstancePermission(ctx, userID, core.PermAdminRolesManage)
}

// canManageInstanceUsers checks the admin.manage-users permission.
func (r *Resolver) canManageInstanceUsers(ctx context.Context, userID string) (bool, error) {
	return r.core.HasInstancePermission(ctx, userID, core.PermAdminUsersManage)
}

// requireRoomManageAuth gates room-level permission mutations on PermRoleManage
// in the relevant space (formerly enforced inside the core wrappers).
func (r *Resolver) requireRoomManageAuth(ctx context.Context, userID, spaceID string) error {
	can, err := r.core.CanSpaceRolesManage(ctx, userID, spaceID)
	if err != nil {
		return err
	}
	if !can {
		return core.ErrPermissionDenied
	}
	return nil
}

// isInstanceAdmin returns true when the user has the owner or admin role.
func (r *Resolver) isInstanceAdmin(ctx context.Context, userID string) (bool, error) {
	isOwner, err := r.core.IsInstanceOwner(ctx, userID)
	if err != nil {
		return false, err
	}
	if isOwner {
		return true, nil
	}
	return r.core.IsInstanceAdmin(ctx, userID)
}

// isInstanceAdmin0 returns true if the user has the owner OR admin role.
// Boolean-only flavour of isInstanceAdmin — convenience for callers that
// previously branched on `isConfigOwner` and now check role membership.
// Errors are swallowed (treated as "not admin") so call sites stay terse.
func (r *Resolver) isInstanceAdmin0(ctx context.Context, userID string) bool {
	ok, err := r.isInstanceAdmin(ctx, userID)
	if err != nil {
		r.logger.Warn("isInstanceAdmin0: role lookup failed; treating as non-admin",
			"user_id", userID, "error", err)
		return false
	}
	return ok
}

// isInstanceOwner0 returns true if the user has the owner role.
// Boolean-only flavour for hierarchy-check call sites that previously
// short-circuited on `isConfigOwner` (the "config-designated top of
// hierarchy" check). Post-Phase-5 a config owner is just an owner-role
// holder, so this stands in for that bypass without re-introducing the
// dual-path lookup.
func (r *Resolver) isInstanceOwner0(ctx context.Context, userID string) bool {
	ok, err := r.core.IsInstanceOwner(ctx, userID)
	if err != nil {
		r.logger.Warn("isInstanceOwner0: role lookup failed; treating as non-owner",
			"user_id", userID, "error", err)
		return false
	}
	return ok
}

// requireInstancePermission verifies the user has a specific server permission.
func requireInstancePermission(ctx context.Context, c *core.ChattoCore, perm core.Permission) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	hasPerm, err := c.HasInstancePermission(ctx, user.Id, perm)
	if err != nil {
		return nil, fmt.Errorf("check permission: %w", err)
	}
	if !hasPerm {
		return nil, core.ErrPermissionDenied
	}

	return user, nil
}
