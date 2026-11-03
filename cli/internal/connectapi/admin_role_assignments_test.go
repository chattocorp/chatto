package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	"github.com/stretchr/testify/require"
)

func TestAdminUserRoleAssignmentsRequireOnlyAssignmentAuthority(t *testing.T) {
	env := newConnectAPITestEnv(t)
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "assignment-target", "Target", "password123")
	require.NoError(t, err)
	require.NoError(t, env.core.AddVerifiedEmailDirect(env.ctx, target.Id, "target@example.com"))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleAssign))
	ctx := withCaller(env.ctx, env.viewer)
	request := connect.NewRequest(&adminv1.GetUserRoleAssignmentsRequest{UserId: target.Id})
	_, err = env.adminUsers.GetUserRoleAssignments(env.ctx, request)
	require.Equal(t, connect.CodeUnauthenticated, errorCode(err))
	_, err = env.adminUsers.GetMember(ctx, connect.NewRequest(&adminv1.GetMemberRequest{Target: &adminv1.GetMemberRequest_UserId{UserId: target.Id}}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))

	// This role grants no new authority; a role.assign-only delegate may use it.
	_, err = env.core.CreateServerRole(env.ctx, core.SystemActorID, "helper", "Helper", "", false)
	require.NoError(t, err)
	state, err := env.adminUsers.GetUserRoleAssignments(ctx, request)
	require.NoError(t, err)
	require.Equal(t, target.Id, state.Msg.GetAssignments().GetUserId())
	require.Empty(t, state.Msg.GetAssignments().GetRoleNames())
	require.Contains(t, state.Msg.GetAssignments().GetAssignableRoleNames(), "helper")
	require.NotContains(t, state.Msg.GetAssignments().GetAssignableRoleNames(), core.RoleModerator)
	require.NotContains(t, state.Msg.GetAssignments().GetAssignableRoleNames(), core.RoleOwner)
	require.NotContains(t, state.Msg.GetAssignments().GetAssignableRoleNames(), core.RoleEveryone)
	_, err = env.adminUsers.AssignRole(ctx, connect.NewRequest(&adminv1.AssignRoleRequest{UserId: target.Id, RoleName: "helper"}))
	require.NoError(t, err)
	state, err = env.adminUsers.GetUserRoleAssignments(ctx, request)
	require.NoError(t, err)
	require.Equal(t, []string{"helper"}, state.Msg.GetAssignments().GetRoleNames())
	require.Contains(t, state.Msg.GetAssignments().GetRevocableRoleNames(), "helper")

	// Scoped ceilings and deny removal must use the mutation policy, not only
	// the actor's server-level permission list.
	groups, err := env.core.ListRoomGroupsOrdered(env.ctx, core.KindChannel)
	require.NoError(t, err)
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, groups[0].GetId(), "assignment-room", "")
	require.NoError(t, err)
	require.NoError(t, env.core.GrantRoomPermission(env.ctx, core.SystemActorID, room.Id, "helper", core.PermMessageManage))
	state, err = env.adminUsers.GetUserRoleAssignments(ctx, request)
	require.NoError(t, err)
	require.NotContains(t, state.Msg.GetAssignments().GetAssignableRoleNames(), "helper")
	require.NotContains(t, state.Msg.GetAssignments().GetRevocableRoleNames(), "helper")
	require.NoError(t, env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermMessageManage))
	state, err = env.adminUsers.GetUserRoleAssignments(ctx, request)
	require.NoError(t, err)
	require.Contains(t, state.Msg.GetAssignments().GetAssignableRoleNames(), "helper")
	require.Contains(t, state.Msg.GetAssignments().GetRevocableRoleNames(), "helper")
	require.NoError(t, env.core.DenyServerPermission(env.ctx, core.SystemActorID, "helper", core.PermRoomCreate))
	state, err = env.adminUsers.GetUserRoleAssignments(ctx, request)
	require.NoError(t, err)
	require.Contains(t, state.Msg.GetAssignments().GetAssignableRoleNames(), "helper")
	require.NotContains(t, state.Msg.GetAssignments().GetRevocableRoleNames(), "helper")

	// Every request rechecks authority, including after a grant is lost/regained.
	require.NoError(t, env.core.ClearUserPermissionState(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleAssign))
	_, err = env.adminUsers.GetUserRoleAssignments(ctx, connect.NewRequest(&adminv1.GetUserRoleAssignmentsRequest{UserId: "missing"}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleAssign))
	_, err = env.adminUsers.GetUserRoleAssignments(ctx, request)
	require.NoError(t, err)
	_, err = env.adminUsers.GetUserRoleAssignments(ctx, connect.NewRequest(&adminv1.GetUserRoleAssignmentsRequest{UserId: "missing"}))
	require.Equal(t, connect.CodeNotFound, errorCode(err))
	_, err = env.adminUsers.GetUserRoleAssignments(ctx, connect.NewRequest(&adminv1.GetUserRoleAssignmentsRequest{}))
	require.Equal(t, connect.CodeInvalidArgument, errorCode(err))

	allowBotCreation(t, env.ctx, env.core, env.viewer.Id)
	bot, err := env.core.CreateBot(env.ctx, env.viewer.Id, "assignment-bot", "Bot")
	require.NoError(t, err)
	_, err = env.adminUsers.GetUserRoleAssignments(ctx, connect.NewRequest(&adminv1.GetUserRoleAssignmentsRequest{UserId: bot.User.GetId()}))
	require.Equal(t, connect.CodeFailedPrecondition, errorCode(err))
	require.NoError(t, env.core.DeleteUser(env.ctx, core.SystemActorID, target.Id))
	_, err = env.adminUsers.GetUserRoleAssignments(ctx, request)
	require.Equal(t, connect.CodeNotFound, errorCode(err))
}

func TestAdminUserRoleAssignmentsMatchMemberLimits(t *testing.T) {
	env := newConnectAPITestEnv(t)
	require.NoError(t, env.core.AssignOwnerRole(env.ctx, env.viewer.Id))
	ctx := withCaller(env.ctx, env.viewer)
	request := connect.NewRequest(&adminv1.GetUserRoleAssignmentsRequest{UserId: env.viewer.Id})
	_, err := env.adminUsers.GetUserRoleAssignments(withBearerCredential(env.ctx, env.viewer, "inactive-review-session"), request)
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = env.adminUsers.GetUserRoleAssignments(withArmedBearerCredential(env.ctx, env.viewer, "active-review-session"), request)
	require.NoError(t, err)
	state, err := env.adminUsers.GetUserRoleAssignments(ctx, connect.NewRequest(&adminv1.GetUserRoleAssignmentsRequest{UserId: env.viewer.Id}))
	require.NoError(t, err)
	details, err := env.adminUsers.GetMember(ctx, connect.NewRequest(&adminv1.GetMemberRequest{Target: &adminv1.GetMemberRequest_UserId{UserId: env.viewer.Id}}))
	require.NoError(t, err)
	require.ElementsMatch(t, details.Msg.GetMember().GetRoles(), state.Msg.GetAssignments().GetRoleNames())
	require.Equal(t, details.Msg.GetAssignableRoleNames(), state.Msg.GetAssignments().GetAssignableRoleNames())
	require.Equal(t, details.Msg.GetRevocableRoleNames(), state.Msg.GetAssignments().GetRevocableRoleNames())
	require.NotContains(t, state.Msg.GetAssignments().GetRevocableRoleNames(), core.RoleOwner)
	require.NotContains(t, state.Msg.GetAssignments().GetRevocableRoleNames(), core.RoleAdmin)
}
