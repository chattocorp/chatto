package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNotificationDeliveryModesPatchSetClearAndValidation(t *testing.T) {
	current := &corev1.NotificationDeliveryModes{
		Reactions:     corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION.Enum(),
		FollowedRooms: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION.Enum(),
	}
	patch := &corev1.NotificationDeliveryModes{DirectMessages: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum()}
	got, err := applyNotificationDeliveryModesPatch(current, patch, &fieldmaskpb.FieldMask{Paths: []string{"direct_messages", "reactions"}})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if got.GetDirectMessages() != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF || got.Reactions != nil || got.GetFollowedRooms() != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION {
		t.Fatalf("patched modes = %+v, want direct messages Off, reactions cleared, followed rooms preserved", got)
	}
	if current.DirectMessages != nil || current.Reactions == nil {
		t.Fatalf("patch mutated source modes: %+v", current)
	}

	invalidMode := corev1.NotificationDeliveryMode(99)
	for _, test := range []struct {
		name  string
		patch *corev1.NotificationDeliveryModes
		mask  *fieldmaskpb.FieldMask
	}{
		{name: "missing mask", mask: nil},
		{name: "empty mask", mask: &fieldmaskpb.FieldMask{}},
		{name: "unknown field", mask: &fieldmaskpb.FieldMask{Paths: []string{"unknown"}}},
		{name: "nested field", mask: &fieldmaskpb.FieldMask{Paths: []string{"reactions.value"}}},
		{name: "unspecified value", patch: &corev1.NotificationDeliveryModes{Reactions: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED.Enum()}, mask: &fieldmaskpb.FieldMask{Paths: []string{"reactions"}}},
		{name: "unknown enum value", patch: &corev1.NotificationDeliveryModes{Reactions: &invalidMode}, mask: &fieldmaskpb.FieldMask{Paths: []string{"reactions"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := applyNotificationDeliveryModesPatch(current, test.patch, test.mask); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("apply patch error = %v, want ErrInvalidArgument", err)
			}
		})
	}
}

func TestEffectiveNotificationDeliveryModesPopulatesEveryBuiltInField(t *testing.T) {
	got := effectiveNotificationDeliveryModes(nil, nil)
	if got.DirectMessages == nil || got.DirectMentions == nil || got.Replies == nil || got.RoleMentions == nil || got.HereMentions == nil || got.AllMentions == nil || got.FollowedThreads == nil || got.FollowedRooms == nil || got.Reactions == nil {
		t.Fatalf("effective defaults are incomplete: %+v", got)
	}
	if got.GetDirectMessages() != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION || got.GetFollowedThreads() != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION || got.GetFollowedRooms() != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF || got.GetReactions() != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION {
		t.Fatalf("effective defaults = %+v", got)
	}
}

func TestConcurrentNotificationPolicyPatchesDoNotLoseUnrelatedFields(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "policy-occ-user", "Policy OCC User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	update := func(patch *corev1.NotificationDeliveryModes, path string) {
		defer ready.Done()
		<-start
		_, err := chattoCore.NotificationPolicy().UpdateNotificationPolicy(ctx, user.Id, "", patch, &fieldmaskpb.FieldMask{Paths: []string{path}})
		errs <- err
	}
	go update(&corev1.NotificationDeliveryModes{DirectMessages: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum()}, "direct_messages")
	go update(&corev1.NotificationDeliveryModes{Reactions: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION.Enum()}, "reactions")
	close(start)
	ready.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent policy update: %v", err)
		}
	}
	policy, err := chattoCore.NotificationPolicy().GetNotificationPolicy(ctx, user.Id, "")
	if err != nil {
		t.Fatalf("GetNotificationPolicy: %v", err)
	}
	if policy.Overrides.GetDirectMessages() != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF || policy.Overrides.GetReactions() != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION {
		t.Fatalf("concurrent policy overrides = %+v, want both fields", policy.Overrides)
	}
}

