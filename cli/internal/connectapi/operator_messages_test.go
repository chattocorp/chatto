package connectapi

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

func TestOperatorImportMessageAppearsWithMappedAuthorInPublicRead(t *testing.T) {
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
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("preview without URL error = %v", err)
	}
}
