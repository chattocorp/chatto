package core

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/stretchr/testify/require"
)

func TestRoomNoticeLifecycleAndAuthorization(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	member, err := c.CreateUser(ctx, SystemActorID, "notice-editor", "Editor", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "notices", "")
	require.NoError(t, err)
	_, err = c.JoinRoom(ctx, member.Id, KindChannel, member.Id, room.Id)
	require.NoError(t, err)
	input := RoomNoticeInput{ActorID: member.Id, RoomID: room.Id, Body: "First"}
	_, err = c.RoomCommands().PublishNotice(ctx, input)
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.NoError(t, c.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomManage))
	_, err = c.RoomCommands().PublishNotice(ctx, input)
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.NoError(t, c.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomManageNotice))
	first, err := c.RoomCommands().PublishNotice(ctx, input)
	require.NoError(t, err)
	require.NotEmpty(t, first.Version)
	require.NotEmpty(t, first.PublicationId)
	require.NotNil(t, first.PublishedAt)
	facts, seq, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomNoticeChanged))
	require.NoError(t, err)
	require.Len(t, facts, 1)
	_, delivered := c.myEventsModel.filterReadyEVTRoomSubjectEvent(ctx, member.Id, map[string]struct{}{room.Id: {}}, room.Id, facts[0], seq)
	require.True(t, delivered)
	_, delivered = c.myEventsModel.filterReadyEVTRoomSubjectEvent(ctx, member.Id, map[string]struct{}{}, room.Id, facts[0], seq)
	require.False(t, delivered)
	_, err = c.RoomCommands().PublishNotice(ctx, input)
	require.ErrorIs(t, err, events.ErrConflict)
	input.ExpectedVersion, input.Body = first.Version, "Correction"
	corrected, err := c.RoomCommands().PublishNotice(ctx, input)
	require.NoError(t, err)
	require.Equal(t, first.PublicationId, corrected.PublicationId)
	require.Equal(t, first.PublishedAt.AsTime(), corrected.PublishedAt.AsTime())
	require.NotEqual(t, first.Version, corrected.Version)
	input.ExpectedVersion, input.Republish = corrected.Version, true
	republished, err := c.RoomCommands().PublishNotice(ctx, input)
	require.NoError(t, err)
	require.NotEqual(t, first.PublicationId, republished.PublicationId)
	require.True(t, republished.PublishedAt.AsTime().After(first.PublishedAt.AsTime()))
	input.ExpectedVersion = republished.Version
	cleared, err := c.RoomCommands().ClearNotice(ctx, input)
	require.NoError(t, err)
	require.Empty(t, cleared.Body)
	require.Empty(t, cleared.PublicationId)
	require.Nil(t, cleared.PublishedAt)
	require.NotEmpty(t, cleared.Version)
	_, err = c.RoomCommands().PublishNotice(ctx, input)
	require.ErrorIs(t, err, events.ErrConflict)
	input.ExpectedVersion, input.Republish = cleared.Version, false
	next, err := c.RoomCommands().PublishNotice(ctx, input)
	require.NoError(t, err)
	require.NotEqual(t, republished.PublicationId, next.PublicationId)
	read, err := c.RoomCommands().GetNotice(ctx, RoomIDInput{ActorID: member.Id, RoomID: room.Id})
	require.NoError(t, err)
	require.Equal(t, next.Version, read.Version)
	require.Equal(t, next.PublishedAt.AsTime(), read.PublishedAt.AsTime())
	facts, _, err = c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomNoticeChanged))
	require.NoError(t, err)
	require.Len(t, facts, 5)
	for i, announced := range []bool{true, false, true, false, true} {
		require.Equal(t, announced, facts[i].GetRoomNoticeChanged().GetAnnounced())
	}
	require.Equal(t, facts[0].GetId(), first.PublicationId)
	require.Equal(t, facts[2].GetId(), republished.PublicationId)
	require.Equal(t, facts[4].GetId(), next.PublicationId)
	for _, body := range []string{"", " \n\t", strings.Repeat("界", MaxRoomNoticeLength+1)} {
		input.Body = body
		_, err = c.RoomCommands().PublishNotice(ctx, input)
		require.ErrorIs(t, err, ErrInvalidArgument)
	}
	outsider, err := c.CreateUser(ctx, SystemActorID, "notice-outsider", "Outsider", "password")
	require.NoError(t, err)
	_, err = c.RoomCommands().GetNotice(ctx, RoomIDInput{ActorID: outsider.Id, RoomID: room.Id})
	require.ErrorIs(t, err, ErrNotRoomMember)
	input.ActorID, input.Body = outsider.Id, "Unauthorized"
	_, err = c.RoomCommands().PublishNotice(ctx, input)
	require.ErrorIs(t, err, ErrNotRoomMember)
	dm, _, err := c.RoomCommands().StartDM(ctx, RoomStartDMInput{ActorID: member.Id, ParticipantIDs: []string{outsider.Id}})
	require.NoError(t, err)
	_, err = c.RoomCommands().GetNotice(ctx, RoomIDInput{ActorID: member.Id, RoomID: dm.Id})
	require.ErrorIs(t, err, ErrInvalidArgument)
	_, err = c.RoomCommands().ArchiveRoom(ctx, RoomIDInput{ActorID: member.Id, RoomID: room.Id})
	require.NoError(t, err)
	_, err = c.RoomCommands().GetNotice(ctx, RoomIDInput{ActorID: member.Id, RoomID: room.Id})
	require.NoError(t, err)
	input.ActorID, input.ExpectedVersion = member.Id, next.Version
	_, err = c.RoomCommands().ClearNotice(ctx, input)
	require.ErrorIs(t, err, ErrRoomArchived)
}

