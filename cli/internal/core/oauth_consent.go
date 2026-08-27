package core

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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

func (c *ChattoCore) HasOAuthConsent(ctx context.Context, userID, redirectOrigin string) (bool, error) {
	return c.HasOAuthClientConsent(ctx, userID, "", redirectOrigin)
}

func (c *ChattoCore) HasOAuthClientConsent(ctx context.Context, userID, clientID, redirectOrigin string) (bool, error) {
	key := OAuthConsentKey(clientID, redirectOrigin)
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
	origin := OAuthConsentOrigin(redirectOrigin)
	key := OAuthConsentKey(clientID, origin)
	if key == "" {
		return nil
	}

	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_OauthConsentGranted{
		OauthConsentGranted: &corev1.OAuthConsentGrantedEvent{
			UserId:         userID,
			RedirectOrigin: origin,
			Request:        auditRequestMetadata(ctx),
			ClientId:       strings.TrimSpace(clientID),
			ClientName:     strings.TrimSpace(clientName),
			ClientUri:      privacySafeOAuthClientURI(clientURI),
		},
	}})
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
	origin := OAuthConsentOrigin(redirectOrigin)
	if OAuthConsentKey(clientID, origin) == "" {
		return nil
	}

	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_OauthConsentDenied{
		OauthConsentDenied: &corev1.OAuthConsentDeniedEvent{
			UserId:         userID,
			RedirectOrigin: origin,
			Request:        auditRequestMetadata(ctx),
			ClientId:       strings.TrimSpace(clientID),
			ClientName:     strings.TrimSpace(clientName),
			ClientUri:      privacySafeOAuthClientURI(clientURI),
		},
	}})
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
