// Package accounts owns Authling's account aggregate and in-memory model.
package accounts

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/ids"
	"hmans.de/authling/internal/keyvault"
	"hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/chatto/pkg/datacrypto"
	"hmans.de/chatto/pkg/events"
)

// ErrIDCollision indicates that a generated account ID already has durable
// history. Callers may retry the command with a fresh generated ID.
var ErrIDCollision = errors.New("generated account id already exists")

// ErrEmailClaimed is intentionally not suitable for direct display; public
// signup responses must not disclose whether an email already exists.
var ErrEmailClaimed = errors.New("email is already claimed")

// ErrEmailUnchanged indicates that an email-change request selected the
// account's current normalized login address.
var ErrEmailUnchanged = errors.New("enter a different email address")

// ErrInvalidPassword indicates that a password does not meet the active
// password policy.
var ErrInvalidPassword = errors.New("password does not meet policy")

type invalidPasswordError struct{ minimumLength int }

func (e invalidPasswordError) Error() string {
	return fmt.Sprintf("password must contain at least %d characters and at most 1024 bytes", e.minimumLength)
}

func (invalidPasswordError) Is(target error) bool { return target == ErrInvalidPassword }

type commonPasswordError struct{}

func (commonPasswordError) Error() string {
	return "password is too common; choose a less predictable password"
}

func (commonPasswordError) Is(target error) bool { return target == ErrInvalidPassword }

// ErrInvalidCredentials deliberately combines absent accounts and password
// mismatches so callers cannot disclose which email addresses are registered.
var ErrInvalidCredentials = errors.New("invalid email or password")

// ErrCredentialChanged indicates that an identity workflow was issued for an
// older local credential or unavailable request and must not overwrite it.
var ErrCredentialChanged = errors.New("local credential changed")

// Account is the current projected structural state of an Authling account.
type Account struct {
	ID                    string
	CreatedAt             time.Time
	AuthenticationVersion uint64
}

type protectedCredential struct {
	accountID                  string
	eventID                    string
	userKeyRef                 string
	credentialKeyRef           string
	emailDigest                [32]byte
	emailNonce                 []byte
	emailCiphertext            []byte
	emailAAD                   []byte
	emailChangeRequestEventID  string
	emailChangeEventID         string
	passwordVerifierNonce      []byte
	passwordVerifierCiphertext []byte
	passwordVerifierAAD        []byte
}

// PasswordResetTarget binds an expiring recovery flow to the credential that
// was current when the email challenge was issued.
type PasswordResetTarget struct {
	AccountID         string
	CredentialEventID string
	RequestEventID    string
}

// EmailChangeTarget binds an expiring verified-mailbox flow to the credential
// for which the current password was reauthenticated.
type EmailChangeTarget struct {
	AccountID         string
	CredentialEventID string
	RequestEventID    string
	OldEmail          string
}

type pendingEmail struct {
	eventID    string
	digest     [32]byte
	credential protectedCredential
	replaces   bool
}

type emailChangeRequest struct {
	credentialEventID string
	sequence          uint64
}

// This comfortably exceeds the global number of code deliveries possible
// during one flow lifetime while bounding replay memory for abandoned audits.
const maxTrackedEmailChangeRequestsPerAccount = 4096

// Projection rebuilds the active account registry from durable events.
type Projection struct {
	events.MemoryProjection
	accounts      map[string]Account
	emails        map[[32]byte]string
	pendingEmails map[string]pendingEmail
	emailChanges  map[string]map[string]emailChangeRequest
	credentials   map[string]protectedCredential
	keyReferences map[string]struct{}
	vault         *keyvault.Vault
	indexKey      []byte
}

// NewProjection creates the protected account projection.
func NewProjection(vault *keyvault.Vault, indexKey []byte) *Projection {
	return &Projection{vault: vault, indexKey: append([]byte(nil), indexKey...)}
}

// Subjects returns the account event family consumed by this projection.
func (*Projection) Subjects() []string {
	return []string{evtstream.AccountSubjectFilter, evtstream.AccountRegistrySubject()}
}

