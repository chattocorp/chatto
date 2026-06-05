package core

import (
	"strings"

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
	appliedEventIDs map[string]struct{}
	// latestBody is the derived current-body index. Updated as
	// MessageEdited / MessageRetracted entries are applied so that
	// LatestBody resolves in O(1) instead of an O(room size) walk
	// of byRoom. A nil entry means "retracted"; absent means "no
	// body payload / not yet projected".
	latestBody     map[string]*corev1.MessageBody
	bodyEventSeqs  map[string][]uint64
	currentBodySeq map[string]uint64
	retractedFlags map[string]struct{}
	// echoLinks maps an original message's event_id to the event_ids
	// of any echoes pointing at it. Maintained as MessagePostedEvents
	// with EchoOfEventId arrive. Used by EditMessage / DeleteMessage
	// to fan mutations across linked messages — pre-cutover the
	// echo + original shared a messageBodyId, so an edit on either
	// updated both via the shared SERVER_BODIES entry; post-cutover
	// each has its own projected body payload and we need explicit
	// propagation.
	echoLinks map[string][]string
	// hiddenEchoes tracks echo MessagePostedEvents that were directly
	// retracted. A direct echo retract removes the room-timeline copy
	// without deleting the original thread reply's content.
	hiddenEchoes map[string]struct{}
	// videoManifests stores the latest durable processing outcome for each
	// original video attachment. A processed event supersedes a failed event
	// and vice versa; generated asset metadata lives in the event payload.
	assetCreations map[string]*corev1.AssetCreatedEvent
	assetChildren  map[string][]string
	videoManifests map[string]*VideoAttachmentManifest
	// assetMessageOwner maps an asset ID to the message that references it,
	// derived from MessagePostedEvent bodies. Upload-time AssetCreatedEvents
	// don't carry message linkage — the message doesn't exist yet — so the
	// deprecated AssetCreatedEvent.message_event_id is never set on new
	// uploads. Message ownership is reconstructed here from the posting
	// message's asset_ids (or legacy embedded attachments).
	assetMessageOwner map[string]assetMessageRef
	shreddedUsers     map[string]struct{}
}

// assetMessageRef is the room + message that owns an asset, captured from
// the MessagePostedEvent that references it.
type assetMessageRef struct {
	roomID         string
	messageEventID string
}

type MessageAssetRef struct {
	RoomID         string
	MessageEventID string
	AssetID        string
}

// VideoAttachmentManifest is the projection's current processing state for
// one original video attachment. Started fires when processing is enqueued;
// Succeeded or Failed fires on terminal outcome. A Started event for a
// previously-finalised asset clears the prior terminal state (treated as a
// retry); a Succeeded/Failed event clears the opposite terminal.
type VideoAttachmentManifest struct {
	Started   *corev1.AssetProcessingStartedEvent
	Succeeded *corev1.AssetProcessingSucceededEvent
	Failed    *corev1.AssetProcessingFailedEvent
}

// VideoProcessingRequest describes an original video/GIF attachment embedded
// in a durable MessagePostedEvent that does not yet have a projected manifest.
type VideoProcessingRequest struct {
	RoomID         string
	MessageEventID string
	Attachment     *corev1.Attachment
}

// TimelineEntry is one event's position in a room timeline. Carries
// the full event proto verbatim — payload, envelope, actor,
// created_at, oneof variant — so resolvers don't need to consult
// the projection's internal state to render.
type TimelineEntry struct {
	StreamSeq uint64
	Event     *corev1.Event
}

