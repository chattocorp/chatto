package core

import (
	"context"
	"fmt"
)

// PermissionExplanation captures the full resolution trace for a single
// permission check, including which level/role produced the winning decision.
//
// State is the overall outcome (allow/deny/none). DecidedAt and DecidedByRole
// identify the trace entry that determined State; both are zero-valued if no
// role had an explicit grant or deny.
type PermissionExplanation struct {
	Permission Permission
	// IncludedBy identifies the broader permission whose allow produced State.
	// It is empty when Permission was resolved directly.
	IncludedBy    Permission
	State         DecisionKind
	DecidedAt     PermissionLevel
	DecidedByRole string
	Trace         []TraceEntry
}

// ExplainServerPermission resolves a server-only permission (no room
// context) and returns the full decision trace.
func (r *PermissionResolver) ExplainServerPermission(ctx context.Context, userID string, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if meta, known := GetPermissionMetadata(perm); known && !permissionMetadataHasScope(meta, ScopeServer) {
		return exp, fmt.Errorf("permission %s does not apply at server scope", perm)
	}

	err := r.collectFullTrace(ctx, userID, KindChannel, "", perm, &exp)
	return exp, err
}

// ExplainServerKindPermission is the kind-aware server-scope explainer used by
// the inspector UI to apply DM boundary rules for DM-kind callers.
func (r *PermissionResolver) ExplainServerKindPermission(ctx context.Context, userID string, kind RoomKind, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if meta, known := GetPermissionMetadata(perm); known {
		if !permissionMetadataHasScope(meta, ScopeServer) {
			return exp, fmt.Errorf("permission %s does not apply at server scope", perm)
		}
	}

	err := r.collectFullTrace(ctx, userID, kind, "", perm, &exp)
	return exp, err
}

// ExplainRoomPermission resolves a permission with a room context and returns
// the full decision trace.
func (r *PermissionResolver) ExplainRoomPermission(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if !PermissionAppliesAtScope(perm, ScopeRoom) && !PermissionAppliesAtScope(perm, ScopeServer) {
		return exp, fmt.Errorf("permission %s does not apply at room scope", perm)
	}

	err := r.collectFullTrace(ctx, userID, kind, roomID, perm, &exp)
	return exp, err
}

// collectFullTrace mirrors Resolve while preserving the nearest decision for
// each direct-user/named-role subject plus the everyone baseline. The baseline
// remains visible in the trace and can win when its deny is nearer than every
// named allow.
func (r *PermissionResolver) collectFullTrace(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission, exp *PermissionExplanation) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return nil
	}
	if isBot, ownerUserID, exists := r.core.userModel.isBotAndOwner(userID); exists && isBot {
		return r.collectBotFullTrace(ctx, userID, ownerUserID, kind, roomID, perm, exp)
	}

	if _, known := GetPermissionMetadata(perm); known {
		isOwner, err := r.core.IsServerOwner(ctx, userID)
		if err != nil {
			return err
		}
		if isOwner {
			exp.State = DecisionAllow
			exp.DecidedAt = LevelServer
			exp.DecidedByRole = RoleOwner
			exp.Trace = []TraceEntry{{
				Level:    LevelServer,
				RoleName: RoleOwner,
				Decision: DecisionAllow,
				ObjectID: ObjectIdAny,
			}}
			return nil
		}
	}

	if kind == KindDM && dmBoundaryDenies(perm) {
		level := LevelServer
		if roomID != "" {
			level = LevelRoom
		}
		exp.applyDMBoundaryDeny(level)
		return nil
	}

	groupID := ""
	if kind == KindChannel && roomID != "" && PermissionAppliesAtScope(perm, ScopeRoom) {
		if room, err := r.core.GetRoom(ctx, KindChannel, roomID); err == nil && room != nil {
			groupID = room.GroupId
		}
	}
	for _, including := range includingPermissions(perm) {
		included := PermissionExplanation{Permission: including, State: DecisionNone}
		if err := r.collectFullTraceExact(ctx, userID, kind, roomID, groupID, including, &included); err != nil {
			return err
		}
		if included.State == DecisionAllow {
			exp.IncludedBy = including
			exp.State = included.State
			exp.DecidedAt = included.DecidedAt
			exp.DecidedByRole = included.DecidedByRole
			exp.Trace = included.Trace
			return nil
		}
	}
	return r.collectFullTraceExact(ctx, userID, kind, roomID, groupID, perm, exp)
}

func (r *PermissionResolver) collectBotFullTrace(ctx context.Context, botUserID, ownerUserID string, kind RoomKind, roomID string, perm Permission, exp *PermissionExplanation) error {
	if perm == PermBotCreate || perm == PermBotManage {
		exp.applyBotPolicyDeny(roomID, "@bot-policy")
		return nil
	}
	if kind == KindDM && dmBoundaryDenies(perm) {
		level := LevelServer
		if roomID != "" {
			level = LevelRoom
		}
		exp.applyDMBoundaryDeny(level)
		return nil
	}
	ownerIsBot, _, ownerExists := r.core.userModel.isBotAndOwner(ownerUserID)
	if !ownerExists || ownerIsBot {
		exp.applyBotPolicyDeny(roomID, "@bot-owner-ceiling")
		return nil
	}

	groupID := ""
	if kind == KindChannel && roomID != "" && PermissionAppliesAtScope(perm, ScopeRoom) {
		if room, err := r.core.GetRoom(ctx, KindChannel, roomID); err == nil && room != nil {
			groupID = room.GroupId
		}
	}
	delegated, sourcePermission, sourceEntry := r.botDelegatedExplanation(botUserID, kind, roomID, groupID, perm)
	if delegated != DecisionAllow {
		if sourceEntry != nil {
			exp.State = DecisionDeny
			exp.DecidedAt = sourceEntry.Level
			exp.DecidedByRole = sourceEntry.RoleName
			exp.Trace = []TraceEntry{*sourceEntry}
		} else {
			exp.applyBotPolicyDeny(roomID, "@bot-allowlist")
		}
		return nil
	}

	ownerExplanation := PermissionExplanation{Permission: perm, State: DecisionNone}
	if err := r.collectFullTrace(ctx, ownerUserID, kind, roomID, perm, &ownerExplanation); err != nil {
		return err
	}
	if ownerExplanation.State != DecisionAllow {
		exp.applyBotPolicyDeny(roomID, "@bot-owner-ceiling")
		exp.Trace = append(exp.Trace, ownerExplanation.Trace...)
		return nil
	}

	if sourcePermission != perm {
		exp.IncludedBy = sourcePermission
	} else {
		exp.IncludedBy = ownerExplanation.IncludedBy
	}
	exp.State = DecisionAllow
	exp.DecidedAt = sourceEntry.Level
	exp.DecidedByRole = sourceEntry.RoleName
	exp.Trace = append([]TraceEntry{*sourceEntry}, ownerExplanation.Trace...)
	return nil
}

