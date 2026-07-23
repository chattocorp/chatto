package connectapi

import (
	"context"
	"errors"
	"sort"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	defaultUserDirectoryLimit       = 20
	defaultRoomMemberDirectoryLimit = 250
	maxMemberDirectoryLimit         = 500
)

func userDirectoryPagination(page *apiv1.PageRequest) (int, int) {
	return apiPagination(page, defaultUserDirectoryLimit, maxMemberDirectoryLimit)
}

func roomMemberDirectoryPagination(page *apiv1.PageRequest) (int, int) {
	return apiPagination(page, defaultRoomMemberDirectoryLimit, maxMemberDirectoryLimit)
}

func (s *userService) ListUsers(ctx context.Context, req *connect.Request[apiv1.ListUsersRequest]) (*connect.Response[apiv1.ListUsersResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	limit, offset := userDirectoryPagination(req.Msg.GetPage())
	members, totalCount, err := s.api.core.GetServerMembers(ctx, req.Msg.GetSearch(), limit, offset)
	if err != nil {
		return nil, connectError(err)
	}

	out := make([]*apiv1.DirectoryMember, 0, len(members))
	skipped := 0
	for _, member := range members {
		user, err := s.api.core.GetUser(ctx, member.UserID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				skipped++
				continue
			}
			return nil, connectError(err)
		}
		apiMember, err := directoryMember(ctx, s.api, user, member.Roles)
		if err != nil {
			return nil, err
		}
		out = append(out, apiMember)
	}

	visibleTotalCount := totalCount - skipped
	if visibleTotalCount < len(out) {
		visibleTotalCount = len(out)
	}
	return connect.NewResponse(&apiv1.ListUsersResponse{
		Users: out,
		Page:  apiPageInfo(visibleTotalCount, offset+len(out) < visibleTotalCount),
	}), nil
}

func (s *userService) GetUser(ctx context.Context, req *connect.Request[apiv1.GetUserRequest]) (*connect.Response[apiv1.GetUserResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	var user *corev1.User
	var err error
	switch req.Msg.GetTarget().(type) {
	case *apiv1.GetUserRequest_UserId:
		user, err = s.api.core.GetUser(ctx, req.Msg.GetUserId())
	case *apiv1.GetUserRequest_Login:
		user, err = s.api.core.GetUserByLogin(ctx, req.Msg.GetLogin())
	default:
		return nil, invalidArgument("user_id or login is required")
	}
	if err != nil {
		return nil, connectError(err)
	}
	member, err := serverMemberForUser(ctx, s.api, user)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetUserResponse{User: member}), nil
}

func (s *userService) BatchGetUsers(ctx context.Context, req *connect.Request[apiv1.BatchGetUsersRequest]) (*connect.Response[apiv1.BatchGetUsersResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(req.Msg.GetUserIds()))
	members := make([]*apiv1.DirectoryMember, 0, len(req.Msg.GetUserIds()))
	for _, userID := range req.Msg.GetUserIds() {
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}

		member, err := serverMember(ctx, s.api, userID)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				continue
			}
			return nil, err
		}
		members = append(members, member)
	}
	return connect.NewResponse(&apiv1.BatchGetUsersResponse{Users: members}), nil
}

