package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

var notificationVisibilitySnapshotContractID = snapshotContractID("v1", &corev1.NotificationVisibilityProjectionSnapshot{})

// NotificationVisibilityProjection keeps the minimum event-time state needed
// to enforce persistent notification privacy boundaries. It snapshots only
// administrative facts that the notification worker still has to acknowledge;
// ordinary membership history therefore does not add work to each cleanup.
type NotificationVisibilityProjection struct {
	mu sync.RWMutex

	rooms  *RoomDirectoryProjection
	groups *RoomGroupLayoutProjection
	rbac   *RBACProjection

	boundaries  map[uint64][]byte
	retainAfter atomic.Uint64
}

func NewNotificationVisibilityProjection() *NotificationVisibilityProjection {
	return &NotificationVisibilityProjection{
		rooms:      NewRoomDirectoryProjection(),
		groups:     NewRoomGroupLayoutProjection(),
		rbac:       NewRBACProjection(),
		boundaries: make(map[uint64][]byte),
	}
}

func (*NotificationVisibilityProjection) Subjects() []string {
	return notificationVisibilityProjectionSubjects()
}

func notificationVisibilityProjectionSubjects() []string {
	return []string{
		evtstream.RoomEventTypeFilter(evtstream.EventRoomCreated),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomUniversalChanged),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomDeleted),
		evtstream.RoomEventTypeFilter(evtstream.EventUserJoinedRoom),
		evtstream.RoomEventTypeFilter(evtstream.EventUserLeftRoom),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomMemberBanned),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomMemberUnbanned),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomGroupCreated),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomGroupDeleted),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomAddedToGroup),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomRemovedFromGroup),
		evtstream.RBACSubjectFilter(),
	}
}

func (p *NotificationVisibilityProjection) Apply(event *corev1.Event, seq uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.rooms.Apply(event, seq); err != nil {
		return err
	}
	if err := p.groups.Apply(event, seq); err != nil {
		return err
	}
	if err := p.rbac.Apply(event, seq); err != nil {
		return err
	}
	if seq <= p.retainAfter.Load() || !notificationVisibilityBoundaryEvent(event) {
		return nil
	}
	payload, err := encodeNotificationVisibilityState(p.rooms, p.groups, p.rbac)
	if err != nil {
		return fmt.Errorf("capture notification visibility boundary %d: %w", seq, err)
	}
	p.boundaries[seq] = payload
	return nil
}

func notificationVisibilityBoundaryEvent(event *corev1.Event) bool {
	if event == nil {
		return false
	}
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_RoomMemberBanned,
		*corev1.Event_RoomUniversalChanged,
		*corev1.Event_RoomAddedToGroup,
		*corev1.Event_RbacRoleAssigned,
		*corev1.Event_RbacRoleRevoked,
		*corev1.Event_RbacRoleDeleted:
		return true
	case *corev1.Event_RbacPermissionGranted:
		return payload.RbacPermissionGranted.GetPermission() == string(PermRoomJoin)
	case *corev1.Event_RbacPermissionDenied:
		return payload.RbacPermissionDenied.GetPermission() == string(PermRoomJoin)
	case *corev1.Event_RbacPermissionCleared:
		return payload.RbacPermissionCleared.GetPermission() == string(PermRoomJoin)
	default:
		return false
	}
}

func (*NotificationVisibilityProjection) SnapshotContractID() string {
	return notificationVisibilitySnapshotContractID
}

func (p *NotificationVisibilityProjection) Snapshot() ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return encodeNotificationVisibilityState(p.rooms, p.groups, p.rbac)
}

func (p *NotificationVisibilityProjection) Restore(data []byte) error {
	rooms, groups, rbac, err := decodeNotificationVisibilityState(data)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.rooms, p.groups, p.rbac = rooms, groups, rbac
	p.boundaries = make(map[uint64][]byte)
	p.mu.Unlock()
	return nil
}

func (p *NotificationVisibilityProjection) CompleteStartupReplay() {
	p.mu.Lock()
	p.rbac.CompleteStartupReplay()
	p.mu.Unlock()
}

func encodeNotificationVisibilityState(rooms *RoomDirectoryProjection, groups *RoomGroupLayoutProjection, rbac *RBACProjection) ([]byte, error) {
	roomData, err := rooms.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("snapshot room visibility: %w", err)
	}
	groupData, err := groups.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("snapshot room-group visibility: %w", err)
	}
	rbacData, err := rbac.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("snapshot RBAC visibility: %w", err)
	}
	snapshot := &corev1.NotificationVisibilityProjectionSnapshot{
		RoomDirectory:   &corev1.RoomDirectoryProjectionSnapshot{},
		RoomGroupLayout: &corev1.RoomGroupLayoutProjectionSnapshot{},
		Rbac:            &corev1.RBACProjectionSnapshot{},
	}
	if err := proto.Unmarshal(roomData, snapshot.RoomDirectory); err != nil {
		return nil, fmt.Errorf("decode room visibility snapshot: %w", err)
	}
	if err := proto.Unmarshal(groupData, snapshot.RoomGroupLayout); err != nil {
		return nil, fmt.Errorf("decode room-group visibility snapshot: %w", err)
	}
	if err := proto.Unmarshal(rbacData, snapshot.Rbac); err != nil {
		return nil, fmt.Errorf("decode RBAC visibility snapshot: %w", err)
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func decodeNotificationVisibilityState(data []byte) (*RoomDirectoryProjection, *RoomGroupLayoutProjection, *RBACProjection, error) {
	snapshot := &corev1.NotificationVisibilityProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return nil, nil, nil, fmt.Errorf("unmarshal notification visibility snapshot: %w", err)
		}
	}
	rooms := NewRoomDirectoryProjection()
	groups := NewRoomGroupLayoutProjection()
	rbac := NewRBACProjection()
	marshalRestore := func(value proto.Message, restore func([]byte) error) error {
		payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(value)
		if err != nil {
			return err
		}
		return restore(payload)
	}
	if err := marshalRestore(snapshot.GetRoomDirectory(), rooms.Restore); err != nil {
		return nil, nil, nil, fmt.Errorf("restore room visibility: %w", err)
	}
	if err := marshalRestore(snapshot.GetRoomGroupLayout(), groups.Restore); err != nil {
		return nil, nil, nil, fmt.Errorf("restore room-group visibility: %w", err)
	}
	if err := marshalRestore(snapshot.GetRbac(), rbac.Restore); err != nil {
		return nil, nil, nil, fmt.Errorf("restore RBAC visibility: %w", err)
	}
	return rooms, groups, rbac, nil
}

