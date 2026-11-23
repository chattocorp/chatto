package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// BenchmarkMyEventsHubRoomFanout measures the hub cost of one message post in
// a room: the per-principal filter, authorization, and queue insertion. Each
// case has the same room audience and a different number of connected
// principals that are not in the room. It thus shows how the cost grows with
// the number of connections instead of the size of the audience.
//
// It calls the fanout step directly. NATS ingress, protobuf decoding, and the
// projection wait are outside the measurement.
func BenchmarkMyEventsHubRoomFanout(b *testing.B) {
	const audience = 100
	for _, bystanders := range []int{0, 1_000, 10_000} {
		b.Run(fmt.Sprintf("audience_%d/bystanders_%d", audience, bystanders), func(b *testing.B) {
			runMyEventsHubRoomFanoutBenchmark(b, audience, bystanders)
		})
	}
}

func runMyEventsHubRoomFanoutBenchmark(b *testing.B, audience, bystanders int) {
	b.Helper()
	core := setupTestCoreWithEncryption(b)
	ctx := context.Background()

	members := make([]string, 0, audience)
	for i := range audience {
		user, err := core.CreateUser(ctx, SystemActorID, fmt.Sprintf("fanout%d", i), fmt.Sprintf("Fanout %d", i), "password123")
		require.NoError(b, err)
		members = append(members, user.Id)
	}
	room, err := core.CreateRoom(ctx, members[0], KindChannel, "", "fanout", "")
	require.NoError(b, err)
	for _, id := range members {
		_, err := core.JoinRoom(ctx, id, KindChannel, id, room.Id)
		require.NoError(b, err)
	}
	posted, err := core.PostMessage(ctx, KindChannel, room.Id, members[0], "fanout benchmark message", nil, "", "", nil, false)
	require.NoError(b, err)

	hub := core.myEventsModel.hub
	subs := make([]*myEventsSubscription, 0, audience)
	hub.mu.Lock()
	register := func(userID string, memberRooms map[string]struct{}) *myEventsSubscription {
		sub := newMyEventsSubscription(myEventsPrincipal{userID: userID})
		hub.nextID++
		sub.id = hub.nextID
		hub.subscribers[sub.id] = sub
		hub.addUserLocked(sub.principal, &myEventsUserState{
			memberRooms: memberRooms,
			subscribers: map[uint64]*myEventsSubscription{sub.id: sub},
		})
		return sub
	}
	for _, id := range members {
		subs = append(subs, register(id, map[string]struct{}{room.Id: {}}))
	}
	for i := range bystanders {
		register(fmt.Sprintf("bystander-%d", i), map[string]struct{}{})
	}
	hub.mu.Unlock()

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		hub.fanoutReadyRoomEvent(ctx, room.Id, posted, uint64(i)+1<<40, 0)
		for _, sub := range subs {
			select {
			case <-sub.C:
			default:
				b.Fatal("room member did not receive the event")
			}
		}
	}
}
