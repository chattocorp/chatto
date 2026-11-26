package connectapi

import (
	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"testing"
)

func TestPersonalGroupOrderCommandAndConcurrentReads(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	first := env.defaultRoomGroupID(t)
	second, err := env.core.RoomLayout().CreateRoomGroup(env.ctx, core.SystemActorID, "Second", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.directory.SetPersonalRoomGroupOrder(env.ctx, connect.NewRequest(&apiv1.SetPersonalRoomGroupOrderRequest{})); errorCode(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated: %v", err)
	}
	if _, err := env.directory.SetPersonalRoomGroupOrder(ctx, connect.NewRequest(&apiv1.SetPersonalRoomGroupOrderRequest{GroupIds: []string{first, first}})); errorCode(err) != connect.CodeInvalidArgument {
		t.Fatalf("duplicate: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 40; i++ {
			ids := []string{second.Id, first}
			if i%2 == 0 {
				ids = nil
			}
			_, err := env.directory.SetPersonalRoomGroupOrder(ctx, connect.NewRequest(&apiv1.SetPersonalRoomGroupOrderRequest{GroupIds: ids}))
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 40; i++ {
		response, err := env.directory.ListRoomGroups(ctx, connect.NewRequest(&apiv1.ListRoomGroupsRequest{}))
		if err != nil {
			t.Fatal(err)
		}
		want := first
		if len(response.Msg.PersonalGroupOrder) > 0 {
			want = response.Msg.PersonalGroupOrder[0]
		}
		if len(response.Msg.Groups) == 0 || response.Msg.Groups[0].Id != want {
			t.Fatal("groups and personal preference came from different states")
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
