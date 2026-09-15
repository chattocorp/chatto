package core

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	projectionv1 "hmans.de/chatto/internal/pb/chatto/core/projection/v1"
)

func TestRoomTimelineSnapshotContainsReferencesButNoEventPayloads(t *testing.T) {
	projection := NewRoomTimelineProjection()
	body := bodyEvent("B1", "M1", "R1", "U1", "super-secret-ciphertext-marker", 1)
	post := postedEvent(postedOpts{envelopeID: "M1", roomID: "R1", actorID: "U1", at: 2})
	require.NoError(t, projection.Apply(body, 1))
	require.NoError(t, projection.Apply(post, 2))

	payload, err := projection.Snapshot()
	require.NoError(t, err)
	require.NotContains(t, payload, []byte("super-secret-ciphertext-marker"))
	snapshot := &projectionv1.RoomTimelineProjectionSnapshot{}
	require.NoError(t, proto.Unmarshal(payload, snapshot))
	require.Equal(t, uint64(2), snapshot.GetEntries()[0].GetStreamSequence())
	require.Equal(t, uint64(1), snapshot.GetBodies()[0].GetCurrentBodySequence())
	require.Nil(t, snapshot.GetEntries()[0].ProtoReflect().Descriptor().Fields().ByName("event"))
	require.Nil(t, snapshot.GetBodies()[0].ProtoReflect().Descriptor().Fields().ByName("body"))
}

func TestRoomTimelineRestoreRejectsCorruptCompactReferences(t *testing.T) {
	valid := &projectionv1.RoomTimelineProjectionSnapshot{
		Entries: []*projectionv1.TimelineEntrySnapshot{
			{StreamSequence: 2, EventId: "M1", RoomId: "R1", ActorId: "U1", EventType: evtstream.EventMessagePosted, ThreadRootEventId: "M1"},
			{StreamSequence: 4, EventId: "JOIN", RoomId: "R1", ActorId: "U2", EventType: evtstream.EventUserJoinedRoom},
		},
		Bodies: []*projectionv1.TimelineBodySnapshot{
			{MessageEventId: "M1", BodyEventSequences: []uint64{1, 3}, CurrentBodySequence: 3, CurrentBodyEventId: "B2", AuthorId: "U1", Active: true},
		},
	}
	tests := map[string]func(*projectionv1.RoomTimelineProjectionSnapshot){
		"unordered entries": func(snapshot *projectionv1.RoomTimelineProjectionSnapshot) {
			snapshot.Entries[1].StreamSequence = 1
		},
		"unknown event type": func(snapshot *projectionv1.RoomTimelineProjectionSnapshot) {
			snapshot.Entries[0].EventType = evtstream.EventMessageEdited
		},
		"missing message root": func(snapshot *projectionv1.RoomTimelineProjectionSnapshot) {
			snapshot.Entries[0].ThreadRootEventId = ""
		},
		"non-message routing": func(snapshot *projectionv1.RoomTimelineProjectionSnapshot) {
			snapshot.Entries[1].ThreadRootEventId = "M1"
		},
		"unordered body history": func(snapshot *projectionv1.RoomTimelineProjectionSnapshot) {
			snapshot.Bodies[0].BodyEventSequences = []uint64{3, 3}
		},
		"missing body event ID": func(snapshot *projectionv1.RoomTimelineProjectionSnapshot) {
			snapshot.Bodies[0].CurrentBodyEventId = ""
		},
		"inactive body attachments": func(snapshot *projectionv1.RoomTimelineProjectionSnapshot) {
			snapshot.Bodies[0].Active = false
			snapshot.Bodies[0].AttachmentCount = 1
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := proto.Clone(valid).(*projectionv1.RoomTimelineProjectionSnapshot)
			mutate(snapshot)
			payload, err := proto.Marshal(snapshot)
			require.NoError(t, err)
			require.Error(t, NewRoomTimelineProjection().Restore(payload))
		})
	}
}

