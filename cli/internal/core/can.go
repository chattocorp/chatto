package core

import (
	"context"
	"time"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// can.go provides semantic helper functions for permission checks. These wrap
// the low-level HasServerPermission / hasServerPermission / hasRoomPermission
// calls with business-meaningful names, making code more readable and
// permission usage easier to audit.
//
// Each function returns (bool, error) where:
//   - bool indicates whether the user has the permission
//   - error is non-nil only if there was a system error checking permissions
//
// Note: These functions check RBAC permissions only. Config-based admin
// status (owners.emails) is materialised as a real owner-role assignment
// elsewhere, so the resolver layer doesn't need a separate fallback.

// ============================================================================
// Server-tier Permissions
// ============================================================================

// CanAdminUsersView checks if a user can view the users page in admin.
func (c *ChattoCore) CanAdminUsersView(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermAdminUsersView)
}

// CanAssignRoles checks if a user can assign/revoke roles to/from users.
// Backed by the canonical role.assign permission. Subsumes the previous
// CanAdminUsersManage (which was a duplicate "edit role assignments").
func (c *ChattoCore) CanAssignRoles(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermRoleAssign)
}

// CanManageRoles checks if a user can create, edit, delete, and reorder
// roles and their permissions. Subsumes the previous CanAdminRolesManage /
// CanSpaceRolesManage pair (which were duplicates).
func (c *ChattoCore) CanManageRoles(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermRoleManage)
}

// CanAdminSystemView checks if a user can view system projection diagnostics
// in admin. The diagnostics endpoint exposes low-level system state and is
// owner-only, so this mirrors GetAdminDiagnostics instead of any grantable
// RBAC permission.
func (c *ChattoCore) CanAdminSystemView(ctx context.Context, userID string) (bool, error) {
	return c.IsServerOwner(ctx, userID)
}

// CanAdminAuditView checks if a user can view the audit log (event log)
// page in admin. The event-log inspection view in /manage/server/event-log
// is the first concrete use; future log exports / search endpoints gate
// on the same permission.
func (c *ChattoCore) CanAdminAuditView(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermAdminAuditView)
}

// CanManageUserPermissions checks if a user can edit direct per-user
// permission overrides.
func (c *ChattoCore) CanManageUserPermissions(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermUserManagePermissions)
}

// CanManageUserAccounts checks if a user can perform cross-user account
// lifecycle and recovery operations.
func (c *ChattoCore) CanManageUserAccounts(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermUserManageAccounts)
}

// CanCreateBots checks whether a human user may create an owned bot account.
func (c *ChattoCore) CanCreateBots(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermBotCreate)
}

// CanManageBots checks whether a human user may manage every bot account.
func (c *ChattoCore) CanManageBots(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermBotManage)
}

// CanStartDM checks if a human user can start DM conversations. Bot accounts
// can never start or fetch DMs through the creation operation, regardless of
// their permissions. DMs are allowed by default for active human users, but an
// applicable server-scope message.post deny still blocks the action. This
// keeps global suspension roles effective without requiring a default
// server-scope message.post allow.
func (c *ChattoCore) CanStartDM(ctx context.Context, userID string) (bool, error) {
	if c.contentView == nil {
		return c.canStartDM(ctx, userID)
	}
	var allowed bool
	err := c.contentView.Read(func(uint64) error {
		var checkErr error
		allowed, checkErr = c.canStartDM(ctx, userID)
		return checkErr
	})
	return allowed, err
}

func (c *ChattoCore) canStartDM(ctx context.Context, userID string) (bool, error) {
	isBot, _, accountExists := c.userModel.isBotAndOwner(userID)
	if !accountExists {
		return false, ErrNotFound
	}
	if isBot {
		return false, nil
	}
	decision, err := c.permissionResolver.resolveWithGroup(ctx, userID, KindDM, "", "", PermMessagePost)
	if err != nil {
		return false, err
	}
	return decision != DecisionDeny, nil
}

// CanDeleteUser checks if an actor can delete a specific user account.
// Returns true if:
//   - The actor is deleting their own account and has user.delete-self, OR
//   - The actor has user.delete-any (the admin power).
func (c *ChattoCore) CanDeleteUser(ctx context.Context, actorID, targetUserID string) (bool, error) {
	if actorID == targetUserID {
		return c.HasServerPermission(ctx, actorID, PermUserDeleteSelf)
	}

	return c.HasServerPermission(ctx, actorID, PermUserDeleteAny)
}

