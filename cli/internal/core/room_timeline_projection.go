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
	byRoom             map[string][]int
	byEventID          map[string]int
	messagePostsByRoom map[string][]int
	replayGuard        projectionReplayGuard
	// bodyStates keeps the current encrypted body and its EVT lifecycle in one
	// entry per message. supersededSequences stays nil until the first edit,
	// avoiding a slice allocation for the common single-body case.
	bodyStates     map[string]timelineBodyState
	retractedFlags map[string]struct{}
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
	// with EchoOfEventId arrive. Used by EditMessage / DeleteMessage
	// to fan mutations across linked messages. Each echo has its own
	// projected body payload, so edits and retractions need explicit
	// propagation.
	echoLinks map[string][]string
	// hiddenEchoes tracks echo MessagePostedEvents that were directly
	// retracted. A direct echo retract removes the room-timeline copy
	// without deleting the original thread reply's content.
	hiddenEchoes  map[string]struct{}
	shreddedUsers map[string]struct{}

	buckets     map[timelineBucketKey]*timelineBucket
	eventLoader roomTimelineEventLoader
	hotWindow   time.Duration
	now         func() time.Time
	retainAll   bool
	logger      *log.Logger
}

// TimelineEntry is one event's position in a room timeline. Event is present
// while the entry's weekly bucket is resident; the remaining fields form the
// lightweight directory used to locate and materialize a cold bucket.
type TimelineEntry struct {
	StreamSeq uint64
	Event     *corev1.Event

	eventID        string
	roomID         string
	authorID       string
	echoOriginalID string
	bucket         timelineBucketKey
	messagePosted  bool
}

type projectedRoomAttachmentMessage struct {
	Entry *TimelineEntry
	Body  *corev1.MessageBody
}

type timelineBodyState struct {
	body                *corev1.MessageBody
	currentSequence     uint64
	supersededSequences []uint64
	bucket              timelineBucketKey
	hasAttachments      bool
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
		entry.eventID = event.GetId()
		entry.roomID = roomIDOfEvent(event)
		entry.authorID = messageAuthorID(event)
		entry.messagePosted = event.GetMessagePosted() != nil
		if posted := event.GetMessagePosted(); posted != nil {
			entry.echoOriginalID = posted.GetEchoOfEventId()
		}
	}
	p.entries = append(p.entries, entry)
	return idx
}

