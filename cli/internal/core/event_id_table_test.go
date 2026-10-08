package core

import (
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// TestSharedEventIDTable_ComponentsShareHandlesAcrossRestore verifies that the
// ServerContentView components hold each message ID once and keep sharing the
// table after a snapshot restore.
func TestSharedEventIDTable_ComponentsShareHandlesAcrossRestore(t *testing.T) {
	t.Parallel()

	eventIDs := newEventIDTable()
	timeline := newRoomTimelineProjection(eventIDs)
	threads := newThreadProjection(eventIDs)
	reactions := newReactionProjection(eventIDs)
	events := []*evtv1.Event{
		{
			Id: "ENV-ROOM", ActorId: "U1", CreatedAt: timestamppb.New(fixedTime(0)),
			Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: "R1", Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL}},
		},
		postedEvent(postedOpts{envelopeID: "M1", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "M2", roomID: "R1", actorID: "U2", inThread: "M1", at: 2}),
		reactionAddedProjectionEvent("REACTION-1", "M1", "U2", "wave", 3),
	}
	for i, event := range events {
		for _, projection := range []testProjection{timeline, threads, reactions} {
			if err := projection.Apply(event, uint64(i+1)); err != nil {
				t.Fatalf("apply event %d to %T: %v", i+1, projection, err)
			}
		}
	}
	// ENV-ROOM, M1, and M2 are indexed once each for all three components.
	if got := eventIDs.Len(); got != 3 {
		t.Fatalf("shared event IDs = %d, want 3", got)
	}

	for _, projection := range []snapshotProjection{timeline, threads, reactions} {
		payload, err := projection.Snapshot()
		if err != nil {
			t.Fatalf("snapshot %T: %v", projection, err)
		}
		if err := projection.Restore(payload); err != nil {
			t.Fatalf("restore %T: %v", projection, err)
		}
	}
	if got := eventIDs.Len(); got != 3 {
		t.Fatalf("shared event IDs after restore = %d, want 3", got)
	}
	if timeline.eventIDs != eventIDs || threads.eventIDs != eventIDs || reactions.messages != eventIDs {
		t.Fatal("restore replaced the shared event ID table")
	}
	if entry, ok := timeline.Get("M2"); !ok || entry.ThreadRootEventID != "M1" || !entry.CreatedAt.Equal(fixedTime(2)) {
		t.Fatalf("timeline Get(M2) = %+v, %v; want thread root M1 at fixed time 2", entry, ok)
	}
	if root, ok := threads.ThreadRootForMessage("R1", "M2"); !ok || root != "M1" {
		t.Fatalf("ThreadRootForMessage(M2) = %q, %v; want M1, true", root, ok)
	}
	if got := reactions.Reactions("M1"); len(got) != 1 || got[0].Emoji != "wave" {
		t.Fatalf("Reactions(M1) = %+v, want one wave reaction", got)
	}
}
