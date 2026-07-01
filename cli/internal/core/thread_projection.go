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
	replyIDs          []string
	replyCount        int
	lastReplyAt       *time.Time
	participantIDs    []string
	participantCounts map[string]int
}

type ThreadFollowState string

const (
	ThreadFollowStateNone       ThreadFollowState = ""
	ThreadFollowStateFollowing  ThreadFollowState = "following"
	ThreadFollowStateUnfollowed ThreadFollowState = "unfollowed"
)

type threadFollowRef struct {
	roomID            string
	threadRootEventID string
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
	followState     map[string]ThreadFollowState
	followers       map[string]map[string]struct{}
	followedByUser  map[string]map[string]threadFollowRef
	appliedEventIDs eventIDSet
	shreddedUsers   map[string]struct{}
}

// NewThreadProjection returns an empty projection.
func NewThreadProjection() *ThreadProjection {
	return &ThreadProjection{
		byThread:        make(map[string][]*TimelineEntry),
		messageToThread: make(map[string]string),
		replySummaries:  make(map[string]*threadReplySummary),
		summaryByThread: make(map[string]*threadSummary),
		followState:     make(map[string]ThreadFollowState),
		followers:       make(map[string]map[string]struct{}),
		followedByUser:  make(map[string]map[string]threadFollowRef),
		appliedEventIDs: newEventIDSet(),
		shreddedUsers:   make(map[string]struct{}),
	}
}

// Subjects implements events.Projection. Threads only need thread lifecycle
// and message mutation families, plus user key-shred events that can hide
// replies during crypto-shredding.
func (p *ThreadProjection) Subjects() []string {
	return []string{
		events.RoomEventTypeFilter(events.EventThreadCreated),
		events.RoomEventTypeFilter(events.EventThreadFollowed),
		events.RoomEventTypeFilter(events.EventThreadUnfollowed),
		events.RoomEventTypeFilter(events.EventMessagePosted),
		events.RoomEventTypeFilter(events.EventMessageEdited),
		events.RoomEventTypeFilter(events.EventMessageRetracted),
		events.UserEventTypeFilter(events.EventUserKeyShredded),
	}
}

// ReplaySubjects keeps Threads on the same physical replay stream as the room
// timeline projection. A separate multi-filter ordered consumer is slower on
// current self-host scale than sharing the broad room replay and skipping
// non-thread events before Apply.
func (p *ThreadProjection) ReplaySubjects() []string {
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

	if p.appliedEventIDs.has(event) {
		return nil
	}
	markApplied := func() {
		p.appliedEventIDs.mark(event)
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
			p.summaryByThread[threadRoot] = newThreadSummary()
		}
		markApplied()

	case *corev1.Event_ThreadFollowed:
		follow := e.ThreadFollowed
		p.setThreadFollowStateLocked(follow.GetUserId(), follow.GetRoomId(), follow.GetThreadRootEventId(), ThreadFollowStateFollowing)
		markApplied()

	case *corev1.Event_ThreadUnfollowed:
		unfollow := e.ThreadUnfollowed
		p.setThreadFollowStateLocked(unfollow.GetUserId(), unfollow.GetRoomId(), unfollow.GetThreadRootEventId(), ThreadFollowStateUnfollowed)
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
			actorID:   messageAuthorID(event),
			createdAt: eventCreatedAt(event),
		}
		summary := p.summaryByThread[threadRoot]
		if summary == nil {
			summary = newThreadSummary()
			p.summaryByThread[threadRoot] = summary
		}
		summary.replyIDs = append(summary.replyIDs, replyID)
		p.applyReplyToSummaryLocked(summary, replyID)
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
			// Retractions are rare and can invalidate last-reply or participant
			// ordering, so recomputing the affected thread keeps the hot reply
			// path O(1) without making removal bookkeeping subtle.
			p.recomputeSummaryLocked(threadRoot)
		}
		markApplied()
	}
	return nil
}

func threadFollowKeyPart(roomID, threadRootEventID string) string {
	return roomID + "\x00" + threadRootEventID
}

