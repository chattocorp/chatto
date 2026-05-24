package core

import (
	"fmt"
	"sync"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomMembershipProjection is the first event-sourced projection (ADR-033).
// It consumes UserJoinedRoomEvent / UserLeftRoomEvent / RoomDeletedEvent
// from the EVT stream and maintains the current set of room
// memberships in memory.
//
// Two indices are kept so both "who's in this room?" and "what rooms is
// this user in?" are O(1) hash lookups. They stay in sync — neither
// drifts independently.
//
// Room kind ("channel" or "dm") is intentionally NOT tracked here. The
// subject scheme is `evt.room.{roomID}.{eventType}` — kind is a property
// of the room itself, not of any individual event. Kind-filtered
// membership queries (e.g. "list this user's DMs") still consult the
// Room KV during the transition; a follow-up can either add a small
// RoomKind projection or fold the lookup into the resolver layer.
type RoomMembershipProjection struct {
	mu sync.RWMutex
	// byRoom: room ID → set of user IDs in that room.
	byRoom map[string]map[string]struct{}
	// byUser: user ID → set of room IDs that user is in. Mirror of
	// byRoom, kept in sync.
	byUser map[string]map[string]struct{}
}

// NewRoomMembershipProjection returns an empty projection. Call Run on a
// Projector wrapping it to populate from the stream.
func NewRoomMembershipProjection() *RoomMembershipProjection {
	return &RoomMembershipProjection{
		byRoom: make(map[string]map[string]struct{}),
		byUser: make(map[string]map[string]struct{}),
	}
}

// Subjects implements events.Projection.
func (p *RoomMembershipProjection) Subjects() []string {
	return []string{events.RoomSubjectFilter()}
}

// Apply implements events.Projection. Apply is called from a single
// goroutine owned by the Projector, so the write path doesn't need to
// take the lock for ordering — but reads are concurrent, so writes
// still take the write lock to publish updated state.
func (p *RoomMembershipProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserJoinedRoom:
		roomID := e.UserJoinedRoom.GetRoomId()
		userID := event.GetActorId()
		if roomID == "" || userID == "" {
			return fmt.Errorf("UserJoinedRoom missing roomID or actorID")
		}
		p.add(roomID, userID)
	case *corev1.Event_UserLeftRoom:
		roomID := e.UserLeftRoom.GetRoomId()
		userID := event.GetActorId()
		if roomID == "" || userID == "" {
			return fmt.Errorf("UserLeftRoom missing roomID or actorID")
		}
		p.remove(roomID, userID)
	case *corev1.Event_RoomDeleted:
		roomID := e.RoomDeleted.GetRoomId()
		if roomID == "" {
			return fmt.Errorf("RoomDeleted missing roomID")
		}
		p.dropRoom(roomID)
	default:
		// Other event types may share the room aggregate subject in the
		// future; skipping them silently is the correct projection
		// behavior (apply what you understand, ignore the rest).
	}
	return nil
}

// Snapshot implements events.Projection. No snapshot support yet (ADR-033
// defers snapshot orchestration). Returning (nil, nil) tells the Projector
// to skip snapshot persistence.
func (p *RoomMembershipProjection) Snapshot() ([]byte, error) { return nil, nil }

// Restore implements events.Projection. No snapshot support yet, so this
// is a no-op. When snapshot orchestration lands this will deserialize the
// indices from `data`.
func (p *RoomMembershipProjection) Restore(_ []byte) error { return nil }

// add inserts a (room, user) membership. Idempotent.
func (p *RoomMembershipProjection) add(roomID, userID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	users, ok := p.byRoom[roomID]
	if !ok {
		users = make(map[string]struct{})
		p.byRoom[roomID] = users
	}
	users[userID] = struct{}{}

	rooms, ok := p.byUser[userID]
	if !ok {
		rooms = make(map[string]struct{})
		p.byUser[userID] = rooms
	}
	rooms[roomID] = struct{}{}
}

// dropRoom removes a room entirely from the projection — used when a
// RoomDeleted event arrives. All members of the room have their entry
// for this room cleared from byUser. Idempotent.
func (p *RoomMembershipProjection) dropRoom(roomID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	users := p.byRoom[roomID]
	if users == nil {
		return
	}
	for u := range users {
		if rooms, ok := p.byUser[u]; ok {
			delete(rooms, roomID)
			if len(rooms) == 0 {
				delete(p.byUser, u)
			}
		}
	}
	delete(p.byRoom, roomID)
}

// remove deletes a (room, user) membership. Idempotent.
func (p *RoomMembershipProjection) remove(roomID, userID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if users, ok := p.byRoom[roomID]; ok {
		delete(users, userID)
		if len(users) == 0 {
			delete(p.byRoom, roomID)
		}
	}
	if rooms, ok := p.byUser[userID]; ok {
		delete(rooms, roomID)
		if len(rooms) == 0 {
			delete(p.byUser, userID)
		}
	}
}

// IsMember reports whether the user is a member of the room.
func (p *RoomMembershipProjection) IsMember(roomID, userID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	users, ok := p.byRoom[roomID]
	if !ok {
		return false
	}
	_, ok = users[userID]
	return ok
}

// Members returns the user IDs of the room's current members. The returned
// slice is a copy; the caller may mutate it freely. Order is unspecified.
func (p *RoomMembershipProjection) Members(roomID string) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	users := p.byRoom[roomID]
	out := make([]string, 0, len(users))
	for u := range users {
		out = append(out, u)
	}
	return out
}

// Rooms returns the room IDs the user is currently a member of, across
// every kind. The returned slice is a copy; order is unspecified.
func (p *RoomMembershipProjection) Rooms(userID string) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	rooms := p.byUser[userID]
	out := make([]string, 0, len(rooms))
	for r := range rooms {
		out = append(out, r)
	}
	return out
}

// Stats returns counts useful for diagnostics. Intended for admin/dev
// endpoints rather than hot paths.
func (p *RoomMembershipProjection) Stats() (rooms int, memberships int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	rooms = len(p.byRoom)
	for _, users := range p.byRoom {
		memberships += len(users)
	}
	return rooms, memberships
}
