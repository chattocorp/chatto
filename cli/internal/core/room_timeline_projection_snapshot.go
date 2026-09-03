package core

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/evtstream"
	projectionv1 "hmans.de/chatto/internal/pb/chatto/core/projection/v1"
)

var roomTimelineSnapshotContractID = snapshotContractID("v7", &projectionv1.RoomTimelineProjectionSnapshot{})

func (*RoomTimelineProjection) SnapshotContractID() string {
	return roomTimelineSnapshotContractID
}

func (p *RoomTimelineProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	snapshot := &projectionv1.RoomTimelineProjectionSnapshot{ReplayGuard: snapshotReplayGuard(p.replayGuard), RetractedEventIds: sortedMapKeys(p.retractedFlags), HiddenEchoEventIds: sortedMapKeys(p.hiddenEchoes), ShreddedUserIds: sortedMapKeys(p.shreddedUsers)}
	for _, entry := range p.entries {
		row := &projectionv1.TimelineEntrySnapshot{
			StreamSequence:    entry.StreamSeq,
			EventId:           entry.EventID,
			RoomId:            entry.RoomID,
			ActorId:           entry.ActorID,
			EventType:         entry.EventType,
			ThreadRootEventId: entry.ThreadRootEventID,
			EchoOfEventId:     entry.EchoOfEventID,
			InThreadEventId:   entry.InThreadEventID,
		}
		if !entry.CreatedAt.IsZero() {
			row.CreatedAt = timestamppb.New(entry.CreatedAt)
		}
		snapshot.Entries = append(snapshot.Entries, row)
	}
	for _, id := range sortedMapKeys(p.bodyStates) {
		state := p.bodyStates[id]
		row := &projectionv1.TimelineBodySnapshot{
			MessageEventId:      id,
			BodyEventSequences:  appendBodySequences(nil, state),
			CurrentBodySequence: state.currentSequence,
			CurrentBodyEventId:  state.currentEventID,
			AuthorId:            state.authorID,
			AttachmentCount:     uint32(state.attachmentCount),
			Active:              state.active,
		}
		snapshot.Bodies = append(snapshot.Bodies, row)
	}
	appendTimes := func(values map[string]time.Time) []*projectionv1.StringTimestampSnapshot {
		rows := make([]*projectionv1.StringTimestampSnapshot, 0, len(values))
		for _, key := range sortedMapKeys(values) {
			if !values[key].IsZero() {
				rows = append(rows, &projectionv1.StringTimestampSnapshot{Key: key, Value: timestamppb.New(values[key])})
			}
		}
		return rows
	}
	snapshot.TombstonedAt = appendTimes(p.tombstonedAt)
	snapshot.ShreddedAt = appendTimes(p.shreddedAt)
	for _, roomID := range sortedMapKeys(p.pinnedMessagesByRoom) {
		for _, messageID := range sortedMapKeys(p.pinnedMessagesByRoom[roomID]) {
			pin := p.pinnedMessagesByRoom[roomID][messageID]
			snapshot.PinnedMessages = append(snapshot.PinnedMessages, &projectionv1.PinnedMessageSnapshot{
				PinEventId: pin.PinEventID, RoomId: pin.RoomID, MessageEventId: pin.MessageEventID,
				PinSequence: pin.PinSequence,
			})
		}
	}
	for _, roomID := range sortedMapKeys(p.latestPinByRoom) {
		latest := p.latestPinByRoom[roomID]
		snapshot.LatestRoomPins = append(snapshot.LatestRoomPins, &projectionv1.LatestRoomPinSnapshot{
			RoomId: roomID, PinEventId: latest.PinEventID, PinSequence: latest.PinSequence,
		})
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *RoomTimelineProjection) Restore(data []byte) error {
	snapshot := &projectionv1.RoomTimelineProjectionSnapshot{}
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
	var previousEntrySequence uint64
	for _, row := range snapshot.GetEntries() {
		if row.GetStreamSequence() == 0 || row.GetEventId() == "" || row.GetRoomId() == "" || !isIndexedRoomTimelineEventType(row.GetEventType()) {
			return fmt.Errorf("room timeline snapshot has invalid timeline entry")
		}
		if row.GetStreamSequence() <= previousEntrySequence {
			return fmt.Errorf("room timeline snapshot entries are not in stream order")
		}
		previousEntrySequence = row.GetStreamSequence()
		if row.GetEventType() == evtstream.EventMessagePosted {
			if row.GetThreadRootEventId() == "" {
				return fmt.Errorf("room timeline snapshot message %q has no thread root", row.GetEventId())
			}
			if row.GetInThreadEventId() != "" && row.GetThreadRootEventId() != row.GetInThreadEventId() {
				return fmt.Errorf("room timeline snapshot message %q has inconsistent thread routing", row.GetEventId())
			}
		} else if row.GetThreadRootEventId() != "" || row.GetInThreadEventId() != "" || row.GetEchoOfEventId() != "" {
			return fmt.Errorf("room timeline snapshot event %q has unexpected message routing", row.GetEventId())
		}
		createdAt, err := snapshotTime(row.GetCreatedAt())
		if err != nil {
			return fmt.Errorf("room timeline snapshot event %q created time: %w", row.GetEventId(), err)
		}
		entry := TimelineEntry{
			StreamSeq:         row.GetStreamSequence(),
			EventID:           row.GetEventId(),
			RoomID:            row.GetRoomId(),
			ActorID:           row.GetActorId(),
			CreatedAt:         createdAt,
			EventType:         row.GetEventType(),
			ThreadRootEventID: row.GetThreadRootEventId(),
			EchoOfEventID:     row.GetEchoOfEventId(),
			InThreadEventID:   row.GetInThreadEventId(),
		}
		index := len(restored.entries)
		restored.entries = append(restored.entries, entry)
		if _, duplicate := restored.byEventID[entry.EventID]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats event %q", entry.EventID)
		}
		restored.byEventID[entry.EventID] = index
		if entry.IsMessagePost() {
			restored.messagePostsByRoom[entry.RoomID] = append(restored.messagePostsByRoom[entry.RoomID], index)
			if entry.EchoOfEventID == "" && entry.ActorID != "" {
				restored.latestOriginalPostAt[roomActorKey{roomID: entry.RoomID, actorID: entry.ActorID}] = entry.CreatedAt
			}
			if entry.EchoOfEventID != "" {
				restored.echoLinks[entry.EchoOfEventID] = append(restored.echoLinks[entry.EchoOfEventID], entry.EventID)
			}
		}
		if entry.InThreadEventID == "" {
			restored.byRoom[entry.RoomID] = append(restored.byRoom[entry.RoomID], index)
		}
	}
	for _, row := range snapshot.GetBodies() {
		id := row.GetMessageEventId()
		if id == "" {
			return fmt.Errorf("room timeline snapshot has empty body message ID")
		}
		if _, duplicate := restored.bodyStates[id]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats body %q", id)
		}
		sequences := row.GetBodyEventSequences()
		if len(sequences) == 0 || sequences[len(sequences)-1] != row.GetCurrentBodySequence() {
			return fmt.Errorf("room timeline snapshot body %q has inconsistent sequence history", id)
		}
		for i, sequence := range sequences {
			if sequence == 0 || (i > 0 && sequence <= sequences[i-1]) {
				return fmt.Errorf("room timeline snapshot body %q has invalid sequence history", id)
			}
		}
		if row.GetCurrentBodyEventId() == "" || row.GetAuthorId() == "" {
			return fmt.Errorf("room timeline snapshot body %q has incomplete current reference", id)
		}
		if !row.GetActive() && row.GetAttachmentCount() != 0 {
			return fmt.Errorf("room timeline snapshot inactive body %q has attachments", id)
		}
		restored.bodyStates[id] = timelineBodyState{
			currentSequence:     row.GetCurrentBodySequence(),
			currentEventID:      row.GetCurrentBodyEventId(),
			authorID:            row.GetAuthorId(),
			attachmentCount:     int(row.GetAttachmentCount()),
			active:              row.GetActive(),
			supersededSequences: append([]uint64(nil), sequences[:len(sequences)-1]...),
		}
	}
	restoreTimes := func(rows []*projectionv1.StringTimestampSnapshot) (map[string]time.Time, error) {
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
	for _, row := range snapshot.GetPinnedMessages() {
		if row.GetPinEventId() == "" || row.GetPinSequence() == 0 || row.GetRoomId() == "" || row.GetMessageEventId() == "" {
			return fmt.Errorf("room timeline snapshot has invalid pinned message")
		}
		pins := restored.pinnedMessagesByRoom[row.GetRoomId()]
		if pins == nil {
			pins = make(map[string]PinnedMessageState)
			restored.pinnedMessagesByRoom[row.GetRoomId()] = pins
		}
		if _, duplicate := pins[row.GetMessageEventId()]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats pinned message %q", row.GetMessageEventId())
		}
		pins[row.GetMessageEventId()] = PinnedMessageState{PinEventID: row.GetPinEventId(), PinSequence: row.GetPinSequence(), RoomID: row.GetRoomId(), MessageEventID: row.GetMessageEventId()}
	}
	for _, row := range snapshot.GetLatestRoomPins() {
		if row.GetRoomId() == "" || row.GetPinEventId() == "" || row.GetPinSequence() == 0 {
			return fmt.Errorf("room timeline snapshot has invalid latest room pin")
		}
		if _, duplicate := restored.latestPinByRoom[row.GetRoomId()]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats latest pin room %q", row.GetRoomId())
		}
		restored.latestPinByRoom[row.GetRoomId()] = latestRoomPinState{PinEventID: row.GetPinEventId(), PinSequence: row.GetPinSequence()}
	}
	for roomID, pins := range restored.pinnedMessagesByRoom {
		latest, ok := restored.latestPinByRoom[roomID]
		if !ok {
			return fmt.Errorf("room timeline snapshot has pins without latest marker for room %q", roomID)
		}
		for _, pin := range pins {
			if pin.PinSequence > latest.PinSequence {
				return fmt.Errorf("room timeline snapshot pin is newer than latest marker for room %q", roomID)
			}
		}
	}
	for messageID, state := range restored.bodyStates {
		if _, retracted := restored.retractedFlags[messageID]; retracted {
			continue
		}
		if _, hidden := restored.hiddenEchoes[messageID]; hidden {
			continue
		}
		entry, ok := restored.entryByEventIDLocked(messageID)
		if !ok || !state.active || state.attachmentCount == 0 {
			continue
		}
		restored.refreshAttachmentMessageLocked(entry.RoomID, messageID)
	}
	p.Lock()
	p.entries, p.byRoom, p.byEventID, p.messagePostsByRoom, p.latestOriginalPostAt, p.replayGuard, p.bodyStates, p.retractedFlags, p.tombstonedAt, p.shreddedAt, p.attachmentMessageIDsByRoom, p.attachmentMessageRoom, p.echoLinks, p.hiddenEchoes, p.shreddedUsers, p.pinnedMessagesByRoom, p.latestPinByRoom = restored.entries, restored.byRoom, restored.byEventID, restored.messagePostsByRoom, restored.latestOriginalPostAt, restored.replayGuard, restored.bodyStates, restored.retractedFlags, restored.tombstonedAt, restored.shreddedAt, restored.attachmentMessageIDsByRoom, restored.attachmentMessageRoom, restored.echoLinks, restored.hiddenEchoes, restored.shreddedUsers, restored.pinnedMessagesByRoom, restored.latestPinByRoom
	p.Unlock()
	return nil
}
