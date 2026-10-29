package core

import (
	"github.com/chattocorp/chatto/cli/internal/readmodel"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
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
	seq   uint64
}

type RoomNameClaimSnapshot struct {
	ConflictingRoomID string
	Seq               uint64
}

// roomCatalogEntry is the in-memory shape held per room. Not exposed
// directly — callers go through Get() which clones into a *readmodel.Room
// for type symmetry with the rest of the codebase.
type roomCatalogEntry struct {
	// Immutable provenance from the RoomCreated event envelope.
	createdAt             *timestamppb.Timestamp
	name                  string
	description           string
	purpose               string
	kind                  evtv1.RoomKind
	archived              bool
	universal             bool
	slowModeSeconds       uint32
	threadSlowModeSeconds *uint32
	threadingMode         evtv1.RoomThreadingMode
	showMembershipEvents  bool
	notice                *readmodel.RoomNotice
}

// NewRoomCatalogProjection returns an empty projection.
func NewRoomCatalogProjection() *RoomCatalogProjection {
	return &RoomCatalogProjection{
		rooms: make(map[string]*roomCatalogEntry),
	}
}

// Subjects implements evtstream.Projection. The catalog is a room-derived read
// model, so it subscribes to the room aggregate namespace and ignores room
// events it does not handle.
func (p *RoomCatalogProjection) Subjects() []string {
	return []string{evtstream.RoomSubjectFilter()}
}

// Apply implements evtstream.Projection.
//
// Recognised events: RoomCreated, RoomUpdated (rename + description),
// RoomArchived, RoomUnarchived, RoomDeleted. Membership events
// (UserJoinedRoom, UserLeftRoom) and any other variants under
// evt.room.> are silently ignored.
func (p *RoomCatalogProjection) Apply(event *evtv1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	if seq > p.seq {
		p.seq = seq
	}
	switch e := event.GetEvent().(type) {
	case *evtv1.Event_RoomCreated:
		c := e.RoomCreated
		p.rooms[c.GetRoomId()] = &roomCatalogEntry{
			createdAt:            proto.Clone(event.GetCreatedAt()).(*timestamppb.Timestamp),
			name:                 c.GetName(),
			description:          c.GetDescription(),
			purpose:              c.GetPurpose(),
			kind:                 c.GetKind(),
			universal:            c.GetUniversal(),
			threadingMode:        c.GetThreadingMode(),
			showMembershipEvents: true,
		}
	case *evtv1.Event_RoomUpdated:
		u := e.RoomUpdated
		if entry := p.rooms[u.GetRoomId()]; entry != nil {
			entry.name = u.GetName()
			entry.description = u.GetDescription()
			if u.Purpose != nil {
				entry.purpose = u.GetPurpose()
			}
		}
	case *evtv1.Event_RoomNoticeChanged:
		if entry := p.rooms[e.RoomNoticeChanged.GetRoomId()]; entry != nil {
			entry.notice = noticeAfterChange(entry.notice, event)
		}
	case *evtv1.Event_RoomArchived:
		if entry := p.rooms[e.RoomArchived.GetRoomId()]; entry != nil {
			entry.archived = true
		}
	case *evtv1.Event_RoomUnarchived:
		if entry := p.rooms[e.RoomUnarchived.GetRoomId()]; entry != nil {
			entry.archived = false
		}
	case *evtv1.Event_RoomUniversalChanged:
		if entry := p.rooms[e.RoomUniversalChanged.GetRoomId()]; entry != nil {
			entry.universal = e.RoomUniversalChanged.GetUniversal()
		}
	case *evtv1.Event_RoomSlowModeChanged:
		if entry := p.rooms[e.RoomSlowModeChanged.GetRoomId()]; entry != nil {
			entry.slowModeSeconds = e.RoomSlowModeChanged.GetSlowModeSeconds()
			entry.threadSlowModeSeconds = e.RoomSlowModeChanged.ThreadSlowModeSeconds
		}
	case *evtv1.Event_RoomThreadingModeChanged:
		if entry := p.rooms[e.RoomThreadingModeChanged.GetRoomId()]; entry != nil {
			entry.threadingMode = e.RoomThreadingModeChanged.GetThreadingMode()
		}
	case *evtv1.Event_RoomMembershipEventsVisibilityChanged:
		if entry := p.rooms[e.RoomMembershipEventsVisibilityChanged.GetRoomId()]; entry != nil {
			entry.showMembershipEvents = e.RoomMembershipEventsVisibilityChanged.GetShowMembershipEvents()
		}
	case *evtv1.Event_RoomDeleted:
		delete(p.rooms, e.RoomDeleted.GetRoomId())
	}
	return nil
}

