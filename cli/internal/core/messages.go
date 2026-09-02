package core

import (
	"context"
	"errors"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	defaultHistoricalMessageLimit = 50
	maxHistoricalMessageLimit     = 500
)

type postMessageOptions struct {
	videoProcessingAssetIDs map[string]struct{}
	createThread            bool
	commitAuthorize         func(context.Context, string) error
	messageAttemptPrepared  func(context.Context) error
	echoAttemptPrepared     func(context.Context) error
}

type editMessageOptions struct {
	channelEcho     *bool
	preserveBody    bool
	authorize       bool
	commitAuthorize func(context.Context) error
	now             func() time.Time
}

type deleteMessageOptions struct {
	commitAuthorize func(context.Context) error
}

// PostMessageOption customizes side effects owned by the message-post command.
type PostMessageOption func(*postMessageOptions)

// EditMessageOption customizes side effects owned by the message-edit command.
type EditMessageOption func(*editMessageOptions)

// DeleteMessageOption customizes the message-retraction command.
type DeleteMessageOption func(*deleteMessageOptions)

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

// WithThreadCreation establishes a durable thread for the new root message and
// follows it for the author in the same atomic append as the message.
func WithThreadCreation() PostMessageOption {
	return func(options *postMessageOptions) {
		options.createThread = true
	}
}

// withPostMessageCommitAuthorization installs the authoritative authorization
// check run inside every OCC attempt. It stays package-private because public
// transports must go through MessageModel, which owns user-facing policy.
func withPostMessageCommitAuthorization(authorize func(context.Context, string) error) PostMessageOption {
	return func(options *postMessageOptions) {
		options.commitAuthorize = authorize
	}
}

// withPostMessageAttemptPrepared installs a package-private test hook after
// source-time mention resolution but before the guarded append. It lets
// concurrency tests deterministically force an OCC retry at that exact
// boundary without exposing another production API.
func withPostMessageAttemptPrepared(hook func(context.Context) error) PostMessageOption {
	return func(options *postMessageOptions) {
		options.messageAttemptPrepared = hook
	}
}

// withThreadReplyEchoAttemptPrepared installs a package-private test hook
// after echo policy validation but before the guarded append. Concurrency tests
// use it to force a room-policy conflict at that exact boundary.
func withThreadReplyEchoAttemptPrepared(hook func(context.Context) error) PostMessageOption {
	return func(options *postMessageOptions) {
		options.echoAttemptPrepared = hook
	}
}

// WithMessageChannelEcho reconciles whether a thread reply should have a
// visible echo in the channel timeline after the edit is saved.
func WithMessageChannelEcho(enabled bool) EditMessageOption {
	return func(options *editMessageOptions) {
		options.channelEcho = &enabled
	}
}

// withPreservedMessageBody keeps the latest committed plaintext while applying
// other edit-time state such as channel-echo reconciliation. It is private so
// transports must express intent through MessageModel's optional body field.
func withPreservedMessageBody() EditMessageOption {
	return func(options *editMessageOptions) {
		options.preserveBody = true
	}
}

// withEditMessageAuthorization enables the authoritative policy check inside
// every OCC attempt. It stays package-private because public transports must
// go through MessageModel, which owns user-facing policy.
func withEditMessageAuthorization() EditMessageOption {
	return func(options *editMessageOptions) {
		options.authorize = true
	}
}

// withEditMessageCommitAuthorization adds an operation-level check inside
// every OCC attempt. The built-in message policy still runs first; this hook is
// package-private so tests can deterministically place a concurrent authority
// change between that decision and the append.
func withEditMessageCommitAuthorization(authorize func(context.Context) error) EditMessageOption {
	return func(options *editMessageOptions) {
		options.commitAuthorize = authorize
	}
}

func withEditMessageClock(now func() time.Time) EditMessageOption {
	return func(options *editMessageOptions) {
		options.now = now
	}
}

