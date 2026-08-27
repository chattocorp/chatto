package core

import (
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/notificationstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
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
	occurrence *corev1.NotificationOccurrence
	tombstoned bool
}

// NotificationProjection materializes the current, bounded notification list
// from the NOTIFICATIONS lifecycle log. Its expiry scheduler is an acceleration
// only: every read also applies the immutable expires_at boundary.
type NotificationProjection struct {
	events.MemoryProjection

	byID       map[string]*corev1.NotificationOccurrence
	idsByUser  map[string]map[string]struct{}
	idsByScope map[notificationReadBoundaryScope]map[string]struct{}
	tombstones map[string]notificationProjectionTombstone
	now        func() time.Time
}

func NewNotificationProjection() *NotificationProjection {
	return &NotificationProjection{
		byID:       make(map[string]*corev1.NotificationOccurrence),
		idsByUser:  make(map[string]map[string]struct{}),
		idsByScope: make(map[notificationReadBoundaryScope]map[string]struct{}),
		tombstones: make(map[string]notificationProjectionTombstone),
		now:        time.Now,
	}
}

func (*NotificationProjection) Subjects() []string {
	return notificationstream.Subjects()
}

func (p *NotificationProjection) Apply(event *corev1.NotificationEvent, sequence uint64) error {
	if event == nil || event.GetId() == "" || event.GetRecipientId() == "" || event.GetNotificationId() == "" || event.GetExpiresAt() == nil || !event.GetExpiresAt().IsValid() {
		return fmt.Errorf("invalid notification event at sequence %d", sequence)
	}
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(p.now().UTC())

	switch payload := event.GetEvent().(type) {
	case *corev1.NotificationEvent_Signalled:
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
		if _, exists := p.byID[event.GetNotificationId()]; exists {
			return nil
		}
		var alertDelivered *bool
		if signalled.GetAlertExpiresAt() != nil && signalled.GetInitiallyRead() {
			alertDelivered = proto.Bool(false)
		}
		stored := &corev1.NotificationOccurrence{
			Id:                         event.GetNotificationId(),
			RecipientId:                event.GetRecipientId(),
			SourceEventId:              signalled.GetSourceEventId(),
			SourceCreatedAt:            signalled.GetSourceCreatedAt(),
			ActorId:                    signalled.GetActorId(),
			Signal:                     proto.Clone(signalled.GetSignal()).(*corev1.NotificationSignal),
			Read:                       signalled.GetInitiallyRead(),
			ExpiresAt:                  event.GetExpiresAt(),
			SourceStreamSequence:       signalled.GetSourceStreamSequence(),
			AttentionLevel:             signalled.GetAttentionLevel(),
			AlertExpiresAt:             signalled.GetAlertExpiresAt(),
			NotificationStreamSequence: sequence,
			AlertDelivered:             alertDelivered,
		}
		p.byID[stored.GetId()] = stored
		if p.idsByUser[stored.GetRecipientId()] == nil {
			p.idsByUser[stored.GetRecipientId()] = make(map[string]struct{})
		}
		p.idsByUser[stored.GetRecipientId()][stored.GetId()] = struct{}{}
		p.addScopeLocked(stored)
	case *corev1.NotificationEvent_Read:
		occurrence := p.byID[event.GetNotificationId()]
		if occurrence == nil || occurrence.GetRecipientId() != event.GetRecipientId() {
			return nil
		}
		occurrence.Read = true
		if occurrence.GetAlertExpiresAt() != nil && occurrence.AlertDelivered == nil {
			occurrence.AlertDelivered = proto.Bool(false)
		}
	case *corev1.NotificationEvent_Removed:
		notificationID := event.GetNotificationId()
		signalSequence := payload.Removed.GetSignalStreamSequence()
		if occurrence := p.byID[notificationID]; occurrence != nil && occurrence.GetRecipientId() == event.GetRecipientId() {
			if signalSequence == 0 {
				signalSequence = occurrence.GetNotificationStreamSequence()
			}
			p.removeLocked(occurrence)
		}
		if previous, exists := p.tombstones[notificationID]; exists && signalSequence == 0 {
			signalSequence = previous.signalSequence
		}
		p.tombstones[notificationID] = notificationProjectionTombstone{
			recipientID:    event.GetRecipientId(),
			expiresAt:      event.GetExpiresAt().AsTime().UTC(),
			signalSequence: signalSequence,
		}
	case *corev1.NotificationEvent_AlertResolved:
		occurrence := p.byID[event.GetNotificationId()]
		if occurrence == nil || occurrence.GetRecipientId() != event.GetRecipientId() || occurrence.AlertDelivered != nil {
			return nil
		}
		occurrence.AlertDelivered = proto.Bool(payload.AlertResolved.GetDelivered())
	default:
		return fmt.Errorf("unsupported notification event at sequence %d", sequence)
	}
	return nil
}

