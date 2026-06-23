package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Notification Key Helpers
// ============================================================================

const (
	notificationTTL       = 90 * 24 * time.Hour
	notificationKeyPrefix = "notification."
)

// notificationKey returns the KV key for a notification.
// Format: notification.{userId}.{notificationId}
func notificationKey(userID, notificationID string) string {
	return fmt.Sprintf("%s%s.%s", notificationKeyPrefix, userID, notificationID)
}

// notificationKeyFilter returns the NATS subject filter for all notifications for a user.
// Uses NATS subject wildcard syntax: "notification.userID.*" matches all keys for the user.
func notificationKeyFilter(userID string) string {
	return notificationKeyPrefix + userID + ".*"
}

// ============================================================================
// Notification CRUD Operations
// ============================================================================

// CreateNotification creates a new notification and publishes a sync event.
// The notification is stored in RUNTIME_STATE with a per-key TTL.
// Authorization: Internal use only - called by message posting logic.
//
// The notification parameter should already have its oneof payload set.
// Example: &corev1.Notification{Notification: &corev1.Notification_DmMessage{...}}
func (c *ChattoCore) CreateNotification(
	ctx context.Context,
	recipientID, actorID string,
	notification *corev1.Notification,
) (*corev1.Notification, error) {
	notificationID := NewNotificationID()
	now := time.Now()

	// Set/override common fields
	notification.Id = notificationID
	notification.RecipientId = recipientID
	notification.CreatedAt = timestamppb.New(now)
	notification.ActorId = actorID

	// Store in KV
	data, err := proto.Marshal(notification)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal notification: %w", err)
	}

	key := notificationKey(recipientID, notificationID)
	_, err = c.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(notificationTTL))
	if err != nil {
		return nil, fmt.Errorf("failed to store notification: %w", err)
	}

	// Publish sync event to recipient for real-time delivery
	c.publishNotificationCreatedEvent(ctx, notification)

	// Call the notification callback for push notifications (if set)
	// Run asynchronously to avoid blocking notification creation if push is slow
	if c.OnNotificationCreated != nil {
		go c.OnNotificationCreated(context.WithoutCancel(ctx), notification)
	}

	c.logger.Debug("Notification created",
		"notification_id", notificationID,
		"recipient_id", recipientID,
		"type", notificationTypeName(notification))

	return notification, nil
}

// GetNotifications returns all notifications for a user, ordered by creation time (newest first).
// Authorization: Caller must verify userID matches authenticated user.
func (c *ChattoCore) GetNotifications(ctx context.Context, userID string) ([]*corev1.Notification, error) {
	prefix := notificationKeyFilter(userID)
	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, prefix)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return []*corev1.Notification{}, nil
		}
		return nil, fmt.Errorf("failed to list notification keys: %w", err)
	}

	var notifications []*corev1.Notification
	for key := range lister.Keys() {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			c.logger.Warn("Failed to get notification", "key", key, "error", err)
			continue
		}

		var notif corev1.Notification
		if err := proto.Unmarshal(entry.Value(), &notif); err != nil {
			c.logger.Warn("Failed to unmarshal notification", "key", key, "error", err)
			continue
		}
		notifications = append(notifications, &notif)
	}

	// Sort by created_at descending (newest first)
	sort.Slice(notifications, func(i, j int) bool {
		return notifications[i].CreatedAt.AsTime().After(notifications[j].CreatedAt.AsTime())
	})

	return notifications, nil
}

// GetNotification retrieves a single notification.
// Returns nil if the notification doesn't exist.
// Authorization: Caller must verify userID matches authenticated user.
func (c *ChattoCore) GetNotification(ctx context.Context, userID, notificationID string) (*corev1.Notification, error) {
	key := notificationKey(userID, notificationID)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get notification: %w", err)
	}

	var notif corev1.Notification
	if err := proto.Unmarshal(entry.Value(), &notif); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notification: %w", err)
	}

	return &notif, nil
}

// DismissNotification deletes a notification and publishes a sync event.
// Returns true if notification existed and was deleted, false if already dismissed.
// Authorization: Caller must verify userID matches authenticated user.
func (c *ChattoCore) DismissNotification(ctx context.Context, userID, notificationID string) (bool, error) {
	key := notificationKey(userID, notificationID)

	// Fetch notification before deleting (needed for push dismissal callback)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil // Already dismissed
		}
		return false, fmt.Errorf("failed to get notification: %w", err)
	}

	var notif corev1.Notification
	if err := proto.Unmarshal(entry.Value(), &notif); err != nil {
		return false, fmt.Errorf("failed to unmarshal notification: %w", err)
	}

	// Delete
	err = c.storage.runtimeStateKV.Delete(ctx, key)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, fmt.Errorf("failed to delete notification: %w", err)
	}

	// Publish sync event for cross-device sync (WebSocket)
	c.publishNotificationDismissedEvent(ctx, userID, notificationID)

	// Call the notification callback for push dismissal (if set)
	// Run asynchronously to avoid blocking notification dismissal
	if c.OnNotificationDismissed != nil {
		go c.OnNotificationDismissed(context.WithoutCancel(ctx), userID, &notif)
	}

	c.logger.Debug("Notification dismissed",
		"notification_id", notificationID,
		"user_id", userID)

	return true, nil
}