func (p *ThreadProjection) setThreadFollowStateLocked(userID, roomID, threadRootEventID string, state ThreadFollowState) {
	if userID == "" || roomID == "" || threadRootEventID == "" {
		return
	}
	key := threadFollowKeyPart(roomID, threadRootEventID)
	stateKey := userID + "\x00" + key
	previous := p.followState[stateKey]
	if previous == state {
		return
	}

	if previous == ThreadFollowStateFollowing {
		if followers := p.followers[key]; followers != nil {
			delete(followers, userID)
			if len(followers) == 0 {
				delete(p.followers, key)
			}
		}
		if followed := p.followedByUser[userID]; followed != nil {
			delete(followed, key)
			if len(followed) == 0 {
				delete(p.followedByUser, userID)
			}
		}
	}

	p.followState[stateKey] = state

	if state == ThreadFollowStateFollowing {
		followers := p.followers[key]
		if followers == nil {
			followers = make(map[string]struct{})
			p.followers[key] = followers
		}
		followers[userID] = struct{}{}

		followed := p.followedByUser[userID]
		if followed == nil {
			followed = make(map[string]threadFollowRef)
			p.followedByUser[userID] = followed
		}
		followed[key] = threadFollowRef{roomID: roomID, threadRootEventID: threadRootEventID}
	}
}

// SeedLegacyThreadFollowState imports pre-EVT thread follow state from
// RUNTIME_STATE. TODO(remove-after-0.4): delete after a documented cutoff or
// migration to canonical ThreadFollowed/ThreadUnfollowed events.
func (p *ThreadProjection) SeedLegacyThreadFollowState(userID, roomID, threadRootEventID string, state ThreadFollowState) {
	p.Lock()
	defer p.Unlock()
	if _, ok := p.followState[userID+"\x00"+threadFollowKeyPart(roomID, threadRootEventID)]; ok {
		return
	}
	p.setThreadFollowStateLocked(userID, roomID, threadRootEventID, state)
}

func newThreadSummary() *threadSummary {
	return &threadSummary{
		participantCounts: make(map[string]int),
	}
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
		summary = newThreadSummary()
		p.summaryByThread[threadRoot] = summary
	} else if summary.participantCounts == nil {
		summary.participantCounts = make(map[string]int)
	}

	summary.replyCount = 0
	summary.lastReplyAt = nil
	summary.participantIDs = nil
	clear(summary.participantCounts)

	for _, replyID := range summary.replyIDs {
		p.applyReplyToSummaryLocked(summary, replyID)
	}
}

func (p *ThreadProjection) applyReplyToSummaryLocked(summary *threadSummary, replyID string) {
	if summary == nil || replyID == "" {
		return
	}
	if summary.participantCounts == nil {
		summary.participantCounts = make(map[string]int)
	}

	reply := p.replySummaries[replyID]
	if reply == nil || reply.retracted {
		return
	}
	if _, shredded := p.shreddedUsers[reply.actorID]; shredded {
		return
	}

	summary.replyCount++
	if !reply.createdAt.IsZero() && (summary.lastReplyAt == nil || reply.createdAt.After(*summary.lastReplyAt)) {
		at := reply.createdAt
		summary.lastReplyAt = &at
	}
	if reply.actorID != "" {
		summary.participantCounts[reply.actorID]++
		if summary.participantCounts[reply.actorID] == 1 && len(summary.participantIDs) < maxThreadParticipants {
			summary.participantIDs = append(summary.participantIDs, reply.actorID)
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

func (p *ThreadProjection) FollowState(userID, roomID, threadRootEventID string) ThreadFollowState {
	p.RLock()
	defer p.RUnlock()
	return p.followState[userID+"\x00"+threadFollowKeyPart(roomID, threadRootEventID)]
}

func (p *ThreadProjection) ThreadFollowers(roomID, threadRootEventID string) []string {
	p.RLock()
	defer p.RUnlock()
	followers := p.followers[threadFollowKeyPart(roomID, threadRootEventID)]
	if len(followers) == 0 {
		return nil
	}
	userIDs := make([]string, 0, len(followers))
	for userID := range followers {
		userIDs = append(userIDs, userID)
	}
	return userIDs
}

func (p *ThreadProjection) FollowedThreadsForUser(userID string) []threadFollowRef {
	p.RLock()
	defer p.RUnlock()
	followed := p.followedByUser[userID]
	if len(followed) == 0 {
		return nil
	}
	refs := make([]threadFollowRef, 0, len(followed))
	for _, ref := range followed {
		refs = append(refs, ref)
	}
	return refs
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
