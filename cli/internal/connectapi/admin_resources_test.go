package connectapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1/adminv1connect"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestAdminCanonicalReadsThroughMountedHandlers(t *testing.T) {
	env := newConnectAPITestEnv(t)
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermAdminUsersView))
	mux := http.NewServeMux()
	for _, handler := range env.api.Handlers() {
		mux.Handle(handler.ServicePath, handler.Handler)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(withCaller(r.Context(), env.viewer)))
	}))
	t.Cleanup(server.Close)
	client := adminv1connect.NewAdminUserServiceClient(server.Client(), server.URL, connect.WithProtoJSON())
	_, err := client.GetUser(env.ctx, connect.NewRequest(&adminv1.GetUserRequest{}))
	require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = env.viewer.Id
	}
	_, err = client.BatchGetUsers(env.ctx, connect.NewRequest(&adminv1.BatchGetUsersRequest{UserIds: ids}))
	require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
	read, err := client.GetUser(env.ctx, connect.NewRequest(&adminv1.GetUserRequest{Target: &adminv1.GetUserRequest_UserId{UserId: env.viewer.Id}}))
	require.NoError(t, err)
	require.Equal(t, env.viewer.Id, read.Msg.GetUser().GetProfile().GetId())
}

func TestAdminCommandsExposeOwnOutcomesWithoutPrivateReadAuthority(t *testing.T) {
	env := newConnectAPITestEnv(t)
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "outcome-target", "Target", "password123")
	require.NoError(t, err)
	for _, permission := range []core.Permission{core.PermRoleAssign, core.PermUserManageAccounts} {
		require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, permission))
	}
	_, err = env.core.CreateServerRole(env.ctx, core.SystemActorID, "outcome-role", "Role", "", false)
	require.NoError(t, err)
	ctx := withCaller(env.ctx, env.viewer)
	assigned, err := env.adminUsers.AssignRole(ctx, connect.NewRequest(&adminv1.AssignRoleRequest{UserId: target.Id, RoleName: "outcome-role"}))
	require.NoError(t, err)
	require.True(t, assigned.Msg.GetAssignment().GetAssigned())
	require.Equal(t, target.Id, assigned.Msg.GetAssignment().GetUserId())
	revoked, err := env.adminUsers.RevokeRole(ctx, connect.NewRequest(&adminv1.RevokeRoleRequest{UserId: target.Id, RoleName: "outcome-role"}))
	require.NoError(t, err)
	require.False(t, revoked.Msg.GetAssignment().GetAssigned())
	require.Equal(t, "outcome-role", revoked.Msg.GetAssignment().GetRoleName())
	deactivated, err := env.adminUsers.SetUserDeactivated(ctx, connect.NewRequest(&adminv1.SetUserDeactivatedRequest{UserId: target.Id, Deactivated: true}))
	require.NoError(t, err)
	require.True(t, deactivated.Msg.GetProfile().GetDeactivated())
	require.True(t, proto.Equal(deactivated.Msg.GetProfile(), deactivated.Msg.GetMember().GetUser()))
	_, err = env.adminUsers.GetUser(ctx, connect.NewRequest(&adminv1.GetUserRequest{Target: &adminv1.GetUserRequest_UserId{UserId: target.Id}}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
}

func TestAdminUserResourcesAreLeanAndRespectVisibility(t *testing.T) {
	env := newConnectAPITestEnv(t)
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "resource-target", "Target", "password123")
	require.NoError(t, err)
	require.NoError(t, env.core.AddVerifiedEmailDirect(env.ctx, target.Id, "target@example.com"))
	ctx := withCaller(env.ctx, env.viewer)
	request := connect.NewRequest(&adminv1.GetUserRequest{Target: &adminv1.GetUserRequest_UserId{UserId: target.Id}})
	_, err = env.adminUsers.GetUser(ctx, request)
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermAdminUsersView))
	read, err := env.adminUsers.GetUser(ctx, request)
	require.NoError(t, err)
	require.Equal(t, target.Id, read.Msg.GetUser().GetProfile().GetId())
	require.Equal(t, []string{"target@example.com"}, read.Msg.GetUser().GetVerifiedEmails())
	require.Nil(t, read.Msg.GetUser().GetUsernameCooldown(), "view-users alone cannot inspect cooldown history")
	byLogin, err := env.adminUsers.GetUser(ctx, connect.NewRequest(&adminv1.GetUserRequest{Target: &adminv1.GetUserRequest_Login{Login: target.GetLogin()}}))
	require.NoError(t, err)
	require.True(t, proto.Equal(read.Msg.GetUser(), byLogin.Msg.GetUser()))
	batch, err := env.adminUsers.BatchGetUsers(ctx, connect.NewRequest(&adminv1.BatchGetUsersRequest{UserIds: []string{target.Id, "missing", env.viewer.Id, target.Id}}))
	require.NoError(t, err)
	require.Len(t, batch.Msg.GetUsers(), 2)
	require.True(t, proto.Equal(read.Msg.GetUser(), batch.Msg.GetUsers()[0]))
	require.Equal(t, env.viewer.Id, batch.Msg.GetUsers()[1].GetProfile().GetId())
	page, err := env.adminUsers.ListUsers(ctx, connect.NewRequest(&adminv1.ListUsersRequest{Search: "resource-target", Page: &apiv1.PageRequest{Limit: 1}}))
	require.NoError(t, err)
	require.Equal(t, []string{target.Id}, page.Msg.GetUserIds())

	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserManageAccounts))
	read, err = env.adminUsers.GetUser(ctx, request)
	require.NoError(t, err)
	require.NotNil(t, read.Msg.GetUser().GetUsernameCooldown())
	require.Nil(t, read.Msg.GetUser().GetUsernameCooldown().GetLastLoginChange())
	require.NoError(t, env.core.ClearUserPermissionState(env.ctx, core.SystemActorID, env.viewer.Id, core.PermAdminUsersView))
	_, err = env.adminUsers.GetUser(ctx, request)
	require.Equal(t, connect.CodePermissionDenied, errorCode(err), "account-management authority must not reveal private account data")
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermAdminUsersView))
	_, err = env.adminUsers.GetUser(withBearerCredential(env.ctx, env.viewer, "inactive-resource-session"), request)
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = env.adminUsers.GetUser(ctx, connect.NewRequest(&adminv1.GetUserRequest{Target: &adminv1.GetUserRequest_UserId{UserId: "missing"}}))
	require.Equal(t, connect.CodeNotFound, errorCode(err))
	require.NoError(t, env.core.DeleteUser(env.ctx, core.SystemActorID, target.Id))
	_, err = env.adminUsers.GetUser(ctx, request)
	require.Equal(t, connect.CodeNotFound, errorCode(err))
}

