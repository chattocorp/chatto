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

// authorizePermissionExplanation enforces the inspector's "self OR admin"
// access rules. Self-inspection requires membership at the requested scope so
// non-members can't probe role configurations of spaces they don't belong to.
func (r *Resolver) authorizePermissionExplanation(ctx context.Context, viewerID, targetID, spaceID, roomID string) error {
	if scopeIsInstance := spaceID == ""; scopeIsInstance {
		if viewerID == targetID {
			return nil
		}
		return r.requireInstanceAdminOrErr(ctx, viewerID)
	}

	// Space or room scope.
	if viewerID == targetID {
		isMember, err := r.core.SpaceMembershipExists(ctx, viewerID, spaceID)
		if err != nil {
			return fmt.Errorf("failed to check space membership: %w", err)
		}
		if !isMember {
			return core.ErrPermissionDenied
		}
		if roomID != "" {
			isRoomMember, err := r.core.RoomMembershipExists(ctx, spaceID, viewerID, roomID)
			if err != nil {
				return fmt.Errorf("failed to check room membership: %w", err)
			}
			if !isRoomMember {
				return core.ErrPermissionDenied
			}
		}
		return nil
	}

	// Inspecting someone else: instance admin OR space admin (roles.manage in spaceID).
	if err := r.requireInstanceAdminOrErr(ctx, viewerID); err == nil {
		return nil
	}
	hasRolesManage, err := r.core.PermResolver().HasSpacePermission(ctx, viewerID, spaceID, core.PermRoleManage)
	if err != nil {
		return fmt.Errorf("failed to check roles.manage: %w", err)
	}
	if !hasRolesManage {
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
