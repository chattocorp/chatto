package connectapi

import (
	"strings"
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
	if _, err := service.CreateInvitationCode(ctx, connect.NewRequest(&adminv1.CreateInvitationCodeRequest{Note: strings.Repeat("界", 251)})); errorCode(err) != connect.CodeInvalidArgument {
		t.Fatalf("overlong note error = %v", err)
	}
	const note = "For the design team"
	created, err := service.CreateInvitationCode(ctx, connect.NewRequest(&adminv1.CreateInvitationCodeRequest{ExpiresAt: timestamppb.New(time.Now().Add(24 * time.Hour)), Note: "  " + note + "  "}))
	if err != nil {
		t.Fatal(err)
	}
	code := created.Msg.GetInvitationCode()
	if code.GetNote() != note {
		t.Fatal("create response lost the note")
	}
	if _, err := service.GetInvitationCode(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.GetInvitationCodeRequest{Id: code.GetId()})); errorCode(err) != connect.CodePermissionDenied {
		t.Fatalf("regular get error = %v, want permission denied", err)
	}
	if _, err := service.CreateInvitationCode(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.CreateInvitationCodeRequest{Note: note})); errorCode(err) != connect.CodePermissionDenied {
		t.Fatalf("regular create error = %v, want permission denied", err)
	}
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
	if listed.Msg.GetInvitationCodes()[0].GetNote() != note {
		t.Fatal("list response lost the note")
	}
	got, err := service.GetInvitationCode(ctx, connect.NewRequest(&adminv1.GetInvitationCodeRequest{Id: code.GetId()}))
	if err != nil || got.Msg.GetInvitationCode().GetCode() != code.GetCode() {
		t.Fatalf("get code: %v", err)
	}
	if got.Msg.GetInvitationCode().GetNote() != note {
		t.Fatal("get response lost the note")
	}
	missingID := core.NewInvitationID()
	for _, id := range []string{code.GetId(), missingID} {
		if _, err := service.RevokeInvitationCode(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.RevokeInvitationCodeRequest{Id: id})); errorCode(err) != connect.CodePermissionDenied {
			t.Fatalf("regular revoke error = %v, want permission denied", err)
		}
	}
	if _, err := service.RevokeInvitationCode(ctx, connect.NewRequest(&adminv1.RevokeInvitationCodeRequest{Id: missingID})); errorCode(err) != connect.CodeNotFound {
		t.Fatalf("missing revoke error = %v, want not found", err)
	}
	revoked, err := service.RevokeInvitationCode(ctx, connect.NewRequest(&adminv1.RevokeInvitationCodeRequest{Id: code.GetId()}))
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Msg.GetInvitationCode().GetStatus() != adminv1.InvitationCodeStatus_INVITATION_CODE_STATUS_REVOKED || revoked.Msg.GetInvitationCode().GetRevokedAt() == nil {
		t.Fatal("expected revoked code")
	}
	if revoked.Msg.GetInvitationCode().GetNote() != note {
		t.Fatal("revocation response lost the note")
	}
	repeated, err := service.RevokeInvitationCode(ctx, connect.NewRequest(&adminv1.RevokeInvitationCodeRequest{Id: code.GetId()}))
	if err != nil {
		t.Fatal(err)
	}
	if !repeated.Msg.GetInvitationCode().GetRevokedAt().AsTime().Equal(revoked.Msg.GetInvitationCode().GetRevokedAt().AsTime()) {
		t.Fatal("repeated revocation changed its timestamp")
	}
}
