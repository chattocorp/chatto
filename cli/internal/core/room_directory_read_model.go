package core

import (
	"context"
	"errors"
	"fmt"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomDirectoryReads returns the operation-level model for public room
// directory/sidebar reads and directory-adjacent join commands.
func (c *ChattoCore) RoomDirectoryReads() *RoomDirectoryReadModel {
	return c.roomDirectoryReads
}

type RoomDirectoryReadModel struct {
	core *ChattoCore
}

type RoomDirectoryListOptions struct {
	IncludeChannels bool
	IncludeDMs      bool
	// IncludeEmptyDMs is for exhaustive projection/authorization reads. Public
	// directory lists keep the conversation-list policy of hiding DMs until
	// they contain a message.
	IncludeEmptyDMs bool
}

type RoomDirectoryGroupOptions struct {
	IncludeArchivedRooms bool
}

type DirectoryRoom struct {
	Room        *corev1.Room
	ViewerState DirectoryRoomViewerState
}

type DirectoryRoomViewerState struct {
	IsMember               bool
	HasUnread              bool
	CanListRoom            bool
	CanJoinRoom            bool
	CanPostMessage         bool
	CanPostInThread        bool
	CanAttach              bool
	CanReact               bool
	CanEchoMessage         bool
	CanManageOthersMessage bool
	CanManageRoom          bool
	CanBanRoomMembers      bool
}

type DirectoryRoomGroup struct {
	Group       *corev1.RoomGroup
	ViewerState DirectoryRoomGroupViewerState
	Rooms       []*DirectoryRoom
	Items       []DirectoryRoomGroupItem
}

type DirectoryRoomGroupViewerState struct {
	CanCreateRoom bool
}

type DirectoryRoomGroupItem struct {
	Room        *DirectoryRoom
	SidebarLink *corev1.SidebarLink
}

func (s *RoomDirectoryReadModel) ListRooms(ctx context.Context, actorID string, opts RoomDirectoryListOptions) ([]*DirectoryRoom, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}

	rooms := []*DirectoryRoom{}
	if opts.IncludeChannels {
		channelRooms, err := s.visibleChannelRooms(ctx, actorID)
		if err != nil {
			return nil, err
		}
		rooms = append(rooms, channelRooms...)
	}
	if opts.IncludeDMs {
		dmRooms, err := s.visibleDMRooms(ctx, actorID, opts.IncludeEmptyDMs)
		if err != nil {
			return nil, err
		}
		rooms = append(rooms, dmRooms...)
	}
	return rooms, nil
}

func (s *RoomDirectoryReadModel) ListRoomGroups(ctx context.Context, actorID string, opts RoomDirectoryGroupOptions) ([]*DirectoryRoomGroup, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}

	visibleRooms, err := s.visibleChannelRoomMap(ctx, actorID, opts.IncludeArchivedRooms)
	if err != nil {
		return nil, err
	}
	groups, err := s.core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		return nil, err
	}

	result := make([]*DirectoryRoomGroup, 0, len(groups))
	for _, group := range groups {
		dirGroup, err := s.directoryGroup(ctx, actorID, group, visibleRooms)
		if err != nil {
			return nil, err
		}
		result = append(result, dirGroup)
	}
	return result, nil
}

func (s *RoomDirectoryReadModel) GetRoomGroup(ctx context.Context, actorID, groupID string, opts RoomDirectoryGroupOptions) (*DirectoryRoomGroup, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	visibleRooms, err := s.visibleChannelRoomMap(ctx, actorID, opts.IncludeArchivedRooms)
	if err != nil {
		return nil, err
	}
	group, err := s.core.GetRoomGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return s.directoryGroup(ctx, actorID, group, visibleRooms)
}

func (s *RoomDirectoryReadModel) BatchGetRoomGroups(ctx context.Context, actorID string, groupIDs []string, opts RoomDirectoryGroupOptions) ([]*DirectoryRoomGroup, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	visibleRooms, err := s.visibleChannelRoomMap(ctx, actorID, opts.IncludeArchivedRooms)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(groupIDs))
	groups := make([]*DirectoryRoomGroup, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		if _, ok := seen[groupID]; ok {
			continue
		}
		seen[groupID] = struct{}{}

		group, err := s.core.GetRoomGroup(ctx, groupID)
		if err != nil {
			if errors.Is(err, ErrRoomGroupNotFound) {
				continue
			}
			return nil, err
		}
		dirGroup, err := s.directoryGroup(ctx, actorID, group, visibleRooms)
		if err != nil {
			return nil, err
		}
		groups = append(groups, dirGroup)
	}
	return groups, nil
}

