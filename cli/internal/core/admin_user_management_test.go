package core

import (
	"errors"
	"slices"
	"testing"

	"hmans.de/chatto/internal/evtstream"
)

func TestChattoCore_AdminMemberReads(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)

	target, err := c.CreateUser(ctx, SystemActorID, "adminmember-target", "Admin Member Target", "password123")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	regular, err := c.CreateUser(ctx, SystemActorID, "adminmember-regular", "Admin Member Regular", "password123")
	if err != nil {
		t.Fatalf("CreateUser regular: %v", err)
	}
	admin, err := c.CreateUser(ctx, SystemActorID, "adminmember-admin", "Admin Member Admin", "password123")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := c.AssignAdminRole(ctx, admin.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, target.Id, RoleModerator); err != nil {
		t.Fatalf("AssignServerRole target: %v", err)
	}
	if err := c.AddVerifiedEmailDirect(ctx, target.Id, "adminmember-target@example.test"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect target: %v", err)
	}
	if _, err := c.UpdateUserLogin(ctx, target.Id, "adminmember-target-renamed"); err != nil {
		t.Fatalf("UpdateUserLogin target: %v", err)
	}

	if _, err := c.ListAdminMembers(ctx, "", AdminMemberListInput{}); !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("ListAdminMembers unauth err = %v, want ErrNotAuthenticated", err)
	}
	if _, err := c.BatchGetAdminMembers(ctx, "", []string{target.Id}); !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("BatchGetAdminMembers unauth err = %v, want ErrNotAuthenticated", err)
	}

	if _, err := c.ListAdminMembers(ctx, regular.Id, AdminMemberListInput{Search: "target", Limit: 10}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("ListAdminMembers regular err = %v, want ErrPermissionDenied", err)
	}
	if _, err := c.GetAdminMemberDetails(ctx, regular.Id, target.Id); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("GetAdminMemberDetails regular err = %v, want ErrPermissionDenied", err)
	}
	if _, err := c.BatchGetAdminMembers(ctx, regular.Id, []string{target.Id}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("BatchGetAdminMembers regular err = %v, want ErrPermissionDenied", err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, regular.Id, PermAdminUsersView); err != nil {
		t.Fatalf("GrantUserPermission admin.view-users: %v", err)
	}
	regularDetails, err := c.GetAdminMemberDetails(ctx, regular.Id, target.Id)
	if err != nil {
		t.Fatalf("GetAdminMemberDetails list-only viewer: %v", err)
	}
	if regularDetails.Member.LastLoginChange != nil {
		t.Fatal("list-only viewer received username-change timestamp without user.manage-accounts")
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, regular.Id, PermUserManageAccounts); err != nil {
		t.Fatalf("GrantUserPermission user.manage-accounts: %v", err)
	}
	accountManagerDetails, err := c.GetAdminMemberDetails(ctx, regular.Id, target.Id)
	if err != nil {
		t.Fatalf("GetAdminMemberDetails account manager: %v", err)
	}
	if accountManagerDetails.Member.LastLoginChange == nil {
		t.Fatal("account manager did not receive username-change timestamp")
	}

	list, err := c.ListAdminMembers(ctx, admin.Id, AdminMemberListInput{Search: "target", Limit: 10})
	if err != nil {
		t.Fatalf("ListAdminMembers: %v", err)
	}
	if list.TotalCount != 1 || len(list.Users) != 1 {
		t.Fatalf("ListAdminMembers returned %d/%d users, want 1/1", len(list.Users), list.TotalCount)
	}
	if got := list.Users[0].Roles; len(got) != 1 || got[0] != RoleModerator {
		t.Fatalf("list user roles = %v, want explicit moderator only", got)
	}
	if !list.Users[0].HasVerifiedEmail || len(list.Users[0].VerifiedEmails) != 1 || list.Users[0].VerifiedEmails[0] != "adminmember-target@example.test" {
		t.Fatalf("list user emails = has:%v emails:%v, want target email", list.Users[0].HasVerifiedEmail, list.Users[0].VerifiedEmails)
	}
	if list.Users[0].LastLoginChange == nil {
		t.Fatal("list user LastLoginChange is nil, want visible cooldown timestamp")
	}

	batch, err := c.BatchGetAdminMembers(ctx, admin.Id, []string{target.Id, "missing-user", regular.Id, target.Id})
	if err != nil {
		t.Fatalf("BatchGetAdminMembers: %v", err)
	}
	if len(batch.Users) != 2 || batch.Users[0].ID != target.Id || batch.Users[1].ID != regular.Id {
		t.Fatalf("BatchGetAdminMembers users = %+v, want target,regular", batch.Users)
	}
	if got := batch.Users[0].Roles; len(got) != 1 || got[0] != RoleModerator {
		t.Fatalf("batch target roles = %v, want explicit moderator only", got)
	}
	if !batch.Users[0].HasVerifiedEmail || len(batch.Users[0].VerifiedEmails) != 1 || batch.Users[0].VerifiedEmails[0] != "adminmember-target@example.test" {
		t.Fatalf("batch target emails = has:%v emails:%v, want target email", batch.Users[0].HasVerifiedEmail, batch.Users[0].VerifiedEmails)
	}
	if batch.Users[0].LastLoginChange == nil {
		t.Fatal("batch target LastLoginChange is nil, want visible cooldown timestamp")
	}
	if len(batch.Roles) == 0 {
		t.Fatal("batch roles are empty")
	}

	adminDetails, err := c.GetAdminMemberDetails(ctx, admin.Id, target.Id)
	if err != nil {
		t.Fatalf("GetAdminMemberDetails admin: %v", err)
	}
	if adminDetails.Member == nil {
		t.Fatal("admin details member is nil")
	}
	if !adminDetails.Member.HasVerifiedEmail || len(adminDetails.Member.VerifiedEmails) != 1 || adminDetails.Member.VerifiedEmails[0] != "adminmember-target@example.test" {
		t.Fatalf("admin details emails = has:%v emails:%v, want target email", adminDetails.Member.HasVerifiedEmail, adminDetails.Member.VerifiedEmails)
	}
	if adminDetails.Member.LastLoginChange == nil {
		t.Fatal("admin details LastLoginChange is nil, want visible cooldown timestamp")
	}
	if !adminDetails.ViewerCanAssignRoles || !adminDetails.ViewerCanManageRoles || !adminDetails.ViewerCanManageUserPermissions {
		t.Fatalf("admin capabilities = assign:%v manage:%v perms:%v, want all true", adminDetails.ViewerCanAssignRoles, adminDetails.ViewerCanManageRoles, adminDetails.ViewerCanManageUserPermissions)
	}
	if len(adminDetails.Roles) == 0 || len(adminDetails.AvailablePermissions) == 0 {
		t.Fatalf("admin details roles/perms empty: roles=%d perms=%d", len(adminDetails.Roles), len(adminDetails.AvailablePermissions))
	}
}

