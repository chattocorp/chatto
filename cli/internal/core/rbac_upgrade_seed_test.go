package core

import (
	"maps"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// v04DefaultDecisions returns the fresh-server defaults without permissions
// introduced in 0.5, plus the 0.4 room-ban grants.
func v04DefaultDecisions() []rbacSeedDecision {
	introduced := map[Permission]bool{}
	for _, permission := range append(v05UpgradePermissions(), callPermissionIDs()...) {
		introduced[permission] = true
	}
	var decisions []rbacSeedDecision
	for _, decision := range defaultRBACDecisions() {
		if !introduced[decision.permission] {
			decisions = append(decisions, decision)
		}
	}
	for _, role := range []string{RoleAdmin, RoleModerator} {
		decisions = append(decisions, rbacSeedDecision{scope: ScopeServer, subject: role, permission: permRoomMemberBanLegacy, decision: DecisionAllow})
	}
	return decisions
}

func appendV04RBACLog(t *testing.T, h *testEventHarness, roles map[string]*evtv1.Role, extra []rbacSeedDecision, events ...*evtv1.Event) {
	t.Helper()
	ctx := testContext(t)
	maps.Copy(roles, defaultRBACRoles())
	entries := rbacSeedEntries(roles, nil, append(v04DefaultDecisions(), extra...))
	entries[0].HasOCC = true
	entries[0].FilterSubject = evtstream.RBACSubjectFilter()
	for _, event := range events {
		entries = append(entries, evtstream.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
	}
	_, err := h.publisher.AppendBatch(ctx, entries)
	require.NoError(t, err)
}

func replayRBACLog(t *testing.T, h *testEventHarness) (*RBACProjection, uint64) {
	t.Helper()
	history, seq, err := h.publisher.SubjectEvents(testContext(t), evtstream.RBACSubjectFilter())
	require.NoError(t, err)
	projection := NewRBACProjection()
	for i, event := range history {
		require.NoError(t, projection.Apply(event, uint64(i+1)))
	}
	return projection, seq
}

func TestUpgradePermissionsRestoreV04Behavior(t *testing.T) {
	h := newTestEventHarness(t)
	ctx := testContext(t)
	custom := &evtv1.Role{Name: "helper", DisplayName: "Helper", Position: PositionCustomFirst}
	gone := &evtv1.Role{Name: "gone", DisplayName: "Gone", Position: PositionCustomFirst + 1}
	legacyGrant := &evtv1.RbacPermissionGrantedEvent{Permission: string(permRoomMemberBanLegacy)}
	legacyGrant.ProtoReflect().SetUnknown(legacyRBACPermissionUnknown("Glegacy", "legacy-role"))
	appendV04RBACLog(t, h,
		map[string]*evtv1.Role{custom.Name: custom, gone.Name: gone},
		[]rbacSeedDecision{
			{scope: ScopeRoom, scopeID: "Rhelp", subject: custom.Name, permission: permRoomMemberBanLegacy, decision: DecisionAllow},
			{scope: ScopeGroup, scopeID: "Gmods", subjectKind: evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER, subject: "Ualice", permission: permRoomMemberBanLegacy, decision: DecisionDeny},
			{scope: ScopeServer, subject: gone.Name, permission: permRoomMemberBanLegacy, decision: DecisionAllow},
			{scope: ScopeServer, subject: custom.Name, permission: permRoomMemberBanLegacy, decision: DecisionAllow},
		},
		newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleDeleted{RbacRoleDeleted: &evtv1.RbacRoleDeletedEvent{RoleName: gone.Name}}}),
		newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionCleared{RbacPermissionCleared: rbacPermissionClearedEvent(ScopeServer, "", evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, custom.Name, permRoomMemberBanLegacy)}}),
		newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionGranted{RbacPermissionGranted: legacyGrant}}),
	)

	core := &ChattoCore{EventPublisher: h.publisher}
	require.NoError(t, core.seedUpgradePermissions(ctx))

	rbac, seq := replayRBACLog(t, h)
	for _, want := range []struct {
		scope   PermissionScope
		scopeID string
		subject string
		perm    Permission
		kind    DecisionKind
	}{
		{ScopeServer, "", RoleEveryone, PermMessageRead, DecisionAllow},
		{ScopeServer, "", RoleEveryone, PermCallStart, DecisionAllow},
		{ScopeServer, "", RoleAdmin, PermRoomMemberRemove, DecisionAllow},
		{ScopeServer, "", RoleModerator, PermRoomMemberRemove, DecisionAllow},
		{ScopeRoom, "Rhelp", custom.Name, PermRoomMemberRemove, DecisionAllow},
		{ScopeGroup, "Gmods", "Ualice", PermRoomMemberRemove, DecisionDeny},
		{ScopeGroup, "Glegacy", "legacy-role", PermRoomMemberRemove, DecisionAllow},
		// A cleared ban decision and a deleted role's decision are not copied.
		{ScopeServer, "", custom.Name, PermRoomMemberRemove, DecisionNone},
		{ScopeServer, "", gone.Name, PermRoomMemberRemove, DecisionNone},
		// Capabilities that are new in 0.5 stay off until an operator grants them.
		{ScopeServer, "", RoleEveryone, PermBotCreate, DecisionNone},
		{ScopeServer, "", RoleAdmin, PermBotCreate, DecisionNone},
		{ScopeServer, "", RoleAdmin, PermBotManage, DecisionNone},
		{ScopeServer, "", RoleAdmin, PermUserInvite, DecisionNone},
		{ScopeServer, "", RoleAdmin, PermServerManageNeighbors, DecisionNone},
	} {
		require.Equal(t, want.kind, rbac.GetDecision(want.scope, want.scopeID, want.subject, want.perm), "%s %s %s %s", want.scope, want.scopeID, want.subject, want.perm)
	}

	// Restart after the committed upgrade is a no-op.
	require.NoError(t, core.seedUpgradePermissions(ctx))
	_, after := replayRBACLog(t, h)
	require.Equal(t, seq, after)
}

