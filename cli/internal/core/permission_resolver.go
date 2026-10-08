package core

import (
	"context"
	"fmt"
	"slices"
	"time"

	"hmans.de/chatto/internal/authctx"
)

// PermissionResolver resolves permissions. explain holds the rules (ADR-116):
// the owner override in privileged mode, DM applicability, inclusion, a deny
// on the user, allows of the user and named roles against the everyone
// baseline, and the privileged-mode gate. Every authorization check,
// explanation, and permission matrix goes through these rules.
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
	LevelDM     PermissionLevel = "dm"
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

// Resolve returns the effective decision (allow / deny / none) for the
// user-permission pair under the request in ctx. It and every other check go
// through explain, which also produces the explanation for the admin UI; there
// is no parallel implementation.
func (r *PermissionResolver) Resolve(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (DecisionKind, error) {
	return r.resolveInContentView(ctx, func(readCtx context.Context) (DecisionKind, error) {
		return r.resolveWithGroup(readCtx, userID, kind, roomID, "", perm)
	})
}

// ResolveGroup is like Resolve but for group-scope checks (no room context).
// Used by CanCreateRoom and other group-scoped capability gates.
func (r *PermissionResolver) ResolveGroup(ctx context.Context, userID string, kind RoomKind, groupID string, perm Permission) (DecisionKind, error) {
	return r.resolveInContentView(ctx, func(readCtx context.Context) (DecisionKind, error) {
		return r.resolveWithGroup(readCtx, userID, kind, "", groupID, perm)
	})
}

func (r *PermissionResolver) resolveInContentView(ctx context.Context, resolve func(context.Context) (DecisionKind, error)) (DecisionKind, error) {
	if r.core.contentView == nil {
		return resolve(ctx)
	}
	decision := DecisionNone
	err := r.core.ReadServerContentView(ctx, func(readCtx context.Context, _ uint64) error {
		var resolveErr error
		decision, resolveErr = resolve(readCtx)
		return resolveErr
	})
	return decision, err
}

// resolveWithGroup resolves effective authorization for the request in ctx.
func (r *PermissionResolver) resolveWithGroup(ctx context.Context, userID string, kind RoomKind, roomID, explicitGroupID string, perm Permission) (DecisionKind, error) {
	exp, err := r.explain(ctx, userID, kind, roomID, explicitGroupID, perm, privilegeRequest)
	return exp.State, err
}

// privilegedModeEvaluationKey carries a fixed privileged-mode state for work
// that is not bound to one request credential.
type privilegedModeEvaluationKey struct{}

// privilegedModeEvaluation is the fixed privileged-mode state of one human.
// Checks for any other user resolve as inactive.
type privilegedModeEvaluation struct {
	userID string
	active bool
}

// withPrivilegedModeEvaluation fixes the privileged-mode state that effective
// authorization uses for userID. Realtime fan-out uses it to evaluate one
// class of sessions with the same state as their request credentials. It
// takes precedence over a request credential in ctx.
func withPrivilegedModeEvaluation(ctx context.Context, userID string, active bool) context.Context {
	return context.WithValue(ctx, privilegedModeEvaluationKey{}, privilegedModeEvaluation{userID: userID, active: active})
}

// PrivilegedModeDeadline returns the end of the active privileged mode of the
// human credential in ctx when it authenticates userID. It returns zero when
// the mode is inactive, ctx has no human credential, or the credential belongs
// to another user. Work that outlives the request, such as a call connection,
// stores this deadline to keep the same state.
func PrivilegedModeDeadline(ctx context.Context, userID string) time.Time {
	credential, ok := authctx.CredentialForContext(ctx)
	if !ok || credential.Kind == authctx.RuntimeCredentialKindBotAPIKey || credential.UserID != userID || !time.Now().Before(credential.PrivilegedModeExpiresAt) {
		return time.Time{}
	}
	return credential.PrivilegedModeExpiresAt
}

// privilegedModeAllows reports whether privileged mode is active for userID.
// A fixed evaluation state wins. Otherwise, internal work without a credential
// keeps entitlement semantics. An authenticated request resolves checks for a
// different user as inactive. A human request uses its credential's
// activation deadline; bot API keys do not use privileged mode.
func privilegedModeAllows(ctx context.Context, userID string, now time.Time) bool {
	if evaluation, ok := ctx.Value(privilegedModeEvaluationKey{}).(privilegedModeEvaluation); ok {
		return evaluation.active && evaluation.userID == userID
	}
	credential, authenticated := authctx.CredentialForContext(ctx)
	if !authenticated {
		return true
	}
	if credential.UserID != userID {
		return false
	}
	if credential.Kind == authctx.RuntimeCredentialKindBotAPIKey {
		return true
	}
	return now.Before(credential.PrivilegedModeExpiresAt)
}

// resolveEntitlement reads assigned authority in a consistent content view,
// without applying the inspecting human's session activation state.
func (r *PermissionResolver) resolveEntitlement(ctx context.Context, userID string, kind RoomKind, roomID, groupID string, perm Permission) (DecisionKind, error) {
	return r.resolveInContentView(ctx, func(readCtx context.Context) (DecisionKind, error) {
		return r.resolveEntitlementWithGroup(readCtx, userID, kind, roomID, groupID, perm)
	})
}

// resolveEntitlementWithGroup resolves durable RBAC entitlement without the
// human-session privileged-mode gate. Use it only for permission discovery,
// delegation ceilings, and other checks that must describe assigned authority.
func (r *PermissionResolver) resolveEntitlementWithGroup(ctx context.Context, userID string, kind RoomKind, roomID, explicitGroupID string, perm Permission) (DecisionKind, error) {
	exp, err := r.explain(ctx, userID, kind, roomID, explicitGroupID, perm, privilegeEntitled)
	return exp.State, err
}

// privilegeMode selects how resolution treats privileged mode (ADR-105).
type privilegeMode int

const (
	// privilegeRequest uses the privileged-mode state of the account in ctx
	// (privilegedModeAllows).
	privilegeRequest privilegeMode = iota
	// privilegeEntitled describes assigned authority: the owner override
	// applies, and elevation-required permissions are not gated.
	privilegeEntitled
	// privilegeInactive evaluates the account as if privileged mode were off,
	// independent of the request in ctx.
	privilegeInactive
	// privilegeActive evaluates the account as if privileged mode were on,
	// independent of the request in ctx.
	privilegeActive
)

// resolveForAccount resolves userID with a fixed privileged-mode state,
// independent of the request in ctx. Admin screens use it to show an
// account's own access rather than the viewer's.
func (r *PermissionResolver) resolveForAccount(ctx context.Context, userID string, kind RoomKind, roomID, groupID string, perm Permission, privileged bool) (DecisionKind, error) {
	mode := privilegeInactive
	if privileged {
		mode = privilegeActive
	}
	return r.resolveInContentView(ctx, func(readCtx context.Context) (DecisionKind, error) {
		exp, err := r.explain(readCtx, userID, kind, roomID, groupID, perm, mode)
		return exp.State, err
	})
}

// explain is the single permission resolver. It returns the effective decision
// for userID and the trace that produced it. Resolve, the entitlement checks,
// the explainer, and the permission matrices all use it.
//
// Order of operations for humans:
//
//  1. Unknown permissions have no decision.
//  2. Permissions that do not apply at the direct-message scope are denied in
//     direct messages.
//  3. Effective owners are allowed everything while privileged mode is active,
//     and in entitlement checks.
//  4. An allow of an including permission allows the included permission.
//  5. A deny on the user decides. Otherwise, an allow of the user or a role
//     at the same scope as everyone's nearest setting, or a more specific
//     one, allows. Otherwise, everyone's nearest setting decides. No setting
//     means no access. Roles only grant: stored role denies and a stored
//     server-scope deny of everyone have no effect (ADR-116).
//  6. An allow of an elevation-required permission needs active privileged
//     mode, except in entitlement checks.
//
// Bots resolve only their own allowlist, capped by their owner's entitlement
// (FDR-038).
func (r *PermissionResolver) explain(ctx context.Context, userID string, kind RoomKind, roomID, groupID string, perm Permission, mode privilegeMode) (PermissionExplanation, error) {
	if isBot, ownerUserID, exists := r.core.userModel.isBotAndOwner(userID); exists && isBot {
		return r.explainBot(ctx, userID, ownerUserID, kind, roomID, groupID, perm)
	}
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}
	metadata, known := GetPermissionMetadata(perm)
	if !known {
		return exp, nil
	}
	if kind == KindDM && !PermissionAppliesAtScope(perm, ScopeDM) {
		exp.applyDMApplicabilityDeny(LevelDM)
		return exp, nil
	}
	active := mode == privilegeEntitled || mode == privilegeActive ||
		(mode == privilegeRequest && privilegedModeAllows(ctx, userID, time.Now()))
	if active && r.core.isServerOwner(userID) {
		exp.allowAsOwner()
		return exp, nil
	}
	groupID = r.groupForRoom(ctx, kind, roomID, groupID, perm)
	if err := r.explainHumanDecisions(ctx, userID, kind, roomID, groupID, perm, &exp); err != nil {
		return exp, err
	}
	if exp.State == DecisionAllow && metadata.RequiresPrivilegedMode && mode != privilegeEntitled && !active {
		exp.applyPrivilegedModeDeny()
	}
	return exp, nil
}

