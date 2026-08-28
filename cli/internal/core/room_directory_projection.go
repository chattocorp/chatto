package core

import (
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// RoomDirectoryProjection combines the room aggregate's structural read models:
// catalog metadata and membership indexes. Keeping them under one projector
// avoids duplicate evt.room.> consumers while preserving the smaller read-model
// APIs used by callers.
type RoomDirectoryProjection struct {
	events.MemoryProjection
	Catalog    *RoomCatalogProjection
	Membership *RoomMembershipProjection
	Bans       *RoomBanProjection
}

func NewRoomDirectoryProjection() *RoomDirectoryProjection {
	return &RoomDirectoryProjection{
		Catalog:    NewRoomCatalogProjection(),
		Membership: NewRoomMembershipProjection(),
		Bans:       NewRoomBanProjection(),
	}
}

func (p *RoomDirectoryProjection) Subjects() []string {
	return []string{evtstream.RoomSubjectFilter()}
}

func (p *RoomDirectoryProjection) Apply(event *evtv1.Event, seq uint64) error {
	if err := p.Catalog.Apply(event, seq); err != nil {
		return err
	}
	if err := p.Membership.Apply(event, seq); err != nil {
		return err
	}
	return p.Bans.Apply(event, seq)
}