func TestGetNotificationPolicyWaitsForCurrentConfigProjection(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "policy-read-fence-user", "Policy Read Fence User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := chattoCore.NotificationPolicy().SetServerNotificationMode(ctx, user.Id,
		notificationTestSignalReaction,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
	); err != nil {
		t.Fatalf("SetServerNotificationMode: %v", err)
	}

	delayedConfig := evtstream.NewProjectionHandle(
		chattoCore.js,
		chattoCore.storage.serverEvtStream,
		NewConfigProjection(),
		testCoreLogger(),
	)
	chattoCore.configModel = NewConfigModel(chattoCore.EventPublisher, delayedConfig)
	type policyResult struct {
		policy *NotificationPolicy
		err    error
	}
	result := make(chan policyResult, 1)
	go func() {
		policy, err := chattoCore.NotificationPolicy().GetNotificationPolicy(ctx, user.Id, "")
		result <- policyResult{policy: policy, err: err}
	}()
	select {
	case early := <-result:
		t.Fatalf("GetNotificationPolicy returned before delayed config projection started: (%+v, %v)", early.policy, early.err)
	case <-time.After(50 * time.Millisecond):
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- delayedConfig.Projector().Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("delayed config projector did not stop")
		}
	})
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("GetNotificationPolicy after catch-up: %v", got.err)
		}
		assertNotificationPolicyIntensity(t, got.policy, notificationTestSignalReaction,
			corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
			corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
			corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
		)
	case <-time.After(5 * time.Second):
		t.Fatal("GetNotificationPolicy did not finish after config projection caught up")
	}
}

func TestSetRoomNotificationPolicyConflictsWithConcurrentMembershipLoss(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "policy-write-fence-user", "Policy Write Fence User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, user.Id, KindChannel, "", "policy-write-fence-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	// Seed a config fact so replacing the config projector gives us a
	// deterministic pause after room access has been checked but before append.
	if _, err := chattoCore.NotificationPolicy().SetServerNotificationMode(ctx, user.Id,
		notificationTestSignalReaction,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
	); err != nil {
		t.Fatalf("SetServerNotificationMode: %v", err)
	}
	delayedConfig := evtstream.NewProjectionHandle(
		chattoCore.js,
		chattoCore.storage.serverEvtStream,
		NewConfigProjection(),
		testCoreLogger(),
	)
	chattoCore.configModel = NewConfigModel(chattoCore.EventPublisher, delayedConfig)

	result := make(chan error, 1)
	go func() {
		_, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(ctx, user.Id, room.Id,
			notificationTestSignalReaction,
			corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
		)
		result <- err
	}()
	select {
	case early := <-result:
		t.Fatalf("SetRoomNotificationMode returned before delayed config projection started: %v", early)
	case <-time.After(50 * time.Millisecond):
	}
	if err := chattoCore.LeaveRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- delayedConfig.Projector().Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("delayed config projector did not stop")
		}
	})
	select {
	case err := <-result:
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("SetRoomNotificationMode after concurrent leave error = %v, want ErrPermissionDenied", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SetRoomNotificationMode did not finish after config projection caught up")
	}
	if got := chattoCore.configModel.notificationRoomMode(user.Id, room.Id, notificationTestSignalReaction); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
		t.Fatalf("room preference after rejected write = %v, want unspecified", got)
	}
}

func TestSetRoomNotificationPolicyConflictsWithConcurrentRoomDeletion(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "policy-delete-fence-user", "Policy Delete Fence User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, user.Id, KindChannel, "", "policy-delete-fence-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if _, err := chattoCore.NotificationPolicy().SetServerNotificationMode(ctx, user.Id,
		notificationTestSignalReaction,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
	); err != nil {
		t.Fatalf("SetServerNotificationMode: %v", err)
	}
	delayedConfig := evtstream.NewProjectionHandle(
		chattoCore.js,
		chattoCore.storage.serverEvtStream,
		NewConfigProjection(),
		testCoreLogger(),
	)
	chattoCore.configModel = NewConfigModel(chattoCore.EventPublisher, delayedConfig)

	result := make(chan error, 1)
	go func() {
		_, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(ctx, user.Id, room.Id,
			notificationTestSignalReaction,
			corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
		)
		result <- err
	}()
	select {
	case early := <-result:
		t.Fatalf("SetRoomNotificationMode returned before delayed config projection started: %v", early)
	case <-time.After(50 * time.Millisecond):
	}
	if err := chattoCore.DeleteRoom(ctx, user.Id, KindChannel, room.Id); err != nil {
		t.Fatalf("DeleteRoom: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- delayedConfig.Projector().Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("delayed config projector did not stop")
		}
	})
	select {
	case err := <-result:
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("SetRoomNotificationMode after concurrent deletion error = %v, want ErrNotFound", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SetRoomNotificationMode did not finish after config projection caught up")
	}
	if got := chattoCore.configModel.notificationRoomMode(user.Id, room.Id, notificationTestSignalReaction); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
		t.Fatalf("room preference after rejected write = %v, want unspecified", got)
	}
}

