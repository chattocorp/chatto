package graph

// Helper methods for the rolePermissions and tierRoles resolvers. Lives
// outside the resolvers file so gqlgen's regenerator doesn't move it into a
// "code that was going to be deleted" comment block.

import (
	"context"
	"fmt"
	"sort"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// authorizeRolePermissions enforces access for both the rolePermissions and
// tierRoles queries.
//
//   - Server scope (no spaceID): requires server admin.
//   - Room scope (spaceID + roomID): passes for server admins, holders of
//     role.manage at server scope, OR holders of room.manage on the
//     specific room being inspected. The room.manage path is what lets a
//     room moderator open their own room's permission editor without
//     needing global role.manage.
func (r *Resolver) authorizeRolePermissions(ctx context.Context, viewerID, spaceID, roomID string) error {
	if spaceID == "" {
		return r.requireServerAdminOrErr(ctx, viewerID)
	}
	kind := core.RoomKindFromLegacySpaceID(spaceID)
	if err := r.requireServerAdminOrErr(ctx, viewerID); err != nil {
		hasRolesManage, hpErr := r.core.PermResolver().HasSpacePermission(ctx, viewerID, kind, core.PermRoleManage)
		if hpErr != nil {
			return fmt.Errorf("failed to check role.manage: %w", hpErr)
		}
		if !hasRolesManage {
			if roomID == "" {
				return core.ErrPermissionDenied
			}
			hasRoomManage, hpErr := r.core.PermResolver().HasRoomPermission(ctx, viewerID, kind, roomID, core.PermRoomManage)
			if hpErr != nil {
				return fmt.Errorf("failed to check room.manage: %w", hpErr)
			}
			if !hasRoomManage {
				return core.ErrPermissionDenied
			}
		}
	}
	return r.requireRoomExists(ctx, kind, roomID)
}

// buildRoleAcrossTiers gathers metadata + per-tier grants/denials for the role.
// Server tier is always populated; room tier is populated when roomID is
// non-empty.
func (r *Resolver) buildRoleAcrossTiers(
	ctx context.Context,
	roleName string,
	spaceID, roomID string,
) (*model.RoleAcrossTiers, error) {
	role, err := r.core.GetServerRole(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to load role: %w", err)
	}
	if role == nil {
		return nil, nil
	}

	out := &model.RoleAcrossTiers{
		RoleName:    roleName,
		DisplayName: role.DisplayName,
		Description: role.Description,
		IsSystem:    role.IsSystem,
		Position:    role.Position,
	}

	for _, meta := range core.PermissionsForScope(tierScope(spaceID, roomID)) {
		out.ApplicablePermissions = append(out.ApplicablePermissions, string(meta.Permission))
	}

	grants, err := r.core.GetServerRolePermissions(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to load grants: %w", err)
	}
	denials, err := r.core.GetServerRolePermissionDenials(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to load denials: %w", err)
	}
	out.Server = newTierPermissions(grants, denials)

	if roomID != "" {
		grants, denials, err := r.core.GetRoomRolePermissions(ctx, roomID, roleName)
		if err != nil {
			return nil, fmt.Errorf("failed to load room overrides: %w", err)
		}
		out.Room = newTierPermissions(grants, denials)
	}

	return out, nil
}

// buildTierRoles assembles the per-tier permission matrix: every role at the
// requested scope, with override + inherited baseline, plus the list of
// permissions configurable at this scope.
//
// groupID is non-empty for set-scope editing (ADR-031): the matrix lists every
// role with its set-scope grants/denials. Set scope has no inheritance — the
// channel-room permission system is rooted at the set, not the server.
func (r *Resolver) buildTierRoles(ctx context.Context, spaceID, roomID, groupID string) (*model.TierRoles, error) {
	scope := tierScope(spaceID, roomID)
	// Set scope shares the same applicable-permissions list as room scope —
	// they're both channel-room permissions — so route through ScopeRoom for
	// the permission-list lookup.
	if groupID != "" {
		scope = core.ScopeRoom
	}

	out := &model.TierRoles{}
	if groupID != "" {
		// Group-scope editing surfaces every permission configurable at
		// group scope. Channel-room permissions live here per ADR-031,
		// and dual-scope perms (server+group, e.g. room.create) are also
		// applicable here.
		for _, meta := range core.PermissionsForScope(core.ScopeGroup) {
			out.ApplicablePermissions = append(out.ApplicablePermissions, string(meta.Permission))
		}
	} else {
		for _, meta := range core.PermissionsForScope(scope) {
			out.ApplicablePermissions = append(out.ApplicablePermissions, string(meta.Permission))
		}
	}

	roles, err := r.core.ListServerRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	sort.SliceStable(roles, func(i, j int) bool {
		return roles[i].Position < roles[j].Position
	})
	for _, role := range roles {
		tr, err := r.buildTierRole(ctx, role, scope, spaceID, roomID, groupID)
		if err != nil {
			return nil, err
		}
		out.Roles = append(out.Roles, tr)
	}
	return out, nil
}

// buildTierRole computes the override + inherited baseline for a role at the
// requested scope.
//
//   - Server scope: the role's server-level state IS the override; nothing
//     is inherited.
//   - Group scope (groupID != ""): the override is the group's grants/denials.
//     Permissions configurable at both server and group scope (e.g.
//     room.create) show the role's server-level state through as inheritance;
//     pure channel-room permissions (rooted at the group per ADR-031) have no
//     inheritance.
//   - Room scope (roomID != "", groupID == ""): the override is the per-room
//     grants/denials. The room's group-level state shows through as
//     inheritance — this is the canonical channel-room walk (ADR-031). For
//     permissions also configurable at server scope, server-level grants are
//     folded in as a third tier.
func (r *Resolver) buildTierRole(
	ctx context.Context,
	role core.RoleWithPermissions,
	scope core.PermissionScope,
	spaceID, roomID, groupID string,
) (*model.TierRole, error) {
	out := &model.TierRole{
		RoleName:    role.Name,
		DisplayName: role.DisplayName,
		Description: role.Description,
		IsSystem:    role.IsSystem,
		Position:    role.Position,
	}

	serverGrants, err := r.core.GetServerRolePermissions(ctx, role.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to load server grants: %w", err)
	}
	serverDenials, err := r.core.GetServerRolePermissionDenials(ctx, role.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to load server denials: %w", err)
	}

	if groupID != "" {
		// Group-scope editing: the group's own grants/denials, with
		// server-tier state shown as inheritance for permissions that
		// are configurable at both tiers.
		grants, denials, err := r.core.GetGroupRolePermissions(ctx, groupID, role.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load group overrides: %w", err)
		}
		out.Override = newTierPermissions(grants, denials)
		out.InheritedAllows = filterByScope(serverGrants, core.ScopeGroup)
		out.InheritedDenials = filterByScope(serverDenials, core.ScopeGroup)
		return out, nil
	}

	switch scope {
	case core.ScopeServer:
		// The role's server-level state is the override; nothing is inherited.
		out.Override = newTierPermissions(serverGrants, serverDenials)
	case core.ScopeRoom:
		grants, denials, err := r.core.GetRoomRolePermissions(ctx, roomID, role.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load room overrides: %w", err)
		}
		out.Override = newTierPermissions(grants, denials)

		// Inherited baseline at room scope: the EFFECTIVE state the
		// resolver would resolve without a per-room override. The walker
		// does group → server for the same role, so group decisions
		// suppress server decisions per permission. mergeInheritedDecisions
		// implements exactly that override-vs-parent shape.
		roomsGroupID, err := r.lookupRoomGroupID(ctx, roomID)
		if err != nil {
			return nil, err
		}
		var groupGrants, groupDenials []core.Permission
		if roomsGroupID != "" {
			groupGrants, groupDenials, err = r.core.GetGroupRolePermissions(ctx, roomsGroupID, role.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to load group overrides for inheritance: %w", err)
			}
		}
		// Filter server perms to those applicable at room scope so we
		// don't fold server-only perms (admin.*, user.*, etc.) into a
		// room-scope baseline.
		filteredServerGrants := scopedPerms(serverGrants, core.ScopeRoom)
		filteredServerDenials := scopedPerms(serverDenials, core.ScopeRoom)
		out.InheritedAllows, out.InheritedDenials = mergeInheritedDecisions(
			groupGrants, groupDenials,
			filteredServerGrants, filteredServerDenials,
		)
	}

	if out.Override == nil {
		out.Override = &model.TierPermissions{}
	}
	return out, nil
}

// lookupRoomGroupID fetches the groupID for a channel room, or "" if the
// room doesn't have one assigned (transitional pre-migration state).
func (r *Resolver) lookupRoomGroupID(ctx context.Context, roomID string) (string, error) {
	if roomID == "" {
		return "", nil
	}
	room, err := r.core.GetRoom(ctx, core.KindChannel, roomID)
	if err != nil {
		return "", fmt.Errorf("failed to load room for inheritance lookup: %w", err)
	}
	if room == nil {
		return "", nil
	}
	return room.GroupId, nil
}

// filterByScope returns the subset of perms applicable at the given scope, as
// strings. Used to surface server-tier state as inheritance into a more
// specific tier.
func filterByScope(perms []core.Permission, scope core.PermissionScope) []string {
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		if core.PermissionAppliesAtScope(p, scope) {
			out = append(out, string(p))
		}
	}
	return out
}

