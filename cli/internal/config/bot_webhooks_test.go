package config

import (
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestBotWebhooksOperatorPolicy(t *testing.T) {
	var cfg ChattoConfig
	require.NoError(t, toml.Unmarshal([]byte("[bot_webhooks]\nmax_attempts=7\nretry_delay='2m'\nexpiry='3d'\nallow_private_networks=true\n"), &cfg))
	require.NoError(t, cfg.BotWebhooks.Validate())
	require.Equal(t, 7, cfg.BotWebhooks.MaxAttemptsOrDefault())
	require.Equal(t, 2*time.Minute, cfg.BotWebhooks.RetryDelayOrDefault())
	require.Equal(t, 72*time.Hour, cfg.BotWebhooks.ExpiryOrDefault())
	require.True(t, cfg.BotWebhooks.AllowPrivateNetworks)
	require.Equal(t, 5, (BotWebhooksConfig{}).MaxAttemptsOrDefault())
	for _, invalid := range []BotWebhooksConfig{{MaxAttempts: -1}, {MaxAttempts: 101}, {RetryDelay: Duration(-time.Second)}, {Expiry: Duration(-time.Second)}, {Expiry: Duration(31 * 24 * time.Hour)}} {
		require.Error(t, invalid.Validate())
	}
}
func TestBotWebhooksEnvironment(t *testing.T) {
	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_BOT_WEBHOOKS_MAX_ATTEMPTS", "9")
	t.Setenv("CHATTO_JOBS_MAX_AGE", "9d")
	t.Setenv("CHATTO_BOT_WEBHOOKS_RETRY_DELAY", "15s")
	t.Setenv("CHATTO_BOT_WEBHOOKS_EXPIRY", "2h")
	cfg, err := ReadConfig("")
	require.NoError(t, err)
	require.Equal(t, 9, cfg.BotWebhooks.MaxAttemptsOrDefault())
	require.Equal(t, 9*24*time.Hour, cfg.Jobs.MaxAgeOrDefault())
	require.Equal(t, 15*time.Second, cfg.BotWebhooks.RetryDelayOrDefault())
	require.Equal(t, 2*time.Hour, cfg.BotWebhooks.ExpiryOrDefault())
}
