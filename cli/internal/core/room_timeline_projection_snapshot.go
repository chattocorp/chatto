package core

import (
	"fmt"
	"slices"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const roomTimelineSnapshotCompatibilityID = "room-timeline-v1"

func (*RoomTimelineProjection) SnapshotCompatibilityID() string {
	return roomTimelineSnapshotCompatibilityID
}

func (p *RoomTimelineProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	snapshot := &corev1.RoomTimelineProjectionSnapshot{ReplayGuard: snapshotReplayGuard(p.replayGuard), RetractedEventIds: sortedMapKeys(p.retractedFlags), HiddenEchoEventIds: sortedMapKeys(p.hiddenEchoes), ShreddedUserIds: sortedMapKeys(p.shreddedUsers)}
	for _, entry := range p.entries {
		snapshot.Entries = append(snapshot.Entries, &corev1.TimelineEntrySnapshot{StreamSequence: entry.StreamSeq, Event: proto.Clone(entry.Event).(*corev1.Event)})
	}
	bodyIDs := make(map[string]struct{}, len(p.latestBody)+len(p.bodyEventSeqs)+len(p.currentBodySeq))
	for id := range p.latestBody {
		bodyIDs[id] = struct{}{}
	}
	for id := range p.bodyEventSeqs {
		bodyIDs[id] = struct{}{}
	}
	for id := range p.currentBodySeq {
		bodyIDs[id] = struct{}{}
	}
	for _, id := range sortedMapKeys(bodyIDs) {
		row := &corev1.TimelineBodySnapshot{MessageEventId: id, BodyEventSequences: slices.Clone(p.bodyEventSeqs[id]), CurrentBodySequence: p.currentBodySeq[id]}
		if p.latestBody[id] != nil {
			row.Body = cloneMessageBody(p.latestBody[id])
		}
		snapshot.Bodies = append(snapshot.Bodies, row)
	}
	appendTimes := func(values map[string]time.Time) []*corev1.StringTimestampSnapshot {
		rows := make([]*corev1.StringTimestampSnapshot, 0, len(values))
		for _, key := range sortedMapKeys(values) {
			if !values[key].IsZero() {
				rows = append(rows, &corev1.StringTimestampSnapshot{Key: key, Value: timestamppb.New(values[key])})
			}
		}
		return rows
	}
	snapshot.TombstonedAt = appendTimes(p.tombstonedAt)
	snapshot.ShreddedAt = appendTimes(p.shreddedAt)
	legacy := &corev1.AssetProjectionSnapshot{}
	for _, assetID := range sortedMapKeys(p.assets.assetCreations) {
		legacy.Creations = append(legacy.Creations, proto.Clone(p.assets.assetCreations[assetID]).(*corev1.AssetCreatedEvent))
	}
	for _, parentID := range sortedMapKeys(p.assets.assetChildren) {
		legacy.Children = append(legacy.Children, &corev1.AssetChildrenSnapshot{ParentAssetId: parentID, ChildAssetIds: slices.Clone(p.assets.assetChildren[parentID])})
	}
	for _, assetID := range sortedMapKeys(p.assets.videoManifests) {
		manifest := p.assets.videoManifests[assetID]
		row := &corev1.AssetManifestSnapshot{AssetId: assetID}
		if manifest.Started != nil {
			row.Started = proto.Clone(manifest.Started).(*corev1.AssetProcessingStartedEvent)
		}
		if manifest.Succeeded != nil {
			row.Succeeded = proto.Clone(manifest.Succeeded).(*corev1.AssetProcessingSucceededEvent)
		}
		if manifest.Failed != nil {
			row.Failed = proto.Clone(manifest.Failed).(*corev1.AssetProcessingFailedEvent)
		}
		legacy.Manifests = append(legacy.Manifests, row)
	}
	snapshot.LegacyAssets = legacy
	for _, assetID := range sortedMapKeys(p.assets.messageOwners) {
		owner := p.assets.messageOwners[assetID]
		snapshot.AssetMessageOwners = append(snapshot.AssetMessageOwners, &corev1.AssetMessageOwnerSnapshot{AssetId: assetID, RoomId: owner.roomID, MessageEventId: owner.messageEventID})
	}
	snapshot.PublicLinkPreviewAssetIds = sortedMapKeys(p.assets.publicLinkPreviewAssets)
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *RoomTimelineProjection) Restore(data []byte) error {
	snapshot := &corev1.RoomTimelineProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal room timeline snapshot: %w", err)
		}
	}
	guard, err := restoreReplayGuard(snapshot.GetReplayGuard())
	if err != nil {
		return fmt.Errorf("room timeline snapshot replay guard: %w", err)
	}
	restored := NewRoomTimelineProjection()
	restored.replayGuard = guard
	for _, row := range snapshot.GetEntries() {
		if row.GetStreamSequence() == 0 || row.GetEvent().GetId() == "" {
			return fmt.Errorf("room timeline snapshot has invalid timeline entry")
		}
		event := proto.Clone(row.GetEvent()).(*corev1.Event)
		index := restored.appendEntryLocked(row.GetStreamSequence(), event)
		if _, duplicate := restored.byEventID[event.GetId()]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats event %q", event.GetId())
		}
		if shouldIndexRoomTimelineEvent(event) {
			restored.byEventID[event.GetId()] = index
		}
		roomID := roomIDOfEvent(event)
		if event.GetMessagePosted() != nil {
			if roomID == "" {
				return fmt.Errorf("room timeline snapshot message %q has no room", event.GetId())
			}
			restored.messagePostsByRoom[roomID] = append(restored.messagePostsByRoom[roomID], index)
			if originalID := event.GetMessagePosted().GetEchoOfEventId(); originalID != "" {
				restored.echoLinks[originalID] = append(restored.echoLinks[originalID], event.GetId())
			}
		}
		if isVisibleRoomTimelineEntry(event) {
			if roomID == "" {
				return fmt.Errorf("room timeline snapshot event %q has no room", event.GetId())
			}
			restored.byRoom[roomID] = append(restored.byRoom[roomID], index)
		}
	}
	for _, row := range snapshot.GetBodies() {
		id := row.GetMessageEventId()
		if id == "" {
			return fmt.Errorf("room timeline snapshot has empty body message ID")
		}
		if _, duplicate := restored.bodyEventSeqs[id]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats body %q", id)
		}
		if row.GetBody() != nil {
			restored.latestBody[id] = cloneMessageBody(row.GetBody())
		}
		restored.bodyEventSeqs[id] = slices.Clone(row.GetBodyEventSequences())
		restored.currentBodySeq[id] = row.GetCurrentBodySequence()
	}
	restoreTimes := func(rows []*corev1.StringTimestampSnapshot) (map[string]time.Time, error) {
		values := make(map[string]time.Time, len(rows))
		for _, row := range rows {
			if row.GetKey() == "" || row.GetValue() == nil {
				return nil, fmt.Errorf("room timeline snapshot has invalid timestamp mapping")
			}
			if _, duplicate := values[row.GetKey()]; duplicate {
				return nil, fmt.Errorf("room timeline snapshot repeats timestamp key %q", row.GetKey())
			}
			value, err := snapshotTime(row.GetValue())
			if err != nil {
				return nil, err
			}
			values[row.GetKey()] = value
		}
		return values, nil
	}
	restored.tombstonedAt, err = restoreTimes(snapshot.GetTombstonedAt())
	if err != nil {
		return fmt.Errorf("room timeline tombstones: %w", err)
	}
	restored.shreddedAt, err = restoreTimes(snapshot.GetShreddedAt())
	if err != nil {
		return fmt.Errorf("room timeline shred timestamps: %w", err)
	}
	fillSet := func(values []string) (map[string]struct{}, error) {
		set := make(map[string]struct{}, len(values))
		for _, value := range values {
			if value == "" {
				return nil, fmt.Errorf("empty set value")
			}
			if _, duplicate := set[value]; duplicate {
				return nil, fmt.Errorf("repeated set value %q", value)
			}
			set[value] = struct{}{}
		}
		return set, nil
	}
	restored.retractedFlags, err = fillSet(snapshot.GetRetractedEventIds())
	if err != nil {
		return fmt.Errorf("room timeline retracted IDs: %w", err)
	}
	restored.hiddenEchoes, err = fillSet(snapshot.GetHiddenEchoEventIds())
	if err != nil {
		return fmt.Errorf("room timeline hidden echoes: %w", err)
	}
	restored.shreddedUsers, err = fillSet(snapshot.GetShreddedUserIds())
	if err != nil {
		return fmt.Errorf("room timeline shredded users: %w", err)
	}
	assets := newRoomTimelineAssetIndex()
	legacy := snapshot.GetLegacyAssets()
	if legacy != nil {
		for _, creation := range legacy.GetCreations() {
			assetID := creation.GetAsset().GetId()
			if assetID == "" {
				return fmt.Errorf("room timeline snapshot has legacy asset without ID")
			}
			if _, duplicate := assets.assetCreations[assetID]; duplicate {
				return fmt.Errorf("room timeline snapshot repeats legacy asset %q", assetID)
			}
			assets.assetCreations[assetID] = proto.Clone(creation).(*corev1.AssetCreatedEvent)
		}
		for _, row := range legacy.GetChildren() {
			if row.GetParentAssetId() == "" {
				return fmt.Errorf("room timeline snapshot has empty legacy asset parent")
			}
			if _, duplicate := assets.assetChildren[row.GetParentAssetId()]; duplicate {
				return fmt.Errorf("room timeline snapshot repeats legacy asset parent")
			}
			assets.assetChildren[row.GetParentAssetId()] = slices.Clone(row.GetChildAssetIds())
		}
		for _, row := range legacy.GetManifests() {
			if row.GetAssetId() == "" {
				return fmt.Errorf("room timeline snapshot has empty legacy manifest ID")
			}
			if row.GetSucceeded() != nil && row.GetFailed() != nil {
				return fmt.Errorf("room timeline snapshot legacy manifest has two outcomes")
			}
			manifest := &VideoAttachmentManifest{}
			if row.GetStarted() != nil {
				manifest.Started = proto.Clone(row.GetStarted()).(*corev1.AssetProcessingStartedEvent)
			}
			if row.GetSucceeded() != nil {
				manifest.Succeeded = proto.Clone(row.GetSucceeded()).(*corev1.AssetProcessingSucceededEvent)
			}
			if row.GetFailed() != nil {
				manifest.Failed = proto.Clone(row.GetFailed()).(*corev1.AssetProcessingFailedEvent)
			}
			assets.videoManifests[row.GetAssetId()] = manifest
		}
	}
	for _, row := range snapshot.GetAssetMessageOwners() {
		if row.GetAssetId() == "" || row.GetRoomId() == "" || row.GetMessageEventId() == "" {
			return fmt.Errorf("room timeline snapshot has invalid asset owner")
		}
		if _, duplicate := assets.messageOwners[row.GetAssetId()]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats asset owner")
		}
		assets.messageOwners[row.GetAssetId()] = assetMessageRef{roomID: row.GetRoomId(), messageEventID: row.GetMessageEventId()}
	}
	assets.publicLinkPreviewAssets, err = fillSet(snapshot.GetPublicLinkPreviewAssetIds())
	if err != nil {
		return fmt.Errorf("room timeline public preview assets: %w", err)
	}
	restored.assets = assets
	for messageID, body := range restored.latestBody {
		entry, ok := restored.entryByEventIDLocked(messageID)
		if !ok || entry.Event == nil {
			continue
		}
		roomID := roomIDOfEvent(entry.Event)
		restored.refreshAttachmentMessageLocked(roomID, messageID, body)
	}
	p.Lock()
	p.entries, p.byRoom, p.byEventID, p.messagePostsByRoom, p.replayGuard, p.latestBody, p.bodyEventSeqs, p.currentBodySeq, p.retractedFlags, p.tombstonedAt, p.shreddedAt, p.attachmentMessageIDsByRoom, p.attachmentMessageRoom, p.echoLinks, p.hiddenEchoes, p.assets, p.shreddedUsers = restored.entries, restored.byRoom, restored.byEventID, restored.messagePostsByRoom, restored.replayGuard, restored.latestBody, restored.bodyEventSeqs, restored.currentBodySeq, restored.retractedFlags, restored.tombstonedAt, restored.shreddedAt, restored.attachmentMessageIDsByRoom, restored.attachmentMessageRoom, restored.echoLinks, restored.hiddenEchoes, restored.assets, restored.shreddedUsers
	p.Unlock()
	return nil
}
