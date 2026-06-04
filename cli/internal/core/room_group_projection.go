package core

import (
	"slices"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomGroupProjection holds the current set of room groups derived
// from evt.group.{G} events: id, name, description, and the ordered
// list of room IDs in the group. The group aggregate owns both
// "what rooms are in me" and "in what order" — see ADR-034 / the
// design discussion accompanying the room-metadata + room-group
// migration PR.
//
// Move-room operations land as two events (one per affected group),
// matching the per-aggregate cascade rule from ADR-034 Approach A.
type RoomGroupProjection struct {
	events.MemoryProjection
	groups map[string]*roomGroupEntry
}

type roomGroupEntry struct {
	name        string
	description string
	roomIDs     []string
}

// NewRoomGroupProjection returns an empty projection.
func NewRoomGroupProjection() *RoomGroupProjection {
	return &RoomGroupProjection{
		groups: make(map[string]*roomGroupEntry),
	}
}

// Subjects implements events.Projection. Room groups are a group-derived read
// model, so the projection subscribes to the group aggregate namespace and
// ignores group events it does not handle.
func (p *RoomGroupProjection) Subjects() []string {
	return []string{events.GroupSubjectFilter()}
}

// Apply implements events.Projection. Recognised events:
// RoomGroupCreated, RoomGroupUpdated, RoomGroupDeleted,
// RoomAddedToGroup, RoomRemovedFromGroup, RoomsInGroupReordered.
// Unrecognised variants are silently ignored.
func (p *RoomGroupProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	switch e := event.GetEvent().(type) {
	case *corev1.Event_RoomGroupCreated:
		c := e.RoomGroupCreated
		// Idempotent: re-creating an existing group overwrites
		// metadata but preserves room membership. In practice the
		// Append OCC scope prevents re-creation; this is defensive.
		entry := p.groups[c.GetGroupId()]
		if entry == nil {
			entry = &roomGroupEntry{}
			p.groups[c.GetGroupId()] = entry
		}
		entry.name = c.GetName()
		entry.description = c.GetDescription()

	case *corev1.Event_RoomGroupUpdated:
		u := e.RoomGroupUpdated
		if entry := p.groups[u.GetGroupId()]; entry != nil {
			entry.name = u.GetName()
			entry.description = u.GetDescription()
		}

	case *corev1.Event_RoomGroupDeleted:
		delete(p.groups, e.RoomGroupDeleted.GetGroupId())

	case *corev1.Event_RoomAddedToGroup:
		a := e.RoomAddedToGroup
		if entry := p.groups[a.GetGroupId()]; entry != nil {
			if !slices.Contains(entry.roomIDs, a.GetRoomId()) {
				entry.roomIDs = append(entry.roomIDs, a.GetRoomId())
			}
		}

	case *corev1.Event_RoomRemovedFromGroup:
		r := e.RoomRemovedFromGroup
		if entry := p.groups[r.GetGroupId()]; entry != nil {
			entry.roomIDs = slices.DeleteFunc(entry.roomIDs, func(id string) bool {
				return id == r.GetRoomId()
			})
		}

	case *corev1.Event_RoomsInGroupReordered:
		r := e.RoomsInGroupReordered
		if entry := p.groups[r.GetGroupId()]; entry != nil {
			entry.roomIDs = slices.Clone(r.GetRoomIds())
		}
	}
	return nil
}

// Get returns the group's data, or (nil, false) if no such group has
// been projected. The returned proto is a fresh value — including a
// cloned room_ids slice — so callers may mutate freely.
func (p *RoomGroupProjection) Get(groupID string) (*corev1.RoomGroup, bool) {
	p.RLock()
	defer p.RUnlock()
	entry, ok := p.groups[groupID]
	if !ok {
		return nil, false
	}
	return entryToGroup(groupID, entry), true
}

// Exists reports whether the group is in the projection.
func (p *RoomGroupProjection) Exists(groupID string) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.groups[groupID]
	return ok
}

// All returns every group in the projection. Order is unspecified;
// the layout aggregate (KV-backed for now) provides the operator-
// preferred sort. Returned protos are fresh values.
func (p *RoomGroupProjection) All() []*corev1.RoomGroup {
	p.RLock()
	defer p.RUnlock()
	out := make([]*corev1.RoomGroup, 0, len(p.groups))
	for id, entry := range p.groups {
		out = append(out, entryToGroup(id, entry))
	}
	return out
}

// GroupForRoom returns the group ID that currently contains the
// given room, or "" if the room isn't in any group. Linear scan;
// fine for the small group counts we expect on a server.
func (p *RoomGroupProjection) GroupForRoom(roomID string) string {
	p.RLock()
	defer p.RUnlock()
	for groupID, entry := range p.groups {
		if slices.Contains(entry.roomIDs, roomID) {
			return groupID
		}
	}
	return ""
}

// Count returns the number of groups projected. Useful for
// admin/diagnostic surfaces.
func (p *RoomGroupProjection) Count() int {
	p.RLock()
	defer p.RUnlock()
	return len(p.groups)
}

// entryToGroup builds a public *corev1.RoomGroup from the private
// entry, including a fresh room_ids slice.
func entryToGroup(id string, entry *roomGroupEntry) *corev1.RoomGroup {
	return &corev1.RoomGroup{
		Id:          id,
		Name:        entry.name,
		Description: entry.description,
		RoomIds:     slices.Clone(entry.roomIDs),
	}
}
