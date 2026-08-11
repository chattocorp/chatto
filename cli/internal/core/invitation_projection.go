package core

import (
	"sort"
	"time"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

// InvitationState is the durable, administrator-visible state derived for one invitation.
type InvitationState struct {
	ID        string
	CreatedBy string
	CreatedAt time.Time
	MaxUses   *uint32
	ExpiresAt *time.Time
	UseCount  uint32
	RevokedAt *time.Time
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

func (p *InvitationProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	switch e := event.GetEvent().(type) {
	case *corev1.Event_InvitationCreated:
		payload := e.InvitationCreated
		state := &InvitationState{
			ID:        payload.GetInvitationId(),
			CreatedBy: event.GetActorId(),
			CreatedAt: event.GetCreatedAt().AsTime(),
		}
		if payload.MaxUses != nil {
			value := payload.GetMaxUses()
			state.MaxUses = &value
		}
		if payload.GetExpiresAt() != nil {
			value := payload.GetExpiresAt().AsTime()
			state.ExpiresAt = &value
		}
		p.invitations[state.ID] = state
	case *corev1.Event_InvitationRedeemed:
		if state := p.invitations[e.InvitationRedeemed.GetInvitationId()]; state != nil {
			state.UseCount++
		}
	case *corev1.Event_InvitationRevoked:
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
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
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
	if state.MaxUses != nil {
		value := *state.MaxUses
		clone.MaxUses = &value
	}
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
		estimatedBytes += int64(len(state.ID) + len(state.CreatedBy) + 32)
	}
	return int64(len(p.invitations)), estimatedBytes, nil
}
