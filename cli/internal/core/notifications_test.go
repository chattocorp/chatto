package core

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestCreateNotification(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := context.Background()

	recipientID := "recipient-user"
	actorID := "actor-user"

	t.Run("creates DM notification", func(t *testing.T) {
		notif := &corev1.Notification{
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{
					RoomId: "dm-room-123",
				},
			},
		}

		created, err := core.CreateNotification(ctx, recipientID, actorID, notif)
		if err != nil {
			t.Fatalf("CreateNotification error: %v", err)
		}
		if created == nil {
			t.Fatal("Expected notification to be non-nil")
		}
		if created.Id == "" {
			t.Error("Expected notification to have an ID")
		}
		if created.RecipientId != recipientID {
			t.Errorf("Expected recipient %s, got %s", recipientID, created.RecipientId)
		}
		if created.ActorId != actorID {
			t.Errorf("Expected actor %s, got %s", actorID, created.ActorId)
		}
		if created.CreatedAt == nil {
			t.Error("Expected CreatedAt to be set")
		}

		// Verify it's a DM notification
		dmNotif := created.GetDmMessage()
		if dmNotif == nil {
			t.Error("Expected DM notification payload")
		}
		if dmNotif.RoomId != "dm-room-123" {
			t.Errorf("Expected room ID dm-room-123, got %s", dmNotif.RoomId)
		}
		if _, err := core.storage.runtimeStateKV.Get(ctx, notificationKey(recipientID, created.Id)); err != nil {
			t.Fatalf("expected notification in RUNTIME_STATE: %v", err)
		}
	})

	t.Run("creates mention notification", func(t *testing.T) {
		notif := &corev1.Notification{
			Notification: &corev1.Notification_Mention{
				Mention: &corev1.MentionNotification{
					RoomId:  "room-456",
					EventId: "event-789",
				},
			},
		}

		created, err := core.CreateNotification(ctx, recipientID, actorID, notif)
		if err != nil {
			t.Fatalf("CreateNotification error: %v", err)
		}

		mentionNotif := created.GetMention()
		if mentionNotif == nil {
			t.Fatal("Expected mention notification payload")
		}
		if mentionNotif.RoomId != "room-456" {
			t.Errorf("Expected room ID room-456, got %s", mentionNotif.RoomId)
		}
		if mentionNotif.EventId != "event-789" {
			t.Errorf("Expected event ID event-789, got %s", mentionNotif.EventId)
		}
	})

	t.Run("creates reply notification", func(t *testing.T) {
		notif := &corev1.Notification{
			Notification: &corev1.Notification_Reply{
				Reply: &corev1.ReplyNotification{
					RoomId:      "room-456",
					EventId:     "reply-event",
					InReplyToId: "root-event",
				},
			},
		}

		created, err := core.CreateNotification(ctx, recipientID, actorID, notif)
		if err != nil {
			t.Fatalf("CreateNotification error: %v", err)
		}

		replyNotif := created.GetReply()
		if replyNotif == nil {
			t.Fatal("Expected reply notification payload")
		}
		if replyNotif.InReplyToId != "root-event" {
			t.Errorf("Expected in reply to ID root-event, got %s", replyNotif.InReplyToId)
		}
	})

	t.Run("publishes room message notification routing context", func(t *testing.T) {
		subject := subjects.LiveSyncUserEvent(recipientID, "notification_created")
		sub, err := nc.SubscribeSync(subject)
		if err != nil {
			t.Fatalf("SubscribeSync(%s): %v", subject, err)
		}
		defer sub.Unsubscribe()
		if err := nc.Flush(); err != nil {
			t.Fatalf("Flush subscription: %v", err)
		}

		created, err := core.CreateNotification(ctx, recipientID, actorID, &corev1.Notification{
			Notification: &corev1.Notification_RoomMessage{
				RoomMessage: &corev1.RoomMessageNotification{
					RoomId:  "all-messages-room",
					EventId: "all-messages-event",
				},
			},
		})
		if err != nil {
			t.Fatalf("CreateNotification error: %v", err)
		}

		msg, err := sub.NextMsg(2 * time.Second)
		if err != nil {
			t.Fatalf("waiting for notification_created live event: %v", err)
		}

		var live corev1.LiveEvent
		if err := proto.Unmarshal(msg.Data, &live); err != nil {
			t.Fatalf("unmarshal live event: %v", err)
		}
		event := live.GetNotificationCreated()
		if event == nil {
			t.Fatalf("expected NotificationCreatedEvent, got %T", live.Event)
		}
		if event.NotificationId != created.Id {
			t.Errorf("NotificationId = %q, want %q", event.NotificationId, created.Id)
		}
		if event.RoomId != "all-messages-room" {
			t.Errorf("RoomId = %q, want all-messages-room", event.RoomId)
		}
		if event.EventId != "all-messages-event" {
			t.Errorf("EventId = %q, want all-messages-event", event.EventId)
		}
	})
}

