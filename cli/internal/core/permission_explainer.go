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
	return r.explainInContentView(ctx, func(readCtx context.Context) (PermissionExplanation, error) {
		return r.explainServerPermission(readCtx, userID, perm)
	})
}

func (r *PermissionResolver) explainServerPermission(ctx context.Context, userID string, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if meta, known := GetPermissionMetadata(perm); known && !permissionMetadataHasScope(meta, ScopeServer) {
		return exp, fmt.Errorf("permission %s does not apply at server scope", perm)
	}

	return r.explain(ctx, userID, KindChannel, "", "", perm, privilegeRequest)
}

func (r *PermissionResolver) explainServerKindPermission(ctx context.Context, userID string, kind RoomKind, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if meta, known := GetPermissionMetadata(perm); known {
		if kind != KindDM && !permissionMetadataHasScope(meta, ScopeServer) {
			return exp, fmt.Errorf("permission %s does not apply at server scope", perm)
		}
	}

	return r.explain(ctx, userID, kind, "", "", perm, privilegeRequest)
}

// ExplainRoomPermission resolves a permission with a room context and returns
// the full decision trace.
func (r *PermissionResolver) ExplainRoomPermission(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (PermissionExplanation, error) {
	return r.explainInContentView(ctx, func(readCtx context.Context) (PermissionExplanation, error) {
		return r.explainRoomPermission(readCtx, userID, kind, roomID, perm)
	})
}

func (r *PermissionResolver) explainRoomPermission(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if !PermissionAppliesAtScope(perm, ScopeRoom) && !PermissionAppliesAtScope(perm, ScopeDM) && !PermissionAppliesAtScope(perm, ScopeServer) {
		return exp, fmt.Errorf("permission %s does not apply at room scope", perm)
	}

	return r.explain(ctx, userID, kind, roomID, "", perm, privilegeRequest)
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
	err := r.core.ReadServerContentView(ctx, func(readCtx context.Context, _ uint64) error {
		var explainErr error
		explanations, explainErr = r.explainAllPermissions(readCtx, userID, kind, roomID)
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

func (r *PermissionResolver) explainInContentView(ctx context.Context, explain func(context.Context) (PermissionExplanation, error)) (PermissionExplanation, error) {
	if r.core.contentView == nil {
		return explain(ctx)
	}
	var explanation PermissionExplanation
	err := r.core.ReadServerContentView(ctx, func(readCtx context.Context, _ uint64) error {
		var explainErr error
		explanation, explainErr = explain(readCtx)
		return explainErr
	})
	return explanation, err
}

// allowAsOwner explains the effective-owner override.
func (exp *PermissionExplanation) allowAsOwner() {
	exp.State = DecisionAllow
	exp.DecidedAt = LevelServer
	exp.DecidedByRole = RoleOwner
	exp.Trace = []TraceEntry{{Level: LevelServer, RoleName: RoleOwner, Decision: DecisionAllow, ObjectID: ObjectIdAny}}
}

// applyPrivilegedModeDeny explains why an allowed elevation-required
// permission is denied: privileged mode is not active. The trace keeps the
// decisions that allowed it, followed by the synthetic @privileged-mode entry.
func (exp *PermissionExplanation) applyPrivilegedModeDeny() {
	exp.State = DecisionDeny
	exp.DecidedByRole = "@privileged-mode"
	exp.Trace = append(exp.Trace, TraceEntry{Level: exp.DecidedAt, RoleName: "@privileged-mode", Decision: DecisionDeny})
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
