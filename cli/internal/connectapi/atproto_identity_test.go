package connectapi

import (
	"testing"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
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
