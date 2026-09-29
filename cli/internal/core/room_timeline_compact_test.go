package core

import (
	"strings"
	"testing"
	"unsafe"

	"google.golang.org/protobuf/proto"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	projectionv1 "hmans.de/chatto/internal/pb/chatto/core/projection/v1"
)

func TestRoomTimeline_ReplyFlagKeepsInThreadOnlyForReplies(t *testing.T) {
	p := NewRoomTimelineProjection()
	applyAll(t, p, []*evtv1.Event{
		postedEvent(postedOpts{envelopeID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "REPLY", roomID: "R1", actorID: "U2", inThread: "ROOT", at: 2}),
		postedEvent(postedOpts{envelopeID: "ECHO", roomID: "R1", actorID: "U2", echoOfEventID: "REPLY", echoFromThreadRootEventID: "ROOT", at: 3}),
	})
	for _, tc := range []struct{ id, root, inThread string }{
		{"ROOT", "ROOT", ""},
		{"REPLY", "ROOT", "ROOT"},
		{"ECHO", "ROOT", ""},
	} {
		entry, ok := p.Get(tc.id)
		if !ok || entry.ThreadRootEventID != tc.root || entry.InThreadEventID != tc.inThread {
			t.Fatalf("Get(%s) = %+v, %v; want root %q and in-thread %q", tc.id, entry, ok, tc.root, tc.inThread)
		}
	}
}

func TestRoomTimeline_BodyFlagsKeepActiveStateAndAttachments(t *testing.T) {
	p := NewRoomTimelineProjection()
	applyAll(t, p, []*evtv1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-M1", roomID: "R1", actorID: "U1", at: 1}),
		bodyEventWithAssets("ENV-BODY-M1", "ENV-M1", "R1", "U1", "files", []string{"A1", "A2", "A3"}, 2),
	})
	if state := bodyStateForTest(p, "ENV-M1"); !state.active() || state.attachmentCount() != 3 {
		t.Fatalf("body state active=%v attachments=%d; want true, 3", state.active(), state.attachmentCount())
	}
	if err := p.Apply(retractedEvent("ENV-RETRACT", "ENV-M1", "R1", "U1", "", 3), 3); err != nil {
		t.Fatal(err)
	}
	if state := bodyStateForTest(p, "ENV-M1"); state.active() || state.attachmentCount() != 0 {
		t.Fatalf("retracted body active=%v attachments=%d; want false, 0", state.active(), state.attachmentCount())
	}
}

func TestRoomTimelineSnapshotRejectsUnrepresentableAttachmentCount(t *testing.T) {
	source := NewRoomTimelineProjection()
	applyAll(t, source, []*evtv1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-M1", roomID: "R1", actorID: "U1", at: 1}),
		bodyEventWithAssets("ENV-BODY-M1", "ENV-M1", "R1", "U1", "file", []string{"A1"}, 2),
	})
	data, err := source.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var snapshot projectionv1.RoomTimelineProjectionSnapshot
	if err := proto.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Bodies[0].AttachmentCount = timelineBodyActive
	mutated, err := proto.Marshal(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewRoomTimelineProjection().Restore(mutated); err == nil || !strings.Contains(err.Error(), "too many attachments") {
		t.Fatalf("Restore error = %v, want too many attachments", err)
	}
}

// TestRoomTimeline_PerMessageStructSizes guards the memory cost of the two
// structs that the timeline keeps for every message.
func TestRoomTimeline_PerMessageStructSizes(t *testing.T) {
	if got := unsafe.Sizeof(timelineRow{}); got > 48 {
		t.Fatalf("timelineRow is %d bytes, want at most 48", got)
	}
	if got := unsafe.Sizeof(timelineBodyState{}); got > 24 {
		t.Fatalf("timelineBodyState is %d bytes, want at most 24", got)
	}
}
