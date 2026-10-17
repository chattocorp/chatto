package connectapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Exercise the public service with real core authorization and human sessions.
// In particular, ownership survives loss of creation authority and global reads
// never bypass the privileged-mode gate.
func TestInviteLinkServiceOwnershipAndPermissions(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "inviter", "Inviter", "password123")
	require.NoError(t, err)
	manager, err := env.core.CreateUser(env.ctx, core.SystemActorID, "invite-manager", "Manager", "password123")
	require.NoError(t, err)
	memberCtx := withBearerCredential(env.ctx, member, "member-session")
	memberCtx = WithRequestBaseURL(memberCtx, "https://chat.example")
	managerCtx := withBearerCredential(env.ctx, manager, "manager-session")
	elevated := withArmedBearerCredential(env.ctx, manager, "manager-session")
	svc := env.inviteLinks

	_, err = svc.CreateInviteLink(memberCtx, connect.NewRequest(&apiv1.CreateInviteLinkRequest{}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, member.Id, core.PermInviteLinkCreate))
	_, err = svc.CreateInviteLink(memberCtx, connect.NewRequest(&apiv1.CreateInviteLinkRequest{ExpiresAt: &timestamppb.Timestamp{Seconds: 253402300800}}))
	require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
	maxUses := uint32(3)
	created, err := svc.CreateInviteLink(memberCtx, connect.NewRequest(&apiv1.CreateInviteLinkRequest{MaxUses: &maxUses, ExpiresAt: timestamppb.New(time.Now().Add(time.Hour))}))
	require.NoError(t, err)
	link := created.Msg.GetInviteLink()
	require.True(t, strings.HasPrefix(link.GetLink(), "https://chat.example/invite/"))
	require.Equal(t, member.Id, link.GetCreatedBy())
	require.Equal(t, maxUses, link.GetMaxUses())
	require.Equal(t, apiv1.InviteLinkStatus_INVITE_LINK_STATUS_ACTIVE, link.GetStatus())

	// Add a second creator's link, so counts and offset pages can expose leaks.
	require.NoError(t, env.core.AssignAdminRole(env.ctx, env.viewer.Id))
	other, err := svc.CreateInviteLink(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.CreateInviteLinkRequest{}))
	require.NoError(t, err)
	list, err := svc.ListInviteLinks(memberCtx, connect.NewRequest(&apiv1.ListInviteLinksRequest{Page: &apiv1.PageRequest{Limit: 1}}))
	require.NoError(t, err)
	require.Len(t, list.Msg.InviteLinks, 1)
	require.Equal(t, link.Id, list.Msg.InviteLinks[0].Id)
	require.EqualValues(t, 1, list.Msg.Page.TotalCount)
	require.False(t, list.Msg.Page.HasMore)
	page, err := svc.ListInviteLinks(memberCtx, connect.NewRequest(&apiv1.ListInviteLinksRequest{Page: &apiv1.PageRequest{Limit: 1, Offset: 1}}))
	require.NoError(t, err)
	require.Empty(t, page.Msg.InviteLinks)
	for _, id := range []string{other.Msg.InviteLink.Id, "missing"} {
		_, err = svc.GetInviteLink(memberCtx, connect.NewRequest(&apiv1.GetInviteLinkRequest{Id: id}))
		require.Equal(t, connect.CodeNotFound, errorCode(err))
		_, err = svc.RevokeInviteLink(memberCtx, connect.NewRequest(&apiv1.RevokeInviteLinkRequest{Id: id}))
		require.Equal(t, connect.CodeNotFound, errorCode(err))
	}

	// Global management does not include creation, and requires elevation.
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, manager.Id, core.PermInviteLinkManage))
	for _, ctx := range []context.Context{managerCtx, elevated} {
		_, err = svc.CreateInviteLink(ctx, connect.NewRequest(&apiv1.CreateInviteLinkRequest{}))
		require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	}
	list, err = svc.ListInviteLinks(managerCtx, connect.NewRequest(&apiv1.ListInviteLinksRequest{}))
	require.NoError(t, err)
	require.Empty(t, list.Msg.InviteLinks)
	_, err = svc.RevokeInviteLink(managerCtx, connect.NewRequest(&apiv1.RevokeInviteLinkRequest{Id: link.Id}))
	require.Equal(t, connect.CodeNotFound, errorCode(err))
	list, err = svc.ListInviteLinks(elevated, connect.NewRequest(&apiv1.ListInviteLinksRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.InviteLinks, 2)
	_, err = svc.GetInviteLink(elevated, connect.NewRequest(&apiv1.GetInviteLinkRequest{Id: link.Id}))
	require.NoError(t, err)
	_, err = svc.RevokeInviteLink(elevated, connect.NewRequest(&apiv1.RevokeInviteLinkRequest{Id: other.Msg.InviteLink.Id}))
	require.NoError(t, err)

	// Losing create authority changes neither ownership nor link validity.
	require.NoError(t, env.core.DenyUserPermission(env.ctx, core.SystemActorID, member.Id, core.PermInviteLinkCreate))
	_, err = svc.CreateInviteLink(memberCtx, connect.NewRequest(&apiv1.CreateInviteLinkRequest{}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = env.core.ValidateInviteLinkToken(env.ctx, strings.TrimPrefix(link.Link, "https://chat.example/invite/"))
	require.NoError(t, err)
	got, err := svc.GetInviteLink(memberCtx, connect.NewRequest(&apiv1.GetInviteLinkRequest{Id: link.Id}))
	require.NoError(t, err)
	require.Equal(t, link.Link, got.Msg.InviteLink.Link)
	for range 2 {
		revoked, err := svc.RevokeInviteLink(memberCtx, connect.NewRequest(&apiv1.RevokeInviteLinkRequest{Id: link.Id}))
		require.NoError(t, err)
		require.Equal(t, apiv1.InviteLinkStatus_INVITE_LINK_STATUS_REVOKED, revoked.Msg.InviteLink.Status)
	}
	// An owner without elevation still receives only their own links.
	require.NoError(t, env.core.AssignOwnerRole(env.ctx, manager.Id))
	list, err = svc.ListInviteLinks(managerCtx, connect.NewRequest(&apiv1.ListInviteLinksRequest{}))
	require.NoError(t, err)
	require.Empty(t, list.Msg.InviteLinks)
}
