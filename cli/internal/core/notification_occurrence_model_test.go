package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNotificationOccurrenceLifecycleAndDeterministicIdentity(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	model := chattoCore.NotificationOccurrences()
	now := time.Now().UTC().Truncate(time.Millisecond)
	model.now = func() time.Time { return now }

	input := CreateNotificationOccurrenceInput{
		RecipientID:   "U-notification-recipient",
		SourceEventID: "E-notification-source",
		SourceCreated: now.Add(-24 * time.Hour),
		ActorID:       "U-notification-actor",
		Target: &corev1.NotificationTarget{
			RoomId:  "R-notification-room",
			EventId: "E-notification-source",
		},
		Reasons: []*corev1.NotificationReasonMatch{
			{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
			},
			{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
			},
			{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
			},
		},
		SkipReadLookup: true,
	}

	created, wasCreated, err := model.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if candidates, err := model.index.alertCandidates(ctx); err != nil || len(candidates) != 1 {
		t.Fatalf("alert candidates after create = (%d, %v), want one", len(candidates), err)
	}
	if !wasCreated || created == nil {
		t.Fatalf("Create = (%v, %v), want a new occurrence", created, wasCreated)
	}
	if created.GetId() != notificationOccurrenceID(input.RecipientID, input.SourceEventID) {
		t.Fatalf("id = %q, want deterministic identity", created.GetId())
	}
	if got := created.GetStrongestIntensity(); got != corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT {
		t.Fatalf("strongest intensity = %v, want ALERT", got)
	}
	if got := len(created.GetReasons()); got != 2 {
		t.Fatalf("reasons = %d, want two deduplicated reasons", got)
	}
	originalExpiry := created.GetExpiresAt().AsTime()

	claim, claimed, err := model.ClaimPendingAlert(ctx)
	if err != nil || !claimed || claim.GetId() != created.GetId() {
		t.Fatalf("ClaimPendingAlert = (%v, %v, %v), want created occurrence", claim, claimed, err)
	}
	renewed, renewedClaim, err := model.RenewAlertClaim(ctx, claim)
	if err != nil || !renewedClaim || !renewed.GetAlertClaimedUntil().AsTime().After(claim.GetAlertClaimedUntil().AsTime()) {
		t.Fatalf("RenewAlertClaim = (%v, %v, %v), want extended exact claim", renewed, renewedClaim, err)
	}
	claim = renewed
	if err := model.CompleteAlertClaim(ctx, claim, false); err != nil {
		t.Fatalf("CompleteAlertClaim failed delivery: %v", err)
	}
	if retry, retryClaimed, err := model.ClaimPendingAlert(ctx); err != nil || retryClaimed || retry != nil {
		t.Fatalf("immediate retry ClaimPendingAlert = (%v, %v, %v), want paced retry", retry, retryClaimed, err)
	}
	now = now.Add(notificationAlertRetryDelay + time.Millisecond)
	claim, claimed, err = model.ClaimPendingAlert(ctx)
	if err != nil || !claimed {
		t.Fatalf("retry ClaimPendingAlert = (%v, %v, %v), want retry", claim, claimed, err)
	}
	if err := model.CompleteAlertClaim(ctx, claim, true); err != nil {
		t.Fatalf("CompleteAlertClaim delivered: %v", err)
	}
	if claim, claimed, err := model.ClaimPendingAlert(ctx); err != nil || claimed || claim != nil {
		t.Fatalf("ClaimPendingAlert after delivery = (%v, %v, %v), want none", claim, claimed, err)
	}

	duplicate, wasCreated, err := model.Create(ctx, input)
	if err != nil {
		t.Fatalf("duplicate Create: %v", err)
	}
	if wasCreated || duplicate.GetId() != created.GetId() {
		t.Fatalf("duplicate Create = (%v, %v), want existing occurrence", duplicate, wasCreated)
	}

	updated, err := model.MarkRead(ctx, input.RecipientID, created.GetId())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.GetInboxState() != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ {
		t.Fatalf("Update = %+v, want read", updated)
	}
	if !updated.GetExpiresAt().AsTime().Equal(originalExpiry) {
		t.Fatalf("expiry changed from %v to %v", originalExpiry, updated.GetExpiresAt().AsTime())
	}
	secondInput := input
	secondInput.SourceEventID = "E-notification-source-2"
	secondInput.SourceCreated = input.SourceCreated.Add(time.Minute)
	secondInput.Target = proto.Clone(input.Target).(*corev1.NotificationTarget)
	secondInput.Target.EventId = secondInput.SourceEventID
	if second, wasCreated, err := model.Create(ctx, secondInput); err != nil || !wasCreated || second == nil {
		t.Fatalf("Create second grouped occurrence = (%v, %v, %v), want occurrence, true, nil", second, wasCreated, err)
	}

	groups, err := model.Groups(ctx, input.RecipientID)
	if err != nil || len(groups) != 1 {
		t.Fatalf("Groups = (%v, %v), want one", groups, err)
	}
	if got := len(groups[0].Occurrences); got != 2 {
		t.Fatalf("Group occurrences = %d, want two", got)
	}
	deleted, err := model.DeleteGroup(ctx, input.RecipientID, groups[0].ID)
	if err != nil || deleted != 2 {
		t.Fatalf("DeleteGroup = (%v, %v), want two", deleted, err)
	}
	if groups, err := model.Groups(ctx, input.RecipientID); err != nil || len(groups) != 0 {
		t.Fatalf("Groups after delete = (%v, %v), want empty", groups, err)
	}
	if _, err := model.Get(ctx, input.RecipientID, created.GetId()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get deleted occurrence = %v, want ErrNotFound", err)
	}
	if recreated, wasCreated, err := model.Create(ctx, input); err != nil || wasCreated || recreated != nil {
		t.Fatalf("Create after tombstone = (%v, %v, %v), want nil, false, nil", recreated, wasCreated, err)
	}
}

