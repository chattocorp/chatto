package core

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

var errOAuthConsentAlreadyGranted = errors.New("OAuth consent already granted")

func OAuthConsentOrigin(redirectOrigin string) string {
	return strings.ToLower(strings.TrimSpace(redirectOrigin))
}

// OAuthConsentKey returns the stable client identity used for consent. Legacy
// origin-only flows retain their origin key until the 0.5 transport cutover.
func OAuthConsentKey(clientID, redirectOrigin string) string {
	if clientID = strings.TrimSpace(clientID); clientID != "" {
		return clientID
	}
	return OAuthConsentOrigin(redirectOrigin)
}

// OAuthScopedConsentKey returns the stable identity for an exact delegated
// grant. Empty resource and scope values retain the legacy client key.
func OAuthScopedConsentKey(clientID, redirectOrigin, resource string, scopes []string) string {
	base := OAuthConsentKey(clientID, redirectOrigin)
	if base == "" || resource == "" && len(scopes) == 0 {
		return base
	}
	return base + "\x00" + resource + "\x00" + strings.Join(scopes, " ")
}

func (c *ChattoCore) HasOAuthConsent(ctx context.Context, userID, redirectOrigin string) (bool, error) {
	return c.HasOAuthClientConsent(ctx, userID, "", redirectOrigin)
}

func (c *ChattoCore) HasOAuthClientConsent(ctx context.Context, userID, clientID, redirectOrigin string) (bool, error) {
	return c.HasOAuthClientScopedConsent(ctx, userID, clientID, redirectOrigin, "", nil)
}

// HasOAuthClientScopedConsent reports whether the user approved the exact
// resource and normalized scope set for this client.
func (c *ChattoCore) HasOAuthClientScopedConsent(ctx context.Context, userID, clientID, redirectOrigin, resource string, scopes []string) (bool, error) {
	key := OAuthScopedConsentKey(clientID, redirectOrigin, resource, scopes)
	if key == "" {
		return false, nil
	}
	if c.userModel != nil {
		if err := c.userModel.waitForUsersCurrent(ctx, "OAuth consent", evtstream.UserAggregate(userID).AllEventsFilter()); err != nil {
			return false, err
		}
	}
	return c.userModel.hasOAuthConsent(userID, key), nil
}

func (c *ChattoCore) GrantOAuthConsent(ctx context.Context, userID, redirectOrigin string) error {
	return c.GrantOAuthClientConsent(ctx, userID, "", "", "", redirectOrigin)
}

func (c *ChattoCore) GrantOAuthClientConsent(ctx context.Context, userID, clientID, clientName, clientURI, redirectOrigin string) error {
	return c.GrantOAuthClientScopedConsent(ctx, userID, clientID, clientName, clientURI, redirectOrigin, "", nil)
}

// GrantOAuthClientScopedConsent records the exact resource and normalized
// scope set approved by the user.
func (c *ChattoCore) GrantOAuthClientScopedConsent(ctx context.Context, userID, clientID, clientName, clientURI, redirectOrigin, resource string, scopes []string) error {
	origin := OAuthConsentOrigin(redirectOrigin)
	key := OAuthScopedConsentKey(clientID, origin, resource, scopes)
	if key == "" {
		return nil
	}

	var payload *evtv1.Event
	if resource != "" || len(scopes) != 0 {
		payload = &evtv1.Event{Event: &evtv1.Event_OauthScopedConsentGranted{
			OauthScopedConsentGranted: &evtv1.OAuthScopedConsentGrantedEvent{
				UserId: userID, RedirectOrigin: origin, Request: auditRequestMetadata(ctx),
				ClientId: strings.TrimSpace(clientID), ClientName: strings.TrimSpace(clientName),
				ClientUri: privacySafeOAuthClientURI(clientURI), Resource: resource,
				Scopes: append([]string(nil), scopes...),
			},
		}}
	} else {
		payload = &evtv1.Event{Event: &evtv1.Event_OauthConsentGranted{
			OauthConsentGranted: &evtv1.OAuthConsentGrantedEvent{
				UserId:         userID,
				RedirectOrigin: origin,
				Request:        auditRequestMetadata(ctx),
				ClientId:       strings.TrimSpace(clientID),
				ClientName:     strings.TrimSpace(clientName),
				ClientUri:      privacySafeOAuthClientURI(clientURI),
			},
		}}
	}
	event := newEvent(userID, payload)
	_, err := c.appendUserEvent(ctx, userID, event, "", func() error {
		if c.userModel.hasOAuthConsent(userID, key) {
			return errOAuthConsentAlreadyGranted
		}
		return nil
	})
	if errors.Is(err, errOAuthConsentAlreadyGranted) {
		return nil
	}
	return err
}

func (c *ChattoCore) RecordOAuthConsentDenied(ctx context.Context, userID, redirectOrigin string) error {
	return c.RecordOAuthClientConsentDenied(ctx, userID, "", "", "", redirectOrigin)
}

func (c *ChattoCore) RecordOAuthClientConsentDenied(ctx context.Context, userID, clientID, clientName, clientURI, redirectOrigin string) error {
	return c.RecordOAuthClientScopedConsentDenied(ctx, userID, clientID, clientName, clientURI, redirectOrigin, "", nil)
}

// RecordOAuthClientScopedConsentDenied records a denied delegated grant for
// audit without changing the set of approved grants.
func (c *ChattoCore) RecordOAuthClientScopedConsentDenied(ctx context.Context, userID, clientID, clientName, clientURI, redirectOrigin, resource string, scopes []string) error {
	origin := OAuthConsentOrigin(redirectOrigin)
	if OAuthScopedConsentKey(clientID, origin, resource, scopes) == "" {
		return nil
	}

	var payload *evtv1.Event
	if resource != "" || len(scopes) != 0 {
		payload = &evtv1.Event{Event: &evtv1.Event_OauthScopedConsentDenied{
			OauthScopedConsentDenied: &evtv1.OAuthScopedConsentDeniedEvent{
				UserId: userID, RedirectOrigin: origin, Request: auditRequestMetadata(ctx),
				ClientId: strings.TrimSpace(clientID), ClientName: strings.TrimSpace(clientName),
				ClientUri: privacySafeOAuthClientURI(clientURI), Resource: resource,
				Scopes: append([]string(nil), scopes...),
			},
		}}
	} else {
		payload = &evtv1.Event{Event: &evtv1.Event_OauthConsentDenied{
			OauthConsentDenied: &evtv1.OAuthConsentDeniedEvent{
				UserId:         userID,
				RedirectOrigin: origin,
				Request:        auditRequestMetadata(ctx),
				ClientId:       strings.TrimSpace(clientID),
				ClientName:     strings.TrimSpace(clientName),
				ClientUri:      privacySafeOAuthClientURI(clientURI),
			},
		}}
	}
	event := newEvent(userID, payload)
	if err := c.appendAuthAuditEvent(ctx, evtstream.UserAggregate(userID), event); err != nil {
		return err
	}
	return nil
}

// privacySafeOAuthClientURI retains only the URI origin needed to recognise a
// client. Paths and queries are attacker-controlled metadata and must not enter
// the durable audit stream.
func privacySafeOAuthClientURI(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
}