// Apply adds one durable account fact to the in-memory registry.
func (p *Projection) Apply(event *corev1.Event, sequence uint64) error {
	if requested := event.GetPasswordResetRequested(); requested != nil {
		p.RLock()
		defer p.RUnlock()
		account, ok := p.accounts[requested.GetAccountId()]
		if !ok {
			return fmt.Errorf("password reset request references an absent account")
		}
		credential, ok := p.credentials[account.ID]
		if !ok || credential.eventID != requested.GetCredentialEventId() {
			return fmt.Errorf("password reset request references another credential")
		}
		return nil
	}
	if requested := event.GetEmailChangeRequested(); requested != nil {
		p.Lock()
		defer p.Unlock()
		account, ok := p.accounts[requested.GetAccountId()]
		if !ok {
			return fmt.Errorf("email change request references an absent account")
		}
		credential, ok := p.credentials[account.ID]
		if !ok || credential.eventID != requested.GetCredentialEventId() {
			return fmt.Errorf("email change request references another credential")
		}
		if p.emailChanges == nil {
			p.emailChanges = make(map[string]map[string]emailChangeRequest)
		}
		requests := p.emailChanges[account.ID]
		if requests == nil {
			requests = make(map[string]emailChangeRequest)
			p.emailChanges[account.ID] = requests
		}
		if len(requests) >= maxTrackedEmailChangeRequestsPerAccount {
			var oldestID string
			oldestSequence := ^uint64(0)
			for id, request := range requests {
				if request.sequence < oldestSequence {
					oldestID, oldestSequence = id, request.sequence
				}
			}
			delete(requests, oldestID)
		}
		requests[event.GetId()] = emailChangeRequest{credentialEventID: requested.GetCredentialEventId(), sequence: sequence}
		return nil
	}
	if changed := event.GetPasswordChanged(); changed != nil {
		p.Lock()
		defer p.Unlock()
		account, ok := p.accounts[changed.GetAccountId()]
		if !ok {
			return fmt.Errorf("password change references an absent account")
		}
		credential, ok := p.credentials[account.ID]
		if !ok || credential.userKeyRef != changed.GetUserKeyRef() || credential.credentialKeyRef != changed.GetCredentialKeyRef() {
			return fmt.Errorf("password change references another credential hierarchy")
		}
		credential.eventID = event.GetId()
		credential.passwordVerifierNonce = append([]byte(nil), changed.GetPasswordVerifierNonce()...)
		credential.passwordVerifierCiphertext = append([]byte(nil), changed.GetPasswordVerifierCiphertext()...)
		credential.passwordVerifierAAD = passwordChangedAAD(event.GetId(), account.ID, credential.userKeyRef, credential.credentialKeyRef)
		p.credentials[account.ID] = credential
		delete(p.emailChanges, account.ID)
		account.AuthenticationVersion++
		p.accounts[account.ID] = account
		return nil
	}
	if changed := event.GetEmailChanged(); changed != nil {
		p.RLock()
		account, ok := p.accounts[changed.GetAccountId()]
		if !ok {
			p.RUnlock()
			return fmt.Errorf("email change references an absent account")
		}
		credential, ok := p.credentials[account.ID]
		if !ok || credential.eventID != changed.GetPriorCredentialEventId() || credential.userKeyRef != changed.GetUserKeyRef() || credential.credentialKeyRef != changed.GetCredentialKeyRef() {
			p.RUnlock()
			return fmt.Errorf("email change references another credential")
		}
		if _, exists := p.pendingEmails[account.ID]; exists {
			p.RUnlock()
			return fmt.Errorf("email change overlaps an unclaimed credential")
		}
		request, ok := p.emailChanges[account.ID][changed.GetEmailChangeRequestEventId()]
		if !ok || request.credentialEventID != changed.GetPriorCredentialEventId() {
			p.RUnlock()
			return fmt.Errorf("email change references another reauthentication request")
		}
		p.RUnlock()
		if changed.GetCredentialEnvelopeVersion() != 1 || p.vault == nil || len(p.indexKey) == 0 {
			return fmt.Errorf("unsupported or unavailable changed email credential envelope")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		dataKey, err := p.vault.ResolveDataKey(ctx, changed.GetCredentialKeyRef(), changed.GetUserKeyRef())
		cancel()
		if err != nil {
			return fmt.Errorf("resolve changed email credential key: %w", err)
		}
		defer clear(dataKey)
		emailAAD := emailChangedAAD(
			event.GetId(),
			account.ID,
			credential.userKeyRef,
			credential.credentialKeyRef,
			changed.GetEmailChangeRequestEventId(),
			changed.GetPriorCredentialEventId(),
		)
		plaintext, err := datacrypto.Open(dataKey, changed.GetEmailCiphertext(), changed.GetEmailNonce(), emailAAD)
		if err != nil {
			return fmt.Errorf("decrypt changed account email: %w", err)
		}
		email := string(plaintext)
		clear(plaintext)
		if email == "" {
			return fmt.Errorf("decode changed account email")
		}
		staged := credential
		staged.eventID = event.GetId()
		staged.emailDigest = digest(p.indexKey, email)
		staged.emailNonce = append([]byte(nil), changed.GetEmailNonce()...)
		staged.emailCiphertext = append([]byte(nil), changed.GetEmailCiphertext()...)
		staged.emailAAD = emailAAD
		staged.emailChangeRequestEventID = changed.GetEmailChangeRequestEventId()
		staged.emailChangeEventID = event.GetId()

		p.Lock()
		defer p.Unlock()
		current, ok := p.credentials[account.ID]
		if !ok || current.eventID != credential.eventID || current.userKeyRef != credential.userKeyRef || current.credentialKeyRef != credential.credentialKeyRef {
			return fmt.Errorf("email change references a credential that changed while decrypting")
		}
		if _, exists := p.pendingEmails[account.ID]; exists {
			return fmt.Errorf("email change overlaps an unclaimed credential")
		}
		currentRequest, ok := p.emailChanges[account.ID][changed.GetEmailChangeRequestEventId()]
		if !ok || currentRequest != request {
			return fmt.Errorf("email change reauthentication request changed while decrypting")
		}
		p.pendingEmails[account.ID] = pendingEmail{eventID: event.GetId(), digest: staged.emailDigest, credential: staged, replaces: true}
		return nil
	}
	payload := event.GetAccountCreated()
	if payload == nil {
		claim := event.GetEmailClaimed()
		if claim == nil {
			return fmt.Errorf("unsupported account event")
		}
		p.Lock()
		defer p.Unlock()
		pending, ok := p.pendingEmails[claim.GetAccountId()]
		if !ok {
			return fmt.Errorf("email claim has no staged account credential")
		}
		if credentialEventID := claim.GetCredentialEventId(); credentialEventID != "" && credentialEventID != pending.eventID {
			return fmt.Errorf("email claim references another staged credential")
		}
		if pending.replaces && claim.GetCredentialEventId() == "" {
			return fmt.Errorf("email replacement claim is missing credential correlation")
		}
		if p.emails == nil {
			p.emails = make(map[[32]byte]string)
		}
		if _, exists := p.emails[pending.digest]; exists {
			return fmt.Errorf("email was claimed more than once")
		}
		if pending.replaces {
			current, ok := p.credentials[claim.GetAccountId()]
			if !ok {
				return fmt.Errorf("email change has no active credential")
			}
			delete(p.emails, current.emailDigest)
			p.credentials[claim.GetAccountId()] = pending.credential
			account := p.accounts[claim.GetAccountId()]
			account.AuthenticationVersion++
			p.accounts[claim.GetAccountId()] = account
			delete(p.emailChanges, claim.GetAccountId())
		}
		p.emails[pending.digest] = claim.GetAccountId()
		delete(p.pendingEmails, claim.GetAccountId())
		return nil
	}
	account := Account{
		ID:        payload.GetAccountId(),
		CreatedAt: event.GetCreatedAt().AsTime(),
	}
	var emailDigest [32]byte
	hasCredential := payload.GetCredentialEnvelopeVersion() != 0
	if hasCredential {
		if payload.GetCredentialEnvelopeVersion() != 1 || p.vault == nil || len(p.indexKey) == 0 {
			return fmt.Errorf("unsupported or unavailable credential envelope")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		dataKey, err := p.vault.ResolveDataKey(ctx, payload.GetCredentialKeyRef(), payload.GetUserKeyRef())
		cancel()
		if err != nil {
			return fmt.Errorf("resolve account credential key: %w", err)
		}
		defer clear(dataKey)
		plaintext, err := datacrypto.Open(dataKey, payload.GetEmailCiphertext(), payload.GetEmailNonce(), credentialAAD(event.GetId(), payload.GetAccountId(), payload.GetUserKeyRef(), payload.GetCredentialKeyRef(), "email"))
		if err != nil {
			return fmt.Errorf("decrypt account email: %w", err)
		}
		email := string(plaintext)
		clear(plaintext)
		if email == "" {
			return fmt.Errorf("decode account email")
		}
		emailDigest = digest(p.indexKey, email)
	}

	p.Lock()
	defer p.Unlock()
	if p.accounts == nil {
		p.accounts = make(map[string]Account)
	}
	if _, exists := p.accounts[account.ID]; exists {
		return fmt.Errorf("account %q was created more than once", account.ID)
	}
	if p.emails == nil {
		p.emails = make(map[[32]byte]string)
	}
	if hasCredential {
		if p.pendingEmails == nil {
			p.pendingEmails = make(map[string]pendingEmail)
		}
		if p.keyReferences == nil {
			p.keyReferences = make(map[string]struct{})
		}
		if p.credentials == nil {
			p.credentials = make(map[string]protectedCredential)
		}
		credential := protectedCredential{
			accountID:                  account.ID,
			eventID:                    event.GetId(),
			userKeyRef:                 payload.GetUserKeyRef(),
			credentialKeyRef:           payload.GetCredentialKeyRef(),
			emailDigest:                emailDigest,
			emailNonce:                 append([]byte(nil), payload.GetEmailNonce()...),
			emailCiphertext:            append([]byte(nil), payload.GetEmailCiphertext()...),
			emailAAD:                   credentialAAD(event.GetId(), account.ID, payload.GetUserKeyRef(), payload.GetCredentialKeyRef(), "email"),
			passwordVerifierNonce:      append([]byte(nil), payload.GetPasswordVerifierNonce()...),
			passwordVerifierCiphertext: append([]byte(nil), payload.GetPasswordVerifierCiphertext()...),
			passwordVerifierAAD:        credentialAAD(event.GetId(), account.ID, payload.GetUserKeyRef(), payload.GetCredentialKeyRef(), "password-verifier"),
		}
		p.pendingEmails[account.ID] = pendingEmail{eventID: event.GetId(), digest: emailDigest, credential: credential}
		p.credentials[account.ID] = credential
		p.keyReferences[payload.GetUserKeyRef()] = struct{}{}
		p.keyReferences[payload.GetCredentialKeyRef()] = struct{}{}
	}
	p.accounts[account.ID] = account
	return nil
}

// KeyReferences returns the active opaque key references discovered by replay.
func (p *Projection) KeyReferences() map[string]struct{} {
	p.RLock()
	defer p.RUnlock()
	result := make(map[string]struct{}, len(p.keyReferences))
	for ref := range p.keyReferences {
		result[ref] = struct{}{}
	}
	return result
}

// HasEmail reports whether normalized email is already claimed.
func (p *Projection) HasEmail(email string) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.emails[digest(p.indexKey, email)]
	return ok
}

func (p *Projection) credentialForEmail(email string) (protectedCredential, bool) {
	p.RLock()
	defer p.RUnlock()
	accountID, ok := p.emails[digest(p.indexKey, email)]
	if !ok {
		return protectedCredential{}, false
	}
	credential, ok := p.credentials[accountID]
	return credential, ok
}

func (p *Projection) credentialForAccount(accountID string) (protectedCredential, bool) {
	p.RLock()
	defer p.RUnlock()
	credential, ok := p.credentials[accountID]
	return credential, ok
}

func (p *Projection) hasEmailChangeRequest(target EmailChangeTarget) bool {
	p.RLock()
	defer p.RUnlock()
	request, ok := p.emailChanges[target.AccountID][target.RequestEventID]
	return ok && request.credentialEventID == target.CredentialEventID
}

func (p *Projection) completedEmailChange(target EmailChangeTarget, newEmail string) (Account, bool) {
	p.RLock()
	defer p.RUnlock()
	credential, ok := p.credentials[target.AccountID]
	if !ok || credential.eventID != credential.emailChangeEventID || credential.emailChangeRequestEventID != target.RequestEventID || credential.emailDigest != digest(p.indexKey, NormalizeEmail(newEmail)) {
		return Account{}, false
	}
	account, ok := p.accounts[target.AccountID]
	return account, ok
}

func (p *Projection) passwordResetTarget(email string) (PasswordResetTarget, bool) {
	credential, ok := p.credentialForEmail(email)
	if !ok {
		return PasswordResetTarget{}, false
	}
	return PasswordResetTarget{AccountID: credential.accountID, CredentialEventID: credential.eventID}, true
}

// Get returns one projected account.
func (p *Projection) Get(accountID string) (Account, bool) {
	p.RLock()
	defer p.RUnlock()
	account, ok := p.accounts[accountID]
	return account, ok
}

// AuthenticationVersion returns the durable credential generation used to
// invalidate browser sessions created before a password change.
func (p *Projection) AuthenticationVersion(accountID string) (uint64, bool) {
	p.RLock()
	defer p.RUnlock()
	account, ok := p.accounts[accountID]
	return account.AuthenticationVersion, ok
}

// UserKeyRef returns the opaque user-key reference for an account that has a
// protected local credential. Historical structural accounts have no key.
func (p *Projection) UserKeyRef(accountID string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	credential, ok := p.credentials[accountID]
	return credential.userKeyRef, ok && credential.userKeyRef != ""
}

// Count returns the number of projected accounts.
func (p *Projection) Count() int {
	p.RLock()
	defer p.RUnlock()
	return len(p.accounts)
}

// Service validates account commands, commits events with OCC, and waits for
// the serving projection before returning.
type Service struct {
	publisher             *evtstream.Publisher
	handle                events.ProjectionHandle[*Projection]
	vault                 *keyvault.Vault
	dummyCredential       protectedCredential
	passwordMinimumLength int
}

// UserKeyRef returns the account's opaque user-key reference for another
// protected account-owned data purpose.
func (s *Service) UserKeyRef(accountID string) (string, bool) {
	return s.handle.Projection().UserKeyRef(accountID)
}

// NewService constructs the account command and read boundary.
func NewService(
	ctx context.Context,
	publisher *evtstream.Publisher,
	handle events.ProjectionHandle[*Projection],
	vault *keyvault.Vault,
	passwordMinimumLength int,
) (*Service, error) {
	userRef, dataRef, dataKey, err := vault.AuthenticationDummyKey(ctx)
	if err != nil {
		return nil, err
	}
	defer clear(dataKey)
	const eventID = "authentication-dummy-event"
	const accountID = "authentication-dummy-account"
	sealed, err := datacrypto.Seal(dataKey, []byte(dummyPasswordVerifier), credentialAAD(eventID, accountID, userRef, dataRef, "password-verifier"))
	if err != nil {
		return nil, fmt.Errorf("seal authentication dummy credential: %w", err)
	}
	return &Service{
		publisher:             publisher,
		handle:                handle,
		vault:                 vault,
		passwordMinimumLength: passwordMinimumLength,
		dummyCredential: protectedCredential{
			accountID: accountID, eventID: eventID, userKeyRef: userRef, credentialKeyRef: dataRef,
			passwordVerifierNonce: sealed.Nonce, passwordVerifierCiphertext: sealed.Ciphertext,
			passwordVerifierAAD: credentialAAD(eventID, accountID, userRef, dataRef, "password-verifier"),
		},
	}, nil
}

// Create creates one opaque account and returns its projected state.
func (s *Service) Create(ctx context.Context) (Account, error) {
	accountID, err := ids.New("acc")
	if err != nil {
		return Account{}, err
	}
	eventID, err := ids.New("evt")
	if err != nil {
		return Account{}, err
	}
	event := &corev1.Event{
		Id:        eventID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_AccountCreated{
			AccountCreated: &corev1.AccountCreatedEvent{AccountId: accountID},
		},
	}
	position, err := s.publisher.AppendAccountCreated(ctx, event)
	if err != nil {
		if errors.Is(err, events.ErrConflict) {
			return Account{}, ErrIDCollision
		}
		return Account{}, fmt.Errorf("commit account creation: %w", err)
	}
	if err := s.handle.Projector().WaitFor(ctx, position); err != nil {
		return Account{}, fmt.Errorf("wait for account projection: %w", err)
	}
	account, ok := s.handle.Projection().Get(accountID)
	if !ok {
		return Account{}, fmt.Errorf("created account is absent from projection")
	}
	return account, nil
}

// CreateLocal creates a verified local email/password account. The caller must
// establish email ownership before invoking this command.
func (s *Service) CreateLocal(ctx context.Context, email, password string) (Account, error) {
	email = NormalizeEmail(email)
	if s.handle.Projection().HasEmail(email) {
		return Account{}, ErrEmailClaimed
	}
	password, err := validatePassword(password, s.passwordMinimumLength)
	if err != nil {
		return Account{}, err
	}
	verifier, err := hashPassword(password)
	if err != nil {
		return Account{}, err
	}
	accountID, err := ids.New("acc")
	if err != nil {
		return Account{}, err
	}
	eventID, err := ids.New("evt")
	if err != nil {
		return Account{}, err
	}
	claimEventID, err := ids.New("evt")
	if err != nil {
		return Account{}, err
	}
	operationRef, userRef, dataRef, dataKey, err := s.vault.ProvisionCredentialKeys(ctx)
	if err != nil {
		return Account{}, fmt.Errorf("provision account keys: %w", err)
	}
	defer clear(dataKey)
	committed := false
	defer func() {
		if !committed {
			cleanupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.vault.RemoveProvisionedCredentialKeys(cleanupContext, operationRef, userRef, dataRef)
		}
	}()
	sealedEmail, err := datacrypto.Seal(dataKey, []byte(email), credentialAAD(eventID, accountID, userRef, dataRef, "email"))
	if err != nil {
		return Account{}, err
	}
	sealedVerifier, err := datacrypto.Seal(dataKey, []byte(verifier), credentialAAD(eventID, accountID, userRef, dataRef, "password-verifier"))
	if err != nil {
		return Account{}, err
	}
	event := &corev1.Event{Id: eventID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_AccountCreated{AccountCreated: &corev1.AccountCreatedEvent{
		AccountId: accountID, UserKeyRef: userRef, CredentialKeyRef: dataRef,
		EmailNonce: sealedEmail.Nonce, EmailCiphertext: sealedEmail.Ciphertext, CredentialEnvelopeVersion: 1,
		PasswordVerifierNonce: sealedVerifier.Nonce, PasswordVerifierCiphertext: sealedVerifier.Ciphertext,
	}}}
	claimEvent := &corev1.Event{Id: claimEventID, CreatedAt: event.CreatedAt, Event: &corev1.Event_EmailClaimed{EmailClaimed: &corev1.EmailClaimedEvent{AccountId: accountID, CredentialEventId: eventID}}}
	for range 5 {
		tail, err := s.publisher.AccountRegistryTail(ctx)
		if err != nil {
			return Account{}, fmt.Errorf("read account registry: %w", err)
		}
		if tail > 0 {
			if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(evtstream.AccountRegistrySubject(), tail)); err != nil {
				return Account{}, fmt.Errorf("wait for account registry: %w", err)
			}
		}
		if s.handle.Projection().HasEmail(email) {
			return Account{}, ErrEmailClaimed
		}
		position, err := s.publisher.AppendRegisteredAccount(ctx, event, claimEvent, tail)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return Account{}, fmt.Errorf("commit local account: %w", err)
		}
		committed = true
		_ = s.vault.CompleteProvisioning(ctx, operationRef)
		if err := s.handle.Projector().WaitFor(ctx, position); err != nil {
			return Account{}, err
		}
		account, ok := s.handle.Projection().Get(accountID)
		if !ok {
			return Account{}, fmt.Errorf("created account is absent from projection")
		}
		return account, nil
	}
	return Account{}, fmt.Errorf("account registry conflict")
}

