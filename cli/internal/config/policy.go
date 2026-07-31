package config

import "strings"

// LimitsConfig contains server-wide resource limits. A value of -1 means unlimited
// (the default when unset); 0 means no creation is allowed; any positive integer caps
// the count at that value.
//
// Enforcement note: limits are checked at the entry point of each gated operation
// (CreateUser) by counting current entries in KV. The check is not atomic with
// the subsequent write, so a burst of concurrent requests at the boundary can
// briefly overshoot by one or two. This soft-limit tradeoff is intentional at
// the current scale; GitHub issue #247 records when a CAS counter is warranted.
type LimitsConfig struct {
	MaxUsers *int `toml:"max_users,commented" env:"CHATTO_LIMITS_MAX_USERS" comment:"Maximum number of verified accounts allowed in this instance. -1 = unlimited (default), 0 = no new signups, positive = cap. Counts users with at least one verified email or linked SSO identity."`
}

// MaxUsersOrDefault returns the configured max-users limit, defaulting to -1 (unlimited).
func (c *LimitsConfig) MaxUsersOrDefault() int {
	if c.MaxUsers == nil {
		return -1
	}
	return *c.MaxUsers
}

// OwnersConfig declares the email addresses that confer owner status.
// A user with a matching verified email is treated as having all instance
// permissions (owner-level), which includes access to /admin routes. This is
// the operator-driven mechanism for designating an server owner — useful
// for both Chatto Cloud (the control plane writes the customer's email here at
// provision time) and self-hosters (who set their own email here in chatto.toml).
type OwnersConfig struct {
	Emails []string `toml:"emails" env:"CHATTO_OWNERS_EMAILS" comment:"Email addresses that confer owner status. Users with these verified emails get full instance access, including /admin routes."`
}

// IsServerOwnerEmail checks if an email is in the owners list.
//
// The comparison is case-insensitive and trims surrounding whitespace on both
// sides. Both `c.Emails` and the user-supplied `email` are normalized at the
// call site rather than at config load so that mutations to `c.Emails` (rare)
// don't need to remember to re-normalize.
func (c *OwnersConfig) IsServerOwnerEmail(email string) bool {
	needle := strings.TrimSpace(email)
	for _, e := range c.Emails {
		if strings.EqualFold(strings.TrimSpace(e), needle) {
			return true
		}
	}
	return false
}
