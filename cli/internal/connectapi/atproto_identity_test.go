package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
	authv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/auth/v1"
	"github.com/stretchr/testify/require"
)

// The public atproto_did follows the linked AT Protocol sign-in identity
// (FDR-056): linking publishes it, and disconnecting removes it.
func TestUserSummaryPublishesLinkedATProtoDID(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	const did = "did:plc:z72i7hdynmk6r22z27h6tvur"
	summaryDID := func() *string {
		user, err := env.core.GetUser(ctx, env.viewer.Id)
		require.NoError(t, err)
		summary, err := userSummary(ctx, env.api, user, nil)
		require.NoError(t, err)
		return summary.AtprotoDid
	}
	require.Nil(t, summaryDID())

	// Other provider types do not publish their subjects.
	require.NoError(t, env.core.LinkExternalIdentity(ctx, "github", config.AuthProviderTypeGitHub, "github", "12345", env.viewer.Id))
	require.Nil(t, summaryDID())

	identity, err := env.core.LinkExternalIdentityAs(ctx, env.viewer.Id, "atproto", config.AuthProviderTypeATProto, "atproto", did, env.viewer.Id)
	require.NoError(t, err)
	require.NotNil(t, summaryDID())
	require.Equal(t, did, *summaryDID())

	require.NoError(t, env.core.DisconnectExternalIdentityAs(ctx, env.viewer.Id, env.viewer.Id, identity.SubjectHash, core.ExternalIdentitySignInPolicy{PasswordLoginEnabled: true, Issuers: []string{"github", "atproto"}}))
	require.Nil(t, summaryDID())
}

// The client writes the account link record only for the DID that the new
// account linked, so account creation returns it (FDR-056).
func TestExternalIdentityAccountCreationReturnsATProtoDID(t *testing.T) {
	env := newConnectAPITestEnv(t)
	for _, tc := range []struct {
		providerType string
		subject      string
		want         *string
	}{
		{providerType: config.AuthProviderTypeATProto, subject: "did:plc:z72i7hdynmk6r22z27h6tvur", want: new("did:plc:z72i7hdynmk6r22z27h6tvur")},
		{providerType: config.AuthProviderTypeOpenIDConnect, subject: "oidc-subject"},
	} {
		token, err := env.core.CreatePendingExternalIdentityCreateFlow(env.ctx, core.PendingExternalIdentityFlow{
			ProviderID: tc.providerType, ProviderType: tc.providerType, Issuer: tc.providerType, Subject: tc.subject,
		})
		require.NoError(t, err)
		resp, err := env.externalAuth.CreateExternalIdentityAccount(env.ctx, connect.NewRequest(&authv1.CreateExternalIdentityAccountRequest{
			Token: token, Login: "new-" + tc.providerType, DisplayName: "New",
		}))
		require.NoError(t, err)
		require.Equal(t, tc.want, resp.Msg.AtprotoDid, tc.providerType)
	}
}
