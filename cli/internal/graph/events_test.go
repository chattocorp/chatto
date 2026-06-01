package graph

import (
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func graphMessagePostedEvent(event *corev1.Event) *model.MessagePostedEvent {
	payload := event.GetMessagePosted()
	return &model.MessagePostedEvent{
		Envelope: event,
		Payload:  payload,
		RoomID:   payload.GetRoomId(),
	}
}

func graphMessagePostedPayload(id string, payload *corev1.MessagePostedEvent) *model.MessagePostedEvent {
	return &model.MessagePostedEvent{
		Envelope: &corev1.Event{Id: id},
		Payload:  payload,
		RoomID:   payload.GetRoomId(),
	}
}

// ============================================================================
// MessagePostedEvent.Reactions Field Resolver Tests
// ============================================================================

func TestMessagePostedEventResolver_Reactions(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.MessagePostedEvent()

	// Post a message to get an event with a real event ID
	event, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Hello world", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}

	msgEvent := graphMessagePostedEvent(event)

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
		_, err := env.core.AddReaction(env.ctx, core.KindChannel, env.testRoom.Id, event.Id, "thumbsup", env.testUser.Id)
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

	event, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Test body content", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}

	msgEvent := graphMessagePostedEvent(event)

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

func TestAttachmentResolver_VideoProcessingFromManifest(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.Attachment()
	attachment := &corev1.Attachment{
		Id:          "A-video",
		RoomId:      env.testRoom.Id,
		Filename:    "video.mp4",
		ContentType: "video/mp4",
	}

	empty, err := resolver.VideoProcessing(env.authContext(), attachment)
	if err != nil {
		t.Fatalf("VideoProcessing without manifest returned error: %v", err)
	}
	if empty != nil {
		t.Fatalf("VideoProcessing without manifest = %+v, want nil", empty)
	}

	event := &corev1.Event{
		Id: "ENV-VIDEO",
		Event: &corev1.Event_AssetProcessingSucceeded{
			AssetProcessingSucceeded: &corev1.AssetProcessingSucceededEvent{
				AssetId: attachment.Id,
				Video: &corev1.AssetProcessedVideo{
					DurationMs:       1234,
					Width:            640,
					Height:           360,
					ThumbnailAssetId: "A-thumb",
					Variants: []*corev1.AssetVideoVariant{{
						Quality: "480p",
						AssetId: "A-480",
					}},
				},
			},
		},
	}
	if err := env.core.RoomTimeline.Apply(testAssetCreatedEvent(env.testRoom.Id, attachment.Id, attachment.ContentType), 1); err != nil {
		t.Fatalf("Apply asset creation: %v", err)
	}
	if err := env.core.RoomTimeline.Apply(testDerivativeAssetCreatedEvent("A-480", attachment.Id, "480p", 854, 480, 42), 1); err != nil {
		t.Fatalf("Apply derivative asset creation: %v", err)
	}
	if err := env.core.RoomTimeline.Apply(event, 1); err != nil {
		t.Fatalf("Apply video manifest: %v", err)
	}

	got, err := resolver.VideoProcessing(env.authContext(), attachment)
	if err != nil {
		t.Fatalf("VideoProcessing with manifest returned error: %v", err)
	}
	if got == nil || got.Status != model.VideoProcessingStatusCompleted {
		t.Fatalf("VideoProcessing = %+v, want completed", got)
	}
	if got.DurationMs == nil || *got.DurationMs != 1234 {
		t.Fatalf("DurationMs = %v, want 1234", got.DurationMs)
	}
	if len(got.Variants) != 1 || got.Variants[0].Quality != "480p" {
		t.Fatalf("Variants = %+v, want 480p", got.Variants)
	}
	if !got.SourceAvailable {
		t.Fatal("SourceAvailable = false, want true")
	}
}

func TestAttachmentResolver_VideoProcessingFailedManifest(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.Attachment()
	attachment := &corev1.Attachment{
		Id:          "A-video",
		RoomId:      env.testRoom.Id,
		Filename:    "video.mp4",
		ContentType: "video/mp4",
	}
	event := &corev1.Event{
		Id: "ENV-VIDEO-FAILED",
		Event: &corev1.Event_AssetProcessingFailed{
			AssetProcessingFailed: &corev1.AssetProcessingFailedEvent{
				AssetId:     attachment.Id,
				FailureCode: corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING,
			},
		},
	}
	if err := env.core.RoomTimeline.Apply(testAssetCreatedEvent(env.testRoom.Id, attachment.Id, attachment.ContentType), 1); err != nil {
		t.Fatalf("Apply asset creation: %v", err)
	}
	if err := env.core.RoomTimeline.Apply(event, 1); err != nil {
		t.Fatalf("Apply video failure: %v", err)
	}

	got, err := resolver.VideoProcessing(env.authContext(), attachment)
	if err != nil {
		t.Fatalf("VideoProcessing failed manifest returned error: %v", err)
	}
	if got == nil || got.Status != model.VideoProcessingStatusFailed {
		t.Fatalf("VideoProcessing = %+v, want failed", got)
	}
	if got.SourceAvailable {
		t.Fatal("SourceAvailable = true, want false")
	}
	if got.ReasonCode == nil || *got.ReasonCode != "original_missing" {
		t.Fatalf("ReasonCode = %v, want original_missing", got.ReasonCode)
	}
}

// TestAssetProcessingSucceededEventResolver_MessageEventID verifies the
// messageEventId is read directly off the event (stamped at publish time by
// the scheduler/worker), not resolved via a projection lookup that would race
// the owning message's own projection. The frontend keys its refetch on it.
func TestAssetProcessingSucceededEventResolver_MessageEventID(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.AssetProcessingSucceededEvent()

	// The owning message id is carried on the event — the resolver returns it
	// without consulting the projection.
	got, err := resolver.MessageEventID(env.authContext(), &corev1.AssetProcessingSucceededEvent{AssetId: "A-video", MessageEventId: "M1"})
	if err != nil {
		t.Fatalf("MessageEventID returned error: %v", err)
	}
	if got != "M1" {
		t.Fatalf("MessageEventID = %q, want M1 (read off the event)", got)
	}

	// One-shot migration events don't carry it; the resolver yields empty.
	if got, err := resolver.MessageEventID(env.authContext(), &corev1.AssetProcessingSucceededEvent{AssetId: "A-video"}); err != nil {
		t.Fatalf("MessageEventID returned error: %v", err)
	} else if got != "" {
		t.Fatalf("MessageEventID without stamp = %q, want empty", got)
	}
}

func testAssetCreatedEvent(roomID, attachmentID, contentType string) *corev1.Event {
	return &corev1.Event{
		Id: "ENV-DECLARED-" + attachmentID,
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				OriginalBinaryAvailable: true,
				Asset: &corev1.AssetRecord{
					Id:          attachmentID,
					ContentType: contentType,
				},
				RoomId: roomID,
			},
		},
	}
}

