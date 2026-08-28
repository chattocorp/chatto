// Package issuer owns Authling's immutable OpenID Connect issuer identity and
// its automatically rotated signing-key lifecycle.
package issuer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/ids"
	"hmans.de/authling/internal/keyvault"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	// SigningKeyPublicationLead keeps a prepared public key in JWKS for twice
	// the endpoint's five-minute cache lifetime before it may sign tokens.
	SigningKeyPublicationLead = 10 * time.Minute
	// SigningKeyRetirementOverlap covers Authling's five-minute ID-token
	// lifetime, five minutes of verifier clock skew, and five minutes of
	// reconciliation and operational margin before the old public key disappears.
	SigningKeyRetirementOverlap = 15 * time.Minute
	defaultReconcileInterval    = time.Minute
)

// Key identifies signing material without exposing its private value.
type Key struct {
	Ref         string
	ID          string
	ActivatedAt time.Time
	ActivateAt  time.Time
	RetireAfter time.Time
}

// State is the durable identity and current signing-key lifecycle of one
// Authling deployment.
type State struct {
	Issuer       string
	Active       Key
	RotationRef  string
	Prepared     Key
	HasPrepared  bool
	Retiring     Key
	HasRetiring  bool
	RetireQueued bool
}

// Projection rebuilds the singleton issuer identity and key lifecycle.
type Projection struct {
	events.MemoryProjection
	mu    sync.RWMutex
	state State
	set   bool
}

// NewProjection constructs an empty issuer projection.
func NewProjection() *Projection { return &Projection{} }

// Subjects returns the singleton issuer aggregate.
func (*Projection) Subjects() []string { return []string{evtstream.IssuerSubject()} }

// Apply validates and materializes one issuer lifecycle transition.
func (p *Projection) Apply(event *corev1.Event, _ uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	createdAt := event.GetCreatedAt().AsTime()
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_IssuerEstablished:
		if p.set {
			return fmt.Errorf("issuer was established more than once")
		}
		p.state = State{
			Issuer: payload.IssuerEstablished.GetIssuer(),
			Active: Key{
				Ref: payload.IssuerEstablished.GetSigningKeyRef(), ID: payload.IssuerEstablished.GetSigningKeyId(), ActivatedAt: createdAt,
			},
		}
		p.set = true
	case *corev1.Event_OidcSigningKeyRotationRequested:
		ref := payload.OidcSigningKeyRotationRequested.GetSigningKeyRef()
		if !p.set || p.state.RotationRef != "" || p.state.HasPrepared || p.state.HasRetiring || ref == p.state.Active.Ref {
			return fmt.Errorf("OIDC signing-key rotation request is out of order")
		}
		p.state.RotationRef = ref
	case *corev1.Event_OidcSigningKeyPrepared:
		prepared := payload.OidcSigningKeyPrepared
		if p.state.RotationRef == "" || prepared.GetSigningKeyRef() != p.state.RotationRef || p.state.HasPrepared || prepared.GetSigningKeyId() == p.state.Active.ID {
			return fmt.Errorf("OIDC signing-key preparation is out of order")
		}
		p.state.Prepared = Key{Ref: prepared.GetSigningKeyRef(), ID: prepared.GetSigningKeyId(), ActivateAt: prepared.GetActivateAt().AsTime()}
		p.state.HasPrepared = true
	case *corev1.Event_OidcSigningKeyActivated:
		activated := payload.OidcSigningKeyActivated
		if !p.state.HasPrepared || createdAt.Before(p.state.Prepared.ActivateAt) || activated.GetSigningKeyRef() != p.state.Prepared.Ref || activated.GetSigningKeyId() != p.state.Prepared.ID || activated.GetPreviousSigningKeyRef() != p.state.Active.Ref || activated.GetPreviousSigningKeyId() != p.state.Active.ID || p.state.HasRetiring {
			return fmt.Errorf("OIDC signing-key activation is out of order")
		}
		previous := p.state.Active
		previous.RetireAfter = activated.GetRetireAfter().AsTime()
		p.state.Active = Key{Ref: p.state.Prepared.Ref, ID: p.state.Prepared.ID, ActivatedAt: createdAt}
		p.state.Retiring = previous
		p.state.HasRetiring = true
		p.state.RotationRef = ""
		p.state.Prepared = Key{}
		p.state.HasPrepared = false
	case *corev1.Event_OidcSigningKeyRetirementRequested:
		retiring := payload.OidcSigningKeyRetirementRequested
		if !p.state.HasRetiring || p.state.RetireQueued || createdAt.Before(p.state.Retiring.RetireAfter) || retiring.GetSigningKeyRef() != p.state.Retiring.Ref || retiring.GetSigningKeyId() != p.state.Retiring.ID {
			return fmt.Errorf("OIDC signing-key retirement request is out of order")
		}
		p.state.RetireQueued = true
	case *corev1.Event_OidcSigningKeyRetired:
		retired := payload.OidcSigningKeyRetired
		if !p.state.HasRetiring || !p.state.RetireQueued || retired.GetSigningKeyRef() != p.state.Retiring.Ref || retired.GetSigningKeyId() != p.state.Retiring.ID {
			return fmt.Errorf("OIDC signing-key retirement is out of order")
		}
		p.state.Retiring = Key{}
		p.state.HasRetiring = false
		p.state.RetireQueued = false
	default:
		return fmt.Errorf("unsupported issuer event")
	}
	return nil
}

