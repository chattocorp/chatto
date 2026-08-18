package core

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/notificationstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const notificationPhysicalDeleteRetry = time.Minute

type CreateNotificationOccurrenceInput struct {
	RecipientID          string
	SourceEventID        string
	SourceCreated        time.Time
	ActorID              string
	Signal               *corev1.NotificationSignal
	Mode                 corev1.NotificationDeliveryMode
	AttentionLevel       corev1.NotificationAttentionLevel
	SourceStreamSequence uint64
	InitiallyRead        bool
	SkipReadLookup       bool
}

// NotificationOccurrenceModel owns commands and reads over the bounded
// NOTIFICATIONS event log and its process-wide framework projection.
type NotificationOccurrenceModel struct {
	core       *ChattoCore
	projection events.ProjectionHandle[*NotificationProjection]
	publisher  *notificationstream.Publisher
	stream     jetstream.Stream
	kv         jetstream.KeyValue
	logger     *log.Logger
	now        func() time.Time

	cleanedMu sync.Mutex
	// cleaned retains successful secure-deletion results only through the
	// matching tombstone lifetime, avoiding repeated broker calls without
	// growing for the lifetime of the process.
	cleaned map[uint64]time.Time
}

func NewNotificationOccurrenceModel(
	core *ChattoCore,
	projection events.ProjectionHandle[*NotificationProjection],
	publisher *notificationstream.Publisher,
	stream jetstream.Stream,
	kv jetstream.KeyValue,
	logger *log.Logger,
) *NotificationOccurrenceModel {
	return &NotificationOccurrenceModel{
		core:       core,
		projection: projection,
		publisher:  publisher,
		stream:     stream,
		kv:         kv,
		logger:     logger,
		now:        time.Now,
		cleaned:    make(map[uint64]time.Time),
	}
}

func (c *ChattoCore) NotificationOccurrences() *NotificationOccurrenceModel {
	return c.notificationOccurrences
}

func (m *NotificationOccurrenceModel) WaitCurrent(ctx context.Context) error {
	if err := m.core.notificationMaterializer.WaitCurrent(ctx); err != nil {
		return err
	}
	return m.projection.Projector().WaitForCurrent(ctx)
}

func (m *NotificationOccurrenceModel) WaitReady(ctx context.Context) error {
	return m.projection.Projector().WaitForStartup(ctx)
}

func (m *NotificationOccurrenceModel) Resync(ctx context.Context) error {
	return m.projection.Projector().WaitForCurrent(ctx)
}

