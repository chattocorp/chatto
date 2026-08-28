package core

import (
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/projection/v1"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
		snapshot.Entries = append(snapshot.Entries, &projectionv1.TimelineEntrySnapshot{StreamSequence: entry.StreamSeq, Event: proto.Clone(entry.Event).(*evtv1.Event)})
	}
	for _, id := range sortedMapKeys(p.bodyStates) {
		state := p.bodyStates[id]
		row := &projectionv1.TimelineBodySnapshot{
			MessageEventId:      id,
			BodyEventSequences:  appendBodySequences(nil, state),
			CurrentBodySequence: state.currentSequence,
		}
		if state.body != nil {
			row.Body = cloneMessageBody(state.body)
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
	for _, row := range snapshot.GetEntries() {
		if row.GetStreamSequence() == 0 || row.GetEvent().GetId() == "" {
			return fmt.Errorf("room timeline snapshot has invalid timeline entry")
		}
		event := proto.Clone(row.GetEvent()).(*evtv1.Event)
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
			if event.GetMessagePosted().GetEchoOfEventId() == "" && event.GetActorId() != "" {
				restored.latestOriginalPostAt[roomActorKey{roomID: roomID, actorID: event.GetActorId()}] = eventCreatedAt(event)
			}
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
		if _, duplicate := restored.bodyStates[id]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats body %q", id)
		}
		sequences := row.GetBodyEventSequences()
		if len(sequences) == 0 || sequences[len(sequences)-1] != row.GetCurrentBodySequence() {
			return fmt.Errorf("room timeline snapshot body %q has inconsistent sequence history", id)
		}
		restored.bodyStates[id] = timelineBodyState{
			body:                cloneMessageBody(row.GetBody()),
			currentSequence:     row.GetCurrentBodySequence(),
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
		if !ok || entry.Event == nil || state.body == nil {
			continue
		}
		roomID := roomIDOfEvent(entry.Event)
		restored.refreshAttachmentMessageLocked(roomID, messageID, state.body)
	}
	p.Lock()
	p.entries, p.byRoom, p.byEventID, p.messagePostsByRoom, p.latestOriginalPostAt, p.replayGuard, p.bodyStates, p.retractedFlags, p.tombstonedAt, p.shreddedAt, p.attachmentMessageIDsByRoom, p.attachmentMessageRoom, p.echoLinks, p.hiddenEchoes, p.shreddedUsers, p.pinnedMessagesByRoom, p.latestPinByRoom = restored.entries, restored.byRoom, restored.byEventID, restored.messagePostsByRoom, restored.latestOriginalPostAt, restored.replayGuard, restored.bodyStates, restored.retractedFlags, restored.tombstonedAt, restored.shreddedAt, restored.attachmentMessageIDsByRoom, restored.attachmentMessageRoom, restored.echoLinks, restored.hiddenEchoes, restored.shreddedUsers, restored.pinnedMessagesByRoom, restored.latestPinByRoom
	p.Unlock()
	return nil
}
