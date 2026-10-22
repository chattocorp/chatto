package core

import (
	"sort"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// RoomInvitationState is derived only from EVT. Notification retention cannot change it.
type RoomInvitationState struct {
	ID, RoomID, SenderID, RecipientID string
	CreatedAt, ExpiresAt              time.Time
	// Status is pending, accepted, or declined; expiry is derived from ExpiresAt.
	Status string
}

func (s RoomInvitationState) pending(now time.Time) bool {
	return s.Status == "pending" && now.Before(s.ExpiresAt)
}

// RoomInvitationProjection cold-replays invitation facts independently of room snapshots.
type RoomInvitationProjection struct {
	events.MemoryProjection
	byID              map[string]RoomInvitationState
	pendingByRoomUser map[[2]string]string
}

// NewRoomInvitationProjection creates an empty projection for cold replay.
func NewRoomInvitationProjection() *RoomInvitationProjection {
	return &RoomInvitationProjection{byID: make(map[string]RoomInvitationState), pendingByRoomUser: make(map[[2]string]string)}
}
func (p *RoomInvitationProjection) Subjects() []string {
	return []string{evtstream.RoomSubjectFilter()}
}
func (p *RoomInvitationProjection) Apply(event *evtv1.Event, _ uint64) error {
	p.Lock()
	defer p.Unlock()
	switch e := event.GetEvent().(type) {
	case *evtv1.Event_RoomInvitationCreated:
		v := e.RoomInvitationCreated
		p.pendingByRoomUser[[2]string{v.RoomId, v.RecipientId}] = v.InvitationId
		p.byID[v.InvitationId] = RoomInvitationState{ID: v.InvitationId, RoomID: v.RoomId, SenderID: event.ActorId, RecipientID: v.RecipientId, CreatedAt: event.CreatedAt.AsTime(), ExpiresAt: v.ExpiresAt.AsTime(), Status: "pending"}
	case *evtv1.Event_RoomInvitationAccepted:
		v, ok := p.byID[e.RoomInvitationAccepted.InvitationId]
		if ok {
			v.Status = "accepted"
			p.byID[v.ID] = v
		}
	case *evtv1.Event_RoomInvitationDeclined:
		v, ok := p.byID[e.RoomInvitationDeclined.InvitationId]
		if ok {
			v.Status = "declined"
			p.byID[v.ID] = v
		}
	case *evtv1.Event_RoomDeleted:
		for id, v := range p.byID {
			if v.RoomID == e.RoomDeleted.RoomId {
				delete(p.byID, id)
				delete(p.pendingByRoomUser, [2]string{v.RoomID, v.RecipientID})
			}
		}
	}
	return nil
}
func (p *RoomInvitationProjection) get(id string) (RoomInvitationState, bool) {
	p.RLock()
	defer p.RUnlock()
	v, ok := p.byID[id]
	return v, ok
}
func (p *RoomInvitationProjection) all() []RoomInvitationState {
	p.RLock()
	defer p.RUnlock()
	result := make([]RoomInvitationState, 0, len(p.byID))
	for _, v := range p.byID {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}
func (p *RoomInvitationProjection) pending(roomID, userID string, now time.Time) (RoomInvitationState, bool) {
	p.RLock()
	defer p.RUnlock()
	id := p.pendingByRoomUser[[2]string{roomID, userID}]
	v, ok := p.byID[id]
	return v, ok && v.pending(now)
}
func (p *RoomInvitationProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	return int64(len(p.byID)), int64(len(p.byID) * 160), nil
}

// roomInvitationParticipants identifies private invitation delivery without room membership.
func roomInvitationParticipants(event *evtv1.Event) (sender, recipient string, ok bool) {
	switch e := event.GetEvent().(type) {
	case *evtv1.Event_RoomInvitationCreated:
		return event.ActorId, e.RoomInvitationCreated.RecipientId, true
	case *evtv1.Event_RoomInvitationAccepted:
		return e.RoomInvitationAccepted.SenderId, e.RoomInvitationAccepted.RecipientId, true
	case *evtv1.Event_RoomInvitationDeclined:
		return e.RoomInvitationDeclined.SenderId, e.RoomInvitationDeclined.RecipientId, true
	}
	return "", "", false
}
