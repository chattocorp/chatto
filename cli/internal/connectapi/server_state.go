package connectapi

import (
	"bytes"
	"context"
	"time"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
)

type serverService struct {
	api *API
}

func (s *serverService) GetMotd(ctx context.Context, _ *connect.Request[apiv1.GetMotdRequest]) (*connect.Response[apiv1.GetMotdResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	motd, err := s.serverMotd(ctx)
	if err != nil {
		return nil, err
	}

	resp := &apiv1.GetMotdResponse{}
	if motd != "" {
		resp.Motd = stringPtr(motd)
	}
	return connect.NewResponse(resp), nil
}

func (s *serverService) GetRuntimeConfig(ctx context.Context, _ *connect.Request[apiv1.GetRuntimeConfigRequest]) (*connect.Response[apiv1.GetRuntimeConfigResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	return connect.NewResponse(&apiv1.GetRuntimeConfigResponse{Runtime: s.serverRuntimeConfig()}), nil
}

func (s *serverService) serverRuntimeConfig() *apiv1.ServerRuntimeConfig {
	maxUploadSize := s.api.core.AssetsConfig().MaxUploadSize
	maxVideoUploadSize := maxUploadSize
	if s.api.config.Video.Enabled {
		maxVideoUploadSize = int64(s.api.config.Video.MaxUploadSizeOrDefault())
	}
	runtime := &apiv1.ServerRuntimeConfig{
		PushNotificationsEnabled: s.api.config.Push.IsConfigured(),
		VideoProcessingEnabled:   s.api.config.Video.Enabled,
		MaxUploadSize:            maxUploadSize,
		MaxVideoUploadSize:       maxVideoUploadSize,
		MessageEditWindowSeconds: int32(core.MessageEditWindow / time.Second),
	}
	if s.api.config.Push.IsConfigured() {
		runtime.VapidPublicKey = stringPtr(s.api.config.Push.VAPIDPublicKey)
	}
	if s.api.config.LiveKit.IsConfigured() {
		runtime.LivekitUrl = stringPtr(s.api.config.LiveKit.URL)
	}
	return runtime
}

func (s *serverService) GetServerConfig(ctx context.Context, _ *connect.Request[adminv1.GetServerConfigRequest]) (*connect.Response[adminv1.GetServerConfigResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	cfg, err := s.api.core.GetManagedServerConfig(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	publicProfile, err := s.api.serverProfile(ctx, serverProfileOptions{})
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&adminv1.GetServerConfigResponse{
		Config:        adminServerConfig(cfg),
		PublicProfile: publicProfile,
	}), nil
}

func (s *serverService) UpdateServerConfig(ctx context.Context, req *connect.Request[adminv1.UpdateServerConfigRequest]) (*connect.Response[adminv1.UpdateServerConfigResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	cfg, err := s.api.core.UpdateServerConfig(ctx, caller.UserID, core.ServerConfigUpdateInput{
		ServerName:     req.Msg.ServerName,
		Description:    req.Msg.Description,
		MOTD:           req.Msg.Motd,
		WelcomeMessage: req.Msg.WelcomeMessage,
	})
	if err != nil {
		return nil, connectError(err)
	}

	publicProfile, err := s.api.serverProfile(ctx, serverProfileOptions{})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.UpdateServerConfigResponse{
		PublicProfile: publicProfile,
		Config:        adminServerConfig(cfg),
	}), nil
}

func (s *serverService) UploadServerLogo(ctx context.Context, req *connect.Request[adminv1.UploadServerLogoRequest]) (*connect.Response[adminv1.UploadServerLogoResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	image := req.Msg.GetImage()
	if image == nil || len(image.GetImage()) == 0 {
		return nil, invalidArgument("image is required")
	}

	if _, err := s.api.core.UploadManagedServerLogo(ctx, caller.UserID, bytes.NewReader(image.GetImage())); err != nil {
		return nil, connectError(err)
	}
	publicProfile, err := s.api.serverProfile(ctx, serverProfileOptions{})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.UploadServerLogoResponse{PublicProfile: publicProfile}), nil
}

func (s *serverService) DeleteServerLogo(ctx context.Context, _ *connect.Request[adminv1.DeleteServerLogoRequest]) (*connect.Response[adminv1.DeleteServerLogoResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.DeleteManagedServerLogo(ctx, caller.UserID); err != nil {
		return nil, connectError(err)
	}
	publicProfile, err := s.api.serverProfile(ctx, serverProfileOptions{})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.DeleteServerLogoResponse{PublicProfile: publicProfile}), nil
}

func (s *serverService) UploadServerBanner(ctx context.Context, req *connect.Request[adminv1.UploadServerBannerRequest]) (*connect.Response[adminv1.UploadServerBannerResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	image := req.Msg.GetImage()
	if image == nil || len(image.GetImage()) == 0 {
		return nil, invalidArgument("image is required")
	}

	if _, err := s.api.core.UploadManagedServerBanner(ctx, caller.UserID, bytes.NewReader(image.GetImage())); err != nil {
		return nil, connectError(err)
	}
	publicProfile, err := s.api.serverProfile(ctx, serverProfileOptions{})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.UploadServerBannerResponse{PublicProfile: publicProfile}), nil
}

func (s *serverService) DeleteServerBanner(ctx context.Context, _ *connect.Request[adminv1.DeleteServerBannerRequest]) (*connect.Response[adminv1.DeleteServerBannerResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.DeleteManagedServerBanner(ctx, caller.UserID); err != nil {
		return nil, connectError(err)
	}
	publicProfile, err := s.api.serverProfile(ctx, serverProfileOptions{})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.DeleteServerBannerResponse{PublicProfile: publicProfile}), nil
}

func (s *serverService) GetServerSecurityConfig(ctx context.Context, _ *connect.Request[adminv1.GetServerSecurityConfigRequest]) (*connect.Response[adminv1.GetServerSecurityConfigResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	blockedUsernames, err := s.api.core.GetServerSecurityConfig(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&adminv1.GetServerSecurityConfigResponse{
		BlockedUsernames: blockedUsernames,
	}), nil
}

func (s *serverService) UpdateBlockedUsernames(ctx context.Context, req *connect.Request[adminv1.UpdateBlockedUsernamesRequest]) (*connect.Response[adminv1.UpdateBlockedUsernamesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	blockedUsernames, err := s.api.core.UpdateBlockedUsernames(ctx, caller.UserID, req.Msg.GetBlockedUsernames())
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&adminv1.UpdateBlockedUsernamesResponse{
		BlockedUsernames: blockedUsernames,
	}), nil
}

func adminServerConfig(cfg *configv1.ServerConfig) *adminv1.ServerConfig {
	if cfg == nil {
		return &adminv1.ServerConfig{}
	}
	return &adminv1.ServerConfig{
		ServerName:     cfg.GetServerName(),
		Description:    cfg.GetDescription(),
		Motd:           cfg.GetMotd(),
		WelcomeMessage: cfg.GetWelcomeMessage(),
	}
}

func (s *serverService) serverMotd(ctx context.Context) (string, error) {
	if cm := s.api.core.ConfigManager(); cm != nil {
		motd, err := cm.GetEffectiveMOTD(ctx)
		if err != nil {
			return "", connectError(err)
		}
		return motd, nil
	}
	return "", nil
}
