package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/nats-io/nats.go/jetstream"

)

// PermissionResolver handles permission resolution using a single
// hierarchy-wins algorithm.
//
// For each role assigned to the user, in hierarchy order (highest rank
// first), check for an explicit decision in this priority order:
//   1. room-level allow (if a room context was provided)
//   2. room-level deny  (if a room context was provided)
//   3. server-level allow
//   4. server-level deny
//
// The first decision encountered is the answer; lower-ranked roles are
// not consulted further. If no role has any decision the result is
// "no decision" (treated as deny at the API boundary).
//
// Consequences worth knowing:
//   - A higher-ranked role's grant overrides a lower-ranked role's deny.
//     This enables patterns like an `#announcements` room where the
//     `everyone` role is denied `message.post` but `moderator` can still
//     post by virtue of an explicit grant.
//   - Within a single role, a room-level decision overrides a server-level
//     decision (room is the more specific scope).
//   - There is no longer a "deny-always-wins" floor at the server level.
//     An operator who wants to forbid an action across the board should
//     deny on the highest-ranked role that should be affected.
//
// The single walkPermission method is the source of truth. The Has*
// wrappers stop on the first decision; the Explain* wrappers keep
// walking and accumulate the full trace.
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
	LevelServer PermissionLevel = "server"
	LevelGroup  PermissionLevel = "group"
	LevelRoom   PermissionLevel = "room"
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
	ObjectID string       // "any" for server scope; groupID for group scope; roomID for room overrides
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

// Resolve is the single resolver entry point. Returns the walker's first
// decision (allow / deny / none) for the user-permission pair. Both the bool
// authorizer (Has*Permission) and the inspector go through this — there is
// no parallel implementation.
//
// Order of operations:
//
//  1. DM boundary deny-list (for kind == KindDM only) — permissions in
//     dmBoundaryDeniedPermissions are unconditionally denied regardless of
//     grants. This is the privacy/category-mismatch floor.
//  2. User-level overrides — explicit grants/denies on the user themselves
//     beat every role grant. Room scope is probed before server scope;
//     first user-level hit wins.
//  3. Role hierarchy walker — iterate the user's roles in hierarchy order
//     (highest rank first) and emit the first allow/deny found at room
//     scope (if roomID is set) or server scope.
//
// There is no "bypass" short-circuit. Owners pass permission checks
// because the owner role is seeded with every server-scope permission
// enumerated, not because the resolver special-cases them. This means
// any deny the operator configures applies uniformly — there is no role
// or user that can sidestep the model.
func (r *PermissionResolver) Resolve(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (DecisionKind, error) {
	return r.resolveWithGroup(ctx, userID, kind, roomID, "", perm)
}

// ResolveGroup is like Resolve but for group-scope checks (no room context).
// Used by CanCreateRoom and other group-scoped capability gates.
func (r *PermissionResolver) ResolveGroup(ctx context.Context, userID string, kind RoomKind, groupID string, perm Permission) (DecisionKind, error) {
	return r.resolveWithGroup(ctx, userID, kind, "", groupID, perm)
}

func (r *PermissionResolver) resolveWithGroup(ctx context.Context, userID string, kind RoomKind, roomID, explicitGroupID string, perm Permission) (DecisionKind, error) {
	if kind == KindDM && dmBoundaryDenies(perm) {
		return DecisionDeny, nil
	}

	// For channel rooms with a room-scope permission, the resolver walks
	// room → group (ADR-031). We look up the room's group once so both the
	// user-level and role-walk phases can probe it without a second KV
	// read. If a groupID was passed explicitly (group-scope check without a
	// room), use it directly.
	groupID := explicitGroupID
	useChannelRoomPath := kind == KindChannel && roomID != "" && PermissionAppliesAtScope(perm, ScopeRoom)
	if useChannelRoomPath && groupID == "" {
		if room, err := r.core.GetRoom(ctx, KindChannel, roomID); err == nil && room != nil {
			groupID = room.GroupId
		}
		// If the room lookup fails or the room has no groupID yet
		// (transitional pre-migration state), the room-scope keys are
		// still probed; the group-scope probe is skipped.
	}

	// Phase 1: user-level overrides.
	decision, err := r.probeUserLevel(ctx, userID, kind, roomID, groupID, perm)
	if err != nil {
		return DecisionNone, err
	}
	if decision != DecisionNone {
		return decision, nil
	}

	// Phase 2: role hierarchy walk.
	result := DecisionNone
	err = r.walkRoles(ctx, userID, kind, roomID, groupID, perm, func(entry TraceEntry) visitOutcome {
		result = entry.Decision
		return visitStop
	})
	return result, err
}

// probeUserLevel checks for an explicit user-level grant/deny.
//
// Walk order:
//   - Channel room (roomID set): room R → group G → server (fallback only if
//     the perm has ScopeServer in addition to ScopeRoom).
//   - Channel group only (groupID set, no roomID): group G → server (fallback
//     only if the perm has ScopeServer in addition to ScopeGroup).
//   - Otherwise (DMs, pure server checks): server allow/deny.
//
// Returns DecisionNone if no user-level decision exists.
func (r *PermissionResolver) probeUserLevel(ctx context.Context, userID string, kind RoomKind, roomID, groupID string, perm Permission) (DecisionKind, error) {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return DecisionNone, nil
	}
	kv := r.core.storage.serverRBACEngine.KV()
	hasServerScope := PermissionAppliesAtScope(perm, ScopeServer)

	if kind == KindChannel && roomID != "" && PermissionAppliesAtScope(perm, ScopeRoom) {
		got, err := r.probeRoomOnce(ctx, kv, userID, parts, roomID)
		if err != nil {
			return DecisionNone, err
		}
		if got != DecisionNone {
			return got, nil
		}
		if groupID != "" {
			got, err := r.probeSetOnce(ctx, kv, userID, parts, groupID)
			if err != nil {
				return DecisionNone, err
			}
			if got != DecisionNone {
				return got, nil
			}
		}
		if hasServerScope {
			return r.probeServerOnce(ctx, kv, userID, parts)
		}
		return DecisionNone, nil
	}

	if kind == KindChannel && groupID != "" && PermissionAppliesAtScope(perm, ScopeGroup) {
		got, err := r.probeSetOnce(ctx, kv, userID, parts, groupID)
		if err != nil {
			return DecisionNone, err
		}
		if got != DecisionNone {
			return got, nil
		}
		if hasServerScope {
			return r.probeServerOnce(ctx, kv, userID, parts)
		}
		return DecisionNone, nil
	}

	return r.probeServerOnce(ctx, kv, userID, parts)
}