func (r *PermissionResolver) botDelegatedExplanation(botUserID string, kind RoomKind, roomID, groupID string, perm Permission) (DecisionKind, Permission, *TraceEntry) {
	for _, candidate := range append(includingPermissions(perm), perm) {
		parts := candidate.KeyParts()
		if parts.Verb == "" || parts.ObjectType == "" {
			continue
		}
		scopes := r.applicableScopeTargets(kind, roomID, groupID, candidate)
		entry, ok := r.nearestDecision(botUserID, parts, scopes)
		if !ok {
			continue
		}
		if entry.Decision == DecisionAllow {
			return DecisionAllow, candidate, &entry
		}
		if candidate == perm {
			return DecisionDeny, candidate, &entry
		}
	}
	return DecisionNone, perm, nil
}

func (r *PermissionResolver) collectFullTraceExact(ctx context.Context, userID string, kind RoomKind, roomID, groupID string, perm Permission, exp *PermissionExplanation) error {
	decisions, err := r.applicableDecisions(ctx, userID, kind, roomID, groupID, perm)
	if err != nil {
		return err
	}
	exp.Trace = append(exp.Trace, decisions.named...)
	if decisions.everyone != nil {
		exp.Trace = append(exp.Trace, *decisions.everyone)
	}
	state, winner, decided := resolveApplicablePermissionDecisions(decisions)
	if decided {
		exp.State = state
		exp.DecidedAt = winner.Level
		exp.DecidedByRole = winner.RoleName
	}
	if exp.State == DecisionNone && kind == KindDM && dmDefaultAllows(perm) {
		exp.State = DecisionAllow
		exp.DecidedAt = LevelRoom
		exp.DecidedByRole = "@dm-policy"
		exp.Trace = []TraceEntry{{
			Level:    LevelRoom,
			RoleName: "@dm-policy",
			Decision: DecisionAllow,
			ObjectID: roomID,
		}}
	}
	return nil
}

// ExplainAllPermissions returns explanations for every permission applicable at
// the given scope:
//   - userID only → server-scoped permissions
//   - userID + kind → server-scoped permissions filtered through DM rules when kind == KindDM
//   - userID + kind + roomID → room-scoped permissions
//
// roomID without kind is invalid and returns an error.
func (r *PermissionResolver) ExplainAllPermissions(ctx context.Context, userID string, kind RoomKind, roomID string) ([]PermissionExplanation, error) {
	if roomID != "" && kind == "" {
		return nil, fmt.Errorf("roomID requires kind")
	}

	scope := ScopeServer
	if roomID != "" {
		scope = ScopeRoom
	}

	metas := PermissionsForScope(scope)
	results := make([]PermissionExplanation, 0, len(metas))
	for _, meta := range metas {
		var (
			exp PermissionExplanation
			err error
		)
		switch {
		case roomID != "":
			exp, err = r.ExplainRoomPermission(ctx, userID, kind, roomID, meta.Permission)
		case kind != "":
			exp, err = r.ExplainServerKindPermission(ctx, userID, kind, meta.Permission)
		default:
			exp, err = r.ExplainServerPermission(ctx, userID, meta.Permission)
		}
		if err != nil {
			return nil, fmt.Errorf("explain %s: %w", meta.Permission, err)
		}
		results = append(results, exp)
	}

	return results, nil
}

// applyDMBoundaryDeny fills in the explanation for a permission that is
// unconditionally denied by the DM privacy boundary. The trace is synthesized
// as a single pseudo-entry attributed to "@dm-policy" so the inspector UI can
// clearly indicate that DM rules (not RBAC) decided this. The level passed
// in matches the caller (LevelRoom from ExplainRoomPermission, LevelServer
// from ExplainServerKindPermission) so the inspector shows the right scope.
func (exp *PermissionExplanation) applyDMBoundaryDeny(level PermissionLevel) {
	exp.State = DecisionDeny
	exp.DecidedAt = level
	exp.DecidedByRole = "@dm-policy"
	exp.Trace = []TraceEntry{{
		Level:    level,
		RoleName: "@dm-policy",
		Decision: DecisionDeny,
	}}
}

func (exp *PermissionExplanation) applyBotPolicyDeny(roomID, marker string) {
	level := LevelServer
	if roomID != "" {
		level = LevelRoom
	}
	exp.State = DecisionDeny
	exp.DecidedAt = level
	exp.DecidedByRole = marker
	exp.Trace = []TraceEntry{{
		Level:    level,
		RoleName: marker,
		Decision: DecisionDeny,
	}}
}
