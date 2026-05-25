package core

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// =============================================================================
// Test helpers
// =============================================================================

// fixedTime returns a deterministic timestamp seeded from a small int,
// so tests can assert on PostedAt / EditedAt without timing flakiness.
func fixedTime(seed int) time.Time {
	return time.Date(2026, 1, 1, 12, 0, seed, 0, time.UTC)
}

// postedEvent builds a *corev1.Event carrying MessagePostedEvent
// with the fields tests usually want to set. Any unspecified field is
// zero-valued.
type postedOpts struct {
	eventID                   string
	roomID                    string
	actorID                   string
	body                      string // plaintext for fixture; encryption is out of scope here
	inReplyTo                 string
	inThread                  string
	echoOfEventID             string
	echoFromThreadRootEventID string
	at                        int // seed for fixedTime
}

func postedEvent(opts postedOpts) *corev1.Event {
	return &corev1.Event{
		Id:        "envelope-" + opts.eventID,
		ActorId:   opts.actorID,
		CreatedAt: timestamppb.New(fixedTime(opts.at)),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				RoomId:                    opts.roomID,
				EventId:                   opts.eventID,
				InReplyTo:                 opts.inReplyTo,
				InThread:                  opts.inThread,
				EchoOfEventId:             opts.echoOfEventID,
				EchoFromThreadRootEventId: opts.echoFromThreadRootEventID,
				Body: &corev1.MessageBody{
					AuthorId:      opts.actorID,
					EncryptedBody: []byte(opts.body),
				},
			},
		},
	}
}

func editedEvent(eventID, roomID, actorID, newBody string, at int) *corev1.Event {
	return &corev1.Event{
		Id:        "envelope-edit-" + eventID,
		ActorId:   actorID,
		CreatedAt: timestamppb.New(fixedTime(at)),
		Event: &corev1.Event_MessageEdited{
			MessageEdited: &corev1.MessageEditedEvent{
				RoomId:  roomID,
				EventId: eventID,
				Body: &corev1.MessageBody{
					AuthorId:      actorID,
					EncryptedBody: []byte(newBody),
				},
			},
		},
	}
}

func retractedEvent(eventID, roomID, actorID, reason string, at int) *corev1.Event {
	return &corev1.Event{
		Id:        "envelope-retract-" + eventID,
		ActorId:   actorID,
		CreatedAt: timestamppb.New(fixedTime(at)),
		Event: &corev1.Event_MessageRetracted{
			MessageRetracted: &corev1.MessageRetractedEvent{
				RoomId:  roomID,
				EventId: eventID,
				Reason:  reason,
			},
		},
	}
}

// applyAll feeds events into a projection in order, with monotonically
// increasing stream sequences starting at 1. Matches what the real
// Projector does.
func applyAll(t *testing.T, p interface {
	Apply(*corev1.Event, uint64) error
}, events []*corev1.Event) {
	t.Helper()
	for i, e := range events {
		if err := p.Apply(e, uint64(i+1)); err != nil {
			t.Fatalf("Apply event %d: %v", i+1, err)
		}
	}
}

// =============================================================================
// MessagesProjection
// =============================================================================

func TestMessagesProjection_Empty(t *testing.T) {
	p := NewMessagesProjection()

	if _, ok := p.Get("any"); ok {
		t.Error("Get on empty projection should return ok=false")
	}
	if got := p.RoomMessageCount("R1"); got != 0 {
		t.Errorf("RoomMessageCount empty = %d, want 0", got)
	}
	if got := p.RoomTimeline("R1", 50, 0); len(got) != 0 {
		t.Errorf("RoomTimeline empty = %d entries, want 0", len(got))
	}
}

