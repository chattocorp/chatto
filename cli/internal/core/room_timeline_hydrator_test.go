package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestRoomTimelineHydratorLoadsAndValidatesEntry(t *testing.T) {
	event := postedEvent(postedOpts{envelopeID: "M1", roomID: "R1", actorID: "U1", at: 1})
	projection := NewRoomTimelineProjection()
	require.NoError(t, projection.Apply(event, 1))
	entry, ok := projection.Get(event.GetId())
	require.True(t, ok)

	hydrated, err := newRoomTimelineHydrator(testTimelineEventReader([]*evtv1.Event{event})).events(context.Background(), []*TimelineEntry{entry})
	require.NoError(t, err)
	require.Len(t, hydrated, 1)
	require.True(t, proto.Equal(event, hydrated[0]))
}

func TestValidateTimelineEntryRecordRejectsMismatchedMetadata(t *testing.T) {
	event := postedEvent(postedOpts{envelopeID: "M1", roomID: "R1", actorID: "U1", inThread: "ROOT", at: 1})
	projection := NewRoomTimelineProjection()
	require.NoError(t, projection.Apply(event, 7))
	reference, ok := projection.Get(event.GetId())
	require.True(t, ok)
	valid := &evtstream.SubjectEvent{
		Subject:  evtstream.RoomAggregate("R1").Subject(evtstream.EventMessagePosted),
		Sequence: 7,
		Event:    event,
	}

	tests := map[string]func(*TimelineEntry, *evtstream.SubjectEvent){
		"sequence": func(_ *TimelineEntry, record *evtstream.SubjectEvent) { record.Sequence++ },
		"subject":  func(_ *TimelineEntry, record *evtstream.SubjectEvent) { record.Subject = "evt.room.other.message_posted" },
		"event ID": func(_ *TimelineEntry, record *evtstream.SubjectEvent) { record.Event.Id = "other" },
		"actor":    func(_ *TimelineEntry, record *evtstream.SubjectEvent) { record.Event.ActorId = "other" },
		"room": func(_ *TimelineEntry, record *evtstream.SubjectEvent) {
			record.Event.GetMessagePosted().RoomId = "other"
		},
		"thread": func(ref *TimelineEntry, _ *evtstream.SubjectEvent) { ref.InThreadEventID = "other" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			refCopy := *reference
			recordCopy := &evtstream.SubjectEvent{
				Subject:  valid.Subject,
				Sequence: valid.Sequence,
				Event:    proto.Clone(valid.Event).(*evtv1.Event),
			}
			mutate(&refCopy, recordCopy)
			require.Error(t, validateTimelineEntryRecord(&refCopy, recordCopy))
		})
	}
}

func TestValidateTimelineBodyRecordRejectsMismatchedMetadata(t *testing.T) {
	event := bodyEventWithAssets("B1", "M1", "R1", "U1", "ciphertext", []string{"A1"}, 1)
	reference := TimelineBodyReference{
		MessageEventID: "M1", BodyEventID: "B1", RoomID: "R1", AuthorID: "U1", StreamSeq: 5, AttachmentCount: 1,
	}
	valid := &evtstream.SubjectEvent{
		Subject:  evtstream.RoomAggregate("R1").Subject(evtstream.EventMessageBody),
		Sequence: 5,
		Event:    event,
	}

	tests := map[string]func(*TimelineBodyReference, *evtstream.SubjectEvent){
		"sequence": func(_ *TimelineBodyReference, record *evtstream.SubjectEvent) { record.Sequence++ },
		"subject":  func(_ *TimelineBodyReference, record *evtstream.SubjectEvent) { record.Subject = "evt.room.other.message_body" },
		"target": func(_ *TimelineBodyReference, record *evtstream.SubjectEvent) {
			record.Event.GetMessageBody().EventId = "other"
		},
		"room": func(_ *TimelineBodyReference, record *evtstream.SubjectEvent) {
			record.Event.GetMessageBody().RoomId = "other"
		},
		"body event ID": func(_ *TimelineBodyReference, record *evtstream.SubjectEvent) {
			record.Event.GetMessageBody().Body.BodyEventId = "other"
		},
		"author": func(_ *TimelineBodyReference, record *evtstream.SubjectEvent) {
			record.Event.GetMessageBody().Body.AuthorId = "other"
		},
		"attachments": func(ref *TimelineBodyReference, _ *evtstream.SubjectEvent) { ref.AttachmentCount++ },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			refCopy := reference
			recordCopy := &evtstream.SubjectEvent{
				Subject:  valid.Subject,
				Sequence: valid.Sequence,
				Event:    proto.Clone(valid.Event).(*evtv1.Event),
			}
			mutate(&refCopy, recordCopy)
			_, err := validateTimelineBodyRecord(refCopy, recordCopy)
			require.Error(t, err)
		})
	}
}

