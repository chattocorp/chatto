package core

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const defaultHistoricalMessageLimit = 50

// PostMessage posts a message to a room. Publishes a
// MessagePostedEvent on evt.room.{R}.message_posted with the
// encrypted body embedded — no separate SERVER_BODIES write.
//
// Threading: inThread is the event ID of the thread root for replies,
// empty for root posts. If inThread is empty but inReplyTo points at
// a message that is itself a thread reply, inThread is derived from
// the target's own inThread so the new message joins that thread.
// inReplyTo is the event ID of the message being responded to
// (attribution only). alsoSendToChannel publishes an echo
// MessagePostedEvent on the same subject with echo_of_event_id set,
// making the reply visible in the channel timeline.
//
// Authorization: Caller must verify room membership and
// CanPostMessage / CanPostInThread before calling, and CanEchoMessage
// (if alsoSendToChannel).
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

	// Encrypt the body with the author's per-user key. Same envelope
	// shape as before the ES cutover (MessageBody.EncryptedBody +
	// EncryptionNonce); only the storage location has changed —
	// embedded in the event instead of stored in SERVER_BODIES.
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

	messageBody := &corev1.MessageBody{
		CreatedAt:       timestamppb.New(now),
		Attachments:     attachments,
		AuthorId:        user_id,
		LinkPreview:     linkPreview,
		EncryptedBody:   encrypted.Ciphertext,
		EncryptionNonce: encrypted.Nonce,
	}

	event := newEvent(user_id, &corev1.Event{
		CreatedAt: timestamppb.New(now),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				RoomId:           room_id,
				InReplyTo:        inReplyTo,
				InThread:         inThread,
				MentionedUserIds: mentionedUserIDs,
				Body:             messageBody,
			},
		},
	})
	// Set MessageBodyId to the envelope id. Post-#597 cutover,
	// MessageBodyId is no longer a {userId}.{bodyId} compound
	// key — it's just an alias for event_id, kept on the proto so
	// legacy code paths that pass MessageBodyId around (resolvers,
	// attachment signed URLs) keep working without changes.
	// eventIDFromBodyKey treats no-dot keys as event_ids, so the
	// projection lookup picks the right entry.
	event.GetMessagePosted().MessageBodyId = event.Id

	// Stamp the event_id onto each attachment so signed attachment
	// URLs can carry it without a separate index lookup at request
	// time.
	for _, att := range attachments {
		if att != nil {
			att.MessageBodyId = event.Id
		}
	}

	// Publish to EVT. MessagePosted is append-only per #597's design, so
	// retrying the same payload after an OCC conflict is safe.
	// AppendEventuallyAndWait blocks until the RoomTimelineProjection
	// has caught up, giving read-your-writes for subsequent reads from
	// this request.
	agg := events.RoomAggregate(room_id)
	sequenceID, err := c.RoomTimelineProjector.AppendEventuallyAndWait(ctx, c.EventPublisher, agg, event)
	if err != nil {
		return nil, fmt.Errorf("failed to publish message event: %w", err)
	}
	// Also wait for ThreadProjection if this is a thread reply, so a
	// subsequent thread-pane fetch from the same request sees it.
	if inThread != "" {
		if err := c.ThreadsProjector.WaitForSeq(ctx, sequenceID); err != nil {
			c.logger.Debug("ThreadsProjector did not catch up", "error", err)
		}
	}

	// Legacy live mirror. Same pattern as RoomMembership / RoomGroups
	// after their ES migration — the durable event lives on EVT, but
	// frontend subscribers consume from live.server.>. We mirror onto
	// the legacy subject family so the existing myEvents pipeline
	// keeps working unchanged. publishLiveServerEvent is fire-and-
	// forget over NATS Core; no second durable copy.
	var liveSubject string
	if inThread == "" {
		liveSubject = subjects.RoomMessage(string(kind), room_id, event.Id)
	} else {
		liveSubject = subjects.RoomThread(string(kind), room_id, inThread, event.Id)
	}
	if err := c.publishLiveServerEvent(ctx, liveSubject, event); err != nil {
		c.logger.Warn("Failed to publish message live mirror", "error", err, "subject", liveSubject)
	}

	// messageBodyKey retained as a label for log lines and downstream
	// notifications that historically logged the compound key — the
	// projection-keyed event_id is the new canonical identifier.
	messageBodyKey := event.Id
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

	// Update thread metadata if this is a thread reply.
	// Reply count / participants / lastReplyAt are derived live from
	// ThreadProjection now, so no KV write — but we still need the
	// root author for the auto-follow logic below.
	if inThread != "" {
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

		// Update the poster's thread read marker to the reply they just wrote.
		// This ensures that on page reload, their own message won't show as "unread".
		if _, err := c.SetThreadLastReadEventID(ctx, kind, user_id, room_id, inThread, event.Id); err != nil {
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
	// The echo references the original event_id, so resolvers can fold
	// it back to the underlying body. We re-use the encrypted body
	// envelope from the original (same ciphertext + nonce) so the
	// echo is a self-contained renderable message.
	if inThread != "" && alsoSendToChannel {
		echoEvent := newEvent(user_id, &corev1.Event{
			CreatedAt: event.CreatedAt,
			Event: &corev1.Event_MessagePosted{
				MessagePosted: &corev1.MessagePostedEvent{
					RoomId:                    room_id,
					InReplyTo:                 inReplyTo,
					MentionedUserIds:          mentionedUserIDs,
					EchoOfEventId:             event.Id,
					EchoFromThreadRootEventId: inThread,
					Body:                      messageBody,
				},
			},
		})
		echoEvent.GetMessagePosted().MessageBodyId = echoEvent.Id

		echoSequenceID, err := c.RoomTimelineProjector.AppendEventuallyAndWait(ctx, c.EventPublisher, agg, echoEvent)
		if err != nil {
			c.logger.Warn("Failed to publish thread reply echo", "error", err, "thread_reply_event_id", event.Id)
		} else {
			echoLiveSubject := subjects.RoomMessage(string(kind), room_id, echoEvent.Id)
			if err := c.publishLiveServerEvent(ctx, echoLiveSubject, echoEvent); err != nil {
				c.logger.Warn("Failed to publish echo live mirror", "error", err)
			}
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
	eventID := eventIDFromBodyKey(messageBodyKey)
	if eventID == "" {
		return ErrMessageNotFound
	}

	// Snapshot the projection state for attachment cleanup before
	// emitting the retract event. After retract, LatestBody returns
	// nil (the message is tombstoned), so we need a copy first.
	originalEntry, ok := c.RoomTimeline.Get(eventID)
	if !ok {
		c.logger.Debug("Delete on unknown message — no-op", "event_id", eventID)
		return nil
	}
	body, retracted, _ := c.RoomTimeline.LatestBody(eventID)
	if retracted {
		// Already tombstoned.
		return nil
	}

	// Emit MessageRetractedEvent on evt.room.{R}.message_retracted.
	// Pure append for the v1 model — last-writer-wins on the per-room
	// retract subject. The projection ignores duplicates by event_id,
	// so retrying after a network glitch is safe.
	agg := events.RoomAggregate(roomID)
	if err := c.publishMessageRetract(ctx, actorID, kind, agg, roomID, eventID); err != nil {
		return err
	}
	// Fan out the retract to echoes / original (legacy "delete one,
	// both go" semantic).
	for _, linkedID := range c.RoomTimeline.LinkedEventIDs(eventID) {
		if err := c.publishMessageRetract(ctx, actorID, kind, agg, roomID, linkedID); err != nil {
			c.logger.Warn("Failed to propagate retract to linked message",
				"source_event_id", eventID, "linked_event_id", linkedID, "error", err)
		}
	}

	// Attachments are referenced by the (now-tombstoned) message but
	// the binary blobs in the asset store don't get cleaned up by the
	// event log. Same posture as the legacy DeleteMessage path —
	// best-effort, log warnings, keep going.
	if body != nil {
		for _, att := range body.GetAttachments() {
			if err := c.DeleteAttachmentFromStorage(ctx, att); err != nil {
				c.logger.Warn("Failed to delete attachment during message deletion",
					"attachment_id", att.GetId(),
					"event_id", eventID,
					"error", err)
			}
		}
	}

	c.logger.Info("Message retracted", "kind", kind, "room_id", roomID, "event_id", eventID, "actor_id", actorID, "envelope_seq", originalEntry.StreamSeq)
	return nil
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

	eventID := eventIDFromBodyKey(messageBodyKey)
	if eventID == "" {
		return ErrMessageNotFound
	}
	originalEntry, ok := c.RoomTimeline.Get(eventID)
	if !ok {
		return ErrMessageNotFound
	}
	origPost := originalEntry.Event.GetMessagePosted()
	if origPost == nil {
		return ErrMessageNotFound
	}

	// Author / edit-window check. Edit window only applies to the
	// original author; moderators bypass it (their authorization is
	// gated upstream at the resolver).
	authorID := originalEntry.Event.GetActorId()
	if authorID == actorID {
		if time.Since(originalEntry.Event.GetCreatedAt().AsTime()) > MessageEditWindow {
			return ErrEditWindowExpired
		}
	}

	// Fold in current body so attachments/link preview/timestamps
	// survive the edit. We then overwrite ciphertext + nonce with the
	// new content.
	current, retracted, _ := c.RoomTimeline.LatestBody(eventID)
	if retracted {
		return ErrMessageNotFound
	}
	if current == nil {
		// Imported legacy event with no body — nothing to edit.
		return ErrMessageNotFound
	}

	key, err := c.encryption.keyManager.GetUserKey(ctx, authorID)
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

	updated := proto.Clone(current).(*corev1.MessageBody)
	updated.EncryptedBody = encrypted.Ciphertext
	updated.EncryptionNonce = encrypted.Nonce
	updated.UpdatedAt = timestamppb.Now()

	agg := events.RoomAggregate(roomID)
	if err := c.publishMessageEdit(ctx, actorID, kind, agg, roomID, eventID, updated); err != nil {
		return err
	}
	// Fan out to echoes (and to the original if this IS an echo) so
	// the legacy "edit one, both update" semantic is preserved.
	for _, linkedID := range c.RoomTimeline.LinkedEventIDs(eventID) {
		linkedBody := proto.Clone(updated).(*corev1.MessageBody)
		if err := c.publishMessageEdit(ctx, actorID, kind, agg, roomID, linkedID, linkedBody); err != nil {
			c.logger.Warn("Failed to propagate edit to linked message",
				"source_event_id", eventID, "linked_event_id", linkedID, "error", err)
		}
	}

	c.logger.Info("Message edited", "kind", kind, "room_id", roomID, "event_id", eventID, "actor_id", actorID)
	return nil
}

// publishMessageRetract emits a single MessageRetractedEvent on EVT
// and the legacy MessageDeletedEvent live mirror on
// live.server.room.{kind}.{r}.message_deleted so the frontend's
// existing handlers fire. Factored out so DeleteMessage can fan to
// linked messages.
func (c *ChattoCore) publishMessageRetract(ctx context.Context, actorID string, kind RoomKind, agg events.Aggregate, roomID, eventID string) error {
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_MessageRetracted{
			MessageRetracted: &corev1.MessageRetractedEvent{
				RoomId:  roomID,
				EventId: eventID,
			},
		},
	})
	if _, err := c.RoomTimelineProjector.AppendEventuallyAndWait(ctx, c.EventPublisher, agg, event); err != nil {
		return fmt.Errorf("publish MessageRetractedEvent: %w", err)
	}

	// Legacy live mirror so the frontend's MessageDeletedEvent handler
	// keeps firing. Fire-and-forget over NATS Core, no durable
	// duplicate.
	liveEvent := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_MessageDeleted{
			MessageDeleted: &corev1.MessageDeletedEvent{
				RoomId:         roomID,
				MessageEventId: eventID,
				MessageBodyId:  eventID,
			},
		},
	})
	liveSubject := subjects.LiveRoomEvent(string(kind), roomID, "message_deleted")
	if err := c.publishLiveServerEvent(ctx, liveSubject, liveEvent); err != nil {
		c.logger.Warn("Failed to publish retract live mirror", "error", err)
	}
	return nil
}

