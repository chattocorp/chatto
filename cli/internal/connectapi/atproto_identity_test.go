package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
)

func TestMyAccountServiceSetAndClearAtprotoIdentity(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	const did = "did:plc:z72i7hdynmk6r22z27h6tvur"

	_, err := env.account.SetAtprotoIdentity(env.ctx, connect.NewRequest(&apiv1.SetAtprotoIdentityRequest{Did: did}))
	requireConnectCode(t, err, connect.CodeUnauthenticated)
	_, err = env.account.SetAtprotoIdentity(ctx, connect.NewRequest(&apiv1.SetAtprotoIdentityRequest{Did: "alice.bsky.social"}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)

	resp, err := env.account.SetAtprotoIdentity(ctx, connect.NewRequest(&apiv1.SetAtprotoIdentityRequest{Did: did}))
	if err != nil {
		t.Fatalf("SetAtprotoIdentity: %v", err)
	}
	if resp.Msg.GetDid() != did {
		t.Fatalf("did = %q, want %q", resp.Msg.GetDid(), did)
	}

	// The DID is part of the public user resource.
	user, err := env.core.GetUser(ctx, env.viewer.Id)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := userSummary(ctx, env.api, user, nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.GetAtprotoDid() != did {
		t.Fatalf("summary atproto_did = %q, want %q", summary.GetAtprotoDid(), did)
	}

	if _, err := env.account.ClearAtprotoIdentity(ctx, connect.NewRequest(&apiv1.ClearAtprotoIdentityRequest{})); err != nil {
		t.Fatalf("ClearAtprotoIdentity: %v", err)
	}
	user, err = env.core.GetUser(ctx, env.viewer.Id)
	if err != nil {
		t.Fatal(err)
	}
	summary, err = userSummary(ctx, env.api, user, nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.AtprotoDid != nil {
		t.Fatalf("summary atproto_did after clear = %q, want absent", summary.GetAtprotoDid())
	}
}
