package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
)

// ErrConfigConflict is returned when a config update fails due to concurrent modification.
var ErrConfigConflict = errors.New("config was modified by another request")

// ConfigManager handles runtime configuration stored in NATS KV.
// Instance configuration lives entirely in KV, not in chatto.toml.
type ConfigManager struct {
	kv jetstream.KeyValue
}

// NewConfigManager creates a new ConfigManager.
func NewConfigManager(kv jetstream.KeyValue) *ConfigManager {
	return &ConfigManager{
		kv: kv,
	}
}

// KV key constants for config sections
const (
	configKeyInstance = "config.instance"
)

// =============================================================================
// Instance Config
// =============================================================================

// GetInstanceConfig retrieves the instance configuration from KV.
// Returns (config, isConfigured, error) where isConfigured indicates if KV value exists.
func (cm *ConfigManager) GetInstanceConfig(ctx context.Context) (*configv1.ServerConfig, bool, error) {
	entry, err := cm.kv.Get(ctx, configKeyInstance)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get instance config: %w", err)
	}

	cfg := &configv1.ServerConfig{}
	if err := proto.Unmarshal(entry.Value(), cfg); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal instance config: %w", err)
	}

	return cfg, true, nil
}

// SetInstanceConfig stores the instance configuration in KV.
// Deprecated: Use UpdateInstanceConfigFunc for concurrent-safe updates.
func (cm *ConfigManager) SetInstanceConfig(ctx context.Context, cfg *configv1.ServerConfig) error {
	data, err := proto.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal instance config: %w", err)
	}

	_, err = cm.kv.Put(ctx, configKeyInstance, data)
	if err != nil {
		return fmt.Errorf("failed to store instance config: %w", err)
	}

	return nil
}

// maxConfigRetries is the maximum number of retry attempts for OCC conflicts.
const maxConfigRetries = 5

// UpdateInstanceConfigFunc atomically updates the instance config using optimistic concurrency control.
// The updateFn receives the current config (or nil if not configured) and should return the updated config.
// If another concurrent update occurs, this will retry up to maxConfigRetries times.
// Returns the final config after successful update.
func (cm *ConfigManager) UpdateInstanceConfigFunc(ctx context.Context, updateFn func(current *configv1.ServerConfig) (*configv1.ServerConfig, error)) (*configv1.ServerConfig, error) {
	for attempt := 0; attempt < maxConfigRetries; attempt++ {
		// Get current entry to obtain revision
		entry, err := cm.kv.Get(ctx, configKeyInstance)

		var currentCfg *configv1.ServerConfig
		var revision uint64

		if err != nil {
			if !errors.Is(err, jetstream.ErrKeyNotFound) {
				return nil, fmt.Errorf("failed to get instance config: %w", err)
			}
			// Key doesn't exist - will use Create
			currentCfg = nil
			revision = 0
		} else {
			// Key exists - unmarshal and get revision
			currentCfg = &configv1.ServerConfig{}
			if err := proto.Unmarshal(entry.Value(), currentCfg); err != nil {
				return nil, fmt.Errorf("failed to unmarshal instance config: %w", err)
			}
			revision = entry.Revision()
		}

		// Apply the update function
		updatedCfg, err := updateFn(currentCfg)
		if err != nil {
			return nil, err
		}

		// Marshal the updated config
		data, err := proto.Marshal(updatedCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal instance config: %w", err)
		}

		// Attempt atomic update
		if revision == 0 {
			// Key doesn't exist - use Create
			_, err = cm.kv.Create(ctx, configKeyInstance, data)
		} else {
			// Key exists - use Update with revision
			_, err = cm.kv.Update(ctx, configKeyInstance, data, revision)
		}

		if err == nil {
			// Success
			return updatedCfg, nil
		}

		// Check if it's a conflict error (key exists when creating, or wrong revision when updating)
		// ErrKeyExists is used for both cases in the JetStream KV API
		if errors.Is(err, jetstream.ErrKeyExists) {
			// Retry
			continue
		}

		// Other error - fail immediately
		return nil, fmt.Errorf("failed to store instance config: %w", err)
	}

	return nil, ErrConfigConflict
}

