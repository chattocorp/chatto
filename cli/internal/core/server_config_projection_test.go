package core

import (
	"testing"

	"github.com/stretchr/testify/require"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func newServerNameChangedEvent(name string) *corev1.Event {
	return &corev1.Event{
		Id: "test-event",
		Event: &corev1.Event_ServerNameChanged{
			ServerNameChanged: &corev1.ServerNameChangedEvent{Name: name},
		},
	}
}

func TestServerConfigProjection_FreshState(t *testing.T) {
	p := NewServerConfigProjection()

	cfg := p.Get()
	require.Nil(t, cfg)

	// Effective accessors fall back to defaults pre-config.
	require.Equal(t, "Chatto", p.EffectiveServerName())
	require.Equal(t, "", p.EffectiveWelcomeMessage())
	require.Equal(t, "", p.EffectiveMOTD())
	require.Equal(t, DefaultDescription, p.EffectiveDescription())
	require.Equal(t, DefaultBlockedUsernames, p.EffectiveBlockedUsernames())
}

func TestServerConfigProjection_AppliesIndependentServerFields(t *testing.T) {
	p := NewServerConfigProjection()

	require.NoError(t, p.Apply(newServerNameChangedEvent("First Server"), 1))
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerWelcomeMessageChanged{
		ServerWelcomeMessageChanged: &corev1.ServerWelcomeMessageChangedEvent{WelcomeMessage: "Welcome!"},
	}}, 2))
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerMotdChanged{
		ServerMotdChanged: &corev1.ServerMotdChangedEvent{Motd: "MOTD-1"},
	}}, 3))

	cfg := p.Get()
	require.NotNil(t, cfg)
	require.Equal(t, "First Server", cfg.ServerName)
	require.Equal(t, "First Server", p.EffectiveServerName())
	require.Equal(t, "Welcome!", p.EffectiveWelcomeMessage())
	require.Equal(t, "MOTD-1", p.EffectiveMOTD())

	require.NoError(t, p.Apply(newServerNameChangedEvent("Second Server"), 4))

	require.Equal(t, "Second Server", p.EffectiveServerName())
	require.Equal(t, "MOTD-1", p.EffectiveMOTD())
	require.Equal(t, "Welcome!", p.EffectiveWelcomeMessage())
}

func TestServerConfigProjection_AppliesSemanticConfigEvents(t *testing.T) {
	p := NewServerConfigProjection()

	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerNameChanged{
		ServerNameChanged: &corev1.ServerNameChangedEvent{Name: "Semantic Server"},
	}}, 1))
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerMotdChanged{
		ServerMotdChanged: &corev1.ServerMotdChangedEvent{Motd: "semantic motd"},
	}}, 2))

	cfg := p.Get()
	require.Equal(t, "Semantic Server", cfg.ServerName)
	require.Equal(t, "semantic motd", cfg.Motd)
	require.Equal(t, "Semantic Server", p.EffectiveServerName())
	require.Equal(t, "semantic motd", p.EffectiveMOTD())

	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerNameChanged{
		ServerNameChanged: &corev1.ServerNameChangedEvent{Name: ""},
	}}, 3))
	cfg = p.Get()
	require.Equal(t, "", cfg.ServerName)
	require.Equal(t, "Chatto", p.EffectiveServerName())
	require.Equal(t, "semantic motd", p.EffectiveMOTD())
}

func TestServerConfigProjection_GetReturnsClone(t *testing.T) {
	p := NewServerConfigProjection()
	require.NoError(t, p.Apply(newServerNameChangedEvent("Original"), 1))

	cfg := p.Get()
	require.Equal(t, "Original", cfg.ServerName)

	// Mutate the returned proto — projection's internal copy must not
	// be affected.
	cfg.ServerName = "Mutated"

	cfg2 := p.Get()
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

	require.Nil(t, p.Get())
}

func TestServerConfigProjection_BrandingDoesNotCreateServerConfig(t *testing.T) {
	p := NewServerConfigProjection()

	logo := &corev1.DeprecatedAsset{Asset: &corev1.DeprecatedAsset_Nats{Nats: &corev1.NATSAsset{Key: "logo-asset"}}}
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerLogoSet{
		ServerLogoSet: &corev1.ServerLogoSetEvent{Asset: logo},
	}}, 1))
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerBannerCleared{
		ServerBannerCleared: &corev1.ServerBannerClearedEvent{},
	}}, 2))

	cfg := p.Get()
	require.Nil(t, cfg)
	require.Equal(t, DefaultBlockedUsernames, p.EffectiveBlockedUsernames())
}

func TestServerConfigProjection_BlockedUsernames(t *testing.T) {
	p := NewServerConfigProjection()

	// Before any config: defaults apply.
	require.True(t, p.IsUsernameBlocked("admin"))
	require.False(t, p.IsUsernameBlocked("alice"))

	// Operator sets a custom list.
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerBlockedUsernamesChanged{
		ServerBlockedUsernamesChanged: &corev1.ServerBlockedUsernamesChangedEvent{BlockedUsernames: "foo\nBAR\nbaz"},
	}}, 1))

	require.True(t, p.IsUsernameBlocked("foo"))
	require.True(t, p.IsUsernameBlocked("bar"))
	require.True(t, p.IsUsernameBlocked("BAR"))
	require.False(t, p.IsUsernameBlocked("admin"))

	// Operator explicitly clears the list. Once there is a blocked-username
	// event, an empty string is meaningful and should not fall back to defaults.
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerBlockedUsernamesChanged{
		ServerBlockedUsernamesChanged: &corev1.ServerBlockedUsernamesChangedEvent{BlockedUsernames: ""},
	}}, 2))

	require.False(t, p.IsUsernameBlocked("foo"))
	require.False(t, p.IsUsernameBlocked("admin"))
	require.Equal(t, "", p.EffectiveBlockedUsernames())
}
