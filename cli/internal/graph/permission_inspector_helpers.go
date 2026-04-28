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
// inspector. Instance scope requires instance admin; space and room scopes
// require role.manage in spaceID or instance admin. There is no
// self-inspection path — the inspector is an admin tool.
//
// At space scope, the target user must be a member of spaceID — querying
// arbitrary instance users would let a space admin probe membership status
// of users outside their space. At room scope, roomID must belong to spaceID.
func (r *Resolver) authorizePermissionExplanation(ctx context.Context, viewerID, targetID, spaceID, roomID string) error {
	if spaceID == "" {
		return r.requireInstanceAdminOrErr(ctx, viewerID)
	}
	if err := r.requireInstanceAdminOrErr(ctx, viewerID); err != nil {
		hasRolesManage, hpErr := r.core.PermResolver().HasSpacePermission(ctx, viewerID, spaceID, core.PermRoleManage)
		if hpErr != nil {
			return fmt.Errorf("failed to check role.manage: %w", hpErr)
		}
		if !hasRolesManage {
			return core.ErrPermissionDenied
		}
	}
	if err := r.requireRoomBelongsToSpace(ctx, spaceID, roomID); err != nil {
		return err
	}
	return r.requireSpaceMembership(ctx, targetID, spaceID)
}

// requireRoomBelongsToSpace returns nil if roomID is empty or if the room
// exists in spaceID's CONFIG bucket. Otherwise returns ErrPermissionDenied.
// We map the "room not found" error to a permission error rather than a
// 404-shaped error to avoid letting callers probe for room existence in
// spaces they shouldn't be querying.
func (r *Resolver) requireRoomBelongsToSpace(ctx context.Context, spaceID, roomID string) error {
	if roomID == "" {
		return nil
	}
	room, err := r.core.GetRoom(ctx, spaceID, roomID)
	if err != nil || room == nil {
		return core.ErrPermissionDenied
	}
	return nil
}

// requireSpaceMembership returns nil if userID is a member of spaceID. The
// inspector exposes "what role-derived permissions does this user have in
// this space" — that's only meaningful for members, and accepting arbitrary
// userIDs here would let a space admin enumerate non-membership across the
// instance via empty traces.
func (r *Resolver) requireSpaceMembership(ctx context.Context, userID, spaceID string) error {
	isMember, err := r.core.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		return fmt.Errorf("failed to check space membership: %w", err)
	}
	if !isMember {
		return core.ErrPermissionDenied
	}
	return nil
}

// requireInstanceAdminOrErr returns nil if the viewer is an instance admin
// (config-based, owner role, or admin role), otherwise core.ErrPermissionDenied.
func (r *Resolver) requireInstanceAdminOrErr(ctx context.Context, viewerID string) error {
	isAdmin, err := r.isInstanceAdmin(ctx, viewerID)
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
	case core.LevelInstance:
		return model.PermissionLevelInstance
	case core.LevelSpace:
		return model.PermissionLevelSpace
	case core.LevelRoom:
		return model.PermissionLevelRoom
	default:
		return model.PermissionLevelInstance
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
