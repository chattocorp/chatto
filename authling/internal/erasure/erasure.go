// Package erasure owns the key-free erasure index and durable destruction worker.
package erasure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/ids"
	"hmans.de/authling/internal/keyvault"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/chatto/pkg/events"
)

// State is non-secret erasure evidence. A non-empty RequestID denies access;
// Complete records that the live key store no longer contains the account keys.
type State struct {
	AccountID, UserRef, DataRef, RequestID string
	RequestSequence, ReleaseSequence       uint64
	Complete                               bool
}

// Projection must replay before projections that decrypt account history.
// Retained key ownership prevents an erasure request from naming another key.
type Projection struct {
	events.MemoryProjection
	accounts map[string]State
	owners   map[string]struct{}
}

// NewProjection creates an empty structural erasure index.
func NewProjection() *Projection {
	return &Projection{accounts: map[string]State{}, owners: map[string]struct{}{}}
}

// Subjects includes account facts and correlated registry releases.
func (*Projection) Subjects() []string {
	return []string{evtstream.AccountSubjectFilter, evtstream.AccountRegistrySubject()}
}

// Apply validates erasure ordering and retains terminal account tombstones.
func (p *Projection) Apply(e *corev1.Event, sequence uint64) error {
	id := evtstream.EventAccountID(e)
	if id == "" {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	state, exists := p.accounts[id]
	if c := e.GetAccountCreated(); c != nil {
		if exists {
			return fmt.Errorf("duplicate account in erasure index")
		}
		for _, ref := range []string{c.GetUserKeyRef(), c.GetCredentialKeyRef()} {
			if ref != "" {
				if _, used := p.owners[ref]; used {
					return fmt.Errorf("account key reference reused")
				}
				p.owners[ref] = struct{}{}
			}
		}
		p.accounts[id] = State{AccountID: id, UserRef: c.GetUserKeyRef(), DataRef: c.GetCredentialKeyRef()}
		return nil
	}
	if !exists {
		return fmt.Errorf("erasure index event references absent account")
	}
	if r := e.GetAccountErasureRequested(); r != nil {
		if state.RequestID != "" || state.UserRef != r.GetUserKeyRef() || state.DataRef != r.GetCredentialKeyRef() {
			return fmt.Errorf("erasure request has invalid key ownership or ordering")
		}
		state.RequestID = e.GetId()
		state.RequestSequence = sequence
	} else if r := e.GetEmailReleased(); r != nil {
		if state.RequestID == "" || state.RequestID != r.GetErasureRequestEventId() || state.ReleaseSequence != 0 {
			return fmt.Errorf("email release has invalid erasure correlation")
		}
		state.ReleaseSequence = sequence
	} else if r := e.GetAccountErased(); r != nil {
		if state.RequestID == "" || state.RequestID != r.GetErasureRequestEventId() || state.ReleaseSequence == 0 || state.Complete {
			return fmt.Errorf("erasure completion has invalid ordering")
		}
		state.Complete = true
	} else if state.RequestID != "" {
		return fmt.Errorf("account event follows erasure request")
	}
	p.accounts[id] = state
	return nil
}

// Get returns a copy of the current structural state.
func (p *Projection) Get(id string) (State, bool) {
	p.RLock()
	defer p.RUnlock()
	s, ok := p.accounts[id]
	return s, ok
}

// Service synchronizes erasure evidence and performs idempotent destruction.
type Service struct {
	publisher *evtstream.Publisher
	handle    events.ProjectionHandle[*Projection]
	vault     *keyvault.Vault
	worker    *events.DurableWorker
	// waitAccount ensures account and grant projections validate the decision.
	waitAccount func(context.Context, State) error
}

// New wires erasure evidence to its account event log and key store.
func New(p *evtstream.Publisher, h events.ProjectionHandle[*Projection], v *keyvault.Vault) *Service {
	return &Service{publisher: p, handle: h, vault: v}
}

// IsRequested crosses the account tail before reporting an erasure tombstone.
// Storage failures are errors, never proof that an account has been erased.
func (s *Service) IsRequested(ctx context.Context, id string) (bool, error) {
	tail, err := s.publisher.AccountTail(ctx, id)
	if err != nil {
		return false, err
	}
	if tail > 0 {
		subject, _ := evtstream.AccountSubject(id)
		if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(subject, tail)); err != nil {
			return false, err
		}
	}
	state, _ := s.handle.Projection().Get(id)
	return state.RequestID != "", nil
}

