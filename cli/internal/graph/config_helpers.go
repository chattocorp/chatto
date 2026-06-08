package graph

import (
	"hmans.de/chatto/internal/graph/model"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
)

// serverConfigToModel converts a protobuf ServerConfig to the GraphQL model.
func serverConfigToModel(cfg *configv1.ServerConfig, blockedUsernames string) *model.AdminServerConfig {
	if cfg == nil {
		return &model.AdminServerConfig{
			ServerName:       "Chatto",
			BlockedUsernames: &blockedUsernames,
		}
	}

	serverName := cfg.ServerName
	if serverName == "" {
		serverName = "Chatto" // Default
	}

	var welcomeMessage *string
	if cfg.WelcomeMessage != "" {
		welcomeMessage = &cfg.WelcomeMessage
	}

	var motd *string
	if cfg.Motd != "" {
		motd = &cfg.Motd
	}

	var description *string
	if cfg.Description != "" {
		description = &cfg.Description
	}

	return &model.AdminServerConfig{
		WelcomeMessage:   welcomeMessage,
		ServerName:       serverName,
		Motd:             motd,
		BlockedUsernames: &blockedUsernames,
		Description:      description,
	}
}

// publicServerConfigToModel converts a protobuf ServerConfig to the public
// GraphQL ServerProfile shape, excluding admin-only fields.
func publicServerConfigToModel(cfg *configv1.ServerConfig) *model.ServerProfile {
	if cfg == nil {
		return &model.ServerProfile{Name: "Chatto"}
	}

	name := cfg.ServerName
	if name == "" {
		name = "Chatto"
	}

	var welcomeMessage *string
	if cfg.WelcomeMessage != "" {
		welcomeMessage = &cfg.WelcomeMessage
	}

	var motd *string
	if cfg.Motd != "" {
		motd = &cfg.Motd
	}

	var description *string
	if cfg.Description != "" {
		description = &cfg.Description
	}

	return &model.ServerProfile{
		Name:           name,
		WelcomeMessage: welcomeMessage,
		Motd:           motd,
		Description:    description,
	}
}
