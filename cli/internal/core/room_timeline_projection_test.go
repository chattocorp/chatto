package core

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// =============================================================================
// Test helpers (shared with thread_projection_test.go)
// =============================================================================

func fixedTime(seed int) time.Time {
	return time.Date(2026, 1, 1, 12, 0, seed, 0, time.UTC)
}

type postedOpts struct {
	envelopeID                string
	eventID                   string
	roomID                    string
	actorID                   string
	body                      string
	inReplyTo                 string
	inThread                  string
	echoOfEventID             string
	echoFromThreadRootEventID string
	at                        int
}

func postedEvent(o postedOpts) *corev1.Event {
	envID := o.envelopeID
	if envID == "" {
		envID = o.eventID
	}
	return &corev1.Event{
		Id:        envID,
		ActorId:   o.actorID,
		CreatedAt: timestamppb.New(fixedTime(o.at)),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				RoomId:                    o.roomID,
				EventId:                   o.eventID,
				InReplyTo:                 o.inReplyTo,
				InThread:                  o.inThread,
				EchoOfEventId:             o.echoOfEventID,
				EchoFromThreadRootEventId: o.echoFromThreadRootEventID,
				Body: &corev1.MessageBody{
					AuthorId:      o.actorID,
					EncryptedBody: []byte(o.body),
				},
			},
		},
	}
}

func editedEvent(envID, targetID, roomID, actorID, newBody string, at int) *corev1.Event {
	return &corev1.Event{
		Id:        envID,
		ActorId:   actorID,
		CreatedAt: timestamppb.New(fixedTime(at)),
		Event: &corev1.Event_MessageEdited{
			MessageEdited: &corev1.MessageEditedEvent{
				RoomId:  roomID,
				EventId: targetID,
				Body: &corev1.MessageBody{
					AuthorId:      actorID,
					EncryptedBody: []byte(newBody),
				},
			},
		},
	}
}

func retractedEvent(envID, targetID, roomID, actorID, reason string, at int) *corev1.Event {
	return &corev1.Event{
		Id:        envID,
		ActorId:   actorID,
		CreatedAt: timestamppb.New(fixedTime(at)),
		Event: &corev1.Event_MessageRetracted{
			MessageRetracted: &corev1.MessageRetractedEvent{
				RoomId:  roomID,
				EventId: targetID,
				Reason:  reason,
			},
		},
	}
}

func joinedEvent(envID, roomID, userID string, at int) *corev1.Event {
	return &corev1.Event{
		Id:        envID,
		ActorId:   userID,
		CreatedAt: timestamppb.New(fixedTime(at)),
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: roomID},
		},
	}
}

func leftEvent(envID, roomID, userID string, at int) *corev1.Event {
	return &corev1.Event{
		Id:        envID,
		ActorId:   userID,
		CreatedAt: timestamppb.New(fixedTime(at)),
		Event: &corev1.Event_UserLeftRoom{
			UserLeftRoom: &corev1.UserLeftRoomEvent{RoomId: roomID},
		},
	}
}

func roomCreatedTimelineEvent(envID, roomID, name string, at int) *corev1.Event {
	return &corev1.Event{
		Id:        envID,
		ActorId:   "SYSTEM",
		CreatedAt: timestamppb.New(fixedTime(at)),
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{
				RoomId: roomID,
				Name:   name,
				Kind:   corev1.RoomKind_ROOM_KIND_CHANNEL,
			},
		},
	}
}

// applyAll feeds events into a projection in order with seq starting at 1.
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
// RoomTimelineProjection
// =============================================================================

func TestRoomTimeline_Empty(t *testing.T) {
	p := NewRoomTimelineProjection()
	if got := p.RoomEvents("R1", 50, 0); len(got) != 0 {
		t.Errorf("RoomEvents on empty = %d entries, want 0", len(got))
	}
	if got := p.RoomEventCount("R1"); got != 0 {
		t.Errorf("RoomEventCount on empty = %d, want 0", got)
	}
	if _, ok := p.Get("nope"); ok {
		t.Error("Get on empty should return ok=false")
	}
}

func TestRoomTimeline_AppendsAllEventKinds(t *testing.T) {
	p := NewRoomTimelineProjection()
	applyAll(t, p, []*corev1.Event{
		roomCreatedTimelineEvent("ENV-CREATE", "R1", "general", 1),
		joinedEvent("ENV-JOIN-U1", "R1", "U1", 2),
		postedEvent(postedOpts{envelopeID: "ENV-M1", eventID: "M1", roomID: "R1", actorID: "U1", body: "hello", at: 3}),
		editedEvent("ENV-EDIT-M1", "M1", "R1", "U1", "hello (edited)", 4),
		joinedEvent("ENV-JOIN-U2", "R1", "U2", 5),
		postedEvent(postedOpts{envelopeID: "ENV-M2", eventID: "M2", roomID: "R1", actorID: "U2", body: "hi", at: 6}),
		retractedEvent("ENV-RETRACT-M2", "M2", "R1", "MOD", "spam", 7),
		leftEvent("ENV-LEFT-U2", "R1", "U2", 8),
	})

	if got := p.RoomEventCount("R1"); got != 8 {
		t.Errorf("RoomEventCount = %d, want 8 (full event log including edits/retracts)", got)
	}

	// Newest-first ordering.
	got := p.RoomEvents("R1", 50, 0)
	if len(got) != 8 {
		t.Fatalf("RoomEvents len = %d, want 8", len(got))
	}
	wantOrder := []string{"ENV-LEFT-U2", "ENV-RETRACT-M2", "ENV-M2", "ENV-JOIN-U2", "ENV-EDIT-M1", "ENV-M1", "ENV-JOIN-U1", "ENV-CREATE"}
	for i, e := range got {
		if e.Event.GetId() != wantOrder[i] {
			t.Errorf("entry[%d] envelope id = %q, want %q", i, e.Event.GetId(), wantOrder[i])
		}
	}
}