func TestSetRoomGroupNotificationPolicyConflictsWithConcurrentGroupDeletion(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "group-policy-delete-fence-user", "Group Policy Delete Fence User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	group, err := chattoCore.CreateRoomGroup(ctx, SystemActorID, "Group Policy Delete Fence", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	if _, err := chattoCore.NotificationPolicy().SetServerNotificationMode(ctx, user.Id,
		notificationTestSignalReaction,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
	); err != nil {
		t.Fatalf("SetServerNotificationMode: %v", err)
	}
	delayedConfig := evtstream.NewProjectionHandle(
		chattoCore.js,
		chattoCore.storage.serverEvtStream,
		NewConfigProjection(),
		testCoreLogger(),
	)
	chattoCore.configModel = NewConfigModel(chattoCore.EventPublisher, delayedConfig)

	result := make(chan error, 1)
	go func() {
		_, err := chattoCore.NotificationPolicy().UpdateScopedNotificationPolicy(ctx, user.Id,
			NotificationPolicyScope{Kind: NotificationPolicyScopeRoomGroup, ID: group.Id},
			&corev1.NotificationDeliveryModes{Reactions: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum()},
			&fieldmaskpb.FieldMask{Paths: []string{"reactions"}},
		)
		result <- err
	}()
	select {
	case early := <-result:
		t.Fatalf("group policy update returned before delayed config projection started: %v", early)
	case <-time.After(50 * time.Millisecond):
	}
	if err := chattoCore.DeleteRoomGroup(ctx, SystemActorID, group.Id); err != nil {
		t.Fatalf("DeleteRoomGroup: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- delayedConfig.Projector().Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("delayed config projector did not stop")
		}
	})
	select {
	case err := <-result:
		if !errors.Is(err, ErrRoomGroupNotFound) {
			t.Fatalf("group policy update after concurrent deletion error = %v, want ErrRoomGroupNotFound", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("group policy update did not finish after config projection caught up")
	}
	if got := chattoCore.configModel.notificationRoomGroupModes(user.Id, group.Id).GetReactions(); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
		t.Fatalf("group preference after rejected write = %v, want unspecified", got)
	}
}

func TestNotificationPolicyInheritanceByCause(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "policy-user", "Policy User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, user.Id, KindChannel, "", "policy-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	preferences := chattoCore.NotificationPolicy()

	policy, err := preferences.GetNotificationPolicy(ctx, user.Id, room.Id)
	if err != nil {
		t.Fatalf("GetNotificationPolicy: %v", err)
	}
	if _, err := preferences.SetServerNotificationMode(ctx, user.Id,
		notificationTestSignalKind("unsupported"),
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
	); err == nil {
		t.Fatal("SetServerNotificationMode accepted reserved notification policy kind 10")
	}
	assertNotificationPolicyIntensity(t, policy, notificationTestSignalFollowedRoom,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
	)
	assertNotificationPolicyIntensity(t, policy, notificationTestSignalDirectMention,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
	)

	policy, err = preferences.SetServerNotificationMode(ctx, user.Id,
		notificationTestSignalFollowedRoom,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
	)
	if err != nil {
		t.Fatalf("SetServerNotificationMode: %v", err)
	}
	assertNotificationPolicyIntensity(t, policy, notificationTestSignalFollowedRoom,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
	)

	policy, err = preferences.SetRoomNotificationMode(ctx, user.Id, room.Id,
		notificationTestSignalFollowedRoom,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
	)
	if err != nil {
		t.Fatalf("SetRoomNotificationMode: %v", err)
	}
	assertNotificationPolicyIntensity(t, policy, notificationTestSignalFollowedRoom,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
	)

	policy, err = preferences.SetRoomNotificationMode(ctx, user.Id, room.Id,
		notificationTestSignalFollowedRoom,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
	)
	if err != nil {
		t.Fatalf("clear room notification mode: %v", err)
	}
	assertNotificationPolicyIntensity(t, policy, notificationTestSignalFollowedRoom,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
	)
}

