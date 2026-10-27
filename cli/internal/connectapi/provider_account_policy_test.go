package connectapi

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"buf.build/go/protovalidate"
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
	require.Equal(t, authv1.UsernameChoiceAvailability_USERNAME_CHOICE_AVAILABILITY_PROVIDER_MANAGED, pending.Msg.Pending.GetChooseUsername())
	require.Equal(t, "person-2", pending.Msg.Pending.LoginHint)
	// Another user takes the preview before confirmation.
	_, err = env.core.CreateUser(env.ctx, core.SystemActorID, "person-2", "Second", "password123")
	require.NoError(t, err)
	request := &authv1.CreateExternalIdentityAccountRequest{Token: token}
	require.NoError(t, protovalidate.Validate(request))
	created, err := env.externalAuth.CreateExternalIdentityAccount(env.ctx, connect.NewRequest(request))
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
	require.Equal(t, "person-3", user.DisplayName)
	ctx := withBearerCredential(env.ctx, user, created.Msg.Token)
	viewer, err := env.viewerService.GetViewer(ctx, connect.NewRequest(&apiv1.GetViewerRequest{}))
	require.NoError(t, err)
	require.Equal(t, apiv1.UsernameChangeAvailability_STATE_PROVIDER_MANAGED, viewer.Msg.User.Actions.ChangeUsername.State)
	// Third-party clients can send a full profile, including the unchanged login.
	profile, err := env.users.UpdateUserProfile(ctx, connect.NewRequest(&apiv1.UpdateUserProfileRequest{
		UserId: user.Id, Login: new(user.Login), DisplayName: new("New display name"), Bio: new("New bio"),
	}))
	require.NoError(t, err)
	require.Equal(t, user.Login, profile.Msg.User.Login)
	require.Equal(t, "New display name", profile.Msg.User.DisplayName)
	require.Equal(t, "New bio", profile.Msg.User.GetBio())
	require.NoError(t, env.core.LinkExternalIdentity(env.ctx, "company", "github", "company", "second", user.Id))
	list, err := env.account.ListExternalIdentities(ctx, connect.NewRequest(&apiv1.ListExternalIdentitiesRequest{}))
	require.NoError(t, err)
	for _, link := range list.Msg.LinkedIdentities {
		require.Equal(t, apiv1.DisconnectEligibility_DISCONNECT_ELIGIBILITY_DISABLED_BY_OPERATOR, link.DisconnectEligibility)
		_, err = env.account.DisconnectExternalIdentity(ctx, connect.NewRequest(&apiv1.DisconnectExternalIdentityRequest{SubjectHash: link.SubjectHash}))
		require.ErrorIs(t, err, core.ErrExternalIdentityUnlinkDisabled)
	}
}

func TestExternalIdentityAccountLoginValidation(t *testing.T) {
	for _, managed := range []bool{false, true} {
		for _, login := range []string{"", "x", strings.Repeat("x", 33), "invalid@name"} {
			t.Run(fmt.Sprintf("managed=%t/login=%q", managed, login), func(t *testing.T) {
				env := newConnectAPITestEnvWithAuth(t, 30*time.Second, config.AuthConfig{Providers: []config.AuthProviderConfig{{
					ID: "company", Type: "github", AutoProvision: new(true), ManageUsernames: managed,
				}}})
				token, err := env.core.CreatePendingExternalIdentityCreateFlow(env.ctx, core.PendingExternalIdentityFlow{
					ProviderID: "company", ProviderType: "github", Issuer: "company", Subject: "new",
					ProviderUsername: "person", DisplayNameHint: strings.Repeat("A", 33),
				})
				require.NoError(t, err)
				request := &authv1.CreateExternalIdentityAccountRequest{Token: token, Login: login}
				require.NoError(t, protovalidate.Validate(request))
				created, err := env.externalAuth.CreateExternalIdentityAccount(env.ctx, connect.NewRequest(request))
				if !managed {
					require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
					return
				}
				require.NoError(t, err)
				require.Equal(t, "person", created.Msg.Login)
				user, err := env.core.GetUser(env.ctx, created.Msg.UserId)
				require.NoError(t, err)
				require.Equal(t, "person", user.DisplayName)
			})
		}
	}
}