// PasswordMinimumLength returns the active local password policy.
func (s *Service) PasswordMinimumLength() int { return s.passwordMinimumLength }

// NormalizeEmail defines Authling's initial deployment-wide comparison value.
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func digest(key []byte, value string) [32]byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func credentialAAD(eventID, accountID, userRef, dataRef, field string) []byte {
	return []byte("authling:event:v1\x00AccountCreated\x00" + eventID + "\x00" + accountID + "\x00credentials\x001\x00" + userRef + "\x00" + dataRef + "\x00" + field)
}

func passwordChangedAAD(eventID, accountID, userRef, dataRef string) []byte {
	return []byte("authling:event:v1\x00PasswordChanged\x00" + eventID + "\x00" + accountID + "\x00credentials\x001\x00" + userRef + "\x00" + dataRef + "\x00password-verifier")
}

func emailChangedAAD(eventID, accountID, userRef, dataRef, requestEventID, priorCredentialEventID string) []byte {
	return []byte("authling:event:v1\x00EmailChanged\x00" + eventID + "\x00" + accountID + "\x00credentials\x001\x00" + userRef + "\x00" + dataRef + "\x00email\x00" + requestEventID + "\x00" + priorCredentialEventID)
}

func validatePassword(password string, minimumLength int) (string, error) {
	password = norm.NFC.String(password)
	if utf8.RuneCountInString(password) < minimumLength || len(password) > 1024 {
		return "", invalidPasswordError{minimumLength: minimumLength}
	}
	if isCommonPassword(password) {
		return "", commonPasswordError{}
	}
	return password, nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

// Get returns one account from the ready in-memory projection.
func (s *Service) Get(accountID string) (Account, bool) {
	return s.handle.Projection().Get(accountID)
}

// Count returns the number of projected accounts.
func (s *Service) Count() int {
	return s.handle.Projection().Count()
}

// HasEmail reports whether a normalized local email is already claimed.
func (s *Service) HasEmail(email string) bool {
	return s.handle.Projection().HasEmail(NormalizeEmail(email))
}

// PrepareEmailChange reauthenticates one local account against its current
// password and binds a prospective change to that exact credential version.
// Callers must apply distributed guessing and concurrency limits around it.
func (s *Service) PrepareEmailChange(ctx context.Context, accountID, password, newEmail string) (EmailChangeTarget, error) {
	credential, ok := s.handle.Projection().credentialForAccount(accountID)
	if !ok {
		return EmailChangeTarget{}, ErrInvalidCredentials
	}
	if err := s.verifyCredentialPassword(ctx, credential, password); err != nil {
		return EmailChangeTarget{}, err
	}
	oldEmail, err := s.decryptCredentialEmail(ctx, credential)
	if err != nil {
		return EmailChangeTarget{}, err
	}
	if NormalizeEmail(oldEmail) == NormalizeEmail(newEmail) {
		return EmailChangeTarget{}, ErrEmailUnchanged
	}
	return EmailChangeTarget{AccountID: accountID, CredentialEventID: credential.eventID, OldEmail: oldEmail}, nil
}

// RecordEmailChangeRequested commits the PII-free request audit fact only if
// the credential reauthenticated by PrepareEmailChange is still current.
func (s *Service) RecordEmailChangeRequested(ctx context.Context, target EmailChangeTarget) (EmailChangeTarget, error) {
	if target.AccountID == "" || target.CredentialEventID == "" {
		return EmailChangeTarget{}, ErrCredentialChanged
	}
	eventID, err := ids.New("evt")
	if err != nil {
		return EmailChangeTarget{}, err
	}
	event := &corev1.Event{Id: eventID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_EmailChangeRequested{EmailChangeRequested: &corev1.EmailChangeRequestedEvent{
		AccountId: target.AccountID, CredentialEventId: target.CredentialEventID,
	}}}
	for range 5 {
		tail, err := s.publisher.AccountTail(ctx, target.AccountID)
		if err != nil {
			return EmailChangeTarget{}, fmt.Errorf("read email-change account tail: %w", err)
		}
		subject, err := evtstream.AccountSubject(target.AccountID)
		if err != nil {
			return EmailChangeTarget{}, ErrCredentialChanged
		}
		if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(subject, tail)); err != nil {
			return EmailChangeTarget{}, fmt.Errorf("wait for email-change account: %w", err)
		}
		current, ok := s.handle.Projection().credentialForAccount(target.AccountID)
		if !ok || current.eventID != target.CredentialEventID {
			return EmailChangeTarget{}, ErrCredentialChanged
		}
		position, err := s.publisher.AppendEmailChangeRequested(ctx, event, tail)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return EmailChangeTarget{}, fmt.Errorf("commit email change request: %w", err)
		}
		if err := s.handle.Projector().WaitFor(ctx, position); err != nil {
			return EmailChangeTarget{}, fmt.Errorf("wait for email change request: %w", err)
		}
		target.RequestEventID = eventID
		return target, nil
	}
	return EmailChangeTarget{}, fmt.Errorf("email change request conflict")
}

