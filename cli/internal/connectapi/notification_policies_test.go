package connectapi

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestNotificationDeliveryModeAliasesPreserveProtoJSONCompatibility(t *testing.T) {
	encoded, err := protojson.Marshal(&apiv1.NotificationDeliveryModes{
		DirectMessages: apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION.Enum(),
		DirectMentions: apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION.Enum(),
		Reactions:      apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE.Enum(),
		RoomMessages:   apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum(),
	})
	if err != nil {
		t.Fatalf("marshal delivery modes: %v", err)
	}
	if got := string(encoded); !strings.Contains(got, "NOTIFICATION_DELIVERY_MODE_SILENT") || !strings.Contains(got, "NOTIFICATION_DELIVERY_MODE_ALERT") || !strings.Contains(got, "NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE") {
		t.Fatalf("legacy JSON names changed: %s", got)
	}

	var decoded apiv1.NotificationDeliveryModes
	if err := protojson.Unmarshal([]byte(`{"directMessages":"NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION","directMentions":"NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION","reactions":"NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE","roomMessages":"NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE"}`), &decoded); err != nil {
		t.Fatalf("unmarshal canonical aliases: %v", err)
	}
	if decoded.GetDirectMessages() != apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION ||
		decoded.GetDirectMentions() != apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION ||
		decoded.GetReactions() != apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE ||
		decoded.GetRoomMessages() != apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE {
		t.Fatalf("decoded delivery modes = %+v", &decoded)
	}
}

func serverNotificationPolicyScope() *apiv1.NotificationPolicyScope {
	return &apiv1.NotificationPolicyScope{Scope: &apiv1.NotificationPolicyScope_Server{Server: &emptypb.Empty{}}}
}

func groupNotificationPolicyScope(groupID string) *apiv1.NotificationPolicyScope {
	return &apiv1.NotificationPolicyScope{Scope: &apiv1.NotificationPolicyScope_RoomGroupId{RoomGroupId: groupID}}
}

func roomNotificationPolicyScope(roomID string) *apiv1.NotificationPolicyScope {
	return &apiv1.NotificationPolicyScope{Scope: &apiv1.NotificationPolicyScope_RoomId{RoomId: roomID}}
}

func TestNotificationPolicyServiceScopesBatchAndLegacyCompatibility(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	group, err := env.core.CreateRoomGroup(ctx, core.SystemActorID, "Policy Group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := env.core.CreateRoom(ctx, core.SystemActorID, core.KindChannel, group.Id, "policy-service-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	other, err := env.core.CreateUser(ctx, core.SystemActorID, "policy-service-other", "Policy Service Other", "password")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	inaccessible, err := env.core.CreateRoom(ctx, other.Id, core.KindChannel, "", "policy-service-private", "")
	if err != nil {
		t.Fatalf("CreateRoom inaccessible: %v", err)
	}

	updated, err := env.notificationPolicies.UpdateNotificationPolicy(ctx, connect.NewRequest(&apiv1.NotificationPolicyServiceUpdateNotificationPolicyRequest{
		Scope: groupNotificationPolicyScope(group.Id),
		Overrides: &apiv1.NotificationDeliveryModes{
			Reactions:    apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE.Enum(),
			RoomMessages: apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION.Enum(),
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"reactions", "room_messages"}},
	}))
	if err != nil {
		t.Fatalf("UpdateNotificationPolicy group: %v", err)
	}
	if got := updated.Msg.GetPolicy().GetPolicy().GetEffective().GetReactions(); got != apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE {
		t.Fatalf("updated group effective reactions = %v, want UNREAD_BADGE", got)
	}
	if got := updated.Msg.GetPolicy().GetPolicy().GetEffective().GetRoomMessages(); got != apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION {
		t.Fatalf("updated group effective room messages = %v, want PUSH_NOTIFICATION", got)
	}

	roomPolicy, err := env.notificationPolicies.GetNotificationPolicy(ctx, connect.NewRequest(&apiv1.NotificationPolicyServiceGetNotificationPolicyRequest{
		Scope: roomNotificationPolicyScope(room.Id),
	}))
	if err != nil {
		t.Fatalf("GetNotificationPolicy room: %v", err)
	}
	if got := roomPolicy.Msg.GetPolicy().GetPolicy().GetEffective().GetReactions(); got != apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE {
		t.Fatalf("room group-inherited reactions = %v, want UNREAD_BADGE", got)
	}
	if got := roomPolicy.Msg.GetPolicy().GetPolicy().GetEffective().GetRoomMessages(); got != apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION {
		t.Fatalf("room group-inherited room messages = %v, want PUSH_NOTIFICATION", got)
	}

	roomID := room.Id
	legacy, err := env.notifications.GetNotificationPolicy(ctx, connect.NewRequest(&apiv1.GetNotificationPolicyRequest{RoomId: &roomID}))
	if err != nil {
		t.Fatalf("legacy GetNotificationPolicy: %v", err)
	}
	if got := legacy.Msg.GetPolicy().GetEffective().GetReactions(); got != apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE {
		t.Fatalf("legacy room effective reactions = %v, want group-inherited UNREAD_BADGE", got)
	}

	batch, err := env.notificationPolicies.BatchGetNotificationPolicies(ctx, connect.NewRequest(&apiv1.BatchGetNotificationPoliciesRequest{Scopes: []*apiv1.NotificationPolicyScope{
		serverNotificationPolicyScope(),
		groupNotificationPolicyScope(group.Id),
		roomNotificationPolicyScope(room.Id),
		serverNotificationPolicyScope(),
		groupNotificationPolicyScope("missing-group"),
		roomNotificationPolicyScope(inaccessible.Id),
	}}))
	if err != nil {
		t.Fatalf("BatchGetNotificationPolicies: %v", err)
	}
	if got := len(batch.Msg.GetPolicies()); got != 3 {
		t.Fatalf("batch policy count = %d, want 3", got)
	}
	if batch.Msg.GetPolicies()[0].GetScope().GetServer() == nil || batch.Msg.GetPolicies()[1].GetScope().GetRoomGroupId() != group.Id || batch.Msg.GetPolicies()[2].GetScope().GetRoomId() != room.Id {
		t.Fatalf("batch policy order/scopes = %+v", batch.Msg.GetPolicies())
	}
}

func TestNotificationPolicyServiceAuthenticationLimitsAndValidation(t *testing.T) {
	env := newConnectAPITestEnv(t)
	_, err := env.notificationPolicies.GetNotificationPolicy(env.ctx, connect.NewRequest(&apiv1.NotificationPolicyServiceGetNotificationPolicyRequest{
		Scope: serverNotificationPolicyScope(),
	}))
	requireConnectCode(t, err, connect.CodeUnauthenticated)

	ctx := withCaller(env.ctx, env.viewer)
	tooMany := make([]*apiv1.NotificationPolicyScope, 101)
	for index := range tooMany {
		tooMany[index] = serverNotificationPolicyScope()
	}
	_, err = env.notificationPolicies.BatchGetNotificationPolicies(ctx, connect.NewRequest(&apiv1.BatchGetNotificationPoliciesRequest{Scopes: tooMany}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)

	_, err = env.notificationPolicies.UpdateNotificationPolicy(ctx, connect.NewRequest(&apiv1.NotificationPolicyServiceUpdateNotificationPolicyRequest{
		Scope:      serverNotificationPolicyScope(),
		Overrides:  &apiv1.NotificationDeliveryModes{},
		UpdateMask: &fieldmaskpb.FieldMask{},
	}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)
}