func (s *RoomDirectoryReadModel) GetRoom(ctx context.Context, actorID, roomID string) (*DirectoryRoom, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	return s.getRoom(ctx, actorID, roomID)
}

func (s *RoomDirectoryReadModel) getRoom(ctx context.Context, actorID, roomID string) (*DirectoryRoom, error) {
	room, err := s.core.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	kind := KindOfRoom(room)
	visible, err := s.canSeeRoom(ctx, actorID, kind, room.Id)
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, ErrPermissionDenied
	}
	return s.directoryRoom(ctx, actorID, room)
}

func (s *RoomDirectoryReadModel) BatchGetRooms(ctx context.Context, actorID string, roomIDs []string) ([]*DirectoryRoom, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(roomIDs))
	rooms := make([]*DirectoryRoom, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		if _, ok := seen[roomID]; ok {
			continue
		}
		seen[roomID] = struct{}{}

		room, err := s.getRoom(ctx, actorID, roomID)
		if err != nil {
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrPermissionDenied) {
				continue
			}
			return nil, err
		}
		rooms = append(rooms, room)
	}
	return rooms, nil
}

func (s *RoomDirectoryReadModel) JoinGroup(ctx context.Context, actorID, groupID string) ([]string, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	group, err := s.core.GetRoomGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}

	joined := make([]string, 0, len(group.GetRoomIds()))
	for _, roomID := range group.GetRoomIds() {
		room, err := s.core.GetRoom(ctx, KindChannel, roomID)
		if err != nil {
			return nil, err
		}
		if room.GetArchived() {
			continue
		}
		alreadyMember, err := s.core.RoomMembershipExists(ctx, KindChannel, actorID, roomID)
		if err != nil {
			return nil, err
		}
		if alreadyMember {
			continue
		}
		canJoin, err := s.core.CanJoinRoomAt(ctx, actorID, KindChannel, roomID)
		if err != nil {
			return nil, err
		}
		if !canJoin {
			continue
		}
		if _, err := s.core.JoinRoom(ctx, actorID, KindChannel, actorID, roomID); err != nil {
			return nil, fmt.Errorf("join %s: %w", roomID, err)
		}
		joined = append(joined, roomID)
	}
	return joined, nil
}

func (s *RoomDirectoryReadModel) visibleChannelRooms(ctx context.Context, actorID string) ([]*DirectoryRoom, error) {
	rooms, err := s.core.ListRooms(ctx, KindChannel)
	if err != nil {
		return nil, err
	}
	result := make([]*DirectoryRoom, 0, len(rooms))
	for _, room := range rooms {
		if room.GetArchived() {
			continue
		}
		visible, err := s.core.CanSeeRoom(ctx, actorID, KindChannel, room.Id)
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		dirRoom, err := s.directoryRoom(ctx, actorID, room)
		if err != nil {
			return nil, err
		}
		result = append(result, dirRoom)
	}
	return result, nil
}

func (s *RoomDirectoryReadModel) visibleDMRooms(ctx context.Context, actorID string, includeEmpty bool) ([]*DirectoryRoom, error) {
	rooms, err := s.core.ListMemberRooms(ctx, KindDM, actorID, MemberRoomListOptions{
		RequireLastMessage:    !includeEmpty,
		SortByLastMessageDesc: true,
	})
	if err != nil {
		return nil, err
	}
	result := make([]*DirectoryRoom, 0, len(rooms))
	for _, room := range rooms {
		dirRoom, err := s.directoryRoom(ctx, actorID, room)
		if err != nil {
			return nil, err
		}
		result = append(result, dirRoom)
	}
	return result, nil
}

func (s *RoomDirectoryReadModel) visibleChannelRoomMap(ctx context.Context, actorID string, includeArchived bool) (map[string]*corev1.Room, error) {
	rooms, err := s.core.ListRooms(ctx, KindChannel)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*corev1.Room, len(rooms))
	for _, room := range rooms {
		if room.GetArchived() && !includeArchived {
			continue
		}
		visible, err := s.core.CanSeeRoom(ctx, actorID, KindChannel, room.Id)
		if err != nil {
			return nil, err
		}
		if visible {
			result[room.Id] = room
		}
	}
	return result, nil
}

