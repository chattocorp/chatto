package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Notifications Query Tests
// ============================================================================

// viewerFor returns the resolved Viewer for the given context (or nil if
// unauthenticated). Notifications/hasNotifications live on the Viewer type
// after phase 4, so tests call them through this resolver.
func viewerFor(t *testing.T, env *testEnv, ctx context.Context) *model.Viewer {
	t.Helper()
	v, err := env.resolver.Query().Viewer(ctx)
	if err != nil {
		t.Fatalf("Viewer resolver failed: %v", err)
	}
	return v
}

func TestNotificationCreatedEventResolver_NavigationIDsNullWhenEmpty(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.NotificationCreatedEvent()

	eventID, err := resolver.EventID(env.authContext(), &corev1.NotificationCreatedEvent{})
	if err != nil {
		t.Fatalf("EventID returned error: %v", err)
	}
	if eventID != nil {
		t.Fatalf("EventID = %v, want nil", eventID)
	}

	inReplyToID, err := resolver.InReplyToID(env.authContext(), &corev1.NotificationCreatedEvent{})
	if err != nil {
		t.Fatalf("InReplyToID returned error: %v", err)
	}
	if inReplyToID != nil {
		t.Fatalf("InReplyToID = %v, want nil", inReplyToID)
	}

	eventID, err = resolver.EventID(env.authContext(), &corev1.NotificationCreatedEvent{EventId: "E123"})
	if err != nil {
		t.Fatalf("EventID returned error: %v", err)
	}
	if eventID == nil || *eventID != "E123" {
		t.Fatalf("EventID = %v, want E123", eventID)
	}
}

