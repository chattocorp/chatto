package core

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/jetstreamutil"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	notificationOccurrenceKeyPrefix = "notification_v2."
	maxNotificationUpdateRetries    = 8
)

type CreateNotificationOccurrenceInput struct {
	RecipientID          string
	SourceEventID        string
	SourceCreated        time.Time
	ActorID              string
	Target               *corev1.NotificationTarget
	Reasons              []*corev1.NotificationReasonMatch
	ReactionEmoji        string
	SourceStreamSequence uint64
	EvaluatedAt          time.Time
	InitialState         corev1.NotificationInboxState
	SkipReadLookup       bool
}

// WaitCurrent waits until the sole durable occurrence/lifecycle writer has
// processed every notification-relevant EVT fact visible at a captured
// boundary. Exhaustive occurrence lists and counts should read after this fence.
func (m *NotificationOccurrenceModel) WaitCurrent(ctx context.Context) error {
	return m.core.notificationMaterializer.WaitCurrent(ctx)
}

// NotificationOccurrenceModel owns the versioned occurrence keyspace, its
// process-wide watcher index, and every recipient triage mutation.
type NotificationOccurrenceModel struct {
	core   *ChattoCore
	kv     jetstream.KeyValue
	index  *NotificationOccurrenceIndex
	logger *log.Logger
	now    func() time.Time
}

func NewNotificationOccurrenceModel(core *ChattoCore, kv jetstream.KeyValue, logger *log.Logger) *NotificationOccurrenceModel {
	return &NotificationOccurrenceModel{
		core:   core,
		kv:     kv,
		index:  NewNotificationOccurrenceIndex(kv, logger.WithPrefix("Index")),
		logger: logger,
		now:    time.Now,
	}
}

func (c *ChattoCore) NotificationOccurrences() *NotificationOccurrenceModel {
	return c.notificationOccurrences
}

func (m *NotificationOccurrenceModel) Run(ctx context.Context) error {
	return m.index.Run(ctx)
}

func (m *NotificationOccurrenceModel) WaitReady(ctx context.Context) error {
	return m.index.WaitReady(ctx)
}

func (m *NotificationOccurrenceModel) Resync(ctx context.Context) error {
	return m.index.Resync(ctx)
}

// WaitForSourceRevision fences a local index before a realtime replacement is
// assembled on a replica other than the one that wrote the occurrence.
func (m *NotificationOccurrenceModel) WaitForSourceRevision(ctx context.Context, recipientID, sourceEventID string, revision uint64) error {
	if revision == 0 || recipientID == "" || sourceEventID == "" {
		return nil
	}
	return m.index.waitForRevision(ctx, notificationOccurrenceKey(recipientID, sourceEventID), revision)
}

func notificationOccurrenceKey(recipientID, sourceEventID string) string {
	return notificationOccurrenceKeyPrefix + recipientID + "." + sourceEventID
}

func notificationOccurrenceID(recipientID, sourceEventID string) string {
	digest := sha256.Sum256([]byte(recipientID + "\x00" + sourceEventID))
	return "ntf_" + base64.RawURLEncoding.EncodeToString(digest[:20])
}

