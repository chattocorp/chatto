package core

import (
	"context"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RBACModel owns RBAC projection reads and readiness.
type RBACModel struct {
	projection *RBACProjection
	projector  *events.Projector
}

func newRBACModel(projection *RBACProjection, projector *events.Projector) *RBACModel {
	return &RBACModel{projection: projection, projector: projector}
}

func (m *RBACModel) waitFor(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("RBAC", m.projector))
}

func (m *RBACModel) role(name string) (*corev1.Role, bool) {
	return m.projection.GetRole(name)
}

func (m *RBACModel) roleExists(name string) bool {
	return m.projection.RoleExists(name)
}

func (m *RBACModel) roles() []*corev1.Role {
	return m.projection.ListRoles()
}

func (m *RBACModel) userRoles(userID string) []string {
	return m.projection.GetUserRoles(userID)
}

func (m *RBACModel) hasRole(userID, roleName string) bool {
	return m.projection.HasRole(userID, roleName)
}

func (m *RBACModel) roleUsers(roleName string) []string {
	return m.projection.GetRoleUsers(roleName)
}

func (m *RBACModel) rolePermissionDecisions(roleName string) []ScopedRolePermissionDecision {
	return m.projection.RolePermissionDecisions(roleName)
}

func (m *RBACModel) decision(scope PermissionScope, scopeID, subject string, permission Permission) DecisionKind {
	return m.projection.GetDecision(scope, scopeID, subject, permission)
}

func (m *RBACModel) decisionsFor(scope PermissionScope, scopeID, subject string) (grants, denials []Permission) {
	return m.projection.DecisionsFor(scope, scopeID, subject)
}

func (m *RBACModel) decisionsForRoleServer(roleName string) (grants, denials []Permission) {
	return m.projection.DecisionsForRoleServer(roleName)
}

func (m *RBACModel) nextAvailablePosition() int32 {
	return m.projection.NextAvailablePosition()
}