// ============================================================================
// Server-tier Admin Permissions
// ============================================================================

// adminPermissions is the set of admin-level server permissions.
// Used by HasAnyAdminPermission to determine "should the Admin link appear".
var adminPermissions = []Permission{
	PermServerManage,
	PermUserInvite,
	PermRoleManage,
	PermRoleAssign,
	PermRoomManage,
	PermRoomMemberBan,
	PermUserDeleteAny,
	PermUserManageAccounts,
	PermUserManagePermissions,
	PermAdminUsersView,
	PermAdminAuditView,
	PermBotManage,
}

// HasAnyAdminPermission checks if a user has any admin-level permission.
// Used to determine whether the server admin link should be visible.
func (c *ChattoCore) HasAnyAdminPermission(ctx context.Context, userID string) (bool, error) {
	if c.contentView == nil {
		return c.hasAnyAdminPermission(ctx, userID)
	}
	var allowed bool
	err := c.contentView.Read(func(uint64) error {
		var checkErr error
		allowed, checkErr = c.hasAnyAdminPermission(ctx, userID)
		return checkErr
	})
	return allowed, err
}

func (c *ChattoCore) hasAnyAdminPermission(ctx context.Context, userID string) (bool, error) {
	for _, perm := range adminPermissions {
		decision, err := c.permissionResolver.resolveWithGroup(ctx, userID, KindChannel, "", "", perm)
		if err != nil {
			return false, err
		}
		if decision == DecisionAllow {
			return true, nil
		}
	}
	return false, nil
}

// CanManageServer checks if a user can update server settings (name, description, logo).
func (c *ChattoCore) CanManageServer(ctx context.Context, userID string) (bool, error) {
	return c.hasServerPermission(ctx, userID, PermServerManage)
}

// CanManageAnyRoom checks if a user can update or delete any room.
// "Any" room as opposed to a specific room — for per-room checks, use the
// room-level resolver via PermissionResolver.HasRoomPermission.
func (c *ChattoCore) CanManageAnyRoom(ctx context.Context, userID string) (bool, error) {
	return c.hasServerPermission(ctx, userID, PermRoomManage)
}

// CanManageRoomGroup checks whether a user can manage room/sidebar layout
// facts owned by a specific room group. Server-scope room.manage still applies
// through the group permission resolver; role.manage is intentionally not a
// substitute for this group-scoped capability.
func (c *ChattoCore) CanManageRoomGroup(ctx context.Context, userID, groupID string) (bool, error) {
	return c.hasGroupPermission(ctx, KindChannel, groupID, userID, PermRoomManage)
}

// ============================================================================
// Server-tier Member Permissions
// ============================================================================

// CanSeeRoom checks if a user can see a specific room in the directory
// or any other surface that enumerates rooms (e.g. the group "Join all"
// affordance). A user can see a room iff they are already a member OR
// `room.list` resolves to allow at the room (room → group → server walk).
//
// `room.list` is distinct from `room.join`: a restricted room can be
// visible in the directory (request-access flow) without being directly
// joinable. Pair with `CanJoinRoomAt` to decide whether to show a "Join"
// button vs a "Restricted" indicator.
//
// DM-sensitive: for KindDM this returns false. DM rooms aren't surfaced
// through the channel room-list API; they use their own listing path.
func (c *ChattoCore) CanSeeRoom(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.readContentDecision(func() (bool, error) {
		return c.canSeeRoom(ctx, userID, kind, roomID)
	})
}

func (c *ChattoCore) canSeeRoom(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	if kind == KindDM {
		return false, nil
	}
	if c.roomModel.hasExplicitRoomMembership(roomID, userID) {
		return true, nil
	}
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return false, err
	}
	if room.GetUniversal() {
		canJoin, joinErr := c.canJoinRoomAt(ctx, userID, kind, roomID)
		if joinErr != nil || canJoin {
			return canJoin, joinErr
		}
	}
	allowed, err := c.permissionResolver.resolveWithGroup(ctx, userID, kind, roomID, "", PermRoomList)
	if err != nil {
		return false, err
	}
	if allowed == DecisionAllow {
		return true, nil
	}
	return false, nil
}

