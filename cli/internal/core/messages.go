package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/encryption"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const defaultHistoricalMessageLimit = 50

// PostMessage posts a message to a room.
// The flow is: store body in BODIES bucket first (using NanoID), then publish event.
// This eliminates race conditions where subscribers receive the event before body is stored.
// Attachments should already be uploaded to ObjectStore; pass their metadata here.
// inThread is the event ID of the thread root message for thread replies, or empty string for top-level messages.
// If inThread is empty but inReplyTo points at a message that is itself in a thread, inThread is
// derived from the target's own inThread so the new message correctly joins that thread.
// inReplyTo is the event ID of the message this responds to (attribution only), or empty string.
// alsoSendToChannel publishes a MessagePostedEvent echo to the root subject for channel visibility.
// Authorization: Caller must verify room membership and CanPostMessage/CanPostInThread before calling, and CanEchoMessage (if alsoSendToChannel).
func (c *ChattoCore) PostMessage(ctx context.Context, kind RoomKind, room_id, user_id, body string, attachments []*corev1.Attachment, inThread, inReplyTo string, linkPreview *corev1.LinkPreview, alsoSendToChannel bool) (*corev1.Event, error) {
	// Validate message body length to prevent DoS via oversized messages
	if len(body) > MaxMessageBodyLength {
		return nil, ErrMessageTooLong
	}

	// Validate that message has either body or attachments.
	// HasVisibleContent rejects messages with only invisible Unicode characters.
	hasBody := HasVisibleContent(body)
	hasAttachments := len(attachments) > 0
	if !hasBody && !hasAttachments {
		return nil, fmt.Errorf("message must have either body or attachments")
	}

	// Verify room exists and isn't archived
	room, err := c.GetRoom(ctx, kind, room_id)
	if err != nil {
		return nil, err
	}
	if room.Archived {
		return nil, ErrRoomArchived
	}

	// If replying to a message inside a thread, inherit its thread root.
	// This keeps the data invariant intact even when callers (bots, older clients,
	// extensions) only set inReplyTo. inReplyTo is attribution-only, so a lookup
	// failure here is not fatal — fall through and let the message post as a root.
	if inReplyTo != "" && inThread == "" {
		target, err := c.GetRoomEventByEventID(ctx, kind, room_id, inReplyTo)
		if err == nil && target != nil {
			if msg := target.GetMessagePosted(); msg != nil && msg.InThread != "" {
				inThread = msg.InThread
			}
		}
	}

	// Validate thread root exists if posting to a thread.
	if inThread != "" {
		rootEvent, err := c.GetRoomEventByEventID(ctx, kind, room_id, inThread)
		if err != nil {
			return nil, fmt.Errorf("failed to get thread root message: %w", err)
		}
		if rootEvent == nil {
			return nil, fmt.Errorf("thread root message not found: event ID %s", inThread)
		}
		rootMsg := rootEvent.GetMessagePosted()
		if rootMsg == nil {
			return nil, fmt.Errorf("thread root is not a message event: event ID %s", inThread)
		}
		// Verify it's actually a root message (not itself a thread reply)
		if rootMsg.InThread != "" {
			return nil, fmt.Errorf("thread root must be a root message, not a thread reply: event ID %s", inThread)
		}
	}

	now := time.Now()

	// Extract and resolve @mentions from message body
	var mentionedUserIDs []string
	if hasBody {
		usernames := ExtractMentionUsernames(body)
		if len(usernames) > 0 {
			resolved, err := c.ResolveMentions(ctx, usernames)
			if err != nil {
				c.logger.Warn("Failed to resolve mentions", "error", err)
				// Continue without mentions - don't fail the message
			} else {
				mentionedUserIDs = resolved
			}
		}
	}

	// STEP 1: Create event first to get the event ID for body storage
	// The compound key format is {userId}.{eventId} to enable efficient user-based filtering
	event := newEvent(user_id, &corev1.Event{
		CreatedAt: timestamppb.New(now),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				SpaceId:          SpaceIDForKind(kind),
				RoomId:           room_id,
				InReplyTo:        inReplyTo,
				InThread:         inThread,
				MentionedUserIds: mentionedUserIDs,
			},
		},
	})

	// Use event ID for body storage key
	messageBodyKey := messageBodyKey(user_id, event.Id)
	event.GetMessagePosted().MessageBodyId = messageBodyKey

	// STEP 2: Store message body in BODIES bucket BEFORE publishing event
	// This eliminates the race condition where subscribers receive event before body exists
	// Note: UpdatedAt is intentionally nil for new messages - only set when message is edited
	messageBody := &corev1.MessageBody{
		CreatedAt:   timestamppb.New(now),
		Attachments: attachments,
		AuthorId:    user_id,
		LinkPreview: linkPreview,
	}

	// Encrypt message body
	key, err := c.encryption.keyManager.GetUserKey(ctx, user_id)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}
	if key == nil {
		return nil, fmt.Errorf("encryption key not found for user %s", user_id)
	}

	encrypted, err := encryption.Encrypt(key, []byte(body))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt message body: %w", err)
	}

	messageBody.EncryptedBody = encrypted.Ciphertext
	messageBody.EncryptionNonce = encrypted.Nonce

	bucket := c.storage.serverBodiesKV

	bodyData, err := proto.Marshal(messageBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message body: %w", err)
	}

	_, err = bucket.Put(ctx, messageBodyKey, bodyData)
	if err != nil {
		return nil, fmt.Errorf("failed to store message body: %w", err)
	}

	// STEP 3: Publish event
	// Choose subject based on whether this is a root message or thread reply
	// Event ID is included in the subject for O(1) lookup via GetLastMsgForSubject
	var subject string
	if inThread == "" {
		subject = subjects.RoomMessage(string(kind), room_id, event.Id)
	} else {
		subject = subjects.RoomThread(string(kind), room_id, inThread, event.Id)
	}

	// Publish with OCC for reliable delivery with retry on concurrent publishes
	sequenceID, err := c.publishServerEventWithOCC(ctx, subject, event)
	if err != nil {
		// Body was stored but event failed to publish - clean up body
		_ = bucket.Delete(ctx, messageBodyKey)
		return nil, fmt.Errorf("failed to publish message event: %w", err)
	}

	c.logger.Info("Message posted", "kind", kind, "room_id", room_id, "message_body_key", messageBodyKey, "sequence_id", sequenceID, "user_id", user_id)

	// Mark the room as read for the poster. For root posts, the just-
	// published event is the new last root. For thread replies, we look up
	// the room's current last root so the read marker tracks a real root
	// event ID (HasUnread expects root events).
	var posterReadEventID string
	if inThread == "" {
		posterReadEventID = event.Id
	} else if lastRootID, _, exists, err := c.GetRoomLastEvent(ctx, kind, room_id); err == nil && exists {
		posterReadEventID = lastRootID
	}
	if posterReadEventID != "" {
		if err := c.SetLastReadEventID(ctx, kind, user_id, room_id, posterReadEventID); err != nil {
			c.logger.Warn("Failed to set last read event for poster", "error", err)
		}
	}

	// Update thread metadata if this is a thread reply
	if inThread != "" {
		// Get the thread root event to find the original author
		rootEvent, err := c.GetRoomEventByEventID(ctx, kind, room_id, inThread)
		if err != nil {
			c.logger.Warn("Failed to get thread root event",
				"thread_root_id", inThread,
				"error", err)
		}

		var rootAuthorID string
		if rootEvent != nil {
			rootAuthorID = rootEvent.ActorId
		}

		if err := c.updateThreadMetadata(ctx, kind, room_id, inThread, rootAuthorID, user_id, now); err != nil {
			c.logger.Warn("Failed to update thread metadata", "error", err, "thread_root_event_id", inThread)
			// Continue anyway - thread metadata is best-effort
		}

		// Update the poster's "last opened" timestamp for this thread.
		// This ensures that on page reload, their own message won't show as "unread".
		if _, err := c.SetThreadLastOpened(ctx, kind, user_id, room_id, inThread); err != nil {
			c.logger.Warn("Failed to update thread last opened for poster", "error", err, "thread_root_event_id", inThread)
			// Continue anyway - this is best-effort
		}

		// Auto-follow the thread for the poster (best-effort).
		// Always follows, even if previously unfollowed — posting implies interest.
		if err := c.FollowThread(ctx, kind, user_id, room_id, inThread); err != nil {
			c.logger.Warn("Failed to auto-follow thread for poster", "error", err, "thread_root_event_id", inThread)
		}

		// Auto-follow the root author only on the first reply to their message.
		// We check the reply count (already updated above): if 1, this is the first reply.
		// On subsequent replies, we don't re-add the root author — they can unfollow freely.
		if rootAuthorID != "" && rootAuthorID != user_id {
			threadMeta, err := c.GetThreadMetadata(ctx, kind, room_id, inThread)
			if err != nil {
				c.logger.Warn("Failed to get thread metadata for root author auto-follow", "error", err, "thread_root_event_id", inThread)
			} else if threadMeta.ReplyCount == 1 {
				if err := c.FollowThread(ctx, kind, rootAuthorID, room_id, inThread); err != nil {
					c.logger.Warn("Failed to auto-follow thread for root author", "error", err, "thread_root_event_id", inThread)
				}
			}
		}
	}

	// Notify mentioned users (best-effort, don't fail the message if this fails)
	if len(mentionedUserIDs) > 0 {
		c.notifyMentionedUsers(ctx, kind, room_id, user_id, event.Id, inThread, mentionedUserIDs)
	}

	// Notify the author of the message being replied to (best-effort).
	// Fires for both room-level replies and in-thread replies with inReplyTo set.
	// Runs before notifyThreadFollowers so the more specific inReplyTo notification
	// takes priority (thread participants dedup against this).
	var replyNotifiedUserID string
	if inReplyTo != "" {
		replyNotifiedUserID = c.notifyInReplyToAuthor(ctx, kind, room_id, user_id, event.Id, inReplyTo, inThread, mentionedUserIDs)
	}

	// Notify all thread participants (best-effort).
	// Skip users already notified by inReplyTo (they get the more specific notification).
	if inThread != "" {
		var skipIDs []string
		if replyNotifiedUserID != "" {
			skipIDs = []string{replyNotifiedUserID}
		}
		c.notifyThreadFollowers(ctx, kind, room_id, user_id, event.Id, inThread, skipIDs)
	}

	// Notify DM participants for every new message (best-effort)
	if kind == KindDM {
		c.notifyDMParticipants(ctx, room_id, user_id, event.Id)
	}

	// Notify room members who have ALL_MESSAGES notification level (root messages only).
	// Build a set of already-notified users to avoid duplicate notifications.
	if inThread == "" {
		alreadyNotified := make(map[string]bool)
		alreadyNotified[user_id] = true // Author
		for _, uid := range mentionedUserIDs {
			alreadyNotified[uid] = true
		}
		// Include in-reply-to author to avoid duplicate notification
		if replyNotifiedUserID != "" {
			alreadyNotified[replyNotifiedUserID] = true
		}
		// Include DM participants to avoid duplicate notifications
		// (they were already notified by notifyDMParticipants above)
		if kind == KindDM {
			if participants, err := c.GetDMParticipants(ctx, room_id); err == nil {
				for _, pid := range participants {
					alreadyNotified[pid] = true
				}
			}
		}
		c.notifyAllMessageSubscribers(ctx, kind, room_id, user_id, event.Id, alreadyNotified)
	}

	// Publish echo event to root subject if "also send to channel" was requested.
	// This creates a separate event visible in GetRoomEvents (main channel timeline).
	// The echo shares the same messageBodyId, so edits/deletes propagate to both.
	if inThread != "" && alsoSendToChannel {
		echoEvent := newEvent(user_id, &corev1.Event{
			CreatedAt: event.CreatedAt,
			Event: &corev1.Event_MessagePosted{
				MessagePosted: &corev1.MessagePostedEvent{
					SpaceId:                   SpaceIDForKind(kind),
					RoomId:                    room_id,
					MessageBodyId:             messageBodyKey,
					InReplyTo:                 inReplyTo,
					MentionedUserIds:          mentionedUserIDs,
					EchoOfEventId:             event.Id,
					EchoFromThreadRootEventId: inThread,
				},
			},
		})

		echoSubject := subjects.RoomMessage(string(kind), room_id, echoEvent.Id)
		echoSequenceID, err := c.publishServerEventWithOCC(ctx, echoSubject, echoEvent)
		if err != nil {
			c.logger.Warn("Failed to publish thread reply echo", "error", err, "thread_reply_event_id", event.Id)
		} else {
			c.logger.Info("Thread reply echo posted",
				"kind", kind, "room_id", room_id,
				"echo_event_id", echoEvent.Id, "original_event_id", event.Id,
				"echo_sequence_id", echoSequenceID)

			// Notify room members with ALL_MESSAGES notification level (best-effort).
			// Build already-notified set: author + mentioned users (already notified above for original reply).
			echoAlreadyNotified := make(map[string]bool)
			echoAlreadyNotified[user_id] = true
			for _, uid := range mentionedUserIDs {
				echoAlreadyNotified[uid] = true
			}
			c.notifyAllMessageSubscribers(ctx, kind, room_id, user_id, echoEvent.Id, echoAlreadyNotified)
		}
	}

	return event, nil
}

