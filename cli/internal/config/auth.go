package config

import (
	"time"
)

const (
	AccountCreationPolicyOpen       = "open"
	AccountCreationPolicyInviteOnly = "invite_only"

	AuthProviderTypeOpenIDConnect = "oidc"
	AuthProviderTypeGitHub        = "github"
	AuthProviderTypeGitLab        = "gitlab"
	AuthProviderTypeGoogle        = "google"
	AuthProviderTypeDiscord       = "discord"
)

var authProviderDefaultLabels = map[string]string{
	AuthProviderTypeOpenIDConnect: "OpenID Connect",
	AuthProviderTypeGitHub:        "GitHub",
	AuthProviderTypeGitLab:        "GitLab",
	AuthProviderTypeGoogle:        "Google",
	AuthProviderTypeDiscord:       "Discord",
}

// AuthProviderConfig contains one configured external login provider. The ID is
// a stable local issuer namespace for OAuth-only providers and must not be
// changed after users link identities through it.
type AuthProviderConfig struct {
	ID              string            `toml:"id" comment:"Stable provider ID used in callback URLs and external identity links. Do not change after users link accounts."`
	Type            string            `toml:"type" comment:"Provider type: oidc, github, gitlab, google, or discord."`
	Label           string            `toml:"label,commented" comment:"Button label shown on the login page. Defaults to the provider type's display name."`
	ClientID        string            `toml:"client_id" comment:"OAuth/OIDC client ID."`
	ClientSecret    string            `toml:"client_secret,commented" comment:"OAuth/OIDC client secret. NEVER SHARE THIS! Optional only for public OIDC clients."`
	IssuerURL       string            `toml:"issuer_url,commented" comment:"OIDC issuer URL. Required when type = 'oidc'."`
	Scopes          []string          `toml:"scopes,commented" comment:"Optional OAuth scopes. Defaults are provider-specific."`
	RequestEmail    *bool             `toml:"request_email,commented" comment:"Whether to request email scopes for providers that support it. Default: false. Chatto still matches by provider subject without an email claim."`
	AutoProvision   *bool             `toml:"auto_provision,commented" comment:"Whether unlinked external identities may create a new passwordless account after explicit confirmation. Default: false. The linked provider identity counts as a verified sign-in factor."`
	ProviderOptions map[string]string `toml:"provider_options,commented" comment:"Provider-specific options reserved for future use."`
}

// LabelOrDefault returns the configured label, or a provider-specific default.
func (c AuthProviderConfig) LabelOrDefault() string {
	if c.Label != "" {
		return c.Label
	}
	if label, ok := authProviderDefaultLabels[c.Type]; ok {
		return label
	}
	return c.ID
}

func (c AuthProviderConfig) RequestEmailOrDefault() bool {
	if c.RequestEmail == nil {
		return false
	}
	return *c.RequestEmail
}

func (c AuthProviderConfig) AutoProvisionOrDefault() bool {
	if c.AutoProvision == nil {
		return false
	}
	return *c.AutoProvision
}

func IsAllowedAuthProviderType(providerType string) bool {
	_, ok := authProviderDefaultLabels[providerType]
	return ok
}

type AuthConfig struct {
	DirectRegistration    *bool                `toml:"direct_registration" env:"CHATTO_AUTH_DIRECT_REGISTRATION" comment:"Enable direct (email/password) registration. When false, self-service account creation is disabled; existing accounts can still sign in. Default: true."`
	DirectLogin           *bool                `toml:"direct_login" env:"CHATTO_AUTH_DIRECT_LOGIN" comment:"Enable direct login with a username or email address and password. When false, users must sign in via configured SSO providers. Default: true."`
	AccountCreationPolicy string               `toml:"account_creation_policy,commented" env:"CHATTO_AUTH_ACCOUNT_CREATION_POLICY" comment:"Account admission policy: open or invite_only. Default: open. Upgrade every serving replica before enabling invite_only."`
	TokenTTL              Duration             `toml:"token_ttl,commented" env:"CHATTO_AUTH_TOKEN_TTL" comment:"Maximum renewable bearer-session lifetime and lifetime of each same-origin cookie credential. Supports human-readable durations like '90d', '2160h'. Default: 90d."`
	AccessTokenTTL        Duration             `toml:"access_token_ttl,commented" env:"CHATTO_AUTH_ACCESS_TOKEN_TTL" comment:"Lifetime for renewable bearer access tokens. Supports human-readable durations like '15m'. Default: 15m."`
	EmailOTP              EmailOTPConfig       `toml:"email_otp,commented" comment:"Email OTP guardrails for registration and email verification."`
	Providers             []AuthProviderConfig `toml:"providers" comment:"External login providers. Configure as repeated [[auth.providers]] tables."`
}

