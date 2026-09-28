// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package connectapi

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// realtimeSnapshotUserHydrationConcurrency bounds the concurrent key-store and
// KMS reads that one snapshot uses to decrypt referenced user profiles.
const realtimeSnapshotUserHydrationConcurrency = 16

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

type realtimeSnapshotUserCapture struct {
	snapshot *core.UserContentSnapshot
	roles    []string
}

type realtimeSnapshotCallCapture struct {
	room     *evtv1.Room
	snapshot core.CallRoomSnapshot
}

type realtimeSnapshotCapture struct {
	sequence    uint64
	server      *apiv1.ServerPublicProfile
	rooms       *apiv1.ListRoomsResponse
	roomGroups  *apiv1.ListRoomGroupsResponse
	users       map[string]realtimeSnapshotUserCapture
	activeCalls []realtimeSnapshotCallCapture
}

// BuildRealtimeSnapshot creates one authorized resource snapshot at an exact
// EVT boundary. It does not include the complete user directory.
func (a *API) BuildRealtimeSnapshot(ctx context.Context, userID string) (*RealtimeSnapshotResources, error) {
	ctx = core.WithDEKRequestCache(ctx)
	capture := &realtimeSnapshotCapture{}
	err := a.core.ReadServerContentView(ctx, func(readCtx context.Context, sequence uint64) error {
		capture.sequence = sequence

		server, err := a.serverProfile(readCtx, serverProfileOptions{})
		if err != nil {
			return fmt.Errorf("assemble server profile: %w", err)
		}
		capture.server = server

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
			apiRoom, err := a.realtimeSnapshotRoom(readCtx, userID, room)
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
		capture.rooms = &apiv1.ListRoomsResponse{Rooms: apiRooms}

		groups, err := a.core.RoomDirectoryReads().ListRoomGroups(readCtx, userID, core.RoomDirectoryGroupOptions{})
		if err != nil {
			return fmt.Errorf("assemble room groups: %w", err)
		}
		apiGroups := make([]*apiv1.RoomGroup, 0, len(groups))
		for _, group := range groups {
			apiGroups = append(apiGroups, apiRoomGroup(group))
		}
		capture.roomGroups = &apiv1.ListRoomGroupsResponse{Groups: apiGroups}

		calls, err := a.captureRealtimeSnapshotActiveCalls(readCtx, userID)
		if err != nil {
			return fmt.Errorf("assemble active calls: %w", err)
		}
		capture.activeCalls = calls
		for _, call := range calls {
			for _, participant := range call.snapshot.Participants {
				if participant.UserID != "" {
					referencedUserIDs[participant.UserID] = struct{}{}
				}
			}
		}

		userIDs := make([]string, 0, len(referencedUserIDs))
		for referencedUserID := range referencedUserIDs {
			userIDs = append(userIDs, referencedUserID)
		}
		sort.Strings(userIDs)
		capture.users = make(map[string]realtimeSnapshotUserCapture, len(userIDs))
		for _, referencedUserID := range userIDs {
			userSnapshot := a.core.CaptureUserContentSnapshot(referencedUserID)
			if userSnapshot == nil {
				continue
			}
			roles, err := a.core.GetUserRoles(readCtx, referencedUserID)
			if err != nil {
				return fmt.Errorf("assemble referenced user roles: %w", err)
			}
			capture.users[referencedUserID] = realtimeSnapshotUserCapture{
				snapshot: userSnapshot,
				roles:    append([]string(nil), roles...),
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	users, apiUsers, err := a.hydrateRealtimeSnapshotUsers(ctx, capture.users)
	if err != nil {
		return nil, err
	}
	filterSnapshotRoomMembers(capture.rooms, apiUsers)
	return &RealtimeSnapshotResources{
		Sequence:    capture.sequence,
		Server:      capture.server,
		Rooms:       capture.rooms,
		RoomGroups:  capture.roomGroups,
		Users:       &apiv1.BatchGetUsersResponse{Users: users},
		ActiveCalls: &apiv1.ListActiveCallsResponse{Calls: materializeRealtimeSnapshotCalls(capture.activeCalls, apiUsers)},
	}, nil
}

func (a *API) realtimeSnapshotRoom(ctx context.Context, userID string, room *core.DirectoryRoom) (*apiv1.RoomWithViewerState, error) {
	result := apiRoomWithViewerState(room)
	if room == nil || room.Room == nil || core.KindOfRoom(room.Room) != core.KindDM || !room.ViewerState.IsMember {
		return result, nil
	}
	exists := false
	if room.ViewerState.CanReadMessages {
		_, _, historyExists, err := a.core.GetRoomLastEvent(ctx, core.KindDM, room.Room.GetId())
		if err != nil {
			return nil, err
		}
		exists = historyExists
	}
	result.HasMessageHistory = &exists
	var err error
	result.MemberUserIds, err = a.core.ListRoomMemberIDsForList(ctx, userID, room.Room.GetId())
	return result, err
}

func (a *API) captureRealtimeSnapshotActiveCalls(ctx context.Context, userID string) ([]realtimeSnapshotCallCapture, error) {
	if !a.config.LiveKit.IsConfigured() {
		return nil, nil
	}
	roomIDs, err := a.core.GetActiveCallRoomIDs(ctx)
	if err != nil {
		return nil, err
	}
	calls := make([]realtimeSnapshotCallCapture, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		room, _, err := a.core.VoiceCallRoomForMember(ctx, userID, roomID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrPermissionDenied) || errors.Is(err, core.ErrNotRoomMember) {
				continue
			}
			return nil, err
		}
		snapshot, err := a.core.GetCallSnapshot(room.GetId())
		if err != nil {
			return nil, err
		}
		if snapshot.Call.CallID == "" {
			continue
		}
		calls = append(calls, realtimeSnapshotCallCapture{room: room, snapshot: snapshot})
	}
	return calls, nil
}

func (a *API) hydrateRealtimeSnapshotUsers(ctx context.Context, captures map[string]realtimeSnapshotUserCapture) ([]*apiv1.DirectoryMember, map[string]*apiv1.User, error) {
	userIDs := make([]string, 0, len(captures))
	for userID := range captures {
		userIDs = append(userIDs, userID)
	}
	sort.Strings(userIDs)
	// Each user has its own data-encryption key, and resolving it reads the key
	// store and the KMS. Hydrate users concurrently so the snapshot does not pay
	// these round trips once per referenced user.
	hydrated := make([]*apiv1.DirectoryMember, len(userIDs))
	hydration, hydrationCtx := errgroup.WithContext(ctx)
	hydration.SetLimit(realtimeSnapshotUserHydrationConcurrency)
	for i, userID := range userIDs {
		capture := captures[userID]
		hydration.Go(func() error {
			if capture.snapshot.Deleted() {
				// Deleted accounts stay DM participants. Send a tombstone so
				// clients show a deleted user rather than dropping the ID.
				hydrated[i] = &apiv1.DirectoryMember{User: &apiv1.User{
					Id:          userID,
					DisplayName: core.DeletedUserDisplayName,
					Deleted:     true,
				}}
				return nil
			}
			content, ok, err := a.core.HydrateUserContentSnapshot(hydrationCtx, capture.snapshot)
			if err != nil {
				return fmt.Errorf("hydrate referenced user: %w", err)
			}
			if !ok || content == nil || content.User == nil {
				return nil
			}
			apiUser, err := a.realtimeSnapshotUser(hydrationCtx, content)
			if err != nil {
				return fmt.Errorf("assemble referenced user profile: %w", err)
			}
			hydrated[i] = &apiv1.DirectoryMember{
				User:      apiUser,
				Roles:     capture.roles,
				CreatedAt: content.User.GetCreatedAt(),
			}
			return nil
		})
	}
	if err := hydration.Wait(); err != nil {
		return nil, nil, err
	}
	members := make([]*apiv1.DirectoryMember, 0, len(userIDs))
	users := make(map[string]*apiv1.User, len(userIDs))
	for i, member := range hydrated {
		if member == nil {
			continue
		}
		users[userIDs[i]] = member.GetUser()
		members = append(members, member)
	}
	return members, users, nil
}

func (a *API) realtimeSnapshotUser(ctx context.Context, content *core.HydratedUserContent) (*apiv1.User, error) {
	user := content.User
	summary := &apiv1.User{
		Id:             user.GetId(),
		Login:          user.GetLogin(),
		DisplayName:    user.GetDisplayName(),
		Deleted:        user.GetDeleted(),
		PresenceStatus: apiv1.PresenceStatus_PRESENCE_STATUS_UNSPECIFIED,
		CustomStatus:   coreCustomStatusToAPI(user.GetCustomStatus()),
	}
	if user.GetIsBot() && !user.GetDeleted() {
		summary.Bot = &apiv1.BotInfo{OwnerUserId: user.GetBotOwnerUserId()}
	}
	if user.GetBio() != "" {
		bio := user.GetBio()
		summary.Bio = &bio
	}
	if preferences := content.Preferences; preferences != nil && preferences.GetShareTimezone() && preferences.GetTimezone() != "" {
		timezone := preferences.GetTimezone()
		summary.Timezone = &timezone
	}
	if content.Avatar != nil {
		assetKey := core.ServerAssetDeliveryKey(content.Avatar)
		if assetKey == "" {
			return nil, fmt.Errorf("unknown avatar asset type")
		}
		avatarURL := a.core.GetTransformedServerAssetURL(assetKey, 96, 96, "cover")
		avatarURL = a.absolutizeServerURL(ctx, avatarURL)
		summary.AvatarUrl = &avatarURL
	}
	return summary, nil
}

func filterSnapshotRoomMembers(rooms *apiv1.ListRoomsResponse, users map[string]*apiv1.User) {
	for _, room := range rooms.GetRooms() {
		memberIDs := room.GetMemberUserIds()
		visible := make([]string, 0, len(memberIDs))
		for _, userID := range memberIDs {
			if users[userID] != nil {
				visible = append(visible, userID)
			}
		}
		room.MemberUserIds = visible
	}
}

func materializeRealtimeSnapshotCalls(captures []realtimeSnapshotCallCapture, users map[string]*apiv1.User) []*apiv1.ActiveCall {
	calls := make([]*apiv1.ActiveCall, 0, len(captures))
	for _, capture := range captures {
		participants := make([]*apiv1.CallParticipant, 0, len(capture.snapshot.Participants))
		for _, participant := range capture.snapshot.Participants {
			user := users[participant.UserID]
			if user == nil {
				continue
			}
			participants = append(participants, &apiv1.CallParticipant{
				User:     user,
				JoinedAt: timestamppb.New(time.Unix(participant.JoinedAt, 0)),
				CallId:   participant.CallID,
			})
		}
		calls = append(calls, &apiv1.ActiveCall{
			Room:         apiRoomSummary(capture.room),
			CallId:       capture.snapshot.Call.CallID,
			Participants: participants,
		})
	}
	return calls
}