// ConfigureWorker creates one shared durable consumer. Replicas may share it;
// key purges are idempotent and completion uses account OCC.
func (s *Service) ConfigureWorker(ctx context.Context, stream jetstream.Stream, logger events.Logger, wait func(context.Context, State) error) error {
	if wait == nil {
		return fmt.Errorf("erasure validation barrier is required")
	}
	s.waitAccount = wait
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{Name: "authling-account-erasure", Durable: "authling-account-erasure", FilterSubject: evtstream.AccountSubjectFilter, AckPolicy: jetstream.AckExplicitPolicy, AckWait: time.Minute, MaxAckPending: 1})
	if err != nil {
		return err
	}
	s.worker, err = events.NewDurableWorker(consumer, s.process, events.DurableWorkerOptions{MaxConcurrent: 1, RetryDelay: time.Second, Logger: logger})
	return err
}

// Run processes durable erasure work until cancellation. ConfigureWorker must
// finish before Run starts, and the required projectors must run concurrently.
func (s *Service) Run(ctx context.Context) error {
	if s.worker == nil {
		return fmt.Errorf("erasure worker is not configured")
	}
	return s.worker.Run(ctx)
}
func (s *Service) process(ctx context.Context, d events.DurableDelivery) error {
	decoded, err := evtstream.Decode(d.Data)
	if err != nil {
		return err
	}
	if decoded.Event.GetAccountErasureRequested() == nil {
		return nil
	}
	return s.Complete(ctx, decoded.Event.GetAccountErasureRequested().GetAccountId())
}

// Complete resumes an accepted erasure. It never authorizes deletion itself.
func (s *Service) Complete(ctx context.Context, id string) error {
	if s.waitAccount == nil {
		return fmt.Errorf("erasure validation barrier is required")
	}
	for range 5 {
		requested, err := s.IsRequested(ctx, id)
		if err != nil {
			return err
		}
		if !requested {
			return fmt.Errorf("erasure not requested")
		}
		registryTail, err := s.publisher.AccountRegistryTail(ctx)
		if err != nil {
			return err
		}
		if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(evtstream.AccountRegistrySubject(), registryTail)); err != nil {
			return err
		}
		state, _ := s.handle.Projection().Get(id)
		if state.Complete {
			return nil
		}
		if state.ReleaseSequence == 0 {
			return fmt.Errorf("erasure email release pending")
		}
		if err := s.waitAccount(ctx, state); err != nil {
			return err
		}
		if err := s.vault.DestroyAccountKeys(ctx, state.UserRef, state.DataRef); err != nil {
			return err
		}
		tail, err := s.publisher.AccountTail(ctx, id)
		if err != nil {
			return err
		}
		// A second worker may have completed while the first destroyed the keys.
		if _, err := s.IsRequested(ctx, id); err != nil {
			return err
		}
		current, _ := s.handle.Projection().Get(id)
		if current.Complete {
			return nil
		}
		eventID, err := ids.New("evt")
		if err != nil {
			return err
		}
		event := &corev1.Event{Id: eventID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_AccountErased{AccountErased: &corev1.AccountErasedEvent{AccountId: id, ErasureRequestEventId: state.RequestID}}}
		position, err := s.publisher.AppendAccountErased(ctx, event, tail)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		return s.handle.Projector().WaitFor(ctx, position)
	}
	return fmt.Errorf("erasure completion conflict")
}
