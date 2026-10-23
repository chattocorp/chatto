package core

import (
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
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

func (p *RoomGroupLayoutProjection) Apply(event *evtv1.Event, seq uint64) error {
	if event != nil {
		if _, ok := event.GetEvent().(*evtv1.Event_RoomGroupsReordered); ok {
			return p.Layout.Apply(event, seq)
		}
	}
	if err := p.Groups.Apply(event, seq); err != nil {
		return err
	}
	return p.Layout.Apply(event, seq)
}
