package core

import (
	"context"
	"fmt"
	"regexp"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// MaxAtprotoDIDLength bounds a published AT Protocol DID. The AT Protocol
// specification limits DIDs to 2 KB.
const MaxAtprotoDIDLength = 2048

// atprotoDIDPattern accepts the DID methods that AT Protocol supports. The
// server checks only syntax: clients verify the claim against the DID's
// repository (ADR-123).
var atprotoDIDPattern = regexp.MustCompile(`^did:(plc|web):[A-Za-z0-9._:%-]*[A-Za-z0-9._-]$`)

// ErrInvalidAtprotoDID reports a DID that is not a syntactically valid
// did:plc or did:web identifier.
var ErrInvalidAtprotoDID = fmt.Errorf("%w: invalid AT Protocol DID", ErrInvalidArgument)

// ValidateAtprotoDID reports whether did is a syntactically valid DID of a
// method that AT Protocol supports.
func ValidateAtprotoDID(did string) error {
	if len(did) > MaxAtprotoDIDLength || !atprotoDIDPattern.MatchString(did) {
		return ErrInvalidAtprotoDID
	}
	return nil
}

// SetUserAtprotoDID publishes did on the user's public profile. The DID is an
// unverified claim (FDR-056). Setting the current DID again appends no event.
func (c *ChattoCore) SetUserAtprotoDID(ctx context.Context, userID, did string) (string, error) {
	if err := ValidateAtprotoDID(did); err != nil {
		return "", err
	}
	return did, c.changeUserAtprotoDID(ctx, userID, did)
}

// ClearUserAtprotoDID removes the published DID from the user's profile. It is
// idempotent: without a published DID, it appends no event.
func (c *ChattoCore) ClearUserAtprotoDID(ctx context.Context, userID string) error {
	return c.changeUserAtprotoDID(ctx, userID, "")
}

// changeUserAtprotoDID appends one UserAtprotoDidChangedEvent when next differs
// from the projected DID. An empty next removes the DID.
func (c *ChattoCore) changeUserAtprotoDID(ctx context.Context, userID, next string) error {
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	if user.GetAtprotoDID() == next {
		return nil
	}

	event := newEvent(userID, &evtv1.Event{Event: &evtv1.Event_UserAtprotoDidChanged{
		UserAtprotoDidChanged: &evtv1.UserAtprotoDidChangedEvent{UserId: userID},
	}})
	if next != "" {
		encrypted, err := c.encryptUserPIIString(ctx, event.GetId(), userID, evtstream.EventUserAtprotoDIDChanged, "atproto_did", next)
		if err != nil {
			return fmt.Errorf("encrypt AT Protocol DID: %w", err)
		}
		event.GetUserAtprotoDidChanged().EncryptedDid = encrypted
	}
	// A concurrent change can make this event redundant. Both orders leave the
	// last requested value, so a retry after a conflict is safe.
	if _, err := c.appendUserEvent(ctx, userID, event, "", nil); err != nil {
		return fmt.Errorf("failed to store AT Protocol DID: %w", err)
	}
	return nil
}