// NewRoomTimelineProjection returns an empty projection.
func NewRoomTimelineProjection() *RoomTimelineProjection {
	return &RoomTimelineProjection{
		byRoom:            make(map[string][]*TimelineEntry),
		visibleByRoom:     make(map[string][]*TimelineEntry),
		byEventID:         make(map[string]*TimelineEntry),
		appliedEventIDs:   make(map[string]struct{}),
		latestBody:        make(map[string]*corev1.MessageBody),
		bodyEventSeqs:     make(map[string][]uint64),
		currentBodySeq:    make(map[string]uint64),
		retractedFlags:    make(map[string]struct{}),
		echoLinks:         make(map[string][]string),
		hiddenEchoes:      make(map[string]struct{}),
		assetCreations:    make(map[string]*corev1.AssetCreatedEvent),
		assetChildren:     make(map[string][]string),
		videoManifests:    make(map[string]*VideoAttachmentManifest),
		assetMessageOwner: make(map[string]assetMessageRef),
		shreddedUsers:     make(map[string]struct{}),
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
	if eid := event.GetId(); eid != "" {
		if _, exists := p.appliedEventIDs[eid]; exists {
			return nil
		}
		p.appliedEventIDs[eid] = struct{}{}
	}

	if ev := event.GetMessageBody(); ev != nil {
		targetID := ev.GetEventId()
		body := ev.GetBody()
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
				} else {
					p.latestBody[targetID] = body
					p.bodyEventSeqs[targetID] = append(p.bodyEventSeqs[targetID], seq)
					p.currentBodySeq[targetID] = seq
					delete(p.retractedFlags, targetID)
				}
			}
			for _, assetID := range ownedAssetIDsFromBody(body) {
				if assetID == "" {
					continue
				}
				if _, exists := p.assetMessageOwner[assetID]; exists {
					continue
				}
				p.assetMessageOwner[assetID] = assetMessageRef{roomID: roomID, messageEventID: targetID}
			}
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
			authorID := messageAuthorID(event, ev.MessagePosted)
			if _, shredded := p.shreddedUsers[authorID]; shredded {
				delete(p.latestBody, targetID)
				p.retractedFlags[targetID] = struct{}{}
			} else if ev.MessagePosted.GetBody() != nil {
				p.latestBody[targetID] = ev.MessagePosted.GetBody()
				delete(p.retractedFlags, targetID)
			}
			// Record message ownership of any referenced assets. This is the
			// source of truth for "which message owns this asset" — the
			// upload-time AssetCreatedEvent can't carry it (no message yet).
			for _, assetID := range ownedAssetIDsFromBody(ev.MessagePosted.GetBody()) {
				if assetID == "" {
					continue
				}
				if _, exists := p.assetMessageOwner[assetID]; exists {
					continue
				}
				p.assetMessageOwner[assetID] = assetMessageRef{roomID: roomID, messageEventID: targetID}
			}
		}
		// Track echo links so edits on either side can fan out to the
		// other, and so original retractions can be reflected when
		// rendering echoes.
		if origID := ev.MessagePosted.GetEchoOfEventId(); origID != "" && targetID != "" {
			p.echoLinks[origID] = append(p.echoLinks[origID], targetID)
		}
	case *corev1.Event_MessageEdited:
		targetID := ev.MessageEdited.GetEventId()
		if targetID != "" && ev.MessageEdited.GetBody() != nil {
			p.latestBody[targetID] = ev.MessageEdited.GetBody()
			delete(p.retractedFlags, targetID)
		}
	case *corev1.Event_MessageRetracted:
		targetID := ev.MessageRetracted.GetEventId()
		if targetID != "" {
			if origID := p.echoOriginalIDLocked(targetID); origID != "" {
				if _, originalRetracted := p.retractedFlags[origID]; !originalRetracted {
					delete(p.latestBody, targetID)
					p.hiddenEchoes[targetID] = struct{}{}
					return nil
				}
			}
			delete(p.latestBody, targetID)
			p.retractedFlags[targetID] = struct{}{}
		}
	case *corev1.Event_AssetCreated:
		assetID := ev.AssetCreated.GetAsset().GetId()
		if assetID != "" {
			p.assetCreations[assetID] = proto.Clone(ev.AssetCreated).(*corev1.AssetCreatedEvent)
			if parentID := ev.AssetCreated.GetParentAssetId(); parentID != "" {
				p.assetChildren[parentID] = appendIfMissing(p.assetChildren[parentID], assetID)
			}
		}
	case *corev1.Event_AssetProcessingStarted:
		assetID := ev.AssetProcessingStarted.GetAssetId()
		if assetID != "" {
			// Started clears any prior terminal state — treat as a retry.
			p.videoManifests[assetID] = &VideoAttachmentManifest{
				Started: proto.Clone(ev.AssetProcessingStarted).(*corev1.AssetProcessingStartedEvent),
			}
		}
	case *corev1.Event_AssetProcessingSucceeded:
		assetID := ev.AssetProcessingSucceeded.GetAssetId()
		if assetID != "" {
			manifest := p.videoManifests[assetID]
			if manifest == nil {
				manifest = &VideoAttachmentManifest{}
			}
			manifest.Succeeded = proto.Clone(ev.AssetProcessingSucceeded).(*corev1.AssetProcessingSucceededEvent)
			manifest.Failed = nil
			p.videoManifests[assetID] = manifest
		}
	case *corev1.Event_AssetProcessingFailed:
		assetID := ev.AssetProcessingFailed.GetAssetId()
		if assetID != "" {
			manifest := p.videoManifests[assetID]
			if manifest == nil {
				manifest = &VideoAttachmentManifest{}
			}
			manifest.Failed = proto.Clone(ev.AssetProcessingFailed).(*corev1.AssetProcessingFailedEvent)
			manifest.Succeeded = nil
			p.videoManifests[assetID] = manifest
		}
	case *corev1.Event_AssetDeleted:
		assetID := ev.AssetDeleted.GetAssetId()
		if assetID != "" {
			if declared := p.assetCreations[assetID]; declared != nil {
				if parentID := declared.GetParentAssetId(); parentID != "" {
					p.assetChildren[parentID] = removeString(p.assetChildren[parentID], assetID)
				}
			}
			delete(p.assetCreations, assetID)
			delete(p.assetChildren, assetID)
			delete(p.videoManifests, assetID)
			delete(p.assetMessageOwner, assetID)
		}
	}
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
		if messageAuthorID(entry.Event, posted) != userID {
			continue
		}
		delete(p.latestBody, eventID)
		p.retractedFlags[eventID] = struct{}{}
	}
}

