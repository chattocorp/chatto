package connectapi

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
)

func TestAdminInvitationServiceLifecycleAndAuthorization(t *testing.T) {
	env := newConnectAPITestEnv(t)
	regular, err := env.core.CreateUser(env.ctx, core.SystemActorID, "invite-api-regular", "Invite API Regular", "password123")
	if err != nil {
		t.Fatalf("CreateUser regular: %v", err)
	}
	if _, err := env.adminInvitations.ListInvitations(
		withCaller(env.ctx, regular),
		connect.NewRequest(&adminv1.ListInvitationsRequest{}),
	); err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular ListInvitations error = %v, want permission denied", err)
	}

	if err := env.core.AssignAdminRole(env.ctx, env.viewer.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	ctx := withCaller(env.ctx, env.viewer)
	if _, err := env.adminInvitations.CreateInvitation(ctx, connect.NewRequest(&adminv1.CreateInvitationRequest{
		ExpiresAt: &timestamppb.Timestamp{Seconds: 253402300800},
	})); err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid expiry error = %v, want invalid argument", err)
	}
	maxUses := uint32(3)
	expiresAt := timestamppb.New(time.Now().Add(24 * time.Hour))
	created, err := env.adminInvitations.CreateInvitation(ctx, connect.NewRequest(&adminv1.CreateInvitationRequest{
		MaxUses:   &maxUses,
		ExpiresAt: expiresAt,
	}))
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	invitation := created.Msg.GetInvitation()
	if invitation.GetId() == "" || invitation.GetCode() == "" || invitation.GetMaxUses() != maxUses || invitation.GetStatus() != adminv1.InvitationStatus_INVITATION_STATUS_ACTIVE {
		t.Fatalf("created invitation = %+v", invitation)
	}

	listed, err := env.adminInvitations.ListInvitations(ctx, connect.NewRequest(&adminv1.ListInvitationsRequest{}))
	if err != nil {
		t.Fatalf("ListInvitations: %v", err)
	}
	if len(listed.Msg.GetInvitations()) != 1 || listed.Msg.GetInvitations()[0].GetCode() != invitation.GetCode() {
		t.Fatalf("listed invitations = %+v, want reconstructed code %q", listed.Msg.GetInvitations(), invitation.GetCode())
	}

	revoked, err := env.adminInvitations.RevokeInvitation(ctx, connect.NewRequest(&adminv1.RevokeInvitationRequest{Id: invitation.GetId()}))
	if err != nil {
		t.Fatalf("RevokeInvitation: %v", err)
	}
	if revoked.Msg.GetInvitation().GetStatus() != adminv1.InvitationStatus_INVITATION_STATUS_REVOKED || revoked.Msg.GetInvitation().GetRevokedAt() == nil {
		t.Fatalf("revoked invitation = %+v", revoked.Msg.GetInvitation())
	}
}