// ChangeEmail atomically replaces a verified local login address and claims
// its global registry entry if the reauthenticated credential is still current.
// The returned account is accepted only while the email-change event remains
// the current credential generation, so callers never inherit a later password
// or email mutation that raced the projection wait.
func (s *Service) ChangeEmail(ctx context.Context, target EmailChangeTarget, newEmail string) (Account, error) {
	newEmail = NormalizeEmail(newEmail)
	if target.AccountID == "" || target.CredentialEventID == "" || target.RequestEventID == "" || newEmail == "" {
		return Account{}, ErrCredentialChanged
	}
	eventID, err := ids.New("evt")
	if err != nil {
		return Account{}, err
	}
	claimEventID, err := ids.New("evt")
	if err != nil {
		return Account{}, err
	}
	createdAt := timestamppb.Now()
	var event, claimEvent *corev1.Event
	for range 5 {
		accountTail, err := s.publisher.AccountTail(ctx, target.AccountID)
		if err != nil {
			return Account{}, fmt.Errorf("read email-change account tail: %w", err)
		}
		registryTail, err := s.publisher.AccountRegistryTail(ctx)
		if err != nil {
			return Account{}, fmt.Errorf("read email-change registry tail: %w", err)
		}
		accountSubject, err := evtstream.AccountSubject(target.AccountID)
		if err != nil {
			return Account{}, ErrCredentialChanged
		}
		if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(accountSubject, accountTail)); err != nil {
			return Account{}, fmt.Errorf("wait for email-change account: %w", err)
		}
		if registryTail > 0 {
			if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(evtstream.AccountRegistrySubject(), registryTail)); err != nil {
				return Account{}, fmt.Errorf("wait for email-change registry: %w", err)
			}
		}
		credential, ok := s.handle.Projection().credentialForAccount(target.AccountID)
		if !ok || credential.eventID != target.CredentialEventID {
			return Account{}, ErrCredentialChanged
		}
		if !s.handle.Projection().hasEmailChangeRequest(target) {
			return Account{}, ErrCredentialChanged
		}
		if s.handle.Projection().HasEmail(newEmail) {
			return Account{}, ErrEmailClaimed
		}
		if event == nil {
			dataKey, err := s.vault.ResolveDataKey(ctx, credential.credentialKeyRef, credential.userKeyRef)
			if err != nil {
				return Account{}, fmt.Errorf("resolve email credential key: %w", err)
			}
			sealedEmail, sealErr := datacrypto.Seal(dataKey, []byte(newEmail), emailChangedAAD(
				eventID,
				target.AccountID,
				credential.userKeyRef,
				credential.credentialKeyRef,
				target.RequestEventID,
				target.CredentialEventID,
			))
			clear(dataKey)
			if sealErr != nil {
				return Account{}, sealErr
			}
			event = &corev1.Event{Id: eventID, CreatedAt: createdAt, Event: &corev1.Event_EmailChanged{EmailChanged: &corev1.EmailChangedEvent{
				AccountId: target.AccountID, UserKeyRef: credential.userKeyRef, CredentialKeyRef: credential.credentialKeyRef,
				CredentialEnvelopeVersion: 1, EmailNonce: sealedEmail.Nonce, EmailCiphertext: sealedEmail.Ciphertext,
				EmailChangeRequestEventId: target.RequestEventID, PriorCredentialEventId: target.CredentialEventID,
			}}}
			claimEvent = &corev1.Event{Id: claimEventID, CreatedAt: createdAt, Event: &corev1.Event_EmailClaimed{EmailClaimed: &corev1.EmailClaimedEvent{
				AccountId: target.AccountID, CredentialEventId: eventID,
			}}}
		}
		position, err := s.publisher.AppendEmailChanged(ctx, event, claimEvent, accountTail, registryTail)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return Account{}, fmt.Errorf("commit email change: %w", err)
		}
		if err := s.handle.Projector().WaitFor(ctx, position); err != nil {
			return Account{}, fmt.Errorf("wait for email change: %w", err)
		}
		account, ok := s.handle.Projection().completedEmailChange(target, newEmail)
		if !ok {
			return Account{}, ErrCredentialChanged
		}
		return account, nil
	}
	return Account{}, fmt.Errorf("email change conflict")
}

