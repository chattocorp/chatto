package connectapi

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	discoveryv1 "hmans.de/chatto/internal/pb/chatto/discovery/v1"
)

func TestAdminServerServiceNeighborCRUDAndPublicDiscovery(t *testing.T) {
	env := newConnectAPITestEnv(t)
	callerCtx := withCaller(env.ctx, env.viewer)

	_, err := env.serverState.ListNeighbors(callerCtx, connect.NewRequest(&adminv1.ListNeighborsRequest{}))
	requireConnectCode(t, err, connect.CodePermissionDenied)
	require.NoError(t, env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermServerManageNeighbors))

	legacyRequestBytes, err := proto.Marshal(&adminv1.CreateNeighborRequest{Origin: "https://Neighbor.Example/"})
	require.NoError(t, err)
	legacyRequestBytes = protowire.AppendTag(legacyRequestBytes, 2, protowire.BytesType)
	legacyRequestBytes = protowire.AppendString(legacyRequestBytes, "legacy testimonial")
	legacyRequest := &adminv1.CreateNeighborRequest{}
	require.NoError(t, proto.Unmarshal(legacyRequestBytes, legacyRequest))
	require.NotEmpty(t, legacyRequest.ProtoReflect().GetUnknown())

	created, err := env.serverState.CreateNeighbor(callerCtx, connect.NewRequest(legacyRequest))
	require.NoError(t, err)
	neighbor := created.Msg.GetNeighbor()
	require.Equal(t, "https://neighbor.example", neighbor.GetOrigin())
	require.NotEmpty(t, neighbor.GetId())
	require.NotEmpty(t, neighbor.GetRevision())

	got, err := env.serverState.GetNeighbor(callerCtx, connect.NewRequest(&adminv1.GetNeighborRequest{NeighborId: neighbor.GetId()}))
	require.NoError(t, err)
	require.Equal(t, neighbor, got.Msg.GetNeighbor())
	_, err = env.serverState.CreateNeighbor(callerCtx, connect.NewRequest(&adminv1.CreateNeighborRequest{Origin: "https://neighbor.example"}))
	requireConnectCode(t, err, connect.CodeAlreadyExists)
	_, err = env.serverState.CreateNeighbor(callerCtx, connect.NewRequest(&adminv1.CreateNeighborRequest{Origin: "neighbor.example"}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)
	_, err = env.serverState.CreateNeighbor(callerCtx, connect.NewRequest(&adminv1.CreateNeighborRequest{Origin: "https://self.example"}))
	requireConnectCode(t, err, connect.CodeFailedPrecondition)

	listed, err := env.serverState.ListNeighbors(callerCtx, connect.NewRequest(&adminv1.ListNeighborsRequest{}))
	require.NoError(t, err)
	require.Len(t, listed.Msg.GetNeighbors(), 1)

	public, err := (&serverDiscoveryService{api: env.api}).ListNeighbors(context.Background(), connect.NewRequest(&discoveryv1.ListNeighborsRequest{}))
	require.NoError(t, err)
	require.Equal(t, []string{"https://neighbor.example"}, public.Msg.GetOrigins())

	updated, err := env.serverState.UpdateNeighbor(callerCtx, connect.NewRequest(&adminv1.UpdateNeighborRequest{
		NeighborId: neighbor.GetId(), Origin: "https://updated.example", Revision: neighbor.GetRevision(),
	}))
	require.NoError(t, err)
	require.Equal(t, "https://updated.example", updated.Msg.GetNeighbor().GetOrigin())
	_, err = env.serverState.UpdateNeighbor(callerCtx, connect.NewRequest(&adminv1.UpdateNeighborRequest{
		NeighborId: neighbor.GetId(), Origin: "https://self.example", Revision: updated.Msg.GetNeighbor().GetRevision(),
	}))
	requireConnectCode(t, err, connect.CodeFailedPrecondition)

	_, err = env.serverState.DeleteNeighbor(callerCtx, connect.NewRequest(&adminv1.DeleteNeighborRequest{NeighborId: neighbor.GetId(), Revision: neighbor.GetRevision()}))
	requireConnectCode(t, err, connect.CodeAborted)
	_, err = env.serverState.DeleteNeighbor(callerCtx, connect.NewRequest(&adminv1.DeleteNeighborRequest{NeighborId: neighbor.GetId(), Revision: updated.Msg.GetNeighbor().GetRevision()}))
	require.NoError(t, err)
}
