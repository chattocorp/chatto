package core

import (
	"strings"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ServerConfigProjection is the second event-sourced projection
// (ADR-033, ADR-035 phase 5 for the config aggregate). It consumes
// ServerConfigChangedEvent on evt.config.> and maintains the
// current server-config snapshot in memory.
//
// "Effective" accessor methods mirror the legacy ConfigManager's
// fallback semantics so resolver/handler call sites swap their reads
// without changing behaviour. The single source of truth is the held
// snapshot — there is no per-field state, only the most recent
// ServerConfig proto.
type ServerConfigProjection struct {
	events.MemoryProjection
	cfg  *configv1.ServerConfig
	seen bool // true once at least one ServerConfigChangedEvent has applied
}

// NewServerConfigProjection returns an empty projection. Call Run on a
// Projector wrapping it to populate from the stream.
func NewServerConfigProjection() *ServerConfigProjection {
	return &ServerConfigProjection{}
}

// Subjects implements events.Projection. Singleton aggregate with one
// event type — the per-(agg, event-type) subject is exact.
func (p *ServerConfigProjection) Subjects() []string {
	return []string{events.ConfigAggregate().Subject(events.EventServerConfigChanged)}
}

// Apply implements events.Projection. ServerConfigChangedEvent replaces
// the held snapshot atomically and flips seen to true. Other event
// types under evt.config.> are ignored (forward-compatibility — future
// granular events can land on the same subject namespace without
// breaking older projections).
func (p *ServerConfigProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	change, ok := event.GetEvent().(*corev1.Event_ServerConfigChanged)
	if !ok {
		return nil
	}
	incoming := change.ServerConfigChanged.GetConfig()
	// Clone the incoming proto so callers reading via accessors can't
	// observe state changing under them mid-read.
	var snapshot *configv1.ServerConfig
	if incoming != nil {
		snapshot = proto.Clone(incoming).(*configv1.ServerConfig)
	}
	p.Lock()
	p.cfg = snapshot
	p.seen = true
	p.Unlock()
	return nil
}

// Get returns the current server config snapshot and a bool indicating
// whether the projection has ever applied a ServerConfigChangedEvent.
// The returned proto is a clone — callers may inspect freely without
// affecting projection state.
//
// isConfigured == false means no config event has been observed yet
// (fresh deployment, projection cold-started before any write); the
// returned *ServerConfig is nil in that case.
func (p *ServerConfigProjection) Get() (cfg *configv1.ServerConfig, isConfigured bool) {
	p.RLock()
	defer p.RUnlock()
	if !p.seen || p.cfg == nil {
		return nil, p.seen
	}
	return proto.Clone(p.cfg).(*configv1.ServerConfig), true
}

// EffectiveServerName returns the configured server name or the default
// fallback ("Chatto") if no name has been set. Matches the legacy
// ConfigManager.GetEffectiveServerName semantics.
func (p *ServerConfigProjection) EffectiveServerName() string {
	p.RLock()
	defer p.RUnlock()
	if p.cfg != nil && p.cfg.ServerName != "" {
		return p.cfg.ServerName
	}
	return "Chatto"
}

// EffectiveWelcomeMessage returns the configured welcome message or "".
func (p *ServerConfigProjection) EffectiveWelcomeMessage() string {
	p.RLock()
	defer p.RUnlock()
	if p.cfg != nil {
		return p.cfg.WelcomeMessage
	}
	return ""
}

// EffectiveMOTD returns the configured MOTD or "".
func (p *ServerConfigProjection) EffectiveMOTD() string {
	p.RLock()
	defer p.RUnlock()
	if p.cfg != nil {
		return p.cfg.Motd
	}
	return ""
}

// EffectiveDescription returns the configured server description or the
// default fallback (DefaultDescription) if unset.
func (p *ServerConfigProjection) EffectiveDescription() string {
	p.RLock()
	defer p.RUnlock()
	if p.cfg != nil && p.cfg.Description != "" {
		return p.cfg.Description
	}
	return DefaultDescription
}

// EffectiveBlockedUsernames returns the configured blocked-usernames
// list, or DefaultBlockedUsernames if no config has ever been written.
// Returns the empty string when the operator has explicitly cleared the
// list (config exists, field is empty) — matching legacy semantics.
func (p *ServerConfigProjection) EffectiveBlockedUsernames() string {
	p.RLock()
	defer p.RUnlock()
	if !p.seen || p.cfg == nil {
		return DefaultBlockedUsernames
	}
	return p.cfg.BlockedUsernames
}

// BlockedUsernamesList returns the blocked usernames as a slice of
// lowercase strings, matching ConfigManager.GetBlockedUsernamesList.
func (p *ServerConfigProjection) BlockedUsernamesList() []string {
	return parseBlockedUsernames(p.EffectiveBlockedUsernames())
}

// IsUsernameBlocked reports whether `login` is in the blocked list
// (case-insensitive).
func (p *ServerConfigProjection) IsUsernameBlocked(login string) bool {
	loginLower := strings.ToLower(login)
	for _, blocked := range p.BlockedUsernamesList() {
		if blocked == loginLower {
			return true
		}
	}
	return false
}
