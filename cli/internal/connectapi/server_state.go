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
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	profile, err := s.serverProfile(ctx)
	if err != nil {
		return nil, err
	}
	capabilities, err := s.serverViewerCapabilities(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}

	maxUploadSize := s.api.core.AssetsConfig().MaxUploadSize
	maxVideoUploadSize := maxUploadSize
	if s.api.config.Video.Enabled {
		maxVideoUploadSize = int64(s.api.config.Video.MaxUploadSizeOrDefault())
	}
	response := &apiv1.GetServerStateResponse{
		Profile:                   profile,
		PushNotificationsEnabled:  s.api.config.Push.IsConfigured(),
		DirectRegistrationEnabled: s.api.config.Auth.DirectRegistrationOrDefault(),
		VideoProcessingEnabled:    s.api.config.Video.Enabled,
		MaxUploadSize:             maxUploadSize,
		MaxVideoUploadSize:        maxVideoUploadSize,
		MessageEditWindowSeconds:  int32(core.MessageEditWindow / time.Second),
		ViewerCapabilities:        capabilities,
	}
	if s.api.config.Push.IsConfigured() {
		response.VapidPublicKey = stringPtr(s.api.config.Push.VAPIDPublicKey)
	}
	if s.api.config.LiveKit.IsConfigured() {
		response.LivekitUrl = stringPtr(s.api.config.LiveKit.URL)
	}

	return connect.NewResponse(response), nil
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

	return connect.NewResponse(&adminv1.UpdateServerConfigResponse{
		Profile: s.serverProfileFromConfig(ctx, cfg),
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

func (s *serverService) serverProfile(ctx context.Context) (*apiv1.ServerProfile, error) {
	profile := &apiv1.ServerProfile{Name: s.api.effectiveServerName(ctx)}

	if cm := s.api.core.ConfigManager(); cm != nil {
		if welcome, err := cm.GetEffectiveWelcomeMessage(ctx); err != nil {
			return nil, connectError(err)
		} else if welcome != "" {
			profile.WelcomeMessage = stringPtr(welcome)
		}
		if motd, err := cm.GetEffectiveMOTD(ctx); err != nil {
			return nil, connectError(err)
		} else if motd != "" {
			profile.Motd = stringPtr(motd)
		}
		if cfg, err := cm.GetServerConfig(ctx); err != nil {
			return nil, connectError(err)
		} else if cfg != nil && cfg.GetDescription() != "" {
			profile.Description = stringPtr(cfg.GetDescription())
		}
	}

	bw, bh := 1200, 630
	if u, err := s.api.core.GetServerBannerURL(ctx, &bw, &bh, "cover"); err != nil {
		return nil, connectError(err)
	} else if u != "" {
		profile.BannerUrl = stringPtr(s.api.absolutizeAssetURL(ctx, u))
	}
	lw, lh := 256, 256
	if u, err := s.api.core.GetServerLogoURL(ctx, &lw, &lh, "cover"); err != nil {
		return nil, connectError(err)
	} else if u != "" {
		profile.LogoUrl = stringPtr(s.api.absolutizeAssetURL(ctx, u))
	}

	return profile, nil
}

func (s *serverService) serverProfileFromConfig(ctx context.Context, cfg *configv1.ServerConfig) *apiv1.ServerProfile {
	profile := &apiv1.ServerProfile{Name: s.api.effectiveServerName(ctx)}
	if cfg != nil {
		if cfg.GetServerName() != "" {
			profile.Name = cfg.GetServerName()
		}
		if cfg.GetWelcomeMessage() != "" {
			profile.WelcomeMessage = stringPtr(cfg.GetWelcomeMessage())
		}
		if cfg.GetMotd() != "" {
			profile.Motd = stringPtr(cfg.GetMotd())
		}
		if cfg.GetDescription() != "" {
			profile.Description = stringPtr(cfg.GetDescription())
		}
	}

	if u, err := s.api.core.GetServerLogoURL(ctx, nil, nil, ""); err == nil && u != "" {
		profile.LogoUrl = stringPtr(s.api.absolutizeAssetURL(ctx, u))
	}
	if u, err := s.api.core.GetServerBannerURL(ctx, nil, nil, ""); err == nil && u != "" {
		profile.BannerUrl = stringPtr(s.api.absolutizeAssetURL(ctx, u))
	}
	return profile
}

func (s *serverService) serverViewerCapabilities(ctx context.Context, userID string) (*apiv1.ServerViewerCapabilities, error) {
	hasAnyAdminPermission, err := s.api.core.HasAnyAdminPermission(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canManageServer, err := s.api.core.CanManageServer(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canCreateRoom, err := s.api.core.CanCreateRoom(ctx, userID, core.KindChannel, "")
	if err != nil {
		return nil, connectError(err)
	}
	canManageRooms, err := s.api.core.CanManageAnyRoom(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	hasUnreadRooms, err := s.viewerHasUnreadRooms(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &apiv1.ServerViewerCapabilities{
		HasAnyAdminPermission: hasAnyAdminPermission,
		CanManageServer:       canManageServer,
		CanCreateRoom:         canCreateRoom,
		CanManageRooms:        canManageRooms,
		HasUnreadRooms:        hasUnreadRooms,
	}, nil
}

func (s *serverService) viewerHasUnreadRooms(ctx context.Context, userID string) (bool, error) {
	rooms, err := s.api.core.ListMemberRooms(ctx, core.KindChannel, userID, core.MemberRoomListOptions{})
	if err != nil {
		return false, connectError(err)
	}
	for _, room := range rooms {
		hasUnread, err := s.api.core.HasUnread(ctx, core.KindChannel, userID, room.GetId())
		if err != nil {
			continue
		}
		if hasUnread {
			return true, nil
		}
	}
	return false, nil
}
