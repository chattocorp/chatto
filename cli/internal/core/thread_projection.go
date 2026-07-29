package core

import (
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type threadReplySummary struct {
	actorID        threadUserRef
	createdSeconds int64
	createdNanos   int32
	hasCreatedAt   bool
	retracted      bool
	known          bool
}

type threadSummary struct {
	replyCount        int
	lastReplyAt       *time.Time
	participantIDs    []threadUserRef
	participantCounts []uint32
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

type threadEventRef uint32
type threadRoomRef uint32
type threadUserRef uint32

type threadFollowKey struct {
	user threadUserRef
	room threadRoomRef
	root threadEventRef
}

type threadTarget struct {
	room threadRoomRef
	root threadEventRef
}

type ThreadTimelineEntry struct {
	EventID   string
	StreamSeq uint64
}

type compactThreadTimelineEntry struct {
	Event threadEventRef
	Seq   uint64
}

// ThreadProjection holds an append-only event log per thread,
// derived from the same evt.room.> firehose RoomTimelineProjection
// consumes.
//
// "Per thread" means: reply posts (MessagePostedEvent with in_thread != "").
// The thread root message itself is NOT stored here; the thread-view resolver
// fetches the root from RoomTimelineProjection.Get(rootEventID) and
// concatenates. Reply rows retain only event IDs and stream sequences, and
// resolvers hydrate the full event from RoomTimelineProjection.
//
// To route edits and retracts to the right thread, we maintain a
// secondary index mapping reply event_id → thread root event_id,
// populated as MessagePostedEvent replies arrive. Edits and
// retracts of root messages (which aren't in any thread bucket)
// are silently skipped here; they'll be handled at the room-
// timeline level.
//
// Edits and retractions targeting replies are folded into cached summaries and
// latest-body state instead of being retained as separate thread rows.
type ThreadProjection struct {
	events.MemoryProjection
	eventIDs        projectionStringTable
	roomIDs         projectionStringTable
	userIDs         projectionStringTable
	byThread        map[threadEventRef][]compactThreadTimelineEntry
	messageToThread []threadEventRef
	replySummaries  []threadReplySummary
	summaryByThread map[threadEventRef]*threadSummary
	followState     map[threadFollowKey]uint8
	followers       map[threadTarget][]threadUserRef
	followedByUser  map[threadUserRef][]threadTarget
	replayGuard     projectionReplayGuard
	shreddedUsers   []bool
}

// NewThreadProjection returns an empty projection.
func NewThreadProjection() *ThreadProjection {
	return &ThreadProjection{
		eventIDs:        newProjectionStringTable(),
		roomIDs:         newProjectionStringTable(),
		userIDs:         newProjectionStringTable(),
		byThread:        make(map[threadEventRef][]compactThreadTimelineEntry),
		messageToThread: make([]threadEventRef, 1),
		replySummaries:  make([]threadReplySummary, 1),
		summaryByThread: make(map[threadEventRef]*threadSummary),
		followState:     make(map[threadFollowKey]uint8),
		followers:       make(map[threadTarget][]threadUserRef),
		followedByUser:  make(map[threadUserRef][]threadTarget),
		replayGuard:     newProjectionReplayGuard(),
		shreddedUsers:   make([]bool, 1),
	}
}

func (p *ThreadProjection) internEventIDLocked(id string) threadEventRef {
	ref := threadEventRef(p.eventIDs.intern(id))
	p.messageToThread = growProjectionSlice(p.messageToThread, uint32(ref))
	p.replySummaries = growProjectionSlice(p.replySummaries, uint32(ref))
	return ref
}

func (p *ThreadProjection) internRoomIDLocked(id string) threadRoomRef {
	return threadRoomRef(p.roomIDs.intern(id))
}

func (p *ThreadProjection) internUserIDLocked(id string) threadUserRef {
	ref := threadUserRef(p.userIDs.intern(id))
	p.shreddedUsers = growProjectionSlice(p.shreddedUsers, uint32(ref))
	return ref
}

func (p *ThreadProjection) eventIDLocked(ref threadEventRef) string {
	return p.eventIDs.value(uint32(ref))
}

func (p *ThreadProjection) roomIDLocked(ref threadRoomRef) string {
	return p.roomIDs.value(uint32(ref))
}

func (p *ThreadProjection) userIDLocked(ref threadUserRef) string {
	return p.userIDs.value(uint32(ref))
}

func (p *ThreadProjection) eventRefLocked(id string) (threadEventRef, bool) {
	ref, ok := p.eventIDs.lookup(id)
	return threadEventRef(ref), ok
}

func (p *ThreadProjection) userRefLocked(id string) (threadUserRef, bool) {
	ref, ok := p.userIDs.lookup(id)
	return threadUserRef(ref), ok
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

// ReplaySubjects uses one stream-wide physical filter because JetStream's
// multi-filter scan is expensive when it combines the broad room wildcard with
// the sparse user-key-shredded family. The Projector rejects unrelated subjects
// before decoding or applying them.
func (p *ThreadProjection) ReplaySubjects() []string {
	return []string{events.EventSubjectFilter()}
}

// Apply implements events.Projection.
//
// Recognised events:
//
//   - MessagePostedEvent with in_thread != "" → append to the
//     thread's slice, remember its event_id → thread mapping.
//   - ThreadCreatedEvent → initialise the thread's bucket even before
//     replies land.
//   - MessageEditedEvent whose target event_id is a known thread reply → mark
//     the fact applied; latest body state lives in RoomTimelineProjection.
//   - MessageRetractedEvent whose target event_id is a known thread reply →
//     fold the retraction into the thread summary.
//
// Everything else (root messages, room lifecycle, memberships,
// edits/retracts of non-reply messages) is silently ignored.
func (p *ThreadProjection) Apply(event *corev1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	if p.replayGuard.seen(event, seq) {
		return nil
	}
	markApplied := func() {
		p.replayGuard.mark(event, seq)
	}

	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserKeyShredded:
		if userID := e.UserKeyShredded.GetUserId(); userID != "" {
			userRef := p.internUserIDLocked(userID)
			p.shreddedUsers[userRef] = true
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
		rootRef := p.internEventIDLocked(threadRoot)
		if _, exists := p.byThread[rootRef]; !exists {
			p.byThread[rootRef] = nil
		}
		if _, exists := p.summaryByThread[rootRef]; !exists {
			p.summaryByThread[rootRef] = newThreadSummary()
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
		rootRef := p.internEventIDLocked(threadRoot)
		replyRef := p.internEventIDLocked(replyID)
		p.byThread[rootRef] = append(p.byThread[rootRef], compactThreadTimelineEntry{Event: replyRef, Seq: seq})
		p.messageToThread[replyRef] = rootRef
		createdAt := eventCreatedAt(event)
		var createdSeconds int64
		var createdNanos int32
		if !createdAt.IsZero() {
			createdSeconds = createdAt.Unix()
			createdNanos = int32(createdAt.Nanosecond())
		}
		p.replySummaries[replyRef] = threadReplySummary{
			actorID:        p.internUserIDLocked(messageAuthorID(event)),
			createdSeconds: createdSeconds,
			createdNanos:   createdNanos,
			hasCreatedAt:   !createdAt.IsZero(),
			known:          true,
		}
		summary := p.summaryByThread[rootRef]
		if summary == nil {
			summary = newThreadSummary()
			p.summaryByThread[rootRef] = summary
		}
		p.applyReplyToSummaryLocked(summary, replyRef)
		markApplied()

	case *corev1.Event_MessageEdited:
		replyRef, ok := p.eventRefLocked(e.MessageEdited.GetEventId())
		if !ok || p.messageToThread[replyRef] == 0 {
			return nil // target isn't a known thread reply
		}
		markApplied()

	case *corev1.Event_MessageRetracted:
		targetID := e.MessageRetracted.GetEventId()
		replyRef, ok := p.eventRefLocked(targetID)
		if !ok || p.messageToThread[replyRef] == 0 {
			return nil
		}
		threadRoot := p.messageToThread[replyRef]
		if reply := &p.replySummaries[replyRef]; reply.known {
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

func (p *ThreadProjection) CompleteStartupReplay() {
	p.Lock()
	defer p.Unlock()
	p.replayGuard.completeReplay()
}

func (p *ThreadProjection) setThreadFollowStateLocked(userID, roomID, threadRootEventID string, state ThreadFollowState) {
	if userID == "" || roomID == "" || threadRootEventID == "" {
		return
	}
	userRef := p.internUserIDLocked(userID)
	target := threadTarget{room: p.internRoomIDLocked(roomID), root: p.internEventIDLocked(threadRootEventID)}
	stateKey := threadFollowKey{user: userRef, room: target.room, root: target.root}
	encodedState := uint8(1)
	if state == ThreadFollowStateUnfollowed {
		encodedState = 2
	}
	previous := p.followState[stateKey]
	if previous == encodedState {
		return
	}

	if previous == 1 {
		p.followers[target] = removeThreadUserRef(p.followers[target], userRef)
		if len(p.followers[target]) == 0 {
			delete(p.followers, target)
		}
		p.followedByUser[userRef] = removeThreadTarget(p.followedByUser[userRef], target)
		if len(p.followedByUser[userRef]) == 0 {
			delete(p.followedByUser, userRef)
		}
	}

	p.followState[stateKey] = encodedState

	if state == ThreadFollowStateFollowing {
		p.followers[target] = append(p.followers[target], userRef)
		p.followedByUser[userRef] = append(p.followedByUser[userRef], target)
	}
}

func newThreadSummary() *threadSummary {
	return &threadSummary{}
}

func eventCreatedAt(event *corev1.Event) time.Time {
	if event == nil || event.GetCreatedAt() == nil {
		return time.Time{}
	}
	return event.GetCreatedAt().AsTime()
}

func (p *ThreadProjection) recomputeSummaryLocked(threadRoot threadEventRef) {
	summary := p.summaryByThread[threadRoot]
	if summary == nil {
		summary = newThreadSummary()
		p.summaryByThread[threadRoot] = summary
	}

	summary.replyCount = 0
	summary.lastReplyAt = nil
	summary.participantIDs = nil
	summary.participantCounts = nil

	for _, reply := range p.byThread[threadRoot] {
		p.applyReplyToSummaryLocked(summary, reply.Event)
	}
}

func (p *ThreadProjection) applyReplyToSummaryLocked(summary *threadSummary, replyID threadEventRef) {
	if summary == nil || replyID == 0 {
		return
	}
	reply := p.replySummaries[replyID]
	if !reply.known || reply.retracted {
		return
	}
	if p.shreddedUsers[reply.actorID] {
		return
	}

	summary.replyCount++
	if reply.hasCreatedAt {
		at := time.Unix(reply.createdSeconds, int64(reply.createdNanos))
		if summary.lastReplyAt == nil || at.After(*summary.lastReplyAt) {
			summary.lastReplyAt = &at
		}
	}
	if reply.actorID != 0 {
		for i, participant := range summary.participantIDs {
			if participant == reply.actorID {
				summary.participantCounts[i]++
				return
			}
		}
		if len(summary.participantIDs) < maxThreadParticipants {
			summary.participantIDs = append(summary.participantIDs, reply.actorID)
			summary.participantCounts = append(summary.participantCounts, 1)
		}
	}
}

func removeThreadUserRef(values []threadUserRef, target threadUserRef) []threadUserRef {
	for i, value := range values {
		if value == target {
			return append(values[:i], values[i+1:]...)
		}
	}
	return values
}

func removeThreadTarget(values []threadTarget, target threadTarget) []threadTarget {
	for i, value := range values {
		if value == target {
			return append(values[:i], values[i+1:]...)
		}
	}
	return values
}

// ThreadEvents returns reply event references for a thread in stream order.
// Edit and retract facts are folded into the projection's summaries and latest
// body state instead of being retained as separate rows.
//
// The root message is NOT included — resolvers fetch it from
// RoomTimelineProjection.Get(rootEventID) and prepend.
func (p *ThreadProjection) ThreadEvents(rootEventID string) []ThreadTimelineEntry {
	p.RLock()
	defer p.RUnlock()
	rootRef, ok := p.eventRefLocked(rootEventID)
	if !ok {
		return nil
	}
	entries := p.byThread[rootRef]
	if len(entries) == 0 {
		return nil
	}
	out := make([]ThreadTimelineEntry, len(entries))
	for i, entry := range entries {
		out[i] = ThreadTimelineEntry{EventID: p.eventIDLocked(entry.Event), StreamSeq: entry.Seq}
	}
	return out
}

// ReplyCount returns how many visible MessagePostedEvent replies the thread
// has accumulated. Edits don't bump the count; retractions and key-shredded
// authors remove replies from the visible summary.
func (p *ThreadProjection) ReplyCount(rootEventID string) int {
	p.RLock()
	defer p.RUnlock()
	rootRef, ok := p.eventRefLocked(rootEventID)
	if !ok {
		return 0
	}
	summary := p.summaryByThread[rootRef]
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
	rootRef, ok := p.eventRefLocked(rootEventID)
	if !ok {
		return &ThreadMetadata{}
	}
	summary := p.summaryByThread[rootRef]
	if summary == nil {
		return &ThreadMetadata{}
	}
	metadata := &ThreadMetadata{ReplyCount: summary.replyCount, ParticipantIDs: make([]string, len(summary.participantIDs))}
	for i, participant := range summary.participantIDs {
		metadata.ParticipantIDs[i] = p.userIDLocked(participant)
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
	userRaw, userOK := p.userIDs.lookup(userID)
	roomRaw, roomOK := p.roomIDs.lookup(roomID)
	rootRaw, rootOK := p.eventIDs.lookup(threadRootEventID)
	if !userOK || !roomOK || !rootOK {
		return ThreadFollowStateNone
	}
	switch p.followState[threadFollowKey{user: threadUserRef(userRaw), room: threadRoomRef(roomRaw), root: threadEventRef(rootRaw)}] {
	case 1:
		return ThreadFollowStateFollowing
	case 2:
		return ThreadFollowStateUnfollowed
	default:
		return ThreadFollowStateNone
	}
}

func (p *ThreadProjection) ThreadFollowers(roomID, threadRootEventID string) []string {
	p.RLock()
	defer p.RUnlock()
	roomRaw, roomOK := p.roomIDs.lookup(roomID)
	rootRaw, rootOK := p.eventIDs.lookup(threadRootEventID)
	if !roomOK || !rootOK {
		return nil
	}
	followers := p.followers[threadTarget{room: threadRoomRef(roomRaw), root: threadEventRef(rootRaw)}]
	if len(followers) == 0 {
		return nil
	}
	userIDs := make([]string, 0, len(followers))
	for _, userRef := range followers {
		userIDs = append(userIDs, p.userIDLocked(userRef))
	}
	return userIDs
}

func (p *ThreadProjection) FollowedThreadsForUser(userID string) []threadFollowRef {
	p.RLock()
	defer p.RUnlock()
	userRef, ok := p.userRefLocked(userID)
	if !ok {
		return nil
	}
	followed := p.followedByUser[userRef]
	if len(followed) == 0 {
		return nil
	}
	refs := make([]threadFollowRef, 0, len(followed))
	for _, target := range followed {
		refs = append(refs, threadFollowRef{
			roomID:            p.roomIDLocked(target.room),
			threadRootEventID: p.eventIDLocked(target.root),
		})
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
	rootRef, known := p.eventRefLocked(rootEventID)
	if !known {
		return false
	}
	_, ok := p.byThread[rootRef]
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
			if entry.Event != 0 {
				replies++
			}
		}
	}
	return threads, entries, replies
}