func (m *NotificationOccurrenceModel) Create(ctx context.Context, input CreateNotificationOccurrenceInput) (*corev1.NotificationOccurrence, bool, error) {
	if strings.TrimSpace(input.RecipientID) == "" || strings.TrimSpace(input.SourceEventID) == "" {
		return nil, false, invalidArgument("recipient_id and source_event_id are required")
	}
	if input.Target == nil || input.Target.GetRoomId() == "" || input.Target.GetEventId() == "" {
		return nil, false, invalidArgument("notification target room_id and event_id are required")
	}
	if input.SourceCreated.IsZero() {
		return nil, false, invalidArgument("source_created_at is required")
	}

	reasons := normalizeNotificationReasons(input.Reasons)
	strongest := strongestNotificationIntensity(reasons)
	if strongest == corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF ||
		strongest == corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED {
		return nil, false, nil
	}

	now := m.now().UTC()
	expiresAt := input.SourceCreated.UTC().Add(notificationTTL)
	remaining := expiresAt.Sub(now)
	if remaining <= 0 {
		return nil, false, nil
	}
	evaluatedAt := input.EvaluatedAt.UTC()
	if evaluatedAt.IsZero() {
		evaluatedAt = now
	}
	state := input.InitialState
	if state == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNSPECIFIED {
		state = corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD
	}
	alertState := corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_NOT_APPLICABLE
	if strongest == corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT {
		// UNSPECIFIED is a persisted initialization fence. Alert claimers ignore
		// it until the authoritative read-boundary check below finalizes the row.
		alertState = corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_UNSPECIFIED
	}
	occurrence := &corev1.NotificationOccurrence{
		Id:                   notificationOccurrenceID(input.RecipientID, input.SourceEventID),
		RecipientId:          input.RecipientID,
		SourceEventId:        input.SourceEventID,
		SourceCreatedAt:      timestamppb.New(input.SourceCreated.UTC()),
		ActorId:              input.ActorID,
		Target:               proto.Clone(input.Target).(*corev1.NotificationTarget),
		Reasons:              reasons,
		ReactionEmoji:        input.ReactionEmoji,
		SourceStreamSequence: input.SourceStreamSequence,
		StrongestIntensity:   strongest,
		AttentionLevel:       notificationAttentionLevelForReasons(reasons),
		InboxState:           state,
		EvaluatedAt:          timestamppb.New(evaluatedAt),
		ExpiresAt:            timestamppb.New(expiresAt),
		AlertState:           alertState,
	}
	data, err := proto.Marshal(occurrence)
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification occurrence: %w", err)
	}
	key := notificationOccurrenceKey(input.RecipientID, input.SourceEventID)
	revision, err := m.kv.Create(ctx, key, data, jetstream.KeyTTL(remaining))
	if jetstreamutil.IsSequenceConflict(err) {
		existing, exists, readErr := m.storedOccurrenceBySource(ctx, input.RecipientID, input.SourceEventID)
		if readErr != nil || !exists {
			return nil, false, readErr
		}
		if existing.occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
			return nil, false, nil
		}
		existingOccurrence, changed, reconcileErr := m.finalizeOccurrence(ctx, existing.occurrence, input.SkipReadLookup)
		if changed {
			m.core.publishNotificationOccurrenceChanged(ctx, existingOccurrence, false, false)
		}
		return existingOccurrence, false, reconcileErr
	}
	if err != nil {
		return nil, false, fmt.Errorf("create notification occurrence: %w", err)
	}
	if err := m.index.waitForRevision(ctx, key, revision); err != nil {
		return nil, false, fmt.Errorf("wait for notification occurrence: %w", err)
	}
	occurrence, _, err = m.finalizeOccurrence(ctx, occurrence, input.SkipReadLookup)
	if err != nil {
		return nil, true, fmt.Errorf("finalize created notification occurrence: %w", err)
	}
	m.logger.Debug("Notification occurrence created",
		"notification_id", occurrence.GetId(),
		"recipient_id", input.RecipientID,
		"source_event_id", input.SourceEventID,
		"intensity", strongest.String(),
	)
	m.core.publishNotificationOccurrenceChanged(ctx, occurrence, true, false)
	return proto.Clone(occurrence).(*corev1.NotificationOccurrence), true, nil
}

// NotificationOccurrenceAttentionLevel returns the stored source-time visual
// importance, deriving it from retained reasons for records written before the
// additive attention field existed. Unknown future causes fail toward visible
// importance instead of silently suppressing their orange indicator.
func NotificationOccurrenceAttentionLevel(occurrence *corev1.NotificationOccurrence) corev1.NotificationAttentionLevel {
	if occurrence == nil {
		return corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_UNSPECIFIED
	}
	level := occurrence.GetAttentionLevel()
	if level == corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT ||
		level == corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT {
		return level
	}
	return notificationAttentionLevelForReasons(occurrence.GetReasons())
}

func notificationAttentionLevelForReasons(reasons []*corev1.NotificationReasonMatch) corev1.NotificationAttentionLevel {
	level := corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_UNSPECIFIED
	for _, match := range reasons {
		if match == nil || match.GetReason() == corev1.NotificationReason_NOTIFICATION_REASON_UNSPECIFIED {
			continue
		}
		if match.GetReason() != corev1.NotificationReason_NOTIFICATION_REASON_REACTION {
			return corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
		}
		level = corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT
	}
	return level
}