// notifyAllMessageSubscribers creates notifications for room members who have the
// ALL_MESSAGES notification level. Only called for root messages (not thread replies).
// Skips users who were already notified (mentions, thread replies, DM notifications).
// This is best-effort - failures are logged but don't affect message posting.
func (c *ChattoCore) notifyAllMessageSubscribers(ctx context.Context, kind RoomKind, roomID, authorID, eventID string, alreadyNotified map[string]bool) {
	members, err := c.GetRoomMembersList(ctx, kind, roomID)
	if err != nil {
		c.logger.Warn("Failed to get room members for all-message notifications",
			"kind", kind, "room_id", roomID, "error", err)
		return
	}

	notifiedCount := 0
	for _, member := range members {
		memberID := member.UserId
		if alreadyNotified[memberID] {
			continue
		}

		level, err := c.GetEffectiveNotificationLevel(ctx, memberID, roomID)
		if err != nil {
			c.logger.Warn("Failed to get notification level for all-message check",
				"user_id", memberID, "error", err)
			continue
		}
		if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES {
			continue
		}

		_, err = c.CreateNotification(ctx, memberID, authorID, &corev1.Notification{
			Notification: &corev1.Notification_RoomMessage{
				RoomMessage: &corev1.RoomMessageNotification{
					SpaceId: SpaceIDForKind(kind),
					RoomId:  roomID,
					EventId: eventID,
				},
			},
		})
		if err != nil {
			c.logger.Warn("Failed to create all-message notification",
				"recipient_id", memberID, "author_id", authorID,
				"kind", kind, "room_id", roomID, "error", err)
		} else {
			notifiedCount++
		}
	}

	if notifiedCount > 0 {
		c.logger.Debug("Created all-message notifications",
			"kind", kind, "room_id", roomID, "count", notifiedCount)
	}
}

