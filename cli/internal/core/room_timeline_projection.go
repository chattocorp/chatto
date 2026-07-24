package core

import (
	"context"
	"io"
	"time"

	"github.com/charmbracelet/log"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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
	entries            []TimelineEntry
	eventIDs           projectionStringTable
	roomIDs            projectionStringTable
	userIDs            projectionStringTable
	entryByEvent       []int32
	byRoom             map[timelineRoomRef][]timelineEntryRef
	messagePostsByRoom map[timelineRoomRef][]timelineEntryRef
	replayGuard        projectionReplayGuard
	// bodyStates keeps compact lifecycle and bucket coordinates in one dense
	// row per retained ID. Decoded current bodies and the uncommon superseded
	// sequence lists live separately so cold, never-edited messages pay for
	// neither pointer-heavy shape.
	bodyStates              []timelineBodyState
	currentBodies           []*corev1.MessageBody
	supersededBodySequences map[timelineEventRef][]uint64
	// tombstonedAt records when message content first became unavailable
	// through a durable retraction or user key-shred fact. It deliberately does
	// not cover missing/corrupt body payloads so clients can distinguish those
	// states from deletions.
	tombstonedAt map[timelineEventRef]time.Time
	shreddedAt   map[timelineUserRef]time.Time
	// attachmentMessagesByRoom tracks messages whose current body contains
	// attachment/asset references. It lets room file reads page over current
	// file-bearing messages instead of decrypting every message body in a room.
	attachmentMessagesByRoom map[timelineRoomRef][]timelineEventRef
	attachmentMessageRoom    []timelineRoomRef
	// echoLinks maps an original message's event_id to the event_ids
	// of any echoes pointing at it. Maintained as MessagePostedEvents
	// with EchoOfEventId arrive. Used by EditMessage / DeleteMessage
	// to fan mutations across linked messages. Each echo has its own
	// projected body payload, so edits and retractions need explicit
	// propagation.
	echoLinks map[timelineEventRef][]timelineEventRef
	// messageFlags packs retracted and directly hidden-echo state by event
	// handle. A direct echo retract removes the room-timeline copy without
	// deleting the original thread reply's content.
	messageFlags  []timelineMessageFlags
	shreddedUsers []bool

	buckets     map[timelineBucketKey]*timelineBucket
	eventLoader roomTimelineEventLoader
	hotWindow   time.Duration
	now         func() time.Time
	retainAll   bool
	logger      *log.Logger
}

type timelineEventRef uint32
type timelineRoomRef uint32
type timelineUserRef uint32
type timelineEntryRef uint32

const missingTimelineEntry = int32(-1)

type timelineMessageFlags uint8

const (
	timelineMessageRetracted timelineMessageFlags = 1 << iota
	timelineMessageHiddenEcho
)

// TimelineEntry is one event's position in a room timeline. Event is present
// while the entry's weekly bucket is resident; the remaining fields form the
// lightweight directory used to locate and materialize a cold bucket.
type TimelineEntry struct {
	StreamSeq uint64
	Event     *corev1.Event

	eventID        timelineEventRef
	roomID         timelineRoomRef
	authorID       timelineUserRef
	echoOriginalID timelineEventRef
	bucket         timelineBucketKey
	messagePosted  bool
}

type projectedRoomAttachmentMessage struct {
	Entry *TimelineEntry
	Body  *corev1.MessageBody
}

type timelineBodyState struct {
	currentSequence uint64
	bucket          timelineBucketKey
	hasAttachments  bool
	known           bool
}

func (p *RoomTimelineProjection) appendEntryLocked(seq uint64, event *corev1.Event) int {
	return p.appendEntryForBucketLocked(seq, event, p.bucketForEventLocked(event))
}

func (p *RoomTimelineProjection) appendEntryForBucketLocked(seq uint64, event *corev1.Event, bucket timelineBucketKey) int {
	idx := len(p.entries)
	entry := TimelineEntry{
		StreamSeq: seq,
		Event:     event,
		bucket:    bucket,
	}
	if event != nil {
		entry.eventID = p.internEventIDLocked(event.GetId())
		entry.roomID = p.internRoomIDLocked(roomIDOfEvent(event))
		entry.authorID = p.internUserIDLocked(messageAuthorID(event))
		entry.messagePosted = event.GetMessagePosted() != nil
		if posted := event.GetMessagePosted(); posted != nil {
			entry.echoOriginalID = p.internEventIDLocked(posted.GetEchoOfEventId())
		}
	}
	p.entries = append(p.entries, entry)
	return idx
}

func (p *RoomTimelineProjection) entryAtLocked(ref timelineEntryRef) *TimelineEntry {
	if int(ref) >= len(p.entries) {
		return nil
	}
	return &p.entries[ref]
}

func (p *RoomTimelineProjection) entryByEventIDLocked(eventID string) (*TimelineEntry, bool) {
	raw, ok := p.eventIDs.lookup(eventID)
	if !ok || int(raw) >= len(p.entryByEvent) {
		return nil, false
	}
	idx := p.entryByEvent[raw]
	if idx == missingTimelineEntry {
		return nil, false
	}
	entry := p.entryAtLocked(timelineEntryRef(idx))
	if entry == nil {
		return nil, false
	}
	return entry, true
}

func (p *RoomTimelineProjection) internEventIDLocked(id string) timelineEventRef {
	ref := timelineEventRef(p.eventIDs.intern(id))
	for len(p.entryByEvent) <= int(ref) {
		p.entryByEvent = append(p.entryByEvent, missingTimelineEntry)
	}
	p.bodyStates = growProjectionSlice(p.bodyStates, uint32(ref))
	p.currentBodies = growProjectionSlice(p.currentBodies, uint32(ref))
	p.messageFlags = growProjectionSlice(p.messageFlags, uint32(ref))
	for i := len(p.attachmentMessageRoom); i <= int(ref); i++ {
		p.attachmentMessageRoom = append(p.attachmentMessageRoom, 0)
	}
	return ref
}

func (p *RoomTimelineProjection) internRoomIDLocked(id string) timelineRoomRef {
	return timelineRoomRef(p.roomIDs.intern(id))
}