func TestRoomNoticeProjectionSnapshotAndDelivery(t *testing.T) {
	t.Parallel()
	p := NewRoomDirectoryProjection()
	created := roomCreatedEvent("R1", "Room", "", evtv1.RoomKind_ROOM_KIND_CHANNEL)
	created.GetRoomCreated().Purpose = "Room purpose"
	require.NoError(t, p.Apply(created, 1))
	event := &evtv1.Event{Id: "version", Event: &evtv1.Event_RoomNoticeChanged{RoomNoticeChanged: &evtv1.RoomNoticeChangedEvent{RoomId: "R1", Body: "Notice", Announced: true}}}
	event.CreatedAt = timestamppb.New(time.Date(2026, 10, 26, 14, 0, 0, 0, time.UTC))
	require.NoError(t, p.Apply(event, 2))
	require.Equal(t, evtstream.EventRoomNoticeChanged, evtstream.EventTypeOf(event))
	require.Equal(t, "R1", roomIDOfEvent(event))
	require.True(t, isDeliverableLiveEVTRoomEvent(event))
	require.False(t, isVisibleRoomTimelineEntry(event))
	snapshot, err := p.Snapshot()
	require.NoError(t, err)
	restored := NewRoomDirectoryProjection()
	require.NoError(t, restored.Restore(snapshot))
	room, ok := restored.Catalog.Get("R1")
	require.True(t, ok)
	require.Equal(t, "Notice", room.Notice.Body)
	require.Equal(t, "Room purpose", room.Purpose)
	require.Equal(t, event.Id, room.Notice.PublicationId)
	require.Equal(t, event.CreatedAt.AsTime(), room.Notice.PublishedAt.AsTime())
	room.Notice.Body = "changed clone"
	room, _ = restored.Catalog.Get("R1")
	require.Equal(t, "Notice", room.Notice.Body)
	require.Contains(t, DefaultAdminPermissions(), PermRoomManageNotice)
	for _, permissions := range [][]Permission{DefaultEveryonePermissions(), DefaultModeratorPermissions()} {
		require.NotContains(t, permissions, PermRoomManageNotice)
	}
	require.False(t, PermissionAppliesAtScope(PermRoomManageNotice, ScopeDM))
}
