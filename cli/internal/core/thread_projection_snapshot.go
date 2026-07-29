package core

import (
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var threadSnapshotContractID = snapshotContractID("v1", &corev1.ThreadProjectionSnapshot{})

func (*ThreadProjection) SnapshotContractID() string {
	return threadSnapshotContractID
}

func (p *ThreadProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()

	snapshot := &corev1.ThreadProjectionSnapshot{
		ReplayGuard: &corev1.ProjectionReplayGuardSnapshot{
			HighestSequence:   p.replayGuard.highestSeq,
			CompatibilityMode: p.replayGuard.compatibilityMode,
			ReplayComplete:    p.replayGuard.replayComplete,
		},
	}

	threadRoots := make([]string, 0, len(p.byThread))
	for root := range p.byThread {
		threadRoots = append(threadRoots, p.eventIDLocked(root))
	}
	sort.Strings(threadRoots)
	for _, root := range threadRoots {
		thread := &corev1.ThreadSnapshot{RootEventId: root}
		rootRef, _ := p.eventRefLocked(root)
		for _, entry := range p.byThread[rootRef] {
			thread.Entries = append(thread.Entries, &corev1.ThreadTimelineEntrySnapshot{
				EventId:        p.eventIDLocked(entry.Event),
				StreamSequence: entry.Seq,
			})
		}
		snapshot.Threads = append(snapshot.Threads, thread)
	}

	for replyRef := threadEventRef(1); int(replyRef) < len(p.messageToThread); replyRef++ {
		if p.messageToThread[replyRef] == 0 {
			continue
		}
		replyID := p.eventIDLocked(replyRef)
		reply := p.replySummaries[replyRef]
		row := &corev1.ThreadReplySnapshot{
			EventId:           replyID,
			ThreadRootEventId: p.eventIDLocked(p.messageToThread[replyRef]),
		}
		if reply.known {
			row.ActorId = p.userIDLocked(reply.actorID)
			row.Retracted = reply.retracted
			if reply.hasCreatedAt {
				row.CreatedAt = timestamppb.New(time.Unix(reply.createdSeconds, int64(reply.createdNanos)))
			}
		}
		snapshot.Replies = append(snapshot.Replies, row)
	}
	sort.Slice(snapshot.Replies, func(i, j int) bool {
		return snapshot.Replies[i].GetEventId() < snapshot.Replies[j].GetEventId()
	})

	for key, state := range p.followState {
		stateName := ThreadFollowStateFollowing
		if state == 2 {
			stateName = ThreadFollowStateUnfollowed
		}
		snapshot.Follows = append(snapshot.Follows, &corev1.ThreadFollowSnapshot{
			UserId:            p.userIDLocked(key.user),
			RoomId:            p.roomIDLocked(key.room),
			ThreadRootEventId: p.eventIDLocked(key.root),
			State:             string(stateName),
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

	for userRef := threadUserRef(1); int(userRef) < len(p.shreddedUsers); userRef++ {
		if p.shreddedUsers[userRef] {
			snapshot.ShreddedUserIds = append(snapshot.ShreddedUserIds, p.userIDLocked(userRef))
		}
	}
	sort.Strings(snapshot.ShreddedUserIds)
	if p.replayGuard.compatibilityMode {
		snapshot.ReplayGuard.EventIds = sortedMapKeys(p.replayGuard.eventIDs)
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *ThreadProjection) Restore(data []byte) (err error) {
	if len(data) == 0 {
		return nil
	}
	var snapshot corev1.ThreadProjectionSnapshot
	if err := proto.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("unmarshal Thread projection snapshot: %w", err)
	}
	restored := NewThreadProjection()

	for _, thread := range snapshot.GetThreads() {
		root := thread.GetRootEventId()
		if root == "" {
			return fmt.Errorf("Thread projection snapshot has empty thread root")
		}
		rootRef := restored.internEventIDLocked(root)
		if _, exists := restored.byThread[rootRef]; exists {
			return fmt.Errorf("Thread projection snapshot repeats thread %q", root)
		}
		entries := make([]compactThreadTimelineEntry, 0, len(thread.GetEntries()))
		for _, entry := range thread.GetEntries() {
			if entry.GetEventId() == "" || entry.GetStreamSequence() == 0 {
				return fmt.Errorf("Thread projection snapshot has invalid entry in thread %q", root)
			}
			entries = append(entries, compactThreadTimelineEntry{
				Event: restored.internEventIDLocked(entry.GetEventId()),
				Seq:   entry.GetStreamSequence(),
			})
		}
		restored.byThread[rootRef] = entries
	}

	for _, row := range snapshot.GetReplies() {
		replyID := row.GetEventId()
		root := row.GetThreadRootEventId()
		if replyID == "" || root == "" {
			return fmt.Errorf("Thread projection snapshot has invalid reply mapping")
		}
		replyRef := restored.internEventIDLocked(replyID)
		if restored.messageToThread[replyRef] != 0 {
			return fmt.Errorf("Thread projection snapshot repeats reply %q", replyID)
		}
		var createdSeconds int64
		var createdNanos int32
		if row.GetCreatedAt() != nil {
			if err := row.GetCreatedAt().CheckValid(); err != nil {
				return fmt.Errorf("Thread projection snapshot reply %q timestamp: %w", replyID, err)
			}
			at := row.GetCreatedAt().AsTime()
			createdSeconds = at.Unix()
			createdNanos = int32(at.Nanosecond())
		}
		restored.messageToThread[replyRef] = restored.internEventIDLocked(root)
		restored.replySummaries[replyRef] = threadReplySummary{
			actorID:        restored.internUserIDLocked(row.GetActorId()),
			createdSeconds: createdSeconds,
			createdNanos:   createdNanos,
			hasCreatedAt:   row.GetCreatedAt() != nil,
			retracted:      row.GetRetracted(),
			known:          true,
		}
	}

	seenEntries := make(map[threadEventRef]struct{}, len(restored.messageToThread))
	replyCount := 0
	for root, entries := range restored.byThread {
		summary := newThreadSummary()
		for _, entry := range entries {
			if _, duplicate := seenEntries[entry.Event]; duplicate {
				return fmt.Errorf("Thread projection snapshot repeats timeline entry %q", restored.eventIDLocked(entry.Event))
			}
			seenEntries[entry.Event] = struct{}{}
			if restored.messageToThread[entry.Event] != root {
				return fmt.Errorf("Thread projection snapshot entry %q has no matching reply", restored.eventIDLocked(entry.Event))
			}
			replyCount++
		}
		restored.summaryByThread[root] = summary
	}
	mappedReplies := 0
	for _, root := range restored.messageToThread {
		if root != 0 {
			mappedReplies++
		}
	}
	if replyCount != mappedReplies {
		return fmt.Errorf("Thread projection snapshot contains replies outside thread timelines")
	}

	for _, userID := range snapshot.GetShreddedUserIds() {
		if userID == "" {
			return fmt.Errorf("Thread projection snapshot has empty shredded user id")
		}
		userRef := restored.internUserIDLocked(userID)
		if restored.shreddedUsers[userRef] {
			return fmt.Errorf("Thread projection snapshot repeats shredded user %q", userID)
		}
		restored.shreddedUsers[userRef] = true
	}
	for root := range restored.summaryByThread {
		restored.recomputeSummaryLocked(root)
	}

	for _, follow := range snapshot.GetFollows() {
		state := ThreadFollowState(follow.GetState())
		if state != ThreadFollowStateFollowing && state != ThreadFollowStateUnfollowed {
			return fmt.Errorf("Thread projection snapshot has invalid follow state %q", state)
		}
		key := threadFollowKey{
			user: restored.internUserIDLocked(follow.GetUserId()),
			room: restored.internRoomIDLocked(follow.GetRoomId()),
			root: restored.internEventIDLocked(follow.GetThreadRootEventId()),
		}
		if _, duplicate := restored.followState[key]; duplicate {
			return fmt.Errorf("Thread projection snapshot repeats follow state")
		}
		restored.setThreadFollowStateLocked(follow.GetUserId(), follow.GetRoomId(), follow.GetThreadRootEventId(), state)
		if _, stored := restored.followState[key]; !stored {
			return fmt.Errorf("Thread projection snapshot has incomplete follow identity")
		}
	}

	guard := snapshot.GetReplayGuard()
	if guard == nil {
		return fmt.Errorf("Thread projection snapshot is missing replay guard")
	}
	restored.replayGuard.highestSeq = guard.GetHighestSequence()
	restored.replayGuard.replayComplete = guard.GetReplayComplete()
	restored.replayGuard.compatibilityMode = guard.GetCompatibilityMode()
	if restored.replayGuard.compatibilityMode {
		restored.replayGuard.eventIDs = make(eventIDSet, len(guard.GetEventIds()))
		for _, eventID := range guard.GetEventIds() {
			if eventID == "" {
				return fmt.Errorf("Thread projection snapshot has empty compatibility event id")
			}
			if _, duplicate := restored.replayGuard.eventIDs[eventID]; duplicate {
				return fmt.Errorf("Thread projection snapshot repeats compatibility event %q", eventID)
			}
			restored.replayGuard.eventIDs[eventID] = struct{}{}
		}
	} else {
		if len(guard.GetEventIds()) != 0 {
			return fmt.Errorf("Thread projection snapshot has event ids outside compatibility mode")
		}
		if restored.replayGuard.replayComplete {
			restored.replayGuard.eventIDs = nil
		}
	}

	p.Lock()
	p.eventIDs = restored.eventIDs
	p.roomIDs = restored.roomIDs
	p.userIDs = restored.userIDs
	p.byThread = restored.byThread
	p.messageToThread = restored.messageToThread
	p.replySummaries = restored.replySummaries
	p.summaryByThread = restored.summaryByThread
	p.followState = restored.followState
	p.followers = restored.followers
	p.followedByUser = restored.followedByUser
	p.replayGuard = restored.replayGuard
	p.shreddedUsers = restored.shreddedUsers
	p.Unlock()
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
