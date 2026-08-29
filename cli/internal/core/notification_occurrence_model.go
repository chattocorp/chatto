package core

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
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
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	notificationPhysicalDeleteRetry = time.Minute
	notificationLifecycleBatchSize  = 100
)

type CreateNotificationOccurrenceInput struct {
	RecipientID          string
	SourceEventID        string
	SourceCreated        time.Time
	ActorID              string
	Signal               *notificationv1.NotificationSignal
	Mode                 evtv1.NotificationDeliveryMode
	AttentionLevel       notificationv1.NotificationAttentionLevel
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
	if err := m.core.notificationBoundaries.waitReady(ctx); err != nil {
		return err
	}
	return m.projection.Projector().WaitForStartup(ctx)
}

func (m *NotificationOccurrenceModel) Resync(ctx context.Context) error {
	if err := m.core.notificationBoundaries.resync(ctx); err != nil {
		return err
	}
	return m.projection.Projector().WaitForCurrent(ctx)
}

func (m *NotificationOccurrenceModel) Run(ctx context.Context) error {
	if err := m.WaitReady(ctx); err != nil {
		return err
	}
	if _, err := m.reconcileCoveredUnread(ctx); err != nil && ctx.Err() == nil {
		m.logger.Warn("Notification read-boundary startup reconciliation will retry", "error", err)
		m.core.notificationBoundaries.requeueReadChanges(nil, true)
	}
	reconcileChangedBoundaries := func() {
		scopes, fullRepair := m.core.notificationBoundaries.takeReadChanges()
		if fullRepair || len(scopes) > 0 {
			if fullRepair {
				scopes = nil
			}
			if _, err := m.reconcileCoveredUnread(ctx, scopes...); err != nil && ctx.Err() == nil {
				m.logger.Warn("Notification read-boundary reconciliation will retry", "error", err)
				m.core.notificationBoundaries.requeueReadChanges(scopes, fullRepair)
			}
		}
	}
	runMaintenance := func() {
		now := m.now().UTC()
		expiredUsers := m.projection.Projection().pruneExpired(now)
		invalidations := make([]*notificationv1.NotificationOccurrence, 0, len(expiredUsers))
		for _, userID := range expiredUsers {
			invalidations = append(invalidations, &notificationv1.NotificationOccurrence{RecipientId: userID})
		}
		m.core.publishNotificationOccurrenceInvalidations(ctx, invalidations, false)
		m.cleanupDismissedSignals(ctx, now)
	}
	runMaintenance()
	ticker := time.NewTicker(notificationPhysicalDeleteRetry)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-m.core.notificationBoundaries.readChanges():
			reconcileChangedBoundaries()
		case <-ticker.C:
			reconcileChangedBoundaries()
			runMaintenance()
		}
	}
}

