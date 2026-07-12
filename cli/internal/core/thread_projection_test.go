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

func threadEventIDs(entries []ThreadTimelineEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.EventID)
	}
	return out
}

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
	if entries[0].EventID != "ENV-R1" || entries[1].EventID != "ENV-R2" {
		t.Errorf("ThreadEvents order = %v, want [ENV-R1, ENV-R2]", threadEventIDs(entries))
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
	if got := entries[0].EventID; got != "ENV-R1" {
		t.Fatalf("reply id = %q, want ENV-R1", got)
	}
	if got := entries[0].StreamSeq; got != 1 {
		t.Fatalf("reply stream seq = %d, want 1", got)
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
	if len(entries) != 1 {
		t.Fatalf("ThreadEvents(ROOT) len = %d, want 1 reply ref", len(entries))
	}
	if got := p.ReplyCount("ROOT"); got != 1 {
		t.Errorf("ReplyCount = %d, want 1", got)
	}
	if got := len(p.replayGuard.retainedEventIDs()); got != 2 {
		t.Errorf("appliedEventIDs = %d, want 2 to confirm edit routed through envelope-id fallback", got)
	}
}

func TestThreadProjection_EditOfReplyDoesNotAddThreadRow(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-ROOT", eventID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "ENV-R1", eventID: "REPLY1", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", body: "original", at: 2}),
		editedEvent("ENV-EDIT-R1", "ENV-R1", "R1", "U2", "edited", 3),
	})

	entries := p.ThreadEvents("ROOT")
	if len(entries) != 1 {
		t.Fatalf("expected only the reply row after edit, got %d entries", len(entries))
	}
	// Reply count counts MessagePostedEvent only.
	if got := p.ReplyCount("ROOT"); got != 1 {
		t.Errorf("ReplyCount after edit = %d, want 1 (edits don't bump)", got)
	}
}

