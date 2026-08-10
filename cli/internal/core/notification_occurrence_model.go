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

	"hmans.de/chatto/internal/jetstreamutil"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	notificationOccurrenceKeyPrefix = "notification_v2."
	maxNotificationUpdateRetries    = 8
	notificationAlertClaimTTL       = 30 * time.Second
	notificationAlertDeliveryTTL    = 2 * time.Minute
	notificationAlertRetryDelay     = 30 * time.Second
)

type CreateNotificationOccurrenceInput struct {
	RecipientID    string
	SourceEventID  string
	SourceCreated  time.Time
	ActorID        string
	Target         *corev1.NotificationTarget
	Reasons        []*corev1.NotificationReasonMatch
	EvaluatedAt    time.Time
	InitialState   corev1.NotificationInboxState
	SkipReadLookup bool
}

type UpdateNotificationOccurrenceInput struct {
	InboxState *corev1.NotificationInboxState
	Saved      *bool
}

type NotificationOccurrenceView int

const (
	NotificationOccurrenceViewInbox NotificationOccurrenceView = iota
	NotificationOccurrenceViewDone
	NotificationOccurrenceViewSaved
)

type NotificationOccurrenceGroup struct {
	ID          string
	Key         string
	Occurrences []*corev1.NotificationOccurrence
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

func notificationGroupID(recipientID, groupKey string) string {
	digest := sha256.Sum256([]byte(recipientID + "\x00" + groupKey))
	return "ntg_" + base64.RawURLEncoding.EncodeToString(digest[:20])
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
		if !input.SkipReadLookup {
			covered, err := m.targetCoveredByReadState(ctx, input.RecipientID, input.Target, input.SourceCreated)
			if err != nil {
				return nil, false, fmt.Errorf("resolve initial notification read state: %w", err)
			}
			if covered {
				state = corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ
			}
		}
	}
	alertState := corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_NOT_APPLICABLE
	if strongest == corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT {
		alertState = corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING
	}
	occurrence := &corev1.NotificationOccurrence{
		Id:                 notificationOccurrenceID(input.RecipientID, input.SourceEventID),
		RecipientId:        input.RecipientID,
		SourceEventId:      input.SourceEventID,
		SourceCreatedAt:    timestamppb.New(input.SourceCreated.UTC()),
		ActorId:            input.ActorID,
		Target:             proto.Clone(input.Target).(*corev1.NotificationTarget),
		Reasons:            reasons,
		StrongestIntensity: strongest,
		InboxState:         state,
		EvaluatedAt:        timestamppb.New(evaluatedAt),
		UpdatedAt:          timestamppb.New(now),
		ExpiresAt:          timestamppb.New(expiresAt),
		AlertState:         alertState,
	}
	data, err := proto.Marshal(occurrence)
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification occurrence: %w", err)
	}
	key := notificationOccurrenceKey(input.RecipientID, input.SourceEventID)
	revision, err := m.kv.Create(ctx, key, data, jetstream.KeyTTL(remaining))
	if jetstreamutil.IsSequenceConflict(err) {
		existing, exists, readErr := m.index.occurrenceBySource(ctx, input.RecipientID, input.SourceEventID)
		if readErr != nil {
			return nil, false, readErr
		}
		if exists {
			if existing.occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
				return nil, false, nil
			}
			return existing.occurrence, false, nil
		}
		entry, getErr := m.kv.Get(ctx, key)
		if getErr != nil {
			return nil, false, fmt.Errorf("read concurrently created notification occurrence: %w", getErr)
		}
		if waitErr := m.index.waitForRevision(ctx, key, entry.Revision()); waitErr != nil {
			return nil, false, waitErr
		}
		existing, exists, readErr = m.index.occurrenceBySource(ctx, input.RecipientID, input.SourceEventID)
		if readErr != nil || !exists {
			return nil, false, readErr
		}
		if existing.occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
			return nil, false, nil
		}
		return existing.occurrence, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("create notification occurrence: %w", err)
	}
	if err := m.index.waitForRevision(ctx, key, revision); err != nil {
		return nil, false, fmt.Errorf("wait for notification occurrence: %w", err)
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

func (m *NotificationOccurrenceModel) List(ctx context.Context, userID string, view NotificationOccurrenceView) ([]*corev1.NotificationOccurrence, error) {
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
		include := false
		switch view {
		case NotificationOccurrenceViewInbox:
			include = occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD ||
				occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ
		case NotificationOccurrenceViewDone:
			include = occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_DONE
		case NotificationOccurrenceViewSaved:
			include = occurrence.GetSaved()
		}
		if include {
			result = append(result, occurrence)
		}
	}
	sort.Slice(result, func(a, b int) bool {
		return result[a].GetSourceCreatedAt().AsTime().After(result[b].GetSourceCreatedAt().AsTime())
	})
	return result, nil
}

