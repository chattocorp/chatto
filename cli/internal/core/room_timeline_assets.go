package core

import (
	"strings"

	"google.golang.org/protobuf/proto"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// roomTimelineAssetIndex is the legacy room-scoped asset compatibility sidecar
// for RoomTimelineProjection. It keeps old evt.room.*.asset_* histories
// replayable and tracks message-owned asset references from message bodies.
//
// New runtime reads should prefer AssetProjection through AssetModel. This
// index exists so RoomTimelineProjection can still route historical room-lane
// asset events and recover message-owned video work without mixing that logic
// into the chat timeline indexes.
type roomTimelineAssetIndex struct {
	assetCreations map[string]*corev1.AssetCreatedEvent
	assetChildren  map[string][]string
	videoManifests map[string]*VideoAttachmentManifest
	messageOwners  map[string]assetMessageRef
}

// assetMessageRef is the room + message that owns an asset, captured from the
// MessageBodyEvent that references it.
type assetMessageRef struct {
	roomID         string
	messageEventID string
}

type MessageAssetRef struct {
	RoomID         string
	MessageEventID string
	AssetID        string
}

// VideoAttachmentManifest is the projection's current processing state for one
// original video attachment. Started fires when processing is enqueued;
// Succeeded or Failed fires on terminal outcome.
type VideoAttachmentManifest struct {
	Started   *corev1.AssetProcessingStartedEvent
	Succeeded *corev1.AssetProcessingSucceededEvent
	Failed    *corev1.AssetProcessingFailedEvent
}

// VideoProcessingRequest describes an original video/GIF attachment embedded
// in a durable MessageBodyEvent that does not yet have a projected manifest.
type VideoProcessingRequest struct {
	RoomID         string
	MessageEventID string
	Attachment     *corev1.Attachment
}

func newRoomTimelineAssetIndex() *roomTimelineAssetIndex {
	return &roomTimelineAssetIndex{
		assetCreations: make(map[string]*corev1.AssetCreatedEvent),
		assetChildren:  make(map[string][]string),
		videoManifests: make(map[string]*VideoAttachmentManifest),
		messageOwners:  make(map[string]assetMessageRef),
	}
}

func (idx *roomTimelineAssetIndex) rememberMessageBodyAssets(roomID, messageEventID string, body *corev1.MessageBody) {
	if idx == nil || roomID == "" || messageEventID == "" {
		return
	}
	for _, assetID := range ownedAssetIDsFromBody(body) {
		if assetID == "" {
			continue
		}
		if _, exists := idx.messageOwners[assetID]; exists {
			continue
		}
		idx.messageOwners[assetID] = assetMessageRef{roomID: roomID, messageEventID: messageEventID}
	}
}

func (idx *roomTimelineAssetIndex) applyLifecycleEvent(event *corev1.Event) {
	if idx == nil || event == nil {
		return
	}
	switch ev := event.GetEvent().(type) {
	case *corev1.Event_AssetCreated:
		assetID := ev.AssetCreated.GetAsset().GetId()
		if assetID != "" {
			idx.assetCreations[assetID] = proto.Clone(ev.AssetCreated).(*corev1.AssetCreatedEvent)
			if parentID := ev.AssetCreated.GetParentAssetId(); parentID != "" {
				idx.assetChildren[parentID] = appendIfMissing(idx.assetChildren[parentID], assetID)
			}
		}
	case *corev1.Event_AssetProcessingStarted:
		assetID := ev.AssetProcessingStarted.GetAssetId()
		if assetID != "" {
			if manifest := idx.videoManifests[assetID]; manifest != nil && (manifest.Succeeded != nil || manifest.Failed != nil) {
				return
			}
			// Started is ignored once a terminal outcome exists. A future
			// explicit retry flow should carry attempt identity instead of
			// letting duplicate workers regress completed state.
			idx.videoManifests[assetID] = &VideoAttachmentManifest{
				Started: proto.Clone(ev.AssetProcessingStarted).(*corev1.AssetProcessingStartedEvent),
			}
		}
	case *corev1.Event_AssetProcessingSucceeded:
		assetID := ev.AssetProcessingSucceeded.GetAssetId()
		if assetID != "" {
			manifest := idx.videoManifests[assetID]
			if manifest == nil {
				manifest = &VideoAttachmentManifest{}
			}
			if manifest.Succeeded != nil || manifest.Failed != nil {
				return
			}
			manifest.Succeeded = proto.Clone(ev.AssetProcessingSucceeded).(*corev1.AssetProcessingSucceededEvent)
			manifest.Failed = nil
			idx.videoManifests[assetID] = manifest
		}
	case *corev1.Event_AssetProcessingFailed:
		assetID := ev.AssetProcessingFailed.GetAssetId()
		if assetID != "" {
			manifest := idx.videoManifests[assetID]
			if manifest == nil {
				manifest = &VideoAttachmentManifest{}
			}
			if manifest.Succeeded != nil || manifest.Failed != nil {
				return
			}
			manifest.Failed = proto.Clone(ev.AssetProcessingFailed).(*corev1.AssetProcessingFailedEvent)
			manifest.Succeeded = nil
			idx.videoManifests[assetID] = manifest
		}
	case *corev1.Event_AssetDeleted:
		assetID := ev.AssetDeleted.GetAssetId()
		if assetID != "" {
			if declared := idx.assetCreations[assetID]; declared != nil {
				if parentID := declared.GetParentAssetId(); parentID != "" {
					idx.assetChildren[parentID] = removeString(idx.assetChildren[parentID], assetID)
				}
			}
			delete(idx.assetCreations, assetID)
			delete(idx.assetChildren, assetID)
			delete(idx.videoManifests, assetID)
			delete(idx.messageOwners, assetID)
		}
	}
}

func (idx *roomTimelineAssetIndex) roomIDOfLifecycleEvent(event *corev1.Event) string {
	if idx == nil || event == nil {
		return ""
	}
	switch {
	case event.GetAssetProcessingStarted() != nil:
		return idx.assetRoomIDLocked(event.GetAssetProcessingStarted().GetAssetId())
	case event.GetAssetProcessingSucceeded() != nil:
		return idx.assetRoomIDLocked(event.GetAssetProcessingSucceeded().GetAssetId())
	case event.GetAssetProcessingFailed() != nil:
		return idx.assetRoomIDLocked(event.GetAssetProcessingFailed().GetAssetId())
	case event.GetAssetDeleted() != nil:
		return idx.assetRoomIDLocked(event.GetAssetDeleted().GetAssetId())
	case event.GetAssetCreated() != nil:
		return idx.roomIDOfAssetCreated(event.GetAssetCreated())
	default:
		return ""
	}
}

func (idx *roomTimelineAssetIndex) videoAttachmentManifest(assetID string) (*VideoAttachmentManifest, bool) {
	if idx == nil || assetID == "" {
		return nil, false
	}
	manifest, ok := idx.videoManifests[assetID]
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

func (idx *roomTimelineAssetIndex) assetCreation(assetID string) (*corev1.AssetCreatedEvent, bool) {
	if idx == nil || assetID == "" {
		return nil, false
	}
	declared, ok := idx.assetCreations[assetID]
	if !ok || declared == nil {
		return nil, false
	}
	return proto.Clone(declared).(*corev1.AssetCreatedEvent), true
}

func (idx *roomTimelineAssetIndex) assetRoomID(assetID string) (string, bool) {
	if idx == nil || assetID == "" {
		return "", false
	}
	roomID := idx.assetRoomIDLocked(assetID)
	return roomID, roomID != ""
}

func (idx *roomTimelineAssetIndex) assetMessageOwner(assetID string) (roomID, messageEventID string, ok bool) {
	if idx == nil || assetID == "" {
		return "", "", false
	}
	owner, found := idx.messageOwners[assetID]
	if !found {
		return "", "", false
	}
	return owner.roomID, owner.messageEventID, true
}

func (idx *roomTimelineAssetIndex) messageAssetsByAuthor(userID string, entryByEventID func(string) (*TimelineEntry, bool)) []MessageAssetRef {
	if idx == nil || userID == "" {
		return nil
	}
	out := make([]MessageAssetRef, 0)
	for assetID, owner := range idx.messageOwners {
		entry, _ := entryByEventID(owner.messageEventID)
		if entry == nil || entry.Event == nil || messageAuthorID(entry.Event) != userID {
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

func (idx *roomTimelineAssetIndex) messageAssetOwners() []MessageAssetRef {
	if idx == nil {
		return nil
	}
	out := make([]MessageAssetRef, 0, len(idx.messageOwners))
	for assetID, owner := range idx.messageOwners {
		out = append(out, MessageAssetRef{
			RoomID:         owner.roomID,
			MessageEventID: owner.messageEventID,
			AssetID:        assetID,
		})
	}
	return out
}

func (idx *roomTimelineAssetIndex) assetSubtreeIDs(assetID string) []string {
	if idx == nil || assetID == "" || idx.assetCreations[assetID] == nil {
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
		if idx.assetCreations[id] == nil {
			continue
		}
		out = append(out, id)
		queue = append(queue, idx.assetChildren[id]...)
	}
	return out
}

func (idx *roomTimelineAssetIndex) unmanifestedVideoAttachments(retractedFlags map[string]struct{}) []VideoProcessingRequest {
	if idx == nil {
		return nil
	}
	var out []VideoProcessingRequest
	for assetID, owner := range idx.messageOwners {
		if owner.roomID == "" || owner.messageEventID == "" {
			continue
		}
		if _, retracted := retractedFlags[owner.messageEventID]; retracted {
			continue
		}
		declared := idx.assetCreations[assetID]
		if declared == nil {
			continue
		}
		asset := declared.GetAsset()
		if asset == nil {
			continue
		}
		if _, hasManifest := idx.videoManifests[assetID]; hasManifest {
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

func (idx *roomTimelineAssetIndex) assetRoomIDLocked(assetID string) string {
	if idx == nil || assetID == "" {
		return ""
	}
	return idx.roomIDOfAssetCreated(idx.assetCreations[assetID])
}

// roomIDOfAssetCreated resolves an asset's room, walking up the derivative
// chain to a parent when the event carries no room of its own. The walk is
// bounded and cycle-guarded: legitimate chains are one level deep, but
// corrupt/replayed EVT data could otherwise loop forever while holding the
// projection mutex.
func (idx *roomTimelineAssetIndex) roomIDOfAssetCreated(event *corev1.AssetCreatedEvent) string {
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
		event = idx.assetCreations[parentID]
	}
	return ""
}

// ownedAssetIDsFromBody returns the asset IDs a message body references,
// preferring the current asset_ids list and falling back to the legacy embedded
// attachments slice.
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
