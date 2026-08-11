package core

import (
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

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
	for _, preference := range policy {
		if preference.Reason == corev1.NotificationReason_NOTIFICATION_REASON_ROOM_INVITATION {
			t.Fatal("policy exposed room invitations without an occurrence producer")
		}
	}
	if _, err := preferences.SetServerNotificationIntensity(ctx, user.Id,
		corev1.NotificationReason_NOTIFICATION_REASON_ROOM_INVITATION,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
	); err == nil {
		t.Fatal("SetServerNotificationIntensity accepted unsupported room invitations")
	}
	assertNotificationPolicyIntensity(t, policy, corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF,
	)
	assertNotificationPolicyIntensity(t, policy, corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
	)

	policy, err = preferences.SetServerNotificationIntensity(ctx, user.Id,
		corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
	)
	if err != nil {
		t.Fatalf("SetServerNotificationIntensity: %v", err)
	}
	assertNotificationPolicyIntensity(t, policy, corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
	)

	policy, err = preferences.SetRoomNotificationIntensity(ctx, user.Id, room.Id,
		corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
	)
	if err != nil {
		t.Fatalf("SetRoomNotificationIntensity: %v", err)
	}
	assertNotificationPolicyIntensity(t, policy, corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
	)

	policy, err = preferences.SetRoomNotificationIntensity(ctx, user.Id, room.Id,
		corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED,
	)
	if err != nil {
		t.Fatalf("clear room notification intensity: %v", err)
	}
	assertNotificationPolicyIntensity(t, policy, corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
	)
}

func assertNotificationPolicyIntensity(
	t *testing.T,
	preferences []NotificationPolicyPreference,
	reason corev1.NotificationReason,
	server, room, effective corev1.NotificationDeliveryIntensity,
) {
	t.Helper()
	for _, preference := range preferences {
		if preference.Reason != reason {
			continue
		}
		if preference.ServerIntensity != server || preference.RoomIntensity != room || preference.Effective != effective {
			t.Fatalf("preference %v = (%v, %v, %v), want (%v, %v, %v)", reason, preference.ServerIntensity, preference.RoomIntensity, preference.Effective, server, room, effective)
		}
		return
	}
	t.Fatalf("preference %v not found", reason)
}
