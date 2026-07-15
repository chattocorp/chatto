package core

import (
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ContentKeyProjection indexes per-user encrypted DEK epochs by purpose.
type ContentKeyProjection struct {
	events.MemoryProjection
	byUserPurposeEpoch map[string]map[corev1.UserDEKPurpose]map[int32]*corev1.UserDEKGeneratedEvent
	activeEpoch        map[string]map[corev1.UserDEKPurpose]int32
	replayGuard        projectionReplayGuard
}

func NewContentKeyProjection() *ContentKeyProjection {
	return &ContentKeyProjection{
		byUserPurposeEpoch: make(map[string]map[corev1.UserDEKPurpose]map[int32]*corev1.UserDEKGeneratedEvent),
		activeEpoch:        make(map[string]map[corev1.UserDEKPurpose]int32),
		replayGuard:        newProjectionReplayGuard(),
	}
}

func (p *ContentKeyProjection) Subjects() []string {
	return []string{
		events.UserEventTypeFilter(events.EventUserDEKGenerated),
		events.UserEventTypeFilter(events.EventUserKeyShredded),
	}
}

func (p *ContentKeyProjection) Apply(event *corev1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	if p.replayGuard.seenOrMark(event, seq) {
		return nil
	}

	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserDekGenerated:
		p.applyDEKGeneratedLocked(e.UserDekGenerated)
	case *corev1.Event_UserKeyShredded:
		userID := e.UserKeyShredded.GetUserId()
		if userID != "" {
			delete(p.byUserPurposeEpoch, userID)
			delete(p.activeEpoch, userID)
		}
	}
	return nil
}

func (p *ContentKeyProjection) CompleteStartupReplay() {
	p.Lock()
	defer p.Unlock()
	p.replayGuard.completeReplay()
}

func (p *ContentKeyProjection) applyDEKGeneratedLocked(e *corev1.UserDEKGeneratedEvent) {
	if e == nil || e.GetUserId() == "" || e.GetEpoch() <= 0 || e.GetContentKeyRef() == "" {
		return
	}
	purpose := e.GetPurpose()
	byPurpose := p.byUserPurposeEpoch[e.GetUserId()]
	if byPurpose == nil {
		byPurpose = make(map[corev1.UserDEKPurpose]map[int32]*corev1.UserDEKGeneratedEvent)
		p.byUserPurposeEpoch[e.GetUserId()] = byPurpose
	}
	epochs := byPurpose[purpose]
	if epochs == nil {
		epochs = make(map[int32]*corev1.UserDEKGeneratedEvent)
		byPurpose[purpose] = epochs
	}
	if _, exists := epochs[e.GetEpoch()]; !exists {
		epochs[e.GetEpoch()] = proto.Clone(e).(*corev1.UserDEKGeneratedEvent)
	}
	activeByPurpose := p.activeEpoch[e.GetUserId()]
	if activeByPurpose == nil {
		activeByPurpose = make(map[corev1.UserDEKPurpose]int32)
		p.activeEpoch[e.GetUserId()] = activeByPurpose
	}
	if e.GetEpoch() > activeByPurpose[purpose] {
		activeByPurpose[purpose] = e.GetEpoch()
	}
}

func (p *ContentKeyProjection) Active(userID string, purpose corev1.UserDEKPurpose) (*corev1.UserDEKGeneratedEvent, bool) {
	p.RLock()
	defer p.RUnlock()
	epoch := p.activeEpoch[userID][purpose]
	if epoch > 0 {
		return p.getLocked(userID, purpose, epoch)
	}
	if purpose == corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED {
		return nil, false
	}
	epoch = p.activeEpoch[userID][corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED]
	if epoch <= 0 {
		return nil, false
	}
	return p.getLocked(userID, corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED, epoch)
}

func (p *ContentKeyProjection) Get(userID string, purpose corev1.UserDEKPurpose, epoch int32) (*corev1.UserDEKGeneratedEvent, bool) {
	p.RLock()
	defer p.RUnlock()
	if event, ok := p.getLocked(userID, purpose, epoch); ok {
		return event, true
	}
	if purpose == corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED {
		return nil, false
	}
	return p.getLocked(userID, corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED, epoch)
}

func (p *ContentKeyProjection) KeyRefs(userID string) []string {
	p.RLock()
	defer p.RUnlock()
	byPurpose := p.byUserPurposeEpoch[userID]
	if byPurpose == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var refs []string
	for _, epochs := range byPurpose {
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
	}
	return refs
}

func (p *ContentKeyProjection) ContentKeyRefs(userID string) []string {
	p.RLock()
	defer p.RUnlock()
	byPurpose := p.byUserPurposeEpoch[userID]
	if byPurpose == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var refs []string
	for _, epochs := range byPurpose {
		for _, event := range epochs {
			ref := event.GetContentKeyRef()
			if ref == "" {
				continue
			}
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			refs = append(refs, ref)
		}
	}
	return refs
}

func (p *ContentKeyProjection) getLocked(userID string, purpose corev1.UserDEKPurpose, epoch int32) (*corev1.UserDEKGeneratedEvent, bool) {
	byPurpose := p.byUserPurposeEpoch[userID]
	if byPurpose == nil {
		return nil, false
	}
	epochs := byPurpose[purpose]
	if epochs == nil {
		return nil, false
	}
	event := epochs[epoch]
	if event == nil {
		return nil, false
	}
	return proto.Clone(event).(*corev1.UserDEKGeneratedEvent), true
}
