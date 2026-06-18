package core

import (
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomTimelineProjection holds an append-only event log per room.
//
// It consumes the full evt.room.> firehose — every event under any
// room aggregate (room lifecycle, memberships, messages, edits,
// retracts) lands in the owning room's slice in stream order. This
// is the v1 shape for the messages migration (issue #597): dead
// simple, no fold logic, no in-place mutation. Resolvers walk the
// slice and decide how to render — fold edits onto their original
// post, mark retracted entries, merge meta + message events, filter
// thread replies out of the channel view, etc.
//
// We will iterate on this significantly (RAM-bounded windows,
// derived caches for current-state lookups, etc.) once the read
// patterns are observed against real data. For now: one slice per
// room, full *corev1.Event protos preserved, every event indexed by
// envelope id for direct lookup.
type RoomTimelineProjection struct {
	events.MemoryProjection
	byRoom          map[string][]*TimelineEntry
	visibleByRoom   map[string][]*TimelineEntry
	byEventID       map[string]*TimelineEntry
	appliedEventIDs eventIDSet
	// latestBody is the derived current-body index. Updated as
	// MessageEdited / MessageRetracted entries are applied so that
	// LatestBody resolves in O(1) instead of an O(room size) walk
	// of byRoom. A nil entry means "retracted"; absent means "no
	// body payload / not yet projected".
	latestBody     map[string]*corev1.MessageBody
	bodyEventSeqs  map[string][]uint64
	currentBodySeq map[string]uint64
	retractedFlags map[string]struct{}
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

// NewRoomTimelineProjection returns an empty projection.
func NewRoomTimelineProjection() *RoomTimelineProjection {
	return &RoomTimelineProjection{
		byRoom:                     make(map[string][]*TimelineEntry),
		visibleByRoom:              make(map[string][]*TimelineEntry),
		byEventID:                  make(map[string]*TimelineEntry),
		appliedEventIDs:            newEventIDSet(),
		latestBody:                 make(map[string]*corev1.MessageBody),
		bodyEventSeqs:              make(map[string][]uint64),
		currentBodySeq:             make(map[string]uint64),
		retractedFlags:             make(map[string]struct{}),
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

// Apply implements events.Projection. Extracts the room_id from
// whichever room-scoped event variant we recognise and appends an
// entry to that room's slice. Events that don't carry a room_id
// (shouldn't appear on evt.room.>, but defensive) are silently
// skipped — projections forward-compat by ignoring what they don't
// understand.
func (p *RoomTimelineProjection) Apply(event *corev1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	if shredded := event.GetUserKeyShredded(); shredded != nil {
		p.applyUserKeyShreddedLocked(shredded.GetUserId())
		return nil
	}

	roomID := p.roomIDOfEventLocked(event)
	if roomID == "" {
		return nil
	}

	// Idempotency: a re-applied event with the same envelope id is a
	// no-op. The Projection.Apply contract is "Apply(e,n) twice ==
	// Apply(e,n) once"; this is how we honour it.
	if p.appliedEventIDs.seenOrMark(event) {
		return nil
	}

	if ev := event.GetMessageBody(); ev != nil {
		targetID := ev.GetEventId()
		body := cloneMessageBody(ev.GetBody())
		if targetID != "" && body != nil {
			if body.GetBodyEventId() == "" {
				body.BodyEventId = event.GetId()
			} else if body.GetBodyEventId() != event.GetId() {
				return nil
			}
			if authorID := body.GetAuthorId(); authorID != "" {
				if _, shredded := p.shreddedUsers[authorID]; shredded {
					delete(p.latestBody, targetID)
					p.retractedFlags[targetID] = struct{}{}
					p.removeAttachmentMessageLocked(targetID)
				} else {
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

	entry := &TimelineEntry{StreamSeq: seq, Event: event}
	p.byRoom[roomID] = append(p.byRoom[roomID], entry)
	if isVisibleRoomTimelineEntry(event) {
		p.visibleByRoom[roomID] = append(p.visibleByRoom[roomID], entry)
	}
	if eid := event.GetId(); eid != "" {
		p.byEventID[eid] = entry
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

func (p *RoomTimelineProjection) applyUserKeyShreddedLocked(userID string) {
	if userID == "" {
		return
	}
	p.shreddedUsers[userID] = struct{}{}
	for eventID, entry := range p.byEventID {
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
		p.removeAttachmentMessageLocked(eventID)
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
// Entries are the raw timeline — no filtering of meta vs message vs
// thread reply, no fold of edits, no tombstone hiding. Resolvers
// pick what to surface.
func (p *RoomTimelineProjection) RoomEvents(roomID string, limit int, beforeStreamSeq uint64) []*TimelineEntry {
	if limit <= 0 {
		return nil
	}
	p.RLock()
	defer p.RUnlock()
	entries := p.byRoom[roomID]
	if len(entries) == 0 {
		return nil
	}
	out := make([]*TimelineEntry, 0, limit)
	for i := len(entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := entries[i]
		if beforeStreamSeq > 0 && e.StreamSeq >= beforeStreamSeq {
			continue
		}
		out = append(out, e)
	}
	return out
}

// RoomEventCount returns the total number of timeline entries in the
// room. Used by the future resolver's small-room fast-path
// equivalent.
func (p *RoomTimelineProjection) RoomEventCount(roomID string) int {
	p.RLock()
	defer p.RUnlock()
	return len(p.byRoom[roomID])
}

// VisibleRoomEventCount returns the total number of room-visible timeline
// entries in the room. Hidden echoes may still be present in the derived slice
// and are excluded by the visible timeline readers.
func (p *RoomTimelineProjection) VisibleRoomEventCount(roomID string) int {
	p.RLock()
	defer p.RUnlock()
	n := 0
	for _, entry := range p.visibleByRoom[roomID] {
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
		for _, entry := range roomEntries {
			if entry != nil && entry.Event.GetMessagePosted() != nil {
				messagePosts++
			}
		}
	}
	return rooms, entries, messagePosts
}

// Get returns a single timeline entry by its envelope id, or
// (nil, false) if no such event has been projected.
func (p *RoomTimelineProjection) Get(eventID string) (*TimelineEntry, bool) {
	p.RLock()
	defer p.RUnlock()
	e, ok := p.byEventID[eventID]
	return e, ok
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
		entry := p.byEventID[eventID]
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
	entry := p.byEventID[eventID]
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
	for i, existingID := range ids {
		existing := p.byEventID[existingID]
		if existing == nil || existing.StreamSeq > streamSeq {
			insertAt = i
			break
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
	entry := p.byEventID[eventID]
	if entry == nil || entry.Event == nil {
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
		if _, ok := p.byEventID[echoID]; !ok {
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
	return p.assets.messageAssetsByAuthor(userID, p.byEventID)
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
	if entry, ok := p.byEventID[eventID]; ok {
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
	entries := p.byRoom[roomID]
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
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

// VisibleRoomTimeline walks the room's timeline newest-first,
// applying `visible` as a per-entry filter, and returns up to
// `limit` matching entries. `beforeStreamSeq > 0` excludes entries
// with stream seq >= that value (exclusive upper bound for
// pagination).
//
// Stops as soon as `limit` visible entries are accumulated — no
// full-slice materialisation. Caller may inspect more than `limit`
// raw entries when the visibility filter rejects some of them
// (e.g. when filtering thread replies out of a channel timeline).
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
	entries := p.byRoom[roomID]
	if visible == nil {
		entries = p.visibleByRoom[roomID]
	}
	out := make([]*TimelineEntry, 0, limit)
	for i := len(entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := entries[i]
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
	entries := p.byRoom[roomID]
	if visible == nil {
		entries = p.visibleByRoom[roomID]
	}
	out := make([]*TimelineEntry, 0, limit)
	for _, e := range entries {
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
// in oldest-first order. It walks the derived visible-room slice instead of the
// raw room log, so edits/reactions/assets/thread replies are not revisited when
// serving "jump to message" style reads.
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
	roomEntries := p.visibleByRoom[roomID]
	targetVisibleIndex := -1
	visibleCount := 0
	for _, entry := range roomEntries {
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
	for _, entry := range roomEntries {
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
