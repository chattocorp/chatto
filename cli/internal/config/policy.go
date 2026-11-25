package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// DefaultMaxLoginLength applies when the operator does not configure a limit.
const DefaultMaxLoginLength = 64

// LoginLengthCeiling bounds login identifiers accepted by the sign-in endpoint.
const LoginLengthCeiling = 254

// CallsConfig controls admission to calls across one Chatto server.
type CallsConfig struct {
	MaxParticipants *int `toml:"max_participants,commented" env:"CHATTO_CORE_CALLS_MAX_PARTICIPANTS" comment:"Maximum concurrent call participants across all rooms. -1 = unlimited (default), 0 = no new admissions."`
}

// MaxParticipantsOrDefault returns -1 when the operator omits the limit.
func (c CallsConfig) MaxParticipantsOrDefault() int {
	if c.MaxParticipants == nil {
		return -1
	}
	return *c.MaxParticipants
}

// AccountsConfig contains server-wide limits for accounts and usernames.
type AccountsConfig struct {
	MaxLoginLength *int `toml:"max_login_length,commented" env:"CHATTO_CORE_ACCOUNTS_MAX_LOGIN_LENGTH" comment:"Maximum length of new or changed usernames, from 2 to 254 characters. Default: 64."`
	// MaxUsers caps verified accounts. Concurrent creations can briefly exceed this soft limit.
	MaxUsers *int `toml:"max_users,commented" env:"CHATTO_CORE_ACCOUNTS_MAX_USERS" comment:"Maximum number of verified accounts allowed in this instance. -1 = unlimited (default), 0 = no new signups, positive = cap. Counts users with at least one verified email or linked SSO identity."`
}

// MaxLoginLengthOrDefault returns the maximum length for newly assigned logins.
func (c AccountsConfig) MaxLoginLengthOrDefault() int {
	if c.MaxLoginLength == nil {
		return DefaultMaxLoginLength
	}
	return *c.MaxLoginLength
}

// MaxUsersOrDefault returns the configured max-users limit, defaulting to -1 (unlimited).
func (c *AccountsConfig) MaxUsersOrDefault() int {
	if c.MaxUsers == nil {
		return -1
	}
	return *c.MaxUsers
}

// OwnersConfig declares verified emails and AT Protocol DIDs that grant the
// owner role. Removing an entry never revokes an existing role assignment.
type OwnersConfig struct {
	DIDs   []string `toml:"dids" env:"CHATTO_OWNERS_DIDS" comment:"AT Protocol DIDs that confer owner status. Handles are not accepted. Removing a DID does not revoke the role."`
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

// IsServerOwnerDID compares stable DID subjects exactly. Handles never match.
func (c *OwnersConfig) IsServerOwnerDID(did string) bool {
	return slices.Contains(c.DIDs, did)
}

// Configured reports whether an operator has declared an owner.
func (c *OwnersConfig) Configured() bool { return len(c.Emails) != 0 || len(c.DIDs) != 0 }

var ownerDIDPattern = regexp.MustCompile(`^did:(plc:[a-z2-7]{24}|web:[a-zA-Z0-9._-]+(?:(?:%[0-9a-fA-F]{2})[a-zA-Z0-9._-]+)*(?::(?:[a-zA-Z0-9._-]|%[0-9a-fA-F]{2})+)*)$`)

// Validate rejects handles and malformed DID entries without disclosing values.
func (c *OwnersConfig) Validate() error {
	for _, did := range c.DIDs {
		if !ownerDIDPattern.MatchString(did) {
			return fmt.Errorf("owners.dids contains an invalid AT Protocol DID")
		}
	}
	return nil
}
