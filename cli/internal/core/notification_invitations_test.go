package core

import (
	"context"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func invitationNotificationFixture(t *testing.T, c *ChattoCore, ctx context.Context) (senderID, recipientID, roomID string) {
	t.Helper()
	sender, err := c.CreateUser(ctx, SystemActorID, "sender", "Sender", "password123")
	require.NoError(t, err)
	recipient, err := c.CreateUser(ctx, SystemActorID, "recipient", "Recipient", "password123")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, sender.Id, KindChannel, "", "private", "Private")
	require.NoError(t, err)
	_, err = c.JoinRoom(ctx, sender.Id, KindChannel, sender.Id, room.Id)
	require.NoError(t, err)
	require.NoError(t, c.GrantUserRoomPermission(ctx, SystemActorID, room.Id, sender.Id, PermRoomInvite))
	for _, permission := range []Permission{PermRoomJoin, PermRoomList, PermMessageRead} {
		require.NoError(t, c.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, permission))
	}
	return sender.Id, recipient.Id, room.Id
}

func TestInvitationNotificationDeliveryModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []evtv1.NotificationDeliveryMode{
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
	} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			c, _ := setupTestCore(t)
			ctx := testContext(t)
			sender, recipient, room := invitationNotificationFixture(t, c, ctx)
			// Leave Push unset to exercise the product default.
			if mode != evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION {
				_, err := c.NotificationPolicy().UpdateNotificationPolicy(ctx, recipient, "", &evtv1.NotificationDeliveryModes{RoomInvitations: mode.Enum()}, &fieldmaskpb.FieldMask{Paths: []string{"room_invitations"}})
				require.NoError(t, err)
			}
			invite, err := c.RoomInvitations().Create(ctx, sender, room, recipient)
			require.NoError(t, err)
			occurrences := testNotificationOccurrences(t, c, recipient)
			if notificationModeProducesOccurrence(mode) {
				require.Len(t, occurrences, 1)
				occurrence := occurrences[0]
				require.Equal(t, invite.ID, occurrence.GetSignal().GetRoomInvitationReceived().GetInvitationId())
				require.Equal(t, room, NotificationOccurrenceRoomID(occurrence))
				require.Nil(t, NotificationOccurrenceMessageReference(occurrence))
				require.Equal(t, invite.ExpiresAt, occurrence.GetExpiresAt().AsTime())
				require.Equal(t, notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT, occurrence.GetAttentionLevel())
				require.Equal(t, mode == evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION, occurrence.GetAlertExpiresAt() != nil)
				visible, err := c.NotificationOccurrences().VisibleOccurrences(ctx, recipient, occurrences)
				require.NoError(t, err)
				require.Len(t, visible, 1, "nonmember without message access can see their invitation")
				other, err := c.NotificationOccurrences().VisibleOccurrences(ctx, sender, occurrences)
				require.NoError(t, err)
				require.Empty(t, other)
				// Duplicate domain command cannot make another occurrence.
				_, err = c.RoomInvitations().Create(ctx, sender, room, recipient)
				require.NoError(t, err)
				require.Len(t, testNotificationOccurrences(t, c, recipient), 1)
			}
			if !notificationModeProducesOccurrence(mode) {
				require.Empty(t, occurrences)
			}
			view, err := c.RoomDirectoryReads().GetRoom(ctx, recipient, room)
			require.NoError(t, err)
			require.Equal(t, mode == evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE, view.ViewerState.HasUnread)
			// A recipient can configure an invited room without gaining membership.
			policy, err := c.NotificationPolicy().GetNotificationPolicy(ctx, recipient, room)
			require.NoError(t, err)
			require.Equal(t, mode, policy.Effective.GetRoomInvitations())
			_, err = c.RoomInvitations().Respond(ctx, recipient, invite.ID, false)
			require.NoError(t, err)
			require.Empty(t, testNotificationOccurrences(t, c, recipient))
			unread, err := c.NotificationOccurrences().HasNotificationUnread(ctx, recipient, room, "")
			require.NoError(t, err)
			require.False(t, unread)
		})
	}
}

