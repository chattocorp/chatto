package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/readmodel"
)

// ErrUsernameManaged reports that a linked provider controls username editing.
var ErrUsernameManaged = errors.New("username is managed by a sign-in provider")

// ErrProviderUsernameUnavailable identifies a provider configuration or naming
// conflict that prevents managed account creation.
var ErrProviderUsernameUnavailable = errors.New("provider username cannot be assigned; contact the server operator")

// ErrExternalIdentityUnlinkDisabled reports an operator restriction on self-service.
var ErrExternalIdentityUnlinkDisabled = errors.New("disconnecting this sign-in provider is disabled")

// providerMatchesIdentity uses the issuer namespace. A legacy hash-only OIDC
// link cannot identify its provider, so OIDC restrictions apply conservatively
// until a verified sign-in completes its metadata.
func providerMatchesIdentity(provider config.AuthProviderConfig, identity ExternalIdentity) bool {
	if identity.Issuer == "" {
		if identity.ProviderType == config.AuthProviderTypeOpenIDConnect {
			return provider.Type == config.AuthProviderTypeOpenIDConnect
		}
		return identity.ProviderID == provider.ID
	}
	issuer := provider.ID
	if provider.Type == config.AuthProviderTypeOpenIDConnect {
		issuer = provider.IssuerURL
	}
	return issuer == identity.Issuer
}

func (c *ChattoCore) usernameManaged(userID string) bool {
	for _, identity := range c.userModel.externalIdentities(userID) {
		for _, provider := range c.config.AuthProviders {
			if provider.ManageUsernames && providerMatchesIdentity(provider, identity) {
				return true
			}
		}
	}
	return false
}

// UsernameManaged returns the effective local-edit restriction after catching
// up account links. Mutations must recheck it under the user OCC guard.
func (c *ChattoCore) UsernameManaged(ctx context.Context, userID string) (bool, error) {
	if err := c.userModel.waitForUsersCurrent(ctx, "username policy", evtstream.UserSubjectFilter()); err != nil {
		return false, err
	}
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return false, err
	}
	// Bot profile edits remain independent of human sign-in policies.
	if user.GetIsBot() {
		return false, nil
	}
	return c.usernameManaged(userID), nil
}

// IdentityUnlinkDisabled reports the configured self-service restriction. It
// does not check whether another usable sign-in method remains.
func (c *ChattoCore) IdentityUnlinkDisabled(identity ExternalIdentity) bool {
	for _, provider := range c.config.AuthProviders {
		if provider.AllowUnlinking != nil && !*provider.AllowUnlinking && providerMatchesIdentity(provider, identity) {
			return true
		}
	}
	return false
}

// NormalizeProviderUsername preserves the provider's name apart from surrounding
// whitespace and case. It never substitutes an email address or display name.
func NormalizeProviderUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// ProviderUsernameManaged returns the current creation policy for a provider.
func (c *ChattoCore) ProviderUsernameManaged(providerID string) bool {
	for _, provider := range c.config.AuthProviders {
		if provider.ID == providerID {
			return provider.ManageUsernames
		}
	}
	return false
}

const maxProviderUsernameCandidates = 1000

// providerUsernameCandidate leaves room for the numeric collision suffix.
func providerUsernameCandidate(base string, number int) string {
	if number == 1 {
		return base
	}
	suffix := fmt.Sprintf("-%d", number)
	return strings.TrimRight(base[:min(len(base), MaxLoginLength-len(suffix))], ".") + suffix
}

func (c *ChattoCore) validateProviderUsername(base string) error {
	if err := ValidateLogin(base); err != nil {
		return err
	}
	if c.configModel.IsUsernameBlocked(base) || c.loginConflictsWithMentionHandle(base) {
		return ErrUsernameBlocked
	}
	return nil
}

// SuggestProviderUsername is a preview, not a reservation. Creation rechecks
// uniqueness atomically and can select the next suffix if the preview was taken.
func (c *ChattoCore) SuggestProviderUsername(base string) (string, error) {
	base = NormalizeProviderUsername(base)
	if err := c.validateProviderUsername(base); err != nil {
		return "", err
	}
	for number := 1; number <= maxProviderUsernameCandidates; number++ {
		candidate := providerUsernameCandidate(base, number)
		if c.IsLoginAvailable(candidate) {
			return candidate, nil
		}
	}
	return "", ErrLoginAlreadyTaken
}

// SyncProviderUsername applies a verified provider username at sign-in when
// enabled by configuration. Existing collision suffixes stay stable. Callers
// must authenticate the provider subject before calling this method.
func (c *ChattoCore) SyncProviderUsername(ctx context.Context, userID, providerID, base string) error {
	enabled := false
	var source config.AuthProviderConfig
	for _, provider := range c.config.AuthProviders {
		if provider.ID == providerID {
			enabled = provider.ManageUsernames && provider.UpdateUsernames
			source = provider
		}
	}
	if !enabled {
		return nil
	}
	if _, err := c.ExternalIdentitiesForUser(ctx, userID); err != nil {
		return err
	}
	check := func() error {
		for _, identity := range c.userModel.externalIdentities(userID) {
			if providerMatchesIdentity(source, identity) {
				return nil
			}
		}
		return ErrExternalIdentityNotFound
	}
	if err := check(); err != nil {
		return err
	}
	base = NormalizeProviderUsername(base)
	if err := c.validateProviderUsername(base); err != nil {
		return err
	}
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	for number := 1; number <= maxProviderUsernameCandidates; number++ {
		if user.Login == providerUsernameCandidate(base, number) {
			return nil
		}
	}
	for number := 1; number <= maxProviderUsernameCandidates; number++ {
		_, err = c.applyLoginChangeChecked(ctx, SystemActorID, userID, providerUsernameCandidate(base, number), false, check)
		if errors.Is(err, ErrLoginAlreadyTaken) || (number > 1 && errors.Is(err, ErrUsernameBlocked)) {
			continue
		}
		return err
	}
	return ErrLoginAlreadyTaken
}

func (c *ChattoCore) createProviderUser(ctx context.Context, login, displayName string, flow *PendingExternalIdentityFlow) (*readmodel.User, error) {
	managed := c.ProviderUsernameManaged(flow.ProviderID)
	base := NormalizeProviderUsername(flow.ProviderUsername)
	if managed {
		if err := c.validateProviderUsername(base); err != nil {
			return nil, ErrProviderUsernameUnavailable
		}
	}
	for number := 1; number <= maxProviderUsernameCandidates; number++ {
		if managed {
			login = providerUsernameCandidate(base, number)
		}
		name := displayName
		if name == "" {
			name = login
		}
		user, err := c.createUserWithOptions(ctx, SystemActorID, login, name, "", userCreationOptions{
			verifiedEmail: flow.VerifiedEmail, external: flow, invitationID: flow.InvitationID,
			selfService: true,
		})
		if managed && (errors.Is(err, ErrLoginAlreadyTaken) || (number > 1 && errors.Is(err, ErrUsernameBlocked))) {
			continue
		}
		return user, err
	}
	return nil, ErrLoginAlreadyTaken
}