func testDerivativeAssetCreatedEvent(assetID, parentAssetID, quality string, width, height int32, size int64) *corev1.Event {
	_ = quality
	return &corev1.Event{
		Id: "ENV-DERIVATIVE-" + assetID,
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				OriginalBinaryAvailable: true,
				Asset: &corev1.AssetRecord{
					Id:          assetID,
					ContentType: "video/mp4",
					Size:        size,
					Width:       width,
					Height:      height,
				},
				ParentAssetId:  parentAssetID,
				DerivativeRole: corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_VIDEO_VARIANT,
			},
		},
	}
}

// ============================================================================
// MessagePostedEvent.InReplyTo Field Resolver Tests
// ============================================================================

func TestMessagePostedEventResolver_InReplyTo(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.MessagePostedEvent()

	t.Run("returns nil for root message", func(t *testing.T) {
		msgEvent := graphMessagePostedPayload("", &corev1.MessagePostedEvent{
			InReplyTo: "",
		})
		result, err := resolver.InReplyTo(env.ctx, msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil for root message, got %s", *result)
		}
	})

	t.Run("returns event ID for reply", func(t *testing.T) {
		msgEvent := graphMessagePostedPayload("", &corev1.MessagePostedEvent{
			InReplyTo: "some-event-id",
		})
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
// MessagePostedEvent.ThreadRootEventID Field Resolver Tests
// ============================================================================

func TestMessagePostedEventResolver_ThreadRootEventID(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.MessagePostedEvent()

	t.Run("returns nil for root message", func(t *testing.T) {
		msgEvent := graphMessagePostedPayload("", &corev1.MessagePostedEvent{
			InThread: "",
		})
		result, err := resolver.ThreadRootEventID(env.ctx, msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil for root message, got %s", *result)
		}
	})

	t.Run("returns thread root ID for thread reply", func(t *testing.T) {
		msgEvent := graphMessagePostedPayload("", &corev1.MessagePostedEvent{
			InThread: "thread-root-id",
		})
		result, err := resolver.ThreadRootEventID(env.ctx, msgEvent)
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
		msgEvent := graphMessagePostedPayload("", &corev1.MessagePostedEvent{
			InThread: "some-thread-root",
		})
		count, err := resolver.ReplyCount(env.ctx, msgEvent)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 for thread reply, got %d", count)
		}
	})

	t.Run("root message with no replies returns 0", func(t *testing.T) {
		event, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Root msg", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post message: %v", err)
		}
		msgEvent := graphMessagePostedEvent(event)

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
// Event.Actor Field Resolver Tests
// ============================================================================

func TestEventResolver_Actor(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.Event()

	t.Run("resolves actor when present", func(t *testing.T) {
		event := &corev1.Event{
			ActorId: env.testUser.Id,
		}
		user, err := resolver.Actor(env.ctx, core.NewEVTEventEnvelope(event))
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
		user, err := resolver.Actor(env.ctx, core.NewEVTEventEnvelope(event))
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
		user, err := resolver.Actor(env.ctx, core.NewEVTEventEnvelope(event))
		if err != nil {
			t.Fatalf("expected success (graceful nil), got error: %v", err)
		}
		if user != nil {
			t.Errorf("expected nil for nonexistent actor, got %+v", user)
		}
	})
}
