package core

import (
	"slices"
	"strings"
	"time"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// RoomTimelineProjection holds the visible append-only event log per room.
//
// It consumes the full evt.room.> firehose, but only room-visible events land
// in the owning room's timeline slice. Folded state such as edits, retractions,
// thread replies, reactions, and asset-processing events is maintained through
// focused derived indexes or sibling projections rather than bloating the room
// timeline readers walk on every page load.
type RoomTimelineProjection struct {
	events.MemoryProjection
	entries []timelineRow
	// unresolvedRefs holds IDs whose target row had not arrived when a row was
	// appended. Most message references use compact one-based row indexes.
	unresolvedRefs     map[int]timelineUnresolvedRefs
	roomIDs            map[string]uint32
	rooms              []string
	userIDs            map[string]uint32
	users              []string
	byRoom             map[string][]int
	byEventID          map[string]int
	messagePostsByRoom map[string][]int
	// latestOriginalPostAt retains the newest committed non-echo post
	// timestamp per room and actor. Slow mode reads this O(1) index; edits and
	// retractions intentionally do not change it.
	latestOriginalPostAt map[roomActorKey]time.Time
	replayGuard          projectionReplayGuard
	// bodyStates is a dense array addressed by message rows. Only bodies that
	// arrive before their post need an event-ID map until that post is indexed.
	// Complete encrypted bodies remain in EVT.
	bodyStates       []timelineBodyState
	orphanBodyStates map[string]timelineBodyState
	retractedFlags   map[string]struct{}
	// tombstonedAt records when message content first became unavailable
	// through a durable retraction or user key-shred fact. It deliberately does
	// not cover missing/corrupt body payloads so clients can distinguish those
	// states from deletions.
	tombstonedAt map[string]time.Time
	shreddedAt   map[string]time.Time
	// attachmentMessageIDsByRoom tracks messages whose current body contains
	// attachment/asset references. It lets room file reads page over current
	// file-bearing messages instead of decrypting every message body in a room.
	attachmentMessageIDsByRoom map[string][]string
	attachmentMessageRoom      map[string]string
	// echoLinks maps an original message's event_id to the event_ids
	// of any echoes pointing at it. Maintained as MessagePostedEvents
	// with EchoOfEventId arrive. Content reads resolve the original;
	// physical body history remains separate for legacy secure deletion.
	echoLinks map[string][]string
	// hiddenEchoes tracks echo MessagePostedEvents that were directly
	// retracted. A direct echo retract removes the room-timeline copy
	// without deleting the original thread reply's content.
	hiddenEchoes         map[string]struct{}
	shreddedUsers        map[string]struct{}
	pinnedMessagesByRoom map[string]map[string]PinnedMessageState
	latestPinByRoom      map[string]latestRoomPinState
}

type latestRoomPinState struct {
	PinEventID  string
	PinSequence uint64
}

type roomActorKey struct {
	roomID  string
	actorID string
}

// TimelineEntry is a detached read reference for one projected room event.
// It carries values needed to select, authorize, order, and validate EVT
// hydration. The projection retains timelineRow values; complete event
// payloads remain in EVT.
type TimelineEntry struct {
	StreamSeq         uint64
	EventID           string
	RoomID            string
	ActorID           string
	MessageAuthorID   string
	CreatedAt         time.Time
	EventType         string
	ThreadRootEventID string
	InThreadEventID   string
	EchoOfEventID     string
	HistoricalImport  bool
}

// timelineRow stores only projection-owned data. Public reads reconstruct a
// detached TimelineEntry; room and user names are shared across all rows.
type timelineRow struct {
	streamSeq        uint64
	eventID          string
	createdAt        time.Time
	threadRoot       uint32
	inThread         uint32
	echoOf           uint32
	room             uint32
	actor            uint32
	author           uint32
	kind             timelineEventKind
	historicalImport bool
	bodyIndex        uint32 // one-based index into bodyStates; zero for non-posts
}

type timelineUnresolvedRefs struct {
	threadRootEventID string
	inThreadEventID   string
	echoOfEventID     string
}

type timelineEventKind uint8

const (
	timelineUnknown timelineEventKind = iota
	timelineMessagePosted
	timelineRoomCreated
	timelineRoomUpdated
	timelineRoomDeleted
	timelineRoomArchived
	timelineRoomUnarchived
	timelineRoomThreadingModeChanged
	timelineUserJoinedRoom
	timelineUserLeftRoom
	timelineCallStarted
	timelineCallEnded
)

func timelineKind(eventType string) timelineEventKind {
	switch eventType {
	case evtstream.EventMessagePosted:
		return timelineMessagePosted
	case evtstream.EventRoomCreated:
		return timelineRoomCreated
	case evtstream.EventRoomUpdated:
		return timelineRoomUpdated
	case evtstream.EventRoomDeleted:
		return timelineRoomDeleted
	case evtstream.EventRoomArchived:
		return timelineRoomArchived
	case evtstream.EventRoomUnarchived:
		return timelineRoomUnarchived
	case evtstream.EventRoomThreadingModeChanged:
		return timelineRoomThreadingModeChanged
	case evtstream.EventUserJoinedRoom:
		return timelineUserJoinedRoom
	case evtstream.EventUserLeftRoom:
		return timelineUserLeftRoom
	case evtstream.EventCallStarted:
		return timelineCallStarted
	case evtstream.EventCallEnded:
		return timelineCallEnded
	default:
		return timelineUnknown
	}
}

func (k timelineEventKind) eventType() string {
	switch k {
	case timelineMessagePosted:
		return evtstream.EventMessagePosted
	case timelineRoomCreated:
		return evtstream.EventRoomCreated
	case timelineRoomUpdated:
		return evtstream.EventRoomUpdated
	case timelineRoomDeleted:
		return evtstream.EventRoomDeleted
	case timelineRoomArchived:
		return evtstream.EventRoomArchived
	case timelineRoomUnarchived:
		return evtstream.EventRoomUnarchived
	case timelineRoomThreadingModeChanged:
		return evtstream.EventRoomThreadingModeChanged
	case timelineUserJoinedRoom:
		return evtstream.EventUserJoinedRoom
	case timelineUserLeftRoom:
		return evtstream.EventUserLeftRoom
	case timelineCallStarted:
		return evtstream.EventCallStarted
	case timelineCallEnded:
		return evtstream.EventCallEnded
	default:
		return ""
	}
}

// IsMessagePost reports whether this reference points to a durable message
// post event.
func (e TimelineEntry) IsMessagePost() bool {
	return e.EventType == evtstream.EventMessagePosted
}

// PinnedMessageState is the current derived pin association for one canonical
// message. Message content remains owned by the timeline projection's normal
// message indexes and is never copied into this state.
type PinnedMessageState struct {
	PinEventID     string
	PinSequence    uint64
	RoomID         string
	MessageEventID string
}

// RoomTimelineMessageHydrationState is the detached projection state needed to
// render one message in a public timeline response.
type RoomTimelineMessageHydrationState struct {
	DeletedAt          time.Time
	HasDeletedAt       bool
	ChannelEchoEventID string
	Pinned             bool
}

type projectedRoomAttachmentMessage struct {
	Entry              *TimelineEntry
	BodyMessageEventID string
	BodySequence       uint64
	BodyEventID        string
	BodyAuthorID       string
	AttachmentCount    int
}

// TimelineBodyReference identifies one active MessageBodyEvent in EVT. The
// reference is detached from projection state and contains no message payload.
type TimelineBodyReference struct {
	MessageEventID  string
	BodyEventID     string
	RoomID          string
	AuthorID        string
	StreamSeq       uint64
	AttachmentCount int
}

type timelineBodyState struct {
	currentSequence     uint64
	currentEventID      string
	authorID            string
	attachmentCount     int
	active              bool
	supersededSequences []uint64
}

func (p *RoomTimelineProjection) appendEntryLocked(seq uint64, event *evtv1.Event) int {
	idx := len(p.entries)
	entry := timelineRow{
		streamSeq: seq,
		eventID:   event.GetId(),
		room:      p.internRoomLocked(roomIDOfEvent(event)),
		actor:     p.internUserLocked(event.GetActorId()),
		createdAt: eventCreatedAt(event),
		kind:      timelineKind(evtstream.EventTypeOf(event)),
	}
	if posted := event.GetMessagePosted(); posted != nil {
		entry.bodyIndex = uint32(len(p.bodyStates) + 1)
		entry.author = p.internUserLocked(posted.GetAuthorId())
		entry.historicalImport = posted.GetHistoricalImport()
		inThreadID := posted.GetInThread()
		rootID := inThreadID
		if rootID == "" {
			rootID = posted.GetEchoFromThreadRootEventId()
		}
		if rootID == "" {
			rootID = entry.eventID
		}
		entry.threadRoot = p.timelineRefLocked(idx, entry.eventID, rootID, 0)
		entry.inThread = p.timelineRefLocked(idx, entry.eventID, inThreadID, 1)
		entry.echoOf = p.timelineRefLocked(idx, entry.eventID, posted.GetEchoOfEventId(), 2)
	}
	p.entries = append(p.entries, entry)
	if entry.bodyIndex != 0 {
		p.bodyStates = append(p.bodyStates, p.orphanBodyStates[entry.eventID])
		delete(p.orphanBodyStates, entry.eventID)
	}
	return idx
}

func (p *RoomTimelineProjection) internRoomLocked(id string) uint32 {
	if id == "" {
		return 0
	}
	if index := p.roomIDs[id]; index != 0 {
		return index
	}
	index := uint32(len(p.rooms))
	p.rooms = append(p.rooms, id)
	p.roomIDs[id] = index
	return index
}

func (p *RoomTimelineProjection) internUserLocked(id string) uint32 {
	if id == "" {
		return 0
	}
	if index := p.userIDs[id]; index != 0 {
		return index
	}
	index := uint32(len(p.users))
	p.users = append(p.users, id)
	p.userIDs[id] = index
	return index
}

func (p *RoomTimelineProjection) appendRestoredEntryLocked(entry TimelineEntry) int {
	idx := len(p.entries)
	row := timelineRow{
		streamSeq: entry.StreamSeq, eventID: entry.EventID, createdAt: entry.CreatedAt,
		room:  p.internRoomLocked(entry.RoomID),
		actor: p.internUserLocked(entry.ActorID), author: p.internUserLocked(entry.MessageAuthorID),
		kind: timelineKind(entry.EventType), historicalImport: entry.HistoricalImport,
	}
	row.threadRoot = p.timelineRefLocked(idx, entry.EventID, entry.ThreadRootEventID, 0)
	row.inThread = p.timelineRefLocked(idx, entry.EventID, entry.InThreadEventID, 1)
	row.echoOf = p.timelineRefLocked(idx, entry.EventID, entry.EchoOfEventID, 2)
	if row.kind == timelineMessagePosted {
		row.bodyIndex = uint32(len(p.bodyStates) + 1)
		p.bodyStates = append(p.bodyStates, timelineBodyState{})
	}
	p.entries = append(p.entries, row)
	return idx
}

// timelineRefLocked uses an existing row index when the referenced message is
// already present. A later or external target keeps its exact ID in the sparse
// fallback so replay and old snapshots retain their original read values.
func (p *RoomTimelineProjection) timelineRefLocked(row int, eventID, targetID string, field int) uint32 {
	if targetID == "" {
		return 0
	}
	if targetID == eventID {
		return uint32(row + 1)
	}
	if target, ok := p.byEventID[targetID]; ok {
		return uint32(target + 1)
	}
	if p.unresolvedRefs == nil {
		p.unresolvedRefs = make(map[int]timelineUnresolvedRefs)
	}
	refs := p.unresolvedRefs[row]
	switch field {
	case 0:
		refs.threadRootEventID = targetID
	case 1:
		refs.inThreadEventID = targetID
	case 2:
		refs.echoOfEventID = targetID
	}
	p.unresolvedRefs[row] = refs
	return 0
}

func (p *RoomTimelineProjection) timelineRefIDLocked(row int, index uint32, field int) string {
	if index != 0 {
		return p.entries[index-1].eventID
	}
	refs := p.unresolvedRefs[row]
	switch field {
	case 0:
		return refs.threadRootEventID
	case 1:
		return refs.inThreadEventID
	case 2:
		return refs.echoOfEventID
	}
	return ""
}

// bodyStateLocked resolves a message's body through its existing event index.
// The fallback retains body facts that precede their MessagePosted fact.
func (p *RoomTimelineProjection) bodyStateLocked(eventID string) (timelineBodyState, bool) {
	if idx, ok := p.byEventID[eventID]; ok {
		if bodyIndex := p.entries[idx].bodyIndex; bodyIndex != 0 {
			state := p.bodyStates[bodyIndex-1]
			return state, state.currentSequence != 0
		}
	}
	state, ok := p.orphanBodyStates[eventID]
	return state, ok
}

func (p *RoomTimelineProjection) putBodyStateLocked(eventID string, state timelineBodyState) {
	if idx, ok := p.byEventID[eventID]; ok {
		if bodyIndex := p.entries[idx].bodyIndex; bodyIndex != 0 {
			p.bodyStates[bodyIndex-1] = state
			return
		}
	}
	p.orphanBodyStates[eventID] = state
}

// entryAtLocked reconstructs a detached read value; callers may return it
// after releasing the projection lock without another copy.
func (p *RoomTimelineProjection) entryAtLocked(idx int) *TimelineEntry {
	if idx < 0 || idx >= len(p.entries) {
		return nil
	}
	row := &p.entries[idx]
	return &TimelineEntry{
		StreamSeq: row.streamSeq, EventID: row.eventID, RoomID: p.rooms[row.room],
		ActorID: p.users[row.actor], MessageAuthorID: p.users[row.author],
		CreatedAt: row.createdAt, EventType: row.kind.eventType(),
		ThreadRootEventID: p.timelineRefIDLocked(idx, row.threadRoot, 0),
		InThreadEventID:   p.timelineRefIDLocked(idx, row.inThread, 1),
		EchoOfEventID:     p.timelineRefIDLocked(idx, row.echoOf, 2),
		HistoricalImport:  row.historicalImport,
	}
}

func (p *RoomTimelineProjection) entryByEventIDLocked(eventID string) (*TimelineEntry, bool) {
	idx, ok := p.byEventID[eventID]
	if !ok {
		return nil, false
	}
	entry := p.entryAtLocked(idx)
	if entry == nil {
		return nil, false
	}
	return entry, true
}

// NewRoomTimelineProjection returns an empty projection.
func NewRoomTimelineProjection() *RoomTimelineProjection {
	return &RoomTimelineProjection{
		roomIDs:                    make(map[string]uint32),
		rooms:                      []string{""},
		userIDs:                    make(map[string]uint32),
		users:                      []string{""},
		byRoom:                     make(map[string][]int),
		byEventID:                  make(map[string]int),
		messagePostsByRoom:         make(map[string][]int),
		latestOriginalPostAt:       make(map[roomActorKey]time.Time),
		replayGuard:                newProjectionReplayGuard(),
		orphanBodyStates:           make(map[string]timelineBodyState),
		retractedFlags:             make(map[string]struct{}),
		tombstonedAt:               make(map[string]time.Time),
		shreddedAt:                 make(map[string]time.Time),
		attachmentMessageIDsByRoom: make(map[string][]string),
		attachmentMessageRoom:      make(map[string]string),
		echoLinks:                  make(map[string][]string),
		hiddenEchoes:               make(map[string]struct{}),
		shreddedUsers:              make(map[string]struct{}),
		pinnedMessagesByRoom:       make(map[string]map[string]PinnedMessageState),
		latestPinByRoom:            make(map[string]latestRoomPinState),
	}
}

// Subjects implements evtstream.Projection. The projection owns the
// "everything that happened in this room" surface, so it subscribes to the
// room aggregate namespace plus the extra user key-shred events it needs.
func (p *RoomTimelineProjection) Subjects() []string {
	return []string{
		evtstream.RoomSubjectFilter(),
		evtstream.UserEventTypeFilter(evtstream.EventUserKeyShreddingRequested),
		evtstream.UserEventTypeFilter(evtstream.EventUserKeyShredded),
	}
}

// ReplaySubjects uses one stream-wide physical filter because JetStream's
// multi-filter scan is expensive when it combines the broad room wildcard with
// the sparse user-key-shredded family. The Projector rejects unrelated subjects
// before decoding or applying them.
func (p *RoomTimelineProjection) ReplaySubjects() []string {
	return []string{evtstream.EventSubjectFilter()}
}

// Apply implements evtstream.Projection. Extracts the room_id from whichever
// room-scoped event variant we recognise and appends visible entries to that
// room's slice. Events that don't carry a room_id (shouldn't appear on
// evt.room.>, but defensive) are silently skipped — projections forward-compat
// by ignoring what they don't understand.
func (p *RoomTimelineProjection) Apply(event *evtv1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	if requested := event.GetUserKeyShreddingRequested(); requested != nil {
		p.applyUserKeyShreddedLocked(requested.GetUserId(), eventCreatedAt(event))
		return nil
	}
	if shredded := event.GetUserKeyShredded(); shredded != nil {
		p.applyUserKeyShreddedLocked(shredded.GetUserId(), eventCreatedAt(event))
		return nil
	}

	roomID := roomIDOfEvent(event)
	if roomID == "" {
		return nil
	}
	if !eventMutatesRoomTimelineProjection(event) {
		return nil
	}

	// Idempotency is envelope-ID based during startup replay. A clean history
	// switches to the monotonic stream-sequence guard once replay completes.
	if p.replayGuard.seenOrMark(event, seq) {
		return nil
	}

	if ev := event.GetMessageBody(); ev != nil {
		targetID := ev.GetEventId()
		body := ev.GetBody()
		if targetID != "" && body != nil {
			if body.GetBodyEventId() != "" && body.GetBodyEventId() != event.GetId() {
				return nil
			}
			if authorID := body.GetAuthorId(); authorID != "" {
				if _, shredded := p.shreddedUsers[authorID]; shredded {
					p.clearCurrentBodyLocked(targetID)
					p.retractedFlags[targetID] = struct{}{}
					p.setTombstonedAtLocked(targetID, p.shreddedAt[authorID])
					p.removeAttachmentMessageLocked(targetID)
				} else {
					bodyEventID := body.GetBodyEventId()
					if bodyEventID == "" {
						bodyEventID = event.GetId()
					}
					p.setCurrentBodyLocked(targetID, bodyEventID, authorID, messageBodyAttachmentCount(body), seq)
					// Retractions are monotonic. Mixed-version replicas or historical
					// replay can present a late body after the tombstone. Retain its
					// sequence for secure deletion without making it active again.
					if _, retracted := p.retractedFlags[targetID]; retracted {
						p.clearCurrentBodyLocked(targetID)
						p.removeAttachmentMessageLocked(targetID)
					} else {
						p.refreshAttachmentMessageLocked(roomID, targetID)
					}
				}
			}
		}
		for _, echoID := range p.echoLinks[targetID] {
			p.refreshAttachmentMessageLocked(roomID, echoID)
		}
		return nil
	}

	entryIdx := -1
	if shouldIndexRoomTimelineEvent(event) {
		entryIdx = p.appendEntryLocked(seq, event)
		if eid := event.GetId(); eid != "" {
			p.byEventID[eid] = entryIdx
		}
	}
	if event.GetMessagePosted() != nil {
		if entryIdx < 0 {
			entryIdx = p.appendEntryLocked(seq, event)
		}
		p.messagePostsByRoom[roomID] = append(p.messagePostsByRoom[roomID], entryIdx)
		if event.GetMessagePosted().GetEchoOfEventId() == "" && !event.GetMessagePosted().GetHistoricalImport() && event.GetActorId() != "" {
			p.latestOriginalPostAt[roomActorKey{roomID: roomID, actorID: event.GetActorId()}] = eventCreatedAt(event)
		}
	}
	if isVisibleRoomTimelineEntry(event) {
		if entryIdx < 0 {
			entryIdx = p.appendEntryLocked(seq, event)
		}
		p.byRoom[roomID] = append(p.byRoom[roomID], entryIdx)
	}

	// Maintain the body-reference and retracted-flag indexes so current body
	// selection is O(1) instead of an O(room) walk per lookup.
	switch ev := event.GetEvent().(type) {
	case *evtv1.Event_MessagePosted:
		targetID := event.GetId()
		if targetID != "" {
			authorID := messageAuthorID(event)
			if _, shredded := p.shreddedUsers[authorID]; shredded {
				p.clearCurrentBodyLocked(targetID)
				p.retractedFlags[targetID] = struct{}{}
				p.setTombstonedAtLocked(targetID, p.shreddedAt[authorID])
				p.removeAttachmentMessageLocked(targetID)
			}
		}
		if state, ok := p.bodyStateLocked(targetID); ok && state.active {
			p.refreshAttachmentMessageLocked(roomID, targetID)
		}
		// Track timeline placements so content and attachment reads can
		// resolve the original without separate echo body state.
		if origID := ev.MessagePosted.GetEchoOfEventId(); origID != "" && targetID != "" {
			p.echoLinks[origID] = append(p.echoLinks[origID], targetID)
			p.refreshAttachmentMessageLocked(roomID, targetID)
		}
	case *evtv1.Event_MessageRetracted:
		targetID := ev.MessageRetracted.GetEventId()
		if targetID != "" {
			p.setTombstonedAtLocked(targetID, eventCreatedAt(event))
			if origID := p.echoOriginalIDLocked(targetID); origID != "" {
				if _, originalRetracted := p.retractedFlags[origID]; !originalRetracted {
					p.clearCurrentBodyLocked(targetID)
					p.hiddenEchoes[targetID] = struct{}{}
					p.removeAttachmentMessageLocked(targetID)
					return nil
				}
			}
			p.clearCurrentBodyLocked(targetID)
			p.retractedFlags[targetID] = struct{}{}
			p.removeAttachmentMessageLocked(targetID)
			if pins := p.pinnedMessagesByRoom[roomID]; pins != nil {
				delete(pins, targetID)
			}
		}
	case *evtv1.Event_MessagePinned:
		messageID := ev.MessagePinned.GetMessageEventId()
		if messageID != "" {
			if latest := p.latestPinByRoom[roomID]; seq > latest.PinSequence {
				p.latestPinByRoom[roomID] = latestRoomPinState{PinEventID: event.GetId(), PinSequence: seq}
			}
			pins := p.pinnedMessagesByRoom[roomID]
			if pins == nil {
				pins = make(map[string]PinnedMessageState)
				p.pinnedMessagesByRoom[roomID] = pins
			}
			pins[messageID] = PinnedMessageState{PinEventID: event.GetId(), PinSequence: seq, RoomID: roomID, MessageEventID: messageID}
		}
	case *evtv1.Event_MessageUnpinned:
		if pins := p.pinnedMessagesByRoom[roomID]; pins != nil {
			delete(pins, ev.MessageUnpinned.GetMessageEventId())
		}
	}
	return nil
}

// LatestPinEventID returns the opaque identity of the latest durable pin fact
// for a room. Unpinning does not move this marker backwards.
func (p *RoomTimelineProjection) LatestPinEventID(roomID string) string {
	p.RLock()
	defer p.RUnlock()
	return p.latestPinByRoom[roomID].PinEventID
}

func (p *RoomTimelineProjection) CompleteStartupReplay() {
	p.Lock()
	defer p.Unlock()
	p.replayGuard.completeReplay()
}

func eventMutatesRoomTimelineProjection(event *evtv1.Event) bool {
	if event == nil {
		return false
	}
	if event.GetMessageBody() != nil || event.GetMessageRetracted() != nil || event.GetMessagePinned() != nil || event.GetMessageUnpinned() != nil {
		return true
	}
	return shouldIndexRoomTimelineEvent(event) || isVisibleRoomTimelineEntry(event)
}

// PinnedMessages returns one room's active pins in newest-pin-first order.
func (p *RoomTimelineProjection) PinnedMessages(roomID string) []PinnedMessageState {
	pins, _ := p.PinnedMessagesWithLatest(roomID)
	return pins
}

// PinnedMessagesWithLatest returns active pins and the opaque latest-pin marker
// from one projection read boundary.
func (p *RoomTimelineProjection) PinnedMessagesWithLatest(roomID string) ([]PinnedMessageState, string) {
	p.RLock()
	defer p.RUnlock()
	pins := p.pinnedMessagesByRoom[roomID]
	out := make([]PinnedMessageState, 0, len(pins))
	for messageID, pin := range pins {
		if _, retracted := p.retractedFlags[messageID]; retracted {
			continue
		}
		out = append(out, pin)
	}
	slices.SortFunc(out, func(left, right PinnedMessageState) int {
		if right.PinSequence < left.PinSequence {
			return -1
		}
		if right.PinSequence > left.PinSequence {
			return 1
		}
		return strings.Compare(right.PinEventID, left.PinEventID)
	})
	return out, p.latestPinByRoom[roomID].PinEventID
}

// PinnedMessage returns one active pin association.
func (p *RoomTimelineProjection) PinnedMessage(roomID, messageEventID string) (PinnedMessageState, bool) {
	p.RLock()
	defer p.RUnlock()
	pin, ok := p.pinnedMessagesByRoom[roomID][messageEventID]
	if _, retracted := p.retractedFlags[messageEventID]; retracted {
		return PinnedMessageState{}, false
	}
	return pin, ok
}

func (p *RoomTimelineProjection) applyUserKeyShreddedLocked(userID string, at time.Time) {
	if userID == "" {
		return
	}
	p.shreddedUsers[userID] = struct{}{}
	if !at.IsZero() {
		if existing, ok := p.shreddedAt[userID]; !ok || at.Before(existing) {
			p.shreddedAt[userID] = at
		}
		at = p.shreddedAt[userID]
	}
	for eventID, idx := range p.byEventID {
		entry := p.entryAtLocked(idx)
		if entry == nil || !entry.IsMessagePost() {
			continue
		}
		if timelineEntryMessageAuthorID(entry) != userID {
			continue
		}
		p.clearCurrentBodyLocked(eventID)
		p.retractedFlags[eventID] = struct{}{}
		p.setTombstonedAtLocked(eventID, at)
		p.removeAttachmentMessageLocked(eventID)
	}
}

func (p *RoomTimelineProjection) setCurrentBodyLocked(eventID, bodyEventID, authorID string, attachmentCount int, sequence uint64) {
	state, exists := p.bodyStateLocked(eventID)
	if exists {
		state.supersededSequences = append(state.supersededSequences, state.currentSequence)
	}
	state.currentSequence = sequence
	state.currentEventID = bodyEventID
	state.authorID = authorID
	state.attachmentCount = attachmentCount
	state.active = true
	p.putBodyStateLocked(eventID, state)
}

func (p *RoomTimelineProjection) clearCurrentBodyLocked(eventID string) {
	state, exists := p.bodyStateLocked(eventID)
	if !exists {
		return
	}
	state.active = false
	state.attachmentCount = 0
	p.putBodyStateLocked(eventID, state)
}

func (p *RoomTimelineProjection) setTombstonedAtLocked(eventID string, at time.Time) {
	if eventID == "" || at.IsZero() {
		return
	}
	if existing, ok := p.tombstonedAt[eventID]; !ok || at.Before(existing) {
		p.tombstonedAt[eventID] = at
	}
}

// RoomEvents returns up to `limit` entries from a room's timeline in
// newest-first order, optionally bounded by an exclusive
// stream-sequence cursor (beforeStreamSeq == 0 means "from the
// newest"). It returns detached compact references that callers can inspect
// without holding the projection lock.
//
// Entries are the room-visible timeline; folded state such as edits, reactions,
// thread replies, asset processing, and directly hidden echoes is excluded.
func (p *RoomTimelineProjection) RoomEvents(roomID string, limit int, beforeStreamSeq uint64) []*TimelineEntry {
	if limit <= 0 {
		return nil
	}
	p.RLock()
	defer p.RUnlock()
	entryIndexes := p.byRoom[roomID]
	if len(entryIndexes) == 0 {
		return nil
	}
	out := make([]*TimelineEntry, 0, limit)
	for i := len(entryIndexes) - 1; i >= 0 && len(out) < limit; i-- {
		e := p.entryAtLocked(entryIndexes[i])
		if e == nil {
			continue
		}
		if beforeStreamSeq > 0 && e.StreamSeq >= beforeStreamSeq {
			continue
		}
		out = append(out, e)
	}
	return out
}

// RoomEventCount returns the total number of non-hidden visible timeline
// entries in the room.
func (p *RoomTimelineProjection) RoomEventCount(roomID string) int {
	return p.VisibleRoomEventCount(roomID)
}

// VisibleRoomEventCount returns the total number of room-visible timeline
// entries in the room. Hidden echoes may still be present in the room slice and
// are excluded by the visible timeline readers.
func (p *RoomTimelineProjection) VisibleRoomEventCount(roomID string) int {
	p.RLock()
	defer p.RUnlock()
	n := 0
	for _, idx := range p.byRoom[roomID] {
		entry := p.entryAtLocked(idx)
		if p.isHiddenEchoEntryLocked(entry) {
			continue
		}
		n++
	}
	return n
}

// Stats returns aggregate counts useful for import/rollout diagnostics.
func (p *RoomTimelineProjection) Stats() (rooms int, entries int, messagePosts int) {
	p.RLock()
	defer p.RUnlock()
	rooms = len(p.byRoom)
	for _, roomEntries := range p.byRoom {
		entries += len(roomEntries)
	}
	for _, roomEntries := range p.messagePostsByRoom {
		messagePosts += len(roomEntries)
	}
	return rooms, entries, messagePosts
}

func shouldIndexRoomTimelineEvent(event *evtv1.Event) bool {
	if event == nil {
		return false
	}
	switch event.GetEvent().(type) {
	case *evtv1.Event_MessagePosted:
		return true
	default:
		return isVisibleRoomTimelineEntry(event)
	}
}

func isIndexedRoomTimelineEventType(eventType string) bool {
	switch eventType {
	case evtstream.EventMessagePosted,
		evtstream.EventRoomCreated,
		evtstream.EventRoomUpdated,
		evtstream.EventRoomDeleted,
		evtstream.EventRoomArchived,
		evtstream.EventRoomUnarchived,
		evtstream.EventRoomThreadingModeChanged,
		evtstream.EventUserJoinedRoom,
		evtstream.EventUserLeftRoom,
		evtstream.EventCallStarted,
		evtstream.EventCallEnded:
		return true
	default:
		return false
	}
}

// Get returns a single timeline entry by its envelope id, or
// (nil, false) if no such event has been projected.
func (p *RoomTimelineProjection) Get(eventID string) (*TimelineEntry, bool) {
	p.RLock()
	defer p.RUnlock()
	entry, ok := p.entryByEventIDLocked(eventID)
	return entry, ok
}

// LastRoomMessageEntry returns the newest non-hidden ordinary post in a room,
// including thread replies that are intentionally absent from byRoom.
// Historical imports do not count as new room activity.
func (p *RoomTimelineProjection) LastRoomMessageEntry(roomID string) (*TimelineEntry, bool) {
	p.RLock()
	defer p.RUnlock()
	entryIndexes := p.messagePostsByRoom[roomID]
	for i := len(entryIndexes) - 1; i >= 0; i-- {
		e := p.entryAtLocked(entryIndexes[i])
		if e == nil {
			continue
		}
		if p.isHiddenEchoEntryLocked(e) {
			continue
		}
		if e.HistoricalImport {
			continue
		}
		return e, true
	}
	return nil, false
}

// LatestOriginalPostAt returns the latest committed non-echo message time for
// one actor in one room. The timestamp remains authoritative after edits or
// retractions so those actions cannot evade slow mode.
func (p *RoomTimelineProjection) LatestOriginalPostAt(roomID, actorID string) (time.Time, bool) {
	p.RLock()
	defer p.RUnlock()
	value, ok := p.latestOriginalPostAt[roomActorKey{roomID: roomID, actorID: actorID}]
	return value, ok && !value.IsZero()
}

// LatestBodyReference returns the current MessageBodyEvent reference for a
// message, or a zero reference plus retracted=true after retraction.
//
// Echo references identify the original content owner, not the echo's
// physical body history. Unknown messages return a zero reference and ok=false.
// Invalid links return a zero reference without reporting a deletion.
//
// O(1): it consults indexes that Apply keeps in lockstep with byRoom.
func (p *RoomTimelineProjection) LatestBodyReference(eventID string) (reference TimelineBodyReference, retracted bool, ok bool) {
	p.RLock()
	defer p.RUnlock()
	return p.latestBodyReferenceLocked(eventID)
}

// ContentEventID resolves a visible message to the owner of its content.
// Echo links must point directly to a thread reply in the same room.
func (p *RoomTimelineProjection) ContentEventID(eventID string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	entry := p.contentEntryLocked(eventID)
	if entry == nil {
		return "", false
	}
	return entry.EventID, true
}

func (p *RoomTimelineProjection) contentEntryLocked(eventID string) *TimelineEntry {
	entry, _ := p.entryByEventIDLocked(eventID)
	if entry == nil || !entry.IsMessagePost() {
		return nil
	}
	if _, hidden := p.hiddenEchoes[eventID]; hidden {
		return nil
	}
	if entry.EchoOfEventID == "" {
		return entry
	}
	original, _ := p.entryByEventIDLocked(entry.EchoOfEventID)
	if original == nil || !original.IsMessagePost() || original.EchoOfEventID != "" ||
		original.InThreadEventID == "" || original.RoomID != entry.RoomID ||
		original.InThreadEventID != entry.ThreadRootEventID {
		return nil
	}
	return original
}

func (p *RoomTimelineProjection) latestBodyReferenceLocked(eventID string) (TimelineBodyReference, bool, bool) {
	visible, _ := p.entryByEventIDLocked(eventID)
	if visible == nil || !visible.IsMessagePost() {
		return TimelineBodyReference{}, false, false
	}
	if _, hidden := p.hiddenEchoes[eventID]; hidden {
		return TimelineBodyReference{}, true, true
	}
	if _, retracted := p.retractedFlags[eventID]; retracted {
		return TimelineBodyReference{}, true, true
	}
	entry := p.contentEntryLocked(eventID)
	if entry == nil {
		return TimelineBodyReference{}, false, true
	}
	if _, retracted := p.retractedFlags[entry.EventID]; retracted {
		return TimelineBodyReference{}, true, true
	}
	if state, has := p.bodyStateLocked(entry.EventID); has && state.active {
		return TimelineBodyReference{
			MessageEventID: entry.EventID, BodyEventID: state.currentEventID, RoomID: entry.RoomID,
			AuthorID: state.authorID, StreamSeq: state.currentSequence, AttachmentCount: state.attachmentCount,
		}, false, true
	}
	return TimelineBodyReference{}, false, true
}

// BodyReferenceCurrent reports whether a detached active body reference still
// represents the current visible message body.
func (p *RoomTimelineProjection) BodyReferenceCurrent(reference TimelineBodyReference) bool {
	current, retracted, ok := p.LatestBodyReference(reference.MessageEventID)
	return ok && !retracted && current == reference
}

// CurrentRoomAttachmentMessages returns current, visible messages whose latest
// body references attachments. Results are newest message first.
func (p *RoomTimelineProjection) CurrentRoomAttachmentMessages(roomID string) []projectedRoomAttachmentMessage {
	p.RLock()
	defer p.RUnlock()

	ids := p.attachmentMessageIDsByRoom[roomID]
	if len(ids) == 0 {
		return nil
	}

	out := make([]projectedRoomAttachmentMessage, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		eventID := ids[i]
		entry, _ := p.entryByEventIDLocked(eventID)
		if entry == nil || p.isHiddenEchoEntryLocked(entry) {
			continue
		}
		if _, retracted := p.retractedFlags[eventID]; retracted {
			continue
		}
		if origID := p.echoOriginalIDLocked(eventID); origID != "" {
			if _, originalRetracted := p.retractedFlags[origID]; originalRetracted {
				continue
			}
		}
		reference, retracted, _ := p.latestBodyReferenceLocked(eventID)
		if retracted || reference.StreamSeq == 0 || reference.AttachmentCount == 0 {
			continue
		}

		out = append(out, projectedRoomAttachmentMessage{
			Entry:              entry,
			BodyMessageEventID: reference.MessageEventID,
			BodySequence:       reference.StreamSeq,
			BodyEventID:        reference.BodyEventID,
			BodyAuthorID:       reference.AuthorID,
			AttachmentCount:    reference.AttachmentCount,
		})
	}
	return out
}

func (p *RoomTimelineProjection) refreshAttachmentMessageLocked(roomID, eventID string) {
	if roomID == "" || eventID == "" {
		return
	}
	reference, retracted, _ := p.latestBodyReferenceLocked(eventID)
	if retracted || reference.StreamSeq == 0 || reference.AttachmentCount == 0 {
		p.removeAttachmentMessageLocked(eventID)
		return
	}
	entry, _ := p.entryByEventIDLocked(eventID)
	if entry == nil {
		return
	}
	p.addAttachmentMessageLocked(roomID, eventID, entry.StreamSeq)
}

func (p *RoomTimelineProjection) addAttachmentMessageLocked(roomID, eventID string, streamSeq uint64) {
	if roomID == "" || eventID == "" {
		return
	}
	if existingRoom := p.attachmentMessageRoom[eventID]; existingRoom != "" {
		if existingRoom == roomID {
			return
		}
		p.removeAttachmentMessageLocked(eventID)
	}

	ids := p.attachmentMessageIDsByRoom[roomID]
	insertAt := len(ids)
	if len(ids) > 0 {
		last, _ := p.entryByEventIDLocked(ids[len(ids)-1])
		if last != nil && last.StreamSeq <= streamSeq {
			ids = append(ids, eventID)
			p.attachmentMessageIDsByRoom[roomID] = ids
			p.attachmentMessageRoom[eventID] = roomID
			return
		}
		for i, existingID := range ids {
			existing, _ := p.entryByEventIDLocked(existingID)
			if existing == nil || existing.StreamSeq > streamSeq {
				insertAt = i
				break
			}
		}
	}
	ids = append(ids, "")
	copy(ids[insertAt+1:], ids[insertAt:])
	ids[insertAt] = eventID
	p.attachmentMessageIDsByRoom[roomID] = ids
	p.attachmentMessageRoom[eventID] = roomID
}

func (p *RoomTimelineProjection) removeAttachmentMessageLocked(eventID string) {
	roomID := p.attachmentMessageRoom[eventID]
	if roomID == "" {
		return
	}
	ids := p.attachmentMessageIDsByRoom[roomID]
	for i, existingID := range ids {
		if existingID != eventID {
			continue
		}
		ids = append(ids[:i], ids[i+1:]...)
		break
	}
	if len(ids) == 0 {
		delete(p.attachmentMessageIDsByRoom, roomID)
	} else {
		p.attachmentMessageIDsByRoom[roomID] = ids
	}
	delete(p.attachmentMessageRoom, eventID)
}

// BodyEventSeqs returns all projected MessageBodyEvent stream sequences for
// a message and identifies the most recently observed body sequence. A
// retracted or hidden message retains this history for secure deletion.
func (p *RoomTimelineProjection) BodyEventSeqs(eventID string) (seqs []uint64, current uint64, ok bool) {
	p.RLock()
	defer p.RUnlock()
	if eventID == "" {
		return nil, 0, false
	}
	if _, exists := p.byEventID[eventID]; !exists {
		return nil, 0, false
	}
	state, hasBodyState := p.bodyStateLocked(eventID)
	if !hasBodyState {
		return nil, 0, true
	}
	seqs = make([]uint64, 0, len(state.supersededSequences)+1)
	seqs = append(seqs, state.supersededSequences...)
	seqs = append(seqs, state.currentSequence)
	return seqs, state.currentSequence, true
}

// ObsoleteBodyEventSeqs returns body event sequences that can be securely
// deleted without losing the current body. For retracted messages, every body
// event is obsolete. For active messages, every non-current body event is
// obsolete.
func (p *RoomTimelineProjection) ObsoleteBodyEventSeqs(eventID string) []uint64 {
	p.RLock()
	defer p.RUnlock()
	if eventID == "" {
		return nil
	}
	state, ok := p.bodyStateLocked(eventID)
	if !ok {
		return nil
	}
	if _, retracted := p.retractedFlags[eventID]; retracted {
		return appendBodySequences(nil, state)
	}
	if _, hidden := p.hiddenEchoes[eventID]; hidden {
		return appendBodySequences(nil, state)
	}
	return append([]uint64(nil), state.supersededSequences...)
}

// AllObsoleteBodyEventSeqs returns every projected MessageBodyEvent seq
// whose payload is no longer needed for the current message state.
func (p *RoomTimelineProjection) AllObsoleteBodyEventSeqs() []uint64 {
	p.RLock()
	defer p.RUnlock()
	var out []uint64
	for _, row := range p.entries {
		if row.bodyIndex == 0 {
			continue
		}
		state := p.bodyStates[row.bodyIndex-1]
		if state.currentSequence == 0 {
			continue
		}
		eventID := row.eventID
		if _, retracted := p.retractedFlags[eventID]; retracted {
			out = appendBodySequences(out, state)
			continue
		}
		if _, hidden := p.hiddenEchoes[eventID]; hidden {
			out = appendBodySequences(out, state)
			continue
		}
		out = append(out, state.supersededSequences...)
	}
	for eventID, state := range p.orphanBodyStates {
		if _, retracted := p.retractedFlags[eventID]; retracted {
			out = appendBodySequences(out, state)
			continue
		}
		if _, hidden := p.hiddenEchoes[eventID]; hidden {
			out = appendBodySequences(out, state)
			continue
		}
		out = append(out, state.supersededSequences...)
	}
	return out
}

func appendBodySequences(dst []uint64, state timelineBodyState) []uint64 {
	dst = append(dst, state.supersededSequences...)
	return append(dst, state.currentSequence)
}

func (p *RoomTimelineProjection) echoOriginalIDLocked(eventID string) string {
	entry, ok := p.entryByEventIDLocked(eventID)
	if !ok || entry == nil {
		return ""
	}
	return entry.EchoOfEventID
}

// IsEcho reports whether eventID is a MessagePostedEvent echo.
func (p *RoomTimelineProjection) IsEcho(eventID string) bool {
	p.RLock()
	defer p.RUnlock()
	return p.echoOriginalIDLocked(eventID) != ""
}

// IsHiddenEcho reports whether an echo has been directly retracted from the
// room timeline.
func (p *RoomTimelineProjection) IsHiddenEcho(eventID string) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.hiddenEchoes[eventID]
	return ok
}