func TestMessagesProjection_PostAndLookup(t *testing.T) {
	p := NewMessagesProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "M1", roomID: "R1", actorID: "U1", body: "hello", at: 1}),
	})

	msg, ok := p.Get("M1")
	if !ok {
		t.Fatalf("Get(M1) ok=false, want true")
	}
	if msg.EventID != "M1" || msg.RoomID != "R1" || msg.AuthorID != "U1" {
		t.Errorf("Get(M1) returned %+v", msg)
	}
	if string(msg.Body.GetEncryptedBody()) != "hello" {
		t.Errorf("Body mismatch: got %q", msg.Body.GetEncryptedBody())
	}
	if !msg.PostedAt.Equal(fixedTime(1)) {
		t.Errorf("PostedAt = %v, want %v", msg.PostedAt, fixedTime(1))
	}
	if msg.EditedAt != nil {
		t.Errorf("EditedAt should be nil on fresh post, got %v", msg.EditedAt)
	}
	if msg.Tombstoned {
		t.Errorf("Tombstoned should be false on fresh post")
	}
	if msg.StreamSeq != 1 {
		t.Errorf("StreamSeq = %d, want 1", msg.StreamSeq)
	}

	if got := p.RoomMessageCount("R1"); got != 1 {
		t.Errorf("RoomMessageCount = %d, want 1", got)
	}
	timeline := p.RoomTimeline("R1", 50, 0)
	if len(timeline) != 1 || timeline[0].EventID != "M1" {
		t.Errorf("RoomTimeline = %+v, want one entry M1", timeline)
	}
}

func TestMessagesProjection_RoomTimelineOrderingAndPagination(t *testing.T) {
	p := NewMessagesProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "M1", roomID: "R1", actorID: "U1", body: "first", at: 1}),
		postedEvent(postedOpts{eventID: "M2", roomID: "R1", actorID: "U1", body: "second", at: 2}),
		postedEvent(postedOpts{eventID: "M3", roomID: "R1", actorID: "U1", body: "third", at: 3}),
		postedEvent(postedOpts{eventID: "OTHER", roomID: "R2", actorID: "U1", body: "elsewhere", at: 4}),
	})

	// Newest-first ordering.
	timeline := p.RoomTimeline("R1", 10, 0)
	if len(timeline) != 3 {
		t.Fatalf("RoomTimeline len = %d, want 3", len(timeline))
	}
	wantOrder := []string{"M3", "M2", "M1"}
	for i, msg := range timeline {
		if msg.EventID != wantOrder[i] {
			t.Errorf("timeline[%d] = %q, want %q", i, msg.EventID, wantOrder[i])
		}
	}

	// Room isolation.
	other := p.RoomTimeline("R2", 10, 0)
	if len(other) != 1 || other[0].EventID != "OTHER" {
		t.Errorf("R2 timeline = %+v, want one entry OTHER", other)
	}

	// Limit.
	limited := p.RoomTimeline("R1", 2, 0)
	if len(limited) != 2 || limited[0].EventID != "M3" || limited[1].EventID != "M2" {
		t.Errorf("limit=2 timeline = %+v, want [M3, M2]", limited)
	}

	// beforeStreamSeq cursor — M2 has streamSeq=2, beforeStreamSeq=2 excludes it
	// and everything newer, so we get only M1.
	older := p.RoomTimeline("R1", 10, 2)
	if len(older) != 1 || older[0].EventID != "M1" {
		t.Errorf("before=2 timeline = %+v, want [M1]", older)
	}
}

func TestMessagesProjection_EditOverlay(t *testing.T) {
	p := NewMessagesProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "M1", roomID: "R1", actorID: "U1", body: "original", at: 1}),
		editedEvent("M1", "R1", "U1", "edited!", 2),
	})

	msg, _ := p.Get("M1")
	if string(msg.Body.GetEncryptedBody()) != "edited!" {
		t.Errorf("Body after edit = %q, want %q", msg.Body.GetEncryptedBody(), "edited!")
	}
	if msg.EditedAt == nil {
		t.Fatalf("EditedAt should be set after edit")
	}
	if !msg.EditedAt.Equal(fixedTime(2)) {
		t.Errorf("EditedAt = %v, want %v", *msg.EditedAt, fixedTime(2))
	}
	if !msg.PostedAt.Equal(fixedTime(1)) {
		t.Errorf("PostedAt should be unchanged by edit, got %v", msg.PostedAt)
	}
}

