package connectapi

import (
	"context"
	"net/url"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/config"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type serverService struct {
	api *API
}

func (s *serverService) GetServer(ctx context.Context, _ *connect.Request[apiv1.GetServerRequest]) (*connect.Response[apiv1.GetServerResponse], error) {
	authMethods := s.api.config.Auth.EnabledProviderMethods()
	if s.api.config.Auth.DirectRegistrationOrDefault() {
		authMethods = append([]string{"password"}, authMethods...)
	}
	if authMethods == nil {
		authMethods = []string{}
	}

	response := &apiv1.GetServerResponse{
		Name:             s.api.effectiveServerName(ctx),
		Version:          s.api.version,
		AuthMethods:      authMethods,
		AuthProviders:    apiAuthProviders(s.api.config.Auth.PublicProviders()),
		RegistrationOpen: s.api.config.Auth.DirectRegistrationOrDefault(),
		AuthorizeUrl:     "/oauth/authorize",
	}
	if s.api.core != nil && s.api.core.ConfigManager() != nil {
		if welcome, err := s.api.core.ConfigManager().GetEffectiveWelcomeMessage(ctx); err == nil {
			response.WelcomeMessage = welcome
		}
		if cfg, err := s.api.core.ConfigManager().GetServerConfig(ctx); err == nil && cfg != nil {
			response.Description = cfg.Description
		}
	}
	if s.api.core != nil {
		bw, bh := 1200, 630
		if u, err := s.api.core.GetServerBannerURL(ctx, &bw, &bh, "cover"); err == nil {
			response.BannerUrl = s.api.absolutizeAssetURL(ctx, u)
		}
		lw, lh := 256, 256
		if u, err := s.api.core.GetServerLogoURL(ctx, &lw, &lh, "cover"); err == nil {
			response.IconUrl = s.api.absolutizeAssetURL(ctx, u)
		}
	}
	return connect.NewResponse(response), nil
}

func (a *API) effectiveServerName(ctx context.Context) string {
	if a.core != nil && a.core.ConfigManager() != nil {
		if n, err := a.core.ConfigManager().GetEffectiveServerName(ctx); err == nil {
			return n
		}
	}
	return "Chatto"
}

func apiAuthProviders(providers []config.AuthProviderConfig) []*apiv1.AuthProvider {
	result := make([]*apiv1.AuthProvider, 0, len(providers))
	for _, provider := range providers {
		result = append(result, &apiv1.AuthProvider{
			Id:       provider.ID,
			Type:     provider.Type,
			Label:    provider.LabelOrDefault(),
			LoginUrl: "/auth/providers/" + url.PathEscape(provider.ID),
		})
	}
	return result
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
