package http_server

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireGetViewer(ctx context.Context, user *corev1.User, requestID string) (*apiv1.GetViewerResponse, *wirev1.WireError) {
	profile, err := c.serverProfileView(ctx)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	permissions, err := c.viewerPermissionsView(ctx, user.GetId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	serverPref, err := c.serverNotificationPreferenceView(ctx, user.GetId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	roomPrefs, err := c.roomNotificationPreferenceViews(ctx, user.GetId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	return &apiv1.GetViewerResponse{
		Viewer: &apiv1.Viewer{
			User:                         cloneUser(user),
			Permissions:                  permissions,
			ServerNotificationPreference: serverPref,
			RoomNotificationPreferences:  roomPrefs,
		},
		ServerProfile: profile,
	}, nil
}

func (c *wireConn) serverProfileView(ctx context.Context) (*apiv1.ServerProfileView, error) {
	name := "Chatto"
	if cm := c.server.core.ConfigManager(); cm != nil {
		resolved, err := cm.GetEffectiveServerName(ctx)
		if err != nil {
			return nil, err
		}
		name = resolved
	}
	logoURL, err := c.server.core.GetServerLogoURL(ctx, nil, nil, "")
	if err != nil {
		return nil, err
	}
	bannerURL, err := c.server.core.GetServerBannerURL(ctx, nil, nil, "")
	if err != nil {
		return nil, err
	}
	return &apiv1.ServerProfileView{Name: name, LogoUrl: logoURL, BannerUrl: bannerURL}, nil
}

func (c *wireConn) viewerPermissionsView(ctx context.Context, userID string) (*apiv1.ViewerPermissionsView, error) {
	view := &apiv1.ViewerPermissionsView{}
	var err error
	if view.CanViewAdmin, err = c.server.core.HasAnyAdminPermission(ctx, userID); err != nil {
		return nil, err
	}
	if view.CanStartDms, err = c.server.core.CanStartDM(ctx, userID); err != nil {
		return nil, err
	}
	if view.CanAdminViewUsers, err = c.server.core.CanAdminUsersView(ctx, userID); err != nil {
		return nil, err
	}
	if view.CanAdminManageUsers, err = c.server.core.CanAssignRoles(ctx, userID); err != nil {
		return nil, err
	}
	if view.CanAdminViewRoles, err = c.server.core.CanManageRoles(ctx, userID); err != nil {
		return nil, err
	}
	if view.CanAdminManageRoles, err = c.server.core.CanManageRoles(ctx, userID); err != nil {
		return nil, err
	}
	if view.CanAdminViewSystem, err = c.server.core.IsServerOwner(ctx, userID); err != nil {
		return nil, err
	}
	if view.CanAdminViewAudit, err = c.server.core.HasServerPermission(ctx, userID, core.PermAdminAuditView); err != nil {
		return nil, err
	}
	if view.CanManageServer, err = c.server.core.CanManageServer(ctx, userID); err != nil {
		return nil, err
	}
	if view.CanManageRooms, err = c.server.core.CanManageAnyRoom(ctx, userID); err != nil {
		return nil, err
	}
	if view.CanManageRoles, err = c.server.core.CanManageRoles(ctx, userID); err != nil {
		return nil, err
	}
	if view.CanAssignRoles, err = c.server.core.CanAssignRoles(ctx, userID); err != nil {
		return nil, err
	}
	if view.CanManageUserPermissions, err = c.server.core.CanManageUserPermissions(ctx, userID); err != nil {
		return nil, err
	}
	return view, nil
}

func (c *wireConn) serverNotificationPreferenceView(ctx context.Context, userID string) (*apiv1.ViewerNotificationPreferenceView, error) {
	level, err := c.server.core.GetSpaceNotificationLevel(ctx, userID)
	if err != nil {
		return nil, err
	}
	effectiveLevel := level
	if effectiveLevel == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
		effectiveLevel = corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	}
	return &apiv1.ViewerNotificationPreferenceView{
		Level:          level,
		EffectiveLevel: effectiveLevel,
	}, nil
}

func (c *wireConn) roomNotificationPreferenceViews(ctx context.Context, userID string) ([]*apiv1.RoomNotificationPreferenceView, error) {
	prefs, err := c.server.core.GetAllRoomNotificationPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]*apiv1.RoomNotificationPreferenceView, 0, len(prefs))
	for _, pref := range prefs {
		out = append(out, &apiv1.RoomNotificationPreferenceView{
			RoomId:         pref.RoomID,
			Level:          pref.Level,
			EffectiveLevel: pref.EffectiveLevel,
		})
	}
	return out, nil
}