// ChannelEchoEventID returns the first visible echo event for an original
// thread reply, if one exists. Hidden/retracted echoes are ignored.
func (p *RoomTimelineProjection) ChannelEchoEventID(originalEventID string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	return p.channelEchoEventIDLocked(originalEventID)
}

func (p *RoomTimelineProjection) channelEchoEventIDLocked(originalEventID string) (string, bool) {
	if originalEventID == "" {
		return "", false
	}
	for _, echoID := range p.echoLinks[originalEventID] {
		if echoID == "" {
			continue
		}
		if _, hidden := p.hiddenEchoes[echoID]; hidden {
			continue
		}
		if _, retracted := p.retractedFlags[echoID]; retracted {
			continue
		}
		if _, ok := p.entryByEventIDLocked(echoID); !ok {
			continue
		}
		if origID := p.echoOriginalIDLocked(echoID); origID != originalEventID {
			continue
		}
		return echoID, true
	}
	return "", false
}

// MessageHydrationState returns the timeline metadata needed to render one
// message. The detached result is captured under one projection read lock so
// deletion, channel-echo, and pin metadata come from the same projection
// moment. Echoes inherit the pin state of their canonical thread reply.
func (p *RoomTimelineProjection) MessageHydrationState(eventID string) RoomTimelineMessageHydrationState {
	p.RLock()
	defer p.RUnlock()

	deletedAt, hasDeletedAt := p.messageTombstonedAtLocked(eventID)
	channelEchoEventID, _ := p.channelEchoEventIDLocked(eventID)
	canonicalEventID := eventID
	if originalEventID := p.echoOriginalIDLocked(eventID); originalEventID != "" {
		canonicalEventID = originalEventID
	}
	roomID := ""
	if entry, ok := p.entryByEventIDLocked(eventID); ok && entry != nil {
		roomID = entry.RoomID
	}
	_, pinned := p.pinnedMessagesByRoom[roomID][canonicalEventID]
	return RoomTimelineMessageHydrationState{
		DeletedAt:          deletedAt,
		HasDeletedAt:       hasDeletedAt,
		ChannelEchoEventID: channelEchoEventID,
		Pinned:             pinned,
	}
}