func withDeleteMessageCommitAuthorization(authorize func(context.Context) error) DeleteMessageOption {
	return func(options *deleteMessageOptions) {
		options.commitAuthorize = authorize
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

func collectDeleteMessageOptions(opts []DeleteMessageOption) deleteMessageOptions {
	var options deleteMessageOptions
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

func (c *ChattoCore) waitForMessageBodyAssets(ctx context.Context, subject string, seq uint64) error {
	if c.assetModel == nil || c.assetModel.assets.Projector() == nil {
		return nil
	}
	return c.assetModel.waitForAssets(ctx, events.SubjectPosition(subject, seq))
}

func (c *ChattoCore) threadCreatedExistsInStream(ctx context.Context, agg evtstream.Aggregate, threadRootEventID string) (bool, error) {
	if threadRootEventID == "" {
		return false, nil
	}
	existing, _, err := c.EventPublisher.SubjectEvents(ctx, agg.Subject(evtstream.EventThreadCreated))
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

type messageAppendAttempt struct {
	roomFilter string
	roomSeq    uint64
}

// prepareMessageAssetBatchEntries adds one AssetAttached event per attachment
// and any associated processing marker to the atomic message batch. Each
// attachment is fenced against the complete asset aggregate, so a competing
// message attachment, pending-asset expiry, deletion, or processing transition
// rejects the batch.
func (c *ChattoCore) prepareMessageAssetBatchEntries(
	ctx context.Context,
	entries []evtstream.BatchEntry,
	assetAttachedEvents []*evtv1.Event,
	processingEvents []*evtv1.Event,
) ([]evtstream.BatchEntry, error) {
	processingByAssetID := make(map[string]*evtv1.Event, len(processingEvents))
	for _, event := range processingEvents {
		if event == nil || event.GetAssetProcessingStarted() == nil {
			return nil, fmt.Errorf("asset processing batch entry has invalid event")
		}
		processingByAssetID[event.GetAssetProcessingStarted().GetAssetId()] = event
	}
	for _, attachedEvent := range assetAttachedEvents {
		attached := attachedEvent.GetAssetAttached()
		assetID := attached.GetAssetId()
		if assetID == "" {
			return nil, fmt.Errorf("asset attachment batch entry missing asset id")
		}
		agg := evtstream.AssetAggregate(assetID)
		filter := agg.AllEventsFilter()
		tail, err := c.EventPublisher.LastSubjectPosition(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("read asset attachment OCC tail: %w", err)
		}
		if !tail.IsZero() {
			if err := c.assetModel.waitForAssets(ctx, tail); err != nil {
				return nil, fmt.Errorf("wait for asset attachment projection: %w", err)
			}
		}
		if err := c.assetModel.validateAssetAttachment(assetID, attached.GetUserId(), attached.GetRoomId(), attached.GetMessageEventId(), time.Now()); err != nil {
			return nil, err
		}
		entries = append(entries, evtstream.BatchEntry{
			Subject:       agg.SubjectFor(attachedEvent),
			Event:         attachedEvent,
			ExpectedSeq:   tail.Seq,
			FilterSubject: filter,
			HasOCC:        true,
		})
		if processingEvent := processingByAssetID[assetID]; processingEvent != nil {
			if c.assetModel.shouldAppendAssetProcessingEvent(assetID, processingEvent) {
				entries = append(entries, evtstream.BatchEntry{
					Subject: agg.SubjectFor(processingEvent),
					Event:   processingEvent,
				})
			}
			delete(processingByAssetID, assetID)
		}
	}
	if len(processingByAssetID) > 0 {
		return nil, fmt.Errorf("asset processing batch entry has no matching attachment")
	}
	return entries, nil
}

func (c *ChattoCore) waitForMessageAssetBatch(
	ctx context.Context,
	entries []evtstream.BatchEntry,
	seqs []uint64,
	first int,
) error {
	for i := first; i < len(entries); i++ {
		if entries[i].Event.GetAssetAttached() == nil && entries[i].Event.GetAssetProcessingStarted() == nil {
			continue
		}
		if err := c.assetModel.waitForAssets(ctx, events.SubjectPosition(entries[i].Subject, seqs[i])); err != nil {
			return err
		}
	}
	return nil
}

// prepareMessageAppendAttempt captures the target room OCC boundary, waits for
// the serving room projections, and evaluates cross-aggregate authorization
// from a stable request-time read. The returned room sequence must guard the
// atomic domain batch.
func (c *ChattoCore) prepareMessageAppendAttempt(
	ctx context.Context,
	agg evtstream.Aggregate,
	authorize func(context.Context) error,
) (messageAppendAttempt, error) {
	attempt := messageAppendAttempt{roomFilter: agg.AllEventsFilter()}
	var err error
	attempt.roomSeq, err = c.EventPublisher.LastSubjectSeq(ctx, attempt.roomFilter)
	if err != nil {
		return messageAppendAttempt{}, fmt.Errorf("read room OCC tail: %w", err)
	}

	if attempt.roomSeq > 0 {
		if err := c.roomModel.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(attempt.roomFilter, attempt.roomSeq)); err != nil {
			return messageAppendAttempt{}, fmt.Errorf("wait for room mutation projections: %w", err)
		}
	}
	// Notification recipients for thread replies depend on the follow model.
	// That projection deliberately consumes only sparse event-type subjects, so
	// wait for those exact tails instead of the broad room tail, which it cannot
	// acknowledge for unrelated room facts.
	for _, eventType := range []string{evtstream.EventThreadFollowed, evtstream.EventThreadUnfollowed} {
		subject := agg.Subject(eventType)
		position, err := c.EventPublisher.LastSubjectPosition(ctx, subject)
		if err != nil {
			return messageAppendAttempt{}, fmt.Errorf("read thread follow tail: %w", err)
		}
		if err := c.roomModel.waitForThreads(ctx, position); err != nil {
			return messageAppendAttempt{}, fmt.Errorf("wait for thread follow projection: %w", err)
		}
	}
	if authorize == nil {
		return attempt, nil
	}
	if err := c.authorizeAtStableInputs(ctx, func() error { return authorize(ctx) }); err != nil {
		return messageAppendAttempt{}, err
	}
	return attempt, nil
}

// prepareMessageRetractionAttempt protects mutable message and room state with
// room aggregate OCC. It evaluates cross-aggregate authorization from a stable
// request-time read. A room conflict causes the complete check to run again.
func (c *ChattoCore) prepareMessageRetractionAttempt(
	ctx context.Context,
	agg evtstream.Aggregate,
	authorize func(context.Context) error,
) (string, uint64, error) {
	roomFilter := agg.AllEventsFilter()
	roomSeq, err := c.EventPublisher.LastSubjectSeq(ctx, roomFilter)
	if err != nil {
		return "", 0, fmt.Errorf("read room OCC tail: %w", err)
	}
	if roomSeq > 0 {
		if err := c.roomModel.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(roomFilter, roomSeq)); err != nil {
			return "", 0, fmt.Errorf("wait for room mutation projections: %w", err)
		}
	}
	if authorize != nil {
		if err := c.authorizeAtStableInputs(ctx, func() error { return authorize(ctx) }); err != nil {
			return "", 0, err
		}
	}
	return roomFilter, roomSeq, nil
}

func (c *ChattoCore) appendBodyAndMessage(
	ctx context.Context,
	agg evtstream.Aggregate,
	bodyEvent, messageEvent *evtv1.Event,
	assetAttachedEvents []*evtv1.Event,
	processingEvents []*evtv1.Event,
	authorize func(context.Context) error,
	prepareMessageAttempt func(context.Context) error,
) (uint64, error) {
	bodySubject := agg.SubjectFor(bodyEvent)
	messageSubject := agg.SubjectFor(messageEvent)
	var lastErr error

	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		guard, err := c.prepareMessageAppendAttempt(ctx, agg, authorize)
		if err != nil {
			return 0, err
		}
		if prepareMessageAttempt != nil {
			if err := prepareMessageAttempt(ctx); err != nil {
				return 0, err
			}
		}
		entries := []evtstream.BatchEntry{
			{
				Subject:       bodySubject,
				Event:         bodyEvent,
				ExpectedSeq:   guard.roomSeq,
				FilterSubject: guard.roomFilter,
				HasOCC:        true,
			},
			{
				Subject: messageSubject,
				Event:   messageEvent,
			},
		}
		baseEntries := len(entries)
		entries, err = c.prepareMessageAssetBatchEntries(ctx, entries, assetAttachedEvents, processingEvents)
		if err != nil {
			return 0, err
		}
		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			messageSeq := seqs[1]
			if err := c.roomModel.waitForTimeline(ctx, events.SubjectPosition(messageSubject, messageSeq)); err != nil {
				return messageSeq, err
			}
			if err := c.waitForMessageBodyAssets(ctx, bodySubject, seqs[0]); err != nil {
				return messageSeq, err
			}
			if err := c.waitForMessageAssetBatch(ctx, entries, seqs, baseEntries); err != nil {
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

func (c *ChattoCore) appendRootMessageWithThread(ctx context.Context, agg evtstream.Aggregate, bodyEvent, messageEvent, threadCreatedEvent, threadFollowedEvent *evtv1.Event, assetAttachedEvents, processingEvents []*evtv1.Event, authorize func(context.Context) error, prepareMessageAttempt func(context.Context) error) (uint64, error) {
	messageSubject := agg.SubjectFor(messageEvent)
	bodySubject := agg.SubjectFor(bodyEvent)
	var lastErr error

	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		guard, err := c.prepareMessageAppendAttempt(ctx, agg, authorize)
		if err != nil {
			return 0, err
		}
		if prepareMessageAttempt != nil {
			if err := prepareMessageAttempt(ctx); err != nil {
				return 0, err
			}
		}

		entries := []evtstream.BatchEntry{
			{
				Subject:       bodySubject,
				Event:         bodyEvent,
				ExpectedSeq:   guard.roomSeq,
				FilterSubject: guard.roomFilter,
				HasOCC:        true,
			},
			{
				Subject: messageSubject,
				Event:   messageEvent,
			},
			{Subject: agg.SubjectFor(threadFollowedEvent), Event: threadFollowedEvent},
			{Subject: agg.SubjectFor(threadCreatedEvent), Event: threadCreatedEvent},
		}
		baseEntries := len(entries)
		entries, err = c.prepareMessageAssetBatchEntries(ctx, entries, assetAttachedEvents, processingEvents)
		if err != nil {
			return 0, err
		}
		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			messageSeq := seqs[1]
			position := events.SubjectPosition(agg.SubjectFor(threadCreatedEvent), seqs[3])
			if err := c.roomModel.waitForTimeline(ctx, position); err != nil {
				return messageSeq, err
			}
			if err := c.roomModel.waitForThreads(ctx, position); err != nil {
				return messageSeq, err
			}
			if err := c.waitForMessageBodyAssets(ctx, bodySubject, seqs[0]); err != nil {
				return messageSeq, err
			}
			if err := c.waitForMessageAssetBatch(ctx, entries, seqs, baseEntries); err != nil {
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

	return 0, fmt.Errorf("append root thread message after %d attempts: %w", maxThreadCreateAppendAttempts, lastErr)
}

func (c *ChattoCore) buildThreadReplyEchoEvents(
	ctx context.Context,
	actorID string,
	originalEvent *evtv1.Event,
	originalPost *evtv1.MessagePostedEvent,
	body *evtv1.MessageBody,
	plaintext string,
) (string, *evtv1.Event, *evtv1.Event, error) {
	return c.buildThreadReplyEchoEventsWithIDs(
		ctx,
		actorID,
		originalEvent,
		originalPost,
		body,
		plaintext,
		NewEventID(),
		NewEventID(),
	)
}

func (c *ChattoCore) buildThreadReplyEchoEventsWithIDs(
	ctx context.Context,
	actorID string,
	originalEvent *evtv1.Event,
	originalPost *evtv1.MessagePostedEvent,
	body *evtv1.MessageBody,
	plaintext string,
	echoID string,
	echoBodyEventID string,
) (string, *evtv1.Event, *evtv1.Event, error) {
	if originalEvent == nil || originalPost == nil || body == nil {
		return "", nil, nil, ErrMessageNotFound
	}
	echoBody := proto.Clone(body).(*evtv1.MessageBody)
	if err := c.encryptMessageBody(ctx, echoBody, originalPost.GetRoomId(), echoID, echoBodyEventID, plaintext); err != nil {
		return "", nil, nil, fmt.Errorf("encrypt thread reply echo: %w", err)
	}
	echoBodyEvent := newEvent(actorID, &evtv1.Event{
		Id:        echoBodyEventID,
		CreatedAt: originalEvent.GetCreatedAt(),
		Event: &evtv1.Event_MessageBody{
			MessageBody: &evtv1.MessageBodyEvent{
				RoomId:  originalPost.GetRoomId(),
				EventId: echoID,
				Body:    echoBody,
			},
		},
	})
	echoEvent := newEvent(actorID, &evtv1.Event{
		Id:        echoID,
		CreatedAt: originalEvent.GetCreatedAt(),
		Event: &evtv1.Event_MessagePosted{
			MessagePosted: &evtv1.MessagePostedEvent{
				RoomId:                    originalPost.GetRoomId(),
				InReplyTo:                 originalPost.GetInReplyTo(),
				MentionedUserIds:          append([]string(nil), originalPost.GetMentionedUserIds()...),
				Mentions:                  cloneMessageMentions(originalPost.GetMentions()),
				EchoOfEventId:             originalEvent.GetId(),
				EchoFromThreadRootEventId: originalPost.GetInThread(),
			},
		},
	})
	return echoID, echoBodyEvent, echoEvent, nil
}

func cloneMessageMentions(mentions []*evtv1.MessageMention) []*evtv1.MessageMention {
	result := make([]*evtv1.MessageMention, 0, len(mentions))
	for _, mention := range mentions {
		if mention != nil {
			result = append(result, proto.Clone(mention).(*evtv1.MessageMention))
		}
	}
	return result
}

func (c *ChattoCore) appendThreadReplyEcho(
	ctx context.Context,
	actorID string,
	kind RoomKind,
	agg evtstream.Aggregate,
	originalEvent *evtv1.Event,
	originalPost *evtv1.MessagePostedEvent,
	body *evtv1.MessageBody,
	plaintext string,
	attemptPrepared func(context.Context) error,
) (string, bool, error) {
	if originalEvent == nil || originalPost == nil || body == nil {
		return "", false, ErrMessageNotFound
	}
	originalID := originalEvent.GetId()
	roomID := originalPost.GetRoomId()
	messageSubject := agg.Subject(evtstream.EventMessagePosted)
	bodySubject := agg.Subject(evtstream.EventMessageBody)
	roomFilter := agg.AllEventsFilter()
	var lastErr error

	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		roomSeq, err := c.EventPublisher.LastSubjectSeq(ctx, roomFilter)
		if err != nil {
			return "", false, fmt.Errorf("read echo room OCC tail: %w", err)
		}
		if roomSeq > 0 {
			if err := c.roomModel.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(roomFilter, roomSeq)); err != nil {
				return "", false, fmt.Errorf("wait for echo room policy: %w", err)
			}
		}
		if echoID, ok := c.roomModel.channelEchoEventID(originalID); ok {
			return echoID, false, nil
		}
		if kind == KindChannel {
			room, err := c.GetRoom(ctx, kind, roomID)
			if err != nil {
				return "", false, err
			}
			if EffectiveRoomThreadingMode(room) == evtv1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED {
				return "", false, fmt.Errorf("%w: channel echoes are disabled in this room", ErrRoomThreadingPolicy)
			}
		}

		echoID, echoBodyEvent, echoEvent, err := c.buildThreadReplyEchoEvents(ctx, actorID, originalEvent, originalPost, body, plaintext)
		if err != nil {
			return "", false, err
		}

		entries := []evtstream.BatchEntry{
			{
				Subject:       bodySubject,
				Event:         echoBodyEvent,
				ExpectedSeq:   roomSeq,
				FilterSubject: roomFilter,
				HasOCC:        true,
			},
			{
				Subject:       messageSubject,
				Event:         echoEvent,
				ExpectedSeq:   roomSeq,
				FilterSubject: roomFilter,
				HasOCC:        true,
			},
		}
		if attemptPrepared != nil {
			if err := attemptPrepared(ctx); err != nil {
				return "", false, err
			}
		}
		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			echoSeq := seqs[len(seqs)-1]
			if err := c.roomModel.waitForTimeline(ctx, events.SubjectPosition(messageSubject, echoSeq)); err != nil {
				return echoID, true, err
			}
			if err := c.waitForMessageBodyAssets(ctx, bodySubject, seqs[0]); err != nil {
				return echoID, true, err
			}
			c.logger.Debug("Thread reply echo posted",
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

func (c *ChattoCore) hideChannelEchoForReply(ctx context.Context, actorID string, kind RoomKind, agg evtstream.Aggregate, roomID, originalEventID string) error {
	retractSubject := agg.Subject(evtstream.EventMessageRetracted)
	var lastErr error

	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		expectedSeq, err := c.EventPublisher.LastSubjectSeq(ctx, retractSubject)
		if err != nil {
			return fmt.Errorf("read echo retract OCC tail: %w", err)
		}
		if expectedSeq > 0 {
			if err := c.roomModel.waitForTimeline(ctx, events.SubjectPosition(retractSubject, expectedSeq)); err != nil {
				return err
			}
		}
		echoID, ok := c.roomModel.channelEchoEventID(originalEventID)
		if !ok {
			return nil
		}

		event := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_MessageRetracted{
				MessageRetracted: &evtv1.MessageRetractedEvent{
					RoomId:  roomID,
					EventId: echoID,
				},
			},
		})
		seq, err := c.EventPublisher.AppendAt(ctx, retractSubject, event, expectedSeq)
		if err == nil {
			if err := c.roomModel.waitForTimeline(ctx, events.SubjectPosition(retractSubject, seq)); err != nil {
				return err
			}
			c.logger.Debug("Message echo hidden", "kind", kind, "room_id", roomID, "event_id", echoID, "actor_id", actorID)
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

func (c *ChattoCore) appendMessageWithOptionalThreadCreated(
	ctx context.Context,
	agg evtstream.Aggregate,
	bodyEvent, messageEvent, threadCreatedEvent *evtv1.Event,
	threadRootEventID string,
	assetAttachedEvents []*evtv1.Event,
	processingEvents []*evtv1.Event,
	authorize func(context.Context) error,
	prepareMessageAttempt func(context.Context) error,
) (uint64, error) {
	if threadCreatedEvent == nil || threadRootEventID == "" || c.roomModel.threadExists(threadRootEventID) {
		return c.appendBodyAndMessage(ctx, agg, bodyEvent, messageEvent, assetAttachedEvents, processingEvents, authorize, prepareMessageAttempt)
	}
	if exists, err := c.threadCreatedExistsInStream(ctx, agg, threadRootEventID); err != nil {
		return 0, fmt.Errorf("check existing thread creation: %w", err)
	} else if exists {
		return c.appendBodyAndMessage(ctx, agg, bodyEvent, messageEvent, assetAttachedEvents, processingEvents, authorize, prepareMessageAttempt)
	}

	threadCreatedSubject := agg.Subject(evtstream.EventThreadCreated)
	bodySubject := agg.SubjectFor(bodyEvent)
	messageSubject := agg.SubjectFor(messageEvent)
	var lastErr error

	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		guard, err := c.prepareMessageAppendAttempt(ctx, agg, authorize)
		if err != nil {
			return 0, err
		}
		if prepareMessageAttempt != nil {
			if err := prepareMessageAttempt(ctx); err != nil {
				return 0, err
			}
		}
		entries := []evtstream.BatchEntry{
			{
				Subject:       threadCreatedSubject,
				Event:         threadCreatedEvent,
				ExpectedSeq:   guard.roomSeq,
				FilterSubject: guard.roomFilter,
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
		}
		baseEntries := len(entries)
		entries, err = c.prepareMessageAssetBatchEntries(ctx, entries, assetAttachedEvents, processingEvents)
		if err != nil {
			return 0, err
		}
		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			messageSeq := seqs[2]
			if err := c.roomModel.waitForTimeline(ctx, events.SubjectPosition(messageSubject, messageSeq)); err != nil {
				return messageSeq, err
			}
			if err := c.waitForMessageBodyAssets(ctx, bodySubject, seqs[1]); err != nil {
				return messageSeq, err
			}
			if err := c.waitForMessageAssetBatch(ctx, entries, seqs, baseEntries); err != nil {
				return messageSeq, err
			}
			return messageSeq, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return 0, err
		}
		lastErr = err

		currentSeq, seqErr := c.EventPublisher.LastSubjectSeq(ctx, guard.roomFilter)
		if seqErr != nil {
			return 0, fmt.Errorf("read room OCC tail after conflict: %w", seqErr)
		}
		if currentSeq > 0 {
			if err := c.roomModel.waitForTimeline(ctx, events.SubjectPosition(guard.roomFilter, currentSeq)); err != nil {
				return 0, err
			}
		}
		if c.roomModel.threadExists(threadRootEventID) {
			return c.appendBodyAndMessage(ctx, agg, bodyEvent, messageEvent, assetAttachedEvents, processingEvents, authorize, prepareMessageAttempt)
		}
		if exists, err := c.threadCreatedExistsInStream(ctx, agg, threadRootEventID); err != nil {
			return 0, fmt.Errorf("check existing thread creation after conflict: %w", err)
		} else if exists {
			return c.appendBodyAndMessage(ctx, agg, bodyEvent, messageEvent, assetAttachedEvents, processingEvents, authorize, prepareMessageAttempt)
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
func (c *ChattoCore) PostMessage(ctx context.Context, kind RoomKind, room_id, user_id, body string, assetIDs []string, inThread, inReplyTo string, linkPreview *evtv1.LinkPreview, alsoSendToChannel bool, opts ...PostMessageOption) (*evtv1.Event, error) {
	options := collectPostMessageOptions(opts)
	if options.createThread && kind == KindDM {
		return nil, ErrDMThreadsUnsupported
	}

	if err := validateMessageAttachmentAssetIDs(assetIDs); err != nil {
		return nil, err
	}

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

	// Resolve referenced assets from the projection. Each must be a live,
	// unattached room asset uploaded by this caller. The same invariant is checked
	// again under asset-aggregate OCC in the atomic message batch.
	resolvedAssets := make([]*evtv1.Attachment, 0, len(assetIDs))
	resolvedAssetIDs := make([]string, 0, len(assetIDs))
	resolvedAssetIDSet := make(map[string]struct{}, len(assetIDs))
	for _, id := range assetIDs {
		if id == "" {
			continue
		}
		if _, seen := resolvedAssetIDSet[id]; seen {
			continue
		}
		if err := c.assetModel.validateAssetAttachment(id, user_id, room_id, "", time.Now()); err != nil {
			return nil, err
		}
		declared, _ := c.assetModel.AssetCreation(id)
		att := attachmentFromAsset(declared.GetAsset())
		if att == nil {
			continue
		}
		att.RoomId = room_id
		resolvedAssets = append(resolvedAssets, att)
		resolvedAssetIDs = append(resolvedAssetIDs, id)
		resolvedAssetIDSet[id] = struct{}{}
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

	// If replying to a message inside a thread, or its visible channel echo,
	// inherit the canonical thread root.
	// This keeps the data invariant intact even when callers (bots, older clients,
	// extensions) only set inReplyTo. inReplyTo is attribution-only, so a lookup
	// failure here is not fatal — fall through and let the message post as a root.
	var replyTarget *evtv1.Event
	if inReplyTo != "" {
		target, err := c.GetRoomEventByEventID(ctx, kind, room_id, inReplyTo)
		if err == nil && target != nil {
			replyTarget = target
			if msg := target.GetMessagePosted(); msg != nil {
				targetThreadRootID := msg.GetInThread()
				if targetThreadRootID == "" && msg.GetEchoOfEventId() != "" {
					targetThreadRootID = msg.GetEchoFromThreadRootEventId()
				}
				if inThread == "" {
					inThread = targetThreadRootID
				}
			}
		}
	}
	if options.createThread && inThread != "" {
		return nil, invalidArgument("thread creation cannot be combined with a thread reply")
	}
	if kind == KindDM && inThread != "" {
		return nil, ErrDMThreadsUnsupported
	}
	if kind == KindChannel {
		switch EffectiveRoomThreadingMode(room) {
		case evtv1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED:
			replyTargetsThread := false
			if replyTarget != nil {
				if targetPost := replyTarget.GetMessagePosted(); targetPost != nil {
					replyTargetsThread = targetPost.GetInThread() != "" || targetPost.GetEchoOfEventId() != ""
				}
			}
			if options.createThread || inThread != "" || replyTargetsThread {
				return nil, fmt.Errorf("%w: threads are disabled in this room", ErrRoomThreadingPolicy)
			}
		case evtv1.RoomThreadingMode_ROOM_THREADING_MODE_REQUIRED:
			if inReplyTo == "" && inThread == "" && !options.createThread {
				return nil, fmt.Errorf("%w: root messages must establish a thread in this room", ErrRoomThreadingPolicy)
			}
			if replyTarget != nil {
				if targetPost := replyTarget.GetMessagePosted(); targetPost != nil {
					targetThreadRootID := targetPost.GetInThread()
					if targetThreadRootID == "" && targetPost.GetEchoOfEventId() != "" {
						targetThreadRootID = targetPost.GetEchoFromThreadRootEventId()
					}
					if targetThreadRootID == "" {
						targetThreadRootID = replyTarget.GetId()
					}
					if inThread != targetThreadRootID {
						return nil, fmt.Errorf("%w: replies to root messages must be posted in that root's thread", ErrRoomThreadingPolicy)
					}
				}
			}
		}
	}
	var commitAuthorize func(context.Context) error
	if options.commitAuthorize != nil {
		commitAuthorize = func(ctx context.Context) error {
			return options.commitAuthorize(ctx, inThread)
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
		if rootMsg.InThread != "" || rootMsg.EchoOfEventId != "" {
			return nil, invalidArgument("thread root must be a root message, not a thread reply")
		}
	}

	now := time.Now()

	// Mention tokens are stable request input, but their recipient expansion is
	// mutable room state and is therefore resolved inside every OCC attempt.
	var mentionUsernames []string
	if hasBody {
		mentionUsernames = ExtractMentionUsernames(body)
	}

	eventID := NewEventID()
	bodyEventID := NewEventID()
	messageBody := &evtv1.MessageBody{
		CreatedAt:   timestamppb.New(now),
		AssetIds:    resolvedAssetIDs,
		AuthorId:    user_id,
		LinkPreview: linkPreview,
	}
	if err := c.encryptMessageBody(ctx, messageBody, room_id, eventID, bodyEventID, body); err != nil {
		return nil, err
	}
	bodyEventEvent := newEvent(user_id, &evtv1.Event{
		Id:        bodyEventID,
		CreatedAt: timestamppb.New(now),
		Event: &evtv1.Event_MessageBody{
			MessageBody: &evtv1.MessageBodyEvent{
				RoomId:  room_id,
				EventId: eventID,
				Body:    messageBody,
			},
		},
	})

	event := newEvent(user_id, &evtv1.Event{
		Id:        eventID,
		CreatedAt: timestamppb.New(now),
		Event: &evtv1.Event_MessagePosted{
			MessagePosted: &evtv1.MessagePostedEvent{
				RoomId:    room_id,
				InReplyTo: inReplyTo,
				InThread:  inThread,
			},
		},
	})
	var directMentionFollowers []string
	prepareMessageAttempt := func(attemptCtx context.Context) error {
		if len(mentionUsernames) > 0 {
			resolved, err := c.ResolveRoomMentionKinds(attemptCtx, kind, room_id, mentionUsernames)
			if err != nil {
				return fmt.Errorf("resolve notification mention recipients: %w", err)
			}
			event.GetMessagePosted().MentionedUserIds = append([]string(nil), resolved.RecipientIDs...)
			event.GetMessagePosted().Mentions = resolvedMessageMentions(resolved)
		} else {
			event.GetMessagePosted().MentionedUserIds = nil
			event.GetMessagePosted().Mentions = nil
		}
		directMentionFollowers = nil
		directMentionUserIDs := directMentionRecipients(event.GetMessagePosted().GetMentions())
		if kind == KindChannel && len(directMentionUserIDs) > 0 {
			if err := c.waitForCurrentNotificationPolicy(attemptCtx); err != nil {
				return err
			}
			directMentionSignal := &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &notificationv1.DirectMentionReceived{}}}
			for _, userID := range directMentionUserIDs {
				if notificationModeProducesAttention(c.GetEffectiveNotificationModeForSignal(userID, room_id, directMentionSignal)) {
					directMentionFollowers = append(directMentionFollowers, userID)
				}
			}
		}
		if options.messageAttemptPrepared != nil {
			if err := options.messageAttemptPrepared(attemptCtx); err != nil {
				return err
			}
		}
		return nil
	}
	assetAttachedEvents := make([]*evtv1.Event, 0, len(resolvedAssetIDs))
	for _, assetID := range resolvedAssetIDs {
		assetAttachedEvents = append(assetAttachedEvents, newEvent(user_id, &evtv1.Event{
			Event: &evtv1.Event_AssetAttached{
				AssetAttached: &evtv1.AssetAttachedEvent{
					AssetId:        assetID,
					RoomId:         room_id,
					MessageEventId: eventID,
					UserId:         user_id,
				},
			},
		}))
	}
	var threadCreatedEvent *evtv1.Event
	if inThread != "" && !c.roomModel.threadExists(inThread) {
		threadCreatedEvent = newEvent(user_id, &evtv1.Event{
			Id:        NewEventID(),
			CreatedAt: timestamppb.New(now),
			Event: &evtv1.Event_ThreadCreated{
				ThreadCreated: &evtv1.ThreadCreatedEvent{
					RoomId:            room_id,
					ThreadRootEventId: inThread,
				},
			},
		})
	}
	var rootThreadFollowedEvent *evtv1.Event
	if options.createThread {
		threadCreatedEvent = newEvent(user_id, &evtv1.Event{
			Id:        NewEventID(),
			CreatedAt: timestamppb.New(now),
			Event: &evtv1.Event_ThreadCreated{
				ThreadCreated: &evtv1.ThreadCreatedEvent{
					RoomId:            room_id,
					ThreadRootEventId: eventID,
				},
			},
		})
		rootThreadFollowedEvent = newEvent(user_id, &evtv1.Event{
			Id:        NewEventID(),
			CreatedAt: timestamppb.New(now),
			Event: &evtv1.Event_ThreadFollowed{
				ThreadFollowed: &evtv1.ThreadFollowedEvent{
					RoomId:            room_id,
					ThreadRootEventId: eventID,
					UserId:            user_id,
					Source:            evtv1.ThreadFollowSource_THREAD_FOLLOW_SOURCE_ROOT_AUTHOR_CREATED,
				},
			},
		})
	}

	// Publish to EVT. MessagePosted is append-only per #597's design, so
	// retrying the same payload after an OCC conflict is safe.
	// AppendEventuallyAndWait blocks until the RoomTimelineProjection
	// has caught up, giving read-your-writes for subsequent reads from
	// this request.
	agg := evtstream.RoomAggregate(room_id)
	processingEvents := make([]*evtv1.Event, 0, len(resolvedAssets))
	if c.VideoUploadsEnabled {
		for _, attachment := range resolvedAssets {
			declared, _ := c.assetModel.AssetCreation(attachment.GetId())
			if !options.shouldScheduleVideoProcessingForID(attachment.GetId()) && (declared == nil || !declared.GetNeedsVideoProcessing()) {
				continue
			}
			processingEvents = append(processingEvents, newEvent(user_id, &evtv1.Event{
				Event: &evtv1.Event_AssetProcessingStarted{
					AssetProcessingStarted: &evtv1.AssetProcessingStartedEvent{
						AssetId:        attachment.GetId(),
						MessageEventId: event.Id,
					},
				},
			}))
		}
	}
	var sequenceID uint64
	if options.createThread {
		sequenceID, err = c.appendRootMessageWithThread(ctx, agg, bodyEventEvent, event, threadCreatedEvent, rootThreadFollowedEvent, assetAttachedEvents, processingEvents, commitAuthorize, prepareMessageAttempt)
	} else {
		sequenceID, err = c.appendMessageWithOptionalThreadCreated(ctx, agg, bodyEventEvent, event, threadCreatedEvent, inThread, assetAttachedEvents, processingEvents, commitAuthorize, prepareMessageAttempt)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to publish message event: %w", err)
	}

	// Also wait for ThreadProjection if this is a thread reply, so a
	// subsequent thread-pane fetch from the same request sees it.
	if inThread != "" {
		if err := c.roomModel.waitForThreads(ctx, events.SubjectPosition(agg.SubjectFor(event), sequenceID)); err != nil {
			c.logger.Debug("ThreadsProjector did not catch up", "error", err)
		}
	}
	if options.createThread {
		c.publishThreadFollowChangedEvent(ctx, user_id, kind, room_id, event.Id, true)
	}

	c.logger.Debug("Message posted", "kind", kind, "room_id", room_id, "event_id", event.Id, "sequence_id", sequenceID, "user_id", user_id)

	// Mark the room as read for the poster. For root posts, the just-
	// published event is the new last root. For thread replies, we look up
	// the room's current last root so the Message Read Cursor tracks a real
	// root event ID for the New messages separator.
	var posterReadEventID string
	if inThread == "" {
		posterReadEventID = event.Id
	} else if lastRootID, _, exists, err := c.GetRoomLastEvent(ctx, kind, room_id); err == nil && exists {
		posterReadEventID = lastRootID
	}
	if posterReadEventID != "" {
		if _, err := c.AdvanceLastReadEventID(ctx, kind, user_id, room_id, posterReadEventID); err != nil {
			c.logger.Warn("Failed to set last read event for poster", "error", err)
		}
		if _, err := c.notificationOccurrences.MarkCoveredRead(ctx, user_id, room_id, "", posterReadEventID); err != nil {
			c.logger.Warn("Failed to cover notifications for poster", "error", err)
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
		if err := c.FollowThreadWithSource(ctx, kind, user_id, room_id, inThread, evtv1.ThreadFollowSource_THREAD_FOLLOW_SOURCE_POSTED_REPLY); err != nil {
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
				if _, err := c.FollowThreadIfNeverSet(ctx, kind, rootAuthorID, room_id, inThread, evtv1.ThreadFollowSource_THREAD_FOLLOW_SOURCE_ROOT_AUTHOR); err != nil {
					c.logger.Warn("Failed to auto-follow thread for root author", "error", err, "thread_root_event_id", inThread)
				}
			}
		}
	}

	// A delivered direct mention follows its thread unless the recipient has
	// explicitly opted out. A root message is also the stable thread root for
	// future replies. This subscription side effect remains best-effort and is
	// distinct from occurrence materialization.
	directMentionThreadRootID := inThread
	if directMentionThreadRootID == "" && kind == KindChannel {
		directMentionThreadRootID = event.Id
	}
	if directMentionThreadRootID != "" {
		for _, mentionedUserID := range directMentionFollowers {
			if _, err := c.FollowThreadIfNeverSet(ctx, kind, mentionedUserID, room_id, directMentionThreadRootID, evtv1.ThreadFollowSource_THREAD_FOLLOW_SOURCE_DIRECT_MENTION); err != nil {
				c.logger.Warn("Failed to auto-follow thread for directly mentioned user",
					"mentioned_user_id", mentionedUserID,
					"room_id", room_id,
					"thread_root_event_id", directMentionThreadRootID,
					"error", err)
			}
		}
	}
	// Recipient attention is an asynchronous durable effect of the committed
	// source message. The materializer owns completion and retry; posting must
	// not make message delivery latency grow with the room's member count.

	// Publish echo event to the message subject if "also send to channel" was requested.
	// The echo references the original event_id, so resolvers can fold
	// it back to the underlying body. The body is encrypted again for the
	// echo event ID because v2 encryption authenticates the event context.
	if inThread != "" && alsoSendToChannel {
		echoID, created, err := c.appendThreadReplyEcho(ctx, user_id, kind, agg, event, event.GetMessagePosted(), messageBody, body, options.echoAttemptPrepared)
		if err != nil {
			c.logger.Warn("Failed to publish thread reply echo", "error", err, "thread_reply_event_id", event.Id)
		} else if created {
			c.logger.Debug("Created channel echo for thread reply", "echo_event_id", echoID, "thread_reply_event_id", event.Id)
		}
	}

	// The durable message fact can reach realtime subscribers before the
	// poster's read boundary and Slow Mode state above are current. Publish one
	// transient, user-scoped reconciliation only after those post-commit updates
	// run. Recipient Badge decisions publish their own invalidations.
	c.NotifyNotificationUnreadChanged(ctx, user_id, user_id, room_id, "")

	return event, nil
}

func validateMessageAttachmentAssetIDs(assetIDs []string) error {
	if len(assetIDs) > MaxMessageAttachmentAssetIDs {
		return invalidArgument(fmt.Sprintf("message attachment asset IDs exceed maximum count of %d", MaxMessageAttachmentAssetIDs))
	}
	for _, assetID := range assetIDs {
		if assetID == "" {
			return invalidArgument("message attachment asset ID must not be empty")
		}
		if len(assetID) > MaxMessageAttachmentAssetIDLength {
			return invalidArgument(fmt.Sprintf("message attachment asset ID exceeds maximum length of %d bytes", MaxMessageAttachmentAssetIDLength))
		}
	}
	return nil
}

type messageMutationAuthorization struct {
	authorOnly                  bool
	enforceEditWindow           bool
	requireEchoPermissions      bool
	channelEchoCreationTargetID string
	requireMessageRead          bool
}

// authorizeMessageMutation resolves every mutable input to a user-facing
// message mutation after the caller has caught the serving projections up to
// its captured OCC boundaries.
func (c *ChattoCore) authorizeMessageMutation(
	ctx context.Context,
	actorID string,
	kind RoomKind,
	roomID, eventID string,
	policy messageMutationAuthorization,
	now time.Time,
) error {
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return err
	}
	if room.GetArchived() {
		return ErrRoomArchived
	}
	member, err := c.RoomMembershipExists(ctx, kind, actorID, roomID)
	if err != nil {
		return err
	}
	if !member {
		return ErrNotRoomMember
	}
	if policy.requireMessageRead {
		canRead, err := c.CanReadMessage(ctx, actorID, kind, roomID, eventID)
		if err != nil {
			return err
		}
		if !canRead {
			return ErrPermissionDenied
		}
	}

	entry, err := c.validateMessageMutationIdentity(ctx, actorID, kind, roomID, eventID, policy, now)
	if err != nil {
		return err
	}
	if entry.Event.GetActorId() != actorID {
		canManage, err := c.CanManageOthersMessage(ctx, actorID, kind, roomID)
		if err != nil {
			return err
		}
		if !canManage {
			return ErrPermissionDenied
		}
	}
	if policy.channelEchoCreationTargetID != "" && kind == KindChannel &&
		EffectiveRoomThreadingMode(room) == evtv1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED {
		if _, exists := c.roomModel.channelEchoEventID(policy.channelEchoCreationTargetID); !exists {
			return fmt.Errorf("%w: channel echoes are disabled in this room", ErrRoomThreadingPolicy)
		}
	}
	if policy.requireEchoPermissions {
		canEcho, err := c.CanEchoMessage(ctx, actorID, kind, roomID)
		if err != nil {
			return err
		}
		canPost, err := c.CanPostMessage(ctx, actorID, kind, roomID)
		if err != nil {
			return err
		}
		if !canEcho || !canPost {
			return ErrPermissionDenied
		}
	}
	return nil
}

// validateMessageMutationIdentity checks current message identity and the
// author-only rules for one mutation attempt. Effective message.manage lets an
// author continue after the edit window closes, but it does not replace an
// authorOnly requirement.
func (c *ChattoCore) validateMessageMutationIdentity(
	ctx context.Context,
	actorID string,
	kind RoomKind,
	roomID, eventID string,
	policy messageMutationAuthorization,
	now time.Time,
) (*TimelineEntry, error) {
	entry, ok := c.roomModel.timelineEntry(eventID)
	if !ok || entry.Event == nil || entry.Event.GetMessagePosted() == nil || roomIDOfEvent(entry.Event) != roomID {
		return nil, ErrMessageNotFound
	}
	current, retracted, _ := c.roomModel.latestBody(eventID)
	if retracted || current == nil {
		return nil, ErrMessageNotFound
	}
	if entry.Event.GetActorId() == actorID {
		if policy.enforceEditWindow && now.After(entry.Event.GetCreatedAt().AsTime().Add(MessageEditWindow)) {
			canManage, err := c.CanManageOthersMessage(ctx, actorID, kind, roomID)
			if err != nil {
				return nil, err
			}
			if !canManage {
				return nil, ErrEditWindowExpired
			}
		}
		return entry, nil
	}
	if policy.authorOnly {
		return nil, ErrNotMessageAuthor
	}
	return entry, nil
}

// DeleteMessage retracts a message. For ordinary messages and original thread
// replies, the retraction removes visible content and attachments for GDPR
// compliance while preserving the event in the stream for audit. For echoes,
// the same durable MessageRetractedEvent hides only the echo artifact from the
// room timeline; the original thread reply remains readable.
// Authorization: Caller must verify the actor is the message author OR
// CanManageOthersMessage before calling.
func (c *ChattoCore) DeleteMessage(ctx context.Context, actorID string, kind RoomKind, roomID, eventID string, opts ...DeleteMessageOption) error {
	options := collectDeleteMessageOptions(opts)
	if eventID == "" {
		return ErrMessageNotFound
	}

	// Snapshot the projection state for attachment cleanup before
	// emitting the retract event. After retract, LatestBody returns
	// nil (the message is tombstoned), so we need a copy first.
	originalEntry, ok := c.roomModel.timelineEntry(eventID)
	if !ok {
		c.logger.Debug("Delete on unknown message — no-op", "event_id", eventID)
		return nil
	}
	isEcho := c.roomModel.isEcho(eventID)
	if isEcho && c.roomModel.isHiddenEcho(eventID) {
		return nil
	}
	body, retracted, _ := c.roomModel.latestBody(eventID)
	if retracted {
		// Already tombstoned.
		return nil
	}

	// Emit MessageRetractedEvent on evt.room.{R}.message_retracted.
	// Pure append for the v1 model — last-writer-wins on the per-room
	// retract subject. The projection ignores duplicates by event_id,
	// so retrying after a network glitch is safe.
	agg := evtstream.RoomAggregate(roomID)
	var authorize func(context.Context) error
	if options.commitAuthorize != nil {
		authorize = options.commitAuthorize
	}
	if err := c.publishMessageRetract(ctx, actorID, kind, agg, roomID, eventID, authorize); err != nil {
		return err
	}
	c.secureDeleteAllMessageBodyEvents(ctx, eventID)
	if isEcho {
		c.logger.Debug("Message echo hidden", "kind", kind, "room_id", roomID, "event_id", eventID, "actor_id", actorID, "envelope_seq", originalEntry.StreamSeq)
		return nil
	}
	for _, linkedID := range c.roomModel.linkedEventIDs(eventID) {
		c.secureDeleteAllMessageBodyEvents(ctx, linkedID)
	}

	// Attachments are referenced by the (now-tombstoned) message but
	// the binary blobs in the asset store don't get cleaned up by the
	// event log. Same posture as the legacy DeleteMessage path —
	// best-effort, log warnings, keep going.
	if body != nil {
		for _, att := range c.mediaModel.MessageBodyAttachments(body) {
			owned, err := c.assetModel.MessageOwnsAsset(ctx, roomID, eventID, att.GetId())
			if err != nil {
				c.logger.Warn("Failed to verify message asset ownership before deletion",
					"attachment_id", att.GetId(), "event_id", eventID, "error", err)
				continue
			}
			if !owned {
				continue
			}
			c.assetModel.DeleteVideoDerivativesForAttachment(ctx, actorID, att.GetId())
			deleted, err := c.assetModel.RecordMessageAssetDeleted(ctx, actorID, roomID, eventID, att.GetId())
			if err != nil {
				c.logger.Warn("Failed to publish asset deletion event",
					"attachment_id", att.GetId(),
					"event_id", eventID,
					"error", err)
				continue
			}
			if !deleted {
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

	c.logger.Debug("Message retracted", "kind", kind, "room_id", roomID, "event_id", eventID, "actor_id", actorID, "envelope_seq", originalEntry.StreamSeq)
	return nil
}

// EditMessage edits a message body. Updates the body content and sets updated_at.
// Publishes a MessageEditedEvent to notify connected clients in real-time.
// Business rule: Authors can edit their own messages within MessageEditWindow
// (3 hours). Effective message.manage bypasses the window and also permits
// editing other authors' messages.
//
// Authorization: Caller must verify the actor is the author OR
// CanManageOthersMessage before calling.
func (c *ChattoCore) EditMessage(ctx context.Context, actorID string, kind RoomKind, roomID, eventID, newBody string, opts ...EditMessageOption) error {
	options := collectEditMessageOptions(opts)
	now := time.Now
	if options.now != nil {
		now = options.now
	}
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

	if eventID == "" {
		return ErrMessageNotFound
	}
	originalEntry, ok := c.roomModel.timelineEntry(eventID)
	if !ok {
		return ErrMessageNotFound
	}
	origPost := originalEntry.Event.GetMessagePosted()
	if origPost == nil {
		return ErrMessageNotFound
	}

	// Check the author window before preparing echo reconciliation. The same
	// rule runs inside every authorized OCC attempt below.
	if _, err := c.validateMessageMutationIdentity(ctx, actorID, kind, roomID, eventID, messageMutationAuthorization{enforceEditWindow: true}, now()); err != nil {
		return err
	}
	channelEchoCreationTargetID := ""
	channelEchoRetractionTargetID := ""
	channelEchoExistedBefore := false
	if options.channelEcho != nil {
		echoTargetEvent := originalEntry.Event
		echoTargetPost := origPost
		if echoOf := origPost.GetEchoOfEventId(); echoOf != "" {
			origEchoEntry, ok := c.roomModel.timelineEntry(echoOf)
			if !ok || origEchoEntry.Event == nil {
				return ErrMessageNotFound
			}
			echoTargetEvent = origEchoEntry.Event
			echoTargetPost = echoTargetEvent.GetMessagePosted()
		}
		if echoTargetPost == nil || echoTargetPost.GetEchoOfEventId() != "" || echoTargetPost.GetInThread() == "" {
			return invalidArgument("channel echo state can only be changed for thread replies")
		}
		if roomIDOfEvent(echoTargetEvent) != roomID {
			return ErrMessageNotFound
		}
		if echoTargetEvent.GetActorId() != actorID {
			return ErrNotMessageAuthor
		}
		if _, err := c.validateMessageMutationIdentity(ctx, actorID, kind, roomID, echoTargetEvent.GetId(), messageMutationAuthorization{authorOnly: true, enforceEditWindow: true}, now()); err != nil {
			return err
		}
		_, channelEchoExistedBefore = c.roomModel.channelEchoEventID(echoTargetEvent.GetId())
		if *options.channelEcho {
			if kind == KindChannel && EffectiveRoomThreadingMode(room) == evtv1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED && !channelEchoExistedBefore {
				return fmt.Errorf("%w: channel echoes are disabled in this room", ErrRoomThreadingPolicy)
			}
			channelEchoCreationTargetID = echoTargetEvent.GetId()
		} else {
			channelEchoRetractionTargetID = echoTargetEvent.GetId()
		}
	}

	agg := evtstream.RoomAggregate(roomID)
	policy := messageMutationAuthorization{
		authorOnly:                  options.channelEcho != nil,
		enforceEditWindow:           true,
		requireEchoPermissions:      options.channelEcho != nil && *options.channelEcho,
		channelEchoCreationTargetID: channelEchoCreationTargetID,
		requireMessageRead:          true,
	}
	var authorize func(context.Context) error
	var validateCommit func() error
	if options.authorize {
		authorize = func(attemptCtx context.Context) error {
			if err := c.authorizeMessageMutation(attemptCtx, actorID, kind, roomID, eventID, policy, now()); err != nil {
				return err
			}
			if options.commitAuthorize != nil {
				return options.commitAuthorize(attemptCtx)
			}
			return nil
		}
		validateCommit = func() error {
			_, err := c.validateMessageMutationIdentity(ctx, actorID, kind, roomID, eventID, policy, now())
			return err
		}
	}
	createdChannelEchoID := ""
	committedPlaintext, err := c.publishMessageEditWithAuthorization(ctx, actorID, agg, roomID, eventID, authorize, validateCommit, channelEchoCreationTargetID, channelEchoRetractionTargetID, &createdChannelEchoID, func(ctx context.Context, updated *evtv1.MessageBody) (string, error) {
		if updated.GetAuthorId() == "" {
			return "", fmt.Errorf("cannot edit: message body author is empty")
		}
		if options.preserveBody {
			plaintext, err := c.decryptMessageBody(ctx, eventID, roomID, updated)
			if err != nil {
				return "", fmt.Errorf("decrypt message body for edit: %w", err)
			}
			return string(plaintext), nil
		}
		return newBody, nil
	})
	if err != nil {
		return err
	}
	c.secureDeleteObsoleteMessageBodyEvents(ctx, eventID)
	// Fan out to echoes (and to the original if this IS an echo) so
	// the legacy "edit one, both update" semantic is preserved.
	for _, linkedID := range c.roomModel.linkedEventIDs(eventID) {
		if linkedID == createdChannelEchoID {
			// The new echo body already landed in the parent edit's atomic
			// batch; another edit would create a duplicate realtime upsert.
			continue
		}
		if _, err := c.publishMessageEdit(ctx, actorID, agg, roomID, linkedID, func(ctx context.Context, linked *evtv1.MessageBody) (string, error) {
			if options.preserveBody {
				plaintext, err := c.decryptMessageBody(ctx, linkedID, roomID, linked)
				if err != nil {
					return "", fmt.Errorf("decrypt linked message body for edit: %w", err)
				}
				return string(plaintext), nil
			}
			return committedPlaintext, nil
		}); err != nil {
			c.logger.Warn("Failed to propagate edit to linked message",
				"source_event_id", eventID, "linked_event_id", linkedID, "error", err)
			continue
		}
		c.secureDeleteObsoleteMessageBodyEvents(ctx, linkedID)
	}

	c.logger.Debug("Message edited", "kind", kind, "room_id", roomID, "event_id", eventID, "actor_id", actorID)
	if options.channelEcho != nil && *options.channelEcho && !channelEchoExistedBefore && createdChannelEchoID != "" {
		c.logger.Debug("Created channel echo while editing thread reply", "echo_event_id", createdChannelEchoID, "thread_reply_event_id", eventID)
	}
	return nil
}

// publishMessageRetract emits a MessageRetractedEvent on EVT. StreamMyEvents
// receives the canonical live.evt.> republish directly. Factored out so
// DeleteMessage can fan to linked messages.
func (c *ChattoCore) publishMessageRetract(
	ctx context.Context,
	actorID string,
	kind RoomKind,
	agg evtstream.Aggregate,
	roomID, eventID string,
	authorize func(context.Context) error,
) error {
	event := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_MessageRetracted{
			MessageRetracted: &evtv1.MessageRetractedEvent{
				RoomId:  roomID,
				EventId: eventID,
			},
		},
	})
	retractSubject := agg.SubjectFor(event)
	var lastErr error
	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		roomFilter, roomSeq, err := c.prepareMessageRetractionAttempt(ctx, agg, authorize)
		if err != nil {
			return err
		}
		entry, ok := c.roomModel.timelineEntry(eventID)
		if !ok || entry.Event == nil || entry.Event.GetMessagePosted() == nil || roomIDOfEvent(entry.Event) != roomID {
			return ErrMessageNotFound
		}
		_, retracted, _ := c.roomModel.latestBody(eventID)
		if retracted {
			return nil
		}

		entries := []evtstream.BatchEntry{{
			Subject:       retractSubject,
			Event:         event,
			FilterSubject: roomFilter,
			ExpectedSeq:   roomSeq,
			HasOCC:        true,
		}}
		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			lastIndex := len(entries) - 1
			if err := c.roomModel.waitForTimeline(ctx, events.SubjectPosition(entries[lastIndex].Subject, seqs[lastIndex])); err != nil {
				return err
			}
			if err := c.notificationMaterializer.WaitThrough(ctx, seqs[lastIndex]); err != nil {
				c.logger.Warn("Notification cleanup did not reach the message retraction before the request completed",
					"room_id", roomID, "event_id", eventID, "error", err)
			}
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return fmt.Errorf("publish MessageRetractedEvent: %w", err)
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return fmt.Errorf("publish MessageRetractedEvent after %d attempts: %w", maxThreadCreateAppendAttempts, lastErr)
}

// publishMessageEdit emits a MessageEditedEvent on EVT. StreamMyEvents
// receives the canonical live.evt.> republish directly. Factored out so
// EditMessage / editEmbeddedBody can fan the same payload to linked messages.
type messageEditMutation func(context.Context, *evtv1.MessageBody) (plaintext string, err error)

func (c *ChattoCore) publishMessageEdit(
	ctx context.Context,
	actorID string,
	agg evtstream.Aggregate,
	roomID, eventID string,
	mutate messageEditMutation,
) (string, error) {
	return c.publishMessageEditWithAuthorization(ctx, actorID, agg, roomID, eventID, nil, nil, "", "", nil, mutate)
}

func (c *ChattoCore) publishAuthorizedMessageEdit(
	ctx context.Context,
	actorID string,
	agg evtstream.Aggregate,
	roomID, eventID string,
	authorize func(context.Context) error,
	validateCommit func() error,
	channelEchoCreationTargetID string,
	channelEchoRetractionTargetID string,
	mutate messageEditMutation,
) (string, error) {
	if authorize == nil || validateCommit == nil {
		return "", fmt.Errorf("message edit commit authorization is incomplete")
	}
	return c.publishMessageEditWithAuthorization(ctx, actorID, agg, roomID, eventID, authorize, validateCommit, channelEchoCreationTargetID, channelEchoRetractionTargetID, nil, mutate)
}

func (c *ChattoCore) publishMessageEditWithAuthorization(
	ctx context.Context,
	actorID string,
	agg evtstream.Aggregate,
	roomID, eventID string,
	authorize func(context.Context) error,
	validateCommit func() error,
	channelEchoCreationTargetID string,
	channelEchoRetractionTargetID string,
	createdChannelEchoID *string,
	mutate messageEditMutation,
) (string, error) {
	if mutate == nil {
		return "", fmt.Errorf("message edit mutation is nil")
	}
	bodySubject := agg.Subject(evtstream.EventMessageBody)
	editSubject := agg.Subject(evtstream.EventMessageEdited)
	if createdChannelEchoID != nil {
		*createdChannelEchoID = ""
	}
	bodyEventID := NewEventID()
	editEventID := NewEventID()
	echoEventID := NewEventID()
	echoBodyEventID := NewEventID()
	echoRetractionEventID := NewEventID()
	committedPlaintext := ""
	committedEntries := []evtstream.BatchEntry(nil)
	committedSequences := []uint64(nil)
	committedEchoBodyIndex := -1
	committedCreatedEchoID := ""
	mutationAttempts := 0
	mutationConflicts := 0
	var lastErr error

	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		mutationAttempts = attempt
		guard, err := c.prepareMessageAppendAttempt(ctx, agg, authorize)
		if err != nil {
			return "", err
		}

		entry, ok := c.roomModel.timelineEntry(eventID)
		if !ok || entry.Event == nil || entry.Event.GetMessagePosted() == nil || roomIDOfEvent(entry.Event) != roomID {
			return "", ErrMessageNotFound
		}
		current, retracted, _ := c.roomModel.latestBody(eventID)
		if retracted || current == nil {
			return "", ErrMessageNotFound
		}
		updated := proto.Clone(current).(*evtv1.MessageBody)
		plaintext, err := mutate(ctx, updated)
		if err != nil {
			return "", err
		}
		updated.UpdatedAt = timestamppb.Now()
		if err := c.encryptMessageBody(ctx, updated, roomID, eventID, bodyEventID, plaintext); err != nil {
			return "", err
		}
		bodyEvent := newEvent(actorID, &evtv1.Event{
			Id: bodyEventID,
			Event: &evtv1.Event_MessageBody{
				MessageBody: &evtv1.MessageBodyEvent{
					RoomId:  roomID,
					EventId: eventID,
					Body:    updated,
				},
			},
		})
		event := newEvent(actorID, &evtv1.Event{
			Id: editEventID,
			Event: &evtv1.Event_MessageEdited{
				MessageEdited: &evtv1.MessageEditedEvent{
					RoomId:  roomID,
					EventId: eventID,
				},
			},
		})
		// JetStream evaluates the room guard and commits the complete batch
		// atomically. Authorization was evaluated from stable request-time
		// inputs above; a later authorization change is concurrent with this
		// command and does not cancel it.
		entries := []evtstream.BatchEntry{
			{
				Subject:       bodySubject,
				Event:         bodyEvent,
				FilterSubject: guard.roomFilter,
				ExpectedSeq:   guard.roomSeq,
				HasOCC:        true,
			},
			{
				Subject: editSubject,
				Event:   event,
			},
		}
		echoBodyIndex := -1
		attemptCreatedEchoID := ""
		if channelEchoCreationTargetID != "" {
			if _, ok := c.roomModel.channelEchoEventID(channelEchoCreationTargetID); !ok {
				if channelEchoCreationTargetID != eventID {
					return "", ErrMessageNotFound
				}
				targetEntry, ok := c.roomModel.timelineEntry(channelEchoCreationTargetID)
				if !ok || targetEntry.Event == nil {
					return "", ErrMessageNotFound
				}
				targetPost := targetEntry.Event.GetMessagePosted()
				if targetPost == nil || targetPost.GetEchoOfEventId() != "" || targetPost.GetInThread() == "" || targetPost.GetRoomId() != roomID {
					return "", invalidArgument("channel echo state can only be changed for thread replies")
				}
				echoID, echoBodyEvent, echoEvent, err := c.buildThreadReplyEchoEventsWithIDs(ctx, actorID, targetEntry.Event, targetPost, updated, plaintext, echoEventID, echoBodyEventID)
				if err != nil {
					return "", err
				}
				attemptCreatedEchoID = echoID
				echoBodyIndex = len(entries)
				entries = append(entries,
					evtstream.BatchEntry{Subject: bodySubject, Event: echoBodyEvent},
					evtstream.BatchEntry{Subject: agg.Subject(evtstream.EventMessagePosted), Event: echoEvent},
				)
			}
		}
		if channelEchoRetractionTargetID != "" {
			if echoID, ok := c.roomModel.channelEchoEventID(channelEchoRetractionTargetID); ok {
				retraction := newEvent(actorID, &evtv1.Event{
					Id: echoRetractionEventID,
					Event: &evtv1.Event_MessageRetracted{
						MessageRetracted: &evtv1.MessageRetractedEvent{RoomId: roomID, EventId: echoID},
					},
				})
				entries = append(entries, evtstream.BatchEntry{
					Subject: agg.Subject(evtstream.EventMessageRetracted),
					Event:   retraction,
				})
			}
		}
		if validateCommit != nil {
			if err := validateCommit(); err != nil {
				return "", err
			}
		}

		sequences, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			committedPlaintext = plaintext
			committedEntries = entries
			committedSequences = sequences
			committedEchoBodyIndex = echoBodyIndex
			committedCreatedEchoID = attemptCreatedEchoID
			break
		}
		if !errors.Is(err, events.ErrConflict) {
			return "", err
		}
		mutationConflicts++
		lastErr = err
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	if len(committedEntries) == 0 {
		return "", fmt.Errorf("publish MessageEditedEvent after %d attempts: %w", maxThreadCreateAppendAttempts, lastErr)
	}
	if len(committedSequences) != len(committedEntries) {
		return "", fmt.Errorf("publish MessageEditedEvent: committed %d sequences for %d events", len(committedSequences), len(committedEntries))
	}
	if createdChannelEchoID != nil {
		*createdChannelEchoID = committedCreatedEchoID
	}
	lastIndex := len(committedEntries) - 1
	if err := c.roomModel.waitForTimeline(ctx, events.SubjectPosition(committedEntries[lastIndex].Subject, committedSequences[lastIndex])); err != nil {
		return "", err
	}
	if err := c.waitForMessageBodyAssets(ctx, bodySubject, committedSequences[0]); err != nil {
		return "", err
	}
	if committedEchoBodyIndex >= 0 {
		if err := c.waitForMessageBodyAssets(ctx, committedEntries[committedEchoBodyIndex].Subject, committedSequences[committedEchoBodyIndex]); err != nil {
			return "", err
		}
	}
	c.logger.Debug("Message edit mutation committed",
		"room_id", roomID,
		"event_id", eventID,
		"mutation_attempts", mutationAttempts,
		"mutation_conflicts", mutationConflicts,
	)
	return committedPlaintext, nil
}

func validateLinkPreview(linkPreview *evtv1.LinkPreview) error {
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
		if err := validateLinkPreviewAsset("link preview image", imageAsset); err != nil {
			return err
		}
		if linkPreview.GetImageAssetId() != "" && imageAsset.GetId() != "" && linkPreview.GetImageAssetId() != imageAsset.GetId() {
			return invalidArgument("link preview image asset ID does not match image asset record")
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
	if socialPost := linkPreview.GetSocialPost(); socialPost != nil {
		if err := validateSocialPostPreview(socialPost, 0); err != nil {
			return err
		}
	}
	return nil
}

func validateSocialPostPreview(socialPost *evtv1.SocialPostPreview, quoteDepth int) error {
	if socialPost == nil {
		return nil
	}
	if socialPost.GetProvider() == "" {
		return invalidArgument("social post provider is required")
	}
	if err := validateStringMaxLength("social post provider", socialPost.GetProvider(), MaxLinkPreviewEmbedTypeLength); err != nil {
		return err
	}
	if err := validateStringMaxLength("social post URL", socialPost.GetUrl(), MaxLinkPreviewURLLength); err != nil {
		return err
	}
	if quoteDepth > 0 && socialPost.GetUrl() == "" {
		return invalidArgument("quoted social post URL is required")
	}
	if err := validateStringMaxLength("social post text", socialPost.GetText(), MaxLinkPreviewDescriptionLength); err != nil {
		return err
	}
	if err := validateStringMaxLength("social post content warning", socialPost.GetContentWarning(), MaxLinkPreviewTitleLength); err != nil {
		return err
	}
	author := socialPost.GetAuthor()
	if author == nil || (author.GetDisplayName() == "" && author.GetHandle() == "") {
		return invalidArgument("social post author is required")
	}
	if author != nil {
		if err := validateStringMaxLength("social post author display name", author.GetDisplayName(), MaxLinkPreviewTitleLength); err != nil {
			return err
		}
		if err := validateStringMaxLength("social post author handle", author.GetHandle(), MaxLinkPreviewSiteNameLength); err != nil {
			return err
		}
		if err := validateLinkPreviewAsset("social post author avatar", author.GetAvatarAsset()); err != nil {
			return err
		}
	}
	if external := socialPost.GetExternalLink(); external != nil {
		if external.GetUrl() == "" {
			return invalidArgument("social post external URL is required")
		}
		if err := validateStringMaxLength("social post external URL", external.GetUrl(), MaxLinkPreviewURLLength); err != nil {
			return err
		}
		if err := validateStringMaxLength("social post external title", external.GetTitle(), MaxLinkPreviewTitleLength); err != nil {
			return err
		}
		if err := validateStringMaxLength("social post external description", external.GetDescription(), MaxLinkPreviewDescriptionLength); err != nil {
			return err
		}
		if err := validateLinkPreviewAsset("social post external image", external.GetImageAsset()); err != nil {
			return err
		}
	}
	if len(socialPost.GetImages()) > 4 {
		return invalidArgument("social post has more than 4 images")
	}
	for _, image := range socialPost.GetImages() {
		if image == nil || image.GetAsset() == nil {
			return invalidArgument("social post image asset is required")
		}
		if err := validateStringMaxLength("social post image alt text", image.GetAlt(), MaxLinkPreviewDescriptionLength); err != nil {
			return err
		}
		if err := validateLinkPreviewAsset("social post image", image.GetAsset()); err != nil {
			return err
		}
	}
	if quotedPost := socialPost.GetQuotedPost(); quotedPost != nil {
		if quoteDepth >= 1 {
			return invalidArgument("social post quote nesting exceeds 1")
		}
		return validateSocialPostPreview(quotedPost, quoteDepth+1)
	}
	return nil
}

func validateLinkPreviewAsset(name string, asset *evtv1.AssetRecord) error {
	if asset == nil {
		return nil
	}
	if err := validateStringMaxLength(name+" asset ID", asset.GetId(), MaxLinkPreviewImageAssetIDLength); err != nil {
		return err
	}
	if asset.GetStorage() == nil {
		return invalidArgument(name + " asset record is missing storage")
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
	roomID, eventID string,
	commitAuthorize func(context.Context) error,
	mutate func(*evtv1.MessageBody) error,
) error {
	if eventID == "" {
		return ErrMessageNotFound
	}
	agg := evtstream.RoomAggregate(roomID)
	policy := messageMutationAuthorization{authorOnly: true}
	authorize := func(attemptCtx context.Context) error {
		if err := c.authorizeMessageMutation(attemptCtx, actorID, kind, roomID, eventID, policy, time.Now()); err != nil {
			return err
		}
		if commitAuthorize != nil {
			return commitAuthorize(attemptCtx)
		}
		return nil
	}
	validateCommit := func() error {
		_, err := c.validateMessageMutationIdentity(ctx, actorID, kind, roomID, eventID, policy, time.Now())
		return err
	}
	_, err := c.publishAuthorizedMessageEdit(ctx, actorID, agg, roomID, eventID, authorize, validateCommit, "", "", func(ctx context.Context, updated *evtv1.MessageBody) (string, error) {
		if updated.GetAuthorId() != actorID {
			return "", ErrNotMessageAuthor
		}
		plaintext, err := c.decryptMessageBody(ctx, eventID, roomID, updated)
		if err != nil {
			return "", fmt.Errorf("decrypt message body for edit: %w", err)
		}
		if err := mutate(updated); err != nil {
			return "", err
		}
		return string(plaintext), nil
	})
	if err != nil {
		return err
	}
	c.secureDeleteObsoleteMessageBodyEvents(ctx, eventID)
	for _, linkedID := range c.roomModel.linkedEventIDs(eventID) {
		if _, err := c.publishMessageEdit(ctx, actorID, agg, roomID, linkedID, func(ctx context.Context, linkedBody *evtv1.MessageBody) (string, error) {
			plaintext, err := c.decryptMessageBody(ctx, linkedID, roomID, linkedBody)
			if err != nil {
				return "", fmt.Errorf("decrypt linked message body for edit: %w", err)
			}
			if err := mutate(linkedBody); err != nil {
				return "", err
			}
			return string(plaintext), nil
		}); err != nil {
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
func (c *ChattoCore) DeleteAttachmentFromMessage(ctx context.Context, actorID string, kind RoomKind, roomID, eventID, attachmentID string) error {
	var removed *evtv1.Attachment
	err := c.editEmbeddedBody(ctx, actorID, kind, roomID, eventID, nil, func(body *evtv1.MessageBody) error {
		// Resolve the attachment (new bodies hold IDs; older bodies hold
		// embedded protos). Then trim from whichever shape holds it.
		for _, att := range c.mediaModel.MessageBodyAttachments(body) {
			if att.GetId() == attachmentID {
				removed = att
				break
			}
		}
		if removed == nil {
			return fmt.Errorf("attachment not found in message: %w", ErrMessageAttachmentNotFound)
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
		owned, err := c.assetModel.MessageOwnsAsset(ctx, roomID, eventID, removed.GetId())
		if err != nil {
			return fmt.Errorf("verify message asset ownership: %w", err)
		}
		if !owned {
			return nil
		}
		c.assetModel.DeleteVideoDerivativesForAttachment(ctx, actorID, removed.GetId())
		deleted, err := c.assetModel.RecordMessageAssetDeleted(ctx, actorID, roomID, eventID, removed.GetId())
		if err != nil {
			c.logger.Warn("Failed to publish asset deletion event",
				"attachment_id", attachmentID,
				"event_id", eventID,
				"error", err)
		} else if deleted {
			if delErr := c.DeleteAttachmentFromStorage(ctx, removed); delErr != nil {
				c.logger.Warn("Failed to delete attachment file after removing from message",
					"attachment_id", attachmentID,
					"event_id", eventID,
					"error", delErr)
			}
		}
	}

	c.logger.Debug("Attachment deleted from message",
		"kind", kind,
		"room_id", roomID,
		"event_id", eventID,
		"attachment_id", attachmentID,
		"actor_id", actorID)
	return nil
}

// DeleteLinkPreviewFromMessage removes a link preview from a message.
// Only the message author can delete link previews from their
// messages.
func (c *ChattoCore) DeleteLinkPreviewFromMessage(ctx context.Context, actorID string, kind RoomKind, roomID, eventID, previewURL string) error {
	err := c.editEmbeddedBody(ctx, actorID, kind, roomID, eventID, nil, func(body *evtv1.MessageBody) error {
		if body.GetLinkPreview() == nil || body.GetLinkPreview().GetUrl() != previewURL {
			return fmt.Errorf("link preview not found in message: %w", ErrMessageLinkPreviewNotFound)
		}
		body.LinkPreview = nil
		return nil
	})
	if err != nil {
		return err
	}
	c.logger.Debug("Link preview deleted from message",
		"kind", kind,
		"room_id", roomID,
		"event_id", eventID,
		"link_preview_removed", true,
		"actor_id", actorID)
	return nil
}
