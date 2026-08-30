package core

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestThreadProjectionSnapshotRoundTripAndTailReplay(t *testing.T) {
	full := NewThreadProjection()
	eventsBefore := []*evtv1.Event{
		roomCreatedTimelineEvent("ROOM", "R1", "room", 1),
		postedEvent(postedOpts{
			envelopeID: "ROOT", eventID: "ROOT", roomID: "R1", actorID: "U1", at: 2,
			mentions: []*evtv1.MessageMention{directThreadMention("U2")},
		}),
		threadCreatedEvent("THREAD", "R1", "ROOT", "U1", 1),
		postedEvent(postedOpts{envelopeID: "REPLY-1", eventID: "REPLY-1", roomID: "R1", actorID: "U2", inThread: "ROOT", at: 2}),
		postedEvent(postedOpts{envelopeID: "REPLY-2", eventID: "REPLY-2", roomID: "R1", actorID: "U3", inThread: "ROOT", at: 3}),
		threadFollowSnapshotTestEvent("FOLLOW", "R1", "ROOT", "U2", true),
		retractedEvent("RETRACT", "REPLY-2", "R1", "U3", "removed", 5),
		userKeyShreddedSnapshotTestEvent("SHRED", "U3"),
	}
	applyAll(t, full, eventsBefore)
	// Historical duplicate IDs activate the replay guard's compatibility mode,
	// which must survive snapshot restore for first-event-wins behavior.
	if err := full.Apply(eventsBefore[3], 9); err != nil {
		t.Fatal(err)
	}
	full.CompleteStartupReplay()

	first, err := full.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	second, err := full.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("Thread snapshot encoding is not deterministic")
	}

	restored := NewThreadProjection()
	if err := restored.Restore(first); err != nil {
		t.Fatal(err)
	}
	restoredBytes, err := restored.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, restoredBytes) {
		t.Fatal("restored canonical Thread state differs from captured state")
	}

	tail := postedEvent(postedOpts{
		envelopeID: "REPLY-3", eventID: "REPLY-3", roomID: "R1", actorID: "U4", inThread: "ROOT", at: 8,
		mentions: []*evtv1.MessageMention{directThreadMention("U2"), directThreadMention("U4")},
	})
	if err := full.Apply(tail, 10); err != nil {
		t.Fatal(err)
	}
	if err := restored.Apply(tail, 10); err != nil {
		t.Fatal(err)
	}
	fullBytes, _ := full.Snapshot()
	restoredBytes, _ = restored.Snapshot()
	if !bytes.Equal(fullBytes, restoredBytes) {
		t.Fatal("snapshot plus tail differs from full projection state")
	}
	if got := restored.ReplyCount("ROOT"); got != 2 {
		t.Fatalf("ReplyCount after restore and tail = %d, want 2", got)
	}
	if got := restored.FollowState("U2", "R1", "ROOT"); got != ThreadFollowStateFollowing {
		t.Fatalf("FollowState after restore = %q", got)
	}
	interaction, ok := restored.Interaction("U2", "R1", "ROOT")
	if !ok || len(interaction.Causes) != 2 {
		t.Fatalf("U2 interaction after restore and tail = %#v, %v; want two direct-mention facts", interaction, ok)
	}
}

func TestThreadProjectionSnapshotContractID(t *testing.T) {
	if got := NewThreadProjection().SnapshotContractID(); !strings.HasPrefix(got, "v2-") {
		t.Fatalf("SnapshotContractID() = %q, want v2 schema contract", got)
	}
}

func TestThreadProjectionSnapshotRestoreFailureIsTransactional(t *testing.T) {
	p := NewThreadProjection()
	if err := p.Apply(threadFollowSnapshotTestEvent("FOLLOW", "R1", "ROOT", "U1", true), 1); err != nil {
		t.Fatal(err)
	}
	if err := p.Restore([]byte("not protobuf")); err == nil {
		t.Fatal("Restore accepted malformed snapshot")
	}
	if got := p.FollowState("U1", "R1", "ROOT"); got != ThreadFollowStateFollowing {
		t.Fatalf("canonical follow state after failed restore = %q", got)
	}
}

