package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestGetNotificationPolicyWaitsForCurrentConfigProjection(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "policy-read-fence-user", "Policy Read Fence User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := chattoCore.NotificationPolicy().SetServerNotificationMode(ctx, user.Id,
		corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
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
		policy []NotificationPolicyPreference
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
		assertNotificationPolicyIntensity(t, got.policy, corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION,
			corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
			corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
			corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
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
		corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
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
			corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION,
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
	if got := chattoCore.configModel.notificationRoomMode(user.Id, room.Id, corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
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
		corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
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
			corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION,
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
	if got := chattoCore.configModel.notificationRoomMode(user.Id, room.Id, corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REACTION); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
		t.Fatalf("room preference after rejected write = %v, want unspecified", got)
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
		corev1.NotificationPreferenceCategory(10),
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
	); err == nil {
		t.Fatal("SetServerNotificationMode accepted reserved notification policy kind 10")
	}
	assertNotificationPolicyIntensity(t, policy, corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_ROOM,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
	)
	assertNotificationPolicyIntensity(t, policy, corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MENTION,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
	)

	policy, err = preferences.SetServerNotificationMode(ctx, user.Id,
		corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_ROOM,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
	)
	if err != nil {
		t.Fatalf("SetServerNotificationMode: %v", err)
	}
	assertNotificationPolicyIntensity(t, policy, corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_ROOM,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
	)

	policy, err = preferences.SetRoomNotificationMode(ctx, user.Id, room.Id,
		corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_ROOM,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_BADGE,
	)
	if err != nil {
		t.Fatalf("SetRoomNotificationMode: %v", err)
	}
	assertNotificationPolicyIntensity(t, policy, corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_ROOM,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_BADGE,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_BADGE,
	)

	policy, err = preferences.SetRoomNotificationMode(ctx, user.Id, room.Id,
		corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_ROOM,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
	)
	if err != nil {
		t.Fatalf("clear room notification mode: %v", err)
	}
	assertNotificationPolicyIntensity(t, policy, corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_FOLLOWED_ROOM,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
	)
}

func assertNotificationPolicyIntensity(
	t *testing.T,
	preferences []NotificationPolicyPreference,
	category corev1.NotificationPreferenceCategory,
	server, room, effective corev1.NotificationDeliveryMode,
) {
	t.Helper()
	for _, preference := range preferences {
		if preference.Category != category {
			continue
		}
		wantOverride := server
		if preference.RoomID != "" {
			wantOverride = room
		}
		actualOverride := corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED
		if preference.Override != nil {
			actualOverride = *preference.Override
		}
		if actualOverride != wantOverride || preference.Effective != effective {
			t.Fatalf("preference %v = (%v, %v), want (%v, %v)", category, actualOverride, preference.Effective, wantOverride, effective)
		}
		return
	}
	t.Fatalf("preference %v not found", category)
}
