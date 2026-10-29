package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// errExternalIdentityAlreadyLinked ends an idempotent link before publication.
var errExternalIdentityAlreadyLinked = errors.New("external identity already linked to this account")

var (
	// ErrExternalIdentityAlreadyClaimed is returned when an external identity is already linked to a different user.
	ErrExternalIdentityAlreadyClaimed = errors.New("external identity is already linked to another account")
)

func externalIdentityHash(issuer, subject string) string {
	hash := sha256.Sum256([]byte(issuer + ":" + subject))
	return hex.EncodeToString(hash[:])
}

// GetUserByExternalIdentity looks up a user by provider issuer namespace and subject.
func (c *ChattoCore) GetUserByExternalIdentity(ctx context.Context, issuer, subject string) (*readmodel.User, error) {
	user, ok, err := c.userModel.userByExternalIdentity(ctx, issuer, subject)
	if err != nil {
		return nil, err
	}
	if ok {
		return user, nil
	}
	return nil, nil
}

// GetUserByExternalIdentityForAuthentication returns the mapped user and the
// authentication generation that was current in the same projection snapshot
// as the identity mapping. Credential issuance must use that generation.
func (c *ChattoCore) GetUserByExternalIdentityForAuthentication(ctx context.Context, issuer, subject string) (*readmodel.User, uint64, error) {
	user, authGeneration, ok, err := c.userModel.userByExternalIdentityForAuthentication(ctx, issuer, subject)
	if err != nil {
		return nil, 0, err
	}
	if !ok {
		return nil, 0, nil
	}
	return user, authGeneration, nil
}

// errLegacyIdentityCompleteOrMissing ends completion without writing when a
// concurrent operation completed or removed the link.
var errLegacyIdentityCompleteOrMissing = errors.New("legacy identity is complete or missing")

// CompleteLegacyOIDCIdentityLink records metadata for an existing hash-only
// link after the caller verifies the OIDC issuer and subject. It never creates
// a missing link or overwrites a complete one. The user OCC check prevents a
// concurrent unlink from being undone; a successful write waits for projections.
// Repeat calls are idempotent.
func (c *ChattoCore) CompleteLegacyOIDCIdentityLink(ctx context.Context, userID, providerID, issuer, subject string) error {
	if userID == "" || providerID == "" || issuer == "" || subject == "" {
		return ErrInvalidArgument
	}
	hash := externalIdentityHash(issuer, subject)
	incomplete := func() bool {
		return slices.ContainsFunc(c.userModel.externalIdentities(userID), func(identity ExternalIdentity) bool {
			return identity.SubjectHash == hash && identity.Issuer == ""
		})
	}
	if !incomplete() {
		return nil
	}
	event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_UserExternalIdentityLinked{
		UserExternalIdentityLinked: &evtv1.UserExternalIdentityLinkedEvent{
			UserId:       userID,
			Issuer:       issuer,
			Subject:      subject,
			SubjectHash:  hash,
			ProviderId:   providerID,
			ProviderType: config.AuthProviderTypeOpenIDConnect,
		},
	}})
	_, err := c.appendUserEvent(ctx, userID, event, evtstream.UserSubjectFilter(), func() error {
		if !incomplete() {
			return errLegacyIdentityCompleteOrMissing
		}
		return nil
	})
	if errors.Is(err, errLegacyIdentityCompleteOrMissing) {
		return nil
	}
	return err
}

// LinkExternalIdentity links a verified provider subject to a user.
// The issuer is the durable identity namespace: the verified OIDC issuer URL
// for OIDC providers and the stable configured provider ID for OAuth-only
// providers. providerID/providerType are event-time metadata and are not used
// for lookup, so config changes do not break existing links. Idempotent for the same user.
func (c *ChattoCore) LinkExternalIdentity(ctx context.Context, providerID, providerType, issuer, subject, userID string) error {
	_, err := c.LinkExternalIdentityAs(ctx, userID, providerID, providerType, issuer, subject, userID)
	return err
}

// LinkExternalIdentityAs links an operator-verified or provider-verified identity
// with explicit actor attribution. It checks ownership across all users and
// waits for authentication projection catch-up. Repeating the same link does
// not append another event. Issuer and subject are exact, opaque identifiers.
func (c *ChattoCore) LinkExternalIdentityAs(ctx context.Context, actorID, providerID, providerType, issuer, subject, userID string) (ExternalIdentity, error) {
	if actorID == "" {
		return ExternalIdentity{}, ErrNotAuthenticated
	}
	for _, value := range []string{providerID, providerType, issuer, subject, userID} {
		if strings.TrimSpace(value) == "" {
			return ExternalIdentity{}, ErrInvalidArgument
		}
	}
	identity := ExternalIdentity{ProviderID: providerID, ProviderType: providerType, Issuer: issuer, Subject: subject, SubjectHash: externalIdentityHash(issuer, subject)}
	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_UserExternalIdentityLinked{
		UserExternalIdentityLinked: &evtv1.UserExternalIdentityLinkedEvent{
			UserId:       userID,
			Issuer:       issuer,
			Subject:      subject,
			SubjectHash:  identity.SubjectHash,
			ProviderId:   providerID,
			ProviderType: providerType,
		},
	}})
	// Check the account after appendUserEvent's projection wait, so a replica
	// that has not yet applied the user's creation does not reject it.
	_, err := c.appendUserEvent(ctx, userID, event, evtstream.UserSubjectFilter(), func() error {
		if err := c.requireHumanUser(ctx, userID); err != nil {
			return err
		}
		existingUserID, claimed := c.userModel.externalIdentityOwnerID(issuer, subject)
		if claimed && existingUserID != userID {
			return ErrExternalIdentityAlreadyClaimed
		}
		if claimed {
			for _, stored := range c.userModel.externalIdentities(userID) {
				if stored.SubjectHash == identity.SubjectHash {
					identity = stored
					break
				}
			}
			return errExternalIdentityAlreadyLinked
		}
		if err := c.requireVerifiedAccountCapacity(ctx, userID); err != nil {
			return err
		}
		return nil
	})
	if errors.Is(err, errExternalIdentityAlreadyLinked) {
		return identity, nil
	}
	if err != nil {
		return ExternalIdentity{}, err
	}
	return identity, nil
}