// finalizeOccurrence closes the cross-replica race between a read action and
// occurrence creation. UpdatedAt is intentionally absent on the initial KV
// row, so a durable redelivery may finish interrupted initialization without
// reapplying read state after initialization has completed.
func (m *NotificationOccurrenceModel) finalizeOccurrence(ctx context.Context, occurrence *corev1.NotificationOccurrence, skipReadLookup bool) (*corev1.NotificationOccurrence, bool, error) {
	if occurrence == nil {
		return nil, false, nil
	}
	for attempt := 0; attempt < maxNotificationUpdateRetries; attempt++ {
		entry, exists, err := m.storedOccurrenceBySource(ctx, occurrence.GetRecipientId(), occurrence.GetSourceEventId())
		if err != nil || !exists {
			return nil, false, err
		}
		if entry.occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED ||
			entry.occurrence.GetUpdatedAt() != nil {
			return entry.occurrence, false, nil
		}
		updated := proto.Clone(entry.occurrence).(*corev1.NotificationOccurrence)
		if !skipReadLookup && updated.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD {
			covered, err := m.occurrenceCoveredByReadBoundary(ctx, updated)
			if err != nil {
				return nil, false, err
			}
			if covered {
				updated.InboxState = corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ
			}
		}
		if updated.GetStrongestIntensity() == corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT {
			updated.AlertState = corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING
			if updated.GetInboxState() != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD {
				updated.AlertState = corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED
			}
		}
		updated.UpdatedAt = timestamppb.New(m.now().UTC())
		written, err := m.updateAtRevision(ctx, entry, updated)
		if jetstreamutil.IsSequenceConflict(err) {
			continue
		}
		return written, err == nil, err
	}
	return nil, false, fmt.Errorf("notification occurrence finalization failed after %d retries", maxNotificationUpdateRetries)
}

func (m *NotificationOccurrenceModel) Get(ctx context.Context, userID, occurrenceID string) (*corev1.NotificationOccurrence, error) {
	entry, exists, err := m.index.occurrenceByID(ctx, userID, occurrenceID)
	if err != nil {
		return nil, err
	}
	if !exists || entry.occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
		return nil, ErrNotFound
	}
	return entry.occurrence, nil
}

func (m *NotificationOccurrenceModel) List(ctx context.Context, userID string) ([]*corev1.NotificationOccurrence, error) {
	entries, err := m.index.userEntries(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]*corev1.NotificationOccurrence, 0, len(entries))
	for _, entry := range entries {
		occurrence := entry.occurrence
		if occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
			continue
		}
		if occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD ||
			occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ ||
			occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_DONE {
			result = append(result, occurrence)
		}
	}
	sort.Slice(result, func(a, b int) bool {
		return result[a].GetSourceCreatedAt().AsTime().After(result[b].GetSourceCreatedAt().AsTime())
	})
	return result, nil
}

