package graph

import (
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
)

// instanceConfigToModel converts a protobuf InstanceConfig to the GraphQL model.
func instanceConfigToModel(cfg *configv1.ServerConfig, isConfigured bool) *model.AdminServerConfig {
	// Default blocked usernames for unconfigured instances
	defaultBlocked := core.DefaultBlockedUsernames

	if cfg == nil {
		return &model.AdminServerConfig{
			IsConfigured:     false,
			ServerName: "Chatto", // Default
			BlockedUsernames: &defaultBlocked,
		}
	}

	instanceName := cfg.ServerName
	if instanceName == "" {
		instanceName = "Chatto" // Default
	}

	var welcomeMessage *string
	if cfg.WelcomeMessage != "" {
		welcomeMessage = &cfg.WelcomeMessage
	}

	var motd *string
	if cfg.Motd != "" {
		motd = &cfg.Motd
	}

	// For blocked usernames: return the configured value (even if empty, meaning admin cleared it)
	// But if not configured at all, return the defaults
	var blockedUsernames *string
	if isConfigured {
		// Return what's in config (even if empty string)
		blockedUsernames = &cfg.BlockedUsernames
	} else {
		// Not configured - return defaults
		blockedUsernames = &defaultBlocked
	}

	var description *string
	if cfg.Description != "" {
		description = &cfg.Description
	}

	return &model.AdminServerConfig{
		IsConfigured:     isConfigured,
		WelcomeMessage:   welcomeMessage,
		ServerName: instanceName,
		Motd:             motd,
		BlockedUsernames: blockedUsernames,
		Description:      description,
	}
}
