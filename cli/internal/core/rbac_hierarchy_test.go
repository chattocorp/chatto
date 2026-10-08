package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestRBACProjection_LegacyAndCompleteRoleOrders(t *testing.T) {
	t.Parallel()

	p := NewRBACProjection()
	for _, role := range []struct {
		name string
		rank int32
	}{{RoleModerator, PositionModerator}, {RoleAdmin, PositionAdmin}, {"alpha", 10}, {"beta", 20}} {
		applyRBACProjectionEvent(t, p, &evtv1.Event{Event: &evtv1.Event_RbacRoleCreated{
			RbacRoleCreated: &evtv1.RbacRoleCreatedEvent{RoleName: role.name, Rank: role.rank},
		}})
	}
	positions := func() map[string]int32 {
		result := map[string]int32{}
		for _, role := range p.ListRoles() {
			result[role.GetName()] = role.GetPosition()
		}
		return result
	}

	// Legacy orders list custom roles only and skip the fixed system positions.
	applyRBACProjectionEvent(t, p, &evtv1.Event{Event: &evtv1.Event_RbacRolesReordered{
		RbacRolesReordered: &evtv1.RbacRolesReorderedEvent{RoleNames: []string{"beta", RoleAdmin, "alpha"}},
	}})
	if got, want := positions(), (map[string]int32{"beta": 1, "alpha": 2, RoleModerator: PositionModerator, RoleAdmin: PositionAdmin}); !mapsEqual(got, want) {
		t.Fatalf("legacy positions = %v, want %v", got, want)
	}

	// A move ranks system and custom roles together and renumbers them.
	move := func(roleName, before string) {
		t.Helper()
		applyRBACProjectionEvent(t, p, &evtv1.Event{Event: &evtv1.Event_RbacRoleMoved{
			RbacRoleMoved: &evtv1.RbacRoleMovedEvent{RoleName: roleName, BeforeRoleName: before},
		}})
	}
	move(RoleModerator, "alpha")
	if got, want := positions(), (map[string]int32{"beta": 1, "alpha": 2, RoleModerator: 3, RoleAdmin: 4}); !mapsEqual(got, want) {
		t.Fatalf("positions after move = %v, want %v", got, want)
	}
	move(RoleAdmin, "")
	move("beta", RoleModerator)
	if got, want := positions(), (map[string]int32{RoleAdmin: 1, "alpha": 2, RoleModerator: 3, "beta": 4}); !mapsEqual(got, want) {
		t.Fatalf("positions after moves = %v, want %v", got, want)
	}
	// Moves that refer to unknown or fixed roles change nothing.
	move("missing", "")
	move("alpha", "missing")
	move(RoleEveryone, "")
	if got, want := positions(), (map[string]int32{RoleAdmin: 1, "alpha": 2, RoleModerator: 3, "beta": 4}); !mapsEqual(got, want) {
		t.Fatalf("positions after ignored moves = %v, want %v", got, want)
	}

	// A new role placed lowest renumbers the order.
	applyRBACProjectionEvent(t, p, &evtv1.Event{Event: &evtv1.Event_RbacRoleCreated{
		RbacRoleCreated: &evtv1.RbacRoleCreatedEvent{RoleName: "gamma", Rank: 99, PlaceLowest: true},
	}})
	if got, want := positions(), (map[string]int32{"gamma": 1, RoleAdmin: 2, "alpha": 3, RoleModerator: 4, "beta": 5}); !mapsEqual(got, want) {
		t.Fatalf("positions after lowest creation = %v, want %v", got, want)
	}
}