// probeServerOnce checks the server-scope (allow, deny) pair for a subject and
// returns the decision. Uses the legacy allow.{subject}.{verb}.{type}.any
// keys. Used for server-scope checks and DM rooms.
func (r *PermissionResolver) probeServerOnce(ctx context.Context, kv jetstream.KeyValue, subject string, parts PermissionKeyParts) (DecisionKind, error) {
	allowed, err := r.keyExists(ctx, kv, AllowKey(subject, parts.Verb, parts.ObjectType, ObjectIdAny))
	if err != nil {
		return DecisionNone, err
	}
	if allowed {
		return DecisionAllow, nil
	}
	denied, err := r.keyExists(ctx, kv, DenyKey(subject, parts.Verb, parts.ObjectType, ObjectIdAny))
	if err != nil {
		return DecisionNone, err
	}
	if denied {
		return DecisionDeny, nil
	}
	return DecisionNone, nil
}

// probeRoomOnce checks the per-room override (allow, deny) pair for a subject
// against a specific roomID. Reads room_allow.{roomId}.{subject}.{verb}.{type}.
func (r *PermissionResolver) probeRoomOnce(ctx context.Context, kv jetstream.KeyValue, subject string, parts PermissionKeyParts, roomID string) (DecisionKind, error) {
	allowed, err := r.keyExists(ctx, kv, RoomAllowKey(roomID, subject, parts.Verb, parts.ObjectType))
	if err != nil {
		return DecisionNone, err
	}
	if allowed {
		return DecisionAllow, nil
	}
	denied, err := r.keyExists(ctx, kv, RoomDenyKey(roomID, subject, parts.Verb, parts.ObjectType))
	if err != nil {
		return DecisionNone, err
	}
	if denied {
		return DecisionDeny, nil
	}
	return DecisionNone, nil
}

