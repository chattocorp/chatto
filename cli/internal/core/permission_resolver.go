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

// PermissionResolver resolves a user's permissions through one consistent
// rule, regardless of the tier being checked:
//
//	Walk the user's applicable roles in hierarchy order (highest rank first).
//	For each role, check its KV at the current tier; if no decision, walk
//	upward to the parent tier (room → space → instance). The first role with
//	any decision wins. If no role decides anywhere, the answer is deny.
//
// In practice this means:
//   - Higher-rank roles always override lower-rank roles (Discord-style).
//   - A role's decisions cascade downward by default — a grant at instance
//     scope is visible at every space and room until something at a lower
//     tier overrides it.
//   - "Suspended" works as a high-rank role with explicit denies; nothing
//     else overrides it because nothing has higher rank.
//
// Roles ranked across tiers: instance roles and space roles are sorted by
// position number; ties go to instance roles (the more global authority).
// Space roles don't apply at instance tier — their walk ends at the space
// tier.
//
// The walker emits TraceEntry events to a visitFunc. The bool path
// (HasXxxPermission) returns visitStop on the first emit, so it short-circuits.
// The explainer (ExplainXxxPermission) returns visitContinue so it accumulates
// the full trace.
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

// TraceEntry is one step in the permission resolution trace. Only entries
// actually backed by a KV value are emitted (allow or deny); roles with no
// KV entry at the level being checked are silent.
type TraceEntry struct {
	Level    PermissionLevel
	RoleName string
	Decision DecisionKind // Allow or Deny only
	ObjectID string       // "any" for instance/space scope; roomID for room overrides
}

// visitOutcome controls walker iteration.
type visitOutcome int

const (
	visitContinue visitOutcome = iota
	visitStop
)

// visitFunc is invoked once per "found" allow/deny KV entry. The first
// invocation corresponds to the entry the bool path would short-circuit on;
// the explain path keeps walking and records every entry.
type visitFunc func(entry TraceEntry) visitOutcome

// applicableRole pairs a role with everything the walker needs to resolve
// it: name, sort key (position), and a callback that opens the KV bucket
// for a given tier (or returns nil if the role doesn't apply at that tier).
type applicableRole struct {
	name        string
	position    int32
	isInstance  bool
	// kvForLevel returns the KV bucket the role's decision would live in for
	// a given tier, or nil if this role can't be configured at that tier.
	kvForLevel func(level PermissionLevel) jetstream.KeyValue
}

// HasInstancePermission checks if a user has a permission at the instance level.
func (r *PermissionResolver) HasInstancePermission(ctx context.Context, userID string, perm Permission) (bool, error) {
	var result bool
	err := r.walk(ctx, userID, "", "", perm, LevelInstance, func(entry TraceEntry) visitOutcome {
		result = entry.Decision == DecisionAllow
		return visitStop
	})
	return result, err
}

// HasSpacePermission checks if a user has a permission at the space level.
//
// For space-scoped permissions (like room.create), the user must be a space
// member. space.join and space.list are exempt (non-members need them for
// discovery).
func (r *PermissionResolver) HasSpacePermission(ctx context.Context, userID, spaceID string, perm Permission) (bool, error) {
	if IsDMSpace(spaceID) {
		return r.resolveDMPermission(perm), nil
	}
	if PermissionAppliesAtScope(perm, ScopeSpace) && perm != PermSpaceJoin && perm != PermSpaceList {
		isMember, err := r.core.SpaceMembershipExists(ctx, userID, spaceID)
		if err != nil {
			return false, fmt.Errorf("failed to check space membership: %w", err)
		}
		if !isMember {
			return false, nil
		}
	}

	var result bool
	err := r.walk(ctx, userID, spaceID, "", perm, LevelSpace, func(entry TraceEntry) visitOutcome {
		result = entry.Decision == DecisionAllow
		return visitStop
	})
	return result, err
}

