package core

import (
	"errors"
	"fmt"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/stretchr/testify/require"
)

func TestRoomModelThreadParticipantsIncludeRoot(t *testing.T) {
	t.Parallel()
	for _, replyCount := range []int{0, 2, 60} {
		t.Run(fmt.Sprintf("replies-%d", replyCount), func(t *testing.T) {
			timeline := NewRoomTimelineProjection()
			threads := NewThreadProjection()
			model := &RoomModel{timeline: detachedTestProjectionHandle(timeline), threads: detachedTestProjectionHandle(threads)}
			// Historical imports use the mapped message author, not the operator.
			root := postedEvent(postedOpts{envelopeID: "ROOT", eventID: "ROOT", roomID: "R1", actorID: "OPERATOR", at: 1})
			root.GetMessagePosted().AuthorId = "AUTHOR"
			facts := []*evtv1.Event{root, threadCreatedEvent("THREAD", "R1", "ROOT", "AUTHOR", 2)}
			for i := range replyCount {
				id := fmt.Sprintf("REPLY-%02d", i)
				facts = append(facts, postedEvent(postedOpts{envelopeID: id, eventID: id, roomID: "R1", actorID: fmt.Sprintf("U%02d", i), inThread: "ROOT", at: i + 3}))
			}
			applyAll(t, timeline, facts)
			applyAll(t, threads, facts)
			check := func(wantCount, wantReplies int) {
				t.Helper()
				metadata := model.threadMetadata("ROOT")
				require.True(t, metadata.Exists)
				require.Equal(t, wantReplies, metadata.ReplyCount)
				require.Equal(t, wantCount, metadata.ParticipantCount)
				require.Len(t, metadata.ParticipantIDs, min(wantCount, maxThreadParticipants))
				require.Equal(t, "AUTHOR", metadata.ParticipantIDs[0])
				ids := threads.participantIDsIncludingRoot("ROOT", model.threadRootAuthorID("ROOT"))
				require.Len(t, ids, wantCount)
				require.Equal(t, "AUTHOR", ids[0])
			}
			check(replyCount+1, replyCount)
			// A late root-author reply must not increase the participant count,
			// including when that author falls outside the reply preview.
			seq := uint64(len(facts) + 1)
			reply := postedEvent(postedOpts{envelopeID: "AUTHOR-REPLY", eventID: "AUTHOR-REPLY", roomID: "R1", actorID: "AUTHOR", inThread: "ROOT", at: 100})
			require.NoError(t, threads.Apply(reply, seq))
			check(replyCount+1, replyCount+1)
			for _, projection := range []interface {
				Snapshot() ([]byte, error)
				Restore([]byte) error
			}{timeline, threads} {
				snapshot, err := projection.Snapshot()
				require.NoError(t, err)
				require.NoError(t, projection.Restore(snapshot))
			}
			check(replyCount+1, replyCount+1)
			require.NoError(t, threads.Apply(retractedEvent("RETRACT", "AUTHOR-REPLY", "R1", "AUTHOR", "removed", 101), seq+1))
			check(replyCount+1, replyCount)
			shred := userKeyShreddedSnapshotTestEvent("SHRED", "AUTHOR")
			require.NoError(t, timeline.Apply(shred, seq+2))
			require.NoError(t, threads.Apply(shred, seq+2))
			metadata := model.threadMetadata("ROOT")
			require.Equal(t, replyCount, metadata.ParticipantCount)
			require.NotContains(t, metadata.ParticipantIDs, "AUTHOR")
			require.NotContains(t, threads.participantIDsIncludingRoot("ROOT", model.threadRootAuthorID("ROOT")), "AUTHOR")
			require.False(t, model.threadMetadata("MISSING").Exists)
		})
	}
}

