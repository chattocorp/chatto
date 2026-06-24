package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	defaultHistoricalMessageLimit = 50
	maxHistoricalMessageLimit     = 500
)

type postMessageOptions struct {
	videoProcessingAssetIDs map[string]struct{}
	largeMentionConfirmed   bool
}

type editMessageOptions struct {
	channelEcho *bool
}

// PostMessageOption customizes side effects owned by the message-post command.
type PostMessageOption func(*postMessageOptions)

// EditMessageOption customizes side effects owned by the message-edit command.
type EditMessageOption func(*editMessageOptions)

// WithVideoProcessingAssets schedules video processing for the listed message
// attachments after their AssetCreatedEvent records have been appended.
func WithVideoProcessingAssets(assetIDs ...string) PostMessageOption {
	return func(options *postMessageOptions) {
		if options.videoProcessingAssetIDs == nil {
			options.videoProcessingAssetIDs = make(map[string]struct{}, len(assetIDs))
		}
		for _, assetID := range assetIDs {
			if assetID != "" {
				options.videoProcessingAssetIDs[assetID] = struct{}{}
			}
		}
	}
}

// WithLargeMentionConfirmed confirms that the caller intentionally wants to
// send a message whose mentions notify more than
// LargeMentionNotificationThreshold users.
func WithLargeMentionConfirmed() PostMessageOption {
	return func(options *postMessageOptions) {
		options.largeMentionConfirmed = true
	}
}

// WithMessageChannelEcho reconciles whether a thread reply should have a
// visible echo in the channel timeline after the edit is saved.
func WithMessageChannelEcho(enabled bool) EditMessageOption {
	return func(options *editMessageOptions) {
		options.channelEcho = &enabled
	}
}

func collectPostMessageOptions(opts []PostMessageOption) postMessageOptions {
	var options postMessageOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return options
}

func collectEditMessageOptions(opts []EditMessageOption) editMessageOptions {
	var options editMessageOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return options
}

func (options postMessageOptions) shouldScheduleVideoProcessingForID(assetID string) bool {
	if assetID == "" || len(options.videoProcessingAssetIDs) == 0 {
		return false
	}
	_, ok := options.videoProcessingAssetIDs[assetID]
	return ok
}

const maxThreadCreateAppendAttempts = 5

func (c *ChattoCore) threadCreatedExistsInStream(ctx context.Context, agg events.Aggregate, threadRootEventID string) (bool, error) {
	if threadRootEventID == "" {
		return false, nil
	}
	existing, _, err := c.EventPublisher.SubjectEvents(ctx, agg.Subject(events.EventThreadCreated))
	if err != nil {
		return false, err
	}
	for _, event := range existing {
		if event.GetThreadCreated().GetThreadRootEventId() == threadRootEventID {
			return true, nil
		}
	}
	return false, nil
}

func (c *ChattoCore) appendBodyAndMessage(ctx context.Context, agg events.Aggregate, bodyEvent, messageEvent *corev1.Event) (uint64, error) {
	bodySubject := agg.SubjectFor(bodyEvent)
	messageSubject := agg.SubjectFor(messageEvent)
	var lastErr error

	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		expectedSeq, err := c.EventPublisher.LastSubjectSeq(ctx, messageSubject)
		if err != nil {
			return 0, fmt.Errorf("read message OCC tail: %w", err)
		}
		seqs, err := c.EventPublisher.AppendBatch(ctx, []events.BatchEntry{
			{
				Subject:       bodySubject,
				Event:         bodyEvent,
				ExpectedSeq:   expectedSeq,
				FilterSubject: messageSubject,
				HasOCC:        true,
			},
			{
				Subject: messageSubject,
				Event:   messageEvent,
			},
		})
		if err == nil {
			messageSeq := seqs[len(seqs)-1]
			if err := c.rooms().waitForTimeline(ctx, events.SubjectPosition(messageSubject, messageSeq)); err != nil {
				return messageSeq, err
			}
			return messageSeq, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return 0, err
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}

	return 0, fmt.Errorf("append message body batch after %d attempts: %w", maxThreadCreateAppendAttempts, lastErr)
}