func TestGetNotifications(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userID := "get-notifs-user"

	t.Run("returns empty list when no notifications", func(t *testing.T) {
		notifs, err := core.GetNotifications(ctx, userID)
		if err != nil {
			t.Fatalf("GetNotifications error: %v", err)
		}
		if len(notifs) != 0 {
			t.Errorf("Expected 0 notifications, got %d", len(notifs))
		}
	})

	t.Run("returns notifications in reverse chronological order", func(t *testing.T) {
		// Create three notifications with small delays
		for i := 0; i < 3; i++ {
			notif := &corev1.Notification{
				Notification: &corev1.Notification_DmMessage{
					DmMessage: &corev1.DMMessageNotification{
						RoomId: "room-" + string(rune('a'+i)),
					},
				},
			}
			_, err := core.CreateNotification(ctx, userID, "actor", notif)
			if err != nil {
				t.Fatalf("CreateNotification error: %v", err)
			}
			time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
		}

		notifs, err := core.GetNotifications(ctx, userID)
		if err != nil {
			t.Fatalf("GetNotifications error: %v", err)
		}
		if len(notifs) != 3 {
			t.Fatalf("Expected 3 notifications, got %d", len(notifs))
		}

		// Verify order (newest first)
		for i := 1; i < len(notifs); i++ {
			if notifs[i-1].CreatedAt.AsTime().Before(notifs[i].CreatedAt.AsTime()) {
				t.Error("Notifications not in descending chronological order")
			}
		}
	})
}

func TestGetNotification(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userID := "single-notif-user"

	t.Run("returns nil for non-existent notification", func(t *testing.T) {
		notif, err := core.GetNotification(ctx, userID, "non-existent-id")
		if err != nil {
			t.Fatalf("GetNotification error: %v", err)
		}
		if notif != nil {
			t.Error("Expected nil for non-existent notification")
		}
	})

	t.Run("returns existing notification", func(t *testing.T) {
		created, _ := core.CreateNotification(ctx, userID, "actor", &corev1.Notification{
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{RoomId: "test-room"},
			},
		})

		retrieved, err := core.GetNotification(ctx, userID, created.Id)
		if err != nil {
			t.Fatalf("GetNotification error: %v", err)
		}
		if retrieved == nil {
			t.Fatal("Expected notification to be found")
		}
		if retrieved.Id != created.Id {
			t.Errorf("Expected ID %s, got %s", created.Id, retrieved.Id)
		}
	})
}