func TestCurrentMessageBodyRetriesAfterConcurrentEdit(t *testing.T) {
	oldBody := bodyEvent("B1", "M1", "R1", "U1", "old", 1)
	post := postedEvent(postedOpts{envelopeID: "M1", roomID: "R1", actorID: "U1", at: 2})
	newBody := bodyEvent("B2", "M1", "R1", "U1", "new", 3)
	projection := NewRoomTimelineProjection()
	require.NoError(t, projection.Apply(oldBody, 1))
	require.NoError(t, projection.Apply(post, 2))
	reader := testTimelineEventReader([]*evtv1.Event{oldBody, post, newBody})
	reader.hook = func() {
		require.NoError(t, projection.Apply(newBody, 3))
		delete(reader.records, 1)
	}
	core := &ChattoCore{
		roomModel:        newTestRoomModel(t, nil, nil, nil, nil, projection, nil, nil, nil, nil, nil),
		timelineHydrator: newRoomTimelineHydrator(reader),
	}

	body, err := core.currentMessageBody(context.Background(), "M1")
	require.NoError(t, err)
	require.Equal(t, "B2", body.GetBodyEventId())
	require.Equal(t, []byte("new"), body.GetEncryptedBody())
	require.Equal(t, [][]uint64{{1}, {3}}, reader.reads)
}

func TestCurrentMessageBodyDoesNotReturnPayloadAfterConcurrentRetraction(t *testing.T) {
	bodyEvent := bodyEvent("B1", "M1", "R1", "U1", "body", 1)
	post := postedEvent(postedOpts{envelopeID: "M1", roomID: "R1", actorID: "U1", at: 2})
	retract := retractedEvent("R1", "M1", "R1", "U1", "deleted", 3)
	projection := NewRoomTimelineProjection()
	require.NoError(t, projection.Apply(bodyEvent, 1))
	require.NoError(t, projection.Apply(post, 2))
	reader := testTimelineEventReader([]*evtv1.Event{bodyEvent, post, retract})
	reader.hook = func() {
		require.NoError(t, projection.Apply(retract, 3))
		delete(reader.records, 1)
	}
	core := &ChattoCore{
		roomModel:        newTestRoomModel(t, nil, nil, nil, nil, projection, nil, nil, nil, nil, nil),
		timelineHydrator: newRoomTimelineHydrator(reader),
	}

	body, err := core.currentMessageBody(context.Background(), "M1")
	require.NoError(t, err)
	require.Nil(t, body)
}

func TestBatchGetMessagesHydratesBodiesAndEventsInBatches(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	room, user := setupRoomAttachmentTest(t, core, ctx)
	first, err := core.PostMessage(ctx, KindChannel, room.GetId(), user.GetId(), "first", nil, "", "", nil, false)
	require.NoError(t, err)
	second, err := core.PostMessage(ctx, KindChannel, room.GetId(), user.GetId(), "second", nil, "", "", nil, false)
	require.NoError(t, err)
	reader := &recordingTimelineEventReader{delegate: core.timelineHydrator.reader}
	core.timelineHydrator = newRoomTimelineHydrator(reader)

	result, err := core.RoomTimelineReads().BatchGetMessages(ctx, user.GetId(), room.GetId(), []string{first.GetId(), second.GetId()})
	require.NoError(t, err)
	require.Len(t, result.Events, 2)
	require.Equal(t, first.GetId(), result.Events[0].GetId())
	require.Equal(t, second.GetId(), result.Events[1].GetId())
	require.Len(t, reader.reads, 2)
	require.Len(t, reader.reads[0], 2)
	require.Len(t, reader.reads[1], 2)
}

func TestGetMessageDoesNotHydrateUnauthorizedPayload(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	room, author := setupRoomAttachmentTest(t, core, ctx)
	viewer, err := core.CreateUser(ctx, SystemActorID, "timeline-payload-viewer", "Timeline Payload Viewer", "password123")
	require.NoError(t, err)
	_, err = core.JoinRoom(ctx, viewer.GetId(), KindChannel, viewer.GetId(), room.GetId())
	require.NoError(t, err)
	message, err := core.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "private", nil, "", "", nil, false)
	require.NoError(t, err)
	require.NoError(t, core.DenyUserRoomPermission(ctx, SystemActorID, room.GetId(), viewer.GetId(), PermMessageRead))
	require.NoError(t, core.DenyUserRoomPermission(ctx, SystemActorID, room.GetId(), viewer.GetId(), PermMessageReadInteractions))
	reader := &recordingTimelineEventReader{delegate: core.timelineHydrator.reader}
	core.timelineHydrator = newRoomTimelineHydrator(reader)

	_, err = core.RoomTimelineReads().GetMessage(ctx, viewer.GetId(), room.GetId(), message.GetId())
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.Empty(t, reader.reads)
}
