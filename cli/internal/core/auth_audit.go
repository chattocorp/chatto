package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type auditRequestMetadataKey struct{}

// WithAuditRequestMetadata attaches security-safe request metadata to ctx for
// auth workflow audit events. The metadata is copied so callers can reuse their
// input struct without mutating future event payloads.
func WithAuditRequestMetadata(ctx context.Context, metadata *corev1.AuditRequestMetadata) context.Context {
	if metadata == nil {
		return ctx
	}
	return context.WithValue(ctx, auditRequestMetadataKey{}, cloneAuditRequestMetadata(metadata))
}

// AuditRequestMetadataFromContext returns a copy of request audit metadata from
// ctx, if present. Missing metadata is normal for non-HTTP callers.
func AuditRequestMetadataFromContext(ctx context.Context) *corev1.AuditRequestMetadata {
	metadata, _ := ctx.Value(auditRequestMetadataKey{}).(*corev1.AuditRequestMetadata)
	return cloneAuditRequestMetadata(metadata)
}

func cloneAuditRequestMetadata(metadata *corev1.AuditRequestMetadata) *corev1.AuditRequestMetadata {
	if metadata == nil {
		return nil
	}
	cloned, ok := proto.Clone(metadata).(*corev1.AuditRequestMetadata)
	if !ok {
		return &corev1.AuditRequestMetadata{}
	}
	return cloned
}

func auditRequestMetadata(ctx context.Context) *corev1.AuditRequestMetadata {
	if metadata := AuditRequestMetadataFromContext(ctx); metadata != nil {
		return metadata
	}
	return &corev1.AuditRequestMetadata{}
}

func tokenExpiresAt(createdAt time.Time, ttl time.Duration) *timestamppb.Timestamp {
	return timestamppb.New(createdAt.Add(ttl))
}

func auditIdentifierHash(identifier string) string {
	normalized := strings.ToLower(strings.TrimSpace(identifier))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func auditValueHash(value string) string {
	normalized := strings.TrimSpace(value)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func auditTokenSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "unknown"
	}
	return source
}

func auditFailureReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "unspecified"
	}
	return reason
}

func (c *ChattoCore) appendAuthAuditEvent(ctx context.Context, aggregate events.Aggregate, event *corev1.Event) error {
	if c.EventPublisher == nil {
		return errors.New("event publisher is not configured")
	}
	if _, err := c.EventPublisher.AppendEventually(ctx, aggregate.SubjectFor(event), event); err != nil {
		return err
	}
	return nil
}

