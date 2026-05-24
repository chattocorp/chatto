package core

import (
	"sync"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomCatalogProjection holds per-room metadata derived from
// evt.room.{R} events: id, name, description, kind, archived state,
// and creation timestamp. It does NOT track group assignment — that's
// the group aggregate's concern.
//
// The projection coexists with RoomMembershipProjection on the same
// subject family (evt.room.>); each ignores the other's event
// variants per the Projection.Apply forward-compat rule. This is the
// first concrete pair of projections on a shared filter — see the
// "single consumer per filter" out-of-scope note in ADR-033.
type RoomCatalogProjection struct {
	mu    sync.RWMutex
	rooms map[string]*roomCatalogEntry
}

// roomCatalogEntry is the in-memory shape held per room. Not exposed
// directly — callers go through Get() which clones into a *corev1.Room
// for type symmetry with the rest of the codebase.
type roomCatalogEntry struct {
	name        string
	description string
	kind        corev1.RoomKind
	archived    bool
}

// NewRoomCatalogProjection returns an empty projection.
func NewRoomCatalogProjection() *RoomCatalogProjection {
	return &RoomCatalogProjection{
		rooms: make(map[string]*roomCatalogEntry),
	}
}

// Subjects implements events.Projection.
func (p *RoomCatalogProjection) Subjects() []string {
	return []string{events.RoomSubjectFilter()}
}

// Apply implements events.Projection. Apply runs from a single
// goroutine in stream order, so the write path doesn't lock for
// ordering — it locks to publish to concurrent readers.
//
// Recognised events: RoomCreated, RoomUpdated (rename + description),
// RoomArchived, RoomUnarchived, RoomDeleted. Membership events
// (UserJoinedRoom, UserLeftRoom) and any other variants under
// evt.room.> are silently ignored.
func (p *RoomCatalogProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	switch e := event.GetEvent().(type) {
	case *corev1.Event_RoomCreated:
		c := e.RoomCreated
		p.mu.Lock()
		p.rooms[c.GetRoomId()] = &roomCatalogEntry{
			name:        c.GetName(),
			description: c.GetDescription(),
			kind:        c.GetKind(),
		}
		p.mu.Unlock()

	case *corev1.Event_RoomUpdated:
		u := e.RoomUpdated
		p.mu.Lock()
		if entry := p.rooms[u.GetRoomId()]; entry != nil {
			entry.name = u.GetName()
			entry.description = u.GetDescription()
		}
		p.mu.Unlock()

	case *corev1.Event_RoomArchived:
		p.mu.Lock()
		if entry := p.rooms[e.RoomArchived.GetRoomId()]; entry != nil {
			entry.archived = true
		}
		p.mu.Unlock()

	case *corev1.Event_RoomUnarchived:
		p.mu.Lock()
		if entry := p.rooms[e.RoomUnarchived.GetRoomId()]; entry != nil {
			entry.archived = false
		}
		p.mu.Unlock()

	case *corev1.Event_RoomDeleted:
		p.mu.Lock()
		delete(p.rooms, e.RoomDeleted.GetRoomId())
		p.mu.Unlock()
	}
	return nil
}

// Snapshot implements events.Projection (deferred per ADR-033).
func (p *RoomCatalogProjection) Snapshot() ([]byte, error) { return nil, nil }

// Restore implements events.Projection (deferred per ADR-033).
func (p *RoomCatalogProjection) Restore(_ []byte) error { return nil }

// Get returns the room's metadata, or (nil, false) if no such room
// has been projected. The returned proto is a fresh value; callers
// may mutate it freely.
func (p *RoomCatalogProjection) Get(roomID string) (*corev1.Room, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	entry, ok := p.rooms[roomID]
	if !ok {
		return nil, false
	}
	return entryToRoom(roomID, entry), true
}

// Exists reports whether the room is present in the catalog.
func (p *RoomCatalogProjection) Exists(roomID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.rooms[roomID]
	return ok
}

// AllByKind returns every room of the given kind. Order is
// unspecified; the caller sorts / joins with grouping info as needed.
// The returned protos are fresh values.
func (p *RoomCatalogProjection) AllByKind(kind corev1.RoomKind) []*corev1.Room {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*corev1.Room, 0)
	for id, entry := range p.rooms {
		if entry.kind == kind {
			out = append(out, entryToRoom(id, entry))
		}
	}
	return out
}

// Count returns the number of rooms in the catalog. Useful for
// admin/diagnostic surfaces.
func (p *RoomCatalogProjection) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.rooms)
}

// entryToRoom converts a private catalog entry into the public
// *corev1.Room shape, so consumers don't depend on the internal
// representation. group_id is intentionally left unset — group
// assignment lives in RoomGroupProjection.
func entryToRoom(id string, entry *roomCatalogEntry) *corev1.Room {
	r := &corev1.Room{
		Id:          id,
		Name:        entry.name,
		Description: entry.description,
		Archived:    entry.archived,
		Kind:        entry.kind,
	}
	// Defensive clone — the proto contains a Mutex internally that
	// vet would flag if we ever returned the same pointer twice and
	// it got passed by value. Cheap insurance.
	return proto.Clone(r).(*corev1.Room)
}
