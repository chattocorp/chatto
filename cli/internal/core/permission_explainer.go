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

// ExplainInstancePermission resolves a permission at instance scope and returns
// the full decision trace. Mirrors HasInstancePermission's algorithm via the
// same walker; the bool of HasInstancePermission corresponds to State == DecisionAllow.
func (r *PermissionResolver) ExplainInstancePermission(ctx context.Context, userID string, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if meta, known := GetPermissionMetadata(perm); known && !permissionMetadataHasScope(meta, ScopeServer) {
		return exp, fmt.Errorf("permission %s does not apply at instance scope", perm)
	}

	err := r.walkInstancePermission(ctx, userID, perm, exp.collect())
	return exp, err
}

// ExplainSpacePermission resolves a permission at space scope and returns the
// full decision trace. For DM spaces the trace is synthesized from the hardcoded
// DM permission rules; for non-members of a space-scoped permission, an empty
// trace with State=DecisionNone is returned (matching HasSpacePermission's false).
func (r *PermissionResolver) ExplainSpacePermission(ctx context.Context, userID, spaceID string, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if meta, known := GetPermissionMetadata(perm); known {
		if !permissionMetadataHasScope(meta, ScopeSpace) && !permissionMetadataHasScope(meta, ScopeServer) {
			return exp, fmt.Errorf("permission %s does not apply at space scope", perm)
		}
	}

	if IsDMSpace(spaceID) {
		exp.applyDMResult(r.resolveDMPermission(perm))
		return exp, nil
	}

	err := r.walkSpacePermission(ctx, userID, spaceID, perm, exp.collect())
	return exp, err
}

// ExplainRoomPermission resolves a permission at room scope and returns the
// full decision trace.
func (r *PermissionResolver) ExplainRoomPermission(ctx context.Context, userID, spaceID, roomID string, perm Permission) (PermissionExplanation, error) {
	exp := PermissionExplanation{Permission: perm, State: DecisionNone}

	if !PermissionAppliesAtScope(perm, ScopeRoom) && !PermissionAppliesAtScope(perm, ScopeSpace) && !PermissionAppliesAtScope(perm, ScopeServer) {
		return exp, fmt.Errorf("permission %s does not apply at room scope", perm)
	}

	if IsDMSpace(spaceID) {
		exp.applyDMResult(r.resolveDMPermission(perm))
		return exp, nil
	}

	err := r.walkRoomPermission(ctx, userID, spaceID, roomID, perm, exp.collect())
	return exp, err
}

// ExplainAllPermissions returns explanations for every permission applicable at
// the given scope:
//   - userID only → instance-scoped permissions
//   - userID + spaceID → space-scoped permissions
//   - userID + spaceID + roomID → room-scoped permissions
//
// roomID without spaceID is invalid and returns an error.
func (r *PermissionResolver) ExplainAllPermissions(ctx context.Context, userID, spaceID, roomID string) ([]PermissionExplanation, error) {
	if roomID != "" && spaceID == "" {
		return nil, fmt.Errorf("roomID requires spaceID")
	}

	var scope PermissionScope
	switch {
	case roomID != "":
		scope = ScopeRoom
	case spaceID != "":
		scope = ScopeSpace
	default:
		scope = ScopeServer
	}

	metas := PermissionsForScope(scope)
	results := make([]PermissionExplanation, 0, len(metas))
	for _, meta := range metas {
		var (
			exp PermissionExplanation
			err error
		)
		switch scope {
		case ScopeServer:
			exp, err = r.ExplainInstancePermission(ctx, userID, meta.Permission)
		case ScopeSpace:
			exp, err = r.ExplainSpacePermission(ctx, userID, spaceID, meta.Permission)
		case ScopeRoom:
			exp, err = r.ExplainRoomPermission(ctx, userID, spaceID, roomID, meta.Permission)
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

// applyDMResult fills in the explanation for a DM-space permission check using
// the bool returned by resolveDMPermission. The trace is synthesized: a single
// pseudo-entry attributed to "@dm-policy" so the inspector UI can clearly
// indicate that DM rules (not RBAC) decided this.
func (exp *PermissionExplanation) applyDMResult(allowed bool) {
	decision := DecisionDeny
	if allowed {
		decision = DecisionAllow
	}
	exp.State = decision
	exp.DecidedAt = LevelInstance
	exp.DecidedByRole = "@dm-policy"
	exp.Trace = []TraceEntry{{
		Level:    LevelInstance,
		RoleName: "@dm-policy",
		Decision: decision,
	}}
}
