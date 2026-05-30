package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

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

const maxConfigUpdateRetries = 5

// ConfigManager handles runtime server configuration.
//
// ADR-035 phase 6: writes are event-only (publish to EVT +
// WaitForSeq for read-your-writes). Reads come from the in-memory
// ConfigProjection. The legacy INSTANCE_CONFIG KV bucket is
// retained as pre-ES import evidence for MigrateServerConfigToES, but
// is not written by this code anymore.
type ConfigManager struct {
	service    *ConfigService
	projection *ConfigProjection
}

// NewConfigManager creates a server-config compatibility facade over the
// semantic ConfigService / ConfigProjection.
func NewConfigManager(
	service *ConfigService,
	projection *ConfigProjection,
) *ConfigManager {
	return &ConfigManager{
		service:    service,
		projection: projection,
	}
}

// =============================================================================
// Instance Config
// =============================================================================

// GetServerConfig returns the raw server configuration values currently held
// by the projection, or nil when no server config fields have been set.
// The error return is preserved for signature compatibility; the
// projection is in-memory and cannot fail to read.
func (cm *ConfigManager) GetServerConfig(_ context.Context) (*configv1.ServerConfig, error) {
	if cm.projection == nil {
		return nil, nil
	}
	return cm.projection.Get(), nil
}

// SetServerConfig stores the server configuration by publishing semantic config
// events and waiting for the projection to apply.
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
// should return the updated config. On conflict, the whole compose step
// is retried against the newer projection snapshot, so field-level edits
// do not publish stale full-config replacements.
func (cm *ConfigManager) UpdateServerConfigFunc(
	ctx context.Context,
	actorID string,
	updateFn func(current *configv1.ServerConfig) (*configv1.ServerConfig, error),
) (*configv1.ServerConfig, error) {
	if cm.service == nil {
		return nil, fmt.Errorf("config manager: event publisher/projector not configured")
	}

	for attempt := 0; attempt < maxConfigUpdateRetries; attempt++ {
		agg, filter, expectedSeq, err := cm.service.prepareSubject(ctx, ConfigSubjectServer)
		if err != nil {
			return nil, err
		}
		baseline := cm.effectiveConfigForUpdate()
		updated, err := updateFn(cloneServerConfig(baseline))
		if err != nil {
			return nil, err
		}
		if updated == nil {
			return nil, fmt.Errorf("update function returned nil config")
		}

		err = cm.service.appendEventsAt(ctx, agg, filter, expectedSeq, serverConfigEvents(actorID, baseline, updated))
		if err == nil {
			return updated, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return nil, err
		}
	}
	return nil, ErrConfigConflict
}

// publish writes the server config by emitting semantic config events on the
// config aggregate and waiting for the projection to apply, giving the caller
// read-your-writes.
func (cm *ConfigManager) publish(ctx context.Context, actorID string, cfg *configv1.ServerConfig) error {
	if cm.service == nil {
		return fmt.Errorf("config manager: event publisher/projector not configured")
	}

	return cm.service.updateSubject(ctx, ConfigSubjectServer, func(_ events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		return serverConfigEvents(actorID, cm.effectiveConfigForUpdate(), cfg), nil
	})
}

func (cm *ConfigManager) effectiveConfigForUpdate() *configv1.ServerConfig {
	cfg := cloneServerConfig(cm.projection.Get())
	if cfg == nil {
		cfg = &configv1.ServerConfig{}
	}
	cfg.BlockedUsernames = cm.projection.EffectiveBlockedUsernames()
	return cfg
}

func cloneServerConfig(cfg *configv1.ServerConfig) *configv1.ServerConfig {
	if cfg == nil {
		return nil
	}
	return proto.Clone(cfg).(*configv1.ServerConfig)
}

func serverConfigEvents(actorID string, current, next *configv1.ServerConfig) []*corev1.Event {
	if next == nil {
		next = &configv1.ServerConfig{}
	}
	var evs []*corev1.Event
	if current.GetServerName() != next.GetServerName() {
		evs = append(evs, newEvent(actorID, &corev1.Event{Event: &corev1.Event_ServerNameChanged{
			ServerNameChanged: &corev1.ServerNameChangedEvent{Name: next.GetServerName()},
		}}))
	}
	if current.GetDescription() != next.GetDescription() {
		evs = append(evs, newEvent(actorID, &corev1.Event{Event: &corev1.Event_ServerDescriptionChanged{
			ServerDescriptionChanged: &corev1.ServerDescriptionChangedEvent{Description: next.GetDescription()},
		}}))
	}
	if current.GetWelcomeMessage() != next.GetWelcomeMessage() {
		evs = append(evs, newEvent(actorID, &corev1.Event{Event: &corev1.Event_ServerWelcomeMessageChanged{
			ServerWelcomeMessageChanged: &corev1.ServerWelcomeMessageChangedEvent{WelcomeMessage: next.GetWelcomeMessage()},
		}}))
	}
	if current.GetMotd() != next.GetMotd() {
		evs = append(evs, newEvent(actorID, &corev1.Event{Event: &corev1.Event_ServerMotdChanged{
			ServerMotdChanged: &corev1.ServerMotdChangedEvent{Motd: next.GetMotd()},
		}}))
	}
	if current.GetBlockedUsernames() != next.GetBlockedUsernames() {
		evs = append(evs, newEvent(actorID, &corev1.Event{Event: &corev1.Event_ServerBlockedUsernamesChanged{
			ServerBlockedUsernamesChanged: &corev1.ServerBlockedUsernamesChangedEvent{BlockedUsernames: next.GetBlockedUsernames()},
		}}))
	}
	return evs
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