func (s *RoomDirectoryReadModel) directoryGroup(ctx context.Context, actorID string, group *corev1.RoomGroup, visibleRooms map[string]*corev1.Room) (*DirectoryRoomGroup, error) {
	state, err := s.roomGroupViewerState(ctx, actorID, group.GetId())
	if err != nil {
		return nil, err
	}
	dirGroup := &DirectoryRoomGroup{Group: group, ViewerState: state}
	for _, roomID := range group.GetRoomIds() {
		room := visibleRooms[roomID]
		if room == nil {
			continue
		}
		dirRoom, err := s.directoryRoom(ctx, actorID, room)
		if err != nil {
			return nil, err
		}
		dirGroup.Rooms = append(dirGroup.Rooms, dirRoom)
	}

	sidebarLinks := make(map[string]*corev1.SidebarLink, len(group.GetSidebarLinks()))
	for _, link := range group.GetSidebarLinks() {
		sidebarLinks[link.GetId()] = link
	}
	for _, entry := range group.GetEntries() {
		switch entry.GetKind() {
		case corev1.SidebarGroupEntry_ROOM:
			room := visibleRooms[entry.GetId()]
			if room == nil {
				continue
			}
			dirRoom, err := s.directoryRoom(ctx, actorID, room)
			if err != nil {
				return nil, err
			}
			dirGroup.Items = append(dirGroup.Items, DirectoryRoomGroupItem{Room: dirRoom})
		case corev1.SidebarGroupEntry_SIDEBAR_LINK:
			link := sidebarLinks[entry.GetId()]
			if link == nil {
				continue
			}
			dirGroup.Items = append(dirGroup.Items, DirectoryRoomGroupItem{SidebarLink: link})
		}
	}
	return dirGroup, nil
}

func (s *RoomDirectoryReadModel) roomGroupViewerState(ctx context.Context, actorID, groupID string) (DirectoryRoomGroupViewerState, error) {
	canCreateRoom, err := s.core.CanCreateRoom(ctx, actorID, KindChannel, groupID)
	if err != nil {
		return DirectoryRoomGroupViewerState{}, err
	}
	return DirectoryRoomGroupViewerState{CanCreateRoom: canCreateRoom}, nil
}

func (s *RoomDirectoryReadModel) directoryRoom(ctx context.Context, actorID string, room *corev1.Room) (*DirectoryRoom, error) {
	state, err := s.roomViewerState(ctx, actorID, room)
	if err != nil {
		return nil, err
	}
	return &DirectoryRoom{Room: room, ViewerState: state}, nil
}

func (s *RoomDirectoryReadModel) roomViewerState(ctx context.Context, actorID string, room *corev1.Room) (DirectoryRoomViewerState, error) {
	kind := KindOfRoom(room)
	isMember, err := s.core.RoomMembershipExists(ctx, kind, actorID, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	hasUnread := false
	if isMember {
		hasUnread, err = s.core.HasUnread(ctx, kind, actorID, room.Id)
		if err != nil {
			return DirectoryRoomViewerState{}, err
		}
	}
	canList, err := s.canSeeRoom(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canJoin, err := s.core.CanJoinRoomAt(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canPostMessage, err := s.core.CanPostMessage(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canPostInThread, err := s.core.CanPostInThread(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canAttach, err := s.core.CanAttachFiles(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canReact, err := s.core.CanReactToMessage(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canEcho, err := s.core.CanEchoMessage(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canManageOthersMessage, err := s.core.CanManageOthersMessage(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canManageRoom, err := s.core.PermResolver().HasRoomPermission(ctx, actorID, kind, room.Id, PermRoomManage)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canBanRoomMembers, err := s.core.PermResolver().HasRoomPermission(ctx, actorID, kind, room.Id, PermRoomMemberBan)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}

	messageActionsEnabled := isMember && !room.GetArchived()
	memberActionsEnabled := isMember
	canJoin = canJoin && !isMember && kind == KindChannel && !room.GetArchived()
	canPostMessage = canPostMessage && messageActionsEnabled
	canPostInThread = canPostInThread && messageActionsEnabled
	canAttach = canAttach && messageActionsEnabled
	canReact = canReact && messageActionsEnabled
	canEcho = canEcho && messageActionsEnabled
	canManageOthersMessage = canManageOthersMessage && memberActionsEnabled
	if kind == KindDM {
		canManageRoom = false
		canBanRoomMembers = false
	}

	return DirectoryRoomViewerState{
		IsMember:               isMember,
		HasUnread:              hasUnread,
		CanListRoom:            canList,
		CanJoinRoom:            canJoin,
		CanPostMessage:         canPostMessage,
		CanPostInThread:        canPostInThread,
		CanAttach:              canAttach,
		CanReact:               canReact,
		CanEchoMessage:         canEcho,
		CanManageOthersMessage: canManageOthersMessage,
		CanManageRoom:          canManageRoom,
		CanBanRoomMembers:      canBanRoomMembers,
	}, nil
}

func (s *RoomDirectoryReadModel) canSeeRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (bool, error) {
	if kind == KindDM {
		return s.core.RoomMembershipExists(ctx, kind, actorID, roomID)
	}
	return s.core.CanSeeRoom(ctx, actorID, kind, roomID)
}
