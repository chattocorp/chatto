// Package issuer owns Authling's immutable OpenID Connect issuer identity.
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

// State is the durable identity of one Authling deployment.
type State struct {
	Issuer        string
	SigningKeyRef string
	SigningKeyID  string
}

// Projection rebuilds the singleton issuer identity.
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

// Apply establishes the issuer exactly once.
func (p *Projection) Apply(event *corev1.Event, _ uint64) error {
	payload := event.GetIssuerEstablished()
	if payload == nil {
		return fmt.Errorf("unsupported issuer event")
	}
	next := State{Issuer: payload.GetIssuer(), SigningKeyRef: payload.GetSigningKeyRef(), SigningKeyID: payload.GetSigningKeyId()}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.set {
		return fmt.Errorf("issuer was established more than once")
	}
	p.state = next
	p.set = true
	return nil
}

// Get returns the established issuer, if any.
func (p *Projection) Get() (State, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state, p.set
}

// Service validates startup configuration against the durable issuer and owns
// access to its corresponding private signing key.
type Service struct {
	publisher        *evtstream.Publisher
	handle           events.ProjectionHandle[*Projection]
	vault            *keyvault.Vault
	configuredIssuer string

	mu    sync.RWMutex
	state State
	key   keyvault.SigningKey
}

// NewService constructs the issuer boundary. Initialize must run after startup
// replay and before serving HTTP.
func NewService(publisher *evtstream.Publisher, handle events.ProjectionHandle[*Projection], vault *keyvault.Vault, configuredIssuer string) *Service {
	return &Service{publisher: publisher, handle: handle, vault: vault, configuredIssuer: strings.TrimSuffix(configuredIssuer, "/")}
}

// Initialize establishes the first issuer or rejects configuration drift.
func (s *Service) Initialize(ctx context.Context) error {
	key, err := s.vault.OIDCSigningKey(ctx)
	if err != nil {
		return err
	}
	state, exists := s.handle.Projection().Get()
	if !exists {
		eventID, err := ids.New("evt")
		if err != nil {
			return err
		}
		event := &corev1.Event{
			Id: eventID, CreatedAt: timestamppb.New(time.Now().UTC()),
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
		if !exists {
			return fmt.Errorf("established OIDC issuer is absent from projection")
		}
	}
	if state.Issuer != s.configuredIssuer {
		return fmt.Errorf("configured public URL %q does not match immutable OIDC issuer %q", s.configuredIssuer, state.Issuer)
	}
	if state.SigningKeyRef != key.Ref || state.SigningKeyID != key.ID {
		return fmt.Errorf("OIDC signing key does not match durable issuer identity")
	}
	s.mu.Lock()
	s.state, s.key = state, key
	s.mu.Unlock()
	return nil
}

// State returns the initialized issuer identity.
func (s *Service) State() (State, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state, s.state.Issuer != ""
}

// SigningKey returns the initialized private signing key.
func (s *Service) SigningKey() (keyvault.SigningKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.key, s.key.Private != nil
}
