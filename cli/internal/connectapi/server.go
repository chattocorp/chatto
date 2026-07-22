package connectapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	discoveryv1 "hmans.de/chatto/internal/pb/chatto/discovery/v1"
)

const discoveryCacheControl = "public, no-cache"

var discoveryProtocolCapabilities = []string{
	"chatto.discovery.v1",
	"chatto.auth.v1",
	"chatto.api.v1",
	"chatto.api.message-search.v1",
	"chatto.admin.v1",
	"chatto.realtime.v1",
	"chatto.realtime.projection.v1",
}

type serverDiscoveryService struct {
	api *API
}

type serverProfileOptions struct {
	tolerateErrors bool
}

func (s *serverDiscoveryService) GetServer(ctx context.Context, _ *connect.Request[discoveryv1.GetServerRequest]) (*connect.Response[discoveryv1.GetServerResponse], error) {
	profile, err := s.api.serverProfile(ctx, serverProfileOptions{tolerateErrors: true})
	if err != nil {
		return nil, err
	}
	response := &discoveryv1.GetServerResponse{
		Profile: profile,
		Login: &apiv1.ServerLogin{
			DirectRegistrationEnabled: s.api.config.Auth.DirectRegistrationOrDefault(),
			Providers:                 apiAuthProviders(s.api.config.Auth.PublicProviders()),
			AuthorizeUrl:              "/oauth/authorize",
		},
		Compatibility: &discoveryv1.ServerCompatibility{
			ProtocolCapabilities: discoveryProtocolCapabilities,
		},
	}
	if callInfo, ok := connect.CallInfoForHandlerContext(ctx); ok && callInfo.HTTPMethod() == http.MethodGet {
		etag, err := discoveryResponseETag(response)
		if err != nil {
			return nil, connectInternalError(fmt.Errorf("marshal discovery response for ETag: %w", err))
		}
		cacheHeaders := http.Header{
			"Cache-Control": []string{discoveryCacheControl},
			"Etag":          []string{etag},
		}
		if ifNoneMatch(callInfo.RequestHeader().Get("If-None-Match"), etag) {
			return nil, connect.NewNotModifiedError(cacheHeaders)
		}
		for name, values := range cacheHeaders {
			callInfo.ResponseHeader()[name] = values
		}
	}
	return connect.NewResponse(response), nil
}

func discoveryResponseETag(response *discoveryv1.GetServerResponse) (string, error) {
	data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(response)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:]) + `"`, nil
}

// ifNoneMatch applies the weak comparison required for If-None-Match. It
// accepts wildcard and comma-separated validators emitted by HTTP caches.
func ifNoneMatch(headerValue, etag string) bool {
	for candidate := range strings.SplitSeq(headerValue, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag {
			return true
		}
		if len(candidate) >= 2 && strings.EqualFold(candidate[:2], "W/") && strings.TrimSpace(candidate[2:]) == etag {
			return true
		}
	}
	return false
}

func (a *API) effectiveServerName() string {
	if a.core != nil && a.core.ConfigModel() != nil {
		return a.core.ConfigModel().GetEffectiveServerName()
	}
	return "Chatto"
}

func (a *API) serverProfile(ctx context.Context, options serverProfileOptions) (*apiv1.ServerPublicProfile, error) {
	profile := &apiv1.ServerPublicProfile{Name: a.effectiveServerName(), Version: a.version}

	if a.core != nil && a.core.ConfigModel() != nil {
		cm := a.core.ConfigModel()
		if welcome := cm.GetEffectiveWelcomeMessage(); welcome != "" {
			profile.WelcomeMessage = stringPtr(welcome)
		}
		if cfg := cm.GetServerConfig(); cfg != nil && cfg.GetDescription() != "" {
			profile.Description = stringPtr(cfg.GetDescription())
		}
	}

	if a.core != nil {
		bw, bh := 1200, 630
		if u, err := a.core.GetServerBannerURL(ctx, &bw, &bh, "cover"); err != nil {
			if !options.tolerateErrors {
				return nil, connectError(err)
			}
		} else if u != "" {
			profile.BannerUrl = stringPtr(a.absolutizeAssetURL(ctx, u))
		}
		lw, lh := 256, 256
		if u, err := a.core.GetServerLogoURL(ctx, &lw, &lh, "cover"); err != nil {
			if !options.tolerateErrors {
				return nil, connectError(err)
			}
		} else if u != "" {
			profile.LogoUrl = stringPtr(a.absolutizeAssetURL(ctx, u))
		}
	}

	return profile, nil
}

func apiAuthProviders(providers []config.AuthProviderConfig) []*apiv1.ProviderMetadata {
	result := make([]*apiv1.ProviderMetadata, 0, len(providers))
	for _, provider := range providers {
		result = append(result, apiProviderMetadata(provider))
	}
	return result
}

func apiProviderMetadata(provider config.AuthProviderConfig) *apiv1.ProviderMetadata {
	return &apiv1.ProviderMetadata{
		Id:       provider.ID,
		Type:     provider.Type,
		Label:    provider.LabelOrDefault(),
		LoginUrl: "/auth/providers/" + url.PathEscape(provider.ID),
	}
}

func (a *API) absolutizeAssetURL(ctx context.Context, assetURL string) string {
	if assetURL == "" || strings.HasPrefix(assetURL, "http://") || strings.HasPrefix(assetURL, "https://") {
		return assetURL
	}
	if a.config.Webserver.URL != "" {
		base, err := url.Parse(a.config.Webserver.URL)
		if err == nil && base.Scheme != "" && base.Host != "" {
			return base.Scheme + "://" + base.Host + assetURL
		}
	}
	if requestBaseURL := requestBaseURLFromContext(ctx); requestBaseURL != "" {
		return requestBaseURL + assetURL
	}
	return assetURL
}
