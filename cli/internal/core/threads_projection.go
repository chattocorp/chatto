package core

import (
	"slices"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ThreadsProjection holds per-thread metadata derived from
// MessagePostedEvent (issue #597 phase 2). It consumes the same EVT
// event filter as MessagesProjection but maintains a separate index
// keyed by the thread-root event ID, so "what's in this thread?" and
// "what's the reply count?" are O(1) without scanning the messages
// projection's per-room list.
//
// What counts as a thread reply: a MessagePostedEvent with
// in_thread != "" AND echo_of_event_id == "". Echoes are mirrors of
// thread replies that show up in the main room timeline; they're not
// themselves replies and don't bump the count.
//
// Retract / edit behaviour:
//
//   - Reply count is monotonic. Retracting a reply does NOT decrement
//     replyCount or recompute lastReplyAt — keeping the counters
//     monotonic avoids re-scanning the reply list on every retract and
//     matches the "event log keeps the full history for audit"
//     semantics of the projection. UIs render retracted replies as
//     placeholders rather than removing them; counters stay honest to
//     "messages ever posted into this thread."
//
//   - Editing a reply changes only its body (handled by
//     MessagesProjection). Thread metadata is unaffected.
//
// Scope explicitly NOT covered here:
//   - Followed-thread set (per-user "who's following this thread").
//     Currently driven by the live-only ThreadFollowChangedEvent which
//     doesn't ride EVT. A durable equivalent is a future event-type
//     addition; see issue #597 open questions.
//   - Tombstone fan-out. The MessagesProjection holds the tombstone
//     bit on the reply itself; resolvers join the two projections at
//     read time.
type ThreadsProjection struct {
	events.MemoryProjection
	byThread map[string]*threadEntry
}

// threadEntry is the in-memory state per thread. Not exposed
// directly — callers go through Thread / MetadataForRoots which clone
// into the public Thread shape.
type threadEntry struct {
	rootEventID  string
	roomID       string
	replyIDs     []string // in stream-arrival order; excludes echoes
	replyCount   int      // == len(replyIDs); kept as a separate counter so it's monotonic across future "skip dropped reply" edge cases
	lastReplyAt  time.Time
	participants map[string]struct{} // user IDs who posted at least one reply (excludes echoes)
}

// Thread is the public view of a thread's aggregate state. Fresh
// value per call; callers may mutate freely.
//
// MetadataForRoots returns the existing core.ThreadMetadata shape from
// threads.go — same type the legacy GetThreadMetadata resolver returns,
// so the future read cutover is a one-line swap rather than a
// resolver-layer translation.
type Thread struct {
	ThreadRootEventID string
	RoomID            string
	ReplyEventIDs     []string
	ReplyCount        int
	LastReplyAt       *time.Time
	ParticipantIDs    []string
}

// NewThreadsProjection returns an empty projection.
func NewThreadsProjection() *ThreadsProjection {
	return &ThreadsProjection{
		byThread: make(map[string]*threadEntry),
	}
}

// Subjects implements events.Projection. Same filter family as
// MessagesProjection; the apply switch ignores variants we don't
// care about. MessageEdited / MessageRetracted are subscribed-to but
// silently no-op'd because thread metadata is monotonic.
func (p *ThreadsProjection) Subjects() []string {
	return []string{
		events.RoomEventTypeFilter(events.EventMessagePosted),
		events.RoomEventTypeFilter(events.EventMessageEdited),
		events.RoomEventTypeFilter(events.EventMessageRetracted),
	}
}

// Apply implements events.Projection.
func (p *ThreadsProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	posted, ok := event.GetEvent().(*corev1.Event_MessagePosted)
	if !ok {
		// Edited / Retracted / anything else: thread metadata is
		// monotonic, nothing to update.
		return nil
	}
	m := posted.MessagePosted
	threadRoot := m.GetInThread()
	if threadRoot == "" {
		return nil // root message, not a reply
	}
	if m.GetEchoOfEventId() != "" {
		// Echo: mirror of a thread reply in the main channel. Doesn't
		// count as a reply (would double-count if it did).
		return nil
	}

	p.Lock()
	defer p.Unlock()
	entry, ok := p.byThread[threadRoot]
	if !ok {
		entry = &threadEntry{
			rootEventID:  threadRoot,
			roomID:       m.GetRoomId(),
			participants: make(map[string]struct{}),
		}
		p.byThread[threadRoot] = entry
	}
	// Idempotency: a re-applied MessagePosted for the same reply
	// event_id is a no-op.
	replyID := m.GetEventId()
	if replyID == "" {
		return nil
	}
	if slices.Contains(entry.replyIDs, replyID) {
		return nil
	}
	entry.replyIDs = append(entry.replyIDs, replyID)
	entry.replyCount = len(entry.replyIDs)
	if t := event.GetCreatedAt().AsTime(); t.After(entry.lastReplyAt) {
		entry.lastReplyAt = t
	}
	if actor := event.GetActorId(); actor != "" {
		entry.participants[actor] = struct{}{}
	}
	return nil
}

// Thread returns the full thread view for a given root event ID, or
// (nil, false) if no replies have landed for that root.
func (p *ThreadsProjection) Thread(rootEventID string) (*Thread, bool) {
	p.RLock()
	defer p.RUnlock()
	entry, ok := p.byThread[rootEventID]
	if !ok {
		return nil, false
	}
	return entryToThread(entry), true
}

// MetadataForRoots returns thread metadata for each requested root,
// indexed by root event ID. Roots with no replies are absent from the
// returned map (no zero entries).
//
// Bulk-shaped because the room timeline resolver wants previews
// attached to many roots at once.
func (p *ThreadsProjection) MetadataForRoots(rootEventIDs []string) map[string]*ThreadMetadata {
	out := make(map[string]*ThreadMetadata, len(rootEventIDs))
	if len(rootEventIDs) == 0 {
		return out
	}
	p.RLock()
	defer p.RUnlock()
	for _, root := range rootEventIDs {
		entry, ok := p.byThread[root]
		if !ok {
			continue
		}
		out[root] = entryToThreadMetadata(entry)
	}
	return out
}

// Count returns the number of threads currently in the projection.
// Useful for diagnostics.
func (p *ThreadsProjection) Count() int {
	p.RLock()
	defer p.RUnlock()
	return len(p.byThread)
}

func entryToThread(entry *threadEntry) *Thread {
	return &Thread{
		ThreadRootEventID: entry.rootEventID,
		RoomID:            entry.roomID,
		ReplyEventIDs:     append([]string(nil), entry.replyIDs...),
		ReplyCount:        entry.replyCount,
		LastReplyAt:       nullableTime(entry.lastReplyAt),
		ParticipantIDs:    participantsSorted(entry.participants),
	}
}

func entryToThreadMetadata(entry *threadEntry) *ThreadMetadata {
	return &ThreadMetadata{
		ReplyCount:     entry.replyCount,
		LastReplyAt:    nullableTime(entry.lastReplyAt),
		ParticipantIDs: participantsSorted(entry.participants),
	}
}

// nullableTime returns a pointer to the time, or nil if t is the zero
// value. Matches the existing core.ThreadMetadata semantics where a
// nil LastReplyAt means "thread has no replies yet" — never happens in
// practice with this projection (we don't create entries with zero
// replies) but keeps the public type contract honest.
func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// participantsSorted returns a fresh slice of participant IDs. Order
// is unspecified (map iteration); callers that need stable order sort
// the result themselves.
func participantsSorted(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}
