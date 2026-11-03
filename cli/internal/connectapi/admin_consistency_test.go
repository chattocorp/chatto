package connectapi

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestPublicReflectionIncludesMountedServices(t *testing.T) {
	env := newConnectAPITestEnv(t)
	resolver, err := publicReflectionResolver(publicReflectionServiceNames)
	require.NoError(t, err)
	for _, handler := range env.api.Handlers() {
		if !strings.HasPrefix(handler.ServicePath, "/chatto.") {
			continue
		}
		name := protoreflect.FullName(strings.Trim(handler.ServicePath, "/"))
		_, err := resolver.FindDescriptorByName(name)
		require.NoError(t, err, "mounted service %s must be discoverable", name)
	}
}

func TestAdminMemberPreservesPublicProfile(t *testing.T) {
	env := newConnectAPITestEnv(t)
	require.NoError(t, env.core.AssignOwnerRole(env.ctx, env.viewer.Id))
	ctx := withCaller(env.ctx, env.viewer)
	_, err := env.users.UpdateUserProfile(ctx, connect.NewRequest(&apiv1.UpdateUserProfileRequest{
		UserId: env.viewer.Id, Bio: new("Synthetic biography"),
	}))
	require.NoError(t, err)
	for _, shared := range []bool{true, false} {
		_, err := env.core.UpdateUserSettings(ctx, env.viewer.Id, core.UserSettingsInput{
			Timezone: new("Europe/Berlin"), ShareTimezone: new(shared),
		})
		require.NoError(t, err)
		public, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{
			Target: &apiv1.GetUserRequest_UserId{UserId: env.viewer.Id},
		}))
		require.NoError(t, err)
		singular, err := env.adminUsers.GetMember(ctx, connect.NewRequest(&adminv1.GetMemberRequest{
			Target: &adminv1.GetMemberRequest_UserId{UserId: env.viewer.Id},
		}))
		require.NoError(t, err)
		batch, err := env.adminUsers.BatchGetMembers(ctx, connect.NewRequest(&adminv1.BatchGetMembersRequest{UserIds: []string{env.viewer.Id}}))
		require.NoError(t, err)
		for _, user := range []*apiv1.User{singular.Msg.GetMember().GetUser(), batch.Msg.GetMembers()[0].GetUser()} {
			require.Equal(t, public.Msg.GetUser().GetUser().Bio, user.Bio)
			require.Equal(t, public.Msg.GetUser().GetUser().Timezone, user.Timezone)
		}
	}
	_, err = env.users.UpdateUserProfile(ctx, connect.NewRequest(&apiv1.UpdateUserProfileRequest{UserId: env.viewer.Id, Bio: new("")}))
	require.NoError(t, err)
	_, err = env.core.UpdateUserSettings(ctx, env.viewer.Id, core.UserSettingsInput{Timezone: new(""), ShareTimezone: new(true)})
	require.NoError(t, err)
	cleared, err := env.adminUsers.GetMember(ctx, connect.NewRequest(&adminv1.GetMemberRequest{Target: &adminv1.GetMemberRequest_UserId{UserId: env.viewer.Id}}))
	require.NoError(t, err)
	require.Nil(t, cleared.Msg.GetMember().GetUser().Bio)
	require.Nil(t, cleared.Msg.GetMember().GetUser().Timezone)
}

func TestAdminRoomGroupCapabilityDoesNotGrantRoomCreation(t *testing.T) {
	env := newConnectAPITestEnv(t)
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoomManage))
	require.NoError(t, env.core.DenyUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoomCreate))
	service := &adminRoomLayoutService{api: env.api}
	ctx := withCaller(env.ctx, env.viewer)
	created, err := service.CreateRoomGroup(ctx, connect.NewRequest(&adminv1.CreateRoomGroupRequest{Name: "Managed group"}))
	require.NoError(t, err)
	require.False(t, created.Msg.GetGroup().GetCanCreateRoom())
	details, err := service.GetRoomGroup(ctx, connect.NewRequest(&adminv1.GetRoomGroupRequest{GroupId: created.Msg.GetGroup().GetId()}))
	require.NoError(t, err)
	require.False(t, details.Msg.GetGroup().GetCanCreateRoom())
}

func TestAdminRoomGroupResponsesPreserveCreationCapability(t *testing.T) {
	env := newConnectAPITestEnv(t)
	require.NoError(t, env.core.AssignOwnerRole(env.ctx, env.viewer.Id))
	ctx := withCaller(env.ctx, env.viewer)
	service := &adminRoomLayoutService{api: env.api}
	created, err := service.CreateRoomGroup(ctx, connect.NewRequest(&adminv1.CreateRoomGroupRequest{Name: "Review group"}))
	require.NoError(t, err)
	require.True(t, created.Msg.GetGroup().GetCanCreateRoom())
	id := created.Msg.GetGroup().GetId()
	details, err := service.GetRoomGroup(ctx, connect.NewRequest(&adminv1.GetRoomGroupRequest{GroupId: id}))
	require.NoError(t, err)
	require.True(t, details.Msg.GetGroup().GetCanCreateRoom())
	require.Empty(t, details.Msg.GetGroup().GetItems(), "metadata read must omit the layout")
	updated, err := service.UpdateRoomGroup(ctx, connect.NewRequest(&adminv1.UpdateRoomGroupRequest{GroupId: id, Name: new("Renamed group")}))
	require.NoError(t, err)
	require.True(t, updated.Msg.GetGroup().GetCanCreateRoom())
	reordered, err := service.ReorderSidebarItemsInGroup(ctx, connect.NewRequest(&adminv1.ReorderSidebarItemsInGroupRequest{GroupId: id}))
	require.NoError(t, err)
	require.True(t, reordered.Msg.GetGroup().GetCanCreateRoom())
	link, err := service.CreateSidebarLink(ctx, connect.NewRequest(&adminv1.CreateSidebarLinkRequest{GroupId: id, Label: "Example", Url: "https://example.com"}))
	require.NoError(t, err)
	moved, err := service.MoveSidebarItem(ctx, connect.NewRequest(&adminv1.MoveSidebarItemRequest{
		GroupId: id,
		Item:    &adminv1.AdminRoomLayoutItemInput{Item: &adminv1.AdminRoomLayoutItemInput_SidebarLinkId{SidebarLinkId: link.Msg.GetSidebarLink().GetId()}},
	}))
	require.NoError(t, err)
	require.True(t, moved.Msg.GetGroup().GetCanCreateRoom())
}
