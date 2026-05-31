package core

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_ServerBrandingUsesConfigEvents(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	logo := &corev1.DeprecatedAsset{Asset: &corev1.DeprecatedAsset_Nats{Nats: &corev1.NATSAsset{Key: "logo-asset"}}}
	if err := core.SetServerLogo(ctx, "admin", logo); err != nil {
		t.Fatalf("SetServerLogo failed: %v", err)
	}

	got, err := core.GetServerLogo(ctx)
	if err != nil {
		t.Fatalf("GetServerLogo failed: %v", err)
	}
	if !proto.Equal(logo, got) {
		t.Fatalf("GetServerLogo = %+v, want %+v", got, logo)
	}
	if _, err := core.storage.serverKV.Get(ctx, serverLogoKey); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("legacy logo KV key exists or failed unexpectedly: %v", err)
	}
	cfg, err := core.ConfigManager().GetServerConfig(ctx)
	if err != nil {
		t.Fatalf("GetServerConfig after logo failed: %v", err)
	}
	if cfg != nil {
		t.Fatalf("logo-only update wrote server config: cfg=%+v", cfg)
	}
	blocked, err := core.ConfigManager().GetEffectiveBlockedUsernames(ctx)
	if err != nil {
		t.Fatalf("GetEffectiveBlockedUsernames after logo failed: %v", err)
	}
	if blocked != DefaultBlockedUsernames {
		t.Fatalf("logo-only update changed effective blocked usernames: got %q", blocked)
	}

	msgs := eventStreamMsgCount(t, core)
	if err := core.SetServerLogo(ctx, "admin", logo); err != nil {
		t.Fatalf("SetServerLogo same value failed: %v", err)
	}
	if after := eventStreamMsgCount(t, core); after != msgs {
		t.Fatalf("same logo write published events: before %d after %d", msgs, after)
	}

	if err := core.DeleteServerLogo(ctx, "admin"); err != nil {
		t.Fatalf("DeleteServerLogo failed: %v", err)
	}
	got, err = core.GetServerLogo(ctx)
	if err != nil {
		t.Fatalf("GetServerLogo after delete failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected logo to be cleared, got %+v", got)
	}
}
