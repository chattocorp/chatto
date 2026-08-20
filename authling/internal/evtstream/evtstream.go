// Package evtstream adapts Authling's protobuf event contract to the shared
// envelope-neutral event framework.
package evtstream

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	accountSubjectPrefix   = "authling.evt.account."
	accountRegistrySubject = "authling.evt.account-registry"
	issuerSubject          = "authling.evt.issuer"
	// AccountSubjectFilter contains every account aggregate.
	AccountSubjectFilter = accountSubjectPrefix + "*"
)

// IssuerSubject is the singleton aggregate that permanently identifies this
// Authling deployment as one OpenID Connect issuer.
func IssuerSubject() string { return issuerSubject }

// IssuerTail returns the singleton issuer aggregate's current OCC token.
func (p *Publisher) IssuerTail(ctx context.Context) (uint64, error) {
	return p.log.LastSubjectSeq(ctx, issuerSubject)
}

// Publisher validates and appends Authling events through the shared event log.
type Publisher struct {
	log *events.EncodedEventLog
}

// AccountRegistrySubject is the PII-free serialization point for local email
// claims. It prevents duplicate registrations across replicas without putting
// identifier digests into durable subjects.
func AccountRegistrySubject() string { return accountRegistrySubject }

// AccountRegistryTail returns the current OCC token for local account claims.
func (p *Publisher) AccountRegistryTail(ctx context.Context) (uint64, error) {
	return p.log.LastSubjectSeq(ctx, accountRegistrySubject)
}

// AccountTail returns the current OCC token for one account aggregate.
func (p *Publisher) AccountTail(ctx context.Context, accountID string) (uint64, error) {
	subject, err := AccountSubject(accountID)
	if err != nil {
		return 0, err
	}
	return p.log.LastSubjectSeq(ctx, subject)
}

// AppendRegisteredAccount commits a local account against a previously read
// registry tail.
func (p *Publisher) AppendRegisteredAccount(ctx context.Context, accountEvent, claimEvent *corev1.Event, expectedRegistry uint64) (events.StreamPosition, error) {
	account := accountEvent.GetAccountCreated()
	claim := claimEvent.GetEmailClaimed()
	if account == nil || claim == nil || account.GetAccountId() != claim.GetAccountId() {
		return events.StreamPosition{}, fmt.Errorf("append registered account: matching account_created and email_claimed payloads are required")
	}
	accountSubject, err := AccountSubject(account.GetAccountId())
	if err != nil {
		return events.StreamPosition{}, err
	}
	accountRecord, err := encode(accountEvent)
	if err != nil {
		return events.StreamPosition{}, err
	}
	claimRecord, err := encode(claimEvent)
	if err != nil {
		return events.StreamPosition{}, err
	}
	sequences, err := p.log.AppendBatch(ctx, []events.EncodedBatchEntry{
		{Subject: accountSubject, Record: accountRecord, ExpectedSeq: 0, HasOCC: true},
		{Subject: accountRegistrySubject, Record: claimRecord, ExpectedSeq: expectedRegistry, HasOCC: true},
	})
	if err != nil {
		return events.StreamPosition{}, err
	}
	return events.SubjectPosition(accountRegistrySubject, sequences[1]), nil
}

// NewPublisher constructs an Authling protobuf publisher.
func NewPublisher(log *events.EncodedEventLog) *Publisher {
	return &Publisher{log: log}
}

