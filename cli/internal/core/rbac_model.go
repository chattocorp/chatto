package core

import (
	"context"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RBACModel owns RBAC projection reads and readiness.
type RBACModel struct {
	rbac events.ProjectionHandle[*RBACProjection]
}

func newRBACModel(rbac events.ProjectionHandle[*RBACProjection]) *RBACModel {
	return &RBACModel{rbac: rbac}
}

func (m *RBACModel) waitFor(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("RBAC", m.rbac.Projector()))
}

func (m *RBACModel) role(name string) (*corev1.Role, bool) {
	return m.rbac.Projection().GetRole(name)
}

func (m *RBACModel) roleExists(name string) bool {
	return m.rbac.Projection().RoleExists(name)
}

func (m *RBACModel) roles() []*corev1.Role {
	return m.rbac.Projection().ListRoles()
}

func (m *RBACModel) userRoles(userID string) []string {
	return m.rbac.Projection().GetUserRoles(userID)
}

func (m *RBACModel) hasRole(userID, roleName string) bool {
	return m.rbac.Projection().HasRole(userID, roleName)
}

func (m *RBACModel) roleUsers(roleName string) []string {
	return m.rbac.Projection().GetRoleUsers(roleName)
}

func (m *RBACModel) rolePermissionDecisions(roleName string) []ScopedRolePermissionDecision {
	return m.rbac.Projection().RolePermissionDecisions(roleName)
}

func (m *RBACModel) decision(scope PermissionScope, scopeID, subject string, permission Permission) DecisionKind {
	return m.rbac.Projection().GetDecision(scope, scopeID, subject, permission)
}

func (m *RBACModel) decisionsFor(scope PermissionScope, scopeID, subject string) (grants, denials []Permission) {
	return m.rbac.Projection().DecisionsFor(scope, scopeID, subject)
}

func (m *RBACModel) decisionsForRoleServer(roleName string) (grants, denials []Permission) {
	return m.rbac.Projection().DecisionsForRoleServer(roleName)
}

func (m *RBACModel) nextAvailablePosition() int32 {
	return m.rbac.Projection().NextAvailablePosition()
}
