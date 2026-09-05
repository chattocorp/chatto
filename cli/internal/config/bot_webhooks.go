package config

import (
	"fmt"
	"time"
)

// BotWebhooksConfig is operator policy for outbound bot delivery. Requests
// capture attempt limits, delay, and source-time expiry when work is created.
type BotWebhooksConfig struct {
	MaxAttempts          int      `toml:"max_attempts,commented" env:"CHATTO_BOT_WEBHOOKS_MAX_ATTEMPTS" comment:"Maximum outbound webhook attempts, including the first request. Default: 5."`
	RetryDelay           Duration `toml:"retry_delay,commented" env:"CHATTO_BOT_WEBHOOKS_RETRY_DELAY" comment:"Initial retry delay. Doubles after each attempt, up to 30m. Default: 30s."`
	Expiry               Duration `toml:"expiry,commented" env:"CHATTO_BOT_WEBHOOKS_EXPIRY" comment:"Delivery lifetime from the source message time. Default: 24h."`
	AllowPrivateNetworks bool     `toml:"allow_private_networks,commented" env:"CHATTO_BOT_WEBHOOKS_ALLOW_PRIVATE_NETWORKS" comment:"Allow private network destinations and HTTP. Enable only when bot managers may access internal services. Default: false."`
}

// MaxAttemptsOrDefault returns the maximum reserved attempts per delivery.
func (c BotWebhooksConfig) MaxAttemptsOrDefault() int {
	if c.MaxAttempts == 0 {
		return 5
	}
	return c.MaxAttempts
}

// RetryDelayOrDefault returns the initial exponential retry delay.
func (c BotWebhooksConfig) RetryDelayOrDefault() time.Duration {
	if c.RetryDelay == 0 {
		return 30 * time.Second
	}
	return time.Duration(c.RetryDelay)
}

// ExpiryOrDefault returns the delivery lifetime from the source message time.
func (c BotWebhooksConfig) ExpiryOrDefault() time.Duration {
	if c.Expiry == 0 {
		return 24 * time.Hour
	}
	return time.Duration(c.Expiry)
}

// Validate rejects policies that cannot bound delivery work safely.
func (c BotWebhooksConfig) Validate() error {
	if c.MaxAttemptsOrDefault() < 1 || c.MaxAttemptsOrDefault() > 100 {
		return fmt.Errorf("bot_webhooks.max_attempts must be between 1 and 100")
	}
	if c.RetryDelayOrDefault() < time.Second || c.RetryDelayOrDefault() > 30*time.Minute {
		return fmt.Errorf("bot_webhooks.retry_delay must be between 1s and 30m")
	}
	if c.ExpiryOrDefault() < time.Second || c.ExpiryOrDefault() > 30*24*time.Hour {
		return fmt.Errorf("bot_webhooks.expiry must be between 1s and 30d")
	}
	return nil
}
