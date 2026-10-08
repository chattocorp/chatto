package connectapi

import (
	"testing"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/google/uuid"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestMessageServiceIdempotencyAcrossCoreAndConnect(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("retry-room")
	ctx := withCaller(env.ctx, env.viewer)
	key := uuid.NewString()
	request := &apiv1.CreateMessageRequest{RoomId: room.Id, Body: "one send", IdempotencyKey: &key}
	first, err := env.messages.CreateMessage(ctx, connect.NewRequest(request))
	if err != nil {
		t.Fatal(err)
	}
	replay, err := env.messages.CreateMessage(ctx, connect.NewRequest(request))
	if err != nil || replay.Msg.Message.Id != first.Msg.Message.Id {
		t.Fatalf("replay = %v, err = %v", replay, err)
	}
	coreReplay, err := env.core.Messages().PostMessage(env.ctx, core.MessagePostInput{
		ActorID: env.viewer.Id, RoomID: room.Id, Body: request.Body, IdempotencyKey: key,
	})
	if err != nil || coreReplay.Event.Id != first.Msg.Message.Id {
		t.Fatalf("core replay = %v, err = %v", coreReplay, err)
	}
	_, err = env.messages.CreateMessage(ctx, connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId: room.Id, Body: "different send", IdempotencyKey: &key,
	}))
	if errorCode(err) != connect.CodeAlreadyExists {
		t.Fatalf("mismatch = %v", err)
	}
	for _, invalid := range []string{"", "not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
		request.IdempotencyKey = &invalid
		if invalid != "00000000-0000-0000-0000-000000000000" && protovalidate.Validate(request) == nil {
			t.Fatalf("wire validation accepted %q", invalid)
		}
		if invalid != "" {
			_, err := env.messages.CreateMessage(ctx, connect.NewRequest(request))
			if errorCode(err) != connect.CodeInvalidArgument {
				t.Fatalf("invalid key = %v", err)
			}
		}
	}
}
