package core

import (
	"strconv"
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var (
	_ events.StartupReplayCompleter = (*AssetProjection)(nil)
	_ events.StartupReplayCompleter = (*ContentKeyProjection)(nil)
	_ events.StartupReplayCompleter = (*ReactionProjection)(nil)
	_ events.StartupReplayCompleter = (*RBACProjection)(nil)
	_ events.StartupReplayCompleter = (*RoomTimelineProjection)(nil)
	_ events.StartupReplayCompleter = (*ThreadProjection)(nil)
	_ events.StartupReplayCompleter = (*UserProjection)(nil)
)

func TestProjectionReplayGuardReleasesCleanHistory(t *testing.T) {
	guard := newProjectionReplayGuard()
	first := &corev1.Event{Id: "E1"}
	second := &corev1.Event{Id: "E2"}

	if guard.seenOrMark(first, 10) {
		t.Fatal("first event was reported as already seen")
	}
	if guard.seenOrMark(second, 20) {
		t.Fatal("second event was reported as already seen")
	}
	if got := len(guard.retainedEventIDs()); got != 2 {
		t.Fatalf("retained event IDs before replay completion = %d, want 2", got)
	}

	guard.completeReplay()

	if guard.retainedEventIDs() != nil {
		t.Fatal("clean replay retained its event-ID set")
	}
	if guard.compatibilityMode {
		t.Fatal("clean replay enabled compatibility mode")
	}
	if !guard.seen(first, 20) {
		t.Fatal("already-applied stream sequence was not suppressed")
	}
	if !guard.seen(first, 19) {
		t.Fatal("lower stream sequence was not suppressed")
	}
	if guard.seenOrMark(&corev1.Event{Id: "E3"}, 21) {
		t.Fatal("new stream sequence was reported as already seen")
	}
	if guard.retainedEventIDs() != nil {
		t.Fatal("steady-state sequence guard allocated an event-ID set")
	}
}

func TestProjectionReplayGuardSteadyStateMemoryIsConstant(t *testing.T) {
	guard := newProjectionReplayGuard()
	guard.completeReplay()

	for seq := uint64(1); seq <= 10_000; seq++ {
		if guard.seenOrMark(&corev1.Event{Id: "E" + strconv.FormatUint(seq, 10)}, seq) {
			t.Fatalf("new stream sequence %d was reported as already seen", seq)
		}
	}
	if guard.highestSeq != 10_000 {
		t.Fatalf("highest sequence = %d, want 10000", guard.highestSeq)
	}
	if guard.retainedEventIDs() != nil {
		t.Fatal("steady-state delivery allocated an event-ID set")
	}
}

func TestProjectionReplayGuardRetainsDuplicateHistoryCompatibility(t *testing.T) {
	guard := newProjectionReplayGuard()
	first := &corev1.Event{Id: "E1"}

	if guard.seenOrMark(first, 10) {
		t.Fatal("first event was reported as already seen")
	}
	if !guard.seenOrMark(first, 20) {
		t.Fatal("duplicate event ID was not suppressed during replay")
	}

	guard.completeReplay()

	if !guard.compatibilityMode {
		t.Fatal("duplicate replay did not enable compatibility mode")
	}
	if got := len(guard.retainedEventIDs()); got != 1 {
		t.Fatalf("retained event IDs after replay completion = %d, want 1", got)
	}
	if !guard.seenOrMark(first, 30) {
		t.Fatal("compatibility mode did not suppress a later duplicate ID")
	}
	if guard.seenOrMark(&corev1.Event{Id: "E2"}, 40) {
		t.Fatal("new event ID was reported as already seen")
	}
	if got := len(guard.retainedEventIDs()); got != 2 {
		t.Fatalf("compatibility event IDs = %d, want 2", got)
	}
}

func TestProjectionReplayGuardCompletesEmptyReplay(t *testing.T) {
	guard := newProjectionReplayGuard()
	guard.completeReplay()
	guard.completeReplay()

	if guard.retainedEventIDs() != nil {
		t.Fatal("empty replay retained an event-ID set")
	}
	if !guard.replayComplete {
		t.Fatal("empty replay was not marked complete")
	}
}

func TestProjectionReplayGuardAdminEstimateReportsRetainedCompatibilityOnly(t *testing.T) {
	message := func() *corev1.Event {
		return &corev1.Event{
			Id: "E1",
			Event: &corev1.Event_MessagePosted{
				MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"},
			},
		}
	}

	clean := NewRoomTimelineProjection()
	if err := clean.Apply(message(), 1); err != nil {
		t.Fatalf("clean Apply: %v", err)
	}
	clean.CompleteStartupReplay()
	_, _, cleanMetrics := clean.adminProjectionEstimate()
	if metric := projectionMetricByName(cleanMetrics, "applied_event_ids"); metric == nil || metric.Value != 0 || metric.Bytes != 0 {
		t.Fatalf("clean applied_event_ids metric = %+v, want zero", metric)
	}
	if metric := projectionMetricByName(cleanMetrics, "event_id_compatibility_mode"); metric == nil || metric.Value != 0 {
		t.Fatalf("clean compatibility metric = %+v, want zero", metric)
	}

	compatible := NewRoomTimelineProjection()
	if err := compatible.Apply(message(), 1); err != nil {
		t.Fatalf("compatible first Apply: %v", err)
	}
	if err := compatible.Apply(message(), 2); err != nil {
		t.Fatalf("compatible duplicate Apply: %v", err)
	}
	compatible.CompleteStartupReplay()
	_, _, compatibilityMetrics := compatible.adminProjectionEstimate()
	if metric := projectionMetricByName(compatibilityMetrics, "applied_event_ids"); metric == nil || metric.Value != 1 || metric.Bytes == 0 {
		t.Fatalf("compatibility applied_event_ids metric = %+v, want one retained ID", metric)
	}
	if metric := projectionMetricByName(compatibilityMetrics, "event_id_compatibility_mode"); metric == nil || metric.Value != 1 {
		t.Fatalf("compatibility metric = %+v, want one", metric)
	}
}