// UnreadCount returns the exact number of unread occurrences. Presentation
// grouping must never change bell, room, or installed-app badge counts.
func (m *NotificationOccurrenceModel) UnreadCount(ctx context.Context, userID string) (int, error) {
	occurrences, err := m.List(ctx, userID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, occurrence := range occurrences {
		if occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD {
			count++
		}
	}
	return count, nil
}

// alertDeliveryCurrent revalidates the exact pending occurrence immediately
// before an external provider side effect.
func (m *NotificationOccurrenceModel) alertDeliveryCurrent(ctx context.Context, expected *corev1.NotificationOccurrence) (bool, error) {
	if expected == nil {
		return false, nil
	}
	entry, exists, err := m.index.occurrenceByID(ctx, expected.GetRecipientId(), expected.GetId())
	if err != nil || !exists {
		return false, err
	}
	return entry.occurrence.GetSourceEventId() == expected.GetSourceEventId() &&
		entry.occurrence.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING &&
		entry.occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD &&
		entry.occurrence.GetRemovalReason() == corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED, nil
}

// completeAlertDelivery records a queue job's terminal outcome if the exact
// occurrence is still pending. Concurrent read/delete mutations win cleanly.
func (m *NotificationOccurrenceModel) completeAlertDelivery(ctx context.Context, job *corev1.NotificationAlertJob, state corev1.NotificationAlertState) error {
	if job == nil || (state != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_DELIVERED &&
		state != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED) {
		return nil
	}
	for attempt := 0; attempt < maxNotificationUpdateRetries; attempt++ {
		entry, exists, err := m.index.occurrenceByID(ctx, job.GetRecipientId(), job.GetNotificationId())
		if err != nil || !exists {
			return err
		}
		if entry.occurrence.GetSourceEventId() != job.GetSourceEventId() ||
			entry.occurrence.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING {
			return nil
		}
		updated := proto.Clone(entry.occurrence).(*corev1.NotificationOccurrence)
		updated.AlertState = state
		updated.AlertClaimedUntil = nil
		updated.UpdatedAt = timestamppb.New(m.now().UTC())
		_, err = m.updateAtRevision(ctx, entry, updated)
		if jetstreamutil.IsSequenceConflict(err) {
			if waitErr := m.index.waitForRevisionAfter(ctx, entry.key, entry.revision); waitErr != nil {
				return waitErr
			}
			continue
		}
		return err
	}
	return fmt.Errorf("complete notification alert delivery after %d attempts", maxNotificationUpdateRetries)
}

func (m *NotificationOccurrenceModel) MarkRead(ctx context.Context, userID, occurrenceID string) (*corev1.NotificationOccurrence, error) {
	for attempt := 0; attempt < maxNotificationUpdateRetries; attempt++ {
		entry, exists, err := m.index.occurrenceByID(ctx, userID, occurrenceID)
		if err != nil {
			return nil, err
		}
		if !exists || entry.occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
			return nil, ErrNotFound
		}
		updated := proto.Clone(entry.occurrence).(*corev1.NotificationOccurrence)
		updated.InboxState = corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ
		if updated.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING ||
			updated.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_CLAIMED {
			updated.AlertState = corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED
			updated.AlertClaimedUntil = nil
		}
		if proto.Equal(updated, entry.occurrence) {
			return updated, nil
		}
		updated.UpdatedAt = timestamppb.New(m.now().UTC())
		written, err := m.updateAtRevision(ctx, entry, updated)
		if jetstreamutil.IsSequenceConflict(err) {
			if waitErr := m.index.waitForRevisionAfter(ctx, entry.key, entry.revision); waitErr != nil {
				return nil, waitErr
			}
			continue
		}
		if err == nil {
			m.core.publishNotificationOccurrenceChanged(ctx, written, false, false)
		}
		return written, err
	}
	return nil, fmt.Errorf("notification occurrence update failed after %d retries", maxNotificationUpdateRetries)
}

func (m *NotificationOccurrenceModel) Delete(ctx context.Context, userID, occurrenceID string, reason corev1.NotificationRemovalReason) (bool, error) {
	return m.delete(ctx, userID, occurrenceID, reason, true)
}

func (m *NotificationOccurrenceModel) delete(ctx context.Context, userID, occurrenceID string, reason corev1.NotificationRemovalReason, publish bool) (bool, error) {
	if reason == corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
		reason = corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_DELETED
	}
	for attempt := 0; attempt < maxNotificationUpdateRetries; attempt++ {
		entry, exists, err := m.index.occurrenceByID(ctx, userID, occurrenceID)
		if err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}
		if entry.occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
			return false, nil
		}
		now := m.now().UTC()
		tombstone := &corev1.NotificationOccurrence{
			Id:              entry.occurrence.GetId(),
			RecipientId:     entry.occurrence.GetRecipientId(),
			SourceEventId:   entry.occurrence.GetSourceEventId(),
			SourceCreatedAt: entry.occurrence.GetSourceCreatedAt(),
			InboxState:      corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ,
			UpdatedAt:       timestamppb.New(now),
			ExpiresAt:       entry.occurrence.GetExpiresAt(),
			RemovalReason:   reason,
			RemovedAt:       timestamppb.New(now),
			AlertState:      corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_NOT_APPLICABLE,
		}
		written, err := m.updateAtRevision(ctx, entry, tombstone)
		if jetstreamutil.IsSequenceConflict(err) {
			if waitErr := m.index.waitForRevisionAfter(ctx, entry.key, entry.revision); waitErr != nil {
				return false, waitErr
			}
			continue
		}
		if err == nil && publish {
			m.core.publishNotificationOccurrenceChanged(ctx, written, false, true)
		}
		return err == nil, err
	}
	return false, fmt.Errorf("notification occurrence delete failed after %d retries", maxNotificationUpdateRetries)
}

