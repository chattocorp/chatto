package core

import (
	"fmt"
	"slices"
	"strings"
)

// PermissionScope marks where a permission can be configured.
// Most permissions apply at the server level (default). Room-overridable
// permissions (e.g. message.post) additionally include ScopeRoom so the UI
// knows to surface them in per-room permission editors.
type PermissionScope string

const (
	ScopeServer PermissionScope = "server"
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

	// PermRoomList allows viewing the list of rooms.
	PermRoomList Permission = "room.list"

	// PermRoomCreate allows creating new rooms.
	PermRoomCreate Permission = "room.create"

	// PermRoomJoin allows joining existing rooms.
	PermRoomJoin Permission = "room.join"

	// PermRoomLeave allows leaving a room.
	PermRoomLeave Permission = "room.leave"

	// PermRoomManage allows updating or deleting any room.
	PermRoomManage Permission = "room.manage"

	// ===== Message Permissions =====

	// PermMessagePost allows posting new root messages in a room.
	PermMessagePost Permission = "message.post"

	// PermMessagePostInThread allows posting messages in a thread (first or subsequent reply).
	PermMessagePostInThread Permission = "message.post-in-thread"

	// PermMessageReply allows using reply attribution (inReplyTo) on room-level messages.
	// Denying this hides the Reply button in the room timeline, encouraging thread usage.
	PermMessageReply Permission = "message.reply"

	// PermMessageReplyInThread allows using reply attribution (inReplyTo) on thread messages.
	PermMessageReplyInThread Permission = "message.reply-in-thread"

	// PermMessageEditOwn allows editing one's own messages.
	PermMessageEditOwn Permission = "message.edit-own"

	// PermMessageEditAny allows editing any user's messages.
	PermMessageEditAny Permission = "message.edit-any"

	// PermMessageDeleteOwn allows deleting one's own messages.
	PermMessageDeleteOwn Permission = "message.delete-own"

	// PermMessageDeleteAny allows deleting any user's messages.
	PermMessageDeleteAny Permission = "message.delete-any"

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
	{PermRoomList, "List Rooms", "See a room in the room list. Deniable per-room to hide channels from non-members.", CategoryRoom, []PermissionScope{ScopeServer, ScopeRoom}},
	{PermRoomCreate, "Create Rooms", "Create new rooms", CategoryRoom, []PermissionScope{ScopeServer}},
	{PermRoomJoin, "Join Rooms", "Join existing rooms", CategoryRoom, []PermissionScope{ScopeServer, ScopeRoom}},
	{PermRoomLeave, "Leave Rooms", "Leave rooms", CategoryRoom, []PermissionScope{ScopeServer, ScopeRoom}},
	{PermRoomManage, "Manage Rooms", "Edit and delete any room", CategoryRoom, []PermissionScope{ScopeServer, ScopeRoom}},

	// Message
	{PermMessagePost, "Post Messages", "Post new messages in rooms", CategoryMessage, []PermissionScope{ScopeServer, ScopeRoom}},
	{PermMessagePostInThread, "Post in Threads", "Post messages in threads", CategoryMessage, []PermissionScope{ScopeServer, ScopeRoom}},
	{PermMessageReply, "Reply in Room", "Use reply attribution on room-level messages", CategoryMessage, []PermissionScope{ScopeServer, ScopeRoom}},
	{PermMessageReplyInThread, "Reply in Thread", "Use reply attribution on thread messages", CategoryMessage, []PermissionScope{ScopeServer, ScopeRoom}},
	{PermMessageEditOwn, "Edit Own Messages", "Edit your own messages", CategoryMessage, []PermissionScope{ScopeServer, ScopeRoom}},
	{PermMessageEditAny, "Edit Any Message", "Edit any user's messages", CategoryMessage, []PermissionScope{ScopeServer, ScopeRoom}},
	{PermMessageDeleteOwn, "Delete Own Messages", "Delete your own messages", CategoryMessage, []PermissionScope{ScopeServer, ScopeRoom}},
	{PermMessageDeleteAny, "Delete Any Message", "Delete any user's messages", CategoryMessage, []PermissionScope{ScopeServer, ScopeRoom}},
	{PermMessageReact, "React to Messages", "Add and remove reactions", CategoryMessage, []PermissionScope{ScopeServer, ScopeRoom}},
	{PermMessageEcho, "Echo to Channel", "Echo thread replies to the main channel for visibility", CategoryMessage, []PermissionScope{ScopeServer, ScopeRoom}},

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

// DefaultModeratorPermissions returns the permissions granted to moderators
// by default. Moderators inherit everyone permissions plus moderation powers
// and admin-panel view access.
func DefaultModeratorPermissions() []Permission {
	return append(DefaultEveryonePermissions(),
		// Admin-panel view access
		PermAdminAccess,
		PermAdminUsersView,
		// Moderation powers
		PermMessageEditAny,
		PermMessageDeleteAny,
	)
}

// DefaultAdminPermissions returns the permissions granted to admins by
// default. Admins receive every server-scope permission. They are
// distinguished from owners by rank, not by any permission they lack:
// admins cannot manage owners (rank check) and cannot revoke their own
// admin role (self-lockout prevention), but the permission set is the
// same.
func DefaultAdminPermissions() []Permission {
	perms := PermissionsForScope(ScopeServer)
	result := make([]Permission, 0, len(perms))
	for _, p := range perms {
		result = append(result, p.Permission)
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