func TestCompleteAlertClaimDoesNotDeliverAfterLeaseExpiry(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	model := chattoCore.NotificationOccurrences()
	now := time.Now().UTC().Truncate(time.Millisecond)
	model.now = func() time.Time { return now }
	created, _, err := model.Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   "U-expired-claim-recipient",
		SourceEventID: "E-expired-claim-source",
		SourceCreated: now,
		Target:        &corev1.NotificationTarget{RoomId: "R-expired-claim", EventId: "E-expired-claim-source"},
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		}},
		SkipReadLookup: true,
	})
	if err != nil || created == nil {
		t.Fatalf("Create = (%v, %v)", created, err)
	}
	claim, claimed, err := model.ClaimPendingAlert(ctx)
	if err != nil || !claimed {
		t.Fatalf("ClaimPendingAlert = (%v, %v, %v)", claim, claimed, err)
	}
	now = claim.GetAlertClaimedUntil().AsTime().Add(time.Millisecond)
	if err := model.CompleteAlertClaim(ctx, claim, true); err != nil {
		t.Fatalf("CompleteAlertClaim: %v", err)
	}
	current, err := model.Get(ctx, created.GetRecipientId(), created.GetId())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if current.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_DELIVERED {
		t.Fatal("expired claim was recorded as delivered")
	}
	if retry, retryClaimed, err := model.ClaimPendingAlert(ctx); err != nil || !retryClaimed || retry == nil {
		t.Fatalf("ClaimPendingAlert after expiry = (%v, %v, %v), want retry", retry, retryClaimed, err)
	}
}