func TestChattoCore_ListAdminMembersPaginationRolesAndDeletion(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, err := c.CreateUser(ctx, SystemActorID, "member-page-admin", "Member Page Admin", "password123")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := c.AssignAdminRole(ctx, admin.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}

	var users []string
	for _, login := range []string{"member-page-one", "member-page-two", "member-page-three"} {
		user, err := c.CreateUser(ctx, SystemActorID, login, "PAGINATED Member", "password123")
		if err != nil {
			t.Fatalf("CreateUser %s: %v", login, err)
		}
		users = append(users, user.Id)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, users[1], RoleModerator); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	page, err := c.ListAdminMembers(ctx, admin.Id, AdminMemberListInput{
		Search: "paginated MEM",
		Limit:  1,
		Offset: 1,
	})
	if err != nil {
		t.Fatalf("ListAdminMembers: %v", err)
	}
	if page.TotalCount != 3 || !page.HasMore || len(page.Users) != 1 {
		t.Fatalf("page = users:%d total:%d hasMore:%v, want 1/3/true", len(page.Users), page.TotalCount, page.HasMore)
	}
	if page.Users[0].ID != users[1] {
		t.Fatalf("page user = %q, want %q", page.Users[0].ID, users[1])
	}
	if got := page.Users[0].Roles; len(got) != 1 || got[0] != RoleModerator {
		t.Fatalf("page roles = %v, want explicit moderator only", got)
	}

	serverMembers, total, err := c.GetServerMembers(ctx, "member-page-two", 1, 0)
	if err != nil {
		t.Fatalf("GetServerMembers: %v", err)
	}
	if total != 1 || len(serverMembers) != 1 || serverMembers[0].User == nil {
		t.Fatalf("server members = %+v total:%d, want one hydrated member", serverMembers, total)
	}
	if got := serverMembers[0].Roles; len(got) != 2 || got[0] != RoleEveryone || got[1] != RoleModerator {
		t.Fatalf("server member roles = %v, want everyone and moderator", got)
	}

	if err := c.DeleteUser(ctx, SystemActorID, users[1]); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	afterDelete, err := c.ListAdminMembers(ctx, admin.Id, AdminMemberListInput{Search: "paginated mem", Limit: 10})
	if err != nil {
		t.Fatalf("ListAdminMembers after deletion: %v", err)
	}
	if afterDelete.TotalCount != 2 || len(afterDelete.Users) != 2 || afterDelete.HasMore {
		t.Fatalf("after deletion = users:%d total:%d hasMore:%v, want 2/2/false", len(afterDelete.Users), afterDelete.TotalCount, afterDelete.HasMore)
	}
	for _, member := range afterDelete.Users {
		if member.ID == users[1] || member.Deleted {
			t.Fatalf("deleted user remained in list: %+v", member)
		}
	}
}