func (p *RoomTimelineProjection) internUserIDLocked(id string) timelineUserRef {
	ref := timelineUserRef(p.userIDs.intern(id))
	p.shreddedUsers = growProjectionSlice(p.shreddedUsers, uint32(ref))
	return ref
}

func (p *RoomTimelineProjection) eventIDLocked(ref timelineEventRef) string {
	return p.eventIDs.value(uint32(ref))
}

func (p *RoomTimelineProjection) roomIDLocked(ref timelineRoomRef) string {
	return p.roomIDs.value(uint32(ref))
}

func (p *RoomTimelineProjection) userIDLocked(ref timelineUserRef) string {
	return p.userIDs.value(uint32(ref))
}

func (p *RoomTimelineProjection) eventRefLocked(id string) (timelineEventRef, bool) {
	ref, ok := p.eventIDs.lookup(id)
	return timelineEventRef(ref), ok
}

func (p *RoomTimelineProjection) roomRefLocked(id string) (timelineRoomRef, bool) {
	ref, ok := p.roomIDs.lookup(id)
	return timelineRoomRef(ref), ok
}

func (p *RoomTimelineProjection) bodyStateByEventIDLocked(id string) (timelineBodyState, bool) {
	ref, ok := p.eventRefLocked(id)
	if !ok || int(ref) >= len(p.bodyStates) || !p.bodyStates[ref].known {
		return timelineBodyState{}, false
	}
	return p.bodyStates[ref], true
}

func (p *RoomTimelineProjection) currentBodyByEventIDLocked(id string) *corev1.MessageBody {
	ref, ok := p.eventRefLocked(id)
	if !ok || int(ref) >= len(p.currentBodies) {
		return nil
	}
	return p.currentBodies[ref]
}

func (p *RoomTimelineProjection) supersededBodySequencesByEventIDLocked(id string) []uint64 {
	ref, ok := p.eventRefLocked(id)
	if !ok {
		return nil
	}
	return p.supersededBodySequences[ref]
}

func (p *RoomTimelineProjection) messageFlagLocked(ref timelineEventRef, flag timelineMessageFlags) bool {
	return int(ref) < len(p.messageFlags) && p.messageFlags[ref]&flag != 0
}

func (p *RoomTimelineProjection) setMessageFlagLocked(ref timelineEventRef, flag timelineMessageFlags, enabled bool) {
	p.messageFlags = growProjectionSlice(p.messageFlags, uint32(ref))
	if enabled {
		p.messageFlags[ref] |= flag
	} else {
		p.messageFlags[ref] &^= flag
	}
}

// NewRoomTimelineProjection returns an empty projection.
func NewRoomTimelineProjection() *RoomTimelineProjection {
	return newRoomTimelineProjection(roomTimelineProjectionOptions{retainAll: true})
}

type roomTimelineProjectionOptions struct {
	eventLoader roomTimelineEventLoader
	hotWindow   time.Duration
	now         func() time.Time
	retainAll   bool
	logger      *log.Logger
}

func newRoomTimelineProjection(options roomTimelineProjectionOptions) *RoomTimelineProjection {
	now := options.now
	if now == nil {
		now = time.Now
	}
	logger := options.logger
	if logger == nil {
		logger = log.New(io.Discard)
	}
	return &RoomTimelineProjection{
		eventIDs:                 newProjectionStringTable(),
		roomIDs:                  newProjectionStringTable(),
		userIDs:                  newProjectionStringTable(),
		entryByEvent:             []int32{missingTimelineEntry},
		byRoom:                   make(map[timelineRoomRef][]timelineEntryRef),
		messagePostsByRoom:       make(map[timelineRoomRef][]timelineEntryRef),
		replayGuard:              newProjectionReplayGuard(),
		bodyStates:               make([]timelineBodyState, 1),
		currentBodies:            make([]*corev1.MessageBody, 1),
		supersededBodySequences:  make(map[timelineEventRef][]uint64),
		tombstonedAt:             make(map[timelineEventRef]time.Time),
		shreddedAt:               make(map[timelineUserRef]time.Time),
		attachmentMessagesByRoom: make(map[timelineRoomRef][]timelineEventRef),
		attachmentMessageRoom:    make([]timelineRoomRef, 1),
		echoLinks:                make(map[timelineEventRef][]timelineEventRef),
		messageFlags:             make([]timelineMessageFlags, 1),
		shreddedUsers:            make([]bool, 1),
		buckets:                  make(map[timelineBucketKey]*timelineBucket),
		eventLoader:              options.eventLoader,
		hotWindow:                options.hotWindow,
		now:                      now,
		retainAll:                options.retainAll,
		logger:                   logger,
	}
}

// Subjects implements events.Projection. The projection owns the
// "everything that happened in this room" surface, so it subscribes to the
// room aggregate namespace plus the extra user key-shred events it needs.
func (p *RoomTimelineProjection) Subjects() []string {
	return []string{events.RoomSubjectFilter(), events.UserEventTypeFilter(events.EventUserKeyShredded)}
}

// ReplaySubjects uses one stream-wide physical filter because JetStream's
// multi-filter scan is expensive when it combines the broad room wildcard with
// the sparse user-key-shredded family. The Projector rejects unrelated subjects
// before decoding or applying them.
func (p *RoomTimelineProjection) ReplaySubjects() []string {
	return []string{events.EventSubjectFilter()}
}