func TestDismissNotification(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userID := "dismiss-user"

	t.Run("returns false for non-existent notification", func(t *testing.T) {
		dismissed, err := core.DismissNotification(ctx, userID, "non-existent-id")
		if err != nil {
			t.Fatalf("DismissNotification error: %v", err)
		}
		if dismissed {
			t.Error("Expected false for non-existent notification")
		}
	})

	t.Run("dismisses existing notification", func(t *testing.T) {
		created, _ := core.CreateNotification(ctx, userID, "actor", &corev1.Notification{
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{RoomId: "test-room"},
			},
		})

		dismissed, err := core.DismissNotification(ctx, userID, created.Id)
		if err != nil {
			t.Fatalf("DismissNotification error: %v", err)
		}
		if !dismissed {
			t.Error("Expected notification to be dismissed")
		}

		// Verify it's gone
		retrieved, _ := core.GetNotification(ctx, userID, created.Id)
		if retrieved != nil {
			t.Error("Expected notification to be deleted")
		}
	})

	t.Run("returns false when dismissing same notification twice", func(t *testing.T) {
		created, _ := core.CreateNotification(ctx, userID, "actor", &corev1.Notification{
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{RoomId: "double-dismiss"},
			},
		})

		// First dismiss
		_, _ = core.DismissNotification(ctx, userID, created.Id)

		// Second dismiss should return false
		dismissed, err := core.DismissNotification(ctx, userID, created.Id)
		if err != nil {
			t.Fatalf("Second DismissNotification error: %v", err)
		}
		if dismissed {
			t.Error("Expected false when dismissing already dismissed notification")
		}
	})
}

func TestDismissAllNotifications(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userID := "dismiss-all-user"

	t.Run("returns 0 when no notifications", func(t *testing.T) {
		callbacks := make(chan *corev1.Notification, 1)
		core.OnNotificationDismissed = func(ctx context.Context, userID string, notification *corev1.Notification) {
			callbacks <- notification
		}

		count, err := core.DismissAllNotifications(ctx, userID)
		if err != nil {
			t.Fatalf("DismissAllNotifications error: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0, got %d", count)
		}

		select {
		case notification := <-callbacks:
			t.Fatalf("Expected no dismiss callback, got notification %s", notification.Id)
		default:
		}
	})

	t.Run("dismisses all notifications for user and sends dismiss callbacks", func(t *testing.T) {
		callbacks := make(chan *corev1.Notification, 3)
		core.OnNotificationDismissed = func(ctx context.Context, userID string, notification *corev1.Notification) {
			callbacks <- notification
		}

		// Create 3 notifications
		expectedRoomsByID := map[string]string{}
		roomIDs := []string{"room-a", "room-b", "room-c"}
		for i := 0; i < 3; i++ {
			created, err := core.CreateNotification(ctx, userID, "actor", &corev1.Notification{
				Notification: &corev1.Notification_DmMessage{
					DmMessage: &corev1.DMMessageNotification{RoomId: roomIDs[i]},
				},
			})
			if err != nil {
				t.Fatalf("CreateNotification error: %v", err)
			}
			expectedRoomsByID[created.Id] = created.GetDmMessage().RoomId
		}

		count, err := core.DismissAllNotifications(ctx, userID)
		if err != nil {
			t.Fatalf("DismissAllNotifications error: %v", err)
		}
		if count != 3 {
			t.Errorf("Expected 3 dismissed, got %d", count)
		}

		// Verify all are gone
		remaining, _ := core.GetNotifications(ctx, userID)
		if len(remaining) != 0 {
			t.Errorf("Expected 0 remaining, got %d", len(remaining))
		}

		received := map[string]string{}
		for i := 0; i < 3; i++ {
			select {
			case notification := <-callbacks:
				dm := notification.GetDmMessage()
				if dm == nil {
					t.Fatalf("Expected DM notification callback, got %T", notification.Notification)
				}
				received[notification.Id] = dm.RoomId
			case <-time.After(time.Second):
				t.Fatalf("Timed out waiting for dismiss callback %d", i+1)
			}
		}
		if len(received) != len(expectedRoomsByID) {
			t.Fatalf("Expected %d callbacks, got %d", len(expectedRoomsByID), len(received))
		}
		for id, expectedRoom := range expectedRoomsByID {
			if received[id] != expectedRoom {
				t.Errorf("Expected callback for %s with room %s, got %s", id, expectedRoom, received[id])
			}
		}
		select {
		case notification := <-callbacks:
			t.Fatalf("Expected no extra dismiss callback, got notification %s", notification.Id)
		default:
		}
	})
}