// Get returns the room's metadata, or (nil, false) if no such room
// has been projected. The returned model is a fresh value; callers
// may mutate it freely.
func (p *RoomCatalogProjection) Get(roomID string) (*readmodel.Room, bool) {
	p.RLock()
	defer p.RUnlock()
	entry, ok := p.rooms[roomID]
	if !ok {
		return nil, false
	}
	return entryToRoom(roomID, entry), true
}

// Kind returns the room's kind, or (ROOM_KIND_UNSPECIFIED, false) if no such
// room has been projected.
func (p *RoomCatalogProjection) Kind(roomID string) (evtv1.RoomKind, bool) {
	p.RLock()
	defer p.RUnlock()
	entry, ok := p.rooms[roomID]
	if !ok {
		return evtv1.RoomKind_ROOM_KIND_UNSPECIFIED, false
	}
	return entry.kind, true
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
// The returned models are fresh values.
func (p *RoomCatalogProjection) AllByKind(kind evtv1.RoomKind) []*readmodel.Room {
	p.RLock()
	defer p.RUnlock()
	out := make([]*readmodel.Room, 0)
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

// FindByName returns the ID of a channel room with an equivalent normalized,
// case-folded name, or "" if no such room exists.
//
// Channel-room only: DM rooms have empty names by convention.
// Includes archived rooms — operators must rename them before
// reclaiming the slot, matching the previous KV-index semantics.
func (p *RoomCatalogProjection) FindByName(name string) string {
	return p.NameClaimSnapshot(name, "").ConflictingRoomID
}

// NameClaimSnapshot returns the room-directory sequence and any room other
// than excludeRoomID whose derived comparison key matches name. Inspecting all
// entries is important when stronger Unicode comparison reveals collisions in
// data written by an older release.
func (p *RoomCatalogProjection) NameClaimSnapshot(name, excludeRoomID string) RoomNameClaimSnapshot {
	target := canonicalRoomName(name)
	if target == "" {
		return RoomNameClaimSnapshot{}
	}
	p.RLock()
	defer p.RUnlock()
	snapshot := RoomNameClaimSnapshot{Seq: p.seq}
	for id, entry := range p.rooms {
		if id == excludeRoomID || entry.kind != evtv1.RoomKind_ROOM_KIND_CHANNEL {
			continue
		}
		if canonicalRoomName(entry.name) == target {
			snapshot.ConflictingRoomID = id
			return snapshot
		}
	}
	return snapshot
}

// entryToRoom returns a detached read model. GroupId stays unset because
// group assignment belongs to RoomGroupProjection.
func entryToRoom(id string, entry *roomCatalogEntry) *readmodel.Room {
	r := &readmodel.Room{
		CreatedAt:             entry.createdAt,
		Notice:                entry.notice,
		Id:                    id,
		Name:                  entry.name,
		Description:           entry.description,
		Purpose:               entry.purpose,
		Archived:              entry.archived,
		Kind:                  entry.kind,
		Universal:             entry.universal,
		SlowModeSeconds:       entry.slowModeSeconds,
		ThreadSlowModeSeconds: entry.threadSlowModeSeconds,
		ThreadingMode:         normalizedRoomThreadingMode(entry.kind, entry.threadingMode),
		ShowMembershipEvents:  entry.showMembershipEvents,
	}
	// Callers can change nested timestamps, notice state, and optional values
	// without changing the projection.
	return r.Clone()
}