func TestMessagesProjection_EditBeforePost_IsNoOp(t *testing.T) {
	p := NewMessagesProjection()
	applyAll(t, p, []*corev1.Event{
		editedEvent("M1", "R1", "U1", "shouldn't matter", 1),
	})

	if _, ok := p.Get("M1"); ok {
		t.Error("Edit-before-post should not create an entry")
	}
}

func TestMessagesProjection_RetractTombstone(t *testing.T) {
	p := NewMessagesProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "M1", roomID: "R1", actorID: "U1", body: "spam", at: 1}),
		retractedEvent("M1", "R1", "MOD", "spam", 2),
	})

	msg, ok := p.Get("M1")
	if !ok {
		t.Fatalf("Get(M1) should still return the entry after retract")
	}
	if !msg.Tombstoned {
		t.Error("Tombstoned should be true after retract")
	}
	if msg.RetractReason != "spam" {
		t.Errorf("RetractReason = %q, want %q", msg.RetractReason, "spam")
	}
	// Body preserved per type doc — retraction is a flag, not a wipe.
	if string(msg.Body.GetEncryptedBody()) != "spam" {
		t.Errorf("Body after retract should be preserved, got %q", msg.Body.GetEncryptedBody())
	}
	// Still in the timeline.
	timeline := p.RoomTimeline("R1", 10, 0)
	if len(timeline) != 1 || !timeline[0].Tombstoned {
		t.Errorf("Retracted message should still appear in timeline (tombstoned=true), got %+v", timeline)
	}
}

func TestMessagesProjection_RetractBeforePost_IsNoOp(t *testing.T) {
	p := NewMessagesProjection()
	applyAll(t, p, []*corev1.Event{
		retractedEvent("M1", "R1", "MOD", "spam", 1),
	})

	if _, ok := p.Get("M1"); ok {
		t.Error("Retract-before-post should not create an entry")
	}
}

func TestMessagesProjection_Idempotency(t *testing.T) {
	p := NewMessagesProjection()
	posted := postedEvent(postedOpts{eventID: "M1", roomID: "R1", actorID: "U1", body: "hello", at: 1})
	// Apply the same event twice with the same seq.
	if err := p.Apply(posted, 1); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if err := p.Apply(posted, 1); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	// Should still only appear once in the room timeline.
	if got := p.RoomMessageCount("R1"); got != 1 {
		t.Errorf("RoomMessageCount after duplicate Apply = %d, want 1", got)
	}
}

func TestMessagesProjection_ThreadRepliesExcludedFromRoomTimeline(t *testing.T) {
	p := NewMessagesProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "ROOT", roomID: "R1", actorID: "U1", body: "root", at: 1}),
		postedEvent(postedOpts{eventID: "REPLY1", roomID: "R1", actorID: "U2", body: "reply", inThread: "ROOT", inReplyTo: "ROOT", at: 2}),
		postedEvent(postedOpts{eventID: "REPLY2", roomID: "R1", actorID: "U1", body: "reply2", inThread: "ROOT", inReplyTo: "REPLY1", at: 3}),
	})

	// All three are in byEventID.
	for _, id := range []string{"ROOT", "REPLY1", "REPLY2"} {
		if _, ok := p.Get(id); !ok {
			t.Errorf("Get(%s) should succeed", id)
		}
	}
	// RoomMessageCount counts everything in the room.
	if got := p.RoomMessageCount("R1"); got != 3 {
		t.Errorf("RoomMessageCount = %d, want 3", got)
	}
	// But RoomTimeline only returns the root.
	timeline := p.RoomTimeline("R1", 10, 0)
	if len(timeline) != 1 || timeline[0].EventID != "ROOT" {
		t.Errorf("RoomTimeline should exclude thread replies, got %+v", timeline)
	}
}