func (p *NotificationProjection) removeLocked(occurrence *corev1.NotificationOccurrence) {
	delete(p.byID, occurrence.GetId())
	ids := p.idsByUser[occurrence.GetRecipientId()]
	delete(ids, occurrence.GetId())
	if len(ids) == 0 {
		delete(p.idsByUser, occurrence.GetRecipientId())
	}
	if scope, ok := notificationOccurrenceReadScope(occurrence); ok {
		ids := p.idsByScope[scope]
		delete(ids, occurrence.GetId())
		if len(ids) == 0 {
			delete(p.idsByScope, scope)
		}
	}
}

func (p *NotificationProjection) addScopeLocked(occurrence *corev1.NotificationOccurrence) {
	scope, ok := notificationOccurrenceReadScope(occurrence)
	if !ok {
		return
	}
	if p.idsByScope[scope] == nil {
		p.idsByScope[scope] = make(map[string]struct{})
	}
	p.idsByScope[scope][occurrence.GetId()] = struct{}{}
}

func notificationOccurrenceReadScope(occurrence *corev1.NotificationOccurrence) (notificationReadBoundaryScope, bool) {
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

func (p *NotificationProjection) occurrence(userID, notificationID string, now time.Time) (*corev1.NotificationOccurrence, bool) {
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(now)
	occurrence := p.byID[notificationID]
	if occurrence == nil || occurrence.GetRecipientId() != userID {
		return nil, false
	}
	return proto.Clone(occurrence).(*corev1.NotificationOccurrence), true
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
		if occurrence := p.byID[ref.notificationID]; occurrence != nil && occurrence.GetRecipientId() == ref.recipientID {
			state.occurrence = proto.Clone(occurrence).(*corev1.NotificationOccurrence)
		}
		if tombstone, exists := p.tombstones[ref.notificationID]; exists && tombstone.recipientID == ref.recipientID && tombstone.expiresAt.After(now) {
			state.tombstoned = true
		}
		states[ref] = state
	}
	return states
}

func (p *NotificationProjection) userOccurrences(userID string, now time.Time) []*corev1.NotificationOccurrence {
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(now)
	result := make([]*corev1.NotificationOccurrence, 0, len(p.idsByUser[userID]))
	for id := range p.idsByUser[userID] {
		result = append(result, proto.Clone(p.byID[id]).(*corev1.NotificationOccurrence))
	}
	return result
}

func (p *NotificationProjection) allOccurrences(now time.Time) []*corev1.NotificationOccurrence {
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(now)
	result := make([]*corev1.NotificationOccurrence, 0, len(p.byID))
	for _, occurrence := range p.byID {
		result = append(result, proto.Clone(occurrence).(*corev1.NotificationOccurrence))
	}
	return result
}

func (p *NotificationProjection) scopeOccurrences(scope notificationReadBoundaryScope, now time.Time) []*corev1.NotificationOccurrence {
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(now)
	result := make([]*corev1.NotificationOccurrence, 0, len(p.idsByScope[scope]))
	for id := range p.idsByScope[scope] {
		result = append(result, proto.Clone(p.byID[id]).(*corev1.NotificationOccurrence))
	}
	return result
}

func (p *NotificationProjection) pruneExpired(now time.Time) []string {
	p.Lock()
	defer p.Unlock()
	return p.pruneExpiredLocked(now)
}