// probeSetOnce checks the set-scope (allow, deny) pair for a subject against
// a specific groupID. Reads group_allow.{groupId}.{subject}.{verb}.{type}.
func (r *PermissionResolver) probeSetOnce(ctx context.Context, kv jetstream.KeyValue, subject string, parts PermissionKeyParts, groupID string) (DecisionKind, error) {
	allowed, err := r.keyExists(ctx, kv, GroupAllowKey(groupID, subject, parts.Verb, parts.ObjectType))
	if err != nil {
		return DecisionNone, err
	}
	if allowed {
		return DecisionAllow, nil
	}
	denied, err := r.keyExists(ctx, kv, GroupDenyKey(groupID, subject, parts.Verb, parts.ObjectType))
	if err != nil {
		return DecisionNone, err
	}
	if denied {
		return DecisionDeny, nil
	}
	return DecisionNone, nil
}

// HasServerPermission checks a server-only permission (no room context).
func (r *PermissionResolver) HasServerPermission(ctx context.Context, userID string, perm Permission) (bool, error) {
	if meta, known := GetPermissionMetadata(perm); known && !permissionMetadataHasScope(meta, ScopeServer) {
		return false, fmt.Errorf("permission %s does not apply at instance scope", perm)
	}
	decision, err := r.Resolve(ctx, userID, KindChannel, "", perm)
	return decision == DecisionAllow, err
}

// HasSpacePermission is a kind-aware server-scope check. KindDM triggers the
// boundary deny-list; otherwise behaves like HasServerPermission.
func (r *PermissionResolver) HasSpacePermission(ctx context.Context, userID string, kind RoomKind, perm Permission) (bool, error) {
	if meta, known := GetPermissionMetadata(perm); known {
		if !permissionMetadataHasScope(meta, ScopeServer) {
			return false, fmt.Errorf("permission %s does not apply at server scope", perm)
		}
	}
	decision, err := r.Resolve(ctx, userID, kind, "", perm)
	return decision == DecisionAllow, err
}

