package core

import (
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ThreadProjection holds an append-only event log per thread,
// derived from the same evt.room.> firehose RoomTimelineProjection
// consumes.
//
// "Per thread" means: events whose semantic scope is a single
// thread — reply posts (MessagePostedEvent with in_thread != "")
// and edits / retracts targeting those replies. The thread root
// message itself is NOT stored here; the thread-view resolver
// fetches the root from RoomTimelineProjection.Get(rootEventID)
// and concatenates. This keeps each projection's "what's in here?"
// answer trivial.
//
// To route edits and retracts to the right thread, we maintain a
// secondary index mapping reply event_id → thread root event_id,
// populated as MessagePostedEvent replies arrive. Edits and
// retracts of root messages (which aren't in any thread bucket)
// are silently skipped here; they'll be handled at the room-
// timeline level.
//
// Same v1-shape framing as RoomTimelineProjection: dead simple,
// append-only, no fold logic, full event protos preserved. We
// iterate later.
type ThreadProjection struct {
	events.MemoryProjection
	byThread        map[string][]*TimelineEntry
	messageToThread map[string]string // reply event_id → thread root event_id
}

// NewThreadProjection returns an empty projection.
func NewThreadProjection() *ThreadProjection {
	return &ThreadProjection{
		byThread:        make(map[string][]*TimelineEntry),
		messageToThread: make(map[string]string),
	}
}

// Subjects implements events.Projection. Subscribes to the same
// firehose as RoomTimelineProjection — the apply switch picks out
// the variants this projection cares about.
func (p *ThreadProjection) Subjects() []string {
	return []string{events.RoomSubjectFilter()}
}

// Apply implements events.Projection.
//
// Recognised events:
//
//   - MessagePostedEvent with in_thread != "" → append to the
//     thread's slice, remember its event_id → thread mapping.
//   - MessageEditedEvent / MessageRetractedEvent whose target
//     event_id is a known thread reply → append to that thread.
//
// Everything else (root messages, room lifecycle, memberships,
// edits/retracts of non-reply messages) is silently ignored.
func (p *ThreadProjection) Apply(event *corev1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	// Idempotency: skip if we've already applied this envelope id.
	if eid := event.GetId(); eid != "" {
		for _, threadEntries := range p.byThread {
			for _, te := range threadEntries {
				if te.Event.GetId() == eid {
					return nil
				}
			}
		}
	}

	switch e := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		m := e.MessagePosted
		threadRoot := m.GetInThread()
		if threadRoot == "" {
			return nil // root-level message; not in any thread bucket
		}
		replyID := m.GetEventId()
		if replyID == "" {
			return nil
		}
		p.byThread[threadRoot] = append(p.byThread[threadRoot], &TimelineEntry{StreamSeq: seq, Event: event})
		p.messageToThread[replyID] = threadRoot

	case *corev1.Event_MessageEdited:
		threadRoot, ok := p.messageToThread[e.MessageEdited.GetEventId()]
		if !ok {
			return nil // target isn't a known thread reply
		}
		p.byThread[threadRoot] = append(p.byThread[threadRoot], &TimelineEntry{StreamSeq: seq, Event: event})

	case *corev1.Event_MessageRetracted:
		threadRoot, ok := p.messageToThread[e.MessageRetracted.GetEventId()]
		if !ok {
			return nil
		}
		p.byThread[threadRoot] = append(p.byThread[threadRoot], &TimelineEntry{StreamSeq: seq, Event: event})
	}
	return nil
}

// ThreadEvents returns the full timeline of a thread (replies +
// any edits / retracts targeting them) in stream order. Returns
// nil if no replies have landed.
//
// The root message is NOT included — resolvers fetch it from
// RoomTimelineProjection.Get(rootEventID) and prepend.
func (p *ThreadProjection) ThreadEvents(rootEventID string) []*TimelineEntry {
	p.RLock()
	defer p.RUnlock()
	entries := p.byThread[rootEventID]
	if len(entries) == 0 {
		return nil
	}
	out := make([]*TimelineEntry, len(entries))
	copy(out, entries)
	return out
}

// ReplyCount returns how many MessagePostedEvent replies the
// thread has accumulated. Edits and retracts don't bump the
// count.
func (p *ThreadProjection) ReplyCount(rootEventID string) int {
	p.RLock()
	defer p.RUnlock()
	n := 0
	for _, e := range p.byThread[rootEventID] {
		if _, ok := e.Event.GetEvent().(*corev1.Event_MessagePosted); ok {
			n++
		}
	}
	return n
}

// ThreadCount returns how many threads are currently in the
// projection. Diagnostics only.
func (p *ThreadProjection) ThreadCount() int {
	p.RLock()
	defer p.RUnlock()
	return len(p.byThread)
}
