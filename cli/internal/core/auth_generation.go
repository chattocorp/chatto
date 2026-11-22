package core

import (
	"context"
	"errors"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/pkg/events"
)

var ErrAuthenticationRevoked = errors.New("authentication revoked")

// CurrentAuthGeneration returns the current auth generation of the user with a
// strict auth generation check: it waits until the User Auth projection of
// this replica has applied the newest generation-relevant event of the user on
// the EVT stream leader. It returns ErrAuthenticationRevoked when the user is
// unknown, deleted, or deactivated. An acceptance starts the auth generation
// check window of the user on this replica (ADR-127).
func (c *ChattoCore) CurrentAuthGeneration(ctx context.Context, userID string) (uint64, error) {
	return c.strictAuthGeneration(ctx, userID, strictReasonPath)
}

// strictAuthGeneration runs the strict auth generation check and records its
// result in the window. reason is the metric label for why the check is strict.
func (c *ChattoCore) strictAuthGeneration(ctx context.Context, userID string, reason strictAuthGenerationReason) (uint64, error) {
	if userID == "" {
		return 0, nil
	}
	c.authWindow.strictChecks[reason].Add(1)
	// Take the start time, the reconnect count, and the gate state before the
	// leader reads. The record then never claims a state newer than the one
	// that the check confirmed.
	startedAt := c.authWindow.now()
	reconnects := c.natsReconnects()
	gatesOpen := c.gatesOpenOrCloseAll()
	if err := c.waitForUserAuthGenerationCurrent(ctx, userID); err != nil {
		c.authWindow.clear(userID)
		return 0, err
	}
	generation, active := c.userModel.authGeneration(userID)
	if !active {
		c.authWindow.clear(userID)
		return 0, ErrAuthenticationRevoked
	}
	// A strict check while a gate is closed, or one that a gate closure
	// interrupted, starts no window (ADR-127 section 4).
	if gatesOpen && c.gatesOpenOrCloseAll() {
		c.authWindow.accepted(userID, startedAt, reconnects)
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
//
// It always runs the strict check. Only the read-only first pass of request
// validation can use the fast check; see requireAuthenticationAllowed.
func (c *ChattoCore) RequireAuthenticationAllowed(ctx context.Context, userID string, authGeneration uint64) error {
	return c.requireAuthenticationAllowed(ctx, userID, authGeneration, strictAuthGenerationCheck)
}

// authGenerationCheck selects which auth generation check a caller permits.
type authGenerationCheck bool

const (
	// strictAuthGenerationCheck requires the strict check on every call.
	strictAuthGenerationCheck authGenerationCheck = false
	// windowedAuthGenerationCheck permits the fast check inside the window.
	// Only the read-only first pass of request validation
	// (fastCredentialRead) uses it. That pass cannot reject or delete a
	// credential: every rejection is decided again by the authoritative pass.
	windowedAuthGenerationCheck authGenerationCheck = true
)

// authGenerationCheckFor returns the check that a credential read permits.
func authGenerationCheckFor(read credentialRead) authGenerationCheck {
	if read == fastCredentialRead {
		return windowedAuthGenerationCheck
	}
	return strictAuthGenerationCheck
}

// requireAuthenticationAllowed is the only place that decides between the
// fast and the strict auth generation check (ADR-127).
//
// The fast check reads the local User Auth projection without a request to
// the stream leader. It is used only when all of these are true:
//   - check is windowedAuthGenerationCheck, and ctx does not require the
//     strict check (WithStrictAuthGenerationCheck);
//   - both gates are open (authGenerationGatesOpen);
//   - a strict check accepted the user less than authGenerationCheckWindow
//     ago, on the current NATS connection, and nothing cleared it since;
//   - the projection shows the user as active with the generation
//     authGeneration.
//
// The fast check can only accept. Every other answer runs the strict check,
// and its result is final. Thus no caller acts on a negative answer that did
// not come from a strict check.
func (c *ChattoCore) requireAuthenticationAllowed(ctx context.Context, userID string, authGeneration uint64, check authGenerationCheck) error {
	reason := strictReasonPath
	if check == windowedAuthGenerationCheck && userID != "" && !strictAuthGenerationCheckRequired(ctx) {
		reason = c.fastAuthGenerationCheck(userID, authGeneration)
		if reason == fastCheckAccepted {
			c.authWindow.fastChecks.Add(1)
			return nil
		}
	}
	currentGeneration, err := c.strictAuthGeneration(ctx, userID, reason)
	if err != nil {
		return err
	}
	if authGeneration != currentGeneration {
		// The user is active, but this credential is revoked. Clear the record
		// too, so the next validation of the user is strict.
		c.authWindow.clear(userID)
		return ErrAuthenticationRevoked
	}
	return nil
}

// fastCheckAccepted is the result of fastAuthGenerationCheck when the fast
// check accepts. It is not a strict reason and is never counted as one.
const fastCheckAccepted = strictReasonCount

// fastAuthGenerationCheck returns fastCheckAccepted when the fast check
// accepts the credential, or else the reason why the check must be strict.
func (c *ChattoCore) fastAuthGenerationCheck(userID string, authGeneration uint64) strictAuthGenerationReason {
	if !c.gatesOpenOrCloseAll() {
		return strictReasonGate
	}
	if !c.authWindow.allows(userID, c.natsReconnects()) {
		return strictReasonWindow
	}
	generation, active := c.userModel.authGeneration(userID)
	if !active || generation != authGeneration {
		return strictReasonMismatch
	}
	return fastCheckAccepted
}

// authGenerationGatesOpen reports whether both gates of ADR-127 section 4 are
// open. Gate 1: the NATS connection is connected and the recovery after the
// last reconnect is complete, as ChattoCore.Ready reads it. Gate 2: the User
// Auth projector has completed startup and reports neither failure nor stall.
//
// Some of these signals are slower than the window, for example the NATS ping
// for a silent partition and the stall threshold. ADR-127 accepts this: the
// window, not the gates, bounds the exposure.
func (c *ChattoCore) authGenerationGatesOpen() bool {
	if c.nc == nil || !c.nc.IsConnected() ||
		c.natsRecoveryState.Load() != natsRecoveryReady ||
		c.natsRecoveredReconnects.Load() != c.nc.Stats().Reconnects {
		return false
	}
	if c.userModel == nil || c.userModel.auth.Projector() == nil {
		return false
	}
	return userAuthProjectorGateOpen(c.userModel.auth.Projector().Status())
}

// gatesOpenOrCloseAll reports whether both gates are open. When a gate is
// closed, it ends the window of every user. Every sample of the gates, by a
// fast or a strict check, goes through it. A gate that closes and opens again
// between two samples is not seen.
func (c *ChattoCore) gatesOpenOrCloseAll() bool {
	if c.authGenerationGatesOpen() {
		return true
	}
	c.authWindow.closeAll()
	return false
}

// userAuthProjectorGateOpen is gate 2 of ADR-127: the User Auth projector has
// completed startup and reports neither failure nor stall.
func userAuthProjectorGateOpen(status events.ProjectorStatus) bool {
	return status.StartupComplete && status.Err == nil && !status.Stalled
}

// natsReconnects returns the reconnect count of the NATS connection. A
// window record is valid only for the count at which its strict check started.
func (c *ChattoCore) natsReconnects() uint64 {
	if c.nc == nil {
		return 0
	}
	return c.nc.Stats().Reconnects
}

// AuthGenerationCheckMetrics returns the auth generation check counters of
// this process.
func (c *ChattoCore) AuthGenerationCheckMetrics() AuthGenerationCheckMetrics {
	return c.authWindow.metrics()
}

type strictAuthGenerationCheckKey struct{}

// WithStrictAuthGenerationCheck returns a context in which every auth
// generation check is strict, also in the first pass of request validation.
// The periodic credential validation of a realtime connection uses it, so a
// connection on a replica that is behind cannot stay open for one timer
// interval plus one window (ADR-127 section 5).
func WithStrictAuthGenerationCheck(ctx context.Context) context.Context {
	return context.WithValue(ctx, strictAuthGenerationCheckKey{}, true)
}

func strictAuthGenerationCheckRequired(ctx context.Context) bool {
	required, _ := ctx.Value(strictAuthGenerationCheckKey{}).(bool)
	return required
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
	if c.userModel == nil {
		return nil
	}
	subjects := userAuthGenerationSubjects(userID)
	return c.userModel.waitForUserProjectionsCurrent(ctx, "user auth generation", subjects[:usersProjectionAuthSubjects], subjects)
}
