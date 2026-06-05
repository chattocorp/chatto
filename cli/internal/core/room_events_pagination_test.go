package core

import (
	"context"
	"fmt"
	"testing"
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
