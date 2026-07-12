package core

import (
	"time"

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
	// latestBody is the derived current-body index. Updated as
	// MessageEdited / MessageRetracted entries are applied so that
	// LatestBody resolves in O(1) instead of an O(room size) walk
	// of byRoom. A nil entry means "retracted"; absent means "no
	// body payload / not yet projected".
	latestBody     map[string]*corev1.MessageBody
	bodyEventSeqs  map[string][]uint64
	currentBodySeq map[string]uint64
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
	hiddenEchoes map[string]struct{}
	// These asset indexes are a compatibility bridge for 0.1.0 beta histories
	// that wrote asset lifecycle events under evt.room.* before assets moved to
	// evt.asset.*. New runtime reads should use AssetProjection; RoomTimeline
	// keeps just enough legacy asset state to route old room-scoped asset events
	// during replay.
	assets        *roomTimelineAssetIndex
	shreddedUsers map[string]struct{}
}

// TimelineEntry is one event's position in a room timeline. Carries
// the full immutable event proto verbatim — payload, envelope, actor,
// created_at, oneof variant — so resolvers don't need to consult
// the projection's internal state to render.
type TimelineEntry struct {
	StreamSeq uint64
	Event     *corev1.Event
}

type projectedRoomAttachmentMessage struct {
	Entry *TimelineEntry
	Body  *corev1.MessageBody
}

func (p *RoomTimelineProjection) appendEntryLocked(seq uint64, event *corev1.Event) int {
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
	return &RoomTimelineProjection{
		byRoom:                     make(map[string][]int),
		byEventID:                  make(map[string]int),
		messagePostsByRoom:         make(map[string][]int),
		replayGuard:                newProjectionReplayGuard(),
		latestBody:                 make(map[string]*corev1.MessageBody),
		bodyEventSeqs:              make(map[string][]uint64),
		currentBodySeq:             make(map[string]uint64),
		retractedFlags:             make(map[string]struct{}),
		tombstonedAt:               make(map[string]time.Time),
		shreddedAt:                 make(map[string]time.Time),
		attachmentMessageIDsByRoom: make(map[string][]string),
		attachmentMessageRoom:      make(map[string]string),
		echoLinks:                  make(map[string][]string),
		hiddenEchoes:               make(map[string]struct{}),
		assets:                     newRoomTimelineAssetIndex(),
		shreddedUsers:              make(map[string]struct{}),
	}
}