// CompletedEmailChange reports whether the current protected credential was
// installed by this exact request. Completion recovery uses it to resolve an
// ambiguous process failure after the atomic event batch committed.
func (s *Service) CompletedEmailChange(target EmailChangeTarget, newEmail string) (Account, bool) {
	return s.handle.Projection().completedEmailChange(target, newEmail)
}

// RecordPasswordResetRequested appends an account-scoped audit fact for an
// existing normalized local email and returns the credential binding for the
// recovery flow. Callers must not expose whether a target exists.
func (s *Service) RecordPasswordResetRequested(ctx context.Context, email string) (PasswordResetTarget, bool, error) {
	email = NormalizeEmail(email)
	eventID, err := ids.New("evt")
	if err != nil {
		return PasswordResetTarget{}, false, err
	}
	createdAt := timestamppb.Now()
	for range 5 {
		target, ok := s.handle.Projection().passwordResetTarget(email)
		if !ok {
			return PasswordResetTarget{}, false, nil
		}
		accountID := target.AccountID
		tail, err := s.publisher.AccountTail(ctx, accountID)
		if err != nil {
			return PasswordResetTarget{}, false, fmt.Errorf("read password reset account tail: %w", err)
		}
		subject, err := evtstream.AccountSubject(accountID)
		if err != nil {
			return PasswordResetTarget{}, false, fmt.Errorf("resolve password reset account subject: %w", err)
		}
		if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(subject, tail)); err != nil {
			return PasswordResetTarget{}, false, fmt.Errorf("wait for password reset account: %w", err)
		}
		target, ok = s.handle.Projection().passwordResetTarget(email)
		if !ok {
			return PasswordResetTarget{}, false, nil
		}
		if target.AccountID != accountID {
			continue
		}
		event := &corev1.Event{Id: eventID, CreatedAt: createdAt, Event: &corev1.Event_PasswordResetRequested{PasswordResetRequested: &corev1.PasswordResetRequestedEvent{
			AccountId: target.AccountID, CredentialEventId: target.CredentialEventID,
		}}}
		position, err := s.publisher.AppendPasswordResetRequested(ctx, event, tail)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return PasswordResetTarget{}, false, fmt.Errorf("commit password reset request: %w", err)
		}
		if err := s.handle.Projector().WaitFor(ctx, position); err != nil {
			return PasswordResetTarget{}, false, fmt.Errorf("wait for password reset request: %w", err)
		}
		target.RequestEventID = eventID
		return target, true, nil
	}
	return PasswordResetTarget{}, false, fmt.Errorf("password reset request conflict")
}

