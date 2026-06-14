package core

import (
	"slices"
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// =============================================================================
// ThreadProjection
// =============================================================================

func TestThreadProjection_Empty(t *testing.T) {
	p := NewThreadProjection()
	if got := p.ThreadEvents("ROOT"); got != nil {
		t.Errorf("ThreadEvents on empty = %v, want nil", got)
	}
	if got := p.ReplyCount("ROOT"); got != 0 {
		t.Errorf("ReplyCount on empty = %d, want 0", got)
	}
	if got := p.ThreadCount(); got != 0 {
		t.Errorf("ThreadCount on empty = %d, want 0", got)
	}
}

func TestThreadProjection_RootMessageNotStored(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-ROOT", eventID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
	})

	if got := p.ThreadCount(); got != 0 {
		t.Errorf("Root message should not create a thread, got ThreadCount=%d", got)
	}
	if got := p.ThreadEvents("ROOT"); got != nil {
		t.Errorf("ThreadEvents(ROOT) should be empty for a thread with no replies, got %d entries", len(got))
	}
}

func TestThreadProjection_ThreadCreatedInitializesEmptyThread(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		threadCreatedEvent("ENV-THREAD", "R1", "ROOT", "U1", 1),
	})

	if !p.ThreadExists("ROOT") {
		t.Fatal("ThreadExists(ROOT) = false, want true")
	}
	if got := p.ThreadCount(); got != 1 {
		t.Errorf("ThreadCount = %d, want 1", got)
	}
	if got := p.ReplyCount("ROOT"); got != 0 {
		t.Errorf("ReplyCount = %d, want 0", got)
	}
	if got := p.ThreadEvents("ROOT"); got != nil {
		t.Errorf("ThreadEvents(ROOT) = %v, want nil before replies", got)
	}
}

func TestThreadProjection_RepliesAppended(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-ROOT", eventID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "ENV-R1", eventID: "REPLY1", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", body: "first", at: 2}),
		postedEvent(postedOpts{envelopeID: "ENV-R2", eventID: "REPLY2", roomID: "R1", actorID: "U3", inThread: "ROOT", inReplyTo: "REPLY1", body: "second", at: 3}),
	})

	entries := p.ThreadEvents("ROOT")
	if len(entries) != 2 {
		t.Fatalf("ThreadEvents(ROOT) len = %d, want 2", len(entries))
	}
	if entries[0].Event.GetId() != "ENV-R1" || entries[1].Event.GetId() != "ENV-R2" {
		t.Errorf("ThreadEvents order = %v, want [ENV-R1, ENV-R2]", timelineEventIDs(entries))
	}
	if got := p.ReplyCount("ROOT"); got != 2 {
		t.Errorf("ReplyCount = %d, want 2", got)
	}
	metadata := p.ThreadMetadata("ROOT")
	if metadata.ReplyCount != 2 {
		t.Errorf("ThreadMetadata ReplyCount = %d, want 2", metadata.ReplyCount)
	}
	if metadata.LastReplyAt == nil {
		t.Fatal("ThreadMetadata LastReplyAt is nil")
	}
	if got, want := *metadata.LastReplyAt, fixedTime(3); !got.Equal(want) {
		t.Errorf("ThreadMetadata LastReplyAt = %v, want %v", got, want)
	}
	if !slices.Equal(metadata.ParticipantIDs, []string{"U2", "U3"}) {
		t.Errorf("ThreadMetadata ParticipantIDs = %v, want [U2 U3]", metadata.ParticipantIDs)
	}
}

func TestThreadProjection_ApplyDoesNotMutateInputEvent(t *testing.T) {
	p := NewThreadProjection()
	reply := postedEvent(postedOpts{envelopeID: "ENV-R1", eventID: "REPLY1", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", at: 1})
	assertApplyDoesNotMutateEvent(t, p, reply, 1)

	entries := p.ThreadEvents("ROOT")
	if len(entries) != 1 {
		t.Fatalf("ThreadEvents(ROOT) len = %d, want 1", len(entries))
	}
	if got := entries[0].Event.GetId(); got != "ENV-R1" {
		t.Fatalf("reply id = %q, want ENV-R1", got)
	}
	if got := entries[0].Event.GetMessagePosted().GetRoomId(); got != "R1" {
		t.Fatalf("reply room id = %q, want R1", got)
	}
	if got := entries[0].Event.GetMessagePosted().GetInThread(); got != "ROOT" {
		t.Fatalf("reply thread root = %q, want ROOT", got)
	}
}

func TestThreadProjection_ReplyWithLegacyEmptyPayloadEventID(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ROOT", eventID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "REPLY1", roomID: "R1", actorID: "U2", inThread: "ROOT", body: "legacy reply", at: 2}),
		editedEvent("EDIT-REPLY1", "REPLY1", "R1", "U2", "edited legacy reply", 3),
	})

	entries := p.ThreadEvents("ROOT")
	if len(entries) != 2 {
		t.Fatalf("ThreadEvents(ROOT) len = %d, want 2", len(entries))
	}
	if got := p.ReplyCount("ROOT"); got != 1 {
		t.Errorf("ReplyCount = %d, want 1", got)
	}
	if entries[1].Event.GetMessageEdited() == nil {
		t.Error("expected edit to route through envelope-id fallback")
	}
}