// reconcileCoveredUnread repairs the intentional two-store read handshake.
// A request records the durable RUNTIME_STATE boundary before it appends read
// lifecycle facts so late materialization cannot reintroduce covered work. If
// the process stops between those writes, every replica's startup/background
// pass idempotently completes the NOTIFICATIONS side of the handshake.
func (m *NotificationOccurrenceModel) reconcileCoveredUnread(ctx context.Context, scopes ...notificationReadBoundaryScope) (int, error) {
	type cachedBoundary struct {
		boundary notificationReadBoundary
		exists   bool
	}
	boundaries := make(map[notificationReadBoundaryScope]cachedBoundary, len(scopes))
	occurrences := make([]*notificationv1.NotificationOccurrence, 0)
	if scopes == nil {
		occurrences = m.projection.Projection().allOccurrences(m.now().UTC())
	} else {
		for _, scope := range scopes {
			occurrences = append(occurrences, m.projection.Projection().scopeOccurrences(scope, m.now().UTC())...)
		}
	}
	matches := make([]*notificationv1.NotificationOccurrence, 0)
	for _, occurrence := range occurrences {
		if occurrence.GetRead() || occurrence.GetSourceStreamSequence() == 0 {
			continue
		}
		message := notificationSignalMessage(occurrence.GetSignal())
		if message == nil {
			continue
		}
		scope := notificationReadBoundaryScope{
			userID: occurrence.GetRecipientId(), roomID: message.GetRoomId(), threadRootEventID: message.GetThreadRootEventId(),
		}
		cached, ok := boundaries[scope]
		if !ok {
			boundary, exists, err := m.notificationReadBoundary(ctx, scope.userID, scope.roomID, scope.threadRootEventID)
			if err != nil {
				return 0, err
			}
			cached = cachedBoundary{boundary: boundary, exists: exists}
			boundaries[scope] = cached
		}
		if cached.exists && m.occurrenceCoveredByBoundary(occurrence, cached.boundary) {
			matches = append(matches, occurrence)
		}
	}
	return m.markReadOccurrences(ctx, matches)
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
		m.cleaned[tombstone.signalSequence] = tombstone.expiresAt.Add(notificationPhysicalCleanupGrace)
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

func newNotificationMessageReference(roomID, eventID string) *notificationv1.NotificationMessageReference {
	return &notificationv1.NotificationMessageReference{RoomId: roomID, EventId: eventID}
}

func notificationSignalMessage(signal *notificationv1.NotificationSignal) *notificationv1.NotificationMessageReference {
	if signal == nil {
		return nil
	}
	switch payload := signal.GetKind().(type) {
	case *notificationv1.NotificationSignal_DirectMessageReceived:
		return payload.DirectMessageReceived.GetMessage()
	case *notificationv1.NotificationSignal_DirectMentionReceived:
		return payload.DirectMentionReceived.GetMessage()
	case *notificationv1.NotificationSignal_ReplyReceived:
		return payload.ReplyReceived.GetMessage()
	case *notificationv1.NotificationSignal_RoleMentionReceived:
		return payload.RoleMentionReceived.GetMessage()
	case *notificationv1.NotificationSignal_HereMentionReceived:
		return payload.HereMentionReceived.GetMessage()
	case *notificationv1.NotificationSignal_AllMentionReceived:
		return payload.AllMentionReceived.GetMessage()
	case *notificationv1.NotificationSignal_FollowedThreadActivity:
		return payload.FollowedThreadActivity.GetMessage()
	case *notificationv1.NotificationSignal_FollowedRoomActivity:
		return payload.FollowedRoomActivity.GetMessage()
	case *notificationv1.NotificationSignal_ReactionReceived:
		return payload.ReactionReceived.GetMessage()
	case *notificationv1.NotificationSignal_RoomMessageReceived:
		return payload.RoomMessageReceived.GetMessage()
	default:
		return nil
	}
}

// NotificationOccurrenceMessageReference returns the exact message target for
// a signal understood by this server version.
func NotificationOccurrenceMessageReference(occurrence *notificationv1.NotificationOccurrence) *notificationv1.NotificationMessageReference {
	if occurrence == nil {
		return nil
	}
	return notificationSignalMessage(occurrence.GetSignal())
}

func notificationSignalIdentity(signal *notificationv1.NotificationSignal) string {
	if signal == nil {
		return ""
	}
	switch signal.GetKind().(type) {
	case *notificationv1.NotificationSignal_DirectMessageReceived:
		return "direct_message_received"
	case *notificationv1.NotificationSignal_DirectMentionReceived:
		return "direct_mention_received"
	case *notificationv1.NotificationSignal_ReplyReceived:
		return "reply_received"
	case *notificationv1.NotificationSignal_RoleMentionReceived:
		return "role_mention_received"
	case *notificationv1.NotificationSignal_HereMentionReceived:
		return "here_mention_received"
	case *notificationv1.NotificationSignal_AllMentionReceived:
		return "all_mention_received"
	case *notificationv1.NotificationSignal_FollowedThreadActivity:
		return "followed_thread_activity"
	case *notificationv1.NotificationSignal_FollowedRoomActivity:
		return "followed_room_activity"
	case *notificationv1.NotificationSignal_ReactionReceived:
		return "reaction_received"
	case *notificationv1.NotificationSignal_RoomMessageReceived:
		return "room_message_received"
	default:
		return ""
	}
}

func NotificationOccurrenceHasUnsupportedSignal(occurrence *notificationv1.NotificationOccurrence) bool {
	signal := occurrence.GetSignal()
	return signal != nil && signal.GetKind() == nil && len(signal.ProtoReflect().GetUnknown()) > 0
}

func NotificationAlertDeadline(occurrence *notificationv1.NotificationOccurrence) time.Time {
	if occurrence == nil {
		return time.Time{}
	}
	if deadline := occurrence.GetAlertExpiresAt(); deadline != nil && deadline.IsValid() {
		return deadline.AsTime().UTC()
	}
	return time.Time{}
}

func (m *NotificationOccurrenceModel) appendAndWait(ctx context.Context, event *notificationv1.NotificationEvent) error {
	position, err := m.publisher.AppendEventually(ctx, event)
	if errors.Is(err, notificationstream.ErrExpiredEvent) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return m.projection.Projector().WaitFor(ctx, position)
}

func (m *NotificationOccurrenceModel) Create(ctx context.Context, input CreateNotificationOccurrenceInput) (*notificationv1.NotificationOccurrence, bool, error) {
	results, err := m.createMany(ctx, []CreateNotificationOccurrenceInput{input})
	if err != nil {
		return nil, false, err
	}
	return results[0].occurrence, results[0].created, nil
}

type createNotificationOccurrenceResult struct {
	occurrence *notificationv1.NotificationOccurrence
	created    bool
}

type pendingNotificationCreate struct {
	occurrence    *notificationv1.NotificationOccurrence
	event         *notificationv1.NotificationEvent
	resultIndexes []int
	skipRead      bool
}

type preparedNotificationCreateInput struct {
	resultIndex    int
	input          CreateNotificationOccurrenceInput
	notificationID string
	expiresAt      time.Time
}

// CreateMany materializes one source fact's complete recipient fanout with
// bounded atomic writes, one projection wait, and one live invalidation per
// recipient. It is intentionally internal to the ordered materializer.
func (m *NotificationOccurrenceModel) CreateMany(ctx context.Context, inputs []CreateNotificationOccurrenceInput) error {
	_, err := m.createMany(ctx, inputs)
	return err
}

func (m *NotificationOccurrenceModel) createMany(ctx context.Context, inputs []CreateNotificationOccurrenceInput) ([]createNotificationOccurrenceResult, error) {
	results := make([]createNotificationOccurrenceResult, len(inputs))
	active := false
	for _, input := range inputs {
		if err := validateNotificationCreateInput(input); err != nil {
			return nil, err
		}
		active = active || notificationModeProducesOccurrence(input.Mode)
	}
	if !active {
		return results, nil
	}
	if err := m.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}

	now := m.now().UTC()
	prepared := make([]preparedNotificationCreateInput, 0, len(inputs))
	refs := make([]notificationOccurrenceRef, 0, len(inputs))
	for i, input := range inputs {
		if !notificationModeProducesOccurrence(input.Mode) {
			continue
		}
		expiresAt := input.SourceCreated.UTC().Add(notificationTTL)
		if !expiresAt.After(now) {
			continue
		}
		notificationID := notificationOccurrenceID(input.RecipientID, input.SourceEventID, notificationSignalIdentity(input.Signal))
		prepared = append(prepared, preparedNotificationCreateInput{
			resultIndex: i, input: input, notificationID: notificationID, expiresAt: expiresAt,
		})
		refs = append(refs, notificationOccurrenceRef{recipientID: input.RecipientID, notificationID: notificationID})
	}
	initialStates := m.projection.Projection().occurrenceStates(refs, now)
	pendingByID := make(map[string]*pendingNotificationCreate)
	pending := make([]*pendingNotificationCreate, 0, len(prepared))
	existingReadCandidates := make([]*notificationv1.NotificationOccurrence, 0)
	for _, candidate := range prepared {
		input := candidate.input
		ref := notificationOccurrenceRef{recipientID: input.RecipientID, notificationID: candidate.notificationID}
		state := initialStates[ref]
		if state.tombstoned {
			continue
		}
		if state.occurrence != nil {
			results[candidate.resultIndex].occurrence = state.occurrence
			if !input.SkipReadLookup && !state.occurrence.GetRead() {
				existingReadCandidates = append(existingReadCandidates, state.occurrence)
			}
			continue
		}
		if duplicate := pendingByID[candidate.notificationID]; duplicate != nil {
			duplicate.resultIndexes = append(duplicate.resultIndexes, candidate.resultIndex)
			duplicate.skipRead = duplicate.skipRead && input.SkipReadLookup
			continue
		}

		var alertExpiresAt *timestamppb.Timestamp
		if input.Mode == evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION {
			alertExpiresAt = timestamppb.New(input.SourceCreated.UTC().Add(notificationAlertDeliveryTTL))
		}
		occurrence := &notificationv1.NotificationOccurrence{
			Id:                   candidate.notificationID,
			RecipientId:          input.RecipientID,
			SourceEventId:        input.SourceEventID,
			SourceCreatedAt:      timestamppb.New(input.SourceCreated.UTC()),
			ActorId:              input.ActorID,
			Signal:               proto.Clone(input.Signal).(*notificationv1.NotificationSignal),
			Read:                 input.InitiallyRead,
			ExpiresAt:            timestamppb.New(candidate.expiresAt),
			SourceStreamSequence: input.SourceStreamSequence,
			AttentionLevel:       input.AttentionLevel,
			AlertExpiresAt:       alertExpiresAt,
		}
		if !input.SkipReadLookup && !occurrence.GetRead() {
			covered, err := m.occurrenceCoveredByReadBoundary(ctx, occurrence)
			if err != nil {
				return nil, err
			}
			if covered {
				occurrence.Read = true
			}
		}
		item := &pendingNotificationCreate{
			occurrence:    occurrence,
			resultIndexes: []int{candidate.resultIndex},
			skipRead:      input.SkipReadLookup,
		}
		item.event = newNotificationSignalledLifecycleEvent(now, occurrence)
		pendingByID[candidate.notificationID] = item
		pending = append(pending, item)
	}
	if len(pending) == 0 {
		if err := m.reconcileOccurrenceReadBoundaries(ctx, existingReadCandidates); err != nil {
			return nil, err
		}
		return results, nil
	}

	eventsToAppend := make([]*notificationv1.NotificationEvent, len(pending))
	for i, item := range pending {
		eventsToAppend[i] = item.event
	}
	committed, appendErr := m.appendEventsAndWait(ctx, eventsToAppend)
	if appendErr != nil && len(committed) == 0 {
		return nil, appendErr
	}
	pendingRefs := make([]notificationOccurrenceRef, 0, len(pending))
	for _, item := range pending {
		pendingRefs = append(pendingRefs, notificationOccurrenceRef{
			recipientID: item.occurrence.GetRecipientId(), notificationID: item.occurrence.GetId(),
		})
	}
	storedStates := m.projection.Projection().occurrenceStates(pendingRefs, m.now().UTC())
	wasCreated := make([]bool, len(pending))
	for i, item := range pending {
		ref := notificationOccurrenceRef{recipientID: item.occurrence.GetRecipientId(), notificationID: item.occurrence.GetId()}
		if storedStates[ref].occurrence == nil {
			continue
		}
		wasCreated[i] = i < len(committed) && committed[i]
	}

	readCandidates := existingReadCandidates
	for _, item := range pending {
		if item.skipRead {
			continue
		}
		ref := notificationOccurrenceRef{recipientID: item.occurrence.GetRecipientId(), notificationID: item.occurrence.GetId()}
		stored := storedStates[ref].occurrence
		if stored == nil {
			continue
		}
		if !stored.GetRead() {
			readCandidates = append(readCandidates, stored)
		}
	}
	if err := m.reconcileOccurrenceReadBoundaries(ctx, readCandidates); err != nil {
		return nil, err
	}
	created := make([]*notificationv1.NotificationOccurrence, 0, len(pending))
	for i, item := range pending {
		ref := notificationOccurrenceRef{recipientID: item.occurrence.GetRecipientId(), notificationID: item.occurrence.GetId()}
		stored := storedStates[ref].occurrence
		if stored == nil {
			continue
		}
		for _, resultIndex := range item.resultIndexes {
			results[resultIndex] = createNotificationOccurrenceResult{occurrence: stored, created: wasCreated[i]}
		}
		if wasCreated[i] {
			created = append(created, stored)
		}
	}
	m.publishCreatedInvalidations(ctx, created)
	if appendErr != nil {
		return nil, appendErr
	}
	return results, nil
}

