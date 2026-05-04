package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/core/rbac"
)

// PermissionResolver handles permission resolution using role hierarchy:
//
// At all levels (instance, space, room), the role with the highest rank
// (lowest position number) whose explicit grant/deny decision is found first wins.
//
// Resolution rules:
// 1. Get user's roles sorted by hierarchy (lower position = higher rank)
// 2. For each role, check for explicit grant or deny
// 3. First explicit decision found → that's the answer
// 4. If no explicit decision at current level → fall back to parent level
//
// This enables patterns like:
// - #announcements rooms where "everyone" is denied message.post but
//   "owner/admin/moderator" can still post because they have higher rank
// - Instance admin not being blocked by a "everyone" denial because
//   admin is checked first in the hierarchy
//
// The internal walk*Permission methods take a visitor callback and form the
// single source of truth for resolution ordering. HasXxxPermission and
// ExplainXxxPermission are both thin wrappers around these walkers — the bool
// path stops on the first decision, the explainer accumulates the full trace.
type PermissionResolver struct {
	core *ChattoCore
}

// NewPermissionResolver creates a new permission resolver.
func NewPermissionResolver(core *ChattoCore) *PermissionResolver {
	return &PermissionResolver{core: core}
}

// PermissionLevel identifies the level at which a permission decision was reached.
type PermissionLevel string

const (
	LevelInstance PermissionLevel = "instance"
	LevelSpace    PermissionLevel = "space"
	LevelRoom     PermissionLevel = "room"
)

// DecisionKind is the kind of decision a role contributed.
type DecisionKind string

const (
	DecisionAllow DecisionKind = "allow"
	DecisionDeny  DecisionKind = "deny"
	DecisionNone  DecisionKind = "none"
)

// TraceEntry is one step in the permission resolution trace.
// Only entries actually backed by a KV value are emitted (allow or deny);
// roles with no KV entry at the level being checked are silent.
type TraceEntry struct {
	Level    PermissionLevel
	RoleName string
	Decision DecisionKind // Allow or Deny only
	ObjectID string       // "any" for instance/space scope; roomID for room overrides
}

// visitOutcome is returned by a visitFunc to control walker iteration.
type visitOutcome int

const (
	visitContinue visitOutcome = iota
	visitStop
)

// visitFunc is invoked once per "found" allow/deny KV entry. The first
// invocation corresponds to the entry the bool path would short-circuit on;
// the explain path keeps walking and records every entry.
type visitFunc func(entry TraceEntry) visitOutcome

// HasInstancePermission checks if a user has a permission at the instance level.
// Only checks instance-level roles and KV. Used for permissions that only apply
// at instance scope (like admin.access, space.create, dm.view).
func (r *PermissionResolver) HasInstancePermission(ctx context.Context, userID string, perm Permission) (bool, error) {
	if meta, known := GetPermissionMetadata(perm); known && !permissionMetadataHasScope(meta, ScopeInstance) {
		return false, fmt.Errorf("permission %s does not apply at instance scope", perm)
	}

	var result bool
	err := r.walkInstancePermission(ctx, userID, perm, func(entry TraceEntry) visitOutcome {
		result = entry.Decision == DecisionAllow
		return visitStop
	})
	return result, err
}

// HasSpacePermission checks if a user has a permission at the space level.
//
// Uses the deny-always-wins model: all denials across all levels are checked
// first, then grants are checked in authority order (instance → space).
//
// For space-scoped permissions (like room.create), the user must be a space
// member. space.join and space.list are exempt (non-members need them for discovery).
func (r *PermissionResolver) HasSpacePermission(ctx context.Context, userID, spaceID string, perm Permission) (bool, error) {
	if meta, known := GetPermissionMetadata(perm); known {
		if !permissionMetadataHasScope(meta, ScopeSpace) && !permissionMetadataHasScope(meta, ScopeInstance) {
			return false, fmt.Errorf("permission %s does not apply at space scope", perm)
		}
	}

	if IsDMSpace(spaceID) {
		return r.resolveDMPermission(perm), nil
	}

	// Discovery permissions (space.join / space.list) used to be exempted from the
	// membership precheck because non-members legitimately need them to see and
	// join. Those permissions are dropped per ADR-028 — discovery is now
	// unrestricted at the helper layer. The precheck below applies to every
	// remaining space-scoped permission.
	if PermissionAppliesAtScope(perm, ScopeSpace) {
		isMember, err := r.core.SpaceMembershipExists(ctx, userID, spaceID)
		if err != nil {
			return false, fmt.Errorf("failed to check space membership: %w", err)
		}
		if !isMember {
			return false, nil
		}
	}

	var result bool
	err := r.walkSpacePermission(ctx, userID, spaceID, perm, func(entry TraceEntry) visitOutcome {
		result = entry.Decision == DecisionAllow
		return visitStop
	})
	return result, err
}

