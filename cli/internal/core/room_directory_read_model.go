package core

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// RoomDirectoryReads returns the operation-level model for public room
// directory and sidebar reads.
func (c *ChattoCore) RoomDirectoryReads() *RoomDirectoryReadModel {
	return c.roomDirectoryReads
}

// RoomDirectoryReadModel composes authorized room views from explicit domain
// dependencies. Construction does not start projectors or background work.
type RoomDirectoryReadModel struct {
	invitations   *RoomInvitationModel
	rooms         *RoomModel
	permissions   *PermissionResolver
	messages      *MessageModel
	authorization roomDirectoryAuthorization
	membership    roomDirectoryMembership
	unread        roomDirectoryUnread
}

// newRoomDirectoryReadModel returns a RoomDirectoryReadModel with all
// collaborators.
func newRoomDirectoryReadModel(
	rooms *RoomModel,
	permissions *PermissionResolver,
	messages *MessageModel,
	authorization roomDirectoryAuthorization,
	membership roomDirectoryMembership,
	unread roomDirectoryUnread,
	invitations *RoomInvitationModel,
) *RoomDirectoryReadModel {
	return &RoomDirectoryReadModel{
		rooms: rooms, permissions: permissions, messages: messages,
		authorization: authorization, membership: membership, unread: unread, invitations: invitations,
	}
}

// RoomArchiveFilter selects room archive state without changing visibility.
type RoomArchiveFilter int

const (
	// RoomArchiveActive preserves the default active-room navigation snapshot.
	RoomArchiveActive RoomArchiveFilter = iota
	// RoomArchiveArchived selects only archived rooms.
	RoomArchiveArchived
	// RoomArchiveAll selects both active and archived rooms.
	RoomArchiveAll
)

func (filter RoomArchiveFilter) matches(archived bool) bool {
	return filter == RoomArchiveAll || (filter == RoomArchiveArchived) == archived
}

type RoomDirectoryListOptions struct {
	IncludeChannels bool
	IncludeDMs      bool
	// ArchiveFilter defaults to active rooms and intersects the room-kind flags.
	ArchiveFilter RoomArchiveFilter
	// IncludeEmptyDMs includes accessible DMs before their first message.
	// Public directory and snapshot reads set this; clients choose whether
	// to hide empty conversations from navigation.
	IncludeEmptyDMs bool
}

type RoomDirectoryGroupOptions struct {
	IncludeArchivedRooms bool
}

type DirectoryRoom struct {
	Room        *readmodel.Room
	ViewerState DirectoryRoomViewerState
	// DM posting activity, exposed only to members with broad message access.
	// Zero means no qualifying message or no access to message-derived state.
	LastMessageAt     time.Time
	LastRootMessageAt time.Time
}

type DirectoryRoomViewerState struct {
	Invitation          *RoomInvitationState
	CanAcceptInvitation bool
	CanInvite           bool
	CallPermissions     CallPermissions // Effective call actions, false for non-members.
	IsMember            bool
	HasUnread           bool
	CanListRoom         bool
	CanJoinRoom         bool
	// CanRejoin retains joining authority even while the viewer is a member.
	CanRejoin              bool
	CanReadMessages        bool
	CanReadInteractions    bool
	CanPostMessage         bool
	CanPostInThread        bool
	CanPostInteractions    bool // RBAC gate only; each reply also needs a thread relationship.
	CanAttach              bool
	CanReact               bool
	CanEchoMessage         bool
	CanPinMessages         bool // Pin mutations in an active channel where the viewer is a member.
	CanManageNotice        bool // Notice mutations by members of an active channel.
	CanManageOthersMessage bool
	CanManageRoom          bool
	CanBanRoomMembers      bool
	SlowModeNextPostAt     time.Time
}

type DirectoryRoomGroup struct {
	Group       *readmodel.RoomGroup
	ViewerState DirectoryRoomGroupViewerState
	Rooms       []*DirectoryRoom
	Items       []DirectoryRoomGroupItem
}

type DirectoryRoomGroupViewerState struct {
	CanCreateRoom      bool
	CanManageRoomGroup bool
}

type DirectoryRoomGroupItem struct {
	Room        *DirectoryRoom
	SidebarLink *readmodel.SidebarLink
}

