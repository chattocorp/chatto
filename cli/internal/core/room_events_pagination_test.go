package core

import (
	"context"
	"fmt"
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_GetRoomEventsAroundReturnsChronologicalWindow(t *testing.T) {
	core := testCoreWithRoomTimeline(t, "R1", 10)

	result, err := core.GetRoomEventsAround(context.Background(), KindChannel, "R1", "M5", 5)
	if err != nil {
		t.Fatalf("GetRoomEventsAround: %v", err)
	}

	assertRoomEventIDs(t, result.Events, []string{"M3", "M4", "M5", "M6", "M7"})
	if result.TargetIndex != 2 {
		t.Errorf("TargetIndex = %d, want 2", result.TargetIndex)
	}
	if !result.HasOlder {
		t.Error("HasOlder = false, want true")
	}
	if !result.HasNewer {
		t.Error("HasNewer = false, want true")
	}

	nearStart, err := core.GetRoomEventsAround(context.Background(), KindChannel, "R1", "M2", 5)
	if err != nil {
		t.Fatalf("GetRoomEventsAround near start: %v", err)
	}
	assertRoomEventIDs(t, nearStart.Events, []string{"M1", "M2", "M3", "M4", "M5"})
	if nearStart.TargetIndex != 1 {
		t.Errorf("near-start TargetIndex = %d, want 1", nearStart.TargetIndex)
	}
	if nearStart.HasOlder {
		t.Error("near-start HasOlder = true, want false")
	}
	if !nearStart.HasNewer {
		t.Error("near-start HasNewer = false, want true")
	}

	nearEnd, err := core.GetRoomEventsAround(context.Background(), KindChannel, "R1", "M9", 5)
	if err != nil {
		t.Fatalf("GetRoomEventsAround near end: %v", err)
	}
	assertRoomEventIDs(t, nearEnd.Events, []string{"M6", "M7", "M8", "M9", "M10"})
	if nearEnd.TargetIndex != 3 {
		t.Errorf("near-end TargetIndex = %d, want 3", nearEnd.TargetIndex)
	}
	if !nearEnd.HasOlder {
		t.Error("near-end HasOlder = false, want true")
	}
	if nearEnd.HasNewer {
		t.Error("near-end HasNewer = true, want false")
	}
}

func TestChattoCore_GetRoomEventsAfterReturnsNearestNewerPage(t *testing.T) {
	core := testCoreWithRoomTimeline(t, "R1", 100)

	result, err := core.GetRoomEventsAfter(context.Background(), KindChannel, "R1", 45, 5)
	if err != nil {
		t.Fatalf("GetRoomEventsAfter: %v", err)
	}

	assertRoomEventIDs(t, result.Events, []string{"M46", "M47", "M48", "M49", "M50"})
	if !result.HasNewer {
		t.Error("HasNewer = false, want true")
	}
	if result.StartCursorSeq != 46 {
		t.Errorf("StartCursorSeq = %d, want 46", result.StartCursorSeq)
	}
	if result.EndCursorSeq != 50 {
		t.Errorf("EndCursorSeq = %d, want 50", result.EndCursorSeq)
	}
}

func TestChattoCore_RoomEventQueriesClampLimits(t *testing.T) {
	core := testCoreWithRoomTimeline(t, "R1", 600)

	t.Run("recent events clamp oversized limits", func(t *testing.T) {
		result, err := core.GetRoomEvents(context.Background(), KindChannel, "R1", 1000, nil)
		if err != nil {
			t.Fatalf("GetRoomEvents: %v", err)
		}

		assertRoomEventIDs(t, result.Events[:2], []string{"M101", "M102"})
		if len(result.Events) != maxHistoricalMessageLimit {
			t.Fatalf("len(Events) = %d, want %d", len(result.Events), maxHistoricalMessageLimit)
		}
		if !result.HasOlder {
			t.Error("HasOlder = false, want true")
		}
	})

	t.Run("forward events clamp oversized limits", func(t *testing.T) {
		result, err := core.GetRoomEventsAfter(context.Background(), KindChannel, "R1", 0, 1000)
		if err != nil {
			t.Fatalf("GetRoomEventsAfter: %v", err)
		}

		if len(result.Events) != maxHistoricalMessageLimit {
			t.Fatalf("len(Events) = %d, want %d", len(result.Events), maxHistoricalMessageLimit)
		}
		assertRoomEventIDs(t, result.Events[:2], []string{"M1", "M2"})
		if !result.HasNewer {
			t.Error("HasNewer = false, want true")
		}
	})

	t.Run("around events clamp oversized limits", func(t *testing.T) {
		result, err := core.GetRoomEventsAround(context.Background(), KindChannel, "R1", "M300", 1000)
		if err != nil {
			t.Fatalf("GetRoomEventsAround: %v", err)
		}

		if len(result.Events) != maxHistoricalMessageLimit {
			t.Fatalf("len(Events) = %d, want %d", len(result.Events), maxHistoricalMessageLimit)
		}
		wantTargetIndex := (maxHistoricalMessageLimit - 1) / 2
		if result.TargetIndex != wantTargetIndex {
			t.Fatalf("TargetIndex = %d, want %d", result.TargetIndex, wantTargetIndex)
		}
	})

	t.Run("non-positive limits use default", func(t *testing.T) {
		result, err := core.GetRoomEvents(context.Background(), KindChannel, "R1", -1, nil)
		if err != nil {
			t.Fatalf("GetRoomEvents: %v", err)
		}

		if len(result.Events) != defaultHistoricalMessageLimit {
			t.Fatalf("len(Events) = %d, want %d", len(result.Events), defaultHistoricalMessageLimit)
		}
	})
}

func TestChattoCore_GetRoomEventsUsesDerivedVisibleTimelineWithNoise(t *testing.T) {
	core := testCoreWithRoomTimelineEvents(t, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "M1", roomID: "R1", actorID: "U1", body: "1", at: 1}),
		postedEvent(postedOpts{envelopeID: "REPLY-M1", roomID: "R1", actorID: "U2", body: "reply", inThread: "M1", at: 2}),
		editedEvent("EDIT-M1", "M1", "R1", "U1", "1 edited", 3),
		reactionAddedEvent("REACT-M1", "R1", "M1", "U2", "thumbsup"),
		attachmentDeclaredEvent("R1", "A1", "image/png"),
		postedEvent(postedOpts{envelopeID: "M2", roomID: "R1", actorID: "U1", body: "2", at: 6}),
		postedEvent(postedOpts{envelopeID: "M3", roomID: "R1", actorID: "U1", body: "3", at: 7}),
		postedEvent(postedOpts{envelopeID: "ECHO-M1", roomID: "R1", actorID: "U2", body: "echo", echoOfEventID: "REPLY-M1", echoFromThreadRootEventID: "M1", at: 8}),
		retractedEvent("RETRACT-ECHO-M1", "ECHO-M1", "R1", "U2", "", 9),
		postedEvent(postedOpts{envelopeID: "M4", roomID: "R1", actorID: "U1", body: "4", at: 10}),
	})

	result, err := core.GetRoomEvents(context.Background(), KindChannel, "R1", 3, nil)
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}

	assertRoomEventIDs(t, result.Events, []string{"M2", "M3", "M4"})
	if !result.HasOlder {
		t.Error("HasOlder = false, want true")
	}
	if result.HasNewer {
		t.Error("HasNewer = true, want false")
	}
	if result.StartCursorSeq != 6 {
		t.Errorf("StartCursorSeq = %d, want 6", result.StartCursorSeq)
	}
	if result.EndCursorSeq != 10 {
		t.Errorf("EndCursorSeq = %d, want 10", result.EndCursorSeq)
	}
}

