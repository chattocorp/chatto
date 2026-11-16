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
	return AdministratorAccess{Stored: account.IsAdministrator, Configured: p.configuredAdministrator(id, p.adminEmailDigests(adminEmails))}, nil
}

// adminEmailDigests converts the configured override list to email-index
// digests. The caller must hold the projection lock.
func (p *Projection) adminEmailDigests(adminEmails []string) [][32]byte {
	digests := make([][32]byte, 0, len(adminEmails))
	for _, email := range adminEmails {
		digests = append(digests, digest(p.indexKey, NormalizeEmail(email)))
	}
	return digests
}

// configuredAdministrator reports whether the account's active, verified email
// matches one of the configured override digests. A staged email change denies
// the override. The caller must hold the projection lock.
func (p *Projection) configuredAdministrator(id string, adminDigests [][32]byte) bool {
	credential, exists := p.credentials[id]
	_, staged := p.pendingEmails[id]
	if !exists || staged || p.emails[credential.emailDigest] != id {
		return false
	}
	return slices.Contains(adminDigests, credential.emailDigest)
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

// DirectorySort selects the column that orders the account directory.
type DirectorySort string

const (
	// SortByEmail orders by case-folded verified email.
	SortByEmail DirectorySort = "email"
	// SortByName orders by case-folded full name, or preferred username when
	// the account has no full name. This is the name the directory displays.
	SortByName DirectorySort = "name"
	// SortByAccess puts administrators before users in ascending order.
	SortByAccess DirectorySort = "access"
	// SortByRegistered orders by account creation time.
	SortByRegistered DirectorySort = "registered"
	// SortByID orders by opaque account ID.
	SortByID DirectorySort = "id"
)

// Valid reports whether the sort is a known directory column.
func (s DirectorySort) Valid() bool {
	switch s {
	case SortByEmail, SortByName, SortByAccess, SortByRegistered, SortByID:
		return true
	}
	return false
}

// DirectoryQuery selects one page of the account directory. Account ID breaks
// ties in every order, so pages are deterministic while the directory is
// unchanged. Offsets shift when accounts register or are erased between pages.
type DirectoryQuery struct {
	Sort       DirectorySort
	Descending bool
	Offset     int
	Limit      int
}

// RegisteredAccounts returns at most query.Limit active accounts and reports
// whether more accounts follow. Callers must authorize administrator access
// first. Access, registration, and ID orders decrypt only the selected page.
// Email and name orders must decrypt that field for every account on each
// request. Accounts erased during the read are omitted; storage failures fail
// the whole page.
func (s *Service) RegisteredAccounts(ctx context.Context, query DirectoryQuery, adminEmails []string) ([]RegisteredAccount, bool, error) {
	if query.Limit < 1 || query.Limit > 100 {
		return nil, false, fmt.Errorf("invalid account page size")
	}
	if query.Offset < 0 {
		return nil, false, fmt.Errorf("invalid account page offset")
	}
	if !query.Sort.Valid() {
		return nil, false, fmt.Errorf("invalid account sort")
	}
	type entry struct {
		account Account
		admin   bool
		key     string
	}
	p := s.handle.Projection()
	p.RLock()
	digests := p.adminEmailDigests(adminEmails)
	entries := make([]entry, 0, len(p.accounts))
	for id, account := range p.accounts {
		entries = append(entries, entry{account: account, admin: account.IsAdministrator || p.configuredAdministrator(id, digests)})
	}
	p.RUnlock()
	if query.Sort == SortByEmail || query.Sort == SortByName {
		kept := entries[:0]
		for _, e := range entries {
			key, err := s.directorySortKey(ctx, query.Sort, e.account.ID)
			if errors.Is(err, ErrInvalidCredentials) {
				continue
			}
			if err != nil {
				return nil, false, err
			}
			e.key = key
			kept = append(kept, e)
		}
		entries = kept
	}
	slices.SortFunc(entries, func(a, b entry) int {
		c := 0
		switch query.Sort {
		case SortByEmail, SortByName:
			c = strings.Compare(a.key, b.key)
		case SortByAccess:
			c = compareBool(b.admin, a.admin)
		case SortByRegistered:
			c = a.account.CreatedAt.Compare(b.account.CreatedAt)
		}
		if c == 0 {
			c = strings.Compare(a.account.ID, b.account.ID)
		}
		if query.Descending {
			return -c
		}
		return c
	})
	if query.Offset >= len(entries) {
		return []RegisteredAccount{}, false, nil
	}
	end := min(query.Offset+query.Limit, len(entries))
	more := end < len(entries)
	rows := make([]RegisteredAccount, 0, end-query.Offset)
	for _, e := range entries[query.Offset:end] {
		row, err := s.registeredAccount(ctx, e.account.ID, adminEmails)
		if errors.Is(err, ErrInvalidCredentials) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		rows = append(rows, row)
	}
	return rows, more, nil
}

// directorySortKey decrypts the case-folded email or display name that orders
// one directory entry. It returns ErrInvalidCredentials for an erased account.
func (s *Service) directorySortKey(ctx context.Context, sort DirectorySort, id string) (string, error) {
	if sort == SortByEmail {
		credential, ok := s.handle.Projection().credentialForAccount(id)
		if !ok {
			return "", ErrInvalidCredentials
		}
		email, err := s.decryptCredentialEmail(ctx, credential)
		return strings.ToLower(email), err
	}
	profile, err := s.decryptProfile(ctx, id)
	if err != nil {
		return "", err
	}
	if profile.FullName != "" {
		return strings.ToLower(profile.FullName), nil
	}
	return strings.ToLower(profile.PreferredUsername), nil
}

// registeredAccount loads and decrypts one directory row.
func (s *Service) registeredAccount(ctx context.Context, id string, adminEmails []string) (RegisteredAccount, error) {
	email, err := s.EmailAddress(ctx, id)
	if err != nil {
		return RegisteredAccount{}, err
	}
	profile, err := s.Profile(ctx, id)
	if err != nil {
		return RegisteredAccount{}, err
	}
	access, err := s.Administrator(ctx, id, adminEmails)
	if err != nil {
		return RegisteredAccount{}, err
	}
	account, ok := s.Get(id)
	if !ok {
		return RegisteredAccount{}, ErrInvalidCredentials
	}
	return RegisteredAccount{Account: account, Email: email, Profile: profile, Access: access}, nil
}

// compareBool orders false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}