func (s *roomService) ListMembers(ctx context.Context, req *connect.Request[apiv1.ListRoomMembersRequest]) (*connect.Response[apiv1.ListRoomMembersResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	users, err := s.api.core.ListRoomMemberReferencesForList(ctx, caller.UserID, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}

	query := strings.ToLower(strings.TrimSpace(req.Msg.GetSearch()))
	if query != "" {
		filtered := users[:0]
		for _, user := range users {
			if strings.Contains(strings.ToLower(user.GetLogin()), query) ||
				strings.Contains(strings.ToLower(user.GetDisplayName()), query) {
				filtered = append(filtered, user)
			}
		}
		users = filtered
	}

	sort.Slice(users, func(i, j int) bool {
		left := strings.ToLower(users[i].GetDisplayName())
		right := strings.ToLower(users[j].GetDisplayName())
		if left == right {
			return strings.ToLower(users[i].GetLogin()) < strings.ToLower(users[j].GetLogin())
		}
		return left < right
	})

	limit, offset := roomMemberDirectoryPagination(req.Msg.GetPage())
	page, totalCount, hasMore := paginateDirectoryUsers(users, limit, offset)
	userIDs := make([]string, len(page))
	for i, user := range page {
		userIDs[i] = user.GetId()
	}
	presences, err := s.api.core.GetUserPresences(ctx, userIDs)
	if err != nil {
		return nil, connectError(err)
	}
	out := make([]*apiv1.DirectoryMember, 0, len(page))
	for _, user := range page {
		apiMember, err := directoryMemberWithPresence(ctx, s.api, user, nil, presences[user.GetId()])
		if err != nil {
			return nil, err
		}
		out = append(out, apiMember)
	}

	return connect.NewResponse(&apiv1.ListRoomMembersResponse{
		Members: out,
		Page:    apiPageInfo(totalCount, hasMore),
	}), nil
}

func (s *roomService) GetMember(ctx context.Context, req *connect.Request[apiv1.GetRoomMemberRequest]) (*connect.Response[apiv1.GetRoomMemberResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	users, err := s.api.core.ListRoomMemberReferencesForLookup(ctx, caller.UserID, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	user := findCoreUserByID(users, req.Msg.GetUserId())
	if user == nil {
		return nil, connectError(core.ErrNotFound)
	}
	member, err := directoryMember(ctx, s.api, user, nil)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetRoomMemberResponse{Member: member}), nil
}

func (s *roomService) BatchGetMembers(ctx context.Context, req *connect.Request[apiv1.BatchGetRoomMembersRequest]) (*connect.Response[apiv1.BatchGetRoomMembersResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	users, err := s.api.core.ListRoomMemberReferencesForLookup(ctx, caller.UserID, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	usersByID := make(map[string]*corev1.User, len(users))
	for _, user := range users {
		usersByID[user.GetId()] = user
	}

	seen := make(map[string]struct{}, len(req.Msg.GetUserIds()))
	members := make([]*apiv1.DirectoryMember, 0, len(req.Msg.GetUserIds()))
	for _, userID := range req.Msg.GetUserIds() {
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}

		user := usersByID[userID]
		if user == nil {
			continue
		}
		member, err := directoryMember(ctx, s.api, user, nil)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return connect.NewResponse(&apiv1.BatchGetRoomMembersResponse{Members: members}), nil
}

func serverMember(ctx context.Context, api *API, userID string) (*apiv1.DirectoryMember, error) {
	user, err := api.core.GetUser(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	return serverMemberForUser(ctx, api, user)
}

func serverMemberForUser(ctx context.Context, api *API, user *corev1.User) (*apiv1.DirectoryMember, error) {
	assigned, err := api.core.GetUserRoles(ctx, user.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	roles := append([]string{core.RoleEveryone}, assigned...)
	return directoryMember(ctx, api, user, roles)
}

func directoryMember(ctx context.Context, api *API, user *corev1.User, roles []string) (*apiv1.DirectoryMember, error) {
	presence, err := api.core.GetUserPresence(ctx, user.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	return directoryMemberWithPresence(ctx, api, user, roles, presence)
}

func directoryMemberWithPresence(ctx context.Context, api *API, user *corev1.User, roles []string, presence string) (*apiv1.DirectoryMember, error) {
	avatarSize := 96
	avatar := &apiv1.ImageTransformOptions{
		Width:  int32(avatarSize),
		Height: int32(avatarSize),
		Fit:    apiv1.ImageFitMode_IMAGE_FIT_MODE_COVER,
	}
	apiUser, err := userSummaryWithPresence(ctx, api, user, avatar, presence)
	if err != nil {
		return nil, err
	}
	member := &apiv1.DirectoryMember{
		User:      apiUser,
		Roles:     append([]string(nil), roles...),
		CreatedAt: user.GetCreatedAt(),
	}

	return member, nil
}

func findCoreUserByID(users []*corev1.User, userID string) *corev1.User {
	for _, user := range users {
		if user.GetId() == userID {
			return user
		}
	}
	return nil
}

func paginateDirectoryUsers(users []*corev1.User, limit, offset int) ([]*corev1.User, int, bool) {
	total := len(users)
	if offset >= total {
		return []*corev1.User{}, total, false
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return users[offset:end], total, end < total
}
