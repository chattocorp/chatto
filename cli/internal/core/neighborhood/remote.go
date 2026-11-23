package neighborhood

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"connectrpc.com/connect"

	discoveryv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/discovery/v1"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/discovery/v1/discoveryv1connect"
)

const (
	// MaxDiscoveryResponseBytes bounds one remote discovery response.
	MaxDiscoveryResponseBytes = 256 << 10
	userAgent                 = "ChattoBot/1.0 (Neighborhood)"
	// ConnectPrefix is the path below a server origin where Chatto mounts
	// its ConnectRPC services. It must equal connectapi.Prefix.
	ConnectPrefix = "/api/connect"
)

// RemoteFetcher loads discovery data through the public ConnectRPC discovery
// API of each remote server.
type RemoteFetcher struct {
	// Client sends the remote requests. It must reject redirects and
	// connections to private network addresses.
	Client *http.Client
}

func (f RemoteFetcher) discoveryClient(origin string) discoveryv1connect.ServerDiscoveryServiceClient {
	return discoveryv1connect.NewServerDiscoveryServiceClient(f.Client, origin+ConnectPrefix,
		connect.WithReadMaxBytes(MaxDiscoveryResponseBytes))
}

// ListNeighbors implements Fetcher.
func (f RemoteFetcher) ListNeighbors(ctx context.Context, origin string) ([]string, error) {
	request := connect.NewRequest(&discoveryv1.ListNeighborsRequest{})
	request.Header().Set("User-Agent", userAgent)
	response, err := f.discoveryClient(origin).ListNeighbors(ctx, request)
	if connect.CodeOf(err) == connect.CodeUnimplemented {
		return nil, ErrUnsupported
	}
	if err != nil {
		return nil, err
	}
	return response.Msg.GetOrigins(), nil
}

// GetProfile implements Fetcher.
func (f RemoteFetcher) GetProfile(ctx context.Context, origin string) (Profile, error) {
	request := connect.NewRequest(&discoveryv1.GetServerRequest{})
	request.Header().Set("User-Agent", userAgent)
	response, err := f.discoveryClient(origin).GetServer(ctx, request)
	if err != nil {
		return Profile{}, err
	}
	profile := response.Msg.GetProfile()
	if profile == nil {
		return Profile{}, fmt.Errorf("remote server returned no public profile")
	}
	return Profile{
		Name:        profile.GetName(),
		Version:     profile.GetVersion(),
		Description: profile.GetDescription(),
		LogoURL:     profile.GetLogoUrl(),
		BannerURL:   profile.GetBannerUrl(),
	}, nil
}

// ResolveImageURL returns the absolute URL of one public profile image. A
// relative image URL resolves against origin. An absolute image URL can use
// another HTTP(S) origin: a server often builds its image URLs from its
// canonical URL, which can differ from the origin that a Neighbor advertises.
// It rejects URLs with credentials and other schemes. The remote image cache
// fetches the image.
func ResolveImageURL(origin, rawURL string) (string, bool) {
	base, err := url.Parse(origin)
	if err != nil {
		return "", false
	}
	reference, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	parsed := base.ResolveReference(reference)
	if parsed.User != nil {
		return "", false
	}
	if _, ok := CanonicalOrigin(parsed.Scheme + "://" + parsed.Host); !ok {
		return "", false
	}
	return parsed.String(), true
}
