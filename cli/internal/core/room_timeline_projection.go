package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/singleflight"
	"google.golang.org/protobuf/proto"
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
	entries            []TimelineEntry
	byRoom             map[string][]int
	byEventID          map[string]int
	messagePostsByRoom map[string][]int
	// latestOriginalPostAt retains the newest committed non-echo post
	// timestamp per room and actor. Slow mode reads this O(1) index; edits and
	// retractions intentionally do not change it.
	latestOriginalPostAt map[roomActorKey]time.Time
	replayGuard          projectionReplayGuard
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
	hiddenEchoes         map[string]struct{}
	shreddedUsers        map[string]struct{}
	pinnedMessagesByRoom map[string]map[string]PinnedMessageState
	latestPinByRoom      map[string]latestRoomPinState

	// buckets is the always-resident reconstruction directory. The cache owns
	// decoded EVT payloads only while a bucket is pinned or recently used.
	bucketInterval       time.Duration
	pinnedPeriod         time.Duration
	idleTimeout          time.Duration
	now                  func() time.Time
	logger               events.Logger
	eventSource          timelineEventSource
	buckets              map[timelineBucketKey]*timelineBucketState
	messageBuckets       map[string]timelineBucketKey
	pendingBodySequences map[string][]uint64
	bucketMessages       map[timelineBucketKey]map[string]struct{}
	cache                map[timelineBucketKey]*timelineBucketCache
	loads                singleflight.Group
	loadSemaphore        chan struct{}
}

type timelineEventSource interface {
	EventAt(context.Context, uint64) (*evtstream.SubjectEvent, error)
}

// RoomTimelineProjectionOptions controls the process-local timeline cache.
// EventSource is required for snapshot restores and cold bucket loads.
type RoomTimelineProjectionOptions struct {
	EventSource  timelineEventSource
	Interval     time.Duration
	PinnedPeriod time.Duration
	IdleTimeout  time.Duration
	Now          func() time.Time
	// Logger receives cache reconstruction, failure, and eviction diagnostics.
	// A nil logger disables these diagnostics.
	Logger events.Logger
}

type timelineBucketKey struct {
	roomID      string
	startUnixNs int64
	undated     bool
}

type timelineBucketState struct {
	sequences []uint64
	revision  uint64
}

type timelineBucketCache struct {
	events     map[uint64]*evtv1.Event
	revision   uint64
	lastAccess time.Time
	pinned     bool
	// complete is false for caches assembled incrementally during startup
	// replay. A snapshot restore can start replay in the middle of a bucket, so
	// matching revisions alone do not prove that all earlier records were read.
	complete bool
}

type latestRoomPinState struct {
	PinEventID  string
	PinSequence uint64
}

type roomActorKey struct {
	roomID  string
	actorID string
}

