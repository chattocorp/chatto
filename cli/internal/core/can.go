package core

import (
	"context"
	"time"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
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
func (a *AuthorizationModel) CanAdminUsersView(ctx context.Context, userID string) (bool, error) {
	return a.HasServerPermission(ctx, userID, PermAdminUsersView)
}

// CanAssignRoles checks if a user can assign/revoke roles to/from users.
// Backed by the canonical role.assign permission. Subsumes the previous
// CanAdminUsersManage (which was a duplicate "edit role assignments").
func (a *AuthorizationModel) CanAssignRoles(ctx context.Context, userID string) (bool, error) {
	return a.HasServerPermission(ctx, userID, PermRoleAssign)
}

// CanManageRoles checks if a user can create, edit, delete, and reorder
// roles and their permissions. Subsumes the previous CanAdminRolesManage /
// CanSpaceRolesManage pair (which were duplicates).
func (a *AuthorizationModel) CanManageRoles(ctx context.Context, userID string) (bool, error) {
	return a.HasServerPermission(ctx, userID, PermRoleManage)
}

// CanAdminSystemView checks if a user can view system projection diagnostics
// in admin. The diagnostics endpoint exposes low-level system state and is
// owner-only, so this mirrors GetAdminDiagnostics instead of any grantable
// RBAC permission.
func (a *AuthorizationModel) CanAdminSystemView(ctx context.Context, userID string) (bool, error) {
	isOwner, err := a.IsServerOwner(ctx, userID)
	if err != nil || !isOwner {
		return isOwner, err
	}
	return privilegedModeAllows(ctx, userID, time.Now()), nil
}

// CanAdminAuditView checks if a user can view the audit log (event log)
// page in admin. The event-log inspection view in /manage/server/event-log
// is the first concrete use; future log exports / search endpoints gate
// on the same permission.
func (a *AuthorizationModel) CanAdminAuditView(ctx context.Context, userID string) (bool, error) {
	return a.HasServerPermission(ctx, userID, PermAdminAuditView)
}

// CanManageUserPermissions checks if a user can edit direct per-user
// permission overrides.
func (a *AuthorizationModel) CanManageUserPermissions(ctx context.Context, userID string) (bool, error) {
	return a.HasServerPermission(ctx, userID, PermUserManagePermissions)
}

// CanManageUserAccounts checks if a user can perform cross-user account
// lifecycle and recovery operations.
func (a *AuthorizationModel) CanManageUserAccounts(ctx context.Context, userID string) (bool, error) {
	return a.HasServerPermission(ctx, userID, PermUserManageAccounts)
}

// CanCreateBots checks whether a human user may create an owned bot account.
func (a *AuthorizationModel) CanCreateBots(ctx context.Context, userID string) (bool, error) {
	return a.HasServerPermission(ctx, userID, PermBotCreate)
}

// CanManageBots checks whether a human user may manage every bot account.
func (a *AuthorizationModel) CanManageBots(ctx context.Context, userID string) (bool, error) {
	return a.HasServerPermission(ctx, userID, PermBotManage)
}

// CanStartDM checks if a human user can start DM conversations. Bot accounts
// can never start or fetch DMs through the creation operation, regardless of
// their permissions. The direct-message-scoped message.post decision controls
// creation and inherits the server decision when no DM override exists.
func (a *AuthorizationModel) CanStartDM(ctx context.Context, userID string) (bool, error) {
	if a.view == nil {
		return a.canStartDM(ctx, userID)
	}
	var allowed bool
	err := a.view.ReadContext(ctx, func(readCtx context.Context, _ uint64) error {
		var checkErr error
		allowed, checkErr = a.canStartDM(readCtx, userID)
		return checkErr
	})
	return allowed, err
}

func (a *AuthorizationModel) canStartDM(ctx context.Context, userID string) (bool, error) {
	_, _, accountExists := a.users.isBotAndOwner(userID)
	if !accountExists {
		return false, ErrNotFound
	}
	decision, err := a.resolver.resolveWithGroup(ctx, userID, KindDM, "", "", PermMessagePost)
	if err != nil {
		return false, err
	}
	return decision == DecisionAllow, nil
}

// CanDeleteUser checks if an actor can delete a specific user account.
// Returns true if:
//   - The actor is deleting their own account and has user.delete-self, OR
//   - The actor has user.delete-any (the admin power).
func (a *AuthorizationModel) CanDeleteUser(ctx context.Context, actorID, targetUserID string) (bool, error) {
	if actorID == targetUserID {
		return a.HasServerPermission(ctx, actorID, PermUserDeleteSelf)
	}

	return a.HasServerPermission(ctx, actorID, PermUserDeleteAny)
}

// ============================================================================
// Server-tier Admin Permissions
// ============================================================================

// adminPermissions is the set of admin-level server permissions.
// Used by HasAnyAdminPermission to determine "should the Admin link appear".
var adminPermissions = []Permission{
	PermServerManage,
	PermUserManageInvites,
	PermRoleManage,
	PermRoleAssign,
	PermRoomManage,
	PermRoomMemberRemove,
	PermUserDeleteAny,
	PermUserManageAccounts,
	PermUserManagePermissions,
	PermAdminUsersView,
	PermAdminAuditView,
	PermBotManage,
}

// HasAnyAdminPermission checks if a user has any admin-level permission.
// Used to determine whether the server admin link should be visible.
func (a *AuthorizationModel) HasAnyAdminPermission(ctx context.Context, userID string) (bool, error) {
	if a.view == nil {
		return a.hasAnyAdminPermission(ctx, userID)
	}
	var allowed bool
	err := a.view.ReadContext(ctx, func(readCtx context.Context, _ uint64) error {
		var checkErr error
		allowed, checkErr = a.hasAnyAdminPermission(readCtx, userID)
		return checkErr
	})
	return allowed, err
}

func (a *AuthorizationModel) hasAnyAdminPermission(ctx context.Context, userID string) (bool, error) {
	for _, perm := range adminPermissions {
		decision, err := a.resolver.resolveEntitlementWithGroup(ctx, userID, KindChannel, "", "", perm)
		if err != nil {
			return false, err
		}
		if decision == DecisionAllow {
			return true, nil
		}
	}
	return false, nil
}

// HasAnyPrivilegedModeEntitlement reports whether a user has at least one
// permission entitlement that an authenticated human session must activate.
// It intentionally ignores the current session state. The any-scope fallback
// detects explicit resource-scoped allows for discovery; effective checks can
// still resolve a deny.
func (a *AuthorizationModel) HasAnyPrivilegedModeEntitlement(ctx context.Context, userID string) (bool, error) {
	for _, metadata := range AllPermissions() {
		if !metadata.RequiresPrivilegedMode || !permissionMetadataHasScope(metadata, ScopeServer) {
			continue
		}
		decision, err := a.resolver.resolveEntitlementWithGroup(ctx, userID, KindChannel, "", "", metadata.Permission)
		if err != nil {
			return false, err
		}
		if decision == DecisionAllow {
			return true, nil
		}
	}
	return a.rbac.hasAnyPrivilegedModeAllow(userID), nil
}

// CanManageServer checks if a user can update server settings (name, description, logo).
func (a *AuthorizationModel) CanManageServer(ctx context.Context, userID string) (bool, error) {
	return a.hasServerPermission(ctx, userID, PermServerManage)
}

// CanManageAnyRoom checks if a user can update or delete any room.
// "Any" room as opposed to a specific room — for per-room checks, use the
// room-level resolver via PermissionResolver.HasRoomPermission.
func (a *AuthorizationModel) CanManageAnyRoom(ctx context.Context, userID string) (bool, error) {
	return a.hasServerPermission(ctx, userID, PermRoomManage)
}

// CanManageRoomGroup checks whether a user can manage room/sidebar layout
// facts owned by a specific room group. Server-scope room.manage still applies
// through the group permission resolver; role.manage is intentionally not a
// substitute for this group-scoped capability.
func (a *AuthorizationModel) CanManageRoomGroup(ctx context.Context, userID, groupID string) (bool, error) {
	return a.hasGroupPermission(ctx, KindChannel, groupID, userID, PermRoomManage)
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
func (a *AuthorizationModel) CanSeeRoom(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return a.readContentDecision(ctx, func(readCtx context.Context) (bool, error) {
		return a.canSeeRoom(readCtx, userID, kind, roomID)
	})
}

func (a *AuthorizationModel) canSeeRoom(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	if kind == KindDM {
		return false, nil
	}
	if a.rooms.hasExplicitRoomMembership(roomID, userID) {
		return true, nil
	}
	room, err := a.rooms.GetRoom(ctx, kind, roomID)
	if err != nil {
		return false, err
	}
	if room.GetUniversal() {
		canJoin, joinErr := a.canJoinRoomAt(ctx, userID, kind, roomID)
		if joinErr != nil || canJoin {
			return canJoin, joinErr
		}
	}
	allowed, err := a.resolver.resolveWithGroup(ctx, userID, kind, roomID, "", PermRoomList)
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
// creation-locked at this layer because room.create does not apply at the DM
// scope. DMs are created through FindOrCreateDM.
func (a *AuthorizationModel) CanCreateRoom(ctx context.Context, userID string, kind RoomKind, groupID string) (bool, error) {
	if kind == KindChannel && groupID != "" {
		return a.hasGroupPermission(ctx, kind, groupID, userID, PermRoomCreate)
	}
	return a.hasKindPermission(ctx, kind, userID, PermRoomCreate)
}

// CanJoinRoom checks if a user can join existing rooms at the server tier
// (no specific room context). Used as a top-level "is the join action
// available at all" check. For per-room decisions — including "is this
// user implicitly a member of this global room" — use CanJoinRoomAt,
// which evaluates room, group, and server decisions.
//
// DM-sensitive: DMs grant join implicitly to participants.
func (a *AuthorizationModel) CanJoinRoom(ctx context.Context, userID string, kind RoomKind) (bool, error) {
	if kind == KindDM {
		return true, nil
	}
	decision, err := a.ResolveUserPermission(ctx, userID, kind, "", PermRoomJoin)
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
func (a *AuthorizationModel) CanJoinRoomAt(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return a.readContentDecision(ctx, func(readCtx context.Context) (bool, error) {
		return a.canJoinRoomAt(readCtx, userID, kind, roomID)
	})
}

func (a *AuthorizationModel) canJoinRoomAt(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	if kind == KindChannel && a.rooms.isRoomBanActive(roomID, userID, time.Now()) {
		return false, nil
	}
	decision, err := a.resolver.resolveWithGroup(ctx, userID, kind, roomID, "", PermRoomJoin)
	return decision == DecisionAllow, err
}

// ============================================================================
// Room-Scoped Permissions
// ============================================================================

// CanReadMessages checks the permission part of message-content access in a
// specific room. Callers must enforce room membership for both room kinds.
func (a *AuthorizationModel) CanReadMessages(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return a.readContentDecision(ctx, func(readCtx context.Context) (bool, error) {
		return a.canReadMessages(readCtx, userID, kind, roomID)
	})
}

func (a *AuthorizationModel) canReadMessages(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	if roomID != "" {
		if room, err := a.rooms.FindRoomByID(ctx, roomID); err == nil && KindOfRoom(room) != kind {
			return false, ErrNotFound
		}
	}
	decision, err := a.resolver.resolveWithGroup(ctx, userID, kind, roomID, "", PermMessageRead)
	return decision == DecisionAllow, err
}

// CanReadMessageInteractions checks the RBAC gate for interaction-scoped
// reads. It does not test a specific thread relationship. Callers must enforce
// current room membership separately.
func (a *AuthorizationModel) CanReadMessageInteractions(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return a.readContentDecision(ctx, func(readCtx context.Context) (bool, error) {
		return a.canReadMessageInteractions(readCtx, userID, kind, roomID)
	})
}

func (a *AuthorizationModel) canReadMessageInteractions(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	decision, err := a.resolver.resolveWithGroup(ctx, userID, kind, roomID, "", PermMessageReadInteractions)
	return decision == DecisionAllow, err
}

// CanAccessRoomMessages reports whether a channel-room account has at least
// one configured read mode. A positive interaction result does not imply that
// any specific thread relationship exists.
func (a *AuthorizationModel) CanAccessRoomMessages(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return a.readContentDecision(ctx, func(readCtx context.Context) (bool, error) {
		return a.canAccessRoomMessages(readCtx, userID, kind, roomID)
	})
}

func (a *AuthorizationModel) canAccessRoomMessages(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	broad, err := a.canReadMessages(ctx, userID, kind, roomID)
	if err != nil || broad {
		return broad, err
	}
	return a.canReadMessageInteractions(ctx, userID, kind, roomID)
}

// CanReadThreadMessages reports whether the account can read one complete
// thread. Callers must enforce current room membership separately.
func (a *AuthorizationModel) CanReadThreadMessages(ctx context.Context, userID string, kind RoomKind, roomID, threadRootEventID string) (bool, error) {
	return a.readContentDecision(ctx, func(readCtx context.Context) (bool, error) {
		return a.canReadThreadMessages(readCtx, userID, kind, roomID, threadRootEventID)
	})
}

func (a *AuthorizationModel) canReadThreadMessages(ctx context.Context, userID string, kind RoomKind, roomID, threadRootEventID string) (bool, error) {
	broad, err := a.canReadMessages(ctx, userID, kind, roomID)
	if err != nil || broad {
		return broad, err
	}
	interactions, err := a.canReadMessageInteractions(ctx, userID, kind, roomID)
	if err != nil || !interactions {
		return false, err
	}
	return a.rooms.hasThreadInteraction(userID, roomID, threadRootEventID), nil
}

// CanReadMessage reports whether the account can read one channel-room
// message. Roots, replies, and channel echoes use their canonical thread root.
// Callers must enforce current room membership separately.
func (a *AuthorizationModel) CanReadMessage(ctx context.Context, userID string, kind RoomKind, roomID, messageEventID string) (bool, error) {
	return a.readContentDecision(ctx, func(readCtx context.Context) (bool, error) {
		return a.canReadMessage(readCtx, userID, kind, roomID, messageEventID)
	})
}

func (a *AuthorizationModel) canReadMessage(ctx context.Context, userID string, kind RoomKind, roomID, messageEventID string) (bool, error) {
	broad, err := a.canReadMessages(ctx, userID, kind, roomID)
	if err != nil || broad {
		return broad, err
	}
	interactions, err := a.canReadMessageInteractions(ctx, userID, kind, roomID)
	if err != nil || !interactions {
		return false, err
	}
	rootID, ok := a.rooms.threadRootForMessage(roomID, messageEventID)
	return ok && a.rooms.hasThreadInteraction(userID, roomID, rootID), nil
}

// CanReadMessageEvent reports whether the account can receive one durable
// message-derived fact. Callers must enforce current room membership.
func (a *AuthorizationModel) CanReadMessageEvent(ctx context.Context, userID string, kind RoomKind, roomID string, event *evtv1.Event) (bool, error) {
	return a.readContentDecision(ctx, func(readCtx context.Context) (bool, error) {
		return a.canReadMessageEvent(readCtx, userID, kind, roomID, event)
	})
}

func (a *AuthorizationModel) canReadMessageEvent(ctx context.Context, userID string, kind RoomKind, roomID string, event *evtv1.Event) (bool, error) {
	broad, err := a.canReadMessages(ctx, userID, kind, roomID)
	if err != nil || broad {
		return broad, err
	}
	interactions, err := a.canReadMessageInteractions(ctx, userID, kind, roomID)
	if err != nil || !interactions {
		return false, err
	}
	rootID, ok := a.messages.MessageEventThreadRoot(roomID, event)
	return ok && a.rooms.hasThreadInteraction(userID, roomID, rootID), nil
}

// CanPostMessage checks if a user can post new root messages in a specific room.
// Uses the permission scope chain for the specified room kind.
func (a *AuthorizationModel) CanPostMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return a.hasRoomPermission(ctx, kind, roomID, userID, PermMessagePost)
}

// CanPostInThread checks room-level authority to reply in any readable thread.
// It includes message.post but not relationship-scoped posting. For a specific
// target, use CanReplyInThread, which also checks access and room policy.
func (a *AuthorizationModel) CanPostInThread(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return a.hasRoomPermission(ctx, kind, roomID, userID, PermMessagePostInThread)
}

// CanReplyInThread checks current membership, room policy, read access, and
// posting authority for one canonical thread root. Interaction posting needs
// an existing relationship; broad read access alone cannot create one.
func (a *AuthorizationModel) CanReplyInThread(ctx context.Context, userID string, kind RoomKind, roomID, threadRootEventID string) (bool, error) {
	return a.readContentDecision(ctx, func(readCtx context.Context) (bool, error) {
		room, err := a.rooms.GetRoom(readCtx, kind, roomID)
		if err != nil {
			return false, err
		}
		if room.Archived || EffectiveRoomThreadingMode(room) == evtv1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED {
			return false, nil
		}
		member, err := a.roomMembershipExists(readCtx, kind, userID, roomID)
		if err != nil || !member {
			return false, err
		}
		readable, err := a.canReadThreadMessages(readCtx, userID, kind, roomID, threadRootEventID)
		if err != nil || !readable {
			return false, err
		}
		broad, err := a.CanPostInThread(readCtx, userID, kind, roomID)
		if err != nil || broad {
			return broad, err
		}
		narrow, err := a.hasRoomPermission(readCtx, kind, roomID, userID, PermMessagePostInInteractions)
		if err != nil || !narrow {
			return false, err
		}
		return a.rooms.hasThreadInteraction(userID, roomID, threadRootEventID), nil
	})
}

// CanAttachFiles checks if a user can attach files to messages in a specific room.
func (a *AuthorizationModel) CanAttachFiles(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return a.hasRoomPermission(ctx, kind, roomID, userID, PermMessageAttach)
}

// CanReactToMessage checks if a user can add/remove reactions in a specific room.
func (a *AuthorizationModel) CanReactToMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return a.hasRoomPermission(ctx, kind, roomID, userID, PermMessageReact)
}

// CanEchoMessage checks if a user can echo thread replies to the room timeline.
func (a *AuthorizationModel) CanEchoMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return a.hasRoomPermission(ctx, kind, roomID, userID, PermMessageEcho)
}

// CanManageOthersMessage checks if a user has effective message.manage in a
// specific room. This permission allows edits and deletions of other users'
// messages. It also lets an author edit their own message after the edit
// window closes.
func (a *AuthorizationModel) CanManageOthersMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return a.hasRoomPermission(ctx, kind, roomID, userID, PermMessageManage)
}
