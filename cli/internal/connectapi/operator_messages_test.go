package connectapi

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	operatorv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestOperatorImportReactionChangesSourceTime(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("operator-reaction-batch")
	service := &operatorMessageService{api: env.api}
	created := time.Date(2020, 3, 14, 12, 0, 0, 0, time.UTC)
	post, err := service.ImportMessage(env.ctx, connect.NewRequest(&operatorv1.ImportMessageRequest{
		RoomId: room.Id, AuthorId: env.viewer.Id, CreatedAt: timestamppb.New(created), Body: "historical body",
	}))
	require.NoError(t, err)
	operation := &operatorv1.HistoricalReactionOperation{
		MessageId: post.Msg.MessageId, OperationKey: uuid.NewString(), ActorId: env.viewer.Id,
		OccurredAt: timestamppb.New(created.Add(time.Hour)),
		Change:     &operatorv1.HistoricalReactionChange{UserId: env.viewer.Id, Emoji: "heart", Present: true},
	}
	_, err = service.ImportReactionChanges(env.ctx, connect.NewRequest(&operatorv1.ImportReactionChangesRequest{RoomId: room.Id, Operations: []*operatorv1.HistoricalReactionOperation{operation}}))
	require.NoError(t, err)
	facts, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, "evt.room.*.reaction_added")
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, operation.OccurredAt.AsTime(), facts[0].CreatedAt.AsTime())
	require.Equal(t, env.viewer.Id, facts[0].GetImportContext().GetSourceActorId())
	require.False(t, facts[0].GetImportContext().GetUnknownTime())
	before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	for _, bad := range []*operatorv1.HistoricalReactionOperation{
		nil,
		{MessageId: post.Msg.MessageId, OperationKey: uuid.NewString()},
		{MessageId: post.Msg.MessageId, OperationKey: uuid.NewString(), Change: operation.Change, OccurredAt: &timestamppb.Timestamp{Seconds: 1 << 62}},
	} {
		_, err := service.ImportReactionChanges(env.ctx, connect.NewRequest(&operatorv1.ImportReactionChangesRequest{RoomId: room.Id, Operations: []*operatorv1.HistoricalReactionOperation{bad}}))
		require.Error(t, err)
	}
	after, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestOperatorImportMembershipEventReactions(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("operator-membership-reactions")
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "joining-user", "Joining User", "password")
	require.NoError(t, err)
	_, err = env.core.JoinRoom(env.ctx, actor.Id, core.KindChannel, actor.Id, room.Id)
	require.NoError(t, err)
	require.NoError(t, env.core.LeaveRoom(env.ctx, actor.Id, core.KindChannel, actor.Id, room.Id))
	timeline, err := env.rooms.GetRoomEvents(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{RoomId: room.Id, Limit: 100}))
	require.NoError(t, err)
	service := &operatorMessageService{api: env.api}
	count := 0
	for _, event := range timeline.Msg.Page.Events {
		if event.ActorId != actor.Id || (event.GetUserJoinedRoom() == nil && event.GetUserLeftRoom() == nil) {
			continue
		}
		count++
		// Historical imports accept reactors who are no longer room members.
		for _, wantAdded := range []bool{true, false} {
			response, err := service.ImportReaction(env.ctx, connect.NewRequest(&operatorv1.ImportReactionRequest{
				RoomId: room.Id, MessageEventId: event.Id, UserId: actor.Id, Emoji: "wave",
			}))
			require.NoError(t, err)
			require.Equal(t, wantAdded, response.Msg.Added)
		}
		reactions, err := env.core.GetReactions(env.ctx, event.Id)
		require.NoError(t, err)
		require.Equal(t, []core.ReactionSummary{{Emoji: "wave", UserIDs: []string{actor.Id}}}, reactions)
	}
	require.Equal(t, 2, count)
}

func TestOperatorImportMessageAppearsWithMappedAuthorInPublicRead(t *testing.T) {
	t.Parallel()

	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("operator-message-public-read")
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-message-author", "Imported Author", "password")
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	edited := created.Add(time.Minute)
	service := &operatorMessageService{api: env.api}
	response, err := service.ImportMessage(env.ctx, connect.NewRequest(&operatorv1.ImportMessageRequest{
		RoomId: room.Id, AuthorId: author.Id, CreatedAt: timestamppb.New(created), EditedAt: timestamppb.New(edited), Body: "Imported body",
		PreviewUrl: "https://example.invalid/source", PreviewTitle: "Exported title", PreviewDescription: "Exported description", PreviewType: "link",
	}))
	if err != nil {
		t.Fatal(err)
	}
	public, err := env.messages.GetMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetMessageRequest{RoomId: room.Id, EventId: response.Msg.GetMessageId()}))
	if err != nil {
		t.Fatal(err)
	}
	message := public.Msg.GetMessage()
	if message == nil || message.GetActorId() != author.Id || message.GetBody() != "Imported body" || !message.GetCreatedAt().AsTime().Equal(created) || !message.GetUpdatedAt().AsTime().Equal(edited) || message.GetLinkPreview().GetTitle() != "Exported title" {
		t.Fatalf("public imported message = %+v", message)
	}
	timeline, err := env.rooms.GetRoomEvents(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{RoomId: room.Id, Limit: 20}))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range timeline.Msg.GetPage().GetEvents() {
		if row.GetId() == response.Msg.GetMessageId() {
			found = row.GetMessagePosted().GetMessage().GetActorId() == author.Id
		}
	}
	if !found {
		t.Fatal("imported message missing or misattributed in public timeline")
	}
	if _, err := service.ImportMessage(env.ctx, connect.NewRequest(&operatorv1.ImportMessageRequest{
		RoomId: room.Id, AuthorId: author.Id, CreatedAt: timestamppb.New(created), Body: "body", PreviewTitle: "title",
	})); errorCode(err) != connect.CodeInvalidArgument {
		t.Fatalf("preview without URL error = %v", err)
	}
}