func TestChattoCore_GetRoomEventByEventIDExposesOnlyVisibleAndMessagePostLookups(t *testing.T) {
	core := testCoreWithRoomTimelineEvents(t, []*corev1.Event{
		roomCreatedTimelineEvent("CREATE", "R1", "general", 1),
		postedEvent(postedOpts{envelopeID: "ROOT", roomID: "R1", actorID: "U1", body: "root", at: 2}),
		threadCreatedEvent("THREAD-CREATED", "R1", "ROOT", "U1", 3),
		postedEvent(postedOpts{envelopeID: "REPLY", roomID: "R1", actorID: "U2", body: "reply", inThread: "ROOT", at: 4}),
		postedEvent(postedOpts{envelopeID: "ECHO", roomID: "R1", actorID: "U2", body: "echo", echoOfEventID: "REPLY", echoFromThreadRootEventID: "ROOT", at: 5}),
		editedEvent("EDIT-ROOT", "ROOT", "R1", "U1", "root edited", 6),
		retractedEvent("HIDE-ECHO", "ECHO", "R1", "U2", "", 7),
		reactionAddedEvent("REACT-ROOT", "R1", "ROOT", "U2", "thumbsup"),
	})

	for _, eventID := range []string{"CREATE", "ROOT", "REPLY"} {
		event, err := core.GetRoomEventByEventID(context.Background(), KindChannel, "R1", eventID)
		if err != nil {
			t.Fatalf("GetRoomEventByEventID(%s): %v", eventID, err)
		}
		if event == nil || event.GetId() != eventID {
			t.Fatalf("GetRoomEventByEventID(%s) = %v, want event", eventID, event)
		}
	}

	for _, eventID := range []string{"THREAD-CREATED", "EDIT-ROOT", "HIDE-ECHO", "REACT-ROOT", "ECHO"} {
		event, err := core.GetRoomEventByEventID(context.Background(), KindChannel, "R1", eventID)
		if err != nil {
			t.Fatalf("GetRoomEventByEventID(%s): %v", eventID, err)
		}
		if event != nil {
			t.Fatalf("GetRoomEventByEventID(%s) returned folded/hidden event %T", eventID, event.GetEvent())
		}
	}

	event, err := core.GetRoomEventByEventID(context.Background(), KindChannel, "R2", "ROOT")
	if err != nil {
		t.Fatalf("GetRoomEventByEventID wrong room: %v", err)
	}
	if event != nil {
		t.Fatalf("GetRoomEventByEventID wrong room returned event %s", event.GetId())
	}
}