func TestRoomTimeline_RoomIsolation(t *testing.T) {
	p := NewRoomTimelineProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-A", eventID: "A", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "ENV-B", eventID: "B", roomID: "R2", actorID: "U1", at: 2}),
		postedEvent(postedOpts{envelopeID: "ENV-C", eventID: "C", roomID: "R1", actorID: "U1", at: 3}),
	})

	if got := p.RoomEventCount("R1"); got != 2 {
		t.Errorf("R1 count = %d, want 2", got)
	}
	if got := p.RoomEventCount("R2"); got != 1 {
		t.Errorf("R2 count = %d, want 1", got)
	}
	r1 := p.RoomEvents("R1", 10, 0)
	if len(r1) != 2 || r1[0].Event.GetId() != "ENV-C" || r1[1].Event.GetId() != "ENV-A" {
		t.Errorf("R1 timeline = %+v, want [ENV-C, ENV-A]", eventIDs(r1))
	}
}

func TestRoomTimeline_PaginationByStreamSeq(t *testing.T) {
	p := NewRoomTimelineProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-1", eventID: "M1", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "ENV-2", eventID: "M2", roomID: "R1", actorID: "U1", at: 2}),
		postedEvent(postedOpts{envelopeID: "ENV-3", eventID: "M3", roomID: "R1", actorID: "U1", at: 3}),
	})

	// limit
	if got := p.RoomEvents("R1", 1, 0); len(got) != 1 || got[0].Event.GetId() != "ENV-3" {
		t.Errorf("limit=1 = %v, want [ENV-3]", eventIDs(got))
	}
	// beforeStreamSeq excludes seq>=given
	if got := p.RoomEvents("R1", 10, 3); len(got) != 2 || got[0].Event.GetId() != "ENV-2" || got[1].Event.GetId() != "ENV-1" {
		t.Errorf("before=3 = %v, want [ENV-2, ENV-1]", eventIDs(got))
	}
	// beforeStreamSeq=1 means strictly older than seq 1 → empty
	if got := p.RoomEvents("R1", 10, 1); len(got) != 0 {
		t.Errorf("before=1 = %v, want []", eventIDs(got))
	}
}

func TestRoomTimeline_LookupByEnvelopeID(t *testing.T) {
	p := NewRoomTimelineProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-M1", eventID: "M1", roomID: "R1", actorID: "U1", body: "hello", at: 1}),
		editedEvent("ENV-EDIT-M1", "M1", "R1", "U1", "hello (edited)", 2),
	})

	// Original post lookup.
	entry, ok := p.Get("ENV-M1")
	if !ok || entry.Event.GetMessagePosted().GetEventId() != "M1" {
		t.Errorf("Get(ENV-M1) = %v, want the post", entry)
	}
	// Edit also indexed.
	entry, ok = p.Get("ENV-EDIT-M1")
	if !ok || entry.Event.GetMessageEdited().GetEventId() != "M1" {
		t.Errorf("Get(ENV-EDIT-M1) = %v, want the edit", entry)
	}
	// Unknown.
	if _, ok := p.Get("nope"); ok {
		t.Error("Get(nope) should be ok=false")
	}
}

func TestRoomTimeline_Idempotency(t *testing.T) {
	p := NewRoomTimelineProjection()
	e := postedEvent(postedOpts{envelopeID: "ENV-M1", eventID: "M1", roomID: "R1", actorID: "U1", at: 1})
	if err := p.Apply(e, 1); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if err := p.Apply(e, 1); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if got := p.RoomEventCount("R1"); got != 1 {
		t.Errorf("RoomEventCount after duplicate Apply = %d, want 1", got)
	}
}

func TestRoomTimeline_NonRoomEventsSkipped(t *testing.T) {
	// SpaceMemberDeletedEvent is in the proto's "Room membership" block
	// (oneof tag 320) but carries no room_id. It's published to
	// server.member.> in practice, never to evt.room.>, but if one ever
	// slipped through the filter, roomIDOfEvent would return "" and the
	// entry would be skipped rather than crashing.
	p := NewRoomTimelineProjection()
	stray := &corev1.Event{
		Id:        "ENV-STRAY",
		CreatedAt: timestamppb.New(fixedTime(1)),
		Event: &corev1.Event_SpaceMemberDeleted{
			SpaceMemberDeleted: &corev1.SpaceMemberDeletedEvent{UserId: "U1"},
		},
	}
	if err := p.Apply(stray, 1); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := p.RoomEventCount("R1"); got != 0 {
		t.Errorf("non-room event should not land in any room timeline, got count=%d", got)
	}
}

func TestRoomTimeline_SubjectFilter(t *testing.T) {
	subjects := NewRoomTimelineProjection().Subjects()
	if len(subjects) != 1 {
		t.Fatalf("expected 1 subject filter, got %d", len(subjects))
	}
	if subjects[0] != "evt.room.>" {
		t.Errorf("subject filter = %q, want evt.room.>", subjects[0])
	}
}

// =============================================================================
// Helpers for assertion noise
// =============================================================================

func eventIDs(entries []*TimelineEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Event.GetId()
	}
	return out
}