func threadFollowSnapshotTestEvent(id, roomID, rootID, userID string, following bool) *evtv1.Event {
	if following {
		return &evtv1.Event{Id: id, Event: &evtv1.Event_ThreadFollowed{ThreadFollowed: &evtv1.ThreadFollowedEvent{RoomId: roomID, ThreadRootEventId: rootID, UserId: userID}}}
	}
	return &evtv1.Event{Id: id, Event: &evtv1.Event_ThreadUnfollowed{ThreadUnfollowed: &evtv1.ThreadUnfollowedEvent{RoomId: roomID, ThreadRootEventId: rootID, UserId: userID}}}
}

func userKeyShreddedSnapshotTestEvent(id, userID string) *evtv1.Event {
	return &evtv1.Event{Id: id, Event: &evtv1.Event_UserKeyShredded{UserKeyShredded: &evtv1.UserKeyShreddedEvent{UserId: userID}}}
}

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

func directThreadMention(userID string) *evtv1.MessageMention {
	return &evtv1.MessageMention{UserId: userID, Cause: &evtv1.MessageMention_Direct{Direct: &evtv1.DirectUserMention{}}}
}

func TestThreadProjection_DerivesInteractionRelationshipsFromTypedMessageFacts(t *testing.T) {
	p := NewThreadProjection()
	root := postedEvent(postedOpts{
		envelopeID: "ROOT", roomID: "R1", actorID: "AUTHOR", at: 2,
		mentionedUserIDs: []string{"LEGACY"},
		mentions: []*evtv1.MessageMention{
			directThreadMention("DIRECT"),
			directThreadMention("AUTHOR"),
			{UserId: "ROLE", Cause: &evtv1.MessageMention_Role{Role: &evtv1.RoleMessageMention{RoleName: "helper"}}},
			{UserId: "HERE", Cause: &evtv1.MessageMention_Here{Here: &evtv1.HereMessageMention{}}},
			{UserId: "ALL", Cause: &evtv1.MessageMention_All{All: &evtv1.AllMessageMention{}}},
		},
	})
	reply := postedEvent(postedOpts{
		envelopeID: "REPLY", roomID: "R1", actorID: "REPLIER", inThread: "ROOT", at: 3,
		mentions: []*evtv1.MessageMention{directThreadMention("DIRECT"), directThreadMention("REPLY-MENTION")},
	})
	echo := postedEvent(postedOpts{
		envelopeID: "ECHO", roomID: "R1", actorID: "ECHO-AUTHOR", echoOfEventID: "REPLY",
		echoFromThreadRootEventID: "ROOT", at: 4, mentions: []*evtv1.MessageMention{directThreadMention("ECHO-MENTION")},
	})
	partialEcho := postedEvent(postedOpts{
		envelopeID: "PARTIAL-ECHO", roomID: "R1", actorID: "PARTIAL-ECHO-AUTHOR",
		echoFromThreadRootEventID: "ROOT", at: 4, mentions: []*evtv1.MessageMention{directThreadMention("PARTIAL-ECHO-MENTION")},
	})
	applyAll(t, p, []*evtv1.Event{
		roomCreatedTimelineEvent("ROOM", "R1", "room", 1), root, reply, echo, partialEcho,
		editedEvent("EDIT", "REPLY", "R1", "REPLIER", "edited", 5),
		retractedEvent("RETRACT", "REPLY", "R1", "REPLIER", "removed", 6),
		roomCreatedEvent("DM1", "", "", evtv1.RoomKind_ROOM_KIND_DM),
		postedEvent(postedOpts{envelopeID: "DM-MESSAGE", roomID: "DM1", actorID: "DM-AUTHOR", at: 8, mentions: []*evtv1.MessageMention{directThreadMention("DM-MENTION")}}),
	})

	for _, userID := range []string{"AUTHOR", "DIRECT", "REPLY-MENTION"} {
		if !p.HasInteraction(userID, "R1", "ROOT") {
			t.Errorf("HasInteraction(%s) = false, want true", userID)
		}
	}
	for _, userID := range []string{"LEGACY", "ROLE", "HERE", "ALL", "REPLIER", "ECHO-AUTHOR", "ECHO-MENTION", "PARTIAL-ECHO-AUTHOR", "PARTIAL-ECHO-MENTION", "DM-AUTHOR", "DM-MENTION"} {
		if p.HasInteraction(userID, "R1", "ROOT") || p.HasInteraction(userID, "DM1", "DM-MESSAGE") {
			t.Errorf("unexpected interaction for %s", userID)
		}
	}
	interaction, ok := p.Interaction("DIRECT", "R1", "ROOT")
	if !ok || len(interaction.Causes) != 2 || interaction.Causes[0].SourceEventID != "ROOT" || interaction.Causes[1].SourceEventID != "REPLY" {
		t.Fatalf("DIRECT interaction = %#v, %v; want root and reply mention facts", interaction, ok)
	}
	for eventID, wantRoot := range map[string]string{"ROOT": "ROOT", "REPLY": "ROOT", "ECHO": "ROOT", "PARTIAL-ECHO": "ROOT"} {
		if got, ok := p.ThreadRootForMessage("R1", eventID); !ok || got != wantRoot {
			t.Errorf("ThreadRootForMessage(%s) = %q, %v; want %q, true", eventID, got, ok, wantRoot)
		}
	}
}

