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
	UserID         string
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
	userEvent := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserPasskeyLinked{UserPasskeyLinked: &corev1.UserPasskeyLinkedEvent{UserId: userID, CredentialHash: hash, Label: label}}})
	credentialEvent := newEvent(userID, &corev1.Event{Event: &corev1.Event_PasskeyCredentialRegistered{PasskeyCredentialRegistered: &corev1.PasskeyCredentialRegisteredEvent{UserId: userID, CredentialHash: hash, CredentialId: credentialID, Credential: credential}}})
	passkeyAgg := events.PasskeyAggregate(hash)
	for attempt := 0; attempt < maxUserMutationRetries; attempt++ {
		userAgg := events.UserAggregate(userID)
		userSeq, err := c.EventPublisher.LastSubjectSeq(ctx, userAgg.AllEventsFilter())
		if err != nil {
			return Passkey{}, fmt.Errorf("read user passkey OCC: %w", err)
		}
		credentialSeq, err := c.EventPublisher.LastSubjectSeq(ctx, passkeyAgg.AllEventsFilter())
		if err != nil {
			return Passkey{}, fmt.Errorf("read credential passkey OCC: %w", err)
		}
		if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(userAgg.AllEventsFilter(), userSeq)); err != nil {
			return Passkey{}, err
		}
		if _, ok := c.Users.Get(userID); !ok {
			return Passkey{}, ErrNotFound
		}
		if err := c.PasskeysProjector.WaitFor(ctx, events.SubjectPosition(passkeyAgg.AllEventsFilter(), credentialSeq)); err != nil {
			return Passkey{}, err
		}
		if existing, ok := c.Passkeys.Get(hash); ok {
			if existing.UserID != userID {
				return Passkey{}, ErrPasskeyClaimed
			}
			return existing, nil
		}
		entries := []events.BatchEntry{{Subject: userAgg.Subject(events.EventUserPasskeyLinked), Event: userEvent, HasOCC: true, ExpectedSeq: userSeq, FilterSubject: userAgg.AllEventsFilter()}, {Subject: passkeyAgg.Subject(events.EventPasskeyCredentialRegistered), Event: credentialEvent, HasOCC: true, ExpectedSeq: credentialSeq, FilterSubject: passkeyAgg.AllEventsFilter()}}
		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(entries[0].Subject, seqs[0])); err != nil {
				return Passkey{}, err
			}
			if err := c.PasskeysProjector.WaitFor(ctx, events.SubjectPosition(entries[1].Subject, seqs[1])); err != nil {
				return Passkey{}, err
			}
			return Passkey{UserID: userID, CredentialHash: hash, CredentialID: append([]byte(nil), credentialID...), Credential: append([]byte(nil), credential...), Label: label}, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return Passkey{}, err
		}
	}
	return Passkey{}, fmt.Errorf("passkey link OCC retry exhausted: %w", events.ErrConflict)
}

func (c *ChattoCore) UnlinkPasskey(ctx context.Context, userID, credentialHash string) error {
	credentialHash = strings.TrimSpace(credentialHash)
	if userID == "" || credentialHash == "" {
		return ErrInvalidArgument
	}
	userEvent := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserPasskeyUnlinked{UserPasskeyUnlinked: &corev1.UserPasskeyUnlinkedEvent{UserId: userID, CredentialHash: credentialHash}}})
	credentialEvent := newEvent(userID, &corev1.Event{Event: &corev1.Event_PasskeyCredentialRemoved{PasskeyCredentialRemoved: &corev1.PasskeyCredentialRemovedEvent{CredentialHash: credentialHash}}})
	userAgg, passkeyAgg := events.UserAggregate(userID), events.PasskeyAggregate(credentialHash)
	for attempt := 0; attempt < maxUserMutationRetries; attempt++ {
		userSeq, err := c.EventPublisher.LastSubjectSeq(ctx, userAgg.AllEventsFilter())
		if err != nil {
			return err
		}
		credentialSeq, err := c.EventPublisher.LastSubjectSeq(ctx, passkeyAgg.AllEventsFilter())
		if err != nil {
			return err
		}
		if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(userAgg.AllEventsFilter(), userSeq)); err != nil {
			return err
		}
		if err := c.PasskeysProjector.WaitFor(ctx, events.SubjectPosition(passkeyAgg.AllEventsFilter(), credentialSeq)); err != nil {
			return err
		}
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
		entries := []events.BatchEntry{{Subject: userAgg.Subject(events.EventUserPasskeyUnlinked), Event: userEvent, HasOCC: true, ExpectedSeq: userSeq, FilterSubject: userAgg.AllEventsFilter()}, {Subject: passkeyAgg.Subject(events.EventPasskeyCredentialRemoved), Event: credentialEvent, HasOCC: true, ExpectedSeq: credentialSeq, FilterSubject: passkeyAgg.AllEventsFilter()}}
		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(entries[0].Subject, seqs[0])); err != nil {
				return err
			}
			return c.PasskeysProjector.WaitFor(ctx, events.SubjectPosition(entries[1].Subject, seqs[1]))
		}
		if !errors.Is(err, events.ErrConflict) {
			return err
		}
	}
	return fmt.Errorf("passkey unlink OCC retry exhausted: %w", events.ErrConflict)
}

// recordUserDeletionAndReleasePasskeys atomically tombstones the account and
// all of its credential aggregates. This releases credential hashes without
// relying on eventual cross-projection cleanup after deletion.
func (c *ChattoCore) recordUserDeletionAndReleasePasskeys(ctx context.Context, userID string, deletedEvent *corev1.Event) error {
	userAgg := events.UserAggregate(userID)
	for attempt := 0; attempt < maxUserMutationRetries; attempt++ {
		userSeq, err := c.EventPublisher.LastSubjectSeq(ctx, userAgg.AllEventsFilter())
		if err != nil {
			return err
		}
		if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(userAgg.AllEventsFilter(), userSeq)); err != nil {
			return err
		}
		links := c.Users.Passkeys(userID)
		entries := []events.BatchEntry{{Subject: userAgg.Subject(events.EventUserAccountDeleted), Event: deletedEvent, HasOCC: true, ExpectedSeq: userSeq, FilterSubject: userAgg.AllEventsFilter()}}
		for _, link := range links {
			agg := events.PasskeyAggregate(link.CredentialHash)
			seq, err := c.EventPublisher.LastSubjectSeq(ctx, agg.AllEventsFilter())
			if err != nil {
				return err
			}
			if err := c.PasskeysProjector.WaitFor(ctx, events.SubjectPosition(agg.AllEventsFilter(), seq)); err != nil {
				return err
			}
			entries = append(entries, events.BatchEntry{Subject: agg.Subject(events.EventPasskeyCredentialRemoved), Event: newEvent(userID, &corev1.Event{Event: &corev1.Event_PasskeyCredentialRemoved{PasskeyCredentialRemoved: &corev1.PasskeyCredentialRemovedEvent{CredentialHash: link.CredentialHash}}}), HasOCC: true, ExpectedSeq: seq, FilterSubject: agg.AllEventsFilter()})
		}
		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(entries[0].Subject, seqs[0])); err != nil {
				return err
			}
			for i := 1; i < len(entries); i++ {
				if err := c.PasskeysProjector.WaitFor(ctx, events.SubjectPosition(entries[i].Subject, seqs[i])); err != nil {
					return err
				}
			}
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return err
		}
	}
	return fmt.Errorf("user deletion passkey OCC retry exhausted: %w", events.ErrConflict)
}
