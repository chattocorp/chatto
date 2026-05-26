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
	// latestBody is the derived current-body index. Updated as
	// MessageEdited / MessageRetracted entries are applied so that
	// LatestBody resolves in O(1) instead of an O(room size) walk
	// of byRoom. A nil entry means "retracted"; absent means "no
	// embedded body / not yet projected".
	latestBody     map[string]*corev1.MessageBody
	retractedFlags map[string]struct{}
	// echoLinks maps an original message's event_id to the event_ids
	// of any echoes pointing at it. Maintained as MessagePostedEvents
	// with EchoOfEventId arrive. Used by EditMessage / DeleteMessage
	// to fan mutations across linked messages — pre-cutover the
	// echo + original shared a messageBodyId, so an edit on either
	// updated both via the shared SERVER_BODIES entry; post-cutover
	// each has its own embedded body and we need explicit
	// propagation.
	echoLinks map[string][]string
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
		byRoom:         make(map[string][]*TimelineEntry),
		byEventID:      make(map[string]*TimelineEntry),
		latestBody:     make(map[string]*corev1.MessageBody),
		retractedFlags: make(map[string]struct{}),
		echoLinks:      make(map[string][]string),
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

	// Maintain the latest-body / retracted-flag derived index so
	// LatestBody is O(1) instead of an O(room) walk per lookup.
	switch ev := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		targetID := ev.MessagePosted.GetEventId()
		if targetID == "" {
			targetID = event.GetId()
		}
		if targetID != "" {
			p.latestBody[targetID] = ev.MessagePosted.GetBody()
			delete(p.retractedFlags, targetID)
		}
		// Track echo links so edits / retracts on either side can fan
		// out to the other.
		if origID := ev.MessagePosted.GetEchoOfEventId(); origID != "" && targetID != "" {
			p.echoLinks[origID] = append(p.echoLinks[origID], targetID)
		}
	case *corev1.Event_MessageEdited:
		targetID := ev.MessageEdited.GetEventId()
		if targetID != "" {
			p.latestBody[targetID] = ev.MessageEdited.GetBody()
			delete(p.retractedFlags, targetID)
		}
	case *corev1.Event_MessageRetracted:
		targetID := ev.MessageRetracted.GetEventId()
		if targetID != "" {
			delete(p.latestBody, targetID)
			p.retractedFlags[targetID] = struct{}{}
		}
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

// LatestBody returns the current body for a message — the original
// MessagePostedEvent.body overlaid with any subsequent
// MessageEditedEvent's body, or nil + retracted=true if a
// MessageRetractedEvent has landed.
//
// Returns (nil, false, false) if the event_id isn't known to the
// projection (caller can treat as "not found yet").
//
// O(1): consults the derived latestBody / retractedFlags indexes
// that Apply keeps in lockstep with byRoom.
func (p *RoomTimelineProjection) LatestBody(eventID string) (body *corev1.MessageBody, retracted bool, ok bool) {
	p.RLock()
	defer p.RUnlock()
	if eventID == "" {
		return nil, false, false
	}
	if _, exists := p.byEventID[eventID]; !exists {
		return nil, false, false
	}
	if _, isRetracted := p.retractedFlags[eventID]; isRetracted {
		return nil, true, true
	}
	if b, has := p.latestBody[eventID]; has {
		return b, false, true
	}
	return nil, false, true
}

// LinkedEventIDs returns the set of event_ids that an edit / retract
// targeting `eventID` should also be applied to: any echoes pointing
// at `eventID`, plus the original message that `eventID` is an echo
// of (if any). Does NOT include `eventID` itself — the caller emits
// the mutation for the target separately.
//
// Used by EditMessage / DeleteMessage to preserve the legacy "edit
// the echo, the original updates too (and vice versa)" semantic
// after the shared-messageBodyId mechanism was retired in #614.
func (p *RoomTimelineProjection) LinkedEventIDs(eventID string) []string {
	p.RLock()
	defer p.RUnlock()
	if eventID == "" {
		return nil
	}
	linked := make([]string, 0, 2)

	// Forward: echoes pointing at this event.
	for _, echoID := range p.echoLinks[eventID] {
		if echoID != eventID {
			linked = append(linked, echoID)
		}
	}

	// Backward: if this event IS an echo, include the original.
	if entry, ok := p.byEventID[eventID]; ok {
		if posted := entry.Event.GetMessagePosted(); posted != nil {
			if origID := posted.GetEchoOfEventId(); origID != "" && origID != eventID {
				linked = append(linked, origID)
				// Also include any sibling echoes of the same original
				// (rare, but possible if "also send to channel" was
				// invoked twice — keep semantics consistent).
				for _, siblingID := range p.echoLinks[origID] {
					if siblingID != eventID && siblingID != origID {
						linked = append(linked, siblingID)
					}
				}
			}
		}
	}
	return linked
}

// LastVisibleRoomEntry walks the room's timeline newest-first and
// returns the first entry that passes `visible`. Useful for
// "last root message", "last activity", and similar single-entry
// lookups that don't need to materialise a full slice. Returns
// (nil, false) if no entry matches.
func (p *RoomTimelineProjection) LastVisibleRoomEntry(
	roomID string,
	visible func(*corev1.Event) bool,
) (*TimelineEntry, bool) {
	p.RLock()
	defer p.RUnlock()
	entries := p.byRoom[roomID]
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if visible != nil && !visible(e.Event) {
			continue
		}
		return e, true
	}
	return nil, false
}

// VisibleRoomTimeline walks the room's timeline newest-first,
// applying `visible` as a per-entry filter, and returns up to
// `limit` matching entries. `beforeStreamSeq > 0` excludes entries
// with stream seq >= that value (exclusive upper bound for
// pagination).
//
// Stops as soon as `limit` visible entries are accumulated — no
// full-slice materialisation. Caller may inspect more than `limit`
// raw entries when the visibility filter rejects some of them
// (e.g. when filtering thread replies out of a channel timeline).
//
// Returns entries in newest-first order. Caller reverses to
// oldest-first if needed.
func (p *RoomTimelineProjection) VisibleRoomTimeline(
	roomID string,
	limit int,
	beforeStreamSeq uint64,
	visible func(*corev1.Event) bool,
) []*TimelineEntry {
	if limit <= 0 {
		return nil
	}
	p.RLock()
	defer p.RUnlock()
	entries := p.byRoom[roomID]
	out := make([]*TimelineEntry, 0, limit)
	for i := len(entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := entries[i]
		if beforeStreamSeq > 0 && e.StreamSeq >= beforeStreamSeq {
			continue
		}
		if visible != nil && !visible(e.Event) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// VisibleRoomTimelineAfter walks the room's timeline oldest-first,
// applying `visible` as a per-entry filter, and returns up to `limit`
// matching entries with stream seq > afterStreamSeq. This is the
// forward-pagination counterpart to VisibleRoomTimeline.
func (p *RoomTimelineProjection) VisibleRoomTimelineAfter(
	roomID string,
	limit int,
	afterStreamSeq uint64,
	visible func(*corev1.Event) bool,
) []*TimelineEntry {
	if limit <= 0 {
		return nil
	}
	p.RLock()
	defer p.RUnlock()
	entries := p.byRoom[roomID]
	out := make([]*TimelineEntry, 0, limit)
	for _, e := range entries {
		if e.StreamSeq <= afterStreamSeq {
			continue
		}
		if visible != nil && !visible(e.Event) {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out
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
