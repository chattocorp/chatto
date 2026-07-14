package core

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const threadSnapshotCompatibilityID = "threads-v2"

func (*ThreadProjection) SnapshotCompatibilityID() string {
	return threadSnapshotCompatibilityID
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

	threadRoots := sortedMapKeys(p.byThread)
	for _, root := range threadRoots {
		thread := &corev1.ThreadSnapshot{RootEventId: root}
		for _, entry := range p.byThread[root] {
			thread.Entries = append(thread.Entries, &corev1.ThreadTimelineEntrySnapshot{
				EventId:        entry.EventID,
				StreamSequence: entry.StreamSeq,
			})
		}
		snapshot.Threads = append(snapshot.Threads, thread)
	}

	replyIDs := sortedMapKeys(p.messageToThread)
	for _, replyID := range replyIDs {
		reply := p.replySummaries[replyID]
		row := &corev1.ThreadReplySnapshot{
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
		snapshot.Follows = append(snapshot.Follows, &corev1.ThreadFollowSnapshot{
			UserId:            parts[0],
			RoomId:            parts[1],
			ThreadRootEventId: parts[2],
			State:             string(p.followState[key]),
		})
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
		replySummaries  map[string]*threadReplySummary
		summaryByThread map[string]*threadSummary
		followState     map[string]ThreadFollowState
		followers       map[string]map[string]struct{}
		followedByUser  map[string]map[string]threadFollowRef
		replayGuard     projectionReplayGuard
		shreddedUsers   map[string]struct{}
	}{p.byThread, p.messageToThread, p.replySummaries, p.summaryByThread, p.followState, p.followers, p.followedByUser, p.replayGuard, p.shreddedUsers}
	defer func() {
		if err == nil {
			return
		}
		p.byThread = previous.byThread
		p.messageToThread = previous.messageToThread
		p.replySummaries = previous.replySummaries
		p.summaryByThread = previous.summaryByThread
		p.followState = previous.followState
		p.followers = previous.followers
		p.followedByUser = previous.followedByUser
		p.replayGuard = previous.replayGuard
		p.shreddedUsers = previous.shreddedUsers
	}()

	p.resetSnapshotStateLocked()

	var snapshot corev1.ThreadProjectionSnapshot
	if err := proto.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("unmarshal Thread projection snapshot: %w", err)
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
