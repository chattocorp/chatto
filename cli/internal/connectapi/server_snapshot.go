package connectapi

import (
	"context"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

const serverSnapshotUserPageSize = 500

// BuildServerSnapshot returns authorized public resource chunks for one
// realtime recovery phase. Each chunk reuses the canonical ConnectRPC response
// shape for that resource family.
func (a *API) BuildServerSnapshot(ctx context.Context, userID string) ([]*apiv1.ServerSnapshotChunk, error) {
	ctx = core.WithDEKRequestCache(ctx)

	server, err := a.serverProfile(ctx, serverProfileOptions{})
	if err != nil {
		return nil, fmt.Errorf("assemble server profile: %w", err)
	}
	viewer, err := a.buildViewer(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assemble viewer: %w", err)
	}
	users, err := a.serverSnapshotUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("assemble users: %w", err)
	}
	rooms, err := a.core.RoomDirectoryReads().ListRooms(ctx, userID, core.RoomDirectoryListOptions{
		IncludeChannels: true,
		IncludeDMs:      true,
		IncludeEmptyDMs: true,
	})
	if err != nil {
		return nil, fmt.Errorf("assemble rooms: %w", err)
	}
	apiRooms := make([]*apiv1.RoomWithViewerState, 0, len(rooms))
	for _, room := range rooms {
		apiRoom, err := a.apiRoomWithViewerState(ctx, userID, room)
		if err != nil {
			return nil, fmt.Errorf("assemble room %q: %w", room.Room.GetId(), err)
		}
		apiRooms = append(apiRooms, apiRoom)
	}
	groups, err := a.serverSnapshotRoomGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assemble room groups: %w", err)
	}
	notifications, err := a.serverSnapshotNotifications(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assemble notifications: %w", err)
	}
	calls, err := a.serverSnapshotActiveCalls(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assemble active calls: %w", err)
	}

	motd := &apiv1.GetMotdResponse{}
	if value := serverMOTD(a); value != "" {
		motd.Motd = stringPtr(value)
	}
	return []*apiv1.ServerSnapshotChunk{
		{Resource: &apiv1.ServerSnapshotChunk_Server{Server: server}},
		{Resource: &apiv1.ServerSnapshotChunk_Motd{Motd: motd}},
		{Resource: &apiv1.ServerSnapshotChunk_RuntimeConfig{RuntimeConfig: &apiv1.GetRuntimeConfigResponse{Runtime: serverRuntimeConfig(a)}}},
		{Resource: &apiv1.ServerSnapshotChunk_Viewer{Viewer: viewer}},
		{Resource: &apiv1.ServerSnapshotChunk_Users{Users: &apiv1.ListUsersResponse{Users: users, Page: apiPageInfo(len(users), false)}}},
		{Resource: &apiv1.ServerSnapshotChunk_Rooms{Rooms: &apiv1.ListRoomsResponse{Rooms: apiRooms}}},
		{Resource: &apiv1.ServerSnapshotChunk_RoomGroups{RoomGroups: &apiv1.ListRoomGroupsResponse{Groups: groups}}},
		{Resource: &apiv1.ServerSnapshotChunk_Notifications{Notifications: notifications}},
		{Resource: &apiv1.ServerSnapshotChunk_ActiveCalls{ActiveCalls: &apiv1.ListActiveCallsResponse{Calls: calls}}},
	}, nil
}

func (a *API) serverSnapshotUsers(ctx context.Context) ([]*apiv1.DirectoryMember, error) {
	var out []*apiv1.DirectoryMember
	for offset := 0; ; offset += serverSnapshotUserPageSize {
		members, total, err := a.core.GetServerMembers(ctx, "", serverSnapshotUserPageSize, offset)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			user, err := a.core.GetUser(ctx, member.UserID)
			if err != nil {
				if errors.Is(err, core.ErrNotFound) {
					continue
				}
				return nil, err
			}
			apiMember, err := directoryMember(ctx, a, user, member.Roles)
			if err != nil {
				return nil, err
			}
			out = append(out, apiMember)
		}
		if offset+len(members) >= total || len(members) == 0 {
			return out, nil
		}
	}
}

func (a *API) serverSnapshotRoomGroups(ctx context.Context, userID string) ([]*apiv1.RoomGroup, error) {
	groups, err := a.core.RoomDirectoryReads().ListRoomGroups(ctx, userID, core.RoomDirectoryGroupOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]*apiv1.RoomGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, apiRoomGroup(group))
	}
	return out, nil
}

func (a *API) serverSnapshotNotifications(ctx context.Context, userID string) (*apiv1.ListNotificationOccurrencesResponse, error) {
	if err := a.core.NotificationOccurrences().WaitCurrent(ctx); err != nil {
		return nil, err
	}
	occurrences, err := a.core.NotificationOccurrences().List(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := requireSupportedNotificationSignals(occurrences...); err != nil {
		return nil, err
	}
	occurrences, err = (&notificationService{api: a}).visibleNotificationOccurrences(ctx, userID, occurrences)
	if err != nil {
		return nil, err
	}
	total := len(occurrences)
	page := occurrences[:min(total, defaultNotificationLimit)]
	summary := notificationSummary(occurrences)
	hydrated, err := newNotificationAssembler(a).occurrences(ctx, page)
	if err != nil {
		return nil, err
	}
	return &apiv1.ListNotificationOccurrencesResponse{
		Occurrences:          hydrated,
		Page:                 apiPageInfo(total, total > len(page)),
		UnreadCount:          summary.unreadCount,
		NextExpiryAt:         summary.nextExpiryAt,
		RoomUnreadCounts:     summary.roomCounts,
		ImportantUnreadCount: summary.importantUnreadCount,
	}, nil
}

func (a *API) serverSnapshotActiveCalls(ctx context.Context, userID string) ([]*apiv1.ActiveCall, error) {
	if !a.config.LiveKit.IsConfigured() {
		return nil, nil
	}
	roomIDs, err := a.core.GetActiveCallRoomIDs(ctx)
	if err != nil {
		return nil, err
	}
	calls := make([]*apiv1.ActiveCall, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		call, err := activeCall(ctx, a, userID, roomID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrPermissionDenied) || errors.Is(err, core.ErrNotRoomMember) {
				continue
			}
			return nil, err
		}
		calls = append(calls, call)
	}
	return calls, nil
}
