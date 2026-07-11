package core

import (
	"time"

	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// AssetProjection owns durable asset lifecycle state. New writes live under
// evt.asset.{assetId}.*, but the projection also consumes the legacy
// evt.room.*.asset_* lanes so beta histories continue to replay.
type AssetProjection struct {
	events.MemoryProjection
	replayGuard      projectionReplayGuard
	assetCreations   map[string]*corev1.AssetCreatedEvent
	assetChildren    map[string][]string
	videoManifests   map[string]*VideoAttachmentManifest
	deletedAssets    map[string]struct{}
	deletedAssetRoom map[string]string
}

func NewAssetProjection() *AssetProjection {
	return &AssetProjection{
		replayGuard:      newProjectionReplayGuard(),
		assetCreations:   make(map[string]*corev1.AssetCreatedEvent),
		assetChildren:    make(map[string][]string),
		videoManifests:   make(map[string]*VideoAttachmentManifest),
		deletedAssets:    make(map[string]struct{}),
		deletedAssetRoom: make(map[string]string),
	}
}

func (p *AssetProjection) Subjects() []string {
	return []string{
		events.AssetSubjectFilter(),
		events.RoomEventTypeFilter(events.EventAssetCreated),
		events.RoomEventTypeFilter(events.EventAssetProcessingStarted),
		events.RoomEventTypeFilter(events.EventAssetProcessingSucceeded),
		events.RoomEventTypeFilter(events.EventAssetProcessingFailed),
		events.RoomEventTypeFilter(events.EventAssetDeleted),
	}
}

func (p *AssetProjection) Apply(event *corev1.Event, seq uint64) error {
	if event == nil || !isAssetLifecycleEvent(event) {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	if p.replayGuard.seenOrMark(event, seq) {
		return nil
	}

	switch ev := event.GetEvent().(type) {
	case *corev1.Event_AssetCreated:
		assetID := ev.AssetCreated.GetAsset().GetId()
		if assetID != "" {
			p.assetCreations[assetID] = proto.Clone(ev.AssetCreated).(*corev1.AssetCreatedEvent)
			delete(p.deletedAssets, assetID)
			delete(p.deletedAssetRoom, assetID)
			if parentID := ev.AssetCreated.GetParentAssetId(); parentID != "" {
				p.assetChildren[parentID] = appendIfMissing(p.assetChildren[parentID], assetID)
			}
		}
	case *corev1.Event_AssetProcessingStarted:
		assetID := ev.AssetProcessingStarted.GetAssetId()
		if assetID != "" {
			if _, deleted := p.deletedAssets[assetID]; deleted {
				return nil
			}
			if manifest := p.videoManifests[assetID]; manifest != nil && (manifest.Succeeded != nil || manifest.Failed != nil) {
				return nil
			}
			p.videoManifests[assetID] = &VideoAttachmentManifest{
				Started: proto.Clone(ev.AssetProcessingStarted).(*corev1.AssetProcessingStartedEvent),
			}
		}
	case *corev1.Event_AssetProcessingSucceeded:
		assetID := ev.AssetProcessingSucceeded.GetAssetId()
		if assetID != "" {
			if _, deleted := p.deletedAssets[assetID]; deleted {
				return nil
			}
			manifest := p.videoManifests[assetID]
			if manifest == nil {
				manifest = &VideoAttachmentManifest{}
			}
			if manifest.Succeeded != nil || manifest.Failed != nil {
				return nil
			}
			manifest.Succeeded = proto.Clone(ev.AssetProcessingSucceeded).(*corev1.AssetProcessingSucceededEvent)
			manifest.Failed = nil
			p.videoManifests[assetID] = manifest
		}
	case *corev1.Event_AssetProcessingFailed:
		assetID := ev.AssetProcessingFailed.GetAssetId()
		if assetID != "" {
			if _, deleted := p.deletedAssets[assetID]; deleted {
				return nil
			}
			manifest := p.videoManifests[assetID]
			if manifest == nil {
				manifest = &VideoAttachmentManifest{}
			}
			if manifest.Succeeded != nil || manifest.Failed != nil {
				return nil
			}
			manifest.Failed = proto.Clone(ev.AssetProcessingFailed).(*corev1.AssetProcessingFailedEvent)
			manifest.Succeeded = nil
			p.videoManifests[assetID] = manifest
		}
	case *corev1.Event_AssetDeleted:
		assetID := ev.AssetDeleted.GetAssetId()
		if assetID != "" {
			p.deletedAssets[assetID] = struct{}{}
			if roomID := p.assetRoomIDLocked(assetID); roomID != "" {
				p.deletedAssetRoom[assetID] = roomID
			}
			if declared := p.assetCreations[assetID]; declared != nil {
				if parentID := declared.GetParentAssetId(); parentID != "" {
					p.assetChildren[parentID] = removeString(p.assetChildren[parentID], assetID)
				}
			}
			delete(p.assetCreations, assetID)
			delete(p.assetChildren, assetID)
			delete(p.videoManifests, assetID)
		}
	}
	return nil
}

func (p *AssetProjection) CompleteStartupReplay() {
	p.Lock()
	defer p.Unlock()
	p.replayGuard.completeReplay()
}

func (p *AssetProjection) AssetCreation(assetID string) (*corev1.AssetCreatedEvent, bool) {
	p.RLock()
	defer p.RUnlock()
	if assetID == "" {
		return nil, false
	}
	declared, ok := p.assetCreations[assetID]
	if !ok || declared == nil {
		return nil, false
	}
	return proto.Clone(declared).(*corev1.AssetCreatedEvent), true
}

func (p *AssetProjection) AssetRoomID(assetID string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	if assetID == "" {
		return "", false
	}
	roomID := p.assetRoomIDLocked(assetID)
	return roomID, roomID != ""
}

func (p *AssetProjection) VideoAttachmentManifest(assetID string) (*VideoAttachmentManifest, bool) {
	p.RLock()
	defer p.RUnlock()
	if assetID == "" {
		return nil, false
	}
	manifest, ok := p.videoManifests[assetID]
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

func (p *AssetProjection) AssetDeleted(assetID string) bool {
	p.RLock()
	defer p.RUnlock()
	if assetID == "" {
		return false
	}
	_, deleted := p.deletedAssets[assetID]
	return deleted
}

func (p *AssetProjection) PendingExpiredAssets(now time.Time) []*corev1.AssetCreatedEvent {
	p.RLock()
	defer p.RUnlock()
	var out []*corev1.AssetCreatedEvent
	for _, declared := range p.assetCreations {
		if declared == nil || declared.GetPendingExpiresAt() == nil {
			continue
		}
		if declared.GetPendingExpiresAt().AsTime().After(now) {
			continue
		}
		out = append(out, proto.Clone(declared).(*corev1.AssetCreatedEvent))
	}
	return out
}

func (p *AssetProjection) AssetSubtreeIDs(assetID string) []string {
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

func (p *AssetProjection) assetRoomIDLocked(assetID string) string {
	if roomID := p.deletedAssetRoom[assetID]; roomID != "" {
		return roomID
	}
	declared := p.assetCreations[assetID]
	return p.roomIDOfAssetCreatedLocked(declared)
}

func (p *AssetProjection) roomIDOfAssetCreatedLocked(event *corev1.AssetCreatedEvent) string {
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

func (p *AssetProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var bytes int64
	for id, created := range p.assetCreations {
		bytes += projectionMapEntryOverhead + int64(len(id))
		if asset := created.GetAsset(); asset != nil {
			bytes += int64(len(asset.GetId())+len(asset.GetFilename())+len(asset.GetContentType())) + 64
		}
	}
	var derivatives int64
	for _, children := range p.assetChildren {
		derivatives += int64(len(children))
		bytes += projectionMapEntryOverhead + int64(len(children))*16
	}
	manifestBytes := int64(len(p.videoManifests)) * (projectionMapEntryOverhead + 128)
	bytes += manifestBytes
	deletedBytes := int64(len(p.deletedAssets)) * (projectionMapEntryOverhead + 32)
	bytes += deletedBytes
	retainedEventIDs := p.replayGuard.retainedEventIDs()
	retainedEventIDsBytes := estimateStringSetBytes(retainedEventIDs)
	bytes += retainedEventIDsBytes
	return int64(len(p.assetCreations) + len(p.videoManifests) + len(p.deletedAssets)), bytes, []ProjectionAdminMetric{
		{Name: "assets", Value: int64(len(p.assetCreations)), Bytes: bytes - manifestBytes - deletedBytes - retainedEventIDsBytes},
		{Name: "derivatives", Value: derivatives, Bytes: 0},
		{Name: "video_manifests", Value: int64(len(p.videoManifests)), Bytes: manifestBytes},
		{Name: "deleted_assets", Value: int64(len(p.deletedAssets)), Bytes: deletedBytes},
		{Name: "applied_event_ids", Value: int64(len(retainedEventIDs)), Bytes: retainedEventIDsBytes},
		{Name: "event_id_compatibility_mode", Value: p.replayGuard.compatibilityValue(), Bytes: 0},
	}
}
