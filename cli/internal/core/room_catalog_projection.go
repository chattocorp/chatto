package core

import (
	"strings"

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
	events.MemoryProjection
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

// Subjects implements events.Projection. The catalog is a room-derived read
// model, so it subscribes to the room aggregate namespace and ignores room
// events it does not handle.
func (p *RoomCatalogProjection) Subjects() []string {
	return []string{events.RoomSubjectFilter()}
}

// Apply implements events.Projection.
//
// Recognised events: RoomCreated, RoomUpdated (rename + description),
// RoomArchived, RoomUnarchived, RoomDeleted. Membership events
// (UserJoinedRoom, UserLeftRoom) and any other variants under
// evt.room.> are silently ignored.
func (p *RoomCatalogProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	switch e := event.GetEvent().(type) {
	case *corev1.Event_RoomCreated:
		c := e.RoomCreated
		p.rooms[c.GetRoomId()] = &roomCatalogEntry{
			name:        c.GetName(),
			description: c.GetDescription(),
			kind:        c.GetKind(),
		}
	case *corev1.Event_RoomUpdated:
		u := e.RoomUpdated
		if entry := p.rooms[u.GetRoomId()]; entry != nil {
			entry.name = u.GetName()
			entry.description = u.GetDescription()
		}
	case *corev1.Event_RoomArchived:
		if entry := p.rooms[e.RoomArchived.GetRoomId()]; entry != nil {
			entry.archived = true
		}
	case *corev1.Event_RoomUnarchived:
		if entry := p.rooms[e.RoomUnarchived.GetRoomId()]; entry != nil {
			entry.archived = false
		}
	case *corev1.Event_RoomDeleted:
		delete(p.rooms, e.RoomDeleted.GetRoomId())
	}
	return nil
}

// Get returns the room's metadata, or (nil, false) if no such room
// has been projected. The returned proto is a fresh value; callers
// may mutate it freely.
func (p *RoomCatalogProjection) Get(roomID string) (*corev1.Room, bool) {
	p.RLock()
	defer p.RUnlock()
	entry, ok := p.rooms[roomID]
	if !ok {
		return nil, false
	}
	return entryToRoom(roomID, entry), true
}

// Exists reports whether the room is present in the catalog.
func (p *RoomCatalogProjection) Exists(roomID string) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.rooms[roomID]
	return ok
}

// AllByKind returns every room of the given kind. Order is
// unspecified; the caller sorts / joins with grouping info as needed.
// The returned protos are fresh values.
func (p *RoomCatalogProjection) AllByKind(kind corev1.RoomKind) []*corev1.Room {
	p.RLock()
	defer p.RUnlock()
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
	p.RLock()
	defer p.RUnlock()
	return len(p.rooms)
}

// FindByName returns the ID of the channel room currently holding
// the given name (case-insensitive, ignoring leading/trailing
// whitespace), or "" if no such room exists. Used by CreateRoom /
// UpdateRoom for the pre-publish uniqueness check.
//
// Channel-room only: DM rooms have empty names by convention.
// Includes archived rooms — operators must rename them before
// reclaiming the slot, matching the previous KV-index semantics.
func (p *RoomCatalogProjection) FindByName(name string) string {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return ""
	}
	p.RLock()
	defer p.RUnlock()
	for id, entry := range p.rooms {
		if entry.kind != corev1.RoomKind_ROOM_KIND_CHANNEL {
			continue
		}
		if strings.ToLower(entry.name) == target {
			return id
		}
	}
	return ""
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
