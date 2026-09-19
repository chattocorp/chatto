package connectapi

import (
	"context"

	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

// assembleAdminMembers reads the shared presence snapshot once per batch.
// Singular reads and mutation responses retain their authoritative KV reads.
func (s *adminUserManagementService) assembleAdminMembers(ctx context.Context, members *core.AdminMemberList) (*adminv1.BatchGetMembersResponse, error) {
	ids := make([]string, 0, len(members.Users))
	for _, member := range members.Users {
		ids = append(ids, member.ID)
	}
	presences, err := s.api.core.GetUserPresences(ctx, ids)
	if err != nil {
		return nil, err
	}
	response := &adminv1.BatchGetMembersResponse{
		Members: make([]*adminv1.AdminMember, 0, len(members.Users)),
		Roles:   make([]*apiv1.Role, 0, len(members.Roles)),
	}
	for _, member := range members.Users {
		response.Members = append(response.Members, s.adminMemberWithPresence(ctx, member, presences[member.ID]))
	}
	for _, role := range members.Roles {
		response.Roles = append(response.Roles, publicAPIRoleFromAdminMemberSummary(role))
	}
	return response, nil
}
