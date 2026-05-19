package core

import (
	"fmt"
	"slices"
	"strings"
)

// PermissionScope marks where a permission can be configured.
// Most permissions apply at the server level (default). Channel-room
// permissions (e.g. message.post) additionally include ScopeGroup (to be
// configured per room group) and ScopeRoom (to be overridden per individual
// room).
type PermissionScope string

const (
	ScopeServer PermissionScope = "server"
	ScopeGroup  PermissionScope = "group"
	ScopeRoom   PermissionScope = "room"
)

// PermissionCategory groups related permissions for UI organization.
type PermissionCategory string

const (
	CategoryServer  PermissionCategory = "server"
	CategoryRoom    PermissionCategory = "room"
	CategoryMessage PermissionCategory = "message"
	CategoryRole    PermissionCategory = "role"
	CategoryAdmin   PermissionCategory = "admin"
	CategoryDM      PermissionCategory = "dm"
	CategoryUser    PermissionCategory = "user"
)

// Permission represents a permission in the permission model.
type Permission string

const (
	// ===== Server Permissions =====

	// PermServerManage allows updating server settings (name, description, logo).
	PermServerManage Permission = "server.manage"

	// ===== Room Permissions =====

	// PermRoomCreate allows creating new rooms.
	PermRoomCreate Permission = "room.create"

	// PermRoomJoin allows joining existing rooms. Distinct from
	// `room.list`: a user can be allowed to *see* a room in the
	// directory (request-access flow) without being allowed to join
	// it directly.
	PermRoomJoin Permission = "room.join"

	// PermRoomList allows seeing a room in the directory and elsewhere
	// the server enumerates rooms (e.g. group "Join all" affordances).
	// Default-granted at server scope so the directory works out of the
	// box; deny it on a restricted room to keep it hidden from
	// non-members.
	PermRoomList Permission = "room.list"

	// PermRoomManage allows updating or deleting any room.
	PermRoomManage Permission = "room.manage"

	// ===== Message Permissions =====

	// PermMessagePost allows posting new root messages in a room.
	PermMessagePost Permission = "message.post"

	// PermMessagePostInThread allows posting messages in a thread (first or subsequent reply).
	PermMessagePostInThread Permission = "message.post-in-thread"

	// PermMessageManage allows moderating other users' messages in a room
	// (editing or deleting). The actor must also strictly outrank the
	// message author — enforced at the API boundary. Authors editing or
	// deleting their own messages do NOT need this permission; it is
	// always allowed.
	PermMessageManage Permission = "message.manage"

	// PermMessageReact allows adding/removing reactions to messages.
	PermMessageReact Permission = "message.react"

	// PermMessageEcho allows echoing thread replies to the main channel.
	PermMessageEcho Permission = "message.echo"

	// ===== Role Management Permissions =====

	// PermRoleManage allows creating, editing, deleting, and reordering roles
	// and their permission grants. Single canonical permission for "manage the
	// server's role definitions" (formerly split between role.manage and
	// admin.manage-roles).
	PermRoleManage Permission = "role.manage"

	// PermRoleAssign allows assigning and revoking roles to/from users.
	// Single canonical permission for "manage user role assignments"
	// (formerly split between role.assign and admin.manage-users).
	PermRoleAssign Permission = "role.assign"

	// ===== Admin Panel Permissions =====

	// PermAdminAccess allows access to the admin panel.
	PermAdminAccess Permission = "admin.access"

	// PermAdminUsersView allows viewing the users page in admin.
	PermAdminUsersView Permission = "admin.view-users"

	// PermAdminSystemView allows viewing system and data pages in admin.
	PermAdminSystemView Permission = "admin.view-system"

	// PermAdminAuditView allows viewing the audit log in admin.
	PermAdminAuditView Permission = "admin.view-audit"

	// ===== DM Permissions =====

	// PermDMView allows accessing DMs and reading direct messages.
	PermDMView Permission = "dm.view"

	// PermDMWrite allows starting DM conversations and sending messages.
	PermDMWrite Permission = "dm.write"

	// ===== User Management Permissions =====
	//
	// "User" is the canonical namespace for user-administration actions.
	// In Chatto's single-server model, "remove a member from the server"
	// and "delete a user account" mean the same thing — there's no other
	// server they could be a member of. We use `user.*` as the
	// administration namespace and `member.*` doesn't exist.

	// PermUserDeleteAny allows admins to delete any user's account.
	// Mirrors message.delete-any: the actor needs the permission AND
	// must strictly outrank the target user (rank check enforced at the
	// API boundary when the cross-user delete mutation is implemented).
	PermUserDeleteAny Permission = "user.delete-any"

	// PermUserDeleteSelf allows users to delete their own account.
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
	// Server
	{PermServerManage, "Manage Server", "Update server settings (name, description, logo)", CategoryServer, []PermissionScope{ScopeServer}},

	// Room
	{PermRoomCreate, "Create Rooms", "Create new rooms in this group (or anywhere if granted at server scope)", CategoryRoom, []PermissionScope{ScopeServer, ScopeGroup}},
	{PermRoomJoin, "Join Rooms", "Join existing rooms", CategoryRoom, []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{PermRoomList, "Discover Rooms", "See rooms in the directory and group 'Join all' affordances", CategoryRoom, []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{PermRoomManage, "Manage Rooms", "Edit, configure permissions on, and delete rooms", CategoryRoom, []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},

	// Message
	{PermMessagePost, "Post Messages", "Post new messages in rooms", CategoryMessage, []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{PermMessagePostInThread, "Post in Threads", "Post messages in threads", CategoryMessage, []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{PermMessageManage, "Manage Messages", "Edit and delete other users' messages (subject to outranking the author)", CategoryMessage, []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{PermMessageReact, "React to Messages", "Add and remove reactions", CategoryMessage, []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{PermMessageEcho, "Echo to Channel", "Echo thread replies to the main channel for visibility", CategoryMessage, []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},

	// Role management
	{PermRoleManage, "Manage Roles", "Create, edit, delete, and reorder roles and their permissions", CategoryRole, []PermissionScope{ScopeServer}},
	{PermRoleAssign, "Assign Roles", "Assign and revoke roles for users", CategoryRole, []PermissionScope{ScopeServer}},

	// Admin
	{PermAdminAccess, "Admin Access", "Access the admin panel", CategoryAdmin, []PermissionScope{ScopeServer}},
	{PermAdminUsersView, "View Users", "View the users page in admin", CategoryAdmin, []PermissionScope{ScopeServer}},
	{PermAdminSystemView, "View System", "View system and data pages in admin", CategoryAdmin, []PermissionScope{ScopeServer}},
	{PermAdminAuditView, "View Audit Log", "View the audit log in admin", CategoryAdmin, []PermissionScope{ScopeServer}},

	// DM
	{PermDMView, "View DMs", "Access DMs and read direct messages", CategoryDM, []PermissionScope{ScopeServer}},
	{PermDMWrite, "Send DMs", "Start DM conversations and send messages", CategoryDM, []PermissionScope{ScopeServer}},

	// User management
	{PermUserDeleteAny, "Delete Any User", "Delete any user's account. Subject to the rank check — actors can only delete users they outrank.", CategoryUser, []PermissionScope{ScopeServer}},
	{PermUserDeleteSelf, "Delete Own Account", "Delete your own account", CategoryUser, []PermissionScope{ScopeServer}},
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

// DefaultEveryonePermissions returns the permissions granted to every
// authenticated user (the implicit everyone role).
func DefaultEveryonePermissions() []Permission {
	return []Permission{
		PermUserDeleteSelf,
		PermDMView,
		PermDMWrite,
		PermRoomList,
		PermRoomJoin,
		PermMessagePost,
		PermMessagePostInThread,
		PermMessageReact,
		PermMessageEcho,
	}
}

// DefaultModeratorPermissions returns the permissions granted to moderators
// by default. Moderators inherit everyone permissions plus moderation powers
// and admin-panel view access.
func DefaultModeratorPermissions() []Permission {
	return append(DefaultEveryonePermissions(),
		// Admin-panel view access
		PermAdminAccess,
		PermAdminUsersView,
		// Moderation powers
		PermMessageManage,
	)
}

// DefaultAdminPermissions returns the permissions granted to admins by
// default. Admins receive every server-scope permission plus every
// channel-room permission (configured at group/room tier). They are
// distinguished from owners by rank, not by any permission they lack:
// admins cannot manage owners (rank check) and cannot revoke their own
// admin role (self-lockout prevention), but the permission set is the
// same.
func DefaultAdminPermissions() []Permission {
	seen := map[Permission]bool{}
	var result []Permission
	for _, scope := range []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom} {
		for _, p := range PermissionsForScope(scope) {
			if seen[p.Permission] {
				continue
			}
			seen[p.Permission] = true
			result = append(result, p.Permission)
		}
	}
	return result
}

// DefaultOwnerPermissions returns the permissions granted to owners by
// default. Functionally identical to DefaultAdminPermissions — owners
// and admins share the same enumerated capability set. The distinction
// is purely hierarchical (rank): owners outrank admins and can manage
// admin-rank users; they cannot be revoked except by another owner via
// CLI / system actor.
//
// This deliberately replaces the previous `admin.bypass` super-permission.
// "Skip the entire permission system" as a primitive made every operator-
// configured deny silently ineffective for owners; auditing the security
// boundary now means enumerating role grants like any other role.
func DefaultOwnerPermissions() []Permission {
	return DefaultAdminPermissions()
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
	ObjectType string // The target type: "server", "room", "message", "admin", etc.
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