// DeleteMessage deletes a message body and its attachments for GDPR compliance.
// This removes the message content from the BODIES bucket and any attachments from the ASSETS
// ObjectStore, while preserving the event in the stream for audit trail purposes.
// Subsequent lazy-loading will result in an empty body field.
// Publishes a MessageDeletedEvent to notify connected clients in real-time.
// The messageBodyKey parameter is the full compound key ({userId}.{bodyId}) stored in the event.
// Authorization: Caller must verify the actor is the message author OR (CanManageOthersMessage AND OutranksAuthor) before calling.
func (c *ChattoCore) DeleteMessage(ctx context.Context, actorID string, kind RoomKind, roomID, messageBodyKey string) error {
	// Get the full message body first to find any attachments
	messageBody, err := c.GetFullMessageBody(ctx, kind, messageBodyKey)
	if err != nil {
		return fmt.Errorf("failed to get message body: %w", err)
	}
	if messageBody == nil {
		// Already deleted, nothing to do
		c.logger.Debug("Message body already deleted", "message_body_key", messageBodyKey)
		return nil
	}

	// Delete all attachments from the ObjectStore (supports both NATS and S3)
	for _, attachment := range messageBody.Attachments {
		if err := c.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
			c.logger.Warn("Failed to delete attachment during message deletion",
				"attachment_id", attachment.Id,
				"message_body_key", messageBodyKey,
				"error", err)
			// Continue deleting other attachments even if one fails
		}
	}

	// Delete the message body from KV
	bucket := c.storage.serverBodiesKV

	err = bucket.Delete(ctx, messageBodyKey)
	if err != nil {
		return fmt.Errorf("failed to delete message body: %w", err)
	}

	c.logger.Info("Message body deleted", "kind", kind, "room_id", roomID, "message_body_key", messageBodyKey, "actor_id", actorID, "attachments_deleted", len(messageBody.Attachments))

	// Publish live event to notify connected clients
	c.publishMessageDeletedEvent(ctx, kind, roomID, messageBodyKey, actorID)

	return nil
}

