package core

import (
	"errors"
	"sync"
	"testing"
	"time"

	"hmans.de/chatto/internal/evtstream"
)

func TestRuntimePoliciesResolveSparseHierarchy(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := chatto.CreateUser(ctx, SystemActorID, "policy-owner", "Policy Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := chatto.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	groupA, err := chatto.CreateRoomGroup(ctx, owner.Id, "Policy A", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup A: %v", err)
	}
	groupB, err := chatto.CreateRoomGroup(ctx, owner.Id, "Policy B", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup B: %v", err)
	}
	room, err := chatto.CreateRoom(ctx, owner.Id, KindChannel, groupA.Id, "policy-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	assertWindow := func(want int32, wantKind PolicyScopeKind, wantID string, product bool) {
		t.Helper()
		got, sources := chatto.EffectiveRoomPolicies(room)
		if got.AuthorEditWindowSeconds != want {
			t.Fatalf("effective window = %d, want %d", got.AuthorEditWindowSeconds, want)
		}
		source := sources.AuthorEditWindow
		if source.Kind != wantKind || source.ID != wantID || source.ProductDefault != product {
			t.Fatalf("source = %+v, want kind=%v id=%q product=%v", source, wantKind, wantID, product)
		}
	}
	assertWindow(int32(DefaultAuthorEditWindow.Seconds()), 0, "", true)

	serverValue := int32(7200)
	if _, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeServer}, PolicyOverrides{AuthorEditWindowSeconds: &serverValue}, PolicyUpdateMask{AuthorEditWindow: true}); err != nil {
		t.Fatalf("set server: %v", err)
	}
	assertWindow(serverValue, PolicyScopeServer, "", false)

	groupAValue := int32(3600)
	if _, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeRoomGroup, ID: groupA.Id}, PolicyOverrides{AuthorEditWindowSeconds: &groupAValue}, PolicyUpdateMask{AuthorEditWindow: true}); err != nil {
		t.Fatalf("set group A: %v", err)
	}
	assertWindow(groupAValue, PolicyScopeRoomGroup, groupA.Id, false)

	roomValue := int32(600)
	if _, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeRoom, ID: room.Id}, PolicyOverrides{AuthorEditWindowSeconds: &roomValue}, PolicyUpdateMask{AuthorEditWindow: true}); err != nil {
		t.Fatalf("set room: %v", err)
	}
	assertWindow(roomValue, PolicyScopeRoom, room.Id, false)
	if _, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeRoom, ID: room.Id}, PolicyOverrides{}, PolicyUpdateMask{AuthorEditWindow: true}); err != nil {
		t.Fatalf("clear room: %v", err)
	}
	assertWindow(groupAValue, PolicyScopeRoomGroup, groupA.Id, false)

	groupBValue := int32(1800)
	if _, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeRoomGroup, ID: groupB.Id}, PolicyOverrides{AuthorEditWindowSeconds: &groupBValue}, PolicyUpdateMask{AuthorEditWindow: true}); err != nil {
		t.Fatalf("set group B: %v", err)
	}
	if err := chatto.MoveRoomToGroup(ctx, owner.Id, room.Id, groupB.Id); err != nil {
		t.Fatalf("MoveRoomToGroup: %v", err)
	}
	assertWindow(groupBValue, PolicyScopeRoomGroup, groupB.Id, false)
}

func TestDeletingPolicyScopesCommitsPolicyCleanup(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, _ := chatto.CreateUser(ctx, SystemActorID, "policy-delete-owner", "Owner", "password123")
	if err := chatto.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("Assign owner: %v", err)
	}
	group, _ := chatto.CreateRoomGroup(ctx, owner.Id, "Policy Delete", "")
	room, _ := chatto.CreateRoom(ctx, owner.Id, KindChannel, group.Id, "policy-delete-room", "")
	value := int32(60)
	if _, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeRoom, ID: room.Id}, PolicyOverrides{AuthorEditWindowSeconds: &value}, PolicyUpdateMask{AuthorEditWindow: true}); err != nil {
		t.Fatalf("set room policy: %v", err)
	}
	if err := chatto.DeleteRoom(ctx, owner.Id, KindChannel, room.Id); err != nil {
		t.Fatalf("DeleteRoom: %v", err)
	}
	roomClears, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.ConfigSubjectAggregate(room.Id).Subject(evtstream.EventAuthorEditWindowCleared))
	if err != nil {
		t.Fatalf("read room policy cleanup: %v", err)
	}
	if len(roomClears) != 1 {
		t.Fatalf("room policy cleanup events = %d, want 1", len(roomClears))
	}

	emptyGroup, _ := chatto.CreateRoomGroup(ctx, owner.Id, "Empty Policy Delete", "")
	if _, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeRoomGroup, ID: emptyGroup.Id}, PolicyOverrides{AuthorEditWindowSeconds: &value}, PolicyUpdateMask{AuthorEditWindow: true}); err != nil {
		t.Fatalf("set room-group policy: %v", err)
	}
	if err := chatto.DeleteRoomGroup(ctx, owner.Id, emptyGroup.Id); err != nil {
		t.Fatalf("DeleteRoomGroup: %v", err)
	}
	groupClears, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.ConfigSubjectAggregate(emptyGroup.Id).Subject(evtstream.EventAuthorEditWindowCleared))
	if err != nil {
		t.Fatalf("read room-group policy cleanup: %v", err)
	}
	if len(groupClears) != 1 {
		t.Fatalf("room-group policy cleanup events = %d, want 1", len(groupClears))
	}
}