func TestMessagesProjection_EchoAppearsInRoomTimeline(t *testing.T) {
	p := NewMessagesProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "ROOT", roomID: "R1", actorID: "U1", body: "root", at: 1}),
		postedEvent(postedOpts{eventID: "REPLY", roomID: "R1", actorID: "U2", body: "reply", inThread: "ROOT", inReplyTo: "ROOT", at: 2}),
		// An echo: root-level message that references the thread reply.
		// Echoes have inThread == "" so they pass the root-timeline filter.
		postedEvent(postedOpts{
			eventID:                   "ECHO",
			roomID:                    "R1",
			actorID:                   "U2",
			body:                      "reply",
			echoOfEventID:             "REPLY",
			echoFromThreadRootEventID: "ROOT",
			at:                        3,
		}),
	})

	timeline := p.RoomTimeline("R1", 10, 0)
	if len(timeline) != 2 {
		t.Fatalf("RoomTimeline len = %d, want 2 (ROOT + ECHO)", len(timeline))
	}
	// Newest-first → ECHO, then ROOT.
	if timeline[0].EventID != "ECHO" || timeline[1].EventID != "ROOT" {
		t.Errorf("timeline = [%s, %s], want [ECHO, ROOT]", timeline[0].EventID, timeline[1].EventID)
	}
	if timeline[0].EchoOfEventID != "REPLY" {
		t.Errorf("Echo metadata not preserved: %+v", timeline[0])
	}
}

// =============================================================================
// ThreadsProjection
// =============================================================================

func TestThreadsProjection_Empty(t *testing.T) {
	p := NewThreadsProjection()

	if _, ok := p.Thread("ROOT"); ok {
		t.Error("Thread on empty projection should return ok=false")
	}
	if got := p.Count(); got != 0 {
		t.Errorf("Count empty = %d, want 0", got)
	}
	if got := p.MetadataForRoots([]string{"ROOT"}); len(got) != 0 {
		t.Errorf("MetadataForRoots empty = %d entries, want 0", len(got))
	}
}

func TestThreadsProjection_RootMessageDoesNotCreateThread(t *testing.T) {
	p := NewThreadsProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "ROOT", roomID: "R1", actorID: "U1", body: "root", at: 1}),
	})
	if got := p.Count(); got != 0 {
		t.Errorf("Posting a root message should not create a thread, got Count=%d", got)
	}
}

func TestThreadsProjection_SingleReply(t *testing.T) {
	p := NewThreadsProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "ROOT", roomID: "R1", actorID: "U1", body: "root", at: 1}),
		postedEvent(postedOpts{eventID: "REPLY1", roomID: "R1", actorID: "U2", body: "reply", inThread: "ROOT", inReplyTo: "ROOT", at: 2}),
	})

	thread, ok := p.Thread("ROOT")
	if !ok {
		t.Fatalf("Thread(ROOT) ok=false, want true")
	}
	if thread.ThreadRootEventID != "ROOT" || thread.RoomID != "R1" {
		t.Errorf("Thread fields wrong: %+v", thread)
	}
	if thread.ReplyCount != 1 {
		t.Errorf("ReplyCount = %d, want 1", thread.ReplyCount)
	}
	if len(thread.ReplyEventIDs) != 1 || thread.ReplyEventIDs[0] != "REPLY1" {
		t.Errorf("ReplyEventIDs = %v, want [REPLY1]", thread.ReplyEventIDs)
	}
	if thread.LastReplyAt == nil || !thread.LastReplyAt.Equal(fixedTime(2)) {
		t.Errorf("LastReplyAt = %v, want %v", thread.LastReplyAt, fixedTime(2))
	}
	if len(thread.ParticipantIDs) != 1 || thread.ParticipantIDs[0] != "U2" {
		t.Errorf("ParticipantIDs = %v, want [U2]", thread.ParticipantIDs)
	}
}