// DeleteMany replaces the exact requested occurrences with anti-recreation
// tombstones. Repeating the same batch is safe because later activity receives
// different occurrence IDs.
func (m *NotificationOccurrenceModel) DeleteMany(ctx context.Context, userID string, occurrenceIDs []string) (int, error) {
	deleted := 0
	seen := make(map[string]struct{}, len(occurrenceIDs))
	var lastDeleted *corev1.NotificationOccurrence
	for _, occurrenceID := range occurrenceIDs {
		if occurrenceID == "" {
			continue
		}
		if _, duplicate := seen[occurrenceID]; duplicate {
			continue
		}
		seen[occurrenceID] = struct{}{}
		occurrence, getErr := m.Get(ctx, userID, occurrenceID)
		if errors.Is(getErr, ErrNotFound) {
			continue
		}
		if getErr != nil {
			return deleted, getErr
		}
		ok, err := m.delete(ctx, userID, occurrenceID, corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_DELETED, false)
		if err != nil {
			if lastDeleted != nil {
				m.core.publishNotificationOccurrenceChanged(ctx, lastDeleted, false, true)
			}
			return deleted, err
		}
		if ok {
			deleted++
			lastDeleted = occurrence
		}
	}
	if lastDeleted != nil {
		m.core.publishNotificationOccurrenceChanged(ctx, lastDeleted, false, true)
	}
	return deleted, nil
}

func (m *NotificationOccurrenceModel) MarkCoveredRead(ctx context.Context, userID, roomID, threadRootEventID, targetEventID string) (int, error) {
	if _, err := m.recordNotificationReadBoundary(ctx, userID, roomID, threadRootEventID, targetEventID); err != nil {
		return 0, err
	}
	// This authoritative scan pairs with Create's post-write boundary read. No
	// matter which cross-key write wins, one side observes and reconciles the
	// other without relying on replica-local watcher timing.
	entries, err := m.storedOccurrenceEntries(ctx, userID)
	if err != nil {
		return 0, err
	}
	updated := 0
	var lastUpdated *corev1.NotificationOccurrence
	for _, entry := range entries {
		occurrence := entry.occurrence
		if occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED ||
			occurrence.GetInboxState() != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD ||
			occurrence.GetTarget().GetRoomId() != roomID ||
			occurrence.GetTarget().GetThreadRootEventId() != threadRootEventID {
			continue
		}
		covered, err := m.occurrenceCoveredByReadBoundary(ctx, occurrence)
		if err != nil {
			return updated, err
		}
		if !covered {
			continue
		}
		var item *corev1.NotificationOccurrence
		for attempt := 0; attempt < maxNotificationUpdateRetries; attempt++ {
			current, exists, err := m.storedOccurrenceBySource(ctx, userID, occurrence.GetSourceEventId())
			if err != nil || !exists {
				if err != nil {
					return updated, err
				}
				break
			}
			if current.occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED ||
				current.occurrence.GetInboxState() != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD {
				break
			}
			covered, err := m.occurrenceCoveredByReadBoundary(ctx, current.occurrence)
			if err != nil {
				return updated, err
			}
			if !covered {
				break
			}
			next := proto.Clone(current.occurrence).(*corev1.NotificationOccurrence)
			next.InboxState = corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ
			if next.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING ||
				next.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_CLAIMED ||
				next.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_UNSPECIFIED {
				next.AlertState = corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED
				next.AlertClaimedUntil = nil
			}
			next.UpdatedAt = timestamppb.New(m.now().UTC())
			item, err = m.updateAtRevision(ctx, current, next)
			if jetstreamutil.IsSequenceConflict(err) {
				continue
			}
			if err != nil {
				return updated, err
			}
			break
		}
		if item == nil {
			continue
		}
		updated++
		lastUpdated = item
	}
	if lastUpdated != nil {
		m.core.publishNotificationOccurrenceChanged(ctx, lastUpdated, false, false)
	}
	return updated, nil
}