// LinkedChannelEchoEventID returns the first non-hidden echo linked to an
// original reply, including a retracted echo that must render as a tombstone.
func (p *RoomTimelineProjection) LinkedChannelEchoEventID(originalEventID string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	if originalEventID == "" {
		return "", false
	}
	for _, echoID := range p.echoLinks[originalEventID] {
		if echoID == "" {
			continue
		}
		if _, hidden := p.hiddenEchoes[echoID]; hidden {
			continue
		}
		if _, ok := p.entryByEventIDLocked(echoID); !ok {
			continue
		}
		if origID := p.echoOriginalIDLocked(echoID); origID == originalEventID {
			return echoID, true
		}
	}
	return "", false
}

func (p *RoomTimelineProjection) MessageTombstoned(eventID string) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.retractedFlags[eventID]
	return ok
}

// MessageDeletedAt returns when the message first became unavailable through
// retraction or account key shredding. Echoes inherit the original message's
// timestamp.
func (p *RoomTimelineProjection) MessageDeletedAt(eventID string) (time.Time, bool) {
	p.RLock()
	defer p.RUnlock()
	return p.messageTombstonedAtLocked(eventID)
}

func (p *RoomTimelineProjection) messageTombstonedAtLocked(eventID string) (time.Time, bool) {
	if at, ok := p.tombstonedAt[eventID]; ok {
		return at, true
	}
	if origID := p.echoOriginalIDLocked(eventID); origID != "" {
		at, ok := p.tombstonedAt[origID]
		return at, ok
	}
	return time.Time{}, false
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func removeString(values []string, value string) []string {
	out := values[:0]
	for _, existing := range values {
		if existing != value {
			out = append(out, existing)
		}
	}
	return out
}

// LinkedEventIDs returns related timeline IDs for physical legacy-body cleanup.
// Content edits must resolve the original instead of writing to these IDs.
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
	if entry, ok := p.entryByEventIDLocked(eventID); ok {
		if origID := entry.EchoOfEventID; origID != "" && origID != eventID {
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
	return linked
}

// LastVisibleRoomEntry walks the room's timeline newest-first and
// returns the first entry that passes `visible`. Useful for
// "last root message", "last activity", and similar single-entry
// lookups that don't need to materialise a full slice. Returns
// (nil, false) if no entry matches.
func (p *RoomTimelineProjection) LastVisibleRoomEntry(
	roomID string,
	visible func(*TimelineEntry) bool,
) (*TimelineEntry, bool) {
	p.RLock()
	defer p.RUnlock()
	entryIndexes := p.byRoom[roomID]
	for i := len(entryIndexes) - 1; i >= 0; i-- {
		e := p.entryAtLocked(entryIndexes[i])
		if e == nil {
			continue
		}
		if p.isHiddenEchoEntryLocked(e) {
			continue
		}
		if visible != nil && !visible(e) {
			continue
		}
		return e, true
	}
	return nil, false
}

// VisibleRoomTimeline walks the room's visible timeline newest-first, applying
// `visible` as an optional per-entry filter, and returns up to `limit` matching
// entries. `beforeStreamSeq > 0` excludes entries with stream seq >= that value
// (exclusive upper bound for pagination).
//
// Stops as soon as `limit` visible entries are accumulated — no full-slice
// materialisation. Caller may inspect more than `limit` entries when a custom
// visibility filter rejects some of them.
//
// Returns entries in newest-first order. Caller reverses to
// oldest-first if needed.
func (p *RoomTimelineProjection) VisibleRoomTimeline(
	roomID string,
	limit int,
	beforeStreamSeq uint64,
	visible func(*TimelineEntry) bool,
) []*TimelineEntry {
	if limit <= 0 {
		return nil
	}
	p.RLock()
	defer p.RUnlock()
	entryIndexes := p.byRoom[roomID]
	out := make([]*TimelineEntry, 0, limit)
	for i := len(entryIndexes) - 1; i >= 0 && len(out) < limit; i-- {
		e := p.entryAtLocked(entryIndexes[i])
		if e == nil {
			continue
		}
		if beforeStreamSeq > 0 && e.StreamSeq >= beforeStreamSeq {
			continue
		}
		if p.isHiddenEchoEntryLocked(e) {
			continue
		}
		if visible != nil && !visible(e) {
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
	visible func(*TimelineEntry) bool,
) []*TimelineEntry {
	if limit <= 0 {
		return nil
	}
	p.RLock()
	defer p.RUnlock()
	entryIndexes := p.byRoom[roomID]
	out := make([]*TimelineEntry, 0, limit)
	for _, idx := range entryIndexes {
		e := p.entryAtLocked(idx)
		if e == nil {
			continue
		}
		if e.StreamSeq <= afterStreamSeq {
			continue
		}
		if p.isHiddenEchoEntryLocked(e) {
			continue
		}
		if visible != nil && !visible(e) {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// VisibleRoomTimelineAround returns a room-visible window centered on eventID
// in oldest-first order. It walks the visible room slice, so edits/reactions/
// assets/thread replies are not revisited when serving "jump to message" style
// reads.
func (p *RoomTimelineProjection) VisibleRoomTimelineAround(
	roomID string,
	eventID string,
	limit int,
	visibility ...func(*TimelineEntry) bool,
) (entries []*TimelineEntry, targetIndex int, hasOlder bool, hasNewer bool, ok bool) {
	if limit <= 0 || eventID == "" {
		return nil, 0, false, false, false
	}
	p.RLock()
	defer p.RUnlock()
	var visible func(*TimelineEntry) bool
	if len(visibility) > 0 {
		visible = visibility[0]
	}
	roomEntries := p.byRoom[roomID]
	targetVisibleIndex := -1
	visibleCount := 0
	for _, idx := range roomEntries {
		entry := p.entryAtLocked(idx)
		if p.isHiddenEchoEntryLocked(entry) {
			continue
		}
		if visible != nil && (entry == nil || !visible(entry)) {
			continue
		}
		if entry != nil && entry.EventID == eventID {
			targetVisibleIndex = visibleCount
		}
		visibleCount++
	}
	if targetVisibleIndex == -1 {
		return nil, 0, false, false, false
	}

	start := targetVisibleIndex - (limit-1)/2
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > visibleCount {
		end = visibleCount
		start = end - limit
		if start < 0 {
			start = 0
		}
	}

	out := make([]*TimelineEntry, 0, end-start)
	visibleIndex := 0
	for _, idx := range roomEntries {
		entry := p.entryAtLocked(idx)
		if p.isHiddenEchoEntryLocked(entry) {
			continue
		}
		if visible != nil && (entry == nil || !visible(entry)) {
			continue
		}
		if visibleIndex >= start && visibleIndex < end {
			out = append(out, entry)
		}
		visibleIndex++
		if visibleIndex >= end {
			break
		}
	}
	return out, targetVisibleIndex - start, start > 0, end < visibleCount, true
}

func (p *RoomTimelineProjection) isHiddenEchoEntryLocked(entry *TimelineEntry) bool {
	if entry == nil {
		return false
	}
	_, hidden := p.hiddenEchoes[entry.EventID]
	return hidden
}
