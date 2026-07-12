package core

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var (
	ErrPasskeyNotFound = errors.New("passkey not found")
	ErrPasskeyClaimed  = errors.New("passkey is linked to another account")
)

// Passkey is the durable WebAuthn credential material required by the server.
// It deliberately excludes browser ceremony data, which belongs in runtime state.
type Passkey struct {
	CredentialHash string
	CredentialID   []byte
	Credential     []byte
	Label          string
}

func passkeyHash(credentialID []byte) string {
	sum := sha256.Sum256(credentialID)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (c *ChattoCore) PasskeysForUser(ctx context.Context, userID string) ([]Passkey, error) {
	if err := c.userModel.waitForUsersCurrent(ctx, "passkeys", events.UserAggregate(userID).AllEventsFilter()); err != nil {
		return nil, err
	}
	if _, ok := c.Users.Get(userID); !ok {
		return nil, ErrNotFound
	}
	return c.Users.Passkeys(userID), nil
}

func (c *ChattoCore) LinkPasskey(ctx context.Context, userID string, credentialID, credential []byte, label string) (Passkey, error) {
	userID, label = strings.TrimSpace(userID), strings.TrimSpace(label)
	if userID == "" || len(credentialID) == 0 || len(credential) == 0 {
		return Passkey{}, ErrInvalidArgument
	}
	if label == "" {
		label = "Passkey"
	}
	if len(label) > 64 {
		return Passkey{}, ErrInvalidArgument
	}
	hash := passkeyHash(credentialID)
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserPasskeyLinked{UserPasskeyLinked: &corev1.UserPasskeyLinkedEvent{UserId: userID, CredentialHash: hash, CredentialId: credentialID, Credential: credential, Label: label}}})
	_, err := c.appendUserEvent(ctx, userID, event, events.UserSubjectFilter(), func() error {
		for _, candidateID := range c.Users.UserIDs() {
			for _, existing := range c.Users.Passkeys(candidateID) {
				if existing.CredentialHash == hash && candidateID != userID {
					return ErrPasskeyClaimed
				}
			}
		}
		return nil
	})
	if err != nil {
		return Passkey{}, err
	}
	return Passkey{CredentialHash: hash, CredentialID: append([]byte(nil), credentialID...), Credential: append([]byte(nil), credential...), Label: label}, nil
}

func (c *ChattoCore) UnlinkPasskey(ctx context.Context, userID, credentialHash string) error {
	credentialHash = strings.TrimSpace(credentialHash)
	if userID == "" || credentialHash == "" {
		return ErrInvalidArgument
	}
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserPasskeyUnlinked{UserPasskeyUnlinked: &corev1.UserPasskeyUnlinkedEvent{UserId: userID, CredentialHash: credentialHash}}})
	_, err := c.appendUserEvent(ctx, userID, event, "", func() error {
		passkeys := c.Users.Passkeys(userID)
		found := false
		for _, passkey := range passkeys {
			if passkey.CredentialHash == credentialHash {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: %s", ErrPasskeyNotFound, credentialHash)
		}
		if _, hasPassword := c.Users.PasswordHash(userID); !hasPassword && len(c.Users.ExternalIdentities(userID)) == 0 && !c.Users.HasVerifiedEmail(userID) && len(passkeys) <= 2 {
			return ErrExternalIdentityLastMethod
		}
		return nil
	})
	return err
}
