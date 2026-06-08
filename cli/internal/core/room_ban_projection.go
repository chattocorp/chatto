package core

import (
	"fmt"
	"sort"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type RoomBan struct {
	EventID     string
	RoomID      string
	UserID      string
	ModeratorID string
	Reason      string
	CreatedAt   time.Time
	ExpiresAt   *time.Time
}

func (b RoomBan) Active(now time.Time) bool {
	return b.ExpiresAt == nil || b.ExpiresAt.After(now)
}

type RoomBanProjection struct {
	events.MemoryProjection
	byRoom map[string]map[string]RoomBan
}

func NewRoomBanProjection() *RoomBanProjection {
	return &RoomBanProjection{
		byRoom: make(map[string]map[string]RoomBan),
	}
}

func (p *RoomBanProjection) Subjects() []string {
	return []string{events.RoomSubjectFilter()}
}

func (p *RoomBanProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	switch e := event.GetEvent().(type) {
	case *corev1.Event_RoomMemberBanned:
		banned := e.RoomMemberBanned
		roomID := banned.GetRoomId()
		userID := banned.GetUserId()
		reason := banned.GetReason()
		if roomID == "" || userID == "" || reason == "" {
			return fmt.Errorf("RoomMemberBanned missing roomID, userID, or reason")
		}
		createdAt := time.Time{}
		if ts := event.GetCreatedAt(); ts != nil {
			createdAt = ts.AsTime()
		}
		var expiresAt *time.Time
		if ts := banned.GetExpiresAt(); ts != nil {
			t := ts.AsTime()
			expiresAt = &t
		}
		users := p.byRoom[roomID]
		if users == nil {
			users = make(map[string]RoomBan)
			p.byRoom[roomID] = users
		}
		users[userID] = RoomBan{
			EventID:     event.GetId(),
			RoomID:      roomID,
			UserID:      userID,
			ModeratorID: event.GetActorId(),
			Reason:      reason,
			CreatedAt:   createdAt,
			ExpiresAt:   expiresAt,
		}
	case *corev1.Event_RoomMemberUnbanned:
		p.removeLocked(e.RoomMemberUnbanned.GetRoomId(), e.RoomMemberUnbanned.GetUserId())
	case *corev1.Event_RoomDeleted:
		delete(p.byRoom, e.RoomDeleted.GetRoomId())
	default:
	}
	return nil
}

func (p *RoomBanProjection) removeLocked(roomID, userID string) {
	users := p.byRoom[roomID]
	if users == nil {
		return
	}
	delete(users, userID)
	if len(users) == 0 {
		delete(p.byRoom, roomID)
	}
}

func (p *RoomBanProjection) IsActive(roomID, userID string, now time.Time) bool {
	p.RLock()
	defer p.RUnlock()
	ban, ok := p.byRoom[roomID][userID]
	return ok && ban.Active(now)
}

func (p *RoomBanProjection) ActiveBan(roomID, userID string, now time.Time) (RoomBan, bool) {
	p.RLock()
	defer p.RUnlock()
	ban, ok := p.byRoom[roomID][userID]
	if !ok || !ban.Active(now) {
		return RoomBan{}, false
	}
	return ban, true
}

func (p *RoomBanProjection) ActiveBans(now time.Time) []RoomBan {
	p.RLock()
	defer p.RUnlock()
	out := []RoomBan{}
	for _, users := range p.byRoom {
		for _, ban := range users {
			if ban.Active(now) {
				out = append(out, ban)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (p *RoomBanProjection) ActiveRoomBans(roomID string, now time.Time) []RoomBan {
	p.RLock()
	defer p.RUnlock()
	out := []RoomBan{}
	for _, ban := range p.byRoom[roomID] {
		if ban.Active(now) {
			out = append(out, ban)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}