func TestNotificationOccurrenceReadCancelsPendingAlert(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	model := chattoCore.NotificationOccurrences()
	now := time.Now().UTC()
	model.now = func() time.Time { return now }
	created, _, err := model.Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   "U-read-alert-recipient",
		SourceEventID: "E-read-alert-source",
		SourceCreated: now,
		Target:        &corev1.NotificationTarget{RoomId: "R-read-alert", EventId: "E-read-alert-source"},
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		}},
		SkipReadLookup: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	updated, err := model.MarkRead(ctx, created.GetRecipientId(), created.GetId())
	if err != nil {
		t.Fatalf("Update read: %v", err)
	}
	if updated.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED {
		t.Fatalf("alert state = %v, want SILENCED", updated.GetAlertState())
	}
	if candidates, err := model.index.alertCandidates(ctx); err != nil || len(candidates) != 0 {
		t.Fatalf("alert candidates after read = (%d, %v), want none", len(candidates), err)
	}
	if claim, claimed, err := model.ClaimPendingAlert(ctx); err != nil || claimed || claim != nil {
		t.Fatalf("ClaimPendingAlert after read = (%v, %v, %v), want none", claim, claimed, err)
	}
}

func TestSilenceAlertClaimTerminatesExactClaim(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	model := chattoCore.NotificationOccurrences()
	now := time.Now().UTC()
	model.now = func() time.Time { return now }
	created, _, err := model.Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   "U-dnd-claim-recipient",
		SourceEventID: "E-dnd-claim-source",
		SourceCreated: now,
		Target:        &corev1.NotificationTarget{RoomId: "R-dnd-claim", EventId: "E-dnd-claim-source"},
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		}},
		SkipReadLookup: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	claim, claimed, err := model.ClaimPendingAlert(ctx)
	if err != nil || !claimed {
		t.Fatalf("ClaimPendingAlert = (%v, %v, %v)", claim, claimed, err)
	}
	if silenced, err := model.SilenceAlertClaim(ctx, claim); err != nil || !silenced {
		t.Fatalf("SilenceAlertClaim = (%v, %v), want true, nil", silenced, err)
	}
	current, err := model.Get(ctx, created.GetRecipientId(), created.GetId())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if current.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED {
		t.Fatalf("alert state = %v, want SILENCED", current.GetAlertState())
	}
	if err := model.CompleteAlertClaim(ctx, claim, true); err != nil {
		t.Fatalf("CompleteAlertClaim after silence: %v", err)
	}
	if current, err = model.Get(ctx, created.GetRecipientId(), created.GetId()); err != nil || current.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED {
		t.Fatalf("state after completion = (%v, %v), want SILENCED", current.GetAlertState(), err)
	}
}

func TestVisibleOccurrencesChecksMessageAndExactReactionLifecycle(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "visible-target-author", "Visible Target Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	actor, err := chattoCore.CreateUser(ctx, SystemActorID, "visible-target-actor", "Visible Target Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "visible-target-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.Id, actor.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "target", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if added, err := chattoCore.ReactionModel().AddReaction(ctx, ReactionMutationInput{
		ActorID: actor.Id, RoomID: room.Id, MessageEventID: posted.Id, Emoji: "thumbsup",
	}); err != nil || !added {
		t.Fatalf("AddReaction = (%v, %v)", added, err)
	}
	occurrences, err := chattoCore.NotificationOccurrences().List(ctx, author.Id)
	if err != nil || len(occurrences) != 1 {
		t.Fatalf("reaction occurrences = (%v, %v), want one", occurrences, err)
	}
	reactionOccurrence := proto.Clone(occurrences[0]).(*corev1.NotificationOccurrence)
	if visible, err := chattoCore.NotificationOccurrences().VisibleOccurrences(ctx, author.Id, []*corev1.NotificationOccurrence{reactionOccurrence}); err != nil || len(visible) != 1 {
		t.Fatalf("VisibleOccurrences before removal = (%v, %v), want one, nil", visible, err)
	}
	if removed, err := chattoCore.ReactionModel().RemoveReaction(ctx, ReactionMutationInput{
		ActorID: actor.Id, RoomID: room.Id, MessageEventID: posted.Id, Emoji: "thumbsup",
	}); err != nil || !removed {
		t.Fatalf("RemoveReaction = (%v, %v)", removed, err)
	}
	if visible, err := chattoCore.NotificationOccurrences().VisibleOccurrences(ctx, author.Id, []*corev1.NotificationOccurrence{reactionOccurrence}); err != nil || len(visible) != 0 {
		t.Fatalf("VisibleOccurrences after reaction removal = (%v, %v), want empty, nil", visible, err)
	}
	if err := chattoCore.DeleteMessage(ctx, author.Id, KindChannel, room.Id, posted.Id); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if visible, err := chattoCore.NotificationOccurrences().VisibleOccurrences(ctx, author.Id, []*corev1.NotificationOccurrence{reactionOccurrence}); err != nil || len(visible) != 0 {
		t.Fatalf("VisibleOccurrences after target retraction = (%v, %v), want empty, nil", visible, err)
	}
}