// HasRoomPermission checks if a user has a permission at the room level.
func (r *PermissionResolver) HasRoomPermission(ctx context.Context, userID, spaceID, roomID string, perm Permission) (bool, error) {
	if IsDMSpace(spaceID) {
		return r.resolveDMPermission(perm), nil
	}

	var result bool
	err := r.walk(ctx, userID, spaceID, roomID, perm, LevelRoom, func(entry TraceEntry) visitOutcome {
		result = entry.Decision == DecisionAllow
		return visitStop
	})
	return result, err
}

// walk is the single source of truth for permission resolution. It assembles
// the user's applicable roles, sorts them by hierarchy, and for each role
// walks the tier chain from `startLevel` upward looking for an explicit
// decision. The first role with any decision wins (the visitor stops the
// walk).
func (r *PermissionResolver) walk(
	ctx context.Context,
	userID, spaceID, roomID string,
	perm Permission,
	startLevel PermissionLevel,
	visit visitFunc,
) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return nil
	}

	roles, err := r.applicableRoles(ctx, userID, spaceID, startLevel)
	if err != nil {
		return err
	}

	for _, role := range roles {
		stop, err := r.walkRoleUpward(ctx, role, parts, startLevel, roomID, visit)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return nil
}

// walkRoleUpward walks a single role's tier chain from startLevel toward the
// root (instance), emitting on the first tier where the role has an explicit
// decision. Returns (true, nil) if the visitor signaled stop.
func (r *PermissionResolver) walkRoleUpward(
	ctx context.Context,
	role applicableRole,
	parts PermissionKeyParts,
	startLevel PermissionLevel,
	roomID string,
	visit visitFunc,
) (bool, error) {
	for _, level := range tiersFrom(startLevel) {
		kv := role.kvForLevel(level)
		if kv == nil {
			// Role doesn't apply at this tier (e.g. space role at instance scope).
			continue
		}

		// At room scope the KV key uses the roomID as object ID so per-room
		// overrides don't collide with the role's space-level grant. At all
		// other tiers the role's grant is keyed by ObjectIdAny.
		objectID := rbac.ObjectIdAny
		if level == LevelRoom {
			objectID = roomID
		}

		granted, err := r.keyExists(ctx, kv, rbac.AllowKey(role.name, parts.Verb, parts.ObjectType, objectID))
		if err != nil {
			return false, err
		}
		if granted {
			r.core.logger.Debug("Permission granted", "role", role.name, "position", role.position, "permission", parts.Verb+"."+parts.ObjectType, "level", level)
			return visit(TraceEntry{Level: level, RoleName: role.name, Decision: DecisionAllow, ObjectID: objectID}) == visitStop, nil
		}

		denied, err := r.keyExists(ctx, kv, rbac.DenyKey(role.name, parts.Verb, parts.ObjectType, objectID))
		if err != nil {
			return false, err
		}
		if denied {
			r.core.logger.Debug("Permission denied", "role", role.name, "position", role.position, "permission", parts.Verb+"."+parts.ObjectType, "level", level)
			return visit(TraceEntry{Level: level, RoleName: role.name, Decision: DecisionDeny, ObjectID: objectID}) == visitStop, nil
		}
	}
	return false, nil
}

// tiersFrom returns the tier chain to walk for a given starting level,
// going from the requested tier upward toward instance.
func tiersFrom(start PermissionLevel) []PermissionLevel {
	switch start {
	case LevelRoom:
		return []PermissionLevel{LevelRoom, LevelSpace, LevelInstance}
	case LevelSpace:
		return []PermissionLevel{LevelSpace, LevelInstance}
	default:
		return []PermissionLevel{LevelInstance}
	}
}

