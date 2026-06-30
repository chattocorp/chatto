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

type memberDirectoryService struct {
	api *API
}

func (s *memberDirectoryService) ListServerMembers(ctx context.Context, req *connect.Request[apiv1.ListServerMembersRequest]) (*connect.Response[apiv1.ListServerMembersResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	limit, offset := apiPagination(req.Msg.GetPage(), defaultMemberDirectoryLimit, maxMemberDirectoryLimit)
	members, totalCount, err := s.api.core.GetServerMembers(ctx, req.Msg.GetSearch(), limit, offset)
	if err != nil {
		return nil, connectError(err)
	}

	out := make([]*apiv1.DirectoryMember, 0, len(members))
	for _, member := range members {
		user, err := s.api.core.GetUser(ctx, member.UserID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				continue
			}
			return nil, connectError(err)
		}
		apiMember, err := s.directoryMember(ctx, user, member.Roles)
		if err != nil {
			return nil, err
		}
		out = append(out, apiMember)
	}

	return connect.NewResponse(&apiv1.ListServerMembersResponse{
		Members: out,
		Page:    apiPageInfo(totalCount, offset+len(out) < totalCount),
	}), nil
}

func (s *memberDirectoryService) ListRoomMembers(ctx context.Context, req *connect.Request[apiv1.ListRoomMembersRequest]) (*connect.Response[apiv1.ListRoomMembersResponse], error) {
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
		apiMember, err := s.directoryMember(ctx, user, nil)
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

func (s *memberDirectoryService) directoryMember(ctx context.Context, user *corev1.User, roles []string) (*apiv1.DirectoryMember, error) {
	avatarSize := 96
	avatar := &apiv1.UserAvatarOptions{
		Width:  int32(avatarSize),
		Height: int32(avatarSize),
		Fit:    apiv1.UserAvatarFitMode_USER_AVATAR_FIT_MODE_COVER,
	}
	profile, err := (&userService{api: s.api}).userPresenceSummary(ctx, user, avatar)
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