func (m *NotificationOccurrenceModel) Groups(ctx context.Context, userID string, view NotificationOccurrenceView) ([]NotificationOccurrenceGroup, error) {
	occurrences, err := m.List(ctx, userID, view)
	if err != nil {
		return nil, err
	}
	grouped := make(map[string][]*corev1.NotificationOccurrence)
	for _, occurrence := range occurrences {
		key := notificationOccurrenceGroupKey(occurrence)
		grouped[key] = append(grouped[key], occurrence)
	}
	groups := make([]NotificationOccurrenceGroup, 0, len(grouped))
	for key, members := range grouped {
		groups = append(groups, NotificationOccurrenceGroup{
			ID:          notificationGroupID(userID, key),
			Key:         key,
			Occurrences: members,
		})
	}
	sort.Slice(groups, func(a, b int) bool {
		return groups[a].Occurrences[0].GetSourceCreatedAt().AsTime().After(groups[b].Occurrences[0].GetSourceCreatedAt().AsTime())
	})
	return groups, nil
}

// UnreadGroupCount returns the bell/app-badge count. Multiple unread
// occurrences in one conversation group count once.
func (m *NotificationOccurrenceModel) UnreadGroupCount(ctx context.Context, userID string) (int, error) {
	groups, err := m.Groups(ctx, userID, NotificationOccurrenceViewInbox)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, group := range groups {
		for _, occurrence := range group.Occurrences {
			if occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD {
				count++
				break
			}
		}
	}
	return count, nil
}

func (m *NotificationOccurrenceModel) Update(ctx context.Context, userID, occurrenceID string, input UpdateNotificationOccurrenceInput) (*corev1.NotificationOccurrence, error) {
	return m.update(ctx, userID, occurrenceID, input, true)
}

