package core

import (
	"errors"
	"testing"
)

func TestReactionModel_AddAndRemoveReaction(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, eventID := setupReactionTest(t, core, ctx)
	service := core.ReactionModel()

	added, err := service.AddReaction(ctx, ReactionMutationInput{
		ActorID:        user.Id,
		RoomID:         room.Id,
		MessageEventID: eventID,
		Emoji:          "thumbsup",
	})
	if err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	if !added {
		t.Fatal("AddReaction added = false, want true")
	}

	added, err = service.AddReaction(ctx, ReactionMutationInput{
		ActorID:        user.Id,
		RoomID:         room.Id,
		MessageEventID: eventID,
		Emoji:          "thumbsup",
	})
	if err != nil {
		t.Fatalf("duplicate AddReaction: %v", err)
	}
	if added {
		t.Fatal("duplicate AddReaction added = true, want false")
	}

	removed, err := service.RemoveReaction(ctx, ReactionMutationInput{
		ActorID:        user.Id,
		RoomID:         room.Id,
		MessageEventID: eventID,
		Emoji:          "thumbsup",
	})
	if err != nil {
		t.Fatalf("RemoveReaction: %v", err)
	}
	if !removed {
		t.Fatal("RemoveReaction removed = false, want true")
	}

	removed, err = service.RemoveReaction(ctx, ReactionMutationInput{
		ActorID:        user.Id,
		RoomID:         room.Id,
		MessageEventID: eventID,
		Emoji:          "thumbsup",
	})
	if err != nil {
		t.Fatalf("duplicate RemoveReaction: %v", err)
	}
	if removed {
		t.Fatal("duplicate RemoveReaction removed = true, want false")
	}
}

func TestReactionModel_AuthorizationAndValidation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, eventID := setupReactionTest(t, core, ctx)
	service := core.ReactionModel()

	t.Run("requires actor", func(t *testing.T) {
		_, err := service.AddReaction(ctx, ReactionMutationInput{
			RoomID:         room.Id,
			MessageEventID: eventID,
			Emoji:          "thumbsup",
		})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Fatalf("error = %v, want ErrNotAuthenticated", err)
		}
	})

	t.Run("requires message event ID", func(t *testing.T) {
		_, err := service.AddReaction(ctx, ReactionMutationInput{
			ActorID: user.Id,
			RoomID:  room.Id,
			Emoji:   "thumbsup",
		})
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("error = %v, want ErrInvalidArgument", err)
		}
	})

	t.Run("requires emoji", func(t *testing.T) {
		_, err := service.AddReaction(ctx, ReactionMutationInput{
			ActorID:        user.Id,
			RoomID:         room.Id,
			MessageEventID: eventID,
		})
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("error = %v, want ErrInvalidArgument", err)
		}
	})

	t.Run("requires membership", func(t *testing.T) {
		outsider, err := core.CreateUser(ctx, "system", "reaction-outsider", "Reaction Outsider", "password123")
		if err != nil {
			t.Fatalf("CreateUser outsider: %v", err)
		}

		_, err = service.AddReaction(ctx, ReactionMutationInput{
			ActorID:        outsider.Id,
			RoomID:         room.Id,
			MessageEventID: eventID,
			Emoji:          "thumbsup",
		})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Fatalf("error = %v, want ErrNotRoomMember", err)
		}
	})

	t.Run("requires message.react", func(t *testing.T) {
		if err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReact); err != nil {
			t.Fatalf("DenyRoomPermission: %v", err)
		}

		_, err := service.AddReaction(ctx, ReactionMutationInput{
			ActorID:        user.Id,
			RoomID:         room.Id,
			MessageEventID: eventID,
			Emoji:          "thumbsup",
		})
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("error = %v, want ErrPermissionDenied", err)
		}
	})
}