func (s *RoomDirectoryReadModel) ListRooms(ctx context.Context, actorID string, opts RoomDirectoryListOptions) ([]*DirectoryRoom, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.invitations.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	if opts.ArchiveFilter < RoomArchiveActive || opts.ArchiveFilter > RoomArchiveAll {
		return nil, invalidArgument("unknown room archive filter")
	}

	rooms := []*DirectoryRoom{}
	if opts.IncludeChannels {
		channelRooms, err := s.visibleChannelRooms(ctx, actorID, opts.ArchiveFilter)
		if err != nil {
			return nil, err
		}
		rooms = append(rooms, channelRooms...)
	}
	if opts.IncludeDMs {
		dmRooms, err := s.visibleDMRooms(ctx, actorID, opts.IncludeEmptyDMs, opts.ArchiveFilter)
		if err != nil {
			return nil, err
		}
		rooms = append(rooms, dmRooms...)
	}
	return rooms, nil
}

// RoomDirectoryPage contains a visible room page and its filtered count.
type RoomDirectoryPage struct {
	Rooms      []*DirectoryRoom
	TotalCount int
	HasMore    bool
}

// ListRoomsPage filters and orders room identities before hydrating the page.
// Complete internal snapshots continue to use ListRooms.
func (s *RoomDirectoryReadModel) ListRoomsPage(ctx context.Context, actorID string, opts RoomDirectoryListOptions, limit, offset int) (*RoomDirectoryPage, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.invitations.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	if opts.ArchiveFilter < RoomArchiveActive || opts.ArchiveFilter > RoomArchiveAll {
		return nil, invalidArgument("unknown room archive filter")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var candidates []*readmodel.Room
	if opts.IncludeChannels {
		rooms, err := s.rooms.ListRooms(ctx, KindChannel)
		if err != nil {
			return nil, err
		}
		for _, room := range rooms {
			if !opts.ArchiveFilter.matches(room.GetArchived()) {
				continue
			}
			visible, err := s.canSeeRoom(ctx, actorID, KindChannel, room.Id)
			if err != nil {
				return nil, err
			}
			if visible {
				candidates = append(candidates, room)
			}
		}
	}
	if opts.IncludeDMs {
		rooms, err := s.membership.ListMemberRooms(ctx, KindDM, actorID, MemberRoomListOptions{})
		if err != nil {
			return nil, err
		}
		for _, room := range rooms {
			if !opts.ArchiveFilter.matches(room.GetArchived()) {
				continue
			}
			if !opts.IncludeEmptyDMs {
				canAccess, err := s.authorization.CanAccessRoomMessages(ctx, actorID, KindDM, room.Id)
				if err != nil {
					return nil, err
				}
				if canAccess {
					at, err := s.rooms.GetRoomLastMessageAt(ctx, KindDM, room.Id)
					if err != nil {
						return nil, err
					}
					if at.IsZero() {
						continue
					}
				}
			}
			candidates = append(candidates, room)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Id < candidates[j].Id })
	selected, total, more := paginateCoreSlice(candidates, limit, offset)
	result := &RoomDirectoryPage{Rooms: make([]*DirectoryRoom, 0, len(selected)), TotalCount: total, HasMore: more}
	for _, room := range selected {
		entry, err := s.directoryRoom(ctx, actorID, room)
		if err != nil {
			return nil, err
		}
		result.Rooms = append(result.Rooms, entry)
	}
	return result, nil
}

func (s *RoomDirectoryReadModel) ListRoomGroups(ctx context.Context, actorID string, opts RoomDirectoryGroupOptions) ([]*DirectoryRoomGroup, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.invitations.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}

	visibleRooms, err := s.visibleChannelRoomMap(ctx, actorID, opts.IncludeArchivedRooms)
	if err != nil {
		return nil, err
	}
	groups, err := s.rooms.ListRoomGroupsOrdered(ctx, KindChannel)
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
	if err := s.invitations.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	visibleRooms, err := s.visibleChannelRoomMap(ctx, actorID, opts.IncludeArchivedRooms)
	if err != nil {
		return nil, err
	}
	group, err := s.rooms.GetRoomGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return s.directoryGroup(ctx, actorID, group, visibleRooms)
}

