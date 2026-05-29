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
	byRoom    map[string][]*TimelineEntry
	byEventID map[string]*TimelineEntry
	// latestBody is the derived current-body index. Updated as
	// MessageEdited / MessageRetracted entries are applied so that
	// LatestBody resolves in O(1) instead of an O(room size) walk
	// of byRoom. A nil entry means "retracted"; absent means "no
	// embedded body / not yet projected".
	latestBody     map[string]*corev1.MessageBody
	retractedFlags map[string]struct{}
	// echoLinks maps an original message's event_id to the event_ids
	// of any echoes pointing at it. Maintained as MessagePostedEvents
	// with EchoOfEventId arrive. Used by EditMessage / DeleteMessage
	// to fan mutations across linked messages — pre-cutover the
	// echo + original shared a messageBodyId, so an edit on either
	// updated both via the shared SERVER_BODIES entry; post-cutover
	// each has its own embedded body and we need explicit
	// propagation.
	echoLinks map[string][]string
	// videoManifests stores the latest durable processing outcome for each
	// original video attachment. A processed event supersedes a failed event
	// and vice versa; generated asset metadata lives in the event payload.
	assetCreations map[string]*corev1.AssetCreatedEvent
	videoManifests map[string]*VideoAttachmentManifest
	// assetMessageOwner maps an asset ID to the message that references it,
	// derived from MessagePostedEvent bodies. Upload-time AssetCreatedEvents
	// don't carry message linkage — the message doesn't exist yet — so the
	// deprecated AssetCreatedEvent.message_event_id is never set on new
	// uploads. Message ownership is reconstructed here from the posting
	// message's asset_ids (or legacy embedded attachments).
	assetMessageOwner map[string]assetMessageRef
}

// assetMessageRef is the room + message that owns an asset, captured from
// the MessagePostedEvent that references it.
type assetMessageRef struct {
	roomID         string
	messageEventID string
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
		byRoom:         make(map[string][]*TimelineEntry),
		byEventID:      make(map[string]*TimelineEntry),
		latestBody:     make(map[string]*corev1.MessageBody),
		retractedFlags: make(map[string]struct{}),
		echoLinks:      make(map[string][]string),
		assetCreations:    make(map[string]*corev1.AssetCreatedEvent),
		videoManifests:    make(map[string]*VideoAttachmentManifest),
		assetMessageOwner: make(map[string]assetMessageRef),
	}
}

// Subjects implements events.Projection. The wildcard filter consumes
// every event under the room aggregate — the projection owns the
// "everything that happened in this room" surface, so it has to see
// all of it.
func (p *RoomTimelineProjection) Subjects() []string {
	return []string{events.RoomSubjectFilter()}
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
	roomID := p.roomIDOfEventLocked(event)
	if roomID == "" {
		return nil
	}

	// Idempotency: a re-applied event with the same envelope id is a
	// no-op. The Projection.Apply contract is "Apply(e,n) twice ==
	// Apply(e,n) once"; this is how we honour it.
	if eid := event.GetId(); eid != "" {
		if _, exists := p.byEventID[eid]; exists {
			return nil
		}
	}

	entry := &TimelineEntry{StreamSeq: seq, Event: event}
	p.byRoom[roomID] = append(p.byRoom[roomID], entry)
	if eid := event.GetId(); eid != "" {
		p.byEventID[eid] = entry
	}

	// Maintain the latest-body / retracted-flag derived index so
	// LatestBody is O(1) instead of an O(room) walk per lookup.
	switch ev := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		targetID := ev.MessagePosted.GetEventId()
		if targetID == "" {
			targetID = event.GetId()
		}
		if targetID != "" {
			p.latestBody[targetID] = ev.MessagePosted.GetBody()
			delete(p.retractedFlags, targetID)
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
		// Track echo links so edits / retracts on either side can fan
		// out to the other.
		if origID := ev.MessagePosted.GetEchoOfEventId(); origID != "" && targetID != "" {
			p.echoLinks[origID] = append(p.echoLinks[origID], targetID)
		}
	case *corev1.Event_MessageEdited:
		targetID := ev.MessageEdited.GetEventId()
		if targetID != "" {
			p.latestBody[targetID] = ev.MessageEdited.GetBody()
			delete(p.retractedFlags, targetID)
		}
	case *corev1.Event_MessageRetracted:
		targetID := ev.MessageRetracted.GetEventId()
		if targetID != "" {
			delete(p.latestBody, targetID)
			p.retractedFlags[targetID] = struct{}{}
		}
	case *corev1.Event_AssetCreated:
		assetID := ev.AssetCreated.GetAsset().GetId()
		if assetID != "" {
			p.assetCreations[assetID] = proto.Clone(ev.AssetCreated).(*corev1.AssetCreatedEvent)
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
			delete(p.assetCreations, assetID)
			delete(p.videoManifests, assetID)
			delete(p.assetMessageOwner, assetID)
		}
	}
	return nil
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
	if _, isRetracted := p.retractedFlags[eventID]; isRetracted {
		return nil, true, true
	}
	if b, has := p.latestBody[eventID]; has {
		return b, false, true
	}
	return nil, false, true
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

// LinkedEventIDs returns the set of event_ids that an edit / retract
// targeting `eventID` should also be applied to: any echoes pointing
// at `eventID`, plus the original message that `eventID` is an echo
// of (if any). Does NOT include `eventID` itself — the caller emits
// the mutation for the target separately.
//
// Used by EditMessage / DeleteMessage to preserve the legacy "edit
// the echo, the original updates too (and vice versa)" semantic
// after the shared-messageBodyId mechanism was retired in #614.
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
	out := make([]*TimelineEntry, 0, limit)
	for i := len(entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := entries[i]
		if beforeStreamSeq > 0 && e.StreamSeq >= beforeStreamSeq {
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
	out := make([]*TimelineEntry, 0, limit)
	for _, e := range entries {
		if e.StreamSeq <= afterStreamSeq {
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
	case *corev1.Event_AssetCreated:
		return ""
	case *corev1.Event_ReactionAdded:
		return e.ReactionAdded.GetRoomId()
	case *corev1.Event_ReactionRemoved:
		return e.ReactionRemoved.GetRoomId()
	}
	return ""
}
