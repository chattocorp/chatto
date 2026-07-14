package core

import (
	"sync"
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestEnsureDefaultRolePermissions_SeedsEmptyRBACAtomically(t *testing.T) {
	harness := newTestEventHarness(t)
	core := newRBACDefaultsTestCore(t, harness)
	ctx := testContext(t)

	if err := core.EnsureDefaultRolePermissions(ctx); err != nil {
		t.Fatalf("EnsureDefaultRolePermissions: %v", err)
	}
	if got := core.RBAC.DefaultsVersion(ScopeServer, ""); got != serverRBACDefaultsVersion {
		t.Fatalf("server defaults version = %d, want %d", got, serverRBACDefaultsVersion)
	}
	for _, decision := range defaultRBACDecisions() {
		if got := core.RBAC.GetDecision(decision.scope, decision.scopeID, decision.subject, decision.permission); got != decision.decision {
			t.Errorf("decision for %s/%s = %s, want %s", decision.subject, decision.permission, got, decision.decision)
		}
	}

	before := rbacEventCount(t, core)
	if want := len(defaultRBACDecisions()) + 1; before != want {
		t.Fatalf("RBAC event count = %d, want %d defaults plus marker", before, want)
	}
	if err := core.EnsureDefaultRolePermissions(ctx); err != nil {
		t.Fatalf("EnsureDefaultRolePermissions second call: %v", err)
	}
	if after := rbacEventCount(t, core); after != before {
		t.Fatalf("idempotent ensure appended events: before=%d after=%d", before, after)
	}
}

func TestRBACDefaultsInitializedEntry_UsesScopedAggregate(t *testing.T) {
	server := rbacDefaultsInitializedEntry(ScopeServer, "", serverRBACDefaultsVersion, 17)
	if want := events.RBACServerAggregate().Subject(events.EventRBACDefaultsInitialized); server.Subject != want {
		t.Fatalf("server marker subject = %q, want %q", server.Subject, want)
	}
	room := rbacDefaultsInitializedEntry(ScopeRoom, "Rabc123", roomRBACDefaultsVersion, 0)
	if want := events.RBACScopedAggregate("Rabc123").Subject(events.EventRBACDefaultsInitialized); room.Subject != want {
		t.Fatalf("room marker subject = %q, want %q", room.Subject, want)
	}
}

func TestEnsureDefaultRolePermissions_MarksExistingRBACWithoutBackfill(t *testing.T) {
	harness := newTestEventHarness(t)
	core := newRBACDefaultsTestCore(t, harness)
	ctx := testContext(t)

	if err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermRoomCreate); err != nil {
		t.Fatalf("GrantServerPermission: %v", err)
	}
	if err := core.EnsureDefaultRolePermissions(ctx); err != nil {
		t.Fatalf("EnsureDefaultRolePermissions: %v", err)
	}

	if got := core.RBAC.GetDecision(ScopeServer, "", RoleEveryone, PermRoomCreate); got != DecisionAllow {
		t.Fatalf("existing decision = %s, want %s", got, DecisionAllow)
	}
	if got := core.RBAC.GetDecision(ScopeServer, "", RoleEveryone, PermRoomJoin); got != DecisionNone {
		t.Fatalf("missing default was backfilled: got %s", got)
	}
	if got := core.RBAC.DefaultsVersion(ScopeServer, ""); got != serverRBACDefaultsVersion {
		t.Fatalf("server defaults version = %d, want %d", got, serverRBACDefaultsVersion)
	}
	if got := rbacEventCount(t, core); got != 2 {
		t.Fatalf("RBAC event count = %d, want existing decision plus marker", got)
	}
}