func TestRBACProjection_RoleOrderHasNoFixedLimit(t *testing.T) {
	t.Parallel()

	p := NewRBACProjection()
	for _, role := range []struct {
		name string
		rank int32
	}{{RoleOwner, PositionOwner}, {RoleEveryone, PositionEveryone}, {RoleAdmin, PositionAdmin}} {
		applyRBACProjectionEvent(t, p, &evtv1.Event{Event: &evtv1.Event_RbacRoleCreated{
			RbacRoleCreated: &evtv1.RbacRoleCreatedEvent{RoleName: role.name, Rank: role.rank},
		}})
	}
	const count = 1200
	for i := range count {
		applyRBACProjectionEvent(t, p, &evtv1.Event{Event: &evtv1.Event_RbacRoleCreated{
			RbacRoleCreated: &evtv1.RbacRoleCreatedEvent{RoleName: fmt.Sprintf("role-%d", i), PlaceLowest: true},
		}})
	}
	roles := p.ListRoles()
	if len(roles) != count+3 {
		t.Fatalf("roles = %d, want %d", len(roles), count+3)
	}
	if first, last := roles[0].GetName(), roles[len(roles)-1]; first != RoleEveryone || last.GetName() != RoleOwner {
		t.Fatalf("order = %s ... %s, want everyone ... owner", first, last.GetName())
	}
	if admin, ok := p.GetRole(RoleAdmin); !ok || admin.GetPosition() != count+1 {
		t.Fatalf("admin position = %d, want %d", admin.GetPosition(), count+1)
	}
	if owner, _ := p.GetRole(RoleOwner); owner.GetPosition() != count+2 {
		t.Fatalf("owner position = %d, want %d", owner.GetPosition(), count+2)
	}
}

func mapsEqual(got, want map[string]int32) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

// hierarchyFixture is a server with an owner, two admins, a moderator, and a
// member who all share one channel room.
type hierarchyFixture struct {
	c                                          *ChattoCore
	ctx                                        context.Context
	owner, admin, peerAdmin, moderator, member string
	roomID                                     string
}

func newHierarchyFixture(t *testing.T) *hierarchyFixture {
	t.Helper()
	c, _ := setupTestCore(t)
	f := &hierarchyFixture{c: c, ctx: testContext(t)}
	create := func(login, role string) string {
		user, err := c.CreateUser(f.ctx, SystemActorID, login, login, "password123")
		if err != nil {
			t.Fatalf("CreateUser %s: %v", login, err)
		}
		if role != "" {
			if err := c.AssignServerRole(f.ctx, SystemActorID, user.Id, role); err != nil {
				t.Fatalf("AssignServerRole %s: %v", login, err)
			}
		}
		return user.Id
	}
	f.owner = create("hierarchy-owner", RoleOwner)
	f.admin = create("hierarchy-admin", RoleAdmin)
	f.peerAdmin = create("hierarchy-peer-admin", RoleAdmin)
	f.moderator = create("hierarchy-moderator", RoleModerator)
	f.member = create("hierarchy-member", "")
	f.roomID = createPermissionEditRoom(t, c, f.ctx, "hierarchy-room")
	for _, userID := range []string{f.owner, f.admin, f.peerAdmin, f.moderator, f.member} {
		if _, err := c.JoinRoom(f.ctx, userID, KindChannel, userID, f.roomID); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	return f
}

func TestHierarchyProtectsAccountsAtOrAboveTheActor(t *testing.T) {
	t.Parallel()

	f := newHierarchyFixture(t)
	c, ctx := f.c, f.ctx
	bio := "Changed by an administrator."
	operations := []struct {
		name string
		run  func(actor, target string) error
	}{
		{"set password", func(actor, target string) error {
			return c.AdminSetUserPasswordAuthorized(ctx, actor, target, "replacement-password-123")
		}},
		{"edit profile", func(actor, target string) error {
			_, err := c.UpdateManagedUserProfile(ctx, actor, target, nil, nil, &bio)
			return err
		}},
		{"clear login cooldown", func(actor, target string) error {
			return c.AdminClearLoginChangeCooldown(ctx, actor, target)
		}},
		{"delete account", func(actor, target string) error {
			allowed, err := c.CanDeleteUser(ctx, actor, target)
			if err == nil && !allowed {
				err = ErrPermissionDenied
			}
			return err
		}},
		{"assign role", func(actor, target string) error {
			return c.AdminAssignServerRole(ctx, actor, target, RoleModerator)
		}},
		{"edit direct permissions", func(actor, target string) error {
			return c.SetUserPermissionState(ctx, actor, target, PermissionTargetScope{Kind: MatrixScopeRoom, ID: f.roomID}, PermMessageReact, PermissionStateDeny)
		}},
		{"remove from room", func(actor, target string) error {
			return c.RoomCommands().RemoveUser(ctx, RoomRemoveUserInput{ActorID: actor, RoomID: f.roomID, UserID: target, Reason: "test"})
		}},
	}
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			for _, target := range []string{f.peerAdmin, f.owner} {
				if err := op.run(f.admin, target); !errors.Is(err, ErrPermissionDenied) {
					t.Fatalf("admin on %s: error = %v, want permission denied", target, err)
				}
			}
			if err := op.run(f.admin, f.member); err != nil {
				t.Fatalf("admin on member: %v", err)
			}
		})
	}

	t.Run("owners act on everyone", func(t *testing.T) {
		if err := c.AdminSetUserPasswordAuthorized(ctx, f.owner, f.admin, "owner-set-password-123"); err != nil {
			t.Fatalf("owner sets admin password: %v", err)
		}
		if err := c.AdminRevokeServerRole(ctx, f.owner, f.peerAdmin, RoleAdmin); err != nil {
			t.Fatalf("owner revokes admin: %v", err)
		}
	})
}

