package core

import (
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNewRoomServiceWiresDependencies(t *testing.T) {
	directory := NewRoomDirectoryProjection()
	directoryProjector := testEventProjector(t)
	groupLayout := NewRoomGroupLayoutProjection()
	groupLayoutProjector := testEventProjector(t)
	timeline := NewRoomTimelineProjection()
	timelineProjector := testEventProjector(t)
	threads := NewThreadProjection()
	threadsProjector := testEventProjector(t)
	reactions := NewReactionProjection()
	reactionsProjector := testEventProjector(t)

	service := newRoomService(
		directory,
		directoryProjector,
		groupLayout,
		groupLayoutProjector,
		timeline,
		timelineProjector,
		threads,
		threadsProjector,
		reactions,
		reactionsProjector,
	)

	if service.directory != directory {
		t.Fatal("directory projection was not wired")
	}
	if service.directoryProjector != directoryProjector {
		t.Fatal("directory projector was not wired")
	}
	if service.groupLayout != groupLayout {
		t.Fatal("group layout projection was not wired")
	}
	if service.groupLayoutProjector != groupLayoutProjector {
		t.Fatal("group layout projector was not wired")
	}
	if service.timeline != timeline {
		t.Fatal("timeline projection was not wired")
	}
	if service.timelineProjector != timelineProjector {
		t.Fatal("timeline projector was not wired")
	}
	if service.threads != threads {
		t.Fatal("threads projection was not wired")
	}
	if service.threadsProjector != threadsProjector {
		t.Fatal("threads projector was not wired")
	}
	if service.reactions != reactions {
		t.Fatal("reactions projection was not wired")
	}
	if service.reactionsProjector != reactionsProjector {
		t.Fatal("reactions projector was not wired")
	}
}

func TestRoomServiceAppendTimelineEventuallyPublishesAndWaits(t *testing.T) {
	harness := newTestEventHarness(t)
	timeline := NewRoomTimelineProjection()
	timelineProjector := harness.projector(timeline)
	startTestProjector(t, timelineProjector)
	service := newRoomService(nil, nil, nil, nil, timeline, timelineProjector, nil, nil, nil, nil)
	ctx := testContext(t)

	event := newEvent(SystemActorID, roomCreatedEvent("R-service", "service-room", "", corev1.RoomKind_ROOM_KIND_CHANNEL))
	pos, err := service.appendTimelineEventually(ctx, harness.publisher, events.RoomAggregate("R-service"), event)
	if err != nil {
		t.Fatalf("appendTimelineEventually returned error: %v", err)
	}

	if pos.Seq == 0 {
		t.Fatal("appendTimelineEventually returned zero stream sequence")
	}
	if got := timeline.RoomEventCount("R-service"); got != 1 {
		t.Fatalf("RoomEventCount = %d, want 1", got)
	}
	entry, ok := timeline.Get(event.GetId())
	if !ok {
		t.Fatal("timeline did not project appended event")
	}
	if entry.StreamSeq != pos.Seq {
		t.Fatalf("projected stream seq = %d, want %d", entry.StreamSeq, pos.Seq)
	}
}

func TestRoomServiceAppendDirectoryEventuallyPublishesAndWaits(t *testing.T) {
	harness := newTestEventHarness(t)
	directory := NewRoomDirectoryProjection()
	directoryProjector := harness.projector(directory)
	startTestProjector(t, directoryProjector)
	service := newRoomService(directory, directoryProjector, nil, nil, nil, nil, nil, nil, nil, nil)
	ctx := testContext(t)

	event := newEvent(SystemActorID, roomCreatedEvent("R-directory", "directory-room", "Directory", corev1.RoomKind_ROOM_KIND_CHANNEL))
	pos, err := service.appendDirectoryEventually(ctx, harness.publisher, events.RoomAggregate("R-directory"), event)
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

func TestRoomServiceAppendGroupLayoutPublishesAndWaits(t *testing.T) {
	harness := newTestEventHarness(t)
	groupLayout := NewRoomGroupLayoutProjection()
	groupLayoutProjector := harness.projector(groupLayout)
	startTestProjector(t, groupLayoutProjector)
	service := newRoomService(nil, nil, groupLayout, groupLayoutProjector, nil, nil, nil, nil, nil, nil)
	ctx := testContext(t)

	created := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_RoomGroupCreated{
			RoomGroupCreated: &corev1.RoomGroupCreatedEvent{GroupId: "G-service", Name: "Service Group"},
		},
	})
	if _, err := service.appendGroupLayoutEventually(ctx, harness.publisher, events.GroupAggregate("G-service"), created); err != nil {
		t.Fatalf("appendGroupLayoutEventually returned error: %v", err)
	}
	group, ok := groupLayout.Groups.Get("G-service")
	if !ok {
		t.Fatal("room group projection did not project appended group")
	}
	if group.GetName() != "Service Group" {
		t.Fatalf("group name = %q, want %q", group.GetName(), "Service Group")
	}

	reordered := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_RoomGroupsReordered{
			RoomGroupsReordered: &corev1.RoomGroupsReorderedEvent{GroupIds: []string{"G-service", "G-other"}},
		},
	})
	if _, err := service.appendGroupLayout(ctx, harness.publisher, events.LayoutAggregate(), reordered); err != nil {
		t.Fatalf("appendGroupLayout returned error: %v", err)
	}
	gotOrder := groupLayout.Layout.Order()
	if len(gotOrder) != 2 || gotOrder[0] != "G-service" || gotOrder[1] != "G-other" {
		t.Fatalf("layout order = %#v, want [G-service G-other]", gotOrder)
	}
}