func messageAuthorID(event *corev1.Event, posted *corev1.MessagePostedEvent) string {
	if posted != nil {
		if authorID := posted.GetBody().GetAuthorId(); authorID != "" {
			return authorID
		}
	}
	if event != nil {
		return event.GetActorId()
	}
	return ""
}

func (p *RoomTimelineProjection) roomIDOfEventLocked(event *corev1.Event) string {
	if started := event.GetAssetProcessingStarted(); started != nil {
		if declared := p.assetCreations[started.GetAssetId()]; declared != nil {
			return assetCreatedRoomID(declared)
		}
		return ""
	}
	if succeeded := event.GetAssetProcessingSucceeded(); succeeded != nil {
		if declared := p.assetCreations[succeeded.GetAssetId()]; declared != nil {
			return assetCreatedRoomID(declared)
		}
		return ""
	}
	if failed := event.GetAssetProcessingFailed(); failed != nil {
		if declared := p.assetCreations[failed.GetAssetId()]; declared != nil {
			return assetCreatedRoomID(declared)
		}
		return ""
	}
	if deleted := event.GetAssetDeleted(); deleted != nil {
		if declared := p.assetCreations[deleted.GetAssetId()]; declared != nil {
			return assetCreatedRoomID(declared)
		}
		return ""
	}
	if created := event.GetAssetCreated(); created != nil {
		return p.roomIDOfAssetCreatedLocked(created)
	}
	return roomIDOfEvent(event)
}

// RoomEvents returns up to `limit` entries from a room's timeline in
// newest-first order, optionally bounded by an exclusive
// stream-sequence cursor (beforeStreamSeq == 0 means "from the
// newest"). Returns a fresh slice; callers may mutate freely.
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

// LatestBody returns the current body for a message — the original
// MessagePostedEvent.body overlaid with any subsequent
// MessageEditedEvent's body, or nil + retracted=true if a
// MessageRetractedEvent has landed.
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
		return b, false, true
	}
	return nil, false, true
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