func TestInvitationNotificationReadDeleteAndAcceptance(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	sender, recipient, room := invitationNotificationFixture(t, c, ctx)
	invite, err := c.RoomInvitations().Create(ctx, sender, room, recipient)
	require.NoError(t, err)
	occurrences := testNotificationOccurrences(t, c, recipient)
	require.Len(t, occurrences, 1)
	_, err = c.NotificationOccurrences().MarkRead(ctx, recipient, occurrences[0].GetId())
	require.NoError(t, err)
	_, err = c.NotificationOccurrences().Delete(ctx, recipient, occurrences[0].GetId())
	require.NoError(t, err)
	stillPending, err := c.RoomInvitations().Get(ctx, recipient, invite.ID)
	require.NoError(t, err)
	require.True(t, stillPending.pending(time.Now()))
	_, err = c.RoomInvitations().Create(ctx, sender, room, recipient)
	require.NoError(t, err)
	require.Empty(t, testNotificationOccurrences(t, c, recipient), "duplicate send does not resurrect dismissed notification")
	_, err = c.RoomInvitations().Respond(ctx, recipient, invite.ID, true)
	require.NoError(t, err)
	require.Empty(t, testNotificationOccurrences(t, c, recipient))
}

func TestExpiredInvitationDoesNotMaterializeNotification(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	sender, recipient, room := invitationNotificationFixture(t, c, ctx)
	source := newEvent(sender, &evtv1.Event{Event: &evtv1.Event_RoomInvitationCreated{RoomInvitationCreated: &evtv1.RoomInvitationCreatedEvent{RoomId: room, RecipientId: recipient, InvitationId: "expired-invitation", ExpiresAt: timestamppb.New(time.Now().Add(-time.Minute))}}})
	_, err := c.EventPublisher.AppendEventually(ctx, evtstream.RoomAggregate(room).SubjectFor(source), source)
	require.NoError(t, err)
	require.Empty(t, testNotificationOccurrences(t, c, recipient))
}

func TestInvitationNotificationCrossReplicaResolution(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	c := startReplica(t, ns, testCoreConfig())
	other := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	sender, recipient, room := invitationNotificationFixture(t, c, ctx)
	invite, err := c.RoomInvitations().Create(ctx, sender, room, recipient)
	require.NoError(t, err)
	occurrences := testNotificationOccurrences(t, other, recipient)
	require.Len(t, occurrences, 1)
	_, err = other.RoomInvitations().Respond(ctx, recipient, invite.ID, true)
	require.NoError(t, err)
	require.Empty(t, testNotificationOccurrences(t, c, recipient))
	require.Empty(t, testNotificationOccurrences(t, other, recipient))
	// A newly started process must not resurrect the resolved notification.
	restarted := startReplica(t, ns, testCoreConfig())
	require.Empty(t, testNotificationOccurrences(t, restarted, recipient))
}

func TestInvitationNotificationAlertDeliveryAndResolution(t *testing.T) {
	t.Parallel()
	c, _ := newTestCore(t)
	delivered := make(chan *notificationv1.NotificationOccurrence, 1)
	c.SetNotificationAlertHandler(func(_ context.Context, occurrence *notificationv1.NotificationOccurrence) error {
		delivered <- occurrence
		return nil
	})
	startCoreServices(t, c)
	ctx := testContext(t)
	sender, recipient, room := invitationNotificationFixture(t, c, ctx)
	invite, err := c.RoomInvitations().Create(ctx, sender, room, recipient)
	require.NoError(t, err)
	var occurrence *notificationv1.NotificationOccurrence
	select {
	case occurrence = <-delivered:
		require.Equal(t, recipient, occurrence.GetRecipientId())
		require.Equal(t, invite.ID, occurrence.GetSignal().GetRoomInvitationReceived().GetInvitationId())
	case <-time.After(5 * time.Second):
		t.Fatal("invitation did not reach the durable alert transport")
	}
	_, err = c.RoomInvitations().Respond(ctx, recipient, invite.ID, false)
	require.NoError(t, err)
	eligible, err := c.NotificationAlertEligible(ctx, occurrence)
	require.NoError(t, err)
	require.False(t, eligible, "a prepared alert cannot deliver after decline")
}

func TestInvitationNotificationSenderDeletionAcrossReplicas(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	c := startReplica(t, ns, testCoreConfig())
	other := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	sender, recipient, room := invitationNotificationFixture(t, c, ctx)
	_, err := c.RoomInvitations().Create(ctx, sender, room, recipient)
	require.NoError(t, err)
	occurrences := testNotificationOccurrences(t, other, recipient)
	require.Len(t, occurrences, 1)
	require.NoError(t, c.DeleteUser(ctx, SystemActorID, sender))
	visible, err := other.NotificationOccurrences().VisibleOccurrences(ctx, recipient, occurrences)
	require.NoError(t, err)
	require.Empty(t, visible, "sender deletion must suppress invitation attention on another replica")
	eligible, err := other.NotificationAlertEligible(ctx, occurrences[0])
	require.NoError(t, err)
	require.False(t, eligible)
}
