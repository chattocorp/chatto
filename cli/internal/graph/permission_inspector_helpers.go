package graph

// Helper methods for the permission inspector and role-permissions resolvers.
// These live outside permission_inspector.resolvers.go so gqlgen's resolver
// regenerator doesn't move them into "code that was going to be deleted"
// comment blocks.

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// authorizePermissionExplanation enforces admin-only access for the
// inspector. Instance scope requires instance admin; room scope requires
// role.manage or instance admin. There is no self-inspection path — the
// inspector is an admin tool.
//
// At room scope, roomID must exist in the corresponding CONFIG bucket.
func (r *Resolver) authorizePermissionExplanation(ctx context.Context, viewerID, targetID string, kind core.RoomKind, roomID string) error {
	if kind == "" {
		return r.requireServerAdminOrErr(ctx, viewerID)
	}
	if err := r.requireServerAdminOrErr(ctx, viewerID); err != nil {
		hasRolesManage, hpErr := r.core.PermResolver().HasSpacePermission(ctx, viewerID, kind, core.PermRoleManage)
		if hpErr != nil {
			return fmt.Errorf("failed to check role.manage: %w", hpErr)
		}
		if !hasRolesManage {
			return core.ErrPermissionDenied
		}
	}
	return r.requireRoomExists(ctx, kind, roomID)
}

// requireRoomExists returns nil if roomID is empty or if the room exists in
// the kind's CONFIG bucket. Otherwise returns ErrPermissionDenied — we map
// "room not found" to a permission error rather than a 404 to avoid letting
// callers probe for room existence.
func (r *Resolver) requireRoomExists(ctx context.Context, kind core.RoomKind, roomID string) error {
	if roomID == "" {
		return nil
	}
	room, err := r.core.GetRoom(ctx, kind, roomID)
	if err != nil || room == nil {
		return core.ErrPermissionDenied
	}
	return nil
}

// requireServerAdminOrErr returns nil if the viewer is an instance admin
// (config-based, owner role, or admin role), otherwise core.ErrPermissionDenied.
func (r *Resolver) requireServerAdminOrErr(ctx context.Context, viewerID string) error {
	isAdmin, err := r.isServerAdmin(ctx, viewerID)
	if err != nil {
		return fmt.Errorf("failed to check instance admin: %w", err)
	}
	if !isAdmin {
		return core.ErrPermissionDenied
	}
	return nil
}

// toModelExplanation converts a core PermissionExplanation into the GraphQL model.
// The first trace entry is marked Applied=true because that's the winning decision
// (matches DecidedAt / DecidedByRole on the outer struct).
func toModelExplanation(exp core.PermissionExplanation) *model.PermissionExplanation {
	out := &model.PermissionExplanation{
		Permission: string(exp.Permission),
		State:      toModelDecision(exp.State),
	}
	if exp.State != core.DecisionNone {
		level := toModelLevel(exp.DecidedAt)
		out.DecidedAt = &level
		role := exp.DecidedByRole
		out.DecidedByRole = &role
	}
	out.Trace = make([]*model.PermissionTraceEntry, 0, len(exp.Trace))
	for i, entry := range exp.Trace {
		out.Trace = append(out.Trace, &model.PermissionTraceEntry{
			Level:    toModelLevel(entry.Level),
			RoleName: entry.RoleName,
			Decision: toModelDecision(entry.Decision),
			Applied:  i == 0,
		})
	}
	return out
}

func toModelLevel(l core.PermissionLevel) model.PermissionLevel {
	switch l {
	case core.LevelServer:
		return model.PermissionLevelServer
	case core.LevelGroup:
		return model.PermissionLevelGroup
	case core.LevelRoom:
		return model.PermissionLevelRoom
	default:
		return model.PermissionLevelServer
	}
}

func toModelDecision(d core.DecisionKind) model.PermissionDecisionKind {
	switch d {
	case core.DecisionAllow:
		return model.PermissionDecisionKindAllow
	case core.DecisionDeny:
		return model.PermissionDecisionKindDeny
	default:
		return model.PermissionDecisionKindNone
	}
}