// VideoAttachmentManifest returns the latest durable processing outcome for
// the original video attachment ID, if one has been projected. The returned
// protos are clones so callers can inspect or adapt them freely.
func (p *RoomTimelineProjection) VideoAttachmentManifest(attachmentID string) (*VideoAttachmentManifest, bool) {
	p.RLock()
	defer p.RUnlock()
	if attachmentID == "" {
		return nil, false
	}
	manifest, ok := p.videoManifests[attachmentID]
	if !ok || manifest == nil {
		return nil, false
	}
	out := &VideoAttachmentManifest{}
	if manifest.Started != nil {
		out.Started = proto.Clone(manifest.Started).(*corev1.AssetProcessingStartedEvent)
	}
	if manifest.Succeeded != nil {
		out.Succeeded = proto.Clone(manifest.Succeeded).(*corev1.AssetProcessingSucceededEvent)
	}
	if manifest.Failed != nil {
		out.Failed = proto.Clone(manifest.Failed).(*corev1.AssetProcessingFailedEvent)
	}
	return out, true
}

// AssetCreation returns the durable creation event for an asset.
func (p *RoomTimelineProjection) AssetCreation(attachmentID string) (*corev1.AssetCreatedEvent, bool) {
	p.RLock()
	defer p.RUnlock()
	if attachmentID == "" {
		return nil, false
	}
	declared, ok := p.assetCreations[attachmentID]
	if !ok || declared == nil {
		return nil, false
	}
	return proto.Clone(declared).(*corev1.AssetCreatedEvent), true
}

// AssetRoomID returns the room that owns an asset. For derivatives, it walks up
// the parent chain when needed so callers can authorize thumbnail and variant
// assets using the original room scope.
func (p *RoomTimelineProjection) AssetRoomID(assetID string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	if assetID == "" {
		return "", false
	}
	declared := p.assetCreations[assetID]
	if declared == nil {
		return "", false
	}
	roomID := p.roomIDOfAssetCreatedLocked(declared)
	return roomID, roomID != ""
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
	if assetID == "" {
		return "", "", false
	}
	owner, found := p.assetMessageOwner[assetID]
	if !found {
		return "", "", false
	}
	return owner.roomID, owner.messageEventID, true
}

func (p *RoomTimelineProjection) MessageAssetsByAuthor(userID string) []MessageAssetRef {
	p.RLock()
	defer p.RUnlock()
	if userID == "" {
		return nil
	}
	out := make([]MessageAssetRef, 0)
	for assetID, owner := range p.assetMessageOwner {
		entry := p.byEventID[owner.messageEventID]
		if entry == nil || entry.Event == nil || messageAuthorID(entry.Event, entry.Event.GetMessagePosted()) != userID {
			continue
		}
		out = append(out, MessageAssetRef{
			RoomID:         owner.roomID,
			MessageEventID: owner.messageEventID,
			AssetID:        assetID,
		})
	}
	return out
}

func (p *RoomTimelineProjection) AssetSubtreeIDs(assetID string) []string {
	p.RLock()
	defer p.RUnlock()
	if assetID == "" || p.assetCreations[assetID] == nil {
		return nil
	}
	var out []string
	queue := []string{assetID}
	seen := make(map[string]struct{})
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if p.assetCreations[id] == nil {
			continue
		}
		out = append(out, id)
		queue = append(queue, p.assetChildren[id]...)
	}
	return out
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
	var out []VideoProcessingRequest
	for assetID, owner := range p.assetMessageOwner {
		if owner.roomID == "" || owner.messageEventID == "" {
			continue
		}
		// Don't recover processing for a retracted message — its video is
		// no longer visible, so transcoding it again is wasted work.
		if _, retracted := p.retractedFlags[owner.messageEventID]; retracted {
			continue
		}
		declared := p.assetCreations[assetID]
		if declared == nil {
			continue
		}
		asset := declared.GetAsset()
		if asset == nil {
			continue
		}
		if _, hasManifest := p.videoManifests[assetID]; hasManifest {
			continue
		}
		contentType := asset.GetContentType()
		if !strings.HasPrefix(contentType, "video/") && contentType != "image/gif" {
			continue
		}
		out = append(out, VideoProcessingRequest{
			RoomID:         owner.roomID,
			MessageEventID: owner.messageEventID,
			Attachment:     attachmentFromAsset(asset),
		})
	}
	return out
}