// Any 0.5 decision, including a clear, shows that an operator or fresh
// bootstrap already owns the 0.5 permission state.
func TestUpgradePermissionsSkipReviewedV05State(t *testing.T) {
	for _, event := range []*evtv1.Event{
		newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionCleared{RbacPermissionCleared: rbacPermissionClearedEvent(ScopeServer, "", evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, RoleEveryone, PermMessageRead)}}),
		newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionDenied{RbacPermissionDenied: rbacPermissionDeniedEvent(ScopeRoom, "Rone", evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER, "Ubob", PermMessagePostInInteractions)}}),
	} {
		h := newTestEventHarness(t)
		ctx := testContext(t)
		appendV04RBACLog(t, h, map[string]*evtv1.Role{}, nil, event)
		require.NoError(t, (&ChattoCore{EventPublisher: h.publisher}).seedUpgradePermissions(ctx))
		rbac, _ := replayRBACLog(t, h)
		require.Equal(t, DecisionAllow, rbac.GetDecision(ScopeServer, "", RoleEveryone, PermCallJoin))
		for _, check := range []struct {
			subject string
			perm    Permission
		}{
			{RoleEveryone, PermMessageRead},
			{RoleModerator, PermRoomMemberRemove},
		} {
			require.Equal(t, DecisionNone, rbac.GetDecision(ScopeServer, "", check.subject, check.perm), "%s %s", check.subject, check.perm)
		}
	}
}

func TestUpgradePermissionsLeaveFreshServerUnchanged(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	before, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.RBACSubjectFilter())
	require.NoError(t, err)
	require.NoError(t, c.seedUpgradePermissions(ctx))
	after, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.RBACSubjectFilter())
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestUpgradePermissionsConcurrentReplicas(t *testing.T) {
	h := newTestEventHarness(t)
	ctx := testContext(t)
	appendV04RBACLog(t, h, map[string]*evtv1.Role{}, nil)
	var group sync.WaitGroup
	results := make(chan error, 3)
	for range 3 {
		group.Go(func() {
			results <- (&ChattoCore{EventPublisher: h.publisher}).seedUpgradePermissions(ctx)
		})
	}
	group.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	history, _, err := h.publisher.SubjectEvents(ctx, evtstream.RBACSubjectFilter())
	require.NoError(t, err)
	grants := map[string]int{}
	for _, event := range history {
		if granted := event.GetRbacPermissionGranted(); granted != nil {
			grants[granted.GetSubject().GetId()+" "+granted.GetPermission()]++
		}
	}
	for _, key := range []string{"everyone message.read", "moderator room.remove-member", "everyone call.start"} {
		require.Equal(t, 1, grants[key], key)
	}
}
