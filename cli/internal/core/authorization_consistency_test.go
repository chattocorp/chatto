package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/evtstream"
)

func TestAuthorizeAtStableInputsRepeatsChangedDecision(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	staleDecision := errors.New("stale authorization decision")
	attempts := 0

	err := chattoCore.authorizeAtStableInputs(ctx, func() error {
		attempts++
		if attempts != 1 {
			return nil
		}
		if _, err := chattoCore.CreateUser(ctx, SystemActorID, "stable-auth-race", "Stable Auth Race", "password123"); err != nil {
			return err
		}
		return staleDecision
	})
	if err != nil {
		t.Fatalf("authorizeAtStableInputs: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("authorization attempts = %d, want 2", attempts)
	}
}

func TestAuthorizeAtStableInputsReturnsStableDecision(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	want := errors.New("stable denial")

	err := chattoCore.authorizeAtStableInputs(ctx, func() error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("authorizeAtStableInputs error = %v, want %v", err, want)
	}
}

func TestAuthorityChangesDoNotWriteLegacyFence(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	before, err := chattoCore.EventPublisher.LastSubjectSeq(ctx, evtstream.AuthorizationSubjectFilter())
	if err != nil {
		t.Fatalf("read legacy authorization fence before mutations: %v", err)
	}

	user, err := chattoCore.CreateUser(ctx, SystemActorID, "no-legacy-fence", "No Legacy Fence", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := chattoCore.GrantUserPermission(ctx, SystemActorID, user.GetId(), PermMessagePost); err != nil {
		t.Fatalf("GrantUserPermission: %v", err)
	}
	group, err := chattoCore.CreateRoomGroup(ctx, SystemActorID, "No Legacy Fence Group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, group.GetId(), "no-legacy-fence-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, user.GetId(), KindChannel, user.GetId(), room.GetId()); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if removed, err := chattoCore.RemoveMember(ctx, SystemActorID, KindChannel, room.GetId(), user.GetId()); err != nil || !removed {
		t.Fatalf("RemoveMember removed=%v error=%v", removed, err)
	}

	after, err := chattoCore.EventPublisher.LastSubjectSeq(ctx, evtstream.AuthorizationSubjectFilter())
	if err != nil {
		t.Fatalf("read legacy authorization fence after mutations: %v", err)
	}
	if after != before {
		t.Fatalf("legacy authorization fence advanced from %d to %d", before, after)
	}
}
