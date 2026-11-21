package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/parallel"
	"github.com/chattocorp/chatto/pkg/events"
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

// userAuthGenerationSubjects returns the subjects of the events of one user
// that can change the result of UserAuthProjection.AuthGeneration: the events
// that create the projected user, that delete or deactivate it, and that
// start a new generation. Keep this list complete when an event gets one of
// these effects. A missing subject lets a replica accept a revoked credential,
// or reject a valid one, until its projection applies the event.
//
// The first usersProjectionAuthSubjects subjects are those for which the
// check also waits for the Users projection.
func userAuthGenerationSubjects(userID string) []string {
	agg := evtstream.UserAggregate(userID)
	return []string{
		agg.Subject(evtstream.EventUserPasswordHashChanged),
		agg.Subject(evtstream.EventUserExternalIdentityUnlinked),
		agg.Subject(evtstream.EventUserAccountDeleted),
		agg.Subject(evtstream.EventUserAccountDeactivationChanged),
		agg.Subject(evtstream.EventUserAccountCreated),
		agg.Subject(evtstream.EventUserKeyShreddingRequested),
		agg.Subject(evtstream.EventUserKeyShredded),
	}
}

const usersProjectionAuthSubjects = 4

// waitForUserAuthGenerationCurrent waits until the projections have applied
// the newest event of this user that can change the auth generation. Every
// validation of a runtime credential calls it, so it reads only the subjects
// of this user and reads them concurrently. It does not wait for events of
// other users or for other event types of the User Auth projection.
func (c *ChattoCore) waitForUserAuthGenerationCurrent(ctx context.Context, userID string) error {
	m := c.userModel
	if m == nil || m.publisher == nil || m.users.Projector() == nil {
		return nil
	}
	positions, err := parallel.Map(ctx, 0, userAuthGenerationSubjects(userID), func(ctx context.Context, _ int, subject string) (events.StreamPosition, error) {
		return m.publisher.LastSubjectPosition(ctx, subject)
	})
	if err != nil {
		return fmt.Errorf("wait for user auth generation: read projection target seq: %w", err)
	}
	if target := newestStreamPosition(positions[:usersProjectionAuthSubjects]); !target.IsZero() {
		if err := m.users.Projector().WaitFor(ctx, target); err != nil {
			return fmt.Errorf("wait for user auth generation: wait for users projection: %w", err)
		}
	}
	if m.auth.Projector() == nil || m.auth.Projection() == nil {
		return nil
	}
	if target := newestStreamPosition(positions); !target.IsZero() {
		if err := m.auth.Projector().WaitFor(ctx, target); err != nil {
			return fmt.Errorf("wait for user auth generation: wait for user auth projection: %w", err)
		}
	}
	return nil
}
