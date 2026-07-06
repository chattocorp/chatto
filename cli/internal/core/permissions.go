package core

// Built-in role names (system-defined, cannot be deleted).
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