// publishMessageDeletedEvent publishes a MessageDeletedEvent directly to the live subject space.
// This notifies connected clients that a message has been deleted so they can update their UI.
func (c *ChattoCore) publishMessageDeletedEvent(ctx context.Context, kind RoomKind, roomID, messageBodyID, userID string) {
	messageEventID := eventIDFromBodyKey(messageBodyID)
	event := newEvent(userID, &corev1.Event{
		Event: &corev1.Event_MessageDeleted{
			MessageDeleted: &corev1.MessageDeletedEvent{
				SpaceId:        SpaceIDForKind(kind),
				RoomId:         roomID,
				MessageBodyId:  messageBodyID,
				MessageEventId: messageEventID,
			},
		},
	})

	// Publish directly to live subject (bypass JetStream)
	subject := subjects.LiveRoomEvent(string(kind), roomID, "message_deleted")
	if err := c.publishLiveServerEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish message deleted event", "error", err)
	}
}

// EditMessage edits a message body. Updates the body content and sets updated_at.
// Publishes a MessageUpdatedEvent to notify connected clients in real-time.
// The messageBodyKey parameter is the full compound key ({userId}.{bodyId}) stored in the event.
//
// Business rule: Authors can only edit their own messages within MessageEditWindow (3 hours).
// Non-authors (moderators with message.manage) can edit at any time.
//
// Authorization: Caller must verify the actor is the author OR (CanManageOthersMessage AND OutranksAuthor) before calling.
func (c *ChattoCore) EditMessage(ctx context.Context, actorID string, kind RoomKind, roomID, messageBodyKey, newBody string) error {
	// Block edits in archived rooms.
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return err
	}
	if room.Archived {
		return ErrRoomArchived
	}

	bucket := c.storage.serverBodiesKV

	// Get message with revision for optimistic locking
	entry, err := bucket.Get(ctx, messageBodyKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return ErrMessageNotFound
		}
		return fmt.Errorf("failed to get message body: %w", err)
	}

	// Unmarshal the message body
	messageBody := &corev1.MessageBody{}
	if err := proto.Unmarshal(entry.Value(), messageBody); err != nil {
		return fmt.Errorf("failed to unmarshal message body: %w", err)
	}

	// Business rule: authors can only edit within the edit window
	// Non-authors (moderators) can edit at any time
	isAuthorEdit := messageBody.AuthorId == actorID
	if isAuthorEdit && time.Since(messageBody.CreatedAt.AsTime()) > MessageEditWindow {
		return ErrEditWindowExpired
	}

	// Update the message body with new encrypted content
	messageBody.UpdatedAt = timestamppb.Now()

	// Encrypt with the author's key and a new nonce
	key, err := c.encryption.keyManager.GetUserKey(ctx, messageBody.AuthorId)
	if err != nil {
		return fmt.Errorf("failed to get encryption key: %w", err)
	}
	if key == nil {
		return fmt.Errorf("cannot edit: encryption key not found (message was crypto-shredded)")
	}

	encrypted, err := encryption.Encrypt(key, []byte(newBody))
	if err != nil {
		return fmt.Errorf("failed to encrypt message body: %w", err)
	}

	messageBody.EncryptedBody = encrypted.Ciphertext
	messageBody.EncryptionNonce = encrypted.Nonce

	// Marshal and store with optimistic locking
	data, err := proto.Marshal(messageBody)
	if err != nil {
		return fmt.Errorf("failed to marshal message body: %w", err)
	}

	_, err = bucket.Update(ctx, messageBodyKey, data, entry.Revision())
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			// Concurrent modification - could retry, but for now just fail
			return fmt.Errorf("message was modified concurrently")
		}
		return fmt.Errorf("failed to update message body: %w", err)
	}

	c.logger.Info("Message body edited", "kind", kind, "room_id", roomID, "message_body_key", messageBodyKey, "actor_id", actorID)

	// Publish live event to notify connected clients
	c.publishMessageUpdatedEvent(ctx, kind, roomID, messageBodyKey, actorID)

	return nil
}

