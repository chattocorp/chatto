package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/events"
)

var ErrAuthenticationRevoked = errors.New("authentication revoked")

// RuntimeCredential is the storage-independent identity and generation data
// carried by bearer tokens, cookie sessions, and OAuth authorization codes.
type RuntimeCredential struct {
	UserID         string
	CreatedAt      time.Time
	AuthGeneration uint64
}

// RuntimeCredentialValidation is the result of validating a runtime credential
// against the user's current auth generation.
type RuntimeCredentialValidation struct {
	UserID                      string
	AuthGeneration              uint64
	ShouldPersistAuthGeneration bool
}

func (c *ChattoCore) CurrentAuthGeneration(ctx context.Context, userID string) (uint64, error) {
	if userID == "" {
		return 0, nil
	}
	if err := c.waitForUserAuthGenerationCurrent(ctx, userID); err != nil {
		return 0, err
	}
	generation, active := c.Users.AuthGeneration(userID)
	if !active {
		return 0, ErrAuthenticationRevoked
	}
	return generation, nil
}

// RequireAuthenticationAllowed rejects credential issuance that proved
// authentication against an older user auth generation.
func (c *ChattoCore) RequireAuthenticationAllowed(ctx context.Context, userID string, authGeneration uint64) error {
	currentGeneration, err := c.CurrentAuthGeneration(ctx, userID)
	if err != nil {
		return err
	}
	if authGeneration != currentGeneration {
		return ErrAuthenticationRevoked
	}
	return nil
}

// ValidateRuntimeCredential is the single policy gate for persisted runtime
// credentials. Storage-specific callers load their record, pass the common
// credential fields here, and persist AuthGeneration when ShouldPersist is true.
//
// Credentials written before auth_generation existed unmarshal as generation 0.
// For compatibility, those records are grandfathered when their CreatedAt is
// not older than the user's current password hash event. Legacy imported
// password hashes only have the legacy user record timestamp, so this
// intentionally preserves upgraded 0.0.x credentials until a new 0.1.x password
// change/reset advances the generation.
func (c *ChattoCore) ValidateRuntimeCredential(ctx context.Context, credential RuntimeCredential) (RuntimeCredentialValidation, error) {
	currentGeneration, err := c.CurrentAuthGeneration(ctx, credential.UserID)
	if err != nil {
		return RuntimeCredentialValidation{}, err
	}
	if credential.AuthGeneration == currentGeneration {
		return RuntimeCredentialValidation{
			UserID:         credential.UserID,
			AuthGeneration: currentGeneration,
		}, nil
	}
	if credential.AuthGeneration != 0 {
		return RuntimeCredentialValidation{}, ErrAuthenticationRevoked
	}
	if currentGeneration == 0 {
		return RuntimeCredentialValidation{
			UserID:         credential.UserID,
			AuthGeneration: currentGeneration,
		}, nil
	}

	_, passwordSetAt, hasPassword := c.Users.PasswordHashWithSetAt(credential.UserID)
	if !hasPassword || credential.CreatedAt.IsZero() || credential.CreatedAt.Before(passwordSetAt) {
		return RuntimeCredentialValidation{}, ErrAuthenticationRevoked
	}
	return RuntimeCredentialValidation{
		UserID:                      credential.UserID,
		AuthGeneration:              currentGeneration,
		ShouldPersistAuthGeneration: true,
	}, nil
}

func (c *ChattoCore) waitForUserAuthGenerationCurrent(ctx context.Context, userID string) error {
	if c.userService == nil {
		return nil
	}
	agg := events.UserAggregate(userID)
	if err := c.userService.waitForUsersCurrent(ctx, "user auth generation",
		agg.Subject(events.EventUserPasswordHashChanged),
		agg.Subject(events.EventUserAccountDeleted),
	); err != nil {
		return fmt.Errorf("wait for user auth generation: %w", err)
	}
	return nil
}
