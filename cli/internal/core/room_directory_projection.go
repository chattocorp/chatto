package core

import (
	"fmt"

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

// Prepare validates the fallible room-directory event shapes before the
// content view commits any component for the event.
func (p *RoomDirectoryProjection) Prepare(event *evtv1.Event, seq uint64) (events.PreparedMutation, error) {
	if err := validateRoomDirectoryEvent(event); err != nil {
		return nil, err
	}
	return events.PreparedMutationFunc(func() {
		if err := p.Apply(event, seq); err != nil {
			panic(fmt.Sprintf("core: validated room directory event failed to apply: %v", err))
		}
	}), nil
}

func validateRoomDirectoryEvent(event *evtv1.Event) error {
	if event == nil {
		return nil
	}
	switch value := event.GetEvent().(type) {
	case *evtv1.Event_UserJoinedRoom:
		if value.UserJoinedRoom.GetRoomId() == "" || event.GetActorId() == "" {
			return fmt.Errorf("UserJoinedRoom missing roomID or userID")
		}
	case *evtv1.Event_UserLeftRoom:
		if value.UserLeftRoom.GetRoomId() == "" || event.GetActorId() == "" {
			return fmt.Errorf("UserLeftRoom missing roomID or userID")
		}
	case *evtv1.Event_RoomMemberBanned:
		banned := value.RoomMemberBanned
		if banned.GetRoomId() == "" || banned.GetUserId() == "" || banned.GetReason() == "" {
			return fmt.Errorf("RoomMemberBanned missing roomID, userID, or reason")
		}
	case *evtv1.Event_RoomDeleted:
		if value.RoomDeleted.GetRoomId() == "" {
			return fmt.Errorf("RoomDeleted missing roomID")
		}
	}
	return nil
}
