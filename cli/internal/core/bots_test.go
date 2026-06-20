package core

import (
	"errors"
	"testing"
	"time"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_BotUsersAreFirstClassButTokenOnly(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	owner, err := core.CreateUser(ctx, SystemActorID, "botowner", "Bot Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	bot, err := core.CreateBotUser(ctx, owner.Id, "deploybot", "Deploy Bot", owner.Id)
	if err != nil {
		t.Fatalf("CreateBotUser: %v", err)
	}
	if bot.GetKind() != corev1.UserKind_USER_KIND_BOT {
		t.Fatalf("bot kind = %v, want BOT", bot.GetKind())
	}
	if bot.GetBotOwnerId() != owner.Id {
		t.Fatalf("bot owner = %q, want %q", bot.GetBotOwnerId(), owner.Id)
	}
	if _, err := core.VerifyPassword(ctx, bot.Login, "password123"); err == nil {
		t.Fatal("bot password login unexpectedly succeeded")
	}
	if _, err := core.CreateAuthToken(ctx, bot.Id); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("CreateAuthToken(bot) error = %v, want ErrAuthTokenNotFound", err)
	}

	secret, meta, err := core.CreateBotToken(ctx, owner.Id, bot.Id, "ci", nil)
	if err != nil {
		t.Fatalf("CreateBotToken: %v", err)
	}
	if secret == "" || meta.ID == "" || meta.BotUserID != bot.Id {
		t.Fatalf("unexpected token result secret=%q meta=%#v", secret, meta)
	}
	authUserID, err := core.ValidateAPIAuthToken(ctx, secret)
	if err != nil {
		t.Fatalf("ValidateAPIAuthToken(bot token): %v", err)
	}
	if authUserID != bot.Id {
		t.Fatalf("bot token authenticated as %q, want %q", authUserID, bot.Id)
	}
}

func TestChattoCore_BotTokenMaxTTLPolicy(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	core.config.BotTokenMaxTTL = 24 * time.Hour

	owner, err := core.CreateUser(ctx, SystemActorID, "ttowner", "TTL Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	bot, err := core.CreateBotUser(ctx, owner.Id, "ttlbot", "TTL Bot", owner.Id)
	if err != nil {
		t.Fatalf("CreateBotUser: %v", err)
	}

	if _, _, err := core.CreateBotToken(ctx, owner.Id, bot.Id, "indefinite", nil); !errors.Is(err, ErrBotTokenIndefinite) {
		t.Fatalf("indefinite token error = %v, want ErrBotTokenIndefinite", err)
	}
	tooFar := time.Now().Add(48 * time.Hour)
	if _, _, err := core.CreateBotToken(ctx, owner.Id, bot.Id, "too-far", &tooFar); !errors.Is(err, ErrBotTokenTTLTooLong) {
		t.Fatalf("too-long token error = %v, want ErrBotTokenTTLTooLong", err)
	}
	okExpiry := time.Now().Add(time.Hour)
	if _, _, err := core.CreateBotToken(ctx, owner.Id, bot.Id, "short", &okExpiry); err != nil {
		t.Fatalf("short token should be allowed: %v", err)
	}
}

func TestChattoCore_BotTokenFixedExpiryAndRevocationMetadata(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	owner, err := core.CreateUser(ctx, SystemActorID, "fixedowner", "Fixed Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	bot, err := core.CreateBotUser(ctx, owner.Id, "fixedbot", "Fixed Bot", owner.Id)
	if err != nil {
		t.Fatalf("CreateBotUser: %v", err)
	}
	expiry := time.Now().UTC().Add(2 * time.Hour)
	secret, meta, err := core.CreateBotToken(ctx, owner.Id, bot.Id, "deploy", &expiry)
	if err != nil {
		t.Fatalf("CreateBotToken: %v", err)
	}
	if meta.ExpiresAt == nil || absDuration(meta.ExpiresAt.Sub(expiry)) > time.Second {
		t.Fatalf("expiresAt = %v, want close to %v", meta.ExpiresAt, expiry)
	}

	if _, err := core.ValidateBotToken(ctx, secret); err != nil {
		t.Fatalf("ValidateBotToken: %v", err)
	}
	tokens, err := core.ListBotTokens(ctx, bot.Id)
	if err != nil {
		t.Fatalf("ListBotTokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("tokens len = %d, want 1", len(tokens))
	}
	if tokens[0].LastUsedAt == nil {
		t.Fatal("LastUsedAt = nil, want timestamp after validation")
	}
	if tokens[0].ExpiresAt == nil || absDuration(tokens[0].ExpiresAt.Sub(expiry)) > time.Second {
		t.Fatalf("expiresAt after use = %v, want fixed close to %v", tokens[0].ExpiresAt, expiry)
	}

	if err := core.RevokeBotToken(ctx, owner.Id, bot.Id, meta.ID, "test"); err != nil {
		t.Fatalf("RevokeBotToken: %v", err)
	}
	if _, err := core.ValidateBotToken(ctx, secret); !errors.Is(err, ErrBotTokenNotFound) {
		t.Fatalf("ValidateBotToken(revoked) error = %v, want ErrBotTokenNotFound", err)
	}
	tokens, err = core.ListBotTokens(ctx, bot.Id)
	if err != nil {
		t.Fatalf("ListBotTokens after revoke: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("tokens len after revoke = %d, want 1", len(tokens))
	}
	if tokens[0].RevokedAt == nil || tokens[0].RevokedBy != owner.Id || tokens[0].RevokeReason != "test" {
		t.Fatalf("revoked metadata = %#v, want revoked by owner with reason", tokens[0])
	}
}

func TestChattoCore_DeleteUserCascadesOwnedBots(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	owner, err := core.CreateUser(ctx, SystemActorID, "cascadeowner", "Cascade Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	bot, err := core.CreateBotUser(ctx, owner.Id, "cascadebot", "Cascade Bot", owner.Id)
	if err != nil {
		t.Fatalf("CreateBotUser: %v", err)
	}
	secret, _, err := core.CreateBotToken(ctx, owner.Id, bot.Id, "webhook", nil)
	if err != nil {
		t.Fatalf("CreateBotToken: %v", err)
	}

	if err := core.DeleteUser(ctx, owner.Id, owner.Id); err != nil {
		t.Fatalf("DeleteUser owner: %v", err)
	}
	if _, err := core.GetUser(ctx, bot.Id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetUser(bot after cascade) error = %v, want ErrNotFound", err)
	}
	if _, err := core.ValidateBotToken(ctx, secret); !errors.Is(err, ErrBotTokenNotFound) {
		t.Fatalf("ValidateBotToken(after cascade) error = %v, want ErrBotTokenNotFound", err)
	}
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
