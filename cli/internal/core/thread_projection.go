package core

import (
	"cmp"
	"slices"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// threadEntry is one reply in a thread timeline and its retained state. event
// is an eventIDs handle and actor is a principalIDs handle. The entry contains
// no Go pointers.
type threadEntry struct {
	streamSeq uint64
	// createdAt is the reply's compact creation time (see projectionTime).
	createdAt int64
	event     uint32
	actor     uint32
	retracted bool
}

// threadSummary caches display metadata for one thread. It is derived from
// the thread's entries and is rebuilt when a retraction or key shredding
// changes which replies are visible.
type threadSummary struct {
	replyCount int
	// lastReplyAt is the latest visible reply's compact creation time (see
	// projectionTime).
	lastReplyAt int64
	// latestReply is an eventIDs handle.
	latestReply uint32
	// participants counts visible replies per author in first-reply order.
	// The first maxThreadParticipants entries form the display preview.
	participants []threadParticipant
	// participantIndex locates an author in participants once the thread has
	// more than threadParticipantIndexMin authors. Smaller threads scan the
	// slice, which costs less memory than a map.
	participantIndex map[uint32]int
}

// threadParticipantIndexMin is the author count above which a thread summary
// indexes its participants.
const threadParticipantIndexMin = 32

// countReply adds one visible reply by actor.
func (s *threadSummary) countReply(actor uint32) {
	index, ok := s.participantIndex[actor]
	if s.participantIndex == nil {
		index = slices.IndexFunc(s.participants, func(participant threadParticipant) bool { return participant.actor == actor })
		ok = index >= 0
	}
	if !ok {
		s.participants = append(s.participants, threadParticipant{actor: actor})
		index = len(s.participants) - 1
		if s.participantIndex != nil {
			s.participantIndex[actor] = index
		} else if len(s.participants) > threadParticipantIndexMin {
			s.participantIndex = make(map[uint32]int, len(s.participants))
			for i, participant := range s.participants {
				s.participantIndex[participant.actor] = i
			}
		}
	}
	s.participants[index].replies++
}

// threadParticipant counts one author's visible replies in a thread. actor is
// a principalIDs handle.
type threadParticipant struct {
	actor   uint32
	replies uint32
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

// threadFollowTarget identifies a followed thread by its principalIDs room
// handle and eventIDs root handle.
type threadFollowTarget struct {
	room uint32
	root uint32
}

// threadFollowKey identifies one user's follow relationship with one thread.
// user is a principalIDs handle.
type threadFollowKey struct {
	user uint32
	threadFollowTarget
}

type compactThreadFollowState uint8

const (
	compactThreadFollowNone compactThreadFollowState = iota
	compactThreadFollowFollowing
	compactThreadFollowUnfollowed
)

func compactFollowState(state ThreadFollowState) compactThreadFollowState {
	if state == ThreadFollowStateFollowing {
		return compactThreadFollowFollowing
	}
	if state == ThreadFollowStateUnfollowed {
		return compactThreadFollowUnfollowed
	}
	return compactThreadFollowNone
}

func (state compactThreadFollowState) public() ThreadFollowState {
	if state == compactThreadFollowFollowing {
		return ThreadFollowStateFollowing
	}
	if state == compactThreadFollowUnfollowed {
		return ThreadFollowStateUnfollowed
	}
	return ThreadFollowStateNone
}

// threadMessageRef maps one projected message to its room and canonical
// thread root. room is a principalIDs handle and root is an eventIDs handle;
// a zero room means the handle does not name a projected message.
type threadMessageRef struct {
	room uint32
	root uint32
}

// threadInteractionKey identifies one account-to-thread relationship by a
// principalIDs user handle and an eventIDs root handle. Event IDs are globally
// unique, so the root determines the room; the interactions map stores the
// room handle as its value. A relationship is a durable post-time fact; the
// projection keeps only its existence because reads ask only whether it
// exists (FDR-039).
type threadInteractionKey struct {
	user uint32
	root uint32
}

// ThreadTimelineEntry is one detached reply reference returned by
// ThreadEvents.
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
	// eventIDs interns message and thread-root event IDs. The ServerContentView
	// shares one table with the room timeline and reaction components; a
	// standalone projection owns a private table. It does not change after
	// construction.
	eventIDs       *eventIDTable
	sharedEventIDs bool
	threadProjectionState
}

// threadProjectionState is the snapshot-restorable state of a ThreadProjection. Restore
// builds a new value and replaces the complete state at once.
type threadProjectionState struct {
	// byThread maps a thread root handle to its replies in stream order. An
	// entry without replies records an explicitly created thread.
	byThread map[uint32][]threadEntry
	// replyRoots maps a reply handle to its thread root handle.
	replyRoots   map[uint32]uint32
	channelRooms map[string]struct{}
	// dmRooms retains membership at the current replay position so a DM post
	// establishes relationships only for accounts that received it.
	dmRooms map[string]map[string]struct{}
	// principalIDs interns user and room IDs. The table stays small, so the
	// lookups on authorization read paths stay cache-resident.
	principalIDs projectionIDTable
	// messageRefs is indexed by eventIDs handle minus one.
	messageRefs handleSlice[threadMessageRef]
	// interactions maps each relationship to its room handle.
	interactions    map[threadInteractionKey]uint32
	summaryByThread map[uint32]*threadSummary
	// followState holds the latest explicit follow state of each user and
	// thread. followers and followedByUser index the current follows in
	// follow order; reads sort them by ID.
	followState    map[threadFollowKey]compactThreadFollowState
	followers      map[threadFollowTarget][]uint32
	followedByUser map[uint32][]threadFollowTarget
	replayGuard    projectionReplayGuard
	shreddedUsers  map[string]struct{}
}

// NewThreadProjection returns an empty projection with a private event ID
// table.
func NewThreadProjection() *ThreadProjection {
	return newThreadProjection(nil)
}

// newThreadProjection returns an empty projection that interns event IDs in
// eventIDs. A nil table gives the projection a private table.
func newThreadProjection(eventIDs *eventIDTable) *ThreadProjection {
	shared := eventIDs != nil
	if !shared {
		eventIDs = newEventIDTable()
	}
	return &ThreadProjection{eventIDs: eventIDs, sharedEventIDs: shared, threadProjectionState: newThreadProjectionState()}
}

func newThreadProjectionState() threadProjectionState {
	return threadProjectionState{
		byThread:        make(map[uint32][]threadEntry),
		replyRoots:      make(map[uint32]uint32),
		channelRooms:    make(map[string]struct{}),
		dmRooms:         make(map[string]map[string]struct{}),
		principalIDs:    newProjectionIDTable(),
		interactions:    make(map[threadInteractionKey]uint32),
		summaryByThread: make(map[uint32]*threadSummary),
		followState:     make(map[threadFollowKey]compactThreadFollowState),
		followers:       make(map[threadFollowTarget][]uint32),
		followedByUser:  make(map[uint32][]threadFollowTarget),
		replayGuard:     newProjectionReplayGuard(),
		shreddedUsers:   make(map[string]struct{}),
	}
}

// Subjects implements evtstream.Projection. Room lifecycle, DM membership,
// and every message post supply the room and relationship indexes. Thread
// lifecycle, message mutation, and user key-shred facts supply the thread views.
func (p *ThreadProjection) Subjects() []string {
	return []string{
		evtstream.RoomEventTypeFilter(evtstream.EventRoomCreated),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomDeleted),
		evtstream.RoomEventTypeFilter(evtstream.EventUserJoinedRoom),
		evtstream.RoomEventTypeFilter(evtstream.EventUserLeftRoom),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomMemberBanned),
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
// Room lifecycle and DM membership establish the recipient set for message
// interactions. Edits/retracts of non-reply messages are silently ignored.
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
		if room.GetRoomId() == "" {
			return nil
		}
		switch room.GetKind() {
		case evtv1.RoomKind_ROOM_KIND_CHANNEL:
			p.channelRooms[room.GetRoomId()] = struct{}{}
		case evtv1.RoomKind_ROOM_KIND_DM:
			p.dmRooms[room.GetRoomId()] = make(map[string]struct{})
		default:
			return nil
		}
		markApplied()

	case *evtv1.Event_UserJoinedRoom:
		if members, dm := p.dmRooms[e.UserJoinedRoom.GetRoomId()]; dm && event.GetActorId() != "" {
			members[event.GetActorId()] = struct{}{}
			markApplied()
		}

	case *evtv1.Event_UserLeftRoom:
		if members, dm := p.dmRooms[e.UserLeftRoom.GetRoomId()]; dm {
			delete(members, event.GetActorId())
			markApplied()
		}

	case *evtv1.Event_RoomMemberBanned:
		if members, dm := p.dmRooms[e.RoomMemberBanned.GetRoomId()]; dm {
			delete(members, e.RoomMemberBanned.GetUserId())
			markApplied()
		}

	case *evtv1.Event_RoomDeleted:
		roomID := e.RoomDeleted.GetRoomId()
		_, channel := p.channelRooms[roomID]
		_, dm := p.dmRooms[roomID]
		if !channel && !dm {
			return nil
		}
		delete(p.channelRooms, roomID)
		delete(p.dmRooms, roomID)
		p.removeRoomInteractionStateLocked(roomID)
		markApplied()

	case *evtv1.Event_UserKeyShreddingRequested:
		p.applyUserKeyShreddedLocked(e.UserKeyShreddingRequested.GetUserId(), markApplied)
	case *evtv1.Event_UserKeyShredded:
		p.applyUserKeyShreddedLocked(e.UserKeyShredded.GetUserId(), markApplied)

	case *evtv1.Event_ThreadCreated:
		threadRootID := e.ThreadCreated.GetThreadRootEventId()
		if threadRootID == "" {
			return nil
		}
		threadRoot := p.eventIDs.intern(threadRootID)
		if _, exists := p.byThread[threadRoot]; !exists {
			p.byThread[threadRoot] = nil
		}
		if _, exists := p.summaryByThread[threadRoot]; !exists {
			p.summaryByThread[threadRoot] = &threadSummary{}
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
		if p.isInteractionRoomLocked(m.GetRoomId()) {
			p.applyMessageInteractionStateLocked(event, m)
		}
		threadRootID := m.GetInThread()
		if threadRootID == "" {
			if p.isInteractionRoomLocked(m.GetRoomId()) {
				markApplied()
			}
			return nil // root-level message; not in any thread bucket
		}
		if event.GetId() == "" {
			return nil
		}
		threadRoot := p.eventIDs.intern(threadRootID)
		entry := threadEntry{
			streamSeq: seq,
			event:     p.eventIDs.intern(event.GetId()),
			actor:     p.principalIDs.intern(messageAuthorID(event)),
			createdAt: eventCreatedNanos(event),
		}
		p.byThread[threadRoot] = append(p.byThread[threadRoot], entry)
		p.replyRoots[entry.event] = threadRoot
		summary := p.summaryByThread[threadRoot]
		if summary == nil {
			summary = &threadSummary{}
			p.summaryByThread[threadRoot] = summary
		}
		p.applyReplyToSummaryLocked(summary, entry)
		markApplied()

	case *evtv1.Event_MessageEdited:
		if _, ok := p.replyRootLocked(e.MessageEdited.GetEventId()); !ok {
			return nil // target isn't a known thread reply
		}
		markApplied()

	case *evtv1.Event_MessageRetracted:
		handle, ok := p.eventIDs.lookup(e.MessageRetracted.GetEventId())
		if !ok {
			return nil
		}
		root, ok := p.replyRoots[handle]
		if !ok {
			return nil
		}
		entries := p.byThread[root]
		for i := range entries {
			if entries[i].event == handle {
				entries[i].retracted = true
			}
		}
		// Retractions are rare and can invalidate last-reply or participant
		// ordering, so recomputing the affected thread keeps the hot reply
		// path O(1) without making removal bookkeeping subtle.
		p.recomputeSummaryLocked(root)
		markApplied()
	}
	return nil
}

