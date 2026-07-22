package core

import (
	"context"
	"fmt"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// PermissionResolver handles permission resolution using a deliberately small
// model:
//
//  1. Effective owners are allowed every known RBAC permission.
//  2. For everyone else, DM boundary denies win for category/privacy mismatches.
//  3. Each direct-user or explicitly assigned role contributes its nearest
//     decision (room, then group, then server). Across those decisions, any
//     deny wins; otherwise any allow grants the permission.
//  4. The implicit everyone role supplies the nearest scope baseline. A named
//     allow overrides an everyone deny only at the same or a nearer scope;
//     named denies always win.
//  5. No decision is denied at the API boundary.
//
// This makes everyone a scoped baseline rather than an absolute restriction: a
// room allow can grant access that everyone lacks in that room, while an
// unrelated server-wide role grant cannot bypass a nearer room baseline. A
// deny from another named role (for example suspended) still blocks the action.
// Scope specificity is evaluated independently for each subject, so a room
// decision replaces that subject's group/server decision for the room.
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
//  3. Resolve the nearest decision for the user and each named role. Any deny
//     beats any allow across those subjects.
//  4. Apply the implicit everyone baseline. A named allow beats an everyone
//     deny only when it is at least as specific; named denies always win.
func (r *PermissionResolver) Resolve(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (DecisionKind, error) {
	return r.resolveWithGroup(ctx, userID, kind, roomID, "", perm)
}

// ResolveGroup is like Resolve but for group-scope checks (no room context).
// Used by CanCreateRoom and other group-scoped capability gates.
func (r *PermissionResolver) ResolveGroup(ctx context.Context, userID string, kind RoomKind, groupID string, perm Permission) (DecisionKind, error) {
	return r.resolveWithGroup(ctx, userID, kind, "", groupID, perm)
}

func (r *PermissionResolver) resolveWithGroup(ctx context.Context, userID string, kind RoomKind, roomID, explicitGroupID string, perm Permission) (DecisionKind, error) {
	accountKind, ownerID, active, exists := r.core.Users.AuthorizationIdentity(userID)
	if exists && accountKind == corev1.UserKind_USER_KIND_BOT {
		if !active {
			return DecisionDeny, nil
		}
		ownerKind, _, ownerActive, ownerExists := r.core.Users.AuthorizationIdentity(ownerID)
		if !ownerExists || !ownerActive || ownerKind == corev1.UserKind_USER_KIND_BOT {
			return DecisionDeny, nil
		}
		botDecision, err := r.resolveAccountWithGroup(ctx, userID, kind, roomID, explicitGroupID, perm)
		if err != nil {
			return DecisionNone, err
		}
		ownerDecision, err := r.resolveAccountWithGroup(ctx, ownerID, kind, roomID, explicitGroupID, perm)
		if err != nil {
			return DecisionNone, err
		}
		return intersectBotAndOwnerDecisions(botDecision, ownerDecision), nil
	}
	return r.resolveAccountWithGroup(ctx, userID, kind, roomID, explicitGroupID, perm)
}

func (r *PermissionResolver) resolveAccountWithGroup(ctx context.Context, userID string, kind RoomKind, roomID, explicitGroupID string, perm Permission) (DecisionKind, error) {
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

	decisions, err := r.applicableDecisions(ctx, userID, kind, roomID, groupID, perm)
	if err != nil {
		return DecisionNone, err
	}
	result, _, _ := resolveApplicablePermissionDecisions(decisions)
	if err == nil && result == DecisionNone && kind == KindDM && dmDefaultAllows(perm) {
		result = DecisionAllow
	}
	return result, err
}

func intersectBotAndOwnerDecisions(bot, owner DecisionKind) DecisionKind {
	if bot == DecisionDeny || owner == DecisionDeny {
		return DecisionDeny
	}
	if bot == DecisionAllow && owner == DecisionAllow {
		return DecisionAllow
	}
	return DecisionNone
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
// grants/denials, group decisions, and server decisions contribute according
// to subject specificity and the everyone fallback rules above.
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

type applicablePermissionDecisions struct {
	named    []TraceEntry
	everyone *TraceEntry
}

// applicableDecisions returns at most one decision per subject: the nearest
// explicit decision at room, group, or server scope. Direct-user and named-role
// decisions are kept separate from the implicit everyone baseline because its
// scope participates differently: a named allow must be at least as specific
// as an everyone deny to override it.
func (r *PermissionResolver) applicableDecisions(
	ctx context.Context, userID string, kind RoomKind, roomID, groupID string, perm Permission,
) (applicablePermissionDecisions, error) {
	var out applicablePermissionDecisions
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return out, nil
	}

	scopes := r.applicableScopeTargets(kind, roomID, groupID, perm)
	if len(scopes) == 0 {
		return out, nil
	}
	roles, err := r.core.GetUserRoles(ctx, userID)
	if err != nil {
		return out, fmt.Errorf("failed to get user roles: %w", err)
	}
	subjects := append([]string{userID}, roles...)

	for _, subject := range subjects {
		if entry, ok := r.nearestDecision(subject, parts, scopes); ok {
			out.named = append(out.named, entry)
		}
	}

	if entry, ok := r.nearestDecision(RoleEveryone, parts, scopes); ok {
		out.everyone = &entry
	}
	return out, nil
}

func (r *PermissionResolver) nearestDecision(subject string, parts PermissionKeyParts, scopes []permissionScopeTarget) (TraceEntry, bool) {
	for _, target := range scopes {
		decision := r.decisionFor(target.scope, target.id, subject, parts)
		if decision == DecisionNone {
			continue
		}
		return TraceEntry{
			Level:    target.level,
			RoleName: subject,
			Decision: decision,
			ObjectID: target.objectID(),
		}, true
	}
	return TraceEntry{}, false
}

// resolveDecisionEntries applies deny-wins across direct-user and named-role
// decisions. It returns the winning entry so the explainer can identify the
// exact subject and scope that determined the result.
func resolveDecisionEntries(entries []TraceEntry) (DecisionKind, TraceEntry, bool) {
	var nearestAllow *TraceEntry
	for i := range entries {
		entry := entries[i]
		if entry.Decision == DecisionDeny {
			return DecisionDeny, entry, true
		}
		if entry.Decision == DecisionAllow && (nearestAllow == nil || permissionLevelSpecificity(entry.Level) > permissionLevelSpecificity(nearestAllow.Level)) {
			nearestAllow = &entries[i]
		}
	}
	if nearestAllow != nil {
		return DecisionAllow, *nearestAllow, true
	}
	return DecisionNone, TraceEntry{}, false
}

// resolveApplicablePermissionDecisions combines named subjects with the scoped
// everyone baseline. Named denies always win. A named allow can override an
// everyone deny only at the same or a nearer scope; this lets a room-specific
// role allowlist work without letting unrelated server grants bypass it.
func resolveApplicablePermissionDecisions(decisions applicablePermissionDecisions) (DecisionKind, TraceEntry, bool) {
	state, winner, decided := resolveDecisionEntries(decisions.named)
	if state == DecisionDeny {
		return state, winner, decided
	}
	if decisions.everyone == nil {
		return state, winner, decided
	}
	baseline := *decisions.everyone
	if state == DecisionNone {
		return baseline.Decision, baseline, true
	}
	if baseline.Decision == DecisionDeny && permissionLevelSpecificity(winner.Level) < permissionLevelSpecificity(baseline.Level) {
		return DecisionDeny, baseline, true
	}
	if baseline.Decision == DecisionAllow && permissionLevelSpecificity(baseline.Level) > permissionLevelSpecificity(winner.Level) {
		return DecisionAllow, baseline, true
	}
	return state, winner, true
}

func permissionLevelSpecificity(level PermissionLevel) int {
	switch level {
	case LevelRoom:
		return 3
	case LevelGroup:
		return 2
	case LevelServer:
		return 1
	default:
		return 0
	}
}

func (r *PermissionResolver) applicableScopeTargets(kind RoomKind, roomID, groupID string, perm Permission) []permissionScopeTarget {
	var targets []permissionScopeTarget
	if roomID != "" && PermissionAppliesAtScope(perm, ScopeRoom) {
		targets = append(targets, permissionScopeTarget{scope: ScopeRoom, level: LevelRoom, id: roomID})
	}
	if kind == KindChannel && groupID != "" && PermissionAppliesAtScope(perm, ScopeGroup) {
		targets = append(targets, permissionScopeTarget{scope: ScopeGroup, level: LevelGroup, id: groupID})
	}
	if PermissionAppliesAtScope(perm, ScopeServer) {
		targets = append(targets, permissionScopeTarget{scope: ScopeServer, level: LevelServer})
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
	// DMs have their own creation / membership APIs and do not support threads.
	PermRoomCreate:          true,
	PermMessagePostInThread: true,
}

func dmBoundaryDenies(perm Permission) bool {
	return dmBoundaryDeniedPermissions[perm]
}

var dmDefaultAllowedPermissions = map[Permission]bool{
	PermRoomJoin:      true,
	PermMessagePost:   true,
	PermMessageAttach: true,
	PermMessageReact:  true,
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
