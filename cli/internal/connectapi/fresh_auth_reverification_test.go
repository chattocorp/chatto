package connectapi

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
)

// On an SSO-only server, a stale bearer session cannot link another provider.
// Re-verification through a linked OIDC provider of the same account upgrades
// that session, and the link then starts without a password. An identity of
// another account does not upgrade it.
func TestStartFreshAuthReverificationResumesLink(t *testing.T) {
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

	other, err := env.core.CreateUser(ctx, core.SystemActorID, "reverify-other", "Other", "")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	if err := env.core.LinkExternalIdentity(ctx, "sso", config.AuthProviderTypeOpenIDConnect, issuer, "subject-other", other.Id); err != nil {
		t.Fatalf("LinkExternalIdentity other: %v", err)
	}
	// A source without a sign-in suffix does not grant initial freshness.
	session, err := env.core.CreateBearerSessionWithSource(ctx, env.viewer.Id, "refresh")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	callerCtx := withBearerCredential(ctx, env.viewer, session.AccessToken)
	start := func(providerID, redirectPath string) (*connect.Response[apiv1.StartFreshAuthReverificationResponse], error) {
		return env.account.StartFreshAuthReverification(callerCtx, connect.NewRequest(&apiv1.StartFreshAuthReverificationRequest{
			ProviderId:   providerID,
			RedirectPath: redirectPath,
		}))
	}
	startLink := func() error {
		_, err := env.account.StartExternalIdentityLink(callerCtx, connect.NewRequest(&apiv1.StartExternalIdentityLinkRequest{
			ProviderId:   "discord",
			RedirectPath: "/chat/-/settings/account",
		}))
		return err
	}

	_, err = start("sso", "/chat/-/settings/account")
	requireConnectCode(t, err, connect.CodeFailedPrecondition)

	if err := env.core.LinkExternalIdentity(ctx, "sso", config.AuthProviderTypeOpenIDConnect, issuer, "subject-viewer", env.viewer.Id); err != nil {
		t.Fatalf("LinkExternalIdentity viewer sso: %v", err)
	}
	if err := env.core.LinkExternalIdentity(ctx, "gh", config.AuthProviderTypeGitHub, "gh", "12345", env.viewer.Id); err != nil {
		t.Fatalf("LinkExternalIdentity viewer gh: %v", err)
	}

	list, err := env.account.ListExternalIdentities(callerCtx, connect.NewRequest(&apiv1.ListExternalIdentitiesRequest{}))
	if err != nil {
		t.Fatalf("ListExternalIdentities: %v", err)
	}
	available := map[string]bool{}
	for _, provider := range list.Msg.GetProviders() {
		available[provider.GetProvider().GetId()] = provider.GetReverificationAvailable()
	}
	if !available["sso"] || available["gh"] || available["discord"] {
		t.Fatalf("reverification_available = %v, want only sso", available)
	}

	for _, tc := range []struct {
		name, providerID, redirect string
		want                       connect.Code
	}{
		{"provider without forced sign-in", "gh", "/chat/-/settings/account", connect.CodeFailedPrecondition},
		{"unlinked provider", "discord", "/chat/-/settings/account", connect.CodeFailedPrecondition},
		{"unknown provider", "missing", "/chat/-/settings/account", connect.CodeNotFound},
		{"external redirect", "sso", "//evil.example/", connect.CodeInvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := start(tc.providerID, tc.redirect)
			requireConnectCode(t, err, tc.want)
		})
	}

	requireConnectCode(t, startLink(), connect.CodeFailedPrecondition)

	reverify := func(t *testing.T, subject string) error {
		t.Helper()
		started, err := start("sso", "/chat/-/settings/account?link_provider=discord")
		if err != nil {
			t.Fatalf("StartFreshAuthReverification: %v", err)
		}
		startURL, err := url.Parse(started.Msg.GetStartUrl())
		if err != nil {
			t.Fatal(err)
		}
		if startURL.Path != "/auth/providers/sso" || startURL.Query().Get("intent") != "reverify" {
			t.Fatalf("start url = %q", started.Msg.GetStartUrl())
		}
		handoff, err := env.core.ConsumeFreshAuthReverificationStart(ctx, startURL.Query().Get("reverify_start"))
		if err != nil {
			t.Fatalf("ConsumeFreshAuthReverificationStart: %v", err)
		}
		if handoff.BoundUserID != env.viewer.Id || handoff.ProviderID != "sso" || handoff.RedirectPath != "/chat/-/settings/account?link_provider=discord" {
			t.Fatalf("handoff = %+v", handoff)
		}
		return env.core.CompleteFreshAuthReverification(ctx, handoff, issuer, subject, time.Now(), "")
	}

	t.Run("identity of another account", func(t *testing.T) {
		if err := reverify(t, "subject-other"); !errors.Is(err, core.ErrFreshAuthReverificationMismatch) {
			t.Fatalf("CompleteFreshAuthReverification = %v, want mismatch", err)
		}
		requireConnectCode(t, startLink(), connect.CodeFailedPrecondition)
	})

	t.Run("same account", func(t *testing.T) {
		if err := reverify(t, "subject-viewer"); err != nil {
			t.Fatalf("CompleteFreshAuthReverification: %v", err)
		}
		if err := startLink(); err != nil {
			t.Fatalf("StartExternalIdentityLink after re-verification: %v", err)
		}
	})
}