// VisibleOccurrences waits this replica's authoritative projections through a
// freshly captured user, room, group-layout, and RBAC boundaries, then returns the occurrences whose
// recipient, membership, target-message lifecycle, and exact reaction remain
// visible. One room-subject boundary covers the whole batch, preventing both
// per-room broker work and projection lag from being mistaken for permanent
// visibility loss.
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
	if occurrence == nil || occurrence.GetRecipientId() != recipientID || occurrence.GetTarget().GetRoomId() == "" {
		return false, nil
	}
	if _, err := m.core.GetUser(ctx, recipientID); errors.Is(err, ErrNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	room, err := m.core.FindRoomByID(ctx, occurrence.GetTarget().GetRoomId())
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
	target := occurrence.GetTarget()
	messageVisible := func(eventID string) bool {
		entry, ok := m.core.roomModel.timelineEntry(eventID)
		if !ok || entry.Event == nil || roomIDOfEvent(entry.Event) != room.GetId() {
			return false
		}
		_, retracted, known := m.core.roomModel.latestBody(eventID)
		return known && !retracted
	}
	if !messageVisible(target.GetEventId()) {
		return false, nil
	}
	if target.GetThreadRootEventId() != "" && !messageVisible(target.GetThreadRootEventId()) {
		return false, nil
	}
	if notificationOccurrenceHasReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_REACTION) {
		snapshot := m.core.roomModel.reactionMutationSnapshot(
			room.GetId(),
			target.GetEventId(),
			occurrence.GetReactionEmoji(),
			occurrence.GetActorId(),
		)
		return snapshot.Exists && snapshot.SourceEventID == occurrence.GetSourceEventId(), nil
	}
	return true, nil
}

func (m *NotificationOccurrenceModel) RemoveTarget(ctx context.Context, roomID, eventID string, reason corev1.NotificationRemovalReason) (int, error) {
	entries, err := m.storedOccurrenceEntries(ctx, "")
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		target := entry.occurrence.GetTarget()
		if target.GetRoomId() != roomID || (target.GetEventId() != eventID && target.GetThreadRootEventId() != eventID) {
			continue
		}
		written, ok, err := m.deleteStoredOccurrence(ctx, entry.occurrence.GetRecipientId(), entry.occurrence.GetSourceEventId(), reason)
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
			m.core.publishNotificationOccurrenceChanged(ctx, written, false, true)
		}
	}
	return removed, nil
}

func (m *NotificationOccurrenceModel) RemoveSource(ctx context.Context, userID, sourceEventID string, reason corev1.NotificationRemovalReason) (bool, error) {
	written, removed, err := m.deleteStoredOccurrence(ctx, userID, sourceEventID, reason)
	if err == nil && removed {
		m.core.publishNotificationOccurrenceChanged(ctx, written, false, true)
	}
	return removed, err
}

func (m *NotificationOccurrenceModel) RemoveReaction(ctx context.Context, recipientID, roomID, messageEventID, actorID, emoji string, removedAtSequence uint64) (int, error) {
	entries, err := m.storedOccurrenceEntries(ctx, recipientID)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		occurrence := entry.occurrence
		target := occurrence.GetTarget()
		if occurrence.GetActorId() != actorID || occurrence.GetReactionEmoji() != emoji ||
			target.GetRoomId() != roomID || target.GetEventId() != messageEventID ||
			!notificationOccurrenceHasReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_REACTION) {
			continue
		}
		if occurrence.GetSourceStreamSequence() >= removedAtSequence {
			continue
		}
		written, ok, err := m.deleteStoredOccurrence(ctx, occurrence.GetRecipientId(), occurrence.GetSourceEventId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_REACTION_REMOVED)
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
			m.core.publishNotificationOccurrenceChanged(ctx, written, false, true)
		}
	}
	return removed, nil
}

func notificationOccurrenceHasReason(occurrence *corev1.NotificationOccurrence, reason corev1.NotificationReason) bool {
	for _, match := range occurrence.GetReasons() {
		if match.GetReason() == reason {
			return true
		}
	}
	return false
}

func (m *NotificationOccurrenceModel) RemoveRoomForUser(ctx context.Context, userID, roomID string, removedThroughSequence uint64, reason corev1.NotificationRemovalReason) (int, error) {
	entries, err := m.storedOccurrenceEntries(ctx, userID)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.occurrence.GetTarget().GetRoomId() != roomID {
			continue
		}
		if removedThroughSequence != 0 && entry.occurrence.GetSourceStreamSequence() >= removedThroughSequence {
			continue
		}
		written, ok, err := m.deleteStoredOccurrence(ctx, userID, entry.occurrence.GetSourceEventId(), reason)
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
			m.core.publishNotificationOccurrenceChanged(ctx, written, false, true)
		}
	}
	return removed, nil
}

