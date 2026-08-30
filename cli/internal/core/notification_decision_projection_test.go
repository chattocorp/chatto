package core

import (
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	notificationv1 "hmans.de/chatto/internal/pb/chatto/core/notification/v1"
)

func TestNotificationDecisionUsesCurrentPolicyAfterSourceEvent(t *testing.T) {
	p := NewNotificationDecisionProjection()
	const (
		roomID      = "R1"
		recipientID = "U1"
	)
	roomScope := roomID
	source := &evtv1.Event{
		Id: "source", ActorId: "U2", CreatedAt: timestamppb.Now(),
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{
			RoomId: roomID, InThread: "root",
			Mentions: []*evtv1.MessageMention{{
				UserId: recipientID,
				Cause:  &evtv1.MessageMention_Direct{Direct: &evtv1.DirectUserMention{}},
			}},
		}},
	}
	events := []*evtv1.Event{
		{Id: "user", Event: &evtv1.Event_UserAccountCreated{UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: recipientID}}},
		{Id: "room", Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: roomID, Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL}}},
		{Id: "read", Event: &evtv1.Event_RbacPermissionGranted{RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeServer, "", RoleEveryone, PermMessageRead)}},
		{Id: "join", ActorId: recipientID, Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: roomID}}},
		{Id: "notify", Event: &evtv1.Event_UserNotificationPolicyChanged{UserNotificationPolicyChanged: &evtv1.UserNotificationPolicyChangedEvent{
			UserId: recipientID, RoomId: &roomScope, Overrides: &evtv1.NotificationDeliveryModes{DirectMentions: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION.Enum()},
		}}},
		source,
		{Id: "off", Event: &evtv1.Event_UserNotificationPolicyChanged{UserNotificationPolicyChanged: &evtv1.UserNotificationPolicyChangedEvent{
			UserId: recipientID, RoomId: &roomScope, Overrides: &evtv1.NotificationDeliveryModes{DirectMentions: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum()},
		}}},
	}
	applyNotificationDecisionEvents(t, p, events)

	var decisions []notificationRecipientDecision
	if err := p.withCurrent(time.Now(), func(snapshot *notificationDecisionSnapshot) error {
		decisions = buildMessageNotificationDecisions(snapshot, source, "", "")
		return nil
	}); err != nil {
		t.Fatalf("withCurrent: %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("decisions = %+v, want current Off policy to suppress the source", decisions)
	}
}

func TestNotificationDecisionUsesCurrentMembershipAfterSourceEvent(t *testing.T) {
	p := NewNotificationDecisionProjection()
	const (
		roomID = "R1"
		oldID  = "old-member"
		newID  = "new-member"
	)
	source := &evtv1.Event{Id: "source", ActorId: "author", CreatedAt: timestamppb.Now(), Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: roomID}}}
	events := []*evtv1.Event{
		{Id: "room", Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: roomID, Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL}}},
		{Id: "read", Event: &evtv1.Event_RbacPermissionGranted{RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeServer, "", RoleEveryone, PermMessageRead)}},
		{Id: "old-user", Event: &evtv1.Event_UserAccountCreated{UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: oldID}}},
		{Id: "new-user", Event: &evtv1.Event_UserAccountCreated{UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: newID}}},
		{Id: "old-join", ActorId: oldID, Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: roomID}}},
		source,
		{Id: "old-leave", ActorId: oldID, Event: &evtv1.Event_UserLeftRoom{UserLeftRoom: &evtv1.UserLeftRoomEvent{RoomId: roomID}}},
		{Id: "new-join", ActorId: newID, Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: roomID}}},
	}
	applyNotificationDecisionEvents(t, p, events)

	var decisions []notificationRecipientDecision
	if err := p.withCurrent(time.Now(), func(snapshot *notificationDecisionSnapshot) error {
		decisions = buildMessageNotificationDecisions(snapshot, source, "", "")
		return nil
	}); err != nil {
		t.Fatalf("withCurrent: %v", err)
	}
	if len(decisions) != 1 || decisions[0].recipientID != newID || notificationSignalIdentity(decisions[0].signal) != string(notificationTestSignalRoomMessage) {
		t.Fatalf("decisions = %+v, want only current member %q", decisions, newID)
	}
}

func TestLegacyMessageMentionIDsDoNotGuessRichMentionCause(t *testing.T) {
	p := NewNotificationDecisionProjection()
	const (
		roomID      = "R1"
		recipientID = "U1"
	)
	source := &evtv1.Event{
		Id: "source", ActorId: "U2", CreatedAt: timestamppb.Now(),
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: roomID, MentionedUserIds: []string{recipientID}}},
	}
	applyNotificationDecisionEvents(t, p, []*evtv1.Event{
		{Id: "user", Event: &evtv1.Event_UserAccountCreated{UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: recipientID}}},
		{Id: "room", Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: roomID, Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL}}},
		{Id: "read", Event: &evtv1.Event_RbacPermissionGranted{RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeServer, "", RoleEveryone, PermMessageRead)}},
		{Id: "join", ActorId: recipientID, Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: roomID}}},
		source,
	})

	var decisions []notificationRecipientDecision
	if err := p.withCurrent(time.Now(), func(snapshot *notificationDecisionSnapshot) error {
		decisions = buildMessageNotificationDecisions(snapshot, source, "", "")
		return nil
	}); err != nil {
		t.Fatalf("withCurrent: %v", err)
	}
	if len(decisions) != 1 || notificationSignalIdentity(decisions[0].signal) != string(notificationTestSignalRoomMessage) {
		t.Fatalf("legacy decisions = %+v, want only room-message cause", decisions)
	}
}