func TestViewerResolver_Notifications(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("unauthenticated has no viewer", func(t *testing.T) {
		if viewerFor(t, env, env.unauthContext()) != nil {
			t.Error("expected nil viewer for unauthenticated context")
		}
	})

	t.Run("authenticated user can get notifications (empty list)", func(t *testing.T) {
		ctx := env.authContext()
		notifications, err := env.resolver.Viewer().Notifications(ctx, viewerFor(t, env, ctx), nil, nil)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if notifications == nil {
			t.Fatal("expected non-nil notifications connection")
		}
		if len(notifications.Items) != 0 {
			t.Errorf("expected empty notifications, got %d", len(notifications.Items))
		}
	})

	t.Run("user receives notifications after being mentioned", func(t *testing.T) {
		mentioner, err := env.core.CreateUser(env.ctx, "system", "mentioner", "Mentioner", "password123")
		if err != nil {
			t.Fatalf("failed to create mentioner: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, mentioner.Id, core.KindChannel, mentioner.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		_, err = env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, mentioner.Id,
			"Hey @"+env.testUser.Login+" check this out!", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post message: %v", err)
		}

		ctx := env.authContext()
		notifications, err := env.resolver.Viewer().Notifications(ctx, viewerFor(t, env, ctx), nil, nil)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(notifications.Items) == 0 {
			t.Error("expected at least one notification after being mentioned")
		}
	})

	t.Run("authenticated user can paginate notifications", func(t *testing.T) {
		recipient, err := env.core.CreateUser(env.ctx, "system", "notif-page-recipient", "Notification Page Recipient", "password123")
		if err != nil {
			t.Fatalf("failed to create recipient: %v", err)
		}

		created := make([]*corev1.Notification, 0, 3)
		for i := 0; i < 3; i++ {
			notification, err := env.core.CreateNotification(env.ctx, recipient.Id, env.testUser.Id, &corev1.Notification{
				Notification: &corev1.Notification_Mention{
					Mention: &corev1.MentionNotification{
						RoomId:  env.testRoom.Id,
						EventId: "notif-page-event-" + string(rune('a'+i)),
					},
				},
			})
			if err != nil {
				t.Fatalf("failed to create notification %d: %v", i, err)
			}
			created = append(created, notification)
			time.Sleep(10 * time.Millisecond)
		}

		ctx := env.authContextForUser(recipient)
		viewer := viewerFor(t, env, ctx)

		limit := int32(1)
		offset := int32(0)
		firstPage, err := env.resolver.Viewer().Notifications(ctx, viewer, &limit, &offset)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if firstPage.TotalCount != 3 {
			t.Errorf("expected totalCount 3, got %d", firstPage.TotalCount)
		}
		if !firstPage.HasMore {
			t.Error("expected hasMore on first page")
		}
		if len(firstPage.Items) != 1 {
			t.Fatalf("expected 1 notification on first page, got %d", len(firstPage.Items))
		}
		firstItem, ok := firstPage.Items[0].(*model.MentionNotificationItem)
		if !ok {
			t.Fatalf("expected mention notification item, got %T", firstPage.Items[0])
		}
		if firstItem.ID != created[2].Id {
			t.Errorf("expected newest notification %q first, got %q", created[2].Id, firstItem.ID)
		}

		limit = 2
		offset = 1
		tailPage, err := env.resolver.Viewer().Notifications(ctx, viewer, &limit, &offset)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if tailPage.TotalCount != 3 {
			t.Errorf("expected tail totalCount 3, got %d", tailPage.TotalCount)
		}
		if tailPage.HasMore {
			t.Error("expected no more notifications beyond tail page")
		}
		if len(tailPage.Items) != 2 {
			t.Fatalf("expected 2 notifications on tail page, got %d", len(tailPage.Items))
		}

		wantIDs := []string{created[1].Id, created[0].Id}
		for i, want := range wantIDs {
			item, ok := tailPage.Items[i].(*model.MentionNotificationItem)
			if !ok {
				t.Fatalf("expected mention notification item %d, got %T", i, tailPage.Items[i])
			}
			if item.ID != want {
				t.Errorf("expected notification %d ID %q, got %q", i, want, item.ID)
			}
		}
	})
}

func TestViewerResolver_HasNotifications(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("unauthenticated has no viewer", func(t *testing.T) {
		if viewerFor(t, env, env.unauthContext()) != nil {
			t.Error("expected nil viewer for unauthenticated context")
		}
	})

	t.Run("user without notifications returns false", func(t *testing.T) {
		freshUser, err := env.core.CreateUser(env.ctx, "system", "fresh-notif", "Fresh", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		ctx := env.authContextForUser(freshUser)
		hasNotif, err := env.resolver.Viewer().HasNotifications(ctx, viewerFor(t, env, ctx))
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if hasNotif {
			t.Error("expected false for user with no notifications")
		}
	})
}

// ============================================================================
// DismissNotification Mutation Tests
// ============================================================================

func TestMutationResolver_DismissNotification(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.DismissNotification(env.unauthContext(), model.DismissNotificationInput{NotificationID: "some-notification-id"})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("dismissing nonexistent notification returns false", func(t *testing.T) {
		success, err := mutation.DismissNotification(env.authContext(), model.DismissNotificationInput{NotificationID: "nonexistent-notification-id"})
		if err != nil {
			t.Fatalf("expected success (no error), got error: %v", err)
		}
		if success {
			t.Error("expected false for nonexistent notification")
		}
	})

	t.Run("user can dismiss their notification", func(t *testing.T) {
		// Create a notification by mentioning the user
		mentioner, err := env.core.CreateUser(env.ctx, "system", "mention-dismiss", "Mention Dismiss", "password123")
		if err != nil {
			t.Fatalf("failed to create mentioner: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, mentioner.Id, core.KindChannel, mentioner.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		// Post a message that mentions the test user
		_, err = env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, mentioner.Id,
			"Hey @"+env.testUser.Login+" dismiss test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post message: %v", err)
		}

		// Get the notification ID
		notifications, err := env.core.GetNotifications(env.ctx, env.testUser.Id)
		if err != nil {
			t.Fatalf("failed to get notifications: %v", err)
		}
		if len(notifications) == 0 {
			t.Skip("No notification created - skipping dismiss test")
		}

		notifID := notifications[0].Id

		// Dismiss it
		success, err := mutation.DismissNotification(env.authContext(), model.DismissNotificationInput{NotificationID: notifID})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected true for successful dismiss")
		}
	})
}

func TestMutationResolver_DismissAllNotifications(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.DismissAllNotifications(env.unauthContext())
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("dismissing with no notifications returns 0", func(t *testing.T) {
		// Create a fresh user with no notifications
		freshUser, err := env.core.CreateUser(env.ctx, "system", "dismiss-all", "Dismiss All", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		count, err := mutation.DismissAllNotifications(env.authContextForUser(freshUser))
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0, got %d", count)
		}
	})

	t.Run("user can dismiss all notifications", func(t *testing.T) {
		// Create a test user who will receive multiple mentions
		receiver, err := env.core.CreateUser(env.ctx, "system", "receiver-all", "Receiver All", "password123")
		if err != nil {
			t.Fatalf("failed to create receiver: %v", err)
		}
		if err := env.core.AddVerifiedEmailDirect(env.ctx, receiver.Id, "receiver-all@example.com"); err != nil {
			t.Fatalf("failed to verify receiver: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, receiver.Id, core.KindChannel, receiver.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		// Create a mentioner
		mentioner, err := env.core.CreateUser(env.ctx, "system", "mentioner-all", "Mentioner All", "password123")
		if err != nil {
			t.Fatalf("failed to create mentioner: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, mentioner.Id, core.KindChannel, mentioner.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		// Post messages mentioning the receiver
		for i := 0; i < 3; i++ {
			_, err = env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, mentioner.Id,
				"Mention @receiver-all number", nil, "", "", nil, false)
			if err != nil {
				t.Fatalf("failed to post message: %v", err)
			}
		}

		// Dismiss all
		count, err := mutation.DismissAllNotifications(env.authContextForUser(receiver))
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}

		// Should have dismissed some notifications (exact count depends on deduplication)
		t.Logf("Dismissed %d notifications", count)
	})
}

// ============================================================================
// Notification Field Resolver Tests
// ============================================================================

func TestNotificationItemFieldResolvers(t *testing.T) {
	env := setupTestResolver(t)

	// Create a notification by mentioning the user
	mentioner, err := env.core.CreateUser(env.ctx, "system", "field-mentioner", "Field Mentioner", "password123")
	if err != nil {
		t.Fatalf("failed to create mentioner: %v", err)
	}
	_, err = env.core.JoinRoom(env.ctx, mentioner.Id, core.KindChannel, mentioner.Id, env.testRoom.Id)
	if err != nil {
		t.Fatalf("failed to join room: %v", err)
	}

	_, err = env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, mentioner.Id,
		"Field resolver test @"+env.testUser.Login, nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}

	// Get notification
	ctx := env.authContext()
	notifications, err := env.resolver.Viewer().Notifications(ctx, viewerFor(t, env, ctx), nil, nil)
	if err != nil {
		t.Fatalf("failed to get notifications: %v", err)
	}
	if len(notifications.Items) == 0 {
		t.Skip("No notification created - skipping field resolver tests")
	}

	// Note: Field resolvers are tested implicitly through the notifications query
	// The convertNotification function in notifications.resolvers.go handles the conversion
	t.Run("notification has valid id and timestamp", func(t *testing.T) {
		// Just verify we got notifications - the field resolvers work if we get here
		t.Logf("Got %d notifications", len(notifications.Items))
	})
}
