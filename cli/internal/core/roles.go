package core

import (
	"hmans.de/chatto/internal/core/rbac"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Role names (system-defined, cannot be deleted).
//
// Per ADR-021 / ADR-028 the post-merge model has a single role namespace —
// the previous instance.* / space.* split collapses into one set, and as of
// PR 4 of the Phase 2 refactor the dual-RBAC engines were merged into one.
//
// Position scheme: owner=0, admin=1, moderator=2, custom=3..n, everyone=MAX
const (
	// RoleOwner has all permissions and is the highest authority.
	// Must be explicitly assigned. Server creator and config-designated
	// owners (owners.emails) get this role.
	RoleOwner = "owner"

	// RoleAdmin has administrative permissions.
	// Must be explicitly assigned. Second-highest role after owner.
	RoleAdmin = "admin"

	// RoleModerator has moderation permissions.
	// Must be explicitly assigned.
	RoleModerator = "moderator"

	// RoleEveryone is implicit for all authenticated users / space members.
	RoleEveryone = "everyone"
)

// IsSystemRole returns true if the role name is a built-in system role that
// cannot be deleted.
func IsSystemRole(name string) bool {
	return name == RoleOwner || name == RoleAdmin || name == RoleModerator || name == RoleEveryone
}

// VirtualRoles returns the virtual role definitions seeded into every RBAC
// engine. Only `everyone` is virtual — `owner`, `admin`, `moderator` are
// explicitly created in KV.
func VirtualRoles() []*corev1.Role {
	return []*corev1.Role{
		{
			Name:        RoleEveryone,
			DisplayName: "Everyone",
			Description: "All authenticated users",
			Position:    rbac.PositionEveryone,
		},
	}
}
