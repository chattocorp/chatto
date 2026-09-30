package core

import (
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/projection/v1"
	"sort"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var threadSnapshotContractID = snapshotContractID("v3", &projectionv1.ThreadProjectionSnapshot{})

func (*ThreadProjection) SnapshotContractID() string {
	return threadSnapshotContractID
}

func (p *ThreadProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()

	snapshot := &projectionv1.ThreadProjectionSnapshot{ReplayGuard: snapshotReplayGuard(p.replayGuard)}
	snapshot.ChannelRoomIds = sortedMapKeys(p.channelRooms)
	for _, roomID := range sortedMapKeys(p.dmRooms) {
		snapshot.DirectMessageRooms = append(snapshot.DirectMessageRooms, &projectionv1.RoomMembershipSnapshot{
			RoomId: roomID, UserIds: sortedMapKeys(p.dmRooms[roomID]),
		})
	}

	for _, root := range sortedHandleKeys(p.eventIDs, p.byThread) {
		thread := &projectionv1.ThreadSnapshot{RootEventId: p.eventIDs.id(root)}
		for _, entry := range p.byThread[root] {
			thread.Entries = append(thread.Entries, &projectionv1.ThreadTimelineEntrySnapshot{
				EventId:        p.eventIDs.id(entry.event),
				StreamSequence: entry.streamSeq,
			})
		}
		snapshot.Threads = append(snapshot.Threads, thread)
	}

	// Replies are ordered by reply ID, independent of their threads.
	for root, entries := range p.byThread {
		for _, entry := range entries {
			row := &projectionv1.ThreadReplySnapshot{
				EventId:           p.eventIDs.id(entry.event),
				ThreadRootEventId: p.eventIDs.id(root),
				ActorId:           p.principalIDs.id(entry.actor),
				Retracted:         entry.retracted,
			}
			if entry.createdAt != 0 {
				row.CreatedAt = timestamppb.New(projectionTime(entry.createdAt))
			}
			snapshot.Replies = append(snapshot.Replies, row)
		}
	}
	sort.Slice(snapshot.Replies, func(i, j int) bool { return snapshot.Replies[i].GetEventId() < snapshot.Replies[j].GetEventId() })

	for key, state := range p.followState {
		snapshot.Follows = append(snapshot.Follows, &projectionv1.ThreadFollowSnapshot{
			UserId:            p.principalIDs.id(key.user),
			RoomId:            p.principalIDs.id(key.room),
			ThreadRootEventId: p.eventIDs.id(key.root),
			State:             string(state.public()),
		})
	}
	sort.Slice(snapshot.Follows, func(i, j int) bool {
		a, b := snapshot.Follows[i], snapshot.Follows[j]
		if a.GetUserId() != b.GetUserId() {
			return a.GetUserId() < b.GetUserId()
		}
		if a.GetRoomId() != b.GetRoomId() {
			return a.GetRoomId() < b.GetRoomId()
		}
		return a.GetThreadRootEventId() < b.GetThreadRootEventId()
	})

	type messageRow struct {
		eventID string
		ref     threadMessageRef
	}
	messages := make([]messageRow, 0, len(p.messageRefs))
	for i, ref := range p.messageRefs {
		if ref.room != 0 {
			messages = append(messages, messageRow{eventID: p.eventIDs.id(uint32(i + 1)), ref: ref})
		}
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].eventID < messages[j].eventID })
	for _, message := range messages {
		snapshot.Messages = append(snapshot.Messages, &projectionv1.ThreadMessageSnapshot{
			EventId: message.eventID, RoomId: p.principalIDs.id(message.ref.room), ThreadRootEventId: p.eventIDs.id(message.ref.root),
		})
	}

	interactions := make([]*projectionv1.ThreadInteractionSnapshot, 0, len(p.interactions))
	for key, room := range p.interactions {
		interactions = append(interactions, &projectionv1.ThreadInteractionSnapshot{
			UserId: p.principalIDs.id(key.user), RoomId: p.principalIDs.id(room), ThreadRootEventId: p.eventIDs.id(key.root),
		})
	}
	sort.Slice(interactions, func(i, j int) bool {
		a, b := interactions[i], interactions[j]
		if a.UserId != b.UserId {
			return a.UserId < b.UserId
		}
		if a.RoomId != b.RoomId {
			return a.RoomId < b.RoomId
		}
		return a.ThreadRootEventId < b.ThreadRootEventId
	})
	snapshot.Interactions = interactions

	snapshot.ShreddedUserIds = sortedMapKeys(p.shreddedUsers)
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