// explainHumanDecisions applies inclusion and the subject rules of explain to
// the stored decisions of a human.
func (r *PermissionResolver) explainHumanDecisions(ctx context.Context, userID string, kind RoomKind, roomID, groupID string, perm Permission, exp *PermissionExplanation) error {
	roles, err := r.core.GetUserRoles(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user roles: %w", err)
	}
	r.explainSubjectDecisions(userID, roles, kind, roomID, groupID, perm, exp)
	return nil
}

// explainSubjectDecisions applies inclusion and the subject rules of explain
// to the settings of userID (empty for none), the named roles, and everyone.
func (r *PermissionResolver) explainSubjectDecisions(userID string, roles []string, kind RoomKind, roomID, groupID string, perm Permission, exp *PermissionExplanation) {
	for _, candidate := range append(includingPermissions(perm), perm) {
		if _, known := GetPermissionMetadata(candidate); !known {
			continue
		}
		scopes := r.applicableScopeTargets(kind, roomID, groupID, candidate)
		decisions := collectApplicableDecisions(func(scope PermissionScope, scopeID, subject string) DecisionKind {
			return r.decisionFor(scope, scopeID, subject, candidate)
		}, userID, roles, scopes)
		state, winner, decided := resolveApplicablePermissionDecisions(decisions)
		if candidate != perm && state != DecisionAllow {
			continue
		}
		exp.Trace = decisions.trace()
		if candidate != perm {
			exp.IncludedBy = candidate
		}
		if decided {
			exp.State, exp.DecidedAt, exp.DecidedByRole = state, winner.Level, winner.RoleName
		}
		return
	}
}

