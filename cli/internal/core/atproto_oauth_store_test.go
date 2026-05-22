package core

import (
	"errors"
	"testing"

	atprotov1 "hmans.de/chatto/internal/pb/chatto/atproto/v1"
)

func TestATProtoAuthRequestStore_RoundTrip(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	req := &atprotov1.ATProtoAuthRequest{
		State:                        "state-abc",
		AuthServerUrl:                "https://bsky.social",
		AccountDid:                   "did:plc:alice",
		Scopes:                       []string{"atproto", "account:email"},
		RequestUri:                   "urn:ietf:params:oauth:request_uri:xyz",
		AuthServerTokenEndpoint:      "https://bsky.social/oauth/token",
		AuthServerRevocationEndpoint: "https://bsky.social/oauth/revoke",
		PkceVerifier:                 "verifier-blob",
		DpopAuthServerNonce:          "nonce-1",
		DpopPrivateKeyMultibase:      "z-fake-multibase-key",
	}

	if err := core.SaveATProtoAuthRequest(ctx, req); err != nil {
		t.Fatalf("SaveATProtoAuthRequest: %v", err)
	}

	got, err := core.GetATProtoAuthRequest(ctx, "state-abc")
	if err != nil {
		t.Fatalf("GetATProtoAuthRequest: %v", err)
	}
	if got.State != req.State || got.AuthServerUrl != req.AuthServerUrl ||
		got.AccountDid != req.AccountDid || got.RequestUri != req.RequestUri ||
		got.PkceVerifier != req.PkceVerifier || got.DpopPrivateKeyMultibase != req.DpopPrivateKeyMultibase {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, req)
	}
	if len(got.Scopes) != len(req.Scopes) {
		t.Fatalf("scope count mismatch: got %v want %v", got.Scopes, req.Scopes)
	}

	if err := core.DeleteATProtoAuthRequest(ctx, "state-abc"); err != nil {
		t.Fatalf("DeleteATProtoAuthRequest: %v", err)
	}
	if _, err := core.GetATProtoAuthRequest(ctx, "state-abc"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestATProtoAuthRequestStore_GetMissing(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if _, err := core.GetATProtoAuthRequest(ctx, "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestATProtoAuthRequestStore_DeleteMissingIsIdempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if err := core.DeleteATProtoAuthRequest(ctx, "never-saved"); err != nil {
		t.Fatalf("delete of missing entry should be idempotent: %v", err)
	}
}

func TestATProtoAuthRequestStore_RequiresState(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	err := core.SaveATProtoAuthRequest(ctx, &atprotov1.ATProtoAuthRequest{})
	if err == nil {
		t.Fatal("expected error when saving without state")
	}
}

func TestATProtoSessionStore_RoundTrip(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	sess := &atprotov1.ATProtoSession{
		AccountDid:                   "did:plc:alice",
		SessionId:                    "sess-123",
		HostUrl:                      "https://alice.host.example",
		AuthServerUrl:                "https://bsky.social",
		AuthServerTokenEndpoint:      "https://bsky.social/oauth/token",
		AuthServerRevocationEndpoint: "https://bsky.social/oauth/revoke",
		Scopes:                       "atproto account:email",
		AccessToken:                  "access-token-blob",
		RefreshToken:                 "refresh-token-blob",
		DpopAuthServerNonce:          "as-nonce",
		DpopHostNonce:                "host-nonce",
		DpopPrivateKeyMultibase:      "z-fake-key",
	}

	if err := core.SaveATProtoSession(ctx, sess); err != nil {
		t.Fatalf("SaveATProtoSession: %v", err)
	}

	got, err := core.GetATProtoSession(ctx, sess.AccountDid, sess.SessionId)
	if err != nil {
		t.Fatalf("GetATProtoSession: %v", err)
	}
	if got.AccessToken != sess.AccessToken || got.RefreshToken != sess.RefreshToken ||
		got.DpopHostNonce != sess.DpopHostNonce || got.Scopes != sess.Scopes {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, sess)
	}

	// Upsert: saving again with new tokens should overwrite, not fail.
	sess.AccessToken = "rotated-access-token"
	sess.DpopHostNonce = "rotated-host-nonce"
	if err := core.SaveATProtoSession(ctx, sess); err != nil {
		t.Fatalf("SaveATProtoSession (upsert): %v", err)
	}
	got2, err := core.GetATProtoSession(ctx, sess.AccountDid, sess.SessionId)
	if err != nil {
		t.Fatalf("GetATProtoSession after upsert: %v", err)
	}
	if got2.AccessToken != "rotated-access-token" || got2.DpopHostNonce != "rotated-host-nonce" {
		t.Fatalf("upsert didn't update fields: got %+v", got2)
	}

	if err := core.DeleteATProtoSession(ctx, sess.AccountDid, sess.SessionId); err != nil {
		t.Fatalf("DeleteATProtoSession: %v", err)
	}
	if _, err := core.GetATProtoSession(ctx, sess.AccountDid, sess.SessionId); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestATProtoSessionStore_RequiresIDs(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	cases := []struct {
		name string
		in   *atprotov1.ATProtoSession
	}{
		{"no DID", &atprotov1.ATProtoSession{SessionId: "x"}},
		{"no session ID", &atprotov1.ATProtoSession{AccountDid: "did:plc:x"}},
		{"both empty", &atprotov1.ATProtoSession{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := core.SaveATProtoSession(ctx, tc.in); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestATProtoSessionStore_DifferentDIDsDontCollide(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Same session_id under different DIDs should be independent entries.
	for _, did := range []string{"did:plc:alice", "did:plc:bob"} {
		sess := &atprotov1.ATProtoSession{
			AccountDid:  did,
			SessionId:   "shared-session-id",
			AccessToken: "token-for-" + did,
		}
		if err := core.SaveATProtoSession(ctx, sess); err != nil {
			t.Fatalf("save for %s: %v", did, err)
		}
	}

	alice, err := core.GetATProtoSession(ctx, "did:plc:alice", "shared-session-id")
	if err != nil {
		t.Fatalf("get alice: %v", err)
	}
	bob, err := core.GetATProtoSession(ctx, "did:plc:bob", "shared-session-id")
	if err != nil {
		t.Fatalf("get bob: %v", err)
	}
	if alice.AccessToken == bob.AccessToken {
		t.Fatalf("expected independent entries, both got %q", alice.AccessToken)
	}
}
