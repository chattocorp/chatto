package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
)

func TestAdminUserDeactivationReturnsConsistentUserState(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "deactivation-target", "Target", "password123")
	if err != nil {
		t.Fatal(err)
	}
	request := connect.NewRequest(&adminv1.SetUserDeactivatedRequest{UserId: target.Id, Deactivated: true})
	if _, err := env.adminUsers.SetUserDeactivated(withCaller(env.ctx, env.viewer), request); errorCode(err) != connect.CodePermissionDenied {
		t.Fatalf("without permission: %v", err)
	}
	if err := env.core.AssignOwnerRole(env.ctx, env.viewer.Id); err != nil {
		t.Fatal(err)
	}
	for _, deactivated := range []bool{true, false} {
		response, err := env.adminUsers.SetUserDeactivated(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.SetUserDeactivatedRequest{
			UserId: target.Id, Deactivated: deactivated,
		}))
		if err != nil {
			t.Fatal(err)
		}
		member := response.Msg.GetMember()
		if member.GetUser().GetId() != target.Id || member.GetDeactivated() != deactivated || member.GetUser().GetDeactivated() != deactivated {
			t.Fatal("administrative member and public user disagree about deactivation")
		}
		if !member.GetViewerCanChangeDeactivation() {
			t.Fatal("administrator cannot reverse the change")
		}
	}
}