// explainBot resolves a bot: its own allowlist, where an allow of an including
// permission counts, capped by its owner's entitlement. Bots never receive
// roles, everyone, owner, or DM-default grants.
func (r *PermissionResolver) explainBot(ctx context.Context, botUserID, ownerUserID string, kind RoomKind, roomID, groupID string, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}
	if !botPermissionDelegable(perm) {
		exp.applyBotPolicyDeny(roomID, "@bot-policy")
		return exp, nil
	}
	if kind == KindDM && !PermissionAppliesAtScope(perm, ScopeDM) {
		exp.applyDMApplicabilityDeny(LevelDM)
		return exp, nil
	}
	ownerIsBot, _, ownerExists := r.core.userModel.isBotAndOwner(ownerUserID)
	if !ownerExists || ownerIsBot {
		exp.applyBotPolicyDeny(roomID, "@bot-owner-ceiling")
		return exp, nil
	}
	groupID = r.groupForRoom(ctx, kind, roomID, groupID, perm)

	var source *TraceEntry
	sourcePermission := perm
	for _, candidate := range append(includingPermissions(perm), perm) {
		if _, known := GetPermissionMetadata(candidate); !known {
			continue
		}
		entry, ok := r.nearestDecision(botUserID, candidate, r.applicableScopeTargets(kind, roomID, groupID, candidate))
		if !ok {
			continue
		}
		if entry.Decision == DecisionAllow {
			source, sourcePermission = &entry, candidate
			break
		}
		if candidate == perm {
			exp.State, exp.DecidedAt, exp.DecidedByRole = DecisionDeny, entry.Level, entry.RoleName
			exp.Trace = []TraceEntry{entry}
			return exp, nil
		}
	}
	if source == nil {
		exp.applyBotPolicyDeny(roomID, "@bot-allowlist")
		return exp, nil
	}

	// The owner ceiling uses entitlement, independent of any human session.
	owner, err := r.explain(ctx, ownerUserID, kind, roomID, groupID, perm, privilegeEntitled)
	if err != nil {
		return exp, err
	}
	if owner.State != DecisionAllow {
		exp.applyBotPolicyDeny(roomID, "@bot-owner-ceiling")
		exp.Trace = append(exp.Trace, owner.Trace...)
		return exp, nil
	}
	if sourcePermission != perm {
		exp.IncludedBy = sourcePermission
	} else {
		exp.IncludedBy = owner.IncludedBy
	}
	exp.State, exp.DecidedAt, exp.DecidedByRole = DecisionAllow, source.Level, source.RoleName
	exp.Trace = append([]TraceEntry{*source}, owner.Trace...)
	return exp, nil
}

