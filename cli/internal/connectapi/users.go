package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type userService struct {
	api *API
}

func (s *userService) GetUser(ctx context.Context, req *connect.Request[apiv1.GetUserRequest]) (*connect.Response[apiv1.GetUserResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	user, err := s.api.core.GetUser(ctx, req.Msg.GetUserId())
	if err != nil {
		return nil, connectError(err)
	}
	profile, err := s.userProfile(ctx, user, req.Msg.GetAvatar())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetUserResponse{User: profile}), nil
}

func (s *userService) GetUserByLogin(ctx context.Context, req *connect.Request[apiv1.GetUserByLoginRequest]) (*connect.Response[apiv1.GetUserByLoginResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	user, err := s.api.core.GetUserByLogin(ctx, req.Msg.GetLogin())
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return connect.NewResponse(&apiv1.GetUserByLoginResponse{}), nil
		}
		return nil, connectError(err)
	}
	profile, err := s.userProfile(ctx, user, req.Msg.GetAvatar())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetUserByLoginResponse{User: profile}), nil
}

func (s *userService) BatchGetUsers(ctx context.Context, req *connect.Request[apiv1.BatchGetUsersRequest]) (*connect.Response[apiv1.BatchGetUsersResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(req.Msg.GetUserIds()))
	users := make([]*apiv1.UserSummary, 0, len(req.Msg.GetUserIds()))
	for _, userID := range req.Msg.GetUserIds() {
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}

		user, err := s.api.core.GetUser(ctx, userID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				continue
			}
			return nil, connectError(err)
		}
		summary, err := s.userSummary(ctx, user, req.Msg.GetAvatar())
		if err != nil {
			return nil, err
		}
		users = append(users, summary)
	}

	return connect.NewResponse(&apiv1.BatchGetUsersResponse{Users: users}), nil
}

func (s *userService) userProfile(ctx context.Context, user *corev1.User, avatar *apiv1.UserAvatarOptions) (*apiv1.DirectoryMember, error) {
	roles, err := s.api.core.GetUserRoles(ctx, user.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	member, err := (&memberDirectoryService{api: s.api}).directoryMember(ctx, user, roles)
	if err != nil {
		return nil, err
	}

	avatarURL, err := s.userAvatarURL(ctx, user.GetId(), avatar)
	if err != nil {
		return nil, err
	}
	if avatarURL != "" {
		member.AvatarUrl = stringPtr(s.api.absolutizeAssetURL(ctx, avatarURL))
	}
	return member, nil
}

func (s *userService) userSummary(ctx context.Context, user *corev1.User, avatar *apiv1.UserAvatarOptions) (*apiv1.UserSummary, error) {
	summary := &apiv1.UserSummary{
		Id:          user.GetId(),
		Login:       user.GetLogin(),
		DisplayName: user.GetDisplayName(),
		Deleted:     user.GetDeleted(),
	}
	avatarURL, err := s.userAvatarURL(ctx, user.GetId(), avatar)
	if err != nil {
		return nil, err
	}
	if avatarURL != "" {
		summary.AvatarUrl = s.api.absolutizeAssetURL(ctx, avatarURL)
	}
	return summary, nil
}

func (s *userService) userAvatarURL(ctx context.Context, userID string, avatar *apiv1.UserAvatarOptions) (string, error) {
	if avatar == nil {
		url, err := s.api.core.GetUserAvatarURL(ctx, userID, nil, nil, "")
		if err != nil {
			return "", connectError(err)
		}
		return url, nil
	}

	width, height := int(avatar.GetWidth()), int(avatar.GetHeight())
	fit := "cover"
	if avatar.GetFit() == apiv1.UserAvatarFitMode_USER_AVATAR_FIT_MODE_CONTAIN {
		fit = "contain"
	}
	url, err := s.api.core.GetUserAvatarURL(ctx, userID, &width, &height, fit)
	if err != nil {
		return "", connectError(err)
	}
	return url, nil
}
