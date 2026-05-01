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

	"hmans.de/chatto/internal/config"
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

// isConfigOwner checks if a user is a config-designated instance owner by
// matching any of their verified emails against owners.emails. Only verified
// emails are considered. A match grants owner-level access (which short-
// circuits all instance-permission checks in requireInstancePermission).
func isConfigOwner(ctx context.Context, c *core.ChattoCore, ownersCfg config.OwnersConfig, userID string) bool {
	verifiedEmails, err := c.GetVerifiedEmails(ctx, userID)
	if err != nil {
		return false
	}
	for _, ve := range verifiedEmails {
		if ownersCfg.IsInstanceOwnerEmail(ve.Email) {
			return true
		}
	}
	return false
}

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

// requireInstanceAdmin verifies that the authenticated user is an instance admin (owner or admin role).
// Checks config-designated owners (owners.emails), owner role, and admin role.
// Returns ErrNotInstanceAdmin if the user is not an admin.
func requireInstanceAdmin(ctx context.Context, c *core.ChattoCore, ownersCfg config.OwnersConfig) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Check config-based admin (bootstrap/fallback) via verified emails
	if isConfigOwner(ctx, c, ownersCfg, user.Id) {
		return user, nil
	}

	// Check RBAC-based owner role
	isOwner, err := c.IsInstanceOwner(ctx, user.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to check owner status: %w", err)
	}
	if isOwner {
		return user, nil
	}

	// Check RBAC-based admin role
	isAdmin, err := c.IsInstanceAdmin(ctx, user.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to check admin status: %w", err)
	}
	if isAdmin {
		return user, nil
	}

	return nil, ErrNotInstanceAdmin
}

// canManageInstanceRoles checks if a user can manage instance roles.
// Requires config admin (via verified emails) or admin.manage-roles permission.
func (r *Resolver) canManageInstanceRoles(ctx context.Context, userID string) (bool, error) {
	if isConfigOwner(ctx, r.core, r.ownersConfig, userID) {
		return true, nil
	}
	return r.core.HasInstancePermission(ctx, userID, core.PermAdminRolesManage)
}

// canManageInstanceUsers checks if a user can manage instance users.
// Requires config admin (via verified emails) or admin.manage-users permission.
func (r *Resolver) canManageInstanceUsers(ctx context.Context, userID string) (bool, error) {
	if isConfigOwner(ctx, r.core, r.ownersConfig, userID) {
		return true, nil
	}
	return r.core.HasInstancePermission(ctx, userID, core.PermAdminUsersManage)
}

// isInstanceAdmin checks if a user is an instance admin (config-based, owner, or admin role).
// This is a helper method for resolvers that need to check admin status without requireInstanceAdmin's
// error handling.
func (r *Resolver) isInstanceAdmin(ctx context.Context, userID string) (bool, error) {
	// Check config-based admin (bootstrap/fallback) via verified emails
	if isConfigOwner(ctx, r.core, r.ownersConfig, userID) {
		return true, nil
	}

	// Check RBAC-based owner role
	isOwner, err := r.core.IsInstanceOwner(ctx, userID)
	if err != nil {
		return false, err
	}
	if isOwner {
		return true, nil
	}

	// Check RBAC-based admin role
	return r.core.IsInstanceAdmin(ctx, userID)
}

// requireInstancePermission verifies the user has a specific instance permission.
// Checks config admin (all permissions via verified emails), RBAC admin role, and member role.
func requireInstancePermission(ctx context.Context, c *core.ChattoCore, ownersCfg config.OwnersConfig, perm core.Permission) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Config-based admins have all permissions (checked via verified emails)
	if isConfigOwner(ctx, c, ownersCfg, user.Id) {
		return user, nil
	}

	// Check RBAC permissions
	hasPerm, err := c.HasInstancePermission(ctx, user.Id, perm)
	if err != nil {
		return nil, fmt.Errorf("failed to check permission: %w", err)
	}
	if !hasPerm {
		return nil, core.ErrPermissionDenied
	}

	return user, nil
}