func (m *NotificationOccurrenceModel) RemoveRoom(ctx context.Context, roomID string, reason corev1.NotificationRemovalReason) (int, error) {
	entries, err := m.storedOccurrenceEntries(ctx, "")
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.occurrence.GetTarget().GetRoomId() != roomID {
			continue
		}
		written, ok, err := m.deleteStoredOccurrence(ctx, entry.occurrence.GetRecipientId(), entry.occurrence.GetSourceEventId(), reason)
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
			m.core.publishNotificationOccurrenceChanged(ctx, written, false, true)
		}
	}
	return removed, nil
}

func (m *NotificationOccurrenceModel) PurgeUser(ctx context.Context, userID string) (int, error) {
	purged := 0
	for {
		entries, err := m.storedOccurrenceEntries(ctx, userID)
		if err != nil {
			return purged, err
		}
		if len(entries) == 0 {
			return purged, nil
		}
		for _, entry := range entries {
			if err := m.kv.Purge(ctx, entry.key, jetstream.LastRevision(entry.revision)); err != nil {
				if jetstreamutil.IsSequenceConflict(err) || errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
					continue
				}
				return purged, err
			}
			if err := m.index.waitForRevisionAfter(ctx, entry.key, entry.revision); err != nil {
				return purged, err
			}
			purged++
		}
		if err := ctx.Err(); err != nil {
			return purged, err
		}
	}
}

func (m *NotificationOccurrenceModel) updateAtRevision(ctx context.Context, entry notificationOccurrenceIndexEntry, updated *corev1.NotificationOccurrence) (*corev1.NotificationOccurrence, error) {
	expiresAt := updated.GetExpiresAt().AsTime()
	remaining := expiresAt.Sub(m.now().UTC())
	if remaining <= 0 {
		return nil, ErrNotFound
	}
	data, err := proto.Marshal(updated)
	if err != nil {
		return nil, fmt.Errorf("marshal notification occurrence: %w", err)
	}
	// KV.Update resets a per-key TTL to its original duration. Publish the
	// revision-checked replacement with the remaining lifetime instead so
	// triage changes can never extend the occurrence's absolute 90-day expiry.
	revision, err := m.core.updateRuntimeStateTokenTTL(ctx, entry.key, data, entry.revision, remaining)
	if err != nil {
		return nil, err
	}
	if err := m.index.waitForRevision(ctx, entry.key, revision); err != nil {
		return nil, err
	}
	fresh, exists, err := m.index.occurrenceBySource(ctx, updated.GetRecipientId(), updated.GetSourceEventId())
	if err != nil || !exists {
		return nil, err
	}
	return fresh.occurrence, nil
}

func normalizeNotificationReasons(input []*corev1.NotificationReasonMatch) []*corev1.NotificationReasonMatch {
	byReason := make(map[corev1.NotificationReason]corev1.NotificationDeliveryIntensity)
	for _, match := range input {
		if match == nil || match.GetReason() == corev1.NotificationReason_NOTIFICATION_REASON_UNSPECIFIED {
			continue
		}
		if match.GetIntensity() > byReason[match.GetReason()] {
			byReason[match.GetReason()] = match.GetIntensity()
		}
	}
	reasons := make([]corev1.NotificationReason, 0, len(byReason))
	for reason := range byReason {
		reasons = append(reasons, reason)
	}
	slices.Sort(reasons)
	result := make([]*corev1.NotificationReasonMatch, 0, len(reasons))
	for _, reason := range reasons {
		result = append(result, &corev1.NotificationReasonMatch{Reason: reason, Intensity: byReason[reason]})
	}
	return result
}

func strongestNotificationIntensity(reasons []*corev1.NotificationReasonMatch) corev1.NotificationDeliveryIntensity {
	strongest := corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
	for _, reason := range reasons {
		if reason != nil && reason.GetIntensity() > strongest {
			strongest = reason.GetIntensity()
		}
	}
	return strongest
}