func TestThreadsProjection_MultipleRepliesAndParticipants(t *testing.T) {
	p := NewThreadsProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "ROOT", roomID: "R1", actorID: "U1", body: "root", at: 1}),
		postedEvent(postedOpts{eventID: "R1A", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", at: 2}),
		postedEvent(postedOpts{eventID: "R1B", roomID: "R1", actorID: "U3", inThread: "ROOT", inReplyTo: "R1A", at: 3}),
		postedEvent(postedOpts{eventID: "R1C", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "R1B", at: 4}),
	})

	thread, ok := p.Thread("ROOT")
	if !ok {
		t.Fatalf("Thread(ROOT) ok=false, want true")
	}
	if thread.ReplyCount != 3 {
		t.Errorf("ReplyCount = %d, want 3", thread.ReplyCount)
	}
	if !thread.LastReplyAt.Equal(fixedTime(4)) {
		t.Errorf("LastReplyAt = %v, want %v", thread.LastReplyAt, fixedTime(4))
	}
	// Order preserved.
	wantOrder := []string{"R1A", "R1B", "R1C"}
	for i, id := range thread.ReplyEventIDs {
		if id != wantOrder[i] {
			t.Errorf("ReplyEventIDs[%d] = %q, want %q", i, id, wantOrder[i])
		}
	}
	// Participant set: U2 and U3 (not U1, the root author who didn't reply).
	gotParticipants := map[string]bool{}
	for _, p := range thread.ParticipantIDs {
		gotParticipants[p] = true
	}
	if !gotParticipants["U2"] || !gotParticipants["U3"] {
		t.Errorf("ParticipantIDs missing U2 or U3: %v", thread.ParticipantIDs)
	}
	if gotParticipants["U1"] {
		t.Errorf("ParticipantIDs should NOT include the root author who didn't reply: %v", thread.ParticipantIDs)
	}
}

func TestThreadsProjection_MultipleThreadsIsolated(t *testing.T) {
	p := NewThreadsProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "T1", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{eventID: "T2", roomID: "R1", actorID: "U1", at: 2}),
		postedEvent(postedOpts{eventID: "T1A", roomID: "R1", actorID: "U2", inThread: "T1", inReplyTo: "T1", at: 3}),
		postedEvent(postedOpts{eventID: "T2A", roomID: "R1", actorID: "U3", inThread: "T2", inReplyTo: "T2", at: 4}),
		postedEvent(postedOpts{eventID: "T2B", roomID: "R1", actorID: "U4", inThread: "T2", inReplyTo: "T2A", at: 5}),
	})

	t1, _ := p.Thread("T1")
	t2, _ := p.Thread("T2")
	if t1.ReplyCount != 1 {
		t.Errorf("T1 ReplyCount = %d, want 1", t1.ReplyCount)
	}
	if t2.ReplyCount != 2 {
		t.Errorf("T2 ReplyCount = %d, want 2", t2.ReplyCount)
	}
}

func TestThreadsProjection_EchoDoesNotCount(t *testing.T) {
	p := NewThreadsProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{eventID: "REPLY", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", at: 2}),
		// Echo: root-level (inThread == ""), references REPLY. Must NOT
		// increment ReplyCount — it's a mirror of REPLY, not a new reply.
		postedEvent(postedOpts{
			eventID:                   "ECHO",
			roomID:                    "R1",
			actorID:                   "U2",
			echoOfEventID:             "REPLY",
			echoFromThreadRootEventID: "ROOT",
			at:                        3,
		}),
	})

	thread, _ := p.Thread("ROOT")
	if thread.ReplyCount != 1 {
		t.Errorf("Echo should not bump ReplyCount, got %d (want 1)", thread.ReplyCount)
	}
}

func TestThreadsProjection_RetractDoesNotDecrement(t *testing.T) {
	p := NewThreadsProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{eventID: "R1A", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", at: 2}),
		postedEvent(postedOpts{eventID: "R1B", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "R1A", at: 3}),
		retractedEvent("R1A", "R1", "MOD", "spam", 4),
	})

	thread, _ := p.Thread("ROOT")
	// Per design decision: retract is monotonic, count stays at 2.
	if thread.ReplyCount != 2 {
		t.Errorf("Retract should not decrement ReplyCount, got %d (want 2)", thread.ReplyCount)
	}
}

