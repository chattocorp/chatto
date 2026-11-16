package accounts

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

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

// RegisteredAccount is one row of the administrator directory. EmailHint and
// NameHint are masked: the directory must not show verbatim personal data. It
// must not be logged or returned to unauthenticated or ordinary accounts.
type RegisteredAccount struct {
	Account
	// EmailHint keeps the first character of the local part and of the domain
	// name, and the top-level domain, for example "a•••@e•••.com".
	EmailHint string
	// NameHint keeps the first character of the full name, or of the preferred
	// username if there is no full name. It is empty if both are empty.
	NameHint string
	Access   AdministratorAccess
}

// DirectorySort selects the column that orders the account directory. Masked
// columns are not sortable: their order would reveal the hidden values.
type DirectorySort string

const (
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
	case SortByAccess, SortByRegistered, SortByID:
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
// first. It sorts projected state and decrypts only the selected page.
// Accounts erased during the read are omitted; storage failures fail the whole
// page.
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
	}
	p := s.handle.Projection()
	p.RLock()
	digests := p.adminEmailDigests(adminEmails)
	entries := make([]entry, 0, len(p.accounts))
	for id, account := range p.accounts {
		entries = append(entries, entry{account: account, admin: account.IsAdministrator || p.configuredAdministrator(id, digests)})
	}
	p.RUnlock()
	slices.SortFunc(entries, func(a, b entry) int {
		c := 0
		switch query.Sort {
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

// registeredAccount loads one directory row and masks its personal data.
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
	name := profile.FullName
	if name == "" {
		name = profile.PreferredUsername
	}
	return RegisteredAccount{Account: account, EmailHint: maskEmail(email), NameHint: maskText(name), Access: access}, nil
}

// maskText keeps only the first character. A fixed mask hides the length.
func maskText(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return ""
	}
	return string(r) + "•••"
}

// maskEmail masks the local part and the domain name, and keeps the top-level
// domain: "alice@example.com" becomes "a•••@e•••.com".
func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return maskText(email)
	}
	local, domain := email[:at], email[at+1:]
	if dot := strings.LastIndex(domain, "."); dot > 0 {
		return maskText(local) + "@" + maskText(domain[:dot]) + domain[dot:]
	}
	return maskText(local) + "@" + maskText(domain)
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
