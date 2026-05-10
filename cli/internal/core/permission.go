package core

import (
	"fmt"
	"slices"
	"strings"
)

// PermissionScope indicates where a permission can be configured.
type PermissionScope string

const (
	ScopeInstance PermissionScope = "instance"
	ScopeSpace    PermissionScope = "space"
	ScopeRoom     PermissionScope = "room"
)

// PermissionCategory groups related permissions for UI organization.
type PermissionCategory string

const (
	CategorySpace    PermissionCategory = "space"
	CategoryRoom     PermissionCategory = "room"
	CategoryMessage  PermissionCategory = "message"
	CategoryMember   PermissionCategory = "member"
	CategoryRole     PermissionCategory = "role"
	CategoryAdmin    PermissionCategory = "admin"
	CategoryDM       PermissionCategory = "dm"
	CategoryUser     PermissionCategory = "user"
)

// Permission represents a permission in the permission model.
// All permissions are defined globally but can be configured at different scopes.
type Permission string

const (
	// ===== Space Permissions =====

	// PermSpaceManage allows updating space settings (name, description, logo).
	// Scope: space only
	PermSpaceManage Permission = "space.manage"

	// PermSpaceDelete allows deleting a space entirely.
	// Scope: space only
	PermSpaceDelete Permission = "space.delete"

	// ===== Room Permissions =====

	// PermRoomList allows viewing the list of rooms in a space.
	// Scope: instance (default), space (override), room (override for specific room)
	PermRoomList Permission = "room.list"

	// PermRoomCreate allows creating new rooms in a space.
	// Scope: instance, space
	PermRoomCreate Permission = "room.create"

	// PermRoomJoin allows joining existing rooms.
	// Scope: instance, space, room
	PermRoomJoin Permission = "room.join"

	// PermRoomLeave allows leaving a room.
	// Scope: instance, space, room
	PermRoomLeave Permission = "room.leave"

	// PermRoomManage allows updating or deleting any room.
	// Scope: space, room
	PermRoomManage Permission = "room.manage"

	// ===== Message Permissions =====

	// PermMessagePost allows posting new root messages in a room.
	// Scope: instance, space, room
	PermMessagePost Permission = "message.post"

	// PermMessagePostInThread allows posting messages in a thread (first or subsequent reply).
	// Scope: instance, space, room
	PermMessagePostInThread Permission = "message.post-in-thread"

	// PermMessageReply allows using reply attribution (inReplyTo) on room-level messages.
	// Denying this hides the Reply button in the room timeline, encouraging thread usage.
	// Scope: instance, space, room
	PermMessageReply Permission = "message.reply"

	// PermMessageReplyInThread allows using reply attribution (inReplyTo) on thread messages.
	// Scope: instance, space, room
	PermMessageReplyInThread Permission = "message.reply-in-thread"

	// PermMessageEditOwn allows editing one's own messages.
	// Scope: instance, space, room
	PermMessageEditOwn Permission = "message.edit-own"

	// PermMessageEditAny allows editing any user's messages.
	// Scope: space, room (moderation)
	PermMessageEditAny Permission = "message.edit-any"

	// PermMessageDeleteOwn allows deleting one's own messages.
	// Scope: instance, space, room
	PermMessageDeleteOwn Permission = "message.delete-own"

	// PermMessageDeleteAny allows deleting any user's messages.
	// Scope: space, room (moderation)
	PermMessageDeleteAny Permission = "message.delete-any"

	// PermMessageReact allows adding/removing reactions to messages.
	// Scope: instance, space, room
	PermMessageReact Permission = "message.react"

	// PermMessageEcho allows echoing thread replies to the main channel.
	// Scope: instance, space, room
	PermMessageEcho Permission = "message.echo"

	// ===== Member Management Permissions =====

	// PermMemberInvite allows inviting new members to a space.
	// Scope: space only
	PermMemberInvite Permission = "member.invite"

	// PermMemberRemove allows removing members from a space.
	// Scope: space only
	PermMemberRemove Permission = "member.remove"

	// ===== Role Management Permissions =====

	// PermRoleManage allows creating, editing, and deleting roles.
	// Scope: space only
	PermRoleManage Permission = "role.manage"

	// PermRoleAssign allows assigning and revoking roles for members.
	// Scope: space only
	PermRoleAssign Permission = "role.assign"

	// ===== Instance Admin Permissions =====

	// PermAdminAccess allows access to the instance admin panel.
	// Scope: instance only
	PermAdminAccess Permission = "admin.access"

	// PermAdminUsersView allows viewing the users page in admin.
	// Scope: instance only
	PermAdminUsersView Permission = "admin.view-users"

	// PermAdminUsersManage allows editing user role assignments at instance level.
	// Scope: instance only
	PermAdminUsersManage Permission = "admin.manage-users"

	// PermAdminRolesView allows viewing the instance roles page in admin.
	// Scope: instance only
	PermAdminRolesView Permission = "admin.view-roles"

	// PermAdminRolesManage allows creating and editing instance roles.
	// Scope: instance only
	PermAdminRolesManage Permission = "admin.manage-roles"

	// PermAdminSystemView allows viewing system and data pages in admin.
	// Scope: instance only
	PermAdminSystemView Permission = "admin.view-system"

	// PermAdminAuditView allows viewing the audit log in admin.
	// Scope: instance only
	PermAdminAuditView Permission = "admin.view-audit"

	// ===== DM Permissions =====

	// PermDMView allows accessing DMs and reading direct messages.
	// Scope: instance only
	PermDMView Permission = "dm.view"

	// PermDMWrite allows starting DM conversations and sending messages.
	// Scope: instance only
	PermDMWrite Permission = "dm.write"

	// ===== User Management Permissions =====

	// PermUserDelete allows deleting user accounts (admin power).
	// Scope: instance only
	PermUserDelete Permission = "user.delete"

	// PermUserDeleteSelf allows users to delete their own account.
	// Scope: instance only
	PermUserDeleteSelf Permission = "user.delete-self"
)