// CanCreateRoom checks if a user can create new rooms. When groupID is
// non-empty, the check is scoped to that room group (a role granted
// room.create at server scope can create in any group; a role granted only
// at a group's scope can create only in that group). DM rooms are
// creation-locked at this layer (the DM boundary in the resolver denies
// room.create unconditionally); DMs are created via FindOrCreateDM.
func (c *ChattoCore) CanCreateRoom(ctx context.Context, userID string, kind RoomKind, groupID string) (bool, error) {
	if kind == KindChannel && groupID != "" {
		return c.hasGroupPermission(ctx, kind, groupID, userID, PermRoomCreate)
	}
	return c.hasKindPermission(ctx, kind, userID, PermRoomCreate)
}

// CanJoinRoom checks if a user can join existing rooms at the server tier
// (no specific room context). Used as a top-level "is the join action
// available at all" check. For per-room decisions — including "is this
// user implicitly a member of this global room" — use CanJoinRoomAt,
// which evaluates room, group, and server decisions.
//
// DM-sensitive: DMs grant join implicitly to participants.
func (c *ChattoCore) CanJoinRoom(ctx context.Context, userID string, kind RoomKind) (bool, error) {
	decision, err := c.ResolveUserPermission(ctx, userID, kind, "", PermRoomJoin)
	if err != nil {
		return false, err
	}
	return decision != DecisionDeny, nil
}

// CanJoinRoomAt checks if a user can join a specific room. Uses room-scope
// permission resolution (room override > group override > server default).
// This is the gate for global-room implicit membership: a global room's
// members are exactly the users for whom this returns true. Active room bans
// deny joins even when RBAC would otherwise allow them.
func (c *ChattoCore) CanJoinRoomAt(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.readContentDecision(func() (bool, error) {
		return c.canJoinRoomAt(ctx, userID, kind, roomID)
	})
}

func (c *ChattoCore) canJoinRoomAt(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	if kind == KindChannel && c.roomModel.isRoomBanActive(roomID, userID, time.Now()) {
		return false, nil
	}
	decision, err := c.permissionResolver.resolveWithGroup(ctx, userID, kind, roomID, "", PermRoomJoin)
	return decision == DecisionAllow, err
}

// ============================================================================
// Room-Scoped Permissions
// ============================================================================

// CanReadMessages checks the permission part of message-content access in a
// specific room. DM membership is the complete DM read boundary, so
// message.read decisions do not restrict DM participants. Callers must enforce
// room membership for both room kinds.
func (c *ChattoCore) CanReadMessages(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.readContentDecision(func() (bool, error) {
		return c.canReadMessages(ctx, userID, kind, roomID)
	})
}

func (c *ChattoCore) canReadMessages(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	if kind == KindDM {
		if _, err := c.GetRoom(ctx, kind, roomID); err != nil {
			return false, err
		}
		return true, nil
	}
	decision, err := c.permissionResolver.resolveWithGroup(ctx, userID, kind, roomID, "", PermMessageRead)
	return decision == DecisionAllow, err
}

// CanReadMessageInteractions checks the RBAC gate for interaction-scoped
// channel-room reads. It does not test a specific thread relationship. DM
// membership remains the complete DM read boundary. Callers must enforce
// current room membership separately.
func (c *ChattoCore) CanReadMessageInteractions(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.readContentDecision(func() (bool, error) {
		return c.canReadMessageInteractions(ctx, userID, kind, roomID)
	})
}

func (c *ChattoCore) canReadMessageInteractions(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	if kind == KindDM {
		if _, err := c.GetRoom(ctx, kind, roomID); err != nil {
			return false, err
		}
		return true, nil
	}
	decision, err := c.permissionResolver.resolveWithGroup(ctx, userID, kind, roomID, "", PermMessageReadInteractions)
	return decision == DecisionAllow, err
}

// CanAccessRoomMessages reports whether a channel-room account has at least
// one configured read mode. A positive interaction result does not imply that
// any specific thread relationship exists.
func (c *ChattoCore) CanAccessRoomMessages(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.readContentDecision(func() (bool, error) {
		return c.canAccessRoomMessages(ctx, userID, kind, roomID)
	})
}

func (c *ChattoCore) canAccessRoomMessages(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	broad, err := c.canReadMessages(ctx, userID, kind, roomID)
	if err != nil || broad || kind == KindDM {
		return broad, err
	}
	return c.canReadMessageInteractions(ctx, userID, kind, roomID)
}

