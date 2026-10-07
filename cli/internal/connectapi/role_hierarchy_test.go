package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestRoleHierarchyFlagsDescribeTheViewer(t *testing.T) {
	t.Parallel()

	env := newConnectAPITestEnv(t)
	create := func(login, role string) *evtv1.User {
		user, err := env.core.CreateUser(env.ctx, core.SystemActorID, login, login, "password123")
		require.NoError(t, err)
		if role != "" {
			require.NoError(t, env.core.AssignServerRole(env.ctx, core.SystemActorID, user.Id, role))
		}
		return user
	}
	admin := create("flags-admin", core.RoleAdmin)
	peer := create("flags-peer-admin", core.RoleAdmin)
	member := create("flags-member", "")
	adminCtx := withCaller(env.ctx, admin)

	roles, err := env.roles.ListRoles(adminCtx, connect.NewRequest(&adminv1.ListRolesRequest{}))
	require.NoError(t, err)
	below := map[string]bool{}
	for _, role := range roles.Msg.GetRoles() {
		below[role.GetRole().GetName()] = role.GetRanksBelowViewer()
	}
	require.Equal(t, map[string]bool{core.RoleOwner: false, core.RoleAdmin: false, core.RoleModerator: true, core.RoleEveryone: true}, below)

	tiers, err := env.permissions.GetRolePermissionTierMatrix(adminCtx, connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{}))
	require.NoError(t, err)
	tierBelow := map[string]bool{}
	for _, role := range tiers.Msg.GetMatrix().GetRoles() {
		tierBelow[role.GetRole().GetName()] = role.GetRanksBelowViewer()
	}
	require.Equal(t, below, tierBelow)

	for _, target := range []struct {
		id       string
		outranks bool
	}{{peer.Id, false}, {member.Id, true}} {
		resp, err := env.adminUsers.GetMember(adminCtx, connect.NewRequest(&adminv1.GetMemberRequest{
			Target: &adminv1.GetMemberRequest_UserId{UserId: target.id},
		}))
		require.NoError(t, err)
		require.Equal(t, target.outranks, resp.Msg.GetMember().GetViewerOutranks(), "viewer_outranks for %s", target.id)
		require.Equal(t, target.outranks, resp.Msg.GetMember().GetViewerCanDeleteAccount(), "viewer_can_delete_account for %s", target.id)
	}

	allowBotCreation(t, env.ctx, env.core, peer.Id)
	bot, err := env.core.CreateBot(env.ctx, peer.Id, "flags_peer_bot", "Flags Peer Bot")
	require.NoError(t, err)
	botResp, err := (&botService{api: env.api}).GetBot(adminCtx, connect.NewRequest(&apiv1.GetBotRequest{BotUserId: bot.User.GetId()}))
	require.NoError(t, err)
	require.False(t, botResp.Msg.GetBot().GetViewerOutranks(), "admin must not outrank a peer admin's bot")
}