func TestNotificationPolicyInheritsThroughRoomGroupAndFollowsRoomMoves(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "group-policy-user", "Group Policy User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	groupA, err := chattoCore.CreateRoomGroup(ctx, SystemActorID, "Group A", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup A: %v", err)
	}
	groupB, err := chattoCore.CreateRoomGroup(ctx, SystemActorID, "Group B", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup B: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, groupA.Id, "group-policy-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	model := chattoCore.NotificationPolicy()
	path := &fieldmaskpb.FieldMask{Paths: []string{"reactions"}}
	update := func(scope NotificationPolicyScope, mode corev1.NotificationDeliveryMode) *NotificationPolicy {
		t.Helper()
		patch := &corev1.NotificationDeliveryModes{}
		if mode != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
			patch.Reactions = mode.Enum()
		}
		policy, updateErr := model.UpdateScopedNotificationPolicy(ctx, user.Id, scope, patch, path)
		if updateErr != nil {
			t.Fatalf("UpdateScopedNotificationPolicy(%+v, %v): %v", scope, mode, updateErr)
		}
		return policy
	}
	serverScope := NotificationPolicyScope{Kind: NotificationPolicyScopeServer}
	groupAScope := NotificationPolicyScope{Kind: NotificationPolicyScopeRoomGroup, ID: groupA.Id}
	groupBScope := NotificationPolicyScope{Kind: NotificationPolicyScopeRoomGroup, ID: groupB.Id}
	roomScope := NotificationPolicyScope{Kind: NotificationPolicyScopeRoom, ID: room.Id}

	update(serverScope, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF)
	update(groupAScope, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION)
	update(groupBScope, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION)
	policy := update(roomScope, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF)
	if got := policy.Effective.GetReactions(); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF {
		t.Fatalf("room override effective mode = %v, want OFF", got)
	}
	policy = update(roomScope, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED)
	if got := policy.Effective.GetReactions(); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION {
		t.Fatalf("group A inherited mode = %v, want PUSH_NOTIFICATION", got)
	}

	if err := chattoCore.MoveRoomToGroup(ctx, SystemActorID, room.Id, groupB.Id); err != nil {
		t.Fatalf("MoveRoomToGroup: %v", err)
	}
	policy, err = model.GetScopedNotificationPolicy(ctx, user.Id, roomScope)
	if err != nil {
		t.Fatalf("GetScopedNotificationPolicy after move: %v", err)
	}
	if got := policy.Effective.GetReactions(); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION {
		t.Fatalf("group B inherited mode after move = %v, want IN_APP_NOTIFICATION", got)
	}

	update(groupBScope, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED)
	policy, err = model.GetScopedNotificationPolicy(ctx, user.Id, roomScope)
	if err != nil {
		t.Fatalf("GetScopedNotificationPolicy after group clear: %v", err)
	}
	if got := policy.Effective.GetReactions(); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF {
		t.Fatalf("server inherited mode = %v, want OFF", got)
	}
	update(serverScope, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED)
	policy, err = model.GetScopedNotificationPolicy(ctx, user.Id, roomScope)
	if err != nil {
		t.Fatalf("GetScopedNotificationPolicy after server clear: %v", err)
	}
	if got := policy.Effective.GetReactions(); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION {
		t.Fatalf("product-default mode = %v, want IN_APP_NOTIFICATION", got)
	}
}

