package connectapi

import (
	"context"
	"errors"

	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/readmodel"
)

func apiAdminRoomGroup(group *readmodel.RoomGroup) *adminv1.AdminRoomGroup {
	return &adminv1.AdminRoomGroup{Id: group.GetId(), Name: group.GetName(), Description: group.GetDescription()}
}

func apiAdminRoomGroupLayout(group *readmodel.RoomGroup, rooms map[string]*readmodel.Room) *adminv1.AdminRoomGroupLayout {
	return &adminv1.AdminRoomGroupLayout{Group: apiAdminRoomGroup(group), Items: apiAdminRoomLayoutGroup(group, rooms).GetItems()}
}

// assembleAdminRoomLayout hydrates room metadata once for a server-wide layout.
func (s *adminRoomLayoutService) assembleAdminRoomLayout(ctx context.Context, groups []*readmodel.RoomGroup) (*adminv1.AdminRoomLayout, error) {
	rooms, err := s.channelRoomsByID(ctx)
	if err != nil {
		return nil, err
	}
	layout := &adminv1.AdminRoomLayout{Groups: make([]*adminv1.AdminRoomGroupLayout, 0, len(groups))}
	for _, group := range groups {
		layout.Groups = append(layout.Groups, apiAdminRoomGroupLayout(group, rooms))
	}
	return layout, nil
}

// assembleAdminRoomGroupLayout reads only this group's rooms. Deleted entries
// are omitted consistently with the existing complete-layout snapshots.
func (s *adminRoomLayoutService) assembleAdminRoomGroupLayout(ctx context.Context, group *readmodel.RoomGroup) (*adminv1.AdminRoomGroupLayout, error) {
	rooms := make(map[string]*readmodel.Room)
	for _, entry := range group.GetEntries() {
		if entry.GetKind() != evtv1.SidebarGroupEntry_ROOM {
			continue
		}
		room, err := s.api.core.GetRoom(ctx, core.KindChannel, entry.GetId())
		if errors.Is(err, core.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		rooms[room.GetId()] = room
	}
	return apiAdminRoomGroupLayout(group, rooms), nil
}