// PermissionMetadata provides display information and scope constraints for a permission.
type PermissionMetadata struct {
	Permission  Permission
	DisplayName string
	Description string
	Category    PermissionCategory
	Scopes      []PermissionScope // Scopes where this permission can be configured
}

// allPermissions holds metadata for all permissions.
var allPermissions = []PermissionMetadata{
	// Space permissions
	{PermSpaceManage, "Manage Space", "Update space settings (name, description, logo)", CategorySpace, []PermissionScope{ScopeSpace}},
	{PermSpaceDelete, "Delete Space", "Delete the space and all its data", CategorySpace, []PermissionScope{ScopeSpace}},

	// Room permissions
	{PermRoomList, "List Rooms", "View the list of rooms", CategoryRoom, []PermissionScope{ScopeInstance, ScopeSpace}},
	{PermRoomCreate, "Create Rooms", "Create new rooms", CategoryRoom, []PermissionScope{ScopeInstance, ScopeSpace}},
	{PermRoomJoin, "Join Rooms", "Join existing rooms", CategoryRoom, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},
	{PermRoomLeave, "Leave Rooms", "Leave rooms", CategoryRoom, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},
	{PermRoomManage, "Manage Rooms", "Edit and delete any room", CategoryRoom, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},

	// Message permissions
	{PermMessagePost, "Post Messages", "Post new messages in rooms", CategoryMessage, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},
	{PermMessagePostInThread, "Post in Threads", "Post messages in threads", CategoryMessage, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},
	{PermMessageReply, "Reply in Room", "Use reply attribution on room-level messages", CategoryMessage, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},
	{PermMessageReplyInThread, "Reply in Thread", "Use reply attribution on thread messages", CategoryMessage, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},
	{PermMessageEditOwn, "Edit Own Messages", "Edit your own messages", CategoryMessage, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},
	{PermMessageEditAny, "Edit Any Message", "Edit any user's messages", CategoryMessage, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},
	{PermMessageDeleteOwn, "Delete Own Messages", "Delete your own messages", CategoryMessage, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},
	{PermMessageDeleteAny, "Delete Any Message", "Delete any user's messages", CategoryMessage, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},
	{PermMessageReact, "React to Messages", "Add and remove reactions", CategoryMessage, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},
	{PermMessageEcho, "Echo to Channel", "Echo thread replies to the main channel for visibility", CategoryMessage, []PermissionScope{ScopeInstance, ScopeSpace, ScopeRoom}},

	// Member management
	{PermMemberInvite, "Invite Members", "Invite new members to the space", CategoryMember, []PermissionScope{ScopeSpace}},
	{PermMemberRemove, "Remove Members", "Remove members from the space", CategoryMember, []PermissionScope{ScopeSpace}},

	// Role management
	{PermRoleManage, "Manage Roles", "Create, edit, and delete roles", CategoryRole, []PermissionScope{ScopeSpace}},
	{PermRoleAssign, "Assign Roles", "Assign and revoke roles for members", CategoryRole, []PermissionScope{ScopeSpace}},

	// Instance admin
	{PermAdminAccess, "Admin Access", "Access the admin panel", CategoryAdmin, []PermissionScope{ScopeInstance}},
	{PermAdminUsersView, "View Users", "View the users page in admin", CategoryAdmin, []PermissionScope{ScopeInstance}},
	{PermAdminUsersManage, "Manage Users", "Edit user role assignments", CategoryAdmin, []PermissionScope{ScopeInstance}},
	{PermAdminRolesView, "View Roles", "View the instance roles page in admin", CategoryAdmin, []PermissionScope{ScopeInstance}},
	{PermAdminRolesManage, "Manage Instance Roles", "Full control over instance roles: create, edit, delete, reorder, and manage permissions", CategoryAdmin, []PermissionScope{ScopeInstance}},
	{PermAdminSystemView, "View System", "View system and data pages in admin", CategoryAdmin, []PermissionScope{ScopeInstance}},
	{PermAdminAuditView, "View Audit Log", "View the audit log in admin", CategoryAdmin, []PermissionScope{ScopeInstance}},

	// DM
	{PermDMView, "View DMs", "Access DMs and read direct messages", CategoryDM, []PermissionScope{ScopeInstance}},
	{PermDMWrite, "Send DMs", "Start DM conversations and send messages", CategoryDM, []PermissionScope{ScopeInstance}},

	// User management
	{PermUserDelete, "Delete Users", "Delete user accounts", CategoryUser, []PermissionScope{ScopeInstance}},
	{PermUserDeleteSelf, "Delete Own Account", "Delete your own account", CategoryUser, []PermissionScope{ScopeInstance}},
}

