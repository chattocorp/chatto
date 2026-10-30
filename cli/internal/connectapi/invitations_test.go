package connectapi

import (
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
	_, err = env.adminInvitationCodes.ListInvitationCodes(ctx, connect.NewRequest(&adminv1.ListInvitationCodesRequest{}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = env.adminInvitationCodes.GetInvitationCode(ctx, connect.NewRequest(&adminv1.GetInvitationCodeRequest{Id: code.GetId()}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = env.adminInvitationCodes.RevokeInvitationCode(ctx, connect.NewRequest(&adminv1.RevokeInvitationCodeRequest{Id: code.GetId()}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = env.adminInvitationCodes.CreateInvitationCode(ctx, connect.NewRequest(&adminv1.CreateInvitationCodeRequest{}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))

	// Management remains privileged even after it is granted to the same member.
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserManageInvites))
	_, err = env.adminInvitationCodes.ListInvitationCodes(ctx, connect.NewRequest(&adminv1.ListInvitationCodesRequest{}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	adminCtx := withArmedBearerCredential(env.ctx, env.viewer, "member-session")
	listed, err := env.adminInvitationCodes.ListInvitationCodes(adminCtx, connect.NewRequest(&adminv1.ListInvitationCodesRequest{}))
	require.NoError(t, err)
	require.Len(t, listed.Msg.InvitationCodes, 1)
	require.Equal(t, env.viewer.Id, listed.Msg.InvitationCodes[0].CreatedBy)
	require.Equal(t, code.Code, listed.Msg.InvitationCodes[0].Code)
}