// scopedPerms returns the subset of perms applicable at the given scope.
// Like filterByScope but keeps the typed Permission slice instead of strings —
// used when the result feeds back into another helper that expects
// []core.Permission.
func scopedPerms(perms []core.Permission, scope core.PermissionScope) []core.Permission {
	out := make([]core.Permission, 0, len(perms))
	for _, p := range perms {
		if core.PermissionAppliesAtScope(p, scope) {
			out = append(out, p)
		}
	}
	return out
}

// mergeInheritedDecisions resolves the effective allow/deny baseline for a
// single role across two tiers (override tier + parent tier). Per permission
// the override tier wins: an entry on the override tier's allow or deny list
// suppresses the parent tier's entries.
func mergeInheritedDecisions(overrideAllow, overrideDeny, parentAllow, parentDeny []core.Permission) ([]string, []string) {
	overridden := make(map[core.Permission]struct{}, len(overrideAllow)+len(overrideDeny))
	for _, p := range overrideAllow {
		overridden[p] = struct{}{}
	}
	for _, p := range overrideDeny {
		overridden[p] = struct{}{}
	}

	allow := make([]string, 0, len(overrideAllow)+len(parentAllow))
	for _, p := range overrideAllow {
		allow = append(allow, string(p))
	}
	for _, p := range parentAllow {
		if _, blocked := overridden[p]; blocked {
			continue
		}
		allow = append(allow, string(p))
	}

	deny := make([]string, 0, len(overrideDeny)+len(parentDeny))
	for _, p := range overrideDeny {
		deny = append(deny, string(p))
	}
	for _, p := range parentDeny {
		if _, blocked := overridden[p]; blocked {
			continue
		}
		deny = append(deny, string(p))
	}
	return allow, deny
}

func tierScope(spaceID, roomID string) core.PermissionScope {
	if roomID != "" {
		return core.ScopeRoom
	}
	_ = spaceID
	return core.ScopeServer
}

func newTierPermissions(grants, denials []core.Permission) *model.TierPermissions {
	out := &model.TierPermissions{
		Permissions:       make([]string, len(grants)),
		PermissionDenials: make([]string, len(denials)),
	}
	for i, g := range grants {
		out.Permissions[i] = string(g)
	}
	for i, d := range denials {
		out.PermissionDenials[i] = string(d)
	}
	return out
}

func permsToStrings(perms []core.Permission) []string {
	out := make([]string, len(perms))
	for i, p := range perms {
		out[i] = string(p)
	}
	return out
}