func TestHierarchyBoundsRoomModeration(t *testing.T) {
	t.Parallel()

	f := newHierarchyFixture(t)
	c, ctx := f.c, f.ctx
	commands := c.RoomCommands()
	remove := func(actor, target string) error {
		return commands.RemoveUser(ctx, RoomRemoveUserInput{ActorID: actor, RoomID: f.roomID, UserID: target, Reason: "test", Suspension: true})
	}
	if err := remove(f.moderator, f.admin); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("moderator suspends admin: error = %v, want permission denied", err)
	}
	if err := remove(f.owner, f.admin); err != nil {
		t.Fatalf("owner suspends admin: %v", err)
	}
	if err := commands.LiftSuspension(ctx, RoomUnbanInput{ActorID: f.moderator, RoomID: f.roomID, UserID: f.admin, Reason: "test"}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("moderator lifts admin suspension: error = %v, want permission denied", err)
	}
	if err := remove(f.moderator, f.member); err != nil {
		t.Fatalf("moderator suspends member: %v", err)
	}
	if err := commands.LiftSuspension(ctx, RoomUnbanInput{ActorID: f.moderator, RoomID: f.roomID, UserID: f.member, Reason: "test"}); err != nil {
		t.Fatalf("moderator lifts member suspension: %v", err)
	}
}