// reconcileOccurrenceReadBoundaries closes both the normal post-append race
// and the redelivery case where a signal was committed before a materializer
// stopped. Existing unread occurrences must be checked too: a boundary repair
// can run before the signal reaches this replica's projection and will not be
// woken again merely because the materializer later retries the source fact.
func (m *NotificationOccurrenceModel) reconcileOccurrenceReadBoundaries(ctx context.Context, occurrences []*notificationv1.NotificationOccurrence) error {
	covered := make([]*notificationv1.NotificationOccurrence, 0, len(occurrences))
	for _, occurrence := range occurrences {
		if occurrence == nil || occurrence.GetRead() {
			continue
		}
		isCovered, err := m.occurrenceCoveredByReadBoundary(ctx, occurrence)
		if err != nil {
			return err
		}
		if isCovered {
			covered = append(covered, occurrence)
		}
	}
	if _, err := m.markReadOccurrences(ctx, covered); err != nil {
		return err
	}
	for _, occurrence := range covered {
		occurrence.Read = true
		if occurrence.GetAlertExpiresAt() != nil && occurrence.AlertDelivered == nil {
			occurrence.AlertDelivered = proto.Bool(false)
		}
	}
	return nil
}

func validateNotificationCreateInput(input CreateNotificationOccurrenceInput) error {
	if strings.TrimSpace(input.RecipientID) == "" || strings.TrimSpace(input.SourceEventID) == "" || input.SourceCreated.IsZero() {
		return invalidArgument("recipient_id, source_event_id, and source_created_at are required")
	}
	message := notificationSignalMessage(input.Signal)
	if message == nil || message.GetRoomId() == "" || message.GetEventId() == "" || notificationSignalIdentity(input.Signal) == "" {
		return invalidArgument("a supported notification signal with an exact message is required")
	}
	if !notificationModeProducesOccurrence(input.Mode) {
		return nil
	}
	if input.AttentionLevel != notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT && input.AttentionLevel != notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT {
		return invalidArgument("a concrete notification attention level is required")
	}
	return nil
}

