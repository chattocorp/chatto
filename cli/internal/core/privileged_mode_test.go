package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"hmans.de/chatto/internal/authctx"
)

func TestPermissionResolver_PrivilegedModeGatesElevatedHumanPermissions(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "privileged-owner", "Privileged Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := chatto.AssignOwnerRole(ctx, user.Id); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}

	credentialContext := func(deadline time.Time) context.Context {
		return authctx.WithCredential(ctx, authctx.RuntimeCredential{
			Kind:                    authctx.RuntimeCredentialKindBearerToken,
			UserID:                  user.Id,
			Handle:                  "test-session",
			PrivilegedModeExpiresAt: deadline,
		})
	}

	unarmed := credentialContext(time.Time{})
	if allowed, err := chatto.HasServerPermission(unarmed, user.Id, PermServerManage); err != nil || allowed {
		t.Fatalf("unarmed server.manage = %v, %v; want false, nil", allowed, err)
	}
	mismatched := authctx.WithCredential(ctx, authctx.RuntimeCredential{
		Kind:                    authctx.RuntimeCredentialKindBearerToken,
		UserID:                  "another-user",
		Handle:                  "another-session",
		PrivilegedModeExpiresAt: time.Now().Add(time.Minute),
	})
	if allowed, err := chatto.HasServerPermission(mismatched, user.Id, PermServerManage); err != nil || allowed {
		t.Fatalf("mismatched session server.manage = %v, %v; want false, nil", allowed, err)
	}
	if allowed, err := chatto.HasServerPermission(unarmed, user.Id, PermMessagePost); err != nil || !allowed {
		t.Fatalf("unarmed message.post = %v, %v; want true, nil", allowed, err)
	}
	if available, err := chatto.HasAnyPrivilegedModeEntitlement(unarmed, user.Id); err != nil || !available {
		t.Fatalf("privileged-mode entitlement = %v, %v; want true, nil", available, err)
	}

	armed := credentialContext(time.Now().Add(time.Minute))
	if allowed, err := chatto.HasServerPermission(armed, user.Id, PermServerManage); err != nil || !allowed {
		t.Fatalf("armed server.manage = %v, %v; want true, nil", allowed, err)
	}
	expired := credentialContext(time.Now().Add(-time.Second))
	if allowed, err := chatto.HasServerPermission(expired, user.Id, PermServerManage); err != nil || allowed {
		t.Fatalf("expired server.manage = %v, %v; want false, nil", allowed, err)
	}
}

func TestPermissionResolver_PrivilegedModeTracksEntitlementChanges(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "changing-privileges", "Changing Privileges", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	armed := authctx.WithCredential(ctx, authctx.RuntimeCredential{
		Kind:                    authctx.RuntimeCredentialKindBearerToken,
		UserID:                  user.Id,
		Handle:                  "changing-session",
		PrivilegedModeExpiresAt: time.Now().Add(time.Minute),
	})
	if allowed, err := chatto.HasServerPermission(armed, user.Id, PermRoomCreate); err != nil || allowed {
		t.Fatalf("permission before grant = %v, %v; want false, nil", allowed, err)
	}
	if err := chatto.GrantUserPermission(ctx, SystemActorID, user.Id, PermRoomCreate); err != nil {
		t.Fatalf("GrantUserPermission: %v", err)
	}
	if allowed, err := chatto.HasServerPermission(armed, user.Id, PermRoomCreate); err != nil || !allowed {
		t.Fatalf("permission after grant = %v, %v; want true, nil", allowed, err)
	}
	if err := chatto.DenyUserPermission(ctx, SystemActorID, user.Id, PermRoomCreate); err != nil {
		t.Fatalf("DenyUserPermission: %v", err)
	}
	if allowed, err := chatto.HasServerPermission(armed, user.Id, PermRoomCreate); err != nil || allowed {
		t.Fatalf("permission after deny = %v, %v; want false, nil", allowed, err)
	}
}