func TestHasUnreadNotifications(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userID := "has-unread-user"

	t.Run("returns false when no notifications", func(t *testing.T) {
		has, err := core.HasUnreadNotifications(ctx, userID)
		if err != nil {
			t.Fatalf("HasUnreadNotifications error: %v", err)
		}
		if has {
			t.Error("Expected false when no notifications")
		}
	})

	t.Run("returns true when has notifications", func(t *testing.T) {
		core.CreateNotification(ctx, userID, "actor", &corev1.Notification{
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{RoomId: "room"},
			},
		})

		has, err := core.HasUnreadNotifications(ctx, userID)
		if err != nil {
			t.Fatalf("HasUnreadNotifications error: %v", err)
		}
		if !has {
			t.Error("Expected true when has notifications")
		}
	})
}

func TestGetNotificationCount(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userID := "count-user"

	t.Run("returns 0 when no notifications", func(t *testing.T) {
		count, err := core.GetNotificationCount(ctx, userID)
		if err != nil {
			t.Fatalf("GetNotificationCount error: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0, got %d", count)
		}
	})

	t.Run("returns correct count", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			core.CreateNotification(ctx, userID, "actor", &corev1.Notification{
				Notification: &corev1.Notification_DmMessage{
					DmMessage: &corev1.DMMessageNotification{RoomId: "room"},
				},
			})
		}

		count, err := core.GetNotificationCount(ctx, userID)
		if err != nil {
			t.Fatalf("GetNotificationCount error: %v", err)
		}
		if count != 5 {
			t.Errorf("Expected 5, got %d", count)
		}
	})
}

func TestNotificationIsolation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userA := "user-a"
	userB := "user-b"

	t.Run("user cannot see other user's notifications", func(t *testing.T) {
		// Create notification for userA
		core.CreateNotification(ctx, userA, "actor", &corev1.Notification{
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{RoomId: "room"},
			},
		})

		// userB should not see userA's notification
		userBNotifs, _ := core.GetNotifications(ctx, userB)
		if len(userBNotifs) != 0 {
			t.Error("userB should not see userA's notifications")
		}

		// userA should see their notification
		userANotifs, _ := core.GetNotifications(ctx, userA)
		if len(userANotifs) != 1 {
			t.Errorf("userA should have 1 notification, got %d", len(userANotifs))
		}
	})

	t.Run("dismissing does not affect other user's notifications", func(t *testing.T) {
		// Clear userA's notifications
		core.DismissAllNotifications(ctx, userA)

		// Create notifications for both users
		core.CreateNotification(ctx, userA, "actor", &corev1.Notification{
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{RoomId: "room"},
			},
		})
		core.CreateNotification(ctx, userB, "actor", &corev1.Notification{
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{RoomId: "room"},
			},
		})

		// Dismiss userA's notifications
		core.DismissAllNotifications(ctx, userA)

		// userB should still have their notification
		userBNotifs, _ := core.GetNotifications(ctx, userB)
		if len(userBNotifs) != 1 {
			t.Errorf("userB should still have 1 notification after userA dismisses, got %d", len(userBNotifs))
		}
	})
}

func TestNotificationTypeName(t *testing.T) {
	tests := []struct {
		name     string
		notif    *corev1.Notification
		expected string
	}{
		{
			name: "dm_message",
			notif: &corev1.Notification{
				Notification: &corev1.Notification_DmMessage{
					DmMessage: &corev1.DMMessageNotification{},
				},
			},
			expected: "dm_message",
		},
		{
			name: "mention",
			notif: &corev1.Notification{
				Notification: &corev1.Notification_Mention{
					Mention: &corev1.MentionNotification{},
				},
			},
			expected: "mention",
		},
		{
			name: "reply",
			notif: &corev1.Notification{
				Notification: &corev1.Notification_Reply{
					Reply: &corev1.ReplyNotification{},
				},
			},
			expected: "reply",
		},
		{
			name: "room_message",
			notif: &corev1.Notification{
				Notification: &corev1.Notification_RoomMessage{
					RoomMessage: &corev1.RoomMessageNotification{},
				},
			},
			expected: "room_message",
		},
		{
			name:     "unknown",
			notif:    &corev1.Notification{},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := notificationTypeName(tt.notif)
			if got != tt.expected {
				t.Errorf("notificationTypeName() = %s, want %s", got, tt.expected)
			}
		})
	}
}