func (c *AuthConfig) AccountCreationPolicyOrDefault() string {
	if c.AccountCreationPolicy == "" {
		return AccountCreationPolicyOpen
	}
	return c.AccountCreationPolicy
}

func (c *AuthConfig) InvitationRequired() bool {
	return c.AccountCreationPolicyOrDefault() == AccountCreationPolicyInviteOnly
}

// EmailOTPConfig controls registration and email-verification one-time-password guardrails.
type EmailOTPConfig struct {
	ThrottlingEnabled *bool    `toml:"throttling_enabled,commented" env:"CHATTO_AUTH_EMAIL_OTP_THROTTLING_ENABLED" comment:"Enable email OTP throttling for registration and email verification. Default: true."`
	TTL               Duration `toml:"ttl,commented" env:"CHATTO_AUTH_EMAIL_OTP_TTL" comment:"How long registration and email-verification codes stay valid. Default: 15m."`
	MaxDeliveredCodes int      `toml:"max_delivered_codes,commented" env:"CHATTO_AUTH_EMAIL_OTP_MAX_DELIVERED_CODES" comment:"Maximum successfully delivered codes per email challenge before throttling. Default: 10."`
	MaxWrongAttempts  int      `toml:"max_wrong_attempts,commented" env:"CHATTO_AUTH_EMAIL_OTP_MAX_WRONG_ATTEMPTS" comment:"Maximum wrong-code attempts per email challenge before throttling. Default: 5."`
}

// ThrottlingEnabledOrDefault returns whether email OTP throttling is enabled (default: true).
func (c *EmailOTPConfig) ThrottlingEnabledOrDefault() bool {
	if c.ThrottlingEnabled == nil {
		return true
	}
	return *c.ThrottlingEnabled
}

// TTLOrDefault returns the configured email OTP TTL, or 15 minutes if unset.
func (c *EmailOTPConfig) TTLOrDefault() time.Duration {
	if c.TTL == 0 {
		return 15 * time.Minute
	}
	return c.TTL.Duration()
}

// MaxDeliveredCodesOrDefault returns the delivered-code limit, or 10 if unset.
func (c *EmailOTPConfig) MaxDeliveredCodesOrDefault() int {
	if c.MaxDeliveredCodes == 0 {
		return 10
	}
	return c.MaxDeliveredCodes
}

// MaxWrongAttemptsOrDefault returns the wrong-code attempt limit, or 5 if unset.
func (c *EmailOTPConfig) MaxWrongAttemptsOrDefault() int {
	if c.MaxWrongAttempts == 0 {
		return 5
	}
	return c.MaxWrongAttempts
}

// TokenTTLOrDefault returns the configured absolute human-session lifetime, or
// 90 days if not set.
func (c *AuthConfig) TokenTTLOrDefault() time.Duration {
	if c.TokenTTL == 0 {
		return 90 * 24 * time.Hour
	}
	return c.TokenTTL.Duration()
}

// AccessTokenTTLOrDefault returns the configured renewable bearer access-token
// lifetime, or 15 minutes if not set.
func (c *AuthConfig) AccessTokenTTLOrDefault() time.Duration {
	if c.AccessTokenTTL == 0 {
		return 15 * time.Minute
	}
	return c.AccessTokenTTL.Duration()
}

// DirectRegistrationOrDefault returns whether direct (email/password) registration is enabled (default: true).
func (c *AuthConfig) DirectRegistrationOrDefault() bool {
	if c.DirectRegistration == nil {
		return true
	}
	return *c.DirectRegistration
}

// DirectLoginOrDefault returns whether direct password login is enabled (default: true).
func (c *AuthConfig) DirectLoginOrDefault() bool {
	if c.DirectLogin == nil {
		return true
	}
	return *c.DirectLogin
}

// EnabledProviders returns a list of configured SSO provider IDs.
func (c *AuthConfig) EnabledProviders() []string {
	providers := make([]string, 0, len(c.Providers))
	for _, provider := range c.Providers {
		providers = append(providers, provider.ID)
	}
	return providers
}

// PublicProviders returns login metadata safe to expose before authentication.
func (c *AuthConfig) PublicProviders() []AuthProviderConfig {
	providers := make([]AuthProviderConfig, 0, len(c.Providers))
	for _, provider := range c.Providers {
		providers = append(providers, AuthProviderConfig{
			ID:            provider.ID,
			Type:          provider.Type,
			Label:         provider.LabelOrDefault(),
			IssuerURL:     provider.IssuerURL,
			AutoProvision: provider.AutoProvision,
		})
	}
	return providers
}