func (s *RoomDirectoryReadModel) BatchGetRoomGroups(ctx context.Context, actorID string, groupIDs []string, opts RoomDirectoryGroupOptions) ([]*DirectoryRoomGroup, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := s.invitations.projection.Projector().WaitForCurrent(ctx); err != nil {
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

		group, err := s.rooms.GetRoomGroup(ctx, groupID)
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
	if err := s.invitations.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	return s.getRoom(ctx, actorID, roomID)
}

func (s *RoomDirectoryReadModel) getRoom(ctx context.Context, actorID, roomID string) (*DirectoryRoom, error) {
	room, err := s.rooms.FindRoomByID(ctx, roomID)
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
	if err := s.invitations.projection.Projector().WaitForCurrent(ctx); err != nil {
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

func (s *RoomDirectoryReadModel) visibleChannelRooms(ctx context.Context, actorID string, archiveFilter RoomArchiveFilter) ([]*DirectoryRoom, error) {
	rooms, err := s.rooms.ListRooms(ctx, KindChannel)
	if err != nil {
		return nil, err
	}
	result := make([]*DirectoryRoom, 0, len(rooms))
	for _, room := range rooms {
		if !archiveFilter.matches(room.GetArchived()) {
			continue
		}
		visible, err := s.canSeeRoom(ctx, actorID, KindChannel, room.Id)
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

func (s *RoomDirectoryReadModel) visibleDMRooms(ctx context.Context, actorID string, includeEmpty bool, archiveFilter RoomArchiveFilter) ([]*DirectoryRoom, error) {
	rooms, err := s.membership.ListMemberRooms(ctx, KindDM, actorID, MemberRoomListOptions{})
	if err != nil {
		return nil, err
	}
	type visibleDMRoom struct {
		room          *DirectoryRoom
		lastMessageAt time.Time
	}
	visible := make([]visibleDMRoom, 0, len(rooms))
	for _, room := range rooms {
		if !archiveFilter.matches(room.GetArchived()) {
			continue
		}
		canAccess, err := s.authorization.CanAccessRoomMessages(ctx, actorID, KindDM, room.GetId())
		if err != nil {
			return nil, err
		}
		var lastMessageAt time.Time
		if canAccess {
			lastMessageAt, err = s.rooms.GetRoomLastMessageAt(ctx, KindDM, room.GetId())
			if err != nil {
				return nil, err
			}
			if !includeEmpty && lastMessageAt.IsZero() {
				continue
			}
		}
		dirRoom, err := s.directoryRoom(ctx, actorID, room)
		if err != nil {
			return nil, err
		}
		visible = append(visible, visibleDMRoom{room: dirRoom, lastMessageAt: lastMessageAt})
	}
	// Only readable message activity affects ordering. A member without read
	// permission still sees the conversation without message-derived state.
	sort.SliceStable(visible, func(i, j int) bool {
		return visible[i].lastMessageAt.After(visible[j].lastMessageAt)
	})
	result := make([]*DirectoryRoom, len(visible))
	for i, room := range visible {
		result[i] = room.room
	}
	return result, nil
}

func (s *RoomDirectoryReadModel) visibleChannelRoomMap(ctx context.Context, actorID string, includeArchived bool) (map[string]*readmodel.Room, error) {
	rooms, err := s.rooms.ListRooms(ctx, KindChannel)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*readmodel.Room, len(rooms))
	for _, room := range rooms {
		if room.GetArchived() && !includeArchived {
			continue
		}
		visible, err := s.canSeeRoom(ctx, actorID, KindChannel, room.Id)
		if err != nil {
			return nil, err
		}
		if visible {
			result[room.Id] = room
		}
	}
	return result, nil
}

func (s *RoomDirectoryReadModel) directoryGroup(ctx context.Context, actorID string, group *readmodel.RoomGroup, visibleRooms map[string]*readmodel.Room) (*DirectoryRoomGroup, error) {
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

	sidebarLinks := make(map[string]*readmodel.SidebarLink, len(group.GetSidebarLinks()))
	for _, link := range group.GetSidebarLinks() {
		sidebarLinks[link.GetId()] = link
	}
	for _, entry := range group.GetEntries() {
		switch entry.GetKind() {
		case evtv1.SidebarGroupEntry_ROOM:
			room := visibleRooms[entry.GetId()]
			if room == nil {
				continue
			}
			dirRoom, err := s.directoryRoom(ctx, actorID, room)
			if err != nil {
				return nil, err
			}
			dirGroup.Items = append(dirGroup.Items, DirectoryRoomGroupItem{Room: dirRoom})
		case evtv1.SidebarGroupEntry_SIDEBAR_LINK:
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
	canCreateRoom, err := s.authorization.CanCreateRoom(ctx, actorID, KindChannel, groupID)
	if err != nil {
		return DirectoryRoomGroupViewerState{}, err
	}
	canManageRoomGroup, err := s.authorization.CanManageRoomGroup(ctx, actorID, groupID)
	if err != nil {
		return DirectoryRoomGroupViewerState{}, err
	}
	return DirectoryRoomGroupViewerState{
		CanCreateRoom:      canCreateRoom,
		CanManageRoomGroup: canManageRoomGroup,
	}, nil
}

func (s *RoomDirectoryReadModel) directoryRoom(ctx context.Context, actorID string, room *readmodel.Room) (*DirectoryRoom, error) {
	state, err := s.roomViewerState(ctx, actorID, room)
	if err != nil {
		return nil, err
	}
	result := &DirectoryRoom{Room: room, ViewerState: state}
	if KindOfRoom(room) == KindDM && state.IsMember && state.CanReadMessages {
		result.LastMessageAt, result.LastRootMessageAt = s.rooms.lastPostedMessageTimes(room.Id)
	}
	return result, nil
}

func (s *RoomDirectoryReadModel) roomViewerState(ctx context.Context, actorID string, room *readmodel.Room) (DirectoryRoomViewerState, error) {
	kind := KindOfRoom(room)
	isMember, err := s.membership.RoomMembershipExists(ctx, kind, actorID, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canReadMessages, err := s.authorization.CanReadMessages(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canReadInteractions, err := s.authorization.CanReadMessageInteractions(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	hasUnread := false
	if isMember && (canReadMessages || canReadInteractions) {
		hasUnread, err = s.unread.HasUnread(ctx, kind, actorID, room.Id)
		if err != nil {
			return DirectoryRoomViewerState{}, err
		}
	}
	canList, err := s.canSeeRoom(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canJoin, err := s.authorization.CanJoinRoomAt(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canPostMessage, err := s.authorization.CanPostMessage(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canPostInThread, err := s.authorization.CanPostInThread(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canPostInteractions, err := s.authorization.hasRoomPermission(ctx, kind, room.Id, actorID, PermMessagePostInInteractions)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canAttach, err := s.authorization.CanAttachFiles(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canReact, err := s.authorization.CanReactToMessage(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canEcho, err := s.authorization.CanEchoMessage(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canManageOthersMessage, err := s.authorization.CanManageOthersMessage(ctx, actorID, kind, room.Id)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canPin, err := s.permissions.HasRoomPermission(ctx, actorID, kind, room.Id, PermMessagePin)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canManageNotice, err := s.permissions.HasRoomPermission(ctx, actorID, kind, room.Id, PermRoomManageNotice)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canManageRoom, err := s.permissions.HasRoomPermission(ctx, actorID, kind, room.Id, PermRoomManage)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	canBanRoomMembers, err := s.permissions.HasRoomPermission(ctx, actorID, kind, room.Id, PermRoomMemberRemove)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}

	var callPermissions CallPermissions
	if isMember {
		callPermissions, err = s.authorization.resolveCallPermissions(ctx, actorID, kind, room.Id)
		if err != nil {
			return DirectoryRoomViewerState{}, err
		}
	}
	messageActionsEnabled := isMember && !room.GetArchived()
	memberActionsEnabled := isMember
	canRejoin := canJoin && kind == KindChannel && !room.GetArchived()
	canJoin = canRejoin && !isMember
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
	slowModeNextPostAt := time.Time{}
	if isMember {
		slowModeNextPostAt = s.messages.slowModeNextPostAt(room, actorID, canManageRoom || canManageOthersMessage, time.Now())
	}

	var invitation *RoomInvitationState
	canAccept := false
	if !isMember && s.invitations != nil && !room.GetArchived() && !room.GetUniversal() {
		if v, ok := s.invitations.projection.Projection().pending(room.Id, actorID, time.Now()); ok {
			invitation = &v
			hasUnread, err = s.unread.HasUnread(ctx, kind, actorID, room.Id)
			if err != nil {
				return DirectoryRoomViewerState{}, err
			}
			canAccept, err = s.invitations.CanAccept(ctx, actorID, v)
			if err != nil {
				return DirectoryRoomViewerState{}, err
			}
		}
	}
	canInvite, err := s.permissions.HasRoomPermission(withPrivilegedModeEvaluation(ctx, actorID, false), actorID, kind, room.Id, PermRoomInvite)
	if err != nil {
		return DirectoryRoomViewerState{}, err
	}
	return DirectoryRoomViewerState{
		Invitation: invitation, CanAcceptInvitation: canAccept, CanInvite: canInvite && isMember && kind == KindChannel && !room.GetArchived() && !room.GetUniversal(),
		CallPermissions:        callPermissions,
		IsMember:               isMember,
		HasUnread:              hasUnread,
		CanListRoom:            canList,
		CanJoinRoom:            canJoin,
		CanRejoin:              canRejoin,
		CanReadMessages:        isMember && canReadMessages,
		CanReadInteractions:    isMember && canReadInteractions,
		CanPostMessage:         canPostMessage,
		CanPostInThread:        canPostInThread,
		CanPostInteractions:    canPostInteractions && messageActionsEnabled,
		CanAttach:              canAttach,
		CanReact:               canReact,
		CanEchoMessage:         canEcho,
		CanPinMessages:         canPin && messageActionsEnabled && kind == KindChannel,
		CanManageNotice:        canManageNotice && messageActionsEnabled && kind == KindChannel,
		CanManageOthersMessage: canManageOthersMessage,
		CanManageRoom:          canManageRoom,
		CanBanRoomMembers:      canBanRoomMembers,
		SlowModeNextPostAt:     slowModeNextPostAt,
	}, nil
}

func (s *RoomDirectoryReadModel) canSeeRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (bool, error) {
	if kind == KindDM {
		return s.membership.RoomMembershipExists(ctx, kind, actorID, roomID)
	}
	visible, err := s.authorization.CanSeeRoom(ctx, actorID, kind, roomID)
	if err != nil || visible || s.invitations == nil {
		return visible, err
	}
	room, err := s.rooms.GetRoom(ctx, kind, roomID)
	if err != nil {
		return false, err
	}
	if room.GetArchived() || room.GetUniversal() {
		return false, nil
	}
	_, invited := s.invitations.projection.Projection().pending(roomID, actorID, time.Now())
	return invited, nil
}

// roomDirectoryAuthorization supplies authorization operations used by the room model.
type roomDirectoryAuthorization interface {
	CanSeeRoom(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error)
	CanAccessRoomMessages(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error)
	CanCreateRoom(ctx context.Context, userID string, kind RoomKind, groupID string) (bool, error)
	CanManageRoomGroup(ctx context.Context, userID, groupID string) (bool, error)
	CanReadMessages(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error)
	CanReadMessageInteractions(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error)
	CanJoinRoomAt(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error)
	CanPostMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error)
	CanPostInThread(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error)
	hasRoomPermission(ctx context.Context, kind RoomKind, roomID, userID string, perm Permission) (bool, error)
	CanAttachFiles(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error)
	CanReactToMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error)
	CanEchoMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error)
	CanManageOthersMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error)
	resolveCallPermissions(ctx context.Context, actorID string, kind RoomKind, roomID string) (CallPermissions, error)
}

// roomDirectoryMembership supplies membership operations used by the room model.
type roomDirectoryMembership interface {
	ListMemberRooms(ctx context.Context, kind RoomKind, userID string, opts MemberRoomListOptions) ([]*readmodel.Room, error)
	RoomMembershipExists(ctx context.Context, kind RoomKind, user_id, room_id string) (bool, error)
}

// roomDirectoryUnread supplies unread operations used by the room model.
type roomDirectoryUnread interface {
	HasUnread(ctx context.Context, kind RoomKind, userID, roomID string) (bool, error)
}
