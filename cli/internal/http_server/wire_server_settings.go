package http_server

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireGetServerSettings(ctx context.Context, userID, requestID string) (*apiv1.GetServerSettingsResponse, *wirev1.WireError) {
	settings, err := c.serverSettingsView(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.GetServerSettingsResponse{Settings: settings}, nil
}

func (c *wireConn) handleWireUpdateServerSettings(ctx context.Context, userID, requestID string, body *apiv1.UpdateServerSettingsRequest) (*apiv1.UpdateServerSettingsResponse, *wirev1.WireError) {
	if body == nil || (body.ServerName == nil && body.Description == nil && body.Motd == nil && body.WelcomeMessage == nil) {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: at least one server setting must be provided", errWireInvalidArgument))
	}

	canManage, err := c.server.core.CanManageServer(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !canManage {
		return nil, c.errorFromRequestErr(requestID, core.ErrPermissionDenied)
	}

	configMgr := c.server.core.ConfigManager()
	if configMgr == nil {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("config manager is not configured"))
	}
	if _, err := configMgr.UpdateServerConfigFunc(ctx, userID, func(current *configv1.ServerConfig) (*configv1.ServerConfig, error) {
		cfg := &configv1.ServerConfig{}
		if current != nil {
			cfg = current
		}
		if body.ServerName != nil {
			cfg.ServerName = body.GetServerName()
		}
		if body.Description != nil {
			cfg.Description = body.GetDescription()
		}
		if body.Motd != nil {
			cfg.Motd = body.GetMotd()
		}
		if body.WelcomeMessage != nil {
			cfg.WelcomeMessage = body.GetWelcomeMessage()
		}
		return cfg, nil
	}); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	c.server.core.PublishServerUpdated(ctx, userID)

	settings, err := c.serverSettingsView(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.UpdateServerSettingsResponse{Settings: settings}, nil
}

func (c *wireConn) handleWireGetAdminSecurityConfig(ctx context.Context, userID, requestID string) (*apiv1.GetAdminSecurityConfigResponse, *wirev1.WireError) {
	if err := c.requireWireServerManage(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	config, err := c.adminSecurityConfigView(ctx)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.GetAdminSecurityConfigResponse{Config: config}, nil
}

func (c *wireConn) handleWireUpdateBlockedUsernames(ctx context.Context, userID, requestID string, body *apiv1.UpdateBlockedUsernamesRequest) (*apiv1.UpdateBlockedUsernamesResponse, *wirev1.WireError) {
	if err := c.requireWireServerManage(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	configMgr := c.server.core.ConfigManager()
	if configMgr == nil {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("config manager is not configured"))
	}
	if _, err := configMgr.UpdateServerConfigFunc(ctx, userID, func(current *configv1.ServerConfig) (*configv1.ServerConfig, error) {
		cfg := &configv1.ServerConfig{}
		if current != nil {
			cfg = current
		}
		cfg.BlockedUsernames = body.GetBlockedUsernames()
		return cfg, nil
	}); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	config, err := c.adminSecurityConfigView(ctx)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.UpdateBlockedUsernamesResponse{Config: config}, nil
}

func (c *wireConn) serverSettingsView(ctx context.Context, userID string) (*apiv1.ServerSettingsView, error) {
	view := &apiv1.ServerSettingsView{Name: "Chatto"}

	var err error
	if view.ViewerCanManageServer, err = c.server.core.CanManageServer(ctx, userID); err != nil {
		return nil, err
	}

	if configMgr := c.server.core.ConfigManager(); configMgr != nil {
		name, err := configMgr.GetEffectiveServerName(ctx)
		if err != nil {
			return nil, err
		}
		view.Name = name

		cfg, err := configMgr.GetServerConfig(ctx)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			view.Description = cfg.GetDescription()
		}

		motd, err := configMgr.GetEffectiveMOTD(ctx)
		if err != nil {
			return nil, err
		}
		view.Motd = motd

		welcomeMessage, err := configMgr.GetEffectiveWelcomeMessage(ctx)
		if err != nil {
			return nil, err
		}
		view.WelcomeMessage = welcomeMessage
	}

	logoURL, err := c.server.core.GetServerLogoURL(ctx, nil, nil, "")
	if err != nil {
		return nil, err
	}
	view.LogoUrl = logoURL

	bannerURL, err := c.server.core.GetServerBannerURL(ctx, nil, nil, "")
	if err != nil {
		return nil, err
	}
	view.BannerUrl = bannerURL

	return view, nil
}

func (c *wireConn) adminSecurityConfigView(ctx context.Context) (*apiv1.AdminSecurityConfigView, error) {
	configMgr := c.server.core.ConfigManager()
	if configMgr == nil {
		return nil, fmt.Errorf("config manager is not configured")
	}
	blockedUsernames, err := configMgr.GetEffectiveBlockedUsernames(ctx)
	if err != nil {
		return nil, err
	}
	return &apiv1.AdminSecurityConfigView{BlockedUsernames: blockedUsernames}, nil
}

func (c *wireConn) requireWireServerManage(ctx context.Context, userID string) error {
	canManage, err := c.server.core.CanManageServer(ctx, userID)
	if err != nil {
		return err
	}
	if !canManage {
		return core.ErrPermissionDenied
	}
	return nil
}
