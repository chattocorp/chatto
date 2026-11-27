package core

import (
	"context"
	"testing"
	"time"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// These bytes are the persisted pre-call policies: field 1 is direct_messages.
func TestCallPolicyUpgradePreservesDirectMessages(t *testing.T) {
	var server, dm evtv1.NotificationDeliveryModes
	require.NoError(t, proto.Unmarshal([]byte{0x08, 0x01}, &server))
	require.NoError(t, proto.Unmarshal([]byte{0x08, 0x02}, &dm))
	off := evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF
	push := evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION
	inApp := evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION

	inherited := effectiveNotificationDeliveryModes(&server, nil)
	require.Equal(t, off, inherited.GetDirectMessages())
	require.Equal(t, push, inherited.GetDirectCalls())
	require.Equal(t, off, inherited.GetRoomCalls())
	patched, err := applyNotificationDeliveryModesPatch(&dm, &evtv1.NotificationDeliveryModes{DirectCalls: off.Enum()}, &fieldmaskpb.FieldMask{Paths: []string{"direct_calls"}})
	require.NoError(t, err)
	require.Equal(t, inApp, effectiveNotificationDeliveryModes(&server, patched).GetDirectMessages())
	cleared, err := applyNotificationDeliveryModesPatch(patched, nil, &fieldmaskpb.FieldMask{Paths: []string{"direct_calls"}})
	require.NoError(t, err)
	require.True(t, proto.Equal(&dm, cleared))
	require.Equal(t, push, effectiveNotificationDeliveryModes(&server, cleared).GetDirectCalls())
}

func TestDirectCallNotificationLifecycle(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	caller, err := c.CreateUser(ctx, SystemActorID, "caller", "Caller", "password")
	require.NoError(t, err)
	recipient, err := c.CreateUser(ctx, SystemActorID, "recipient", "Recipient", "password")
	require.NoError(t, err)
	dm, _, err := c.roomCommands.StartDM(ctx, RoomStartDMInput{ActorID: caller.Id, ParticipantIDs: []string{recipient.Id}})
	require.NoError(t, err)
	off := evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF
	_, err = c.NotificationPolicy().UpdateNotificationPolicy(ctx, recipient.Id, "", &evtv1.NotificationDeliveryModes{DirectMessages: off.Enum()}, &fieldmaskpb.FieldMask{Paths: []string{"direct_messages"}})
	require.NoError(t, err)
	require.NoError(t, c.JoinVoiceCall(ctx, caller.Id, dm.Id))
	rows := testNotificationOccurrences(t, c, recipient.Id)
	require.Len(t, rows, 1)
	call := rows[0].GetSignal().GetIncomingCall()
	require.Equal(t, dm.Id, call.GetRoomId())
	require.NotEmpty(t, call.GetCallId())
	require.NotNil(t, rows[0].GetAlertExpiresAt())
	require.Empty(t, testNotificationOccurrences(t, c, caller.Id))
	visible, err := c.NotificationOccurrences().VisibleOccurrences(ctx, recipient.Id, rows)
	require.NoError(t, err)
	require.Len(t, visible, 1)
	require.NoError(t, c.JoinVoiceCall(ctx, recipient.Id, dm.Id))
	require.Empty(t, testNotificationOccurrences(t, c, recipient.Id))
	for _, id := range []string{recipient.Id, caller.Id} {
		require.NoError(t, c.RecordCallParticipantLeft(ctx, dm.Id, id, evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER))
	}
	policy, err := c.NotificationPolicy().UpdateNotificationPolicy(ctx, recipient.Id, dm.Id, &evtv1.NotificationDeliveryModes{DirectCalls: off.Enum()}, &fieldmaskpb.FieldMask{Paths: []string{"direct_calls"}})
	require.NoError(t, err)
	require.Equal(t, off, policy.Effective.GetDirectMessages())
	require.NoError(t, c.JoinVoiceCall(ctx, caller.Id, dm.Id))
	require.Empty(t, testNotificationOccurrences(t, c, recipient.Id))
}

func TestRoomCallNotificationOverridesAndCleanup(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	caller, err := c.CreateUser(ctx, SystemActorID, "room-caller", "Caller", "password")
	require.NoError(t, err)
	recipient, err := c.CreateUser(ctx, SystemActorID, "room-recipient", "Recipient", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-room", "")
	require.NoError(t, err)
	for _, id := range []string{caller.Id, recipient.Id} {
		_, err = c.JoinRoom(ctx, id, KindChannel, id, room.Id)
		require.NoError(t, err)
	}
	for _, mode := range []evtv1.NotificationDeliveryMode{
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE,
	} {
		if mode != evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF {
			_, err = c.NotificationPolicy().UpdateNotificationPolicy(ctx, recipient.Id, room.Id, &evtv1.NotificationDeliveryModes{RoomCalls: mode.Enum()}, &fieldmaskpb.FieldMask{Paths: []string{"room_calls"}})
			require.NoError(t, err)
		}
		require.NoError(t, c.JoinVoiceCall(ctx, caller.Id, room.Id))
		rows := testNotificationOccurrences(t, c, recipient.Id)
		if mode == evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION {
			require.Len(t, rows, 1)
			require.Equal(t, room.Id, rows[0].GetSignal().GetIncomingCall().GetRoomId())
			require.Nil(t, rows[0].GetAlertExpiresAt())
		} else {
			require.Empty(t, rows)
		}
		badge, err := c.NotificationOccurrences().HasNotificationUnread(ctx, recipient.Id, room.Id, "")
		require.NoError(t, err)
		require.Equal(t, mode == evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE, badge)
		if mode == evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE {
			require.NoError(t, c.JoinVoiceCall(ctx, recipient.Id, room.Id))
			require.NoError(t, c.RecordCallParticipantLeft(ctx, room.Id, recipient.Id, evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER))
			waitForNotificationMaterializer(t, c)
			badge, err = c.NotificationOccurrences().HasNotificationUnread(ctx, recipient.Id, room.Id, "")
			require.NoError(t, err)
			require.False(t, badge, "leaving a call does not make its start unread again")
		}
		require.NoError(t, c.RecordCallParticipantLeft(ctx, room.Id, caller.Id, evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER))
		require.Empty(t, testNotificationOccurrences(t, c, recipient.Id))
		badge, err = c.NotificationOccurrences().HasNotificationUnread(ctx, recipient.Id, room.Id, "")
		require.NoError(t, err)
		require.False(t, badge)
	}
}

// Start the call on one replica and revoke access on another. The push worker
// uses the same original call identity and policy as the live notification.
func TestCallNotificationAcrossReplicas(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	ctx := testContext(t)
	delivered := make(chan *notificationv1.NotificationOccurrence, 1)
	start := func() *ChattoCore {
		replica, err := NewChattoCore(ctx, newReplicaConn(t, ns), testCoreConfig())
		require.NoError(t, err)
		replica.SetNotificationAlertHandler(func(ctx context.Context, occurrence *notificationv1.NotificationOccurrence) error {
			select {
			case delivered <- occurrence:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		startCoreServices(t, replica)
		return replica
	}
	callerReplica, receiver := start(), start()
	caller, err := callerReplica.CreateUser(ctx, SystemActorID, "call-sender", "Caller", "password")
	require.NoError(t, err)
	recipient, err := receiver.CreateUser(ctx, SystemActorID, "call-receiver", "Recipient", "password")
	require.NoError(t, err)
	group, err := callerReplica.roomLayout.CreateRoomGroup(ctx, SystemActorID, "Calls", "")
	require.NoError(t, err)
	room, err := callerReplica.CreateRoom(ctx, SystemActorID, KindChannel, group.Id, "room-calls", "")
	require.NoError(t, err)
	for _, id := range []string{caller.Id, recipient.Id} {
		_, err = callerReplica.JoinRoom(ctx, id, KindChannel, id, room.Id)
		require.NoError(t, err)
	}
	push := evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION
	_, err = receiver.NotificationPolicy().UpdateScopedNotificationPolicy(ctx, recipient.Id, NotificationPolicyScope{Kind: NotificationPolicyScopeRoomGroup, ID: group.Id}, &evtv1.NotificationDeliveryModes{RoomCalls: push.Enum()}, &fieldmaskpb.FieldMask{Paths: []string{"room_calls"}})
	require.NoError(t, err)
	require.NoError(t, callerReplica.JoinVoiceCall(ctx, caller.Id, room.Id))
	select {
	case occurrence := <-delivered:
		require.Equal(t, recipient.Id, occurrence.GetRecipientId())
		call, ok := callerReplica.callModel.activeCall(room.Id)
		require.True(t, ok)
		require.Equal(t, call.CallID, occurrence.GetSignal().GetIncomingCall().GetCallId())
	case <-time.After(5 * time.Second):
		t.Fatal("room group call preference did not reach push delivery")
	}
	require.NoError(t, receiver.DenyUserRoomPermission(ctx, SystemActorID, room.Id, recipient.Id, PermCallJoin))
	require.Empty(t, testNotificationOccurrences(t, callerReplica, recipient.Id))
}