func TestNewRoomModelWiresDependencies(t *testing.T) {
	t.Parallel()

	directory := NewRoomDirectoryProjection()
	groupLayout := NewRoomGroupLayoutProjection()
	timeline := NewRoomTimelineProjection()
	threads := NewThreadProjection()
	reactions := NewReactionProjection()
	directoryHandle := detachedTestProjectionHandle(directory)
	groupLayoutHandle := detachedTestProjectionHandle(groupLayout)
	timelineHandle := detachedTestProjectionHandle(timeline)
	threadsHandle := detachedTestProjectionHandle(threads)
	reactionsHandle := detachedTestProjectionHandle(reactions)

	service := newRoomModel(
		directoryHandle,
		groupLayoutHandle,
		timelineHandle,
		threadsHandle,
		reactionsHandle,
	)

	if service.directory.Projection() != directory {
		t.Fatal("directory projection was not wired")
	}
	if service.directory.Projector() != directoryHandle.Projector() {
		t.Fatal("directory projector was not wired")
	}
	if service.groupLayout.Projection() != groupLayout {
		t.Fatal("group layout projection was not wired")
	}
	if service.groupLayout.Projector() != groupLayoutHandle.Projector() {
		t.Fatal("group layout projector was not wired")
	}
	if service.timeline.Projection() != timeline {
		t.Fatal("timeline projection was not wired")
	}
	if service.timeline.Projector() != timelineHandle.Projector() {
		t.Fatal("timeline projector was not wired")
	}
	if service.threads.Projection() != threads {
		t.Fatal("threads projection was not wired")
	}
	if service.threads.Projector() != threadsHandle.Projector() {
		t.Fatal("threads projector was not wired")
	}
	if service.reactions.Projection() != reactions {
		t.Fatal("reactions projection was not wired")
	}
	if service.reactions.Projector() != reactionsHandle.Projector() {
		t.Fatal("reactions projector was not wired")
	}
}

func TestRoomModelAppendDirectoryEventuallyPublishesAndWaits(t *testing.T) {
	t.Parallel()

	harness := newTestEventHarness(t)
	directory := NewRoomDirectoryProjection()
	directoryProjector := harness.projector(directory)
	startTestProjector(t, directoryProjector)
	service := newTestRoomModel(t, directory, directoryProjector, nil, nil, nil, nil, nil, nil, nil, nil)
	ctx := testContext(t)

	event := newEvent(SystemActorID, roomCreatedEvent("R-directory", "directory-room", "Directory", evtv1.RoomKind_ROOM_KIND_CHANNEL))
	pos, err := service.appendDirectoryEventually(ctx, harness.publisher, evtstream.RoomAggregate("R-directory"), event)
	if err != nil {
		t.Fatalf("appendDirectoryEventually returned error: %v", err)
	}

	if pos.Seq == 0 {
		t.Fatal("appendDirectoryEventually returned zero stream sequence")
	}
	room, ok := directory.Catalog.Get("R-directory")
	if !ok {
		t.Fatal("directory catalog did not project appended room")
	}
	if room.GetName() != "directory-room" {
		t.Fatalf("room name = %q, want %q", room.GetName(), "directory-room")
	}
}

func TestRoomModelWaitForDirectoryAndTimeline(t *testing.T) {
	t.Parallel()

	harness := newTestEventHarness(t)
	directory := NewRoomDirectoryProjection()
	directoryProjector := harness.projector(directory)
	startTestProjector(t, directoryProjector)
	timeline := NewRoomTimelineProjection()
	timelineProjector := harness.projector(timeline)
	startTestProjector(t, timelineProjector)
	service := newTestRoomModel(t, directory, directoryProjector, nil, nil, timeline, timelineProjector, nil, nil, nil, nil)
	ctx := testContext(t)

	event := newEvent(SystemActorID, roomCreatedEvent("R-both", "both-room", "", evtv1.RoomKind_ROOM_KIND_CHANNEL))
	subject := evtstream.RoomAggregate("R-both").SubjectFor(event)
	seq, err := harness.publisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}
	if err := service.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("waitForDirectoryAndTimeline returned error: %v", err)
	}

	if _, ok := directory.Catalog.Get("R-both"); !ok {
		t.Fatal("directory catalog did not catch up")
	}
	if got := timeline.RoomEventCount("R-both"); got != 1 {
		t.Fatalf("timeline room event count = %d, want 1", got)
	}
}