func TestThreadProjection_RetractOfReplyFoldsIntoSummary(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-ROOT", eventID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "ENV-R1", eventID: "REPLY1", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", at: 2}),
		retractedEvent("ENV-RETRACT-R1", "ENV-R1", "R1", "MOD", "spam", 3),
	})

	entries := p.ThreadEvents("ROOT")
	if len(entries) != 1 {
		t.Fatalf("expected only the reply row after retract, got %d entries", len(entries))
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
	if entries[0].EventID != "ENV-R1" {
		t.Errorf("entry = %q, want ENV-R1", entries[0].EventID)
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
	p.CompleteStartupReplay()
	if err := p.Apply(reply, 1); err != nil {
		t.Fatalf("Apply duplicate after replay: %v", err)
	}
	if got := p.ReplyCount("ROOT"); got != 1 {
		t.Errorf("duplicate Apply doubled ReplyCount: %d, want 1", got)
	}
	if got := len(p.ThreadEvents("ROOT")); got != 1 {
		t.Errorf("duplicate Apply doubled ThreadEvents: %d, want 1", got)
	}
	if got := len(p.replayGuard.retainedEventIDs()); got != 1 {
		t.Errorf("duplicate replay retained event IDs = %d, want 1", got)
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
	if got := len(p.replayGuard.retainedEventIDs()); got != 0 {
		t.Fatalf("ignored root events populated appliedEventIDs with %d entries, want 0", got)
	}

	reply := postedEvent(postedOpts{envelopeID: "ENV-ROOT", eventID: "REPLY1", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", at: 2})
	if err := p.Apply(reply, 2); err != nil {
		t.Fatalf("reply Apply after ignored same-id root: %v", err)
	}
	if got := p.ReplyCount("ROOT"); got != 1 {
		t.Fatalf("ReplyCount = %d, want 1", got)
	}
	if got := len(p.replayGuard.retainedEventIDs()); got != 1 {
		t.Fatalf("appliedEventIDs after relevant event = %d, want 1", got)
	}
}

func TestThreadProjection_ThreadFollowEventsUpdateIndexes(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*corev1.Event{
		{
			Id:      "FOLLOW-U1",
			ActorId: "U1",
			Event: &corev1.Event_ThreadFollowed{
				ThreadFollowed: &corev1.ThreadFollowedEvent{
					RoomId:            "R1",
					ThreadRootEventId: "ROOT",
					UserId:            "U1",
					Source:            corev1.ThreadFollowSource_THREAD_FOLLOW_SOURCE_MANUAL,
				},
			},
		},
		{
			Id:      "FOLLOW-U2",
			ActorId: "U2",
			Event: &corev1.Event_ThreadFollowed{
				ThreadFollowed: &corev1.ThreadFollowedEvent{
					RoomId:            "R1",
					ThreadRootEventId: "ROOT",
					UserId:            "U2",
					Source:            corev1.ThreadFollowSource_THREAD_FOLLOW_SOURCE_DIRECT_MENTION,
				},
			},
		},
	})

	if got := p.FollowState("U1", "R1", "ROOT"); got != ThreadFollowStateFollowing {
		t.Fatalf("FollowState(U1) = %q, want following", got)
	}
	if got := p.FollowState("U2", "R1", "ROOT"); got != ThreadFollowStateFollowing {
		t.Fatalf("FollowState(U2) = %q, want following", got)
	}
	followers := p.ThreadFollowers("R1", "ROOT")
	slices.Sort(followers)
	if !slices.Equal(followers, []string{"U1", "U2"}) {
		t.Fatalf("ThreadFollowers = %v, want [U1 U2]", followers)
	}
	followed := p.FollowedThreadsForUser("U1")
	if len(followed) != 1 || followed[0].roomID != "R1" || followed[0].threadRootEventID != "ROOT" {
		t.Fatalf("FollowedThreadsForUser(U1) = %#v, want R1/ROOT", followed)
	}

	applyAll(t, p, []*corev1.Event{
		{
			Id:      "UNFOLLOW-U1",
			ActorId: "U1",
			Event: &corev1.Event_ThreadUnfollowed{
				ThreadUnfollowed: &corev1.ThreadUnfollowedEvent{
					RoomId:            "R1",
					ThreadRootEventId: "ROOT",
					UserId:            "U1",
				},
			},
		},
	})

	if got := p.FollowState("U1", "R1", "ROOT"); got != ThreadFollowStateUnfollowed {
		t.Fatalf("FollowState(U1) after unfollow = %q, want unfollowed", got)
	}
	followers = p.ThreadFollowers("R1", "ROOT")
	if !slices.Equal(followers, []string{"U2"}) {
		t.Fatalf("ThreadFollowers after unfollow = %v, want [U2]", followers)
	}
	if followed := p.FollowedThreadsForUser("U1"); len(followed) != 0 {
		t.Fatalf("FollowedThreadsForUser(U1) after unfollow = %#v, want empty", followed)
	}
}

func TestThreadProjection_SeedLegacyThreadFollowState(t *testing.T) {
	p := NewThreadProjection()
	p.SeedLegacyThreadFollowState("U1", "R1", "ROOT", ThreadFollowStateFollowing)

	if got := p.FollowState("U1", "R1", "ROOT"); got != ThreadFollowStateFollowing {
		t.Fatalf("seeded FollowState = %q, want following", got)
	}
	if followers := p.ThreadFollowers("R1", "ROOT"); !slices.Equal(followers, []string{"U1"}) {
		t.Fatalf("seeded ThreadFollowers = %v, want [U1]", followers)
	}

	applyAll(t, p, []*corev1.Event{
		{
			Id:      "UNFOLLOW-U1",
			ActorId: "U1",
			Event: &corev1.Event_ThreadUnfollowed{
				ThreadUnfollowed: &corev1.ThreadUnfollowedEvent{
					RoomId:            "R1",
					ThreadRootEventId: "ROOT",
					UserId:            "U1",
				},
			},
		},
	})

	if got := p.FollowState("U1", "R1", "ROOT"); got != ThreadFollowStateUnfollowed {
		t.Fatalf("EVT should override seeded legacy state, got %q", got)
	}
}

func TestThreadProjection_SubjectFilter(t *testing.T) {
	subjects := NewThreadProjection().Subjects()
	want := map[string]bool{
		events.RoomEventTypeFilter(events.EventThreadCreated):    true,
		events.RoomEventTypeFilter(events.EventThreadFollowed):   true,
		events.RoomEventTypeFilter(events.EventThreadUnfollowed): true,
		events.RoomEventTypeFilter(events.EventMessagePosted):    true,
		events.RoomEventTypeFilter(events.EventMessageEdited):    true,
		events.RoomEventTypeFilter(events.EventMessageRetracted): true,
		events.UserEventTypeFilter(events.EventUserKeyShredded):  true,
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
	if slices.Contains(subjects, events.RoomSubjectFilter()) {
		t.Errorf("unexpected broad room subject filter %q", events.RoomSubjectFilter())
	}
}
