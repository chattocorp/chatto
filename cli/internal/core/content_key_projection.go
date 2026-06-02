package core

import (
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ContentKeyProjection indexes per-user encrypted content key epochs.
type ContentKeyProjection struct {
	events.MemoryProjection
	byUserEpoch map[string]map[int32]*corev1.UserContentKeyGeneratedEvent
	activeEpoch map[string]int32
	eventIDSeen map[string]struct{}
}

func NewContentKeyProjection() *ContentKeyProjection {
	return &ContentKeyProjection{
		byUserEpoch: make(map[string]map[int32]*corev1.UserContentKeyGeneratedEvent),
		activeEpoch: make(map[string]int32),
		eventIDSeen: make(map[string]struct{}),
	}
}

func (p *ContentKeyProjection) Subjects() []string {
	return []string{events.UserSubjectFilter()}
}

func (p *ContentKeyProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	if id := event.GetId(); id != "" {
		if _, ok := p.eventIDSeen[id]; ok {
			return nil
		}
		p.eventIDSeen[id] = struct{}{}
	}

	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserContentKeyGenerated:
		p.applyContentKeyGeneratedLocked(e.UserContentKeyGenerated)
	case *corev1.Event_UserKeyShredded:
		userID := e.UserKeyShredded.GetUserId()
		if userID != "" {
			delete(p.byUserEpoch, userID)
			delete(p.activeEpoch, userID)
		}
	}
	return nil
}

func (p *ContentKeyProjection) applyContentKeyGeneratedLocked(e *corev1.UserContentKeyGeneratedEvent) {
	if e == nil || e.GetUserId() == "" || e.GetEpoch() <= 0 {
		return
	}
	epochs := p.byUserEpoch[e.GetUserId()]
	if epochs == nil {
		epochs = make(map[int32]*corev1.UserContentKeyGeneratedEvent)
		p.byUserEpoch[e.GetUserId()] = epochs
	}
	if _, exists := epochs[e.GetEpoch()]; !exists {
		epochs[e.GetEpoch()] = proto.Clone(e).(*corev1.UserContentKeyGeneratedEvent)
	}
	if e.GetEpoch() > p.activeEpoch[e.GetUserId()] {
		p.activeEpoch[e.GetUserId()] = e.GetEpoch()
	}
}

func (p *ContentKeyProjection) Active(userID string) (*corev1.UserContentKeyGeneratedEvent, bool) {
	p.RLock()
	defer p.RUnlock()
	epoch := p.activeEpoch[userID]
	if epoch <= 0 {
		return nil, false
	}
	return p.getLocked(userID, epoch)
}

func (p *ContentKeyProjection) Get(userID string, epoch int32) (*corev1.UserContentKeyGeneratedEvent, bool) {
	p.RLock()
	defer p.RUnlock()
	return p.getLocked(userID, epoch)
}

func (p *ContentKeyProjection) KeyRefs(userID string) []string {
	p.RLock()
	defer p.RUnlock()
	epochs := p.byUserEpoch[userID]
	if epochs == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var refs []string
	for _, event := range epochs {
		ref := event.GetWrappingKeyRef()
		if ref == "" {
			ref = kms.LegacyUserKeyRef(userID)
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
}

func (p *ContentKeyProjection) getLocked(userID string, epoch int32) (*corev1.UserContentKeyGeneratedEvent, bool) {
	epochs := p.byUserEpoch[userID]
	if epochs == nil {
		return nil, false
	}
	event := epochs[epoch]
	if event == nil {
		return nil, false
	}
	return proto.Clone(event).(*corev1.UserContentKeyGeneratedEvent), true
}
