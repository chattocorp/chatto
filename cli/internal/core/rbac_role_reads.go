package core

import (
	"context"
)

// GetUserRoles returns the explicit role assignments for a user. The implicit
// `everyone` role is omitted — callers that need it can prepend it themselves
// based on the relevant scope (e.g. space membership).
func (m *RBACModel) GetUserRoles(ctx context.Context, userID string) ([]string, error) {
	assignedRoles := m.userRoles(userID)
	result := make([]string, 0, len(assignedRoles))
	for _, role := range assignedRoles {
		if role != RoleEveryone {
			result = append(result, role)
		}
	}

	return result, nil
}
