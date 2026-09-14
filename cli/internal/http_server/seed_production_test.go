//go:build !bootstrap && !test_endpoints

package http_server

import (
	"connectrpc.com/connect"
	"context"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/config"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
	"hmans.de/chatto/internal/pb/chatto/operator/v1/operatorv1connect"
	"testing"
)

func TestProductionDoesNotMountOperatorSeed(t *testing.T) {
	s, _ := setupConnectTestServerWithConfig(t, config.ChattoConfig{})
	server := newOperatorAPITestServer(t, s)
	client := operatorv1connect.NewOperatorSeedServiceClient(server.Client(), server.URL+connectAPIPrefix)
	_, err := client.SeedData(context.Background(), connect.NewRequest(&operatorv1.SeedDataRequest{}))
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
