package core

import (
	"testing"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

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