func (p *RoomTimelineProjection) entryAtLocked(idx int) *TimelineEntry {
	if idx < 0 || idx >= len(p.entries) {
		return nil
	}
	return &p.entries[idx]
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
		byRoom:                     make(map[string][]int),
		byEventID:                  make(map[string]int),
		messagePostsByRoom:         make(map[string][]int),
		replayGuard:                newProjectionReplayGuard(),
		bodyStates:                 make(map[string]timelineBodyState),
		retractedFlags:             make(map[string]struct{}),
		tombstonedAt:               make(map[string]time.Time),
		shreddedAt:                 make(map[string]time.Time),
		attachmentMessageIDsByRoom: make(map[string][]string),
		attachmentMessageRoom:      make(map[string]string),
		echoLinks:                  make(map[string][]string),
		hiddenEchoes:               make(map[string]struct{}),
		shreddedUsers:              make(map[string]struct{}),
		buckets:                    make(map[timelineBucketKey]*timelineBucket),
		eventLoader:                options.eventLoader,
		hotWindow:                  options.hotWindow,
		now:                        now,
		retainAll:                  options.retainAll,
		logger:                     logger,
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
				bucket := p.messageBucketLocked(targetID, roomID, event)
				p.recordBucketRefLocked(bucket, seq, true)
				resident := p.bucketResidentLocked(bucket)
				if _, shredded := p.shreddedUsers[authorID]; shredded {
					p.clearBodyLocked(targetID)
					p.retractedFlags[targetID] = struct{}{}
					p.setTombstonedAtLocked(targetID, p.shreddedAt[authorID])
					p.removeAttachmentMessageLocked(targetID)
				} else {
					body = cloneMessageBody(body)
					if body.GetBodyEventId() == "" {
						body.BodyEventId = event.GetId()
					}
					p.setCurrentBodyLocked(targetID, body, seq, bucket, resident)
					delete(p.retractedFlags, targetID)
					p.refreshAttachmentMessageMetadataLocked(roomID, targetID, body)
				}
			}
		}
		return nil
	}

	bucket := p.bucketForMutationLocked(event, roomID)
	p.recordBucketRefLocked(bucket, seq, false)
	resident := p.bucketResidentLocked(bucket)
	retainedEvent := event
	if !resident {
		retainedEvent = nil
	}
	entryIdx := -1
	if shouldIndexRoomTimelineEvent(event) {
		entryIdx = p.appendEntryForBucketLocked(seq, event, bucket)
		p.entries[entryIdx].Event = retainedEvent
		if retainedEvent != nil {
			p.buckets[bucket].residentBytes += int64(proto.Size(retainedEvent))
		}
		if eid := event.GetId(); eid != "" {
			p.byEventID[eid] = entryIdx
		}
	}
	if event.GetMessagePosted() != nil {
		if entryIdx < 0 {
			entryIdx = p.appendEntryLocked(seq, event)
		}
		p.messagePostsByRoom[roomID] = append(p.messagePostsByRoom[roomID], entryIdx)
	}
	if isVisibleRoomTimelineEntry(event) {
		if entryIdx < 0 {
			entryIdx = p.appendEntryLocked(seq, event)
		}
		p.byRoom[roomID] = append(p.byRoom[roomID], entryIdx)
	}

	// Maintain the latest-body / retracted-flag derived index so
	// LatestBody is O(1) instead of an O(room) walk per lookup.
	switch ev := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		targetID := event.GetId()
		if targetID != "" {
			authorID := messageAuthorID(event)
			if _, shredded := p.shreddedUsers[authorID]; shredded {
				p.clearBodyLocked(targetID)
				p.retractedFlags[targetID] = struct{}{}
				p.setTombstonedAtLocked(targetID, p.shreddedAt[authorID])
				p.removeAttachmentMessageLocked(targetID)
			}
		}
		if state, ok := p.bodyStates[targetID]; ok && state.hasAttachments {
			p.addAttachmentMessageLocked(roomID, targetID, p.entries[entryIdx].StreamSeq)
		}
		// Track echo links so edits on either side can fan out to the
		// other, and so original retractions can be reflected when
		// rendering echoes.
		if origID := ev.MessagePosted.GetEchoOfEventId(); origID != "" && targetID != "" {
			p.echoLinks[origID] = append(p.echoLinks[origID], targetID)
		}
	case *corev1.Event_MessageRetracted:
		targetID := ev.MessageRetracted.GetEventId()
		if targetID != "" {
			p.setTombstonedAtLocked(targetID, eventCreatedAt(event))
			if origID := p.echoOriginalIDLocked(targetID); origID != "" {
				if _, originalRetracted := p.retractedFlags[origID]; !originalRetracted {
					p.clearBodyLocked(targetID)
					p.hiddenEchoes[targetID] = struct{}{}
					p.removeAttachmentMessageLocked(targetID)
					return nil
				}
			}
			p.clearBodyLocked(targetID)
			p.retractedFlags[targetID] = struct{}{}
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
	p.shreddedUsers[userID] = struct{}{}
	if !at.IsZero() {
		if existing, ok := p.shreddedAt[userID]; !ok || at.Before(existing) {
			p.shreddedAt[userID] = at
		}
		at = p.shreddedAt[userID]
	}
	for eventID, idx := range p.byEventID {
		entry := p.entryAtLocked(idx)
		if entry == nil {
			continue
		}
		if !entry.messagePosted {
			continue
		}
		if entry.authorID != userID {
			continue
		}
		p.clearBodyLocked(eventID)
		p.retractedFlags[eventID] = struct{}{}
		p.setTombstonedAtLocked(eventID, at)
		p.removeAttachmentMessageLocked(eventID)
	}
}

func (p *RoomTimelineProjection) setCurrentBodyLocked(eventID string, body *corev1.MessageBody, sequence uint64, bucket timelineBucketKey, resident bool) {
	state, exists := p.bodyStates[eventID]
	if exists {
		state.supersededSequences = append(state.supersededSequences, state.currentSequence)
		if state.body != nil {
			if oldBucket := p.buckets[state.bucket]; oldBucket != nil {
				oldBucket.residentBytes -= int64(proto.Size(state.body))
			}
		}
	}
	if resident {
		state.body = body
		if targetBucket := p.buckets[bucket]; targetBucket != nil {
			targetBucket.residentBytes += int64(proto.Size(body))
		}
	} else {
		state.body = nil
	}
	state.currentSequence = sequence
	state.bucket = bucket
	state.hasAttachments = messageBodyReferencesAttachments(body)
	p.bodyStates[eventID] = state
}

