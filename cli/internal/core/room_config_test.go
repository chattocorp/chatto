package core

import (
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestRoomConfigUpdateAffectsPublicClients(t *testing.T) {
	for name, test := range map[string]struct {
		change *corev1.RoomConfigUpdatedEvent
		want   bool
	}{
		"nil event":    {},
		"missing mask": {change: &corev1.RoomConfigUpdatedEvent{}},
		"private only": {change: &corev1.RoomConfigUpdatedEvent{UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"future_private_setting"}}}},
		"public only":  {change: &corev1.RoomConfigUpdatedEvent{UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}}, want: true},
		"mixed":        {change: &corev1.RoomConfigUpdatedEvent{UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"future_private_setting", roomConfigPathAuthorEditWindow}}}, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := RoomConfigUpdateAffectsPublicClients(test.change); got != test.want {
				t.Fatalf("RoomConfigUpdateAffectsPublicClients() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRoomConfigProjectionAppliesKnownMaskPathsAndIgnoresUnknownPaths(t *testing.T) {
	projection := NewConfigProjection()
	scope := RoomConfigScope{Kind: RoomConfigScopeRoom, ID: "room-1"}
	value := time.Minute
	set := roomConfigUpdatedEvent("", scope, RoomConfig{AuthorEditWindow: &value}, "author_edit_window", "future_setting")
	if err := projection.Apply(set, 1); err != nil {
		t.Fatalf("apply set: %v", err)
	}
	if got := projection.roomConfigLayer(roomConfigScopeKeyFor(scope)).authorEditWindow; got == nil || *got != value {
		t.Fatalf("projected value = %v, want %s", got, value)
	}

	futureValue := 2 * time.Minute
	unknownOnly := roomConfigUpdatedEvent("", scope, RoomConfig{AuthorEditWindow: &futureValue}, "future_setting")
	if err := projection.Apply(unknownOnly, 2); err != nil {
		t.Fatalf("apply unknown path: %v", err)
	}
	if got := projection.roomConfigLayer(roomConfigScopeKeyFor(scope)).authorEditWindow; got == nil || *got != value {
		t.Fatalf("value after unknown path = %v, want %s", got, value)
	}

	clear := roomConfigUpdatedEvent("", scope, RoomConfig{}, "author_edit_window")
	if err := projection.Apply(clear, 3); err != nil {
		t.Fatalf("apply clear: %v", err)
	}
	if got := projection.roomConfigLayer(roomConfigScopeKeyFor(scope)).authorEditWindow; got != nil {
		t.Fatalf("projected value after clear = %s, want absent", *got)
	}
}

func TestRoomConfigResolveSparseHierarchy(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := chatto.CreateUser(ctx, SystemActorID, "config-owner", "Config Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := chatto.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	groupA, err := chatto.CreateRoomGroup(ctx, owner.Id, "Config A", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup A: %v", err)
	}
	groupB, err := chatto.CreateRoomGroup(ctx, owner.Id, "Config B", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup B: %v", err)
	}
	room, err := chatto.CreateRoom(ctx, owner.Id, KindChannel, groupA.Id, "config-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	assertWindow := func(want time.Duration) {
		t.Helper()
		got := chatto.EffectiveRoomConfig(room)
		if got.AuthorEditWindow == nil || *got.AuthorEditWindow != want {
			t.Fatalf("effective window = %s, want %s", got.AuthorEditWindow, want)
		}
	}
	assertWindow(DefaultAuthorEditWindow)

	serverValue := 2 * time.Hour
	if _, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeServer}, RoomConfig{AuthorEditWindow: &serverValue}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}); err != nil {
		t.Fatalf("set server: %v", err)
	}
	assertWindow(serverValue)

	groupAValue := time.Hour
	if _, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeRoomGroup, ID: groupA.Id}, RoomConfig{AuthorEditWindow: &groupAValue}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}); err != nil {
		t.Fatalf("set group A: %v", err)
	}
	assertWindow(groupAValue)

	roomValue := 10 * time.Minute
	if _, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeRoom, ID: room.Id}, RoomConfig{AuthorEditWindow: &roomValue}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}); err != nil {
		t.Fatalf("set room: %v", err)
	}
	assertWindow(roomValue)
	if _, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeRoom, ID: room.Id}, RoomConfig{}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}); err != nil {
		t.Fatalf("clear room: %v", err)
	}
	assertWindow(groupAValue)

	groupBValue := 30 * time.Minute
	if _, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeRoomGroup, ID: groupB.Id}, RoomConfig{AuthorEditWindow: &groupBValue}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}); err != nil {
		t.Fatalf("set group B: %v", err)
	}
	if err := chatto.MoveRoomToGroup(ctx, owner.Id, room.Id, groupB.Id); err != nil {
		t.Fatalf("MoveRoomToGroup: %v", err)
	}
	assertWindow(groupBValue)
}

