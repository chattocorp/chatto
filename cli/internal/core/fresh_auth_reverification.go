package core

import (
	"context"
	"errors"
	"time"

	"github.com/chattocorp/chatto/cli/internal/config"
)

const (
	// freshAuthReverificationClockSkew allows for the provider's whole-second
	// auth_time and small clock differences between servers.
	freshAuthReverificationClockSkew = 30 * time.Second
	freshAuthReverificationSource    = "external_identity_reverification"
)

var (
	// ErrFreshAuthReverificationMismatch reports a provider identity that is
	// not linked to the account that started the re-verification, or a
	// callback in a browser that no longer holds a session of that account.
	ErrFreshAuthReverificationMismatch = errors.New("re-verified identity does not match the current account")
	// ErrFreshAuthReverificationStale reports a provider sign-in that did not
	// happen during this re-verification, so it does not prove presence.
	ErrFreshAuthReverificationStale = errors.New("provider did not authenticate the user again")
)

// LinkedIdentityForProvider returns the identity in identities that provider
// signs in. An OIDC identity also matches a provider with the same issuer.
func LinkedIdentityForProvider(provider config.AuthProviderConfig, identities []ExternalIdentity) (ExternalIdentity, bool) {
	for _, identity := range identities {
		if identity.ProviderID == provider.ID {
			return identity, true
		}
		if provider.Type == config.AuthProviderTypeOpenIDConnect &&
			identity.ProviderType == config.AuthProviderTypeOpenIDConnect &&
			identity.Issuer == provider.IssuerURL {
			return identity, true
		}
	}
	return ExternalIdentity{}, false
}

// ProviderCanReverify reports whether provider can confirm a recent sign-in
// of the user with identities. Only OpenID Connect can force a new provider
// sign-in (prompt=login, max_age=0) and prove it with auth_time. Other
// provider types can pass an existing provider session through silently.
func ProviderCanReverify(provider config.AuthProviderConfig, identities []ExternalIdentity) bool {
	if provider.Type != config.AuthProviderTypeOpenIDConnect {
		return false
	}
	_, linked := LinkedIdentityForProvider(provider, identities)
	return linked
}

// CookieSessionUser returns the user of a valid first-party cookie session.
func (c *ChattoCore) CookieSessionUser(ctx context.Context, cookieHandle string) (string, error) {
	credential, err := c.validateRuntimeCredential(ctx, cookieHandle, AuthTokenPresentationCookie, authoritativeCredentialRead)
	if err != nil {
		return "", err
	}
	if credential.Kind != AuthTokenKindFirstPartySession {
		return "", ErrFreshAuthRequired
	}
	return credential.UserID, nil
}

// CompleteFreshAuthReverification marks cookieHandle fresh after a provider
// verified issuer and subject with a sign-in at authTime. userID and startedAt
// come from the start of the re-verification in the same browser. The
// identity and the cookie session must both belong to userID, and authTime
// must not be earlier than startedAt. It never creates a session.
func (c *ChattoCore) CompleteFreshAuthReverification(ctx context.Context, userID string, startedAt time.Time, issuer, subject string, authTime time.Time, cookieHandle string) error {
	if userID == "" || startedAt.IsZero() {
		return ErrFreshAuthReverificationMismatch
	}
	user, _, err := c.GetUserByExternalIdentityForAuthentication(ctx, issuer, subject)
	if err != nil {
		return err
	}
	if user == nil || user.Id != userID {
		return ErrFreshAuthReverificationMismatch
	}
	if authTime.IsZero() || authTime.Before(startedAt.Add(-freshAuthReverificationClockSkew)) {
		return ErrFreshAuthReverificationStale
	}
	if cookieHandle == "" {
		return ErrFreshAuthReverificationMismatch
	}
	sessionUserID, err := c.CookieSessionUser(ctx, cookieHandle)
	if err != nil {
		return err
	}
	if sessionUserID != userID {
		return ErrFreshAuthReverificationMismatch
	}
	return c.MarkCookieSessionFresh(ctx, cookieHandle, "external_identity", freshAuthReverificationSource)
}