// DeleteAttachmentFromMessage deletes a single attachment from a message.
// Only the message author can delete their attachments.
// Removes the attachment from the MessageBody and deletes the file from ObjectStore.
// Publishes a MessageUpdatedEvent to notify connected clients in real-time.
// The messageBodyKey parameter is the full compound key ({userId}.{bodyId}) stored in the event.
func (c *ChattoCore) DeleteAttachmentFromMessage(ctx context.Context, actorID string, kind RoomKind, roomID, messageBodyKey, attachmentID string) error {
	bucket := c.storage.serverBodiesKV

	// Get message with revision for optimistic locking
	entry, err := bucket.Get(ctx, messageBodyKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return ErrMessageNotFound
		}
		return fmt.Errorf("failed to get message body: %w", err)
	}

	// Unmarshal the message body
	messageBody := &corev1.MessageBody{}
	if err := proto.Unmarshal(entry.Value(), messageBody); err != nil {
		return fmt.Errorf("failed to unmarshal message body: %w", err)
	}

	// Check ownership - only the author can delete their attachments
	if messageBody.AuthorId != actorID {
		return ErrNotMessageAuthor
	}

	// Find and remove the attachment from the slice
	attachmentIndex := -1
	for i, att := range messageBody.Attachments {
		if att.Id == attachmentID {
			attachmentIndex = i
			break
		}
	}
	if attachmentIndex == -1 {
		return fmt.Errorf("attachment not found in message")
	}

	// Save reference before removing from slice (needed for storage-aware deletion)
	removedAttachment := messageBody.Attachments[attachmentIndex]

	// Remove the attachment from the slice
	messageBody.Attachments = append(messageBody.Attachments[:attachmentIndex], messageBody.Attachments[attachmentIndex+1:]...)
	messageBody.UpdatedAt = timestamppb.Now()

	// Marshal and store with optimistic locking
	data, err := proto.Marshal(messageBody)
	if err != nil {
		return fmt.Errorf("failed to marshal message body: %w", err)
	}

	_, err = bucket.Update(ctx, messageBodyKey, data, entry.Revision())
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("message was modified concurrently")
		}
		return fmt.Errorf("failed to update message body: %w", err)
	}

	// Delete the attachment file from storage (supports both NATS and S3)
	if err := c.DeleteAttachmentFromStorage(ctx, removedAttachment); err != nil {
		c.logger.Warn("Failed to delete attachment file after removing from message",
			"attachment_id", attachmentID,
			"message_body_key", messageBodyKey,
			"error", err)
		// Don't fail the operation - the attachment reference is already removed
	}

	c.logger.Info("Attachment deleted from message",
		"kind", kind,
		"room_id", roomID,
		"message_body_key", messageBodyKey,
		"attachment_id", attachmentID,
		"actor_id", actorID)

	// Publish live event to notify connected clients
	c.publishMessageUpdatedEvent(ctx, kind, roomID, messageBodyKey, actorID)

	return nil
}

