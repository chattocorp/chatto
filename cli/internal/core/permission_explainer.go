package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core/rbac"
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

// ExplainInstancePermission resolves a server-only permission (no room
// context) and returns the full decision trace.
func (r *PermissionResolver) ExplainInstancePermission(ctx context.Context, userID string, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if meta, known := GetPermissionMetadata(perm); known && !permissionMetadataHasScope(meta, ScopeServer) {
		return exp, fmt.Errorf("permission %s does not apply at instance scope", perm)
	}

	err := r.collectFullTrace(ctx, userID, "", perm, &exp)
	return exp, err
}

// ExplainSpacePermission is the legacy server-scope explainer kept for the
// inspector UI until callers migrate to ExplainInstancePermission.
func (r *PermissionResolver) ExplainSpacePermission(ctx context.Context, userID string, kind RoomKind, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if meta, known := GetPermissionMetadata(perm); known {
		if !permissionMetadataHasScope(meta, ScopeServer) {
			return exp, fmt.Errorf("permission %s does not apply at server scope", perm)
		}
	}

	if kind == KindDM && dmBoundaryDenies(perm) {
		exp.applyDMBoundaryDeny(LevelInstance)
		return exp, nil
	}

	err := r.collectFullTrace(ctx, userID, "", perm, &exp)
	return exp, err
}

// ExplainRoomPermission resolves a permission with a room context and returns
// the full decision trace.
func (r *PermissionResolver) ExplainRoomPermission(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if !PermissionAppliesAtScope(perm, ScopeRoom) && !PermissionAppliesAtScope(perm, ScopeServer) {
		return exp, fmt.Errorf("permission %s does not apply at room scope", perm)
	}

	if kind == KindDM && dmBoundaryDenies(perm) {
		exp.applyDMBoundaryDeny(LevelRoom)
		return exp, nil
	}

	err := r.collectFullTrace(ctx, userID, roomID, perm, &exp)
	return exp, err
}

// collectFullTrace populates the explanation by walking both the user-level
// probes and the role hierarchy. Mirrors Resolve's resolution order but
// records every encountered entry so the inspector can show the full trace.
func (r *PermissionResolver) collectFullTrace(ctx context.Context, userID, roomID string, perm Permission, exp *PermissionExplanation) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return nil
	}
	kv := r.core.storage.serverRBACEngine.KV()
	roomScoped := roomID != "" && PermissionAppliesAtScope(perm, ScopeRoom)
	visit := exp.collect()

	// User-level probes (room then server).
	userSubj := roleWithPosition{name: userID, position: 0}
	if roomScoped {
		if _, _, err := r.probe(ctx, kv, userSubj, parts, roomID, LevelRoom, visit); err != nil {
			return err
		}
	}
	if _, _, err := r.probe(ctx, kv, userSubj, parts, rbac.ObjectIdAny, LevelInstance, visit); err != nil {
		return err
	}

	// Role hierarchy walk.
	return r.walkRoles(ctx, userID, roomID, perm, visit)
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
			exp, err = r.ExplainSpacePermission(ctx, userID, kind, meta.Permission)
		default:
			exp, err = r.ExplainInstancePermission(ctx, userID, meta.Permission)
		}
		if err != nil {
			return nil, fmt.Errorf("explain %s: %w", meta.Permission, err)
		}
		results = append(results, exp)
	}

	return results, nil
}

// collect returns a visitFunc that appends every visited entry to the
// explanation's trace and captures the first entry as the winning decision.
func (exp *PermissionExplanation) collect() visitFunc {
	return func(entry TraceEntry) visitOutcome {
		if exp.State == DecisionNone {
			exp.State = entry.Decision
			exp.DecidedAt = entry.Level
			exp.DecidedByRole = entry.RoleName
		}
		exp.Trace = append(exp.Trace, entry)
		return visitContinue
	}
}

// applyDMBoundaryDeny fills in the explanation for a permission that is
// unconditionally denied by the DM privacy boundary. The trace is synthesized
// as a single pseudo-entry attributed to "@dm-policy" so the inspector UI can
// clearly indicate that DM rules (not RBAC) decided this. The level passed
// in matches the caller (LevelRoom from ExplainRoomPermission, LevelInstance
// from ExplainSpacePermission) so the inspector shows the right scope.
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