func TestDeletingRoomConfigScopesCommitsLayerCleanup(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, _ := chatto.CreateUser(ctx, SystemActorID, "config-delete-owner", "Owner", "password123")
	if err := chatto.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("Assign owner: %v", err)
	}
	group, _ := chatto.CreateRoomGroup(ctx, owner.Id, "Config Delete", "")
	room, _ := chatto.CreateRoom(ctx, owner.Id, KindChannel, group.Id, "config-delete-room", "")
	value := time.Minute
	if _, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeRoom, ID: room.Id}, RoomConfig{AuthorEditWindow: &value}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}); err != nil {
		t.Fatalf("set room config: %v", err)
	}
	if err := chatto.DeleteRoom(ctx, owner.Id, KindChannel, room.Id); err != nil {
		t.Fatalf("DeleteRoom: %v", err)
	}
	roomClears, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomConfigAggregate(room.Id).Subject(evtstream.EventRoomConfigUpdated))
	if err != nil {
		t.Fatalf("read room config cleanup: %v", err)
	}
	if len(roomClears) != 2 || roomClears[1].GetRoomConfigUpdated().GetConfig().AuthorEditWindow != nil {
		t.Fatalf("room configuration events = %+v, want set followed by layer cleanup", roomClears)
	}

	emptyGroup, _ := chatto.CreateRoomGroup(ctx, owner.Id, "Empty Config Delete", "")
	if _, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeRoomGroup, ID: emptyGroup.Id}, RoomConfig{AuthorEditWindow: &value}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}); err != nil {
		t.Fatalf("set room-group config: %v", err)
	}
	if err := chatto.DeleteRoomGroup(ctx, owner.Id, emptyGroup.Id); err != nil {
		t.Fatalf("DeleteRoomGroup: %v", err)
	}
	groupClears, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomConfigAggregate(emptyGroup.Id).Subject(evtstream.EventRoomConfigUpdated))
	if err != nil {
		t.Fatalf("read room-group config cleanup: %v", err)
	}
	if len(groupClears) != 2 || groupClears[1].GetRoomConfigUpdated().GetConfig().AuthorEditWindow != nil {
		t.Fatalf("room-group configuration events = %+v, want set followed by layer cleanup", groupClears)
	}
}