func TestVisibleOccurrencesWaitsForGroupAndRBACProjectionTails(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "visible-fence-author", "Visible Fence Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "visible-fence-recipient", "Visible Fence Recipient", "password")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "visible-fence-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.SetRoomUniversal(ctx, author.Id, KindChannel, room.Id, true); err != nil {
		t.Fatalf("SetRoomUniversal: %v", err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "visibility fence target", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	sequence, err := chattoCore.GetEventSequence(ctx, KindChannel, room.Id, posted.Id)
	if err != nil {
		t.Fatalf("GetEventSequence: %v", err)
	}
	occurrence, created, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID: recipient.Id, SourceEventID: "visible-fence-source", SourceCreated: posted.GetCreatedAt().AsTime(),
		SourceStreamSequence: sequence, ActorID: author.Id,
		Target:         &corev1.NotificationTarget{RoomId: room.Id, EventId: posted.Id},
		Reasons:        []*corev1.NotificationReasonMatch{{Reason: corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION, Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE}},
		SkipReadLookup: true,
	})
	if err != nil || !created {
		t.Fatalf("Create occurrence = (%+v, %v, %v)", occurrence, created, err)
	}
	group, err := chattoCore.CreateRoomGroup(ctx, author.Id, "Visibility Fence Group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	if err := chattoCore.MoveRoomToGroup(ctx, author.Id, room.Id, group.GetId()); err != nil {
		t.Fatalf("MoveRoomToGroup: %v", err)
	}
	if err := chattoCore.DenyUserRoomPermission(ctx, SystemActorID, room.Id, recipient.Id, PermRoomJoin); err != nil {
		t.Fatalf("DenyUserRoomPermission: %v", err)
	}

	delayedGroup := evtstream.NewProjectionHandle(
		chattoCore.js,
		chattoCore.storage.serverEvtStream,
		NewRoomGroupLayoutProjection(),
		testCoreLogger(),
	)
	delayedRBAC := evtstream.NewProjectionHandle(
		chattoCore.js,
		chattoCore.storage.serverEvtStream,
		NewRBACProjection(),
		testCoreLogger(),
	)
	chattoCore.roomModel.groupLayout = delayedGroup
	chattoCore.rbacModel = newRBACModel(delayedRBAC)

	type visibleResult struct {
		items []*corev1.NotificationOccurrence
		err   error
	}
	result := make(chan visibleResult, 1)
	go func() {
		items, err := chattoCore.NotificationOccurrences().VisibleOccurrences(ctx, recipient.Id, []*corev1.NotificationOccurrence{occurrence})
		result <- visibleResult{items: items, err: err}
	}()
	select {
	case early := <-result:
		t.Fatalf("VisibleOccurrences returned before delayed authorization projections started: (%+v, %v)", early.items, early.err)
	case <-time.After(50 * time.Millisecond):
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	go func() { done <- delayedGroup.Projector().Run(runCtx) }()
	go func() { done <- delayedRBAC.Projector().Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		for range 2 {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("delayed visibility projector did not stop")
			}
		}
	})
	select {
	case got := <-result:
		if got.err != nil || len(got.items) != 0 {
			t.Fatalf("VisibleOccurrences after authorization catch-up = (%+v, %v), want empty", got.items, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("VisibleOccurrences did not finish after authorization projections caught up")
	}
}

func TestNotificationOccurrenceIndexConvergesAcrossReplicas(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	second := NewNotificationOccurrenceModel(chattoCore, chattoCore.storage.runtimeStateKV, testCoreLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- second.Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("second notification index did not stop")
		}
	})
	if err := second.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	sourceTime := time.Now().UTC()
	created, _, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   "U-replica-recipient",
		SourceEventID: "E-replica-source",
		SourceCreated: sourceTime,
		Target: &corev1.NotificationTarget{
			RoomId:  "R-replica-room",
			EventId: "E-replica-source",
		},
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_REPLY,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
		}},
		SkipReadLookup: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	entry, err := chattoCore.storage.runtimeStateKV.Get(ctx, notificationOccurrenceKey("U-replica-recipient", "E-replica-source"))
	if err != nil {
		t.Fatalf("Get KV entry: %v", err)
	}
	if err := second.WaitForSourceRevision(ctx, created.GetRecipientId(), created.GetSourceEventId(), entry.Revision()); err != nil {
		t.Fatalf("wait for source revision on second model: %v", err)
	}
	got, exists, err := second.index.occurrenceByID(ctx, "U-replica-recipient", created.GetId())
	if err != nil || !exists || got.occurrence.GetId() != created.GetId() {
		t.Fatalf("second index occurrence = (%v, %v, %v)", got.occurrence, exists, err)
	}
}