// AppendAccountCreated creates a new account aggregate at expected sequence
// zero. A non-zero tail is returned as an events.ErrConflict.
func (p *Publisher) AppendAccountCreated(
	ctx context.Context,
	event *corev1.Event,
) (events.StreamPosition, error) {
	payload := event.GetAccountCreated()
	if payload == nil {
		return events.StreamPosition{}, fmt.Errorf("append account created: event payload is not account_created")
	}
	subject, err := AccountSubject(payload.GetAccountId())
	if err != nil {
		return events.StreamPosition{}, err
	}
	record, err := encode(event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	sequence, err := p.log.AppendAt(ctx, subject, record, 0)
	if err != nil {
		return events.StreamPosition{}, err
	}
	return events.SubjectPosition(subject, sequence), nil
}

// AppendPasswordChanged replaces a credential at an explicitly observed
// account tail so concurrent reset flows cannot overwrite each other.
func (p *Publisher) AppendPasswordChanged(
	ctx context.Context,
	event *corev1.Event,
	expectedTail uint64,
) (events.StreamPosition, error) {
	payload := event.GetPasswordChanged()
	if payload == nil {
		return events.StreamPosition{}, fmt.Errorf("append password changed: event payload is not password_changed")
	}
	subject, err := AccountSubject(payload.GetAccountId())
	if err != nil {
		return events.StreamPosition{}, err
	}
	record, err := encode(event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	sequence, err := p.log.AppendAt(ctx, subject, record, expectedTail)
	if err != nil {
		return events.StreamPosition{}, err
	}
	return events.SubjectPosition(subject, sequence), nil
}

// AppendPasswordResetRequested records an accepted recovery request at an
// explicitly observed account tail.
func (p *Publisher) AppendPasswordResetRequested(
	ctx context.Context,
	event *corev1.Event,
	expectedTail uint64,
) (events.StreamPosition, error) {
	payload := event.GetPasswordResetRequested()
	if payload == nil {
		return events.StreamPosition{}, fmt.Errorf("append password reset requested: event payload is not password_reset_requested")
	}
	subject, err := AccountSubject(payload.GetAccountId())
	if err != nil {
		return events.StreamPosition{}, err
	}
	record, err := encode(event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	sequence, err := p.log.AppendAt(ctx, subject, record, expectedTail)
	if err != nil {
		return events.StreamPosition{}, err
	}
	return events.SubjectPosition(subject, sequence), nil
}

// AppendEmailChangeRequested records successful reauthentication for an email
// change at an explicitly observed account tail.
func (p *Publisher) AppendEmailChangeRequested(
	ctx context.Context,
	event *corev1.Event,
	expectedTail uint64,
) (events.StreamPosition, error) {
	payload := event.GetEmailChangeRequested()
	if payload == nil {
		return events.StreamPosition{}, fmt.Errorf("append email change requested: event payload is not email_change_requested")
	}
	subject, err := AccountSubject(payload.GetAccountId())
	if err != nil {
		return events.StreamPosition{}, err
	}
	record, err := encode(event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	sequence, err := p.log.AppendAt(ctx, subject, record, expectedTail)
	if err != nil {
		return events.StreamPosition{}, err
	}
	return events.SubjectPosition(subject, sequence), nil
}

// AppendEmailChanged atomically stages an encrypted replacement address on the
// account aggregate and activates its PII-free global claim.
func (p *Publisher) AppendEmailChanged(
	ctx context.Context,
	changeEvent, claimEvent *corev1.Event,
	expectedAccount, expectedRegistry uint64,
) (events.StreamPosition, error) {
	change := changeEvent.GetEmailChanged()
	claim := claimEvent.GetEmailClaimed()
	if change == nil || claim == nil || change.GetAccountId() != claim.GetAccountId() || claim.GetCredentialEventId() != changeEvent.GetId() {
		return events.StreamPosition{}, fmt.Errorf("append email changed: matching email_changed and email_claimed payloads are required")
	}
	accountSubject, err := AccountSubject(change.GetAccountId())
	if err != nil {
		return events.StreamPosition{}, err
	}
	changeRecord, err := encode(changeEvent)
	if err != nil {
		return events.StreamPosition{}, err
	}
	claimRecord, err := encode(claimEvent)
	if err != nil {
		return events.StreamPosition{}, err
	}
	sequences, err := p.log.AppendBatch(ctx, []events.EncodedBatchEntry{
		{Subject: accountSubject, Record: changeRecord, ExpectedSeq: expectedAccount, HasOCC: true},
		{Subject: accountRegistrySubject, Record: claimRecord, ExpectedSeq: expectedRegistry, HasOCC: true},
	})
	if err != nil {
		return events.StreamPosition{}, err
	}
	return events.SubjectPosition(accountRegistrySubject, sequences[1]), nil
}

// AppendIssuerEstablished creates the singleton issuer aggregate.
func (p *Publisher) AppendIssuerEstablished(ctx context.Context, event *corev1.Event) (events.StreamPosition, error) {
	if event.GetIssuerEstablished() == nil {
		return events.StreamPosition{}, fmt.Errorf("append issuer established: event payload is not issuer_established")
	}
	record, err := encode(event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	sequence, err := p.log.AppendAt(ctx, issuerSubject, record, 0)
	if err != nil {
		return events.StreamPosition{}, err
	}
	return events.SubjectPosition(issuerSubject, sequence), nil
}

// Decode validates and decodes one persisted Authling event.
func Decode(data []byte) (events.DecodedEvent[*corev1.Event], error) {
	var event corev1.Event
	if err := proto.Unmarshal(data, &event); err != nil {
		return events.DecodedEvent[*corev1.Event]{}, fmt.Errorf("decode Authling event: %w", err)
	}
	if err := validate(&event); err != nil {
		return events.DecodedEvent[*corev1.Event]{}, err
	}
	return events.DecodedEvent[*corev1.Event]{Event: &event, ID: event.GetId()}, nil
}

func encode(event *corev1.Event) (events.EncodedRecord, error) {
	if err := validate(event); err != nil {
		return events.EncodedRecord{}, err
	}
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(event)
	if err != nil {
		return events.EncodedRecord{}, fmt.Errorf("encode Authling event: %w", err)
	}
	return events.EncodedRecord{ID: event.GetId(), Data: data}, nil
}

func validate(event *corev1.Event) error {
	if event == nil {
		return fmt.Errorf("Authling event is nil")
	}
	if strings.TrimSpace(event.GetId()) == "" {
		return fmt.Errorf("Authling event id is required")
	}
	if event.GetCreatedAt() == nil {
		return fmt.Errorf("Authling event created_at is required")
	}
	if err := event.GetCreatedAt().CheckValid(); err != nil {
		return fmt.Errorf("Authling event created_at: %w", err)
	}
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_AccountCreated:
		if _, err := AccountSubject(payload.AccountCreated.GetAccountId()); err != nil {
			return err
		}
		credential := payload.AccountCreated
		hasCredential := credential.GetCredentialEnvelopeVersion() != 0 || credential.GetUserKeyRef() != "" || credential.GetCredentialKeyRef() != "" || len(credential.GetEmailNonce()) != 0 || len(credential.GetEmailCiphertext()) != 0 || len(credential.GetPasswordVerifierNonce()) != 0 || len(credential.GetPasswordVerifierCiphertext()) != 0
		if hasCredential {
			if credential.GetCredentialEnvelopeVersion() != 1 || !validSubjectToken(credential.GetUserKeyRef()) || !validSubjectToken(credential.GetCredentialKeyRef()) || len(credential.GetEmailNonce()) == 0 || len(credential.GetEmailCiphertext()) == 0 || len(credential.GetPasswordVerifierNonce()) == 0 || len(credential.GetPasswordVerifierCiphertext()) == 0 {
				return fmt.Errorf("account credential envelope is incomplete or unsupported")
			}
		}
	case *corev1.Event_EmailClaimed:
		if !validSubjectToken(payload.EmailClaimed.GetAccountId()) {
			return fmt.Errorf("invalid account id")
		}
		if credentialEventID := payload.EmailClaimed.GetCredentialEventId(); credentialEventID != "" && !validSubjectToken(credentialEventID) {
			return fmt.Errorf("email claim credential event id is invalid")
		}
	case *corev1.Event_PasswordChanged:
		credential := payload.PasswordChanged
		if !validSubjectToken(credential.GetAccountId()) || credential.GetCredentialEnvelopeVersion() != 1 || !validSubjectToken(credential.GetUserKeyRef()) || !validSubjectToken(credential.GetCredentialKeyRef()) || len(credential.GetPasswordVerifierNonce()) == 0 || len(credential.GetPasswordVerifierCiphertext()) == 0 {
			return fmt.Errorf("password credential envelope is incomplete or unsupported")
		}
		if requestID := credential.GetPasswordResetRequestEventId(); requestID != "" && !validSubjectToken(requestID) {
			return fmt.Errorf("password reset request event id is invalid")
		}
	case *corev1.Event_PasswordResetRequested:
		request := payload.PasswordResetRequested
		if !validSubjectToken(request.GetAccountId()) || !validSubjectToken(request.GetCredentialEventId()) {
			return fmt.Errorf("password reset request is incomplete")
		}
	case *corev1.Event_EmailChangeRequested:
		request := payload.EmailChangeRequested
		if !validSubjectToken(request.GetAccountId()) || !validSubjectToken(request.GetCredentialEventId()) {
			return fmt.Errorf("email change request is incomplete")
		}
	case *corev1.Event_EmailChanged:
		credential := payload.EmailChanged
		if !validSubjectToken(credential.GetAccountId()) || credential.GetCredentialEnvelopeVersion() != 1 || !validSubjectToken(credential.GetUserKeyRef()) || !validSubjectToken(credential.GetCredentialKeyRef()) || len(credential.GetEmailNonce()) == 0 || len(credential.GetEmailCiphertext()) == 0 || !validSubjectToken(credential.GetEmailChangeRequestEventId()) || !validSubjectToken(credential.GetPriorCredentialEventId()) {
			return fmt.Errorf("email credential envelope is incomplete or unsupported")
		}
	case *corev1.Event_IssuerEstablished:
		if payload.IssuerEstablished.GetIssuer() == "" || payload.IssuerEstablished.GetSigningKeyRef() == "" || payload.IssuerEstablished.GetSigningKeyId() == "" {
			return fmt.Errorf("issuer establishment is incomplete")
		}
	default:
		return fmt.Errorf("Authling event payload is required")
	}
	return nil
}

// AccountSubject returns the durable subject for one account aggregate.
func AccountSubject(accountID string) (string, error) {
	if !validSubjectToken(accountID) {
		return "", fmt.Errorf("invalid account id")
	}
	return accountSubjectPrefix + accountID, nil
}

func validSubjectToken(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' ||
			char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' ||
			char == '_' ||
			char == '-' {
			continue
		}
		return false
	}
	return true
}