// Apply implements events.Projection. Extracts the room_id from whichever
// room-scoped event variant we recognise and appends visible entries to that
// room's slice. Events that don't carry a room_id (shouldn't appear on
// evt.room.>, but defensive) are silently skipped — projections forward-compat
// by ignoring what they don't understand.
func (p *RoomTimelineProjection) Apply(event *corev1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
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
				targetRef := p.internEventIDLocked(targetID)
				authorRef := p.internUserIDLocked(authorID)
				bucket := p.messageBucketLocked(targetID, roomID, event)
				if err := p.recordBucketRefLocked(bucket, seq, true); err != nil {
					return err
				}
				resident := p.bucketResidentLocked(bucket)
				if p.shreddedUsers[authorRef] {
					p.clearBodyLocked(targetID)
					p.setMessageFlagLocked(targetRef, timelineMessageRetracted, true)
					p.setTombstonedAtLocked(targetID, p.shreddedAt[authorRef])
					p.removeAttachmentMessageLocked(targetID)
				} else {
					body = cloneMessageBody(body)
					if body.GetBodyEventId() == "" {
						body.BodyEventId = event.GetId()
					}
					p.setCurrentBodyLocked(targetID, body, seq, bucket, resident)
					p.setMessageFlagLocked(targetRef, timelineMessageRetracted, false)
					p.refreshAttachmentMessageMetadataLocked(roomID, targetID, body)
				}
			}
		}
		return nil
	}

	bucket := p.bucketForMutationLocked(event, roomID)
	if err := p.recordBucketRefLocked(bucket, seq, false); err != nil {
		return err
	}
	resident := p.bucketResidentLocked(bucket)
	retainedEvent := event
	if !resident {
		retainedEvent = nil
	}
	entryIdx := int32(-1)
	if shouldIndexRoomTimelineEvent(event) {
		entryIdx = int32(p.appendEntryForBucketLocked(seq, event, bucket))
		p.entries[entryIdx].Event = retainedEvent
		if retainedEvent != nil {
			p.buckets[bucket].residentBytes += int64(proto.Size(retainedEvent))
		}
		if eventRef := p.entries[entryIdx].eventID; eventRef != 0 {
			p.entryByEvent[eventRef] = entryIdx
		}
	}
	if event.GetMessagePosted() != nil {
		if entryIdx < 0 {
			entryIdx = int32(p.appendEntryLocked(seq, event))
		}
		roomRef := p.internRoomIDLocked(roomID)
		p.messagePostsByRoom[roomRef] = append(p.messagePostsByRoom[roomRef], timelineEntryRef(entryIdx))
	}
	if isVisibleRoomTimelineEntry(event) {
		if entryIdx < 0 {
			entryIdx = int32(p.appendEntryLocked(seq, event))
		}
		roomRef := p.internRoomIDLocked(roomID)
		p.byRoom[roomRef] = append(p.byRoom[roomRef], timelineEntryRef(entryIdx))
	}

	// Maintain the latest-body / retracted-flag derived index so
	// LatestBody is O(1) instead of an O(room) walk per lookup.
	switch ev := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		targetID := event.GetId()
		targetRef := p.internEventIDLocked(targetID)
		if targetID != "" {
			authorRef := p.internUserIDLocked(messageAuthorID(event))
			if p.shreddedUsers[authorRef] {
				p.clearBodyLocked(targetID)
				p.setMessageFlagLocked(targetRef, timelineMessageRetracted, true)
				p.setTombstonedAtLocked(targetID, p.shreddedAt[authorRef])
				p.removeAttachmentMessageLocked(targetID)
			}
		}
		if state := p.bodyStates[targetRef]; state.known && state.hasAttachments {
			p.addAttachmentMessageLocked(roomID, targetID, p.entries[entryIdx].StreamSeq)
		}
		// Track echo links so edits on either side can fan out to the
		// other, and so original retractions can be reflected when
		// rendering echoes.
		if origID := ev.MessagePosted.GetEchoOfEventId(); origID != "" && targetID != "" {
			origRef := p.internEventIDLocked(origID)
			p.echoLinks[origRef] = append(p.echoLinks[origRef], targetRef)
		}
	case *corev1.Event_MessageRetracted:
		targetID := ev.MessageRetracted.GetEventId()
		if targetID != "" {
			targetRef := p.internEventIDLocked(targetID)
			p.setTombstonedAtLocked(targetID, eventCreatedAt(event))
			if origID := p.echoOriginalIDLocked(targetID); origID != "" {
				origRef, _ := p.eventRefLocked(origID)
				if !p.messageFlagLocked(origRef, timelineMessageRetracted) {
					p.clearBodyLocked(targetID)
					p.setMessageFlagLocked(targetRef, timelineMessageHiddenEcho, true)
					p.removeAttachmentMessageLocked(targetID)
					return nil
				}
			}
			p.clearBodyLocked(targetID)
			p.setMessageFlagLocked(targetRef, timelineMessageRetracted, true)
			p.removeAttachmentMessageLocked(targetID)
		}
	}
	return nil
}

func (p *RoomTimelineProjection) CompleteStartupReplay() {
	p.Lock()
	defer p.Unlock()
	p.replayGuard.completeReplay()
}

func eventMutatesRoomTimelineProjection(event *corev1.Event) bool {
	if event == nil {
		return false
	}
	if event.GetMessageBody() != nil || event.GetMessageRetracted() != nil {
		return true
	}
	return shouldIndexRoomTimelineEvent(event) || isVisibleRoomTimelineEntry(event)
}

func (p *RoomTimelineProjection) applyUserKeyShreddedLocked(userID string, at time.Time) {
	if userID == "" {
		return
	}
	userRef := p.internUserIDLocked(userID)
	p.shreddedUsers[userRef] = true
	if !at.IsZero() {
		if existing, ok := p.shreddedAt[userRef]; !ok || at.Before(existing) {
			p.shreddedAt[userRef] = at
		}
		at = p.shreddedAt[userRef]
	}
	for idx := range p.entries {
		entry := &p.entries[idx]
		if entry == nil {
			continue
		}
		if !entry.messagePosted {
			continue
		}
		if entry.authorID != userRef {
			continue
		}
		eventRef := entry.eventID
		eventID := p.eventIDLocked(eventRef)
		p.clearBodyLocked(eventID)
		p.setMessageFlagLocked(eventRef, timelineMessageRetracted, true)
		p.setTombstonedAtLocked(eventID, at)
		p.removeAttachmentMessageLocked(eventID)
	}
}