func (c *wireConn) handleWireListMyRooms(ctx context.Context, userID, requestID string, body *apiv1.ListMyRoomsRequest) (*apiv1.ListMyRoomsResponse, *wirev1.WireError) {
	kind := roomKindFromProto(body.GetKind())
	opts := core.MemberRoomListOptions{}
	if kind == core.KindDM {
		opts.RequireLastMessage = true
		opts.SortByLastMessageDesc = true
	}

	rooms, err := c.server.core.ListMemberRooms(ctx, kind, userID, opts)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	resp := &apiv1.ListMyRoomsResponse{
		Rooms:        cloneRooms(rooms),
		ViewerUserId: userID,
		RoomViews:    make([]*apiv1.RoomListItemView, 0, len(rooms)),
	}
	if kind == core.KindChannel {
		groups, err := c.server.core.ListRoomGroupsOrdered(ctx, core.KindChannel)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		resp.RoomGroups = cloneRoomGroups(groups)
	}

	for _, room := range rooms {
		if room == nil || room.GetArchived() {
			continue
		}
		view, err := c.roomListItemView(ctx, userID, kind, room)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		resp.RoomViews = append(resp.RoomViews, view)
	}
	return resp, nil
}

func (c *wireConn) roomListItemView(ctx context.Context, userID string, kind core.RoomKind, room *corev1.Room) (*apiv1.RoomListItemView, error) {
	hasUnread, err := c.server.core.HasUnread(ctx, kind, userID, room.GetId())
	if err != nil {
		return nil, err
	}
	level, err := c.server.core.GetRoomNotificationLevel(ctx, userID, room.GetId())
	if err != nil {
		return nil, err
	}
	effectiveLevel, err := c.server.core.GetEffectiveNotificationLevel(ctx, userID, room.GetId())
	if err != nil {
		return nil, err
	}

	members, err := c.sortedRoomMembers(ctx, kind, room.GetId())
	if err != nil {
		return nil, err
	}
	if len(members) > 100 {
		members = members[:100]
	}

	return &apiv1.RoomListItemView{
		Room:      cloneRoom(room),
		HasUnread: hasUnread,
		ViewerNotificationPreference: &apiv1.ViewerNotificationPreferenceView{
			Level:          level,
			EffectiveLevel: effectiveLevel,
		},
		Members: cloneUsers(members),
	}, nil
}