func (p *NotificationProjection) pruneExpiredLocked(now time.Time) []string {
	users := make(map[string]struct{})
	for _, occurrence := range p.byID {
		if expiresAt := occurrence.GetExpiresAt(); expiresAt == nil || !expiresAt.IsValid() || !expiresAt.AsTime().After(now) {
			users[occurrence.GetRecipientId()] = struct{}{}
			p.removeLocked(occurrence)
		}
	}
	for id, tombstone := range p.tombstones {
		if !tombstone.expiresAt.Add(notificationPhysicalCleanupGrace).After(now) {
			delete(p.tombstones, id)
		}
	}
	result := make([]string, 0, len(users))
	for userID := range users {
		result = append(result, userID)
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
	var estimatedBytes int64
	for id, occurrence := range p.byID {
		estimatedBytes += projectionMapEntryOverhead + int64(len(id)+proto.Size(occurrence))
	}
	for id, tombstone := range p.tombstones {
		estimatedBytes += projectionMapEntryOverhead + int64(len(id)+len(tombstone.recipientID)) + 24
	}
	return int64(len(p.byID) + len(p.tombstones)), estimatedBytes, []ProjectionAdminMetric{
		{Name: "visible_notifications", Value: int64(len(p.byID))},
		{Name: "notification_tombstones", Value: int64(len(p.tombstones))},
	}
}

var notificationSnapshotContractID = snapshotContractID("v2", &corev1.NotificationProjectionSnapshot{})

func (*NotificationProjection) SnapshotContractID() string { return notificationSnapshotContractID }

func (p *NotificationProjection) Snapshot() ([]byte, error) {
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(p.now().UTC())
	snapshot := &corev1.NotificationProjectionSnapshot{}
	ids := make([]string, 0, len(p.byID))
	for id := range p.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		snapshot.Notifications = append(snapshot.Notifications, proto.Clone(p.byID[id]).(*corev1.NotificationOccurrence))
	}
	tombstoneIDs := make([]string, 0, len(p.tombstones))
	for id := range p.tombstones {
		tombstoneIDs = append(tombstoneIDs, id)
	}
	sort.Strings(tombstoneIDs)
	for _, id := range tombstoneIDs {
		tombstone := p.tombstones[id]
		snapshot.Tombstones = append(snapshot.Tombstones, &corev1.NotificationProjectionTombstone{
			NotificationId:       id,
			RecipientId:          tombstone.recipientID,
			ExpiresAt:            timestamp(tombstone.expiresAt),
			SignalStreamSequence: tombstone.signalSequence,
		})
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *NotificationProjection) Restore(data []byte) error {
	snapshot := &corev1.NotificationProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal notification snapshot: %w", err)
		}
	}
	now := p.now().UTC()
	byID := make(map[string]*corev1.NotificationOccurrence)
	idsByUser := make(map[string]map[string]struct{})
	idsByScope := make(map[notificationReadBoundaryScope]map[string]struct{})
	for _, occurrence := range snapshot.GetNotifications() {
		if occurrence.GetId() == "" || occurrence.GetRecipientId() == "" || occurrence.GetExpiresAt() == nil || !occurrence.GetExpiresAt().IsValid() {
			return fmt.Errorf("notification snapshot contains an invalid occurrence")
		}
		if !occurrence.GetExpiresAt().AsTime().After(now) {
			continue
		}
		if _, duplicate := byID[occurrence.GetId()]; duplicate {
			return fmt.Errorf("notification snapshot repeats occurrence %q", occurrence.GetId())
		}
		byID[occurrence.GetId()] = proto.Clone(occurrence).(*corev1.NotificationOccurrence)
		if idsByUser[occurrence.GetRecipientId()] == nil {
			idsByUser[occurrence.GetRecipientId()] = make(map[string]struct{})
		}
		idsByUser[occurrence.GetRecipientId()][occurrence.GetId()] = struct{}{}
		if scope, ok := notificationOccurrenceReadScope(occurrence); ok {
			if idsByScope[scope] == nil {
				idsByScope[scope] = make(map[string]struct{})
			}
			idsByScope[scope][occurrence.GetId()] = struct{}{}
		}
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
		if occurrence := byID[row.GetNotificationId()]; occurrence != nil {
			delete(byID, row.GetNotificationId())
			delete(idsByUser[occurrence.GetRecipientId()], occurrence.GetId())
			if len(idsByUser[occurrence.GetRecipientId()]) == 0 {
				delete(idsByUser, occurrence.GetRecipientId())
			}
			if scope, ok := notificationOccurrenceReadScope(occurrence); ok {
				delete(idsByScope[scope], occurrence.GetId())
				if len(idsByScope[scope]) == 0 {
					delete(idsByScope, scope)
				}
			}
		}
		tombstones[row.GetNotificationId()] = notificationProjectionTombstone{recipientID: row.GetRecipientId(), expiresAt: expiresAt, signalSequence: row.GetSignalStreamSequence()}
	}
	p.Lock()
	p.byID = byID
	p.idsByUser = idsByUser
	p.idsByScope = idsByScope
	p.tombstones = tombstones
	p.Unlock()
	return nil
}

func timestamp(value time.Time) *timestamppb.Timestamp {
	return timestamppb.New(value.UTC())
}
