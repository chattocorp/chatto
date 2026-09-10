package connectapi

import (
	"connectrpc.com/connect"
	"fmt"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"sort"
	"testing"
)

func TestAdminRoleMembersPaginationAndAuthorization(t *testing.T) {
	env := newConnectAPITestEnv(t)
	request := connect.NewRequest(&adminv1.AdminRoleServiceListMembersRequest{Name: "missing"})
	if _, err := env.roles.ListMembers(env.ctx, request); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("anonymous: %v", err)
	}
	ctx := withCaller(env.ctx, env.viewer)
	if _, err := env.roles.ListMembers(ctx, request); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("unauthorized missing role: %v", err)
	}
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleAssign); err != nil {
		t.Fatal(err)
	}
	if _, err := env.roles.ListMembers(ctx, request); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("authorized missing role: %v", err)
	}
	// Assignment authority alone must not grant the broader admin user directory.
	if _, err := env.adminUsers.ListMembers(ctx, connect.NewRequest(&adminv1.ListMembersRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("admin directory: %v", err)
	}
	if _, err := env.core.CreateServerRole(env.ctx, core.SystemActorID, "paged", "Paged", ""); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 103)
	for i := range ids {
		user, err := env.core.CreateUser(env.ctx, core.SystemActorID, fmt.Sprintf("roster-%03d", i), "Roster member", "password")
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = user.Id
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, user.Id, "paged"); err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(ids)
	for _, tc := range []struct {
		name         string
		page         *apiv1.PageRequest
		start, count int
		more         bool
	}{
		{"default", nil, 0, 20, true},
		{"first", &apiv1.PageRequest{Limit: 2}, 0, 2, true},
		{"next", &apiv1.PageRequest{Limit: 2, Offset: 2}, 2, 2, true},
		{"cap", &apiv1.PageRequest{Limit: 500}, 0, 100, true},
		{"last", &apiv1.PageRequest{Limit: 20, Offset: 100}, 100, 3, false},
		{"past end", &apiv1.PageRequest{Offset: 2147483647}, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := env.roles.ListMembers(ctx, connect.NewRequest(&adminv1.AdminRoleServiceListMembersRequest{Name: "paged", Page: tc.page}))
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Msg.Members) != tc.count || res.Msg.Page.TotalCount != 103 || res.Msg.Page.HasMore != tc.more {
				t.Fatalf("unexpected page: %v", res.Msg.Page)
			}
			for i, user := range res.Msg.Members {
				if user.Id != ids[tc.start+i] {
					t.Fatal("unstable ordering")
				}
			}
		})
	}
	for _, page := range []*apiv1.PageRequest{{Limit: -1}, {Offset: -1}} {
		if _, err := env.roles.ListMembers(ctx, connect.NewRequest(&adminv1.AdminRoleServiceListMembersRequest{Name: "paged", Page: page})); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("invalid page: %v", err)
		}
	}
	res, err := env.roles.ListMembers(ctx, connect.NewRequest(&adminv1.AdminRoleServiceListMembersRequest{Name: core.RoleEveryone}))
	if err != nil || len(res.Msg.GetMembers()) != 0 || res.Msg.Page.TotalCount != 0 || res.Msg.Page.HasMore {
		t.Fatalf("everyone page: %v", err)
	}
	// Account deletion revokes its assignments; later pages reflect that change.
	if err := env.core.DeleteUser(env.ctx, core.SystemActorID, ids[0]); err != nil {
		t.Fatal(err)
	}
	res, err = env.roles.ListMembers(ctx, connect.NewRequest(&adminv1.AdminRoleServiceListMembersRequest{Name: "paged", Page: &apiv1.PageRequest{Limit: 1}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Msg.Members) != 1 || res.Msg.Members[0].Id != ids[1] || res.Msg.Page.TotalCount != 102 {
		t.Fatal("expected deleted account to leave the roster")
	}
	// Detail reads do not carry roster data.
	detail, err := env.roles.GetRole(ctx, connect.NewRequest(&adminv1.GetRoleRequest{Name: "paged"}))
	if err != nil || detail.Msg.ProtoReflect().Descriptor().Fields().ByName("users") != nil {
		t.Fatalf("role detail: %v", err)
	}
}
