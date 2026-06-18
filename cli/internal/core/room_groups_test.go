package core

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func groupIDOfTestGroupEvent(t *testing.T, event *corev1.Event) string {
	t.Helper()
	switch e := event.GetEvent().(type) {
	case *corev1.Event_RoomGroupCreated:
		return e.RoomGroupCreated.GetGroupId()
	case *corev1.Event_RoomGroupUpdated:
		return e.RoomGroupUpdated.GetGroupId()
	case *corev1.Event_RoomGroupDeleted:
		return e.RoomGroupDeleted.GetGroupId()
	case *corev1.Event_RoomAddedToGroup:
		return e.RoomAddedToGroup.GetGroupId()
	case *corev1.Event_RoomRemovedFromGroup:
		return e.RoomRemovedFromGroup.GetGroupId()
	case *corev1.Event_RoomsInGroupReordered:
		return e.RoomsInGroupReordered.GetGroupId()
	case *corev1.Event_SidebarLinkAddedToGroup:
		return e.SidebarLinkAddedToGroup.GetGroupId()
	case *corev1.Event_SidebarLinkUpdated:
		return e.SidebarLinkUpdated.GetGroupId()
	case *corev1.Event_SidebarLinkRemovedFromGroup:
		return e.SidebarLinkRemovedFromGroup.GetGroupId()
	case *corev1.Event_SidebarGroupEntriesReordered:
		return e.SidebarGroupEntriesReordered.GetGroupId()
	default:
		t.Fatalf("unsupported test group event %T", event.GetEvent())
		return ""
	}
}

func TestCreateRoomGroup(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, err := core.CreateRoomGroup(ctx, "actor", "Engineering", "Eng team rooms")
	if err != nil {
		t.Fatalf("CreateRoomGroup failed: %v", err)
	}
	if set.Name != "Engineering" {
		t.Errorf("Name = %q, want %q", set.Name, "Engineering")
	}
	if set.Description != "Eng team rooms" {
		t.Errorf("Description = %q, want %q", set.Description, "Eng team rooms")
	}
	if set.Id == "" {
		t.Error("Expected an ID to be assigned")
	}

	// Verify persisted. The seed "Lobby" group is created at boot, so the
	// reconciled list contains it plus the just-created Engineering group.
	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered failed: %v", err)
	}
	found := false
	for _, g := range groups {
		if g.Id == set.Id {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("New group not present in reconciled list: %+v", groups)
	}
}

func TestCreateRoomGroup_TrimsName(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, err := core.CreateRoomGroup(ctx, "actor", "  General  ", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup failed: %v", err)
	}
	if set.Name != "General" {
		t.Errorf("Name = %q, want trimmed %q", set.Name, "General")
	}
}

func TestCreateRoomGroup_EmptyNameRejected(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	for _, name := range []string{"", "   ", "\t"} {
		_, err := core.CreateRoomGroup(ctx, "actor", name, "")
		if !errors.Is(err, ErrRoomGroupNameEmpty) {
			t.Errorf("CreateRoomGroup(%q) error = %v, want ErrRoomGroupNameEmpty", name, err)
		}
	}
}

func TestRoomGroupMetadataLengthLimits(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("create accepts values at max length", func(t *testing.T) {
		set, err := core.CreateRoomGroup(
			ctx,
			"actor",
			strings.Repeat("n", MaxRoomGroupNameLength),
			strings.Repeat("d", MaxRoomGroupDescriptionLength),
		)
		if err != nil {
			t.Fatalf("CreateRoomGroup at max lengths: %v", err)
		}
		if len(set.Name) != MaxRoomGroupNameLength || len(set.Description) != MaxRoomGroupDescriptionLength {
			t.Fatalf("created group lengths = name:%d description:%d", len(set.Name), len(set.Description))
		}
	})

	t.Run("create rejects over-limit name", func(t *testing.T) {
		_, err := core.CreateRoomGroup(ctx, "actor", strings.Repeat("n", MaxRoomGroupNameLength+1), "")
		assertStringLengthError(t, err, "room group name", MaxRoomGroupNameLength)
	})

	t.Run("create rejects over-limit description", func(t *testing.T) {
		_, err := core.CreateRoomGroup(ctx, "actor", "Group", strings.Repeat("d", MaxRoomGroupDescriptionLength+1))
		assertStringLengthError(t, err, "room group description", MaxRoomGroupDescriptionLength)
	})

	t.Run("update rejects over-limit metadata", func(t *testing.T) {
		set, err := core.CreateRoomGroup(ctx, "actor", "Short", "")
		if err != nil {
			t.Fatalf("CreateRoomGroup: %v", err)
		}
		_, err = core.UpdateRoomGroup(ctx, "actor", set.Id, "Short", strings.Repeat("d", MaxRoomGroupDescriptionLength+1))
		assertStringLengthError(t, err, "room group description", MaxRoomGroupDescriptionLength)
	})
}

