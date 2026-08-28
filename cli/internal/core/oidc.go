package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

var (
	// ErrOIDCSubjectAlreadyClaimed is returned when an OIDC subject is already linked to a different user.
	ErrOIDCSubjectAlreadyClaimed = errors.New("OIDC subject is already linked to another account")
	// ErrExternalIdentityAlreadyClaimed is returned when an external identity is already linked to a different user.
	ErrExternalIdentityAlreadyClaimed = errors.New("external identity is already linked to another account")
)

// userByOIDCSubjectKey returns the KV key for the OIDC subject-to-user index.
// Uses SHA256 hash of "issuer:subject" to ensure valid NATS subject characters.
func userByOIDCSubjectKey(issuer, subject string) string {
	return fmt.Sprintf("user_by_oidc.%s", oidcSubjectHash(issuer, subject))
}

func oidcSubjectHash(issuer, subject string) string {
	return externalIdentityHash(issuer, subject)
}

func externalIdentityHash(issuer, subject string) string {
	hash := sha256.Sum256([]byte(issuer + ":" + subject))
	return hex.EncodeToString(hash[:])
}

// GetUserByExternalIdentity looks up a user by provider issuer namespace and subject.
func (c *ChattoCore) GetUserByExternalIdentity(ctx context.Context, issuer, subject string) (*evtv1.User, error) {
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
func (c *ChattoCore) GetUserByExternalIdentityForAuthentication(ctx context.Context, issuer, subject string) (*evtv1.User, uint64, error) {
	user, authGeneration, ok, err := c.userModel.userByExternalIdentityForAuthentication(ctx, issuer, subject)
	if err != nil {
		return nil, 0, err
	}
	if !ok {
		return nil, 0, nil
	}
	return user, authGeneration, nil
}

// GetUserByOIDCSubject looks up a user by their OIDC issuer and subject.
func (c *ChattoCore) GetUserByOIDCSubject(ctx context.Context, issuer, subject string) (*evtv1.User, error) {
	return c.GetUserByExternalIdentity(ctx, issuer, subject)
}

// LinkExternalIdentity links a verified provider subject to a user.
// The issuer is the durable identity namespace: the verified OIDC issuer URL
// for OIDC providers and the stable configured provider ID for OAuth-only
// providers. providerID/providerType are event-time metadata and are not used
// for lookup, so config changes do not break existing links. Idempotent for the same user.
func (c *ChattoCore) LinkExternalIdentity(ctx context.Context, providerID, providerType, issuer, subject, userID string) error {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return err
	}
	event := newEvent(userID, &evtv1.Event{Event: &evtv1.Event_UserExternalIdentityLinked{
		UserExternalIdentityLinked: &evtv1.UserExternalIdentityLinkedEvent{
			UserId:       userID,
			Issuer:       issuer,
			Subject:      subject,
			SubjectHash:  externalIdentityHash(issuer, subject),
			ProviderId:   providerID,
			ProviderType: providerType,
		},
	}})
	_, err := c.appendUserEvent(ctx, userID, event, evtstream.UserSubjectFilter(), func() error {
		_, ok, err := c.userModel.user(ctx, userID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
		existingUserID, claimed := c.userModel.externalIdentityOwnerID(issuer, subject)
		if claimed && existingUserID != userID {
			return ErrExternalIdentityAlreadyClaimed
		}
		if !claimed {
			if err := c.requireVerifiedAccountCapacity(ctx, userID); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

// LinkOIDCSubject links an OIDC subject to a user. Uses atomic create
// to prevent race conditions. Idempotent if already linked to the same user.
func (c *ChattoCore) LinkOIDCSubject(ctx context.Context, issuer, subject, userID string) error {
	err := c.LinkExternalIdentity(ctx, "oidc", "oidc", issuer, subject, userID)
	if errors.Is(err, ErrExternalIdentityAlreadyClaimed) {
		return ErrOIDCSubjectAlreadyClaimed
	}
	return err
}