// resolveRoleHolder returns what a human member who holds only roleName gets:
// the role's allows combined with the everyone baseline, with inclusion, and
// without settings on the member, the owner override, or the privileged-mode
// gate. For everyone, it is the baseline alone. Role grids in the admin UI use
// it, so they show the same result as authorization (ADR-116).
func (r *PermissionResolver) resolveRoleHolder(roleName string, kind RoomKind, roomID, groupID string, perm Permission) DecisionKind {
	if _, known := GetPermissionMetadata(perm); !known {
		return DecisionNone
	}
	if kind == KindDM && !PermissionAppliesAtScope(perm, ScopeDM) {
		return DecisionDeny
	}
	var roles []string
	if roleName != RoleEveryone {
		roles = []string{roleName}
	}
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}
	r.explainSubjectDecisions("", roles, kind, roomID, groupID, perm, &exp)
	return exp.State
}

// groupForRoom returns groupID, or the group of a channel room when the
// permission applies at room scope and no group was given.
func (r *PermissionResolver) groupForRoom(ctx context.Context, kind RoomKind, roomID, groupID string, perm Permission) string {
	if groupID != "" || kind != KindChannel || roomID == "" || !PermissionAppliesAtScope(perm, ScopeRoom) {
		return groupID
	}
	if room, err := r.core.GetRoom(ctx, KindChannel, roomID); err == nil && room != nil {
		return room.GroupId
	}
	return groupID
}

// HasServerPermission checks a server-only permission (no room context).
func (r *PermissionResolver) HasServerPermission(ctx context.Context, userID string, perm Permission) (bool, error) {
	if meta, known := GetPermissionMetadata(perm); known && !permissionMetadataHasScope(meta, ScopeServer) {
		return false, fmt.Errorf("permission %s does not apply at instance scope", perm)
	}
	decision, err := r.Resolve(ctx, userID, KindChannel, "", perm)
	return decision == DecisionAllow, err
}

// HasSpacePermission is a kind-aware singleton-scope check. KindDM resolves
// the direct-message scope before the inherited server scope.
func (r *PermissionResolver) HasSpacePermission(ctx context.Context, userID string, kind RoomKind, perm Permission) (bool, error) {
	if meta, known := GetPermissionMetadata(perm); known {
		if kind != KindDM && !permissionMetadataHasScope(meta, ScopeServer) {
			return false, fmt.Errorf("permission %s does not apply at server scope", perm)
		}
	}
	decision, err := r.Resolve(ctx, userID, kind, "", perm)
	return decision == DecisionAllow, err
}

