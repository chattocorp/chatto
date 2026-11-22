package core

import (
	"fmt"
	"math"
	"sort"
	"time"

	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
	projectionv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/projection/v1"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/notificationstream"
	"github.com/chattocorp/chatto/pkg/events"
)

type notificationProjectionTombstone struct {
	recipientID    string
	expiresAt      time.Time
	signalSequence uint64
}

type notificationOccurrenceRef struct {
	recipientID    string
	notificationID string
}

type notificationOccurrenceState struct {
	occurrence *notificationv1.NotificationOccurrence
	tombstoned bool
}

// NotificationProjection materializes the current, bounded notification list
// from the NOTIFICATIONS lifecycle log. Its expiry scheduler is an acceleration
// only: every read also applies the immutable expires_at boundary.
type NotificationProjection struct {
	events.MemoryProjection

	rows       *notificationRows
	tombstones map[string]notificationProjectionTombstone
	// nextPrune is the earliest time, in Unix nanoseconds, at which an
	// occurrence or a tombstone can expire. A prune before this time has no
	// work, so it does not scan the collection.
	nextPrune int64
	now       func() time.Time
}

// NewNotificationProjection returns an empty projection. eventIDs is the
// process-wide event ID table. With nil, the projection owns a private table.
func NewNotificationProjection(eventIDs *eventIDTable) *NotificationProjection {
	return &NotificationProjection{
		rows:       newNotificationRows(eventIDs),
		tombstones: make(map[string]notificationProjectionTombstone),
		nextPrune:  math.MaxInt64,
		now:        time.Now,
	}
}

func (*NotificationProjection) Subjects() []string {
	return notificationstream.Subjects()
}

func (p *NotificationProjection) Apply(event *notificationv1.NotificationEvent, sequence uint64) error {
	if event == nil || event.GetId() == "" || event.GetRecipientId() == "" || event.GetNotificationId() == "" || event.GetExpiresAt() == nil || !event.GetExpiresAt().IsValid() {
		return fmt.Errorf("invalid notification event at sequence %d", sequence)
	}
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(p.now().UTC())

	switch payload := event.GetEvent().(type) {
	case *notificationv1.NotificationEvent_Signalled:
		signalled := payload.Signalled
		if signalled.GetSourceEventId() == "" || signalled.GetSourceCreatedAt() == nil || !signalled.GetSourceCreatedAt().IsValid() || signalled.GetSignal() == nil {
			return fmt.Errorf("invalid notification signal event at sequence %d", sequence)
		}
		expiresAt := event.GetExpiresAt().AsTime().UTC()
		if !expiresAt.After(p.now().UTC()) {
			return nil
		}
		if _, removed := p.tombstones[event.GetNotificationId()]; removed {
			return nil
		}
		if _, exists := p.rows.byID[event.GetNotificationId()]; exists {
			return nil
		}
		var alertDelivered *bool
		if signalled.GetAlertExpiresAt() != nil && signalled.GetInitiallyRead() {
			alertDelivered = new(false)
		}
		p.addLocked(&notificationv1.NotificationOccurrence{
			Id:                         event.GetNotificationId(),
			RecipientId:                event.GetRecipientId(),
			SourceEventId:              signalled.GetSourceEventId(),
			SourceCreatedAt:            signalled.GetSourceCreatedAt(),
			ActorId:                    signalled.GetActorId(),
			Signal:                     signalled.GetSignal(),
			Read:                       signalled.GetInitiallyRead(),
			ExpiresAt:                  event.GetExpiresAt(),
			SourceStreamSequence:       signalled.GetSourceStreamSequence(),
			AttentionLevel:             signalled.GetAttentionLevel(),
			AlertExpiresAt:             signalled.GetAlertExpiresAt(),
			NotificationStreamSequence: sequence,
			AlertDelivered:             alertDelivered,
		})
	case *notificationv1.NotificationEvent_Read:
		row := p.rows.get(event.GetRecipientId(), event.GetNotificationId())
		if row == nil {
			return nil
		}
		row.flags |= notificationRowRead
		if row.has(notificationRowHasAlertExpiry) {
			// An absent result becomes Suppressed. A present result stays.
			row.flags |= notificationRowAlertResolved
		}
	case *notificationv1.NotificationEvent_Removed:
		notificationID := event.GetNotificationId()
		signalSequence := payload.Removed.GetSignalStreamSequence()
		if row := p.rows.get(event.GetRecipientId(), notificationID); row != nil {
			if signalSequence == 0 {
				signalSequence = row.notificationSequence
			}
			p.rows.remove(row)
		}
		if previous, exists := p.tombstones[notificationID]; exists && signalSequence == 0 {
			signalSequence = previous.signalSequence
		}
		tombstone := notificationProjectionTombstone{
			recipientID:    event.GetRecipientId(),
			expiresAt:      event.GetExpiresAt().AsTime().UTC(),
			signalSequence: signalSequence,
		}
		p.tombstones[notificationID] = tombstone
		p.nextPrune = min(p.nextPrune, tombstone.pruneAt())
	case *notificationv1.NotificationEvent_AlertResolved:
		row := p.rows.get(event.GetRecipientId(), event.GetNotificationId())
		if row == nil || row.has(notificationRowAlertResolved) {
			return nil
		}
		row.flags |= notificationRowAlertResolved
		if payload.AlertResolved.GetDelivered() {
			row.flags |= notificationRowAlertDelivered
		}
	default:
		return fmt.Errorf("unsupported notification event at sequence %d", sequence)
	}
	return nil
}

