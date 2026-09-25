package core

import (
	"context"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/evtstream"
)

var ErrAuthenticationRevoked = errors.New("authentication revoked")

func (c *ChattoCore) CurrentAuthGeneration(ctx context.Context, userID string) (uint64, error) {
	if userID == "" {
		return 0, nil
	}
	if err := c.waitForUserAuthGenerationCurrent(ctx, userID); err != nil {
		return 0, err
	}
	generation, active := c.userModel.authGeneration(userID)
	if !active {
		return 0, ErrAuthenticationRevoked
	}
	return generation, nil
}

// RequireAuthenticationAllowed is the single auth-generation policy gate for
// runtime credentials. It returns ErrAuthenticationRevoked unless authGeneration
// is the user's current auth generation. Issuance passes the generation that the
// authentication proved. Validation passes the generation that the stored
// credential records. Every credential record that validation accepts records
// the generation it was issued against, so any mismatch means that a later
// password, account, or identity event revoked the credential.
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

func (c *ChattoCore) waitForUserAuthGenerationCurrent(ctx context.Context, userID string) error {
	if c.userModel == nil {
		return nil
	}
	agg := evtstream.UserAggregate(userID)
	if err := c.userModel.waitForUsersCurrent(ctx, "user auth generation",
		agg.Subject(evtstream.EventUserPasswordHashChanged),
		agg.Subject(evtstream.EventUserExternalIdentityUnlinked),
		agg.Subject(evtstream.EventUserAccountDeleted),
	); err != nil {
		return fmt.Errorf("wait for user auth generation: %w", err)
	}
	return nil
}