// SetRestoreMaxCutoff binds snapshot restore to the notification consumer's
// acknowledged floor. Pending deliveries are replayed into exact boundary
// snapshots instead of being hidden behind a newer projection snapshot.
func (p *NotificationVisibilityProjection) SetRestoreMaxCutoff(sequence uint64) {
	p.retainAfter.Store(sequence)
}

func (p *NotificationVisibilityProjection) RestoreMaxCutoff() uint64 {
	return p.retainAfter.Load()
}

func (p *NotificationVisibilityProjection) Boundary(sequence uint64, at time.Time) (*notificationVisibilitySnapshot, error) {
	p.mu.RLock()
	payload := append([]byte(nil), p.boundaries[sequence]...)
	p.mu.RUnlock()
	if len(payload) == 0 {
		return nil, fmt.Errorf("notification visibility boundary %d is unavailable", sequence)
	}
	rooms, groups, rbac, err := decodeNotificationVisibilityState(payload)
	if err != nil {
		return nil, fmt.Errorf("restore notification visibility boundary %d: %w", sequence, err)
	}
	return &notificationVisibilitySnapshot{rooms: rooms, groups: groups, rbac: rbac, at: at}, nil
}

func (p *NotificationVisibilityProjection) ReleaseThrough(sequence uint64) {
	p.mu.Lock()
	for boundary := range p.boundaries {
		if boundary <= sequence {
			delete(p.boundaries, boundary)
		}
	}
	p.mu.Unlock()
}

func (p *NotificationVisibilityProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	roomEntries, roomBytes, roomMetrics := p.rooms.adminProjectionEstimate()
	groupEntries, groupBytes, groupMetrics := p.groups.adminProjectionEstimate()
	rbacEntries, rbacBytes, rbacMetrics := p.rbac.adminProjectionEstimate()
	metrics := append(roomMetrics, groupMetrics...)
	metrics = append(metrics, rbacMetrics...)
	return roomEntries + groupEntries + rbacEntries, roomBytes + groupBytes + rbacBytes, metrics
}

// cappedNotificationVisibilitySnapshotSource prevents projection restore from
// advancing beyond the shared worker's acknowledged floor.
type cappedNotificationVisibilitySnapshotSource struct {
	source     events.ProjectionSnapshotSource
	projection *NotificationVisibilityProjection
}

func (s cappedNotificationVisibilitySnapshotSource) LoadProjectionSnapshot(ctx context.Context, request events.ProjectionSnapshotLoadRequest) (events.ProjectionSnapshot, error) {
	if cutoff := s.projection.RestoreMaxCutoff(); cutoff < request.MaxCutoff {
		request.MaxCutoff = cutoff
	}
	return s.source.LoadProjectionSnapshot(ctx, request)
}

type notificationVisibilitySnapshot struct {
	rooms  *RoomDirectoryProjection
	groups *RoomGroupLayoutProjection
	rbac   *RBACProjection
	at     time.Time
}

func (s *notificationVisibilitySnapshot) membershipExists(userID, roomID string) bool {
	if s.rooms.Membership.IsMember(roomID, userID) {
		return true
	}
	room, exists := s.rooms.Catalog.Get(roomID)
	if !exists || room.GetKind() != corev1.RoomKind_ROOM_KIND_CHANNEL || !room.GetUniversal() {
		return false
	}
	if s.rooms.Bans.IsActive(roomID, userID, s.at) {
		return false
	}
	return s.roomJoinAllowed(userID, roomID, s.groups.Groups.GroupForRoom(roomID))
}

func (s *notificationVisibilitySnapshot) roomJoinAllowed(userID, roomID, groupID string) bool {
	if s.rbac.HasRole(userID, RoleOwner) {
		return true
	}
	scopes := []permissionScopeTarget{{scope: ScopeRoom, level: LevelRoom, id: roomID}}
	if groupID != "" {
		scopes = append(scopes, permissionScopeTarget{scope: ScopeGroup, level: LevelGroup, id: groupID})
	}
	scopes = append(scopes, permissionScopeTarget{scope: ScopeServer, level: LevelServer})

	nearest := func(subject string) (TraceEntry, bool) {
		for _, target := range scopes {
			decision := s.rbac.GetDecision(target.scope, target.id, subject, PermRoomJoin)
			if decision != DecisionNone {
				return TraceEntry{Level: target.level, RoleName: subject, Decision: decision, ObjectID: target.objectID()}, true
			}
		}
		return TraceEntry{}, false
	}

	var decisions applicablePermissionDecisions
	for _, subject := range append([]string{userID}, s.rbac.GetUserRoles(userID)...) {
		if entry, ok := nearest(subject); ok {
			decisions.named = append(decisions.named, entry)
		}
	}
	if entry, ok := nearest(RoleEveryone); ok {
		decisions.everyone = &entry
	}
	decision, _, _ := resolveApplicablePermissionDecisions(decisions)
	return decision == DecisionAllow
}
