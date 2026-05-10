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
// Space-scoped permissions require the user to be a space member.
func (r *PermissionResolver) HasSpacePermission(ctx context.Context, userID, spaceID string, perm Permission) (bool, error) {
	if meta, known := GetPermissionMetadata(perm); known {
		if !permissionMetadataHasScope(meta, ScopeSpace) && !permissionMetadataHasScope(meta, ScopeInstance) {
			return false, fmt.Errorf("permission %s does not apply at space scope", perm)
		}
	}

	if IsDMSpace(spaceID) {
		return r.resolveDMPermission(perm), nil
	}

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
	kv := r.core.storage.serverRBACEngine.KV()

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

// walkSpacePermission walks the space-level resolution sequence: phase 1 scans
// all denials across instance + space (deny-always-wins), phase 2 scans grants
// in authority order (instance → space). The walker stops as soon as visit
// returns visitStop, but the order is identical regardless.
func (r *PermissionResolver) walkSpacePermission(
	ctx context.Context, userID, spaceID string, perm Permission, visit visitFunc,
) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return nil
	}

	instanceRoles, err := r.getUserInstanceRoles(ctx, userID)
	if err != nil {
		return err
	}
	spaceRoles, err := r.getUserSpaceRoles(ctx, spaceID, userID)
	if err != nil {
		return err
	}
	instanceOnlyRoles := filterOutSpaceRoles(instanceRoles, spaceRoles)

	instanceKV := r.core.storage.serverRBACEngine.KV()
	spaceKV, err := r.core.getSpaceRBACKV(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to get space RBAC KV: %w", err)
	}

	// Phase 1: denials across all levels.
	for _, role := range instanceRoles {
		denied, err := r.keyExists(ctx, instanceKV, rbac.DenyKey(role, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if denied {
			r.core.logger.Debug("Permission denied by instance role", "role", role, "permission", string(perm), "user", userID)
			if visit(TraceEntry{Level: LevelInstance, RoleName: role, Decision: DecisionDeny, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}
	for _, role := range spaceRoles {
		denied, err := r.keyExists(ctx, spaceKV, rbac.DenyKey(role, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if denied {
			r.core.logger.Debug("Permission denied by space role", "role", role, "permission", string(perm), "space", spaceID, "user", userID)
			if visit(TraceEntry{Level: LevelSpace, RoleName: role, Decision: DecisionDeny, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}
	for _, role := range instanceOnlyRoles {
		denied, err := r.keyExists(ctx, spaceKV, rbac.DenyKey(role, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if denied {
			r.core.logger.Debug("Permission denied by instance role (space config)", "role", role, "permission", string(perm), "space", spaceID, "user", userID)
			if visit(TraceEntry{Level: LevelSpace, RoleName: role, Decision: DecisionDeny, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}

	// Phase 2: grants in authority order (instance → space).
	if PermissionAppliesAtScope(perm, ScopeInstance) {
		for _, role := range instanceRoles {
			granted, err := r.keyExists(ctx, instanceKV, rbac.AllowKey(role, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
			if err != nil {
				return err
			}
			if granted {
				if visit(TraceEntry{Level: LevelInstance, RoleName: role, Decision: DecisionAllow, ObjectID: rbac.ObjectIdAny}) == visitStop {
					return nil
				}
			}
		}
	}
	for _, role := range spaceRoles {
		granted, err := r.keyExists(ctx, spaceKV, rbac.AllowKey(role, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if granted {
			if visit(TraceEntry{Level: LevelSpace, RoleName: role, Decision: DecisionAllow, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}
	for _, role := range instanceOnlyRoles {
		granted, err := r.keyExists(ctx, spaceKV, rbac.AllowKey(role, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if granted {
			if visit(TraceEntry{Level: LevelSpace, RoleName: role, Decision: DecisionAllow, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}

	return nil
}

// walkRoomPermission walks the room-level resolution sequence: instance + space
// denials (deny-always-wins), then a hierarchy walk over room overrides
// (allow-or-deny per role, first found wins), then instance + space grants as
// fallback when nothing decided at the room level.
func (r *PermissionResolver) walkRoomPermission(
	ctx context.Context, userID, spaceID, roomID string, perm Permission, visit visitFunc,
) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return nil
	}

	instanceRoles, err := r.getUserInstanceRoles(ctx, userID)
	if err != nil {
		return err
	}
	spaceRoles, err := r.getUserSpaceRoles(ctx, spaceID, userID)
	if err != nil {
		return err
	}
	instanceOnlyRoles := filterOutSpaceRoles(instanceRoles, spaceRoles)

	instanceKV := r.core.storage.serverRBACEngine.KV()
	spaceKV, err := r.core.getSpaceRBACKV(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to get space RBAC KV: %w", err)
	}

	// Phase 1: instance + space denials.
	for _, role := range instanceRoles {
		denied, err := r.keyExists(ctx, instanceKV, rbac.DenyKey(role, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if denied {
			r.core.logger.Debug("Permission denied by instance role", "role", role, "permission", string(perm), "room", roomID, "user", userID)
			if visit(TraceEntry{Level: LevelInstance, RoleName: role, Decision: DecisionDeny, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}
	for _, role := range spaceRoles {
		denied, err := r.keyExists(ctx, spaceKV, rbac.DenyKey(role, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if denied {
			r.core.logger.Debug("Permission denied by space role", "role", role, "permission", string(perm), "room", roomID, "user", userID)
			if visit(TraceEntry{Level: LevelSpace, RoleName: role, Decision: DecisionDeny, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}
	for _, role := range instanceOnlyRoles {
		denied, err := r.keyExists(ctx, spaceKV, rbac.DenyKey(role, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err != nil {
			return err
		}
		if denied {
			r.core.logger.Debug("Permission denied by instance role (space config)", "role", role, "permission", string(perm), "room", roomID, "user", userID)
			if visit(TraceEntry{Level: LevelSpace, RoleName: role, Decision: DecisionDeny, ObjectID: rbac.ObjectIdAny}) == visitStop {
				return nil
			}
		}
	}

	// Phase 2: room-level hierarchy walk.
	if PermissionAppliesAtScope(perm, ScopeRoom) {
		rolesWithPos, err := r.getUserSpaceRolesWithPositions(ctx, spaceID, userID)
		if err != nil {
			return err
		}

		for _, rp := range rolesWithPos {
			granted, err := r.keyExists(ctx, spaceKV, rbac.AllowKey(rp.name, parts.Verb, parts.ObjectType, roomID))
			if err != nil {
				return err
			}
			if granted {
				r.core.logger.Debug("Permission granted by space role (room config, hierarchy)", "role", rp.name, "position", rp.position, "permission", string(perm), "room", roomID, "user", userID)
				if visit(TraceEntry{Level: LevelRoom, RoleName: rp.name, Decision: DecisionAllow, ObjectID: roomID}) == visitStop {
					return nil
				}
				continue
			}

			denied, err := r.keyExists(ctx, spaceKV, rbac.DenyKey(rp.name, parts.Verb, parts.ObjectType, roomID))
			if err != nil {
				return err
			}
			if denied {
				r.core.logger.Debug("Permission denied by space role (room config, hierarchy)", "role", rp.name, "position", rp.position, "permission", string(perm), "room", roomID, "user", userID)
				if visit(TraceEntry{Level: LevelRoom, RoleName: rp.name, Decision: DecisionDeny, ObjectID: roomID}) == visitStop {
					return nil
				}
			}
		}

		for _, role := range instanceOnlyRoles {
			granted, err := r.keyExists(ctx, spaceKV, rbac.AllowKey(role, parts.Verb, parts.ObjectType, roomID))
			if err != nil {
				return err
			}
			if granted {
				r.core.logger.Debug("Permission granted by instance role (room config)", "role", role, "permission", string(perm), "room", roomID, "user", userID)
				if visit(TraceEntry{Level: LevelRoom, RoleName: role, Decision: DecisionAllow, ObjectID: roomID}) == visitStop {
					return nil
				}
				continue
			}

			denied, err := r.keyExists(ctx, spaceKV, rbac.DenyKey(role, parts.Verb, parts.ObjectType, roomID))
			if err != nil {
				return err
			}
			if denied {
				r.core.logger.Debug("Permission denied by instance role (room config)", "role", role, "permission", string(perm), "room", roomID, "user", userID)
				if visit(TraceEntry{Level: LevelRoom, RoleName: role, Decision: DecisionDeny, ObjectID: roomID}) == visitStop {
					return nil
				}
			}
		}
	}

	// Phase 3: instance + space grants (fallback when no room-level decision).
	if PermissionAppliesAtScope(perm, ScopeInstance) {
		for _, role := range instanceRoles {
			granted, err := r.keyExists(ctx, instanceKV, rbac.AllowKey(role, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
			if err != nil {
				return err
			}
			if granted {
				if visit(TraceEntry{Level: LevelInstance, RoleName: role, Decision: DecisionAllow, ObjectID: rbac.ObjectIdAny}) == visitStop {
					return nil
				}
			}
		}
	}
	if PermissionAppliesAtScope(perm, ScopeSpace) {
		for _, role := range spaceRoles {
			granted, err := r.keyExists(ctx, spaceKV, rbac.AllowKey(role, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
			if err != nil {
				return err
			}
			if granted {
				if visit(TraceEntry{Level: LevelSpace, RoleName: role, Decision: DecisionAllow, ObjectID: rbac.ObjectIdAny}) == visitStop {
					return nil
				}
			}
		}
		for _, role := range instanceOnlyRoles {
			granted, err := r.keyExists(ctx, spaceKV, rbac.AllowKey(role, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
			if err != nil {
				return err
			}
			if granted {
				if visit(TraceEntry{Level: LevelSpace, RoleName: role, Decision: DecisionAllow, ObjectID: rbac.ObjectIdAny}) == visitStop {
					return nil
				}
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

// filterOutSpaceRoles returns instance roles that don't appear in spaceRoles.
// Universal roles (everyone) appear in both lists; this avoids redundant
// space KV lookups since they're already checked via the space roles loop.
func filterOutSpaceRoles(instanceRoles, spaceRoles []string) []string {
	spaceSet := make(map[string]struct{}, len(spaceRoles))
	for _, r := range spaceRoles {
		spaceSet[r] = struct{}{}
	}
	var result []string
	for _, r := range instanceRoles {
		if _, ok := spaceSet[r]; !ok {
			result = append(result, r)
		}
	}
	return result
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

	engine := r.core.storage.serverRBACEngine

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