// Restore replaces the projection state with a snapshot. Empty data restores
// an empty projection. A failed restore leaves the current state unchanged; it
// can only add IDs to the append-only event ID table.
func (p *ThreadProjection) Restore(data []byte) error {
	var snapshot projectionv1.ThreadProjectionSnapshot
	if err := proto.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("unmarshal Thread projection snapshot: %w", err)
	}
	// The restored model interns into the same table so handles stay shared
	// with the other ServerContentView components.
	restored := newThreadProjection(p.eventIDs)
	if len(data) > 0 {
		if err := restored.loadSnapshot(&snapshot); err != nil {
			return err
		}
	}
	p.Lock()
	p.threadProjectionState = restored.threadProjectionState
	p.Unlock()
	return nil
}

// loadSnapshot fills an empty, unpublished projection from a snapshot.
func (p *ThreadProjection) loadSnapshot(snapshot *projectionv1.ThreadProjectionSnapshot) error {
	for _, roomID := range snapshot.GetChannelRoomIds() {
		if roomID == "" {
			return fmt.Errorf("Thread projection snapshot has empty channel room id")
		}
		if _, duplicate := p.channelRooms[roomID]; duplicate {
			return fmt.Errorf("Thread projection snapshot repeats channel room %q", roomID)
		}
		p.channelRooms[roomID] = struct{}{}
	}
	for _, room := range snapshot.GetDirectMessageRooms() {
		roomID := room.GetRoomId()
		if roomID == "" {
			return fmt.Errorf("Thread projection snapshot has empty direct-message room id")
		}
		if _, duplicate := p.dmRooms[roomID]; duplicate {
			return fmt.Errorf("Thread projection snapshot repeats direct-message room %q", roomID)
		}
		if _, channel := p.channelRooms[roomID]; channel {
			return fmt.Errorf("Thread projection snapshot room %q has conflicting kinds", roomID)
		}
		members := make(map[string]struct{})
		for _, userID := range room.GetUserIds() {
			if userID == "" {
				return fmt.Errorf("Thread projection snapshot has empty DM member id")
			}
			if _, duplicate := members[userID]; duplicate {
				return fmt.Errorf("Thread projection snapshot repeats DM member")
			}
			members[userID] = struct{}{}
		}
		p.dmRooms[roomID] = members
	}

	for _, message := range snapshot.GetMessages() {
		eventID := message.GetEventId()
		roomID := message.GetRoomId()
		rootID := message.GetThreadRootEventId()
		if eventID == "" || roomID == "" || rootID == "" {
			return fmt.Errorf("Thread projection snapshot has invalid message mapping")
		}
		if !p.isInteractionRoomLocked(roomID) {
			return fmt.Errorf("Thread projection snapshot message %q has unknown room", eventID)
		}
		handle := p.eventIDs.intern(eventID)
		if _, duplicate := p.messageRefs.get(handle); duplicate {
			return fmt.Errorf("Thread projection snapshot repeats message mapping %q", eventID)
		}
		p.messageRefs.set(handle, threadMessageRef{room: p.principalIDs.intern(roomID), root: p.eventIDs.intern(rootID)})
	}

	for _, thread := range snapshot.GetThreads() {
		rootID := thread.GetRootEventId()
		if rootID == "" {
			return fmt.Errorf("Thread projection snapshot has empty thread root")
		}
		root := p.eventIDs.intern(rootID)
		if _, exists := p.byThread[root]; exists {
			return fmt.Errorf("Thread projection snapshot repeats thread %q", rootID)
		}
		entries := make([]threadEntry, 0, len(thread.GetEntries()))
		for _, entry := range thread.GetEntries() {
			if entry.GetEventId() == "" || entry.GetStreamSequence() == 0 {
				return fmt.Errorf("Thread projection snapshot has invalid entry in thread %q", rootID)
			}
			entries = append(entries, threadEntry{event: p.eventIDs.intern(entry.GetEventId()), streamSeq: entry.GetStreamSequence()})
		}
		p.byThread[root] = entries
	}

	// Each timeline entry needs exactly one reply row with the same root. The
	// row supplies the entry's author, time, and visibility.
	type entryLocation struct {
		root  uint32
		index int
	}
	locations := make(map[uint32]entryLocation)
	for root, entries := range p.byThread {
		for i, entry := range entries {
			if _, duplicate := locations[entry.event]; duplicate {
				return fmt.Errorf("Thread projection snapshot repeats timeline entry %q", p.eventIDs.id(entry.event))
			}
			locations[entry.event] = entryLocation{root: root, index: i}
		}
		p.summaryByThread[root] = &threadSummary{}
	}
	for _, row := range snapshot.GetReplies() {
		replyID := row.GetEventId()
		rootID := row.GetThreadRootEventId()
		if replyID == "" || rootID == "" {
			return fmt.Errorf("Thread projection snapshot has invalid reply mapping")
		}
		handle := p.eventIDs.intern(replyID)
		if _, exists := p.replyRoots[handle]; exists {
			return fmt.Errorf("Thread projection snapshot repeats reply %q", replyID)
		}
		location, ok := locations[handle]
		if !ok {
			return fmt.Errorf("Thread projection snapshot contains replies outside thread timelines")
		}
		if location.root != p.eventIDs.intern(rootID) {
			return fmt.Errorf("Thread projection snapshot entry %q has no matching reply", replyID)
		}
		entry := &p.byThread[location.root][location.index]
		entry.actor = p.principalIDs.intern(row.GetActorId())
		entry.retracted = row.GetRetracted()
		if row.GetCreatedAt() != nil {
			if err := row.GetCreatedAt().CheckValid(); err != nil {
				return fmt.Errorf("Thread projection snapshot reply %q timestamp: %w", replyID, err)
			}
			entry.createdAt = projectionUnixNanos(row.GetCreatedAt().AsTime())
		}
		p.replyRoots[handle] = location.root
	}
	if len(p.replyRoots) != len(locations) {
		for handle := range locations {
			if _, ok := p.replyRoots[handle]; !ok {
				return fmt.Errorf("Thread projection snapshot entry %q has no matching reply", p.eventIDs.id(handle))
			}
		}
	}

	for _, userID := range snapshot.GetShreddedUserIds() {
		if userID == "" {
			return fmt.Errorf("Thread projection snapshot has empty shredded user id")
		}
		if _, duplicate := p.shreddedUsers[userID]; duplicate {
			return fmt.Errorf("Thread projection snapshot repeats shredded user %q", userID)
		}
		p.shreddedUsers[userID] = struct{}{}
	}
	for root := range p.summaryByThread {
		p.recomputeSummaryLocked(root)
	}

	for _, follow := range snapshot.GetFollows() {
		state := ThreadFollowState(follow.GetState())
		if state != ThreadFollowStateFollowing && state != ThreadFollowStateUnfollowed {
			return fmt.Errorf("Thread projection snapshot has invalid follow state %q", state)
		}
		if follow.GetUserId() == "" || follow.GetRoomId() == "" || follow.GetThreadRootEventId() == "" {
			return fmt.Errorf("Thread projection snapshot has incomplete follow identity")
		}
		key := threadFollowKey{
			user:               p.principalIDs.intern(follow.GetUserId()),
			threadFollowTarget: threadFollowTarget{room: p.principalIDs.intern(follow.GetRoomId()), root: p.eventIDs.intern(follow.GetThreadRootEventId())},
		}
		if _, duplicate := p.followState[key]; duplicate {
			return fmt.Errorf("Thread projection snapshot repeats follow state")
		}
		p.setThreadFollowStateLocked(follow.GetUserId(), follow.GetRoomId(), follow.GetThreadRootEventId(), state)
	}

	for _, row := range snapshot.GetInteractions() {
		userID := row.GetUserId()
		roomID := row.GetRoomId()
		rootID := row.GetThreadRootEventId()
		if userID == "" || roomID == "" || rootID == "" {
			return fmt.Errorf("Thread projection snapshot has incomplete interaction identity")
		}
		if !p.isInteractionRoomLocked(roomID) {
			return fmt.Errorf("Thread projection snapshot interaction has unknown room %q", roomID)
		}
		room := p.principalIDs.intern(roomID)
		root := p.eventIDs.intern(rootID)
		rootRef, rootExists := p.messageRefs.get(root)
		if !rootExists || rootRef.room != room || rootRef.root != root {
			return fmt.Errorf("Thread projection snapshot interaction has invalid root %q", rootID)
		}
		key := threadInteractionKey{user: p.principalIDs.intern(userID), root: root}
		if _, duplicate := p.interactions[key]; duplicate {
			return fmt.Errorf("Thread projection snapshot repeats interaction")
		}
		p.interactions[key] = room
	}

	if snapshot.GetReplayGuard() == nil {
		return fmt.Errorf("Thread projection snapshot is missing replay guard")
	}
	guard, err := restoreReplayGuard(snapshot.GetReplayGuard())
	if err != nil {
		return fmt.Errorf("Thread projection snapshot replay guard: %w", err)
	}
	p.replayGuard = guard
	return nil
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
