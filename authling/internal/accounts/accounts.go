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

// Account is the current projected structural state of an Authling account.
type Account struct {
	ID        string
	CreatedAt time.Time
}

type protectedCredential struct {
	accountID                  string
	eventID                    string
	userKeyRef                 string
	credentialKeyRef           string
	passwordVerifierNonce      []byte
	passwordVerifierCiphertext []byte
}

// Projection rebuilds the active account registry from durable events.
type Projection struct {
	events.MemoryProjection
	accounts      map[string]Account
	emails        map[[32]byte]string
	pendingEmails map[string][32]byte
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
func (p *Projection) Apply(event *corev1.Event, _ uint64) error {
	payload := event.GetAccountCreated()
	if payload == nil {
		claim := event.GetEmailClaimed()
		if claim == nil {
			return fmt.Errorf("unsupported account event")
		}
		p.Lock()
		defer p.Unlock()
		digestValue, ok := p.pendingEmails[claim.GetAccountId()]
		if !ok {
			return fmt.Errorf("email claim has no staged account credential")
		}
		if p.emails == nil {
			p.emails = make(map[[32]byte]string)
		}
		if _, exists := p.emails[digestValue]; exists {
			return fmt.Errorf("email was claimed more than once")
		}
		p.emails[digestValue] = claim.GetAccountId()
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
			p.pendingEmails = make(map[string][32]byte)
		}
		if p.keyReferences == nil {
			p.keyReferences = make(map[string]struct{})
		}
		if p.credentials == nil {
			p.credentials = make(map[string]protectedCredential)
		}
		p.pendingEmails[account.ID] = emailDigest
		p.credentials[account.ID] = protectedCredential{
			accountID:                  account.ID,
			eventID:                    event.GetId(),
			userKeyRef:                 payload.GetUserKeyRef(),
			credentialKeyRef:           payload.GetCredentialKeyRef(),
			passwordVerifierNonce:      append([]byte(nil), payload.GetPasswordVerifierNonce()...),
			passwordVerifierCiphertext: append([]byte(nil), payload.GetPasswordVerifierCiphertext()...),
		}
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

// Get returns one projected account.
func (p *Projection) Get(accountID string) (Account, bool) {
	p.RLock()
	defer p.RUnlock()
	account, ok := p.accounts[accountID]
	return account, ok
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
	password = norm.NFC.String(password)
	if s.handle.Projection().HasEmail(email) {
		return Account{}, ErrEmailClaimed
	}
	if utf8.RuneCountInString(password) < s.passwordMinimumLength || len(password) > 1024 {
		return Account{}, invalidPasswordError{minimumLength: s.passwordMinimumLength}
	}
	if isCommonPassword(password) {
		return Account{}, commonPasswordError{}
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
	claimEvent := &corev1.Event{Id: claimEventID, CreatedAt: event.CreatedAt, Event: &corev1.Event_EmailClaimed{EmailClaimed: &corev1.EmailClaimedEvent{AccountId: accountID}}}
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

// AuthenticateLocal verifies an email/password credential without retaining
// plaintext protected data in the projection. Absent accounts still perform
// the same Argon2id work as password mismatches.
func (s *Service) AuthenticateLocal(ctx context.Context, email, password string) (Account, error) {
	email = NormalizeEmail(email)
	password = norm.NFC.String(password)
	credential, exists := s.handle.Projection().credentialForEmail(email)
	if !exists {
		credential = s.dummyCredential
	}
	dataKey, err := s.vault.ResolveDataKey(ctx, credential.credentialKeyRef, credential.userKeyRef)
	if err != nil {
		return Account{}, fmt.Errorf("resolve local credential key: %w", err)
	}
	defer clear(dataKey)
	plaintext, err := datacrypto.Open(
		dataKey,
		credential.passwordVerifierCiphertext,
		credential.passwordVerifierNonce,
		credentialAAD(credential.eventID, credential.accountID, credential.userKeyRef, credential.credentialKeyRef, "password-verifier"),
	)
	if err != nil {
		return Account{}, fmt.Errorf("decrypt password verifier: %w", err)
	}
	verifier := string(plaintext)
	clear(plaintext)
	valid, err := verifyPassword(verifier, password)
	if err != nil {
		return Account{}, fmt.Errorf("decode password verifier: %w", err)
	}
	if !valid {
		return Account{}, ErrInvalidCredentials
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
