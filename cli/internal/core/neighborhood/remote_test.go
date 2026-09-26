package neighborhood

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	discoveryv1 "hmans.de/chatto/internal/pb/chatto/discovery/v1"
	"hmans.de/chatto/internal/pb/chatto/discovery/v1/discoveryv1connect"
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

func TestRemoteFetcherFetchImage(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nimage")
	mux := http.NewServeMux()
	mux.HandleFunc("/logo.png", func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Cookie"))
		require.Empty(t, r.Header.Get("Referer"))
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
	mux.HandleFunc("/image.svg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte("<svg/>"))
	})
	mux.HandleFunc("/redirect.png", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/logo.png", http.StatusFound)
	})
	mux.HandleFunc("/large.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(strings.Repeat("x", MaxImageBytes+1)))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	origin, ok := CanonicalOrigin(server.URL)
	require.True(t, ok)
	fetcher := RemoteFetcher{Client: noRedirectClient()}

	data, err := fetcher.FetchImage(context.Background(), origin, server.URL+"/logo.png")
	require.NoError(t, err)
	require.Equal(t, png, data)

	// A relative URL resolves against the advertised origin.
	data, err = fetcher.FetchImage(context.Background(), origin, "/logo.png")
	require.NoError(t, err)
	require.Equal(t, png, data)

	// A server can build image URLs from a canonical URL that differs from
	// the advertised origin.
	data, err = fetcher.FetchImage(context.Background(), "https://advertised.example", server.URL+"/logo.png")
	require.NoError(t, err)
	require.Equal(t, png, data)

	for name, rawURL := range map[string]string{
		"unsupported scheme":  "ftp://elsewhere.example/logo.png",
		"credentials":         strings.Replace(server.URL, "http://", "http://user@", 1) + "/logo.png",
		"unsupported type":    server.URL + "/image.svg",
		"redirect":            server.URL + "/redirect.png",
		"oversized response":  server.URL + "/large.png",
		"missing image route": server.URL + "/missing.png",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := fetcher.FetchImage(context.Background(), origin, rawURL)
			require.ErrorIs(t, err, ErrRejectedImage)
		})
	}
}
