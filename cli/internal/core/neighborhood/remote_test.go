package neighborhood

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	discoveryv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/discovery/v1"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/discovery/v1/discoveryv1connect"
)

type discoveryStub struct {
	discoveryv1connect.UnimplementedServerDiscoveryServiceHandler
	neighbors []string
}

func (s discoveryStub) GetServer(context.Context, *connect.Request[discoveryv1.GetServerRequest]) (*connect.Response[discoveryv1.GetServerResponse], error) {
	logo := "https://remote.example/logo.png"
	description := "A test server"
	return connect.NewResponse(&discoveryv1.GetServerResponse{Profile: &apiv1.ServerPublicProfile{
		Name: "Remote", Version: "0.5.0", Description: &description, LogoUrl: &logo,
	}}), nil
}

func (s discoveryStub) ListNeighbors(context.Context, *connect.Request[discoveryv1.ListNeighborsRequest]) (*connect.Response[discoveryv1.ListNeighborsResponse], error) {
	if s.neighbors == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, nil)
	}
	return connect.NewResponse(&discoveryv1.ListNeighborsResponse{Origins: s.neighbors}), nil
}

// discoveryMux mounts the discovery service below ConnectPrefix, like a
// Chatto server.
func discoveryMux(handler discoveryv1connect.ServerDiscoveryServiceHandler) http.Handler {
	path, serviceHandler := discoveryv1connect.NewServerDiscoveryServiceHandler(handler)
	mux := http.NewServeMux()
	mux.Handle(ConnectPrefix+path, http.StripPrefix(ConnectPrefix, serviceHandler))
	return mux
}

func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func TestRemoteFetcherDiscoveryCalls(t *testing.T) {
	server := httptest.NewServer(discoveryMux(discoveryStub{neighbors: []string{"https://a.example"}}))
	t.Cleanup(server.Close)
	fetcher := RemoteFetcher{Client: noRedirectClient()}

	neighbors, err := fetcher.ListNeighbors(context.Background(), server.URL)
	require.NoError(t, err)
	require.Equal(t, []string{"https://a.example"}, neighbors)

	profile, err := fetcher.GetProfile(context.Background(), server.URL)
	require.NoError(t, err)
	require.Equal(t, Profile{Name: "Remote", Version: "0.5.0", Description: "A test server", LogoURL: "https://remote.example/logo.png"}, profile)

	legacy := httptest.NewServer(discoveryMux(discoveryStub{}))
	t.Cleanup(legacy.Close)
	_, err = fetcher.ListNeighbors(context.Background(), legacy.URL)
	require.ErrorIs(t, err, ErrUnsupported)
}

func TestResolveImageURL(t *testing.T) {
	const origin = "https://remote.example"
	for name, tc := range map[string]struct {
		rawURL string
		want   string
	}{
		"absolute URL": {"https://remote.example/logo.png", "https://remote.example/logo.png"},
		// A relative URL resolves against the advertised origin.
		"relative URL": {"/assets/logo.png?v=2", "https://remote.example/assets/logo.png?v=2"},
		// A server can build image URLs from a canonical URL that differs
		// from the advertised origin.
		"other origin": {"https://cdn.example/logo.png", "https://cdn.example/logo.png"},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := ResolveImageURL(origin, tc.rawURL)
			require.True(t, ok)
			require.Equal(t, tc.want, got)
		})
	}
	for name, rawURL := range map[string]string{
		"unsupported scheme": "ftp://elsewhere.example/logo.png",
		"credentials":        "https://user@remote.example/logo.png",
		"malformed URL":      "https://[::1",
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := ResolveImageURL(origin, rawURL)
			require.False(t, ok)
		})
	}
}