func (m *NotificationOccurrenceModel) update(ctx context.Context, userID, occurrenceID string, input UpdateNotificationOccurrenceInput, publish bool) (*corev1.NotificationOccurrence, error) {
	for attempt := 0; attempt < maxNotificationUpdateRetries; attempt++ {
		entry, exists, err := m.index.occurrenceByID(ctx, userID, occurrenceID)
		if err != nil {
			return nil, err
		}
		if !exists || entry.occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
			return nil, ErrNotFound
		}
		updated := proto.Clone(entry.occurrence).(*corev1.NotificationOccurrence)
		if input.InboxState != nil {
			switch *input.InboxState {
			case corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD,
				corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ,
				corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_DONE:
				updated.InboxState = *input.InboxState
			default:
				return nil, invalidArgument("inbox_state must be UNREAD, READ, or DONE")
			}
			if *input.InboxState != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD &&
				(updated.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING ||
					updated.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_CLAIMED) {
				updated.AlertState = corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED
				updated.AlertClaimedUntil = nil
			}
		}
		if input.Saved != nil {
			updated.Saved = *input.Saved
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
		if err == nil && publish {
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
			InboxState:      corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_DONE,
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

func (m *NotificationOccurrenceModel) UpdateGroup(ctx context.Context, userID, groupID string, view NotificationOccurrenceView, input UpdateNotificationOccurrenceInput) ([]*corev1.NotificationOccurrence, error) {
	groups, err := m.Groups(ctx, userID, view)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if group.ID != groupID {
			continue
		}
		updated := make([]*corev1.NotificationOccurrence, 0, len(group.Occurrences))
		for _, occurrence := range group.Occurrences {
			item, err := m.update(ctx, userID, occurrence.GetId(), input, false)
			if err != nil {
				if len(updated) > 0 {
					m.core.publishNotificationOccurrenceChanged(ctx, updated[len(updated)-1], false, false)
				}
				return updated, err
			}
			updated = append(updated, item)
		}
		if len(updated) > 0 {
			// The last KV revision fences every earlier write in this ordered
			// mutation, so one live invalidation is sufficient on every replica.
			m.core.publishNotificationOccurrenceChanged(ctx, updated[len(updated)-1], false, false)
		}
		return updated, nil
	}
	return nil, ErrNotFound
}

func (m *NotificationOccurrenceModel) DeleteGroup(ctx context.Context, userID, groupID string, view NotificationOccurrenceView) (int, error) {
	groups, err := m.Groups(ctx, userID, view)
	if err != nil {
		return 0, err
	}
	for _, group := range groups {
		if group.ID != groupID {
			continue
		}
		deleted := 0
		var lastDeleted *corev1.NotificationOccurrence
		for _, occurrence := range group.Occurrences {
			ok, err := m.delete(ctx, userID, occurrence.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_DELETED, false)
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
	return 0, ErrNotFound
}

func (m *NotificationOccurrenceModel) MarkCoveredRead(ctx context.Context, userID, roomID, threadRootEventID string, readThrough time.Time) (int, error) {
	entries, err := m.index.userEntries(ctx, userID)
	if err != nil {
		return 0, err
	}
	updated := 0
	var lastUpdated *corev1.NotificationOccurrence
	read := corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ
	for _, entry := range entries {
		occurrence := entry.occurrence
		if occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED ||
			occurrence.GetInboxState() != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD ||
			occurrence.GetTarget().GetRoomId() != roomID ||
			occurrence.GetTarget().GetThreadRootEventId() != threadRootEventID ||
			occurrence.GetSourceCreatedAt().AsTime().After(readThrough) {
			continue
		}
		item, err := m.update(ctx, userID, occurrence.GetId(), UpdateNotificationOccurrenceInput{InboxState: &read}, false)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if lastUpdated != nil {
				m.core.publishNotificationOccurrenceChanged(ctx, lastUpdated, false, false)
			}
			return updated, err
		}
		updated++
		lastUpdated = item
	}
	if lastUpdated != nil {
		m.core.publishNotificationOccurrenceChanged(ctx, lastUpdated, false, false)
	}
	return updated, nil
}

// ClaimPendingAlert leases one interruptive delivery to this replica. A
// crashed or failed claim becomes eligible again after the short lease.
func (m *NotificationOccurrenceModel) ClaimPendingAlert(ctx context.Context) (*corev1.NotificationOccurrence, bool, error) {
	entries, err := m.index.allEntries(ctx)
	if err != nil {
		return nil, false, err
	}
	now := m.now().UTC()
	for _, entry := range entries {
		occurrence := entry.occurrence
		claimExpired := occurrence.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_CLAIMED &&
			occurrence.GetAlertClaimedUntil() != nil && occurrence.GetAlertClaimedUntil().AsTime().Before(now)
		if occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED ||
			occurrence.GetInboxState() != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD ||
			occurrence.GetStrongestIntensity() != corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT ||
			(occurrence.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING && !claimExpired) {
			continue
		}
		if m.core.suppressesNotificationAlertsForPresence(ctx, occurrence.GetRecipientId()) {
			if _, updateErr := m.setAlertState(ctx, entry, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED, time.Time{}); updateErr != nil && !jetstreamutil.IsSequenceConflict(updateErr) {
				return nil, false, updateErr
			}
			continue
		}
		claimed, updateErr := m.setAlertState(ctx, entry, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_CLAIMED, now.Add(notificationAlertClaimTTL))
		if jetstreamutil.IsSequenceConflict(updateErr) {
			continue
		}
		if updateErr != nil {
			return nil, false, updateErr
		}
		return claimed, true, nil
	}
	return nil, false, nil
}

// CompleteAlertClaim records whether a claimed alert reached its configured
// delivery callback. Failed attempts return to Pending for retry.
func (m *NotificationOccurrenceModel) CompleteAlertClaim(ctx context.Context, occurrence *corev1.NotificationOccurrence, delivered bool) error {
	if occurrence == nil {
		return nil
	}
	entry, exists, err := m.index.occurrenceBySource(ctx, occurrence.GetRecipientId(), occurrence.GetSourceEventId())
	if err != nil || !exists || entry.occurrence.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_CLAIMED ||
		entry.occurrence.GetAlertClaimedUntil() == nil || occurrence.GetAlertClaimedUntil() == nil ||
		!entry.occurrence.GetAlertClaimedUntil().AsTime().Equal(occurrence.GetAlertClaimedUntil().AsTime()) {
		return err
	}
	state := corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_CLAIMED
	claimedUntil := m.now().UTC().Add(notificationAlertRetryDelay)
	if delivered {
		state = corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_DELIVERED
		claimedUntil = time.Time{}
	}
	_, err = m.setAlertState(ctx, entry, state, claimedUntil)
	return err
}

// AlertClaimCurrent checks that the caller still owns the exact unexpired
// claim and that user triage has not made the occurrence ineligible.
func (m *NotificationOccurrenceModel) AlertClaimCurrent(ctx context.Context, expected *corev1.NotificationOccurrence) (bool, error) {
	if expected == nil || expected.GetAlertClaimedUntil() == nil {
		return false, nil
	}
	current, err := m.Get(ctx, expected.GetRecipientId(), expected.GetId())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return current.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD &&
		current.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_CLAIMED &&
		current.GetAlertClaimedUntil() != nil &&
		current.GetAlertClaimedUntil().AsTime().Equal(expected.GetAlertClaimedUntil().AsTime()) &&
		current.GetAlertClaimedUntil().AsTime().After(m.now().UTC()), nil
}

// RenewAlertClaim fences provider delivery with a fresh delivery-sized lease.
// The caller must use the returned occurrence when completing the claim.
func (m *NotificationOccurrenceModel) RenewAlertClaim(ctx context.Context, expected *corev1.NotificationOccurrence) (*corev1.NotificationOccurrence, bool, error) {
	if expected == nil || expected.GetAlertClaimedUntil() == nil {
		return nil, false, nil
	}
	entry, exists, err := m.index.occurrenceBySource(ctx, expected.GetRecipientId(), expected.GetSourceEventId())
	if err != nil || !exists {
		return nil, false, err
	}
	current := entry.occurrence
	if current.GetInboxState() != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD ||
		current.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_CLAIMED ||
		current.GetAlertClaimedUntil() == nil ||
		!current.GetAlertClaimedUntil().AsTime().Equal(expected.GetAlertClaimedUntil().AsTime()) ||
		!current.GetAlertClaimedUntil().AsTime().After(m.now().UTC()) {
		return nil, false, nil
	}
	renewed, err := m.setAlertState(ctx, entry, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_CLAIMED, m.now().UTC().Add(notificationAlertDeliveryTTL))
	if jetstreamutil.IsSequenceConflict(err) {
		return nil, false, nil
	}
	return renewed, err == nil, err
}

// TargetVisible revalidates the recipient's current room membership before an
// occurrence is hydrated or delivered outside Chatto.
func (m *NotificationOccurrenceModel) TargetVisible(ctx context.Context, recipientID string, occurrence *corev1.NotificationOccurrence) (bool, error) {
	if occurrence == nil || occurrence.GetRecipientId() != recipientID || occurrence.GetTarget().GetRoomId() == "" {
		return false, nil
	}
	room, err := m.core.FindRoomByID(ctx, occurrence.GetTarget().GetRoomId())
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return m.core.RoomMembershipExists(ctx, KindOfRoom(room), recipientID, room.GetId())
}

func (m *NotificationOccurrenceModel) setAlertState(ctx context.Context, entry notificationOccurrenceIndexEntry, state corev1.NotificationAlertState, claimedUntil time.Time) (*corev1.NotificationOccurrence, error) {
	updated := proto.Clone(entry.occurrence).(*corev1.NotificationOccurrence)
	updated.AlertState = state
	updated.AlertClaimedUntil = nil
	if !claimedUntil.IsZero() {
		updated.AlertClaimedUntil = timestamppb.New(claimedUntil.UTC())
	}
	updated.UpdatedAt = timestamppb.New(m.now().UTC())
	return m.updateAtRevision(ctx, entry, updated)
}

func (m *NotificationOccurrenceModel) RemoveTarget(ctx context.Context, roomID, eventID string, reason corev1.NotificationRemovalReason) (int, error) {
	entries, err := m.index.allEntries(ctx)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		target := entry.occurrence.GetTarget()
		if target.GetRoomId() != roomID || (target.GetEventId() != eventID && target.GetThreadRootEventId() != eventID) {
			continue
		}
		ok, err := m.Delete(ctx, entry.occurrence.GetRecipientId(), entry.occurrence.GetId(), reason)
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
		}
	}
	return removed, nil
}

func (m *NotificationOccurrenceModel) RemoveSource(ctx context.Context, userID, sourceEventID string, reason corev1.NotificationRemovalReason) (bool, error) {
	entry, exists, err := m.index.occurrenceBySource(ctx, userID, sourceEventID)
	if err != nil || !exists {
		return false, err
	}
	return m.Delete(ctx, userID, entry.occurrence.GetId(), reason)
}

func (m *NotificationOccurrenceModel) RemoveRoomForUser(ctx context.Context, userID, roomID string, removedThrough time.Time, reason corev1.NotificationRemovalReason) (int, error) {
	entries, err := m.index.userEntries(ctx, userID)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.occurrence.GetTarget().GetRoomId() != roomID {
			continue
		}
		if !removedThrough.IsZero() && entry.occurrence.GetSourceCreatedAt().AsTime().After(removedThrough) {
			continue
		}
		ok, err := m.Delete(ctx, userID, entry.occurrence.GetId(), reason)
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
		}
	}
	return removed, nil
}