func TestThreadProjection_EditOfReplyAppendedToThread(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-ROOT", eventID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "ENV-R1", eventID: "REPLY1", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", body: "original", at: 2}),
		editedEvent("ENV-EDIT-R1", "ENV-R1", "R1", "U2", "edited", 3),
	})

	entries := p.ThreadEvents("ROOT")
	if len(entries) != 2 {
		t.Fatalf("expected post + edit, got %d entries", len(entries))
	}
	if entries[1].Event.GetMessageEdited() == nil {
		t.Error("expected entries[1] to be a MessageEditedEvent")
	}
	// Reply count counts MessagePostedEvent only.
	if got := p.ReplyCount("ROOT"); got != 1 {
		t.Errorf("ReplyCount after edit = %d, want 1 (edits don't bump)", got)
	}
}

func TestThreadProjection_RetractOfReplyAppendedToThread(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-ROOT", eventID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "ENV-R1", eventID: "REPLY1", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", at: 2}),
		retractedEvent("ENV-RETRACT-R1", "ENV-R1", "R1", "MOD", "spam", 3),
	})

	entries := p.ThreadEvents("ROOT")
	if len(entries) != 2 {
		t.Fatalf("expected post + retract, got %d entries", len(entries))
	}
	if entries[1].Event.GetMessageRetracted() == nil {
		t.Error("expected entries[1] to be a MessageRetractedEvent")
	}
	if got := p.ReplyCount("ROOT"); got != 0 {
		t.Errorf("ReplyCount after retract = %d, want 0", got)
	}
	metadata := p.ThreadMetadata("ROOT")
	if metadata.ReplyCount != 0 {
		t.Errorf("ThreadMetadata ReplyCount after retract = %d, want 0", metadata.ReplyCount)
	}
	if metadata.LastReplyAt != nil {
		t.Errorf("ThreadMetadata LastReplyAt after retract = %v, want nil", metadata.LastReplyAt)
	}
}

func TestThreadProjection_EditOfRootMessageNotInThreadBucket(t *testing.T) {
	// Root message edits/retracts are room-timeline concerns, not
	// thread-projection ones. Confirm they don't leak into the thread.
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-ROOT", eventID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "ENV-R1", eventID: "REPLY1", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", at: 2}),
		editedEvent("ENV-EDIT-ROOT", "ROOT", "R1", "U1", "edited root", 3), // targets ROOT, not REPLY1
	})

	entries := p.ThreadEvents("ROOT")
	if len(entries) != 1 {
		t.Fatalf("expected only the reply, got %d entries", len(entries))
	}
	if entries[0].Event.GetId() != "ENV-R1" {
		t.Errorf("entry = %q, want ENV-R1", entries[0].Event.GetId())
	}
}

func TestThreadProjection_OutOfOrderEditDropped(t *testing.T) {
	// Edit arrives before the reply post. Without messageToThread
	// mapping, the edit doesn't know which thread it belongs to and is
	// silently dropped.
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		editedEvent("ENV-EDIT", "REPLY1", "R1", "U2", "edited", 1),
	})
	if got := p.ThreadCount(); got != 0 {
		t.Errorf("Out-of-order edit shouldn't create a thread, got ThreadCount=%d", got)
	}
}

