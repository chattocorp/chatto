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

func TestRoleCatalogsDescribeTheViewerAndRoleOrder(t *testing.T) {
	t.Parallel()

	env := newConnectAPITestEnv(t)
	admin, err := env.core.CreateUser(env.ctx, core.SystemActorID, "catalog-admin", "catalog-admin", "password123")
	require.NoError(t, err)
	require.NoError(t, env.core.AssignServerRole(env.ctx, core.SystemActorID, admin.Id, core.RoleAdmin))
	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "catalog-member", "catalog-member", "password123")
	require.NoError(t, err)

	wantOrder := []string{core.RoleOwner, core.RoleAdmin, core.RoleModerator, core.RoleEveryone}
	for _, viewer := range []struct {
		user *evtv1.User
		want string
	}{{admin, core.RoleAdmin}, {member, core.RoleEveryone}} {
		resp, err := env.publicRoles.ListRoles(withCaller(env.ctx, viewer.user), connect.NewRequest(&apiv1.ListRolesRequest{}))
		require.NoError(t, err)
		require.Equal(t, viewer.want, resp.Msg.GetViewerHighestRole())
		names := []string{}
		for _, role := range resp.Msg.GetRoles() {
			names = append(names, role.GetName())
		}
		require.Equal(t, wantOrder, names, "public catalog lists roles highest first")
	}

	adminCtx := withCaller(env.ctx, admin)
	roles, err := env.roles.ListRoles(adminCtx, connect.NewRequest(&adminv1.ListRolesRequest{}))
	require.NoError(t, err)
	adminNames := []string{}
	for _, role := range roles.Msg.GetRoles() {
		adminNames = append(adminNames, role.GetRole().GetName())
	}
	require.Equal(t, wantOrder, adminNames, "admin catalog lists roles highest first")

	tiers, err := env.permissions.GetRolePermissionTierMatrix(adminCtx, connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{}))
	require.NoError(t, err)
	tierNames := []string{}
	for _, role := range tiers.Msg.GetMatrix().GetRoles() {
		tierNames = append(tierNames, role.GetRole().GetName())
	}
	require.Equal(t, wantOrder, tierNames, "tier matrix lists roles highest first")
}
