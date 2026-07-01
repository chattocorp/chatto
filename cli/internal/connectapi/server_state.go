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

func (s *serverService) GetServerState(ctx context.Context, _ *connect.Request[apiv1.GetServerStateRequest]) (*connect.Response[apiv1.GetServerStateResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	profile, err := s.serverProfile(ctx)
	if err != nil {
		return nil, err
	}

	maxUploadSize := s.api.core.AssetsConfig().MaxUploadSize
	maxVideoUploadSize := maxUploadSize
	if s.api.config.Video.Enabled {
		maxVideoUploadSize = int64(s.api.config.Video.MaxUploadSizeOrDefault())
	}
	response := &apiv1.GetServerStateResponse{
		Profile: profile,
		Runtime: &apiv1.ServerRuntimeConfig{
			PushNotificationsEnabled:  s.api.config.Push.IsConfigured(),
			DirectRegistrationEnabled: s.api.config.Auth.DirectRegistrationOrDefault(),
			VideoProcessingEnabled:    s.api.config.Video.Enabled,
			MaxUploadSize:             maxUploadSize,
			MaxVideoUploadSize:        maxVideoUploadSize,
			MessageEditWindowSeconds:  int32(core.MessageEditWindow / time.Second),
		},
	}
	if s.api.config.Push.IsConfigured() {
		response.Runtime.VapidPublicKey = stringPtr(s.api.config.Push.VAPIDPublicKey)
	}
	if s.api.config.LiveKit.IsConfigured() {
		response.Runtime.LivekitUrl = stringPtr(s.api.config.LiveKit.URL)
	}

	return connect.NewResponse(response), nil
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
	profile, err := s.serverProfile(ctx)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&adminv1.GetServerConfigResponse{
		Config:  adminServerConfig(cfg),
		Profile: profile,
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

	profile, err := s.serverProfile(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.UpdateServerConfigResponse{
		Profile: profile,
		Config:  adminServerConfig(cfg),
	}), nil
}

func (s *serverService) UploadServerLogo(ctx context.Context, req *connect.Request[adminv1.UploadServerLogoRequest]) (*connect.Response[adminv1.UploadServerLogoResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if len(req.Msg.GetImage()) == 0 {
		return nil, invalidArgument("image is required")
	}

	if _, err := s.api.core.UploadManagedServerLogo(ctx, caller.UserID, bytes.NewReader(req.Msg.GetImage())); err != nil {
		return nil, connectError(err)
	}
	profile, err := s.serverProfile(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.UploadServerLogoResponse{Profile: profile}), nil
}

func (s *serverService) DeleteServerLogo(ctx context.Context, _ *connect.Request[adminv1.DeleteServerLogoRequest]) (*connect.Response[adminv1.DeleteServerLogoResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.DeleteManagedServerLogo(ctx, caller.UserID); err != nil {
		return nil, connectError(err)
	}
	profile, err := s.serverProfile(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.DeleteServerLogoResponse{Profile: profile}), nil
}

func (s *serverService) UploadServerBanner(ctx context.Context, req *connect.Request[adminv1.UploadServerBannerRequest]) (*connect.Response[adminv1.UploadServerBannerResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if len(req.Msg.GetImage()) == 0 {
		return nil, invalidArgument("image is required")
	}

	if _, err := s.api.core.UploadManagedServerBanner(ctx, caller.UserID, bytes.NewReader(req.Msg.GetImage())); err != nil {
		return nil, connectError(err)
	}
	profile, err := s.serverProfile(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.UploadServerBannerResponse{Profile: profile}), nil
}

func (s *serverService) DeleteServerBanner(ctx context.Context, _ *connect.Request[adminv1.DeleteServerBannerRequest]) (*connect.Response[adminv1.DeleteServerBannerResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.DeleteManagedServerBanner(ctx, caller.UserID); err != nil {
		return nil, connectError(err)
	}
	profile, err := s.serverProfile(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.DeleteServerBannerResponse{Profile: profile}), nil
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

func (s *serverService) serverProfile(ctx context.Context) (*apiv1.ServerProfile, error) {
	publicProfile, err := s.api.serverProfile(ctx, serverProfileOptions{})
	if err != nil {
		return nil, err
	}
	profile := &apiv1.ServerProfile{PublicProfile: publicProfile}
	if cm := s.api.core.ConfigManager(); cm != nil {
		motd, err := cm.GetEffectiveMOTD(ctx)
		if err != nil {
			return nil, connectError(err)
		}
		if motd != "" {
			profile.Motd = stringPtr(motd)
		}
	}
	return profile, nil
}

func (a *API) serverViewerState(ctx context.Context, userID string) (*apiv1.ServerViewerPermissions, *apiv1.ServerViewerState, error) {
	hasUnreadRooms, err := a.viewerHasUnreadRooms(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	permissions := &apiv1.ServerViewerPermissions{
		Permissions: make([]*apiv1.PermissionGrant, 0, len(core.AllPermissions())),
	}
	for _, meta := range core.AllPermissions() {
		granted, err := a.core.HasUserPermissionViaRoles(ctx, userID, meta.Permission)
		if err != nil {
			return nil, nil, connectError(err)
		}
		permissions.Permissions = append(permissions.Permissions, &apiv1.PermissionGrant{
			Permission: string(meta.Permission),
			Granted:    granted,
		})
	}

	return permissions, &apiv1.ServerViewerState{HasUnreadRooms: hasUnreadRooms}, nil
}

func (a *API) viewerHasUnreadRooms(ctx context.Context, userID string) (bool, error) {
	rooms, err := a.core.ListMemberRooms(ctx, core.KindChannel, userID, core.MemberRoomListOptions{})
	if err != nil {
		return false, connectError(err)
	}
	for _, room := range rooms {
		hasUnread, err := a.core.HasUnread(ctx, core.KindChannel, userID, room.GetId())
		if err != nil {
			continue
		}
		if hasUnread {
			return true, nil
		}
	}
	return false, nil
}