func TestUpdateRoomGroup(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomGroup(ctx, "actor", "Old Name", "old desc")
	updated, err := core.UpdateRoomGroup(ctx, "actor", set.Id, "New Name", "new desc")
	if err != nil {
		t.Fatalf("UpdateRoomGroup failed: %v", err)
	}
	if updated.Name != "New Name" || updated.Description != "new desc" {
		t.Errorf("Update mismatch: %+v", updated)
	}
	if updated.Id != set.Id {
		t.Errorf("ID changed: %q → %q", set.Id, updated.Id)
	}
}

func TestUpdateRoomGroup_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.UpdateRoomGroup(ctx, "actor", "nonexistent", "x", "")
	if !errors.Is(err, ErrRoomGroupNotFound) {
		t.Errorf("UpdateRoomGroup on missing set: err = %v, want ErrRoomGroupNotFound", err)
	}
}

func TestGetRoomGroup(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	created, _ := core.CreateRoomGroup(ctx, "actor", "Engineering", "")
	got, err := core.GetRoomGroup(ctx, created.Id)
	if err != nil {
		t.Fatalf("GetRoomGroup failed: %v", err)
	}
	if got.Id != created.Id || got.Name != "Engineering" {
		t.Errorf("GetRoomGroup mismatch: got %+v, want id=%q name=%q", got, created.Id, "Engineering")
	}
}

func TestGetRoomGroup_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.GetRoomGroup(ctx, "nonexistent")
	if !errors.Is(err, ErrRoomGroupNotFound) {
		t.Errorf("GetRoomGroup on missing set: err = %v, want ErrRoomGroupNotFound", err)
	}
}

func TestDeleteRoomGroup_Empty(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomGroup(ctx, "actor", "Empty", "")
	if err := core.DeleteRoomGroup(ctx, "actor", set.Id); err != nil {
		t.Fatalf("DeleteRoomGroup failed: %v", err)
	}

	_, err := core.GetRoomGroup(ctx, set.Id)
	if !errors.Is(err, ErrRoomGroupNotFound) {
		t.Errorf("Set still exists after deletion: err = %v", err)
	}
}