func TestRoomConfigUpdateWaitsForConcurrentRoomDeletionProjection(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, _ := chatto.CreateUser(ctx, SystemActorID, "config-delete-race-owner", "Owner", "password123")
	if err := chatto.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("Assign owner: %v", err)
	}
	group, _ := chatto.CreateRoomGroup(ctx, owner.Id, "Config Delete Race", "")
	room, _ := chatto.CreateRoom(ctx, owner.Id, KindChannel, group.Id, "config-delete-race-room", "")
	initial := time.Minute
	if _, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeRoom, ID: room.Id}, RoomConfig{AuthorEditWindow: &initial}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}); err != nil {
		t.Fatalf("set room config: %v", err)
	}

	catalog := chatto.roomModel.directory.Projection().Catalog
	deleteReady := make(chan struct{})
	continueDelete := make(chan struct{})
	chatto.beforeRoomDeleteCommit = func() {
		close(deleteReady)
		<-continueDelete
	}
	deleteErr := make(chan error, 1)
	go func() { deleteErr <- chatto.DeleteRoom(ctx, owner.Id, KindChannel, room.Id) }()
	<-deleteReady
	catalog.Lock()
	locked := true
	defer func() {
		if locked {
			catalog.Unlock()
		}
	}()
	close(continueDelete)
	deletedSubject := evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomDeleted)
	deadline := time.Now().Add(5 * time.Second)
	for {
		seq, err := chatto.EventPublisher.LastSubjectSeq(ctx, deletedSubject)
		if err != nil {
			catalog.Unlock()
			locked = false
			t.Fatalf("read deletion tail: %v", err)
		}
		if seq > 0 {
			break
		}
		if time.Now().After(deadline) {
			catalog.Unlock()
			locked = false
			t.Fatal("room deletion did not commit while projection was paused")
		}
		time.Sleep(time.Millisecond)
	}

	updated := 2 * time.Minute
	updateErr := make(chan error, 1)
	go func() {
		_, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeRoom, ID: room.Id}, RoomConfig{AuthorEditWindow: &updated}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}})
		updateErr <- err
	}()
	select {
	case err := <-updateErr:
		catalog.Unlock()
		locked = false
		t.Fatalf("config update returned before deletion projection caught up: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	catalog.Unlock()
	locked = false

	if err := <-deleteErr; err != nil {
		t.Fatalf("DeleteRoom: %v", err)
	}
	if err := <-updateErr; !errors.Is(err, ErrNotFound) {
		t.Fatalf("racing config update error = %v, want not found", err)
	}
	setEvents, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomConfigAggregate(room.Id).Subject(evtstream.EventRoomConfigUpdated))
	if err != nil {
		t.Fatalf("read room config events: %v", err)
	}
	setCount := 0
	for _, event := range setEvents {
		if event.GetRoomConfigUpdated().GetConfig().AuthorEditWindow != nil {
			setCount++
		}
	}
	if setCount != 1 {
		t.Fatalf("room configuration set events = %d, want only the pre-deletion event", setCount)
	}
}

func TestRoomConfigValidationAuthorizationAndConcurrentWrites(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, _ := chatto.CreateUser(ctx, SystemActorID, "config-concurrent-owner", "Owner", "password123")
	regular, _ := chatto.CreateUser(ctx, SystemActorID, "config-regular", "Regular", "password123")
	if err := chatto.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	value := time.Minute
	if _, err := chatto.UpdateRoomConfig(ctx, regular.Id, RoomConfigScope{Kind: RoomConfigScopeServer}, RoomConfig{AuthorEditWindow: &value}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("unauthorized update error = %v, want permission denied", err)
	}
	invalid := MaxAuthorEditWindow + time.Second
	if _, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeServer}, RoomConfig{AuthorEditWindow: &invalid}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("invalid update error = %v, want invalid argument", err)
	}
	if _, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeServer}, RoomConfig{}, &fieldmaskpb.FieldMask{Paths: []string{"future_setting"}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("unknown update-mask path error = %v, want invalid argument", err)
	}

	values := []time.Duration{2 * time.Minute, 4 * time.Minute}
	errs := make(chan error, len(values))
	var wg sync.WaitGroup
	for _, candidate := range values {
		candidate := candidate
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeServer}, RoomConfig{AuthorEditWindow: &candidate}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent update: %v", err)
		}
	}
	effective := chatto.EffectiveServerRoomConfig()
	if effective.AuthorEditWindow == nil || (*effective.AuthorEditWindow != values[0] && *effective.AuthorEditWindow != values[1]) {
		t.Fatalf("effective window after concurrent updates = %s", effective.AuthorEditWindow)
	}
}