func TestHierarchyLimitsRoleManagementToLowerRoles(t *testing.T) {
	t.Parallel()

	f := newHierarchyFixture(t)
	c, ctx := f.c, f.ctx
	if _, err := c.CreateServerRole(ctx, SystemActorID, "senior", "Senior", ""); err != nil {
		t.Fatalf("CreateServerRole senior: %v", err)
	}
	if _, err := c.CreateServerRole(ctx, SystemActorID, "junior", "Junior", ""); err != nil {
		t.Fatalf("CreateServerRole junior: %v", err)
	}
	// The owner places senior above admin. Junior, created last, is lowest.
	if _, err := c.MoveServerRole(ctx, f.owner, "senior", RoleAdmin); err != nil {
		t.Fatalf("owner moves senior: %v", err)
	}

	update := func(roleName string) error {
		_, err := c.AdminUpdateServerRole(ctx, f.admin, AdminRoleUpdateInput{Name: roleName, DisplayName: new("Renamed " + roleName)})
		return err
	}
	for _, roleName := range []string{RoleAdmin, "senior"} {
		if err := update(roleName); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("admin updates %s: error = %v, want permission denied", roleName, err)
		}
		if err := c.SetRolePermissionState(ctx, f.admin, roleName, PermissionTargetScope{Kind: MatrixScopeServer}, PermMessageReact, PermissionStateDeny); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("admin edits %s permissions: error = %v, want permission denied", roleName, err)
		}
		if err := c.AdminAssignServerRole(ctx, f.admin, f.member, roleName); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("admin assigns %s: error = %v, want permission denied", roleName, err)
		}
	}
	if err := c.AdminDeleteServerRole(ctx, f.admin, "senior"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("admin deletes senior: error = %v, want permission denied", err)
	}
	if err := update(RoleModerator); err != nil {
		t.Fatalf("admin updates moderator: %v", err)
	}
	if err := c.AdminAssignServerRole(ctx, f.admin, f.member, "junior"); err != nil {
		t.Fatalf("admin assigns junior: %v", err)
	}

	t.Run("role managers move every role", func(t *testing.T) {
		if _, err := c.AdminMoveServerRole(ctx, f.member, "junior", ""); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("member without role.manage moves junior: error = %v, want permission denied", err)
		}
		for _, move := range []struct{ role, before string }{
			{"senior", ""},            // a role above the actor
			{RoleAdmin, "senior"},     // the actor's own role
			{"junior", RoleAdmin},     // to a place above the actor
			{RoleModerator, "junior"}, // to the top
		} {
			if _, err := c.AdminMoveServerRole(ctx, f.admin, move.role, move.before); err != nil {
				t.Fatalf("admin moves %s above %q: %v", move.role, move.before, err)
			}
		}
		if got, want := c.orderableRoleNames(), []string{"senior", RoleAdmin, "junior", RoleModerator}; !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})
}

func TestBotsHoldRolesWithinTheirOwnersCeiling(t *testing.T) {
	t.Parallel()

	f := newHierarchyFixture(t)
	c, ctx := f.c, f.ctx
	allowBotCreation(t, ctx, c, f.member)
	bot, err := c.CreateBot(ctx, f.member, "hierarchy_bot", "Hierarchy Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	botID := bot.User.GetId()
	if _, err := c.CreateServerRole(ctx, SystemActorID, "helpers", "Helpers", ""); err != nil {
		t.Fatalf("CreateServerRole helpers: %v", err)
	}
	for _, perm := range []Permission{PermMessagePost, PermRoomCreate} {
		if err := c.GrantServerPermission(ctx, SystemActorID, "helpers", perm); err != nil {
			t.Fatalf("GrantServerPermission %s: %v", perm, err)
		}
	}
	resolve := func(perm Permission) DecisionKind {
		t.Helper()
		decision, err := c.PermResolver().Resolve(ctx, botID, KindChannel, "", perm)
		if err != nil {
			t.Fatalf("Resolve %s: %v", perm, err)
		}
		return decision
	}

	if err := c.AdminAssignServerRole(ctx, f.admin, botID, "helpers"); err != nil {
		t.Fatalf("admin assigns helpers to bot: %v", err)
	}
	if got := resolve(PermMessagePost); got != DecisionAllow {
		t.Fatalf("bot message.post from role = %s, want allow", got)
	}
	if got := resolve(PermRoomCreate); got != DecisionDeny {
		t.Fatalf("bot room.create beyond owner ceiling = %s, want deny", got)
	}
	if err := c.AdminAssignServerRole(ctx, f.owner, botID, RoleOwner); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("assign owner to bot: error = %v, want ErrHumanAccountRequired", err)
	}

	// A restriction role wins over the bot's own allow.
	if _, err := c.CreateServerRole(ctx, SystemActorID, "muted", "Muted", ""); err != nil {
		t.Fatalf("CreateServerRole muted: %v", err)
	}
	if err := c.DenyServerPermission(ctx, SystemActorID, "muted", PermMessagePost); err != nil {
		t.Fatalf("DenyServerPermission muted: %v", err)
	}
	if err := c.SetUserPermissionState(ctx, f.member, botID, PermissionTargetScope{Kind: MatrixScopeServer}, PermMessagePost, PermissionStateAllow); err != nil {
		t.Fatalf("owner allows message.post: %v", err)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, botID, "muted"); err != nil {
		t.Fatalf("AssignServerRole muted: %v", err)
	}
	if got := resolve(PermMessagePost); got != DecisionDeny {
		t.Fatalf("muted bot message.post = %s, want deny", got)
	}
}