func newNotificationSignalledLifecycleEvent(now time.Time, occurrence *notificationv1.NotificationOccurrence) *notificationv1.NotificationEvent {
	return &notificationv1.NotificationEvent{
		Id:             notificationLifecycleEventID("signal", occurrence.GetId()),
		RecipientId:    occurrence.GetRecipientId(),
		NotificationId: occurrence.GetId(),
		OccurredAt:     timestamppb.New(now),
		ExpiresAt:      occurrence.GetExpiresAt(),
		Event: &notificationv1.NotificationEvent_Signalled{Signalled: &notificationv1.NotificationSignalled{
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
}

func (m *NotificationOccurrenceModel) publishCreatedInvalidations(ctx context.Context, occurrences []*notificationv1.NotificationOccurrence) {
	byRecipient := make(map[string]*notificationv1.NotificationOccurrence)
	for _, occurrence := range occurrences {
		if occurrence == nil || occurrence.GetRecipientId() == "" {
			continue
		}
		current := byRecipient[occurrence.GetRecipientId()]
		if current == nil || (!NotificationAlertPending(current) && NotificationAlertPending(occurrence)) {
			byRecipient[occurrence.GetRecipientId()] = occurrence
		}
	}
	invalidations := make([]*notificationv1.NotificationOccurrence, 0, len(byRecipient))
	for _, occurrence := range byRecipient {
		invalidations = append(invalidations, occurrence)
	}
	m.core.publishNotificationOccurrenceInvalidations(ctx, invalidations, true)
}

func (m *NotificationOccurrenceModel) Get(_ context.Context, userID, notificationID string) (*notificationv1.NotificationOccurrence, error) {
	occurrence, exists := m.projection.Projection().occurrence(userID, notificationID, m.now().UTC())
	if !exists {
		return nil, ErrNotFound
	}
	return occurrence, nil
}

func (m *NotificationOccurrenceModel) List(_ context.Context, userID string) ([]*notificationv1.NotificationOccurrence, error) {
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

func (m *NotificationOccurrenceModel) MarkRead(ctx context.Context, userID, notificationID string) (*notificationv1.NotificationOccurrence, error) {
	occurrence, err := m.Get(ctx, userID, notificationID)
	if err != nil {
		return nil, err
	}
	if occurrence.GetRead() {
		return occurrence, nil
	}
	if _, err := m.markReadOccurrences(ctx, []*notificationv1.NotificationOccurrence{occurrence}); err != nil {
		return nil, err
	}
	return m.Get(ctx, userID, notificationID)
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
	deleted, err := m.deleteOccurrences(ctx, []*notificationv1.NotificationOccurrence{occurrence})
	return deleted == 1, err
}

func (m *NotificationOccurrenceModel) DeleteMany(ctx context.Context, userID string, notificationIDs []string) (int, error) {
	seen := make(map[string]struct{}, len(notificationIDs))
	refs := make([]notificationOccurrenceRef, 0, len(notificationIDs))
	for _, id := range notificationIDs {
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		refs = append(refs, notificationOccurrenceRef{recipientID: userID, notificationID: id})
	}
	states := m.projection.Projection().occurrenceStates(refs, m.now().UTC())
	occurrences := make([]*notificationv1.NotificationOccurrence, 0, len(refs))
	for _, ref := range refs {
		if occurrence := states[ref].occurrence; occurrence != nil {
			occurrences = append(occurrences, occurrence)
		}
	}
	return m.deleteOccurrences(ctx, occurrences)
}

// deleteOccurrences appends exact removal facts, waits once through the last
// committed position, emits one replacement invalidation per recipient, and
// performs one secure-delete sweep. This keeps bulk privacy cleanup linear in
// the number of occurrences instead of repeatedly rescanning all tombstones.
func (m *NotificationOccurrenceModel) deleteOccurrences(ctx context.Context, occurrences []*notificationv1.NotificationOccurrence) (int, error) {
	now := m.now().UTC()
	seen := make(map[string]struct{}, len(occurrences))
	unique := make([]*notificationv1.NotificationOccurrence, 0, len(occurrences))
	eventsToAppend := make([]*notificationv1.NotificationEvent, 0, len(occurrences))
	for _, occurrence := range occurrences {
		if occurrence == nil || occurrence.GetRecipientId() == "" || occurrence.GetId() == "" {
			continue
		}
		key := occurrence.GetRecipientId() + "\x00" + occurrence.GetId()
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, occurrence)
		eventsToAppend = append(eventsToAppend, &notificationv1.NotificationEvent{
			Id:             notificationLifecycleEventID("remove", occurrence.GetId()),
			RecipientId:    occurrence.GetRecipientId(),
			NotificationId: occurrence.GetId(),
			OccurredAt:     timestamppb.New(now),
			ExpiresAt:      occurrence.GetExpiresAt(),
			Event: &notificationv1.NotificationEvent_Removed{Removed: &notificationv1.NotificationRemoved{
				SignalStreamSequence: occurrence.GetNotificationStreamSequence(),
			}},
		})
	}
	committed, err := m.appendEventsAndWait(ctx, eventsToAppend)
	changed := committedOccurrences(unique, committed)
	m.publishNotificationInvalidations(ctx, changed)
	m.cleanupDismissedSignals(ctx, now)
	return len(changed), err
}

func (m *NotificationOccurrenceModel) markReadOccurrences(ctx context.Context, occurrences []*notificationv1.NotificationOccurrence) (int, error) {
	now := m.now().UTC()
	unique := make([]*notificationv1.NotificationOccurrence, 0, len(occurrences))
	seen := make(map[string]struct{}, len(occurrences))
	eventsToAppend := make([]*notificationv1.NotificationEvent, 0, len(occurrences))
	for _, occurrence := range occurrences {
		if occurrence == nil || occurrence.GetRead() || occurrence.GetRecipientId() == "" || occurrence.GetId() == "" {
			continue
		}
		key := occurrence.GetRecipientId() + "\x00" + occurrence.GetId()
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, occurrence)
		eventsToAppend = append(eventsToAppend, &notificationv1.NotificationEvent{
			Id:             notificationLifecycleEventID("read", occurrence.GetId()),
			RecipientId:    occurrence.GetRecipientId(),
			NotificationId: occurrence.GetId(),
			OccurredAt:     timestamppb.New(now),
			ExpiresAt:      occurrence.GetExpiresAt(),
			Event:          &notificationv1.NotificationEvent_Read{Read: &notificationv1.NotificationRead{}},
		})
	}
	committed, err := m.appendEventsAndWait(ctx, eventsToAppend)
	changed := committedOccurrences(unique, committed)
	m.publishNotificationInvalidations(ctx, changed)
	return len(changed), err
}

func committedOccurrences(occurrences []*notificationv1.NotificationOccurrence, committed []bool) []*notificationv1.NotificationOccurrence {
	changed := make([]*notificationv1.NotificationOccurrence, 0, len(committed))
	for i, wasCommitted := range committed {
		if wasCommitted && i < len(occurrences) {
			changed = append(changed, occurrences[i])
		}
	}
	return changed
}

func (m *NotificationOccurrenceModel) appendEventsAndWait(ctx context.Context, lifecycleEvents []*notificationv1.NotificationEvent) ([]bool, error) {
	committed := make([]bool, 0, len(lifecycleEvents))
	var lastPosition events.StreamPosition
	var appendErr error
	for start := 0; start < len(lifecycleEvents); start += notificationLifecycleBatchSize {
		end := min(start+notificationLifecycleBatchSize, len(lifecycleEvents))
		results, err := m.publisher.AppendBatchEventuallyResults(ctx, lifecycleEvents[start:end])
		for _, result := range results {
			committed = append(committed, result.Committed)
			if result.Position.Seq > lastPosition.Seq {
				lastPosition = result.Position
			}
		}
		if err != nil {
			appendErr = err
			break
		}
	}
	if !lastPosition.IsZero() {
		if err := m.projection.Projector().WaitFor(ctx, lastPosition); err != nil {
			return committed, err
		}
	}
	return committed, appendErr
}

func (m *NotificationOccurrenceModel) publishNotificationInvalidations(ctx context.Context, occurrences []*notificationv1.NotificationOccurrence) {
	byRecipient := make(map[string]*notificationv1.NotificationOccurrence)
	for _, occurrence := range occurrences {
		if occurrence != nil && occurrence.GetRecipientId() != "" {
			byRecipient[occurrence.GetRecipientId()] = occurrence
		}
	}
	invalidations := make([]*notificationv1.NotificationOccurrence, 0, len(byRecipient))
	for _, occurrence := range byRecipient {
		invalidations = append(invalidations, occurrence)
	}
	m.core.publishNotificationOccurrenceInvalidations(ctx, invalidations, false)
}

func (m *NotificationOccurrenceModel) MarkCoveredRead(ctx context.Context, userID, roomID, threadRootEventID, targetEventID string) (int, error) {
	occurrences, err := m.List(ctx, userID)
	if err != nil {
		return 0, err
	}
	boundary, err := m.recordNotificationReadBoundary(ctx, userID, roomID, threadRootEventID, targetEventID)
	if err != nil {
		return 0, err
	}
	matches := make([]*notificationv1.NotificationOccurrence, 0)
	for _, occurrence := range occurrences {
		message := notificationSignalMessage(occurrence.GetSignal())
		if message == nil || message.GetRoomId() != roomID || message.GetThreadRootEventId() != threadRootEventID || occurrence.GetRead() {
			continue
		}
		if !m.occurrenceCoveredByBoundary(occurrence, boundary) {
			continue
		}
		matches = append(matches, occurrence)
	}
	if _, err := m.markReadOccurrences(ctx, matches); err != nil {
		return 0, err
	}
	// The watched-boundary repair may win the idempotent read append race after
	// recordNotificationReadBoundary returns. Report the request's snapshotted
	// affected set so its caller still emits the corresponding room-read reset.
	return len(matches), nil
}

func (m *NotificationOccurrenceModel) VisibleOccurrences(ctx context.Context, recipientID string, occurrences []*notificationv1.NotificationOccurrence) ([]*notificationv1.NotificationOccurrence, error) {
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
	visible := make([]*notificationv1.NotificationOccurrence, 0, len(occurrences))
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

func (m *NotificationOccurrenceModel) targetVisibleFromCurrentProjections(ctx context.Context, recipientID string, occurrence *notificationv1.NotificationOccurrence) (bool, error) {
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
	entry, ok := m.core.roomModel.timelineEntry(message.GetEventId())
	if !ok || entry == nil || entry.Event == nil {
		return false, nil
	}
	messagePosition := events.SubjectPosition(evtstream.RoomAggregate(room.GetId()).SubjectFor(entry.Event), entry.StreamSeq)
	if err := m.core.roomModel.waitForThreads(ctx, messagePosition); err != nil {
		return false, fmt.Errorf("wait for notification message relationship: %w", err)
	}
	allowed, err := m.core.CanReadMessage(ctx, recipientID, KindOfRoom(room), room.GetId(), message.GetEventId())
	if err != nil || !allowed {
		return allowed, err
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
	return m.removeMatching(ctx, func(occurrence *notificationv1.NotificationOccurrence) bool {
		message := notificationSignalMessage(occurrence.GetSignal())
		return message != nil && message.GetRoomId() == roomID && (message.GetEventId() == eventID || message.GetThreadRootEventId() == eventID)
	})
}

func (m *NotificationOccurrenceModel) RemoveReaction(ctx context.Context, recipientID, roomID, messageEventID, actorID, emoji string, removedAtSequence uint64) (int, error) {
	occurrences, err := m.List(ctx, recipientID)
	if err != nil {
		return 0, err
	}
	matches := make([]*notificationv1.NotificationOccurrence, 0)
	for _, occurrence := range occurrences {
		message := notificationSignalMessage(occurrence.GetSignal())
		reaction := occurrence.GetSignal().GetReactionReceived()
		if message == nil || reaction == nil || occurrence.GetActorId() != actorID || reaction.GetEmoji() != emoji || message.GetRoomId() != roomID || message.GetEventId() != messageEventID || occurrence.GetSourceStreamSequence() >= removedAtSequence {
			continue
		}
		matches = append(matches, occurrence)
	}
	return m.deleteOccurrences(ctx, matches)
}

func (m *NotificationOccurrenceModel) RemoveRoomForUser(ctx context.Context, userID, roomID string, removedThroughSequence uint64) (int, error) {
	occurrences, err := m.List(ctx, userID)
	if err != nil {
		return 0, err
	}
	matches := make([]*notificationv1.NotificationOccurrence, 0)
	for _, occurrence := range occurrences {
		message := notificationSignalMessage(occurrence.GetSignal())
		if message == nil || message.GetRoomId() != roomID || (removedThroughSequence != 0 && occurrence.GetSourceStreamSequence() >= removedThroughSequence) {
			continue
		}
		matches = append(matches, occurrence)
	}
	return m.deleteOccurrences(ctx, matches)
}

func (m *NotificationOccurrenceModel) RemoveRoom(ctx context.Context, roomID string) (int, error) {
	return m.removeMatching(ctx, func(occurrence *notificationv1.NotificationOccurrence) bool {
		message := notificationSignalMessage(occurrence.GetSignal())
		return message != nil && message.GetRoomId() == roomID
	})
}

func (m *NotificationOccurrenceModel) PurgeUser(ctx context.Context, userID string) (int, error) {
	occurrences, err := m.List(ctx, userID)
	if err != nil {
		return 0, err
	}
	return m.deleteOccurrences(ctx, occurrences)
}

func (m *NotificationOccurrenceModel) removeMatching(ctx context.Context, match func(*notificationv1.NotificationOccurrence) bool) (int, error) {
	occurrences := m.projection.Projection().allOccurrences(m.now().UTC())
	matches := make([]*notificationv1.NotificationOccurrence, 0)
	for _, occurrence := range occurrences {
		if !match(occurrence) {
			continue
		}
		matches = append(matches, occurrence)
	}
	return m.deleteOccurrences(ctx, matches)
}

func (m *NotificationOccurrenceModel) deliveryCurrent(ctx context.Context, expected *notificationv1.NotificationOccurrence) (*notificationv1.NotificationOccurrence, error) {
	if expected == nil {
		return nil, nil
	}
	if err := m.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	current, err := m.Get(ctx, expected.GetRecipientId(), expected.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if current.GetSourceEventId() != expected.GetSourceEventId() {
		return nil, nil
	}
	return current, nil

}

func (m *NotificationOccurrenceModel) alertDeliveryCurrent(ctx context.Context, expected *notificationv1.NotificationOccurrence) (bool, error) {
	current, err := m.deliveryCurrent(ctx, expected)
	return NotificationAlertPending(current), err
}

// NotificationAlertPending reports whether an occurrence still has unresolved
// interruptive delivery work.
func NotificationAlertPending(occurrence *notificationv1.NotificationOccurrence) bool {
	return occurrence != nil && occurrence.GetAlertExpiresAt() != nil && !occurrence.GetRead() && occurrence.AlertDelivered == nil
}

func (m *NotificationOccurrenceModel) completeAlertDelivery(ctx context.Context, occurrence *notificationv1.NotificationOccurrence, delivered bool) error {
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
	event := &notificationv1.NotificationEvent{
		Id:             notificationLifecycleEventID("alert-resolved", occurrence.GetId()),
		RecipientId:    occurrence.GetRecipientId(),
		NotificationId: occurrence.GetId(),
		OccurredAt:     timestamppb.New(now),
		ExpiresAt:      occurrence.GetExpiresAt(),
		Event: &notificationv1.NotificationEvent_AlertResolved{AlertResolved: &notificationv1.NotificationAlertResolved{
			Delivered: delivered,
		}},
	}
	return m.appendAndWait(ctx, event)
}
