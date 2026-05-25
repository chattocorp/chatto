package core

import (
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomTimelineProjection holds an append-only event log per room.
//
// It consumes the full evt.room.> firehose — every event under any
// room aggregate (room lifecycle, memberships, messages, edits,
// retracts) lands in the owning room's slice in stream order. This
// is the v1 shape for the messages migration (issue #597): dead
// simple, no fold logic, no in-place mutation. Resolvers walk the
// slice and decide how to render — fold edits onto their original
// post, mark retracted entries, merge meta + message events, filter
// thread replies out of the channel view, etc.
//
// We will iterate on this significantly (RAM-bounded windows,
// derived caches for current-state lookups, etc.) once the read
// patterns are observed against real data. For now: one slice per
// room, full *corev1.Event protos preserved, every event indexed by
// envelope id for direct lookup.
type RoomTimelineProjection struct {
	events.MemoryProjection
	byRoom    map[string][]*TimelineEntry
	byEventID map[string]*TimelineEntry
}

// TimelineEntry is one event's position in a room timeline. Carries
// the full event proto verbatim — payload, envelope, actor,
// created_at, oneof variant — so resolvers don't need to consult
// the projection's internal state to render.
type TimelineEntry struct {
	StreamSeq uint64
	Event     *corev1.Event
}

// NewRoomTimelineProjection returns an empty projection.
func NewRoomTimelineProjection() *RoomTimelineProjection {
	return &RoomTimelineProjection{
		byRoom:    make(map[string][]*TimelineEntry),
		byEventID: make(map[string]*TimelineEntry),
	}
}

// Subjects implements events.Projection. The wildcard filter consumes
// every event under the room aggregate — the projection owns the
// "everything that happened in this room" surface, so it has to see
// all of it.
func (p *RoomTimelineProjection) Subjects() []string {
	return []string{events.RoomSubjectFilter()}
}

// Apply implements events.Projection. Extracts the room_id from
// whichever room-scoped event variant we recognise and appends an
// entry to that room's slice. Events that don't carry a room_id
// (shouldn't appear on evt.room.>, but defensive) are silently
// skipped — projections forward-compat by ignoring what they don't
// understand.
func (p *RoomTimelineProjection) Apply(event *corev1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	roomID := roomIDOfEvent(event)
	if roomID == "" {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	// Idempotency: a re-applied event with the same envelope id is a
	// no-op. The Projection.Apply contract is "Apply(e,n) twice ==
	// Apply(e,n) once"; this is how we honour it.
	if eid := event.GetId(); eid != "" {
		if _, exists := p.byEventID[eid]; exists {
			return nil
		}
	}

	entry := &TimelineEntry{StreamSeq: seq, Event: event}
	p.byRoom[roomID] = append(p.byRoom[roomID], entry)
	if eid := event.GetId(); eid != "" {
		p.byEventID[eid] = entry
	}
	return nil
}

// RoomEvents returns up to `limit` entries from a room's timeline in
// newest-first order, optionally bounded by an exclusive
// stream-sequence cursor (beforeStreamSeq == 0 means "from the
// newest"). Returns a fresh slice; callers may mutate freely.
//
// Entries are the raw timeline — no filtering of meta vs message vs
// thread reply, no fold of edits, no tombstone hiding. Resolvers
// pick what to surface.
func (p *RoomTimelineProjection) RoomEvents(roomID string, limit int, beforeStreamSeq uint64) []*TimelineEntry {
	if limit <= 0 {
		return nil
	}
	p.RLock()
	defer p.RUnlock()
	entries := p.byRoom[roomID]
	if len(entries) == 0 {
		return nil
	}
	out := make([]*TimelineEntry, 0, limit)
	for i := len(entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := entries[i]
		if beforeStreamSeq > 0 && e.StreamSeq >= beforeStreamSeq {
			continue
		}
		out = append(out, e)
	}
	return out
}

// RoomEventCount returns the total number of timeline entries in the
// room. Used by the future resolver's small-room fast-path
// equivalent.
func (p *RoomTimelineProjection) RoomEventCount(roomID string) int {
	p.RLock()
	defer p.RUnlock()
	return len(p.byRoom[roomID])
}

// Get returns a single timeline entry by its envelope id, or
// (nil, false) if no such event has been projected.
func (p *RoomTimelineProjection) Get(eventID string) (*TimelineEntry, bool) {
	p.RLock()
	defer p.RUnlock()
	e, ok := p.byEventID[eventID]
	return e, ok
}

// LatestBody returns the current body for a message, folding any
// subsequent MessageEditedEvent / MessageRetractedEvent entries
// targeting the message's event_id onto its original
// MessagePostedEvent.body. Returns (nil, true, true) if the
// message has been retracted; (nil, false, false) if the event_id
// doesn't refer to a known posted message; (body, false, true) for
// a live message (possibly edited).
//
// O(room timeline length) per call — fine for v1 with small dev
// data, gets a derived-cache treatment when read patterns warrant
// it.
func (p *RoomTimelineProjection) LatestBody(eventID string) (body *corev1.MessageBody, retracted bool, ok bool) {
	p.RLock()
	defer p.RUnlock()

	origEntry, exists := p.byEventID[eventID]
	if !exists {
		return nil, false, false
	}
	origPost := origEntry.Event.GetMessagePosted()
	if origPost == nil {
		// Looked-up envelope is something other than a post (e.g. a
		// MessageEdited envelope id passed in by mistake). Not a
		// valid "message" target.
		return nil, false, false
	}

	roomID := origPost.GetRoomId()
	current := origPost.GetBody()

	for _, e := range p.byRoom[roomID] {
		if e.StreamSeq <= origEntry.StreamSeq {
			continue
		}
		switch ev := e.Event.GetEvent().(type) {
		case *corev1.Event_MessageEdited:
			if ev.MessageEdited.GetEventId() == eventID {
				current = ev.MessageEdited.GetBody()
				retracted = false
			}
		case *corev1.Event_MessageRetracted:
			if ev.MessageRetracted.GetEventId() == eventID {
				current = nil
				retracted = true
			}
		}
	}
	return current, retracted, true
}

// roomIDOfEvent extracts the room_id from any room-scoped event
// variant. Returns "" for non-room events.
//
// Kept as a free function rather than a method on Event so the
// switch lives next to its sole consumer — easier to spot when a
// new room-scoped event type is added and this list needs an
// extension.
func roomIDOfEvent(event *corev1.Event) string {
	if event == nil {
		return ""
	}
	switch e := event.GetEvent().(type) {
	case *corev1.Event_RoomCreated:
		return e.RoomCreated.GetRoomId()
	case *corev1.Event_RoomUpdated:
		return e.RoomUpdated.GetRoomId()
	case *corev1.Event_RoomDeleted:
		return e.RoomDeleted.GetRoomId()
	case *corev1.Event_RoomArchived:
		return e.RoomArchived.GetRoomId()
	case *corev1.Event_RoomUnarchived:
		return e.RoomUnarchived.GetRoomId()
	case *corev1.Event_UserJoinedRoom:
		return e.UserJoinedRoom.GetRoomId()
	case *corev1.Event_UserLeftRoom:
		return e.UserLeftRoom.GetRoomId()
	case *corev1.Event_MessagePosted:
		return e.MessagePosted.GetRoomId()
	case *corev1.Event_MessageEdited:
		return e.MessageEdited.GetRoomId()
	case *corev1.Event_MessageRetracted:
		return e.MessageRetracted.GetRoomId()
	}
	return ""
}
