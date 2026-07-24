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

	snapshot := &corev1.RoomTimelineProjectionSnapshot{
		ReplayGuard:        snapshotReplayGuard(p.replayGuard),
		RetractedEventIds:  sortedMapKeys(p.retractedFlags),
		HiddenEchoEventIds: sortedMapKeys(p.hiddenEchoes),
		ShreddedUserIds:    sortedMapKeys(p.shreddedUsers),
	}
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
			RoomId:          key.roomID,
			WeekStartUnix:   key.weekStart,
			PayloadResident: payloadResident,
		}
		for _, ref := range bucket.refs {
			row.EventReferences = append(row.EventReferences, &corev1.TimelineBucketEventReferenceSnapshot{
				StreamSequence: ref.sequence,
				OptionalBody:   ref.optionalBody,
			})
		}
		snapshot.Buckets = append(snapshot.Buckets, row)
	}

	visibleIndexes := make(map[int]struct{})
	for _, indexes := range p.byRoom {
		for _, idx := range indexes {
			visibleIndexes[idx] = struct{}{}
		}
	}
	for index := range p.entries {
		entry := &p.entries[index]
		row := &corev1.TimelineEntryMetadataSnapshot{
			StreamSequence:      entry.StreamSeq,
			EventId:             entry.eventID,
			RoomId:              entry.roomID,
			AuthorId:            entry.authorID,
			EchoOriginalEventId: entry.echoOriginalID,
			WeekStartUnix:       entry.bucket.weekStart,
			MessagePosted:       entry.messagePosted,
		}
		_, row.RoomVisible = visibleIndexes[index]
		if residentBuckets[entry.bucket] && entry.Event != nil {
			row.ResidentEvent = proto.Clone(entry.Event).(*corev1.Event)
		}
		snapshot.Entries = append(snapshot.Entries, row)
	}
	for _, id := range sortedMapKeys(p.bodyStates) {
		state := p.bodyStates[id]
		row := &corev1.TimelineBodyMetadataSnapshot{
			MessageEventId:      id,
			BodyEventSequences:  appendBodySequences(nil, state),
			CurrentBodySequence: state.currentSequence,
			RoomId:              state.bucket.roomID,
			WeekStartUnix:       state.bucket.weekStart,
			HasAttachments:      state.hasAttachments,
		}
		if residentBuckets[state.bucket] && state.body != nil {
			row.ResidentBody = cloneMessageBody(state.body)
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
		if bucket.resident {
			bucket.lastAccess = restored.now()
		}
		for _, ref := range row.GetEventReferences() {
			if ref.GetStreamSequence() == 0 {
				return fmt.Errorf("room timeline snapshot bucket %s/%d has zero sequence", key.roomID, key.weekStart)
			}
			bucket.refs = append(bucket.refs, timelineBucketEventRef{
				sequence:     ref.GetStreamSequence(),
				optionalBody: ref.GetOptionalBody(),
			})
		}
		restored.buckets[key] = bucket
	}

	for _, row := range snapshot.GetEntries() {
		key := timelineBucketKey{roomID: row.GetRoomId(), weekStart: row.GetWeekStartUnix()}
		bucket, bucketExists := restored.buckets[key]
		if row.GetStreamSequence() == 0 || row.GetEventId() == "" || !bucketExists {
			return fmt.Errorf("room timeline snapshot has invalid timeline entry")
		}
		entry := TimelineEntry{
			StreamSeq:      row.GetStreamSequence(),
			eventID:        row.GetEventId(),
			roomID:         row.GetRoomId(),
			authorID:       row.GetAuthorId(),
			echoOriginalID: row.GetEchoOriginalEventId(),
			bucket:         key,
			messagePosted:  row.GetMessagePosted(),
		}
		if bucket.resident {
			if row.GetResidentEvent() == nil || row.GetResidentEvent().GetId() != entry.eventID {
				return fmt.Errorf("room timeline snapshot resident bucket %s/%d is missing event %q", key.roomID, key.weekStart, entry.eventID)
			}
			entry.Event = proto.Clone(row.GetResidentEvent()).(*corev1.Event)
		}
		index := len(restored.entries)
		restored.entries = append(restored.entries, entry)
		if _, duplicate := restored.byEventID[entry.eventID]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats event %q", entry.eventID)
		}
		restored.byEventID[entry.eventID] = index
		if entry.messagePosted {
			restored.messagePostsByRoom[entry.roomID] = append(restored.messagePostsByRoom[entry.roomID], index)
			if entry.echoOriginalID != "" {
				restored.echoLinks[entry.echoOriginalID] = append(restored.echoLinks[entry.echoOriginalID], entry.eventID)
			}
		}
		if row.GetRoomVisible() {
			restored.byRoom[entry.roomID] = append(restored.byRoom[entry.roomID], index)
		}
	}

	for _, row := range snapshot.GetBodies() {
		id := row.GetMessageEventId()
		key := timelineBucketKey{roomID: row.GetRoomId(), weekStart: row.GetWeekStartUnix()}
		bucket, bucketExists := restored.buckets[key]
		if id == "" || !bucketExists {
			return fmt.Errorf("room timeline snapshot has invalid body metadata")
		}
		if _, duplicate := restored.bodyStates[id]; duplicate {
			return fmt.Errorf("room timeline snapshot repeats body %q", id)
		}
		sequences := row.GetBodyEventSequences()
		if len(sequences) == 0 || sequences[len(sequences)-1] != row.GetCurrentBodySequence() {
			return fmt.Errorf("room timeline snapshot body %q has inconsistent sequence history", id)
		}
		state := timelineBodyState{
			currentSequence:     row.GetCurrentBodySequence(),
			supersededSequences: append([]uint64(nil), sequences[:len(sequences)-1]...),
			bucket:              key,
			hasAttachments:      row.GetHasAttachments(),
		}
		if bucket.resident && row.GetResidentBody() != nil {
			state.body = cloneMessageBody(row.GetResidentBody())
		}
		restored.bodyStates[id] = state
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

	for messageID, state := range restored.bodyStates {
		entry, entryExists := restored.entryByEventIDLocked(messageID)
		_, retracted := restored.retractedFlags[messageID]
		_, hidden := restored.hiddenEchoes[messageID]
		shredded := false
		if entryExists {
			_, shredded = restored.shreddedUsers[entry.authorID]
		}
		if restored.buckets[state.bucket].resident && state.body == nil && !retracted && !hidden && !shredded {
			return fmt.Errorf("room timeline snapshot resident bucket %s/%d is missing body %q", state.bucket.roomID, state.bucket.weekStart, messageID)
		}
		if !entryExists {
			continue
		}
		if state.hasAttachments && !retracted && !hidden && !shredded {
			restored.addAttachmentMessageLocked(entry.roomID, messageID, entry.StreamSeq)
		}
		if retracted || hidden || shredded {
			state.body = nil
			restored.bodyStates[messageID] = state
		}
	}
	for index := range restored.entries {
		entry := &restored.entries[index]
		if entry.Event != nil {
			restored.buckets[entry.bucket].residentBytes += int64(proto.Size(entry.Event))
		}
	}
	for _, state := range restored.bodyStates {
		if state.body != nil {
			restored.buckets[state.bucket].residentBytes += int64(proto.Size(state.body))
		}
	}

	p.Lock()
	p.entries = restored.entries
	p.byRoom = restored.byRoom
	p.byEventID = restored.byEventID
	p.messagePostsByRoom = restored.messagePostsByRoom
	p.replayGuard = restored.replayGuard
	p.bodyStates = restored.bodyStates
	p.retractedFlags = restored.retractedFlags
	p.tombstonedAt = restored.tombstonedAt
	p.shreddedAt = restored.shreddedAt
	p.attachmentMessageIDsByRoom = restored.attachmentMessageIDsByRoom
	p.attachmentMessageRoom = restored.attachmentMessageRoom
	p.echoLinks = restored.echoLinks
	p.hiddenEchoes = restored.hiddenEchoes
	p.shreddedUsers = restored.shreddedUsers
	p.buckets = restored.buckets
	p.Unlock()
	return nil
}
