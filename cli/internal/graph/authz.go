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
	ErrNotServerAdmin   = errors.New("access denied: server admin required")
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

// requireSpaceMember verifies the caller can access the given room kind.
//
// Post-consolidation every authenticated user is implicitly a server member,
// so this check collapses to `requireAuth`. DM read access is room membership,
// not a server-scope read permission.
func requireSpaceMember(ctx context.Context, c *core.ChattoCore, kind core.RoomKind) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// requireRoomMember verifies that the authenticated user is a member of the room.
// Returns ErrNotRoomMember if the user is not a member.
func requireRoomMember(ctx context.Context, c *core.ChattoCore, kind core.RoomKind, roomID string) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	isMember, err := c.RoomMembershipExists(ctx, kind, user.Id, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify room membership: %w", err)
	}
	if !isMember {
		return nil, ErrNotRoomMember
	}

	return user, nil
}

// requireServerAdmin verifies that the authenticated user has owner or admin
// role in the unified server RBAC. Owner-by-config (owners.emails) is
// materialised as a real role assignment by boot-time owner synchronization
// and by `addVerifiedEmail` at email-verification time, so the dual-path
// check the pre-Phase-5 helper used to do collapses to a single role lookup.
func requireServerAdmin(ctx context.Context, c *core.ChattoCore) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	isOwner, err := c.IsServerOwner(ctx, user.Id)
	if err != nil {
		return nil, fmt.Errorf("check owner status: %w", err)
	}
	if isOwner {
		return user, nil
	}

	isAdmin, err := c.IsServerAdmin(ctx, user.Id)
	if err != nil {
		return nil, fmt.Errorf("check admin status: %w", err)
	}
	if isAdmin {
		return user, nil
	}

	return nil, ErrNotServerAdmin
}

// canManageServerRoles checks the role.manage permission.
func (r *Resolver) canManageServerRoles(ctx context.Context, userID string) (bool, error) {
	return r.core.HasServerPermission(ctx, userID, core.PermRoleManage)
}

// canManageServerUsers checks the role.assign permission (i.e. who is
// allowed to change other users' role assignments).
func (r *Resolver) canManageServerUsers(ctx context.Context, userID string) (bool, error) {
	return r.core.HasServerPermission(ctx, userID, core.PermRoleAssign)
}

// requireRoomManageAuth gates room-level permission mutations. Passes for
// holders of role.manage at server scope (the broad "edit permissions
// anywhere" power) OR holders of room.manage on the specific target room
// (the narrow "moderator can edit their own room's permissions" power).
func (r *Resolver) requireRoomManageAuth(ctx context.Context, userID, roomID string) error {
	can, err := r.core.CanManageRoles(ctx, userID)
	if err != nil {
		return err
	}
	if can {
		return nil
	}
	if roomID != "" {
		has, err := r.core.PermResolver().HasRoomPermission(ctx, userID, core.KindChannel, roomID, core.PermRoomManage)
		if err != nil {
			return err
		}
		if has {
			return nil
		}
	}
	return core.ErrPermissionDenied
}

// requireUserAdminTarget verifies the caller can administer the given
// target user.
//
// Self-actions always pass. For caller != target, the caller must hold
// role.assign. Owners are protected from lockout by the effective-owner
// permission override, not by target-rank gates.
func (r *Resolver) requireUserAdminTarget(ctx context.Context, callerID, targetID string) error {
	if callerID == targetID {
		return nil
	}
	canManage, err := r.canManageServerUsers(ctx, callerID)
	if err != nil {
		return fmt.Errorf("failed to check admin permission: %w", err)
	}
	if !canManage {
		return core.ErrPermissionDenied
	}
	return nil
}

// requireSelfOrUserAdminTarget authenticates the caller and gates via
// requireUserAdminTarget. Convenience for the four self-or-admin
// mutations (updateProfile / uploadAvatar / deleteAvatar / updateSettings).
func (r *Resolver) requireSelfOrUserAdminTarget(ctx context.Context, targetUserID string) (*corev1.User, error) {
	caller, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.requireUserAdminTarget(ctx, caller.Id, targetUserID); err != nil {
		return nil, err
	}
	return caller, nil
}

// requireUserPermissionTarget gates mutations that change a user's
// authorization state — `grantUserPermission`, `denyUserPermission`,
// `clearUserPermissionState`.
//
// The permission gate is `user.manage-permissions`, not `role.assign`.
// Granting a permission directly to a user is separate from assigning an
// existing role and can target any user, including self.
func (r *Resolver) requireUserPermissionTarget(ctx context.Context, callerID, targetID string) error {
	canManage, err := r.core.CanManageUserPermissions(ctx, callerID)
	if err != nil {
		return fmt.Errorf("failed to check user.manage-permissions permission: %w", err)
	}
	if !canManage {
		return core.ErrPermissionDenied
	}
	return nil
}

// isServerAdmin returns true when the user has the owner or admin role.
func (r *Resolver) isServerAdmin(ctx context.Context, userID string) (bool, error) {
	isOwner, err := r.core.IsServerOwner(ctx, userID)
	if err != nil {
		return false, err
	}
	if isOwner {
		return true, nil
	}
	return r.core.IsServerAdmin(ctx, userID)
}

// requireRoleRosterAccess gates the role-roster and per-user-permission
// resolvers (Server.roleUsers / userEffectivePermissions / userEffectiveDenials).
// The caller must hold `role.assign` — the same permission required to
// actually modify role assignments. Non-admin callers cannot enumerate
// "who has the admin role" or read another user's effective permissions.
func (r *Resolver) requireRoleRosterAccess(ctx context.Context) error {
	caller, err := requireAuth(ctx)
	if err != nil {
		return err
	}
	can, err := r.canManageServerUsers(ctx, caller.Id)
	if err != nil {
		return err
	}
	if !can {
		return core.ErrPermissionDenied
	}
	return nil
}

// canViewUserEmails returns true when the caller is either the target
// user themselves or holds the admin.view-users permission. Used by
// User.verifiedEmails and User.hasVerifiedEmail to gate access to email
// content.
func (r *Resolver) canViewUserEmails(ctx context.Context, targetUserID string) bool {
	caller := auth.ForContext(ctx)
	if caller == nil {
		return false
	}
	if caller.Id == targetUserID {
		return true
	}
	can, err := r.core.CanAdminUsersView(ctx, caller.Id)
	if err != nil {
		r.logger.Warn("canViewUserEmails: permission check failed; treating as unauthorized", "error", err)
		return false
	}
	return can
}

// requireServerPermission verifies the user has a specific server permission.
func requireServerPermission(ctx context.Context, c *core.ChattoCore, perm core.Permission) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	hasPerm, err := c.HasServerPermission(ctx, user.Id, perm)
	if err != nil {
		return nil, fmt.Errorf("check permission: %w", err)
	}
	if !hasPerm {
		return nil, core.ErrPermissionDenied
	}

	return user, nil
}