func (c *wireConn) handleWireGetRoom(ctx context.Context, userID, requestID string, body *apiv1.GetRoomRequest) (*apiv1.GetRoomResponse, *wirev1.WireError) {
	room, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	members, errWire := c.roomMembersPage(ctx, userID, requestID, kind, body.GetRoomId(), int(body.GetMembersLimit()), int(body.GetMembersOffset()))
	if errWire != nil {
		return nil, errWire
	}

	serverName := "Chatto"
	if cm := c.server.core.ConfigManager(); cm != nil {
		name, err := cm.GetEffectiveServerName(ctx)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		serverName = name
	}

	resp := &apiv1.GetRoomResponse{
		Room:         cloneRoom(room),
		ServerName:   serverName,
		Members:      members,
		ViewerUserId: userID,
		MentionRoles: c.mentionRoles(ctx),
	}

	if resp.ViewerCanPostMessage, err = c.server.core.CanPostMessage(ctx, userID, kind, body.GetRoomId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if resp.ViewerCanPostInThread, err = c.server.core.CanPostInThread(ctx, userID, kind, body.GetRoomId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if resp.ViewerCanReact, err = c.server.core.CanReactToMessage(ctx, userID, kind, body.GetRoomId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if resp.ViewerCanManageOthersMessage, err = c.server.core.CanManageOthersMessage(ctx, userID, kind, body.GetRoomId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if resp.ViewerCanEchoMessage, err = c.server.core.CanEchoMessage(ctx, userID, kind, body.GetRoomId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if resp.ViewerCanManageRoom, err = c.server.core.PermResolver().HasRoomPermission(ctx, userID, kind, body.GetRoomId(), core.PermRoomManage); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if resp.ViewerCanBanRoomMembers, err = c.server.core.PermResolver().HasRoomPermission(ctx, userID, kind, body.GetRoomId(), core.PermRoomMemberBan); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if resp.ViewerCanManageRooms, err = c.server.core.CanManageAnyRoom(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	return resp, nil
}

func (c *wireConn) handleWireGetRoomMembers(ctx context.Context, userID, requestID string, body *apiv1.GetRoomMembersRequest) (*apiv1.GetRoomMembersResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	members, errWire := c.roomMembersPage(ctx, userID, requestID, kind, body.GetRoomId(), int(body.GetLimit()), int(body.GetOffset()))
	if errWire != nil {
		return nil, errWire
	}
	return &apiv1.GetRoomMembersResponse{Members: members}, nil
}

func (c *wireConn) handleWireGetRoomDirectory(ctx context.Context, userID, requestID string) (*apiv1.GetRoomDirectoryResponse, *wirev1.WireError) {
	rooms, err := c.server.core.ListRooms(ctx, core.KindChannel)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	resp := &apiv1.GetRoomDirectoryResponse{
		RoomViews: make([]*apiv1.RoomDirectoryItemView, 0, len(rooms)),
	}
	for _, room := range rooms {
		visible, err := c.server.core.CanSeeRoom(ctx, userID, core.KindChannel, room.GetId())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if !visible {
			continue
		}
		canJoin, err := c.server.core.CanJoinRoomAt(ctx, userID, core.KindChannel, room.GetId())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		resp.RoomViews = append(resp.RoomViews, &apiv1.RoomDirectoryItemView{
			Room:              cloneRoom(room),
			ViewerCanJoinRoom: canJoin,
		})
	}
	return resp, nil
}

func (c *wireConn) handleWireSearchMembers(ctx context.Context, userID, requestID string, body *apiv1.SearchMembersRequest) (*apiv1.SearchMembersResponse, *wirev1.WireError) {
	limit, offset := normalizeWirePagination(int(body.GetLimit()), int(body.GetOffset()), 20, 100)
	members, totalCount, err := c.server.core.GetServerMembers(ctx, body.GetSearch(), limit, offset)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	users := make([]*corev1.User, 0, len(members))
	for _, member := range members {
		u, err := c.server.core.GetUser(ctx, member.UserID)
		if err != nil {
			c.server.logger.Debug("Failed to get user for wire member search", "user_id", member.UserID, "error", err)
			continue
		}
		if u != nil {
			users = append(users, cloneUser(u))
		}
	}

	canStartDMs, err := c.server.core.CanStartDM(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	return &apiv1.SearchMembersResponse{
		Users:             users,
		TotalCount:        int32(totalCount),
		HasMore:           offset+len(users) < totalCount,
		ViewerUserId:      userID,
		ViewerCanStartDms: canStartDMs,
	}, nil
}

func (c *wireConn) handleWireStartDM(ctx context.Context, userID, requestID string, body *apiv1.StartDMRequest) (*apiv1.StartDMResponse, *wirev1.WireError) {
	can, err := c.server.core.CanStartDM(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !can {
		return nil, c.errorFromRequestErr(requestID, core.ErrPermissionDenied)
	}
	room, created, err := c.server.core.FindOrCreateDM(ctx, userID, body.GetParticipantIds())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.StartDMResponse{
		Room:    cloneRoom(room),
		Created: created,
	}, nil
}

func (c *wireConn) handleWireCreateRoom(ctx context.Context, userID, requestID string, body *apiv1.CreateRoomRequest) (*apiv1.CreateRoomResponse, *wirev1.WireError) {
	const kind = core.KindChannel
	can, err := c.server.core.CanCreateRoom(ctx, userID, kind, body.GetGroupId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !can {
		return nil, c.errorFromRequestErr(requestID, core.ErrPermissionDenied)
	}
	room, err := c.server.core.CreateRoom(ctx, userID, kind, body.GetGroupId(), body.GetName(), body.GetDescription())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.CreateRoomResponse{Room: cloneRoom(room)}, nil
}

func (c *wireConn) handleWireJoinRoom(ctx context.Context, userID, requestID string, body *apiv1.JoinRoomRequest) (*apiv1.JoinRoomResponse, *wirev1.WireError) {
	if body.GetRoomId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: room_id is required", errWireInvalidArgument))
	}
	kind, err := c.server.core.FindRoomKind(ctx, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	canJoin, err := c.server.core.CanJoinRoomAt(ctx, userID, kind, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !canJoin {
		return nil, wireError(requestID, wirev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "permission denied", false)
	}
	if _, err := c.server.core.JoinRoom(ctx, userID, kind, userID, body.GetRoomId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	room, err := c.server.core.GetRoom(ctx, kind, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.JoinRoomResponse{Room: cloneRoom(room)}, nil
}

func (c *wireConn) handleWireLeaveRoom(ctx context.Context, userID, requestID string, body *apiv1.LeaveRoomRequest) (*apiv1.LeaveRoomResponse, *wirev1.WireError) {
	if body.GetRoomId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: room_id is required", errWireInvalidArgument))
	}
	kind, err := c.server.core.FindRoomKind(ctx, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.LeaveRoom(ctx, userID, kind, userID, body.GetRoomId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.LeaveRoomResponse{}, nil
}

func (c *wireConn) handleWireJoinGroup(ctx context.Context, userID, requestID string, body *apiv1.JoinGroupRequest) (*apiv1.JoinGroupResponse, *wirev1.WireError) {
	if body.GetGroupId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: group_id is required", errWireInvalidArgument))
	}
	group, err := c.server.core.GetRoomGroup(ctx, body.GetGroupId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	joined := make([]string, 0, len(group.GetRoomIds()))
	for _, roomID := range group.GetRoomIds() {
		room, err := c.server.core.GetRoom(ctx, core.KindChannel, roomID)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if room.GetArchived() {
			continue
		}
		alreadyMember, err := c.server.core.RoomMembershipExists(ctx, core.KindChannel, userID, roomID)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if alreadyMember {
			continue
		}
		canJoin, err := c.server.core.CanJoinRoomAt(ctx, userID, core.KindChannel, roomID)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if !canJoin {
			continue
		}
		if _, err := c.server.core.JoinRoom(ctx, userID, core.KindChannel, userID, roomID); err != nil {
			return nil, c.errorFromRequestErr(requestID, fmt.Errorf("join %s: %w", roomID, err))
		}
		joined = append(joined, roomID)
	}
	return &apiv1.JoinGroupResponse{JoinedRoomIds: joined}, nil
}

func (c *wireConn) handleWireBanRoomMember(ctx context.Context, userID, requestID string, body *apiv1.BanRoomMemberRequest) (*apiv1.BanRoomMemberResponse, *wirev1.WireError) {
	if body.GetRoomId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: room_id is required", errWireInvalidArgument))
	}
	if body.GetUserId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: user_id is required", errWireInvalidArgument))
	}
	reason := strings.TrimSpace(body.GetReason())
	if reason == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: ban reason is required", errWireInvalidArgument))
	}
	if len([]rune(reason)) > core.MaxRoomBanReasonLength {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: ban reason exceeds %d characters", errWireInvalidArgument, core.MaxRoomBanReasonLength))
	}

	kind, err := c.server.core.FindRoomKind(ctx, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if kind == core.KindDM {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: cannot ban members from DM rooms", errWireInvalidArgument))
	}

	canBan, err := c.server.core.PermResolver().HasRoomPermission(ctx, userID, core.KindChannel, body.GetRoomId(), core.PermRoomMemberBan)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !canBan {
		return nil, c.errorFromRequestErr(requestID, core.ErrPermissionDenied)
	}

	var expiresAt *time.Time
	if body.GetExpiresAt() != nil {
		value := body.GetExpiresAt().AsTime()
		if !value.After(time.Now()) {
			return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: ban expiry must be in the future", errWireInvalidArgument))
		}
		expiresAt = &value
	}

	if _, err := c.server.core.BanRoomMember(ctx, userID, core.KindChannel, body.GetRoomId(), body.GetUserId(), reason, expiresAt); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.BanRoomMemberResponse{}, nil
}

func (c *wireConn) handleWireGetLinkPreview(ctx context.Context, requestID string, body *apiv1.GetLinkPreviewRequest) (*apiv1.GetLinkPreviewResponse, *wirev1.WireError) {
	if body.GetUrl() == "" {
		return nil, c.errorFromRequestErr(requestID, errWireInvalidArgument)
	}
	preview, err := c.server.core.GetLinkPreview(ctx, body.GetUrl())
	if err != nil {
		c.server.logger.Debug("Failed to get wire link preview", "error", err)
		return &apiv1.GetLinkPreviewResponse{}, nil
	}
	return &apiv1.GetLinkPreviewResponse{Preview: c.linkPreviewView(preview)}, nil
}

func (c *wireConn) handleWireListMyFollowedThreads(ctx context.Context, userID, requestID string, body *apiv1.ListMyFollowedThreadsRequest) (*apiv1.ListMyFollowedThreadsResponse, *wirev1.WireError) {
	limit, offset := normalizeWirePagination(int(body.GetLimit()), int(body.GetOffset()), 20, 100)
	kind := core.KindChannel
	page, err := c.server.core.ListFollowedThreadsPage(ctx, userID, []string{core.LegacySpaceIDForRoomKind(kind)}, limit, offset)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	resp := &apiv1.ListMyFollowedThreadsResponse{
		Threads:    make([]*apiv1.FollowedThreadView, 0, len(page.Threads)),
		TotalCount: int32(page.TotalCount),
		HasMore:    page.HasMore,
	}
	for _, thread := range page.Threads {
		if thread == nil {
			continue
		}
		room, err := c.server.core.GetRoom(ctx, kind, thread.RoomID)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		root, err := c.threadRootEventView(ctx, userID, kind, thread.RoomID, thread.ThreadRootEventID)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}

		view := &apiv1.FollowedThreadView{
			RoomId:            thread.RoomID,
			Room:              cloneRoom(room),
			ThreadRootEventId: thread.ThreadRootEventID,
			RootMessage:       root,
			ReplyCount:        int32(thread.ReplyCount),
			HasUnread:         thread.HasUnread,
		}
		if thread.LastReplyAt != nil {
			view.LastReplyAt = timestamppb.New(*thread.LastReplyAt)
		}
		for i, participantID := range thread.ParticipantIDs {
			if i >= 3 {
				break
			}
			if user := c.optionalUser(ctx, participantID); user != nil {
				view.ThreadParticipants = append(view.ThreadParticipants, user)
			}
		}
		resp.Threads = append(resp.Threads, view)
	}
	return resp, nil
}

func (c *wireConn) handleWireFollowThread(ctx context.Context, userID, requestID string, body *apiv1.FollowThreadRequest) (*apiv1.FollowThreadResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.FollowThread(ctx, kind, userID, body.GetRoomId(), body.GetThreadRootEventId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.FollowThreadResponse{Changed: true}, nil
}

func (c *wireConn) handleWireUnfollowThread(ctx context.Context, userID, requestID string, body *apiv1.UnfollowThreadRequest) (*apiv1.UnfollowThreadResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.UnfollowThread(ctx, kind, userID, body.GetRoomId(), body.GetThreadRootEventId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.UnfollowThreadResponse{Changed: true}, nil
}

func (c *wireConn) handleWireMarkRoomAsRead(ctx context.Context, userID, requestID string, body *apiv1.MarkRoomAsReadRequest) (*apiv1.MarkRoomAsReadResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	previousEventID, err := c.server.core.GetLastReadEventID(ctx, kind, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	var (
		lastEventID string
		lastTime    time.Time
		hasLast     bool
	)
	if body.GetUpToEventId() != "" {
		targetTime, err := c.server.core.GetEventTimestamp(ctx, kind, body.GetRoomId(), body.GetUpToEventId())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if !targetTime.IsZero() {
			lastEventID = body.GetUpToEventId()
			lastTime = targetTime
			hasLast = true
		}
	}
	if !hasLast {
		lastEventID, lastTime, hasLast, err = c.server.core.GetRoomLastEvent(ctx, kind, body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
	}

	if hasLast {
		shouldWrite := true
		if previousEventID != "" && previousEventID != lastEventID {
			prevTime, err := c.server.core.GetEventTimestamp(ctx, kind, body.GetRoomId(), previousEventID)
			if err == nil && !prevTime.IsZero() && !lastTime.After(prevTime) {
				shouldWrite = false
				lastEventID = previousEventID
				lastTime = prevTime
			}
		}
		if shouldWrite {
			if err := c.server.core.SetLastReadEventID(ctx, kind, userID, body.GetRoomId(), lastEventID); err != nil {
				return nil, c.errorFromRequestErr(requestID, err)
			}
		}
	}

	c.server.core.NotifyRoomMarkedAsRead(ctx, userID, kind, body.GetRoomId())

	resp := &apiv1.MarkRoomAsReadResponse{}
	if hasLast && !lastTime.IsZero() {
		resp.LastReadAt = timestamppb.New(lastTime)
	}
	if previousEventID != "" {
		if t, err := c.server.core.GetEventTimestamp(ctx, kind, body.GetRoomId(), previousEventID); err == nil && !t.IsZero() {
			resp.PreviousLastReadAt = timestamppb.New(t)
		}
	}
	return resp, nil
}

func (c *wireConn) handleWireMarkThreadAsRead(ctx context.Context, userID, requestID string, body *apiv1.MarkThreadAsReadRequest) (*apiv1.MarkThreadAsReadResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	markerEventID := ""
	if body.GetUpToEventId() != "" {
		event, err := c.server.core.GetRoomEventByEventID(ctx, kind, body.GetRoomId(), body.GetUpToEventId())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if event != nil {
			markerEventID = event.GetId()
		}
	} else {
		events, err := c.server.core.GetThreadEvents(ctx, kind, body.GetRoomId(), body.GetThreadRootEventId())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		for i := len(events) - 1; i >= 0; i-- {
			if events[i] != nil && events[i].GetMessagePosted() != nil {
				markerEventID = events[i].GetId()
				break
			}
		}
	}

	previousReadAt, err := c.server.core.SetThreadLastReadEventID(ctx, kind, userID, body.GetRoomId(), body.GetThreadRootEventId(), markerEventID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	resp := &apiv1.MarkThreadAsReadResponse{}
	if !previousReadAt.IsZero() {
		resp.PreviousReadAt = timestamppb.New(previousReadAt)
	}
	return resp, nil
}

func (c *wireConn) roomMembersPage(ctx context.Context, userID, requestID string, kind core.RoomKind, roomID string, limit, offset int) (*apiv1.RoomMembersPage, *wirev1.WireError) {
	isMember, err := c.server.core.RoomMembershipExists(ctx, kind, userID, roomID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !isMember {
		return nil, c.errorFromRequestErr(requestID, core.ErrNotRoomMember)
	}

	users, err := c.sortedRoomMembers(ctx, kind, roomID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	limit, offset = normalizeWirePagination(limit, offset, 20, 100)
	totalCount := len(users)
	if offset > totalCount {
		offset = totalCount
	}
	end := offset + limit
	if end > totalCount {
		end = totalCount
	}

	return &apiv1.RoomMembersPage{
		Users:      cloneUsers(users[offset:end]),
		TotalCount: int32(totalCount),
		HasMore:    end < totalCount,
	}, nil
}

func (c *wireConn) sortedRoomMembers(ctx context.Context, kind core.RoomKind, roomID string) ([]*corev1.User, error) {
	memberships, err := c.server.core.GetRoomMembersList(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	users := make([]*corev1.User, 0, len(memberships))
	for _, membership := range memberships {
		u, err := c.server.core.GetUser(ctx, membership.GetUserId())
		if err != nil {
			return nil, err
		}
		if u != nil {
			users = append(users, u)
		}
	}
	sort.Slice(users, func(i, j int) bool {
		left := strings.ToLower(users[i].GetDisplayName())
		right := strings.ToLower(users[j].GetDisplayName())
		if left == right {
			return strings.ToLower(users[i].GetLogin()) < strings.ToLower(users[j].GetLogin())
		}
		return left < right
	})
	return users, nil
}

func (c *wireConn) mentionRoles(ctx context.Context) []*apiv1.MentionRole {
	roles, err := c.server.core.ListServerRoles(ctx)
	if err != nil {
		c.server.logger.Debug("Failed to load wire mention roles", "error", err)
		return nil
	}
	out := make([]*apiv1.MentionRole, 0, len(roles))
	for _, role := range roles {
		if role.Name == core.RoleEveryone {
			continue
		}
		out = append(out, &apiv1.MentionRole{
			Name:     role.Name,
			IsSystem: role.IsSystem,
			Position: role.Position,
			Pingable: role.Pingable,
		})
	}
	return out
}

func normalizeWirePagination(limit, offset, defaultLimit, maxLimit int) (int, int) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
