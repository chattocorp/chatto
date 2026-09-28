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

	snapshot := &projectionv1.ThreadProjectionSnapshot{
		ReplayGuard: &projectionv1.ProjectionReplayGuardSnapshot{
			HighestSequence:   p.replayGuard.highestSeq,
			CompatibilityMode: p.replayGuard.compatibilityMode,
			ReplayComplete:    p.replayGuard.replayComplete,
		},
	}
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

	for _, handle := range sortedHandleKeys(p.eventIDs, p.replies) {
		reply := p.replies[handle]
		row := &projectionv1.ThreadReplySnapshot{
			EventId:           p.eventIDs.id(handle),
			ThreadRootEventId: p.eventIDs.id(reply.root),
			ActorId:           p.principalIDs.id(reply.actor),
			Retracted:         reply.retracted,
		}
		if reply.hasCreatedAt {
			row.CreatedAt = timestamppb.New(reply.createdAtTime())
		}
		snapshot.Replies = append(snapshot.Replies, row)
	}

	followKeys := make([]threadFollowStateKey, 0, len(p.followState))
	for key := range p.followState {
		followKeys = append(followKeys, key)
	}
	sort.Slice(followKeys, func(i, j int) bool {
		a, b := followKeys[i], followKeys[j]
		if a.userID != b.userID {
			return a.userID < b.userID
		}
		if a.roomID != b.roomID {
			return a.roomID < b.roomID
		}
		return a.threadRootEventID < b.threadRootEventID
	})
	for _, key := range followKeys {
		snapshot.Follows = append(snapshot.Follows, &projectionv1.ThreadFollowSnapshot{
			UserId:            key.userID,
			RoomId:            key.roomID,
			ThreadRootEventId: key.threadRootEventID,
			State:             string(p.followState[key].public()),
		})
	}

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
	if p.replayGuard.compatibilityMode {
		snapshot.ReplayGuard.EventIds = sortedMapKeys(p.replayGuard.eventIDs)
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *ThreadProjection) Restore(data []byte) (err error) {
	if len(data) == 0 {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	previous := struct {
		byThread        map[uint32][]threadEntry
		replies         map[uint32]threadReply
		channelRooms    map[string]struct{}
		dmRooms         map[string]map[string]struct{}
		principalIDs    projectionIDTable
		messageRefs     handleSlice[threadMessageRef]
		interactions    map[threadInteractionKey]uint32
		summaryByThread map[uint32]*threadSummary
		followState     map[threadFollowStateKey]compactThreadFollowState
		followers       map[threadFollowRef]map[string]struct{}
		followedByUser  map[string]map[threadFollowRef]struct{}
		replayGuard     projectionReplayGuard
		shreddedUsers   map[string]struct{}
	}{p.byThread, p.replies, p.channelRooms, p.dmRooms, p.principalIDs, p.messageRefs, p.interactions, p.summaryByThread, p.followState, p.followers, p.followedByUser, p.replayGuard, p.shreddedUsers}
	defer func() {
		if err == nil {
			return
		}
		p.byThread = previous.byThread
		p.replies = previous.replies
		p.channelRooms = previous.channelRooms
		p.dmRooms = previous.dmRooms
		p.principalIDs = previous.principalIDs
		p.messageRefs = previous.messageRefs
		p.interactions = previous.interactions
		p.summaryByThread = previous.summaryByThread
		p.followState = previous.followState
		p.followers = previous.followers
		p.followedByUser = previous.followedByUser
		p.replayGuard = previous.replayGuard
		p.shreddedUsers = previous.shreddedUsers
	}()

	p.resetSnapshotStateLocked()

	var snapshot projectionv1.ThreadProjectionSnapshot
	if err := proto.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("unmarshal Thread projection snapshot: %w", err)
	}

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

	for _, row := range snapshot.GetReplies() {
		replyID := row.GetEventId()
		rootID := row.GetThreadRootEventId()
		if replyID == "" || rootID == "" {
			return fmt.Errorf("Thread projection snapshot has invalid reply mapping")
		}
		handle := p.eventIDs.intern(replyID)
		if _, exists := p.replies[handle]; exists {
			return fmt.Errorf("Thread projection snapshot repeats reply %q", replyID)
		}
		reply := threadReply{root: p.eventIDs.intern(rootID), actor: p.principalIDs.intern(row.GetActorId()), retracted: row.GetRetracted()}
		if row.GetCreatedAt() != nil {
			if err := row.GetCreatedAt().CheckValid(); err != nil {
				return fmt.Errorf("Thread projection snapshot reply %q timestamp: %w", replyID, err)
			}
			reply.createdAt = row.GetCreatedAt().AsTime().UnixNano()
			reply.hasCreatedAt = true
		}
		p.replies[handle] = reply
	}

	seenEntries := make(map[uint32]struct{}, len(p.replies))
	for root, entries := range p.byThread {
		for _, entry := range entries {
			if _, duplicate := seenEntries[entry.event]; duplicate {
				return fmt.Errorf("Thread projection snapshot repeats timeline entry %q", p.eventIDs.id(entry.event))
			}
			seenEntries[entry.event] = struct{}{}
			if reply, ok := p.replies[entry.event]; !ok || reply.root != root {
				return fmt.Errorf("Thread projection snapshot entry %q has no matching reply", p.eventIDs.id(entry.event))
			}
		}
		p.summaryByThread[root] = newThreadSummary()
	}
	if len(seenEntries) != len(p.replies) {
		return fmt.Errorf("Thread projection snapshot contains replies outside thread timelines")
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
		key := threadFollowStateKey{userID: follow.GetUserId(), threadFollowRef: threadFollowRef{roomID: follow.GetRoomId(), threadRootEventID: follow.GetThreadRootEventId()}}
		if _, duplicate := p.followState[key]; duplicate {
			return fmt.Errorf("Thread projection snapshot repeats follow state")
		}
		p.setThreadFollowStateLocked(follow.GetUserId(), follow.GetRoomId(), follow.GetThreadRootEventId(), state)
		if _, stored := p.followState[key]; !stored {
			return fmt.Errorf("Thread projection snapshot has incomplete follow identity")
		}
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

	guard := snapshot.GetReplayGuard()
	if guard == nil {
		return fmt.Errorf("Thread projection snapshot is missing replay guard")
	}
	p.replayGuard.highestSeq = guard.GetHighestSequence()
	p.replayGuard.replayComplete = guard.GetReplayComplete()
	p.replayGuard.compatibilityMode = guard.GetCompatibilityMode()
	if p.replayGuard.compatibilityMode {
		p.replayGuard.eventIDs = make(eventIDSet, len(guard.GetEventIds()))
		for _, eventID := range guard.GetEventIds() {
			if eventID == "" {
				return fmt.Errorf("Thread projection snapshot has empty compatibility event id")
			}
			if _, duplicate := p.replayGuard.eventIDs[eventID]; duplicate {
				return fmt.Errorf("Thread projection snapshot repeats compatibility event %q", eventID)
			}
			p.replayGuard.eventIDs[eventID] = struct{}{}
		}
	} else {
		if len(guard.GetEventIds()) != 0 {
			return fmt.Errorf("Thread projection snapshot has event ids outside compatibility mode")
		}
		if p.replayGuard.replayComplete {
			p.replayGuard.eventIDs = nil
		}
	}

	return nil
}

func (p *ThreadProjection) resetSnapshotStateLocked() {
	p.byThread = make(map[uint32][]threadEntry)
	p.replies = make(map[uint32]threadReply)
	p.channelRooms = make(map[string]struct{})
	p.dmRooms = make(map[string]map[string]struct{})
	p.principalIDs = newProjectionIDTable()
	// The event ID table is append-only and can be shared with other
	// ServerContentView components, so a restore keeps it.
	p.messageRefs = nil
	p.interactions = make(map[threadInteractionKey]uint32)
	p.summaryByThread = make(map[uint32]*threadSummary)
	p.followState = make(map[threadFollowStateKey]compactThreadFollowState)
	p.followers = make(map[threadFollowRef]map[string]struct{})
	p.followedByUser = make(map[string]map[threadFollowRef]struct{})
	p.replayGuard = newProjectionReplayGuard()
	p.shreddedUsers = make(map[string]struct{})
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
