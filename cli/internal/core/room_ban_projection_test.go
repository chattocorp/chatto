package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestRoomBanProjectionStableOrder(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse_replay_%t", reverse), func(t *testing.T) {
			p := NewRoomBanProjection()
			for n := 0; n < 10; n++ {
				i := n
				if reverse {
					i = 9 - n
				}
				created := now
				if i == 9 {
					created = now.Add(time.Hour)
				}
				if i == 0 {
					created = now.Add(-time.Hour)
				}
				require.NoError(t, p.Apply(&evtv1.Event{
					Id:        fmt.Sprintf("ban-%02d", i),
					CreatedAt: timestamppb.New(created),
					Event: &evtv1.Event_RoomMemberBanned{RoomMemberBanned: &evtv1.RoomMemberBannedEvent{
						RoomId: "room", UserId: fmt.Sprintf("user-%02d", i), Reason: "test",
					}},
				}, uint64(n+1)))
			}
			// Both list paths must give the same page boundaries on repeated reads.
			for read := 0; read < 32; read++ {
				for _, bans := range [][]RoomBan{p.ActiveBans(now), p.ActiveRoomBans("room", now)} {
					require.Len(t, bans, 10)
					for i, ban := range bans {
						want := []int{9, 1, 2, 3, 4, 5, 6, 7, 8, 0}
						require.Equal(t, fmt.Sprintf("ban-%02d", want[i]), ban.EventID)
					}
				}
			}
		})
	}
}
