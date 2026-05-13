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

// requireSpaceMember verifies the caller can access the given room kind.
//
// Post-consolidation every authenticated user is implicitly a server member,
// so for channel rooms the check collapses to `requireAuth`. The DM kind is
// still a real gate — callers without `dm.view` are rejected here.
func requireSpaceMember(ctx context.Context, c *core.ChattoCore, kind core.RoomKind) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if kind == core.KindDM {
		can, err := c.HasInstancePermission(ctx, user.Id, core.PermDMView)
		if err != nil {
			return nil, fmt.Errorf("failed to check DM permission: %w", err)
		}
		if !can {
			return nil, core.ErrPermissionDenied
		}
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

// canManageServerRoles checks the role.manage permission.
func (r *Resolver) canManageServerRoles(ctx context.Context, userID string) (bool, error) {
	return r.core.HasInstancePermission(ctx, userID, core.PermRoleManage)
}

// canManageInstanceUsers checks the role.assign permission (i.e. who is
// allowed to change other users' role assignments).
func (r *Resolver) canManageInstanceUsers(ctx context.Context, userID string) (bool, error) {
	return r.core.HasInstancePermission(ctx, userID, core.PermRoleAssign)
}

// requireRoomManageAuth gates room-level permission mutations on PermRoleManage.
func (r *Resolver) requireRoomManageAuth(ctx context.Context, userID string) error {
	can, err := r.core.CanManageRoles(ctx, userID)
	if err != nil {
		return err
	}
	if !can {
		return core.ErrPermissionDenied
	}
	return nil
}

// requireUserAdminTarget verifies the caller can administer the given
// target user via a "permission AND rank" two-step gate.
//
// Self-actions always pass. For caller != target, the caller must:
//   - hold the role.assign permission (canManageInstanceUsers), AND
//   - strictly outrank the target.
//
// Peer ranks deny — including peer owners. If two owners need to
// administer each other's identity, one of them must demote the other
// first. This matches RevokeServerRole's symmetric peer-deny.
//
// This is the canonical gate for targeted user mutations like
// updateProfile / uploadAvatar / deleteAvatar / updateSettings /
// AdminMutations.updateUser / ClearUsernameCooldown. Rank-only gating
// is a bug — see .claude/rules/authorization.md and issue #435.
func (r *Resolver) requireUserAdminTarget(ctx context.Context, callerID, targetID string) error {
	if callerID == targetID {
		return nil
	}
	canManage, err := r.canManageInstanceUsers(ctx, callerID)
	if err != nil {
		return fmt.Errorf("failed to check admin permission: %w", err)
	}
	if !canManage {
		return core.ErrPermissionDenied
	}
	outranks, err := r.core.OutranksUser(ctx, callerID, targetID)
	if err != nil {
		return fmt.Errorf("failed to check role hierarchy: %w", err)
	}
	if !outranks {
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
// Unlike requireUserAdminTarget, this helper has NO self-bypass. Editing
// your own display name is privilege-neutral; granting yourself a
// permission is a security-boundary change. The two operation categories
// must not share a gate. Self-action falls through the same two checks
// as any other caller: the strict-outrank step (`OutranksUser`) always
// returns false for self, so self-action is impossible by construction
// without a special branch.
//
// The permission gate is `role.manage`, not `role.assign`. Granting a
// permission directly to a user is strictly more powerful than assigning
// a role: it can attach any permission, including ones operators chose
// not to put on any role. That matches the trust level of `role.manage`
// (which is what lets you put a permission on a role in the first
// place) rather than `role.assign` (which only shuffles existing role
// memberships).
func (r *Resolver) requireUserPermissionTarget(ctx context.Context, callerID, targetID string) error {
	canManage, err := r.core.CanManageRoles(ctx, callerID)
	if err != nil {
		return fmt.Errorf("failed to check role.manage permission: %w", err)
	}
	if !canManage {
		return core.ErrPermissionDenied
	}
	outranks, err := r.core.OutranksUser(ctx, callerID, targetID)
	if err != nil {
		return fmt.Errorf("failed to check role hierarchy: %w", err)
	}
	if !outranks {
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

// requireOutranksAuthor enforces the message-moderation rank check: when
// the actor is moderating someone else's message (edit-any / delete-any),
// they must strictly outrank the author. Self-action callers should skip
// this — they don't need the rank check.
//
// Combine with the permission check: permission says "is this role allowed
// to moderate at all?", rank says "are you allowed to moderate THIS
// specific user?". This is the same "permission AND OutranksUser" shape as
// requireUserAdminTarget, applied to message-content moderation. It
// prevents a moderator from editing or deleting messages from higher-rank
// users (admins, owners), and prevents peer-rank message moderation
// generally.
//
// Returns nil for self (defensive — callers route self-edits through
// CanEditOwnMessage, but the guard is here for completeness).
func (r *Resolver) requireOutranksAuthor(ctx context.Context, actorID, authorID string) error {
	if actorID == authorID {
		return nil
	}
	outranks, err := r.core.OutranksUser(ctx, actorID, authorID)
	if err != nil {
		return fmt.Errorf("failed to check author rank: %w", err)
	}
	if !outranks {
		return core.ErrPermissionDenied
	}
	return nil
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
	can, err := r.canManageInstanceUsers(ctx, caller.Id)
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