func TestRoomModelWaitForTimelineAndThreads(t *testing.T) {
	t.Parallel()

	harness := newTestEventHarness(t)
	timeline := NewRoomTimelineProjection()
	timelineProjector := harness.projector(timeline)
	startTestProjector(t, timelineProjector)
	threads := NewThreadProjection()
	threadsProjector := harness.projector(threads)
	startTestProjector(t, threadsProjector)
	service := newTestRoomModel(t, nil, nil, nil, nil, timeline, timelineProjector, threads, threadsProjector, nil, nil)
	ctx := testContext(t)

	event := newEvent(SystemActorID, &evtv1.Event{
		Event: &evtv1.Event_ThreadCreated{
			ThreadCreated: &evtv1.ThreadCreatedEvent{RoomId: "R-thread", ThreadRootEventId: "E-root"},
		},
	})
	subject := evtstream.RoomAggregate("R-thread").SubjectFor(event)
	seq, err := harness.publisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}
	if err := service.waitForTimelineAndThreads(ctx, events.SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("waitForTimelineAndThreads returned error: %v", err)
	}

	if got := timelineProjector.Status().LastSeq; got < seq {
		t.Fatalf("timeline projector last seq = %d, want at least %d", got, seq)
	}
	if !threads.ThreadExists("E-root") {
		t.Fatal("thread projection did not catch up")
	}
}

func TestRoomModelWaitForLiveEVTEventSkipsThreadsForReaction(t *testing.T) {
	t.Parallel()

	harness := newTestEventHarness(t)
	timeline := NewRoomTimelineProjection()
	timelineProjector := harness.projector(timeline)
	startTestProjector(t, timelineProjector)
	threads := NewThreadProjection()
	threadsProjector := harness.projector(threads)
	startTestProjector(t, threadsProjector)
	reactions := NewReactionProjection()
	reactionsProjector := harness.projector(reactions)
	startTestProjector(t, reactionsProjector)
	service := newTestRoomModel(t, nil, nil, nil, nil, timeline, timelineProjector, threads, threadsProjector, reactions, reactionsProjector)
	ctx := testContext(t)

	event := newEvent("U-reactor", &evtv1.Event{
		Event: &evtv1.Event_ReactionAdded{
			ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "R-live-reaction", MessageEventId: "E-message", Emoji: "wave"},
		},
	})
	subject := evtstream.RoomAggregate("R-live-reaction").SubjectFor(event)
	seq, err := harness.publisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}

	if err := service.waitForLiveEVTEvent(ctx, events.SubjectPosition(subject, seq), event); err != nil {
		t.Fatalf("waitForLiveEVTEvent returned error: %v", err)
	}
	if !reactions.HasReaction("E-message", "wave", "U-reactor") {
		t.Fatal("reaction projection did not catch up")
	}
}

func TestRoomModelWaitForLiveEVTEventSkipsThreadsForCall(t *testing.T) {
	t.Parallel()

	harness := newTestEventHarness(t)
	timeline := NewRoomTimelineProjection()
	timelineProjector := harness.projector(timeline)
	startTestProjector(t, timelineProjector)
	threads := NewThreadProjection()
	threadsProjector := harness.projector(threads)
	startTestProjector(t, threadsProjector)
	service := newTestRoomModel(t, nil, nil, nil, nil, timeline, timelineProjector, threads, threadsProjector, nil, nil)
	ctx := testContext(t)

	event := newEvent("U-caller", &evtv1.Event{
		Event: &evtv1.Event_VoiceCallParticipantJoined{
			VoiceCallParticipantJoined: &evtv1.CallParticipantJoinedEvent{RoomId: "R-live-call", CallId: "C1"},
		},
	})
	subject := evtstream.RoomAggregate("R-live-call").SubjectFor(event)
	seq, err := harness.publisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}

	if err := service.waitForLiveEVTEvent(ctx, events.SubjectPosition(subject, seq), event); err != nil {
		t.Fatalf("waitForLiveEVTEvent returned error: %v", err)
	}
}

func TestRoomModelWaitForThreads(t *testing.T) {
	t.Parallel()

	harness := newTestEventHarness(t)
	threads := NewThreadProjection()
	threadsProjector := harness.projector(threads)
	startTestProjector(t, threadsProjector)
	service := newTestRoomModel(t, nil, nil, nil, nil, nil, nil, threads, threadsProjector, nil, nil)
	ctx := testContext(t)

	event := newEvent(SystemActorID, &evtv1.Event{
		Event: &evtv1.Event_ThreadCreated{
			ThreadCreated: &evtv1.ThreadCreatedEvent{RoomId: "R-thread-direct", ThreadRootEventId: "E-root-direct"},
		},
	})
	subject := evtstream.RoomAggregate("R-thread-direct").SubjectFor(event)
	seq, err := harness.publisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}
	if err := service.waitForThreads(ctx, events.SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("waitForThreads returned error: %v", err)
	}

	if !threads.ThreadExists("E-root-direct") {
		t.Fatal("thread projection did not catch up")
	}
}

