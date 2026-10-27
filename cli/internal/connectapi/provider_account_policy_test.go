package connectapi

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	authv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/auth/v1"
	"github.com/stretchr/testify/require"
)

func TestManagedProviderAccountAPI(t *testing.T) {
	env := newConnectAPITestEnvWithAuth(t, 30*time.Second, config.AuthConfig{Providers: []config.AuthProviderConfig{{
		ID: "company", Type: "github", AutoProvision: new(true), ManageUsernames: true, AllowUnlinking: new(false),
	}}})
	_, err := env.core.CreateUser(env.ctx, core.SystemActorID, "person", "Existing", "password123")
	require.NoError(t, err)
	token, err := env.core.CreatePendingExternalIdentityCreateFlow(env.ctx, core.PendingExternalIdentityFlow{
		ProviderID: "company", ProviderType: "github", Issuer: "company", Subject: "new", ProviderUsername: "person",
	})
	require.NoError(t, err)
	pending, err := env.externalAuth.GetPendingExternalIdentity(env.ctx, connect.NewRequest(&authv1.GetPendingExternalIdentityRequest{Token: token}))
	require.NoError(t, err)
	require.True(t, pending.Msg.Pending.UsernameManaged)
	require.Equal(t, "person-2", pending.Msg.Pending.LoginHint)
	// Another user takes the preview before confirmation.
	_, err = env.core.CreateUser(env.ctx, core.SystemActorID, "person-2", "Second", "password123")
	require.NoError(t, err)
	created, err := env.externalAuth.CreateExternalIdentityAccount(env.ctx, connect.NewRequest(&authv1.CreateExternalIdentityAccountRequest{Token: token, Login: "forged", DisplayName: "Person"}))
	require.NoError(t, err)
	require.Equal(t, "person-3", created.Msg.Login)
	invalidToken, err := env.core.CreatePendingExternalIdentityCreateFlow(env.ctx, core.PendingExternalIdentityFlow{
		ProviderID: "company", ProviderType: "github", Issuer: "company", Subject: "invalid-name", ProviderUsername: "invalid@name",
	})
	require.NoError(t, err)
	_, err = env.externalAuth.GetPendingExternalIdentity(env.ctx, connect.NewRequest(&authv1.GetPendingExternalIdentityRequest{Token: invalidToken}))
	require.Equal(t, connect.CodeFailedPrecondition, errorCode(err))
	user, err := env.core.GetUser(env.ctx, created.Msg.UserId)
	require.NoError(t, err)
	ctx := withBearerCredential(env.ctx, user, created.Msg.Token)
	viewer, err := env.viewerService.GetViewer(ctx, connect.NewRequest(&apiv1.GetViewerRequest{}))
	require.NoError(t, err)
	require.True(t, viewer.Msg.User.GetUsernameManaged())
	require.NoError(t, env.core.LinkExternalIdentity(env.ctx, "company", "github", "company", "second", user.Id))
	list, err := env.account.ListExternalIdentities(ctx, connect.NewRequest(&apiv1.ListExternalIdentitiesRequest{}))
	require.NoError(t, err)
	for _, link := range list.Msg.LinkedIdentities {
		require.Equal(t, apiv1.DisconnectEligibility_DISCONNECT_ELIGIBILITY_DISABLED_BY_OPERATOR, link.DisconnectEligibility)
		_, err = env.account.DisconnectExternalIdentity(ctx, connect.NewRequest(&apiv1.DisconnectExternalIdentityRequest{SubjectHash: link.SubjectHash}))
		require.ErrorIs(t, err, core.ErrExternalIdentityUnlinkDisabled)
	}
}
