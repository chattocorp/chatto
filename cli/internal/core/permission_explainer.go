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
	Permission    Permission
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
