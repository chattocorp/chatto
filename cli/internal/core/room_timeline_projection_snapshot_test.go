package core

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
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