func TestDeletedRoomGroupNotificationPolicyBecomesInert(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "deleted-group-policy-user", "Deleted Group Policy User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	group, err := chattoCore.CreateRoomGroup(ctx, SystemActorID, "Temporary", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	scope := NotificationPolicyScope{Kind: NotificationPolicyScopeRoomGroup, ID: group.Id}
	if _, err := chattoCore.NotificationPolicy().UpdateScopedNotificationPolicy(ctx, user.Id, scope,
		&corev1.NotificationDeliveryModes{Reactions: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION.Enum()},
		&fieldmaskpb.FieldMask{Paths: []string{"reactions"}},
	); err != nil {
		t.Fatalf("UpdateScopedNotificationPolicy: %v", err)
	}
	if err := chattoCore.DeleteRoomGroup(ctx, SystemActorID, group.Id); err != nil {
		t.Fatalf("DeleteRoomGroup: %v", err)
	}
	if _, err := chattoCore.NotificationPolicy().GetScopedNotificationPolicy(ctx, user.Id, scope); !errors.Is(err, ErrRoomGroupNotFound) {
		t.Fatalf("GetScopedNotificationPolicy deleted group error = %v, want ErrRoomGroupNotFound", err)
	}
	if got := chattoCore.configModel.notificationRoomGroupModes(user.Id, group.Id).GetReactions(); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION {
		t.Fatalf("retained inert group preference = %v, want PUSH_NOTIFICATION", got)
	}
}

func TestDirectMessageNotificationPolicySkipsRoomGroupTier(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "dm-policy-user", "DM Policy User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	peer, err := chattoCore.CreateUser(ctx, SystemActorID, "dm-policy-peer", "DM Policy Peer", "password")
	if err != nil {
		t.Fatalf("CreateUser peer: %v", err)
	}
	dm, _, err := chattoCore.FindOrCreateDM(ctx, user.Id, []string{peer.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	group, err := chattoCore.CreateRoomGroup(ctx, SystemActorID, "Unrelated Group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	model := chattoCore.NotificationPolicy()
	if _, err := model.UpdateScopedNotificationPolicy(ctx, user.Id,
		NotificationPolicyScope{Kind: NotificationPolicyScopeServer},
		&corev1.NotificationDeliveryModes{DirectMessages: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum()},
		&fieldmaskpb.FieldMask{Paths: []string{"direct_messages"}},
	); err != nil {
		t.Fatalf("update server policy: %v", err)
	}
	if _, err := model.UpdateScopedNotificationPolicy(ctx, user.Id,
		NotificationPolicyScope{Kind: NotificationPolicyScopeRoomGroup, ID: group.Id},
		&corev1.NotificationDeliveryModes{DirectMessages: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION.Enum()},
		&fieldmaskpb.FieldMask{Paths: []string{"direct_messages"}},
	); err != nil {
		t.Fatalf("update group policy: %v", err)
	}
	policy, err := model.GetScopedNotificationPolicy(ctx, user.Id, NotificationPolicyScope{Kind: NotificationPolicyScopeRoom, ID: dm.Id})
	if err != nil {
		t.Fatalf("GetScopedNotificationPolicy DM: %v", err)
	}
	if got := policy.Effective.GetDirectMessages(); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF {
		t.Fatalf("DM effective direct-message mode = %v, want server OFF", got)
	}
}

func assertNotificationPolicyIntensity(
	t *testing.T,
	policy *NotificationPolicy,
	category notificationTestSignalKind,
	server, room, effective corev1.NotificationDeliveryMode,
) {
	t.Helper()
	if policy == nil {
		t.Fatal("notification policy is nil")
	}
	wantOverride := server
	if policy.RoomID != "" {
		wantOverride = room
	}
	actualOverride := notificationModeForSignal(policy.Overrides, testNotificationSignal(category, "room", "event"))
	actualEffective := notificationModeForSignal(policy.Effective, testNotificationSignal(category, "room", "event"))
	if actualOverride != wantOverride || actualEffective != effective {
		t.Fatalf("preference %v = (%v, %v), want (%v, %v)", category, actualOverride, actualEffective, wantOverride, effective)
	}
}
