package core

import (
	"context"
	"fmt"
	"sort"
	"time"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// notificationVisibilitySnapshot reconstructs only the projections needed to
// decide effective room membership at an exact EVT boundary. Administrative
// visibility changes are rare, while using latest projections here would lose
// a revoke-then-restore transition whenever those projections outrun the
// notification worker.
type notificationVisibilitySnapshot struct {
	rooms  *RoomDirectoryProjection
	groups *RoomGroupLayoutProjection
	rbac   *RBACProjection
	at     time.Time
}

type notificationVisibilityProjectionEvent struct {
	sequence uint64
	event    *corev1.Event
}

func (m *NotificationMaterializer) visibilitySnapshotAt(ctx context.Context, boundary uint64, at time.Time) (*notificationVisibilitySnapshot, error) {
	rooms := NewRoomDirectoryProjection()
	groups := NewRoomGroupLayoutProjection()
	rbac := NewRBACProjection()

	roomEvents, err := m.notificationVisibilityEventsThrough(ctx, boundary, []string{
		evtstream.RoomEventTypeFilter(evtstream.EventRoomCreated),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomUniversalChanged),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomDeleted),
		evtstream.RoomEventTypeFilter(evtstream.EventUserJoinedRoom),
		evtstream.RoomEventTypeFilter(evtstream.EventUserLeftRoom),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomMemberBanned),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomMemberUnbanned),
	})
	if err != nil {
		return nil, fmt.Errorf("load room visibility history: %w", err)
	}
	for _, item := range roomEvents {
		if err := rooms.Apply(item.event, item.sequence); err != nil {
			return nil, fmt.Errorf("replay room visibility event at %d: %w", item.sequence, err)
		}
	}

	groupEvents, err := m.notificationVisibilityEventsThrough(ctx, boundary, []string{
		evtstream.GroupEventTypeFilter(evtstream.EventRoomGroupCreated),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomGroupDeleted),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomAddedToGroup),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomRemovedFromGroup),
	})
	if err != nil {
		return nil, fmt.Errorf("load room group visibility history: %w", err)
	}
	for _, item := range groupEvents {
		if err := groups.Apply(item.event, item.sequence); err != nil {
			return nil, fmt.Errorf("replay room group visibility event at %d: %w", item.sequence, err)
		}
	}

	rbacEvents, err := m.notificationVisibilityEventsThrough(ctx, boundary, []string{evtstream.RBACSubjectFilter()})
	if err != nil {
		return nil, fmt.Errorf("load RBAC visibility history: %w", err)
	}
	for _, item := range rbacEvents {
		if err := rbac.Apply(item.event, item.sequence); err != nil {
			return nil, fmt.Errorf("replay RBAC visibility event at %d: %w", item.sequence, err)
		}
	}

	return &notificationVisibilitySnapshot{rooms: rooms, groups: groups, rbac: rbac, at: at}, nil
}

func (m *NotificationMaterializer) notificationVisibilityEventsThrough(ctx context.Context, boundary uint64, filters []string) ([]notificationVisibilityProjectionEvent, error) {
	items := make([]notificationVisibilityProjectionEvent, 0)
	for _, filter := range filters {
		eventsOnSubject, _, err := m.core.EventPublisher.SubjectEventsWithSubjectsAfter(ctx, filter, 0)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", filter, err)
		}
		for _, event := range eventsOnSubject {
			if event.Sequence > boundary {
				continue
			}
			items = append(items, notificationVisibilityProjectionEvent{sequence: event.Sequence, event: event.Event})
		}
	}
	sort.Slice(items, func(a, b int) bool { return items[a].sequence < items[b].sequence })
	return items, nil
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
	scopes := []permissionScopeTarget{
		{scope: ScopeRoom, level: LevelRoom, id: roomID},
	}
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
