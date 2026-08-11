package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestBotRuntimeDirectMessageCapabilitiesAndContext(t *testing.T) {
	env := newConnectAPITestEnv(t)
	bot, err := env.core.CreateBot(env.ctx, core.SystemActorID, env.viewer.GetId(), "runtime_bot", "Runtime Bot", "Answers only in direct messages.")
	if err != nil {
		t.Fatal(err)
	}
	botCtx := withCaller(env.ctx, bot)

	if _, err := env.botRuntime.ListBotDirectMessages(botCtx, connect.NewRequest(&apiv1.ListBotDirectMessagesRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("list without capability code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.core.SetBotCapabilities(env.ctx, core.SystemActorID, bot.GetId(), []string{
		string(core.ApplicationCapabilityDMMessageRead), string(core.ApplicationCapabilityMessageWrite),
	}); err != nil {
		t.Fatal(err)
	}
	dm, _, err := env.core.RoomCommands().StartDM(env.ctx, core.RoomStartDMInput{
		ActorID: env.viewer.GetId(), ParticipantIDs: []string{bot.GetId()},
	})
	if err != nil {
		t.Fatalf("StartDM with capable bot: %v", err)
	}
	humanMessage, err := env.core.PostMessage(env.ctx, core.KindDM, dm.GetId(), env.viewer.GetId(), "hello bot", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}

	listed, err := env.botRuntime.ListBotDirectMessages(botCtx, connect.NewRequest(&apiv1.ListBotDirectMessagesRequest{}))
	if err != nil {
		t.Fatalf("ListBotDirectMessages: %v", err)
	}
	if len(listed.Msg.GetRooms()) != 1 || listed.Msg.GetRooms()[0].GetId() != dm.GetId() || listed.Msg.GetPage().GetTotalCount() != 1 {
		t.Fatalf("listed DMs = %+v", listed.Msg)
	}
	read, err := env.botRuntime.GetBotDirectMessageEvents(botCtx, connect.NewRequest(&apiv1.GetBotDirectMessageEventsRequest{RoomId: dm.GetId(), Limit: 50}))
	if err != nil {
		t.Fatalf("GetBotDirectMessageEvents: %v", err)
	}
	if !timelinePageContains(read.Msg.GetPage(), humanMessage.GetId()) {
		t.Fatalf("DM page does not contain human message: %+v", read.Msg.GetPage())
	}
	posted, err := env.botRuntime.CreateBotDirectMessage(botCtx, connect.NewRequest(&apiv1.CreateBotDirectMessageRequest{
		RoomId: dm.GetId(), Body: "hello human", InReplyTo: humanMessage.GetId(),
	}))
	if err != nil {
		t.Fatalf("CreateBotDirectMessage: %v", err)
	}
	if posted.Msg.GetMessage().GetBody() != "hello human" {
		t.Fatalf("posted bot message = %+v", posted.Msg.GetMessage())
	}

	channel := env.createJoinedRoom("bot-runtime-hidden-channel")
	if _, err := env.botRuntime.GetBotDirectMessageEvents(botCtx, connect.NewRequest(&apiv1.GetBotDirectMessageEventsRequest{RoomId: channel.GetId()})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("channel read code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.botRuntime.CreateBotDirectMessage(botCtx, connect.NewRequest(&apiv1.CreateBotDirectMessageRequest{RoomId: channel.GetId(), Body: "no"})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("channel post code = %v, want permission_denied", connect.CodeOf(err))
	}

	if _, err := env.core.SetBotCapabilities(env.ctx, core.SystemActorID, bot.GetId(), []string{string(core.ApplicationCapabilityMessageWrite)}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.botRuntime.GetBotDirectMessageEvents(botCtx, connect.NewRequest(&apiv1.GetBotDirectMessageEventsRequest{RoomId: dm.GetId()})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("read after capability removal code = %v, want permission_denied", connect.CodeOf(err))
	}
	if err := env.core.DenyUserPermission(env.ctx, core.SystemActorID, env.viewer.GetId(), core.PermMessagePost); err != nil {
		t.Fatal(err)
	}
	if _, err := env.botRuntime.CreateBotDirectMessage(botCtx, connect.NewRequest(&apiv1.CreateBotDirectMessageRequest{RoomId: dm.GetId(), Body: "blocked by owner"})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("post after owner deny code = %v, want permission_denied", connect.CodeOf(err))
	}
}
