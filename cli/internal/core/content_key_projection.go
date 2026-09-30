package core

import (
	"bytes"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// ContentKeyProjection indexes per-user encrypted DEK epochs by purpose.
type ContentKeyProjection struct {
	events.MemoryProjection
	contentKeyState
}

// contentKeyState is the snapshot-restorable state of a ContentKeyProjection.
// Restore builds a new value and replaces the complete state at once.
type contentKeyState struct {
	// users interns the user IDs of stored keys.
	users projectionIDTable
	// keys holds each stored DEK epoch in compact form. Reads rebuild the
	// UserDEKGeneratedEvent.
	keys map[contentKeyID]contentKeyRecord
	// activeEpoch holds the newest epoch of each user and purpose.
	activeEpoch map[contentKeyPurposeID]int32
	// algorithms interns wrapping algorithm names, which few distinct values
	// repeat across all keys.
	algorithms    map[string]string
	shreddedUsers map[string]struct{}
	replayGuard   projectionReplayGuard
}

// contentKeyPurposeID identifies one user's keys for one purpose. user is a
// users handle.
type contentKeyPurposeID struct {
	user    uint32
	purpose evtv1.UserDEKPurpose
}

// contentKeyID identifies one DEK epoch.
type contentKeyID struct {
	contentKeyPurposeID
	epoch int32
}

// contentKeyRecord holds the stored fields of one UserDEKGeneratedEvent other
// than its identity.
type contentKeyRecord struct {
	contentKeyRef     string
	wrappingKeyRef    string
	wrappingAlgorithm string
	wrappingMetadata  []byte
}

func NewContentKeyProjection() *ContentKeyProjection {
	return &ContentKeyProjection{contentKeyState: contentKeyState{
		users:         newProjectionIDTable(),
		keys:          make(map[contentKeyID]contentKeyRecord),
		activeEpoch:   make(map[contentKeyPurposeID]int32),
		algorithms:    make(map[string]string),
		shreddedUsers: make(map[string]struct{}),
		replayGuard:   newProjectionReplayGuard(),
	}}
}

func (p *ContentKeyProjection) Subjects() []string {
	return []string{
		evtstream.UserEventTypeFilter(evtstream.EventUserDEKGenerated),
		evtstream.UserEventTypeFilter(evtstream.EventUserKeyShreddingRequested),
		evtstream.UserEventTypeFilter(evtstream.EventUserKeyShredded),
	}
}

func (p *ContentKeyProjection) Apply(event *evtv1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	if p.replayGuard.seenOrMark(event, seq) {
		return nil
	}

	switch e := event.GetEvent().(type) {
	case *evtv1.Event_UserDekGenerated:
		p.applyDEKGeneratedLocked(e.UserDekGenerated)
	case *evtv1.Event_UserKeyShreddingRequested:
		p.clearUserLocked(e.UserKeyShreddingRequested.GetUserId())
	case *evtv1.Event_UserKeyShredded:
		p.clearUserLocked(e.UserKeyShredded.GetUserId())
	}
	return nil
}

func (p *ContentKeyProjection) clearUserLocked(userID string) {
	if userID == "" {
		return
	}
	if user, known := p.users.lookup(userID); known {
		for id := range p.keys {
			if id.user == user {
				delete(p.keys, id)
			}
		}
		for id := range p.activeEpoch {
			if id.user == user {
				delete(p.activeEpoch, id)
			}
		}
	}
	p.shreddedUsers[userID] = struct{}{}
}

func (p *ContentKeyProjection) CompleteStartupReplay() {
	p.Lock()
	defer p.Unlock()
	p.replayGuard.completeReplay()
}

func (p *ContentKeyProjection) applyDEKGeneratedLocked(e *evtv1.UserDEKGeneratedEvent) {
	if e == nil || e.GetUserId() == "" || e.GetEpoch() <= 0 || e.GetContentKeyRef() == "" {
		return
	}
	if _, shredded := p.shreddedUsers[e.GetUserId()]; shredded {
		return
	}
	purpose := contentKeyPurposeID{user: p.users.intern(e.GetUserId()), purpose: e.GetPurpose()}
	id := contentKeyID{contentKeyPurposeID: purpose, epoch: e.GetEpoch()}
	if _, exists := p.keys[id]; !exists {
		algorithm, known := p.algorithms[e.GetWrappingAlgorithm()]
		if !known {
			algorithm = e.GetWrappingAlgorithm()
			p.algorithms[algorithm] = algorithm
		}
		p.keys[id] = contentKeyRecord{
			contentKeyRef:     e.GetContentKeyRef(),
			wrappingKeyRef:    e.GetWrappingKeyRef(),
			wrappingAlgorithm: algorithm,
			wrappingMetadata:  bytes.Clone(e.GetWrappingMetadata()),
		}
	}
	if e.GetEpoch() > p.activeEpoch[purpose] {
		p.activeEpoch[purpose] = e.GetEpoch()
	}
}

func (p *ContentKeyProjection) Active(userID string, purpose evtv1.UserDEKPurpose) (*evtv1.UserDEKGeneratedEvent, bool) {
	p.RLock()
	defer p.RUnlock()
	user, known := p.users.lookup(userID)
	if !known {
		return nil, false
	}
	if epoch := p.activeEpoch[contentKeyPurposeID{user: user, purpose: purpose}]; epoch > 0 {
		return p.getLocked(userID, contentKeyID{contentKeyPurposeID: contentKeyPurposeID{user: user, purpose: purpose}, epoch: epoch})
	}
	if purpose == evtv1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED {
		return nil, false
	}
	legacy := contentKeyPurposeID{user: user, purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED}
	epoch := p.activeEpoch[legacy]
	if epoch <= 0 {
		return nil, false
	}
	return p.getLocked(userID, contentKeyID{contentKeyPurposeID: legacy, epoch: epoch})
}

func (p *ContentKeyProjection) Get(userID string, purpose evtv1.UserDEKPurpose, epoch int32) (*evtv1.UserDEKGeneratedEvent, bool) {
	p.RLock()
	defer p.RUnlock()
	user, known := p.users.lookup(userID)
	if !known {
		return nil, false
	}
	if event, ok := p.getLocked(userID, contentKeyID{contentKeyPurposeID: contentKeyPurposeID{user: user, purpose: purpose}, epoch: epoch}); ok {
		return event, true
	}
	if purpose == evtv1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED {
		return nil, false
	}
	return p.getLocked(userID, contentKeyID{contentKeyPurposeID: contentKeyPurposeID{user: user, purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED}, epoch: epoch})
}

// getLocked rebuilds a detached event for a stored key.
func (p *ContentKeyProjection) getLocked(userID string, id contentKeyID) (*evtv1.UserDEKGeneratedEvent, bool) {
	record, ok := p.keys[id]
	if !ok {
		return nil, false
	}
	return record.event(userID, id), true
}

func (r contentKeyRecord) event(userID string, id contentKeyID) *evtv1.UserDEKGeneratedEvent {
	return &evtv1.UserDEKGeneratedEvent{
		UserId:            userID,
		Purpose:           id.purpose,
		Epoch:             id.epoch,
		ContentKeyRef:     r.contentKeyRef,
		WrappingKeyRef:    r.wrappingKeyRef,
		WrappingAlgorithm: r.wrappingAlgorithm,
		WrappingMetadata:  bytes.Clone(r.wrappingMetadata),
	}
}