// CanReadThreadMessages reports whether the account can read one complete
// thread. Callers must enforce current room membership separately.
func (c *ChattoCore) CanReadThreadMessages(ctx context.Context, userID string, kind RoomKind, roomID, threadRootEventID string) (bool, error) {
	return c.readContentDecision(func() (bool, error) {
		return c.canReadThreadMessages(ctx, userID, kind, roomID, threadRootEventID)
	})
}

func (c *ChattoCore) canReadThreadMessages(ctx context.Context, userID string, kind RoomKind, roomID, threadRootEventID string) (bool, error) {
	broad, err := c.canReadMessages(ctx, userID, kind, roomID)
	if err != nil || broad || kind == KindDM {
		return broad, err
	}
	interactions, err := c.canReadMessageInteractions(ctx, userID, kind, roomID)
	if err != nil || !interactions {
		return false, err
	}
	return c.roomModel.hasThreadInteraction(userID, roomID, threadRootEventID), nil
}

// CanReadMessage reports whether the account can read one channel-room
// message. Roots, replies, and channel echoes use their canonical thread root.
// Callers must enforce current room membership separately.
func (c *ChattoCore) CanReadMessage(ctx context.Context, userID string, kind RoomKind, roomID, messageEventID string) (bool, error) {
	return c.readContentDecision(func() (bool, error) {
		return c.canReadMessage(ctx, userID, kind, roomID, messageEventID)
	})
}

func (c *ChattoCore) canReadMessage(ctx context.Context, userID string, kind RoomKind, roomID, messageEventID string) (bool, error) {
	broad, err := c.canReadMessages(ctx, userID, kind, roomID)
	if err != nil || broad || kind == KindDM {
		return broad, err
	}
	interactions, err := c.canReadMessageInteractions(ctx, userID, kind, roomID)
	if err != nil || !interactions {
		return false, err
	}
	rootID, ok := c.roomModel.threadRootForMessage(roomID, messageEventID)
	return ok && c.roomModel.hasThreadInteraction(userID, roomID, rootID), nil
}

// CanReadMessageEvent reports whether the account can receive one durable
// message-derived fact. Callers must enforce current room membership.
func (c *ChattoCore) CanReadMessageEvent(ctx context.Context, userID string, kind RoomKind, roomID string, event *evtv1.Event) (bool, error) {
	return c.readContentDecision(func() (bool, error) {
		return c.canReadMessageEvent(ctx, userID, kind, roomID, event)
	})
}

func (c *ChattoCore) canReadMessageEvent(ctx context.Context, userID string, kind RoomKind, roomID string, event *evtv1.Event) (bool, error) {
	broad, err := c.canReadMessages(ctx, userID, kind, roomID)
	if err != nil || broad || kind == KindDM {
		return broad, err
	}
	interactions, err := c.canReadMessageInteractions(ctx, userID, kind, roomID)
	if err != nil || !interactions {
		return false, err
	}
	rootID, ok := c.MessageEventThreadRoot(roomID, event)
	return ok && c.roomModel.hasThreadInteraction(userID, roomID, rootID), nil
}

// CanPostMessage checks if a user can post new root messages in a specific room.
// Uses room-level permission resolution (checks room overrides, then server defaults).
func (c *ChattoCore) CanPostMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessagePost)
}

// CanPostInThread checks if a user can post messages in a thread.
// Threads are a channel-room-only capability; the room-kind invariant applies
// even to effective owners before room-level permission resolution.
func (c *ChattoCore) CanPostInThread(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	if kind == KindDM {
		return false, nil
	}
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessagePostInThread)
}

// CanAttachFiles checks if a user can attach files to messages in a specific room.
func (c *ChattoCore) CanAttachFiles(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageAttach)
}

// CanReactToMessage checks if a user can add/remove reactions in a specific room.
func (c *ChattoCore) CanReactToMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageReact)
}

// CanEchoMessage checks if a user can echo thread replies to the main channel.
func (c *ChattoCore) CanEchoMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageEcho)
}

// CanManageOthersMessage checks if a user has effective message.manage in a
// specific room. This permission allows edits and deletions of other users'
// messages. It also lets an author edit their own message after the edit
// window closes.
func (c *ChattoCore) CanManageOthersMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageManage)
}