// ownedAssetIDsFromBody returns the asset IDs a message body references,
// preferring the current asset_ids list and falling back to the legacy
// embedded attachments slice.
func ownedAssetIDsFromBody(body *corev1.MessageBody) []string {
	if body == nil {
		return nil
	}
	if ids := body.GetAssetIds(); len(ids) > 0 {
		return ids
	}
	atts := body.GetAttachments()
	out := make([]string, 0, len(atts))
	for _, att := range atts {
		if id := att.GetId(); id != "" {
			out = append(out, id)
		}
	}
	return out
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

	beforeCount := (limit - 1) / 2
	afterCount := limit - beforeCount - 1
	start := targetVisibleIndex - beforeCount
	if start < 0 {
		start = 0
	}
	end := targetVisibleIndex + afterCount + 1
	if end > visibleCount {
		end = visibleCount
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

func assetCreatedRoomID(event *corev1.AssetCreatedEvent) string {
	if event == nil {
		return ""
	}
	return event.GetRoomId()
}

// roomIDOfAssetCreatedLocked resolves an asset's room, walking up the
// derivative chain to a parent when the event carries no room of its own.
// The walk is bounded and cycle-guarded: legitimate chains are one level
// deep, but corrupt/replayed EVT data could otherwise loop forever while
// holding the projection mutex.
func (p *RoomTimelineProjection) roomIDOfAssetCreatedLocked(event *corev1.AssetCreatedEvent) string {
	seen := map[string]struct{}{}
	for event != nil {
		if roomID := event.GetRoomId(); roomID != "" {
			return roomID
		}
		parentID := event.GetParentAssetId()
		if parentID == "" {
			return ""
		}
		if _, looped := seen[parentID]; looped {
			return ""
		}
		seen[parentID] = struct{}{}
		event = p.assetCreations[parentID]
	}
	return ""
}

// roomIDOfEvent extracts the room_id from any room-scoped event
// variant. Returns "" for non-room events.
//
// Kept as a free function rather than a method on Event so the
// switch lives next to its sole consumer — easier to spot when a
// new room-scoped event type is added and this list needs an
// extension.
func roomIDOfEvent(event *corev1.Event) string {
	if event == nil {
		return ""
	}
	switch e := event.GetEvent().(type) {
	case *corev1.Event_RoomCreated:
		return e.RoomCreated.GetRoomId()
	case *corev1.Event_RoomUpdated:
		return e.RoomUpdated.GetRoomId()
	case *corev1.Event_RoomDeleted:
		return e.RoomDeleted.GetRoomId()
	case *corev1.Event_RoomArchived:
		return e.RoomArchived.GetRoomId()
	case *corev1.Event_RoomUnarchived:
		return e.RoomUnarchived.GetRoomId()
	case *corev1.Event_UserJoinedRoom:
		return e.UserJoinedRoom.GetRoomId()
	case *corev1.Event_UserLeftRoom:
		return e.UserLeftRoom.GetRoomId()
	case *corev1.Event_MessagePosted:
		return e.MessagePosted.GetRoomId()
	case *corev1.Event_MessageEdited:
		return e.MessageEdited.GetRoomId()
	case *corev1.Event_MessageRetracted:
		return e.MessageRetracted.GetRoomId()
	case *corev1.Event_MessageBody:
		return e.MessageBody.GetRoomId()
	case *corev1.Event_ThreadCreated:
		return e.ThreadCreated.GetRoomId()
	case *corev1.Event_AssetCreated:
		return ""
	case *corev1.Event_ReactionAdded:
		return e.ReactionAdded.GetRoomId()
	case *corev1.Event_ReactionRemoved:
		return e.ReactionRemoved.GetRoomId()
	}
	return ""
}
