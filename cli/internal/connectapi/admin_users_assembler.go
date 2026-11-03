package connectapi

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// assembleAdminUser retains a direct presence read for singular account reads.
func (s *adminUserManagementService) assembleAdminUser(ctx context.Context, user core.AdminMember) (*adminv1.AdminUser, error) {
	presence, err := s.api.core.GetUserPresence(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return s.adminUserWithPresence(ctx, user, presence), nil
}

// assembleAdminUsers reads one shared presence snapshot for the complete batch.
func (s *adminUserManagementService) assembleAdminUsers(ctx context.Context, users []core.AdminMember) (*adminv1.BatchGetUsersResponse, error) {
	ids := make([]string, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	presences, err := s.api.core.GetUserPresences(ctx, ids)
	if err != nil {
		return nil, err
	}
	response := &adminv1.BatchGetUsersResponse{Users: make([]*adminv1.AdminUser, 0, len(users))}
	for _, user := range users {
		response.Users = append(response.Users, s.adminUserWithPresence(ctx, user, presences[user.ID]))
	}
	return response, nil
}

func (s *adminUserManagementService) adminUserWithPresence(ctx context.Context, user core.AdminMember, presence string) *adminv1.AdminUser {
	profile := adminMemberUserWithPresence(user, presence)
	if user.AvatarURL != "" {
		profile.AvatarUrl = new(s.api.absolutizeServerURL(ctx, user.AvatarURL))
	}
	result := &adminv1.AdminUser{
		Profile: profile, RoleNames: append([]string{}, user.Roles...), CreatedAt: user.CreatedAt,
		VerifiedEmails: append([]string{}, user.VerifiedEmails...),
	}
	if user.PrimaryVerifiedEmail != "" {
		result.PrimaryVerifiedEmail = new(user.PrimaryVerifiedEmail)
	}
	if user.UsernameCooldownVisible {
		result.UsernameCooldown = &adminv1.AdminUsernameCooldown{}
		if user.LastLoginChange != nil {
			result.UsernameCooldown.LastLoginChange = timestamppb.New(*user.LastLoginChange)
		}
	}
	return result
}