func (m *NotificationOccurrenceModel) Run(ctx context.Context) error {
	if err := m.WaitReady(ctx); err != nil {
		return err
	}
	for {
		now := m.now().UTC()
		for _, userID := range m.projection.Projection().pruneExpired(now) {
			m.core.publishNotificationOccurrencesInvalidated(ctx, &corev1.NotificationOccurrence{RecipientId: userID}, false)
		}
		m.cleanupDismissedSignals(ctx, now)

		delay := notificationPhysicalDeleteRetry
		if next, ok := m.projection.Projection().nextExpiry(now); ok {
			until := next.Sub(now)
			if until < delay {
				delay = until
			}
		}
		if delay < time.Millisecond {
			delay = time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-m.projection.Projection().expiryChanges():
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (m *NotificationOccurrenceModel) cleanupDismissedSignals(ctx context.Context, now time.Time) {
	m.cleanedMu.Lock()
	for sequence, expiresAt := range m.cleaned {
		if !expiresAt.After(now) {
			delete(m.cleaned, sequence)
		}
	}
	m.cleanedMu.Unlock()
	for notificationID, tombstone := range m.projection.Projection().pendingPhysicalDeletes(now) {
		m.cleanedMu.Lock()
		_, cleaned := m.cleaned[tombstone.signalSequence]
		m.cleanedMu.Unlock()
		if cleaned {
			continue
		}
		if err := m.stream.SecureDeleteMsg(ctx, tombstone.signalSequence); err != nil && !notificationSignalAlreadyAbsent(err) {
			m.logger.Warn("Notification signal physical deletion will retry", "notification_id", notificationID, "error", err)
			continue
		}
		m.cleanedMu.Lock()
		m.cleaned[tombstone.signalSequence] = tombstone.expiresAt
		m.cleanedMu.Unlock()
	}
}

// NATS does not currently expose these server API error codes as Go constants.
const (
	jetStreamMessageNotFoundErrorCode  = 10057
	jetStreamSequenceNotFoundErrorCode = 10043
)

func notificationSignalAlreadyAbsent(err error) bool {
	if errors.Is(err, jetstream.ErrMsgNotFound) {
		return true
	}
	var jsErr jetstream.JetStreamError
	if !errors.As(err, &jsErr) || jsErr.APIError() == nil {
		return false
	}
	switch jsErr.APIError().ErrorCode {
	case jetStreamMessageNotFoundErrorCode, jetStreamSequenceNotFoundErrorCode:
		return true
	default:
		return false
	}
}

func notificationOccurrenceID(recipientID, sourceEventID, signalKind string) string {
	digest := sha256.Sum256([]byte(recipientID + "\x00" + sourceEventID + "\x00" + signalKind))
	return "ntf_" + base64.RawURLEncoding.EncodeToString(digest[:20])
}

func notificationLifecycleEventID(operation, notificationID string) string {
	digest := sha256.Sum256([]byte("notification-lifecycle\x00" + operation + "\x00" + notificationID))
	return "nte_" + base64.RawURLEncoding.EncodeToString(digest[:20])
}

func newNotificationMessageReference(roomID, eventID string) *corev1.NotificationMessageReference {
	return &corev1.NotificationMessageReference{RoomId: roomID, EventId: eventID}
}

func notificationSignalMessage(signal *corev1.NotificationSignal) *corev1.NotificationMessageReference {
	if signal == nil {
		return nil
	}
	switch payload := signal.GetKind().(type) {
	case *corev1.NotificationSignal_DirectMessageReceived:
		return payload.DirectMessageReceived.GetMessage()
	case *corev1.NotificationSignal_DirectMentionReceived:
		return payload.DirectMentionReceived.GetMessage()
	case *corev1.NotificationSignal_ReplyReceived:
		return payload.ReplyReceived.GetMessage()
	case *corev1.NotificationSignal_RoleMentionReceived:
		return payload.RoleMentionReceived.GetMessage()
	case *corev1.NotificationSignal_HereMentionReceived:
		return payload.HereMentionReceived.GetMessage()
	case *corev1.NotificationSignal_AllMentionReceived:
		return payload.AllMentionReceived.GetMessage()
	case *corev1.NotificationSignal_FollowedThreadActivity:
		return payload.FollowedThreadActivity.GetMessage()
	case *corev1.NotificationSignal_FollowedRoomActivity:
		return payload.FollowedRoomActivity.GetMessage()
	case *corev1.NotificationSignal_ReactionReceived:
		return payload.ReactionReceived.GetMessage()
	default:
		return nil
	}
}

// NotificationOccurrenceMessageReference returns the exact message target for
// a signal understood by this server version.
func NotificationOccurrenceMessageReference(occurrence *corev1.NotificationOccurrence) *corev1.NotificationMessageReference {
	if occurrence == nil {
		return nil
	}
	return notificationSignalMessage(occurrence.GetSignal())
}

func notificationSignalPreferenceCategory(signal *corev1.NotificationSignal) corev1.NotificationPreferenceCategory {
	if signal == nil {
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_UNSPECIFIED
	}
	switch signal.GetKind().(type) {
	case *corev1.NotificationSignal_DirectMessageReceived:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MESSAGE
	case *corev1.NotificationSignal_DirectMentionReceived:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MENTION
	case *corev1.NotificationSignal_ReplyReceived:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REPLY
	case *corev1.NotificationSignal_RoleMentionReceived:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_ROLE_MENTION
	case *corev1.NotificationSignal_HereMentionReceived:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_HERE
	case *corev1.NotificationSignal_AllMentionReceived:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_ALL
	case *corev1.NotificationSignal_FollowedThreadActivity:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_THREAD
	case *corev1.NotificationSignal_FollowedRoomActivity:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_ROOM
	case *corev1.NotificationSignal_ReactionReceived:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION
	default:
		return corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_UNSPECIFIED
	}
}

func notificationSignalForPreferenceCategory(category corev1.NotificationPreferenceCategory, message *corev1.NotificationMessageReference, emoji string) *corev1.NotificationSignal {
	if message == nil {
		return nil
	}
	cloned := func() *corev1.NotificationMessageReference {
		return proto.Clone(message).(*corev1.NotificationMessageReference)
	}
	signal := &corev1.NotificationSignal{}
	switch category {
	case corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MESSAGE:
		signal.Kind = &corev1.NotificationSignal_DirectMessageReceived{DirectMessageReceived: &corev1.DirectMessageReceived{Message: cloned()}}
	case corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MENTION:
		signal.Kind = &corev1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &corev1.DirectMentionReceived{Message: cloned()}}
	case corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REPLY:
		signal.Kind = &corev1.NotificationSignal_ReplyReceived{ReplyReceived: &corev1.ReplyReceived{Message: cloned()}}
	case corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_ROLE_MENTION:
		signal.Kind = &corev1.NotificationSignal_RoleMentionReceived{RoleMentionReceived: &corev1.RoleMentionReceived{Message: cloned()}}
	case corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_HERE:
		signal.Kind = &corev1.NotificationSignal_HereMentionReceived{HereMentionReceived: &corev1.HereMentionReceived{Message: cloned()}}
	case corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_ALL:
		signal.Kind = &corev1.NotificationSignal_AllMentionReceived{AllMentionReceived: &corev1.AllMentionReceived{Message: cloned()}}
	case corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_THREAD:
		signal.Kind = &corev1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &corev1.FollowedThreadActivity{Message: cloned()}}
	case corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_ROOM:
		signal.Kind = &corev1.NotificationSignal_FollowedRoomActivity{FollowedRoomActivity: &corev1.FollowedRoomActivity{Message: cloned()}}
	case corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION:
		signal.Kind = &corev1.NotificationSignal_ReactionReceived{ReactionReceived: &corev1.ReactionReceived{Message: cloned(), Emoji: emoji}}
	default:
		return nil
	}
	return signal
}

func notificationSignalIdentity(signal *corev1.NotificationSignal) string {
	if signal == nil {
		return ""
	}
	switch signal.GetKind().(type) {
	case *corev1.NotificationSignal_DirectMessageReceived:
		return "direct_message_received"
	case *corev1.NotificationSignal_DirectMentionReceived:
		return "direct_mention_received"
	case *corev1.NotificationSignal_ReplyReceived:
		return "reply_received"
	case *corev1.NotificationSignal_RoleMentionReceived:
		return "role_mention_received"
	case *corev1.NotificationSignal_HereMentionReceived:
		return "here_mention_received"
	case *corev1.NotificationSignal_AllMentionReceived:
		return "all_mention_received"
	case *corev1.NotificationSignal_FollowedThreadActivity:
		return "followed_thread_activity"
	case *corev1.NotificationSignal_FollowedRoomActivity:
		return "followed_room_activity"
	case *corev1.NotificationSignal_ReactionReceived:
		return "reaction_received"
	default:
		return ""
	}
}

func NotificationOccurrenceHasUnsupportedSignal(occurrence *corev1.NotificationOccurrence) bool {
	signal := occurrence.GetSignal()
	return signal != nil && signal.GetKind() == nil && len(signal.ProtoReflect().GetUnknown()) > 0
}

func NotificationAlertDeadline(occurrence *corev1.NotificationOccurrence) time.Time {
	if occurrence == nil {
		return time.Time{}
	}
	if deadline := occurrence.GetAlertExpiresAt(); deadline != nil && deadline.IsValid() {
		return deadline.AsTime().UTC()
	}
	return time.Time{}
}

func (m *NotificationOccurrenceModel) appendAndWait(ctx context.Context, event *corev1.NotificationEvent) error {
	position, err := m.publisher.AppendEventually(ctx, event)
	if errors.Is(err, notificationstream.ErrExpiredEvent) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return m.projection.Projector().WaitFor(ctx, position)
}

func (m *NotificationOccurrenceModel) Create(ctx context.Context, input CreateNotificationOccurrenceInput) (*corev1.NotificationOccurrence, bool, error) {
	if strings.TrimSpace(input.RecipientID) == "" || strings.TrimSpace(input.SourceEventID) == "" || input.SourceCreated.IsZero() {
		return nil, false, invalidArgument("recipient_id, source_event_id, and source_created_at are required")
	}
	message := notificationSignalMessage(input.Signal)
	category := notificationSignalPreferenceCategory(input.Signal)
	signalKind := notificationSignalIdentity(input.Signal)
	if message == nil || message.GetRoomId() == "" || message.GetEventId() == "" || category == corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_UNSPECIFIED || signalKind == "" {
		return nil, false, invalidArgument("a supported notification signal with an exact message is required")
	}
	switch input.Mode {
	case corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF:
		return nil, false, nil
	case corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_BADGE,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT:
	default:
		return nil, false, invalidArgument("unsupported notification delivery mode")
	}
	if err := m.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, false, err
	}
	expiresAt := input.SourceCreated.UTC().Add(notificationTTL)
	if !expiresAt.After(m.now().UTC()) {
		return nil, false, nil
	}
	notificationID := notificationOccurrenceID(input.RecipientID, input.SourceEventID, signalKind)
	if m.projection.Projection().tombstoned(input.RecipientID, notificationID, m.now().UTC()) {
		return nil, false, nil
	}
	if existing, err := m.Get(ctx, input.RecipientID, notificationID); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}

	attention := input.AttentionLevel
	if attention != corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT && attention != corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT {
		return nil, false, invalidArgument("a concrete notification attention level is required")
	}
	alertRequested := input.Mode == corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT
	var alertExpiresAt *timestamppb.Timestamp
	if alertRequested {
		alertExpiresAt = timestamppb.New(input.SourceCreated.UTC().Add(notificationAlertDeliveryTTL))
	}
	occurrence := &corev1.NotificationOccurrence{
		Id:                   notificationID,
		RecipientId:          input.RecipientID,
		SourceEventId:        input.SourceEventID,
		SourceCreatedAt:      timestamppb.New(input.SourceCreated.UTC()),
		ActorId:              input.ActorID,
		Signal:               proto.Clone(input.Signal).(*corev1.NotificationSignal),
		Read:                 input.InitiallyRead,
		ExpiresAt:            timestamppb.New(expiresAt),
		SourceStreamSequence: input.SourceStreamSequence,
		AttentionLevel:       attention,
		AlertExpiresAt:       alertExpiresAt,
	}
	if !input.SkipReadLookup && !occurrence.GetRead() {
		covered, err := m.occurrenceCoveredByReadBoundary(ctx, occurrence)
		if err != nil {
			return nil, false, err
		}
		if covered {
			occurrence.Read = true
			if occurrence.GetAlertExpiresAt() != nil {
				occurrence.AlertDelivered = proto.Bool(false)
			}
		}
	}
	event := &corev1.NotificationEvent{
		Id:             notificationLifecycleEventID("signal", notificationID),
		RecipientId:    input.RecipientID,
		NotificationId: notificationID,
		OccurredAt:     timestamppb.New(m.now().UTC()),
		ExpiresAt:      timestamppb.New(expiresAt),
		Event: &corev1.NotificationEvent_Signalled{Signalled: &corev1.NotificationSignalled{
			SourceEventId:        occurrence.GetSourceEventId(),
			SourceCreatedAt:      occurrence.GetSourceCreatedAt(),
			ActorId:              occurrence.GetActorId(),
			Signal:               occurrence.GetSignal(),
			InitiallyRead:        occurrence.GetRead(),
			SourceStreamSequence: occurrence.GetSourceStreamSequence(),
			AttentionLevel:       occurrence.GetAttentionLevel(),
			AlertExpiresAt:       occurrence.GetAlertExpiresAt(),
		}},
	}
	if err := m.appendAndWait(ctx, event); err != nil {
		return nil, false, err
	}
	stored, err := m.Get(ctx, input.RecipientID, notificationID)
	if errors.Is(err, ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !input.SkipReadLookup && !stored.GetRead() {
		covered, err := m.occurrenceCoveredByReadBoundary(ctx, stored)
		if err != nil {
			return nil, true, err
		}
		if covered {
			stored, err = m.MarkRead(ctx, input.RecipientID, notificationID)
			if err != nil {
				return nil, true, err
			}
		}
	}
	m.core.publishNotificationOccurrencesInvalidated(ctx, stored, true)
	return stored, true, nil
}

