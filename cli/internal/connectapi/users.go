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
	profile, err := s.userPresenceSummary(ctx, user, req.Msg.GetAvatar())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetUserResponse{User: profile}), nil
}

func (s *userService) BatchGetUsers(ctx context.Context, req *connect.Request[apiv1.BatchGetUsersRequest]) (*connect.Response[apiv1.BatchGetUsersResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(req.Msg.GetUserIds()))
	users := make([]*apiv1.UserProfile, 0, len(req.Msg.GetUserIds()))
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
		summary, err := s.userPresenceSummary(ctx, user, req.Msg.GetAvatar())
		if err != nil {
			return nil, err
		}
		users = append(users, summary)
	}

	return connect.NewResponse(&apiv1.BatchGetUsersResponse{Users: users}), nil
}

func (s *userService) userPresenceSummary(ctx context.Context, user *corev1.User, avatar *apiv1.UserAvatarOptions) (*apiv1.UserProfile, error) {
	summary, err := s.userSummary(ctx, user, avatar)
	if err != nil {
		return nil, err
	}
	presence, err := s.api.core.GetUserPresence(ctx, user.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	return &apiv1.UserProfile{
		User:           summary,
		PresenceStatus: corePresenceStatusToAPI(presence),
		CustomStatus:   coreCustomStatusToAPI(user.GetCustomStatus()),
	}, nil
}

func (s *userService) userSummary(ctx context.Context, user *corev1.User, avatar *apiv1.UserAvatarOptions) (*apiv1.User, error) {
	summary := &apiv1.User{
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
		summary.AvatarUrl = stringPtr(s.api.absolutizeAssetURL(ctx, avatarURL))
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
