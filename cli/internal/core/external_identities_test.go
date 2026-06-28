package core

import (
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestChattoCore_PendingExternalIdentityCreateFlow(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	token, err := core.CreatePendingExternalIdentityCreateFlow(ctx, PendingExternalIdentityFlow{
		ProviderID:    "github-main",
		ProviderType:  "github",
		ProviderLabel: "GitHub",
		Issuer:        "github-main",
		Subject:       "12345",
		VerifiedEmail: "external@example.com",
		LoginHint:     "external",
	})
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityCreateFlow: %v", err)
	}

	key := core.externalIdentityCreateTokenKey(token)
	if _, err := core.storage.runtimeStateKV.Get(ctx, key); err != nil {
		t.Fatalf("expected pending external identity flow in RUNTIME_STATE: %v", err)
	}
	assertRuntimeKVHasTTL(t, core, key)
	assertRawRuntimeTokenKeyAbsent(t, core, externalIdentityCreateTokenKeyPrefix+token)

	flow, err := core.GetPendingExternalIdentityCreateFlow(ctx, token)
	if err != nil {
		t.Fatalf("GetPendingExternalIdentityCreateFlow: %v", err)
	}
	if flow.Kind != ExternalIdentityFlowKindCreate || flow.ProviderID != "github-main" || flow.SubjectHash == "" {
		t.Fatalf("flow = %+v", flow)
	}

	user, err := core.CreateUserForExternalIdentity(ctx, "externaluser", "External User", flow)
	if err != nil {
		t.Fatalf("CreateUserForExternalIdentity: %v", err)
	}
	found, err := core.GetUserByExternalIdentity(ctx, "github-main", "12345")
	if err != nil {
		t.Fatalf("GetUserByExternalIdentity: %v", err)
	}
	if found == nil || found.Id != user.Id {
		t.Fatalf("external identity lookup = %v, want user %s", found, user.Id)
	}
	emails, err := core.GetVerifiedEmails(ctx, user.Id)
	if err != nil {
		t.Fatalf("GetVerifiedEmails: %v", err)
	}
	if len(emails) != 1 || emails[0].Email != "external@example.com" {
		t.Fatalf("verified emails = %+v", emails)
	}

	if err := core.DeletePendingExternalIdentityFlow(ctx, token); err != nil {
		t.Fatalf("DeletePendingExternalIdentityFlow: %v", err)
	}
	if _, err := core.GetPendingExternalIdentityFlow(ctx, token); !errors.Is(err, ErrExternalIdentityFlowNotFound) {
		t.Fatalf("GetPendingExternalIdentityFlow after delete error = %v, want ErrExternalIdentityFlowNotFound", err)
	}
}

func TestChattoCore_PendingExternalIdentityLinkStart(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	token, err := core.CreatePendingExternalIdentityLinkStart(ctx, "github-main", "/chat/-/settings/account", "U1")
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityLinkStart: %v", err)
	}

	key := core.externalIdentityLinkStartKey(token)
	if _, err := core.storage.runtimeStateKV.Get(ctx, key); err != nil {
		t.Fatalf("expected pending external identity link start in RUNTIME_STATE: %v", err)
	}
	assertRuntimeKVHasTTL(t, core, key)
	assertRawRuntimeTokenKeyAbsent(t, core, externalIdentityLinkStartKeyPrefix+token)

	start, err := core.ConsumePendingExternalIdentityLinkStart(ctx, token)
	if err != nil {
		t.Fatalf("ConsumePendingExternalIdentityLinkStart: %v", err)
	}
	if start.ProviderID != "github-main" || start.BoundUserID != "U1" || start.RedirectPath != "/chat/-/settings/account" {
		t.Fatalf("link start = %+v", start)
	}
	if _, err := core.ConsumePendingExternalIdentityLinkStart(ctx, token); !errors.Is(err, ErrExternalIdentityFlowNotFound) {
		t.Fatalf("ConsumePendingExternalIdentityLinkStart after delete error = %v, want ErrExternalIdentityFlowNotFound", err)
	}
}

func TestChattoCore_PendingExternalIdentityLinkFlowIsUserBound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "linkuser", "Link User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	other, err := core.CreateUser(ctx, SystemActorID, "otherlinkuser", "Other Link User", "password123")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}

	token, err := core.CreatePendingExternalIdentityLinkFlow(ctx, PendingExternalIdentityFlow{
		ProviderID:   "discord-main",
		ProviderType: "discord",
		Issuer:       "discord-main",
		Subject:      "abc123",
	}, user.Id)
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityLinkFlow: %v", err)
	}

	if _, err := core.GetPendingExternalIdentityCreateFlow(ctx, token); !errors.Is(err, ErrExternalIdentityFlowNotFound) {
		t.Fatalf("GetPendingExternalIdentityCreateFlow on link token error = %v, want ErrExternalIdentityFlowNotFound", err)
	}
	if _, err := core.GetPendingExternalIdentityLinkFlow(ctx, token, other.Id); !errors.Is(err, ErrExternalIdentityFlowUserBound) {
		t.Fatalf("GetPendingExternalIdentityLinkFlow wrong user error = %v, want ErrExternalIdentityFlowUserBound", err)
	}

	flow, err := core.GetPendingExternalIdentityLinkFlow(ctx, token, user.Id)
	if err != nil {
		t.Fatalf("GetPendingExternalIdentityLinkFlow: %v", err)
	}
	identity, err := core.LinkPendingExternalIdentity(ctx, user.Id, flow)
	if err != nil {
		t.Fatalf("LinkPendingExternalIdentity: %v", err)
	}
	if identity.ProviderID != "discord-main" || identity.SubjectHash == "" {
		t.Fatalf("identity = %+v", identity)
	}
	found, err := core.GetUserByExternalIdentity(ctx, "discord-main", "abc123")
	if err != nil {
		t.Fatalf("GetUserByExternalIdentity: %v", err)
	}
	if found == nil || found.Id != user.Id {
		t.Fatalf("external identity lookup = %v, want user %s", found, user.Id)
	}
}

func TestChattoCore_PendingExternalIdentityFlowExpiresByCreatedAt(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	token, err := core.CreatePendingExternalIdentityCreateFlow(ctx, PendingExternalIdentityFlow{
		ProviderID:   "github-main",
		ProviderType: "github",
		Issuer:       "github-main",
		Subject:      "expired",
		CreatedAt:    time.Now().Add(-ExternalIdentityFlowTTL * 2),
	})
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityCreateFlow: %v", err)
	}
	if _, err := core.GetPendingExternalIdentityFlow(ctx, token); !errors.Is(err, ErrExternalIdentityFlowExpired) {
		t.Fatalf("GetPendingExternalIdentityFlow expired error = %v, want ErrExternalIdentityFlowExpired", err)
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, core.externalIdentityCreateTokenKey(token)); !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		t.Fatalf("expired flow still present in RUNTIME_STATE: %v", err)
	}
}
