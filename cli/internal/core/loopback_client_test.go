package core

import (
	"errors"
	"testing"
	"time"

	"hmans.de/chatto/internal/config"
)

func TestChattoCore_LoopbackClientRequiresConfiguration(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "loopback-disabled-user", "Loopback User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := chattoCore.RequireOAuthClientAllowed(ctx, config.ChattoLoopbackClientID); !errors.Is(err, ErrOAuthClientBlocked) {
		t.Fatalf("RequireOAuthClientAllowed(disabled loopback) = %v, want ErrOAuthClientBlocked", err)
	}
	if _, err := chattoCore.CreateOAuthBearerSessionForClient(ctx, user.Id, config.ChattoLoopbackClientID, mustCurrentAuthGeneration(t, chattoCore, user.Id)); !errors.Is(err, ErrOAuthClientBlocked) {
		t.Fatalf("CreateOAuthBearerSessionForClient(disabled loopback) = %v, want ErrOAuthClientBlocked", err)
	}
}

func TestChattoCore_DisablingLoopbackClientEndsExistingSessions(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	chattoCore.config.AuthLoopbackClientEnabled = true
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "loopback-revoked-user", "Loopback User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	credentials, err := chattoCore.CreateOAuthBearerSessionForClient(ctx, user.Id, config.ChattoLoopbackClientID, mustCurrentAuthGeneration(t, chattoCore, user.Id))
	if err != nil {
		t.Fatalf("CreateOAuthBearerSessionForClient: %v", err)
	}
	if _, err := chattoCore.ValidateAuthToken(ctx, credentials.AccessToken); err != nil {
		t.Fatalf("ValidateAuthToken(enabled) = %v", err)
	}

	chattoCore.config.AuthLoopbackClientEnabled = false
	if _, err := chattoCore.ValidateAuthToken(ctx, credentials.AccessToken); err == nil {
		t.Fatal("loopback access token remained valid after the client was disabled")
	}
	if _, err := chattoCore.RefreshBearerSession(ctx, credentials.RefreshToken, testRefreshRequestIDA, config.ChattoLoopbackClientID); err == nil {
		t.Fatal("loopback refresh credential remained valid after the client was disabled")
	}
}

func TestChattoCore_LoopbackClientSessionHasFixedLifetime(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	chattoCore.config.AuthLoopbackClientEnabled = true
	chattoCore.config.AuthTokenTTL = 90 * 24 * time.Hour
	chattoCore.config.AuthAccessTokenTTL = time.Hour
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "loopback-lifetime-user", "Loopback User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	before := time.Now()
	initial, err := chattoCore.CreateOAuthBearerSessionForClient(ctx, user.Id, config.ChattoLoopbackClientID, mustCurrentAuthGeneration(t, chattoCore, user.Id))
	if err != nil {
		t.Fatalf("CreateOAuthBearerSessionForClient: %v", err)
	}
	after := time.Now()
	if initial.SessionExpiresAt.Before(before.Add(config.ChattoLoopbackSessionLifetime)) ||
		initial.SessionExpiresAt.After(after.Add(config.ChattoLoopbackSessionLifetime)) {
		t.Fatalf("loopback session expiry = %v, want %v after issuance", initial.SessionExpiresAt, config.ChattoLoopbackSessionLifetime)
	}

	// A refresh inside the normal renewal quarter must not extend the window.
	nearExpiry := initial.SessionExpiresAt.Add(-30 * time.Minute)
	rotated, err := chattoCore.refreshBearerSessionAt(ctx, initial.RefreshToken, testRefreshRequestIDA, config.ChattoLoopbackClientID, nearExpiry)
	if err != nil {
		t.Fatalf("refreshBearerSessionAt: %v", err)
	}
	if !rotated.SessionExpiresAt.Equal(initial.SessionExpiresAt) {
		t.Fatalf("refreshed loopback session expiry = %v, want fixed %v", rotated.SessionExpiresAt, initial.SessionExpiresAt)
	}
	if !rotated.AccessTokenExpiresAt.Equal(initial.SessionExpiresAt) {
		t.Fatalf("access expiry = %v, want capped at session end %v", rotated.AccessTokenExpiresAt, initial.SessionExpiresAt)
	}
}
