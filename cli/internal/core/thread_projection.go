package core

import (
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type threadReplySummary struct {
	actorID   string
	createdAt time.Time
	retracted bool
}

type threadSummary struct {
	replyIDs       []string
	replyCount     int
	lastReplyAt    *time.Time
	participantIDs []string
}

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
	replySummaries  map[string]*threadReplySummary
	summaryByThread map[string]*threadSummary
	appliedEventIDs map[string]struct{}
	shreddedUsers   map[string]struct{}
}

// NewThreadProjection returns an empty projection.
func NewThreadProjection() *ThreadProjection {
	return &ThreadProjection{
		byThread:        make(map[string][]*TimelineEntry),
		messageToThread: make(map[string]string),
		replySummaries:  make(map[string]*threadReplySummary),
		summaryByThread: make(map[string]*threadSummary),
		appliedEventIDs: make(map[string]struct{}),
		shreddedUsers:   make(map[string]struct{}),
	}
}

// Subjects implements events.Projection. Threads are a room-derived view, so
// the projection subscribes to the room aggregate namespace plus the extra user
// key-shred events it needs.
func (p *ThreadProjection) Subjects() []string {
	return []string{events.RoomSubjectFilter(), events.UserEventTypeFilter(events.EventUserKeyShredded)}
}

// Apply implements events.Projection.
//
// Recognised events:
//
//   - MessagePostedEvent with in_thread != "" → append to the
//     thread's slice, remember its event_id → thread mapping.
//   - ThreadCreatedEvent → initialise the thread's bucket even before
//     replies land.
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

	eid := event.GetId()
	if eid != "" {
		if _, exists := p.appliedEventIDs[eid]; exists {
			return nil
		}
	}
	markApplied := func() {
		if eid != "" {
			p.appliedEventIDs[eid] = struct{}{}
		}
	}

	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserKeyShredded:
		if userID := e.UserKeyShredded.GetUserId(); userID != "" {
			p.shreddedUsers[userID] = struct{}{}
			for threadRoot := range p.summaryByThread {
				p.recomputeSummaryLocked(threadRoot)
			}
			markApplied()
		}

	case *corev1.Event_ThreadCreated:
		threadRoot := e.ThreadCreated.GetThreadRootEventId()
		if threadRoot == "" {
			return nil
		}
		if _, exists := p.byThread[threadRoot]; !exists {
			p.byThread[threadRoot] = nil
		}
		if _, exists := p.summaryByThread[threadRoot]; !exists {
			p.summaryByThread[threadRoot] = &threadSummary{}
		}
		markApplied()

	case *corev1.Event_MessagePosted:
		m := e.MessagePosted
		threadRoot := m.GetInThread()
		if threadRoot == "" {
			return nil // root-level message; not in any thread bucket
		}
		replyID := event.GetId()
		if replyID == "" {
			return nil
		}
		p.byThread[threadRoot] = append(p.byThread[threadRoot], &TimelineEntry{StreamSeq: seq, Event: event})
		p.messageToThread[replyID] = threadRoot
		p.replySummaries[replyID] = &threadReplySummary{
			actorID:   messageAuthorID(event, m),
			createdAt: eventCreatedAt(event),
		}
		summary := p.summaryByThread[threadRoot]
		if summary == nil {
			summary = &threadSummary{}
			p.summaryByThread[threadRoot] = summary
		}
		summary.replyIDs = append(summary.replyIDs, replyID)
		p.recomputeSummaryLocked(threadRoot)
		markApplied()

	case *corev1.Event_MessageEdited:
		threadRoot, ok := p.messageToThread[e.MessageEdited.GetEventId()]
		if !ok {
			return nil // target isn't a known thread reply
		}
		p.byThread[threadRoot] = append(p.byThread[threadRoot], &TimelineEntry{StreamSeq: seq, Event: event})
		markApplied()

	case *corev1.Event_MessageRetracted:
		threadRoot, ok := p.messageToThread[e.MessageRetracted.GetEventId()]
		if !ok {
			return nil
		}
		p.byThread[threadRoot] = append(p.byThread[threadRoot], &TimelineEntry{StreamSeq: seq, Event: event})
		if reply := p.replySummaries[e.MessageRetracted.GetEventId()]; reply != nil {
			reply.retracted = true
			p.recomputeSummaryLocked(threadRoot)
		}
		markApplied()
	}
	return nil
}