// Get returns a copy of the established issuer state, if any.
func (p *Projection) Get() (State, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state, p.set
}

// Option customizes lifecycle timing for deterministic tests.
type Option func(*Service)

// WithClock replaces the wall clock used to decide lifecycle transitions.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// WithLifecycleTiming replaces the publication, retirement, and reconciliation
// intervals. It is intended for deterministic protocol and worker tests.
func WithLifecycleTiming(publicationLead, retirementOverlap, reconcileInterval time.Duration) Option {
	return func(s *Service) {
		s.publicationLead = publicationLead
		s.retirementOverlap = retirementOverlap
		s.reconcileInterval = reconcileInterval
	}
}

// Service validates startup configuration, resolves private signing material,
// and reconciles the automatic key-rotation state machine.
type Service struct {
	publisher         *evtstream.Publisher
	handle            events.ProjectionHandle[*Projection]
	vault             *keyvault.Vault
	configuredIssuer  string
	rotationInterval  time.Duration
	publicationLead   time.Duration
	retirementOverlap time.Duration
	reconcileInterval time.Duration
	now               func() time.Time
	ready             chan struct{}
	readyOnce         sync.Once
}

// NewService constructs the issuer boundary. Initialize must run after startup
// replay and before serving HTTP.
func NewService(publisher *evtstream.Publisher, handle events.ProjectionHandle[*Projection], vault *keyvault.Vault, configuredIssuer string, rotationInterval time.Duration, options ...Option) *Service {
	service := &Service{
		publisher: publisher, handle: handle, vault: vault,
		configuredIssuer: strings.TrimSuffix(configuredIssuer, "/"), rotationInterval: rotationInterval,
		publicationLead: SigningKeyPublicationLead, retirementOverlap: SigningKeyRetirementOverlap,
		reconcileInterval: defaultReconcileInterval, now: time.Now, ready: make(chan struct{}),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// Initialize establishes the first issuer or rejects configuration and key
// drift. Existing rotated deployments resolve their active key by event ref.
func (s *Service) Initialize(ctx context.Context) error {
	state, exists := s.handle.Projection().Get()
	if !exists {
		key, err := s.vault.OIDCSigningKey(ctx)
		if err != nil {
			return err
		}
		eventID, err := ids.New("evt")
		if err != nil {
			return err
		}
		event := &corev1.Event{
			Id: eventID, CreatedAt: timestamppb.New(s.now().UTC()),
			Event: &corev1.Event_IssuerEstablished{IssuerEstablished: &corev1.IssuerEstablishedEvent{
				Issuer: s.configuredIssuer, SigningKeyRef: key.Ref, SigningKeyId: key.ID,
			}},
		}
		position, appendErr := s.publisher.AppendIssuerEstablished(ctx, event)
		if appendErr != nil {
			if !errors.Is(appendErr, events.ErrConflict) {
				return fmt.Errorf("establish OIDC issuer: %w", appendErr)
			}
			tail, tailErr := s.publisher.IssuerTail(ctx)
			if tailErr != nil {
				return fmt.Errorf("read raced OIDC issuer: %w", tailErr)
			}
			position = events.SubjectPosition(evtstream.IssuerSubject(), tail)
		}
		if err := s.handle.Projector().WaitFor(ctx, position); err != nil {
			return fmt.Errorf("wait for OIDC issuer: %w", err)
		}
		state, exists = s.handle.Projection().Get()
	}
	if !exists {
		return fmt.Errorf("established OIDC issuer is absent from projection")
	}
	if state.Issuer != s.configuredIssuer {
		return fmt.Errorf("configured public URL %q does not match immutable OIDC issuer %q", s.configuredIssuer, state.Issuer)
	}
	if _, err := s.SigningKey(ctx); err != nil {
		return err
	}
	if _, err := s.VerificationKeys(ctx); err != nil {
		return err
	}
	s.readyOnce.Do(func() { close(s.ready) })
	return nil
}

// Run automatically advances due signing-key lifecycle transitions.
func (s *Service) Run(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case <-s.ready:
	}
	for {
		if err := s.Reconcile(ctx); err != nil {
			return fmt.Errorf("reconcile OIDC signing keys: %w", err)
		}
		timer := time.NewTimer(s.reconcileInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
	}
}

// Reconcile advances all immediately actionable lifecycle steps. OCC conflicts
// are harmless: the authoritative projection is refreshed before retrying.
func (s *Service) Reconcile(ctx context.Context) error {
	for range 8 {
		tail, err := s.publisher.IssuerTail(ctx)
		if err != nil {
			return err
		}
		if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(evtstream.IssuerSubject(), tail)); err != nil {
			return err
		}
		state, ok := s.handle.Projection().Get()
		if !ok {
			return fmt.Errorf("OIDC issuer is not initialized")
		}
		now := s.now().UTC()
		var event *corev1.Event
		switch {
		case state.RetireQueued:
			if err := s.vault.DestroyOIDCSigningKey(ctx, state.Retiring.Ref); err != nil {
				return err
			}
			event, err = newEvent(now, &corev1.Event_OidcSigningKeyRetired{OidcSigningKeyRetired: &corev1.OIDCSigningKeyRetiredEvent{SigningKeyRef: state.Retiring.Ref, SigningKeyId: state.Retiring.ID}})
		case state.HasRetiring && !now.Before(state.Retiring.RetireAfter):
			event, err = newEvent(now, &corev1.Event_OidcSigningKeyRetirementRequested{OidcSigningKeyRetirementRequested: &corev1.OIDCSigningKeyRetirementRequestedEvent{SigningKeyRef: state.Retiring.Ref, SigningKeyId: state.Retiring.ID}})
		case state.HasPrepared && !now.Before(state.Prepared.ActivateAt):
			event, err = newEvent(now, &corev1.Event_OidcSigningKeyActivated{OidcSigningKeyActivated: &corev1.OIDCSigningKeyActivatedEvent{
				SigningKeyRef: state.Prepared.Ref, SigningKeyId: state.Prepared.ID,
				PreviousSigningKeyRef: state.Active.Ref, PreviousSigningKeyId: state.Active.ID,
				RetireAfter: timestamppb.New(now.Add(s.retirementOverlap)),
			}})
		case state.RotationRef != "" && !state.HasPrepared:
			key, err := s.vault.EnsureOIDCSigningKey(ctx, state.RotationRef)
			if err != nil {
				return err
			}
			event, err = newEvent(now, &corev1.Event_OidcSigningKeyPrepared{OidcSigningKeyPrepared: &corev1.OIDCSigningKeyPreparedEvent{
				SigningKeyRef: key.Ref, SigningKeyId: key.ID, ActivateAt: timestamppb.New(now.Add(s.publicationLead)),
			}})
		case !state.HasRetiring && !state.HasPrepared && state.RotationRef == "" && !now.Before(state.Active.ActivatedAt.Add(s.rotationInterval)):
			keyID, err := ids.New("key")
			if err != nil {
				return err
			}
			event, err = newEvent(now, &corev1.Event_OidcSigningKeyRotationRequested{OidcSigningKeyRotationRequested: &corev1.OIDCSigningKeyRotationRequestedEvent{SigningKeyRef: "system.oidc-signing." + keyID}})
		default:
			return nil
		}
		if err != nil {
			return err
		}
		position, err := s.publisher.AppendIssuerLifecycle(ctx, event, tail)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		if err := s.handle.Projector().WaitFor(ctx, position); err != nil {
			return err
		}
	}
	return fmt.Errorf("OIDC signing-key reconciliation exceeded transition bound")
}

// State returns the established issuer, if any.
func (s *Service) State() (State, bool) { return s.handle.Projection().Get() }

// SigningKey resolves the current private signing key and fails closed if the
// key store does not match the durable issuer projection.
func (s *Service) SigningKey(ctx context.Context) (keyvault.SigningKey, error) {
	state, ok := s.handle.Projection().Get()
	if !ok || state.Active.Ref == "" {
		return keyvault.SigningKey{}, fmt.Errorf("OIDC issuer is not initialized")
	}
	key, err := s.vault.ResolveOIDCSigningKey(ctx, state.Active.Ref)
	if err != nil {
		return keyvault.SigningKey{}, err
	}
	if key.ID != state.Active.ID {
		return keyvault.SigningKey{}, fmt.Errorf("OIDC signing key does not match durable issuer identity")
	}
	return key, nil
}

// VerificationKeys resolves every public key that must currently appear in
// JWKS: the active key, a pre-published successor, and an unexpired predecessor.
func (s *Service) VerificationKeys(ctx context.Context) ([]keyvault.SigningKey, error) {
	state, ok := s.handle.Projection().Get()
	if !ok {
		return nil, fmt.Errorf("OIDC issuer is not initialized")
	}
	wanted := []Key{state.Active}
	if state.HasPrepared {
		wanted = append(wanted, state.Prepared)
	}
	if state.HasRetiring && !state.RetireQueued {
		wanted = append(wanted, state.Retiring)
	}
	keys := make([]keyvault.SigningKey, 0, len(wanted))
	for _, expected := range wanted {
		key, err := s.vault.ResolveOIDCSigningKey(ctx, expected.Ref)
		if err != nil {
			return nil, err
		}
		if key.ID != expected.ID {
			return nil, fmt.Errorf("OIDC verification key does not match durable issuer identity")
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func newEvent(now time.Time, payload any) (*corev1.Event, error) {
	id, err := ids.New("evt")
	if err != nil {
		return nil, err
	}
	event := &corev1.Event{Id: id, CreatedAt: timestamppb.New(now)}
	switch payload := payload.(type) {
	case *corev1.Event_OidcSigningKeyRotationRequested:
		event.Event = payload
	case *corev1.Event_OidcSigningKeyPrepared:
		event.Event = payload
	case *corev1.Event_OidcSigningKeyActivated:
		event.Event = payload
	case *corev1.Event_OidcSigningKeyRetirementRequested:
		event.Event = payload
	case *corev1.Event_OidcSigningKeyRetired:
		event.Event = payload
	default:
		return nil, fmt.Errorf("unsupported issuer lifecycle payload")
	}
	return event, nil
}
