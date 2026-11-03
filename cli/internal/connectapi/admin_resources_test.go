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
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1/apiv1connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestAdminAccountReadsThroughMountedHandlers(t *testing.T) {
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
	_, err := client.ListVerifiedEmails(env.ctx, connect.NewRequest(&adminv1.ListVerifiedEmailsRequest{}))
	require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
	_, err = client.GetUsernameCooldown(env.ctx, connect.NewRequest(&adminv1.GetUsernameCooldownRequest{}))
	require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
	read, err := client.GetUsernameCooldown(env.ctx, connect.NewRequest(&adminv1.GetUsernameCooldownRequest{UserId: env.viewer.Id}))
	require.NoError(t, err)
	require.NotNil(t, read.Msg.GetCooldown())
	require.Equal(t, env.viewer.Id, read.Msg.GetCooldown().GetUserId())
	require.Nil(t, read.Msg.GetCooldown().GetLastLoginChange())
}

func TestMainUserReadsIncludeAssignedRolesWithoutAdminAuthority(t *testing.T) {
	env := newConnectAPITestEnv(t)
	_, err := env.core.CreateServerRole(env.ctx, core.SystemActorID, "directory-helper", "Helper", "", false)
	require.NoError(t, err)
	require.NoError(t, env.core.AssignServerRoleToExistingUser(env.ctx, core.SystemActorID, env.viewer.Id, "directory-helper"))
	mux := http.NewServeMux()
	for _, handler := range env.api.Handlers() {
		mux.Handle(handler.ServicePath, handler.Handler)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(withCaller(r.Context(), env.viewer)))
	}))
	t.Cleanup(server.Close)
	client := apiv1connect.NewUserServiceClient(server.Client(), server.URL, connect.WithProtoJSON())
	read, err := client.GetUser(env.ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_UserId{UserId: env.viewer.Id}}))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{core.RoleEveryone, "directory-helper"}, read.Msg.GetUser().GetRoles())
	require.NotNil(t, read.Msg.GetUser().GetCreatedAt())
	page, err := client.ListUsers(env.ctx, connect.NewRequest(&apiv1.ListUsersRequest{Search: env.viewer.Login}))
	require.NoError(t, err)
	require.Len(t, page.Msg.GetUsers(), 1)
	require.True(t, proto.Equal(read.Msg.GetUser(), page.Msg.GetUsers()[0]))
	batch, err := client.BatchGetUsers(env.ctx, connect.NewRequest(&apiv1.BatchGetUsersRequest{UserIds: []string{env.viewer.Id, "missing", env.viewer.Id}}))
	require.NoError(t, err)
	require.Len(t, batch.Msg.GetUsers(), 1)
	require.True(t, proto.Equal(read.Msg.GetUser(), batch.Msg.GetUsers()[0]))
	_, err = env.adminUsers.ListVerifiedEmails(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.ListVerifiedEmailsRequest{UserId: env.viewer.Id}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
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
	_, err = env.adminUsers.ListVerifiedEmails(ctx, connect.NewRequest(&adminv1.ListVerifiedEmailsRequest{UserId: target.Id}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
}

func TestAdminAccountReadsRespectVisibility(t *testing.T) {
	env := newConnectAPITestEnv(t)
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "resource-target", "Target", "password123")
	require.NoError(t, err)
	require.NoError(t, env.core.AddVerifiedEmailDirect(env.ctx, target.Id, "target@example.com"))
	ctx := withCaller(env.ctx, env.viewer)
	request := connect.NewRequest(&adminv1.ListVerifiedEmailsRequest{UserId: target.Id})
	cooldownRequest := connect.NewRequest(&adminv1.GetUsernameCooldownRequest{UserId: target.Id})
	_, err = env.adminUsers.ListVerifiedEmails(ctx, request)
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = env.adminUsers.GetUsernameCooldown(ctx, cooldownRequest)
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermAdminUsersView))
	read, err := env.adminUsers.ListVerifiedEmails(ctx, request)
	require.NoError(t, err)
	require.Len(t, read.Msg.GetVerifiedEmails(), 1)
	require.Equal(t, "target@example.com", read.Msg.GetVerifiedEmails()[0].GetEmail())
	require.True(t, read.Msg.GetVerifiedEmails()[0].GetPrimary())
	require.NotNil(t, read.Msg.GetVerifiedEmails()[0].GetVerifiedAt())
	empty, err := env.adminUsers.ListVerifiedEmails(ctx, connect.NewRequest(&adminv1.ListVerifiedEmailsRequest{UserId: env.viewer.Id}))
	require.NoError(t, err)
	require.Empty(t, empty.Msg.GetVerifiedEmails())
	_, err = env.adminUsers.GetUsernameCooldown(ctx, cooldownRequest)
	require.Equal(t, connect.CodePermissionDenied, errorCode(err), "view-users alone cannot inspect another user's history")
	_, err = env.adminUsers.GetUsernameCooldown(ctx, connect.NewRequest(&adminv1.GetUsernameCooldownRequest{UserId: "missing"}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err), "authorization precedes target lookup")
	self, err := env.adminUsers.GetUsernameCooldown(ctx, connect.NewRequest(&adminv1.GetUsernameCooldownRequest{UserId: env.viewer.Id}))
	require.NoError(t, err)
	require.NotNil(t, self.Msg.GetCooldown())
	require.Nil(t, self.Msg.GetCooldown().GetLastLoginChange())

	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserManageAccounts))
	cooldown, err := env.adminUsers.GetUsernameCooldown(ctx, cooldownRequest)
	require.NoError(t, err)
	require.NotNil(t, cooldown.Msg.GetCooldown())
	require.Nil(t, cooldown.Msg.GetCooldown().GetLastLoginChange())
	require.NoError(t, env.core.ClearUserPermissionState(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserManageAccounts))
	_, err = env.adminUsers.GetUsernameCooldown(ctx, cooldownRequest)
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserManageAccounts))
	_, err = env.core.UpdateUserLogin(env.ctx, target.Id, "resource-renamed")
	require.NoError(t, err)
	cooldown, err = env.adminUsers.GetUsernameCooldown(ctx, cooldownRequest)
	require.NoError(t, err)
	require.NotNil(t, cooldown.Msg.GetCooldown().GetLastLoginChange())
	_, err = env.adminUsers.ClearUsernameCooldown(ctx, connect.NewRequest(&adminv1.ClearUsernameCooldownRequest{UserId: target.Id}))
	require.NoError(t, err)
	cooldown, err = env.adminUsers.GetUsernameCooldown(ctx, cooldownRequest)
	require.NoError(t, err)
	require.Nil(t, cooldown.Msg.GetCooldown().GetLastLoginChange())
	require.NoError(t, env.core.ClearUserPermissionState(env.ctx, core.SystemActorID, env.viewer.Id, core.PermAdminUsersView))
	_, err = env.adminUsers.ListVerifiedEmails(ctx, request)
	require.Equal(t, connect.CodePermissionDenied, errorCode(err), "account-management authority must not reveal private account data")
	_, err = env.adminUsers.GetUsernameCooldown(ctx, cooldownRequest)
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermAdminUsersView))
	_, err = env.adminUsers.ListVerifiedEmails(withBearerCredential(env.ctx, env.viewer, "inactive-resource-session"), request)
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = env.adminUsers.ListVerifiedEmails(ctx, connect.NewRequest(&adminv1.ListVerifiedEmailsRequest{UserId: "missing"}))
	require.Equal(t, connect.CodeNotFound, errorCode(err))
	_, err = env.adminUsers.GetUsernameCooldown(ctx, connect.NewRequest(&adminv1.GetUsernameCooldownRequest{UserId: "missing"}))
	require.Equal(t, connect.CodeNotFound, errorCode(err))
	require.NoError(t, env.core.DeleteUser(env.ctx, core.SystemActorID, target.Id))
	_, err = env.adminUsers.ListVerifiedEmails(ctx, request)
	require.Equal(t, connect.CodeNotFound, errorCode(err))
	_, err = env.adminUsers.GetUsernameCooldown(ctx, cooldownRequest)
	require.Equal(t, connect.CodeNotFound, errorCode(err))
}

