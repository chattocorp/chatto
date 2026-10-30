package accounts

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/chattocorp/chatto/authling/internal/ids"
	corev1 "github.com/chattocorp/chatto/authling/internal/pb/authling/core/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AdministratorAccess distinguishes durable assignments from operator overrides.
type AdministratorAccess struct {
	Stored     bool
	Configured bool
}

// Allowed reports whether either source grants administration access.
func (a AdministratorAccess) Allowed() bool { return a.Stored || a.Configured }

// Administrator checks current durable state, never a flag cached in a session.
// Only an active, verified email claim can match the operator override. A staged
// email change denies that override until the new claim is active.
func (s *Service) Administrator(ctx context.Context, id string, adminEmails []string) (AdministratorAccess, error) {
	if err := s.RequireActive(ctx, id); err != nil {
		return AdministratorAccess{}, err
	}
	p := s.handle.Projection()
	p.RLock()
	defer p.RUnlock()
	account, ok := p.accounts[id]
	if !ok {
		return AdministratorAccess{}, ErrInvalidCredentials
	}
	access := AdministratorAccess{Stored: account.IsAdministrator}
	credential, exists := p.credentials[id]
	_, staged := p.pendingEmails[id]
	if exists && !staged && p.emails[credential.emailDigest] == id {
		for _, email := range adminEmails {
			if credential.emailDigest == digest(p.indexKey, NormalizeEmail(email)) {
				access.Configured = true
				break
			}
		}
	}
	return access, nil
}

// SetAdministrator is a trusted internal command, not a public self-service API.
// A future administrative mutation surface must authorize and reauthenticate its
// caller and prevent last-administrator lockout before calling this command.
// This read-only PoC exposes no HTTP or CLI operation that changes the flag.
func (s *Service) SetAdministrator(ctx context.Context, id string, enabled bool) error {
	for range 5 {
		if err := s.RequireActive(ctx, id); err != nil {
			return err
		}
		credential, ok := s.handle.Projection().credentialForIdentityMutation(id)
		if !ok {
			return ErrCredentialChanged
		}
		tail, _, err := s.identityCredentialAtTail(ctx, id, credential.eventID)
		if err != nil {
			return err
		}
		eventID, err := ids.New("evt")
		if err != nil {
			return err
		}
		event := &corev1.Event{Id: eventID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_AdministratorChanged{AdministratorChanged: &corev1.AdministratorChangedEvent{AccountId: id, IsAdministrator: enabled}}}
		position, err := s.publisher.AppendAdministratorChanged(ctx, event, tail)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		return s.handle.Projector().WaitFor(ctx, position)
	}
	return fmt.Errorf("administrator change conflict")
}

// RegisteredAccount contains protected display data for the administrator list.
// It must not be logged or returned to unauthenticated or ordinary accounts.
type RegisteredAccount struct {
	Account
	Email   string
	Profile Profile
	Access  AdministratorAccess
}

// RegisteredAccounts returns at most limit active accounts ordered by opaque ID.
// Callers must authorize administrator access first. Only the selected page is
// decrypted. Accounts erased during the read are omitted; storage failures fail
// the whole page. The cursor is an exclusive account ID, with no personal data.
func (s *Service) RegisteredAccounts(ctx context.Context, after string, limit int, adminEmails []string) ([]RegisteredAccount, string, error) {
	if limit < 1 || limit > 100 {
		return nil, "", fmt.Errorf("invalid account page size")
	}
	p := s.handle.Projection()
	p.RLock()
	ids := make([]string, 0, len(p.accounts))
	for id := range p.accounts {
		if id > after {
			ids = append(ids, id)
		}
	}
	p.RUnlock()
	slices.SortFunc(ids, strings.Compare)
	next := ""
	if len(ids) > limit {
		ids = ids[:limit]
		next = ids[len(ids)-1]
	}
	rows := make([]RegisteredAccount, 0, len(ids))
	for _, id := range ids {
		email, err := s.EmailAddress(ctx, id)
		if errors.Is(err, ErrInvalidCredentials) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		profile, err := s.Profile(ctx, id)
		if errors.Is(err, ErrInvalidCredentials) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		access, err := s.Administrator(ctx, id, adminEmails)
		if errors.Is(err, ErrInvalidCredentials) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		account, ok := s.Get(id)
		if ok {
			rows = append(rows, RegisteredAccount{Account: account, Email: email, Profile: profile, Access: access})
		}
	}
	return rows, next, nil
}