// AuthenticationVersion returns the generation embedded in new browser
// sessions. Password and verified-email changes advance it durably.
func (s *Service) AuthenticationVersion(accountID string) (uint64, bool) {
	return s.handle.Projection().AuthenticationVersion(accountID)
}

// ResetPassword replaces a local password only if the credential bound to the
// recovery flow is still current. The stable account ID and email are unchanged.
func (s *Service) ResetPassword(ctx context.Context, target PasswordResetTarget, password string) (Account, error) {
	if target.AccountID == "" || target.CredentialEventID == "" || target.RequestEventID == "" {
		return Account{}, ErrCredentialChanged
	}
	password, err := validatePassword(password, s.passwordMinimumLength)
	if err != nil {
		return Account{}, err
	}
	tail, credential, err := s.passwordResetCredentialAtTail(ctx, target)
	if err != nil {
		return Account{}, err
	}
	verifier, err := hashPassword(password)
	if err != nil {
		return Account{}, err
	}
	dataKey, err := s.vault.ResolveDataKey(ctx, credential.credentialKeyRef, credential.userKeyRef)
	if err != nil {
		return Account{}, fmt.Errorf("resolve password credential key: %w", err)
	}
	defer clear(dataKey)
	eventID, err := ids.New("evt")
	if err != nil {
		return Account{}, err
	}
	sealedVerifier, err := datacrypto.Seal(dataKey, []byte(verifier), passwordChangedAAD(eventID, target.AccountID, credential.userKeyRef, credential.credentialKeyRef))
	if err != nil {
		return Account{}, err
	}
	event := &corev1.Event{Id: eventID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_PasswordChanged{PasswordChanged: &corev1.PasswordChangedEvent{
		AccountId: target.AccountID, UserKeyRef: credential.userKeyRef, CredentialKeyRef: credential.credentialKeyRef,
		CredentialEnvelopeVersion: 1, PasswordVerifierNonce: sealedVerifier.Nonce, PasswordVerifierCiphertext: sealedVerifier.Ciphertext,
		PasswordResetRequestEventId: target.RequestEventID,
	}}}
	for range 5 {
		position, err := s.publisher.AppendPasswordChanged(ctx, event, tail)
		if errors.Is(err, events.ErrConflict) {
			tail, _, err = s.passwordResetCredentialAtTail(ctx, target)
			if err != nil {
				return Account{}, err
			}
			continue
		}
		if err != nil {
			return Account{}, fmt.Errorf("commit password change: %w", err)
		}
		if err := s.handle.Projector().WaitFor(ctx, position); err != nil {
			return Account{}, fmt.Errorf("wait for password change: %w", err)
		}
		account, ok := s.handle.Projection().Get(target.AccountID)
		if !ok {
			return Account{}, fmt.Errorf("reset account is absent from projection")
		}
		return account, nil
	}
	return Account{}, fmt.Errorf("password change conflict")
}

