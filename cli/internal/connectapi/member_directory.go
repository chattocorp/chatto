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
	defaultMemberDirectoryLimit = 20
	maxMemberDirectoryLimit     = 500
)

type serverMemberService struct {
	api *API
}

type roomMemberService struct {
	api *API
}

func (s *serverMemberService) ListMembers(ctx context.Context, req *connect.Request[apiv1.ListServerMembersRequest]) (*connect.Response[apiv1.ListServerMembersResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	limit, offset := apiPagination(req.Msg.GetPage(), defaultMemberDirectoryLimit, maxMemberDirectoryLimit)
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
	return connect.NewResponse(&apiv1.ListServerMembersResponse{
		Members: out,
		Page:    apiPageInfo(visibleTotalCount, offset+len(out) < visibleTotalCount),
	}), nil
}

func (s *serverMemberService) GetMember(ctx context.Context, req *connect.Request[apiv1.GetServerMemberRequest]) (*connect.Response[apiv1.GetServerMemberResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	member, err := serverMember(ctx, s.api, req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetServerMemberResponse{Member: member}), nil
}

func (s *serverMemberService) BatchGetMembers(ctx context.Context, req *connect.Request[apiv1.BatchGetServerMembersRequest]) (*connect.Response[apiv1.BatchGetServerMembersResponse], error) {
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
	return connect.NewResponse(&apiv1.BatchGetServerMembersResponse{Members: members}), nil
}

func (s *roomMemberService) ListMembers(ctx context.Context, req *connect.Request[apiv1.ListRoomMembersRequest]) (*connect.Response[apiv1.ListRoomMembersResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	users, err := s.api.core.ListRoomMemberReferences(ctx, caller.UserID, req.Msg.GetRoomId())
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

	limit, offset := apiPagination(req.Msg.GetPage(), defaultMemberDirectoryLimit, maxMemberDirectoryLimit)
	page, totalCount, hasMore := paginateDirectoryUsers(users, limit, offset)
	out := make([]*apiv1.DirectoryMember, 0, len(page))
	for _, user := range page {
		apiMember, err := directoryMember(ctx, s.api, user, nil)
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

func (s *roomMemberService) GetMember(ctx context.Context, req *connect.Request[apiv1.GetRoomMemberRequest]) (*connect.Response[apiv1.GetRoomMemberResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	users, err := s.api.core.ListRoomMemberReferences(ctx, caller.UserID, req.Msg.GetRoomId())
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

func (s *roomMemberService) BatchGetMembers(ctx context.Context, req *connect.Request[apiv1.BatchGetRoomMembersRequest]) (*connect.Response[apiv1.BatchGetRoomMembersResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	users, err := s.api.core.ListRoomMemberReferences(ctx, caller.UserID, req.Msg.GetRoomId())
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
	assigned, err := api.core.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	roles := append([]string{core.RoleEveryone}, assigned...)
	return directoryMember(ctx, api, user, roles)
}

func directoryMember(ctx context.Context, api *API, user *corev1.User, roles []string) (*apiv1.DirectoryMember, error) {
	avatarSize := 96
	avatar := &apiv1.UserAvatarOptions{
		Width:  int32(avatarSize),
		Height: int32(avatarSize),
		Fit:    apiv1.UserAvatarFitMode_USER_AVATAR_FIT_MODE_COVER,
	}
	profile, err := (&userService{api: api}).userPresenceSummary(ctx, user, avatar)
	if err != nil {
		return nil, err
	}
	member := &apiv1.DirectoryMember{
		Profile:   profile,
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