func TestThreadProjection_MultipleThreadsIsolated(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-T1A", eventID: "T1A", roomID: "R1", actorID: "U1", inThread: "T1", inReplyTo: "T1", at: 1}),
		postedEvent(postedOpts{envelopeID: "ENV-T2A", eventID: "T2A", roomID: "R1", actorID: "U1", inThread: "T2", inReplyTo: "T2", at: 2}),
		postedEvent(postedOpts{envelopeID: "ENV-T1B", eventID: "T1B", roomID: "R1", actorID: "U2", inThread: "T1", inReplyTo: "T1A", at: 3}),
	})

	if got := p.ReplyCount("T1"); got != 2 {
		t.Errorf("T1 reply count = %d, want 2", got)
	}
	if got := p.ReplyCount("T2"); got != 1 {
		t.Errorf("T2 reply count = %d, want 1", got)
	}
	if got := p.ThreadCount(); got != 2 {
		t.Errorf("ThreadCount = %d, want 2", got)
	}
}

func TestThreadProjection_MetadataRecomputesWhenLatestReplyRetracted(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-R1", eventID: "REPLY1", roomID: "R1", actorID: "U1", inThread: "ROOT", inReplyTo: "ROOT", at: 2}),
		postedEvent(postedOpts{envelopeID: "ENV-R2", eventID: "REPLY2", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "REPLY1", at: 3}),
		retractedEvent("ENV-RETRACT-R2", "ENV-R2", "R1", "MOD", "spam", 4),
	})

	metadata := p.ThreadMetadata("ROOT")
	if metadata.ReplyCount != 1 {
		t.Fatalf("ReplyCount = %d, want 1", metadata.ReplyCount)
	}
	if metadata.LastReplyAt == nil {
		t.Fatal("LastReplyAt is nil")
	}
	if got, want := *metadata.LastReplyAt, fixedTime(2); !got.Equal(want) {
		t.Errorf("LastReplyAt = %v, want %v", got, want)
	}
	if !slices.Equal(metadata.ParticipantIDs, []string{"U1"}) {
		t.Errorf("ParticipantIDs = %v, want [U1]", metadata.ParticipantIDs)
	}
}

func TestThreadProjection_Idempotency(t *testing.T) {
	p := NewThreadProjection()
	reply := postedEvent(postedOpts{envelopeID: "ENV-R1", eventID: "REPLY1", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", at: 1})
	if err := p.Apply(reply, 1); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if err := p.Apply(reply, 1); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if got := p.ReplyCount("ROOT"); got != 1 {
		t.Errorf("duplicate Apply doubled ReplyCount: %d, want 1", got)
	}
	if got := len(p.ThreadEvents("ROOT")); got != 1 {
		t.Errorf("duplicate Apply doubled ThreadEvents: %d, want 1", got)
	}
}

func TestThreadProjection_IdempotencyDoesNotIndexIgnoredRoomEvents(t *testing.T) {
	p := NewThreadProjection()
	root := postedEvent(postedOpts{envelopeID: "ENV-ROOT", eventID: "ROOT", roomID: "R1", actorID: "U1", at: 1})
	if err := p.Apply(root, 1); err != nil {
		t.Fatalf("first root Apply: %v", err)
	}
	if err := p.Apply(root, 1); err != nil {
		t.Fatalf("second root Apply: %v", err)
	}
	if got := len(p.appliedEventIDs); got != 0 {
		t.Fatalf("ignored root events populated appliedEventIDs with %d entries, want 0", got)
	}

	reply := postedEvent(postedOpts{envelopeID: "ENV-ROOT", eventID: "REPLY1", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", at: 2})
	if err := p.Apply(reply, 2); err != nil {
		t.Fatalf("reply Apply after ignored same-id root: %v", err)
	}
	if got := p.ReplyCount("ROOT"); got != 1 {
		t.Fatalf("ReplyCount = %d, want 1", got)
	}
	if got := len(p.appliedEventIDs); got != 1 {
		t.Fatalf("appliedEventIDs after relevant event = %d, want 1", got)
	}
}

func TestThreadProjection_SubjectFilter(t *testing.T) {
	subjects := NewThreadProjection().Subjects()
	want := map[string]bool{
		events.RoomSubjectFilter():                              true,
		events.UserEventTypeFilter(events.EventUserKeyShredded): true,
	}
	if len(subjects) != len(want) {
		t.Fatalf("expected %d subject filters, got %d", len(want), len(subjects))
	}
	for subject := range want {
		if !slices.Contains(subjects, subject) {
			t.Errorf("missing subject filter %q", subject)
		}
	}
	if slices.Contains(subjects, events.UserSubjectFilter()) {
		t.Errorf("unexpected broad user subject filter %q", events.UserSubjectFilter())
	}
}