func (s *Service) passwordResetCredentialAtTail(ctx context.Context, target PasswordResetTarget) (uint64, protectedCredential, error) {
	tail, err := s.publisher.AccountTail(ctx, target.AccountID)
	if err != nil {
		return 0, protectedCredential{}, fmt.Errorf("read account tail: %w", err)
	}
	if tail == 0 {
		return 0, protectedCredential{}, ErrCredentialChanged
	}
	subject, err := evtstream.AccountSubject(target.AccountID)
	if err != nil {
		return 0, protectedCredential{}, ErrCredentialChanged
	}
	if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(subject, tail)); err != nil {
		return 0, protectedCredential{}, fmt.Errorf("wait for account credential: %w", err)
	}
	credential, ok := s.handle.Projection().credentialForAccount(target.AccountID)
	if !ok || credential.eventID != target.CredentialEventID {
		return 0, protectedCredential{}, ErrCredentialChanged
	}
	return tail, credential, nil
}

// AuthenticateLocal verifies an email/password credential without retaining
// plaintext protected data in the projection. Absent accounts still perform
// the same Argon2id work as password mismatches.
func (s *Service) AuthenticateLocal(ctx context.Context, email, password string) (Account, error) {
	email = NormalizeEmail(email)
	credential, exists := s.handle.Projection().credentialForEmail(email)
	if !exists {
		credential = s.dummyCredential
	}
	if err := s.verifyCredentialPassword(ctx, credential, password); err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return Account{}, ErrInvalidCredentials
		}
		return Account{}, err
	}
	if !exists {
		return Account{}, ErrInvalidCredentials
	}
	account, ok := s.handle.Projection().Get(credential.accountID)
	if !ok {
		return Account{}, fmt.Errorf("authenticated account is absent from projection")
	}
	return account, nil
}