// Subjects implements events.Projection. The projection owns the
// "everything that happened in this room" surface, so it subscribes to the
// room aggregate namespace plus the extra user key-shred events it needs.
func (p *RoomTimelineProjection) Subjects() []string {
	return []string{events.RoomSubjectFilter(), events.UserEventTypeFilter(events.EventUserKeyShredded)}
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

	roomID := p.roomIDOfEventLocked(event)
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
					delete(p.latestBody, targetID)
					p.retractedFlags[targetID] = struct{}{}
					p.setTombstonedAtLocked(targetID, p.shreddedAt[authorID])
					p.removeAttachmentMessageLocked(targetID)
				} else {
					body = cloneMessageBody(body)
					if body.GetBodyEventId() == "" {
						body.BodyEventId = event.GetId()
					}
					p.latestBody[targetID] = body
					p.bodyEventSeqs[targetID] = append(p.bodyEventSeqs[targetID], seq)
					p.currentBodySeq[targetID] = seq
					delete(p.retractedFlags, targetID)
					p.refreshAttachmentMessageLocked(roomID, targetID, body)
				}
			}
			p.assets.rememberMessageBodyAssets(roomID, targetID, body)
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
				delete(p.latestBody, targetID)
				p.retractedFlags[targetID] = struct{}{}
				p.setTombstonedAtLocked(targetID, p.shreddedAt[authorID])
				p.removeAttachmentMessageLocked(targetID)
			}
		}
		if body := p.latestBody[targetID]; body != nil {
			p.refreshAttachmentMessageLocked(roomID, targetID, body)
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
					delete(p.latestBody, targetID)
					p.hiddenEchoes[targetID] = struct{}{}
					p.removeAttachmentMessageLocked(targetID)
					return nil
				}
			}
			delete(p.latestBody, targetID)
			p.retractedFlags[targetID] = struct{}{}
			p.removeAttachmentMessageLocked(targetID)
		}
	}
	p.assets.applyLifecycleEvent(event)
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
	if isAssetLifecycleEvent(event) {
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
		delete(p.latestBody, eventID)
		p.retractedFlags[eventID] = struct{}{}
		p.setTombstonedAtLocked(eventID, at)
		p.removeAttachmentMessageLocked(eventID)
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

func (p *RoomTimelineProjection) roomIDOfEventLocked(event *corev1.Event) string {
	if isAssetLifecycleEvent(event) {
		return p.assets.roomIDOfLifecycleEvent(event)
	}
	return roomIDOfEvent(event)
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

// LatestBody returns the current MessageBodyEvent body for a message, or nil +
// retracted=true if a MessageRetractedEvent has landed.
//
// Returns (nil, false, false) if the event_id isn't known to the
// projection (caller can treat as "not found yet").
//
// O(1): consults the derived latestBody / retractedFlags indexes
// that Apply keeps in lockstep with byRoom.
func (p *RoomTimelineProjection) LatestBody(eventID string) (body *corev1.MessageBody, retracted bool, ok bool) {
	p.RLock()
	defer p.RUnlock()
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
	if b, has := p.latestBody[eventID]; has {
		return cloneMessageBody(b), false, true
	}
	return nil, false, true
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
		body := p.latestBody[eventID]
		if !messageBodyReferencesAttachments(body) {
			continue
		}
		out = append(out, projectedRoomAttachmentMessage{
			Entry: entry,
			Body:  cloneMessageBody(body),
		})
	}
	return out
}

func (p *RoomTimelineProjection) refreshAttachmentMessageLocked(roomID, eventID string, body *corev1.MessageBody) {
	if roomID == "" || eventID == "" {
		return
	}
	if !messageBodyReferencesAttachments(body) {
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
	seqs = append([]uint64(nil), p.bodyEventSeqs[eventID]...)
	return seqs, p.currentBodySeq[eventID], true
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
	all := p.bodyEventSeqs[eventID]
	if len(all) == 0 {
		return nil
	}
	if _, retracted := p.retractedFlags[eventID]; retracted {
		return append([]uint64(nil), all...)
	}
	if _, hidden := p.hiddenEchoes[eventID]; hidden {
		return append([]uint64(nil), all...)
	}
	current := p.currentBodySeq[eventID]
	out := make([]uint64, 0, len(all))
	for _, seq := range all {
		if seq != current {
			out = append(out, seq)
		}
	}
	return out
}

// AllObsoleteBodyEventSeqs returns every projected MessageBodyEvent seq
// whose payload is no longer needed for the current message state.
func (p *RoomTimelineProjection) AllObsoleteBodyEventSeqs() []uint64 {
	p.RLock()
	defer p.RUnlock()
	var out []uint64
	for eventID, all := range p.bodyEventSeqs {
		if len(all) == 0 {
			continue
		}
		if _, retracted := p.retractedFlags[eventID]; retracted {
			out = append(out, all...)
			continue
		}
		if _, hidden := p.hiddenEchoes[eventID]; hidden {
			out = append(out, all...)
			continue
		}
		current := p.currentBodySeq[eventID]
		for _, seq := range all {
			if seq != current {
				out = append(out, seq)
			}
		}
	}
	return out
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

// VideoAttachmentManifest returns the latest durable processing outcome for
// the original video attachment ID, if one has been projected. The returned
// protos are clones so callers can inspect or adapt them freely.
func (p *RoomTimelineProjection) VideoAttachmentManifest(attachmentID string) (*VideoAttachmentManifest, bool) {
	p.RLock()
	defer p.RUnlock()
	return p.assets.videoAttachmentManifest(attachmentID)
}

// AssetCreation returns the durable creation event for an asset.
func (p *RoomTimelineProjection) AssetCreation(attachmentID string) (*corev1.AssetCreatedEvent, bool) {
	p.RLock()
	defer p.RUnlock()
	return p.assets.assetCreation(attachmentID)
}

// AssetRoomID returns the room that owns an asset. For derivatives, it walks up
// the parent chain when needed so callers can authorize thumbnail and variant
// assets using the original room scope.
func (p *RoomTimelineProjection) AssetRoomID(assetID string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	return p.assets.assetRoomID(assetID)
}

// AssetMessageOwner returns the room and message that own an asset, derived
// from the MessagePostedEvent that referenced it. Reports ok=false when no
// projected message has claimed the asset yet (e.g. an upload that was never
// posted, or whose message hasn't been projected). The deprecated
// AssetCreatedEvent.message_event_id is not consulted — new uploads never
// set it.
func (p *RoomTimelineProjection) AssetMessageOwner(assetID string) (roomID, messageEventID string, ok bool) {
	p.RLock()
	defer p.RUnlock()
	return p.assets.assetMessageOwner(assetID)
}

func (p *RoomTimelineProjection) MessageAssetsByAuthor(userID string) []MessageAssetRef {
	p.RLock()
	defer p.RUnlock()
	return p.assets.messageAssetsByAuthor(userID, p.entryByEventIDLocked)
}

func (p *RoomTimelineProjection) MessageAssetOwners() []MessageAssetRef {
	p.RLock()
	defer p.RUnlock()
	return p.assets.messageAssetOwners()
}

func (p *RoomTimelineProjection) AssetSubtreeIDs(assetID string) []string {
	p.RLock()
	defer p.RUnlock()
	return p.assets.assetSubtreeIDs(assetID)
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

// UnmanifestedVideoAttachments returns message-owned video/GIF assets that
// do not yet have a durable processed/failed manifest. Ownership comes from
// the posting message (assetMessageOwner), not the deprecated
// AssetCreatedEvent.message_event_id, which new uploads never set.
func (p *RoomTimelineProjection) UnmanifestedVideoAttachments() []VideoProcessingRequest {
	p.RLock()
	defer p.RUnlock()
	return p.assets.unmanifestedVideoAttachments(p.retractedFlags)
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
	visible func(*corev1.Event) bool,
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
	visible func(*corev1.Event) bool,
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
	visible func(*corev1.Event) bool,
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
) (entries []*TimelineEntry, targetIndex int, hasOlder bool, hasNewer bool, ok bool) {
	if limit <= 0 || eventID == "" {
		return nil, 0, false, false, false
	}
	p.RLock()
	defer p.RUnlock()
	roomEntries := p.byRoom[roomID]
	targetVisibleIndex := -1
	visibleCount := 0
	for _, idx := range roomEntries {
		entry := p.entryAtLocked(idx)
		if p.isHiddenEchoEntryLocked(entry) {
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