// addLocked stores occurrence, which must have a valid expires_at and an ID
// that is not stored.
func (p *NotificationProjection) addLocked(occurrence *notificationv1.NotificationOccurrence) {
	row := p.rows.add(occurrence)
	p.nextPrune = min(p.nextPrune, row.expiresAt)
}

// pruneAt returns the time, in Unix nanoseconds, at which the tombstone is
// removed.
func (t notificationProjectionTombstone) pruneAt() int64 {
	return saturatedUnixNano(t.expiresAt.Add(notificationPhysicalCleanupGrace))
}

func notificationOccurrenceReadScope(occurrence *notificationv1.NotificationOccurrence) (notificationReadBoundaryScope, bool) {
	if occurrence == nil || occurrence.GetRecipientId() == "" {
		return notificationReadBoundaryScope{}, false
	}
	message := notificationSignalMessage(occurrence.GetSignal())
	if message == nil || message.GetRoomId() == "" {
		return notificationReadBoundaryScope{}, false
	}
	return notificationReadBoundaryScope{
		userID: occurrence.GetRecipientId(), roomID: message.GetRoomId(), threadRootEventID: message.GetThreadRootEventId(),
	}, true
}

func (p *NotificationProjection) occurrence(userID, notificationID string, now time.Time) (*notificationv1.NotificationOccurrence, bool) {
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(now)
	row := p.rows.get(userID, notificationID)
	if row == nil {
		return nil, false
	}
	return p.rows.occurrence(row), true
}

// occurrenceStates prunes once and resolves a complete set of point lookups
// under one projection lock. High-fanout materialization must use this instead
// of turning each recipient into another full expiry scan.
func (p *NotificationProjection) occurrenceStates(refs []notificationOccurrenceRef, now time.Time) map[notificationOccurrenceRef]notificationOccurrenceState {
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(now)
	states := make(map[notificationOccurrenceRef]notificationOccurrenceState, len(refs))
	for _, ref := range refs {
		state := notificationOccurrenceState{}
		if row := p.rows.get(ref.recipientID, ref.notificationID); row != nil {
			state.occurrence = p.rows.occurrence(row)
		}
		if tombstone, exists := p.tombstones[ref.notificationID]; exists && tombstone.recipientID == ref.recipientID && tombstone.expiresAt.After(now) {
			state.tombstoned = true
		}
		states[ref] = state
	}
	return states
}

func (p *NotificationProjection) userOccurrences(userID string, now time.Time) []*notificationv1.NotificationOccurrence {
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(now)
	rows := p.rows.userRows(userID)
	result := make([]*notificationv1.NotificationOccurrence, 0, len(rows))
	for _, row := range rows {
		result = append(result, p.rows.occurrence(row))
	}
	return result
}

func (p *NotificationProjection) allOccurrences(now time.Time) []*notificationv1.NotificationOccurrence {
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(now)
	result := make([]*notificationv1.NotificationOccurrence, 0, len(p.rows.byID))
	for _, row := range p.rows.byID {
		result = append(result, p.rows.occurrence(row))
	}
	return result
}

// unreadScopeOccurrences returns detached, unexpired unread occurrences for
// one recipient and timeline. Read commands must not prune the server-wide
// notification collection or copy notifications from unrelated timelines.
// Expired entries are omitted here; periodic maintenance removes them.
func (p *NotificationProjection) unreadScopeOccurrences(scope notificationReadBoundaryScope, now time.Time) []*notificationv1.NotificationOccurrence {
	p.RLock()
	defer p.RUnlock()
	var result []*notificationv1.NotificationOccurrence
	for _, row := range p.rows.scopeRows(scope) {
		if row.has(notificationRowRead) || row.expiresAt <= saturatedUnixNano(now) {
			continue
		}
		result = append(result, p.rows.occurrence(row))
	}
	return result
}

func (p *NotificationProjection) pruneExpired(now time.Time) []string {
	p.Lock()
	defer p.Unlock()
	return p.pruneExpiredLocked(now)
}

// pruneExpiredLocked removes expired occurrences and tombstones. It returns
// the sorted IDs of the recipients that lost an occurrence.
func (p *NotificationProjection) pruneExpiredLocked(now time.Time) []string {
	nowNano := saturatedUnixNano(now)
	if nowNano < p.nextPrune {
		return nil
	}
	next := int64(math.MaxInt64)
	users := make(map[notificationIDHandle]struct{})
	for _, row := range p.rows.byID {
		if row.expiresAt <= nowNano {
			users[row.recipient] = struct{}{}
			p.rows.remove(row)
			continue
		}
		next = min(next, row.expiresAt)
	}
	for id, tombstone := range p.tombstones {
		if !tombstone.expiresAt.Add(notificationPhysicalCleanupGrace).After(now) {
			delete(p.tombstones, id)
			continue
		}
		next = min(next, tombstone.pruneAt())
	}
	p.nextPrune = next
	result := make([]string, 0, len(users))
	for recipient := range users {
		result = append(result, p.rows.ids.Resolve(recipient))
	}
	sort.Strings(result)
	return result
}