// publishMessageEdit emits a single MessageEditedEvent on EVT and a
// synthesised MessageUpdatedEvent live mirror on
// live.server.room.{kind}.{r}.message_updated so the frontend's
// existing handlers fire. Factored out so EditMessage /
// editEmbeddedBody can fan the same payload to linked messages.
func (c *ChattoCore) publishMessageEdit(ctx context.Context, actorID string, kind RoomKind, agg events.Aggregate, roomID, eventID string, body *corev1.MessageBody) error {
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_MessageEdited{
			MessageEdited: &corev1.MessageEditedEvent{
				RoomId:  roomID,
				EventId: eventID,
				Body:    body,
			},
		},
	})
	if _, err := c.RoomTimelineProjector.AppendEventuallyAndWait(ctx, c.EventPublisher, agg, event); err != nil {
		return fmt.Errorf("publish MessageEditedEvent: %w", err)
	}

	liveEvent := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_MessageUpdated{
			MessageUpdated: &corev1.MessageUpdatedEvent{
				RoomId:         roomID,
				MessageEventId: eventID,
				MessageBodyId:  eventID,
			},
		},
	})
	liveSubject := subjects.LiveRoomEvent(string(kind), roomID, "message_updated")
	if err := c.publishLiveServerEvent(ctx, liveSubject, liveEvent); err != nil {
		c.logger.Warn("Failed to publish edit live mirror", "error", err)
	}
	return nil
}