func TestPermissionResolver_BotCredentialDoesNotRequirePrivilegedMode(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := chatto.CreateUser(ctx, SystemActorID, "privileged-bot-owner", "Privileged Bot Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := chatto.AssignOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}
	bot, err := chatto.CreateBot(ctx, owner.Id, "privileged_bot", "Privileged Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	if err := chatto.SetUserPermissionState(ctx, owner.Id, bot.User.Id, PermissionTargetScope{Kind: MatrixScopeServer}, PermRoomCreate, PermissionStateAllow); err != nil {
		t.Fatalf("SetUserPermissionState: %v", err)
	}
	botContext := authctx.WithCredential(ctx, authctx.RuntimeCredential{
		Kind:   authctx.RuntimeCredentialKindBotAPIKey,
		UserID: bot.User.Id,
		Handle: bot.APIKey,
	})
	if allowed, err := chatto.HasServerPermission(botContext, bot.User.Id, PermRoomCreate); err != nil || !allowed {
		t.Fatalf("bot room.create = %v, %v; want true, nil", allowed, err)
	}
}

func TestChattoCore_BearerPrivilegedModeUsesRenewableSession(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "privileged-bearer", "Privileged Bearer", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	credentials, err := chatto.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}

	deadline, err := chatto.SetBearerPrivilegedMode(ctx, credentials.AccessToken, true)
	if err != nil {
		t.Fatalf("SetBearerPrivilegedMode(active): %v", err)
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > PrivilegedModeWindow {
		t.Fatalf("activation remaining = %v, want within (0, %v]", remaining, PrivilegedModeWindow)
	}
	validated, err := chatto.ValidatePublicBearerCredential(ctx, credentials.AccessToken)
	if err != nil {
		t.Fatalf("ValidatePublicBearerCredential: %v", err)
	}
	if !validated.PrivilegedModeExpiresAt.Equal(deadline) {
		t.Fatalf("validated deadline = %v, want %v", validated.PrivilegedModeExpiresAt, deadline)
	}
	rotated, err := chatto.RefreshBearerSession(ctx, credentials.RefreshToken, testRefreshRequestIDA, "")
	if err != nil {
		t.Fatalf("RefreshBearerSession: %v", err)
	}
	validated, err = chatto.ValidatePublicBearerCredential(ctx, rotated.AccessToken)
	if err != nil {
		t.Fatalf("ValidatePublicBearerCredential(rotated): %v", err)
	}
	if !validated.PrivilegedModeExpiresAt.Equal(deadline) {
		t.Fatalf("rotated deadline = %v, want %v", validated.PrivilegedModeExpiresAt, deadline)
	}
	retried, err := chatto.SetBearerPrivilegedMode(ctx, credentials.AccessToken, true)
	if err != nil || !retried.Equal(deadline) {
		t.Fatalf("idempotent activation = %v, %v; want %v, nil", retried, err, deadline)
	}
	if deactivated, err := chatto.SetBearerPrivilegedMode(ctx, credentials.AccessToken, false); err != nil || !deactivated.IsZero() {
		t.Fatalf("deactivation = %v, %v; want zero, nil", deactivated, err)
	}
	validated, err = chatto.ValidatePublicBearerCredential(ctx, credentials.AccessToken)
	if err != nil || !validated.PrivilegedModeExpiresAt.IsZero() {
		t.Fatalf("validated deactivation = %v, %v; want zero, nil", validated.PrivilegedModeExpiresAt, err)
	}
}

func TestChattoCore_CookiePrivilegedModeRoundTrip(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "privileged-cookie", "Privileged Cookie", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID, _, err := chatto.CreateCookieSession(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}
	deadline, err := chatto.SetCookiePrivilegedMode(ctx, sessionID, true)
	if err != nil {
		t.Fatalf("SetCookiePrivilegedMode(active): %v", err)
	}
	validated, err := chatto.ValidateCookieCredential(ctx, sessionID)
	if err != nil {
		t.Fatalf("ValidateCookieCredential: %v", err)
	}
	if got := validated.GetPrivilegedModeExpiresAt().AsTime(); !got.Equal(deadline) {
		t.Fatalf("validated deadline = %v, want %v", got, deadline)
	}
	if _, err := chatto.SetCookiePrivilegedMode(ctx, sessionID, false); err != nil {
		t.Fatalf("SetCookiePrivilegedMode(inactive): %v", err)
	}
	validated, err = chatto.ValidateCookieCredential(ctx, sessionID)
	if err != nil || validated.GetPrivilegedModeExpiresAt() != nil {
		t.Fatalf("validated deactivation = %v, %v; want nil, nil", validated.GetPrivilegedModeExpiresAt(), err)
	}
}

func TestChattoCore_CookiePrivilegedModeRejectsRevokedSession(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "revoked-privileged-cookie", "Revoked Privileged Cookie", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID, _, err := chatto.CreateCookieSession(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}
	if err := chatto.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	if _, err := chatto.SetCookiePrivilegedMode(ctx, sessionID, true); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("SetCookiePrivilegedMode error = %v, want ErrCookieSessionNotFound", err)
	}
}