func TestChattoCore_GetRoomEventsAfterUsesDerivedVisibleTimelineWithNoise(t *testing.T) {
	core := testCoreWithRoomTimelineEvents(t, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "M1", roomID: "R1", actorID: "U1", body: "1", at: 1}),
		postedEvent(postedOpts{envelopeID: "REPLY-M1", roomID: "R1", actorID: "U2", body: "reply", inThread: "M1", at: 2}),
		editedEvent("EDIT-M1", "M1", "R1", "U1", "1 edited", 3),
		reactionAddedEvent("REACT-M1", "R1", "M1", "U2", "thumbsup"),
		postedEvent(postedOpts{envelopeID: "M2", roomID: "R1", actorID: "U1", body: "2", at: 5}),
		postedEvent(postedOpts{envelopeID: "ECHO-M1", roomID: "R1", actorID: "U2", body: "echo", echoOfEventID: "REPLY-M1", echoFromThreadRootEventID: "M1", at: 6}),
		retractedEvent("RETRACT-ECHO-M1", "ECHO-M1", "R1", "U2", "", 7),
		postedEvent(postedOpts{envelopeID: "M3", roomID: "R1", actorID: "U1", body: "3", at: 8}),
		attachmentDeclaredEvent("R1", "A1", "image/png"),
		postedEvent(postedOpts{envelopeID: "M4", roomID: "R1", actorID: "U1", body: "4", at: 10}),
	})

	result, err := core.GetRoomEventsAfter(context.Background(), KindChannel, "R1", 1, 2)
	if err != nil {
		t.Fatalf("GetRoomEventsAfter: %v", err)
	}

	assertRoomEventIDs(t, result.Events, []string{"M2", "M3"})
	if !result.HasOlder {
		t.Error("HasOlder = false, want true")
	}
	if !result.HasNewer {
		t.Error("HasNewer = false, want true")
	}
	if result.StartCursorSeq != 5 {
		t.Errorf("StartCursorSeq = %d, want 5", result.StartCursorSeq)
	}
	if result.EndCursorSeq != 8 {
		t.Errorf("EndCursorSeq = %d, want 8", result.EndCursorSeq)
	}
}

func TestChattoCore_GetRoomEventsAroundUsesDerivedVisibleTimelineWithHiddenEcho(t *testing.T) {
	core := testCoreWithRoomTimelineEvents(t, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "M1", roomID: "R1", actorID: "U1", body: "1", at: 1}),
		postedEvent(postedOpts{envelopeID: "REPLY-M1", roomID: "R1", actorID: "U2", body: "reply", inThread: "M1", at: 2}),
		postedEvent(postedOpts{envelopeID: "ECHO-M1", roomID: "R1", actorID: "U2", body: "echo", echoOfEventID: "REPLY-M1", echoFromThreadRootEventID: "M1", at: 3}),
		retractedEvent("RETRACT-ECHO-M1", "ECHO-M1", "R1", "U2", "", 4),
		postedEvent(postedOpts{envelopeID: "M2", roomID: "R1", actorID: "U1", body: "2", at: 5}),
		editedEvent("EDIT-M2", "M2", "R1", "U1", "2 edited", 6),
		postedEvent(postedOpts{envelopeID: "M3", roomID: "R1", actorID: "U1", body: "3", at: 7}),
		postedEvent(postedOpts{envelopeID: "M4", roomID: "R1", actorID: "U1", body: "4", at: 8}),
	})

	result, err := core.GetRoomEventsAround(context.Background(), KindChannel, "R1", "M2", 3)
	if err != nil {
		t.Fatalf("GetRoomEventsAround: %v", err)
	}

	assertRoomEventIDs(t, result.Events, []string{"M1", "M2", "M3"})
	if result.TargetIndex != 1 {
		t.Errorf("TargetIndex = %d, want 1", result.TargetIndex)
	}
	if result.HasOlder {
		t.Error("HasOlder = true, want false")
	}
	if !result.HasNewer {
		t.Error("HasNewer = false, want true")
	}
}

