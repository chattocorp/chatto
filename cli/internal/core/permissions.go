package core

import (
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Server role names (system-defined, cannot be deleted).
//
// Phase 5 of #330 collapsed the dual-layer instance/space RBAC into a single
// server-shaped layer. The legacy `instance-` prefix is gone — the unified
// owner/admin/moderator roles cover what the two parallel layers used to.
const (
	// RoleOwner has all permissions and is the highest authority. Must be
	// explicitly assigned. Config-designated owners (matched against
	// `owners.emails` in chatto.toml) get this role materialized at boot
	// and on email verification.
	RoleOwner = "owner"

	// RoleAdmin has administrative permissions. Must be explicitly assigned.
	RoleAdmin = "admin"

	// RoleModerator has moderation permissions. Must be explicitly assigned.
	RoleModerator = "moderator"

	// RoleEveryone is implicit for all authenticated users. Virtual — not
	// stored in KV; permission grants on this role apply to every
	// authenticated viewer.
	RoleEveryone = "everyone"
)

// IsSystemRole returns true if the role name is a system role that cannot be
// deleted. Custom roles must avoid these names.
func IsSystemRole(name string) bool {
	return name == RoleOwner || name == RoleAdmin || name == RoleModerator || name == RoleEveryone
}

// VirtualRoles returns the virtual role definitions for the server RBAC
// engine. Only `everyone` is virtual — owner, admin, and moderator are
// explicitly created in KV at boot. Position scheme: owner=0, admin=1,
// moderator=2, custom=3..n, everyone=MAX.
func VirtualRoles() []*corev1.Role {
	return []*corev1.Role{
		{
			Name:        RoleEveryone,
			DisplayName: "Everyone",
			Description: "All authenticated users",
			Position:    PositionEveryone,
		},
	}
}
