package core

import (
	"fmt"
	"slices"
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
	CategoryUser    PermissionCategory = "user"
	CategoryBot     PermissionCategory = "bot"
)

// Permission is an opaque, stable identifier in the permission model. Its
// punctuation does not define authorization relationships.
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

	// PermRoomManage allows updating or deleting channel rooms.
	PermRoomManage Permission = "room.manage"

	// PermRoomMemberBan allows banning members from channel rooms.
	PermRoomMemberBan Permission = "room.ban-member"

	// ===== Message Permissions =====

	// PermMessageRead allows reading message content in channel rooms. Room
	// membership remains a separate requirement. DM membership authorizes DM
	// reads without this permission.
	PermMessageRead Permission = "message.read"

	// PermMessageReadInteractions allows reading channel-room threads that the
	// account authored or where another account directly mentioned it. Room
	// membership remains a separate requirement. PermMessageRead explicitly
	// includes this permission.
	PermMessageReadInteractions Permission = "message.read-interactions"

	// PermMessagePost allows posting new root messages in rooms. Server-scope
	// decisions act as global defaults/overrides; room or group denies can narrow
	// that default where a room should be more restrictive.
	PermMessagePost Permission = "message.post"

	// PermMessagePostInThread allows posting messages in a thread (first or subsequent reply).
	PermMessagePostInThread Permission = "message.post-in-thread"

	// PermMessageAttach allows attaching files to new messages.
	PermMessageAttach Permission = "message.attach"

	// PermMessageManage allows moderating other users' messages in a room
	// (editing or deleting). Authors editing or deleting their own messages do
	// NOT need this permission; it is always allowed.
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

	// PermAdminUsersView allows viewing the users page in admin.
	PermAdminUsersView Permission = "admin.view-users"

	// PermAdminAuditView allows viewing the audit log in admin.
	PermAdminAuditView Permission = "admin.view-audit"

	// ===== User Management Permissions =====
	//
	// "User" is the canonical namespace for user-administration actions.
	// In Chatto's single-server model, "remove a member from the server"
	// and "delete a user account" mean the same thing — there's no other
	// server they could be a member of. We use `user.*` as the
	// administration namespace and `member.*` doesn't exist.

	// PermUserDeleteAny allows admins to delete any user's account.
	PermUserDeleteAny Permission = "user.delete-any"

	// PermUserDeleteSelf allows users to delete their own account.
	PermUserDeleteSelf Permission = "user.delete-self"

	// PermUserInvite allows listing, creating, copying, and revoking invite links.
	PermUserInvite Permission = "user.invite"

	// PermUserManageAccounts allows account lifecycle and recovery operations
	// for other users, such as creating accounts, admin profile edits, password
	// resets, verified-email attachment, and login-cooldown resets.
	PermUserManageAccounts Permission = "user.manage-accounts"

	// PermUserManagePermissions allows editing direct per-user permission
	// overrides.
	PermUserManagePermissions Permission = "user.manage-permissions"

	// ===== Bot Account Permissions =====

	// PermBotCreate allows a human user to create bot accounts they own.
	PermBotCreate Permission = "bot.create"

	// PermBotManage allows a human user to manage every bot on the server.
	PermBotManage Permission = "bot.manage"
)

// PermissionMetadata defines one known permission. Includes lists other
// permissions that an allow for this permission also grants. Each relationship
// is direct and explicit. Denials do not follow inclusion relationships.
type PermissionMetadata struct {
	Permission Permission
	Category   PermissionCategory
	Scopes     []PermissionScope // Scopes where this permission can be configured
	Includes   []Permission      // Permissions granted by an allow for this permission
}