func TestRBACDefaultsVersionAdvance_DoesNotReapplyDefaults(t *testing.T) {
	harness := newTestEventHarness(t)
	core := newRBACDefaultsTestCore(t, harness)
	ctx := testContext(t)

	if err := core.ensureRBACDefaultsInitialized(ctx, ScopeServer, "", 1, rbacDefaultsAdoptOnly, defaultRBACDecisions(), 0); err != nil {
		t.Fatalf("write version 1 marker: %v", err)
	}
	if err := core.ensureRBACDefaultsInitialized(ctx, ScopeServer, "", 2, rbacDefaultsSeedWhenEmpty, defaultRBACDecisions(), 0); err != nil {
		t.Fatalf("advance to version 2: %v", err)
	}

	if core.RBAC.HasAnyPermissionDecisions() {
		t.Fatal("version advance reapplied defaults")
	}
	if got := core.RBAC.DefaultsVersion(ScopeServer, ""); got != 2 {
		t.Fatalf("server defaults version = %d, want 2", got)
	}
}

func TestSeedDefaultChannelRoomPermissions_MarkerPreservesClear(t *testing.T) {
	harness := newTestEventHarness(t)
	core := newRBACDefaultsTestCore(t, harness)
	ctx := testContext(t)
	roomID := "Rannouncements"

	if err := core.SeedDefaultChannelRoomPermissions(ctx, roomID, AnnouncementsRoomName); err != nil {
		t.Fatalf("SeedDefaultChannelRoomPermissions: %v", err)
	}
	if got := core.RBAC.GetDecision(ScopeRoom, roomID, RoleEveryone, PermMessagePost); got != DecisionDeny {
		t.Fatalf("announcement decision = %s, want %s", got, DecisionDeny)
	}
	if got := core.RBAC.DefaultsVersion(ScopeRoom, roomID); got != roomRBACDefaultsVersion {
		t.Fatalf("room defaults version = %d, want %d", got, roomRBACDefaultsVersion)
	}

	if err := core.ClearRoomPermissionState(ctx, SystemActorID, roomID, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("ClearRoomPermissionState: %v", err)
	}
	if err := core.SeedDefaultChannelRoomPermissions(ctx, roomID, AnnouncementsRoomName); err != nil {
		t.Fatalf("SeedDefaultChannelRoomPermissions after clear: %v", err)
	}
	if got := core.RBAC.GetDecision(ScopeRoom, roomID, RoleEveryone, PermMessagePost); got != DecisionNone {
		t.Fatalf("cleared announcement decision was recreated: %s", got)
	}
}

func TestSeedDefaultChannelRoomPermissions_FillsMissingDefaultsAlongsideExistingDecision(t *testing.T) {
	harness := newTestEventHarness(t)
	core := newRBACDefaultsTestCore(t, harness)
	ctx := testContext(t)
	roomID := "Rpartial"

	if err := core.DenyRoomPermission(ctx, SystemActorID, roomID, RoleEveryone, PermMessageReact); err != nil {
		t.Fatalf("DenyRoomPermission unrelated override: %v", err)
	}
	if err := core.SeedDefaultChannelRoomPermissions(ctx, roomID, AnnouncementsRoomName); err != nil {
		t.Fatalf("SeedDefaultChannelRoomPermissions: %v", err)
	}

	if got := core.RBAC.GetDecision(ScopeRoom, roomID, RoleEveryone, PermMessageReact); got != DecisionDeny {
		t.Fatalf("existing unrelated decision = %s, want %s", got, DecisionDeny)
	}
	if got := core.RBAC.GetDecision(ScopeRoom, roomID, RoleEveryone, PermMessagePost); got != DecisionDeny {
		t.Fatalf("missing announcements default = %s, want %s", got, DecisionDeny)
	}
}

func TestSeedDefaultChannelRoomPermissions_PreservesExistingSameKeyDecision(t *testing.T) {
	harness := newTestEventHarness(t)
	core := newRBACDefaultsTestCore(t, harness)
	ctx := testContext(t)
	roomID := "Roverride"

	if err := core.GrantRoomPermission(ctx, SystemActorID, roomID, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("GrantRoomPermission override: %v", err)
	}
	if err := core.SeedDefaultChannelRoomPermissions(ctx, roomID, AnnouncementsRoomName); err != nil {
		t.Fatalf("SeedDefaultChannelRoomPermissions: %v", err)
	}

	if got := core.RBAC.GetDecision(ScopeRoom, roomID, RoleEveryone, PermMessagePost); got != DecisionAllow {
		t.Fatalf("same-key decision = %s, want preserved %s", got, DecisionAllow)
	}
}

