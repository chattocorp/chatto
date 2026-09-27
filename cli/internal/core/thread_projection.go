package core

import (
	"time"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// threadReply is the retained state of one thread reply. root is an eventIDs
// handle and actor is a principalIDs handle.
type threadReply struct {
	// createdAt is the reply's Unix time in nanoseconds when hasCreatedAt is
	// set. Server-assigned post times fall well inside the int64 range.
	createdAt    int64
	root         uint32
	actor        uint32
	hasCreatedAt bool
	retracted    bool
}

func (r threadReply) createdAtTime() time.Time {
	if !r.hasCreatedAt {
		return time.Time{}
	}
	return time.Unix(0, r.createdAt).UTC()
}

// threadEntry is one reply reference in a thread timeline; event is an
// eventIDs handle.
type threadEntry struct {
	streamSeq uint64
	event     uint32
}

// threadSummary caches display metadata for one thread. It is derived from
// the thread's entries and replies and is rebuilt when a retraction or key
// shredding changes which replies are visible.
type threadSummary struct {
	replyCount int
	// lastReplyAt is the latest visible reply's Unix time in nanoseconds when
	// hasLastReplyAt is set.
	lastReplyAt    int64
	hasLastReplyAt bool
	// latestReply is an eventIDs handle.
	latestReply uint32
	// participants is the bounded display preview in first-reply order, as
	// principalIDs handles.
	participants []uint32
	// participantCounts counts visible replies per principalIDs author handle.
	participantCounts map[uint32]int
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

// threadFollowStateKey avoids allocating a joined user/room/thread string for
// every follow fact. The three IDs already live in the event-derived indexes.
type threadFollowStateKey struct {
	userID string
	threadFollowRef
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
	// byThread maps a thread root handle to its reply references in stream
	// order. An entry without replies records an explicitly created thread.
	byThread map[uint32][]threadEntry
	// replies maps a reply handle to its root, author, and visibility.
	replies      map[uint32]threadReply
	channelRooms map[string]struct{}
	// dmRooms retains membership at the current replay position so a DM post
	// establishes relationships only for accounts that received it.
	dmRooms map[string]map[string]struct{}
	// principalIDs interns user and room IDs. The table stays small, so the
	// lookups on authorization read paths stay cache-resident.
	principalIDs projectionIDTable
	// eventIDs interns message and thread-root event IDs.
	eventIDs projectionIDTable
	// messageRefs is indexed by eventIDs handle minus one.
	messageRefs handleSlice[threadMessageRef]
	// interactions maps each relationship to its room handle.
	interactions    map[threadInteractionKey]uint32
	summaryByThread map[uint32]*threadSummary
	followState     map[threadFollowStateKey]compactThreadFollowState
	followers       map[threadFollowRef]map[string]struct{}
	followedByUser  map[string]map[threadFollowRef]struct{}
	replayGuard     projectionReplayGuard
	shreddedUsers   map[string]struct{}
}

// NewThreadProjection returns an empty projection.
func NewThreadProjection() *ThreadProjection {
	return &ThreadProjection{
		byThread:        make(map[uint32][]threadEntry),
		replies:         make(map[uint32]threadReply),
		channelRooms:    make(map[string]struct{}),
		dmRooms:         make(map[string]map[string]struct{}),
		principalIDs:    newProjectionIDTable(),
		eventIDs:        newProjectionIDTable(),
		interactions:    make(map[threadInteractionKey]uint32),
		summaryByThread: make(map[uint32]*threadSummary),
		followState:     make(map[threadFollowStateKey]compactThreadFollowState),
		followers:       make(map[threadFollowRef]map[string]struct{}),
		followedByUser:  make(map[string]map[threadFollowRef]struct{}),
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
		replyHandle := p.eventIDs.intern(event.GetId())
		p.byThread[threadRoot] = append(p.byThread[threadRoot], threadEntry{event: replyHandle, streamSeq: seq})
		reply := threadReply{root: threadRoot, actor: p.principalIDs.intern(messageAuthorID(event))}
		if created := event.GetCreatedAt(); created != nil {
			reply.createdAt = created.AsTime().UnixNano()
			reply.hasCreatedAt = true
		}
		p.replies[replyHandle] = reply
		summary := p.summaryByThread[threadRoot]
		if summary == nil {
			summary = newThreadSummary()
			p.summaryByThread[threadRoot] = summary
		}
		p.applyReplyToSummaryLocked(summary, replyHandle)
		markApplied()

	case *evtv1.Event_MessageEdited:
		if _, ok := p.replyLocked(e.MessageEdited.GetEventId()); !ok {
			return nil // target isn't a known thread reply
		}
		markApplied()

	case *evtv1.Event_MessageRetracted:
		handle, ok := p.eventIDs.lookup(e.MessageRetracted.GetEventId())
		if !ok {
			return nil
		}
		reply, ok := p.replies[handle]
		if !ok {
			return nil
		}
		reply.retracted = true
		p.replies[handle] = reply
		// Retractions are rare and can invalidate last-reply or participant
		// ordering, so recomputing the affected thread keeps the hot reply
		// path O(1) without making removal bookkeeping subtle.
		p.recomputeSummaryLocked(reply.root)
		markApplied()
	}
	return nil
}

// replyLocked returns the retained state of a known thread reply.
func (p *ThreadProjection) replyLocked(eventID string) (threadReply, bool) {
	handle, ok := p.eventIDs.lookup(eventID)
	if !ok {
		return threadReply{}, false
	}
	reply, ok := p.replies[handle]
	return reply, ok
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
// a deleted room. The room's message IDs stay in the append-only ID table;
// only a snapshot restore, which interns live state only, removes them.
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

func threadFollowKeyPart(roomID, threadRootEventID string) string {
	return roomID + "\x00" + threadRootEventID
}

func (p *ThreadProjection) setThreadFollowStateLocked(userID, roomID, threadRootEventID string, state ThreadFollowState) {
	if userID == "" || roomID == "" || threadRootEventID == "" {
		return
	}
	key := threadFollowRef{roomID: roomID, threadRootEventID: threadRootEventID}
	stateKey := threadFollowStateKey{userID: userID, threadFollowRef: key}
	previous := p.followState[stateKey]
	compactState := compactFollowState(state)
	if previous == compactState {
		return
	}

	if previous == compactThreadFollowFollowing {
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

	p.followState[stateKey] = compactState

	if state == ThreadFollowStateFollowing {
		followers := p.followers[key]
		if followers == nil {
			followers = make(map[string]struct{})
			p.followers[key] = followers
		}
		followers[userID] = struct{}{}

		followed := p.followedByUser[userID]
		if followed == nil {
			followed = make(map[threadFollowRef]struct{})
			p.followedByUser[userID] = followed
		}
		followed[key] = struct{}{}
	}
}

func newThreadSummary() *threadSummary {
	return &threadSummary{
		participantCounts: make(map[uint32]int),
	}
}

func (p *ThreadProjection) recomputeSummaryLocked(threadRoot uint32) {
	summary := p.summaryByThread[threadRoot]
	if summary == nil {
		summary = newThreadSummary()
		p.summaryByThread[threadRoot] = summary
	} else if summary.participantCounts == nil {
		summary.participantCounts = make(map[uint32]int)
	}

	summary.replyCount = 0
	summary.lastReplyAt = 0
	summary.hasLastReplyAt = false
	summary.latestReply = 0
	summary.participants = nil
	clear(summary.participantCounts)

	for _, entry := range p.byThread[threadRoot] {
		p.applyReplyToSummaryLocked(summary, entry.event)
	}
}

func (p *ThreadProjection) applyReplyToSummaryLocked(summary *threadSummary, replyHandle uint32) {
	if summary == nil || replyHandle == 0 {
		return
	}
	if summary.participantCounts == nil {
		summary.participantCounts = make(map[uint32]int)
	}

	reply, ok := p.replies[replyHandle]
	if !ok || reply.retracted {
		return
	}
	if _, shredded := p.shreddedUsers[p.principalIDs.id(reply.actor)]; shredded {
		return
	}

	summary.replyCount++
	summary.latestReply = replyHandle
	summary.lastReplyAt = reply.createdAt
	summary.hasLastReplyAt = reply.hasCreatedAt
	if reply.actor != 0 {
		summary.participantCounts[reply.actor]++
		if summary.participantCounts[reply.actor] == 1 && len(summary.participants) < maxThreadParticipants {
			summary.participants = append(summary.participants, reply.actor)
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
		ParticipantCount:   len(summary.participantCounts),
	}
	if len(summary.participants) > 0 {
		metadata.ParticipantIDs = make([]string, len(summary.participants))
		for i, participant := range summary.participants {
			metadata.ParticipantIDs[i] = p.principalIDs.id(participant)
		}
	}
	if summary.hasLastReplyAt {
		at := time.Unix(0, summary.lastReplyAt).UTC()
		metadata.LastReplyAt = &at
	}
	return metadata
}

func (p *ThreadProjection) FollowState(userID, roomID, threadRootEventID string) ThreadFollowState {
	p.RLock()
	defer p.RUnlock()
	return p.followState[threadFollowStateKey{userID: userID, threadFollowRef: threadFollowRef{roomID: roomID, threadRootEventID: threadRootEventID}}].public()
}

func (p *ThreadProjection) ThreadFollowers(roomID, threadRootEventID string) []string {
	p.RLock()
	defer p.RUnlock()
	followers := p.followers[threadFollowRef{roomID: roomID, threadRootEventID: threadRootEventID}]
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
	for ref := range followed {
		refs = append(refs, ref)
	}
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
	ids := make([]string, 0, len(summary.participantCounts))
	for participant, count := range summary.participantCounts {
		if count > 0 {
			ids = append(ids, p.principalIDs.id(participant))
		}
	}
	return ids
}
