package graph

import (
	"testing"

	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// MessagePostedEvent.Reactions Field Resolver Tests
// ============================================================================

func TestMessagePostedEventResolver_Reactions(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.MessagePostedEvent()

	// Post a message to get an event with a real event ID
	event, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Hello world", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}

	msgEvent := event.Event.(*corev1.Event_MessagePosted).MessagePosted
	// PostMessage doesn't set EventId on the inner event; it's on the SpaceEvent wrapper.
	// Set it manually so resolvers can use it for reactions lookups.
	msgEvent.EventId = event.Id

	t.Run("no reactions returns empty list", func(t *testing.T) {
		reactions, err := resolver.Reactions(env.authContext(), msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(reactions) != 0 {
			t.Errorf("expected empty reactions, got %d", len(reactions))
		}
	})

	t.Run("returns reactions after adding one", func(t *testing.T) {
		_, err := env.core.AddReaction(env.ctx, env.testSpace.Id, env.testRoom.Id, event.Id, "thumbsup", env.testUser.Id)
		if err != nil {
			t.Fatalf("failed to add reaction: %v", err)
		}

		reactions, err := resolver.Reactions(env.authContext(), msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(reactions) != 1 {
			t.Fatalf("expected 1 reaction group, got %d", len(reactions))
		}
		if reactions[0].Emoji != "thumbsup" {
			t.Errorf("expected emoji 'thumbsup', got %s", reactions[0].Emoji)
		}
		if reactions[0].Count != 1 {
			t.Errorf("expected count 1, got %d", reactions[0].Count)
		}
		if !reactions[0].HasReacted {
			t.Error("expected hasReacted true for the user who reacted")
		}
	})

	t.Run("hasReacted is false for other user", func(t *testing.T) {
		otherUser, err := env.core.CreateUser(env.ctx, "system", "other-reaction", "Other Reaction", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		reactions, err := resolver.Reactions(env.authContextForUser(otherUser), msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(reactions) != 1 {
			t.Fatalf("expected 1 reaction group, got %d", len(reactions))
		}
		if reactions[0].HasReacted {
			t.Error("expected hasReacted false for user who didn't react")
		}
	})

	t.Run("unauthenticated returns empty list", func(t *testing.T) {
		reactions, err := resolver.Reactions(env.unauthContext(), msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(reactions) != 0 {
			t.Errorf("expected empty reactions for unauthenticated, got %d", len(reactions))
		}
	})
}

// ============================================================================
// MessagePostedEvent.Body Field Resolver Tests
// ============================================================================

func TestMessagePostedEventResolver_Body(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.MessagePostedEvent()

	event, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Test body content", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}

	msgEvent := event.Event.(*corev1.Event_MessagePosted).MessagePosted

	t.Run("resolves message body", func(t *testing.T) {
		body, err := resolver.Body(env.ctx, msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if body == nil {
			t.Fatal("expected body, got nil")
		}
		if *body != "Test body content" {
			t.Errorf("expected 'Test body content', got %s", *body)
		}
	})
}

// ============================================================================
// MessagePostedEvent.InReplyTo Field Resolver Tests
// ============================================================================

func TestMessagePostedEventResolver_InReplyTo(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.MessagePostedEvent()

	t.Run("returns nil for root message", func(t *testing.T) {
		msgEvent := &corev1.MessagePostedEvent{
			InReplyTo: "",
		}
		result, err := resolver.InReplyTo(env.ctx, msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil for root message, got %s", *result)
		}
	})

	t.Run("returns event ID for reply", func(t *testing.T) {
		msgEvent := &corev1.MessagePostedEvent{
			InReplyTo: "some-event-id",
		}
		result, err := resolver.InReplyTo(env.ctx, msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if *result != "some-event-id" {
			t.Errorf("expected 'some-event-id', got %s", *result)
		}
	})
}

// ============================================================================
// MessagePostedEvent.InThread Field Resolver Tests
// ============================================================================

func TestMessagePostedEventResolver_InThread(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.MessagePostedEvent()

	t.Run("returns nil for root message", func(t *testing.T) {
		msgEvent := &corev1.MessagePostedEvent{
			InThread: "",
		}
		result, err := resolver.InThread(env.ctx, msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil for root message, got %s", *result)
		}
	})

	t.Run("returns thread root ID for thread reply", func(t *testing.T) {
		msgEvent := &corev1.MessagePostedEvent{
			InThread: "thread-root-id",
		}
		result, err := resolver.InThread(env.ctx, msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if *result != "thread-root-id" {
			t.Errorf("expected 'thread-root-id', got %s", *result)
		}
	})
}

// ============================================================================
// MessagePostedEvent.ReplyCount Field Resolver Tests
// ============================================================================

func TestMessagePostedEventResolver_ReplyCount(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.MessagePostedEvent()

	t.Run("thread reply returns 0", func(t *testing.T) {
		msgEvent := &corev1.MessagePostedEvent{
			InThread: "some-thread-root",
		}
		count, err := resolver.ReplyCount(env.ctx, msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 for thread reply, got %d", count)
		}
	})

	t.Run("root message with no replies returns 0", func(t *testing.T) {
		event, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Root msg", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post message: %v", err)
		}
		msgEvent := event.Event.(*corev1.Event_MessagePosted).MessagePosted
		msgEvent.EventId = event.Id

		count, err := resolver.ReplyCount(env.ctx, msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 replies, got %d", count)
		}
	})
}

// ============================================================================
// PresenceChangedEvent.Status Field Resolver Tests
// ============================================================================

func TestPresenceChangedEventResolver_Status(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.PresenceChangedEvent()

	tests := []struct {
		input    string
		expected model.PresenceStatus
	}{
		{"ONLINE", model.PresenceStatusOnline},
		{"OFFLINE", model.PresenceStatusOffline},
		{"AWAY", model.PresenceStatusAway},
		{"DO_NOT_DISTURB", model.PresenceStatusDoNotDisturb},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			event := &corev1.PresenceChangedEvent{
				Status: tt.input,
			}
			status, err := resolver.Status(env.ctx, event)
			if err != nil {
				t.Fatalf("expected success, got error: %v", err)
			}
			if status != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, status)
			}
		})
	}
}

// ============================================================================
// SpaceEvent.Actor Field Resolver Tests
// ============================================================================

func TestSpaceEventResolver_Actor(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.RoomEvent()

	t.Run("resolves actor when present", func(t *testing.T) {
		event := &corev1.Event{
			ActorId: env.testUser.Id,
		}
		user, err := resolver.Actor(env.ctx, event)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if user == nil {
			t.Fatal("expected user, got nil")
		}
		if user.Id != env.testUser.Id {
			t.Errorf("expected user ID %s, got %s", env.testUser.Id, user.Id)
		}
	})

	t.Run("returns nil when no actor", func(t *testing.T) {
		event := &corev1.Event{
			ActorId: "",
		}
		user, err := resolver.Actor(env.ctx, event)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if user != nil {
			t.Errorf("expected nil for empty actor, got %+v", user)
		}
	})

	t.Run("returns nil for deleted user", func(t *testing.T) {
		event := &corev1.Event{
			ActorId: "nonexistent-user",
		}
		user, err := resolver.Actor(env.ctx, event)
		if err != nil {
			t.Fatalf("expected success (graceful nil), got error: %v", err)
		}
		if user != nil {
			t.Errorf("expected nil for nonexistent actor, got %+v", user)
		}
	})
}