func (p *NotificationProjection) pendingPhysicalDeletes(now time.Time) map[string]notificationProjectionTombstone {
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(now)
	result := make(map[string]notificationProjectionTombstone)
	for id, tombstone := range p.tombstones {
		if tombstone.signalSequence != 0 {
			result[id] = tombstone
		}
	}
	return result
}

func (p *NotificationProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	occurrenceBytes := p.rows.estimatedBytes()
	var tombstoneBytes int64
	for id, tombstone := range p.tombstones {
		tombstoneBytes += projectionMapEntryOverhead + int64(len(id)+len(tombstone.recipientID)) + 24
	}
	return int64(len(p.rows.byID) + len(p.tombstones)), occurrenceBytes + tombstoneBytes, []ProjectionAdminMetric{
		{Name: "visible_notifications", Value: int64(len(p.rows.byID)), Bytes: occurrenceBytes},
		{Name: "notification_tombstones", Value: int64(len(p.tombstones)), Bytes: tombstoneBytes},
	}
}

var notificationSnapshotContractID = snapshotContractID("v2", &projectionv1.NotificationProjectionSnapshot{})

func (*NotificationProjection) SnapshotContractID() string { return notificationSnapshotContractID }

func (p *NotificationProjection) Snapshot() ([]byte, error) {
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(p.now().UTC())
	snapshot := &projectionv1.NotificationProjectionSnapshot{}
	ids := make([]string, 0, len(p.rows.byID))
	for id := range p.rows.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		snapshot.Notifications = append(snapshot.Notifications, p.rows.occurrence(p.rows.byID[id]))
	}
	tombstoneIDs := make([]string, 0, len(p.tombstones))
	for id := range p.tombstones {
		tombstoneIDs = append(tombstoneIDs, id)
	}
	sort.Strings(tombstoneIDs)
	for _, id := range tombstoneIDs {
		tombstone := p.tombstones[id]
		snapshot.Tombstones = append(snapshot.Tombstones, &projectionv1.NotificationProjectionTombstone{
			NotificationId:       id,
			RecipientId:          tombstone.recipientID,
			ExpiresAt:            timestamp(tombstone.expiresAt),
			SignalStreamSequence: tombstone.signalSequence,
		})
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *NotificationProjection) Restore(data []byte) error {
	snapshot := &projectionv1.NotificationProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal notification snapshot: %w", err)
		}
	}
	now := p.now().UTC()
	rows := newNotificationRows(p.sharedEventIDs())
	for _, occurrence := range snapshot.GetNotifications() {
		if occurrence.GetId() == "" || occurrence.GetRecipientId() == "" || occurrence.GetExpiresAt() == nil || !occurrence.GetExpiresAt().IsValid() {
			return fmt.Errorf("notification snapshot contains an invalid occurrence")
		}
		if !occurrence.GetExpiresAt().AsTime().After(now) {
			continue
		}
		if _, duplicate := rows.byID[occurrence.GetId()]; duplicate {
			return fmt.Errorf("notification snapshot repeats occurrence %q", occurrence.GetId())
		}
		rows.add(occurrence)
	}
	tombstones := make(map[string]notificationProjectionTombstone)
	for _, row := range snapshot.GetTombstones() {
		if row.GetNotificationId() == "" || row.GetRecipientId() == "" || row.GetExpiresAt() == nil || !row.GetExpiresAt().IsValid() {
			return fmt.Errorf("notification snapshot contains an invalid tombstone")
		}
		expiresAt := row.GetExpiresAt().AsTime().UTC()
		if !expiresAt.Add(notificationPhysicalCleanupGrace).After(now) {
			continue
		}
		if _, duplicate := tombstones[row.GetNotificationId()]; duplicate {
			return fmt.Errorf("notification snapshot repeats tombstone %q", row.GetNotificationId())
		}
		if occurrence := rows.byID[row.GetNotificationId()]; occurrence != nil {
			rows.remove(occurrence)
		}
		tombstones[row.GetNotificationId()] = notificationProjectionTombstone{recipientID: row.GetRecipientId(), expiresAt: expiresAt, signalSequence: row.GetSignalStreamSequence()}
	}
	p.Lock()
	p.rows = rows
	p.tombstones = tombstones
	// The next prune scans the restored state and sets the real value.
	p.nextPrune = math.MinInt64
	p.Unlock()
	return nil
}

// sharedEventIDs returns the event ID table when production wiring supplied
// it, or nil when the projection owns a private table.
func (p *NotificationProjection) sharedEventIDs() *eventIDTable {
	p.RLock()
	defer p.RUnlock()
	if !p.rows.sharedEventIDs {
		return nil
	}
	return p.rows.eventIDs
}

func timestamp(value time.Time) *timestamppb.Timestamp {
	return timestamppb.New(value.UTC())
}