// replyRootLocked returns the thread root handle of a known thread reply.
func (p *ThreadProjection) replyRootLocked(eventID string) (uint32, bool) {
	handle, ok := p.eventIDs.lookup(eventID)
	if !ok {
		return 0, false
	}
	root, ok := p.replyRoots[handle]
	return root, ok
}

// summaryLocked returns the cached summary for a thread root ID.
func (p *ThreadProjection) summaryLocked(rootEventID string) *threadSummary {
	root, ok := p.eventIDs.lookup(rootEventID)
	if !ok {
		return nil
	}
	return p.summaryByThread[root]
}

func (p *ThreadProjection) isInteractionRoomLocked(roomID string) bool {
	if _, channel := p.channelRooms[roomID]; channel {
		return true
	}
	_, dm := p.dmRooms[roomID]
	return dm
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
	room := p.principalIDs.intern(message.GetRoomId())
	root := p.eventIDs.intern(rootID)
	p.messageRefs.set(p.eventIDs.intern(event.GetId()), threadMessageRef{room: room, root: root})
	if message.GetHistoricalImport() {
		return
	}

	// Either echo field identifies derived channel-echo state. Malformed or
	// partially upgraded echo facts must not create interactions.
	if message.GetEchoOfEventId() != "" || message.GetEchoFromThreadRootEventId() != "" {
		return
	}
	if message.GetInThread() == "" {
		p.addInteractionLocked(event.GetActorId(), room, root)
	}
	for userID := range p.dmRooms[message.GetRoomId()] {
		if userID == event.GetActorId() {
			continue
		}
		p.addInteractionLocked(userID, room, root)
	}
	for _, mention := range message.GetMentions() {
		if mention == nil || mention.GetUserId() == "" || mention.GetUserId() == event.GetActorId() {
			continue
		}
		if _, direct := mention.GetCause().(*evtv1.MessageMention_Direct); !direct {
			continue
		}
		p.addInteractionLocked(mention.GetUserId(), room, root)
	}
}

