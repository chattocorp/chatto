package core

import (
	"sort"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// InvitationState is the durable, administrator-visible state derived for one invitation.
type InvitationState struct {
	ID        string
	CreatedBy string
	CreatedAt time.Time
	ExpiresAt *time.Time
	Used      bool
	RevokedAt *time.Time
	// Note is creation-time metadata, visible only to invitation managers.
	Note string
}

type InvitationProjection struct {
	events.MemoryProjection
	invitations map[string]*InvitationState
}

func NewInvitationProjection() *InvitationProjection {
	return &InvitationProjection{invitations: make(map[string]*InvitationState)}
}

func (p *InvitationProjection) Subjects() []string {
	return []string{evtstream.InvitationSubjectFilter()}
}

func (p *InvitationProjection) Apply(event *evtv1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	switch e := event.GetEvent().(type) {
	case *evtv1.Event_InvitationCreated:
		payload := e.InvitationCreated
		state := &InvitationState{
			ID:        payload.GetInvitationId(),
			CreatedBy: event.GetActorId(),
			CreatedAt: event.GetCreatedAt().AsTime(),
			Note:      payload.GetNote(),
		}
		if payload.GetExpiresAt() != nil {
			value := payload.GetExpiresAt().AsTime()
			state.ExpiresAt = &value
		}
		p.invitations[state.ID] = state
	case *evtv1.Event_InvitationRedeemed:
		if state := p.invitations[e.InvitationRedeemed.GetInvitationId()]; state != nil {
			state.Used = true
		}
	case *evtv1.Event_InvitationRevoked:
		if state := p.invitations[e.InvitationRevoked.GetInvitationId()]; state != nil {
			value := event.GetCreatedAt().AsTime()
			state.RevokedAt = &value
		}
	}
	return nil
}

func (p *InvitationProjection) get(id string) (InvitationState, bool) {
	p.RLock()
	defer p.RUnlock()
	state, ok := p.invitations[id]
	if !ok {
		return InvitationState{}, false
	}
	return cloneInvitationState(state), true
}

func (p *InvitationProjection) all() []InvitationState {
	p.RLock()
	defer p.RUnlock()
	result := make([]InvitationState, 0, len(p.invitations))
	for _, state := range p.invitations {
		result = append(result, cloneInvitationState(state))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

func (p *InvitationProjection) ids() []string {
	p.RLock()
	defer p.RUnlock()
	result := make([]string, 0, len(p.invitations))
	for id := range p.invitations {
		result = append(result, id)
	}
	return result
}

func (p *InvitationProjection) count() int {
	p.RLock()
	defer p.RUnlock()
	return len(p.invitations)
}

func cloneInvitationState(state *InvitationState) InvitationState {
	clone := *state
	if state.ExpiresAt != nil {
		value := *state.ExpiresAt
		clone.ExpiresAt = &value
	}
	if state.RevokedAt != nil {
		value := *state.RevokedAt
		clone.RevokedAt = &value
	}
	return clone
}

func (p *InvitationProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var estimatedBytes int64
	for _, state := range p.invitations {
		estimatedBytes += int64(len(state.ID) + len(state.CreatedBy) + len(state.Note) + 32)
	}
	return int64(len(p.invitations)), estimatedBytes, nil
}