// HasRoomPermission checks a permission with a room context. The room kind
// selects either the direct-message chain or the channel room and group chain.
func (r *PermissionResolver) HasRoomPermission(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (bool, error) {
	if !PermissionAppliesAtScope(perm, ScopeRoom) && !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeDM) && !PermissionAppliesAtScope(perm, ScopeServer) {
		return false, fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	decision, err := r.Resolve(ctx, userID, kind, roomID, perm)
	return decision == DecisionAllow, err
}

// permissionMetadataHasScope checks if a permission applies at the given scope.
func permissionMetadataHasScope(meta PermissionMetadata, scope PermissionScope) bool {
	return slices.Contains(meta.Scopes, scope)
}

// ============================================================================
// Decision Collector (single source of truth for resolution inputs)
// ============================================================================

type permissionScopeTarget struct {
	scope PermissionScope
	level PermissionLevel
	id    string
}

// applicablePermissionDecisions holds the inputs of one resolution: the
// nearest setting of the user, the nearest allow of each named role, and the
// nearest setting of everyone.
type applicablePermissionDecisions struct {
	user     *TraceEntry
	roles    []TraceEntry
	everyone *TraceEntry
}

// trace lists the collected decisions: the user, then the roles, then everyone.
func (d applicablePermissionDecisions) trace() []TraceEntry {
	var out []TraceEntry
	if d.user != nil {
		out = append(out, *d.user)
	}
	out = append(out, d.roles...)
	if d.everyone != nil {
		out = append(out, *d.everyone)
	}
	return out
}

// collectApplicableDecisions collects the decisions that resolution uses from
// decision, a lookup of stored decisions for one permission. scopes lists the
// applicable scopes, most specific first. Named-role denies and a server-scope
// deny of everyone are skipped (ADR-116).
func collectApplicableDecisions(decision func(scope PermissionScope, scopeID, subject string) DecisionKind, userID string, roles []string, scopes []permissionScopeTarget) applicablePermissionDecisions {
	nearest := func(subject string, allowOnly bool) (TraceEntry, bool) {
		if subject == "" {
			return TraceEntry{}, false
		}
		for _, target := range scopes {
			found := decision(target.scope, target.id, subject)
			if found == DecisionNone || (allowOnly && found != DecisionAllow) {
				continue
			}
			// A stored deny of everyone at server scope means the same as no
			// setting (ADR-116).
			if subject == RoleEveryone && found == DecisionDeny && target.scope == ScopeServer {
				continue
			}
			return TraceEntry{Level: target.level, RoleName: subject, Decision: found, ObjectID: target.objectID()}, true
		}
		return TraceEntry{}, false
	}
	var out applicablePermissionDecisions
	if entry, ok := nearest(userID, false); ok {
		out.user = &entry
	}
	for _, role := range roles {
		if role == RoleEveryone {
			continue
		}
		if entry, ok := nearest(role, true); ok {
			out.roles = append(out.roles, entry)
		}
	}
	if entry, ok := nearest(RoleEveryone, false); ok {
		out.everyone = &entry
	}
	return out
}

func (r *PermissionResolver) nearestDecision(subject string, perm Permission, scopes []permissionScopeTarget) (TraceEntry, bool) {
	for _, target := range scopes {
		decision := r.decisionFor(target.scope, target.id, subject, perm)
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

// resolveApplicablePermissionDecisions applies the subject rules: a deny on
// the user decides. Otherwise, the most specific allow of the user or a role
// wins when it is at the same scope as everyone's setting or a more specific
// one. Otherwise, everyone's setting decides. It returns the winning entry.
//
// A user allow follows the same scope rule as a role allow, so a setting on
// one user cannot open a room that denies everyone at a nearer scope.
func resolveApplicablePermissionDecisions(decisions applicablePermissionDecisions) (DecisionKind, TraceEntry, bool) {
	if decisions.user != nil && decisions.user.Decision == DecisionDeny {
		return DecisionDeny, *decisions.user, true
	}
	var allow *TraceEntry
	if decisions.user != nil {
		allow = decisions.user
	}
	for i := range decisions.roles {
		if allow == nil || permissionLevelSpecificity(decisions.roles[i].Level) > permissionLevelSpecificity(allow.Level) {
			allow = &decisions.roles[i]
		}
	}
	everyone := decisions.everyone
	if allow != nil && (everyone == nil || permissionLevelSpecificity(allow.Level) >= permissionLevelSpecificity(everyone.Level)) {
		return DecisionAllow, *allow, true
	}
	if everyone != nil {
		return everyone.Decision, *everyone, true
	}
	return DecisionNone, TraceEntry{}, false
}

func permissionLevelSpecificity(level PermissionLevel) int {
	switch level {
	case LevelRoom:
		return 3
	case LevelDM:
		return 2
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
	if kind == KindDM && PermissionAppliesAtScope(perm, ScopeDM) {
		targets = append(targets, permissionScopeTarget{scope: ScopeDM, level: LevelDM})
	} else if roomID != "" && PermissionAppliesAtScope(perm, ScopeRoom) {
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

// ============================================================================
// Helper Methods
// ============================================================================

// decisionFor returns the current projection-backed RBAC decision for a
// subject at a specific scope.
func (r *PermissionResolver) decisionFor(scope PermissionScope, scopeID, subject string, perm Permission) DecisionKind {
	if subject == "" {
		return DecisionNone
	}
	if _, known := GetPermissionMetadata(perm); !known {
		return DecisionNone
	}
	return r.core.rbacModel.decision(scope, scopeID, subject, perm)
}
