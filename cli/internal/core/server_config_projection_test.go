package core

import (
	"testing"

	"github.com/stretchr/testify/require"

	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func newConfigChangedEvent(cfg *configv1.ServerConfig) *corev1.Event {
	return &corev1.Event{
		Id: "test-event",
		Event: &corev1.Event_ServerConfigChanged{
			ServerConfigChanged: &corev1.ServerConfigChangedEvent{
				Config: cfg,
			},
		},
	}
}

func TestServerConfigProjection_FreshState(t *testing.T) {
	p := NewServerConfigProjection()

	cfg, configured := p.Get()
	require.False(t, configured)
	require.Nil(t, cfg)

	// Effective accessors fall back to defaults pre-config.
	require.Equal(t, "Chatto", p.EffectiveServerName())
	require.Equal(t, "", p.EffectiveWelcomeMessage())
	require.Equal(t, "", p.EffectiveMOTD())
	require.Equal(t, DefaultDescription, p.EffectiveDescription())
	require.Equal(t, DefaultBlockedUsernames, p.EffectiveBlockedUsernames())
}

func TestServerConfigProjection_ApplyReplacesSnapshot(t *testing.T) {
	p := NewServerConfigProjection()

	first := &configv1.ServerConfig{
		ServerName:     "First Server",
		WelcomeMessage: "Welcome!",
		Motd:           "MOTD-1",
	}
	require.NoError(t, p.Apply(newConfigChangedEvent(first), 1))

	cfg, configured := p.Get()
	require.True(t, configured)
	require.NotNil(t, cfg)
	require.Equal(t, "First Server", cfg.ServerName)
	require.Equal(t, "First Server", p.EffectiveServerName())
	require.Equal(t, "Welcome!", p.EffectiveWelcomeMessage())
	require.Equal(t, "MOTD-1", p.EffectiveMOTD())

	// A subsequent event REPLACES (not merges) — empty fields go back
	// to defaults via the effective accessors.
	second := &configv1.ServerConfig{
		ServerName: "Second Server",
		// MOTD intentionally empty
	}
	require.NoError(t, p.Apply(newConfigChangedEvent(second), 2))

	require.Equal(t, "Second Server", p.EffectiveServerName())
	require.Equal(t, "", p.EffectiveMOTD())
	require.Equal(t, "", p.EffectiveWelcomeMessage())
}

func TestServerConfigProjection_GetReturnsClone(t *testing.T) {
	p := NewServerConfigProjection()
	require.NoError(t, p.Apply(newConfigChangedEvent(&configv1.ServerConfig{ServerName: "Original"}), 1))

	cfg, _ := p.Get()
	require.Equal(t, "Original", cfg.ServerName)

	// Mutate the returned proto — projection's internal copy must not
	// be affected.
	cfg.ServerName = "Mutated"

	cfg2, _ := p.Get()
	require.Equal(t, "Original", cfg2.ServerName)
}

func TestServerConfigProjection_UnknownEventTypesIgnored(t *testing.T) {
	p := NewServerConfigProjection()

	// An unrelated event variant under the same subject namespace
	// must not affect the projection (forward-compatibility).
	other := &corev1.Event{
		Id: "unrelated",
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"},
		},
	}
	require.NoError(t, p.Apply(other, 1))

	_, configured := p.Get()
	require.False(t, configured)
}

func TestServerConfigProjection_BlockedUsernames(t *testing.T) {
	p := NewServerConfigProjection()

	// Before any config: defaults apply.
	require.True(t, p.IsUsernameBlocked("admin"))
	require.False(t, p.IsUsernameBlocked("alice"))

	// Operator sets a custom list.
	require.NoError(t, p.Apply(newConfigChangedEvent(&configv1.ServerConfig{
		BlockedUsernames: "foo\nBAR\nbaz",
	}), 1))

	require.True(t, p.IsUsernameBlocked("foo"))
	require.True(t, p.IsUsernameBlocked("bar"))
	require.True(t, p.IsUsernameBlocked("BAR"))
	require.False(t, p.IsUsernameBlocked("admin"))

	// Operator explicitly clears the list — empty string after a
	// known-configured state should be respected (no fallback).
	require.NoError(t, p.Apply(newConfigChangedEvent(&configv1.ServerConfig{
		BlockedUsernames: "",
	}), 2))

	require.False(t, p.IsUsernameBlocked("foo"))
	require.False(t, p.IsUsernameBlocked("admin"))
	require.Equal(t, "", p.EffectiveBlockedUsernames())
}