// HasRoomPermission checks if a user has a permission at the room level.
//
// Resolution order:
// 1. Instance/space denials (deny-always-wins at these levels)
// 2. Room-level permissions: walk roles in hierarchy order, allow-or-deny per role
// 3. Instance/space grants (fallback when no room-level decision)
func (r *PermissionResolver) HasRoomPermission(ctx context.Context, userID, spaceID, roomID string, perm Permission) (bool, error) {
	if !PermissionAppliesAtScope(perm, ScopeRoom) && !PermissionAppliesAtScope(perm, ScopeSpace) && !PermissionAppliesAtScope(perm, ScopeInstance) {
		return false, fmt.Errorf("permission %s does not apply at room scope", perm)
	}

	if IsDMSpace(spaceID) {
		return r.resolveDMPermission(perm), nil
	}

	var result bool
	err := r.walkRoomPermission(ctx, userID, spaceID, roomID, perm, func(entry TraceEntry) visitOutcome {
		result = entry.Decision == DecisionAllow
		return visitStop
	})
	return result, err
}

// permissionMetadataHasScope checks if a permission applies at the given scope.
func permissionMetadataHasScope(meta PermissionMetadata, scope PermissionScope) bool {
	for _, s := range meta.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// ============================================================================
// Walker Methods (single source of truth for resolution ordering)
// ============================================================================

// walkInstancePermission walks the instance-level resolution sequence: roles in
// hierarchy order (highest rank first), allow-then-deny per role, first found wins.
func (r *PermissionResolver) walkInstancePermission(
	ctx context.Context, userID string, perm Permission, visit visitFunc,
) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return nil
	}

	rolesWithPos, err := r.getUserInstanceRolesWithPositions(ctx, userID)
	if err != nil {
		return err
	}
	kv := r.core.instanceRBACEngine.KV()

	for _, rp := range rolesWithPos {
		granted, err := r.keyExists(ctx, kv, rbac.AllowKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if granted {
			r.core.logger.Debug("Permission granted by instance role (hierarchy)", "role", rp.name, "position", rp.position, "permission", string(perm), "user", userID)
			if visit(TraceEntry{Level: LevelInstance, RoleName: rp.name, Decision: DecisionAllow, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
			continue
		}

		denied, err := r.keyExists(ctx, kv, rbac.DenyKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if denied {
			r.core.logger.Debug("Permission denied by instance role (hierarchy)", "role", rp.name, "position", rp.position, "permission", string(perm), "user", userID)
			if visit(TraceEntry{Level: LevelInstance, RoleName: rp.name, Decision: DecisionDeny, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}

	return nil
}

// walkSpacePermission walks the server-level resolution sequence in the
// post-merge model: roles in hierarchy order, deny then allow per role, first
// found wins. The instance/space distinction is gone (one bucket, one role
// namespace) so this is now structurally identical to walkInstancePermission;
// it stays as a separate entry point through PR 9 to keep call sites stable.
func (r *PermissionResolver) walkSpacePermission(
	ctx context.Context, userID, spaceID string, perm Permission, visit visitFunc,
) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return nil
	}

	_ = spaceID // retained for call-site compatibility through PR 9
	rolesWithPos, err := r.getUserInstanceRolesWithPositions(ctx, userID)
	if err != nil {
		return err
	}
	kv := r.core.instanceRBACEngine.KV()

	// Phase 1: denials in hierarchy order.
	for _, rp := range rolesWithPos {
		denied, err := r.keyExists(ctx, kv, rbac.DenyKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if denied {
			r.core.logger.Debug("Permission denied by role", "role", rp.name, "permission", string(perm), "user", userID)
			if visit(TraceEntry{Level: LevelSpace, RoleName: rp.name, Decision: DecisionDeny, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}

	// Phase 2: grants in hierarchy order.
	for _, rp := range rolesWithPos {
		granted, err := r.keyExists(ctx, kv, rbac.AllowKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if granted {
			if visit(TraceEntry{Level: LevelSpace, RoleName: rp.name, Decision: DecisionAllow, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}

	return nil
}

// walkRoomPermission walks the room-level resolution sequence: server-scope
// denials (deny-always-wins), then a hierarchy walk over room overrides
// (allow-or-deny per role, first found wins), then server-scope grants as
// fallback when nothing decided at the room level.
//
// Per ADR-021 / ADR-028 (PR 4) the instance/space distinction is gone; all
// resolution happens against the unified bucket using a single role list.
func (r *PermissionResolver) walkRoomPermission(
	ctx context.Context, userID, spaceID, roomID string, perm Permission, visit visitFunc,
) error {
	_ = spaceID // retained for call-site compatibility through PR 9
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return nil
	}

	rolesWithPos, err := r.getUserInstanceRolesWithPositions(ctx, userID)
	if err != nil {
		return err
	}
	kv := r.core.instanceRBACEngine.KV()

	// Phase 1: server-scope denials.
	for _, rp := range rolesWithPos {
		denied, err := r.keyExists(ctx, kv, rbac.DenyKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if denied {
			r.core.logger.Debug("Permission denied by role", "role", rp.name, "permission", string(perm), "room", roomID, "user", userID)
			if visit(TraceEntry{Level: LevelSpace, RoleName: rp.name, Decision: DecisionDeny, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}

	// Phase 2: room-level hierarchy walk (allow-or-deny per role).
	if PermissionAppliesAtScope(perm, ScopeRoom) {
		for _, rp := range rolesWithPos {
			granted, err := r.keyExists(ctx, kv, rbac.AllowKey(rp.name, parts.Verb, parts.ObjectType, roomID))
			if err != nil {
				return err
			}
			if granted {
				r.core.logger.Debug("Permission granted by role (room override)", "role", rp.name, "position", rp.position, "permission", string(perm), "room", roomID, "user", userID)
				if visit(TraceEntry{Level: LevelRoom, RoleName: rp.name, Decision: DecisionAllow, ObjectID: roomID}) == visitStop {
					return nil
				}
				continue
			}

			denied, err := r.keyExists(ctx, kv, rbac.DenyKey(rp.name, parts.Verb, parts.ObjectType, roomID))
			if err != nil {
				return err
			}
			if denied {
				r.core.logger.Debug("Permission denied by role (room override)", "role", rp.name, "position", rp.position, "permission", string(perm), "room", roomID, "user", userID)
				if visit(TraceEntry{Level: LevelRoom, RoleName: rp.name, Decision: DecisionDeny, ObjectID: roomID}) == visitStop {
					return nil
				}
			}
		}
	}

	// Phase 3: server-scope grants (fallback when no room-level decision).
	for _, rp := range rolesWithPos {
		granted, err := r.keyExists(ctx, kv, rbac.AllowKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if granted {
			if visit(TraceEntry{Level: LevelSpace, RoleName: rp.name, Decision: DecisionAllow, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}

	return nil
}

// resolveDMPermission returns whether a permission is allowed in DM context.
// DM space uses simplified permissions - only certain actions are allowed.
func (r *PermissionResolver) resolveDMPermission(perm Permission) bool {
	switch perm {
	case PermMessagePost, PermMessageEditOwn, PermMessageDeleteOwn, PermMessageReact,
		PermMessageReply, PermRoomJoin, PermRoomLeave:
		return true
	default:
		return false
	}
}

// ============================================================================
// Helper Methods
// ============================================================================

// keyExists checks if a key exists in a KV bucket.
func (r *PermissionResolver) keyExists(ctx context.Context, kv jetstream.KeyValue, key string) (bool, error) {
	_, err := kv.Get(ctx, key)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check key %s: %w", key, err)
}

// getUserInstanceRoles returns the user's instance roles (including implicit ones).
func (r *PermissionResolver) getUserInstanceRoles(ctx context.Context, userID string) ([]string, error) {
	roles, err := r.core.GetUserInstanceRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user instance roles: %w", err)
	}

	// Always include "everyone" for authenticated users
	if !slices.Contains(roles, RoleEveryone) {
		roles = append(roles, RoleEveryone)
	}

	return roles, nil
}

// getUserSpaceRoles returns the user's space roles (including implicit everyone role if member).
func (r *PermissionResolver) getUserSpaceRoles(ctx context.Context, spaceID, userID string) ([]string, error) {
	roles, err := r.core.GetUserRoles(ctx, spaceID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user space roles: %w", err)
	}

	return roles, nil
}

// roleWithPosition pairs a role name with its position for hierarchy sorting.
type roleWithPosition struct {
	name     string
	position int32
}

// getUserSpaceRolesWithPositions returns user's space roles with positions, sorted by hierarchy.
// Lower position = higher rank (checked first in permission resolution).
func (r *PermissionResolver) getUserSpaceRolesWithPositions(ctx context.Context, spaceID, userID string) ([]roleWithPosition, error) {
	roleNames, err := r.getUserSpaceRoles(ctx, spaceID, userID)
	if err != nil {
		return nil, err
	}

	engine, err := r.core.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get space RBAC engine: %w", err)
	}

	result := make([]roleWithPosition, 0, len(roleNames))
	for _, name := range roleNames {
		pos := rbac.PositionEveryone // Default for virtual roles or if lookup fails
		if role, err := engine.GetRole(ctx, name); err == nil && role != nil {
			pos = role.Position
		}
		result = append(result, roleWithPosition{name: name, position: pos})
	}

	// Sort by position ascending (lower = higher rank = checked first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].position < result[j].position
	})

	return result, nil
}

// getUserInstanceRolesWithPositions returns user's instance roles with positions, sorted by hierarchy.
// Lower position = higher rank (checked first in permission resolution).
func (r *PermissionResolver) getUserInstanceRolesWithPositions(ctx context.Context, userID string) ([]roleWithPosition, error) {
	roleNames, err := r.getUserInstanceRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	engine := r.core.instanceRBACEngine

	result := make([]roleWithPosition, 0, len(roleNames))
	for _, name := range roleNames {
		pos := rbac.PositionEveryone // Default for virtual roles or if lookup fails
		if role, err := engine.GetRole(ctx, name); err == nil && role != nil {
			pos = role.Position
		}
		result = append(result, roleWithPosition{name: name, position: pos})
	}

	// Sort by position ascending (lower = higher rank = checked first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].position < result[j].position
	})

	return result, nil
}