func TestNotificationOccurrenceIndexPrunesExpiredRecordsWithoutKVDeleteEvent(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	now := time.Now().UTC()
	created, _, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   "U-expired-recipient",
		SourceEventID: "E-expired-source",
		SourceCreated: now,
		Target:        &corev1.NotificationTarget{RoomId: "R-expired-room", EventId: "E-expired-source"},
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_REPLY,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
		}},
		SkipReadLookup: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	key := notificationOccurrenceKey(created.GetRecipientId(), created.GetSourceEventId())
	entry, err := chattoCore.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get occurrence KV: %v", err)
	}
	expired := proto.Clone(created).(*corev1.NotificationOccurrence)
	expired.ExpiresAt = timestamppb.New(now.Add(-time.Second))
	data, err := proto.Marshal(expired)
	if err != nil {
		t.Fatalf("marshal expired occurrence: %v", err)
	}
	revision, err := chattoCore.storage.runtimeStateKV.Update(ctx, key, data, entry.Revision())
	if err != nil {
		t.Fatalf("write stale expired occurrence: %v", err)
	}
	if err := chattoCore.NotificationOccurrences().index.waitForRevision(ctx, key, revision); err != nil {
		t.Fatalf("wait for stale expiry revision: %v", err)
	}

	if occurrences, err := chattoCore.NotificationOccurrences().List(ctx, created.GetRecipientId()); err != nil || len(occurrences) != 0 {
		t.Fatalf("List expired occurrences = (%v, %v), want empty", occurrences, err)
	}
	if _, exists, err := chattoCore.NotificationOccurrences().index.occurrenceBySource(ctx, created.GetRecipientId(), created.GetSourceEventId()); err != nil || exists {
		t.Fatalf("expired index entry exists=%v, err=%v", exists, err)
	}
	chattoCore.NotificationOccurrences().index.mu.RLock()
	_, retainsRevisionFence := chattoCore.NotificationOccurrences().index.keyRevisions[key]
	chattoCore.NotificationOccurrences().index.mu.RUnlock()
	if retainsRevisionFence {
		t.Fatal("expired index entry retained its revision fence")
	}
	if err := chattoCore.NotificationOccurrences().index.waitForRevision(ctx, key, revision); err != nil {
		t.Fatalf("wait for pruned expiry revision: %v", err)
	}
}