func TestChattoCore_AdminRoleAssignmentAuthorization(t *testing.T) {
	t.Run("unauthenticated actor is rejected", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		target, err := c.CreateUser(ctx, SystemActorID, "adminrole-target", "Target", "password123")
		if err != nil {
			t.Fatalf("CreateUser target: %v", err)
		}
		if err := c.AdminAssignServerRole(ctx, "", target.Id, RoleModerator); !errors.Is(err, ErrNotAuthenticated) {
			t.Fatalf("AdminAssignServerRole err = %v, want ErrNotAuthenticated", err)
		}
	})

	t.Run("regular user cannot assign or revoke roles", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		regular, err := c.CreateUser(ctx, SystemActorID, "adminrole-regular", "Regular", "password123")
		if err != nil {
			t.Fatalf("CreateUser regular: %v", err)
		}
		target, err := c.CreateUser(ctx, SystemActorID, "adminrole-target2", "Target", "password123")
		if err != nil {
			t.Fatalf("CreateUser target: %v", err)
		}
		if err := c.AdminAssignServerRole(ctx, regular.Id, target.Id, RoleModerator); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("AdminAssignServerRole err = %v, want ErrPermissionDenied", err)
		}
		if err := c.AdminRevokeServerRole(ctx, regular.Id, target.Id, RoleModerator); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("AdminRevokeServerRole err = %v, want ErrPermissionDenied", err)
		}
	})

	t.Run("role assigner can assign and revoke roles", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		admin, err := c.CreateUser(ctx, SystemActorID, "adminrole-admin", "Admin", "password123")
		if err != nil {
			t.Fatalf("CreateUser admin: %v", err)
		}
		if err := c.AssignAdminRole(ctx, admin.Id); err != nil {
			t.Fatalf("AssignAdminRole: %v", err)
		}
		target, err := c.CreateUser(ctx, SystemActorID, "adminrole-target3", "Target", "password123")
		if err != nil {
			t.Fatalf("CreateUser target: %v", err)
		}
		if err := c.AdminAssignServerRole(ctx, admin.Id, target.Id, RoleModerator); err != nil {
			t.Fatalf("AdminAssignServerRole: %v", err)
		}
		roles, err := c.GetUserRoles(ctx, target.Id)
		if err != nil {
			t.Fatalf("GetUserRoles after assign: %v", err)
		}
		if len(roles) != 1 || roles[0] != RoleModerator {
			t.Fatalf("roles after assign = %v, want moderator", roles)
		}
		if err := c.AdminRevokeServerRole(ctx, admin.Id, target.Id, RoleModerator); err != nil {
			t.Fatalf("AdminRevokeServerRole: %v", err)
		}
		roles, err = c.GetUserRoles(ctx, target.Id)
		if err != nil {
			t.Fatalf("GetUserRoles after revoke: %v", err)
		}
		if len(roles) != 0 {
			t.Fatalf("roles after revoke = %v, want none", roles)
		}
	})

	t.Run("missing target user does not persist role facts", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		admin, err := c.CreateUser(ctx, SystemActorID, "adminrole-missing-target-admin", "Admin", "password123")
		if err != nil {
			t.Fatalf("CreateUser admin: %v", err)
		}
		if err := c.AssignAdminRole(ctx, admin.Id); err != nil {
			t.Fatalf("AssignAdminRole: %v", err)
		}

		const missingUserID = "UmissingAdminRoleTarget"
		if err := c.AdminAssignServerRole(ctx, admin.Id, missingUserID, RoleModerator); !errors.Is(err, ErrNotFound) {
			t.Fatalf("AdminAssignServerRole missing user err = %v, want ErrNotFound", err)
		}
		if c.rbacModel.hasRole(missingUserID, RoleModerator) {
			t.Fatal("missing user was assigned moderator role")
		}

		beforeRevocations, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RBACAggregate().Subject(evtstream.EventRBACRoleRevoked))
		if err != nil {
			t.Fatalf("SubjectEvents role revoked before: %v", err)
		}
		if err := c.AdminRevokeServerRole(ctx, admin.Id, missingUserID, RoleModerator); !errors.Is(err, ErrNotFound) {
			t.Fatalf("AdminRevokeServerRole missing user err = %v, want ErrNotFound", err)
		}
		afterRevocations, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RBACAggregate().Subject(evtstream.EventRBACRoleRevoked))
		if err != nil {
			t.Fatalf("SubjectEvents role revoked after: %v", err)
		}
		if len(afterRevocations) != len(beforeRevocations) {
			t.Fatalf("role revocation events changed from %d to %d for missing user", len(beforeRevocations), len(afterRevocations))
		}
	})

	t.Run("cannot revoke own owner or admin role", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		admin, err := c.CreateUser(ctx, SystemActorID, "adminrole-self", "Self", "password123")
		if err != nil {
			t.Fatalf("CreateUser admin: %v", err)
		}
		if err := c.AssignAdminRole(ctx, admin.Id); err != nil {
			t.Fatalf("AssignAdminRole: %v", err)
		}
		if err := c.AssignOwnerRole(ctx, admin.Id); err != nil {
			t.Fatalf("AssignOwnerRole: %v", err)
		}
		if err := c.AdminRevokeServerRole(ctx, admin.Id, admin.Id, RoleAdmin); !errors.Is(err, ErrCannotRevokeSelfAdmin) {
			t.Fatalf("AdminRevokeServerRole admin err = %v, want ErrCannotRevokeSelfAdmin", err)
		}
		if err := c.AdminRevokeServerRole(ctx, admin.Id, admin.Id, RoleOwner); !errors.Is(err, ErrCannotRevokeSelfAdmin) {
			t.Fatalf("AdminRevokeServerRole owner err = %v, want ErrCannotRevokeSelfAdmin", err)
		}
		details, err := c.GetAdminMemberDetails(ctx, admin.Id, admin.Id)
		if err != nil {
			t.Fatalf("GetAdminMemberDetails self: %v", err)
		}
		if slices.Contains(details.RevocableRoleNames, RoleAdmin) || slices.Contains(details.RevocableRoleNames, RoleOwner) {
			t.Fatalf("self revocable roles = %v, must omit protected admin and owner roles", details.RevocableRoleNames)
		}
	})
}