func TestRoomServiceWaitForDirectoryAndTimeline(t *testing.T) {
	harness := newTestEventHarness(t)
	directory := NewRoomDirectoryProjection()
	directoryProjector := harness.projector(directory)
	startTestProjector(t, directoryProjector)
	timeline := NewRoomTimelineProjection()
	timelineProjector := harness.projector(timeline)
	startTestProjector(t, timelineProjector)
	service := newRoomService(directory, directoryProjector, nil, nil, timeline, timelineProjector, nil, nil, nil, nil)
	ctx := testContext(t)

	event := newEvent(SystemActorID, roomCreatedEvent("R-both", "both-room", "", corev1.RoomKind_ROOM_KIND_CHANNEL))
	subject := events.RoomAggregate("R-both").SubjectFor(event)
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

func TestRoomServiceWaitForTimelineAndThreads(t *testing.T) {
	harness := newTestEventHarness(t)
	timeline := NewRoomTimelineProjection()
	timelineProjector := harness.projector(timeline)
	startTestProjector(t, timelineProjector)
	threads := NewThreadProjection()
	threadsProjector := harness.projector(threads)
	startTestProjector(t, threadsProjector)
	service := newRoomService(nil, nil, nil, nil, timeline, timelineProjector, threads, threadsProjector, nil, nil)
	ctx := testContext(t)

	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_ThreadCreated{
			ThreadCreated: &corev1.ThreadCreatedEvent{RoomId: "R-thread", ThreadRootEventId: "E-root"},
		},
	})
	subject := events.RoomAggregate("R-thread").SubjectFor(event)
	seq, err := harness.publisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}
	if err := service.waitForTimelineAndThreads(ctx, events.SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("waitForTimelineAndThreads returned error: %v", err)
	}

	if got := timeline.RoomEventCount("R-thread"); got != 1 {
		t.Fatalf("timeline room event count = %d, want 1", got)
	}
	if !threads.ThreadExists("E-root") {
		t.Fatal("thread projection did not catch up")
	}
}

func TestRoomServiceWaitForThreads(t *testing.T) {
	harness := newTestEventHarness(t)
	threads := NewThreadProjection()
	threadsProjector := harness.projector(threads)
	startTestProjector(t, threadsProjector)
	service := newRoomService(nil, nil, nil, nil, nil, nil, threads, threadsProjector, nil, nil)
	ctx := testContext(t)

	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_ThreadCreated{
			ThreadCreated: &corev1.ThreadCreatedEvent{RoomId: "R-thread-direct", ThreadRootEventId: "E-root-direct"},
		},
	})
	subject := events.RoomAggregate("R-thread-direct").SubjectFor(event)
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

func TestRoomServiceWaitForReactionsCurrent(t *testing.T) {
	harness := newTestEventHarness(t)
	reactions := NewReactionProjection()
	reactionsProjector := harness.projector(reactions)
	startTestProjector(t, reactionsProjector)
	service := newRoomService(nil, nil, nil, nil, nil, nil, nil, nil, reactions, reactionsProjector)
	ctx := testContext(t)

	event := newEvent("U-reactor", &corev1.Event{
		Event: &corev1.Event_ReactionAdded{
			ReactionAdded: &corev1.ReactionAddedEvent{RoomId: "R-reactions", MessageEventId: "E-message", Emoji: "wave"},
		},
	})
	if _, err := harness.publisher.AppendEventually(ctx, events.RoomAggregate("R-reactions").SubjectFor(event), event); err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}
	if err := service.waitForReactionsCurrent(ctx, harness.publisher, "R-reactions"); err != nil {
		t.Fatalf("waitForReactionsCurrent returned error: %v", err)
	}

	if !reactions.HasReaction("E-message", "wave", "U-reactor") {
		t.Fatal("reaction projection did not catch up")
	}
}

func TestRoomServiceWaitForReactions(t *testing.T) {
	harness := newTestEventHarness(t)
	reactions := NewReactionProjection()
	reactionsProjector := harness.projector(reactions)
	startTestProjector(t, reactionsProjector)
	service := newRoomService(nil, nil, nil, nil, nil, nil, nil, nil, reactions, reactionsProjector)
	ctx := testContext(t)

	event := newEvent("U-reactor", &corev1.Event{
		Event: &corev1.Event_ReactionAdded{
			ReactionAdded: &corev1.ReactionAddedEvent{RoomId: "R-reactions-direct", MessageEventId: "E-message", Emoji: "sparkles"},
		},
	})
	subject := events.RoomAggregate("R-reactions-direct").SubjectFor(event)
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
