package core

import (
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

// RoomGroupLayoutProjection combines room-group state and explicit sidebar
// ordering. RoomModel owns both read surfaces and their shared projector.
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
	return []string{evtstream.GroupSubjectFilter(), evtstream.LayoutSubjectFilter()}
}

func (p *RoomGroupLayoutProjection) Apply(event *corev1.Event, seq uint64) error {
	if event != nil {
		if _, ok := event.GetEvent().(*corev1.Event_RoomGroupsReordered); ok {
			return p.Layout.Apply(event, seq)
		}
	}
	if err := p.Groups.Apply(event, seq); err != nil {
		return err
	}
	return p.Layout.Apply(event, seq)
}