// Echo content selection must remain identical after cold replay and restore,
// while legacy body records retain their physical ownership for secure deletion.
func TestRoomTimelineEchoReferencesSurviveReplayAndRestore(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy=%v", legacy), func(t *testing.T) {
			p := NewRoomTimelineProjection()
			history := []*evtv1.Event{
				bodyEventWithAssets("B1", "REPLY", "R1", "U1", "original", []string{"A1"}, 1),
				postedEvent(postedOpts{envelopeID: "REPLY", roomID: "R1", actorID: "U1", inThread: "ROOT", at: 2}),
			}
			if legacy {
				history = append(history, bodyEvent("OLD", "ECHO", "R1", "U1", "stale copy", 3))
			}
			history = append(history, postedEvent(postedOpts{envelopeID: "ECHO", roomID: "R1", actorID: "U1", echoOfEventID: "REPLY", echoFromThreadRootEventID: "ROOT", at: 4}))
			for i, event := range history {
				require.NoError(t, p.Apply(event, uint64(i+1)))
			}
			snapshot, err := p.Snapshot()
			require.NoError(t, err)
			restored := NewRoomTimelineProjection()
			require.NoError(t, restored.Restore(snapshot))
			for _, projection := range []*RoomTimelineProjection{p, restored} {
				reference, retracted, known := projection.LatestBodyReference("ECHO")
				require.True(t, known)
				require.False(t, retracted)
				require.Equal(t, "REPLY", reference.MessageEventID)
				require.Equal(t, "B1", reference.BodyEventID)
				require.Len(t, projection.CurrentRoomAttachmentMessages("R1"), 2)
				seqs, _, _ := projection.BodyEventSeqs("ECHO")
				if legacy {
					require.Equal(t, []uint64{3}, seqs)
				} else {
					require.Empty(t, seqs)
				}
				seq := uint64(len(history) + 1)
				// A canonical edit changes echo file membership without an echo body.
				require.NoError(t, projection.Apply(bodyEvent("B2", "REPLY", "R1", "U1", "edited", 5), seq))
				require.Empty(t, projection.CurrentRoomAttachmentMessages("R1"))
				reference, _, _ = projection.LatestBodyReference("ECHO")
				require.Equal(t, "B2", reference.BodyEventID)
				if legacy {
					require.NoError(t, projection.Apply(bodyEvent("LATE", "ECHO", "R1", "U1", "late copy", 6), seq+1))
					reference, _, _ = projection.LatestBodyReference("ECHO")
					require.Equal(t, "B2", reference.BodyEventID)
				}
				require.NoError(t, projection.Apply(retractedEvent("HIDE", "ECHO", "R1", "U1", "", 7), seq+2))
				_, retracted, _ = projection.LatestBodyReference("ECHO")
				require.True(t, retracted)
				original, retracted, _ := projection.LatestBodyReference("REPLY")
				require.False(t, retracted)
				require.Equal(t, "B2", original.BodyEventID)
				for _, obsolete := range projection.ObsoleteBodyEventSeqs("ECHO") {
					require.NotEqual(t, seq, obsolete, "echo deletion must not select the original body")
				}
			}
		})
	}
}

func TestRoomTimelineEchoRejectsInvalidContentLinks(t *testing.T) {
	for _, tc := range []struct{ name, room, thread, target string }{
		{"missing", "R1", "ROOT", "MISSING"},
		{"cross room", "R2", "ROOT", "REPLY"},
		{"wrong thread", "R1", "OTHER", "REPLY"},
		{"cycle", "R1", "ROOT", "ECHO"},
		{"echo chain", "R1", "ROOT", "LINK"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := NewRoomTimelineProjection()
			history := []*evtv1.Event{
				bodyEvent("B1", "REPLY", "R1", "U1", "original", 1),
				postedEvent(postedOpts{envelopeID: "REPLY", roomID: "R1", actorID: "U1", inThread: "ROOT", at: 2}),
				postedEvent(postedOpts{envelopeID: "LINK", roomID: "R1", actorID: "U1", echoOfEventID: "REPLY", echoFromThreadRootEventID: "ROOT", at: 3}),
				bodyEvent("OLD", "ECHO", tc.room, "U1", "must never fall back", 4),
				postedEvent(postedOpts{envelopeID: "ECHO", roomID: tc.room, actorID: "U1", echoOfEventID: tc.target, echoFromThreadRootEventID: tc.thread, at: 5}),
			}
			for i, event := range history {
				require.NoError(t, p.Apply(event, uint64(i+1)))
			}
			reference, retracted, known := p.LatestBodyReference("ECHO")
			require.True(t, known)
			require.False(t, retracted, "invalid content is not a deletion")
			require.Zero(t, reference.StreamSeq)
			_, ok := p.ContentEventID("ECHO")
			require.False(t, ok)
		})
	}
}