func TestThreadProjection_InteractionCauseOrderIsDeterministic(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*evtv1.Event{
		roomCreatedTimelineEvent("ROOM", "R1", "room", 1),
		postedEvent(postedOpts{envelopeID: "ROOT", roomID: "R1", actorID: "AUTHOR", at: 2}),
		postedEvent(postedOpts{envelopeID: "MENTION-Z", roomID: "R1", actorID: "AUTHOR", inThread: "ROOT", at: 3, mentions: []*evtv1.MessageMention{directThreadMention("TARGET")}}),
		postedEvent(postedOpts{envelopeID: "MENTION-A", roomID: "R1", actorID: "AUTHOR", inThread: "ROOT", at: 3, mentions: []*evtv1.MessageMention{directThreadMention("TARGET")}}),
	})

	interaction, ok := p.Interaction("TARGET", "R1", "ROOT")
	if !ok || len(interaction.Causes) != 2 {
		t.Fatalf("Interaction = %#v, %v; want two causes", interaction, ok)
	}
	if interaction.Causes[0].SourceEventID != "MENTION-A" || interaction.Causes[1].SourceEventID != "MENTION-Z" {
		t.Fatalf("cause order = %#v; want source event ID tie-break", interaction.Causes)
	}
}

func TestThreadProjection_RootMessageNotStored(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*evtv1.Event{
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
	applyAll(t, p, []*evtv1.Event{
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
	applyAll(t, p, []*evtv1.Event{
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
	if got, want := metadata.LatestReplyEventID, "ENV-R2"; got != want {
		t.Errorf("ThreadMetadata LatestReplyEventID = %q, want %q", got, want)
	}
	if !slices.Equal(metadata.ParticipantIDs, []string{"U2", "U3"}) {
		t.Errorf("ThreadMetadata ParticipantIDs = %v, want [U2 U3]", metadata.ParticipantIDs)
	}
}

func TestThreadProjection_LatestReplyUsesStreamOrderDespiteClockSkew(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*evtv1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-R1", eventID: "REPLY1", roomID: "R1", actorID: "U1", inThread: "ROOT", at: 3}),
		postedEvent(postedOpts{envelopeID: "ENV-R2", eventID: "REPLY2", roomID: "R1", actorID: "U2", inThread: "ROOT", at: 2}),
	})

	metadata := p.ThreadMetadata("ROOT")
	if got, want := metadata.LatestReplyEventID, "ENV-R2"; got != want {
		t.Fatalf("LatestReplyEventID = %q, want stream-latest %q", got, want)
	}
	if metadata.LastReplyAt == nil || !metadata.LastReplyAt.Equal(fixedTime(2)) {
		t.Fatalf("LastReplyAt = %v, want stream-latest time %v", metadata.LastReplyAt, fixedTime(2))
	}
}

func TestThreadProjection_LatestReplyRetainsZeroTimestampEvent(t *testing.T) {
	p := NewThreadProjection()
	first := postedEvent(postedOpts{envelopeID: "ENV-R1", eventID: "REPLY1", roomID: "R1", actorID: "U1", inThread: "ROOT", at: 1})
	latest := postedEvent(postedOpts{envelopeID: "ENV-R2", eventID: "REPLY2", roomID: "R1", actorID: "U2", inThread: "ROOT", at: 2})
	latest.CreatedAt = nil
	applyAll(t, p, []*evtv1.Event{first, latest})

	metadata := p.ThreadMetadata("ROOT")
	if got, want := metadata.LatestReplyEventID, "ENV-R2"; got != want {
		t.Fatalf("LatestReplyEventID = %q, want stream-latest %q", got, want)
	}
	if metadata.LastReplyAt != nil {
		t.Fatalf("LastReplyAt = %v, want nil for zero-timestamp latest reply", metadata.LastReplyAt)
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
	applyAll(t, p, []*evtv1.Event{
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
	applyAll(t, p, []*evtv1.Event{
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
	applyAll(t, p, []*evtv1.Event{
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
	applyAll(t, p, []*evtv1.Event{
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
	applyAll(t, p, []*evtv1.Event{
		editedEvent("ENV-EDIT", "REPLY1", "R1", "U2", "edited", 1),
	})
	if got := p.ThreadCount(); got != 0 {
		t.Errorf("Out-of-order edit shouldn't create a thread, got ThreadCount=%d", got)
	}
}

func TestThreadProjection_MultipleThreadsIsolated(t *testing.T) {
	p := NewThreadProjection()
	applyAll(t, p, []*evtv1.Event{
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
	applyAll(t, p, []*evtv1.Event{
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
	if got, want := metadata.LatestReplyEventID, "ENV-R1"; got != want {
		t.Errorf("LatestReplyEventID = %q, want %q", got, want)
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
	applyAll(t, p, []*evtv1.Event{
		{
			Id:      "FOLLOW-U1",
			ActorId: "U1",
			Event: &evtv1.Event_ThreadFollowed{
				ThreadFollowed: &evtv1.ThreadFollowedEvent{
					RoomId:            "R1",
					ThreadRootEventId: "ROOT",
					UserId:            "U1",
					Source:            evtv1.ThreadFollowSource_THREAD_FOLLOW_SOURCE_MANUAL,
				},
			},
		},
		{
			Id:      "FOLLOW-U2",
			ActorId: "U2",
			Event: &evtv1.Event_ThreadFollowed{
				ThreadFollowed: &evtv1.ThreadFollowedEvent{
					RoomId:            "R1",
					ThreadRootEventId: "ROOT",
					UserId:            "U2",
					Source:            evtv1.ThreadFollowSource_THREAD_FOLLOW_SOURCE_DIRECT_MENTION,
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

	applyAll(t, p, []*evtv1.Event{
		{
			Id:      "UNFOLLOW-U1",
			ActorId: "U1",
			Event: &evtv1.Event_ThreadUnfollowed{
				ThreadUnfollowed: &evtv1.ThreadUnfollowedEvent{
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

func TestThreadProjection_SubjectFilter(t *testing.T) {
	subjects := NewThreadProjection().Subjects()
	want := map[string]bool{
		evtstream.RoomEventTypeFilter(evtstream.EventRoomCreated):               true,
		evtstream.RoomEventTypeFilter(evtstream.EventRoomDeleted):               true,
		evtstream.RoomEventTypeFilter(evtstream.EventThreadCreated):             true,
		evtstream.RoomEventTypeFilter(evtstream.EventThreadFollowed):            true,
		evtstream.RoomEventTypeFilter(evtstream.EventThreadUnfollowed):          true,
		evtstream.RoomEventTypeFilter(evtstream.EventMessagePosted):             true,
		evtstream.RoomEventTypeFilter(evtstream.EventMessageEdited):             true,
		evtstream.RoomEventTypeFilter(evtstream.EventMessageRetracted):          true,
		evtstream.UserEventTypeFilter(evtstream.EventUserKeyShreddingRequested): true,
		evtstream.UserEventTypeFilter(evtstream.EventUserKeyShredded):           true,
	}
	if len(subjects) != len(want) {
		t.Fatalf("expected %d subject filters, got %d", len(want), len(subjects))
	}
	for subject := range want {
		if !slices.Contains(subjects, subject) {
			t.Errorf("missing subject filter %q", subject)
		}
	}
	if slices.Contains(subjects, evtstream.UserSubjectFilter()) {
		t.Errorf("unexpected broad user subject filter %q", evtstream.UserSubjectFilter())
	}
	if slices.Contains(subjects, evtstream.RoomSubjectFilter()) {
		t.Errorf("unexpected broad room subject filter %q", evtstream.RoomSubjectFilter())
	}
}