func (c *ChattoCore) appendThreadReplyEcho(
	ctx context.Context,
	actorID string,
	kind RoomKind,
	agg events.Aggregate,
	originalEvent *corev1.Event,
	originalPost *corev1.MessagePostedEvent,
	body *corev1.MessageBody,
	plaintext string,
) (string, bool, error) {
	if originalEvent == nil || originalPost == nil || body == nil {
		return "", false, ErrMessageNotFound
	}
	originalID := originalEvent.GetId()
	roomID := originalPost.GetRoomId()
	messageSubject := agg.Subject(events.EventMessagePosted)
	bodySubject := agg.Subject(events.EventMessageBody)
	var lastErr error

	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		expectedSeq, err := c.EventPublisher.LastSubjectSeq(ctx, messageSubject)
		if err != nil {
			return "", false, fmt.Errorf("read echo OCC tail: %w", err)
		}
		if expectedSeq > 0 {
			if err := c.roomService.waitForTimeline(ctx, events.SubjectPosition(messageSubject, expectedSeq)); err != nil {
				return "", false, err
			}
		}
		if echoID, ok := c.RoomTimeline.ChannelEchoEventID(originalID); ok {
			return echoID, false, nil
		}

		echoID := NewEventID()
		echoBodyEventID := NewEventID()
		echoBody := proto.Clone(body).(*corev1.MessageBody)
		if err := c.encryptMessageBody(ctx, echoBody, roomID, echoID, echoBodyEventID, plaintext); err != nil {
			return "", false, fmt.Errorf("encrypt thread reply echo: %w", err)
		}
		echoBodyEvent := newEvent(actorID, &corev1.Event{
			Id:        echoBodyEventID,
			CreatedAt: originalEvent.GetCreatedAt(),
			Event: &corev1.Event_MessageBody{
				MessageBody: &corev1.MessageBodyEvent{
					RoomId:  roomID,
					EventId: echoID,
					Body:    echoBody,
				},
			},
		})
		echoEvent := newEvent(actorID, &corev1.Event{
			Id:        echoID,
			CreatedAt: originalEvent.GetCreatedAt(),
			Event: &corev1.Event_MessagePosted{
				MessagePosted: &corev1.MessagePostedEvent{
					RoomId:                    roomID,
					InReplyTo:                 originalPost.GetInReplyTo(),
					MentionedUserIds:          append([]string(nil), originalPost.GetMentionedUserIds()...),
					EchoOfEventId:             originalID,
					EchoFromThreadRootEventId: originalPost.GetInThread(),
				},
			},
		})

		seqs, err := c.EventPublisher.AppendBatch(ctx, []events.BatchEntry{
			{
				Subject:       bodySubject,
				Event:         echoBodyEvent,
				ExpectedSeq:   expectedSeq,
				FilterSubject: messageSubject,
				HasOCC:        true,
			},
			{
				Subject:       messageSubject,
				Event:         echoEvent,
				ExpectedSeq:   expectedSeq,
				FilterSubject: messageSubject,
				HasOCC:        true,
			},
		})
		if err == nil {
			echoSeq := seqs[len(seqs)-1]
			if err := c.roomService.waitForTimeline(ctx, events.SubjectPosition(messageSubject, echoSeq)); err != nil {
				return echoID, true, err
			}
			c.logger.Info("Thread reply echo posted",
				"kind", kind, "room_id", roomID,
				"echo_event_id", echoID, "original_event_id", originalID,
				"echo_sequence_id", echoSeq)
			return echoID, true, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return "", false, fmt.Errorf("publish thread reply echo: %w", err)
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return "", false, fmt.Errorf("publish thread reply echo after %d attempts: %w", maxThreadCreateAppendAttempts, lastErr)
}

func (c *ChattoCore) hideChannelEchoForReply(ctx context.Context, actorID string, kind RoomKind, agg events.Aggregate, roomID, originalEventID string) error {
	retractSubject := agg.Subject(events.EventMessageRetracted)
	var lastErr error

	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		expectedSeq, err := c.EventPublisher.LastSubjectSeq(ctx, retractSubject)
		if err != nil {
			return fmt.Errorf("read echo retract OCC tail: %w", err)
		}
		if expectedSeq > 0 {
			if err := c.roomService.waitForTimeline(ctx, events.SubjectPosition(retractSubject, expectedSeq)); err != nil {
				return err
			}
		}
		echoID, ok := c.RoomTimeline.ChannelEchoEventID(originalEventID)
		if !ok {
			return nil
		}

		event := newEvent(actorID, &corev1.Event{
			Event: &corev1.Event_MessageRetracted{
				MessageRetracted: &corev1.MessageRetractedEvent{
					RoomId:  roomID,
					EventId: echoID,
				},
			},
		})
		seq, err := c.EventPublisher.AppendAt(ctx, retractSubject, event, expectedSeq)
		if err == nil {
			if err := c.roomService.waitForTimeline(ctx, events.SubjectPosition(retractSubject, seq)); err != nil {
				return err
			}
			c.logger.Info("Message echo hidden", "kind", kind, "room_id", roomID, "event_id", echoID, "actor_id", actorID)
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return fmt.Errorf("publish echo retraction: %w", err)
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return fmt.Errorf("publish echo retraction after %d attempts: %w", maxThreadCreateAppendAttempts, lastErr)
}

func (c *ChattoCore) appendMessageWithOptionalThreadCreated(ctx context.Context, agg events.Aggregate, bodyEvent, messageEvent, threadCreatedEvent *corev1.Event, threadRootEventID string) (uint64, error) {
	if threadCreatedEvent == nil || threadRootEventID == "" || c.rooms().threadExists(threadRootEventID) {
		return c.appendBodyAndMessage(ctx, agg, bodyEvent, messageEvent)
	}
	if exists, err := c.threadCreatedExistsInStream(ctx, agg, threadRootEventID); err != nil {
		return 0, fmt.Errorf("check existing thread creation: %w", err)
	} else if exists {
		return c.appendBodyAndMessage(ctx, agg, bodyEvent, messageEvent)
	}

	roomFilter := agg.AllEventsFilter()
	threadCreatedSubject := agg.Subject(events.EventThreadCreated)
	bodySubject := agg.SubjectFor(bodyEvent)
	messageSubject := agg.SubjectFor(messageEvent)
	var lastErr error

	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		expectedSeq, err := c.EventPublisher.LastSubjectSeq(ctx, roomFilter)
		if err != nil {
			return 0, fmt.Errorf("read room OCC tail: %w", err)
		}
		seqs, err := c.EventPublisher.AppendBatch(ctx, []events.BatchEntry{
			{
				Subject:       threadCreatedSubject,
				Event:         threadCreatedEvent,
				ExpectedSeq:   expectedSeq,
				FilterSubject: roomFilter,
				HasOCC:        true,
			},
			{
				Subject: bodySubject,
				Event:   bodyEvent,
			},
			{
				Subject: messageSubject,
				Event:   messageEvent,
			},
		})
		if err == nil {
			messageSeq := seqs[len(seqs)-1]
			if err := c.rooms().waitForTimeline(ctx, events.SubjectPosition(messageSubject, messageSeq)); err != nil {
				return messageSeq, err
			}
			return messageSeq, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return 0, err
		}
		lastErr = err

		currentSeq, seqErr := c.EventPublisher.LastSubjectSeq(ctx, roomFilter)
		if seqErr != nil {
			return 0, fmt.Errorf("read room OCC tail after conflict: %w", seqErr)
		}
		if currentSeq > 0 {
			if err := c.rooms().waitForTimeline(ctx, events.SubjectPosition(roomFilter, currentSeq)); err != nil {
				return 0, err
			}
		}
		if c.rooms().threadExists(threadRootEventID) {
			return c.appendBodyAndMessage(ctx, agg, bodyEvent, messageEvent)
		}
		if exists, err := c.threadCreatedExistsInStream(ctx, agg, threadRootEventID); err != nil {
			return 0, fmt.Errorf("check existing thread creation after conflict: %w", err)
		} else if exists {
			return c.appendBodyAndMessage(ctx, agg, bodyEvent, messageEvent)
		}
	}

	return 0, fmt.Errorf("append thread creation after %d attempts: %w", maxThreadCreateAppendAttempts, lastErr)
}

// PostMessage posts a message to a room. Publishes a
// MessagePostedEvent on evt.room.{R}.message_posted with the
// encrypted body in a companion MessageBodyEvent.
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
func (c *ChattoCore) PostMessage(ctx context.Context, kind RoomKind, room_id, user_id, body string, assetIDs []string, inThread, inReplyTo string, linkPreview *corev1.LinkPreview, alsoSendToChannel bool, opts ...PostMessageOption) (*corev1.Event, error) {
	options := collectPostMessageOptions(opts)

	// Validate message body length to prevent DoS via oversized messages
	if len(body) > MaxMessageBodyLength {
		return nil, ErrMessageTooLong
	}
	if err := validateLinkPreview(linkPreview); err != nil {
		return nil, err
	}
	if err := c.HydrateLinkPreviewImageAsset(ctx, linkPreview); err != nil {
		return nil, err
	}
	if err := validateLinkPreview(linkPreview); err != nil {
		return nil, err
	}

	// Validate that message has either body or attachments.
	// HasVisibleContent rejects messages with only invisible Unicode characters.
	hasBody := HasVisibleContent(body)
	hasAttachments := len(assetIDs) > 0
	if !hasBody && !hasAttachments {
		return nil, invalidArgument("message must have either body or attachments")
	}

	// Resolve referenced assets from the projection. Each must already exist
	// (UploadAttachment emitted AssetCreatedEvent before the caller routed
	// the id here). Missing ids are dropped with a warning rather than
	// failing the post — the user already typed and clicked Send; a transient
	// projection lag for one attachment is better swallowed than fatal.
	resolvedAssets := make([]*corev1.Attachment, 0, len(assetIDs))
	resolvedAssetIDs := make([]string, 0, len(assetIDs))
	for _, id := range assetIDs {
		if id == "" {
			continue
		}
		declared, ok := c.assetLifecycle().AssetCreation(id)
		if !ok || declared == nil || declared.GetAsset() == nil {
			c.logger.Warn("PostMessage references unknown asset; dropping",
				"asset_id", id, "room_id", room_id, "actor_id", user_id)
			continue
		}
		assetRoomID, ok := c.assetLifecycle().AssetRoomID(id)
		if !ok || assetRoomID != room_id {
			c.logger.Warn("PostMessage references asset outside room; dropping",
				"asset_id", id, "asset_room_id", assetRoomID, "room_id", room_id, "actor_id", user_id)
			continue
		}
		att := attachmentFromAsset(declared.GetAsset())
		if att == nil {
			continue
		}
		att.RoomId = room_id
		resolvedAssets = append(resolvedAssets, att)
		resolvedAssetIDs = append(resolvedAssetIDs, id)
	}
	if !hasBody && len(resolvedAssetIDs) == 0 {
		return nil, invalidArgument("message must have either body or attachments")
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
			return nil, fmt.Errorf("thread root message not found: %w", ErrMessageNotFound)
		}
		rootMsg := rootEvent.GetMessagePosted()
		if rootMsg == nil {
			return nil, invalidArgument("thread root is not a message event")
		}
		// Verify it's actually a root message (not itself a thread reply)
		if rootMsg.InThread != "" {
			return nil, invalidArgument("thread root must be a root message, not a thread reply")
		}
	}

	now := time.Now()

	// Extract and resolve @mentions from message body
	var mentionedUserIDs []string
	if hasBody {
		usernames := ExtractMentionUsernames(body)
		if len(usernames) > 0 {
			resolved, err := c.ResolveRoomMentions(ctx, kind, room_id, usernames)
			if err != nil {
				c.logger.Warn("Failed to resolve mentions", "error", err)
				// Continue without mentions - don't fail the message
			} else {
				mentionedUserIDs = resolved
			}
		}
	}

	if len(mentionedUserIDs) > 0 && !options.largeMentionConfirmed {
		recipientCount, err := c.mentionNotificationRecipientCount(ctx, room_id, user_id, mentionedUserIDs)
		if err != nil {
			c.logger.Warn("Failed to count mention notification recipients", "error", err, "room_id", room_id)
		} else if recipientCount > LargeMentionNotificationThreshold {
			return nil, &MentionConfirmationRequiredError{RecipientCount: recipientCount}
		}
	}

	eventID := NewEventID()
	bodyEventID := NewEventID()
	messageBody := &corev1.MessageBody{
		CreatedAt:   timestamppb.New(now),
		AssetIds:    resolvedAssetIDs,
		AuthorId:    user_id,
		LinkPreview: linkPreview,
	}
	if err := c.encryptMessageBody(ctx, messageBody, room_id, eventID, bodyEventID, body); err != nil {
		return nil, err
	}
	bodyEventEvent := newEvent(user_id, &corev1.Event{
		Id:        bodyEventID,
		CreatedAt: timestamppb.New(now),
		Event: &corev1.Event_MessageBody{
			MessageBody: &corev1.MessageBodyEvent{
				RoomId:  room_id,
				EventId: eventID,
				Body:    messageBody,
			},
		},
	})

	event := newEvent(user_id, &corev1.Event{
		Id:        eventID,
		CreatedAt: timestamppb.New(now),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				RoomId:           room_id,
				InReplyTo:        inReplyTo,
				InThread:         inThread,
				MentionedUserIds: mentionedUserIDs,
			},
		},
	})
	var threadCreatedEvent *corev1.Event
	if inThread != "" && !c.rooms().threadExists(inThread) {
		threadCreatedEvent = newEvent(user_id, &corev1.Event{
			Id:        NewEventID(),
			CreatedAt: timestamppb.New(now),
			Event: &corev1.Event_ThreadCreated{
				ThreadCreated: &corev1.ThreadCreatedEvent{
					RoomId:            room_id,
					ThreadRootEventId: inThread,
				},
			},
		})
	}
	// Schedule any video processing before MessagePosted so AssetProcessing-
	// Started fires before subscribers see the message; the frontend uses
	// the started marker to render the "Processing…" placeholder.
	//
	// The asset itself was already created at upload time
	// (UploadAttachment → AssetCreatedEvent); here we just trigger derivative
	// processing for any referenced asset the caller flagged as a video.
	for _, att := range resolvedAssets {
		if !options.shouldScheduleVideoProcessingForID(att.GetId()) {
			continue
		}
		if err := c.ScheduleVideoProcessingForMessageAttachment(ctx, user_id, kind, room_id, event.Id, att); err != nil {
			c.logger.Warn("Failed to schedule video processing",
				"room_id", room_id,
				"message_event_id", event.Id,
				"asset_id", att.GetId(),
				"error", err)
		}
	}

	// Publish to EVT. MessagePosted is append-only per #597's design, so
	// retrying the same payload after an OCC conflict is safe.
	// AppendEventuallyAndWait blocks until the RoomTimelineProjection
	// has caught up, giving read-your-writes for subsequent reads from
	// this request.
	agg := events.RoomAggregate(room_id)
	sequenceID, err := c.appendMessageWithOptionalThreadCreated(ctx, agg, bodyEventEvent, event, threadCreatedEvent, inThread)
	if err != nil {
		return nil, fmt.Errorf("failed to publish message event: %w", err)
	}
	// Also wait for ThreadProjection if this is a thread reply, so a
	// subsequent thread-pane fetch from the same request sees it.
	if inThread != "" {
		if err := c.rooms().waitForThreads(ctx, events.SubjectPosition(agg.SubjectFor(event), sequenceID)); err != nil {
			c.logger.Debug("ThreadsProjector did not catch up", "error", err)
		}
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
			if participants, err := c.GetRoomMembersList(ctx, KindDM, room_id); err == nil {
				for _, participant := range participants {
					alreadyNotified[participant.UserId] = true
				}
			}
		}
		c.notifyAllMessageSubscribers(ctx, kind, room_id, user_id, event.Id, alreadyNotified)
	}

	// Publish echo event to the message subject if "also send to channel" was requested.
	// The echo references the original event_id, so resolvers can fold
	// it back to the underlying body. The body is encrypted again for the
	// echo event ID because v2 encryption authenticates the event context.
	if inThread != "" && alsoSendToChannel {
		echoID, created, err := c.appendThreadReplyEcho(ctx, user_id, kind, agg, event, event.GetMessagePosted(), messageBody, body)
		if err != nil {
			c.logger.Warn("Failed to publish thread reply echo", "error", err, "thread_reply_event_id", event.Id)
		} else if created {
			// Notify room members with ALL_MESSAGES notification level (best-effort).
			// Build already-notified set: author + mentioned users (already notified above for original reply).
			echoAlreadyNotified := make(map[string]bool)
			echoAlreadyNotified[user_id] = true
			for _, uid := range mentionedUserIDs {
				echoAlreadyNotified[uid] = true
			}
			c.notifyAllMessageSubscribers(ctx, kind, room_id, user_id, echoID, echoAlreadyNotified)
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

// DeleteMessage retracts a message. For ordinary messages and original thread
// replies, the retraction removes visible content and attachments for GDPR
// compliance while preserving the event in the stream for audit. For echoes,
// the same durable MessageRetractedEvent hides only the echo artifact from the
// room timeline; the original thread reply remains readable.
// The messageBodyKey parameter is the legacy body key or canonical event ID.
// Authorization: Caller must verify the actor is the message author OR
// CanManageOthersMessage before calling.
func (c *ChattoCore) DeleteMessage(ctx context.Context, actorID string, kind RoomKind, roomID, messageBodyKey string) error {
	eventID := eventIDFromBodyKey(messageBodyKey)
	if eventID == "" {
		return ErrMessageNotFound
	}

	// Snapshot the projection state for attachment cleanup before
	// emitting the retract event. After retract, LatestBody returns
	// nil (the message is tombstoned), so we need a copy first.
	originalEntry, ok := c.rooms().timelineEntry(eventID)
	if !ok {
		c.logger.Debug("Delete on unknown message — no-op", "event_id", eventID)
		return nil
	}
	isEcho := c.rooms().isEcho(eventID)
	if isEcho && c.rooms().isHiddenEcho(eventID) {
		return nil
	}
	body, retracted, _ := c.rooms().latestBody(eventID)
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
	c.secureDeleteAllMessageBodyEvents(ctx, eventID)
	if isEcho {
		c.logger.Info("Message echo hidden", "kind", kind, "room_id", roomID, "event_id", eventID, "actor_id", actorID, "envelope_seq", originalEntry.StreamSeq)
		return nil
	}
	for _, linkedID := range c.rooms().linkedEventIDs(eventID) {
		c.secureDeleteAllMessageBodyEvents(ctx, linkedID)
	}

	// Attachments are referenced by the (now-tombstoned) message but
	// the binary blobs in the asset store don't get cleaned up by the
	// event log. Same posture as the legacy DeleteMessage path —
	// best-effort, log warnings, keep going.
	if body != nil {
		for _, att := range c.MessageBodyAttachments(body) {
			c.DeleteVideoDerivativesForAttachment(ctx, actorID, kind, att.GetId())
			if err := c.RecordAssetDeleted(ctx, actorID, kind, roomID, att.GetId()); err != nil {
				c.logger.Warn("Failed to publish asset deletion event",
					"attachment_id", att.GetId(),
					"event_id", eventID,
					"error", err)
				continue
			}
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
// Publishes a MessageEditedEvent to notify connected clients in real-time.
// The messageBodyKey parameter is the full compound key ({userId}.{bodyId}) stored in the event.
//
// Business rule: Authors can only edit their own messages within MessageEditWindow (3 hours).
// Non-authors (moderators with message.manage) can edit at any time.
//
// Authorization: Caller must verify the actor is the author OR
// CanManageOthersMessage before calling.
func (c *ChattoCore) EditMessage(ctx context.Context, actorID string, kind RoomKind, roomID, messageBodyKey, newBody string, opts ...EditMessageOption) error {
	options := collectEditMessageOptions(opts)
	if len(newBody) > MaxMessageBodyLength {
		return ErrMessageTooLong
	}

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
	originalEntry, ok := c.rooms().timelineEntry(eventID)
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
	if options.channelEcho != nil {
		echoTargetEvent := originalEntry.Event
		echoTargetPost := origPost
		if echoOf := origPost.GetEchoOfEventId(); echoOf != "" {
			origEchoEntry, ok := c.RoomTimeline.Get(echoOf)
			if !ok || origEchoEntry.Event == nil {
				return ErrMessageNotFound
			}
			echoTargetEvent = origEchoEntry.Event
			echoTargetPost = echoTargetEvent.GetMessagePosted()
		}
		if echoTargetPost == nil || echoTargetPost.GetEchoOfEventId() != "" || echoTargetPost.GetInThread() == "" {
			return fmt.Errorf("channel echo state can only be changed for thread replies")
		}
		if roomIDOfEvent(echoTargetEvent) != roomID {
			return ErrMessageNotFound
		}
		if echoTargetEvent.GetActorId() != actorID {
			return ErrNotMessageAuthor
		}
		if time.Since(echoTargetEvent.GetCreatedAt().AsTime()) > MessageEditWindow {
			return ErrEditWindowExpired
		}
	}

	// Fold in current body so attachments/link preview/timestamps
	// survive the edit. We then overwrite ciphertext + nonce with the
	// new content.
	current, retracted, _ := c.rooms().latestBody(eventID)
	if retracted {
		return ErrMessageNotFound
	}
	if current == nil {
		// Imported legacy event with no body — nothing to edit.
		return ErrMessageNotFound
	}

	updated := proto.Clone(current).(*corev1.MessageBody)
	updated.UpdatedAt = timestamppb.Now()
	bodyEventID := NewEventID()
	if err := c.encryptMessageBody(ctx, updated, roomID, eventID, bodyEventID, newBody); err != nil {
		if updated.GetAuthorId() == "" {
			return fmt.Errorf("cannot edit: message body author is empty")
		}
		return err
	}

	agg := events.RoomAggregate(roomID)
	if err := c.publishMessageEdit(ctx, actorID, kind, agg, roomID, eventID, updated); err != nil {
		return err
	}
	c.secureDeleteObsoleteMessageBodyEvents(ctx, eventID)
	// Fan out to echoes (and to the original if this IS an echo) so
	// the legacy "edit one, both update" semantic is preserved.
	for _, linkedID := range c.rooms().linkedEventIDs(eventID) {
		linkedBody := proto.Clone(updated).(*corev1.MessageBody)
		linkedBodyEventID := NewEventID()
		if err := c.encryptMessageBody(ctx, linkedBody, roomID, linkedID, linkedBodyEventID, newBody); err != nil {
			c.logger.Warn("Failed to encrypt linked message edit",
				"source_event_id", eventID, "linked_event_id", linkedID, "error", err)
			continue
		}
		if err := c.publishMessageEdit(ctx, actorID, kind, agg, roomID, linkedID, linkedBody); err != nil {
			c.logger.Warn("Failed to propagate edit to linked message",
				"source_event_id", eventID, "linked_event_id", linkedID, "error", err)
			continue
		}
		c.secureDeleteObsoleteMessageBodyEvents(ctx, linkedID)
	}

	c.logger.Info("Message edited", "kind", kind, "room_id", roomID, "event_id", eventID, "actor_id", actorID)
	if options.channelEcho != nil {
		if err := c.reconcileEditedMessageChannelEcho(ctx, actorID, kind, roomID, eventID, *options.channelEcho); err != nil {
			return err
		}
	}
	return nil
}

func (c *ChattoCore) reconcileEditedMessageChannelEcho(ctx context.Context, actorID string, kind RoomKind, roomID, eventID string, enabled bool) error {
	entry, ok := c.RoomTimeline.Get(eventID)
	if !ok || entry.Event == nil {
		return ErrMessageNotFound
	}
	posted := entry.Event.GetMessagePosted()
	if posted == nil {
		return ErrMessageNotFound
	}

	originalEvent := entry.Event
	originalPost := posted
	originalID := eventID
	if echoOf := posted.GetEchoOfEventId(); echoOf != "" {
		originalID = echoOf
		originalEntry, ok := c.RoomTimeline.Get(originalID)
		if !ok || originalEntry.Event == nil {
			return ErrMessageNotFound
		}
		originalEvent = originalEntry.Event
		originalPost = originalEvent.GetMessagePosted()
	}
	if originalPost == nil || originalPost.GetEchoOfEventId() != "" || originalPost.GetInThread() == "" {
		return fmt.Errorf("channel echo state can only be changed for thread replies")
	}
	if roomIDOfEvent(originalEvent) != roomID {
		return ErrMessageNotFound
	}
	if originalEvent.GetActorId() != actorID {
		return ErrNotMessageAuthor
	}
	if time.Since(originalEvent.GetCreatedAt().AsTime()) > MessageEditWindow {
		return ErrEditWindowExpired
	}
	current, retracted, _ := c.RoomTimeline.LatestBody(originalID)
	if retracted || current == nil {
		return ErrMessageNotFound
	}

	agg := events.RoomAggregate(roomID)
	if !enabled {
		return c.hideChannelEchoForReply(ctx, actorID, kind, agg, roomID, originalID)
	}
	plaintext, err := c.decryptMessageBody(ctx, originalID, roomID, current)
	if err != nil {
		return fmt.Errorf("decrypt message body for echo: %w", err)
	}
	echoID, created, err := c.appendThreadReplyEcho(ctx, actorID, kind, agg, originalEvent, originalPost, current, string(plaintext))
	if err != nil {
		return err
	}
	if created && echoID != "" {
		alreadyNotified := make(map[string]bool)
		alreadyNotified[actorID] = true
		for _, uid := range originalPost.GetMentionedUserIds() {
			alreadyNotified[uid] = true
		}
		c.notifyAllMessageSubscribers(ctx, kind, roomID, actorID, echoID, alreadyNotified)
	}
	return nil
}

// publishMessageRetract emits a MessageRetractedEvent on EVT. StreamMyEvents
// receives the canonical live.evt.> republish directly. Factored out so
// DeleteMessage can fan to linked messages.
func (c *ChattoCore) publishMessageRetract(ctx context.Context, actorID string, kind RoomKind, agg events.Aggregate, roomID, eventID string) error {
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_MessageRetracted{
			MessageRetracted: &corev1.MessageRetractedEvent{
				RoomId:  roomID,
				EventId: eventID,
			},
		},
	})
	if _, err := c.rooms().appendTimelineEventually(ctx, c.EventPublisher, agg, event); err != nil {
		return fmt.Errorf("publish MessageRetractedEvent: %w", err)
	}

	return nil
}

// publishMessageEdit emits a MessageEditedEvent on EVT. StreamMyEvents
// receives the canonical live.evt.> republish directly. Factored out so
// EditMessage / editEmbeddedBody can fan the same payload to linked messages.
func (c *ChattoCore) publishMessageEdit(ctx context.Context, actorID string, kind RoomKind, agg events.Aggregate, roomID, eventID string, body *corev1.MessageBody) error {
	if body == nil {
		return fmt.Errorf("message edit body is nil")
	}
	bodyEventID := body.GetBodyEventId()
	if bodyEventID == "" {
		return fmt.Errorf("message edit body event ID is empty")
	}
	bodyEvent := newEvent(actorID, &corev1.Event{
		Id: bodyEventID,
		Event: &corev1.Event_MessageBody{
			MessageBody: &corev1.MessageBodyEvent{
				RoomId:  roomID,
				EventId: eventID,
				Body:    body,
			},
		},
	})
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_MessageEdited{
			MessageEdited: &corev1.MessageEditedEvent{
				RoomId:  roomID,
				EventId: eventID,
			},
		},
	})
	bodySubject := agg.SubjectFor(bodyEvent)
	editSubject := agg.SubjectFor(event)
	var lastErr error
	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		expectedSeq, err := c.EventPublisher.LastSubjectSeq(ctx, editSubject)
		if err != nil {
			return fmt.Errorf("read edit OCC tail: %w", err)
		}
		seqs, err := c.EventPublisher.AppendBatch(ctx, []events.BatchEntry{
			{
				Subject:       bodySubject,
				Event:         bodyEvent,
				ExpectedSeq:   expectedSeq,
				FilterSubject: editSubject,
				HasOCC:        true,
			},
			{
				Subject: editSubject,
				Event:   event,
			},
		})
		if err == nil {
			if err := c.rooms().waitForTimeline(ctx, events.SubjectPosition(editSubject, seqs[len(seqs)-1])); err != nil {
				return err
			}
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return fmt.Errorf("publish MessageEditedEvent: %w", err)
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("publish MessageEditedEvent after %d attempts: %w", maxThreadCreateAppendAttempts, lastErr)
	}
	return fmt.Errorf("publish MessageEditedEvent: retry loop exited unexpectedly")
}

func validateLinkPreview(linkPreview *corev1.LinkPreview) error {
	if linkPreview == nil {
		return nil
	}
	if err := validateStringMaxLength("link preview URL", linkPreview.GetUrl(), MaxLinkPreviewURLLength); err != nil {
		return err
	}
	if err := validateStringMaxLength("link preview title", linkPreview.GetTitle(), MaxLinkPreviewTitleLength); err != nil {
		return err
	}
	if err := validateStringMaxLength("link preview description", linkPreview.GetDescription(), MaxLinkPreviewDescriptionLength); err != nil {
		return err
	}
	if err := validateStringMaxLength("link preview image asset ID", linkPreview.GetImageAssetId(), MaxLinkPreviewImageAssetIDLength); err != nil {
		return err
	}
	if imageAsset := linkPreview.GetImageAsset(); imageAsset != nil {
		if err := validateStringMaxLength("link preview image asset ID", imageAsset.GetId(), MaxLinkPreviewImageAssetIDLength); err != nil {
			return err
		}
		if linkPreview.GetImageAssetId() != "" && imageAsset.GetId() != "" && linkPreview.GetImageAssetId() != imageAsset.GetId() {
			return invalidArgument("link preview image asset ID does not match image asset record")
		}
		if imageAsset.GetStorage() == nil {
			return invalidArgument("link preview image asset record is missing storage")
		}
	}
	if err := validateStringMaxLength("link preview site name", linkPreview.GetSiteName(), MaxLinkPreviewSiteNameLength); err != nil {
		return err
	}
	if err := validateStringMaxLength("link preview embed type", linkPreview.GetEmbedType(), MaxLinkPreviewEmbedTypeLength); err != nil {
		return err
	}
	if err := validateStringMaxLength("link preview embed ID", linkPreview.GetEmbedId(), MaxLinkPreviewEmbedIDLength); err != nil {
		return err
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
	entry, ok := c.rooms().timelineEntry(eventID)
	if !ok {
		return ErrMessageNotFound
	}
	if entry.Event.GetMessagePosted() == nil {
		return ErrMessageNotFound
	}
	current, retracted, _ := c.rooms().latestBody(eventID)
	if retracted || current == nil {
		return ErrMessageNotFound
	}
	if current.GetAuthorId() != actorID {
		return ErrNotMessageAuthor
	}
	plaintext, err := c.decryptMessageBody(ctx, eventID, roomID, current)
	if err != nil {
		return fmt.Errorf("decrypt message body for edit: %w", err)
	}
	updated := proto.Clone(current).(*corev1.MessageBody)
	if err := mutate(updated); err != nil {
		return err
	}
	updated.UpdatedAt = timestamppb.Now()
	bodyEventID := NewEventID()
	if err := c.encryptMessageBody(ctx, updated, roomID, eventID, bodyEventID, string(plaintext)); err != nil {
		return err
	}

	agg := events.RoomAggregate(roomID)
	if err := c.publishMessageEdit(ctx, actorID, kind, agg, roomID, eventID, updated); err != nil {
		return err
	}
	c.secureDeleteObsoleteMessageBodyEvents(ctx, eventID)
	for _, linkedID := range c.rooms().linkedEventIDs(eventID) {
		linkedCurrent, linkedRetracted, _ := c.rooms().latestBody(linkedID)
		if linkedRetracted || linkedCurrent == nil {
			continue
		}
		linkedBody := proto.Clone(linkedCurrent).(*corev1.MessageBody)
		metadata := proto.Clone(updated).(*corev1.MessageBody)
		linkedBody.AssetIds = append([]string(nil), metadata.GetAssetIds()...)
		linkedBody.Attachments = metadata.GetAttachments()
		linkedBody.LinkPreview = metadata.GetLinkPreview()
		linkedBody.UpdatedAt = metadata.GetUpdatedAt()
		linkedBodyEventID := NewEventID()
		if err := c.encryptMessageBody(ctx, linkedBody, roomID, linkedID, linkedBodyEventID, string(plaintext)); err != nil {
			c.logger.Warn("Failed to encrypt linked message partial edit",
				"source_event_id", eventID, "linked_event_id", linkedID, "error", err)
			continue
		}
		if err := c.publishMessageEdit(ctx, actorID, kind, agg, roomID, linkedID, linkedBody); err != nil {
			c.logger.Warn("Failed to propagate partial edit to linked message",
				"source_event_id", eventID, "linked_event_id", linkedID, "error", err)
			continue
		}
		c.secureDeleteObsoleteMessageBodyEvents(ctx, linkedID)
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
		// Resolve the attachment (new bodies hold IDs; older bodies hold
		// embedded protos). Then trim from whichever shape holds it.
		for _, att := range c.MessageBodyAttachments(body) {
			if att.GetId() == attachmentID {
				removed = att
				break
			}
		}
		if removed == nil {
			return fmt.Errorf("attachment not found in message")
		}
		trimmedIDs := body.AssetIds[:0]
		for _, id := range body.GetAssetIds() {
			if id != attachmentID {
				trimmedIDs = append(trimmedIDs, id)
			}
		}
		body.AssetIds = trimmedIDs
		trimmedAttachments := body.Attachments[:0]
		for _, att := range body.GetAttachments() {
			if att.GetId() != attachmentID {
				trimmedAttachments = append(trimmedAttachments, att)
			}
		}
		body.Attachments = trimmedAttachments
		return nil
	})
	if err != nil {
		return err
	}

	if removed != nil {
		c.DeleteVideoDerivativesForAttachment(ctx, actorID, kind, removed.GetId())
		if err := c.RecordAssetDeleted(ctx, actorID, kind, roomID, removed.GetId()); err != nil {
			c.logger.Warn("Failed to publish asset deletion event",
				"attachment_id", attachmentID,
				"message_body_key", messageBodyKey,
				"error", err)
		} else if delErr := c.DeleteAttachmentFromStorage(ctx, removed); delErr != nil {
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
