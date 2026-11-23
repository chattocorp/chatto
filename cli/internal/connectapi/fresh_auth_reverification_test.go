package connectapi

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/authctx"
	"github.com/chattocorp/chatto/cli/internal/config"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
)

// ListExternalIdentities offers re-verification only for a linked OIDC
// provider and only to a cookie session. core.TestFreshAuthReverification
// covers the freshness upgrade itself.
func TestListExternalIdentitiesReverificationAvailable(t *testing.T) {
	const issuer = "https://idp.example"
	directLogin := false
	env := newConnectAPITestEnvWithAuth(t, 30*time.Second, config.AuthConfig{
		DirectLogin: &directLogin,
		Providers: []config.AuthProviderConfig{
			{ID: "sso", Type: config.AuthProviderTypeOpenIDConnect, Label: "SSO", IssuerURL: issuer, ClientID: "client"},
			{ID: "gh", Type: config.AuthProviderTypeGitHub, Label: "GitHub", ClientID: "client", ClientSecret: "secret"},
			{ID: "discord", Type: config.AuthProviderTypeDiscord, Label: "Discord", ClientID: "client", ClientSecret: "secret"},
		},
	})
	ctx := env.ctx
	if err := env.core.LinkExternalIdentity(ctx, "sso", config.AuthProviderTypeOpenIDConnect, issuer, "subject-viewer", env.viewer.Id); err != nil {
		t.Fatalf("LinkExternalIdentity sso: %v", err)
	}
	if err := env.core.LinkExternalIdentity(ctx, "gh", config.AuthProviderTypeGitHub, "gh", "12345", env.viewer.Id); err != nil {
		t.Fatalf("LinkExternalIdentity gh: %v", err)
	}
	cookieHandle, _, err := env.core.CreateCookieSession(ctx, env.viewer.Id, "refresh")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}
	bearer, err := env.core.CreateBearerSessionWithSource(ctx, env.viewer.Id, "refresh")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	cookieCtx := authctx.WithCredential(withCaller(ctx, env.viewer), authctx.RuntimeCredential{
		Kind: authctx.RuntimeCredentialKindCookieSession, UserID: env.viewer.Id, Handle: cookieHandle,
	})

	available := func(t *testing.T, callerCtx context.Context) map[string]bool {
		t.Helper()
		list, err := env.account.ListExternalIdentities(callerCtx, connect.NewRequest(&apiv1.ListExternalIdentitiesRequest{}))
		if err != nil {
			t.Fatalf("ListExternalIdentities: %v", err)
		}
		result := map[string]bool{}
		for _, provider := range list.Msg.GetProviders() {
			result[provider.GetProvider().GetId()] = provider.GetReverificationAvailable()
		}
		return result
	}
	if got := available(t, cookieCtx); !got["sso"] || got["gh"] || got["discord"] {
		t.Fatalf("cookie session reverification_available = %v, want only sso", got)
	}
	if got := available(t, withBearerCredential(ctx, env.viewer, bearer.AccessToken)); got["sso"] || got["gh"] || got["discord"] {
		t.Fatalf("bearer reverification_available = %v, want none", got)
	}
}