func TestDirectMentionAllowsCurrentInteractionScopedVisibility(t *testing.T) {
	p := NewNotificationDecisionProjection()
	const (
		roomID      = "R1"
		recipientID = "U1"
	)
	source := &evtv1.Event{
		Id: "source", ActorId: "U2", CreatedAt: timestamppb.Now(),
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{
			RoomId: roomID,
			Mentions: []*evtv1.MessageMention{{
				UserId: recipientID,
				Cause:  &evtv1.MessageMention_Direct{Direct: &evtv1.DirectUserMention{}},
			}},
		}},
	}
	applyNotificationDecisionEvents(t, p, []*evtv1.Event{
		{Id: "user", Event: &evtv1.Event_UserAccountCreated{UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: recipientID}}},
		{Id: "room", Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: roomID, Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL}}},
		{Id: "interaction-read", Event: &evtv1.Event_RbacPermissionGranted{RbacPermissionGranted: rbacUserPermissionGrantedEvent(ScopeRoom, roomID, recipientID, PermMessageReadInteractions)}},
		{Id: "join", ActorId: recipientID, Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: roomID}}},
		source,
	})

	var decisions []notificationRecipientDecision
	if err := p.withCurrent(time.Now(), func(snapshot *notificationDecisionSnapshot) error {
		decisions = buildMessageNotificationDecisions(snapshot, source, "", "")
		return nil
	}); err != nil {
		t.Fatalf("withCurrent: %v", err)
	}
	if len(decisions) != 1 || decisions[0].recipientID != recipientID || notificationSignalIdentity(decisions[0].signal) != string(notificationTestSignalDirectMention) {
		t.Fatalf("interaction-scoped decisions = %+v, want direct mention for %s", decisions, recipientID)
	}
}

func TestThreadMessageDoesNotProduceRoomMessageSignal(t *testing.T) {
	p := NewNotificationDecisionProjection()
	source := &evtv1.Event{Id: "reply", ActorId: "author", CreatedAt: timestamppb.Now(), Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1", InThread: "root"}}}
	applyNotificationDecisionEvents(t, p, []*evtv1.Event{
		{Id: "user", Event: &evtv1.Event_UserAccountCreated{UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: "recipient"}}},
		{Id: "room", Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: "R1", Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL}}},
		{Id: "read", Event: &evtv1.Event_RbacPermissionGranted{RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeServer, "", RoleEveryone, PermMessageRead)}},
		{Id: "join", ActorId: "recipient", Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: "R1"}}},
		source,
	})

	var decisions []notificationRecipientDecision
	if err := p.withCurrent(time.Now(), func(snapshot *notificationDecisionSnapshot) error {
		decisions = buildMessageNotificationDecisions(snapshot, source, "", "")
		return nil
	}); err != nil {
		t.Fatalf("withCurrent: %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("thread decisions = %+v, want no room-message output", decisions)
	}
}

func TestNotificationDecisionSnapshotRestoresCurrentState(t *testing.T) {
	p := NewNotificationDecisionProjection()
	applyNotificationDecisionEvents(t, p, []*evtv1.Event{
		{Id: "user", Event: &evtv1.Event_UserAccountCreated{UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: "U1"}}},
		{Id: "room", Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: "R1", Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL}}},
		{Id: "join", ActorId: "U1", Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: "R1"}}},
	})
	data, err := p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	restored := NewNotificationDecisionProjection()
	if err := restored.Restore(data); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if err := restored.withCurrent(time.Now(), func(snapshot *notificationDecisionSnapshot) error {
		if !snapshot.membershipExists("U1", "R1") {
			t.Fatal("restored current state does not contain membership")
		}
		return nil
	}); err != nil {
		t.Fatalf("withCurrent: %v", err)
	}
}

func TestNotificationOccurrenceInputRetainsRoleMentionNames(t *testing.T) {
	source := &evtv1.Event{Id: "source", ActorId: "actor", CreatedAt: timestamppb.Now()}
	message := &notificationv1.NotificationMessageReference{RoomId: "room", EventId: "source"}
	inputs := newNotificationOccurrenceInputs(source, []notificationRecipientDecision{{
		recipientID: "recipient",
		signal: &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_RoleMentionReceived{RoleMentionReceived: &notificationv1.RoleMentionReceived{
			Message: message, RoleNames: []string{"moderator", "staff"},
		}}},
		mode: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
	}})
	if len(inputs) != 1 {
		t.Fatalf("inputs = %d, want 1", len(inputs))
	}
	got := inputs[0].Signal.GetRoleMentionReceived().GetRoleNames()
	if !slices.Equal(got, []string{"moderator", "staff"}) {
		t.Fatalf("role names = %v, want source role handles", got)
	}
}

func applyNotificationDecisionEvents(t *testing.T, projection *NotificationDecisionProjection, events []*evtv1.Event) {
	t.Helper()
	for index, event := range events {
		if err := projection.Apply(event, uint64(index+1)); err != nil {
			t.Fatalf("Apply sequence %d: %v", index+1, err)
		}
	}
}
