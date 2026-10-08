package core

import (
	"context"
	"fmt"
	"slices"
)

// AccessSummary tells who can find and join a channel room, or the rooms of a
// room group. New rooms and room groups start closed (ADR-116), so the admin
// pages show it to warn operators about rooms that nobody else can reach.
// Explicit room members, and owners in privileged mode, have access whatever
// the summary says.
type AccessSummary struct {
	// EveryoneCanList and EveryoneCanJoin report whether every member can
	// find (room.list) and join (room.join) the room.
	EveryoneCanList bool
	EveryoneCanJoin bool
	// EveryoneCanRead reports whether every member can read the room
	// (message.read). A room that everyone can join but not read is not open.
	EveryoneCanRead bool
	// RolesCanList names the named roles whose holders can find the room
	// although everyone cannot. RolesCanJoin names those whose holders can
	// join and read it although everyone cannot. Both are highest first, and
	// empty when everyone can.
	RolesCanList []string
	RolesCanJoin []string
}

// GetAccessSummary summarizes access to the channel room roomID, or to the
// rooms of the room group groupID. Pass exactly one. It requires the access
// that the role permission matrix of that room or group requires. The result
// comes from the same resolver as authorization, for a member who holds only
// the role, without settings on single users.
func (c *ChattoCore) GetAccessSummary(ctx context.Context, actorID, roomID, groupID string) (*AccessSummary, error) {
	if (roomID == "") == (groupID == "") {
		return nil, fmt.Errorf("%w: pass a room id or a group id", ErrInvalidArgument)
	}
	if roomID != "" {
		if err := c.requireCanManageRolePermissionsForRoom(ctx, actorID, roomID); err != nil {
			return nil, err
		}
		groupID = c.PermResolver().groupForRoom(ctx, KindChannel, roomID, "", PermRoomJoin)
	} else if err := c.requireCanManageRolePermissionsForGroup(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	roles, err := c.ListServerRoles(ctx)
	if err != nil {
		return nil, err
	}

	resolver := c.PermResolver()
	allowed := func(roleName string, perm Permission) bool {
		return resolver.resolveRoleHolder(roleName, KindChannel, roomID, groupID, perm) == DecisionAllow
	}
	rolesWith := func(perms ...Permission) []string {
		var names []string
		for _, role := range roles {
			if role.Name == RoleOwner || role.Name == RoleEveryone {
				continue
			}
			if !slices.ContainsFunc(perms, func(perm Permission) bool { return !allowed(role.Name, perm) }) {
				names = append(names, role.Name)
			}
		}
		return names
	}

	summary := &AccessSummary{
		EveryoneCanList: allowed(RoleEveryone, PermRoomList),
		EveryoneCanJoin: allowed(RoleEveryone, PermRoomJoin),
		EveryoneCanRead: allowed(RoleEveryone, PermMessageRead),
	}
	if !summary.EveryoneCanList {
		summary.RolesCanList = rolesWith(PermRoomList)
	}
	if !summary.EveryoneCanJoin || !summary.EveryoneCanRead {
		summary.RolesCanJoin = rolesWith(PermRoomJoin, PermMessageRead)
	}
	return summary, nil
}