func TestActingOnBotsRequiresOutrankingTheirOwners(t *testing.T) {
	t.Parallel()

	f := newHierarchyFixture(t)
	c, ctx := f.c, f.ctx
	if err := c.GrantUserPermission(ctx, SystemActorID, f.moderator, PermBotManage); err != nil {
		t.Fatalf("GrantUserPermission bot.manage: %v", err)
	}
	newBot := func(ownerID, login string) string {
		allowBotCreation(t, ctx, c, ownerID)
		bot, err := c.CreateBot(ctx, ownerID, login, login)
		if err != nil {
			t.Fatalf("CreateBot %s: %v", login, err)
		}
		return bot.User.GetId()
	}
	memberBot := newBot(f.member, "member_bot")
	adminBot := newBot(f.admin, "admin_bot")
	server := PermissionTargetScope{Kind: MatrixScopeServer}
	allowReact := func(actor, botID string) error {
		return c.SetUserPermissionState(ctx, actor, botID, server, PermMessageReact, PermissionStateAllow)
	}

	if err := allowReact(f.moderator, memberBot); err != nil {
		t.Fatalf("moderator manages member's bot: %v", err)
	}
	if err := allowReact(f.moderator, adminBot); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("moderator manages admin's bot: error = %v, want permission denied", err)
	}

	// The owner of a bot manages it even when the bot outranks them.
	if err := c.AssignServerRole(ctx, SystemActorID, memberBot, RoleModerator); err != nil {
		t.Fatalf("AssignServerRole moderator to bot: %v", err)
	}
	if err := allowReact(f.member, memberBot); err != nil {
		t.Fatalf("owner manages their higher-ranked bot: %v", err)
	}
	if err := allowReact(f.moderator, memberBot); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("moderator manages a peer-ranked bot: error = %v, want permission denied", err)
	}

	t.Run("reassignment stays below the actor", func(t *testing.T) {
		if _, err := c.ReassignBotOwner(ctx, f.admin, adminBot, f.peerAdmin); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("admin hands bot to peer admin: error = %v, want permission denied", err)
		}
		if _, err := c.ReassignBotOwner(ctx, f.admin, adminBot, f.moderator); err != nil {
			t.Fatalf("admin hands own bot to moderator: %v", err)
		}
		if _, err := c.ReassignBotOwner(ctx, f.moderator, adminBot, f.admin); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("moderator hands own bot to admin: error = %v, want permission denied", err)
		}
		if _, err := c.ReassignBotOwner(ctx, f.moderator, adminBot, f.member); err != nil {
			t.Fatalf("moderator hands own bot to member: %v", err)
		}
	})
}