func TestRoomGroupReadsUsePermissionsAndArchiveFilters(t *testing.T) {
	env := newConnectAPITestEnv(t)
	directory := &roomDirectoryService{api: env.api}
	admin := &adminRoomLayoutService{api: env.api}
	ctx := withCaller(env.ctx, env.viewer)
	group, err := env.core.RoomLayout().CreateRoomGroup(env.ctx, core.SystemActorID, "Managed", "Description")
	require.NoError(t, err)
	other, err := env.core.RoomLayout().CreateRoomGroup(env.ctx, core.SystemActorID, "Other", "")
	require.NoError(t, err)
	active, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, group.GetId(), "Hidden active", "")
	require.NoError(t, err)
	archived, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, group.GetId(), "Hidden archive", "")
	require.NoError(t, err)
	outside, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, other.GetId(), "Outside", "")
	require.NoError(t, err)
	for _, room := range []string{active.Id, archived.Id, outside.Id} {
		for _, permission := range []core.Permission{core.PermRoomList, core.PermRoomJoin, core.PermMessageRead, core.PermMessageReadInteractions} {
			require.NoError(t, env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room, core.RoleEveryone, permission))
		}
	}
	_, err = env.core.ArchiveRoom(env.ctx, core.SystemActorID, core.KindChannel, archived.Id)
	require.NoError(t, err)
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleManage))
	all := apiv1.RoomArchiveFilter_ROOM_ARCHIVE_FILTER_ALL
	read, err := directory.GetRoomGroup(ctx, connect.NewRequest(&apiv1.GetRoomGroupRequest{GroupId: group.Id, ArchiveFilter: all}))
	require.NoError(t, err)
	require.Empty(t, read.Msg.GetGroup().GetItems(), "role.manage must not expose hidden room names")
	legacyMetadata, err := admin.GetRoomGroup(ctx, connect.NewRequest(&adminv1.GetRoomGroupRequest{GroupId: group.Id}))
	require.NoError(t, err)
	require.Empty(t, legacyMetadata.Msg.GetGroup().GetItems())

	require.NoError(t, env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, group.Id, env.viewer.Id, core.PermRoomManage))
	link, err := admin.CreateSidebarLink(ctx, connect.NewRequest(&adminv1.CreateSidebarLinkRequest{GroupId: group.Id, Label: "Link", Url: "https://example.com"}))
	require.NoError(t, err)
	inactive := withBearerCredential(env.ctx, env.viewer, "inactive-layout-session")
	inactiveGroup, err := directory.GetRoomGroup(inactive, connect.NewRequest(&apiv1.GetRoomGroupRequest{GroupId: group.Id, ArchiveFilter: all}))
	require.NoError(t, err)
	require.Len(t, inactiveGroup.Msg.GetGroup().GetItems(), 1, "inactive privileged permissions do not expose hidden room metadata")
	read, err = directory.GetRoomGroup(ctx, connect.NewRequest(&apiv1.GetRoomGroupRequest{GroupId: group.Id, ArchiveFilter: all}))
	require.NoError(t, err)
	require.Equal(t, "Description", read.Msg.GetGroup().GetDescription())
	require.Len(t, read.Msg.GetGroup().GetItems(), 3)
	require.Equal(t, active.Id, read.Msg.GetGroup().GetItems()[0].GetRoom().GetRoom().GetId())
	require.True(t, read.Msg.GetGroup().GetItems()[1].GetRoom().GetRoom().GetArchived())
	require.True(t, proto.Equal(link.Msg.GetSidebarLink(), read.Msg.GetGroup().GetItems()[2].GetSidebarLink()))
	page, err := directory.ListRoomGroups(ctx, connect.NewRequest(&apiv1.ListRoomGroupsRequest{ArchiveFilter: all}))
	require.NoError(t, err)
	require.True(t, proto.Equal(read.Msg.GetGroup(), findDirectoryGroup(page.Msg.GetGroups(), group.Id)))
	require.Empty(t, findDirectoryGroup(page.Msg.GetGroups(), other.Id).GetItems(), "delegated managers cannot inspect another group's hidden rooms")
	batch, err := directory.BatchGetRoomGroups(ctx, connect.NewRequest(&apiv1.BatchGetRoomGroupsRequest{GroupIds: []string{group.Id, "missing", group.Id}, ArchiveFilter: all}))
	require.NoError(t, err)
	require.Len(t, batch.Msg.GetGroups(), 1)
	require.True(t, proto.Equal(read.Msg.GetGroup(), batch.Msg.GetGroups()[0]))
	ordinary, err := directory.GetRoomGroup(ctx, connect.NewRequest(&apiv1.GetRoomGroupRequest{GroupId: group.Id}))
	require.NoError(t, err)
	require.Len(t, ordinary.Msg.GetGroup().GetItems(), 2, "the default filter excludes archived rooms, preserving links")
	onlyArchived, err := directory.GetRoomGroup(ctx, connect.NewRequest(&apiv1.GetRoomGroupRequest{GroupId: group.Id, ArchiveFilter: apiv1.RoomArchiveFilter_ROOM_ARCHIVE_FILTER_ARCHIVED}))
	require.NoError(t, err)
	require.Len(t, onlyArchived.Msg.GetGroup().GetItems(), 2)
	require.Equal(t, archived.Id, onlyArchived.Msg.GetGroup().GetItems()[0].GetRoom().GetRoom().GetId())

	room, err := directory.GetRoom(ctx, connect.NewRequest(&apiv1.GetRoomRequest{RoomId: active.Id}))
	require.NoError(t, err)
	require.False(t, room.Msg.GetRoom().GetViewerState().GetIsMember())
	for _, permission := range room.Msg.GetRoom().GetViewerState().GetPermissions() {
		if permission.GetPermission() == string(core.PermRoomList) || permission.GetPermission() == string(core.PermMessageRead) {
			require.False(t, permission.GetGranted(), "metadata reads must not create navigation or message grants")
		}
	}
	_, err = env.core.RoomTimelineReads().BatchGetMessages(env.ctx, env.viewer.Id, active.Id, []string{"missing"})
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	rooms, err := directory.BatchGetRooms(ctx, connect.NewRequest(&apiv1.BatchGetRoomsRequest{RoomIds: []string{active.Id, outside.Id, active.Id}}))
	require.NoError(t, err)
	require.Len(t, rooms.Msg.GetRooms(), 1)
	listed, err := directory.ListRooms(ctx, connect.NewRequest(&apiv1.ListRoomsRequest{Scope: apiv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_CHANNELS, ArchiveFilter: all}))
	require.NoError(t, err)
	require.True(t, directoryRoomsContainAPI(listed.Msg.GetRooms(), active.Id))
	require.True(t, directoryRoomsContainAPI(listed.Msg.GetRooms(), archived.Id))
	require.False(t, directoryRoomsContainAPI(listed.Msg.GetRooms(), outside.Id))

	updated, err := admin.UpdateRoomGroup(ctx, connect.NewRequest(&adminv1.UpdateRoomGroupRequest{GroupId: group.Id, Description: new("Updated")}))
	require.NoError(t, err)
	require.Equal(t, "Updated", updated.Msg.GetRoomGroup().GetDescription())
	require.Len(t, updated.Msg.GetRoomGroup().GetItems(), 3)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "layout-owner", "Owner", "password123")
	require.NoError(t, err)
	require.NoError(t, env.core.AssignOwnerRole(env.ctx, owner.Id))
	require.NoError(t, env.core.SetUserPermissionState(env.ctx, owner.Id, env.viewer.Id, core.PermissionTargetScope{Kind: core.MatrixScopeGroup, ID: group.Id}, core.PermRoomManage, core.PermissionStateNone))
	read, err = directory.GetRoomGroup(ctx, connect.NewRequest(&apiv1.GetRoomGroupRequest{GroupId: group.Id, ArchiveFilter: all}))
	require.NoError(t, err)
	require.Len(t, read.Msg.GetGroup().GetItems(), 1, "permission loss removes hidden room metadata but preserves public links")
	_, err = directory.GetRoom(ctx, connect.NewRequest(&apiv1.GetRoomRequest{RoomId: active.Id}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	require.NoError(t, env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, active.Id, env.viewer.Id, core.PermRoomManage))
	read, err = directory.GetRoomGroup(ctx, connect.NewRequest(&apiv1.GetRoomGroupRequest{GroupId: group.Id, ArchiveFilter: all}))
	require.NoError(t, err)
	require.Len(t, read.Msg.GetGroup().GetItems(), 2, "room-specific managers receive only their own room's hidden metadata")
	require.NoError(t, env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, group.Id, env.viewer.Id, core.PermRoomManage))
	read, err = directory.GetRoomGroup(ctx, connect.NewRequest(&apiv1.GetRoomGroupRequest{GroupId: group.Id, ArchiveFilter: all}))
	require.NoError(t, err)
	require.Len(t, read.Msg.GetGroup().GetItems(), 3)
	_, err = directory.GetRoomGroup(ctx, connect.NewRequest(&apiv1.GetRoomGroupRequest{GroupId: "missing"}))
	require.Equal(t, connect.CodeNotFound, errorCode(err))
}

func directoryRoomsContainAPI(rooms []*apiv1.RoomWithViewerState, id string) bool {
	for _, room := range rooms {
		if room.GetRoom().GetId() == id {
			return true
		}
	}
	return false
}
