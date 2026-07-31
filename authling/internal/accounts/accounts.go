// Package accounts owns Authling's account aggregate and in-memory model.
package accounts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/ids"
	"hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/chatto/pkg/events"
)

// ErrIDCollision indicates that a generated account ID already has durable
// history. Callers may retry the command with a fresh generated ID.
var ErrIDCollision = errors.New("generated account id already exists")

// Account is the current projected structural state of an Authling account.
type Account struct {
	ID        string
	CreatedAt time.Time
}

// Projection rebuilds the active account registry from durable events.
type Projection struct {
	events.MemoryProjection
	accounts map[string]Account
}

// Subjects returns the account event family consumed by this projection.
func (*Projection) Subjects() []string {
	return []string{evtstream.AccountSubjectFilter}
}

// Apply adds one durable account fact to the in-memory registry.
func (p *Projection) Apply(event *corev1.Event, _ uint64) error {
	payload := event.GetAccountCreated()
	if payload == nil {
		return fmt.Errorf("unsupported account event")
	}
	account := Account{
		ID:        payload.GetAccountId(),
		CreatedAt: event.GetCreatedAt().AsTime(),
	}

	p.Lock()
	defer p.Unlock()
	if p.accounts == nil {
		p.accounts = make(map[string]Account)
	}
	if _, exists := p.accounts[account.ID]; exists {
		return fmt.Errorf("account %q was created more than once", account.ID)
	}
	p.accounts[account.ID] = account
	return nil
}

// Get returns one projected account.
func (p *Projection) Get(accountID string) (Account, bool) {
	p.RLock()
	defer p.RUnlock()
	account, ok := p.accounts[accountID]
	return account, ok
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
	publisher *evtstream.Publisher
	handle    events.ProjectionHandle[*Projection]
}

// NewService constructs the account command and read boundary.
func NewService(
	publisher *evtstream.Publisher,
	handle events.ProjectionHandle[*Projection],
) *Service {
	return &Service{publisher: publisher, handle: handle}
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

// Get returns one account from the ready in-memory projection.
func (s *Service) Get(accountID string) (Account, bool) {
	return s.handle.Projection().Get(accountID)
}

// Count returns the number of projected accounts.
func (s *Service) Count() int {
	return s.handle.Projection().Count()
}