func (c *ChattoCore) recordRegistrationLinkIssued(ctx context.Context, email string, createdAt time.Time) error {
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RegistrationLinkIssued{
		RegistrationLinkIssued: &corev1.RegistrationLinkIssuedEvent{
			EmailHash: emailHash(email),
			ExpiresAt: tokenExpiresAt(createdAt, RegistrationTokenTTL),
			Request:   auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.AuthAggregate(), event); err != nil {
		return fmt.Errorf("append registration link audit event: %w", err)
	}
	return nil
}

func (c *ChattoCore) recordEmailVerificationLinkIssued(ctx context.Context, userID, email string, createdAt time.Time) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_EmailVerificationLinkIssued{
		EmailVerificationLinkIssued: &corev1.EmailVerificationLinkIssuedEvent{
			UserId:    userID,
			EmailHash: emailHash(email),
			ExpiresAt: tokenExpiresAt(createdAt, EmailVerificationTokenTTL),
			Request:   auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(userID), event); err != nil {
		return fmt.Errorf("append email verification link audit event: %w", err)
	}
	return nil
}

func (c *ChattoCore) recordPasswordResetLinkIssued(ctx context.Context, userID, email string, createdAt time.Time) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_PasswordResetLinkIssued{
		PasswordResetLinkIssued: &corev1.PasswordResetLinkIssuedEvent{
			UserId:    userID,
			EmailHash: emailHash(email),
			ExpiresAt: tokenExpiresAt(createdAt, PasswordResetTokenTTL),
			Request:   auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(userID), event); err != nil {
		return fmt.Errorf("append password reset link audit event: %w", err)
	}
	return nil
}

func (c *ChattoCore) recordAccountDeletionConfirmationIssued(ctx context.Context, userID string, createdAt time.Time) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_AccountDeletionConfirmationIssued{
		AccountDeletionConfirmationIssued: &corev1.AccountDeletionConfirmationIssuedEvent{
			UserId:    userID,
			ExpiresAt: tokenExpiresAt(createdAt, AccountDeletionTokenTTL),
			Request:   auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(userID), event); err != nil {
		return fmt.Errorf("append account deletion confirmation audit event: %w", err)
	}
	return nil
}

func passwordResetCompletedEvent(ctx context.Context, userID string) *corev1.Event {
	return newEvent(userID, &corev1.Event{Event: &corev1.Event_PasswordResetCompleted{
		PasswordResetCompleted: &corev1.PasswordResetCompletedEvent{
			UserId:  userID,
			Request: auditRequestMetadata(ctx),
		},
	}})
}

// RecordLoginSucceeded appends a durable audit fact for a completed login.
func (c *ChattoCore) RecordLoginSucceeded(ctx context.Context, userID, identifier string) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_LoginSucceeded{
		LoginSucceeded: &corev1.LoginSucceededEvent{
			UserId:         userID,
			IdentifierHash: auditIdentifierHash(identifier),
			Request:        auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(userID), event); err != nil {
		return fmt.Errorf("append login success audit event: %w", err)
	}
	return nil
}

// RecordLoginFailed appends a durable audit fact for an unsuccessful login
// attempt without recording whether the identifier matched an account.
func (c *ChattoCore) RecordLoginFailed(ctx context.Context, identifier string) error {
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_LoginFailed{
		LoginFailed: &corev1.LoginFailedEvent{
			IdentifierHash: auditIdentifierHash(identifier),
			Request:        auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.AuthAggregate(), event); err != nil {
		return fmt.Errorf("append login failure audit event: %w", err)
	}
	return nil
}

// RecordLogoutSucceeded appends a durable audit fact for a completed logout.
func (c *ChattoCore) RecordLogoutSucceeded(ctx context.Context, userID string) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_LogoutSucceeded{
		LogoutSucceeded: &corev1.LogoutSucceededEvent{
			UserId:  userID,
			Request: auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(userID), event); err != nil {
		return fmt.Errorf("append logout audit event: %w", err)
	}
	return nil
}

func (c *ChattoCore) recordAuthCodeIssued(ctx context.Context, userID, redirectURI string, createdAt time.Time) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_AuthCodeIssued{
		AuthCodeIssued: &corev1.AuthCodeIssuedEvent{
			UserId:          userID,
			RedirectUriHash: auditValueHash(redirectURI),
			ExpiresAt:       tokenExpiresAt(createdAt, authCodeTTL),
			Request:         auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(userID), event); err != nil {
		return fmt.Errorf("append auth code issuance audit event: %w", err)
	}
	return nil
}

func (c *ChattoCore) recordAuthCodeExchangeSucceeded(ctx context.Context, userID, redirectURI string) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_AuthCodeExchangeSucceeded{
		AuthCodeExchangeSucceeded: &corev1.AuthCodeExchangeSucceededEvent{
			UserId:          userID,
			RedirectUriHash: auditValueHash(redirectURI),
			Request:         auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(userID), event); err != nil {
		return fmt.Errorf("append auth code exchange success audit event: %w", err)
	}
	return nil
}

func (c *ChattoCore) recordAuthCodeExchangeFailed(ctx context.Context, userID, redirectURI, reason string) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_AuthCodeExchangeFailed{
		AuthCodeExchangeFailed: &corev1.AuthCodeExchangeFailedEvent{
			UserId:          userID,
			RedirectUriHash: auditValueHash(redirectURI),
			Reason:          auditFailureReason(reason),
			Request:         auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(userID), event); err != nil {
		return fmt.Errorf("append auth code exchange failure audit event: %w", err)
	}
	return nil
}

func (c *ChattoCore) recordBearerTokenIssued(ctx context.Context, userID string, createdAt time.Time, source string) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_BearerTokenIssued{
		BearerTokenIssued: &corev1.BearerTokenIssuedEvent{
			UserId:    userID,
			ExpiresAt: tokenExpiresAt(createdAt, c.authTokenTTL()),
			Source:    auditTokenSource(source),
			Request:   auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(userID), event); err != nil {
		return fmt.Errorf("append bearer token issuance audit event: %w", err)
	}
	return nil
}

func (c *ChattoCore) recordBearerTokenRevoked(ctx context.Context, userID, reason string) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_BearerTokenRevoked{
		BearerTokenRevoked: &corev1.BearerTokenRevokedEvent{
			UserId:  userID,
			Reason:  auditFailureReason(reason),
			Request: auditRequestMetadata(ctx),
		},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(userID), event); err != nil {
		return fmt.Errorf("append bearer token revocation audit event: %w", err)
	}
	return nil
}
