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
	return r.explainInContentView(func() (PermissionExplanation, error) {
		return r.explainServerPermission(ctx, userID, perm)
	})
}

func (r *PermissionResolver) explainServerPermission(ctx context.Context, userID string, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if meta, known := GetPermissionMetadata(perm); known && !permissionMetadataHasScope(meta, ScopeServer) {
		return exp, fmt.Errorf("permission %s does not apply at server scope", perm)
	}

	err := r.collectFullTrace(ctx, userID, KindChannel, "", perm, &exp)
	return exp, err
}

// ExplainServerKindPermission is the kind-aware singleton-scope explainer used
// by the inspector UI. KindDM resolves the direct-message scope first.
func (r *PermissionResolver) ExplainServerKindPermission(ctx context.Context, userID string, kind RoomKind, perm Permission) (PermissionExplanation, error) {
	return r.explainInContentView(func() (PermissionExplanation, error) {
		return r.explainServerKindPermission(ctx, userID, kind, perm)
	})
}

func (r *PermissionResolver) explainServerKindPermission(ctx context.Context, userID string, kind RoomKind, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if meta, known := GetPermissionMetadata(perm); known {
		if kind != KindDM && !permissionMetadataHasScope(meta, ScopeServer) {
			return exp, fmt.Errorf("permission %s does not apply at server scope", perm)
		}
	}

	err := r.collectFullTrace(ctx, userID, kind, "", perm, &exp)
	return exp, err
}

// ExplainRoomPermission resolves a permission with a room context and returns
// the full decision trace.
func (r *PermissionResolver) ExplainRoomPermission(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (PermissionExplanation, error) {
	return r.explainInContentView(func() (PermissionExplanation, error) {
		return r.explainRoomPermission(ctx, userID, kind, roomID, perm)
	})
}

func (r *PermissionResolver) explainRoomPermission(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if !PermissionAppliesAtScope(perm, ScopeRoom) && !PermissionAppliesAtScope(perm, ScopeDM) && !PermissionAppliesAtScope(perm, ScopeServer) {
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
	if _, known := GetPermissionMetadata(perm); !known {
		return nil
	}
	if isBot, ownerUserID, exists := r.core.userModel.isBotAndOwner(userID); exists && isBot {
		return r.collectBotFullTrace(ctx, userID, ownerUserID, kind, roomID, perm, exp)
	}

	if _, known := GetPermissionMetadata(perm); known {
		if kind == KindDM && !PermissionAppliesAtScope(perm, ScopeDM) {
			exp.applyDMApplicabilityDeny(LevelDM)
			return nil
		}
		if r.core.isServerOwner(userID) {
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
	if kind == KindDM && !PermissionAppliesAtScope(perm, ScopeDM) {
		exp.applyDMApplicabilityDeny(LevelDM)
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
		if _, known := GetPermissionMetadata(candidate); !known {
			continue
		}
		scopes := r.applicableScopeTargets(kind, roomID, groupID, candidate)
		entry, ok := r.nearestDecision(botUserID, candidate, scopes)
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
	return nil
}

// ExplainAllPermissions returns explanations for every permission applicable at
// the given scope:
//   - userID only → server-scoped permissions
//   - userID + KindDM → direct-message permissions with Server inheritance
//   - userID + kind + roomID → room-scoped permissions
//
// roomID without kind is invalid and returns an error.
func (r *PermissionResolver) ExplainAllPermissions(ctx context.Context, userID string, kind RoomKind, roomID string) ([]PermissionExplanation, error) {
	if r.core.contentView == nil {
		return r.explainAllPermissions(ctx, userID, kind, roomID)
	}
	var explanations []PermissionExplanation
	err := r.core.contentView.Read(func(uint64) error {
		var explainErr error
		explanations, explainErr = r.explainAllPermissions(ctx, userID, kind, roomID)
		return explainErr
	})
	return explanations, err
}

func (r *PermissionResolver) explainAllPermissions(ctx context.Context, userID string, kind RoomKind, roomID string) ([]PermissionExplanation, error) {
	if roomID != "" && kind == "" {
		return nil, fmt.Errorf("roomID requires kind")
	}

	scope := ScopeServer
	if kind == KindDM {
		scope = ScopeDM
	} else if roomID != "" {
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
			exp, err = r.explainRoomPermission(ctx, userID, kind, roomID, meta.Permission)
		case kind != "":
			exp, err = r.explainServerKindPermission(ctx, userID, kind, meta.Permission)
		default:
			exp, err = r.explainServerPermission(ctx, userID, meta.Permission)
		}
		if err != nil {
			return nil, fmt.Errorf("explain %s: %w", meta.Permission, err)
		}
		results = append(results, exp)
	}

	return results, nil
}

func (r *PermissionResolver) explainInContentView(explain func() (PermissionExplanation, error)) (PermissionExplanation, error) {
	if r.core.contentView == nil {
		return explain()
	}
	var explanation PermissionExplanation
	err := r.core.contentView.Read(func(uint64) error {
		var explainErr error
		explanation, explainErr = explain()
		return explainErr
	})
	return explanation, err
}

// applyDMApplicabilityDeny explains why a permission that is outside the
// direct-message scope is denied. The synthetic trace entry shows that the DM
// applicability rule, and not an RBAC decision, produced the result.
func (exp *PermissionExplanation) applyDMApplicabilityDeny(level PermissionLevel) {
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
