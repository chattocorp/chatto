package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ErrConfigConflict is returned when a config update fails due to
// concurrent modification. The OCC scope is the EVT event publish;
// ErrConfigConflict surfaces when retries on the publish path exhaust
// without success. Callers can retry the whole UpdateServerConfigFunc
// call.
var ErrConfigConflict = errors.New("config was modified by another request")

// ConfigManager handles runtime server configuration.
//
// ADR-035 phase 6: writes are event-only (publish to EVT +
// WaitForSeq for read-your-writes). Reads come from the in-memory
// ServerConfigProjection. The legacy INSTANCE_CONFIG KV bucket is
// retained as pre-ES import evidence for MigrateServerConfigToES, but
// is not written by this code anymore.
type ConfigManager struct {
	publisher  *events.Publisher
	projector  *events.Projector
	projection *ServerConfigProjection
}

// NewConfigManager creates a new ConfigManager. publisher / projector /
// projection are required for event-only writes and projection-backed
// reads.
func NewConfigManager(
	publisher *events.Publisher,
	projector *events.Projector,
	projection *ServerConfigProjection,
) *ConfigManager {
	return &ConfigManager{
		publisher:  publisher,
		projector:  projector,
		projection: projection,
	}
}

// =============================================================================
// Instance Config
// =============================================================================

// GetServerConfig returns the current server configuration from the
// projection. The second return value indicates whether a
// ServerConfigChangedEvent has ever applied — i.e. whether the
// projection holds a real snapshot vs. a cold "no config yet" state.
// The error return is preserved for signature compatibility; the
// projection is in-memory and cannot fail to read.
func (cm *ConfigManager) GetServerConfig(_ context.Context) (*configv1.ServerConfig, bool, error) {
	if cm.projection == nil {
		return nil, false, nil
	}
	cfg, isConfigured := cm.projection.Get()
	return cfg, isConfigured, nil
}

// SetServerConfig stores the server configuration by publishing a
// ServerConfigChangedEvent and waiting for the projection to apply.
//
// Deprecated for runtime callers — they should use UpdateServerConfigFunc
// to compose against the current state. SetServerConfig is kept for
// migration code and tests that bypass the compose step.
func (cm *ConfigManager) SetServerConfig(ctx context.Context, actorID string, cfg *configv1.ServerConfig) error {
	return cm.publish(ctx, actorID, cfg)
}

// UpdateServerConfigFunc atomically updates the server config using
// optimistic concurrency control. The updateFn receives the current
// projection snapshot (or nil if no config has been written yet) and
// should return the updated config. OCC retries live inside the
// publisher's Append loop — if the underlying publish exhausts retries,
// this returns ErrConfigConflict. Returns the final config after a
// successful write.
func (cm *ConfigManager) UpdateServerConfigFunc(
	ctx context.Context,
	actorID string,
	updateFn func(current *configv1.ServerConfig) (*configv1.ServerConfig, error),
) (*configv1.ServerConfig, error) {
	// Read current projection state for the compose step. Clone is
	// already returned by Projection.Get so updateFn may mutate the
	// input freely.
	current, _ := cm.projection.Get()

	updated, err := updateFn(current)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, fmt.Errorf("update function returned nil config")
	}

	if err := cm.publish(ctx, actorID, updated); err != nil {
		if errors.Is(err, events.ErrConflict) {
			return nil, ErrConfigConflict
		}
		return nil, err
	}
	return updated, nil
}

// publish writes the server config by emitting a
// ServerConfigChangedEvent on the config aggregate and waiting for the
// projection to apply, giving the caller read-your-writes. OCC + retry
// live inside publisher.Append.
func (cm *ConfigManager) publish(ctx context.Context, actorID string, cfg *configv1.ServerConfig) error {
	if cm.publisher == nil || cm.projector == nil {
		return fmt.Errorf("config manager: event publisher/projector not configured")
	}

	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_ServerConfigChanged{
			ServerConfigChanged: &corev1.ServerConfigChangedEvent{
				Config: cfg,
			},
		},
	})

	if _, err := cm.projector.AppendAndWait(ctx, cm.publisher, events.ConfigAggregate(), event); err != nil {
		return fmt.Errorf("publish ServerConfigChangedEvent: %w", err)
	}
	return nil
}

// =============================================================================
// Effective accessors — all read from the projection.
// =============================================================================

// GetEffectiveWelcomeMessage returns the welcome message from the
// projection. Empty string if not configured.
func (cm *ConfigManager) GetEffectiveWelcomeMessage(_ context.Context) (string, error) {
	if cm.projection == nil {
		return "", nil
	}
	return cm.projection.EffectiveWelcomeMessage(), nil
}

// GetEffectiveServerName returns the server name from the projection,
// falling back to "Chatto" if unset.
func (cm *ConfigManager) GetEffectiveServerName(_ context.Context) (string, error) {
	if cm.projection == nil {
		return "Chatto", nil
	}
	return cm.projection.EffectiveServerName(), nil
}

// GetEffectiveMOTD returns the Message of the Day from the projection.
// Empty string if not configured.
func (cm *ConfigManager) GetEffectiveMOTD(_ context.Context) (string, error) {
	if cm.projection == nil {
		return "", nil
	}
	return cm.projection.EffectiveMOTD(), nil
}

// DefaultDescription is the fallback server description used when no
// admin-configured description is set. Surfaced via OG meta tags and the
// /api/server discovery endpoint.
const DefaultDescription = "Come join our community!"

// GetEffectiveDescription returns the server description from the
// projection, falling back to DefaultDescription if unset.
func (cm *ConfigManager) GetEffectiveDescription(_ context.Context) (string, error) {
	if cm.projection == nil {
		return DefaultDescription, nil
	}
	return cm.projection.EffectiveDescription(), nil
}

// =============================================================================
// Blocked Usernames
// =============================================================================

// DefaultBlockedUsernames is the default list of blocked usernames for
// new servers (used when no config has been written yet).
const DefaultBlockedUsernames = "root\nadmin\nsuperuser\nop\noperator\nsupport"

// GetEffectiveBlockedUsernames returns the blocked usernames string
// from the projection. Returns DefaultBlockedUsernames if no config has
// ever been written; returns "" if the operator explicitly cleared it.
func (cm *ConfigManager) GetEffectiveBlockedUsernames(_ context.Context) (string, error) {
	if cm.projection == nil {
		return DefaultBlockedUsernames, nil
	}
	return cm.projection.EffectiveBlockedUsernames(), nil
}

// GetBlockedUsernamesList returns the blocked usernames as a slice of
// lowercase strings.
func (cm *ConfigManager) GetBlockedUsernamesList(ctx context.Context) ([]string, error) {
	raw, err := cm.GetEffectiveBlockedUsernames(ctx)
	if err != nil {
		return nil, err
	}
	return parseBlockedUsernames(raw), nil
}

// IsUsernameBlocked checks if a username is in the blocked list
// (case-insensitive).
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

// parseBlockedUsernames parses a newline-separated string into a slice
// of lowercase strings. Empty lines are ignored.
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