func (m *NotificationOccurrenceModel) RemoveRoom(ctx context.Context, roomID string, reason corev1.NotificationRemovalReason) (int, error) {
	entries, err := m.index.allEntries(ctx)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.occurrence.GetTarget().GetRoomId() != roomID {
			continue
		}
		ok, err := m.Delete(ctx, entry.occurrence.GetRecipientId(), entry.occurrence.GetId(), reason)
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
		}
	}
	return removed, nil
}

func (m *NotificationOccurrenceModel) PurgeUser(ctx context.Context, userID string) (int, error) {
	entries, err := m.index.userEntries(ctx, userID)
	if err != nil {
		return 0, err
	}
	purged := 0
	for _, entry := range entries {
		if err := m.kv.Purge(ctx, entry.key, jetstream.LastRevision(entry.revision)); err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
				continue
			}
			return purged, err
		}
		purged++
	}
	return purged, nil
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

func (m *NotificationOccurrenceModel) targetCoveredByReadState(ctx context.Context, userID string, target *corev1.NotificationTarget, sourceCreated time.Time) (bool, error) {
	room, err := m.core.FindRoomByID(ctx, target.GetRoomId())
	if err != nil {
		return false, err
	}
	kind := KindOfRoom(room)
	if target.GetThreadRootEventId() != "" {
		readAt, err := m.core.GetThreadLastOpened(ctx, kind, userID, target.GetRoomId(), target.GetThreadRootEventId())
		return !readAt.IsZero() && !readAt.Before(sourceCreated), err
	}
	markerID, exists, err := m.core.PeekLastReadEventID(ctx, userID, target.GetRoomId())
	if err != nil || !exists || markerID == "" {
		return false, err
	}
	readAt, err := m.core.GetEventTimestamp(ctx, kind, target.GetRoomId(), markerID)
	return !readAt.IsZero() && !readAt.Before(sourceCreated), err
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

func notificationOccurrenceGroupKey(occurrence *corev1.NotificationOccurrence) string {
	target := occurrence.GetTarget()
	if hasNotificationReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_REACTION) {
		return "reaction:" + target.GetRoomId() + ":" + target.GetEventId()
	}
	if target.GetThreadRootEventId() != "" {
		return "thread:" + target.GetRoomId() + ":" + target.GetThreadRootEventId()
	}
	if hasNotificationReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MESSAGE) {
		return "dm:" + target.GetRoomId()
	}
	return "room:" + target.GetRoomId()
}

func hasNotificationReason(occurrence *corev1.NotificationOccurrence, wanted corev1.NotificationReason) bool {
	for _, reason := range occurrence.GetReasons() {
		if reason.GetReason() == wanted {
			return true
		}
	}
	return false
}