func TestDeleteRoomGroup_RejectsNonEmpty(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomGroup(ctx, "actor", "WithRooms", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")
	if err := core.MoveRoomToGroup(ctx, "actor", room.Id, set.Id); err != nil {
		t.Fatalf("MoveRoomToGroup failed: %v", err)
	}

	err := core.DeleteRoomGroup(ctx, "actor", set.Id)
	if !errors.Is(err, ErrRoomGroupHasRooms) {
		t.Errorf("DeleteRoomGroup on populated set: err = %v, want ErrRoomGroupHasRooms", err)
	}
}

func TestMoveRoomToSet(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	setA, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	setB, _ := core.CreateRoomGroup(ctx, "actor", "B", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")

	if err := core.MoveRoomToGroup(ctx, "actor", room.Id, setA.Id); err != nil {
		t.Fatalf("MoveRoomToGroup A failed: %v", err)
	}

	gotA, _ := core.GetRoomGroup(ctx, setA.Id)
	if len(gotA.RoomIds) != 1 || gotA.RoomIds[0] != room.Id {
		t.Errorf("Set A should contain the room: %+v", gotA.RoomIds)
	}

	// Move to set B; room should leave A
	if err := core.MoveRoomToGroup(ctx, "actor", room.Id, setB.Id); err != nil {
		t.Fatalf("MoveRoomToGroup B failed: %v", err)
	}

	gotA, _ = core.GetRoomGroup(ctx, setA.Id)
	gotB, _ := core.GetRoomGroup(ctx, setB.Id)
	if len(gotA.RoomIds) != 0 {
		t.Errorf("Set A should be empty after move: %+v", gotA.RoomIds)
	}
	if len(gotB.RoomIds) != 1 || gotB.RoomIds[0] != room.Id {
		t.Errorf("Set B should contain the room: %+v", gotB.RoomIds)
	}
}

func TestMoveRoomToSet_TargetNotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")
	err := core.MoveRoomToGroup(ctx, "actor", room.Id, "nonexistent")
	if !errors.Is(err, ErrRoomGroupNotFound) {
		t.Errorf("err = %v, want ErrRoomGroupNotFound", err)
	}
}

func TestMoveRoomToSet_FromSourceRejectsChangedSource(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	setA, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	setB, _ := core.CreateRoomGroup(ctx, "actor", "B", "")
	setC, _ := core.CreateRoomGroup(ctx, "actor", "C", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")

	if err := core.MoveRoomToGroup(ctx, "actor", room.Id, setA.Id); err != nil {
		t.Fatalf("MoveRoomToGroup A failed: %v", err)
	}
	if err := core.MoveRoomToGroup(ctx, "other-actor", room.Id, setB.Id); err != nil {
		t.Fatalf("MoveRoomToGroup B failed: %v", err)
	}

	err := core.MoveRoomToGroupFromSource(ctx, "actor", room.Id, setA.Id, setC.Id)
	if !errors.Is(err, ErrRoomMoveSourceChanged) {
		t.Fatalf("MoveRoomToGroupFromSource err = %v, want ErrRoomMoveSourceChanged", err)
	}

	gotB, _ := core.GetRoomGroup(ctx, setB.Id)
	if len(gotB.RoomIds) != 1 || gotB.RoomIds[0] != room.Id {
		t.Fatalf("Set B should still contain the room after rejected move: %+v", gotB.RoomIds)
	}
	gotC, _ := core.GetRoomGroup(ctx, setC.Id)
	if len(gotC.RoomIds) != 0 {
		t.Fatalf("Set C should remain empty after rejected move: %+v", gotC.RoomIds)
	}
}

func TestMoveRoomToSet_FromSourceRejectsChangedSourceAfterOCCRetry(t *testing.T) {
	harness := newTestEventHarness(t)
	ctx := testContext(t)

	groupLayout := NewRoomGroupLayoutProjection()
	groupLayoutProjector := harness.projector(groupLayout)
	core := &ChattoCore{
		nc:                       harness.nc,
		logger:                   testServiceLogger(),
		EventPublisher:           harness.publisher,
		RoomGroupLayout:          groupLayout,
		RoomGroupLayoutProjector: groupLayoutProjector,
		RoomGroups:               groupLayout.Groups,
		RoomLayout:               groupLayout.Layout,
	}
	core.roomService = newRoomService(nil, nil, groupLayout, groupLayoutProjector, nil, nil, nil, nil, nil, nil)

	eventsToAppend := []*corev1.Event{
		newEvent("actor", groupCreatedEvent("G-source", "Source", "")),
		newEvent("actor", groupCreatedEvent("G-other", "Other", "")),
		newEvent("actor", groupCreatedEvent("G-target", "Target", "")),
		newEvent("actor", roomAddedToGroupEvent("G-source", "R1")),
		newEvent("other-actor", roomRemovedFromGroupEvent("G-source", "R1")),
		newEvent("other-actor", roomAddedToGroupEvent("G-other", "R1")),
	}
	for i, event := range eventsToAppend {
		subject := events.GroupAggregate(groupIDOfTestGroupEvent(t, event)).SubjectFor(event)
		seq, err := harness.publisher.AppendEventually(ctx, subject, event)
		if err != nil {
			t.Fatalf("append setup event %d: %v", i, err)
		}
		if i < 4 {
			if err := groupLayout.Apply(event, seq); err != nil {
				t.Fatalf("seed stale group projection event %d: %v", i, err)
			}
		}
	}
	if got := core.RoomGroups.GroupForRoom("R1"); got != "G-source" {
		t.Fatalf("test setup source group = %q, want stale G-source", got)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- core.MoveRoomToGroupFromSource(ctx, "actor", "R1", "G-source", "G-target")
	}()

	select {
	case err := <-errCh:
		t.Fatalf("MoveRoomToGroupFromSource returned before OCC retry catch-up: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	startTestProjector(t, groupLayoutProjector)
	if err := <-errCh; !errors.Is(err, ErrRoomMoveSourceChanged) {
		t.Fatalf("MoveRoomToGroupFromSource after OCC retry err = %v, want ErrRoomMoveSourceChanged", err)
	}
	if got := core.RoomGroups.GroupForRoom("R1"); got != "G-other" {
		t.Fatalf("room source after rejected move = %q, want G-other", got)
	}
	target, ok := core.RoomGroups.Get("G-target")
	if !ok {
		t.Fatal("target group missing after catch-up")
	}
	if len(target.RoomIds) != 0 {
		t.Fatalf("target room IDs = %v, want empty", target.RoomIds)
	}
}

func TestMoveRoomToSet_TargetCreatedBeforeProjectionCatchup(t *testing.T) {
	harness := newTestEventHarness(t)
	ctx := testContext(t)

	groupLayout := NewRoomGroupLayoutProjection()
	groupLayoutProjector := harness.projector(groupLayout)
	core := &ChattoCore{
		nc:                       harness.nc,
		logger:                   testServiceLogger(),
		EventPublisher:           harness.publisher,
		RoomGroupLayout:          groupLayout,
		RoomGroupLayoutProjector: groupLayoutProjector,
		RoomGroups:               groupLayout.Groups,
		RoomLayout:               groupLayout.Layout,
	}
	core.roomService = newRoomService(nil, nil, groupLayout, groupLayoutProjector, nil, nil, nil, nil, nil, nil)

	created := newEvent("actor", &corev1.Event{
		Event: &corev1.Event_RoomGroupCreated{
			RoomGroupCreated: &corev1.RoomGroupCreatedEvent{GroupId: "G-late", Name: "Late"},
		},
	})
	if _, err := harness.publisher.AppendEventually(ctx, events.GroupAggregate("G-late").SubjectFor(created), created); err != nil {
		t.Fatalf("append group-created event: %v", err)
	}
	if core.RoomGroups.Exists("G-late") {
		t.Fatal("test setup expected group projection to start stale")
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- core.MoveRoomToGroup(ctx, "actor", "R-late", "G-late")
	}()

	select {
	case err := <-errCh:
		t.Fatalf("MoveRoomToGroup returned before projection catch-up: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	startTestProjector(t, groupLayoutProjector)
	if err := <-errCh; err != nil {
		t.Fatalf("MoveRoomToGroup after projection catch-up: %v", err)
	}

	group, ok := core.RoomGroups.Get("G-late")
	if !ok {
		t.Fatal("target group missing after catch-up")
	}
	if len(group.RoomIds) != 1 || group.RoomIds[0] != "R-late" {
		t.Fatalf("group room IDs = %v, want [R-late]", group.RoomIds)
	}
}

func TestMoveRoomToSet_IdempotentNoopRefreshesStaleSnapshot(t *testing.T) {
	harness := newTestEventHarness(t)
	ctx := testContext(t)

	groupLayout := NewRoomGroupLayoutProjection()
	groupLayoutProjector := harness.projector(groupLayout)
	core := &ChattoCore{
		nc:                       harness.nc,
		logger:                   testServiceLogger(),
		EventPublisher:           harness.publisher,
		RoomGroupLayout:          groupLayout,
		RoomGroupLayoutProjector: groupLayoutProjector,
		RoomGroups:               groupLayout.Groups,
		RoomLayout:               groupLayout.Layout,
	}
	core.roomService = newRoomService(nil, nil, groupLayout, groupLayoutProjector, nil, nil, nil, nil, nil, nil)

	eventsToAppend := []*corev1.Event{
		newEvent("actor", groupCreatedEvent("G-target", "Target", "")),
		newEvent("actor", groupCreatedEvent("G-other", "Other", "")),
		newEvent("actor", roomAddedToGroupEvent("G-target", "R1")),
		newEvent("actor", roomRemovedFromGroupEvent("G-target", "R1")),
		newEvent("actor", roomAddedToGroupEvent("G-other", "R1")),
	}
	for i, event := range eventsToAppend {
		subject := events.GroupAggregate(groupIDOfTestGroupEvent(t, event)).SubjectFor(event)
		seq, err := harness.publisher.AppendEventually(ctx, subject, event)
		if err != nil {
			t.Fatalf("append setup event %d: %v", i, err)
		}
		if i < 3 {
			if err := groupLayout.Apply(event, seq); err != nil {
				t.Fatalf("seed stale group projection event %d: %v", i, err)
			}
		}
	}
	if got := core.RoomGroups.GroupForRoom("R1"); got != "G-target" {
		t.Fatalf("test setup source group = %q, want stale G-target", got)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- core.MoveRoomToGroup(ctx, "actor", "R1", "G-target")
	}()

	select {
	case err := <-errCh:
		t.Fatalf("MoveRoomToGroup returned before stale no-op catch-up: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	startTestProjector(t, groupLayoutProjector)
	if err := <-errCh; err != nil {
		t.Fatalf("MoveRoomToGroup after stale no-op catch-up: %v", err)
	}

	target, ok := core.RoomGroups.Get("G-target")
	if !ok {
		t.Fatal("target group missing after catch-up")
	}
	if len(target.RoomIds) != 1 || target.RoomIds[0] != "R1" {
		t.Fatalf("target room IDs = %v, want [R1]", target.RoomIds)
	}
	other, ok := core.RoomGroups.Get("G-other")
	if !ok {
		t.Fatal("other group missing after catch-up")
	}
	if len(other.RoomIds) != 0 {
		t.Fatalf("other room IDs = %v, want empty", other.RoomIds)
	}
}

func TestMoveRoomToSet_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomGroup(ctx, "actor", "S", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")

	if err := core.MoveRoomToGroup(ctx, "actor", room.Id, set.Id); err != nil {
		t.Fatalf("first move failed: %v", err)
	}
	if err := core.MoveRoomToGroup(ctx, "actor", room.Id, set.Id); err != nil {
		t.Fatalf("second move (idempotent) failed: %v", err)
	}

	got, _ := core.GetRoomGroup(ctx, set.Id)
	if len(got.RoomIds) != 1 {
		t.Errorf("Room appears %d times in set, want exactly 1", len(got.RoomIds))
	}
}

func TestMoveRoomToSet_ConcurrentMovesLeaveSingleAssignment(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	setA, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	setB, _ := core.CreateRoomGroup(ctx, "actor", "B", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")

	for i := 0; i < 25; i++ {
		if err := core.MoveRoomToGroup(ctx, "actor", room.Id, setA.Id); err != nil {
			t.Fatalf("reset move to A failed: %v", err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, target := range []string{setB.Id, setA.Id} {
			wg.Add(1)
			go func(target string) {
				defer wg.Done()
				<-start
				errs <- core.MoveRoomToGroup(ctx, "actor", room.Id, target)
			}(target)
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("concurrent MoveRoomToGroup failed: %v", err)
			}
		}

		groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
		if err != nil {
			t.Fatalf("ListRoomGroupsOrdered: %v", err)
		}
		assignments := 0
		for _, group := range groups {
			for _, roomID := range group.RoomIds {
				if roomID == room.Id {
					assignments++
				}
			}
		}
		if assignments != 1 {
			t.Fatalf("iteration %d: room has %d group assignments, want exactly 1", i, assignments)
		}
	}
}

func TestSidebarLinkLifecycleAndOrdering(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	group, _ := core.CreateRoomGroup(ctx, "actor", "Links", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, group.Id, "general", "")
	link, err := core.CreateSidebarLink(ctx, "actor", group.Id, "Docs", "https://example.com/docs")
	if err != nil {
		t.Fatalf("CreateSidebarLink: %v", err)
	}

	got, err := core.GetRoomGroup(ctx, group.Id)
	if err != nil {
		t.Fatalf("GetRoomGroup: %v", err)
	}
	if len(got.GetSidebarLinks()) != 1 || got.GetSidebarLinks()[0].GetId() != link.Id {
		t.Fatalf("sidebar links = %+v, want created link", got.GetSidebarLinks())
	}
	if got.GetEntries()[0].GetKind() != corev1.SidebarGroupEntry_ROOM || got.GetEntries()[0].GetId() != room.Id {
		t.Fatalf("first entry = %+v, want room %s", got.GetEntries()[0], room.Id)
	}
	if got.GetEntries()[1].GetKind() != corev1.SidebarGroupEntry_SIDEBAR_LINK || got.GetEntries()[1].GetId() != link.Id {
		t.Fatalf("second entry = %+v, want link %s", got.GetEntries()[1], link.Id)
	}

	updated, err := core.UpdateSidebarLink(ctx, "actor", link.Id, "Documentation", "https://example.com/reference")
	if err != nil {
		t.Fatalf("UpdateSidebarLink: %v", err)
	}
	if updated.Label != "Documentation" || updated.Url != "https://example.com/reference" {
		t.Fatalf("updated link = %+v", updated)
	}

	if err := core.ReorderSidebarItemsInGroup(ctx, "actor", group.Id, []*corev1.SidebarGroupEntry{
		{Kind: corev1.SidebarGroupEntry_SIDEBAR_LINK, Id: link.Id},
		{Kind: corev1.SidebarGroupEntry_ROOM, Id: room.Id},
	}); err != nil {
		t.Fatalf("ReorderSidebarItemsInGroup: %v", err)
	}
	got, _ = core.GetRoomGroup(ctx, group.Id)
	if got.GetEntries()[0].GetKind() != corev1.SidebarGroupEntry_SIDEBAR_LINK {
		t.Fatalf("entries after reorder = %+v", got.GetEntries())
	}
	if len(got.GetRoomIds()) != 1 || got.GetRoomIds()[0] != room.Id {
		t.Fatalf("room_ids after mixed reorder = %v, want [%s]", got.GetRoomIds(), room.Id)
	}

	if err := core.DeleteSidebarLink(ctx, "actor", link.Id); err != nil {
		t.Fatalf("DeleteSidebarLink: %v", err)
	}
	got, _ = core.GetRoomGroup(ctx, group.Id)
	if len(got.GetSidebarLinks()) != 0 {
		t.Fatalf("links after delete = %+v, want empty", got.GetSidebarLinks())
	}
	for _, entry := range got.GetEntries() {
		if entry.GetKind() == corev1.SidebarGroupEntry_SIDEBAR_LINK {
			t.Fatalf("link entry survived delete: %+v", got.GetEntries())
		}
	}
}

func TestMoveSidebarLinkToGroup(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	source, _ := core.CreateRoomGroup(ctx, "actor", "Source", "")
	target, _ := core.CreateRoomGroup(ctx, "actor", "Target", "")
	link, err := core.CreateSidebarLink(ctx, "actor", source.Id, "Status", "https://status.example.com")
	if err != nil {
		t.Fatalf("CreateSidebarLink: %v", err)
	}

	if err := core.MoveSidebarLinkToGroup(ctx, "actor", link.Id, target.Id); err != nil {
		t.Fatalf("MoveSidebarLinkToGroup: %v", err)
	}

	sourceGroup, _ := core.GetRoomGroup(ctx, source.Id)
	targetGroup, _ := core.GetRoomGroup(ctx, target.Id)
	if len(sourceGroup.GetSidebarLinks()) != 0 {
		t.Fatalf("source links = %+v, want empty", sourceGroup.GetSidebarLinks())
	}
	if len(targetGroup.GetSidebarLinks()) != 1 || targetGroup.GetSidebarLinks()[0].GetId() != link.Id {
		t.Fatalf("target links = %+v, want moved link", targetGroup.GetSidebarLinks())
	}
}

func TestSidebarLinkCreateAndGroupDeleteDoNotBothWin(t *testing.T) {
	for i := 0; i < 30; i++ {
		core, _ := setupTestCore(t)
		ctx := testContext(t)
		group, _ := core.CreateRoomGroup(ctx, "actor", "Race", "")

		start := make(chan struct{})
		createDone := make(chan error, 1)
		deleteDone := make(chan error, 1)
		go func() {
			<-start
			_, err := core.CreateSidebarLink(ctx, "actor", group.Id, "Docs", "https://example.com/docs")
			createDone <- err
		}()
		go func() {
			<-start
			deleteDone <- core.DeleteRoomGroup(ctx, "actor", group.Id)
		}()
		close(start)

		createErr := <-createDone
		deleteErr := <-deleteDone
		if createErr == nil && deleteErr == nil {
			t.Fatalf("iteration %d: create link and delete group both succeeded", i)
		}
		if createErr == nil {
			got, err := core.GetRoomGroup(ctx, group.Id)
			if err != nil {
				t.Fatalf("iteration %d: created link but group missing: %v", i, err)
			}
			if len(got.GetSidebarLinks()) != 1 {
				t.Fatalf("iteration %d: created link but group links = %+v", i, got.GetSidebarLinks())
			}
		}
	}
}

func TestMoveSidebarLinkToGroupPreservesConcurrentUpdate(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	source, _ := core.CreateRoomGroup(ctx, "actor", "Source", "")
	target, _ := core.CreateRoomGroup(ctx, "actor", "Target", "")
	link, err := core.CreateSidebarLink(ctx, "actor", source.Id, "Old", "https://old.example.com")
	if err != nil {
		t.Fatalf("CreateSidebarLink: %v", err)
	}

	staleSnapshot := core.RoomGroups.SidebarLinkMoveSnapshot(link.Id, target.Id)
	if staleSnapshot.Link == nil {
		t.Fatal("expected stale move snapshot to include sidebar link")
	}
	if _, err := core.UpdateSidebarLink(ctx, "actor", link.Id, "New", "https://new.example.com"); err != nil {
		t.Fatalf("UpdateSidebarLink: %v", err)
	}

	removed := newEvent("actor", &corev1.Event{
		Event: &corev1.Event_SidebarLinkRemovedFromGroup{
			SidebarLinkRemovedFromGroup: &corev1.SidebarLinkRemovedFromGroupEvent{
				GroupId: staleSnapshot.SourceGroupID,
				LinkId:  link.Id,
			},
		},
	})
	added := newEvent("actor", &corev1.Event{
		Event: &corev1.Event_SidebarLinkAddedToGroup{
			SidebarLinkAddedToGroup: &corev1.SidebarLinkAddedToGroupEvent{
				GroupId: target.Id,
				LinkId:  link.Id,
				Label:   staleSnapshot.Link.Label,
				Url:     staleSnapshot.Link.Url,
			},
		},
	})
	_, err = core.EventPublisher.AppendBatch(ctx, []events.BatchEntry{
		{
			Subject:       events.GroupAggregate(staleSnapshot.SourceGroupID).SubjectFor(removed),
			Event:         removed,
			HasOCC:        true,
			ExpectedSeq:   staleSnapshot.Seq,
			FilterSubject: events.GroupSubjectFilter(),
		},
		{
			Subject: events.GroupAggregate(target.Id).SubjectFor(added),
			Event:   added,
		},
	})
	if !errors.Is(err, events.ErrConflict) {
		t.Fatalf("stale move err = %v, want ErrConflict", err)
	}

	if err := core.MoveSidebarLinkToGroup(ctx, "actor", link.Id, target.Id); err != nil {
		t.Fatalf("MoveSidebarLinkToGroup: %v", err)
	}

	targetGroup, err := core.GetRoomGroup(ctx, target.Id)
	if err != nil {
		t.Fatalf("GetRoomGroup(target): %v", err)
	}
	if len(targetGroup.GetSidebarLinks()) != 1 {
		t.Fatalf("target links = %+v, want one", targetGroup.GetSidebarLinks())
	}
	got := targetGroup.GetSidebarLinks()[0]
	if got.GetLabel() != "New" || got.GetUrl() != "https://new.example.com" {
		t.Fatalf("moved link = %+v, want updated label/url", got)
	}
}

func TestReorderRoomGroups(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// The boot seed creates one "Lobby" group; capture it so we can include
	// it in the reorder list (ReorderRoomGroups requires a permutation of
	// every existing group).
	seedGroups, _ := core.ListRoomGroupsOrdered(ctx, KindChannel)
	seedID := seedGroups[0].Id

	a, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	b, _ := core.CreateRoomGroup(ctx, "actor", "B", "")
	c, _ := core.CreateRoomGroup(ctx, "actor", "C", "")

	if err := core.ReorderRoomGroups(ctx, "actor", []string{c.Id, a.Id, b.Id, seedID}); err != nil {
		t.Fatalf("ReorderRoomGroups failed: %v", err)
	}

	gs, _ := core.ListRoomGroupsOrdered(ctx, KindChannel)
	got := []string{gs[0].Id, gs[1].Id, gs[2].Id, gs[3].Id}
	want := []string{c.Id, a.Id, b.Id, seedID}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReorderRoomGroups_RejectsIncompleteList(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	a, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	_, _ = core.CreateRoomGroup(ctx, "actor", "B", "")

	// Missing the seed group + one of the created groups.
	err := core.ReorderRoomGroups(ctx, "actor", []string{a.Id})
	if !errors.Is(err, ErrRoomGroupOrderMismatch) {
		t.Errorf("err = %v, want ErrRoomGroupOrderMismatch", err)
	}
}

func TestSeedSetIncludesPreExistingRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Rooms created at boot or via the test helpers (e.g. before #454)
	// land in the seed "Lobby" group so the layout invariant ("every channel
	// room belongs to exactly one set") is preserved.
	room, err := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// The boot-time hook already ran in setupTestCore; CreateRoom with
	// groupID="" also lands the room in the seed set if there is one.
	// Re-run the migration hook to verify idempotence + that an
	// orphaned room would get adopted.
	if err := core.ensureChannelRoomsAreInAGroup(ctx); err != nil {
		t.Fatalf("ensureChannelRoomsAreInAGroup failed: %v", err)
	}

	groups, _ := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if len(groups) == 0 {
		t.Fatal("Expected seed group to exist")
	}

	// The room should be in exactly one group, with its proto GroupId stamped.
	count := 0
	var assignedGroupID string
	for _, g := range groups {
		for _, rid := range g.RoomIds {
			if rid == room.Id {
				count++
				assignedGroupID = g.Id
			}
		}
	}
	if count != 1 {
		t.Errorf("Room appears in %d groups, want exactly 1", count)
	}

	refreshed, _ := core.GetRoom(ctx, KindChannel, room.Id)
	if refreshed.GroupId != assignedGroupID {
		t.Errorf("Room.GroupId = %q, want %q (the group it appears in)", refreshed.GroupId, assignedGroupID)
	}
}

func TestReorderRoomGroups_RejectsUnknownID(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	seedGroups, _ := core.ListRoomGroupsOrdered(ctx, KindChannel)
	seedID := seedGroups[0].Id
	a, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	err := core.ReorderRoomGroups(ctx, "actor", []string{seedID, a.Id, "unknown"})
	if !errors.Is(err, ErrRoomGroupOrderMismatch) {
		t.Errorf("err = %v, want ErrRoomGroupOrderMismatch", err)
	}
}
