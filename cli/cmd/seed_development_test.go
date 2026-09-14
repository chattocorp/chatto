//go:build bootstrap || test_endpoints

package cmd

import (
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"hmans.de/chatto/internal/core"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
	"testing"
)

func TestSeedCLIUsesOperatorSocket(t *testing.T) {
	env := newAdminCLITestEnv(t)
	output := env.run(t, "operator", "seed", "--seed", "73", "--users", "2", "--rooms", "1", "--messages", "4", "--thread-replies", "2", "--json")
	var result operatorv1.SeedDataResponse
	require.NoError(t, protojson.Unmarshal([]byte(output), &result))
	require.EqualValues(t, 73, result.Seed)
	require.Len(t, result.Users, 2)
	require.Len(t, result.Rooms, 1)
	require.Len(t, result.Messages, 4)
	require.NotEmpty(t, result.Rooms[0].MemberIds)
	for _, userID := range result.Rooms[0].MemberIds {
		member, err := env.core.GetRoomMembership(env.ctx, core.KindChannel, userID, result.Rooms[0].Id)
		require.NoError(t, err)
		require.Equal(t, userID, member.UserId)
	}
	for _, message := range result.Messages {
		body, err := env.core.GetMessageBody(env.ctx, message.Id)
		require.NoError(t, err)
		require.Equal(t, message.Body, body)
	}
}