func TestEnsureDefaultChannelRoomPermissions_RecoversPostCutoffRoomWithExistingDecision(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "Rpostcutoff"
	roomEvent := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RoomCreated{
		RoomCreated: &corev1.RoomCreatedEvent{
			RoomId: roomID,
			Kind:   corev1.RoomKind_ROOM_KIND_CHANNEL,
			Name:   AnnouncementsRoomName,
		},
	}})
	filterSeq, err := core.EventPublisher.LastSubjectSeq(ctx, events.RoomSubjectFilter())
	if err != nil {
		t.Fatalf("read room tail: %v", err)
	}
	roomSubject := events.RoomAggregate(roomID).SubjectFor(roomEvent)
	roomSeq, err := core.EventPublisher.AppendAtFilter(ctx, roomSubject, roomEvent, events.RoomSubjectFilter(), filterSeq)
	if err != nil {
		t.Fatalf("append post-cutoff room: %v", err)
	}
	if err := core.RoomDirectoryProjector.WaitFor(ctx, events.SubjectPosition(roomSubject, roomSeq)); err != nil {
		t.Fatalf("wait for post-cutoff room: %v", err)
	}
	if err := core.DenyRoomPermission(ctx, SystemActorID, roomID, RoleEveryone, PermMessageReact); err != nil {
		t.Fatalf("write unrelated partial decision: %v", err)
	}

	if err := core.EnsureDefaultChannelRoomPermissions(ctx); err != nil {
		t.Fatalf("EnsureDefaultChannelRoomPermissions: %v", err)
	}
	if got := core.RBAC.GetDecision(ScopeRoom, roomID, RoleEveryone, PermMessageReact); got != DecisionDeny {
		t.Fatalf("existing unrelated decision = %s, want %s", got, DecisionDeny)
	}
	if got := core.RBAC.GetDecision(ScopeRoom, roomID, RoleEveryone, PermMessagePost); got != DecisionDeny {
		t.Fatalf("recovered announcements default = %s, want %s", got, DecisionDeny)
	}
	if got := core.RBAC.DefaultsVersion(ScopeRoom, roomID); got != roomRBACDefaultsVersion {
		t.Fatalf("room defaults version = %d, want %d", got, roomRBACDefaultsVersion)
	}
}

func TestExistingRoomAdoption_PreservesCompletelyClearedScope(t *testing.T) {
	harness := newTestEventHarness(t)
	core := newRBACDefaultsTestCore(t, harness)
	ctx := testContext(t)
	roomID := "Rexisting"

	if err := core.DenyRoomPermission(ctx, SystemActorID, roomID, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	if err := core.ClearRoomPermissionState(ctx, SystemActorID, roomID, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("ClearRoomPermissionState: %v", err)
	}
	if err := core.ensureRBACDefaultsInitialized(
		ctx,
		ScopeRoom,
		roomID,
		roomRBACDefaultsVersion,
		rbacDefaultsAdoptOnly,
		defaultChannelRoomDecisions(roomID, AnnouncementsRoomName),
		0,
	); err != nil {
		t.Fatalf("adopt existing room: %v", err)
	}

	if got := core.RBAC.GetDecision(ScopeRoom, roomID, RoleEveryone, PermMessagePost); got != DecisionNone {
		t.Fatalf("existing cleared decision was recreated: %s", got)
	}
	if got := core.RBAC.DefaultsVersion(ScopeRoom, roomID); got != roomRBACDefaultsVersion {
		t.Fatalf("room defaults version = %d, want %d", got, roomRBACDefaultsVersion)
	}
}

func TestRoomDefaultsRolloutBoundaryUsesCreationSequence(t *testing.T) {
	core := &ChattoCore{RBAC: NewRBACProjection(), RoomCatalog: NewRoomCatalogProjection()}
	applyRoomCreated := func(roomID string, seq uint64) {
		t.Helper()
		event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{RoomId: roomID, Kind: corev1.RoomKind_ROOM_KIND_CHANNEL},
		}})
		if err := core.RoomCatalog.Apply(event, seq); err != nil {
			t.Fatalf("apply room creation: %v", err)
		}
	}

	applyRoomCreated("Rbefore", 10)
	applyRoomCreated("Rbetween", 30)
	marker := rbacDefaultsInitializedEntry(ScopeServer, "", serverRBACDefaultsVersion, 20)
	if err := core.RBAC.Apply(marker.Event, 40); err != nil {
		t.Fatalf("apply server marker: %v", err)
	}

	cutoff := core.RBAC.ServerDefaultsRoomStreamCutoff()
	if core.shouldRecoverUnmarkedRoom("Rbefore", true, cutoff) {
		t.Fatal("room created before the room cutoff would receive defaults")
	}
	if !core.shouldRecoverUnmarkedRoom("Rbetween", true, cutoff) {
		t.Fatal("room created after the cutoff but before the server marker would not recover defaults")
	}
}