func TestRoomModelWaitForReactionsCurrent(t *testing.T) {
	t.Parallel()

	harness := newTestEventHarness(t)
	reactions := NewReactionProjection()
	reactionsProjector := harness.projector(reactions)
	startTestProjector(t, reactionsProjector)
	service := newTestRoomModel(t, nil, nil, nil, nil, nil, nil, nil, nil, reactions, reactionsProjector)
	ctx := testContext(t)

	event := newEvent("U-reactor", &evtv1.Event{
		Event: &evtv1.Event_ReactionAdded{
			ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "R-reactions", MessageEventId: "E-message", Emoji: "wave"},
		},
	})
	if _, err := harness.publisher.AppendEventually(ctx, evtstream.RoomAggregate("R-reactions").SubjectFor(event), event); err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}
	if err := service.waitForReactionsCurrent(ctx, harness.publisher, "R-reactions"); err != nil {
		t.Fatalf("waitForReactionsCurrent returned error: %v", err)
	}

	if !reactions.HasReaction("E-message", "wave", "U-reactor") {
		t.Fatal("reaction projection did not catch up")
	}
}

func TestRoomModelWaitForReactions(t *testing.T) {
	t.Parallel()

	harness := newTestEventHarness(t)
	reactions := NewReactionProjection()
	reactionsProjector := harness.projector(reactions)
	startTestProjector(t, reactionsProjector)
	service := newTestRoomModel(t, nil, nil, nil, nil, nil, nil, nil, nil, reactions, reactionsProjector)
	ctx := testContext(t)

	event := newEvent("U-reactor", &evtv1.Event{
		Event: &evtv1.Event_ReactionAdded{
			ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "R-reactions-direct", MessageEventId: "E-message", Emoji: "sparkles"},
		},
	})
	subject := evtstream.RoomAggregate("R-reactions-direct").SubjectFor(event)
	seq, err := harness.publisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}
	if err := service.waitForReactions(ctx, events.SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("waitForReactions returned error: %v", err)
	}

	if !reactions.HasReaction("E-message", "sparkles", "U-reactor") {
		t.Fatal("reaction projection did not catch up")
	}
}

// Catalog reads must work with only their projection owner and return detached
// values, so a caller cannot change state observed by another service.
func TestRoomModelCatalogReadsWithoutCore(t *testing.T) {
	t.Parallel()
	directory := NewRoomDirectoryProjection()
	groups := NewRoomGroupLayoutProjection()
	model := newRoomModel(detachedTestProjectionHandle(directory), detachedTestProjectionHandle(groups), events.ProjectionHandle[*RoomTimelineProjection]{}, events.ProjectionHandle[*ThreadProjection]{}, events.ProjectionHandle[*ReactionProjection]{})
	ctx := testContext(t)
	if err := directory.Apply(newEvent("actor", roomCreatedEvent("R-one", "original", "", evtv1.RoomKind_ROOM_KIND_CHANNEL)), 1); err != nil {
		t.Fatal(err)
	}
	for i, event := range []*evtv1.Event{
		groupCreatedEvent("G-one", "Group", ""),
		roomAddedToGroupEvent("G-one", "R-one"),
	} {
		if err := groups.Apply(newEvent("actor", event), uint64(i+2)); err != nil {
			t.Fatal(err)
		}
	}
	room, err := model.GetRoom(ctx, KindChannel, "R-one")
	if err != nil || room.GetGroupId() != "G-one" {
		t.Fatalf("group hydration: room=%v err=%v", room, err)
	}
	room.Name = "changed by caller"
	listed, err := model.ListRooms(ctx, KindChannel)
	if err != nil || len(listed) != 1 || listed[0].GetName() != "original" || listed[0].GetGroupId() != "G-one" {
		t.Fatalf("detached room reads: rooms=%v err=%v", listed, err)
	}
	if _, err := model.GetRoom(ctx, KindDM, "R-one"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong kind: %v", err)
	}
	if _, err := model.FindRoomByID(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing room: %v", err)
	}
	group, err := model.GetRoomGroup(ctx, "G-one")
	if err != nil {
		t.Fatal(err)
	}
	group.Name = "changed by caller"
	ordered, err := model.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil || len(ordered) != 1 || ordered[0].GetName() != "Group" {
		t.Fatalf("detached group reads: groups=%v err=%v", ordered, err)
	}
}