func (m *NotificationOccurrenceModel) Get(_ context.Context, userID, notificationID string) (*corev1.NotificationOccurrence, error) {
	occurrence, exists := m.projection.Projection().occurrence(userID, notificationID, m.now().UTC())
	if !exists {
		return nil, ErrNotFound
	}
	return occurrence, nil
}

func (m *NotificationOccurrenceModel) List(_ context.Context, userID string) ([]*corev1.NotificationOccurrence, error) {
	result := m.projection.Projection().userOccurrences(userID, m.now().UTC())
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		leftCreated, rightCreated := left.GetSourceCreatedAt().AsTime(), right.GetSourceCreatedAt().AsTime()
		if !leftCreated.Equal(rightCreated) {
			return leftCreated.After(rightCreated)
		}
		if left.GetSourceStreamSequence() != right.GetSourceStreamSequence() {
			return left.GetSourceStreamSequence() > right.GetSourceStreamSequence()
		}
		return left.GetId() > right.GetId()
	})
	return result, nil
}

func (m *NotificationOccurrenceModel) UnreadCount(ctx context.Context, userID string) (int, error) {
	occurrences, err := m.List(ctx, userID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, occurrence := range occurrences {
		if !occurrence.GetRead() {
			count++
		}
	}
	return count, nil
}

func (m *NotificationOccurrenceModel) MarkRead(ctx context.Context, userID, notificationID string) (*corev1.NotificationOccurrence, error) {
	occurrence, err := m.Get(ctx, userID, notificationID)
	if err != nil {
		return nil, err
	}
	if occurrence.GetRead() {
		return occurrence, nil
	}
	now := m.now().UTC()
	event := &corev1.NotificationEvent{
		Id:             notificationLifecycleEventID("read", notificationID),
		RecipientId:    userID,
		NotificationId: notificationID,
		OccurredAt:     timestamppb.New(now),
		ExpiresAt:      occurrence.GetExpiresAt(),
		Event:          &corev1.NotificationEvent_Read{Read: &corev1.NotificationRead{}},
	}
	if err := m.appendAndWait(ctx, event); err != nil {
		return nil, err
	}
	updated, err := m.Get(ctx, userID, notificationID)
	if err == nil {
		m.core.publishNotificationOccurrencesInvalidated(ctx, updated, false)
	}
	return updated, err
}

func (m *NotificationOccurrenceModel) Delete(ctx context.Context, userID, notificationID string) (bool, error) {
	occurrence, err := m.Get(ctx, userID, notificationID)
	if errors.Is(err, ErrNotFound) {
		m.cleanupDismissedSignals(ctx, m.now().UTC())
		return false, nil
	}
	if err != nil {
		return false, err
	}
	now := m.now().UTC()
	event := &corev1.NotificationEvent{
		Id:             notificationLifecycleEventID("remove", notificationID),
		RecipientId:    userID,
		NotificationId: notificationID,
		OccurredAt:     timestamppb.New(now),
		ExpiresAt:      occurrence.GetExpiresAt(),
		Event:          &corev1.NotificationEvent_Removed{Removed: &corev1.NotificationRemoved{}},
	}
	if err := m.appendAndWait(ctx, event); err != nil {
		return false, err
	}
	m.core.publishNotificationOccurrencesInvalidated(ctx, occurrence, false)
	m.cleanupDismissedSignals(ctx, now)
	return true, nil
}

func (m *NotificationOccurrenceModel) DeleteMany(ctx context.Context, userID string, notificationIDs []string) (int, error) {
	seen := make(map[string]struct{}, len(notificationIDs))
	deleted := 0
	for _, id := range notificationIDs {
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		ok, err := m.Delete(ctx, userID, id)
		if err != nil {
			return deleted, err
		}
		if ok {
			deleted++
		}
	}
	return deleted, nil
}

func (m *NotificationOccurrenceModel) MarkCoveredRead(ctx context.Context, userID, roomID, threadRootEventID, targetEventID string) (int, error) {
	if _, err := m.recordNotificationReadBoundary(ctx, userID, roomID, threadRootEventID, targetEventID); err != nil {
		return 0, err
	}
	occurrences, err := m.List(ctx, userID)
	if err != nil {
		return 0, err
	}
	updated := 0
	for _, occurrence := range occurrences {
		message := notificationSignalMessage(occurrence.GetSignal())
		if message == nil || message.GetRoomId() != roomID || message.GetThreadRootEventId() != threadRootEventID || occurrence.GetRead() {
			continue
		}
		covered, err := m.occurrenceCoveredByReadBoundary(ctx, occurrence)
		if err != nil {
			return updated, err
		}
		if !covered {
			continue
		}
		if _, err := m.MarkRead(ctx, userID, occurrence.GetId()); err != nil && !errors.Is(err, ErrNotFound) {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

func (m *NotificationOccurrenceModel) VisibleOccurrences(ctx context.Context, recipientID string, occurrences []*corev1.NotificationOccurrence) ([]*corev1.NotificationOccurrence, error) {
	if len(occurrences) == 0 {
		return nil, nil
	}
	userPosition, err := m.core.EventPublisher.LastSubjectPosition(ctx, evtstream.UserAggregate(recipientID).AllEventsFilter())
	if err != nil {
		return nil, fmt.Errorf("capture notification recipient boundary: %w", err)
	}
	roomPosition, err := m.core.EventPublisher.LastSubjectPosition(ctx, evtstream.RoomSubjectFilter())
	if err != nil {
		return nil, fmt.Errorf("capture notification rooms boundary: %w", err)
	}
	groupPosition, err := m.core.EventPublisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
	if err != nil {
		return nil, fmt.Errorf("capture notification room-group boundary: %w", err)
	}
	rbacPosition, err := m.core.EventPublisher.LastSubjectPosition(ctx, evtstream.RBACSubjectFilter())
	if err != nil {
		return nil, fmt.Errorf("capture notification RBAC boundary: %w", err)
	}
	if !userPosition.IsZero() {
		if err := m.core.userModel.waitForUsers(ctx, userPosition); err != nil {
			return nil, fmt.Errorf("wait for notification recipient boundary: %w", err)
		}
	}
	if !roomPosition.IsZero() {
		if err := waitForPositionAll(ctx, roomPosition,
			waitForProjection("notification room directory", m.core.roomModel.directory.Projector()),
			waitForProjection("notification room timeline", m.core.roomModel.timeline.Projector()),
			waitForProjection("notification reactions", m.core.roomModel.reactions.Projector()),
		); err != nil {
			return nil, fmt.Errorf("wait for notification rooms visibility boundary: %w", err)
		}
	}
	if err := m.core.roomModel.waitForGroupLayout(ctx, groupPosition); err != nil {
		return nil, fmt.Errorf("wait for notification room-group visibility boundary: %w", err)
	}
	if err := m.core.rbacModel.waitFor(ctx, rbacPosition); err != nil {
		return nil, fmt.Errorf("wait for notification RBAC visibility boundary: %w", err)
	}
	visible := make([]*corev1.NotificationOccurrence, 0, len(occurrences))
	for _, occurrence := range occurrences {
		allowed, err := m.targetVisibleFromCurrentProjections(ctx, recipientID, occurrence)
		if err != nil {
			return nil, err
		}
		if allowed {
			visible = append(visible, occurrence)
		}
	}
	return visible, nil
}

func (m *NotificationOccurrenceModel) targetVisibleFromCurrentProjections(ctx context.Context, recipientID string, occurrence *corev1.NotificationOccurrence) (bool, error) {
	if occurrence == nil || occurrence.GetRecipientId() != recipientID {
		return false, nil
	}
	message := notificationSignalMessage(occurrence.GetSignal())
	if message == nil || message.GetRoomId() == "" {
		return false, nil
	}
	if _, err := m.core.GetUser(ctx, recipientID); errors.Is(err, ErrNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	room, err := m.core.FindRoomByID(ctx, message.GetRoomId())
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	member, err := m.core.RoomMembershipExists(ctx, KindOfRoom(room), recipientID, room.GetId())
	if err != nil || !member {
		return member, err
	}
	messageVisible := func(eventID string) bool {
		entry, ok := m.core.roomModel.timelineEntry(eventID)
		if !ok || entry.Event == nil || roomIDOfEvent(entry.Event) != room.GetId() {
			return false
		}
		_, retracted, known := m.core.roomModel.latestBody(eventID)
		return known && !retracted
	}
	if !messageVisible(message.GetEventId()) || (message.GetThreadRootEventId() != "" && !messageVisible(message.GetThreadRootEventId())) {
		return false, nil
	}
	if reaction := occurrence.GetSignal().GetReactionReceived(); reaction != nil {
		snapshot := m.core.roomModel.reactionMutationSnapshot(room.GetId(), message.GetEventId(), reaction.GetEmoji(), occurrence.GetActorId())
		return snapshot.Exists && snapshot.SourceEventID == occurrence.GetSourceEventId(), nil
	}
	return true, nil
}

func (m *NotificationOccurrenceModel) RemoveTarget(ctx context.Context, roomID, eventID string) (int, error) {
	return m.removeMatching(ctx, func(occurrence *corev1.NotificationOccurrence) bool {
		message := notificationSignalMessage(occurrence.GetSignal())
		return message != nil && message.GetRoomId() == roomID && (message.GetEventId() == eventID || message.GetThreadRootEventId() == eventID)
	})
}

func (m *NotificationOccurrenceModel) RemoveReaction(ctx context.Context, recipientID, roomID, messageEventID, actorID, emoji string, removedAtSequence uint64) (int, error) {
	occurrences, err := m.List(ctx, recipientID)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, occurrence := range occurrences {
		message := notificationSignalMessage(occurrence.GetSignal())
		reaction := occurrence.GetSignal().GetReactionReceived()
		if message == nil || reaction == nil || occurrence.GetActorId() != actorID || reaction.GetEmoji() != emoji || message.GetRoomId() != roomID || message.GetEventId() != messageEventID || occurrence.GetSourceStreamSequence() >= removedAtSequence {
			continue
		}
		ok, err := m.Delete(ctx, recipientID, occurrence.GetId())
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
		}
	}
	return removed, nil
}

func notificationOccurrenceHasPreferenceCategory(occurrence *corev1.NotificationOccurrence, category corev1.NotificationPreferenceCategory) bool {
	return occurrence != nil && notificationSignalPreferenceCategory(occurrence.GetSignal()) == category
}

func (m *NotificationOccurrenceModel) RemoveRoomForUser(ctx context.Context, userID, roomID string, removedThroughSequence uint64) (int, error) {
	occurrences, err := m.List(ctx, userID)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, occurrence := range occurrences {
		message := notificationSignalMessage(occurrence.GetSignal())
		if message == nil || message.GetRoomId() != roomID || (removedThroughSequence != 0 && occurrence.GetSourceStreamSequence() >= removedThroughSequence) {
			continue
		}
		ok, err := m.Delete(ctx, userID, occurrence.GetId())
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
		}
	}
	return removed, nil
}

func (m *NotificationOccurrenceModel) RemoveRoom(ctx context.Context, roomID string) (int, error) {
	return m.removeMatching(ctx, func(occurrence *corev1.NotificationOccurrence) bool {
		message := notificationSignalMessage(occurrence.GetSignal())
		return message != nil && message.GetRoomId() == roomID
	})
}

func (m *NotificationOccurrenceModel) PurgeUser(ctx context.Context, userID string) (int, error) {
	occurrences, err := m.List(ctx, userID)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, occurrence := range occurrences {
		ok, err := m.Delete(ctx, userID, occurrence.GetId())
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
		}
	}
	return removed, nil
}

func (m *NotificationOccurrenceModel) removeMatching(ctx context.Context, match func(*corev1.NotificationOccurrence) bool) (int, error) {
	occurrences := m.projection.Projection().allOccurrences(m.now().UTC())
	removed := 0
	for _, occurrence := range occurrences {
		if !match(occurrence) {
			continue
		}
		ok, err := m.Delete(ctx, occurrence.GetRecipientId(), occurrence.GetId())
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
		}
	}
	return removed, nil
}

func (m *NotificationOccurrenceModel) alertDeliveryCurrent(ctx context.Context, expected *corev1.NotificationOccurrence) (bool, error) {
	if expected == nil {
		return false, nil
	}
	if err := m.projection.Projector().WaitForCurrent(ctx); err != nil {
		return false, err
	}
	current, err := m.Get(ctx, expected.GetRecipientId(), expected.GetId())
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return current.GetSourceEventId() == expected.GetSourceEventId() && NotificationAlertPending(current), nil
}

// NotificationAlertPending reports whether an occurrence still has unresolved
// interruptive delivery work.
func NotificationAlertPending(occurrence *corev1.NotificationOccurrence) bool {
	return occurrence != nil && occurrence.GetAlertExpiresAt() != nil && !occurrence.GetRead() && occurrence.AlertDelivered == nil
}

func (m *NotificationOccurrenceModel) completeAlertDelivery(ctx context.Context, occurrence *corev1.NotificationOccurrence, delivered bool) error {
	if occurrence == nil {
		return nil
	}
	current, err := m.Get(ctx, occurrence.GetRecipientId(), occurrence.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil || !NotificationAlertPending(current) {
		return err
	}
	now := m.now().UTC()
	event := &corev1.NotificationEvent{
		Id:             notificationLifecycleEventID("alert-resolved", occurrence.GetId()),
		RecipientId:    occurrence.GetRecipientId(),
		NotificationId: occurrence.GetId(),
		OccurredAt:     timestamppb.New(now),
		ExpiresAt:      occurrence.GetExpiresAt(),
		Event: &corev1.NotificationEvent_AlertResolved{AlertResolved: &corev1.NotificationAlertResolved{
			Delivered: delivered,
		}},
	}
	return m.appendAndWait(ctx, event)
}