func eventCreatedAt(event *corev1.Event) time.Time {
	if event == nil || event.GetCreatedAt() == nil {
		return time.Time{}
	}
	return event.GetCreatedAt().AsTime()
}

func (p *ThreadProjection) recomputeSummaryLocked(threadRoot string) {
	summary := p.summaryByThread[threadRoot]
	if summary == nil {
		summary = &threadSummary{}
		p.summaryByThread[threadRoot] = summary
	}

	summary.replyCount = 0
	summary.lastReplyAt = nil
	summary.participantIDs = nil
	participants := make(map[string]struct{})

	for _, replyID := range summary.replyIDs {
		reply := p.replySummaries[replyID]
		if reply == nil || reply.retracted {
			continue
		}
		if _, shredded := p.shreddedUsers[reply.actorID]; shredded {
			continue
		}

		summary.replyCount++
		if !reply.createdAt.IsZero() && (summary.lastReplyAt == nil || reply.createdAt.After(*summary.lastReplyAt)) {
			at := reply.createdAt
			summary.lastReplyAt = &at
		}
		if reply.actorID != "" {
			if _, seen := participants[reply.actorID]; !seen && len(summary.participantIDs) < maxThreadParticipants {
				participants[reply.actorID] = struct{}{}
				summary.participantIDs = append(summary.participantIDs, reply.actorID)
			}
		}
	}
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

// ReplyCount returns how many visible MessagePostedEvent replies the thread
// has accumulated. Edits don't bump the count; retractions and key-shredded
// authors remove replies from the visible summary.
func (p *ThreadProjection) ReplyCount(rootEventID string) int {
	p.RLock()
	defer p.RUnlock()
	summary := p.summaryByThread[rootEventID]
	if summary == nil {
		return 0
	}
	return summary.replyCount
}

// ThreadMetadata returns cached display metadata for a thread. The projection
// keeps this summary updated as thread events arrive, so callers do not need to
// scan the full reply timeline for every followed-thread list item.
func (p *ThreadProjection) ThreadMetadata(rootEventID string) *ThreadMetadata {
	p.RLock()
	defer p.RUnlock()
	summary := p.summaryByThread[rootEventID]
	if summary == nil {
		return &ThreadMetadata{}
	}
	metadata := &ThreadMetadata{
		ReplyCount:     summary.replyCount,
		ParticipantIDs: append([]string(nil), summary.participantIDs...),
	}
	if summary.lastReplyAt != nil {
		at := *summary.lastReplyAt
		metadata.LastReplyAt = &at
	}
	return metadata
}

// ThreadCount returns how many threads are currently in the
// projection. Diagnostics only.
func (p *ThreadProjection) ThreadCount() int {
	p.RLock()
	defer p.RUnlock()
	return len(p.byThread)
}

// ThreadExists reports whether an explicit ThreadCreatedEvent or at least one
// reply has established this thread in the projection.
func (p *ThreadProjection) ThreadExists(rootEventID string) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.byThread[rootEventID]
	return ok
}

// Stats returns aggregate counts useful for import/rollout diagnostics.
func (p *ThreadProjection) Stats() (threads int, entries int, replies int) {
	p.RLock()
	defer p.RUnlock()
	threads = len(p.byThread)
	for _, threadEntries := range p.byThread {
		entries += len(threadEntries)
		for _, entry := range threadEntries {
			if entry != nil && entry.Event.GetMessagePosted() != nil {
				replies++
			}
		}
	}
	return threads, entries, replies
}