// addInteractionLocked records that userID has a relationship with the thread.
// Repeated causes for the same relationship are idempotent.
func (p *ThreadProjection) addInteractionLocked(userID string, room, root uint32) {
	if userID == "" || room == 0 || root == 0 {
		return
	}
	key := threadInteractionKey{user: p.principalIDs.intern(userID), root: root}
	// Posting validates that a thread root belongs to the reply's room, so the
	// first recorded room stays authoritative.
	if _, exists := p.interactions[key]; !exists {
		p.interactions[key] = room
	}
}

// removeRoomInteractionStateLocked drops the message refs and relationships of
// a deleted room. The room's message IDs stay in the append-only event ID
// table.
func (p *ThreadProjection) removeRoomInteractionStateLocked(roomID string) {
	room, ok := p.principalIDs.lookup(roomID)
	if !ok {
		return
	}
	for i := range p.messageRefs {
		if p.messageRefs[i].room == room {
			p.messageRefs[i] = threadMessageRef{}
		}
	}
	for key, interactionRoom := range p.interactions {
		if interactionRoom == room {
			delete(p.interactions, key)
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

func (p *ThreadProjection) setThreadFollowStateLocked(userID, roomID, threadRootEventID string, state ThreadFollowState) {
	if userID == "" || roomID == "" || threadRootEventID == "" {
		return
	}
	target := threadFollowTarget{room: p.principalIDs.intern(roomID), root: p.eventIDs.intern(threadRootEventID)}
	user := p.principalIDs.intern(userID)
	key := threadFollowKey{user: user, threadFollowTarget: target}
	previous := p.followState[key]
	compactState := compactFollowState(state)
	if previous == compactState {
		return
	}

	if previous == compactThreadFollowFollowing {
		if followers := slices.DeleteFunc(p.followers[target], func(follower uint32) bool { return follower == user }); len(followers) == 0 {
			delete(p.followers, target)
		} else {
			p.followers[target] = followers
		}
		if followed := slices.DeleteFunc(p.followedByUser[user], func(thread threadFollowTarget) bool { return thread == target }); len(followed) == 0 {
			delete(p.followedByUser, user)
		} else {
			p.followedByUser[user] = followed
		}
	}

	p.followState[key] = compactState

	if state == ThreadFollowStateFollowing {
		p.followers[target] = append(p.followers[target], user)
		p.followedByUser[user] = append(p.followedByUser[user], target)
	}
}

// followTargetLocked returns the handles of a thread without interning its IDs.
func (p *ThreadProjection) followTargetLocked(roomID, threadRootEventID string) (threadFollowTarget, bool) {
	room, roomKnown := p.principalIDs.lookup(roomID)
	root, rootKnown := p.eventIDs.lookup(threadRootEventID)
	return threadFollowTarget{room: room, root: root}, roomKnown && rootKnown
}

func (p *ThreadProjection) recomputeSummaryLocked(threadRoot uint32) {
	summary := p.summaryByThread[threadRoot]
	if summary == nil {
		summary = &threadSummary{}
		p.summaryByThread[threadRoot] = summary
	}
	*summary = threadSummary{participants: summary.participants[:0]}
	for _, entry := range p.byThread[threadRoot] {
		p.applyReplyToSummaryLocked(summary, entry)
	}
	if len(summary.participants) == 0 {
		summary.participants = nil
	}
}

func (p *ThreadProjection) applyReplyToSummaryLocked(summary *threadSummary, entry threadEntry) {
	if summary == nil || entry.event == 0 || entry.retracted {
		return
	}
	if _, shredded := p.shreddedUsers[p.principalIDs.id(entry.actor)]; shredded {
		return
	}

	summary.replyCount++
	summary.latestReply = entry.event
	summary.lastReplyAt = entry.createdAt
	if entry.actor != 0 {
		summary.countReply(entry.actor)
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
	root, ok := p.eventIDs.lookup(rootEventID)
	if !ok {
		return nil
	}
	entries := p.byThread[root]
	if len(entries) == 0 {
		return nil
	}
	out := make([]ThreadTimelineEntry, len(entries))
	for i, entry := range entries {
		out[i] = ThreadTimelineEntry{EventID: p.eventIDs.id(entry.event), StreamSeq: entry.streamSeq}
	}
	return out
}

// ReplyCount returns how many visible MessagePostedEvent replies the thread
// has accumulated. Edits don't bump the count; retractions and key-shredded
// authors remove replies from the visible summary.
func (p *ThreadProjection) ReplyCount(rootEventID string) int {
	p.RLock()
	defer p.RUnlock()
	summary := p.summaryLocked(rootEventID)
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
	summary := p.summaryLocked(rootEventID)
	if summary == nil {
		return &ThreadMetadata{}
	}
	metadata := &ThreadMetadata{
		Exists:             true,
		ReplyCount:         summary.replyCount,
		LatestReplyEventID: p.eventIDs.id(summary.latestReply),
		ParticipantCount:   len(summary.participants),
	}
	if preview := summary.participants[:min(len(summary.participants), maxThreadParticipants)]; len(preview) > 0 {
		metadata.ParticipantIDs = make([]string, len(preview))
		for i, participant := range preview {
			metadata.ParticipantIDs[i] = p.principalIDs.id(participant.actor)
		}
	}
	if summary.lastReplyAt != 0 {
		at := projectionTime(summary.lastReplyAt)
		metadata.LastReplyAt = &at
	}
	return metadata
}

func (p *ThreadProjection) FollowState(userID, roomID, threadRootEventID string) ThreadFollowState {
	p.RLock()
	defer p.RUnlock()
	user, userKnown := p.principalIDs.lookup(userID)
	target, targetKnown := p.followTargetLocked(roomID, threadRootEventID)
	if !userKnown || !targetKnown {
		return ThreadFollowStateNone
	}
	return p.followState[threadFollowKey{user: user, threadFollowTarget: target}].public()
}

// ThreadFollowers returns the users who currently follow a thread, sorted by
// user ID. A restore does not keep the follow order, so reads do not expose
// it.
func (p *ThreadProjection) ThreadFollowers(roomID, threadRootEventID string) []string {
	p.RLock()
	defer p.RUnlock()
	target, known := p.followTargetLocked(roomID, threadRootEventID)
	followers := p.followers[target]
	if !known || len(followers) == 0 {
		return nil
	}
	userIDs := make([]string, len(followers))
	for i, user := range followers {
		userIDs[i] = p.principalIDs.id(user)
	}
	slices.Sort(userIDs)
	return userIDs
}

// FollowedThreadsForUser returns the threads that a user currently follows,
// sorted by room ID and then thread root ID.
func (p *ThreadProjection) FollowedThreadsForUser(userID string) []threadFollowRef {
	p.RLock()
	defer p.RUnlock()
	user, known := p.principalIDs.lookup(userID)
	followed := p.followedByUser[user]
	if !known || len(followed) == 0 {
		return nil
	}
	refs := make([]threadFollowRef, len(followed))
	for i, target := range followed {
		refs[i] = threadFollowRef{roomID: p.principalIDs.id(target.room), threadRootEventID: p.eventIDs.id(target.root)}
	}
	slices.SortFunc(refs, func(a, b threadFollowRef) int {
		return cmp.Or(cmp.Compare(a.roomID, b.roomID), cmp.Compare(a.threadRootEventID, b.threadRootEventID))
	})
	return refs
}

// ThreadRootForMessage returns the canonical thread root for one projected
// channel-room message, including roots, replies, and channel echoes.
func (p *ThreadProjection) ThreadRootForMessage(roomID, eventID string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	handle, ok := p.eventIDs.lookup(eventID)
	if !ok {
		return "", false
	}
	ref, ok := p.messageRefs.get(handle)
	if !ok || ref.root == 0 || p.principalIDs.id(ref.room) != roomID {
		return "", false
	}
	return p.eventIDs.id(ref.root), true
}

// HasInteraction reports whether userID has a derived relationship with one
// channel-room or DM thread.
func (p *ThreadProjection) HasInteraction(userID, roomID, threadRootEventID string) bool {
	p.RLock()
	defer p.RUnlock()
	user, userKnown := p.principalIDs.lookup(userID)
	root, rootKnown := p.eventIDs.lookup(threadRootEventID)
	if !userKnown || !rootKnown {
		return false
	}
	room, ok := p.interactions[threadInteractionKey{user: user, root: root}]
	return ok && p.principalIDs.id(room) == roomID
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
	root, ok := p.eventIDs.lookup(rootEventID)
	if !ok {
		return false
	}
	_, ok = p.byThread[root]
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
			if entry.event != 0 {
				replies++
			}
		}
	}
	return threads, entries, replies
}

// ParticipantIDs returns the complete current reply-author set, independent of
// the bounded display preview. Retractions and key shredding update this set.
func (p *ThreadProjection) ParticipantIDs(rootEventID string) []string {
	p.RLock()
	defer p.RUnlock()
	summary := p.summaryLocked(rootEventID)
	if summary == nil {
		return nil
	}
	ids := make([]string, len(summary.participants))
	for i, participant := range summary.participants {
		ids[i] = p.principalIDs.id(participant.actor)
	}
	return ids
}
