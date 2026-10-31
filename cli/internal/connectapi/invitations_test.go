package connectapi

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/stretchr/testify/require"
)

func TestMemberInvitationPermissionBoundary(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	service := &invitationCodeService{api: env.api}
	ctx := withBearerCredential(env.ctx, env.viewer, "member-session")
	request := connect.NewRequest(&apiv1.CreateInvitationCodeRequest{})
	_, err := service.CreateInvitationCode(ctx, request)
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserInvite))
	before := time.Now()
	created, err := service.CreateInvitationCode(ctx, request)
	require.NoError(t, err)
	code := created.Msg.GetInvitationCode()
	require.NotEmpty(t, code.GetCode())
	require.WithinDuration(t, before.Add(7*24*time.Hour), code.GetExpiresAt().AsTime(), 5*time.Second)
	require.Equal(t, "no-store", created.Header().Get("Cache-Control"))
	allowed, err := env.core.HasAnyAdminPermission(ctx, env.viewer.Id)
	require.NoError(t, err)
	require.False(t, allowed)
	creator, err := env.core.CreateUser(env.ctx, core.SystemActorID, "other-creator", "Other Creator", "password123")
	require.NoError(t, err)
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, creator.Id, core.PermUserInvite))
	other, err := env.core.CreateInvitationCode(env.ctx, creator.Id, "Other creator's note")
	require.NoError(t, err)
	listed, err := env.adminInvitationCodes.ListInvitationCodes(ctx, connect.NewRequest(&adminv1.ListInvitationCodesRequest{Page: &apiv1.PageRequest{Limit: 1}}))
	require.NoError(t, err)
	require.Len(t, listed.Msg.InvitationCodes, 1)
	require.Equal(t, code.Id, listed.Msg.InvitationCodes[0].Id)
	require.EqualValues(t, 1, listed.Msg.Page.TotalCount)
	require.False(t, listed.Msg.Page.HasMore)
	_, err = env.adminInvitationCodes.GetInvitationCode(ctx, connect.NewRequest(&adminv1.GetInvitationCodeRequest{Id: code.GetId()}))
	require.NoError(t, err)
	for _, id := range []string{other.ID, core.NewInvitationID()} {
		_, err = env.adminInvitationCodes.GetInvitationCode(ctx, connect.NewRequest(&adminv1.GetInvitationCodeRequest{Id: id}))
		require.Equal(t, connect.CodeNotFound, errorCode(err))
		_, err = env.adminInvitationCodes.RevokeInvitationCode(ctx, connect.NewRequest(&adminv1.RevokeInvitationCodeRequest{Id: id}))
		require.Equal(t, connect.CodeNotFound, errorCode(err))
	}
	_, err = env.adminInvitationCodes.RevokeInvitationCode(ctx, connect.NewRequest(&adminv1.RevokeInvitationCodeRequest{Id: code.GetId()}))
	require.NoError(t, err)
	_, err = env.adminInvitationCodes.CreateInvitationCode(ctx, connect.NewRequest(&adminv1.CreateInvitationCodeRequest{}))
	require.NoError(t, err)

	// Management remains privileged even after it is granted to the same member.
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserManageInvites))
	listed, err = env.adminInvitationCodes.ListInvitationCodes(ctx, connect.NewRequest(&adminv1.ListInvitationCodesRequest{}))
	require.NoError(t, err)
	require.Len(t, listed.Msg.InvitationCodes, 2)
	adminCtx := withArmedBearerCredential(env.ctx, env.viewer, "member-session")
	listed, err = env.adminInvitationCodes.ListInvitationCodes(adminCtx, connect.NewRequest(&adminv1.ListInvitationCodesRequest{}))
	require.NoError(t, err)
	require.Len(t, listed.Msg.InvitationCodes, 3)
	_, err = env.adminInvitationCodes.RevokeInvitationCode(adminCtx, connect.NewRequest(&adminv1.RevokeInvitationCodeRequest{Id: other.ID}))
	require.NoError(t, err)
	// Losing issuance also removes access to owned codes outside privileged mode.
	require.NoError(t, env.core.DenyUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserInvite))
	_, err = env.adminInvitationCodes.GetInvitationCode(ctx, connect.NewRequest(&adminv1.GetInvitationCodeRequest{Id: code.Id}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = env.adminInvitationCodes.RevokeInvitationCode(ctx, connect.NewRequest(&adminv1.RevokeInvitationCodeRequest{Id: code.Id}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	// Management alone still allows creation and access to all codes when armed.
	_, err = env.adminInvitationCodes.CreateInvitationCode(adminCtx, connect.NewRequest(&adminv1.CreateInvitationCodeRequest{}))
	require.NoError(t, err)
}

func TestInvitationExpiryIsServerControlledForEveryCaller(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserInvite))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserManageInvites))
	service := env.adminInvitationCodes
	request := &adminv1.CreateInvitationCodeRequest{}
	require.Nil(t, request.ProtoReflect().Descriptor().Fields().ByName("expires_at"))
	for _, ctx := range []context.Context{
		withBearerCredential(env.ctx, env.viewer, "member-session"),
		withArmedBearerCredential(env.ctx, env.viewer, "member-session"),
	} {
		created, err := service.CreateInvitationCode(ctx, connect.NewRequest(request))
		require.NoError(t, err)
		code := created.Msg.InvitationCode
		require.NotNil(t, code.ExpiresAt)
		require.Equal(t, code.CreatedAt.AsTime().Add(7*24*time.Hour), code.ExpiresAt.AsTime())
	}
}