// HasRoomPermission checks a permission with a room context. Room-scoped
// grants/denials take precedence over server-scoped ones within the same role;
// across roles the hierarchy walk decides (see walkPermission's docstring).
func (r *PermissionResolver) HasRoomPermission(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (bool, error) {
	if !PermissionAppliesAtScope(perm, ScopeRoom) && !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeServer) {
		return false, fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	decision, err := r.Resolve(ctx, userID, kind, roomID, perm)
	return decision == DecisionAllow, err
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
// Walker (single source of truth for resolution ordering)
// ============================================================================

// walkRoles walks the role-level resolution sequence: iterate the user's
// roles in hierarchy order (highest rank first), emitting the first
// allow/deny found at room scope (if roomID is set) or server scope.
// User-level overrides are checked separately by Resolve before this
// walker runs.
//
// Resolution priority (the first emitted decision wins):
//   1. User-level overrides — checked before any role:
//      a. room-level allow / deny (only when roomID != "")
//      b. server-level allow / deny
//   2. Role-level decisions — for each role assigned to the user, sorted by
//      hierarchy (highest rank first):
//      a. room-level allow / deny (only when roomID != "")
//      b. server-level allow / deny
//
// User-level overrides "outrank" every role grant: an explicit user-deny
// blocks the action even for owners, and an explicit user-grant allows it
// even when no role grants it. This is the mechanism for "this single user
// can do X" (server-wide grant) and "this user is suspended" (server-wide
// deny) without inventing custom roles.
//
// Within a single subject (user OR a given role), room-scope decisions win
// over server-scope ones — same-subject specificity. Across roles,
// hierarchy decides: a higher-rank role's allow beats a lower-rank role's
// deny.
//
// The visit callback chooses whether to keep walking. The Has* path stops on
// the first emission; the Explain* path keeps walking to accumulate the trace.
// If no subject emits anything, the result is "no decision" — the Has*
// wrappers treat this as deny.
func (r *PermissionResolver) walkRoles(
	ctx context.Context, userID string, kind RoomKind, roomID, groupID string, perm Permission, visit visitFunc,
) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return nil
	}

	kv := r.core.storage.serverRBACEngine.KV()
	hasServerScope := PermissionAppliesAtScope(perm, ScopeServer)
	useChannelRoomPath := kind == KindChannel && roomID != "" && PermissionAppliesAtScope(perm, ScopeRoom)
	useChannelGroupPath := !useChannelRoomPath && kind == KindChannel && groupID != "" && PermissionAppliesAtScope(perm, ScopeGroup)

	rolesWithPos, err := r.getUserServerRolesWithPositions(ctx, userID)
	if err != nil {
		return err
	}
	for _, rp := range rolesWithPos {
		if useChannelRoomPath {
			// Room override
			decided, stop, err := r.probeRoom(ctx, kv, rp, parts, roomID, visit)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
			if decided {
				continue
			}

			// Group scope (only when the room is in a group)
			if groupID != "" {
				decided, stop, err := r.probeSet(ctx, kv, rp, parts, groupID, visit)
				if err != nil {
					return err
				}
				if stop {
					return nil
				}
				if decided {
					continue
				}
			}

			// Server-scope fallback for perms configurable at both group
			// and server scope (e.g. room.create).
			if hasServerScope {
				_, stop, err := r.probeServer(ctx, kv, rp, parts, visit)
				if err != nil {
					return err
				}
				if stop {
					return nil
				}
			}
			continue
		}

		if useChannelGroupPath {
			decided, stop, err := r.probeSet(ctx, kv, rp, parts, groupID, visit)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
			if decided {
				continue
			}
			if hasServerScope {
				_, stop, err := r.probeServer(ctx, kv, rp, parts, visit)
				if err != nil {
					return err
				}
				if stop {
					return nil
				}
			}
			continue
		}

		// Server-scope / DM path (legacy allow.{role}.{verb}.{type}.any).
		_, stop, err := r.probeServer(ctx, kv, rp, parts, visit)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}

	return nil
}

// probeServer emits a TraceEntry for a server-scope (allow, deny) hit on the
// given role. Used for server-scope and DM resolution paths.
func (r *PermissionResolver) probeServer(
	ctx context.Context, kv jetstream.KeyValue, rp roleWithPosition,
	parts PermissionKeyParts, visit visitFunc,
) (decided, stop bool, err error) {
	granted, err := r.keyExists(ctx, kv, AllowKey(rp.name, parts.Verb, parts.ObjectType, ObjectIdAny))
	if err != nil {
		return false, false, err
	}
	if granted {
		return true, visit(TraceEntry{Level: LevelServer, RoleName: rp.name, Decision: DecisionAllow, ObjectID: ObjectIdAny}) == visitStop, nil
	}
	denied, err := r.keyExists(ctx, kv, DenyKey(rp.name, parts.Verb, parts.ObjectType, ObjectIdAny))
	if err != nil {
		return false, false, err
	}
	if denied {
		return true, visit(TraceEntry{Level: LevelServer, RoleName: rp.name, Decision: DecisionDeny, ObjectID: ObjectIdAny}) == visitStop, nil
	}
	return false, false, nil
}

// probeRoom emits a TraceEntry for a per-room (allow, deny) hit on the given
// role. Reads room_allow.{roomId}.{subject}.{verb}.{type}.
func (r *PermissionResolver) probeRoom(
	ctx context.Context, kv jetstream.KeyValue, rp roleWithPosition,
	parts PermissionKeyParts, roomID string, visit visitFunc,
) (decided, stop bool, err error) {
	granted, err := r.keyExists(ctx, kv, RoomAllowKey(roomID, rp.name, parts.Verb, parts.ObjectType))
	if err != nil {
		return false, false, err
	}
	if granted {
		return true, visit(TraceEntry{Level: LevelRoom, RoleName: rp.name, Decision: DecisionAllow, ObjectID: roomID}) == visitStop, nil
	}
	denied, err := r.keyExists(ctx, kv, RoomDenyKey(roomID, rp.name, parts.Verb, parts.ObjectType))
	if err != nil {
		return false, false, err
	}
	if denied {
		return true, visit(TraceEntry{Level: LevelRoom, RoleName: rp.name, Decision: DecisionDeny, ObjectID: roomID}) == visitStop, nil
	}
	return false, false, nil
}

