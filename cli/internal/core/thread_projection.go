package core

import (
	"sort"
	"time"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
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

type ThreadTimelineEntry struct {
	EventID   string
	StreamSeq uint64
}

// BotThreadContext is one active mention-derived thread grant.
type BotThreadContext struct {
	RoomID            string
	ThreadRootEventID string
	InviterIDs        []string
	InvitedAt         time.Time
}

type botThreadAccess struct {
	roomID            string
	threadRootEventID string
	inviterIDs        map[string]struct{}
	invitedAt         time.Time
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
	replySummaries  map[string]*threadReplySummary
	summaryByThread map[string]*threadSummary
	followState     map[string]ThreadFollowState
	followers       map[string]map[string]struct{}
	followedByUser  map[string]map[string]threadFollowRef
	botThreadAccess map[string]map[string]*botThreadAccess
	replayGuard     projectionReplayGuard
	shreddedUsers   map[string]struct{}
}

// NewThreadProjection returns an empty projection.
func NewThreadProjection() *ThreadProjection {
	return &ThreadProjection{
		byThread:        make(map[string][]ThreadTimelineEntry),
		messageToThread: make(map[string]string),
		replySummaries:  make(map[string]*threadReplySummary),
		summaryByThread: make(map[string]*threadSummary),
		followState:     make(map[string]ThreadFollowState),
		followers:       make(map[string]map[string]struct{}),
		followedByUser:  make(map[string]map[string]threadFollowRef),
		botThreadAccess: make(map[string]map[string]*botThreadAccess),
		replayGuard:     newProjectionReplayGuard(),
		shreddedUsers:   make(map[string]struct{}),
	}
}

// Subjects implements evtstream.Projection. Threads only need thread lifecycle
// and message mutation families, plus user key-shred events that can hide
// replies during crypto-shredding.
func (p *ThreadProjection) Subjects() []string {
	return []string{
		evtstream.RoomEventTypeFilter(evtstream.EventThreadCreated),
		evtstream.RoomEventTypeFilter(evtstream.EventThreadFollowed),
		evtstream.RoomEventTypeFilter(evtstream.EventThreadUnfollowed),
		evtstream.RoomEventTypeFilter(evtstream.EventBotThreadAccessRemoved),
		evtstream.RoomEventTypeFilter(evtstream.EventMessagePosted),
		evtstream.RoomEventTypeFilter(evtstream.EventUserLeftRoom),
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
// Root messages and replies with direct bot mentions also create scoped bot
// thread contexts. A bot leave or explicit revocation removes those contexts.
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
	case *corev1.Event_UserKeyShreddingRequested:
		p.applyUserKeyShreddedLocked(e.UserKeyShreddingRequested.GetUserId(), markApplied)
	case *corev1.Event_UserKeyShredded:
		p.applyUserKeyShreddedLocked(e.UserKeyShredded.GetUserId(), markApplied)

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

	case *corev1.Event_BotThreadAccessRemoved:
		removed := e.BotThreadAccessRemoved
		p.removeBotThreadAccessLocked(removed.GetBotId(), removed.GetRoomId(), removed.GetThreadRootEventId())
		markApplied()

	case *corev1.Event_UserLeftRoom:
		p.removeBotRoomAccessLocked(event.GetActorId(), e.UserLeftRoom.GetRoomId())
		markApplied()

	case *corev1.Event_MessagePosted:
		m := e.MessagePosted
		threadRoot := m.GetInThread()
		invited := false
		if m.GetEchoOfEventId() == "" && len(m.GetDirectMentionedBotIds()) > 0 {
			if threadRoot == "" {
				threadRoot = event.GetId()
			}
			for _, userID := range m.GetDirectMentionedBotIds() {
				p.addBotThreadAccessLocked(userID, m.GetRoomId(), threadRoot, event.GetActorId(), eventCreatedAt(event))
				invited = true
			}
		}
		threadRoot = m.GetInThread()
		if threadRoot == "" {
			if invited {
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

	case *corev1.Event_MessageEdited:
		_, ok := p.messageToThread[e.MessageEdited.GetEventId()]
		if !ok {
			return nil // target isn't a known thread reply
		}
		markApplied()

	case *corev1.Event_MessageRetracted:
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

func (p *ThreadProjection) addBotThreadAccessLocked(botID, roomID, threadRootEventID, inviterID string, invitedAt time.Time) {
	if botID == "" || roomID == "" || threadRootEventID == "" || inviterID == "" {
		return
	}
	contexts := p.botThreadAccess[botID]
	if contexts == nil {
		contexts = make(map[string]*botThreadAccess)
		p.botThreadAccess[botID] = contexts
	}
	key := threadFollowKeyPart(roomID, threadRootEventID)
	context := contexts[key]
	if context == nil {
		context = &botThreadAccess{
			roomID:            roomID,
			threadRootEventID: threadRootEventID,
			inviterIDs:        make(map[string]struct{}),
			invitedAt:         invitedAt,
		}
		contexts[key] = context
	}
	context.inviterIDs[inviterID] = struct{}{}
	if invitedAt.After(context.invitedAt) {
		context.invitedAt = invitedAt
	}
}

func (p *ThreadProjection) removeBotThreadAccessLocked(botID, roomID, threadRootEventID string) {
	contexts := p.botThreadAccess[botID]
	if contexts == nil {
		return
	}
	delete(contexts, threadFollowKeyPart(roomID, threadRootEventID))
	if len(contexts) == 0 {
		delete(p.botThreadAccess, botID)
	}
}

func (p *ThreadProjection) removeBotRoomAccessLocked(botID, roomID string) {
	contexts := p.botThreadAccess[botID]
	for key, context := range contexts {
		if context != nil && context.roomID == roomID {
			delete(contexts, key)
		}
	}
	if len(contexts) == 0 {
		delete(p.botThreadAccess, botID)
	}
}

// BotThreadContext returns a detached active context, when present.
func (p *ThreadProjection) BotThreadContext(botID, roomID, threadRootEventID string) (BotThreadContext, bool) {
	p.RLock()
	defer p.RUnlock()
	context := p.botThreadAccess[botID][threadFollowKeyPart(roomID, threadRootEventID)]
	if context == nil {
		return BotThreadContext{}, false
	}
	return detachedBotThreadContext(context), true
}

// BotThreadContexts returns all active contexts for a bot in stable order.
func (p *ThreadProjection) BotThreadContexts(botID string) []BotThreadContext {
	p.RLock()
	defer p.RUnlock()
	contexts := p.botThreadAccess[botID]
	out := make([]BotThreadContext, 0, len(contexts))
	for _, context := range contexts {
		if context != nil {
			out = append(out, detachedBotThreadContext(context))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RoomID == out[j].RoomID {
			return out[i].ThreadRootEventID < out[j].ThreadRootEventID
		}
		return out[i].RoomID < out[j].RoomID
	})
	return out
}

func detachedBotThreadContext(context *botThreadAccess) BotThreadContext {
	inviterIDs := make([]string, 0, len(context.inviterIDs))
	for inviterID := range context.inviterIDs {
		inviterIDs = append(inviterIDs, inviterID)
	}
	sort.Strings(inviterIDs)
	return BotThreadContext{
		RoomID: context.roomID, ThreadRootEventID: context.threadRootEventID,
		InviterIDs: inviterIDs, InvitedAt: context.invitedAt,
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
		Exists:         true,
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
			if entry.EventID != "" {
				replies++
			}
		}
	}
	return threads, entries, replies
}
