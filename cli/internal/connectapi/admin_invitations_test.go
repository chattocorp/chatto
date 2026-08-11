package connectapi

import (
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
)

func TestAdminInviteLinkServiceLifecycleAndAuthorization(t *testing.T) {
	env := newConnectAPITestEnv(t)
	regular, err := env.core.CreateUser(env.ctx, core.SystemActorID, "invite-api-regular", "Invite API Regular", "password123")
	if err != nil {
		t.Fatalf("CreateUser regular: %v", err)
	}
	if _, err := env.adminInviteLinks.ListInviteLinks(
		withCaller(env.ctx, regular),
		connect.NewRequest(&adminv1.ListInviteLinksRequest{}),
	); err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular ListInviteLinks error = %v, want permission denied", err)
	}

	if err := env.core.AssignAdminRole(env.ctx, env.viewer.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	ctx := WithRequestBaseURL(withCaller(env.ctx, env.viewer), "https://chat.example")
	if _, err := env.adminInviteLinks.CreateInviteLink(ctx, connect.NewRequest(&adminv1.CreateInviteLinkRequest{
		ExpiresAt: &timestamppb.Timestamp{Seconds: 253402300800},
	})); err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid expiry error = %v, want invalid argument", err)
	}
	maxUses := uint32(3)
	expiresAt := timestamppb.New(time.Now().Add(24 * time.Hour))
	created, err := env.adminInviteLinks.CreateInviteLink(ctx, connect.NewRequest(&adminv1.CreateInviteLinkRequest{
		MaxUses:   &maxUses,
		ExpiresAt: expiresAt,
	}))
	if err != nil {
		t.Fatalf("CreateInviteLink: %v", err)
	}
	inviteLink := created.Msg.GetInviteLink()
	if inviteLink.GetId() == "" || !strings.HasPrefix(inviteLink.GetLink(), "https://chat.example/invite/") || inviteLink.GetMaxUses() != maxUses || inviteLink.GetStatus() != adminv1.InviteLinkStatus_INVITE_LINK_STATUS_ACTIVE {
		t.Fatalf("created invite link = %+v", inviteLink)
	}

	listed, err := env.adminInviteLinks.ListInviteLinks(ctx, connect.NewRequest(&adminv1.ListInviteLinksRequest{}))
	if err != nil {
		t.Fatalf("ListInviteLinks: %v", err)
	}
	if len(listed.Msg.GetInviteLinks()) != 1 || listed.Msg.GetInviteLinks()[0].GetLink() != inviteLink.GetLink() {
		t.Fatalf("listed invite links = %+v, want reconstructed link %q", listed.Msg.GetInviteLinks(), inviteLink.GetLink())
	}

	revoked, err := env.adminInviteLinks.RevokeInviteLink(ctx, connect.NewRequest(&adminv1.RevokeInviteLinkRequest{Id: inviteLink.GetId()}))
	if err != nil {
		t.Fatalf("RevokeInviteLink: %v", err)
	}
	if revoked.Msg.GetInviteLink().GetStatus() != adminv1.InviteLinkStatus_INVITE_LINK_STATUS_REVOKED || revoked.Msg.GetInviteLink().GetRevokedAt() == nil {
		t.Fatalf("revoked invite link = %+v", revoked.Msg.GetInviteLink())
	}
}