func (p *RoomTimelineProjection) setCurrentBodyLocked(eventID string, body *corev1.MessageBody, sequence uint64, bucket timelineBucketKey, resident bool) {
	eventRef := p.internEventIDLocked(eventID)
	state := p.bodyStates[eventRef]
	if state.known {
		p.supersededBodySequences[eventRef] = append(p.supersededBodySequences[eventRef], state.currentSequence)
		if p.currentBodies[eventRef] != nil {
			if oldBucket := p.buckets[state.bucket]; oldBucket != nil {
				oldBucket.residentBytes -= int64(proto.Size(p.currentBodies[eventRef]))
			}
		}
	}
	if resident {
		p.currentBodies[eventRef] = body
		if targetBucket := p.buckets[bucket]; targetBucket != nil {
			targetBucket.residentBytes += int64(proto.Size(body))
		}
	} else {
		p.currentBodies[eventRef] = nil
	}
	state.currentSequence = sequence
	state.bucket = bucket
	state.hasAttachments = messageBodyReferencesAttachments(body)
	state.known = true
	p.bodyStates[eventRef] = state
}

func (p *RoomTimelineProjection) clearBodyLocked(eventID string) {
	eventRef, exists := p.eventRefLocked(eventID)
	if !exists {
		return
	}
	state := p.bodyStates[eventRef]
	if !state.known {
		return
	}
	if p.currentBodies[eventRef] != nil {
		if bucket := p.buckets[state.bucket]; bucket != nil {
			bucket.residentBytes -= int64(proto.Size(p.currentBodies[eventRef]))
		}
	}
	p.currentBodies[eventRef] = nil
}

func (p *RoomTimelineProjection) setTombstonedAtLocked(eventID string, at time.Time) {
	if eventID == "" || at.IsZero() {
		return
	}
	eventRef := p.internEventIDLocked(eventID)
	if existing, ok := p.tombstonedAt[eventRef]; !ok || at.Before(existing) {
		p.tombstonedAt[eventRef] = at
	}
}

// RoomEvents returns up to `limit` entries from a room's timeline in
// newest-first order, optionally bounded by an exclusive
// stream-sequence cursor (beforeStreamSeq == 0 means "from the
// newest"). It materializes any cold bucket needed by the page. Returns a
// fresh slice; entries and event payloads are immutable and must be treated as
// read-only by callers.
//
// Entries are the room-visible timeline; folded state such as edits, reactions,
// thread replies, asset processing, and directly hidden echoes is excluded.
func (p *RoomTimelineProjection) RoomEvents(roomID string, limit int, beforeStreamSeq uint64) []*TimelineEntry {
	entries, _ := p.RoomEventsContext(context.Background(), roomID, limit, beforeStreamSeq)
	return entries
}

