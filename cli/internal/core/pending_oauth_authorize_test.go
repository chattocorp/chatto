package core

import (
	"errors"
	"strings"
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestPendingOAuthAuthorizeUsesOpaqueSingleUseRuntimeState(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	pending := PendingOAuthAuthorize{
		RedirectURI:         "https://callback.example/" + strings.Repeat("r", 1900),
		CodeChallenge:       "challenge",
		CodeChallengeMethod: "S256",
		State:               "state",
		ClientID:            "https://client.example/" + strings.Repeat("c", 1900),
		ClientName:          "Example Client",
		ClientURI:           "https://client.example",
	}

	token, err := core.CreatePendingOAuthAuthorize(ctx, pending)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || strings.Contains(token, "client.example") {
		t.Fatalf("pending token = %q, want opaque handle", token)
	}
	entry, err := core.storage.runtimeStateKV.Get(ctx, core.pendingOAuthAuthorizeKey(token))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(entry.Key(), token) {
		t.Fatalf("runtime-state key leaked raw pending token: %q", entry.Key())
	}

	loaded, err := core.GetPendingOAuthAuthorize(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RedirectURI != pending.RedirectURI || loaded.ClientID != pending.ClientID {
		t.Fatalf("loaded pending request lost validated values")
	}
	consumed, err := core.ConsumePendingOAuthAuthorize(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if consumed.ClientName != pending.ClientName {
		t.Fatalf("consumed client name = %q", consumed.ClientName)
	}
	if _, err := core.ConsumePendingOAuthAuthorize(ctx, token); !errors.Is(err, ErrPendingOAuthAuthorizeNotFound) {
		t.Fatalf("second consume error = %v, want not found", err)
	}
}

func TestPendingOAuthAuthorizeContinuesAcrossReplicas(t *testing.T) {
	first, nc := setupTestCore(t)
	ctx := testContext(t)
	second, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	pending := PendingOAuthAuthorize{
		RedirectURI:         "com.example.chatto:/oauth/callback",
		CodeChallenge:       "challenge",
		CodeChallengeMethod: "S256",
		ClientID:            "https://mobile.example/oauth/metadata.json",
		ClientName:          "Example Mobile",
		ClientURI:           "https://mobile.example",
	}

	token, err := first.CreatePendingOAuthAuthorize(ctx, pending)
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := second.ConsumePendingOAuthAuthorize(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if consumed.RedirectURI != pending.RedirectURI || consumed.ClientID != pending.ClientID {
		t.Fatalf("second replica loaded %#v, want %#v", consumed, pending)
	}
}