// editEmbeddedBody is the shared engine behind partial-edit
// operations (DeleteAttachmentFromMessage, DeleteLinkPreviewFromMessage).
// Reads the current body from the projection, applies `mutate` to a
// clone, encrypts no further (the body's ciphertext is unchanged —
// only metadata moves), and emits a MessageEditedEvent.
//
// `actorID` is the user performing the edit; ownership is checked
// against the body's author.
func (c *ChattoCore) editEmbeddedBody(
	ctx context.Context,
	actorID string,
	kind RoomKind,
	roomID, messageBodyKey string,
	mutate func(*corev1.MessageBody) error,
) error {
	eventID := eventIDFromBodyKey(messageBodyKey)
	if eventID == "" {
		return ErrMessageNotFound
	}
	entry, ok := c.RoomTimeline.Get(eventID)
	if !ok {
		return ErrMessageNotFound
	}
	if entry.Event.GetMessagePosted() == nil {
		return ErrMessageNotFound
	}
	current, retracted, _ := c.RoomTimeline.LatestBody(eventID)
	if retracted || current == nil {
		return ErrMessageNotFound
	}
	if current.GetAuthorId() != actorID {
		return ErrNotMessageAuthor
	}
	updated := proto.Clone(current).(*corev1.MessageBody)
	if err := mutate(updated); err != nil {
		return err
	}
	updated.UpdatedAt = timestamppb.Now()

	agg := events.RoomAggregate(roomID)
	if err := c.publishMessageEdit(ctx, actorID, kind, agg, roomID, eventID, updated); err != nil {
		return err
	}
	for _, linkedID := range c.RoomTimeline.LinkedEventIDs(eventID) {
		linkedBody := proto.Clone(updated).(*corev1.MessageBody)
		if err := c.publishMessageEdit(ctx, actorID, kind, agg, roomID, linkedID, linkedBody); err != nil {
			c.logger.Warn("Failed to propagate partial edit to linked message",
				"source_event_id", eventID, "linked_event_id", linkedID, "error", err)
		}
	}
	return nil
}

