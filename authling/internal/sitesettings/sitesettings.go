// Package sitesettings owns the event-backed public presentation settings.
package sitesettings

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/chattocorp/chatto/authling/internal/config"
	"github.com/chattocorp/chatto/authling/internal/evtstream"
	"github.com/chattocorp/chatto/authling/internal/ids"
	corev1 "github.com/chattocorp/chatto/authling/internal/pb/authling/core/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// These errors contain no submitted text or account identifiers.
var (
	ErrInvalid   = errors.New("invalid site settings")
	ErrConflict  = errors.New("site settings changed")
	ErrForbidden = errors.New("administrator access required")
)

// Snapshot carries the event stream sequence an editor must submit with its
// replacement. Revision zero means that only server configuration exists.
type Snapshot struct {
	Site           config.SiteConfig
	Revision       uint64
	ActorAccountID string
	ChangedAt      time.Time
}

// Projection rebuilds current settings and validates the replacement chain.
type Projection struct {
	events.MemoryProjection
	mu    sync.RWMutex
	state Snapshot
}

// NewProjection constructs an empty settings projection.
func NewProjection() *Projection { return &Projection{} }

// Subjects selects only the singleton settings aggregate.
func (*Projection) Subjects() []string { return []string{evtstream.SiteSettingsSubject()} }

// Apply validates metadata and text before advancing the materialized state.
func (p *Projection) Apply(event *corev1.Event, sequence uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	change := event.GetSiteSettingsChanged()
	if change == nil || change.GetSettings() == nil || event.GetId() == "" || event.GetCreatedAt() == nil || event.GetCreatedAt().CheckValid() != nil || change.GetActorAccountId() == "" || change.GetPreviousRevision() != p.state.Revision || sequence <= p.state.Revision {
		return ErrInvalid
	}
	record := change.GetSettings()
	site := config.SiteConfig{Name: record.Name, Description: record.Description, Introduction: record.Introduction}
	if err := Validate(site); err != nil {
		return err
	}
	p.state = Snapshot{Site: site, Revision: sequence, ActorAccountID: change.GetActorAccountId(), ChangedAt: event.GetCreatedAt().AsTime()}
	return nil
}

// Get returns a value copy; no caller can mutate projected state.
func (p *Projection) Get() Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// Service applies current authorization and subject-level optimistic concurrency.
type Service struct {
	publisher *evtstream.Publisher
	handle    events.ProjectionHandle[*Projection]
	defaults  config.SiteConfig
	authorize func(context.Context, string) error
}

// New binds the command service to its required projection. Authorization must
// check current administrator access; nil denies all writes.
func New(publisher *evtstream.Publisher, handle events.ProjectionHandle[*Projection], defaults config.SiteConfig, authorize func(context.Context, string) error) *Service {
	return &Service{publisher: publisher, handle: handle, defaults: defaults, authorize: authorize}
}

// Read waits for the authoritative subject tail before reading the projection.
// Defaults apply only when no settings event exists, never on a storage failure.
func (s *Service) Read(ctx context.Context) (Snapshot, error) {
	tail, err := s.publisher.LastSubjectSeq(ctx, evtstream.SiteSettingsSubject())
	if err != nil {
		return Snapshot{}, err
	}
	if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(evtstream.SiteSettingsSubject(), tail)); err != nil {
		return Snapshot{}, err
	}
	state := s.handle.Projection().Get()
	if state.Revision == 0 {
		state.Site = s.defaults
	}
	return state, nil
}

// Name resolves current branding for transactional email.
func (s *Service) Name(ctx context.Context) (string, error) {
	snapshot, err := s.Read(ctx)
	return snapshot.Site.Name, err
}

// Validate checks text before trimming, including email-header safety.
func Validate(site config.SiteConfig) error {
	if strings.TrimSpace(site.Name) == "" || site.Validate() != nil {
		return ErrInvalid
	}
	return nil
}

// Save records the actor, timestamp, and replacement text, then waits for local
// application. A conflict refreshes the projection but never retries stale input.
func (s *Service) Save(ctx context.Context, actor string, input Snapshot) error {
	if err := Validate(input.Site); err != nil {
		return err
	}
	if s.authorize == nil {
		return ErrForbidden
	}
	if err := s.authorize(ctx, actor); err != nil {
		return err
	}
	current, err := s.Read(ctx)
	if err != nil {
		return err
	}
	if current.Revision != input.Revision {
		return ErrConflict
	}
	event, err := newEvent(actor, input)
	if err != nil {
		return err
	}
	// Projection synchronization can block, so check privileges again at commit.
	if err := s.authorize(ctx, actor); err != nil {
		return err
	}
	position, err := s.publisher.AppendSiteSettingsChanged(ctx, event, input.Revision)
	if errors.Is(err, events.ErrConflict) {
		if _, readErr := s.Read(ctx); readErr != nil {
			return readErr
		}
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return s.handle.Projector().WaitFor(ctx, position)
}

func newEvent(actor string, input Snapshot) (*corev1.Event, error) {
	id, err := ids.New("evt")
	if err != nil {
		return nil, err
	}
	site := input.Site.Resolve("")
	change := &corev1.SiteSettingsChangedEvent{ActorAccountId: actor, PreviousRevision: input.Revision, Settings: &corev1.SiteSettings{Name: site.Name, Description: site.Description, Introduction: site.Introduction}}
	event := &corev1.Event{Id: id, CreatedAt: timestamppb.Now(), Event: &corev1.Event_SiteSettingsChanged{SiteSettingsChanged: change}}
	return event, nil
}
