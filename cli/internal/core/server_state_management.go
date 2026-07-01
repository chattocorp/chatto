package core

import (
	"context"
	"fmt"
	"io"
	"strings"

	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type ServerConfigUpdateInput struct {
	ServerName     *string
	Description    *string
	MOTD           *string
	WelcomeMessage *string
}

func (c *ChattoCore) GetManagedServerConfig(ctx context.Context, actorID string) (*configv1.ServerConfig, error) {
	if err := c.requireCanManageServer(ctx, actorID); err != nil {
		return nil, err
	}

	cfg, err := c.ConfigManager().GetServerConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &configv1.ServerConfig{}, nil
	}
	return cloneServerConfig(cfg), nil
}

func (c *ChattoCore) UpdateServerConfig(ctx context.Context, actorID string, input ServerConfigUpdateInput) (*configv1.ServerConfig, error) {
	if err := c.requireCanManageServer(ctx, actorID); err != nil {
		return nil, err
	}

	cfg, err := c.ConfigManager().UpdateServerConfigFunc(ctx, actorID, func(current *configv1.ServerConfig) (*configv1.ServerConfig, error) {
		cfg := &configv1.ServerConfig{}
		if current != nil {
			cfg = current
		}
		if input.ServerName != nil {
			cfg.ServerName = *input.ServerName
		}
		if input.Description != nil {
			cfg.Description = *input.Description
		}
		if input.MOTD != nil {
			cfg.Motd = *input.MOTD
		}
		if input.WelcomeMessage != nil {
			cfg.WelcomeMessage = *input.WelcomeMessage
		}
		return cfg, nil
	})
	if err != nil {
		return nil, err
	}

	c.PublishServerUpdated(ctx, actorID)
	return cfg, nil
}

func (c *ChattoCore) GetServerSecurityConfig(ctx context.Context, actorID string) ([]string, error) {
	if err := c.requireCanManageServer(ctx, actorID); err != nil {
		return nil, err
	}
	return c.ConfigManager().GetBlockedUsernamesList(ctx)
}

func (c *ChattoCore) UpdateBlockedUsernames(ctx context.Context, actorID string, blockedUsernames []string) ([]string, error) {
	if err := c.requireCanManageServer(ctx, actorID); err != nil {
		return nil, err
	}

	configMgr := c.ConfigManager()
	normalized := normalizeBlockedUsernameEntries(blockedUsernames)
	if _, err := configMgr.UpdateServerConfigFunc(ctx, actorID, func(current *configv1.ServerConfig) (*configv1.ServerConfig, error) {
		cfg := &configv1.ServerConfig{}
		if current != nil {
			cfg = current
		}
		cfg.BlockedUsernames = normalized
		return cfg, nil
	}); err != nil {
		return nil, err
	}

	return configMgr.GetBlockedUsernamesList(ctx)
}

func normalizeBlockedUsernameEntries(entries []string) string {
	if len(entries) == 0 {
		return ""
	}
	return strings.Join(parseBlockedUsernames(strings.Join(entries, "\n")), "\n")
}

func (c *ChattoCore) UploadManagedServerLogo(ctx context.Context, actorID string, reader io.Reader) (*corev1.AssetRecord, error) {
	if err := c.requireCanManageServer(ctx, actorID); err != nil {
		return nil, err
	}
	asset, err := c.UploadServerLogo(ctx, reader)
	if err != nil {
		return nil, err
	}
	if err := c.SetServerLogo(ctx, actorID, asset); err != nil {
		c.CleanupAsset(ctx, DeprecatedAssetFromAsset(asset))
		return nil, err
	}
	return asset, nil
}

func (c *ChattoCore) UploadManagedServerBanner(ctx context.Context, actorID string, reader io.Reader) (*corev1.AssetRecord, error) {
	if err := c.requireCanManageServer(ctx, actorID); err != nil {
		return nil, err
	}
	asset, err := c.UploadServerBanner(ctx, reader)
	if err != nil {
		return nil, err
	}
	if err := c.SetServerBanner(ctx, actorID, asset); err != nil {
		c.CleanupAsset(ctx, DeprecatedAssetFromAsset(asset))
		return nil, err
	}
	return asset, nil
}

func (c *ChattoCore) DeleteManagedServerLogo(ctx context.Context, actorID string) error {
	if err := c.requireCanManageServer(ctx, actorID); err != nil {
		return err
	}
	return c.DeleteServerLogo(ctx, actorID)
}

func (c *ChattoCore) DeleteManagedServerBanner(ctx context.Context, actorID string) error {
	if err := c.requireCanManageServer(ctx, actorID); err != nil {
		return err
	}
	return c.DeleteServerBanner(ctx, actorID)
}

func (c *ChattoCore) requireCanManageServer(ctx context.Context, actorID string) error {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return err
	}
	canManage, err := c.CanManageServer(ctx, actorID)
	if err != nil {
		return fmt.Errorf("check server.manage: %w", err)
	}
	if !canManage {
		return ErrPermissionDenied
	}
	return nil
}