// permissionIndex provides fast lookup of permission metadata by permission value.
var permissionIndex map[Permission]PermissionMetadata

func init() {
	permissionIndex = make(map[Permission]PermissionMetadata, len(allPermissions))
	for _, p := range allPermissions {
		permissionIndex[p.Permission] = p
	}
}

// AllPermissions returns all defined permissions with their metadata.
func AllPermissions() []PermissionMetadata {
	return allPermissions
}

// GetPermissionMetadata returns metadata for a specific permission.
// Returns zero value if permission not found.
func GetPermissionMetadata(perm Permission) (PermissionMetadata, bool) {
	meta, ok := permissionIndex[perm]
	return meta, ok
}

// ValidatePermission checks if a permission value is valid.
func ValidatePermission(perm Permission) error {
	if _, ok := permissionIndex[perm]; !ok {
		return fmt.Errorf("%w: %s", ErrInvalidPermission, perm)
	}
	return nil
}

// ValidatePermissionString checks if a string is a valid permission.
func ValidatePermissionString(perm string) error {
	return ValidatePermission(Permission(perm))
}

// PermissionAppliesAtScope checks if a permission can be configured at a given scope.
func PermissionAppliesAtScope(perm Permission, scope PermissionScope) bool {
	meta, ok := permissionIndex[perm]
	if !ok {
		return false
	}
	return slices.Contains(meta.Scopes, scope)
}

// PermissionsForScope returns all permissions that can be configured at a given scope.
func PermissionsForScope(scope PermissionScope) []PermissionMetadata {
	var result []PermissionMetadata
	for _, p := range allPermissions {
		if slices.Contains(p.Scopes, scope) {
			result = append(result, p)
		}
	}
	return result
}

// PermissionsForCategory returns all permissions in a given category.
func PermissionsForCategory(category PermissionCategory) []PermissionMetadata {
	var result []PermissionMetadata
	for _, p := range allPermissions {
		if p.Category == category {
			result = append(result, p)
		}
	}
	return result
}

// ============================================================================
// Default Role Permissions
// ============================================================================

// DefaultInstanceEveryonePermissions returns permissions granted to all authenticated users.
func DefaultInstanceEveryonePermissions() []Permission {
	return []Permission{
		PermUserDeleteSelf, // Can delete own account
		PermDMView,         // Can view DMs
		PermDMWrite,        // Can send DMs
	}
}

// DefaultInstanceModeratorPermissions returns permissions granted to instance moderators.
// Only instance-scoped permissions. Space-scoped permissions are managed by space roles.
func DefaultInstanceModeratorPermissions() []Permission {
	return []Permission{
		// Same as verified
		PermDMView,
		PermDMWrite,
		// Plus admin view access (no management permissions)
		PermAdminAccess,
		PermAdminUsersView,
		PermAdminRolesView,
	}
}

// DefaultSpaceEveryonePermissions returns permissions granted to space members by default.
// Note: room.create is NOT included - space admins must explicitly grant it.
func DefaultSpaceEveryonePermissions() []Permission {
	return []Permission{
		PermRoomList,
		PermRoomJoin,
		PermRoomLeave,
		PermMessagePost,
		PermMessagePostInThread,
		PermMessageReply,
		PermMessageReplyInThread,
		PermMessageEditOwn,
		PermMessageDeleteOwn,
		PermMessageReact,
		PermMessageEcho,
	}
}

