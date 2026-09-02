// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package connectapi

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

// RealtimeSnapshotResources is the finite public resource set captured from
// one exact ServerContentView generation. Users contains only accounts that
// the other snapshot resources reference. Timelines and other paginated data
// remain explicit ConnectRPC resources.
type RealtimeSnapshotResources struct {
	Sequence    uint64
	Server      *apiv1.ServerPublicProfile
	Rooms       *apiv1.ListRoomsResponse
	RoomGroups  *apiv1.ListRoomGroupsResponse
	Users       *apiv1.BatchGetUsersResponse
	ActiveCalls *apiv1.ListActiveCallsResponse
}

// BuildRealtimeSnapshot creates one authorized resource snapshot at an exact
// EVT boundary. It does not include the complete user directory.
func (a *API) BuildRealtimeSnapshot(ctx context.Context, userID string) (*RealtimeSnapshotResources, error) {
	ctx = core.WithDEKRequestCache(ctx)
	result := &RealtimeSnapshotResources{}
	err := a.core.ReadServerContentView(ctx, func(readCtx context.Context, sequence uint64) error {
		result.Sequence = sequence

		server, err := a.serverProfile(readCtx, serverProfileOptions{})
		if err != nil {
			return fmt.Errorf("assemble server profile: %w", err)
		}
		result.Server = server

		rooms, err := a.core.RoomDirectoryReads().ListRooms(readCtx, userID, core.RoomDirectoryListOptions{
			IncludeChannels: true,
			IncludeDMs:      true,
			IncludeEmptyDMs: true,
		})
		if err != nil {
			return fmt.Errorf("assemble rooms: %w", err)
		}
		apiRooms := make([]*apiv1.RoomWithViewerState, 0, len(rooms))
		referencedUserIDs := map[string]struct{}{userID: {}}
		for _, room := range rooms {
			apiRoom, err := a.apiRoomWithViewerState(readCtx, userID, room)
			if err != nil {
				return fmt.Errorf("assemble room %q: %w", room.Room.GetId(), err)
			}
			apiRooms = append(apiRooms, apiRoom)
			for _, memberUserID := range apiRoom.GetMemberUserIds() {
				if memberUserID != "" {
					referencedUserIDs[memberUserID] = struct{}{}
				}
			}
		}
		result.Rooms = &apiv1.ListRoomsResponse{Rooms: apiRooms}

		groups, err := a.core.RoomDirectoryReads().ListRoomGroups(readCtx, userID, core.RoomDirectoryGroupOptions{})
		if err != nil {
			return fmt.Errorf("assemble room groups: %w", err)
		}
		apiGroups := make([]*apiv1.RoomGroup, 0, len(groups))
		for _, group := range groups {
			apiGroups = append(apiGroups, apiRoomGroup(group))
		}
		result.RoomGroups = &apiv1.ListRoomGroupsResponse{Groups: apiGroups}

		calls, err := a.realtimeSnapshotActiveCalls(readCtx, userID)
		if err != nil {
			return fmt.Errorf("assemble active calls: %w", err)
		}
		result.ActiveCalls = &apiv1.ListActiveCallsResponse{Calls: calls}

		userIDs := make([]string, 0, len(referencedUserIDs))
		for referencedUserID := range referencedUserIDs {
			userIDs = append(userIDs, referencedUserID)
		}
		sort.Strings(userIDs)
		members := make([]*apiv1.DirectoryMember, 0, len(userIDs))
		for _, referencedUserID := range userIDs {
			user, err := a.core.GetUser(readCtx, referencedUserID)
			if err != nil {
				if errors.Is(err, core.ErrNotFound) {
					continue
				}
				return fmt.Errorf("assemble referenced user: %w", err)
			}
			roles, err := a.core.GetUserRoles(readCtx, referencedUserID)
			if err != nil {
				return fmt.Errorf("assemble referenced user roles: %w", err)
			}
			member, err := directoryMemberWithPresence(readCtx, a, user, roles, "")
			if err != nil {
				return fmt.Errorf("assemble referenced user profile: %w", err)
			}
			members = append(members, member)
		}
		result.Users = &apiv1.BatchGetUsersResponse{Users: members}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (a *API) realtimeSnapshotActiveCalls(ctx context.Context, userID string) ([]*apiv1.ActiveCall, error) {
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