func (p *RoomTimelineProjection) clearBodyLocked(eventID string) {
	state, exists := p.bodyStates[eventID]
	if !exists {
		return
	}
	if state.body != nil {
		if bucket := p.buckets[state.bucket]; bucket != nil {
			bucket.residentBytes -= int64(proto.Size(state.body))
		}
	}
	state.body = nil
	p.bodyStates[eventID] = state
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
	entryIndexes := p.byRoom[roomID]
	if len(entryIndexes) == 0 {
		p.RUnlock()
		return nil, nil
	}
	selected := make([]int, 0, limit)
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
	entryIndexes := p.messagePostsByRoom[roomID]
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
		if err := p.ensureEntryIndexes(ctx, []int{index}); err != nil {
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
// O(1): consults the derived bodyStates / retractedFlags indexes
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
	if _, exists := p.byEventID[eventID]; !exists {
		p.RUnlock()
		return nil, false, false, nil
	}
	if _, hidden := p.hiddenEchoes[eventID]; hidden {
		p.RUnlock()
		return nil, true, true, nil
	}
	if _, isRetracted := p.retractedFlags[eventID]; isRetracted {
		p.RUnlock()
		return nil, true, true, nil
	}
	if origID := p.echoOriginalIDLocked(eventID); origID != "" {
		if _, originalRetracted := p.retractedFlags[origID]; originalRetracted {
			p.RUnlock()
			return nil, true, true, nil
		}
	}
	state, has := p.bodyStates[eventID]
	if !has {
		p.RUnlock()
		return nil, false, true, nil
	}
	bucket := state.bucket
	if state.body != nil {
		body = cloneMessageBody(state.body)
		p.RUnlock()
		return body, false, true, nil
	}
	p.RUnlock()
	if err := p.ensureBucket(ctx, bucket); err != nil {
		return nil, false, false, err
	}
	p.RLock()
	defer p.RUnlock()
	if _, exists := p.byEventID[eventID]; !exists {
		return nil, false, false, nil
	}
	if _, hidden := p.hiddenEchoes[eventID]; hidden {
		return nil, true, true, nil
	}
	if _, isRetracted := p.retractedFlags[eventID]; isRetracted {
		return nil, true, true, nil
	}
	if origID := p.echoOriginalIDLocked(eventID); origID != "" {
		if _, originalRetracted := p.retractedFlags[origID]; originalRetracted {
			return nil, true, true, nil
		}
	}
	if state, has := p.bodyStates[eventID]; has && state.body != nil {
		return cloneMessageBody(state.body), false, true, nil
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

	ids := append([]string(nil), p.attachmentMessageIDsByRoom[roomID]...)
	if len(ids) == 0 {
		p.RUnlock()
		return nil, nil
	}
	keys := make(map[timelineBucketKey]struct{})
	for _, eventID := range ids {
		if state, ok := p.bodyStates[eventID]; ok {
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
		eventID := ids[i]
		entry, _ := p.entryByEventIDLocked(eventID)
		if entry == nil || entry.Event == nil || p.isHiddenEchoEntryLocked(entry) {
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
		body := p.bodyStates[eventID].body
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
	state := p.bodyStates[eventID]
	state.hasAttachments = messageBodyReferencesAttachments(body)
	p.bodyStates[eventID] = state
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
	if _, exists := p.byEventID[eventID]; !exists {
		return nil, 0, false
	}
	state, hasBodyState := p.bodyStates[eventID]
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
	state, ok := p.bodyStates[eventID]
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
	for eventID, state := range p.bodyStates {
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
	if !entry.messagePosted {
		return ""
	}
	return entry.echoOriginalID
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
	return entry.roomID, entry.echoOriginalID, true
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

	// Forward: echoes pointing at this event.
	for _, echoID := range p.echoLinks[eventID] {
		if echoID != eventID {
			linked = append(linked, echoID)
		}
	}

	// Backward: if this event IS an echo, include the original.
	if entry, ok := p.entryByEventIDLocked(eventID); ok {
		if entry.messagePosted {
			if origID := entry.echoOriginalID; origID != "" && origID != eventID {
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
	entryIndexes := append([]int(nil), p.byRoom[roomID]...)
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
	entryIndexes := append([]int(nil), p.byRoom[roomID]...)
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
	entryIndexes := append([]int(nil), p.byRoom[roomID]...)
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
	roomEntries := p.byRoom[roomID]
	targetVisibleIndex := -1
	visibleCount := 0
	for _, idx := range roomEntries {
		entry := p.entryAtLocked(idx)
		if p.isHiddenEchoEntryLocked(entry) {
			continue
		}
		if entry != nil && entry.eventID == eventID {
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

	selected := make([]int, 0, end-start)
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
	_, hidden := p.hiddenEchoes[entry.eventID]
	return hidden
}