func TestChattoCore_GetDMRoomEventsUsesDerivedVisibleTimeline(t *testing.T) {
	core := testCoreWithRoomTimelineEvents(t, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "DM-M1", roomID: "DM1", actorID: "U1", body: "1", at: 1}),
		postedEvent(postedOpts{envelopeID: "DM-REPLY-M1", roomID: "DM1", actorID: "U2", body: "reply", inThread: "DM-M1", at: 2}),
		editedEvent("DM-EDIT-M1", "DM-M1", "DM1", "U1", "1 edited", 3),
		reactionAddedEvent("DM-REACT-M1", "DM1", "DM-M1", "U2", "thumbsup"),
		postedEvent(postedOpts{envelopeID: "DM-M2", roomID: "DM1", actorID: "U2", body: "2", at: 5}),
		postedEvent(postedOpts{envelopeID: "DM-ECHO-M1", roomID: "DM1", actorID: "U2", body: "echo", echoOfEventID: "DM-REPLY-M1", echoFromThreadRootEventID: "DM-M1", at: 6}),
		retractedEvent("DM-RETRACT-ECHO-M1", "DM-ECHO-M1", "DM1", "U2", "", 7),
		postedEvent(postedOpts{envelopeID: "DM-M3", roomID: "DM1", actorID: "U1", body: "3", at: 8}),
		postedEvent(postedOpts{envelopeID: "DM-M4", roomID: "DM1", actorID: "U2", body: "4", at: 9}),
	})

	page, err := core.GetRoomEvents(context.Background(), KindDM, "DM1", 2, nil)
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}
	assertRoomEventIDs(t, page.Events, []string{"DM-M3", "DM-M4"})
	if !page.HasOlder {
		t.Error("HasOlder = false, want true")
	}

	around, err := core.GetRoomEventsAround(context.Background(), KindDM, "DM1", "DM-M2", 3)
	if err != nil {
		t.Fatalf("GetRoomEventsAround: %v", err)
	}
	assertRoomEventIDs(t, around.Events, []string{"DM-M1", "DM-M2", "DM-M3"})
	if around.TargetIndex != 1 {
		t.Errorf("TargetIndex = %d, want 1", around.TargetIndex)
	}
	if around.HasOlder {
		t.Error("HasOlder = true, want false")
	}
	if !around.HasNewer {
		t.Error("HasNewer = false, want true")
	}
}

func testCoreWithRoomTimeline(t *testing.T, roomID string, count int) *ChattoCore {
	t.Helper()
	projection := NewRoomTimelineProjection()
	for i := 1; i <= count; i++ {
		eventID := fmt.Sprintf("M%d", i)
		event := postedEvent(postedOpts{
			envelopeID: eventID,
			eventID:    eventID,
			roomID:     roomID,
			actorID:    "U1",
			body:       eventID,
			at:         i,
		})
		if err := projection.Apply(event, uint64(i)); err != nil {
			t.Fatalf("apply event %s: %v", eventID, err)
		}
	}
	return &ChattoCore{RoomTimeline: projection}
}

func testCoreWithRoomTimelineEvents(t *testing.T, events []*corev1.Event) *ChattoCore {
	t.Helper()
	projection := NewRoomTimelineProjection()
	applyAll(t, projection, events)
	return &ChattoCore{RoomTimeline: projection}
}

func reactionAddedEvent(envID, roomID, messageID, actorID, emoji string) *corev1.Event {
	return &corev1.Event{
		Id:      envID,
		ActorId: actorID,
		Event: &corev1.Event_ReactionAdded{
			ReactionAdded: &corev1.ReactionAddedEvent{
				RoomId:         roomID,
				MessageEventId: messageID,
				Emoji:          emoji,
			},
		},
	}
}

func assertRoomEventIDs(t *testing.T, events []*RoomEvent, want []string) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("len(events) = %d, want %d; got %v", len(events), len(want), roomEventIDs(events))
	}
	for i, event := range events {
		if event.GetId() != want[i] {
			t.Fatalf("events[%d].Id = %q, want %q; got %v", i, event.GetId(), want[i], roomEventIDs(events))
		}
	}
}

func roomEventIDs(events []*RoomEvent) []string {
	out := make([]string, len(events))
	for i, event := range events {
		if event == nil || event.Event == nil {
			out[i] = "<nil>"
			continue
		}
		out[i] = event.GetId()
	}
	return out
}
