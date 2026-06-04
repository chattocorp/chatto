package core

import (
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomGroupLayoutProjection combines room-group state and explicit sidebar
// ordering. The read APIs remain split between RoomGroups and RoomLayout, but a
// single projector now tracks the group aggregate plus the layout aggregate.
type RoomGroupLayoutProjection struct {
	events.MemoryProjection
	Groups *RoomGroupProjection
	Layout *RoomLayoutProjection
}

func NewRoomGroupLayoutProjection() *RoomGroupLayoutProjection {
	return &RoomGroupLayoutProjection{
		Groups: NewRoomGroupProjection(),
		Layout: NewRoomLayoutProjection(),
	}
}

func (p *RoomGroupLayoutProjection) Subjects() []string {
	return []string{events.GroupSubjectFilter(), events.LayoutSubjectFilter()}
}

func (p *RoomGroupLayoutProjection) Apply(event *corev1.Event, seq uint64) error {
	if err := p.Groups.Apply(event, seq); err != nil {
		return err
	}
	return p.Layout.Apply(event, seq)
}