func TestHighestRoleDecidesRank(t *testing.T) {
	t.Parallel()

	f := newHierarchyFixture(t)
	if err := f.c.AssignServerRole(f.ctx, SystemActorID, f.moderator, RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	ranks := map[string]int32{}
	for _, userID := range []string{f.owner, f.admin, f.moderator, f.member} {
		ranks[userID] = f.c.accountRank(userID)
	}
	if ranks[f.moderator] != ranks[f.admin] || !slices.IsSorted([]int32{ranks[f.member], ranks[f.admin], ranks[f.owner]}) || ranks[f.member] != PositionEveryone {
		t.Fatalf("ranks = %v, want member < admin = moderator-with-admin < owner", ranks)
	}
	for userID, want := range map[string]string{f.owner: RoleOwner, f.admin: RoleAdmin, f.moderator: RoleAdmin, f.member: RoleEveryone} {
		if got := f.c.ViewerHighestRole(userID); got != want {
			t.Fatalf("ViewerHighestRole(%s) = %q, want %q", userID, got, want)
		}
	}
}

func TestBotsActWithAtMostTheirOwnersRank(t *testing.T) {
	t.Parallel()

	f := newHierarchyFixture(t)
	c, ctx := f.c, f.ctx
	other, err := c.CreateUser(ctx, SystemActorID, "hierarchy-other", "Other", "password123")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	if _, err := c.JoinRoom(ctx, other.Id, KindChannel, other.Id, f.roomID); err != nil {
		t.Fatalf("JoinRoom other: %v", err)
	}
	// The member may remove members in this room, but ranks with everyone.
	if err := c.GrantUserRoomPermission(ctx, SystemActorID, f.roomID, f.member, PermRoomMemberRemove); err != nil {
		t.Fatalf("GrantUserRoomPermission: %v", err)
	}
	allowBotCreation(t, ctx, c, f.member)
	bot, err := c.CreateBot(ctx, f.member, "rank_capped_bot", "Rank Capped Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	botID := bot.User.GetId()
	if err := c.SetUserPermissionState(ctx, f.member, botID, PermissionTargetScope{Kind: MatrixScopeRoom, ID: f.roomID}, PermRoomMemberRemove, PermissionStateAllow); err != nil {
		t.Fatalf("delegate room.remove-member: %v", err)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, botID, RoleModerator); err != nil {
		t.Fatalf("AssignServerRole moderator to bot: %v", err)
	}
	if got := c.ViewerHighestRole(botID); got != RoleEveryone {
		t.Fatalf("ViewerHighestRole(bot) = %q, want its owner's %q", got, RoleEveryone)
	}
	if err := c.RoomCommands().RemoveUser(ctx, RoomRemoveUserInput{ActorID: botID, RoomID: f.roomID, UserID: other.Id, Reason: "test"}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("bot removes a peer of its owner: error = %v, want permission denied", err)
	}
}

func TestBotOwnersCannotChangeRolesOfHigherRankedBots(t *testing.T) {
	t.Parallel()

	f := newHierarchyFixture(t)
	c, ctx := f.c, f.ctx
	grantTestRank(t, c, ctx, f.member)
	if err := c.GrantUserPermission(ctx, SystemActorID, f.member, PermRoleAssign); err != nil {
		t.Fatalf("GrantUserPermission role.assign: %v", err)
	}
	allowBotCreation(t, ctx, c, f.member)
	bot, err := c.CreateBot(ctx, f.member, "restricted_bot", "Restricted Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	botID := bot.User.GetId()
	if _, err := c.CreateServerRole(ctx, SystemActorID, "muted", "Muted", ""); err != nil {
		t.Fatalf("CreateServerRole muted: %v", err)
	}
	for _, roleName := range []string{"muted", RoleAdmin} {
		if err := c.AssignServerRole(ctx, SystemActorID, botID, roleName); err != nil {
			t.Fatalf("AssignServerRole %s: %v", roleName, err)
		}
	}
	if err := c.AdminRevokeServerRole(ctx, f.member, botID, "muted"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("owner revokes restriction from higher-ranked bot: error = %v, want permission denied", err)
	}
	if err := c.AdminRevokeServerRole(ctx, f.admin, botID, "muted"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("admin revokes role from peer-ranked bot: error = %v, want permission denied", err)
	}
	if err := c.AdminRevokeServerRole(ctx, f.owner, botID, "muted"); err != nil {
		t.Fatalf("owner of the server revokes restriction: %v", err)
	}
}

func TestDeletingAUserRequiresOutrankingTheirBots(t *testing.T) {
	t.Parallel()

	f := newHierarchyFixture(t)
	c, ctx := f.c, f.ctx
	if err := c.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermUserDeleteAny); err != nil {
		t.Fatalf("GrantServerPermission user.delete-any: %v", err)
	}
	allowBotCreation(t, ctx, c, f.member)
	bot, err := c.CreateBot(ctx, f.member, "cascade_bot", "Cascade Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	if _, err := c.CreateServerRole(ctx, SystemActorID, "ops", "Ops", ""); err != nil {
		t.Fatalf("CreateServerRole ops: %v", err)
	}
	if _, err := c.MoveServerRole(ctx, SystemActorID, "ops", RoleModerator); err != nil {
		t.Fatalf("MoveServerRole ops: %v", err)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, bot.User.GetId(), "ops"); err != nil {
		t.Fatalf("AssignServerRole ops: %v", err)
	}

	if allowed, err := c.CanDeleteUser(ctx, f.moderator, f.member); err != nil || allowed {
		t.Fatalf("moderator CanDeleteUser(member with higher bot) = %v, %v; want false", allowed, err)
	}
	if err := c.AdminDeleteUserAs(ctx, f.moderator, f.member); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("moderator deletes member with higher bot: error = %v, want permission denied", err)
	}
	if allowed, err := c.CanDeleteUser(ctx, f.admin, f.member); err != nil || !allowed {
		t.Fatalf("admin CanDeleteUser(member) = %v, %v; want true", allowed, err)
	}
}

func TestBotManagersGrantOnlyWhatTheyHold(t *testing.T) {
	t.Parallel()

	f := newHierarchyFixture(t)
	c, ctx := f.c, f.ctx
	if err := c.GrantUserPermission(ctx, SystemActorID, f.moderator, PermBotManage); err != nil {
		t.Fatalf("GrantUserPermission bot.manage: %v", err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, f.member, PermRoomCreate); err != nil {
		t.Fatalf("GrantUserPermission room.create: %v", err)
	}
	allowBotCreation(t, ctx, c, f.member)
	bot, err := c.CreateBot(ctx, f.member, "ceiling_bot", "Ceiling Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	botID := bot.User.GetId()
	server := PermissionTargetScope{Kind: MatrixScopeServer}

	// call.screenshare needs no privileged mode, so only the grant ceiling
	// stops the manager.
	if err := c.DenyServerPermission(ctx, SystemActorID, RoleModerator, PermCallScreenShare); err != nil {
		t.Fatalf("DenyServerPermission call.screenshare: %v", err)
	}
	if err := c.SetUserPermissionState(ctx, f.moderator, botID, server, PermCallScreenShare, PermissionStateAllow); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("manager grants call.screenshare they lack: error = %v, want permission denied", err)
	}
	if err := c.SetUserPermissionState(ctx, f.moderator, botID, server, PermMessageReact, PermissionStateAllow); err != nil {
		t.Fatalf("manager grants held permission: %v", err)
	}
	if err := c.SetUserPermissionState(ctx, f.member, botID, server, PermRoomCreate, PermissionStateAllow); err != nil {
		t.Fatalf("owner grants own permission: %v", err)
	}
	other, err := c.CreateUser(ctx, SystemActorID, "ceiling-recipient", "Recipient", "password123")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	if _, err := c.ReassignBotOwner(ctx, f.moderator, botID, other.Id); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("manager reassigns bot with grants they lack: error = %v, want permission denied", err)
	}

	// Reading management data and adding a bot to a room do not depend on rank.
	adminBot, err := c.CreateBot(ctx, f.admin, "ceiling_admin_bot", "Ceiling Admin Bot")
	if err != nil {
		t.Fatalf("CreateBot admin bot: %v", err)
	}
	if _, err := c.GetUserPermissionMatrix(ctx, f.moderator, adminBot.User.GetId()); err != nil {
		t.Fatalf("manager reads higher-ranked bot matrix: %v", err)
	}
	if err := allowReactForHierarchy(c, ctx, f.moderator, adminBot.User.GetId()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("manager changes higher-ranked bot: error = %v, want permission denied", err)
	}
}

func allowReactForHierarchy(c *ChattoCore, ctx context.Context, actorID, botID string) error {
	return c.SetUserPermissionState(ctx, actorID, botID, PermissionTargetScope{Kind: MatrixScopeServer}, PermMessageReact, PermissionStateAllow)
}