func (p *RoomTimelineProjection) RoomEventsContext(ctx context.Context, roomID string, limit int, beforeStreamSeq uint64) ([]*TimelineEntry, error) {
	if limit <= 0 {
		return nil, nil
	}
	p.RLock()
	roomRef, ok := p.roomRefLocked(roomID)
	if !ok {
		p.RUnlock()
		return nil, nil
	}
	entryIndexes := p.byRoom[roomRef]
	if len(entryIndexes) == 0 {
		p.RUnlock()
		return nil, nil
	}
	selected := make([]timelineEntryRef, 0, limit)
	for i := len(entryIndexes) - 1; i >= 0 && len(selected) < limit; i-- {
		idx := entryIndexes[i]
		e := p.entryAtLocked(idx)
		if e == nil {
			continue
		}
		if beforeStreamSeq > 0 && e.StreamSeq >= beforeStreamSeq {
			continue
		}
		selected = append(selected, idx)
	}
	p.RUnlock()
	if err := p.ensureEntryIndexes(ctx, selected); err != nil {
		return nil, err
	}
	p.RLock()
	out := make([]*TimelineEntry, 0, len(selected))
	for _, idx := range selected {
		if entry := p.entryAtLocked(idx); entry != nil {
			out = append(out, entry)
		}
	}
	p.RUnlock()
	return out, nil
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
	roomRef, ok := p.roomRefLocked(roomID)
	if !ok {
		return 0
	}
	n := 0
	for _, idx := range p.byRoom[roomRef] {
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

func shouldIndexRoomTimelineEvent(event *corev1.Event) bool {
	if event == nil {
		return false
	}
	switch event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		return true
	default:
		return isVisibleRoomTimelineEntry(event)
	}
}

// Get returns a single timeline entry by its envelope id, or
// (nil, false) if no such event has been projected.
func (p *RoomTimelineProjection) Get(eventID string) (*TimelineEntry, bool) {
	entry, ok, _ := p.GetContext(context.Background(), eventID)
	return entry, ok
}

// EventSequence returns an event's EVT stream position without materializing
// its bucket payload.
func (p *RoomTimelineProjection) EventSequence(eventID string) (uint64, bool) {
	p.RLock()
	defer p.RUnlock()

	entry, ok := p.entryByEventIDLocked(eventID)
	if !ok {
		return 0, false
	}
	return entry.StreamSeq, true
}

func (p *RoomTimelineProjection) GetContext(ctx context.Context, eventID string) (*TimelineEntry, bool, error) {
	p.RLock()
	entry, ok := p.entryByEventIDLocked(eventID)
	if !ok {
		p.RUnlock()
		return nil, false, nil
	}
	bucket := entry.bucket
	p.RUnlock()
	if err := p.ensureBucket(ctx, bucket); err != nil {
		return nil, false, err
	}
	p.RLock()
	entry, ok = p.entryByEventIDLocked(eventID)
	p.RUnlock()
	return entry, ok, nil
}

// LastRoomMessageEntry returns the newest non-hidden MessagePostedEvent in a
// room, including thread replies that are intentionally absent from byRoom.
func (p *RoomTimelineProjection) LastRoomMessageEntry(roomID string) (*TimelineEntry, bool) {
	entry, ok, _ := p.LastRoomMessageEntryContext(context.Background(), roomID)
	return entry, ok
}

func (p *RoomTimelineProjection) LastRoomMessageEntryContext(ctx context.Context, roomID string) (*TimelineEntry, bool, error) {
	p.RLock()
	roomRef, ok := p.roomRefLocked(roomID)
	if !ok {
		p.RUnlock()
		return nil, false, nil
	}
	entryIndexes := p.messagePostsByRoom[roomRef]
	for i := len(entryIndexes) - 1; i >= 0; i-- {
		e := p.entryAtLocked(entryIndexes[i])
		if e == nil {
			continue
		}
		if p.isHiddenEchoEntryLocked(e) {
			continue
		}
		index := entryIndexes[i]
		p.RUnlock()
		if err := p.ensureEntryIndexes(ctx, []timelineEntryRef{index}); err != nil {
			return nil, false, err
		}
		p.RLock()
		e = p.entryAtLocked(index)
		p.RUnlock()
		return e, true, nil
	}
	p.RUnlock()
	return nil, false, nil
}

// LatestBody returns the current MessageBodyEvent body for a message, or nil +
// retracted=true if a MessageRetractedEvent has landed.
//
// Returns (nil, false, false) if the event_id isn't known to the
// projection (caller can treat as "not found yet").
//
// O(1): consults the dense body-state / message-flag indexes
// that Apply keeps in lockstep with byRoom.
func (p *RoomTimelineProjection) LatestBody(eventID string) (body *corev1.MessageBody, retracted bool, ok bool) {
	body, retracted, ok, _ = p.LatestBodyContext(context.Background(), eventID)
	return body, retracted, ok
}

func (p *RoomTimelineProjection) LatestBodyContext(ctx context.Context, eventID string) (body *corev1.MessageBody, retracted bool, ok bool, err error) {
	p.RLock()
	if eventID == "" {
		p.RUnlock()
		return nil, false, false, nil
	}
	eventRef, exists := p.eventRefLocked(eventID)
	if !exists || int(eventRef) >= len(p.entryByEvent) || p.entryByEvent[eventRef] == missingTimelineEntry {
		p.RUnlock()
		return nil, false, false, nil
	}
	if p.messageFlagLocked(eventRef, timelineMessageHiddenEcho) {
		p.RUnlock()
		return nil, true, true, nil
	}
	if p.messageFlagLocked(eventRef, timelineMessageRetracted) {
		p.RUnlock()
		return nil, true, true, nil
	}
	if origID := p.echoOriginalIDLocked(eventID); origID != "" {
		origRef, _ := p.eventRefLocked(origID)
		if p.messageFlagLocked(origRef, timelineMessageRetracted) {
			p.RUnlock()
			return nil, true, true, nil
		}
	}
	state := p.bodyStates[eventRef]
	if !state.known {
		p.RUnlock()
		return nil, false, true, nil
	}
	bucket := state.bucket
	if currentBody := p.currentBodies[eventRef]; currentBody != nil {
		body = cloneMessageBody(currentBody)
		p.RUnlock()
		return body, false, true, nil
	}
	p.RUnlock()
	if err := p.ensureBucket(ctx, bucket); err != nil {
		return nil, false, false, err
	}
	p.RLock()
	defer p.RUnlock()
	eventRef, exists = p.eventRefLocked(eventID)
	if !exists || int(eventRef) >= len(p.entryByEvent) || p.entryByEvent[eventRef] == missingTimelineEntry {
		return nil, false, false, nil
	}
	if p.messageFlagLocked(eventRef, timelineMessageHiddenEcho) {
		return nil, true, true, nil
	}
	if p.messageFlagLocked(eventRef, timelineMessageRetracted) {
		return nil, true, true, nil
	}
	if origID := p.echoOriginalIDLocked(eventID); origID != "" {
		origRef, _ := p.eventRefLocked(origID)
		if p.messageFlagLocked(origRef, timelineMessageRetracted) {
			return nil, true, true, nil
		}
	}
	if state := p.bodyStates[eventRef]; state.known && p.currentBodies[eventRef] != nil {
		return cloneMessageBody(p.currentBodies[eventRef]), false, true, nil
	}
	return nil, false, true, nil
}

// CurrentRoomAttachmentMessages returns current, visible messages whose latest
// body references attachments. Results are newest message first.
func (p *RoomTimelineProjection) CurrentRoomAttachmentMessages(roomID string) []projectedRoomAttachmentMessage {
	messages, _ := p.CurrentRoomAttachmentMessagesContext(context.Background(), roomID)
	return messages
}

func (p *RoomTimelineProjection) CurrentRoomAttachmentMessagesContext(ctx context.Context, roomID string) ([]projectedRoomAttachmentMessage, error) {
	p.RLock()

	roomRef, ok := p.roomRefLocked(roomID)
	if !ok {
		p.RUnlock()
		return nil, nil
	}
	ids := append([]timelineEventRef(nil), p.attachmentMessagesByRoom[roomRef]...)
	if len(ids) == 0 {
		p.RUnlock()
		return nil, nil
	}
	keys := make(map[timelineBucketKey]struct{})
	for _, eventRef := range ids {
		if state := p.bodyStates[eventRef]; state.known {
			keys[state.bucket] = struct{}{}
		}
	}
	p.RUnlock()
	for key := range keys {
		if err := p.ensureBucket(ctx, key); err != nil {
			return nil, err
		}
	}
	p.RLock()
	defer p.RUnlock()

	out := make([]projectedRoomAttachmentMessage, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		eventRef := ids[i]
		eventID := p.eventIDLocked(eventRef)
		entry, _ := p.entryByEventIDLocked(eventID)
		if entry == nil || entry.Event == nil || p.isHiddenEchoEntryLocked(entry) {
			continue
		}
		if p.messageFlagLocked(eventRef, timelineMessageRetracted) {
			continue
		}
		if origID := p.echoOriginalIDLocked(eventID); origID != "" {
			origRef, _ := p.eventRefLocked(origID)
			if p.messageFlagLocked(origRef, timelineMessageRetracted) {
				continue
			}
		}
		body := p.currentBodies[eventRef]
		if !messageBodyReferencesAttachments(body) {
			continue
		}
		out = append(out, projectedRoomAttachmentMessage{
			Entry: entry,
			Body:  cloneMessageBody(body),
		})
	}
	return out, nil
}

func (p *RoomTimelineProjection) refreshAttachmentMessageMetadataLocked(roomID, eventID string, body *corev1.MessageBody) {
	eventRef := p.internEventIDLocked(eventID)
	state := p.bodyStates[eventRef]
	state.hasAttachments = messageBodyReferencesAttachments(body)
	p.bodyStates[eventRef] = state
	if !state.hasAttachments {
		p.removeAttachmentMessageLocked(eventID)
		return
	}
	if entry, ok := p.entryByEventIDLocked(eventID); ok {
		p.addAttachmentMessageLocked(roomID, eventID, entry.StreamSeq)
	}
}

func (p *RoomTimelineProjection) addAttachmentMessageLocked(roomID, eventID string, streamSeq uint64) {
	if roomID == "" || eventID == "" {
		return
	}
	roomRef := p.internRoomIDLocked(roomID)
	eventRef := p.internEventIDLocked(eventID)
	if existingRoom := p.attachmentMessageRoom[eventRef]; existingRoom != 0 {
		if existingRoom == roomRef {
			return
		}
		p.removeAttachmentMessageLocked(eventID)
	}

	ids := p.attachmentMessagesByRoom[roomRef]
	insertAt := len(ids)
	if len(ids) > 0 {
		last, _ := p.entryByEventIDLocked(p.eventIDLocked(ids[len(ids)-1]))
		if last != nil && last.StreamSeq <= streamSeq {
			ids = append(ids, eventRef)
			p.attachmentMessagesByRoom[roomRef] = ids
			p.attachmentMessageRoom[eventRef] = roomRef
			return
		}
		for i, existingRef := range ids {
			existing, _ := p.entryByEventIDLocked(p.eventIDLocked(existingRef))
			if existing == nil || existing.StreamSeq > streamSeq {
				insertAt = i
				break
			}
		}
	}
	ids = append(ids, 0)
	copy(ids[insertAt+1:], ids[insertAt:])
	ids[insertAt] = eventRef
	p.attachmentMessagesByRoom[roomRef] = ids
	p.attachmentMessageRoom[eventRef] = roomRef
}

func (p *RoomTimelineProjection) removeAttachmentMessageLocked(eventID string) {
	eventRef, ok := p.eventRefLocked(eventID)
	if !ok || int(eventRef) >= len(p.attachmentMessageRoom) {
		return
	}
	roomRef := p.attachmentMessageRoom[eventRef]
	if roomRef == 0 {
		return
	}
	ids := p.attachmentMessagesByRoom[roomRef]
	for i, existingRef := range ids {
		if existingRef != eventRef {
			continue
		}
		ids = append(ids[:i], ids[i+1:]...)
		break
	}
	if len(ids) == 0 {
		delete(p.attachmentMessagesByRoom, roomRef)
	} else {
		p.attachmentMessagesByRoom[roomRef] = ids
	}
	p.attachmentMessageRoom[eventRef] = 0
}

func messageBodyReferencesAttachments(body *corev1.MessageBody) bool {
	return len(ownedAssetIDsFromBody(body)) > 0
}

// BodyEventSeqs returns all projected MessageBodyEvent stream sequences for
// a message, plus the current body sequence if one is still active.
func (p *RoomTimelineProjection) BodyEventSeqs(eventID string) (seqs []uint64, current uint64, ok bool) {
	p.RLock()
	defer p.RUnlock()
	if eventID == "" {
		return nil, 0, false
	}
	eventRef, exists := p.eventRefLocked(eventID)
	if !exists || int(eventRef) >= len(p.entryByEvent) || p.entryByEvent[eventRef] == missingTimelineEntry {
		return nil, 0, false
	}
	state := p.bodyStates[eventRef]
	if !state.known {
		return nil, 0, true
	}
	superseded := p.supersededBodySequences[eventRef]
	seqs = make([]uint64, 0, len(superseded)+1)
	seqs = append(seqs, superseded...)
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
	eventRef, ok := p.eventRefLocked(eventID)
	if !ok {
		return nil
	}
	state := p.bodyStates[eventRef]
	if !state.known {
		return nil
	}
	if p.messageFlagLocked(eventRef, timelineMessageRetracted) {
		return p.appendBodySequencesLocked(nil, eventRef, state)
	}
	if p.messageFlagLocked(eventRef, timelineMessageHiddenEcho) {
		return p.appendBodySequencesLocked(nil, eventRef, state)
	}
	return append([]uint64(nil), p.supersededBodySequences[eventRef]...)
}

// AllObsoleteBodyEventSeqs returns every projected MessageBodyEvent seq
// whose payload is no longer needed for the current message state.
func (p *RoomTimelineProjection) AllObsoleteBodyEventSeqs() []uint64 {
	p.RLock()
	defer p.RUnlock()
	var out []uint64
	for eventRef, state := range p.bodyStates {
		if !state.known {
			continue
		}
		ref := timelineEventRef(eventRef)
		if p.messageFlagLocked(ref, timelineMessageRetracted) {
			out = p.appendBodySequencesLocked(out, ref, state)
			continue
		}
		if p.messageFlagLocked(ref, timelineMessageHiddenEcho) {
			out = p.appendBodySequencesLocked(out, ref, state)
			continue
		}
		out = append(out, p.supersededBodySequences[ref]...)
	}
	return out
}

func (p *RoomTimelineProjection) appendBodySequencesLocked(dst []uint64, ref timelineEventRef, state timelineBodyState) []uint64 {
	dst = append(dst, p.supersededBodySequences[ref]...)
	return append(dst, state.currentSequence)
}

func (p *RoomTimelineProjection) echoOriginalIDLocked(eventID string) string {
	entry, ok := p.entryByEventIDLocked(eventID)
	if !ok || entry == nil {
		return ""
	}
	if !entry.messagePosted {
		return ""
	}
	return p.eventIDLocked(entry.echoOriginalID)
}

// IsEcho reports whether eventID is a MessagePostedEvent echo.
func (p *RoomTimelineProjection) IsEcho(eventID string) bool {
	p.RLock()
	defer p.RUnlock()
	return p.echoOriginalIDLocked(eventID) != ""
}

func (p *RoomTimelineProjection) messageLocator(eventID string) (roomID, echoOriginalID string, ok bool) {
	p.RLock()
	defer p.RUnlock()
	entry, ok := p.entryByEventIDLocked(eventID)
	if !ok || entry == nil || !entry.messagePosted {
		return "", "", false
	}
	return p.roomIDLocked(entry.roomID), p.eventIDLocked(entry.echoOriginalID), true
}

// IsHiddenEcho reports whether an echo has been directly retracted from the
// room timeline.
func (p *RoomTimelineProjection) IsHiddenEcho(eventID string) bool {
	p.RLock()
	defer p.RUnlock()
	ref, ok := p.eventRefLocked(eventID)
	return ok && p.messageFlagLocked(ref, timelineMessageHiddenEcho)
}

// ChannelEchoEventID returns the first visible echo event for an original
// thread reply, if one exists. Hidden/retracted echoes are ignored.
func (p *RoomTimelineProjection) ChannelEchoEventID(originalEventID string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	if originalEventID == "" {
		return "", false
	}
	originalRef, ok := p.eventRefLocked(originalEventID)
	if !ok {
		return "", false
	}
	for _, echoRef := range p.echoLinks[originalRef] {
		if echoRef == 0 {
			continue
		}
		if p.messageFlagLocked(echoRef, timelineMessageHiddenEcho) {
			continue
		}
		if p.messageFlagLocked(echoRef, timelineMessageRetracted) {
			continue
		}
		echoID := p.eventIDLocked(echoRef)
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

// LinkedChannelEchoEventID returns the first non-hidden echo linked to an
// original reply, including a retracted echo that must render as a tombstone.
func (p *RoomTimelineProjection) LinkedChannelEchoEventID(originalEventID string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	if originalEventID == "" {
		return "", false
	}
	originalRef, ok := p.eventRefLocked(originalEventID)
	if !ok {
		return "", false
	}
	for _, echoRef := range p.echoLinks[originalRef] {
		if echoRef == 0 {
			continue
		}
		if p.messageFlagLocked(echoRef, timelineMessageHiddenEcho) {
			continue
		}
		echoID := p.eventIDLocked(echoRef)
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
	ref, ok := p.eventRefLocked(eventID)
	return ok && p.messageFlagLocked(ref, timelineMessageRetracted)
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
	ref, ok := p.eventRefLocked(eventID)
	if ok {
		if at, exists := p.tombstonedAt[ref]; exists {
			return at, true
		}
	}
	if origID := p.echoOriginalIDLocked(eventID); origID != "" {
		origRef, exists := p.eventRefLocked(origID)
		at, found := p.tombstonedAt[origRef]
		return at, exists && found
	}
	return time.Time{}, false
}

func cloneMessageBody(body *corev1.MessageBody) *corev1.MessageBody {
	if body == nil {
		return nil
	}
	return proto.Clone(body).(*corev1.MessageBody)
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

// LinkedEventIDs returns the set of event_ids that an edit targeting
// `eventID` should also be applied to: any echoes pointing
// at `eventID`, plus the original message that `eventID` is an echo
// of (if any). Does NOT include `eventID` itself — the caller emits
// the mutation for the target separately.
//
// Used by EditMessage to preserve the legacy "edit the echo, the
// original updates too (and vice versa)" semantic after the shared-
// messageBodyId mechanism was retired in #614.
func (p *RoomTimelineProjection) LinkedEventIDs(eventID string) []string {
	p.RLock()
	defer p.RUnlock()
	if eventID == "" {
		return nil
	}
	linked := make([]string, 0, 2)
	eventRef, exists := p.eventRefLocked(eventID)
	if !exists {
		return nil
	}

	// Forward: echoes pointing at this event.
	for _, echoRef := range p.echoLinks[eventRef] {
		if echoRef != eventRef {
			linked = append(linked, p.eventIDLocked(echoRef))
		}
	}

	// Backward: if this event IS an echo, include the original.
	if entry, ok := p.entryByEventIDLocked(eventID); ok {
		if entry.messagePosted {
			if origRef := entry.echoOriginalID; origRef != 0 && origRef != eventRef {
				origID := p.eventIDLocked(origRef)
				linked = append(linked, origID)
				// Also include any sibling echoes of the same original
				// (rare, but possible if "also send to channel" was
				// invoked twice — keep semantics consistent).
				for _, siblingRef := range p.echoLinks[origRef] {
					if siblingRef != eventRef && siblingRef != origRef {
						linked = append(linked, p.eventIDLocked(siblingRef))
					}
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
	visible func(*corev1.Event) bool,
) (*TimelineEntry, bool) {
	entry, ok, _ := p.LastVisibleRoomEntryContext(context.Background(), roomID, visible)
	return entry, ok
}

func (p *RoomTimelineProjection) LastVisibleRoomEntryContext(
	ctx context.Context,
	roomID string,
	visible func(*corev1.Event) bool,
) (*TimelineEntry, bool, error) {
	p.RLock()
	roomRef, ok := p.roomRefLocked(roomID)
	if !ok {
		p.RUnlock()
		return nil, false, nil
	}
	entryIndexes := append([]timelineEntryRef(nil), p.byRoom[roomRef]...)
	p.RUnlock()
	for i := len(entryIndexes) - 1; i >= 0; i-- {
		idx := entryIndexes[i]
		p.RLock()
		e := p.entryAtLocked(idx)
		if e == nil {
			p.RUnlock()
			continue
		}
		if p.isHiddenEchoEntryLocked(e) {
			p.RUnlock()
			continue
		}
		bucket := e.bucket
		p.RUnlock()
		if err := p.ensureBucket(ctx, bucket); err != nil {
			return nil, false, err
		}
		p.RLock()
		e = p.entryAtLocked(idx)
		if visible != nil && !visible(e.Event) {
			p.RUnlock()
			continue
		}
		p.RUnlock()
		return e, true, nil
	}
	return nil, false, nil
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
	visible func(*corev1.Event) bool,
) []*TimelineEntry {
	entries, _ := p.VisibleRoomTimelineContext(context.Background(), roomID, limit, beforeStreamSeq, visible)
	return entries
}

func (p *RoomTimelineProjection) VisibleRoomTimelineContext(
	ctx context.Context,
	roomID string,
	limit int,
	beforeStreamSeq uint64,
	visible func(*corev1.Event) bool,
) ([]*TimelineEntry, error) {
	if limit <= 0 {
		return nil, nil
	}
	p.RLock()
	roomRef, ok := p.roomRefLocked(roomID)
	if !ok {
		p.RUnlock()
		return nil, nil
	}
	entryIndexes := append([]timelineEntryRef(nil), p.byRoom[roomRef]...)
	p.RUnlock()
	out := make([]*TimelineEntry, 0, limit)
	for i := len(entryIndexes) - 1; i >= 0 && len(out) < limit; i-- {
		idx := entryIndexes[i]
		p.RLock()
		e := p.entryAtLocked(idx)
		if e == nil {
			p.RUnlock()
			continue
		}
		if beforeStreamSeq > 0 && e.StreamSeq >= beforeStreamSeq {
			p.RUnlock()
			continue
		}
		if p.isHiddenEchoEntryLocked(e) {
			p.RUnlock()
			continue
		}
		bucket := e.bucket
		p.RUnlock()
		if err := p.ensureBucket(ctx, bucket); err != nil {
			return nil, err
		}
		p.RLock()
		e = p.entryAtLocked(idx)
		if visible != nil && !visible(e.Event) {
			p.RUnlock()
			continue
		}
		out = append(out, e)
		p.RUnlock()
	}
	return out, nil
}

// VisibleRoomTimelineAfter walks the room's timeline oldest-first,
// applying `visible` as a per-entry filter, and returns up to `limit`
// matching entries with stream seq > afterStreamSeq. This is the
// forward-pagination counterpart to VisibleRoomTimeline.
func (p *RoomTimelineProjection) VisibleRoomTimelineAfter(
	roomID string,
	limit int,
	afterStreamSeq uint64,
	visible func(*corev1.Event) bool,
) []*TimelineEntry {
	entries, _ := p.VisibleRoomTimelineAfterContext(context.Background(), roomID, limit, afterStreamSeq, visible)
	return entries
}

func (p *RoomTimelineProjection) VisibleRoomTimelineAfterContext(
	ctx context.Context,
	roomID string,
	limit int,
	afterStreamSeq uint64,
	visible func(*corev1.Event) bool,
) ([]*TimelineEntry, error) {
	if limit <= 0 {
		return nil, nil
	}
	p.RLock()
	roomRef, ok := p.roomRefLocked(roomID)
	if !ok {
		p.RUnlock()
		return nil, nil
	}
	entryIndexes := append([]timelineEntryRef(nil), p.byRoom[roomRef]...)
	p.RUnlock()
	out := make([]*TimelineEntry, 0, limit)
	for _, idx := range entryIndexes {
		p.RLock()
		e := p.entryAtLocked(idx)
		if e == nil {
			p.RUnlock()
			continue
		}
		if e.StreamSeq <= afterStreamSeq {
			p.RUnlock()
			continue
		}
		if p.isHiddenEchoEntryLocked(e) {
			p.RUnlock()
			continue
		}
		bucket := e.bucket
		p.RUnlock()
		if err := p.ensureBucket(ctx, bucket); err != nil {
			return nil, err
		}
		p.RLock()
		e = p.entryAtLocked(idx)
		if visible != nil && !visible(e.Event) {
			p.RUnlock()
			continue
		}
		out = append(out, e)
		p.RUnlock()
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// VisibleRoomTimelineAround returns a room-visible window centered on eventID
// in oldest-first order. It walks the visible room slice, so edits/reactions/
// assets/thread replies are not revisited when serving "jump to message" style
// reads.
func (p *RoomTimelineProjection) VisibleRoomTimelineAround(
	roomID string,
	eventID string,
	limit int,
) (entries []*TimelineEntry, targetIndex int, hasOlder bool, hasNewer bool, ok bool) {
	entries, targetIndex, hasOlder, hasNewer, ok, _ = p.VisibleRoomTimelineAroundContext(context.Background(), roomID, eventID, limit)
	return entries, targetIndex, hasOlder, hasNewer, ok
}

func (p *RoomTimelineProjection) VisibleRoomTimelineAroundContext(
	ctx context.Context,
	roomID string,
	eventID string,
	limit int,
) (entries []*TimelineEntry, targetIndex int, hasOlder bool, hasNewer bool, ok bool, err error) {
	if limit <= 0 || eventID == "" {
		return nil, 0, false, false, false, nil
	}
	p.RLock()
	roomRef, roomKnown := p.roomRefLocked(roomID)
	if !roomKnown {
		p.RUnlock()
		return nil, 0, false, false, false, nil
	}
	targetRef, targetKnown := p.eventRefLocked(eventID)
	if !targetKnown {
		p.RUnlock()
		return nil, 0, false, false, false, nil
	}
	roomEntries := p.byRoom[roomRef]
	targetVisibleIndex := -1
	visibleCount := 0
	for _, idx := range roomEntries {
		entry := p.entryAtLocked(idx)
		if p.isHiddenEchoEntryLocked(entry) {
			continue
		}
		if entry != nil && entry.eventID == targetRef {
			targetVisibleIndex = visibleCount
		}
		visibleCount++
	}
	if targetVisibleIndex == -1 {
		p.RUnlock()
		return nil, 0, false, false, false, nil
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

	selected := make([]timelineEntryRef, 0, end-start)
	visibleIndex := 0
	for _, idx := range roomEntries {
		entry := p.entryAtLocked(idx)
		if p.isHiddenEchoEntryLocked(entry) {
			continue
		}
		if visibleIndex >= start && visibleIndex < end {
			selected = append(selected, idx)
		}
		visibleIndex++
		if visibleIndex >= end {
			break
		}
	}
	p.RUnlock()
	if err := p.ensureEntryIndexes(ctx, selected); err != nil {
		return nil, 0, false, false, false, err
	}
	p.RLock()
	out := make([]*TimelineEntry, 0, len(selected))
	for _, idx := range selected {
		if entry := p.entryAtLocked(idx); entry != nil {
			out = append(out, entry)
		}
	}
	p.RUnlock()
	return out, targetVisibleIndex - start, start > 0, end < visibleCount, true, nil
}

func (p *RoomTimelineProjection) isHiddenEchoEntryLocked(entry *TimelineEntry) bool {
	if entry == nil {
		return false
	}
	return p.messageFlagLocked(entry.eventID, timelineMessageHiddenEcho)
}
