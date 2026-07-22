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

func newConfigProjectionUnderModel() (*ConfigProjection, *ConfigModel) {
	p := NewConfigProjection()
	return p, NewConfigModel(nil, nil, p)
}

func TestConfigProjection_FreshState(t *testing.T) {
	_, model := newConfigProjectionUnderModel()

	cfg := model.GetServerConfig()
	require.Nil(t, cfg)

	// Effective accessors fall back to defaults pre-config.
	require.Equal(t, "Chatto", model.GetEffectiveServerName())
	require.Equal(t, "", model.GetEffectiveWelcomeMessage())
	require.Equal(t, "", model.GetEffectiveMOTD())
	require.Equal(t, DefaultDescription, model.GetEffectiveDescription())
	require.Equal(t, DefaultBlockedUsernames, model.GetEffectiveBlockedUsernames())
}

func TestConfigProjection_AppliesIndependentServerFields(t *testing.T) {
	p, model := newConfigProjectionUnderModel()

	require.NoError(t, p.Apply(newServerNameChangedEvent("First Server"), 1))
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerWelcomeMessageChanged{
		ServerWelcomeMessageChanged: &corev1.ServerWelcomeMessageChangedEvent{WelcomeMessage: "Welcome!"},
	}}, 2))
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerMotdChanged{
		ServerMotdChanged: &corev1.ServerMotdChangedEvent{Motd: "MOTD-1"},
	}}, 3))

	cfg := model.GetServerConfig()
	require.NotNil(t, cfg)
	require.Equal(t, "First Server", cfg.ServerName)
	require.Equal(t, "First Server", model.GetEffectiveServerName())
	require.Equal(t, "Welcome!", model.GetEffectiveWelcomeMessage())
	require.Equal(t, "MOTD-1", model.GetEffectiveMOTD())

	require.NoError(t, p.Apply(newServerNameChangedEvent("Second Server"), 4))

	require.Equal(t, "Second Server", model.GetEffectiveServerName())
	require.Equal(t, "MOTD-1", model.GetEffectiveMOTD())
	require.Equal(t, "Welcome!", model.GetEffectiveWelcomeMessage())
}

func TestConfigProjection_AppliesSemanticConfigEvents(t *testing.T) {
	p, model := newConfigProjectionUnderModel()

	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerNameChanged{
		ServerNameChanged: &corev1.ServerNameChangedEvent{Name: "Semantic Server"},
	}}, 1))
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerMotdChanged{
		ServerMotdChanged: &corev1.ServerMotdChangedEvent{Motd: "semantic motd"},
	}}, 2))

	cfg := model.GetServerConfig()
	require.Equal(t, "Semantic Server", cfg.ServerName)
	require.Equal(t, "semantic motd", cfg.Motd)
	require.Equal(t, "Semantic Server", model.GetEffectiveServerName())
	require.Equal(t, "semantic motd", model.GetEffectiveMOTD())

	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerNameChanged{
		ServerNameChanged: &corev1.ServerNameChangedEvent{Name: ""},
	}}, 3))
	cfg = model.GetServerConfig()
	require.Equal(t, "", cfg.ServerName)
	require.Equal(t, "Chatto", model.GetEffectiveServerName())
	require.Equal(t, "semantic motd", model.GetEffectiveMOTD())
}

func TestConfigModel_GetServerConfigReturnsClone(t *testing.T) {
	p, model := newConfigProjectionUnderModel()
	require.NoError(t, p.Apply(newServerNameChangedEvent("Original"), 1))

	cfg := model.GetServerConfig()
	require.Equal(t, "Original", cfg.ServerName)

	// Mutate the returned proto — projection's internal copy must not
	// be affected.
	cfg.ServerName = "Mutated"

	cfg2 := model.GetServerConfig()
	require.Equal(t, "Original", cfg2.ServerName)
}

func TestConfigProjection_UnknownEventTypesIgnored(t *testing.T) {
	p, model := newConfigProjectionUnderModel()

	// An unrelated event variant under the same subject namespace
	// must not affect the projection (forward-compatibility).
	other := &corev1.Event{
		Id: "unrelated",
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"},
		},
	}
	require.NoError(t, p.Apply(other, 1))

	require.Nil(t, model.GetServerConfig())
}

func TestConfigProjection_BrandingDoesNotCreateServerConfig(t *testing.T) {
	p, model := newConfigProjectionUnderModel()

	logo := &corev1.AssetRecord{
		Id:          "logo-asset",
		Filename:    "logo.webp",
		ContentType: "image/webp",
		Storage:     &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: "logo-asset"}},
	}
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerLogoSet{
		ServerLogoSet: &corev1.ServerLogoSetEvent{Asset: logo},
	}}, 1))
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerBannerCleared{
		ServerBannerCleared: &corev1.ServerBannerClearedEvent{},
	}}, 2))

	cfg := model.GetServerConfig()
	require.Nil(t, cfg)
	require.Equal(t, DefaultBlockedUsernames, model.GetEffectiveBlockedUsernames())
}

func TestConfigProjection_BlockedUsernames(t *testing.T) {
	p, model := newConfigProjectionUnderModel()

	// Before any config: defaults apply.
	require.True(t, model.IsUsernameBlocked("admin"))
	require.False(t, model.IsUsernameBlocked("alice"))

	// Operator sets a custom list.
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerBlockedUsernamesChanged{
		ServerBlockedUsernamesChanged: &corev1.ServerBlockedUsernamesChangedEvent{BlockedUsernames: "foo\nBAR\nbaz"},
	}}, 1))

	require.True(t, model.IsUsernameBlocked("foo"))
	require.True(t, model.IsUsernameBlocked("bar"))
	require.True(t, model.IsUsernameBlocked("BAR"))
	require.False(t, model.IsUsernameBlocked("admin"))

	// Operator explicitly clears the list. Once there is a blocked-username
	// event, an empty string is meaningful and should not fall back to defaults.
	require.NoError(t, p.Apply(&corev1.Event{Event: &corev1.Event_ServerBlockedUsernamesChanged{
		ServerBlockedUsernamesChanged: &corev1.ServerBlockedUsernamesChangedEvent{BlockedUsernames: ""},
	}}, 2))

	require.False(t, model.IsUsernameBlocked("foo"))
	require.False(t, model.IsUsernameBlocked("admin"))
	require.Equal(t, "", model.GetEffectiveBlockedUsernames())
}
