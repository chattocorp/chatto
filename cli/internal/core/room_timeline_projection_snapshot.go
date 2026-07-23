package core

import (
	"fmt"
	"slices"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const roomTimelineSnapshotContractID = "v2"

func (*RoomTimelineProjection) SnapshotContractID() string {
	return roomTimelineSnapshotContractID
}

func (p *RoomTimelineProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	snapshot := &corev1.RoomTimelineProjectionSnapshotV2{ReplayGuard: snapshotReplayGuard(p.replayGuard), RetractedEventIds: sortedMapKeys(p.retractedFlags), HiddenEchoEventIds: sortedMapKeys(p.hiddenEchoes), ShreddedUserIds: sortedMapKeys(p.shreddedUsers)}
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
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *RoomTimelineProjection) Restore(data []byte) error {
	snapshot := &corev1.RoomTimelineProjectionSnapshotV2{}
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
	for messageID, body := range restored.latestBody {
		entry, ok := restored.entryByEventIDLocked(messageID)
		if !ok || entry.Event == nil {
			continue
		}
		roomID := roomIDOfEvent(entry.Event)
		restored.refreshAttachmentMessageLocked(roomID, messageID, body)
	}
	p.Lock()
	p.entries, p.byRoom, p.byEventID, p.messagePostsByRoom, p.replayGuard, p.latestBody, p.bodyEventSeqs, p.currentBodySeq, p.retractedFlags, p.tombstonedAt, p.shreddedAt, p.attachmentMessageIDsByRoom, p.attachmentMessageRoom, p.echoLinks, p.hiddenEchoes, p.shreddedUsers = restored.entries, restored.byRoom, restored.byEventID, restored.messagePostsByRoom, restored.replayGuard, restored.latestBody, restored.bodyEventSeqs, restored.currentBodySeq, restored.retractedFlags, restored.tombstonedAt, restored.shreddedAt, restored.attachmentMessageIDsByRoom, restored.attachmentMessageRoom, restored.echoLinks, restored.hiddenEchoes, restored.shreddedUsers
	p.Unlock()
	return nil
}