func TestThreadsProjection_EditDoesNotAffectMetadata(t *testing.T) {
	p := NewThreadsProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{eventID: "R1A", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", at: 2}),
		editedEvent("R1A", "R1", "U2", "edited body", 5),
	})

	thread, _ := p.Thread("ROOT")
	if thread.ReplyCount != 1 {
		t.Errorf("Edit should not affect ReplyCount, got %d (want 1)", thread.ReplyCount)
	}
	// LastReplyAt stays at the original reply time, not the edit time.
	if !thread.LastReplyAt.Equal(fixedTime(2)) {
		t.Errorf("Edit should not bump LastReplyAt, got %v (want %v)", thread.LastReplyAt, fixedTime(2))
	}
}

func TestThreadsProjection_MetadataForRoots_Bulk(t *testing.T) {
	p := NewThreadsProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{eventID: "T1", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{eventID: "T2", roomID: "R1", actorID: "U1", at: 2}),
		postedEvent(postedOpts{eventID: "T1A", roomID: "R1", actorID: "U2", inThread: "T1", inReplyTo: "T1", at: 3}),
	})

	got := p.MetadataForRoots([]string{"T1", "T2", "NEVER_REPLIED"})
	// T1 has a reply, present in map.
	if _, ok := got["T1"]; !ok {
		t.Error("T1 should be present in MetadataForRoots")
	}
	if got["T1"].ReplyCount != 1 {
		t.Errorf("T1 ReplyCount via bulk = %d, want 1", got["T1"].ReplyCount)
	}
	// T2 has no replies → absent.
	if _, ok := got["T2"]; ok {
		t.Error("T2 with no replies should be absent from MetadataForRoots")
	}
	// Unknown root → absent.
	if _, ok := got["NEVER_REPLIED"]; ok {
		t.Error("unknown root should be absent")
	}
}

func TestThreadsProjection_Idempotency(t *testing.T) {
	p := NewThreadsProjection()
	reply := postedEvent(postedOpts{eventID: "R1A", roomID: "R1", actorID: "U2", inThread: "ROOT", inReplyTo: "ROOT", at: 1})
	if err := p.Apply(reply, 1); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if err := p.Apply(reply, 1); err != nil {
		t.Fatalf("second Apply: %v", err)
	}

	thread, _ := p.Thread("ROOT")
	if thread.ReplyCount != 1 {
		t.Errorf("Duplicate Apply should not double-count, got ReplyCount=%d", thread.ReplyCount)
	}
	if len(thread.ReplyEventIDs) != 1 {
		t.Errorf("Duplicate Apply should not double-append, got ReplyEventIDs=%v", thread.ReplyEventIDs)
	}
}

// =============================================================================
// Subjects() — narrow filter sanity
// =============================================================================

func TestMessagesProjection_SubjectsAreNarrow(t *testing.T) {
	p := NewMessagesProjection()
	subjects := p.Subjects()
	if len(subjects) != 3 {
		t.Fatalf("Subjects() len = %d, want 3", len(subjects))
	}
	for _, s := range subjects {
		// All three should be evt.room.*.message_*. Sanity-check rather
		// than asserting exact strings (those live in the events package).
		if got := s; len(got) == 0 || got[:len("evt.room.*.")] != "evt.room.*." {
			t.Errorf("Subjects() entry %q is not an evt.room.*.message_* filter", s)
		}
	}
}

func TestThreadsProjection_SubjectsMatchMessagesProjection(t *testing.T) {
	// Both projections consume the same filter family. If one ever diverges
	// from the other, the projector setup needs to know — this test acts as
	// a tripwire on accidental drift.
	m := NewMessagesProjection().Subjects()
	th := NewThreadsProjection().Subjects()
	if len(m) != len(th) {
		t.Fatalf("Subjects len differs: messages=%d threads=%d", len(m), len(th))
	}
	for i := range m {
		if m[i] != th[i] {
			t.Errorf("Subjects[%d] differ: messages=%q threads=%q", i, m[i], th[i])
		}
	}
}