// probeSet emits a TraceEntry for a set-scope (allow, deny) hit on the given
// role. Reads group_allow.{groupId}.{subject}.{verb}.{type}.
func (r *PermissionResolver) probeSet(
	ctx context.Context, kv jetstream.KeyValue, rp roleWithPosition,
	parts PermissionKeyParts, groupID string, visit visitFunc,
) (decided, stop bool, err error) {
	granted, err := r.keyExists(ctx, kv, GroupAllowKey(groupID, rp.name, parts.Verb, parts.ObjectType))
	if err != nil {
		return false, false, err
	}
	if granted {
		return true, visit(TraceEntry{Level: LevelGroup, RoleName: rp.name, Decision: DecisionAllow, ObjectID: groupID}) == visitStop, nil
	}
	denied, err := r.keyExists(ctx, kv, GroupDenyKey(groupID, rp.name, parts.Verb, parts.ObjectType))
	if err != nil {
		return false, false, err
	}
	if denied {
		return true, visit(TraceEntry{Level: LevelGroup, RoleName: rp.name, Decision: DecisionDeny, ObjectID: groupID}) == visitStop, nil
	}
	return false, false, nil
}

// dmBoundaryDeniedPermissions are capabilities that DM rooms forbid
// unconditionally, regardless of any role grants. The deny applies to every
// role including owner. Two reasons appear in this set:
//
//   - **Privacy**: operators cannot moderate DM contents.
//   - **Category mismatch**: capabilities that semantically don't apply to
//     DMs (DMs have their own listing/creation/membership APIs).
//
// Everything else resolves through the standard hierarchy walk. Access to
// DM rooms is gated by participation at the API boundary (`requireRoomMember`
// / `dm.view`); this set only governs *what* a participant can do once
// inside, and *what* the DM space refuses to answer for channel-style
// operations.
var dmBoundaryDeniedPermissions = map[Permission]bool{
	// Privacy boundary.
	PermRoomManage:    true,
	PermMessageManage: true,
	PermMessageEcho:   true,
	// DMs have their own creation / membership APIs.
	PermRoomCreate: true,
}

func dmBoundaryDenies(perm Permission) bool {
	return dmBoundaryDeniedPermissions[perm]
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

// getUserServerRoles returns the user's roles (including implicit ones).
func (r *PermissionResolver) getUserServerRoles(ctx context.Context, userID string) ([]string, error) {
	roles, err := r.core.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user roles: %w", err)
	}

	// Always include "everyone" for authenticated users
	if !slices.Contains(roles, RoleEveryone) {
		roles = append(roles, RoleEveryone)
	}

	return roles, nil
}

// roleWithPosition pairs a role name with its position for hierarchy sorting.
type roleWithPosition struct {
	name     string
	position int32
}

// getUserServerRolesWithPositions returns the user's roles with positions, sorted by hierarchy.
func (r *PermissionResolver) getUserServerRolesWithPositions(ctx context.Context, userID string) ([]roleWithPosition, error) {
	roleNames, err := r.getUserServerRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	engine := r.core.storage.serverRBACEngine

	result := make([]roleWithPosition, 0, len(roleNames))
	for _, name := range roleNames {
		pos := PositionEveryone // Default for virtual roles or if lookup fails
		if role, err := engine.GetRole(ctx, name); err == nil && role != nil {
			pos = role.Position
		}
		result = append(result, roleWithPosition{name: name, position: pos})
	}

	// Sort by position descending (higher = higher rank = checked first).
	// Use sort.SliceStable + role name as a deterministic secondary key so
	// two roles at the same position always resolve in the same order
	// across calls. Otherwise position collisions would let the walker's
	// "first decision wins" depend on map iteration order — a real
	// security risk.
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].position != result[j].position {
			return result[i].position > result[j].position
		}
		return result[i].name < result[j].name
	})

	return result, nil
}