func TestScopedAdminLayoutReadsAndCanonicalMutationResults(t *testing.T) {
	env := newConnectAPITestEnv(t)
	service := &adminRoomLayoutService{api: env.api}
	group, err := env.core.RoomLayout().CreateRoomGroup(env.ctx, core.SystemActorID, "Managed", "Description")
	require.NoError(t, err)
	other, err := env.core.RoomLayout().CreateRoomGroup(env.ctx, core.SystemActorID, "Other", "")
	require.NoError(t, err)
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, group.GetId(), "Hidden archive", "")
	require.NoError(t, err)
	_, err = env.core.ArchiveRoom(env.ctx, core.SystemActorID, core.KindChannel, room.GetId())
	require.NoError(t, err)
	hidden, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, group.GetId(), "Hidden active", "")
	require.NoError(t, err)
	require.NoError(t, env.core.DenyRoomPermission(env.ctx, core.SystemActorID, hidden.GetId(), core.RoleEveryone, core.PermRoomList))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleManage))
	ctx := withCaller(env.ctx, env.viewer)
	metadata, err := service.GetRoomGroup(ctx, connect.NewRequest(&adminv1.GetRoomGroupRequest{GroupId: group.GetId()}))
	require.NoError(t, err)
	require.Equal(t, group.GetId(), metadata.Msg.GetRoomGroup().GetId())
	require.Empty(t, metadata.Msg.GetGroup().GetItems())
	_, err = service.GetRoomGroupLayout(ctx, connect.NewRequest(&adminv1.GetRoomGroupLayoutRequest{GroupId: group.GetId()}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	require.NoError(t, env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, group.GetId(), env.viewer.Id, core.PermRoomManage))
	link, err := service.CreateSidebarLink(ctx, connect.NewRequest(&adminv1.CreateSidebarLinkRequest{GroupId: group.GetId(), Label: "Link", Url: "https://example.com"}))
	require.NoError(t, err)
	require.Equal(t, group.GetId(), link.Msg.GetLink().GetGroupId())
	require.True(t, proto.Equal(link.Msg.GetSidebarLink(), link.Msg.GetLink().GetLink()))
	readLink, err := service.GetSidebarLink(ctx, connect.NewRequest(&adminv1.GetSidebarLinkRequest{LinkId: link.Msg.GetSidebarLink().GetId()}))
	require.NoError(t, err)
	require.True(t, proto.Equal(link.Msg.GetLink(), readLink.Msg.GetLink()))
	layout, err := service.GetRoomGroupLayout(ctx, connect.NewRequest(&adminv1.GetRoomGroupLayoutRequest{GroupId: group.GetId()}))
	require.NoError(t, err)
	require.True(t, proto.Equal(metadata.Msg.GetRoomGroup(), layout.Msg.GetLayout().GetGroup()))
	require.Len(t, layout.Msg.GetLayout().GetItems(), 3)
	require.True(t, layout.Msg.GetLayout().GetItems()[0].GetRoom().GetArchived())
	_, err = service.GetRoomLayout(ctx, connect.NewRequest(&adminv1.GetRoomLayoutRequest{}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err), "scoped management must not reveal the whole server layout")
	updated, err := service.UpdateSidebarLink(ctx, connect.NewRequest(&adminv1.UpdateSidebarLinkRequest{LinkId: readLink.Msg.GetLink().GetLink().GetId(), Label: new("Updated")}))
	require.NoError(t, err)
	require.Equal(t, group.GetId(), updated.Msg.GetLink().GetGroupId())
	require.Equal(t, "Updated", updated.Msg.GetLink().GetLink().GetLabel())
	require.NoError(t, env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, other.GetId(), env.viewer.Id, core.PermRoomManage))
	moved, err := service.MoveSidebarLinkToGroup(ctx, connect.NewRequest(&adminv1.MoveSidebarLinkToGroupRequest{LinkId: readLink.Msg.GetLink().GetLink().GetId(), GroupId: other.GetId()}))
	require.NoError(t, err)
	require.Equal(t, other.GetId(), moved.Msg.GetLink().GetGroupId())
	// Losing authority in the destination must block later link reads.
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "layout-owner", "Owner", "password123")
	require.NoError(t, err)
	require.NoError(t, env.core.AssignOwnerRole(env.ctx, owner.Id))
	require.NoError(t, env.core.SetUserPermissionState(env.ctx, owner.Id, env.viewer.Id, core.PermissionTargetScope{Kind: core.MatrixScopeGroup, ID: other.GetId()}, core.PermRoomManage, core.PermissionStateNone))
	_, err = service.GetSidebarLink(ctx, connect.NewRequest(&adminv1.GetSidebarLinkRequest{LinkId: readLink.Msg.GetLink().GetLink().GetId()}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = service.GetSidebarLink(ctx, connect.NewRequest(&adminv1.GetSidebarLinkRequest{LinkId: "missing"}))
	require.Equal(t, connect.CodeNotFound, errorCode(err))
	_, err = service.GetRoomGroupLayout(ctx, connect.NewRequest(&adminv1.GetRoomGroupLayoutRequest{GroupId: "missing"}))
	require.Equal(t, connect.CodeNotFound, errorCode(err))
	require.NoError(t, env.core.AssignOwnerRole(env.ctx, env.viewer.Id))
	full, err := service.GetRoomLayout(ctx, connect.NewRequest(&adminv1.GetRoomLayoutRequest{}))
	require.NoError(t, err)
	legacy, err := service.ListRoomGroups(ctx, connect.NewRequest(&adminv1.ListRoomGroupsRequest{}))
	require.NoError(t, err)
	require.Len(t, full.Msg.GetLayout().GetGroups(), len(legacy.Msg.GetGroups()))
	for i, old := range legacy.Msg.GetGroups() {
		require.Equal(t, old.GetId(), full.Msg.GetLayout().GetGroups()[i].GetGroup().GetId())
		require.Len(t, full.Msg.GetLayout().GetGroups()[i].GetItems(), len(old.GetItems()))
	}
}
