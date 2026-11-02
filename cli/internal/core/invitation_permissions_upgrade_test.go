package core

import (
	"sync"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/stretchr/testify/require"
)

func TestInvitationPermissionUpgrade(t *testing.T) {
	t.Parallel()
	h := newTestEventHarness(t)
	ctx := testContext(t)
	decisions := []rbacSeedDecision{
		{scope: ScopeServer, subject: RoleAdmin, permission: PermUserInvite, decision: DecisionAllow},
		{scope: ScopeServer, subject: RoleEveryone, permission: PermUserInvite, decision: DecisionDeny},
		{scope: ScopeServer, subjectKind: evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER, subject: "Udenied", permission: PermUserInvite, decision: DecisionDeny},
	}
	entries := rbacSeedEntries(defaultRBACRoles(), nil, decisions)
	entries[0].Expect = events.ExpectFilterSeq(evtstream.RBACSubjectFilter(), 0)
	_, err := h.publisher.AppendBatch(ctx, entries)
	require.NoError(t, err)
	c := &ChattoCore{EventPublisher: h.publisher}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { results <- c.splitInvitationPermissions(ctx) })
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	rbac, _ := replayRBACLog(t, h)
	for _, decision := range decisions {
		require.Equal(t, decision.decision, rbac.GetDecision(ScopeServer, "", decision.subject, PermUserManageInvites))
		require.Equal(t, decision.decision, rbac.GetDecision(ScopeServer, "", decision.subject, PermUserInvite))
	}
	clear := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacPermissionClearedEvent(ScopeServer, "", evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, RoleAdmin, PermUserManageInvites),
	}})
	_, seq := replayRBACLog(t, h)
	_, err = h.publisher.AppendBatch(ctx, []evtstream.BatchEntry{{Subject: rbacSubjectForEvent(clear), Event: clear, Expect: events.ExpectFilterSeq(evtstream.RBACSubjectFilter(), seq)}})
	require.NoError(t, err)
	_, before := replayRBACLog(t, h)
	require.NoError(t, c.splitInvitationPermissions(ctx))
	rbac, after := replayRBACLog(t, h)
	require.Equal(t, before, after)
	require.Equal(t, DecisionNone, rbac.GetDecision(ScopeServer, "", RoleAdmin, PermUserManageInvites))
}

func TestInvitationPermissionUpgradeClosesEmptyGate(t *testing.T) {
	t.Parallel()
	h := newTestEventHarness(t)
	ctx := testContext(t)
	c := &ChattoCore{EventPublisher: h.publisher}
	require.NoError(t, c.splitInvitationPermissions(ctx))
	entries := rbacSeedEntries(nil, nil, []rbacSeedDecision{
		{scope: ScopeServer, subject: RoleEveryone, permission: PermUserInvite, decision: DecisionAllow},
	})
	_, seq := replayRBACLog(t, h)
	entries[0].Expect = events.ExpectFilterSeq(evtstream.RBACSubjectFilter(), seq)
	_, err := h.publisher.AppendBatch(ctx, entries)
	require.NoError(t, err)
	require.NoError(t, c.splitInvitationPermissions(ctx))
	rbac, _ := replayRBACLog(t, h)
	require.Equal(t, DecisionNone, rbac.GetDecision(ScopeServer, "", RoleEveryone, PermUserManageInvites))
}

func TestInvitationPermissionBootstrap(t *testing.T) {
	t.Parallel()
	require.Contains(t, DefaultAdminPermissions(), PermUserInvite)
	require.Contains(t, DefaultAdminPermissions(), PermUserManageInvites)
	require.NotContains(t, DefaultEveryonePermissions(), PermUserInvite)
	require.NotContains(t, DefaultEveryonePermissions(), PermUserManageInvites)
}