func TestOperatorImportTombstonePublicRead(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("operator-tombstone")
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "tombstone-author", "Author", "password")
	require.NoError(t, err)
	service := &operatorMessageService{api: env.api}
	root, err := service.ImportMessage(env.ctx, connect.NewRequest(&operatorv1.ImportMessageRequest{
		RoomId: room.Id, AuthorId: author.Id, CreatedAt: timestamppb.New(time.Unix(1500000000, 0)), Deleted: true, CreateThread: true,
	}))
	require.NoError(t, err)
	for _, wantAdded := range []bool{true, false} {
		result, err := service.ImportReaction(env.ctx, connect.NewRequest(&operatorv1.ImportReactionRequest{
			RoomId: room.Id, MessageEventId: root.Msg.GetMessageId(), UserId: author.Id, Emoji: "heart",
		}))
		require.NoError(t, err)
		require.Equal(t, wantAdded, result.Msg.GetAdded())
	}
	timeline, err := env.rooms.GetRoomEvents(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{RoomId: room.Id, Limit: 10}))
	require.NoError(t, err)
	deleted := timelinePageEvent(timeline.Msg.GetPage(), root.Msg.GetMessageId()).GetMessagePosted().GetMessage()
	require.NotNil(t, deleted.GetDeletedAt())
	require.Empty(t, deleted.GetBody())
	require.Len(t, deleted.GetReactions(), 1)
	require.Equal(t, "heart", deleted.GetReactions()[0].GetEmoji())
	require.EqualValues(t, 1, deleted.GetReactions()[0].GetCount())
	reply, err := service.ImportMessage(env.ctx, connect.NewRequest(&operatorv1.ImportMessageRequest{
		RoomId: room.Id, AuthorId: author.Id, CreatedAt: timestamppb.Now(), Body: "reply", ThreadRootEventId: root.Msg.GetMessageId(),
	}))
	require.NoError(t, err)
	public, err := env.messages.GetMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetMessageRequest{
		RoomId: room.Id, EventId: reply.Msg.GetMessageId(),
	}))
	require.NoError(t, err)
	require.Equal(t, "reply", public.Msg.GetMessage().GetBody())
}

func TestOperatorImportThreadAndReactionPublicRead(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("operator-thread-public")
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-thread-author", "Author", "password")
	require.NoError(t, err)
	service := &operatorMessageService{api: env.api}
	root, err := service.ImportMessage(env.ctx, connect.NewRequest(&operatorv1.ImportMessageRequest{
		RoomId: room.Id, AuthorId: author.Id, Body: "root", CreatedAt: timestamppb.New(time.Unix(1700000000, 0)), CreateThread: true,
		IdempotencyKey: "d04a221e-8a36-44ce-95a4-c422f6063135",
	}))
	require.NoError(t, err)
	retried, err := service.ImportMessage(env.ctx, connect.NewRequest(&operatorv1.ImportMessageRequest{
		RoomId: room.Id, AuthorId: author.Id, Body: "root", CreatedAt: timestamppb.New(time.Unix(1700000000, 0)), CreateThread: true,
		IdempotencyKey: "d04a221e-8a36-44ce-95a4-c422f6063135",
	}))
	require.NoError(t, err)
	require.Equal(t, root.Msg.GetMessageId(), retried.Msg.GetMessageId())
	reply, err := service.ImportMessage(env.ctx, connect.NewRequest(&operatorv1.ImportMessageRequest{
		RoomId: room.Id, AuthorId: author.Id, Body: "reply", CreatedAt: timestamppb.Now(), ThreadRootEventId: root.Msg.GetMessageId(),
	}))
	require.NoError(t, err)
	added, err := service.ImportReaction(env.ctx, connect.NewRequest(&operatorv1.ImportReactionRequest{
		RoomId: room.Id, MessageEventId: reply.Msg.GetMessageId(), UserId: author.Id, Emoji: "heart",
	}))
	require.NoError(t, err)
	require.True(t, added.Msg.GetAdded())
	message, err := env.messages.GetMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetMessageRequest{RoomId: room.Id, EventId: reply.Msg.GetMessageId()}))
	require.NoError(t, err)
	require.Equal(t, author.Id, message.Msg.GetMessage().GetActorId())
	reactions, err := env.messages.ListReactionUsers(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.ListReactionUsersRequest{
		RoomId: room.Id, MessageEventId: reply.Msg.GetMessageId(), Emoji: "heart",
	}))
	require.NoError(t, err)
	require.Equal(t, []string{author.Id}, reactions.Msg.GetUserIds())
	_, err = service.ImportMessage(env.ctx, connect.NewRequest(&operatorv1.ImportMessageRequest{
		RoomId: room.Id, AuthorId: author.Id, Body: "invalid", CreatedAt: timestamppb.Now(), ThreadRootEventId: root.Msg.GetMessageId(), CreateThread: true,
	}))
	require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
}
