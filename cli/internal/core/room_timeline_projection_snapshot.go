package core

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	projectionv1 "hmans.de/chatto/internal/pb/chatto/core/projection/v1"
)

var roomTimelineSnapshotContractID = snapshotContractID("v8", &projectionv1.RoomTimelineProjectionSnapshot{})

func (*RoomTimelineProjection) SnapshotContractID() string {
	return roomTimelineSnapshotContractID
}

func (p *RoomTimelineProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	snapshot := &projectionv1.RoomTimelineProjectionSnapshot{ReplayGuard: snapshotReplayGuard(p.replayGuard), RetractedEventIds: sortedMapKeys(p.retractedFlags), HiddenEchoEventIds: sortedMapKeys(p.hiddenEchoes), ShreddedUserIds: sortedMapKeys(p.shreddedUsers), BucketIntervalNanoseconds: int64(p.bucketInterval)}
	for _, entry := range p.entries {
		snapshot.Entries = append(snapshot.Entries, &projectionv1.TimelineEntrySnapshot{StreamSequence: entry.StreamSeq, Event: proto.Clone(entry.Event).(*evtv1.Event)})
	}
	for _, id := range sortedMapKeys(p.bodyStates) {
		state := p.bodyStates[id]
		row := &projectionv1.TimelineBodyReferenceSnapshot{
			MessageEventId:      id,
			BodyEventSequences:  appendBodySequences(nil, state),
			CurrentBodySequence: state.currentSequence,
			AttachmentCount:     uint32(state.attachmentCount),
			CurrentAssetIds:     append([]string(nil), state.currentAssetIDs...),
		}
		snapshot.BodyReferences = append(snapshot.BodyReferences, row)
	}
	bucketKeys := make([]timelineBucketKey, 0, len(p.buckets))
	for key := range p.buckets {
		bucketKeys = append(bucketKeys, key)
	}
	slices.SortFunc(bucketKeys, func(left, right timelineBucketKey) int {
		if byRoom := strings.Compare(left.roomID, right.roomID); byRoom != 0 {
			return byRoom
		}
		if left.undated != right.undated {
			if left.undated {
				return -1
			}
			return 1
		}
		if left.startUnixNs < right.startUnixNs {
			return -1
		}
		if left.startUnixNs > right.startUnixNs {
			return 1
		}
		return 0
	})
	for _, key := range bucketKeys {
		bucket := p.buckets[key]
		snapshot.Buckets = append(snapshot.Buckets, &projectionv1.TimelineBucketSnapshot{
			RoomId: key.roomID, StartUnixNanoseconds: key.startUnixNs, Undated: key.undated,
			EventSequences: append([]uint64(nil), bucket.sequences...), Revision: bucket.revision,
		})
	}
	for _, messageID := range sortedMapKeys(p.messageBuckets) {
		key := p.messageBuckets[messageID]
		snapshot.MessageBuckets = append(snapshot.MessageBuckets, &projectionv1.MessageBucketSnapshot{
			MessageEventId: messageID, RoomId: key.roomID, StartUnixNanoseconds: key.startUnixNs, Undated: key.undated,
		})
	}
	for _, messageID := range sortedMapKeys(p.pendingBodySequences) {
		snapshot.PendingBodyReferences = append(snapshot.PendingBodyReferences, &projectionv1.PendingBodyReferenceSnapshot{
			MessageEventId: messageID, EventSequences: append([]uint64(nil), p.pendingBodySequences[messageID]...),
		})
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
	} else {
		snapshot.BucketIntervalNanoseconds = int64(p.bucketInterval)
	}
	guard, err := restoreReplayGuard(snapshot.GetReplayGuard())
	if err != nil {
		return fmt.Errorf("room timeline snapshot replay guard: %w", err)
	}
	if snapshot.GetBucketIntervalNanoseconds() <= 0 || time.Duration(snapshot.GetBucketIntervalNanoseconds()) != p.bucketInterval {
		return fmt.Errorf("room timeline snapshot bucket interval %s does not match configured interval %s", time.Duration(snapshot.GetBucketIntervalNanoseconds()), p.bucketInterval)
	}
	restored := NewRoomTimelineProjectionWithOptions(RoomTimelineProjectionOptions{
		EventSource: p.eventSource, Interval: p.bucketInterval, PinnedPeriod: p.pinnedPeriod,
		IdleTimeout: p.idleTimeout, Now: p.now, Logger: p.logger,
	})
	restored.replayGuard = guard
	var lastEntrySequence uint64
	for _, row := range snapshot.GetEntries() {
		if row.GetStreamSequence() == 0 || row.GetStreamSequence() <= lastEntrySequence || row.GetEvent().GetId() == "" {
			return fmt.Errorf("room timeline snapshot has invalid timeline entry")
		}
		lastEntrySequence = row.GetStreamSequence()
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
	for _, row := range snapshot.GetBodyReferences() {
		id := row.GetMessageEventId()
		if id == "" {
			return fmt.Errorf("room timeline snapshot has empty body message ID")
		}
		if _, duplicate := restored.bodyStates[id]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats body %q", id)
		}
		sequences := row.GetBodyEventSequences()
		if len(sequences) == 0 || sequences[0] == 0 || sequences[len(sequences)-1] != row.GetCurrentBodySequence() {
			return fmt.Errorf("room timeline snapshot body %q has inconsistent sequence history", id)
		}
		for index := 1; index < len(sequences); index++ {
			if sequences[index] <= sequences[index-1] {
				return fmt.Errorf("room timeline snapshot body %q has unordered sequence history", id)
			}
		}
		attachmentCount := int(row.GetAttachmentCount())
		assetIDs := append([]string(nil), row.GetCurrentAssetIds()...)
		if len(assetIDs) > 0 && len(assetIDs) != attachmentCount {
			return fmt.Errorf("room timeline snapshot body %q has inconsistent asset metadata", id)
		}
		for _, assetID := range assetIDs {
			if assetID == "" {
				return fmt.Errorf("room timeline snapshot body %q has an empty asset ID", id)
			}
		}
		restored.bodyStates[id] = timelineBodyState{
			currentSequence:     row.GetCurrentBodySequence(),
			supersededSequences: append([]uint64(nil), sequences[:len(sequences)-1]...),
			attachmentCount:     attachmentCount,
			currentAssetIDs:     assetIDs,
		}
	}
	for _, row := range snapshot.GetBuckets() {
		key := timelineBucketKey{roomID: row.GetRoomId(), startUnixNs: row.GetStartUnixNanoseconds(), undated: row.GetUndated()}
		if key.roomID == "" || (key.undated && key.startUnixNs != 0) || row.GetRevision() != uint64(len(row.GetEventSequences())) || len(row.GetEventSequences()) == 0 {
			return fmt.Errorf("room timeline snapshot has invalid bucket")
		}
		if !key.undated && restored.bucketKeyLocked(key.roomID, time.Unix(0, key.startUnixNs)) != key {
			return fmt.Errorf("room timeline snapshot has an unaligned bucket for room %q", key.roomID)
		}
		for index := 1; index < len(row.GetEventSequences()); index++ {
			if row.GetEventSequences()[index] <= row.GetEventSequences()[index-1] {
				return fmt.Errorf("room timeline snapshot bucket for room %q has unordered sequences", key.roomID)
			}
		}
		if _, duplicate := restored.buckets[key]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats bucket for room %q", key.roomID)
		}
		restored.buckets[key] = &timelineBucketState{sequences: append([]uint64(nil), row.GetEventSequences()...), revision: row.GetRevision()}
	}
	for _, row := range snapshot.GetMessageBuckets() {
		key := timelineBucketKey{roomID: row.GetRoomId(), startUnixNs: row.GetStartUnixNanoseconds(), undated: row.GetUndated()}
		entry, entryExists := restored.entryByEventIDLocked(row.GetMessageEventId())
		if row.GetMessageEventId() == "" || key.roomID == "" || restored.buckets[key] == nil || !entryExists || entry.Event.GetMessagePosted() == nil || roomIDOfEvent(entry.Event) != key.roomID || restored.bucketKeyLocked(key.roomID, eventCreatedAt(entry.Event)) != key {
			return fmt.Errorf("room timeline snapshot has invalid message bucket")
		}
		if _, duplicate := restored.messageBuckets[row.GetMessageEventId()]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats message bucket %q", row.GetMessageEventId())
		}
		restored.messageBuckets[row.GetMessageEventId()] = key
		messages := restored.bucketMessages[key]
		if messages == nil {
			messages = make(map[string]struct{})
			restored.bucketMessages[key] = messages
		}
		messages[row.GetMessageEventId()] = struct{}{}
	}
	for _, row := range snapshot.GetPendingBodyReferences() {
		if row.GetMessageEventId() == "" || len(row.GetEventSequences()) == 0 {
			return fmt.Errorf("room timeline snapshot has invalid pending body reference")
		}
		if _, duplicate := restored.pendingBodySequences[row.GetMessageEventId()]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats pending body reference %q", row.GetMessageEventId())
		}
		for index, sequence := range row.GetEventSequences() {
			if sequence == 0 || (index > 0 && sequence <= row.GetEventSequences()[index-1]) {
				return fmt.Errorf("room timeline snapshot has invalid pending body sequence")
			}
		}
		restored.pendingBodySequences[row.GetMessageEventId()] = append([]uint64(nil), row.GetEventSequences()...)
	}
	for messageID, state := range restored.bodyStates {
		key, mapped := restored.messageBuckets[messageID]
		pending, hasPending := restored.pendingBodySequences[messageID]
		if mapped == hasPending {
			return fmt.Errorf("room timeline snapshot body %q must have exactly one bucket association", messageID)
		}
		indexed := pending
		if mapped {
			indexed = restored.buckets[key].sequences
		}
		for _, sequence := range appendBodySequences(nil, state) {
			if _, found := slices.BinarySearch(indexed, sequence); !found {
				return fmt.Errorf("room timeline snapshot body %q has unindexed sequence %d", messageID, sequence)
			}
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
	for roomID, entryIndexes := range restored.messagePostsByRoom {
		for _, entryIndex := range entryIndexes {
			entry := restored.entryAtLocked(entryIndex)
			messageID := entry.Event.GetId()
			if restored.bodyStates[messageID].attachmentCount <= 0 || restored.messageBodyUnavailableLocked(messageID) {
				continue
			}
			restored.attachmentMessageIDsByRoom[roomID] = append(restored.attachmentMessageIDsByRoom[roomID], messageID)
			restored.attachmentMessageRoom[messageID] = roomID
		}
	}
	p.Lock()
	p.entries, p.byRoom, p.byEventID, p.messagePostsByRoom, p.latestOriginalPostAt, p.replayGuard, p.bodyStates, p.retractedFlags, p.tombstonedAt, p.shreddedAt, p.attachmentMessageIDsByRoom, p.attachmentMessageRoom, p.echoLinks, p.hiddenEchoes, p.shreddedUsers, p.pinnedMessagesByRoom, p.latestPinByRoom = restored.entries, restored.byRoom, restored.byEventID, restored.messagePostsByRoom, restored.latestOriginalPostAt, restored.replayGuard, restored.bodyStates, restored.retractedFlags, restored.tombstonedAt, restored.shreddedAt, restored.attachmentMessageIDsByRoom, restored.attachmentMessageRoom, restored.echoLinks, restored.hiddenEchoes, restored.shreddedUsers, restored.pinnedMessagesByRoom, restored.latestPinByRoom
	p.buckets, p.messageBuckets, p.pendingBodySequences, p.bucketMessages, p.cache = restored.buckets, restored.messageBuckets, restored.pendingBodySequences, restored.bucketMessages, restored.cache
	p.Unlock()
	return nil
}
