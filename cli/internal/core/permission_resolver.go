package core

import (
	"context"
	"fmt"
	"slices"
)

// PermissionResolver handles permission resolution using a deliberately small
// model:
//
//  1. Effective owners are allowed every known RBAC permission.
//  2. For everyone else, DM boundary denies win for category/privacy mismatches.
//  3. For everyone else, any applicable deny wins.
//  4. If there is no deny, any applicable allow grants the permission.
//  5. No decision is denied at the API boundary.
//
// Applicable decisions include user-level overrides and all roles assigned to
// the user, including the implicit everyone role. For room checks, server,
// group, and room scopes can all contribute. Server-scope message/room
// decisions therefore act as global defaults/overrides, while room/group
// decisions are local exceptions.
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
// Only explicit projection-backed decisions are emitted (allow or deny);
// roles with no decision at the level being checked are silent.
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

// visitFunc is invoked once per explicit allow/deny decision. The first
// invocation corresponds to the entry the bool path would short-circuit on;
// the explain path keeps walking and records every entry.
type visitFunc func(entry TraceEntry) visitOutcome

// Resolve is the single resolver entry point. Returns the effective decision
// (allow / deny / none) for the user-permission pair. Both the bool authorizer
// (Has*Permission) and the inspector go through this — there is no parallel
// implementation.
//
// Order of operations:
//
//  1. Effective-owner override.
//  2. DM boundary deny-list (for kind == KindDM only) — permissions in
//     dmBoundaryDeniedPermissions are unconditionally denied regardless of
//     grants for non-owners. This is the privacy/category-mismatch floor.
//  3. Collect applicable user and role decisions across all valid scopes.
//     Any deny beats any allow; any allow beats no decision.
func (r *PermissionResolver) Resolve(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (DecisionKind, error) {
	return r.resolveWithGroup(ctx, userID, kind, roomID, "", perm)
}

// ResolveGroup is like Resolve but for group-scope checks (no room context).
// Used by CanCreateRoom and other group-scoped capability gates.
func (r *PermissionResolver) ResolveGroup(ctx context.Context, userID string, kind RoomKind, groupID string, perm Permission) (DecisionKind, error) {
	return r.resolveWithGroup(ctx, userID, kind, "", groupID, perm)
}

func (r *PermissionResolver) resolveWithGroup(ctx context.Context, userID string, kind RoomKind, roomID, explicitGroupID string, perm Permission) (DecisionKind, error) {
	if _, known := GetPermissionMetadata(perm); known {
		isOwner, err := r.core.IsServerOwner(ctx, userID)
		if err != nil {
			return DecisionNone, err
		}
		if isOwner {
			return DecisionAllow, nil
		}
	}

	if kind == KindDM && dmBoundaryDenies(perm) {
		return DecisionDeny, nil
	}

	// For channel rooms with a room-scope permission, look up the room's group
	// once so the decision collector can include group-scope decisions.
	groupID := explicitGroupID
	if kind == KindChannel && roomID != "" && PermissionAppliesAtScope(perm, ScopeRoom) && groupID == "" {
		if room, err := r.core.GetRoom(ctx, KindChannel, roomID); err == nil && room != nil {
			groupID = room.GroupId
		}
	}

	result := DecisionNone
	err := r.visitApplicableDecisions(ctx, userID, kind, roomID, groupID, perm, func(entry TraceEntry) visitOutcome {
		if entry.Decision == DecisionDeny {
			result = DecisionDeny
			return visitStop
		}
		if result == DecisionNone {
			result = DecisionAllow
		}
		return visitContinue
	})
	if err == nil && result == DecisionNone && kind == KindDM && dmDefaultAllows(perm) {
		result = DecisionAllow
	}
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
	hasServerScope := PermissionAppliesAtScope(perm, ScopeServer)

	if kind == KindChannel && roomID != "" && PermissionAppliesAtScope(perm, ScopeRoom) {
		got, err := r.probeRoomOnce(ctx, userID, parts, roomID)
		if err != nil {
			return DecisionNone, err
		}
		if got != DecisionNone {
			return got, nil
		}
		if groupID != "" {
			got, err := r.probeSetOnce(ctx, userID, parts, groupID)
			if err != nil {
				return DecisionNone, err
			}
			if got != DecisionNone {
				return got, nil
			}
		}
		if hasServerScope {
			return r.probeServerOnce(ctx, userID, parts)
		}
		return DecisionNone, nil
	}

	if kind == KindChannel && groupID != "" && PermissionAppliesAtScope(perm, ScopeGroup) {
		got, err := r.probeSetOnce(ctx, userID, parts, groupID)
		if err != nil {
			return DecisionNone, err
		}
		if got != DecisionNone {
			return got, nil
		}
		if hasServerScope {
			return r.probeServerOnce(ctx, userID, parts)
		}
		return DecisionNone, nil
	}

	return r.probeServerOnce(ctx, userID, parts)
}

// probeServerOnce checks the server-scope decision for a subject. Used for
// server-scope checks and DM rooms.
func (r *PermissionResolver) probeServerOnce(_ context.Context, subject string, parts PermissionKeyParts) (DecisionKind, error) {
	return r.decisionFor(ScopeServer, "", subject, parts), nil
}

// probeRoomOnce checks the per-room decision for a subject against a specific
// roomID.
func (r *PermissionResolver) probeRoomOnce(_ context.Context, subject string, parts PermissionKeyParts, roomID string) (DecisionKind, error) {
	return r.decisionFor(ScopeRoom, roomID, subject, parts), nil
}

// probeSetOnce checks the set-scope decision for a subject against a specific
// groupID.
func (r *PermissionResolver) probeSetOnce(_ context.Context, subject string, parts PermissionKeyParts, groupID string) (DecisionKind, error) {
	return r.decisionFor(ScopeGroup, groupID, subject, parts), nil
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
// grants/denials, group decisions, and server decisions all contribute; any
// applicable deny wins for non-owners.
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
// Decision Collector (single source of truth for resolution inputs)
// ============================================================================

type permissionScopeTarget struct {
	scope PermissionScope
	level PermissionLevel
	id    string
}

// visitApplicableDecisions emits every projection-backed decision that can
// affect this permission check. The caller decides how to combine those
// decisions; Resolve uses deny-wins, Explain records the full trace.
func (r *PermissionResolver) visitApplicableDecisions(
	ctx context.Context, userID string, kind RoomKind, roomID, groupID string, perm Permission, visit visitFunc,
) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return nil
	}

	scopes := r.applicableScopeTargets(kind, roomID, groupID, perm)
	if len(scopes) == 0 {
		return nil
	}
	roles, err := r.getUserServerRoles(ctx, userID)
	if err != nil {
		return err
	}
	subjects := append([]string{userID}, roles...)

	for _, subject := range subjects {
		for _, target := range scopes {
			switch r.decisionFor(target.scope, target.id, subject, parts) {
			case DecisionAllow:
				if visit(TraceEntry{Level: target.level, RoleName: subject, Decision: DecisionAllow, ObjectID: target.objectID()}) == visitStop {
					return nil
				}
			case DecisionDeny:
				if visit(TraceEntry{Level: target.level, RoleName: subject, Decision: DecisionDeny, ObjectID: target.objectID()}) == visitStop {
					return nil
				}
			}
		}
	}

	return nil
}

