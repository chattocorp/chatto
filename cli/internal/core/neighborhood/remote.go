package neighborhood

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"

	"connectrpc.com/connect"

	discoveryv1 "hmans.de/chatto/internal/pb/chatto/discovery/v1"
	"hmans.de/chatto/internal/pb/chatto/discovery/v1/discoveryv1connect"
)

const (
	// MaxDiscoveryResponseBytes bounds one remote discovery response.
	MaxDiscoveryResponseBytes = 256 << 10
	// MaxImageBytes bounds one remote profile image before decoding.
	MaxImageBytes = 5 << 20
	userAgent     = "ChattoBot/1.0 (Neighborhood)"
	// ConnectPrefix is the path below a server origin where Chatto mounts
	// its ConnectRPC services. It must equal connectapi.Prefix.
	ConnectPrefix = "/api/connect"
)

// acceptedImageTypes are the declared media types that discovery decodes.
var acceptedImageTypes = map[string]struct{}{
	"image/gif":  {},
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

// ErrRejectedImage means that a remote image did not meet the profile image
// rules.
var ErrRejectedImage = errors.New("remote profile image rejected")

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

// FetchImage loads one public profile image. The image URL must use the
// canonical origin of the server that advertised it. The request sends no
// credentials or referrer. The response must declare a supported raster
// media type and contain at most MaxImageBytes.
func (f RemoteFetcher) FetchImage(ctx context.Context, origin, rawURL string) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || !parsed.IsAbs() || parsed.User != nil {
		return nil, ErrRejectedImage
	}
	imageOrigin, ok := CanonicalOrigin(parsed.Scheme + "://" + parsed.Host)
	if !ok || imageOrigin != origin {
		return nil, ErrRejectedImage
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, ErrRejectedImage
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Accept", "image/webp,image/png,image/jpeg,image/gif")
	response, err := f.Client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrRejectedImage, response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return nil, ErrRejectedImage
	}
	if _, accepted := acceptedImageTypes[mediaType]; !accepted {
		return nil, ErrRejectedImage
	}
	if response.ContentLength > MaxImageBytes {
		return nil, ErrRejectedImage
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxImageBytes {
		return nil, ErrRejectedImage
	}
	return data, nil
}
