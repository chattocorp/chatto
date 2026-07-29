package core

import (
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var roomTimelineSnapshotContractID = snapshotContractID("v2", &corev1.RoomTimelineProjectionSnapshot{})

func (*RoomTimelineProjection) SnapshotContractID() string {
	return roomTimelineSnapshotContractID
}

func (p *RoomTimelineProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()

	snapshot := &corev1.RoomTimelineProjectionSnapshot{ReplayGuard: snapshotReplayGuard(p.replayGuard)}
	for ref := timelineEventRef(1); int(ref) < len(p.messageFlags); ref++ {
		if p.messageFlagLocked(ref, timelineMessageRetracted) {
			snapshot.RetractedEventIds = append(snapshot.RetractedEventIds, p.eventIDLocked(ref))
		}
		if p.messageFlagLocked(ref, timelineMessageHiddenEcho) {
			snapshot.HiddenEchoEventIds = append(snapshot.HiddenEchoEventIds, p.eventIDLocked(ref))
		}
	}
	for ref := timelineUserRef(1); int(ref) < len(p.shreddedUsers); ref++ {
		if p.shreddedUsers[ref] {
			snapshot.ShreddedUserIds = append(snapshot.ShreddedUserIds, p.userIDLocked(ref))
		}
	}
	sort.Strings(snapshot.RetractedEventIds)
	sort.Strings(snapshot.HiddenEchoEventIds)
	sort.Strings(snapshot.ShreddedUserIds)
	residentBuckets := make(map[timelineBucketKey]bool, len(p.buckets))
	bucketKeys := make([]timelineBucketKey, 0, len(p.buckets))
	for key := range p.buckets {
		bucketKeys = append(bucketKeys, key)
	}
	sort.Slice(bucketKeys, func(i, j int) bool {
		if bucketKeys[i].roomID != bucketKeys[j].roomID {
			return bucketKeys[i].roomID < bucketKeys[j].roomID
		}
		return bucketKeys[i].weekStart < bucketKeys[j].weekStart
	})
	for _, key := range bucketKeys {
		bucket := p.buckets[key]
		payloadResident := bucket.resident && p.bucketIsHotLocked(key)
		residentBuckets[key] = payloadResident
		row := &corev1.TimelineBucketSnapshot{
			RoomId:                 key.roomID,
			WeekStartUnix:          key.weekStart,
			EncodedEventReferences: append([]byte(nil), bucket.encodedRefs...),
			PayloadResident:        payloadResident,
		}
		snapshot.Buckets = append(snapshot.Buckets, row)
	}

	visibleIndexes := make(map[timelineEntryRef]struct{})
	for _, indexes := range p.byRoom {
		for _, idx := range indexes {
			visibleIndexes[idx] = struct{}{}
		}
	}
	for index := range p.entries {
		entry := &p.entries[index]
		row := &corev1.TimelineEntryMetadataSnapshot{
			StreamSequence:      entry.StreamSeq,
			EventId:             p.eventIDLocked(entry.eventID),
			RoomId:              p.roomIDLocked(entry.roomID),
			AuthorId:            p.userIDLocked(entry.authorID),
			EchoOriginalEventId: p.eventIDLocked(entry.echoOriginalID),
			WeekStartUnix:       entry.bucket.weekStart,
			MessagePosted:       entry.messagePosted,
		}
		_, row.RoomVisible = visibleIndexes[timelineEntryRef(index)]
		if residentBuckets[entry.bucket] && entry.Event != nil {
			row.ResidentEvent = proto.Clone(entry.Event).(*corev1.Event)
		}
		snapshot.Entries = append(snapshot.Entries, row)
	}
	for ref := timelineEventRef(1); int(ref) < len(p.bodyStates); ref++ {
		state := p.bodyStates[ref]
		if !state.known {
			continue
		}
		row := &corev1.TimelineBodyMetadataSnapshot{
			MessageEventId:      p.eventIDLocked(ref),
			BodyEventSequences:  p.appendBodySequencesLocked(nil, ref, state),
			CurrentBodySequence: state.currentSequence,
			RoomId:              state.bucket.roomID,
			WeekStartUnix:       state.bucket.weekStart,
			HasAttachments:      state.hasAttachments,
		}
		if residentBuckets[state.bucket] && p.currentBodies[ref] != nil {
			row.ResidentBody = cloneMessageBody(p.currentBodies[ref])
		}
		snapshot.Bodies = append(snapshot.Bodies, row)
	}
	sort.Slice(snapshot.Bodies, func(i, j int) bool {
		return snapshot.Bodies[i].GetMessageEventId() < snapshot.Bodies[j].GetMessageEventId()
	})
	appendEventTimes := func(values map[timelineEventRef]time.Time) []*corev1.StringTimestampSnapshot {
		rows := make([]*corev1.StringTimestampSnapshot, 0, len(values))
		for key, value := range values {
			if !value.IsZero() {
				rows = append(rows, &corev1.StringTimestampSnapshot{Key: p.eventIDLocked(key), Value: timestamppb.New(value)})
			}
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].GetKey() < rows[j].GetKey() })
		return rows
	}
	appendUserTimes := func(values map[timelineUserRef]time.Time) []*corev1.StringTimestampSnapshot {
		rows := make([]*corev1.StringTimestampSnapshot, 0, len(values))
		for key, value := range values {
			if !value.IsZero() {
				rows = append(rows, &corev1.StringTimestampSnapshot{Key: p.userIDLocked(key), Value: timestamppb.New(value)})
			}
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].GetKey() < rows[j].GetKey() })
		return rows
	}
	snapshot.TombstonedAt = appendEventTimes(p.tombstonedAt)
	snapshot.ShreddedAt = appendUserTimes(p.shreddedAt)
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
	restored := newRoomTimelineProjection(roomTimelineProjectionOptions{
		eventLoader: p.eventLoader,
		hotWindow:   p.hotWindow,
		now:         p.now,
		retainAll:   p.retainAll,
	})
	restored.replayGuard = guard

	for _, row := range snapshot.GetBuckets() {
		key := timelineBucketKey{roomID: row.GetRoomId(), weekStart: row.GetWeekStartUnix()}
		if key.roomID == "" {
			return fmt.Errorf("room timeline snapshot has bucket without room")
		}
		if _, duplicate := restored.buckets[key]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats bucket %s/%d", key.roomID, key.weekStart)
		}
		bucket := &timelineBucket{
			resident: row.GetPayloadResident() && restored.bucketIsHotLocked(key),
		}
		referenceCount, lastSequence, err := inspectTimelineBucketEventRefs(row.GetEncodedEventReferences())
		if err != nil {
			return fmt.Errorf("room timeline snapshot bucket %s/%d references: %w", key.roomID, key.weekStart, err)
		}
		bucket.encodedRefs = append([]byte(nil), row.GetEncodedEventReferences()...)
		bucket.referenceCount = referenceCount
		bucket.lastSequence = lastSequence
		if bucket.resident {
			bucket.lastAccess = restored.now()
		}
		restored.buckets[key] = bucket
	}

	for _, row := range snapshot.GetEntries() {
		key := timelineBucketKey{roomID: row.GetRoomId(), weekStart: row.GetWeekStartUnix()}
		bucket, bucketExists := restored.buckets[key]
		if row.GetStreamSequence() == 0 || row.GetEventId() == "" || !bucketExists {
			return fmt.Errorf("room timeline snapshot has invalid timeline entry")
		}
		eventRef := restored.internEventIDLocked(row.GetEventId())
		if restored.entryByEvent[eventRef] != missingTimelineEntry {
			return fmt.Errorf("room timeline snapshot repeats event %q", row.GetEventId())
		}
		entry := TimelineEntry{
			StreamSeq:      row.GetStreamSequence(),
			eventID:        eventRef,
			roomID:         restored.internRoomIDLocked(row.GetRoomId()),
			authorID:       restored.internUserIDLocked(row.GetAuthorId()),
			echoOriginalID: restored.internEventIDLocked(row.GetEchoOriginalEventId()),
			bucket:         key,
			messagePosted:  row.GetMessagePosted(),
		}
		if bucket.resident {
			if row.GetResidentEvent() == nil || row.GetResidentEvent().GetId() != row.GetEventId() {
				return fmt.Errorf("room timeline snapshot resident bucket %s/%d is missing event %q", key.roomID, key.weekStart, row.GetEventId())
			}
			entry.Event = proto.Clone(row.GetResidentEvent()).(*corev1.Event)
		}
		index := len(restored.entries)
		restored.entries = append(restored.entries, entry)
		restored.entryByEvent[eventRef] = int32(index)
		if entry.messagePosted {
			restored.messagePostsByRoom[entry.roomID] = append(restored.messagePostsByRoom[entry.roomID], timelineEntryRef(index))
			if entry.echoOriginalID != 0 {
				restored.echoLinks[entry.echoOriginalID] = append(restored.echoLinks[entry.echoOriginalID], entry.eventID)
			}
		}
		if row.GetRoomVisible() {
			restored.byRoom[entry.roomID] = append(restored.byRoom[entry.roomID], timelineEntryRef(index))
		}
	}

	for _, row := range snapshot.GetBodies() {
		id := row.GetMessageEventId()
		key := timelineBucketKey{roomID: row.GetRoomId(), weekStart: row.GetWeekStartUnix()}
		bucket, bucketExists := restored.buckets[key]
		if id == "" || !bucketExists {
			return fmt.Errorf("room timeline snapshot has invalid body metadata")
		}
		eventRef := restored.internEventIDLocked(id)
		if restored.bodyStates[eventRef].known {
			return fmt.Errorf("room timeline snapshot repeats body %q", id)
		}
		sequences := row.GetBodyEventSequences()
		if len(sequences) == 0 || sequences[len(sequences)-1] != row.GetCurrentBodySequence() {
			return fmt.Errorf("room timeline snapshot body %q has inconsistent sequence history", id)
		}
		state := timelineBodyState{
			currentSequence: row.GetCurrentBodySequence(),
			bucket:          key,
			hasAttachments:  row.GetHasAttachments(),
			known:           true,
		}
		if bucket.resident && row.GetResidentBody() != nil {
			restored.currentBodies[eventRef] = cloneMessageBody(row.GetResidentBody())
		}
		restored.bodyStates[eventRef] = state
		if len(sequences) > 1 {
			restored.supersededBodySequences[eventRef] = append([]uint64(nil), sequences[:len(sequences)-1]...)
		}
	}

	restoreEventTimes := func(rows []*corev1.StringTimestampSnapshot) (map[timelineEventRef]time.Time, error) {
		values := make(map[timelineEventRef]time.Time, len(rows))
		for _, row := range rows {
			if row.GetKey() == "" || row.GetValue() == nil {
				return nil, fmt.Errorf("room timeline snapshot has invalid timestamp mapping")
			}
			ref := restored.internEventIDLocked(row.GetKey())
			if _, duplicate := values[ref]; duplicate {
				return nil, fmt.Errorf("room timeline snapshot repeats timestamp key %q", row.GetKey())
			}
			value, err := snapshotTime(row.GetValue())
			if err != nil {
				return nil, err
			}
			values[ref] = value
		}
		return values, nil
	}
	restoreUserTimes := func(rows []*corev1.StringTimestampSnapshot) (map[timelineUserRef]time.Time, error) {
		values := make(map[timelineUserRef]time.Time, len(rows))
		for _, row := range rows {
			if row.GetKey() == "" || row.GetValue() == nil {
				return nil, fmt.Errorf("room timeline snapshot has invalid timestamp mapping")
			}
			ref := restored.internUserIDLocked(row.GetKey())
			if _, duplicate := values[ref]; duplicate {
				return nil, fmt.Errorf("room timeline snapshot repeats timestamp key %q", row.GetKey())
			}
			value, err := snapshotTime(row.GetValue())
			if err != nil {
				return nil, err
			}
			values[ref] = value
		}
		return values, nil
	}
	restored.tombstonedAt, err = restoreEventTimes(snapshot.GetTombstonedAt())
	if err != nil {
		return fmt.Errorf("room timeline tombstones: %w", err)
	}
	restored.shreddedAt, err = restoreUserTimes(snapshot.GetShreddedAt())
	if err != nil {
		return fmt.Errorf("room timeline shred timestamps: %w", err)
	}
	for _, id := range snapshot.GetRetractedEventIds() {
		ref := restored.internEventIDLocked(id)
		if restored.messageFlagLocked(ref, timelineMessageRetracted) {
			return fmt.Errorf("room timeline retracted IDs: repeated set value %q", id)
		}
		restored.setMessageFlagLocked(ref, timelineMessageRetracted, true)
	}
	for _, id := range snapshot.GetHiddenEchoEventIds() {
		ref := restored.internEventIDLocked(id)
		if restored.messageFlagLocked(ref, timelineMessageHiddenEcho) {
			return fmt.Errorf("room timeline hidden echoes: repeated set value %q", id)
		}
		restored.setMessageFlagLocked(ref, timelineMessageHiddenEcho, true)
	}
	for _, id := range snapshot.GetShreddedUserIds() {
		ref := restored.internUserIDLocked(id)
		if restored.shreddedUsers[ref] {
			return fmt.Errorf("room timeline shredded users: repeated set value %q", id)
		}
		restored.shreddedUsers[ref] = true
	}

	for eventIndex, state := range restored.bodyStates {
		if !state.known {
			continue
		}
		eventRef := timelineEventRef(eventIndex)
		messageID := restored.eventIDLocked(eventRef)
		entry, entryExists := restored.entryByEventIDLocked(messageID)
		retracted := restored.messageFlagLocked(eventRef, timelineMessageRetracted)
		hidden := restored.messageFlagLocked(eventRef, timelineMessageHiddenEcho)
		shredded := false
		if entryExists {
			shredded = restored.shreddedUsers[entry.authorID]
		}
		if restored.buckets[state.bucket].resident && restored.currentBodies[eventRef] == nil && !retracted && !hidden && !shredded {
			return fmt.Errorf("room timeline snapshot resident bucket %s/%d is missing body %q", state.bucket.roomID, state.bucket.weekStart, messageID)
		}
		if !entryExists {
			continue
		}
		if state.hasAttachments && !retracted && !hidden && !shredded {
			restored.addAttachmentMessageLocked(restored.roomIDLocked(entry.roomID), messageID, entry.StreamSeq)
		}
		if retracted || hidden || shredded {
			restored.currentBodies[eventRef] = nil
		}
	}
	for index := range restored.entries {
		entry := &restored.entries[index]
		if entry.Event != nil {
			restored.buckets[entry.bucket].residentBytes += int64(proto.Size(entry.Event))
		}
	}
	for eventIndex, state := range restored.bodyStates {
		if state.known && restored.currentBodies[eventIndex] != nil {
			restored.buckets[state.bucket].residentBytes += int64(proto.Size(restored.currentBodies[eventIndex]))
		}
	}

	p.Lock()
	p.entries = restored.entries
	p.eventIDs = restored.eventIDs
	p.roomIDs = restored.roomIDs
	p.userIDs = restored.userIDs
	p.entryByEvent = restored.entryByEvent
	p.byRoom = restored.byRoom
	p.messagePostsByRoom = restored.messagePostsByRoom
	p.replayGuard = restored.replayGuard
	p.bodyStates = restored.bodyStates
	p.currentBodies = restored.currentBodies
	p.supersededBodySequences = restored.supersededBodySequences
	p.tombstonedAt = restored.tombstonedAt
	p.shreddedAt = restored.shreddedAt
	p.attachmentMessagesByRoom = restored.attachmentMessagesByRoom
	p.attachmentMessageRoom = restored.attachmentMessageRoom
	p.echoLinks = restored.echoLinks
	p.messageFlags = restored.messageFlags
	p.shreddedUsers = restored.shreddedUsers
	p.buckets = restored.buckets
	p.Unlock()
	return nil
}