func TestEnsureDefaultRolePermissions_PersistsRoomStreamCutoff(t *testing.T) {
	harness := newTestEventHarness(t)
	core := newRBACDefaultsTestCore(t, harness)
	ctx := testContext(t)
	roomID := "Rcutoff"
	roomEvent := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RoomCreated{
		RoomCreated: &corev1.RoomCreatedEvent{RoomId: roomID, Kind: corev1.RoomKind_ROOM_KIND_CHANNEL},
	}})
	roomSubject := events.RoomAggregate(roomID).SubjectFor(roomEvent)
	roomSeq, err := harness.publisher.AppendAtFilter(ctx, roomSubject, roomEvent, events.RoomSubjectFilter(), 0)
	if err != nil {
		t.Fatalf("append room creation: %v", err)
	}

	if err := core.EnsureDefaultRolePermissions(ctx); err != nil {
		t.Fatalf("EnsureDefaultRolePermissions: %v", err)
	}
	if got := core.RBAC.ServerDefaultsRoomStreamCutoff(); got != roomSeq {
		t.Fatalf("room stream cutoff = %d, want %d", got, roomSeq)
	}
}

func TestEnsureDefaultRolePermissions_ConcurrentInitializersConverge(t *testing.T) {
	harness := newTestEventHarness(t)
	coreA := newRBACDefaultsTestCore(t, harness)
	coreB := newRBACDefaultsTestCore(t, harness)
	ctx := testContext(t)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, core := range []*ChattoCore{coreA, coreB} {
		wg.Add(1)
		go func(core *ChattoCore) {
			defer wg.Done()
			<-start
			errs <- core.EnsureDefaultRolePermissions(ctx)
		}(core)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent EnsureDefaultRolePermissions: %v", err)
		}
	}

	if got, want := rbacEventCount(t, coreA), len(defaultRBACDecisions())+1; got != want {
		t.Fatalf("RBAC event count = %d, want one atomic initialization of %d events", got, want)
	}
}

func newRBACDefaultsTestCore(t *testing.T, harness *testEventHarness) *ChattoCore {
	t.Helper()
	projection := NewRBACProjection()
	projector := harness.projector(projection)
	core := &ChattoCore{
		EventPublisher: harness.publisher,
		RBAC:           projection,
		RBACProjector:  projector,
		logger:         testCoreLogger(),
	}
	core.rbacModel = newRBACModel(projection, projector)
	startTestProjector(t, projector)
	return core
}

func rbacEventCount(t *testing.T, core *ChattoCore) int {
	t.Helper()
	eventsFound, _, err := core.EventPublisher.SubjectEvents(testContext(t), events.RBACSubjectFilter())
	if err != nil {
		t.Fatalf("SubjectEvents(RBAC): %v", err)
	}
	return len(eventsFound)
}
