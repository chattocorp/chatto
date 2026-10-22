package connectapi

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAdminInvitationCodeServiceLifecycleAndAuthorization(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	regular, err := env.core.CreateUser(env.ctx, core.SystemActorID, "invite-api-regular", "Invite API Regular", "password123")
	if err != nil {
		t.Fatal(err)
	}
	service := env.adminInvitationCodes
	if _, err := service.ListInvitationCodes(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.ListInvitationCodesRequest{})); errorCode(err) != connect.CodePermissionDenied {
		t.Fatalf("regular list error = %v, want permission denied", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, env.viewer.Id); err != nil {
		t.Fatal(err)
	}
	ctx := withCaller(env.ctx, env.viewer)
	if _, err := service.CreateInvitationCode(ctx, connect.NewRequest(&adminv1.CreateInvitationCodeRequest{ExpiresAt: &timestamppb.Timestamp{Seconds: 253402300800}})); errorCode(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid expiry error = %v", err)
	}
	created, err := service.CreateInvitationCode(ctx, connect.NewRequest(&adminv1.CreateInvitationCodeRequest{ExpiresAt: timestamppb.New(time.Now().Add(24 * time.Hour))}))
	if err != nil {
		t.Fatal(err)
	}
	code := created.Msg.GetInvitationCode()
	if code.GetId() == "" || code.GetCode() == "" || code.GetStatus() != adminv1.InvitationCodeStatus_INVITATION_CODE_STATUS_ACTIVE {
		t.Fatal("incomplete created code")
	}
	if id, err := env.core.ValidateInvitationCode(ctx, code.GetCode()); err != nil || id != code.GetId() {
		t.Fatalf("validate created code: %v", err)
	}
	listed, err := service.ListInvitationCodes(ctx, connect.NewRequest(&adminv1.ListInvitationCodesRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Msg.GetInvitationCodes()) != 1 || listed.Msg.GetInvitationCodes()[0].GetCode() != code.GetCode() {
		t.Fatal("list must reconstruct the same code")
	}
	got, err := service.GetInvitationCode(ctx, connect.NewRequest(&adminv1.GetInvitationCodeRequest{Id: code.GetId()}))
	if err != nil || got.Msg.GetInvitationCode().GetCode() != code.GetCode() {
		t.Fatalf("get code: %v", err)
	}
	revoked, err := service.RevokeInvitationCode(ctx, connect.NewRequest(&adminv1.RevokeInvitationCodeRequest{Id: code.GetId()}))
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Msg.GetInvitationCode().GetStatus() != adminv1.InvitationCodeStatus_INVITATION_CODE_STATUS_REVOKED || revoked.Msg.GetInvitationCode().GetRevokedAt() == nil {
		t.Fatal("expected revoked code")
	}
}