func TestRoomConfigUpdateReturnsCommittedStateAfterPermissionRevocation(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, _ := chatto.CreateUser(ctx, SystemActorID, "config-response-owner", "Owner", "password123")
	manager, _ := chatto.CreateUser(ctx, SystemActorID, "config-response-manager", "Manager", "password123")
	if err := chatto.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("Assign owner: %v", err)
	}
	room, _ := chatto.CreateRoom(ctx, owner.Id, KindChannel, "", "config-response-room", "")
	if _, err := chatto.JoinRoom(ctx, manager.Id, KindChannel, manager.Id, room.Id); err != nil {
		t.Fatalf("Join manager: %v", err)
	}
	if err := chatto.GrantUserRoomPermission(ctx, owner.Id, room.Id, manager.Id, PermRoomManage); err != nil {
		t.Fatalf("Grant room.manage: %v", err)
	}
	chatto.afterRoomConfigCommit = func() {
		if err := chatto.DenyUserRoomPermission(ctx, owner.Id, room.Id, manager.Id, PermRoomManage); err != nil {
			t.Errorf("revoke room.manage after commit: %v", err)
		}
	}
	value := 90 * time.Second
	state, err := chatto.UpdateRoomConfig(ctx, manager.Id, RoomConfigScope{Kind: RoomConfigScopeRoom, ID: room.Id}, RoomConfig{AuthorEditWindow: &value}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}})
	if err != nil {
		t.Fatalf("UpdateRoomConfig returned an error after commit: %v", err)
	}
	if state.Effective.AuthorEditWindow == nil || *state.Effective.AuthorEditWindow != value {
		t.Fatalf("committed effective value = %s, want %s", state.Effective.AuthorEditWindow, value)
	}
	if _, err := chatto.GetRoomConfig(ctx, manager.Id, RoomConfigScope{Kind: RoomConfigScopeRoom, ID: room.Id}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("GetRoomConfig after revocation error = %v, want permission denied", err)
	}
}

func TestAuthorEditWindowUsesCurrentConfigAndModeratorBypassesIt(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, _ := chatto.CreateUser(ctx, SystemActorID, "config-edit-owner", "Owner", "password123")
	author, _ := chatto.CreateUser(ctx, SystemActorID, "config-edit-author", "Author", "password123")
	moderator, _ := chatto.CreateUser(ctx, SystemActorID, "config-edit-moderator", "Moderator", "password123")
	if err := chatto.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("Assign owner: %v", err)
	}
	group, _ := chatto.CreateRoomGroup(ctx, owner.Id, "Edit Config", "")
	room, _ := chatto.CreateRoom(ctx, owner.Id, KindChannel, group.Id, "edit-config-room", "")
	if _, err := chatto.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("Join author: %v", err)
	}
	if _, err := chatto.JoinRoom(ctx, moderator.Id, KindChannel, moderator.Id, room.Id); err != nil {
		t.Fatalf("Join moderator: %v", err)
	}
	if err := chatto.GrantUserRoomPermission(ctx, SystemActorID, room.Id, moderator.Id, PermMessageManage); err != nil {
		t.Fatalf("grant moderator: %v", err)
	}
	posted, err := chatto.PostMessage(ctx, KindChannel, room.Id, author.Id, "original", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	zero := time.Duration(0)
	if _, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeRoom, ID: room.Id}, RoomConfig{AuthorEditWindow: &zero}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}); err != nil {
		t.Fatalf("set zero window: %v", err)
	}
	if err := chatto.EditMessage(ctx, author.Id, KindChannel, room.Id, posted.Id, "author edit"); !errors.Is(err, ErrEditWindowExpired) {
		t.Fatalf("author edit error = %v, want window expired", err)
	}
	if err := chatto.editEmbeddedBody(ctx, author.Id, KindChannel, room.Id, posted.Id, func(*corev1.MessageBody) error { return nil }); !errors.Is(err, ErrEditWindowExpired) {
		t.Fatalf("partial author edit error = %v, want window expired", err)
	}
	if err := chatto.EditMessage(ctx, moderator.Id, KindChannel, room.Id, posted.Id, "moderator edit"); err != nil {
		t.Fatalf("moderator edit: %v", err)
	}
	moderatorPost, err := chatto.PostMessage(ctx, KindChannel, room.Id, moderator.Id, "moderator original", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage moderator: %v", err)
	}
	if err := chatto.EditMessage(ctx, moderator.Id, KindChannel, room.Id, moderatorPost.Id, "moderator self edit"); err != nil {
		t.Fatalf("moderator self edit with zero window: %v", err)
	}
	open := time.Hour
	if _, err := chatto.UpdateRoomConfig(ctx, owner.Id, RoomConfigScope{Kind: RoomConfigScopeRoom, ID: room.Id}, RoomConfig{AuthorEditWindow: &open}, &fieldmaskpb.FieldMask{Paths: []string{roomConfigPathAuthorEditWindow}}); err != nil {
		t.Fatalf("reopen window: %v", err)
	}
	if err := chatto.EditMessage(ctx, author.Id, KindChannel, room.Id, posted.Id, "author edit reopened"); err != nil {
		t.Fatalf("author edit after raising window: %v", err)
	}
}
