//go:build !bootstrap && !test_endpoints

package http_server

import (
	"connectrpc.com/connect"
	"context"
	"github.com/chattocorp/chatto/cli/internal/config"
	operatorv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1/operatorv1connect"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProductionDoesNotMountOperatorSeed(t *testing.T) {
	s, _ := setupConnectTestServerWithConfig(t, config.ChattoConfig{})
	server := newOperatorAPITestServer(t, s)
	client := operatorv1connect.NewOperatorSeedServiceClient(server.Client(), server.URL+connectAPIPrefix)
	_, err := client.SeedData(context.Background(), connect.NewRequest(&operatorv1.SeedDataRequest{}))
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