func (s *Service) verifyCredentialPassword(ctx context.Context, credential protectedCredential, password string) error {
	password = norm.NFC.String(password)
	dataKey, err := s.vault.ResolveDataKey(ctx, credential.credentialKeyRef, credential.userKeyRef)
	if err != nil {
		return fmt.Errorf("resolve local credential key: %w", err)
	}
	defer clear(dataKey)
	plaintext, err := datacrypto.Open(
		dataKey,
		credential.passwordVerifierCiphertext,
		credential.passwordVerifierNonce,
		credential.passwordVerifierAAD,
	)
	if err != nil {
		return fmt.Errorf("decrypt password verifier: %w", err)
	}
	verifier := string(plaintext)
	clear(plaintext)
	valid, err := verifyPassword(verifier, password)
	if err != nil {
		return fmt.Errorf("decode password verifier: %w", err)
	}
	if !valid {
		return ErrInvalidCredentials
	}
	return nil
}

func (s *Service) decryptCredentialEmail(ctx context.Context, credential protectedCredential) (string, error) {
	dataKey, err := s.vault.ResolveDataKey(ctx, credential.credentialKeyRef, credential.userKeyRef)
	if err != nil {
		return "", fmt.Errorf("resolve email credential key: %w", err)
	}
	defer clear(dataKey)
	plaintext, err := datacrypto.Open(dataKey, credential.emailCiphertext, credential.emailNonce, credential.emailAAD)
	if err != nil {
		return "", fmt.Errorf("decrypt account email: %w", err)
	}
	defer clear(plaintext)
	email := string(plaintext)
	if email == "" {
		return "", fmt.Errorf("decode account email")
	}
	return email, nil
}

const dummyPasswordVerifier = "$argon2id$v=19$m=19456,t=2,p=1$MDEyMzQ1Njc4OWFiY2RlZg$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func verifyPassword(verifier, password string) (bool, error) {
	parts := strings.Split(verifier, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, fmt.Errorf("unsupported verifier header")
	}
	var memory uint64
	var iterations uint64
	var parallelism uint64
	parameters := strings.Split(parts[3], ",")
	if len(parameters) != 3 {
		return false, fmt.Errorf("invalid verifier parameters")
	}
	for _, parameter := range parameters {
		name, value, ok := strings.Cut(parameter, "=")
		if !ok {
			return false, fmt.Errorf("invalid verifier parameter")
		}
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return false, fmt.Errorf("invalid verifier parameter value")
		}
		switch name {
		case "m":
			memory = parsed
		case "t":
			iterations = parsed
		case "p":
			parallelism = parsed
		default:
			return false, fmt.Errorf("unsupported verifier parameter")
		}
	}
	if memory < 8*1024 || memory > 64*1024 || iterations < 1 || iterations > 5 || parallelism < 1 || parallelism > 4 {
		return false, fmt.Errorf("unsafe verifier parameters")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false, fmt.Errorf("invalid verifier salt")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false, fmt.Errorf("invalid verifier hash")
	}
	tooLong := len(password) > 1024
	if tooLong {
		password = password[:1024]
	}
	got := argon2.IDKey([]byte(password), salt, uint32(iterations), uint32(memory), uint8(parallelism), uint32(len(want)))
	return !tooLong && subtle.ConstantTimeCompare(got, want) == 1, nil
}
