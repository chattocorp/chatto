package core

import (
	"slices"
	"time"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

type threadReplySummary struct {
	actorID   string
	createdAt time.Time
	retracted bool
}

type threadSummary struct {
	replyIDs           []string
	replyCount         int
	lastReplyAt        *time.Time
	latestReplyEventID string
	participantIDs     []string
	participantCounts  map[string]int
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

// ThreadInteractionCauseKind identifies the durable fact that established one
// account's relationship with a channel-room thread.
type ThreadInteractionCauseKind string

const (
	ThreadInteractionCauseRootAuthored  ThreadInteractionCauseKind = "root-authored"
	ThreadInteractionCauseDirectMention ThreadInteractionCauseKind = "direct-mention"
)

// ThreadInteractionCause is one immutable post-time reason that an account is
// related to a channel-room thread.
type ThreadInteractionCause struct {
	Kind          ThreadInteractionCauseKind
	SourceEventID string
	CreatedAt     time.Time
}

// ThreadInteraction is one account-to-thread relationship derived from
// durable message facts.
type ThreadInteraction struct {
	RoomID            string
	ThreadRootEventID string
	Causes            []ThreadInteractionCause
}

type threadMessageRef struct {
	roomID            string
	threadRootEventID string
}

type projectedThreadInteraction struct {
	roomID            string
	threadRootEventID string
	causes            map[string]ThreadInteractionCause
}

type ThreadTimelineEntry struct {
	EventID   string
	StreamSeq uint64
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
	byThread        map[string][]ThreadTimelineEntry
	messageToThread map[string]string // reply event_id → thread root event_id
	channelRooms    map[string]struct{}
	messageThreads  map[string]threadMessageRef
	interactions    map[string]map[string]*projectedThreadInteraction
	replySummaries  map[string]*threadReplySummary
	summaryByThread map[string]*threadSummary
	followState     map[string]ThreadFollowState
	followers       map[string]map[string]struct{}
	followedByUser  map[string]map[string]threadFollowRef
	replayGuard     projectionReplayGuard
	shreddedUsers   map[string]struct{}
}

// NewThreadProjection returns an empty projection.
func NewThreadProjection() *ThreadProjection {
	return &ThreadProjection{
		byThread:        make(map[string][]ThreadTimelineEntry),
		messageToThread: make(map[string]string),
		channelRooms:    make(map[string]struct{}),
		messageThreads:  make(map[string]threadMessageRef),
		interactions:    make(map[string]map[string]*projectedThreadInteraction),
		replySummaries:  make(map[string]*threadReplySummary),
		summaryByThread: make(map[string]*threadSummary),
		followState:     make(map[string]ThreadFollowState),
		followers:       make(map[string]map[string]struct{}),
		followedByUser:  make(map[string]map[string]threadFollowRef),
		replayGuard:     newProjectionReplayGuard(),
		shreddedUsers:   make(map[string]struct{}),
	}
}

// Subjects implements evtstream.Projection. Room lifecycle and every message
// post supply the channel and relationship indexes. Thread lifecycle, message
// mutation, and user key-shred facts supply the existing thread views.
func (p *ThreadProjection) Subjects() []string {
	return []string{
		evtstream.RoomEventTypeFilter(evtstream.EventRoomCreated),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomDeleted),
		evtstream.RoomEventTypeFilter(evtstream.EventThreadCreated),
		evtstream.RoomEventTypeFilter(evtstream.EventThreadFollowed),
		evtstream.RoomEventTypeFilter(evtstream.EventThreadUnfollowed),
		evtstream.RoomEventTypeFilter(evtstream.EventMessagePosted),
		evtstream.RoomEventTypeFilter(evtstream.EventMessageEdited),
		evtstream.RoomEventTypeFilter(evtstream.EventMessageRetracted),
		evtstream.UserEventTypeFilter(evtstream.EventUserKeyShreddingRequested),
		evtstream.UserEventTypeFilter(evtstream.EventUserKeyShredded),
	}
}

// ReplaySubjects uses one stream-wide physical filter because JetStream's
// multi-filter scan is expensive when it combines the broad room wildcard with
// the sparse user-key-shredded family. The Projector rejects unrelated subjects
// before decoding or applying them.
func (p *ThreadProjection) ReplaySubjects() []string {
	return []string{evtstream.EventSubjectFilter()}
}

// Apply implements evtstream.Projection.
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
func (p *ThreadProjection) Apply(event *evtv1.Event, seq uint64) error {
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
	case *evtv1.Event_RoomCreated:
		room := e.RoomCreated
		if room.GetRoomId() == "" || room.GetKind() != evtv1.RoomKind_ROOM_KIND_CHANNEL {
			return nil
		}
		p.channelRooms[room.GetRoomId()] = struct{}{}
		markApplied()

	case *evtv1.Event_RoomDeleted:
		roomID := e.RoomDeleted.GetRoomId()
		if _, ok := p.channelRooms[roomID]; !ok {
			return nil
		}
		delete(p.channelRooms, roomID)
		p.removeRoomInteractionStateLocked(roomID)
		markApplied()

	case *evtv1.Event_UserKeyShreddingRequested:
		p.applyUserKeyShreddedLocked(e.UserKeyShreddingRequested.GetUserId(), markApplied)
	case *evtv1.Event_UserKeyShredded:
		p.applyUserKeyShreddedLocked(e.UserKeyShredded.GetUserId(), markApplied)

	case *evtv1.Event_ThreadCreated:
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

	case *evtv1.Event_ThreadFollowed:
		follow := e.ThreadFollowed
		p.setThreadFollowStateLocked(follow.GetUserId(), follow.GetRoomId(), follow.GetThreadRootEventId(), ThreadFollowStateFollowing)
		markApplied()

	case *evtv1.Event_ThreadUnfollowed:
		unfollow := e.ThreadUnfollowed
		p.setThreadFollowStateLocked(unfollow.GetUserId(), unfollow.GetRoomId(), unfollow.GetThreadRootEventId(), ThreadFollowStateUnfollowed)
		markApplied()

	case *evtv1.Event_MessagePosted:
		m := e.MessagePosted
		if _, channel := p.channelRooms[m.GetRoomId()]; channel {
			p.applyMessageInteractionStateLocked(event, m)
		}
		threadRoot := m.GetInThread()
		if threadRoot == "" {
			if _, channel := p.channelRooms[m.GetRoomId()]; channel {
				markApplied()
			}
			return nil // root-level message; not in any thread bucket
		}
		replyID := event.GetId()
		if replyID == "" {
			return nil
		}
		p.byThread[threadRoot] = append(p.byThread[threadRoot], ThreadTimelineEntry{EventID: replyID, StreamSeq: seq})
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

	case *evtv1.Event_MessageEdited:
		_, ok := p.messageToThread[e.MessageEdited.GetEventId()]
		if !ok {
			return nil // target isn't a known thread reply
		}
		markApplied()

	case *evtv1.Event_MessageRetracted:
		targetID := e.MessageRetracted.GetEventId()
		threadRoot, ok := p.messageToThread[targetID]
		if !ok {
			return nil
		}
		if reply := p.replySummaries[targetID]; reply != nil {
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

func (p *ThreadProjection) applyMessageInteractionStateLocked(event *evtv1.Event, message *evtv1.MessagePostedEvent) {
	if event == nil || message == nil || event.GetId() == "" || message.GetRoomId() == "" {
		return
	}
	rootID := message.GetInThread()
	if rootID == "" {
		rootID = message.GetEchoFromThreadRootEventId()
	}
	if rootID == "" {
		rootID = event.GetId()
	}
	p.messageThreads[event.GetId()] = threadMessageRef{roomID: message.GetRoomId(), threadRootEventID: rootID}

	// Either echo field identifies derived channel-echo state. Malformed or
	// partially upgraded echo facts must not create interaction causes.
	if message.GetEchoOfEventId() != "" || message.GetEchoFromThreadRootEventId() != "" {
		return
	}
	if message.GetInThread() == "" {
		p.addInteractionCauseLocked(event.GetActorId(), message.GetRoomId(), rootID, ThreadInteractionCause{
			Kind: ThreadInteractionCauseRootAuthored, SourceEventID: event.GetId(), CreatedAt: eventCreatedAt(event),
		})
	}
	for _, mention := range message.GetMentions() {
		if mention == nil || mention.GetUserId() == "" || mention.GetUserId() == event.GetActorId() {
			continue
		}
		if _, direct := mention.GetCause().(*evtv1.MessageMention_Direct); !direct {
			continue
		}
		p.addInteractionCauseLocked(mention.GetUserId(), message.GetRoomId(), rootID, ThreadInteractionCause{
			Kind: ThreadInteractionCauseDirectMention, SourceEventID: event.GetId(), CreatedAt: eventCreatedAt(event),
		})
	}
}

func (p *ThreadProjection) addInteractionCauseLocked(userID, roomID, rootID string, cause ThreadInteractionCause) {
	if userID == "" || roomID == "" || rootID == "" || cause.Kind == "" || cause.SourceEventID == "" {
		return
	}
	byThread := p.interactions[userID]
	if byThread == nil {
		byThread = make(map[string]*projectedThreadInteraction)
		p.interactions[userID] = byThread
	}
	key := threadFollowKeyPart(roomID, rootID)
	interaction := byThread[key]
	if interaction == nil {
		interaction = &projectedThreadInteraction{
			roomID: roomID, threadRootEventID: rootID,
			causes: make(map[string]ThreadInteractionCause),
		}
		byThread[key] = interaction
	}
	causeKey := string(cause.Kind) + "\x00" + cause.SourceEventID
	if _, exists := interaction.causes[causeKey]; !exists {
		interaction.causes[causeKey] = cause
	}
}

func (p *ThreadProjection) removeRoomInteractionStateLocked(roomID string) {
	for eventID, ref := range p.messageThreads {
		if ref.roomID == roomID {
			delete(p.messageThreads, eventID)
		}
	}
	for userID, byThread := range p.interactions {
		for key, interaction := range byThread {
			if interaction != nil && interaction.roomID == roomID {
				delete(byThread, key)
			}
		}
		if len(byThread) == 0 {
			delete(p.interactions, userID)
		}
	}
}

func (p *ThreadProjection) applyUserKeyShreddedLocked(userID string, markApplied func()) {
	if userID == "" {
		return
	}
	p.shreddedUsers[userID] = struct{}{}
	for threadRoot := range p.summaryByThread {
		p.recomputeSummaryLocked(threadRoot)
	}
	markApplied()
}

func (p *ThreadProjection) CompleteStartupReplay() {
	p.Lock()
	defer p.Unlock()
	p.replayGuard.completeReplay()
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

func newThreadSummary() *threadSummary {
	return &threadSummary{
		participantCounts: make(map[string]int),
	}
}

func eventCreatedAt(event *evtv1.Event) time.Time {
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
	summary.latestReplyEventID = ""
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
	summary.latestReplyEventID = replyID
	summary.lastReplyAt = nil
	if !reply.createdAt.IsZero() {
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

// ThreadEvents returns reply event references for a thread in stream order.
// Edit and retract facts are folded into the projection's summaries and latest
// body state instead of being retained as separate rows.
//
// The root message is NOT included — resolvers fetch it from
// RoomTimelineProjection.Get(rootEventID) and prepend.
func (p *ThreadProjection) ThreadEvents(rootEventID string) []ThreadTimelineEntry {
	p.RLock()
	defer p.RUnlock()
	entries := p.byThread[rootEventID]
	if len(entries) == 0 {
		return nil
	}
	out := make([]ThreadTimelineEntry, len(entries))
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
		Exists:             true,
		ReplyCount:         summary.replyCount,
		LatestReplyEventID: summary.latestReplyEventID,
		ParticipantIDs:     append([]string(nil), summary.participantIDs...),
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

// ThreadRootForMessage returns the canonical thread root for one projected
// channel-room message, including roots, replies, and channel echoes.
func (p *ThreadProjection) ThreadRootForMessage(roomID, eventID string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	ref, ok := p.messageThreads[eventID]
	if !ok || ref.roomID != roomID || ref.threadRootEventID == "" {
		return "", false
	}
	return ref.threadRootEventID, true
}

// HasInteraction reports whether userID has a derived relationship with one
// channel-room thread.
func (p *ThreadProjection) HasInteraction(userID, roomID, threadRootEventID string) bool {
	p.RLock()
	defer p.RUnlock()
	interaction := p.interactions[userID][threadFollowKeyPart(roomID, threadRootEventID)]
	return interaction != nil && len(interaction.causes) > 0
}

// Interaction returns a detached relationship for one account and thread.
func (p *ThreadProjection) Interaction(userID, roomID, threadRootEventID string) (*ThreadInteraction, bool) {
	p.RLock()
	defer p.RUnlock()
	interaction := p.interactions[userID][threadFollowKeyPart(roomID, threadRootEventID)]
	if interaction == nil || len(interaction.causes) == 0 {
		return nil, false
	}
	return cloneThreadInteraction(interaction), true
}

func cloneThreadInteraction(interaction *projectedThreadInteraction) *ThreadInteraction {
	if interaction == nil {
		return nil
	}
	causes := make([]ThreadInteractionCause, 0, len(interaction.causes))
	for _, cause := range interaction.causes {
		causes = append(causes, cause)
	}
	slices.SortFunc(causes, func(a, b ThreadInteractionCause) int {
		if byTime := a.CreatedAt.Compare(b.CreatedAt); byTime != 0 {
			return byTime
		}
		if a.Kind < b.Kind {
			return -1
		}
		if a.Kind > b.Kind {
			return 1
		}
		if a.SourceEventID < b.SourceEventID {
			return -1
		}
		if a.SourceEventID > b.SourceEventID {
			return 1
		}
		return 0
	})
	return &ThreadInteraction{RoomID: interaction.roomID, ThreadRootEventID: interaction.threadRootEventID, Causes: causes}
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
			if entry.EventID != "" {
				replies++
			}
		}
	}
	return threads, entries, replies
}