// ResetInstanceConfig removes the instance configuration from KV.
func (cm *ConfigManager) ResetInstanceConfig(ctx context.Context) error {
	err := cm.kv.Delete(ctx, configKeyInstance)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to reset instance config: %w", err)
	}
	return nil
}

// GetEffectiveWelcomeMessage returns the welcome message from instance config.
// Returns empty string if not configured.
func (cm *ConfigManager) GetEffectiveWelcomeMessage(ctx context.Context) (string, error) {
	cfg, _, err := cm.GetInstanceConfig(ctx)
	if err != nil {
		return "", err
	}
	if cfg != nil {
		return cfg.WelcomeMessage, nil
	}
	return "", nil
}

// GetEffectiveInstanceName returns the instance name from config.
// Returns "Chatto" as default if not configured.
func (cm *ConfigManager) GetEffectiveInstanceName(ctx context.Context) (string, error) {
	cfg, _, err := cm.GetInstanceConfig(ctx)
	if err != nil {
		return "", err
	}
	if cfg != nil && cfg.ServerName != "" {
		return cfg.ServerName, nil
	}
	return "Chatto", nil
}

// GetEffectiveMOTD returns the Message of the Day from config.
// Returns empty string if not configured.
func (cm *ConfigManager) GetEffectiveMOTD(ctx context.Context) (string, error) {
	cfg, _, err := cm.GetInstanceConfig(ctx)
	if err != nil {
		return "", err
	}
	if cfg != nil {
		return cfg.Motd, nil
	}
	return "", nil
}

// DefaultDescription is the fallback server description used when no
// admin-configured description is set. Surfaced via OG meta tags and the
// /api/server discovery endpoint.
const DefaultDescription = "Come join our community!"

// GetEffectiveDescription returns the server description from config,
// falling back to DefaultDescription if unset.
func (cm *ConfigManager) GetEffectiveDescription(ctx context.Context) (string, error) {
	cfg, _, err := cm.GetInstanceConfig(ctx)
	if err != nil {
		return "", err
	}
	if cfg != nil && cfg.Description != "" {
		return cfg.Description, nil
	}
	return DefaultDescription, nil
}

// =============================================================================
// Blocked Usernames
// =============================================================================

// DefaultBlockedUsernames is the default list of blocked usernames for new instances.
const DefaultBlockedUsernames = "root\nadmin\nsuperuser\nop\noperator\nsupport"

// GetEffectiveBlockedUsernames returns the blocked usernames string from config.
// Returns DefaultBlockedUsernames if not configured.
func (cm *ConfigManager) GetEffectiveBlockedUsernames(ctx context.Context) (string, error) {
	cfg, isConfigured, err := cm.GetInstanceConfig(ctx)
	if err != nil {
		return "", err
	}
	// If not configured at all, return defaults
	if !isConfigured || cfg == nil {
		return DefaultBlockedUsernames, nil
	}
	// If configured but blocked_usernames field is empty, that means admin cleared it
	return cfg.BlockedUsernames, nil
}

// GetBlockedUsernamesList returns the blocked usernames as a slice of lowercase strings.
// Returns the parsed list from config, or the default list if not configured.
func (cm *ConfigManager) GetBlockedUsernamesList(ctx context.Context) ([]string, error) {
	raw, err := cm.GetEffectiveBlockedUsernames(ctx)
	if err != nil {
		return nil, err
	}
	return parseBlockedUsernames(raw), nil
}

// IsUsernameBlocked checks if a username is in the blocked list (case-insensitive).
func (cm *ConfigManager) IsUsernameBlocked(ctx context.Context, login string) (bool, error) {
	blockedList, err := cm.GetBlockedUsernamesList(ctx)
	if err != nil {
		return false, err
	}
	loginLower := strings.ToLower(login)
	for _, blocked := range blockedList {
		if blocked == loginLower {
			return true, nil
		}
	}
	return false, nil
}

// parseBlockedUsernames parses a newline-separated string into a slice of lowercase strings.
// Empty lines are ignored.
func parseBlockedUsernames(raw string) []string {
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, strings.ToLower(trimmed))
		}
	}
	return result
}