func TestPolicyUpdateWaitsForConcurrentRoomDeletionProjection(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, _ := chatto.CreateUser(ctx, SystemActorID, "policy-delete-race-owner", "Owner", "password123")
	if err := chatto.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("Assign owner: %v", err)
	}
	group, _ := chatto.CreateRoomGroup(ctx, owner.Id, "Policy Delete Race", "")
	room, _ := chatto.CreateRoom(ctx, owner.Id, KindChannel, group.Id, "policy-delete-race-room", "")
	initial := int32(60)
	if _, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeRoom, ID: room.Id}, PolicyOverrides{AuthorEditWindowSeconds: &initial}, PolicyUpdateMask{AuthorEditWindow: true}); err != nil {
		t.Fatalf("set room policy: %v", err)
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

	updated := int32(120)
	updateErr := make(chan error, 1)
	go func() {
		_, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeRoom, ID: room.Id}, PolicyOverrides{AuthorEditWindowSeconds: &updated}, PolicyUpdateMask{AuthorEditWindow: true})
		updateErr <- err
	}()
	select {
	case err := <-updateErr:
		catalog.Unlock()
		locked = false
		t.Fatalf("policy update returned before deletion projection caught up: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	catalog.Unlock()
	locked = false

	if err := <-deleteErr; err != nil {
		t.Fatalf("DeleteRoom: %v", err)
	}
	if err := <-updateErr; !errors.Is(err, ErrNotFound) {
		t.Fatalf("racing policy update error = %v, want not found", err)
	}
	setEvents, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.ConfigSubjectAggregate(room.Id).Subject(evtstream.EventAuthorEditWindowSet))
	if err != nil {
		t.Fatalf("read room policy events: %v", err)
	}
	if len(setEvents) != 1 {
		t.Fatalf("room policy set events = %d, want only the pre-deletion event", len(setEvents))
	}
}

func TestRuntimePolicyValidationAuthorizationAndConcurrentWrites(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, _ := chatto.CreateUser(ctx, SystemActorID, "policy-concurrent-owner", "Owner", "password123")
	regular, _ := chatto.CreateUser(ctx, SystemActorID, "policy-regular", "Regular", "password123")
	if err := chatto.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	value := int32(60)
	if _, err := chatto.UpdatePolicyConfiguration(ctx, regular.Id, PolicyScope{Kind: PolicyScopeServer}, PolicyOverrides{AuthorEditWindowSeconds: &value}, PolicyUpdateMask{AuthorEditWindow: true}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("unauthorized update error = %v, want permission denied", err)
	}
	invalid := int32(MaxAuthorEditWindow.Seconds()) + 1
	if _, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeServer}, PolicyOverrides{AuthorEditWindowSeconds: &invalid}, PolicyUpdateMask{AuthorEditWindow: true}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("invalid update error = %v, want invalid argument", err)
	}

	values := []int32{120, 240}
	errs := make(chan error, len(values))
	var wg sync.WaitGroup
	for _, candidate := range values {
		candidate := candidate
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeServer}, PolicyOverrides{AuthorEditWindowSeconds: &candidate}, PolicyUpdateMask{AuthorEditWindow: true})
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
	effective, _ := chatto.EffectiveServerPolicies()
	if effective.AuthorEditWindowSeconds != values[0] && effective.AuthorEditWindowSeconds != values[1] {
		t.Fatalf("effective window after concurrent updates = %d", effective.AuthorEditWindowSeconds)
	}
}

func TestAuthorEditWindowUsesCurrentPolicyAndModeratorBypassesIt(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, _ := chatto.CreateUser(ctx, SystemActorID, "policy-edit-owner", "Owner", "password123")
	author, _ := chatto.CreateUser(ctx, SystemActorID, "policy-edit-author", "Author", "password123")
	moderator, _ := chatto.CreateUser(ctx, SystemActorID, "policy-edit-moderator", "Moderator", "password123")
	if err := chatto.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("Assign owner: %v", err)
	}
	group, _ := chatto.CreateRoomGroup(ctx, owner.Id, "Edit Policy", "")
	room, _ := chatto.CreateRoom(ctx, owner.Id, KindChannel, group.Id, "edit-policy-room", "")
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
	zero := int32(0)
	if _, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeRoom, ID: room.Id}, PolicyOverrides{AuthorEditWindowSeconds: &zero}, PolicyUpdateMask{AuthorEditWindow: true}); err != nil {
		t.Fatalf("set zero window: %v", err)
	}
	if err := chatto.EditMessage(ctx, author.Id, KindChannel, room.Id, posted.Id, "author edit"); !errors.Is(err, ErrEditWindowExpired) {
		t.Fatalf("author edit error = %v, want window expired", err)
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
	open := int32(3600)
	if _, err := chatto.UpdatePolicyConfiguration(ctx, owner.Id, PolicyScope{Kind: PolicyScopeRoom, ID: room.Id}, PolicyOverrides{AuthorEditWindowSeconds: &open}, PolicyUpdateMask{AuthorEditWindow: true}); err != nil {
		t.Fatalf("reopen window: %v", err)
	}
	if err := chatto.EditMessage(ctx, author.Id, KindChannel, room.Id, posted.Id, "author edit reopened"); err != nil {
		t.Fatalf("author edit after raising window: %v", err)
	}
}
