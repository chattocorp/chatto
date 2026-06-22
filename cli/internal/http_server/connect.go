package http_server

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"connectrpc.com/connect"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/auth"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const connectAPIPrefix = "/api/connect"

type serverService struct {
	server *HTTPServer
}

func (s *HTTPServer) setupConnectAPI() {
	s.mountConnectHandler(apiv1connect.NewServerServiceHandler(&serverService{server: s}))
	s.mountConnectHandler(apiv1connect.NewNotificationPreferencesServiceHandler(&notificationPreferencesService{server: s}))
}

func (s *HTTPServer) mountConnectHandler(servicePath string, serviceHandler http.Handler) {
	handler := http.StripPrefix(connectAPIPrefix, serviceHandler)
	s.router.Any(connectAPIPrefix+servicePath+"*connectPath", func(c *gin.Context) {
		handler.ServeHTTP(c.Writer, s.injectUserIntoContext(c))
	})
}

func (s *serverService) GetServer(ctx context.Context, _ *connect.Request[apiv1.GetServerRequest]) (*connect.Response[apiv1.GetServerResponse], error) {
	authMethods := s.server.config.Auth.EnabledProviderMethods()
	if s.server.config.Auth.DirectRegistrationOrDefault() {
		authMethods = append([]string{"password"}, authMethods...)
	}
	if authMethods == nil {
		authMethods = []string{}
	}

	response := &apiv1.GetServerResponse{
		Name:             s.server.effectiveServerName(ctx),
		Version:          s.server.version,
		AuthMethods:      authMethods,
		AuthProviders:    apiAuthProviders(s.server.config.Auth.PublicProviders()),
		RegistrationOpen: s.server.config.Auth.DirectRegistrationOrDefault(),
		AuthorizeUrl:     "/oauth/authorize",
	}
	if s.server.core != nil && s.server.core.ConfigManager() != nil {
		if welcome, err := s.server.core.ConfigManager().GetEffectiveWelcomeMessage(ctx); err == nil {
			response.WelcomeMessage = welcome
		}
		if cfg, err := s.server.core.ConfigManager().GetServerConfig(ctx); err == nil && cfg != nil {
			response.Description = cfg.Description
		}
	}
	if s.server.core != nil {
		bw, bh := 1200, 630
		if u, err := s.server.core.GetServerBannerURL(ctx, &bw, &bh, "cover"); err == nil {
			response.BannerUrl = s.server.absolutizeAssetURLFromConfig(u)
		}
		lw, lh := 256, 256
		if u, err := s.server.core.GetServerLogoURL(ctx, &lw, &lh, "cover"); err == nil {
			response.IconUrl = s.server.absolutizeAssetURLFromConfig(u)
		}
	}
	return connect.NewResponse(response), nil
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

func requireConnectAuth(ctx context.Context) (*corev1.User, error) {
	user := auth.ForContext(ctx)
	if user == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return user, nil
}

func connectError(err error) error {
	if err == nil {
		return nil
	}
	if connect.CodeOf(err) != connect.CodeUnknown {
		return err
	}
	if errors.Is(err, core.ErrNotAuthenticated) {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	if errors.Is(err, core.ErrPermissionDenied) || errors.Is(err, core.ErrNotRoomMember) {
		return connect.NewError(connect.CodePermissionDenied, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}
