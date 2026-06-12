package core

import (
	"context"
	"errors"
	"strings"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var errOAuthConsentAlreadyGranted = errors.New("OAuth consent already granted")

func OAuthConsentOrigin(redirectOrigin string) string {
	return strings.ToLower(strings.TrimSpace(redirectOrigin))
}

func (c *ChattoCore) HasOAuthConsent(ctx context.Context, userID, redirectOrigin string) (bool, error) {
	origin := OAuthConsentOrigin(redirectOrigin)
	if origin == "" {
		return false, nil
	}
	if c.userService != nil {
		if err := c.userService.waitForUsersCurrent(ctx, "OAuth consent", events.UserAggregate(userID).AllEventsFilter()); err != nil {
			return false, err
		}
	}
	return c.Users.HasOAuthConsent(userID, origin), nil
}

func (c *ChattoCore) GrantOAuthConsent(ctx context.Context, userID, redirectOrigin string) error {
	origin := OAuthConsentOrigin(redirectOrigin)
	if origin == "" {
		return nil
	}

	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_OauthConsentGranted{
		OauthConsentGranted: &corev1.OAuthConsentGrantedEvent{
			UserId:         userID,
			RedirectOrigin: origin,
			Request:        auditRequestMetadata(ctx),
		},
	}})
	_, err := c.appendUserEvent(ctx, userID, event, "", func() error {
		if c.Users.HasOAuthConsent(userID, origin) {
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
	origin := OAuthConsentOrigin(redirectOrigin)
	if origin == "" {
		return nil
	}

	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_OauthConsentDenied{
		OauthConsentDenied: &corev1.OAuthConsentDeniedEvent{
			UserId:         userID,
			RedirectOrigin: origin,
			Request:        auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(userID), event); err != nil {
		return err
	}
	return nil
}