// DeleteAttachmentFromMessage deletes a single attachment from a
// message. Only the message author can delete their attachments.
// Emits a MessageEditedEvent with the attachment removed; also
// deletes the file from the asset store best-effort.
func (c *ChattoCore) DeleteAttachmentFromMessage(ctx context.Context, actorID string, kind RoomKind, roomID, messageBodyKey, attachmentID string) error {
	var removed *corev1.Attachment
	err := c.editEmbeddedBody(ctx, actorID, kind, roomID, messageBodyKey, func(body *corev1.MessageBody) error {
		idx := -1
		for i, att := range body.GetAttachments() {
			if att.GetId() == attachmentID {
				idx = i
				break
			}
		}
		if idx == -1 {
			return fmt.Errorf("attachment not found in message")
		}
		removed = body.Attachments[idx]
		body.Attachments = append(body.Attachments[:idx], body.Attachments[idx+1:]...)
		return nil
	})
	if err != nil {
		return err
	}

	if removed != nil {
		if delErr := c.DeleteAttachmentFromStorage(ctx, removed); delErr != nil {
			c.logger.Warn("Failed to delete attachment file after removing from message",
				"attachment_id", attachmentID,
				"message_body_key", messageBodyKey,
				"error", delErr)
		}
	}

	c.logger.Info("Attachment deleted from message",
		"kind", kind,
		"room_id", roomID,
		"message_body_key", messageBodyKey,
		"attachment_id", attachmentID,
		"actor_id", actorID)
	return nil
}

// DeleteLinkPreviewFromMessage removes a link preview from a message.
// Only the message author can delete link previews from their
// messages.
func (c *ChattoCore) DeleteLinkPreviewFromMessage(ctx context.Context, actorID string, kind RoomKind, roomID, messageBodyKey, previewURL string) error {
	err := c.editEmbeddedBody(ctx, actorID, kind, roomID, messageBodyKey, func(body *corev1.MessageBody) error {
		if body.GetLinkPreview() == nil || body.GetLinkPreview().GetUrl() != previewURL {
			return fmt.Errorf("link preview not found in message")
		}
		body.LinkPreview = nil
		return nil
	})
	if err != nil {
		return err
	}
	c.logger.Info("Link preview deleted from message",
		"kind", kind,
		"room_id", roomID,
		"message_body_key", messageBodyKey,
		"preview_url", previewURL,
		"actor_id", actorID)
	return nil
}
