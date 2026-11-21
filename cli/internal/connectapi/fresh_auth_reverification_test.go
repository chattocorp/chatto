package connectapi

import (
	"net/url"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/authctx"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
)

// StartFreshAuthReverification accepts only a cookie session and a linked
// OIDC provider, and binds the handoff to the caller and its return path.
// core.TestFreshAuthReverification covers the freshness upgrade itself.
func TestStartFreshAuthReverification(t *testing.T) {
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
	cookieHandle, _, err := env.core.CreateCookieSession(ctx, env.viewer.Id, "refresh")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}
	callerCtx := authctx.WithCredential(withCaller(ctx, env.viewer), authctx.RuntimeCredential{
		Kind: authctx.RuntimeCredentialKindCookieSession, UserID: env.viewer.Id, Handle: cookieHandle,
	})
	bearer, err := env.core.CreateBearerSessionWithSource(ctx, env.viewer.Id, "refresh")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	bearerCtx := withBearerCredential(ctx, env.viewer, bearer.AccessToken)
	start := func(providerID, redirectPath string) (*connect.Response[apiv1.StartFreshAuthReverificationResponse], error) {
		return env.account.StartFreshAuthReverification(callerCtx, connect.NewRequest(&apiv1.StartFreshAuthReverificationRequest{
			ProviderId:   providerID,
			RedirectPath: redirectPath,
		}))
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
	bearerList, err := env.account.ListExternalIdentities(bearerCtx, connect.NewRequest(&apiv1.ListExternalIdentitiesRequest{}))
	if err != nil {
		t.Fatalf("ListExternalIdentities bearer: %v", err)
	}
	for _, provider := range bearerList.Msg.GetProviders() {
		if provider.GetReverificationAvailable() {
			t.Fatalf("bearer caller sees reverification_available for %s", provider.GetProvider().GetId())
		}
	}
	_, err = env.account.StartFreshAuthReverification(bearerCtx, connect.NewRequest(&apiv1.StartFreshAuthReverificationRequest{ProviderId: "sso"}))
	requireConnectCode(t, err, connect.CodeFailedPrecondition)

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
	if err := env.core.CompleteFreshAuthReverification(ctx, handoff, issuer, "subject-viewer", time.Now(), cookieHandle); err != nil {
		t.Fatalf("CompleteFreshAuthReverification: %v", err)
	}
}