// applicableRoles returns the user's roles that participate in resolution at
// the requested scope, sorted by hierarchy: lowest position first; ties go
// to instance roles. Each entry knows how to resolve its own KV per tier.
func (r *PermissionResolver) applicableRoles(
	ctx context.Context,
	userID, spaceID string,
	startLevel PermissionLevel,
) ([]applicableRole, error) {
	instanceRoleNames, err := r.getUserInstanceRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	instanceEngine := r.core.instanceRBACEngine
	instanceKV := instanceEngine.KV()

	// Space KV is loaded lazily — only needed when the walk touches space/room.
	var spaceKV jetstream.KeyValue
	getSpaceKV := func() (jetstream.KeyValue, error) {
		if spaceKV != nil || spaceID == "" || startLevel == LevelInstance {
			return spaceKV, nil
		}
		kv, err := r.core.getSpaceRBACKV(ctx, spaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get space RBAC KV: %w", err)
		}
		spaceKV = kv
		return kv, nil
	}

	roles := make([]applicableRole, 0, len(instanceRoleNames)+4)

	// Instance roles. Resolved by the instance engine for instance-tier
	// lookups; their space and room overrides live in the space KV (under
	// the same role name — the engine uses the role string as the key prefix).
	for _, name := range instanceRoleNames {
		pos := rbac.PositionEveryone
		if role, err := instanceEngine.GetRole(ctx, name); err == nil && role != nil {
			pos = role.Position
		}
		// Capture the name in a local for the closure.
		roleName := name
		roles = append(roles, applicableRole{
			name:       roleName,
			position:   pos,
			isInstance: true,
			kvForLevel: func(level PermissionLevel) jetstream.KeyValue {
				switch level {
				case LevelInstance:
					return instanceKV
				case LevelSpace, LevelRoom:
					kv, err := getSpaceKV()
					if err != nil || kv == nil {
						return nil
					}
					return kv
				}
				return nil
			},
		})
	}

	// Space roles only participate at space and room tiers.
	if startLevel != LevelInstance && spaceID != "" {
		spaceRoleNames, err := r.getUserSpaceRoles(ctx, spaceID, userID)
		if err != nil {
			return nil, err
		}
		spaceEngine, err := r.core.spaceRBACEngine(ctx, spaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get space RBAC engine: %w", err)
		}
		// Don't double-walk roles that share a name with an instance role
		// (e.g. the universal "everyone" role). The instance entry already
		// looks them up in the space KV via getSpaceKV().
		instanceSet := make(map[string]struct{}, len(instanceRoleNames))
		for _, n := range instanceRoleNames {
			instanceSet[n] = struct{}{}
		}
		for _, name := range spaceRoleNames {
			if _, ok := instanceSet[name]; ok {
				continue
			}
			pos := rbac.PositionEveryone
			if role, err := spaceEngine.GetRole(ctx, name); err == nil && role != nil {
				pos = role.Position
			}
			roleName := name
			roles = append(roles, applicableRole{
				name:       roleName,
				position:   pos,
				isInstance: false,
				kvForLevel: func(level PermissionLevel) jetstream.KeyValue {
					switch level {
					case LevelSpace, LevelRoom:
						kv, err := getSpaceKV()
						if err != nil || kv == nil {
							return nil
						}
						return kv
					}
					return nil // space roles don't apply at instance tier
				},
			})
		}
	}

	// Stable sort by (position asc, isInstance desc) — instance roles win ties.
	sort.SliceStable(roles, func(i, j int) bool {
		if roles[i].position != roles[j].position {
			return roles[i].position < roles[j].position
		}
		return roles[i].isInstance && !roles[j].isInstance
	})

	return roles, nil
}

// resolveDMPermission returns whether a permission is allowed in DM context.
// DM space uses simplified permissions — only certain actions are allowed.
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
// Helpers
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

// getUserInstanceRoles returns the user's instance roles (including implicit
// "everyone").
func (r *PermissionResolver) getUserInstanceRoles(ctx context.Context, userID string) ([]string, error) {
	roles, err := r.core.GetUserInstanceRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user instance roles: %w", err)
	}
	if !slices.Contains(roles, InstRoleEveryone) {
		roles = append(roles, InstRoleEveryone)
	}
	return roles, nil
}

// getUserSpaceRoles returns the user's space roles.
func (r *PermissionResolver) getUserSpaceRoles(ctx context.Context, spaceID, userID string) ([]string, error) {
	roles, err := r.core.GetUserRoles(ctx, spaceID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user space roles: %w", err)
	}
	return roles, nil
}
