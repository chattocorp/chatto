package connectapi

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	operatorv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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

func TestOperatorImportThreadAndReactionPublicRead(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("operator-thread-public")
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-thread-author", "Author", "password")
	require.NoError(t, err)
	service := &operatorMessageService{api: env.api}
	root, err := service.ImportMessage(env.ctx, connect.NewRequest(&operatorv1.ImportMessageRequest{
		RoomId: room.Id, AuthorId: author.Id, Body: "root", CreatedAt: timestamppb.Now(), CreateThread: true,
	}))
	require.NoError(t, err)
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
