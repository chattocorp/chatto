package core

import (
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/projection/v1"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var threadSnapshotContractID = snapshotContractID("v2", &projectionv1.ThreadProjectionSnapshot{})

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

	threadRoots := sortedMapKeys(p.byThread)
	for _, root := range threadRoots {
		thread := &projectionv1.ThreadSnapshot{RootEventId: root}
		for _, entry := range p.byThread[root] {
			thread.Entries = append(thread.Entries, &projectionv1.ThreadTimelineEntrySnapshot{
				EventId:        entry.EventID,
				StreamSequence: entry.StreamSeq,
			})
		}
		snapshot.Threads = append(snapshot.Threads, thread)
	}

	replyIDs := sortedMapKeys(p.messageToThread)
	for _, replyID := range replyIDs {
		reply := p.replySummaries[replyID]
		row := &projectionv1.ThreadReplySnapshot{
			EventId:           replyID,
			ThreadRootEventId: p.messageToThread[replyID],
		}
		if reply != nil {
			row.ActorId = reply.actorID
			row.Retracted = reply.retracted
			if !reply.createdAt.IsZero() {
				row.CreatedAt = timestamppb.New(reply.createdAt)
			}
		}
		snapshot.Replies = append(snapshot.Replies, row)
	}

	followKeys := sortedMapKeys(p.followState)
	for _, key := range followKeys {
		parts := strings.SplitN(key, "\x00", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid thread follow key in projection")
		}
		snapshot.Follows = append(snapshot.Follows, &projectionv1.ThreadFollowSnapshot{
			UserId:            parts[0],
			RoomId:            parts[1],
			ThreadRootEventId: parts[2],
			State:             string(p.followState[key]),
		})
	}

	messageIDs := sortedMapKeys(p.messageThreads)
	for _, eventID := range messageIDs {
		ref := p.messageThreads[eventID]
		snapshot.Messages = append(snapshot.Messages, &projectionv1.ThreadMessageSnapshot{
			EventId: eventID, RoomId: ref.roomID, ThreadRootEventId: ref.threadRootEventID,
		})
	}

	userIDs := sortedMapKeys(p.interactions)
	for _, userID := range userIDs {
		byThread := p.interactions[userID]
		keys := sortedMapKeys(byThread)
		for _, key := range keys {
			interaction := byThread[key]
			if interaction == nil {
				return nil, fmt.Errorf("nil thread interaction in projection")
			}
			causeKeys := make([]string, 0, len(interaction.causes))
			for causeKey := range interaction.causes {
				causeKeys = append(causeKeys, causeKey)
			}
			sort.Strings(causeKeys)
			for _, causeKey := range causeKeys {
				cause := interaction.causes[causeKey]
				row := &projectionv1.ThreadInteractionSnapshot{
					UserId: userID, RoomId: interaction.roomID, ThreadRootEventId: interaction.threadRootEventID,
					Cause: string(cause.Kind), SourceEventId: cause.SourceEventID,
				}
				if !cause.CreatedAt.IsZero() {
					row.CreatedAt = timestamppb.New(cause.CreatedAt)
				}
				snapshot.Interactions = append(snapshot.Interactions, row)
			}
		}
	}

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
		byThread        map[string][]ThreadTimelineEntry
		messageToThread map[string]string
		channelRooms    map[string]struct{}
		messageThreads  map[string]threadMessageRef
		interactions    map[string]map[string]*projectedThreadInteraction
		replySummaries  map[string]*threadReplySummary
		summaryByThread map[string]*threadSummary
		followState     map[string]ThreadFollowState
		followers       map[string]map[string]struct{}
		followedByUser  map[string]map[string]threadFollowRef
		replayGuard     projectionReplayGuard
		shreddedUsers   map[string]struct{}
	}{p.byThread, p.messageToThread, p.channelRooms, p.messageThreads, p.interactions, p.replySummaries, p.summaryByThread, p.followState, p.followers, p.followedByUser, p.replayGuard, p.shreddedUsers}
	defer func() {
		if err == nil {
			return
		}
		p.byThread = previous.byThread
		p.messageToThread = previous.messageToThread
		p.channelRooms = previous.channelRooms
		p.messageThreads = previous.messageThreads
		p.interactions = previous.interactions
		p.replySummaries = previous.replySummaries
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

	for _, message := range snapshot.GetMessages() {
		eventID := message.GetEventId()
		roomID := message.GetRoomId()
		rootID := message.GetThreadRootEventId()
		if eventID == "" || roomID == "" || rootID == "" {
			return fmt.Errorf("Thread projection snapshot has invalid message mapping")
		}
		if _, channel := p.channelRooms[roomID]; !channel {
			return fmt.Errorf("Thread projection snapshot message %q has unknown channel room", eventID)
		}
		if _, duplicate := p.messageThreads[eventID]; duplicate {
			return fmt.Errorf("Thread projection snapshot repeats message mapping %q", eventID)
		}
		p.messageThreads[eventID] = threadMessageRef{roomID: roomID, threadRootEventID: rootID}
	}

	for _, thread := range snapshot.GetThreads() {
		root := thread.GetRootEventId()
		if root == "" {
			return fmt.Errorf("Thread projection snapshot has empty thread root")
		}
		if _, exists := p.byThread[root]; exists {
			return fmt.Errorf("Thread projection snapshot repeats thread %q", root)
		}
		entries := make([]ThreadTimelineEntry, 0, len(thread.GetEntries()))
		for _, entry := range thread.GetEntries() {
			if entry.GetEventId() == "" || entry.GetStreamSequence() == 0 {
				return fmt.Errorf("Thread projection snapshot has invalid entry in thread %q", root)
			}
			entries = append(entries, ThreadTimelineEntry{EventID: entry.GetEventId(), StreamSeq: entry.GetStreamSequence()})
		}
		p.byThread[root] = entries
	}

	for _, row := range snapshot.GetReplies() {
		replyID := row.GetEventId()
		root := row.GetThreadRootEventId()
		if replyID == "" || root == "" {
			return fmt.Errorf("Thread projection snapshot has invalid reply mapping")
		}
		if _, exists := p.messageToThread[replyID]; exists {
			return fmt.Errorf("Thread projection snapshot repeats reply %q", replyID)
		}
		var createdAt time.Time
		if row.GetCreatedAt() != nil {
			if err := row.GetCreatedAt().CheckValid(); err != nil {
				return fmt.Errorf("Thread projection snapshot reply %q timestamp: %w", replyID, err)
			}
			createdAt = row.GetCreatedAt().AsTime()
		}
		p.messageToThread[replyID] = root
		p.replySummaries[replyID] = &threadReplySummary{actorID: row.GetActorId(), createdAt: createdAt, retracted: row.GetRetracted()}
	}

	seenEntries := make(map[string]struct{}, len(p.messageToThread))
	for root, entries := range p.byThread {
		summary := newThreadSummary()
		for _, entry := range entries {
			if _, duplicate := seenEntries[entry.EventID]; duplicate {
				return fmt.Errorf("Thread projection snapshot repeats timeline entry %q", entry.EventID)
			}
			seenEntries[entry.EventID] = struct{}{}
			if mappedRoot, ok := p.messageToThread[entry.EventID]; !ok || mappedRoot != root {
				return fmt.Errorf("Thread projection snapshot entry %q has no matching reply", entry.EventID)
			}
			summary.replyIDs = append(summary.replyIDs, entry.EventID)
		}
		p.summaryByThread[root] = summary
	}
	if len(seenEntries) != len(p.messageToThread) {
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
		key := follow.GetUserId() + "\x00" + threadFollowKeyPart(follow.GetRoomId(), follow.GetThreadRootEventId())
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
		kind := ThreadInteractionCauseKind(row.GetCause())
		if userID == "" || roomID == "" || rootID == "" || row.GetSourceEventId() == "" {
			return fmt.Errorf("Thread projection snapshot has incomplete interaction identity")
		}
		if kind != ThreadInteractionCauseRootAuthored && kind != ThreadInteractionCauseDirectMention {
			return fmt.Errorf("Thread projection snapshot has invalid interaction cause %q", kind)
		}
		if _, channel := p.channelRooms[roomID]; !channel {
			return fmt.Errorf("Thread projection snapshot interaction has unknown channel room %q", roomID)
		}
		rootRef, rootExists := p.messageThreads[rootID]
		if !rootExists || rootRef.roomID != roomID || rootRef.threadRootEventID != rootID {
			return fmt.Errorf("Thread projection snapshot interaction has invalid root %q", rootID)
		}
		sourceRef, sourceExists := p.messageThreads[row.GetSourceEventId()]
		if !sourceExists || sourceRef.roomID != roomID || sourceRef.threadRootEventID != rootID {
			return fmt.Errorf("Thread projection snapshot interaction has invalid source %q", row.GetSourceEventId())
		}
		var createdAt time.Time
		if row.GetCreatedAt() != nil {
			if err := row.GetCreatedAt().CheckValid(); err != nil {
				return fmt.Errorf("Thread projection snapshot interaction timestamp: %w", err)
			}
			createdAt = row.GetCreatedAt().AsTime()
		}
		key := threadFollowKeyPart(roomID, rootID)
		if existing := p.interactions[userID][key]; existing != nil {
			causeKey := string(kind) + "\x00" + row.GetSourceEventId()
			if _, duplicate := existing.causes[causeKey]; duplicate {
				return fmt.Errorf("Thread projection snapshot repeats interaction cause")
			}
		}
		p.addInteractionCauseLocked(userID, roomID, rootID, ThreadInteractionCause{
			Kind: kind, SourceEventID: row.GetSourceEventId(), CreatedAt: createdAt,
		})
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
	p.byThread = make(map[string][]ThreadTimelineEntry)
	p.messageToThread = make(map[string]string)
	p.channelRooms = make(map[string]struct{})
	p.messageThreads = make(map[string]threadMessageRef)
	p.interactions = make(map[string]map[string]*projectedThreadInteraction)
	p.replySummaries = make(map[string]*threadReplySummary)
	p.summaryByThread = make(map[string]*threadSummary)
	p.followState = make(map[string]ThreadFollowState)
	p.followers = make(map[string]map[string]struct{})
	p.followedByUser = make(map[string]map[string]threadFollowRef)
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
