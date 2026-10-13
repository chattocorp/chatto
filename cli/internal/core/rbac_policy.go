package core

import (
	"context"
)

// HasServerPermission checks a server-only permission (no room context).
func (a *AuthorizationModel) HasServerPermission(ctx context.Context, userID string, perm Permission) (bool, error) {
	return a.resolver.HasServerPermission(ctx, userID, perm)
}

// IsServerAdmin checks if a user has the admin role via RBAC.
// Does NOT check config fallback (owners.emails) - caller should check that separately.
func (a *AuthorizationModel) IsServerAdmin(ctx context.Context, userID string) (bool, error) {
	return a.readContentBool(ctx, func(context.Context) bool {
		return a.rbac.hasRole(userID, RoleAdmin)
	})
}

// IsServerOwner checks whether a user has the durable owner role. Configured
// owners.emails entries are materialized into that role at boot and by the
// durable notification-effects worker after email verification, so live and
// event-time authorization cannot diverge.
func (a *AuthorizationModel) IsServerOwner(ctx context.Context, userID string) (bool, error) {
	return a.readContentBool(ctx, func(context.Context) bool {
		return a.isServerOwner(userID)
	})
}

func (a *AuthorizationModel) isServerOwner(userID string) bool {
	return a.rbac.hasRole(userID, RoleOwner)
}

func (a *AuthorizationModel) readContentBool(ctx context.Context, read func(context.Context) bool) (bool, error) {
	return a.readContentDecision(ctx, func(readCtx context.Context) (bool, error) {
		return read(readCtx), nil
	})
}

func (a *AuthorizationModel) readContentDecision(ctx context.Context, read func(context.Context) (bool, error)) (bool, error) {
	if a.view == nil {
		return read(ctx)
	}
	var result bool
	err := a.view.ReadContext(ctx, func(readCtx context.Context, _ uint64) error {
		var readErr error
		result, readErr = read(readCtx)
		return readErr
	})
	return result, err
}

// ResolveUserPermission returns the walker's decision (allow / deny / none)
// for a user-permission pair. Single source of truth for both the bool
// authorizer and the inspector. Pass roomID="" with KindDM to resolve the
// singleton direct-message scope before the server scope.
func (a *AuthorizationModel) ResolveUserPermission(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (DecisionKind, error) {
	return a.resolver.Resolve(ctx, userID, kind, roomID, perm)
}

// hasServerPermission checks a server-wide permission via the unified
// permission resolver. Internal building block — use the Can* helpers
// in can.go for authorization checks.
func (a *AuthorizationModel) hasServerPermission(ctx context.Context, userID string, perm Permission) (bool, error) {
	return a.resolver.HasSpacePermission(ctx, userID, KindChannel, perm)
}

// hasKindPermission is the kind-sensitive variant of hasServerPermission.
// For KindDM, the resolver uses the direct-message scope before Server.
func (a *AuthorizationModel) hasKindPermission(ctx context.Context, kind RoomKind, userID string, perm Permission) (bool, error) {
	return a.resolver.HasSpacePermission(ctx, userID, kind, perm)
}

// hasRoomPermission checks a permission at the room level. Each direct user or
// named role contributes its nearest room/group/server decision; named denies
// win across those subjects. Everyone supplies a scoped baseline: a named allow
// overrides its deny only at the same or a nearer scope.
func (a *AuthorizationModel) hasRoomPermission(ctx context.Context, kind RoomKind, roomID, userID string, perm Permission) (bool, error) {
	return a.resolver.HasRoomPermission(ctx, userID, kind, roomID, perm)
}

// hasGroupPermission checks a permission at the room-group level (no room
// context). Used for group-scoped capability gates like room.create. For
// permissions that are also ScopeServer, the resolver falls back to the
// server tier when no group decision exists.
func (a *AuthorizationModel) hasGroupPermission(ctx context.Context, kind RoomKind, groupID, userID string, perm Permission) (bool, error) {
	decision, err := a.resolver.ResolveGroup(ctx, userID, kind, groupID, perm)
	if err != nil {
		return false, err
	}
	return decision == DecisionAllow, nil
}