func (r *PermissionResolver) applicableScopeTargets(kind RoomKind, roomID, groupID string, perm Permission) []permissionScopeTarget {
	var targets []permissionScopeTarget
	if PermissionAppliesAtScope(perm, ScopeServer) {
		targets = append(targets, permissionScopeTarget{scope: ScopeServer, level: LevelServer})
	}
	if kind == KindChannel && groupID != "" && PermissionAppliesAtScope(perm, ScopeGroup) {
		targets = append(targets, permissionScopeTarget{scope: ScopeGroup, level: LevelGroup, id: groupID})
	}
	if roomID != "" && PermissionAppliesAtScope(perm, ScopeRoom) {
		targets = append(targets, permissionScopeTarget{scope: ScopeRoom, level: LevelRoom, id: roomID})
	}
	return targets
}

func (t permissionScopeTarget) objectID() string {
	if t.id == "" {
		return ObjectIdAny
	}
	return t.id
}

// dmBoundaryDeniedPermissions are capabilities that DM rooms forbid for
// non-owners, regardless of any role grants. Two reasons appear in this set:
//
//   - **Privacy**: operators cannot moderate DM contents.
//   - **Category mismatch**: capabilities that semantically don't apply to
//     DMs (DMs have their own listing/creation/membership APIs).
//
// Everything else resolves through the standard deny-wins resolver. Access to
// DM rooms is gated by participation at the API boundary (`requireRoomMember`);
// this set only governs *what* a participant can do once inside, and *what*
// DM rooms refuse to answer for channel-style operations.
var dmBoundaryDeniedPermissions = map[Permission]bool{
	// Privacy boundary.
	PermRoomManage:    true,
	PermRoomMemberBan: true,
	PermMessageManage: true,
	PermMessageEcho:   true,
	// DMs have their own creation / membership APIs.
	PermRoomCreate: true,
}

func dmBoundaryDenies(perm Permission) bool {
	return dmBoundaryDeniedPermissions[perm]
}

var dmDefaultAllowedPermissions = map[Permission]bool{
	PermRoomJoin:            true,
	PermMessagePost:         true,
	PermMessagePostInThread: true,
	PermMessageAttach:       true,
	PermMessageReact:        true,
}

func dmDefaultAllows(perm Permission) bool {
	return dmDefaultAllowedPermissions[perm]
}

// ============================================================================
// Helper Methods
// ============================================================================

// decisionFor returns the current projection-backed RBAC decision for a
// subject at a specific scope.
func (r *PermissionResolver) decisionFor(scope PermissionScope, scopeID, subject string, parts PermissionKeyParts) DecisionKind {
	if subject == "" || parts.Verb == "" || parts.ObjectType == "" {
		return DecisionNone
	}
	perm := ReconstructPermission(parts.Verb, parts.ObjectType)
	if perm == "" {
		return DecisionNone
	}
	return r.core.RBAC.GetDecision(scope, scopeID, subject, perm)
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