// TimelineEntry is one event's position in a room timeline. Carries
// the full immutable event proto verbatim — payload, envelope, actor,
// created_at, oneof variant — so resolvers don't need to consult
// the projection's internal state to render.
type TimelineEntry struct {
	StreamSeq uint64
	Event     *evtv1.Event
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

type projectedRoomAttachmentMessageReference struct {
	Entry           *TimelineEntry
	AttachmentCount int
	AssetIDs        []string
}

type timelineBodyState struct {
	body                *evtv1.MessageBody
	currentSequence     uint64
	supersededSequences []uint64
	attachmentCount     int
	currentAssetIDs     []string
}

func (p *RoomTimelineProjection) appendEntryLocked(seq uint64, event *evtv1.Event) int {
	idx := len(p.entries)
	p.entries = append(p.entries, TimelineEntry{StreamSeq: seq, Event: event})
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
	return NewRoomTimelineProjectionWithOptions(RoomTimelineProjectionOptions{})
}

// NewRoomTimelineProjectionWithOptions returns an empty projection with a
// bounded, reconstructable timeline cache. A nil event source is intended for
// focused reducer tests and retains applied payloads for their lifetime.
func NewRoomTimelineProjectionWithOptions(options RoomTimelineProjectionOptions) *RoomTimelineProjection {
	if options.Interval <= 0 {
		options.Interval = 7 * 24 * time.Hour
	}
	if options.PinnedPeriod <= 0 {
		options.PinnedPeriod = 4 * 7 * 24 * time.Hour
	}
	if options.IdleTimeout <= 0 {
		options.IdleTimeout = 15 * time.Minute
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &RoomTimelineProjection{
		byRoom:                     make(map[string][]int),
		byEventID:                  make(map[string]int),
		messagePostsByRoom:         make(map[string][]int),
		latestOriginalPostAt:       make(map[roomActorKey]time.Time),
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
		pinnedMessagesByRoom:       make(map[string]map[string]PinnedMessageState),
		latestPinByRoom:            make(map[string]latestRoomPinState),
		bucketInterval:             options.Interval,
		pinnedPeriod:               options.PinnedPeriod,
		idleTimeout:                options.IdleTimeout,
		now:                        options.Now,
		logger:                     options.Logger,
		eventSource:                options.EventSource,
		buckets:                    make(map[timelineBucketKey]*timelineBucketState),
		messageBuckets:             make(map[string]timelineBucketKey),
		pendingBodySequences:       make(map[string][]uint64),
		bucketMessages:             make(map[timelineBucketKey]map[string]struct{}),
		cache:                      make(map[timelineBucketKey]*timelineBucketCache),
		loadSemaphore:              make(chan struct{}, 4),
	}
}

func (p *RoomTimelineProjection) bucketLogFields(key timelineBucketKey) []interface{} {
	fields := []interface{}{
		"room_id", key.roomID,
		"bucket_undated", key.undated,
	}
	if !key.undated {
		start := time.Unix(0, key.startUnixNs).UTC()
		fields = append(fields,
			"bucket_start", start,
			"bucket_end", start.Add(p.bucketInterval),
		)
	}
	return fields
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

// bucketKeyLocked maps a room event time to a stable UTC bucket. The Monday
// after the Unix epoch is the anchor, so the default weekly buckets start on
// Monday at 00:00 UTC.
func (p *RoomTimelineProjection) bucketKeyLocked(roomID string, at time.Time) timelineBucketKey {
	if at.IsZero() {
		return timelineBucketKey{roomID: roomID, undated: true}
	}
	anchor := time.Date(1970, time.January, 5, 0, 0, 0, 0, time.UTC)
	delta := at.UTC().Sub(anchor)
	steps := delta / p.bucketInterval
	if delta < 0 && delta%p.bucketInterval != 0 {
		steps--
	}
	return timelineBucketKey{roomID: roomID, startUnixNs: anchor.Add(steps * p.bucketInterval).UnixNano()}
}

func (p *RoomTimelineProjection) bucketPinnedLocked(key timelineBucketKey, now time.Time) bool {
	if p.eventSource == nil || key.undated {
		return p.eventSource == nil
	}
	start := time.Unix(0, key.startUnixNs).UTC()
	end := start.Add(p.bucketInterval)
	current := p.bucketKeyLocked(key.roomID, now)
	return key == current || (start.Before(now) && end.After(now.Add(-p.pinnedPeriod)))
}

func (p *RoomTimelineProjection) addBucketSequenceLocked(key timelineBucketKey, sequence uint64) {
	if key.roomID == "" || sequence == 0 {
		return
	}
	bucket := p.buckets[key]
	if bucket == nil {
		bucket = &timelineBucketState{}
		p.buckets[key] = bucket
	}
	insertAt, found := slices.BinarySearch(bucket.sequences, sequence)
	if found {
		return
	}
	bucket.sequences = append(bucket.sequences, 0)
	copy(bucket.sequences[insertAt+1:], bucket.sequences[insertAt:])
	bucket.sequences[insertAt] = sequence
	bucket.revision++
}

func (p *RoomTimelineProjection) cacheAppliedEventLocked(key timelineBucketKey, sequence uint64, event *evtv1.Event) {
	now := p.now()
	pinned := p.bucketPinnedLocked(key, now)
	cached := p.cache[key]
	if cached == nil && !pinned {
		return
	}
	if cached == nil {
		cached = &timelineBucketCache{events: make(map[uint64]*evtv1.Event)}
		p.cache[key] = cached
	}
	cached.pinned = pinned
	retainPayload := true
	if bodyEvent := event.GetMessageBody(); bodyEvent != nil {
		if target, ok := p.messageBuckets[bodyEvent.GetEventId()]; ok && target != key {
			retainPayload = false
		}
	}
	if retainPayload {
		cached.events[sequence] = proto.Clone(event).(*evtv1.Event)
	}
	cached.revision = p.buckets[key].revision
	cached.lastAccess = now
}

func (p *RoomTimelineProjection) linkMessageBucketLocked(messageID string, key timelineBucketKey) {
	if messageID == "" || key.roomID == "" {
		return
	}
	p.messageBuckets[messageID] = key
	messages := p.bucketMessages[key]
	if messages == nil {
		messages = make(map[string]struct{})
		p.bucketMessages[key] = messages
	}
	messages[messageID] = struct{}{}
	for _, sequence := range p.pendingBodySequences[messageID] {
		p.addBucketSequenceLocked(key, sequence)
		for cachedKey, cached := range p.cache {
			if cachedKey != key {
				delete(cached.events, sequence)
			}
		}
	}
	delete(p.pendingBodySequences, messageID)
	state, exists := p.bodyStates[messageID]
	if !exists || state.body != nil || state.currentSequence == 0 {
		return
	}
	if cached := p.cache[key]; cached != nil {
		if event := cached.events[state.currentSequence]; event != nil && event.GetMessageBody().GetBody() != nil {
			state.body = event.GetMessageBody().GetBody()
			p.bodyStates[messageID] = state
		}
	}
}

func timelineMutationTarget(event *evtv1.Event) string {
	if event == nil {
		return ""
	}
	switch value := event.GetEvent().(type) {
	case *evtv1.Event_MessageBody:
		return value.MessageBody.GetEventId()
	case *evtv1.Event_MessageEdited:
		return value.MessageEdited.GetEventId()
	case *evtv1.Event_MessageRetracted:
		return value.MessageRetracted.GetEventId()
	case *evtv1.Event_MessagePinned:
		return value.MessagePinned.GetMessageEventId()
	case *evtv1.Event_MessageUnpinned:
		return value.MessageUnpinned.GetMessageEventId()
	default:
		return ""
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

	// Idempotency is envelope-ID based during startup replay. A clean history
	// switches to the monotonic stream-sequence guard once replay completes.
	if p.replayGuard.seenOrMark(event, seq) {
		return nil
	}
	occurrenceBucket := p.bucketKeyLocked(roomID, eventCreatedAt(event))
	p.addBucketSequenceLocked(occurrenceBucket, seq)
	p.cacheAppliedEventLocked(occurrenceBucket, seq, event)
	if targetID := timelineMutationTarget(event); targetID != "" {
		if targetBucket, ok := p.messageBuckets[targetID]; ok {
			p.addBucketSequenceLocked(targetBucket, seq)
			p.cacheAppliedEventLocked(targetBucket, seq, event)
		} else if event.GetMessageBody() != nil {
			p.pendingBodySequences[targetID] = append(p.pendingBodySequences[targetID], seq)
		}
	}
	if !eventMutatesRoomTimelineProjection(event) {
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
					p.clearBodyLocked(targetID)
					p.retractedFlags[targetID] = struct{}{}
					p.setTombstonedAtLocked(targetID, p.shreddedAt[authorID])
					p.removeAttachmentMessageLocked(targetID)
				} else {
					body = cloneMessageBody(body)
					if body.GetBodyEventId() == "" {
						body.BodyEventId = event.GetId()
					}
					p.setCurrentBodyLocked(targetID, body, seq)
					// Retractions are monotonic. Mixed-version replicas or historical
					// replay can present a late body after the tombstone; retain its
					// sequence for secure deletion without making it visible again.
					if _, retracted := p.retractedFlags[targetID]; retracted {
						p.clearBodyLocked(targetID)
						p.removeAttachmentMessageLocked(targetID)
					} else {
						p.refreshAttachmentMessageLocked(roomID, targetID)
					}
				}
			}
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
		p.linkMessageBucketLocked(event.GetId(), occurrenceBucket)
		if entryIdx < 0 {
			entryIdx = p.appendEntryLocked(seq, event)
		}
		p.messagePostsByRoom[roomID] = append(p.messagePostsByRoom[roomID], entryIdx)
		if event.GetMessagePosted().GetEchoOfEventId() == "" && event.GetActorId() != "" {
			p.latestOriginalPostAt[roomActorKey{roomID: roomID, actorID: event.GetActorId()}] = eventCreatedAt(event)
		}
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
	case *evtv1.Event_MessagePosted:
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
		if state, ok := p.bodyStates[targetID]; ok && state.attachmentCount > 0 {
			p.addAttachmentMessageLocked(roomID, targetID, seq)
		}
		// Track echo links so edits on either side can fan out to the
		// other, and so original retractions can be reflected when
		// rendering echoes.
		if origID := ev.MessagePosted.GetEchoOfEventId(); origID != "" && targetID != "" {
			p.echoLinks[origID] = append(p.echoLinks[origID], targetID)
		}
	case *evtv1.Event_MessageRetracted:
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
		if entry == nil || entry.Event == nil {
			continue
		}
		posted := entry.Event.GetMessagePosted()
		if posted == nil {
			continue
		}
		if messageAuthorID(entry.Event) != userID {
			continue
		}
		p.clearBodyLocked(eventID)
		p.retractedFlags[eventID] = struct{}{}
		p.setTombstonedAtLocked(eventID, at)
		p.removeAttachmentMessageLocked(eventID)
	}
}

func (p *RoomTimelineProjection) setCurrentBodyLocked(eventID string, body *evtv1.MessageBody, sequence uint64) {
	state, exists := p.bodyStates[eventID]
	if exists {
		state.supersededSequences = append(state.supersededSequences, state.currentSequence)
		p.removeCachedEventSequenceLocked(state.currentSequence)
	}
	state.currentAssetIDs, state.attachmentCount = messageBodyAttachmentMetadata(body)
	if key, ok := p.messageBuckets[eventID]; p.eventSource == nil || (ok && (p.bucketPinnedLocked(key, p.now()) || p.cache[key] != nil)) {
		state.body = body
		if cached := p.cache[key]; ok && cached != nil {
			if cachedEvent := cached.events[sequence]; cachedEvent != nil && cachedEvent.GetMessageBody().GetBody() != nil {
				cachedBody := cachedEvent.GetMessageBody().GetBody()
				if cachedBody.GetBodyEventId() == "" {
					cachedBody.BodyEventId = body.GetBodyEventId()
				}
				state.body = cachedBody
			}
		}
	} else {
		state.body = nil
	}
	state.currentSequence = sequence
	p.bodyStates[eventID] = state
}

func (p *RoomTimelineProjection) clearBodyLocked(eventID string) {
	state, exists := p.bodyStates[eventID]
	if !exists {
		return
	}
	for _, sequence := range appendBodySequences(nil, state) {
		p.removeCachedEventSequenceLocked(sequence)
	}
	state.body = nil
	state.attachmentCount = 0
	state.currentAssetIDs = nil
	p.bodyStates[eventID] = state
}

func (p *RoomTimelineProjection) removeCachedEventSequenceLocked(sequence uint64) {
	if sequence == 0 {
		return
	}
	for _, cached := range p.cache {
		delete(cached.events, sequence)
	}
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
// newest"). Returns a fresh slice; entries and event payloads are immutable
// and must be treated as read-only by callers.
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

// Get returns a single timeline entry by its envelope id, or
// (nil, false) if no such event has been projected.
func (p *RoomTimelineProjection) Get(eventID string) (*TimelineEntry, bool) {
	p.RLock()
	defer p.RUnlock()
	return p.entryByEventIDLocked(eventID)
}

// LastRoomMessageEntry returns the newest non-hidden MessagePostedEvent in a
// room, including thread replies that are intentionally absent from byRoom.
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

func (p *RoomTimelineProjection) loadBucket(ctx context.Context, key timelineBucketKey) error {
	if p.eventSource == nil {
		return fmt.Errorf("room timeline bucket %q has no EVT source", key.roomID)
	}
	waitStartedAt := time.Now()
	logCanceledWait := func(err error) {
		if p.logger == nil {
			return
		}
		fields := p.bucketLogFields(key)
		fields = append(fields,
			"duration", time.Since(waitStartedAt),
			"error", err,
		)
		p.logger.Debug("Room timeline bucket wait canceled", fields...)
	}
	if err := ctx.Err(); err != nil {
		logCanceledWait(err)
		return err
	}
	loadKey := fmt.Sprintf("%s/%d/%t", key.roomID, key.startUnixNs, key.undated)
	result := p.loads.DoChan(loadKey, func() (_ any, resultErr error) {
		startedAt := time.Now()
		attempts := 0
		sequenceCount := 0
		var revision uint64
		defer func() {
			if resultErr == nil || p.logger == nil {
				return
			}
			fields := p.bucketLogFields(key)
			fields = append(fields,
				"revision", revision,
				"sequence_count", sequenceCount,
				"attempts", attempts,
				"duration", time.Since(startedAt),
				"error", resultErr,
			)
			p.logger.Error("Room timeline bucket reconstruction failed", fields...)
		}()
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		select {
		case p.loadSemaphore <- struct{}{}:
			defer func() { <-p.loadSemaphore }()
		case <-loadCtx.Done():
			return nil, loadCtx.Err()
		}
		eventsBySequence := make(map[uint64]*evtv1.Event)
		fetchedSequences := make(map[uint64]struct{})
		for {
			if err := loadCtx.Err(); err != nil {
				return nil, err
			}
			attempts++
			p.Lock()
			bucket := p.buckets[key]
			if bucket == nil {
				p.Unlock()
				return nil, nil
			}
			if cached := p.cache[key]; cached != nil && cached.complete && cached.revision == bucket.revision {
				cached.lastAccess = p.now()
				p.Unlock()
				return nil, nil
			}
			revision = bucket.revision
			sequences := append([]uint64(nil), bucket.sequences...)
			sequenceCount = len(sequences)
			p.Unlock()
			if p.logger != nil {
				fields := p.bucketLogFields(key)
				fields = append(fields,
					"revision", revision,
					"sequence_count", sequenceCount,
					"attempt", attempts,
				)
				p.logger.Debug("Room timeline bucket reconstruction started", fields...)
			}

			for _, sequence := range sequences {
				if _, fetched := fetchedSequences[sequence]; fetched {
					continue
				}
				fetchedSequences[sequence] = struct{}{}
				record, readErr := p.eventSource.EventAt(loadCtx, sequence)
				if errors.Is(readErr, jetstream.ErrMsgNotFound) {
					// Secure deletion can remove obsolete private body records.
					continue
				}
				if readErr != nil {
					return nil, fmt.Errorf("read EVT sequence %d for room timeline bucket: %w", sequence, readErr)
				}
				if record == nil || record.Event == nil || record.Sequence != sequence || roomIDOfEvent(record.Event) != key.roomID {
					return nil, fmt.Errorf("EVT sequence %d does not match room timeline bucket %q", sequence, key.roomID)
				}
				cachedEvent := proto.Clone(record.Event).(*evtv1.Event)
				if bodyEvent := cachedEvent.GetMessageBody(); bodyEvent != nil && bodyEvent.GetBody() != nil && bodyEvent.GetBody().GetBodyEventId() == "" {
					bodyEvent.GetBody().BodyEventId = cachedEvent.GetId()
				}
				eventsBySequence[sequence] = cachedEvent
			}

			p.Lock()
			current := p.buckets[key]
			if current == nil || current.revision != revision {
				p.Unlock()
				continue
			}
			for sequence, event := range eventsBySequence {
				bodyEvent := event.GetMessageBody()
				if bodyEvent == nil {
					continue
				}
				state := p.bodyStates[bodyEvent.GetEventId()]
				if state.currentSequence != sequence || p.messageBuckets[bodyEvent.GetEventId()] != key || p.messageBodyUnavailableLocked(bodyEvent.GetEventId()) {
					delete(eventsBySequence, sequence)
				}
			}
			for messageID := range p.bucketMessages[key] {
				state := p.bodyStates[messageID]
				if state.currentSequence == 0 || p.messageBodyUnavailableLocked(messageID) {
					continue
				}
				bodyEvent := eventsBySequence[state.currentSequence].GetMessageBody()
				if bodyEvent == nil || bodyEvent.GetBody() == nil {
					p.Unlock()
					return nil, fmt.Errorf("room timeline bucket is missing current body sequence %d for message %q", state.currentSequence, messageID)
				}
			}
			now := p.now()
			pinned := p.bucketPinnedLocked(key, now)
			p.cache[key] = &timelineBucketCache{
				events: eventsBySequence, revision: revision, lastAccess: now,
				pinned: pinned, complete: true,
			}
			for messageID := range p.bucketMessages[key] {
				state := p.bodyStates[messageID]
				event := eventsBySequence[state.currentSequence]
				if bodyEvent := event.GetMessageBody(); bodyEvent != nil && bodyEvent.GetBody() != nil {
					body := bodyEvent.GetBody()
					_, retracted := p.retractedFlags[messageID]
					_, hidden := p.hiddenEchoes[messageID]
					_, authorShredded := p.shreddedUsers[body.GetAuthorId()]
					if originalID := p.echoOriginalIDLocked(messageID); originalID != "" {
						_, originalRetracted := p.retractedFlags[originalID]
						retracted = retracted || originalRetracted
					}
					if retracted || hidden || authorShredded {
						continue
					}
					state.body = body
					p.bodyStates[messageID] = state
				}
			}
			messageCount := len(p.bucketMessages[key])
			p.Unlock()
			if p.logger != nil {
				fields := p.bucketLogFields(key)
				fields = append(fields,
					"revision", revision,
					"sequence_count", sequenceCount,
					"event_count", len(eventsBySequence),
					"message_count", messageCount,
					"attempts", attempts,
					"pinned", pinned,
					"duration", time.Since(startedAt),
				)
				p.logger.Debug("Room timeline bucket reconstructed", fields...)
			}
			return nil, nil
		}
	})
	select {
	case completed := <-result:
		return completed.Err
	case <-ctx.Done():
		logCanceledWait(ctx.Err())
		return ctx.Err()
	}
}

func (p *RoomTimelineProjection) messageBodyUnavailableLocked(messageID string) bool {
	if _, retracted := p.retractedFlags[messageID]; retracted {
		return true
	}
	if _, hidden := p.hiddenEchoes[messageID]; hidden {
		return true
	}
	if originalID := p.echoOriginalIDLocked(messageID); originalID != "" {
		if _, retracted := p.retractedFlags[originalID]; retracted {
			return true
		}
	}
	entry, _ := p.entryByEventIDLocked(messageID)
	if entry != nil && entry.Event != nil {
		_, shredded := p.shreddedUsers[messageAuthorID(entry.Event)]
		return shredded
	}
	return false
}

// WarmPinned reconstructs all current and recent pinned buckets. Startup calls
// it before the core reports readiness after a snapshot restore.
func (p *RoomTimelineProjection) WarmPinned(ctx context.Context) error {
	p.RLock()
	now := p.now()
	keys := make([]timelineBucketKey, 0, len(p.buckets))
	for key := range p.buckets {
		if p.bucketPinnedLocked(key, now) {
			keys = append(keys, key)
		}
	}
	p.RUnlock()
	for _, key := range keys {
		if err := p.loadBucket(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// RunBucketCache evicts unpinned materializations after their idle timeout.
func (p *RoomTimelineProjection) RunBucketCache(ctx context.Context) error {
	interval := p.idleTimeout / 2
	if interval > time.Minute {
		interval = time.Minute
	}
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			p.evictIdleBuckets(now)
		}
	}
}

func (p *RoomTimelineProjection) evictIdleBuckets(now time.Time) {
	type eviction struct {
		key          timelineBucketKey
		revision     uint64
		eventCount   int
		messageCount int
		idleFor      time.Duration
	}
	var evictions []eviction
	p.Lock()
	for key, cached := range p.cache {
		if p.bucketPinnedLocked(key, now) {
			cached.pinned = true
			continue
		}
		if cached.pinned {
			cached.pinned = false
			cached.lastAccess = now
			continue
		}
		if now.Sub(cached.lastAccess) < p.idleTimeout {
			continue
		}
		evictions = append(evictions, eviction{
			key:          key,
			revision:     cached.revision,
			eventCount:   len(cached.events),
			messageCount: len(p.bucketMessages[key]),
			idleFor:      now.Sub(cached.lastAccess),
		})
		delete(p.cache, key)
		for messageID := range p.bucketMessages[key] {
			state := p.bodyStates[messageID]
			state.body = nil
			p.bodyStates[messageID] = state
		}
	}
	remainingBuckets := len(p.cache)
	p.Unlock()
	if p.logger == nil {
		return
	}
	for _, evicted := range evictions {
		fields := p.bucketLogFields(evicted.key)
		fields = append(fields,
			"revision", evicted.revision,
			"event_count", evicted.eventCount,
			"message_count", evicted.messageCount,
			"idle_for", evicted.idleFor,
			"materialized_buckets_remaining", remainingBuckets,
		)
		p.logger.Debug("Room timeline bucket evicted", fields...)
	}
}

// LatestBodyContext loads the owning bucket when its materialization was
// evicted, then returns the current body state.
func (p *RoomTimelineProjection) LatestBodyContext(ctx context.Context, eventID string) (*evtv1.MessageBody, bool, bool, error) {
	p.Lock()
	key, hasBucket := p.messageBuckets[eventID]
	state := p.bodyStates[eventID]
	body, isRetracted, ok := p.latestBodyLocked(eventID)
	if body != nil && hasBucket {
		if cached := p.cache[key]; cached != nil {
			cached.lastAccess = p.now()
		}
	}
	p.Unlock()
	if body == nil && !isRetracted && state.currentSequence != 0 && hasBucket {
		if err := p.loadBucket(ctx, key); err != nil {
			return nil, false, true, err
		}
		p.Lock()
		body, isRetracted, ok = p.latestBodyLocked(eventID)
		if body != nil {
			if cached := p.cache[key]; cached != nil {
				cached.lastAccess = p.now()
			}
		}
		p.Unlock()
	}
	return body, isRetracted, ok, nil
}

// LatestBody returns the current MessageBodyEvent body for a message, or nil +
// retracted=true if a MessageRetractedEvent has landed.
//
// Returns (nil, false, false) if the event_id isn't known to the
// projection (caller can treat as "not found yet").
//
// O(1): consults the derived bodyStates / retractedFlags indexes
// that Apply keeps in lockstep with byRoom.
func (p *RoomTimelineProjection) LatestBody(eventID string) (body *evtv1.MessageBody, retracted bool, ok bool) {
	p.RLock()
	defer p.RUnlock()
	return p.latestBodyLocked(eventID)
}

func (p *RoomTimelineProjection) latestBodyLocked(eventID string) (body *evtv1.MessageBody, retracted bool, ok bool) {
	if eventID == "" {
		return nil, false, false
	}
	if _, exists := p.byEventID[eventID]; !exists {
		return nil, false, false
	}
	if _, hidden := p.hiddenEchoes[eventID]; hidden {
		return nil, true, true
	}
	if _, isRetracted := p.retractedFlags[eventID]; isRetracted {
		return nil, true, true
	}
	if origID := p.echoOriginalIDLocked(eventID); origID != "" {
		if _, originalRetracted := p.retractedFlags[origID]; originalRetracted {
			return nil, true, true
		}
	}
	if state, has := p.bodyStates[eventID]; has && state.body != nil {
		return cloneMessageBody(state.body), false, true
	}
	return nil, false, true
}

// CurrentRoomAttachmentMessageReferences returns current, visible messages
// whose latest bodies reference attachments. Results are newest message first.
// The references contain no message-body payload and do not load timeline
// buckets.
func (p *RoomTimelineProjection) CurrentRoomAttachmentMessageReferences(roomID string) []projectedRoomAttachmentMessageReference {
	p.RLock()
	defer p.RUnlock()

	ids := p.attachmentMessageIDsByRoom[roomID]
	if len(ids) == 0 {
		return nil
	}

	out := make([]projectedRoomAttachmentMessageReference, 0, len(ids))
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
		state := p.bodyStates[eventID]
		if state.attachmentCount <= 0 {
			continue
		}
		out = append(out, projectedRoomAttachmentMessageReference{
			Entry:           entry,
			AttachmentCount: state.attachmentCount,
			AssetIDs:        append([]string(nil), state.currentAssetIDs...),
		})
	}
	return out
}

func (p *RoomTimelineProjection) refreshAttachmentMessageLocked(roomID, eventID string) {
	if roomID == "" || eventID == "" {
		return
	}
	if p.bodyStates[eventID].attachmentCount <= 0 {
		p.removeAttachmentMessageLocked(eventID)
		return
	}
	entry, _ := p.entryByEventIDLocked(eventID)
	if entry == nil || entry.Event == nil || p.isHiddenEchoEntryLocked(entry) {
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

func messageBodyAttachmentMetadata(body *evtv1.MessageBody) ([]string, int) {
	if body == nil {
		return nil, 0
	}
	if referenced := body.GetAssetIds(); len(referenced) > 0 {
		assetIDs := make([]string, 0, len(referenced))
		for _, assetID := range referenced {
			if assetID != "" {
				assetIDs = append(assetIDs, assetID)
			}
		}
		return assetIDs, len(assetIDs)
	}
	count := 0
	for _, attachment := range body.GetAttachments() {
		if attachment != nil && attachment.GetId() != "" {
			count++
		}
	}
	return nil, count
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
	if !ok || entry == nil || entry.Event == nil {
		return ""
	}
	posted := entry.Event.GetMessagePosted()
	if posted == nil {
		return ""
	}
	return posted.GetEchoOfEventId()
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
		roomID = roomIDOfEvent(entry.Event)
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

func cloneMessageBody(body *evtv1.MessageBody) *evtv1.MessageBody {
	if body == nil {
		return nil
	}
	return proto.Clone(body).(*evtv1.MessageBody)
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
		if posted := entry.Event.GetMessagePosted(); posted != nil {
			if origID := posted.GetEchoOfEventId(); origID != "" && origID != eventID {
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
	visible func(*evtv1.Event) bool,
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
		if visible != nil && !visible(e.Event) {
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
	visible func(*evtv1.Event) bool,
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
		if visible != nil && !visible(e.Event) {
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
	visible func(*evtv1.Event) bool,
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
		if visible != nil && !visible(e.Event) {
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
	visibility ...func(*evtv1.Event) bool,
) (entries []*TimelineEntry, targetIndex int, hasOlder bool, hasNewer bool, ok bool) {
	if limit <= 0 || eventID == "" {
		return nil, 0, false, false, false
	}
	p.RLock()
	defer p.RUnlock()
	var visible func(*evtv1.Event) bool
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
		if visible != nil && (entry == nil || !visible(entry.Event)) {
			continue
		}
		if entry != nil && entry.Event != nil && entry.Event.GetId() == eventID {
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
		if visible != nil && (entry == nil || !visible(entry.Event)) {
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
	if entry == nil || entry.Event == nil {
		return false
	}
	_, hidden := p.hiddenEchoes[entry.Event.GetId()]
	return hidden
}