// DeleteLinkPreviewFromMessage removes a link preview from a message.
// Only the message author can delete link previews from their messages.
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) DeleteLinkPreviewFromMessage(ctx context.Context, actorID string, kind RoomKind, roomID, messageBodyKey, previewURL string) error {
	bucket := c.storage.serverBodiesKV

	// Get message with revision for optimistic locking
	entry, err := bucket.Get(ctx, messageBodyKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return ErrMessageNotFound
		}
		return fmt.Errorf("failed to get message body: %w", err)
	}

	// Unmarshal the message body
	messageBody := &corev1.MessageBody{}
	if err := proto.Unmarshal(entry.Value(), messageBody); err != nil {
		return fmt.Errorf("failed to unmarshal message body: %w", err)
	}

	// Check ownership - only the author can delete their link preview
	if messageBody.AuthorId != actorID {
		return ErrNotMessageAuthor
	}

	// Verify the preview exists and matches the requested URL
	if messageBody.LinkPreview == nil || messageBody.LinkPreview.Url != previewURL {
		return fmt.Errorf("link preview not found in message")
	}

	// Remove the link preview
	messageBody.LinkPreview = nil
	messageBody.UpdatedAt = timestamppb.Now()

	// Marshal and store with optimistic locking
	data, err := proto.Marshal(messageBody)
	if err != nil {
		return fmt.Errorf("failed to marshal message body: %w", err)
	}

	_, err = bucket.Update(ctx, messageBodyKey, data, entry.Revision())
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("message was modified concurrently, please retry")
		}
		return fmt.Errorf("failed to update message body: %w", err)
	}

	c.logger.Info("Link preview deleted from message",
		"kind", kind,
		"room_id", roomID,
		"message_body_key", messageBodyKey,
		"preview_url", previewURL,
		"actor_id", actorID)

	// Publish live event to notify connected clients
	c.publishMessageUpdatedEvent(ctx, kind, roomID, messageBodyKey, actorID)

	return nil
}

// publishMessageUpdatedEvent publishes a MessageUpdatedEvent directly to the live subject space.
// This notifies connected clients that a message has been edited so they can update their UI.
func (c *ChattoCore) publishMessageUpdatedEvent(ctx context.Context, kind RoomKind, roomID, messageBodyID, userID string) {
	messageEventID := eventIDFromBodyKey(messageBodyID)
	event := newEvent(userID, &corev1.Event{
		Event: &corev1.Event_MessageUpdated{
			MessageUpdated: &corev1.MessageUpdatedEvent{
				SpaceId:        SpaceIDForKind(kind),
				RoomId:         roomID,
				MessageBodyId:  messageBodyID,
				MessageEventId: messageEventID,
			},
		},
	})

	// Publish directly to live subject (bypass JetStream)
	subject := subjects.LiveRoomEvent(string(kind), roomID, "message_updated")
	if err := c.publishLiveServerEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish message updated event", "error", err)
	}
}
