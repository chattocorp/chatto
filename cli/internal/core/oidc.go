package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var (
	// ErrOIDCSubjectAlreadyClaimed is returned when an OIDC subject is already linked to a different user.
	ErrOIDCSubjectAlreadyClaimed = errors.New("OIDC subject is already linked to another account")
)

// userByOIDCSubjectKey returns the KV key for the OIDC subject-to-user index.
// Uses SHA256 hash of "issuer:subject" to ensure valid NATS subject characters.
func userByOIDCSubjectKey(issuer, subject string) string {
	return fmt.Sprintf("user_by_oidc.%s", oidcSubjectHash(issuer, subject))
}

func oidcSubjectHash(issuer, subject string) string {
	hash := sha256.Sum256([]byte(issuer + ":" + subject))
	return hex.EncodeToString(hash[:])
}

// GetUserByOIDCSubject looks up a user by their OIDC issuer and subject.
func (c *ChattoCore) GetUserByOIDCSubject(ctx context.Context, issuer, subject string) (*corev1.User, error) {
	if user, ok := c.Users.GetByOIDCSubject(issuer, subject); ok {
		return user, nil
	}
	return nil, nil
}

// LinkOIDCSubject links an OIDC subject to a Chatto user. Uses atomic create
// to prevent race conditions. Idempotent if already linked to the same user.
func (c *ChattoCore) LinkOIDCSubject(ctx context.Context, issuer, subject, userID string) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserOidcSubjectLinked{
		UserOidcSubjectLinked: &corev1.UserOIDCSubjectLinkedEvent{
			UserId:      userID,
			Issuer:      issuer,
			Subject:     subject,
			SubjectHash: oidcSubjectHash(issuer, subject),
		},
	}})
	_, err := c.appendUserEvent(ctx, userID, event, events.UserSubjectFilter(), func() error {
		existing, ok := c.Users.GetByOIDCSubject(issuer, subject)
		if ok && existing.GetId() != userID {
			return ErrOIDCSubjectAlreadyClaimed
		}
		return nil
	})
	return err
}