// DismissAllNotifications deletes all notifications for a user.
// Returns the count of deleted notifications.
// Authorization: Caller must verify userID matches authenticated user.
func (c *ChattoCore) DismissAllNotifications(ctx context.Context, userID string) (int, error) {
	prefix := notificationKeyFilter(userID)
	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, prefix)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list notification keys: %w", err)
	}

	// Collect keys first to avoid modifying while iterating
	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}

	deleted := 0
	for _, key := range keys {
		var notif *corev1.Notification
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if !errors.Is(err, jetstream.ErrKeyNotFound) {
				c.logger.Warn("Failed to get notification before dismissing", "key", key, "error", err)
			}
		} else {
			var decoded corev1.Notification
			if err := proto.Unmarshal(entry.Value(), &decoded); err != nil {
				c.logger.Warn("Failed to unmarshal notification before dismissing", "key", key, "error", err)
			} else {
				notif = &decoded
			}
		}

		if err := c.storage.runtimeStateKV.Delete(ctx, key); err != nil {
			if !errors.Is(err, jetstream.ErrKeyNotFound) {
				c.logger.Warn("Failed to delete notification", "key", key, "error", err)
			}
			continue
		}
		deleted++

		// Extract notification ID from key and publish sync event
		// Key format: notification.{userId}.{notificationId}
		keyPrefix := notificationKeyPrefix + userID + "."
		if notificationID := strings.TrimPrefix(key, keyPrefix); notificationID != key {
			c.publishNotificationDismissedEvent(ctx, userID, notificationID)
		}

		if notif != nil && c.OnNotificationDismissed != nil {
			go c.OnNotificationDismissed(context.WithoutCancel(ctx), userID, notif)
		}
	}

	c.logger.Debug("Dismissed all notifications",
		"user_id", userID,
		"count", deleted)

	return deleted, nil
}

// HasUnreadNotifications checks if a user has any notifications.
// Used for the bell icon indicator.
// Authorization: Caller must verify userID matches authenticated user.
func (c *ChattoCore) HasUnreadNotifications(ctx context.Context, userID string) (bool, error) {
	prefix := notificationKeyFilter(userID)
	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, prefix)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check notifications: %w", err)
	}

	// Just need to check if at least one key exists
	for range lister.Keys() {
		return true, nil
	}
	return false, nil
}

// GetNotificationCount returns the count of notifications for a user.
// Authorization: Caller must verify userID matches authenticated user.
func (c *ChattoCore) GetNotificationCount(ctx context.Context, userID string) (int, error) {
	prefix := notificationKeyFilter(userID)
	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, prefix)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to count notifications: %w", err)
	}

	count := 0
	for range lister.Keys() {
		count++
	}
	return count, nil
}

// ============================================================================
// Real-time Sync Events
// ============================================================================

// publishNotificationCreatedEvent publishes a live event for cross-device sync.
func (c *ChattoCore) publishNotificationCreatedEvent(ctx context.Context, notif *corev1.Notification) {
	// Extract navigation context from the notification payload
	var roomID, eventID, inReplyToID string
	switch n := notif.Notification.(type) {
	case *corev1.Notification_DmMessage:
		roomID = n.DmMessage.RoomId
	case *corev1.Notification_Mention:
		roomID = n.Mention.RoomId
		eventID = n.Mention.EventId
	case *corev1.Notification_Reply:
		roomID = n.Reply.RoomId
		eventID = n.Reply.EventId
		inReplyToID = n.Reply.InReplyToId
	case *corev1.Notification_RoomMessage:
		roomID = n.RoomMessage.RoomId
		eventID = n.RoomMessage.EventId
	}

	event := newLiveEvent(notif.ActorId, &corev1.LiveEvent{
		CreatedAt: notif.CreatedAt,
		Event: &corev1.LiveEvent_NotificationCreated{
			NotificationCreated: &corev1.NotificationCreatedEvent{
				NotificationId: notif.Id,
				RoomId:         roomID,
				EventId:        eventID,
				InReplyToId:    inReplyToID,
			},
		},
	})

	subject := subjects.LiveSyncUserEvent(notif.RecipientId, "notification_created")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish notification created event",
			"notification_id", notif.Id,
			"error", err)
	}
}

// publishNotificationDismissedEvent publishes a live event for cross-device sync.
func (c *ChattoCore) publishNotificationDismissedEvent(ctx context.Context, userID, notificationID string) {
	event := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_NotificationDismissed{
			NotificationDismissed: &corev1.NotificationDismissedEvent{
				NotificationId: notificationID,
			},
		},
	})

	subject := subjects.LiveSyncUserEvent(userID, "notification_dismissed")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish notification dismissed event",
			"notification_id", notificationID,
			"error", err)
	}
}

// ============================================================================
// Helpers
// ============================================================================

// notificationTypeName returns a string name for the notification type.
func notificationTypeName(notif *corev1.Notification) string {
	switch notif.Notification.(type) {
	case *corev1.Notification_DmMessage:
		return "dm_message"
	case *corev1.Notification_Mention:
		return "mention"
	case *corev1.Notification_Reply:
		return "reply"
	case *corev1.Notification_RoomMessage:
		return "room_message"
	default:
		return "unknown"
	}
}
