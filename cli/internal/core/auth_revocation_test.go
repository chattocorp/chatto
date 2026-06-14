package core

import (
	"errors"
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_AuthGenerationRejectsStaleTokenIssuance(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "generation-stale-user", "Generation Stale User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	authGeneration, err := core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}
	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	if _, err := core.CreateAuthTokenWithSourceGeneration(ctx, user.Id, "password_login", authGeneration); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("CreateAuthTokenWithSourceGeneration err = %v, want ErrAuthTokenNotFound", err)
	}

	freshGeneration, err := core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration fresh: %v", err)
	}
	if freshGeneration == authGeneration {
		t.Fatal("auth generation should advance after password change")
	}
	if token, err := core.CreateAuthTokenWithSourceGeneration(ctx, user.Id, "password_login", freshGeneration); err != nil {
		t.Fatalf("fresh token issuance should succeed: %v", err)
	} else if gotUserID, err := core.ValidateAuthToken(ctx, token); err != nil {
		t.Fatalf("fresh token should validate: %v", err)
	} else if gotUserID != user.Id {
		t.Fatalf("validated user ID = %q, want %q", gotUserID, user.Id)
	}
}

func TestChattoCore_AuthGenerationRejectsDeletedUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "generation-delete-user", "Generation Delete User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	deletedEvent := newEvent("system", &corev1.Event{Event: &corev1.Event_UserAccountDeleted{
		UserAccountDeleted: &corev1.UserAccountDeletedEvent{UserId: user.Id},
	}})
	if _, err := core.appendUserEvent(ctx, user.Id, deletedEvent, "", nil); err != nil {
		t.Fatalf("append delete event: %v", err)
	}

	if _, err := core.ValidateAuthToken(ctx, token); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("ValidateAuthToken err = %v, want ErrAuthTokenNotFound", err)
	}
}