// allPermissions holds metadata for all permissions.
var allPermissions = []PermissionMetadata{
	// Server
	{Permission: PermServerManage, Category: CategoryServer, Scopes: []PermissionScope{ScopeServer}},

	// Room
	{Permission: PermRoomCreate, Category: CategoryRoom, Scopes: []PermissionScope{ScopeServer, ScopeGroup}},
	{Permission: PermRoomJoin, Category: CategoryRoom, Scopes: []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{Permission: PermRoomList, Category: CategoryRoom, Scopes: []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{Permission: PermRoomManage, Category: CategoryRoom, Scopes: []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{Permission: PermRoomMemberBan, Category: CategoryRoom, Scopes: []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},

	// Message
	{Permission: PermMessageRead, Category: CategoryMessage, Scopes: []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}, Includes: []Permission{PermMessageReadInteractions}},
	{Permission: PermMessageReadInteractions, Category: CategoryMessage, Scopes: []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{Permission: PermMessagePost, Category: CategoryMessage, Scopes: []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{Permission: PermMessagePostInThread, Category: CategoryMessage, Scopes: []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{Permission: PermMessageAttach, Category: CategoryMessage, Scopes: []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{Permission: PermMessageManage, Category: CategoryMessage, Scopes: []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{Permission: PermMessageReact, Category: CategoryMessage, Scopes: []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},
	{Permission: PermMessageEcho, Category: CategoryMessage, Scopes: []PermissionScope{ScopeServer, ScopeGroup, ScopeRoom}},

	// Role management
	{Permission: PermRoleManage, Category: CategoryRole, Scopes: []PermissionScope{ScopeServer}},
	{Permission: PermRoleAssign, Category: CategoryRole, Scopes: []PermissionScope{ScopeServer}},

	// Admin
	{Permission: PermAdminUsersView, Category: CategoryAdmin, Scopes: []PermissionScope{ScopeServer}},
	{Permission: PermAdminAuditView, Category: CategoryAdmin, Scopes: []PermissionScope{ScopeServer}},

	// User management
	{Permission: PermUserDeleteAny, Category: CategoryUser, Scopes: []PermissionScope{ScopeServer}},
	{Permission: PermUserDeleteSelf, Category: CategoryUser, Scopes: []PermissionScope{ScopeServer}},
	{Permission: PermUserInvite, Category: CategoryUser, Scopes: []PermissionScope{ScopeServer}},
	{Permission: PermUserManageAccounts, Category: CategoryUser, Scopes: []PermissionScope{ScopeServer}},
	{Permission: PermUserManagePermissions, Category: CategoryUser, Scopes: []PermissionScope{ScopeServer}},

	// Bot accounts
	{Permission: PermBotCreate, Category: CategoryBot, Scopes: []PermissionScope{ScopeServer}},
	{Permission: PermBotManage, Category: CategoryBot, Scopes: []PermissionScope{ScopeServer}},
}

// permissionIndex provides fast lookup of permission metadata by permission value.
var permissionIndex map[Permission]PermissionMetadata

func init() {
	var err error
	permissionIndex, err = validatePermissionCatalog(allPermissions)
	if err != nil {
		panic(err)
	}
}

func validatePermissionCatalog(catalog []PermissionMetadata) (map[Permission]PermissionMetadata, error) {
	index := make(map[Permission]PermissionMetadata, len(catalog))
	for _, metadata := range catalog {
		if metadata.Permission == "" {
			return nil, fmt.Errorf("permission identifier must not be empty")
		}
		if _, exists := index[metadata.Permission]; exists {
			return nil, fmt.Errorf("permission catalog contains duplicate permission %s", metadata.Permission)
		}
		index[metadata.Permission] = metadata
	}
	for _, metadata := range catalog {
		for _, includedPermission := range metadata.Includes {
			included, exists := index[includedPermission]
			if !exists {
				return nil, fmt.Errorf("permission %s includes unknown permission %s", metadata.Permission, includedPermission)
			}
			if metadata.Permission == includedPermission {
				return nil, fmt.Errorf("permission %s includes itself", metadata.Permission)
			}
			if metadata.Category != included.Category {
				return nil, fmt.Errorf("permission %s and included permission %s use different categories", metadata.Permission, includedPermission)
			}
			if !samePermissionScopes(metadata.Scopes, included.Scopes) {
				return nil, fmt.Errorf("permission %s and included permission %s use different scopes", metadata.Permission, includedPermission)
			}
		}
	}
	return index, nil
}

func samePermissionScopes(left, right []PermissionScope) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[PermissionScope]int, len(left))
	for _, scope := range left {
		counts[scope]++
	}
	for _, scope := range right {
		counts[scope]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
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

// includingPermissions returns registered permissions whose allows directly
// include the requested permission.
func includingPermissions(perm Permission) []Permission {
	return includingPermissionsFrom(permissionIndex, perm)
}

func includingPermissionsFrom(index map[Permission]PermissionMetadata, perm Permission) []Permission {
	if _, registered := index[perm]; !registered {
		return nil
	}
	var result []Permission
	for candidate, metadata := range index {
		if slices.Contains(metadata.Includes, perm) {
			result = append(result, candidate)
		}
	}
	slices.Sort(result)
	return result
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

// DefaultEveryonePermissions returns server-scope permissions granted to every
// authenticated user (the implicit everyone role). These defaults make normal
// rooms usable out of the box; operators can deny the room/group permissions at
// room or group scope where they need local restrictions.
func DefaultEveryonePermissions() []Permission {
	return []Permission{
		PermUserDeleteSelf,
		PermRoomList,
		PermRoomJoin,
		PermMessageRead,
		PermMessagePost,
		PermMessagePostInThread,
		PermMessageAttach,
		PermMessageReact,
		PermMessageEcho,
		PermBotCreate,
	}
}

// DefaultModeratorPermissions returns the permissions granted to moderators
// by default. Moderators inherit the implicit everyone role at runtime; this
// list contains only moderator-specific server-scope capabilities.
func DefaultModeratorPermissions() []Permission {
	return []Permission{
		PermMessageManage,
		PermRoomMemberBan,
	}
}

// DefaultAdminPermissions returns the server-scope permissions granted to
// admins by default. Admins inherit the implicit everyone role at runtime, so
// this list contains only admin-specific capabilities plus global room
// administration defaults.
func DefaultAdminPermissions() []Permission {
	return []Permission{
		PermServerManage,
		PermUserInvite,
		PermRoomCreate,
		PermRoomJoin,
		PermRoomList,
		PermRoomManage,
		PermRoomMemberBan,
		PermMessageManage,
		PermRoleManage,
		PermRoleAssign,
		PermAdminUsersView,
		PermAdminAuditView,
		PermUserDeleteAny,
		PermUserDeleteSelf,
		PermUserManageAccounts,
		PermUserManagePermissions,
		PermBotManage,
	}
}

// DefaultOwnerPermissions returns the persisted permissions granted to owners
// by default. Owners are resolved through the effective-owner override instead
// of stored grants, so fresh servers do not materialize owner permission rows.
func DefaultOwnerPermissions() []Permission {
	return nil
}

// AnnouncementsRoomName is the canonical name for the seeded announcement-only
// room whose creation-time permission facts differ from ordinary rooms.
const AnnouncementsRoomName = "announcements"

// DefaultAnnouncementsEveryoneDenials returns the room-scope denials for the
// built-in announcements room. This blocks root posts unless a named role or
// direct user has a room-local allow. Effective owners bypass the decision.
func DefaultAnnouncementsEveryoneDenials() []Permission {
	return []Permission{PermMessagePost}
}

// DefaultAnnouncementsAdminPermissions returns the room-scope grants that let
// admins publish announcements. Other ordinary content capabilities continue
// to come from the everyone baseline, and moderators receive no announcement-
// specific grant.
func DefaultAnnouncementsAdminPermissions() []Permission {
	return []Permission{PermMessagePost}
}
