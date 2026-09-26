package connectapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/core/neighborhood"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	cachestatev1 "hmans.de/chatto/internal/pb/chatto/core/cache_state/v1"
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

func TestServerDiscoveryListNeighborhoodServers(t *testing.T) {
	env := newConnectAPITestEnv(t)
	service := &serverDiscoveryService{api: env.api}

	// Core discovery writes an empty directory after boot because the server
	// has no Neighbors.
	var initial *cachestatev1.NeighborhoodDirectory
	require.Eventually(t, func() bool {
		directory, err := env.core.NeighborhoodDirectory(env.ctx)
		require.NoError(t, err)
		initial = directory
		return directory != nil
	}, 5*time.Second, 10*time.Millisecond)
	empty, err := service.ListNeighborhoodServers(env.ctx, connect.NewRequest(&discoveryv1.ListNeighborhoodServersRequest{}))
	require.NoError(t, err)
	require.Empty(t, empty.Msg.GetServers())
	require.NotNil(t, empty.Msg.GetRefreshedAt())

	logoName := strings.Repeat("a", 64)
	data, err := proto.Marshal(&cachestatev1.NeighborhoodDirectory{
		RefreshedAt:       timestamppb.Now(),
		SourceFingerprint: initial.GetSourceFingerprint(),
		Servers: []*cachestatev1.NeighborhoodServerRecord{{
			Origin:               "https://a.example",
			Name:                 "A",
			Version:              "0.5.0",
			Description:          "First",
			DirectNeighbor:       true,
			RecommendedByOrigins: []string{"https://b.example"},
			Logo:                 &cachestatev1.NeighborhoodImage{SourceUrl: "https://a.example/logo.png", ObjectName: logoName},
		}},
	})
	require.NoError(t, err)
	js, err := jetstream.New(env.nc)
	require.NoError(t, err)
	kv, err := js.KeyValue(env.ctx, "MEMORY_CACHE")
	require.NoError(t, err)
	_, err = kv.Put(env.ctx, "neighborhood.directory", data)
	require.NoError(t, err)

	api := New(env.core, config.ChattoConfig{Webserver: config.WebserverConfig{URL: "https://self.example"}}, "test")
	response, err := (&serverDiscoveryService{api: api}).ListNeighborhoodServers(env.ctx, connect.NewRequest(&discoveryv1.ListNeighborhoodServersRequest{}))
	require.NoError(t, err)
	require.Len(t, response.Msg.GetServers(), 1)
	server := response.Msg.GetServers()[0]
	require.Equal(t, "https://a.example", server.GetOrigin())
	require.True(t, server.GetDirectNeighbor())
	require.Equal(t, []string{"https://b.example"}, server.GetRecommendedByOrigins())
	require.Equal(t, "A", server.GetProfile().GetName())
	require.Equal(t, "0.5.0", server.GetProfile().GetVersion())
	require.Equal(t, "First", server.GetProfile().GetDescription())
	require.Equal(t, "/assets/neighborhood/"+logoName, server.GetProfile().GetLogoUrl())
	require.Nil(t, server.GetProfile().BannerUrl)
	require.Nil(t, server.GetProfile().WelcomeMessage)
}

func TestNeighborhoodDiscoveryUsesConnectPrefix(t *testing.T) {
	require.Equal(t, Prefix, neighborhood.ConnectPrefix)
}
