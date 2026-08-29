package core

import (
	"context"
	"errors"
	"testing"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/pkg/events"
)

type interceptingReactionMutationExecutor struct {
	delegate          reactionMutationExecutor
	beforeFirstCommit func(context.Context) error
	attempts          int
	intercepted       bool
}

func (e *interceptingReactionMutationExecutor) ExecuteMutation(
	ctx context.Context,
	boundary events.MutationBoundary,
	decide func(context.Context, events.MutationAttempt) ([]evtstream.MutationEntry, error),
) (events.MutationResult, error) {
	return e.delegate.ExecuteMutation(ctx, boundary, func(ctx context.Context, attempt events.MutationAttempt) ([]evtstream.MutationEntry, error) {
		e.attempts++
		entries, err := decide(ctx, attempt)
		if err != nil || len(entries) == 0 || e.intercepted {
			return entries, err
		}
		e.intercepted = true
		if err := e.beforeFirstCommit(ctx); err != nil {
			return nil, err
		}
		return entries, nil
	})
}

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

	t.Run("requires message.read", func(t *testing.T) {
		if err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageRead); err != nil {
			t.Fatalf("DenyRoomPermission: %v", err)
		}
		if err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReadInteractions); err != nil {
			t.Fatalf("DenyRoomPermission message.read-interactions: %v", err)
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
		if err := core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageRead); err != nil {
			t.Fatalf("ClearRoomPermissionState: %v", err)
		}
		if err := core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReadInteractions); err != nil {
			t.Fatalf("ClearRoomPermissionState message.read-interactions: %v", err)
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

func TestReactionModel_AllowsAuthorizedAttemptAcrossConcurrentPermissionChange(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, eventID := setupReactionTest(t, core, ctx)
	service := core.ReactionModel()
	reactionSubject := evtstream.RoomAggregate(room.Id).Subject(evtstream.EventReactionAdded)
	beforeSeq, err := core.EventPublisher.LastSubjectSeq(ctx, reactionSubject)
	if err != nil {
		t.Fatalf("read reaction subject before mutation: %v", err)
	}

	executor := &interceptingReactionMutationExecutor{
		delegate: core.EventPublisher,
		beforeFirstCommit: func(ctx context.Context) error {
			return core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReact)
		},
	}
	service.mutations = executor

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
	if executor.attempts != 1 {
		t.Fatalf("mutation attempts = %d, want 1", executor.attempts)
	}
	afterSeq, err := core.EventPublisher.LastSubjectSeq(ctx, reactionSubject)
	if err != nil {
		t.Fatalf("read reaction subject after mutation: %v", err)
	}
	if afterSeq <= beforeSeq {
		t.Fatalf("reaction subject remained at %d after authorized mutation", afterSeq)
	}

	added, err = service.AddReaction(ctx, ReactionMutationInput{
		ActorID:        user.Id,
		RoomID:         room.Id,
		MessageEventID: eventID,
		Emoji:          "heart",
	})
	if added {
		t.Fatal("subsequent AddReaction added = true after permission revocation")
	}
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("subsequent AddReaction error = %v, want ErrPermissionDenied", err)
	}
}
