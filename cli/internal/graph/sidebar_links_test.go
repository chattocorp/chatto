package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

func TestSidebarLinkMutationsUseGroupRoomManage(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()
	manager := env.createVerifiedUser(t, "sidebar-link-manager", "Sidebar Link Manager", "password123")

	sourceGroupID := env.testRoom.GroupId
	targetGroup, err := env.core.CreateRoomGroup(env.ctx, core.SystemActorID, "Links Target", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	if err := env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, sourceGroupID, manager.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserGroupPermission(source): %v", err)
	}
	managerCtx := env.authContextForUser(manager)

	_, err = mutation.CreateSidebarLink(managerCtx, model.CreateSidebarLinkInput{
		GroupID: targetGroup.Id,
		Label:   "Denied",
		URL:     "https://denied.example.com",
	})
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Fatalf("create in unmanaged group err = %v, want ErrPermissionDenied", err)
	}

	link, err := mutation.CreateSidebarLink(managerCtx, model.CreateSidebarLinkInput{
		GroupID: sourceGroupID,
		Label:   "Docs",
		URL:     "/docs",
	})
	if err != nil {
		t.Fatalf("CreateSidebarLink in managed group: %v", err)
	}
	if link.Url != "/docs" {
		t.Fatalf("created link URL = %q, want /docs", link.Url)
	}
	updated, err := mutation.UpdateSidebarLink(managerCtx, model.UpdateSidebarLinkInput{
		LinkID: link.Id,
		Label:  "Docs Updated",
		URL:    "/docs/updated",
	})
	if err != nil {
		t.Fatalf("UpdateSidebarLink in managed group: %v", err)
	}
	if updated.Url != "/docs/updated" {
		t.Fatalf("updated link URL = %q, want /docs/updated", updated.Url)
	}

	_, err = mutation.MoveSidebarLinkToGroup(managerCtx, model.MoveSidebarLinkToGroupInput{
		LinkID:  link.Id,
		GroupID: targetGroup.Id,
	})
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Fatalf("move to unmanaged target err = %v, want ErrPermissionDenied", err)
	}

	if err := env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, targetGroup.Id, manager.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserGroupPermission(target): %v", err)
	}
	if _, err := mutation.MoveSidebarLinkToGroup(managerCtx, model.MoveSidebarLinkToGroupInput{
		LinkID:  link.Id,
		GroupID: targetGroup.Id,
	}); err != nil {
		t.Fatalf("MoveSidebarLinkToGroup after target grant: %v", err)
	}
	if ok, err := mutation.DeleteSidebarLink(managerCtx, model.DeleteSidebarLinkInput{LinkID: link.Id}); err != nil || !ok {
		t.Fatalf("DeleteSidebarLink after move ok=%v err=%v", ok, err)
	}
}