// DefaultSpaceModeratorPermissions returns permissions granted to moderators.
func DefaultSpaceModeratorPermissions() []Permission {
	return []Permission{
		// Same as member
		PermRoomList,
		PermRoomCreate,
		PermRoomJoin,
		PermRoomLeave,
		PermMessagePost,
		PermMessagePostInThread,
		PermMessageReply,
		PermMessageReplyInThread,
		PermMessageEditOwn,
		PermMessageDeleteOwn,
		PermMessageReact,
		PermMessageEcho,
		// Plus moderation powers
		PermRoomManage,
		PermMemberRemove,
		PermMessageDeleteAny,
	}
}

// DefaultSpaceAdminPermissions returns permissions granted to space admins.
func DefaultSpaceAdminPermissions() []Permission {
	return []Permission{
		// Same as moderator
		PermRoomList,
		PermRoomCreate,
		PermRoomJoin,
		PermRoomLeave,
		PermMessagePost,
		PermMessagePostInThread,
		PermMessageReply,
		PermMessageReplyInThread,
		PermMessageEditOwn,
		PermMessageDeleteOwn,
		PermMessageReact,
		PermMessageEcho,
		PermRoomManage,
		PermMemberRemove,
		PermMessageDeleteAny,
		// Plus admin powers
		PermSpaceManage,
		PermMemberInvite,
		PermRoleManage,
		PermRoleAssign,
	}
}

// ============================================================================
// Role Naming
// ============================================================================

// ScopedRoleSeparator is used to combine scope and role name in KV keys.
// We use dot (.) to leverage NATS KV's hierarchical key structure.
// Example: instance.admin, space.member
const ScopedRoleSeparator = "."

// ScopedRoleName returns the scoped role name for use in KV keys.
// Instance roles: "instance.admin", "instance.moderator", "instance.everyone"
// Space roles: "space.admin", "space.member", "space.moderator"
func ScopedRoleName(scope PermissionScope, roleName string) string {
	return string(scope) + ScopedRoleSeparator + roleName
}

// ParseScopedRoleName extracts the scope and role name from a scoped role name.
// Returns empty strings if the format is invalid.
// Expects format: "scope.roleName" (e.g., "instance.admin", "space.everyone")
func ParseScopedRoleName(scopedName string) (scope PermissionScope, roleName string) {
	for i := 0; i < len(scopedName); i++ {
		if scopedName[i] == ScopedRoleSeparator[0] {
			return PermissionScope(scopedName[:i]), scopedName[i+1:]
		}
	}
	return "", ""
}

// ============================================================================
// Permission Key Parts (for KV key generation)
// ============================================================================

// PermissionKeyParts holds the verb and objectType components for KV key generation.
// Permission strings follow the format "{objectType}.{verb}" (e.g., "room.create",
// "message.delete-own", "admin.view-users"), so key parts are derived directly from
// the permission string — no separate mapping needed.
type PermissionKeyParts struct {
	Verb       string // The action: "create", "join", "delete-own", "view-users", etc.
	ObjectType string // The target type: "space", "room", "message", "admin", etc.
}

// parseKeyParts splits a permission string into its objectType and verb components.
// All permissions follow the "{objectType}.{verb}" convention.
func parseKeyParts(perm string) PermissionKeyParts {
	objectType, verb, ok := strings.Cut(perm, ".")
	if !ok {
		return PermissionKeyParts{}
	}
	return PermissionKeyParts{Verb: verb, ObjectType: objectType}
}

func init() {
	// Validate that all permission strings follow the "{objectType}.{verb}" format.
	for _, p := range allPermissions {
		parts := parseKeyParts(string(p.Permission))
		if parts.Verb == "" || parts.ObjectType == "" {
			panic(fmt.Sprintf("permission %q does not follow {objectType}.{verb} format", p.Permission))
		}
		if strings.Contains(parts.Verb, ".") {
			panic(fmt.Sprintf("permission %q has nested dots — verb %q must use dashes instead", p.Permission, parts.Verb))
		}
	}
}

// GetPermissionKeyParts returns the verb and objectType for a permission.
func GetPermissionKeyParts(perm Permission) PermissionKeyParts {
	return parseKeyParts(string(perm))
}

// KeyParts returns the verb and objectType for this permission.
func (p Permission) KeyParts() PermissionKeyParts {
	return parseKeyParts(string(p))
}

// ReconstructPermission builds a Permission from verb and objectType.
// Returns empty string if the resulting permission is not registered.
func ReconstructPermission(verb, objectType string) Permission {
	perm := Permission(objectType + "." + verb)
	if _, ok := permissionIndex[perm]; ok {
		return perm
	}
	return ""
}
