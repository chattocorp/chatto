package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

func TestNotificationAlertQueueConfigurationIsDurableAndBackedUp(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	info, err := chattoCore.storage.notificationQueue.Info(ctx)
	if err != nil {
		t.Fatalf("notification queue info: %v", err)
	}
	if info.Config.Name != notificationQueueStreamName || info.Config.Storage != jetstream.FileStorage ||
		info.Config.Retention != jetstream.WorkQueuePolicy || info.Config.MaxAge != notificationAlertDeliveryTTL {
		t.Fatalf("notification queue config = %+v", info.Config)
	}
	consumer, err := chattoCore.storage.notificationQueue.Consumer(ctx, notificationAlertConsumerName)
	if err != nil {
		t.Fatalf("notification alert consumer: %v", err)
	}
	consumerInfo, err := consumer.Info(ctx)
	if err != nil || consumerInfo.Config.AckPolicy != jetstream.AckExplicitPolicy {
		t.Fatalf("notification consumer info = (%+v, %v)", consumerInfo, err)
	}
}

func TestNotificationAlertQueueDeliversMaterializedOccurrence(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	delivered := make(chan *corev1.NotificationOccurrence, 1)
	chattoCore.SetNotificationAlertHandler(func(_ context.Context, occurrence *corev1.NotificationOccurrence) error {
		delivered <- occurrence
		return nil
	})

	alice, err := chattoCore.CreateUser(ctx, SystemActorID, "queue-alice", "Queue Alice", "password")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	bob, err := chattoCore.CreateUser(ctx, SystemActorID, "queue-bob", "Queue Bob", "password")
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	room, _, err := chattoCore.FindOrCreateDM(ctx, alice.Id, []string{bob.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindDM, room.Id, bob.Id, "queued", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	select {
	case occurrence := <-delivered:
		if occurrence.GetRecipientId() != alice.Id || occurrence.GetSourceEventId() != posted.GetId() {
			t.Fatalf("delivered occurrence = %+v", occurrence)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("notification alert was not delivered from the durable queue")
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		occurrence, getErr := chattoCore.NotificationOccurrences().Get(ctx, alice.Id, notificationOccurrenceID(alice.Id, posted.GetId()))
		if getErr == nil && occurrence.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_DELIVERED {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("terminal alert state = (%v, %v), want delivered", occurrence, getErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNotificationAlertDeliveryFencesMaterializerBeforeOccurrenceLookup(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "restored-alert-author", "Restored Alert Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "restored-alert-recipient", "Restored Alert Recipient", "password")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "restored-alert-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.Id, recipient.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "restored alert target", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("WaitCurrent before test: %v", err)
	}
	if occurrences, err := chattoCore.NotificationOccurrences().List(ctx, recipient.Id); err != nil || len(occurrences) != 0 {
		t.Fatalf("occurrences before restored job = (%+v, %v), want none", occurrences, err)
	}
	sequence, err := chattoCore.GetEventSequence(ctx, KindChannel, room.Id, posted.Id)
	if err != nil {
		t.Fatalf("GetEventSequence: %v", err)
	}
	input := CreateNotificationOccurrenceInput{
		RecipientID:          recipient.Id,
		SourceEventID:        posted.Id,
		SourceCreated:        posted.GetCreatedAt().AsTime(),
		SourceStreamSequence: sequence,
		ActorID:              author.Id,
		Target:               newNotificationRoomMessageTarget(room.Id, posted.Id),
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		}},
		SkipReadLookup: true,
	}
	job := &corev1.NotificationAlertJob{
		RecipientId:    recipient.Id,
		SourceEventId:  posted.Id,
		NotificationId: notificationOccurrenceID(recipient.Id, posted.Id),
	}
	data, err := proto.Marshal(job)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	fenced := false
	chattoCore.notificationAlertDelivery.waitForMaterializerCurrent = func(context.Context) error {
		fenced = true
		occurrence, created, err := chattoCore.NotificationOccurrences().Create(ctx, input)
		if err != nil || !created || occurrence == nil {
			t.Fatalf("materialize restored occurrence = (%+v, %v, %v), want created", occurrence, created, err)
		}
		return nil
	}
	providerCalls := 0
	chattoCore.SetNotificationAlertHandler(func(context.Context, *corev1.NotificationOccurrence) error {
		providerCalls++
		return nil
	})

	if err := chattoCore.notificationAlertDelivery.processDelivery(ctx, events.DurableDelivery{
		Data: data, PublishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("process restored delivery: %v", err)
	}
	if !fenced || providerCalls != 1 {
		t.Fatalf("restored delivery = (fenced %v, provider calls %d), want true, 1", fenced, providerCalls)
	}
	stored, err := chattoCore.NotificationOccurrences().Get(ctx, recipient.Id, job.GetNotificationId())
	if err != nil || stored.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_DELIVERED {
		t.Fatalf("restored alert state = (%+v, %v), want delivered", stored, err)
	}
}

func TestNotificationAlertQueueSilencesExpiredWorkWithoutCallingProvider(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	called := false
	chattoCore.SetNotificationAlertHandler(func(context.Context, *corev1.NotificationOccurrence) error {
		called = true
		return nil
	})
	now := time.Now().UTC()
	sourceCreated := now.Add(-notificationAlertDeliveryTTL - time.Second)
	occurrence, _, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   "U-expired-alert",
		SourceEventID: "E-expired-alert",
		SourceCreated: sourceCreated,
		Target:        newNotificationRoomMessageTarget("R-expired-alert", "E-expired-alert"),
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		}},
		SkipReadLookup: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	job := &corev1.NotificationAlertJob{
		RecipientId: occurrence.GetRecipientId(), SourceEventId: occurrence.GetSourceEventId(), NotificationId: occurrence.GetId(),
	}
	data, err := proto.Marshal(job)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := chattoCore.notificationAlertDelivery.processDelivery(ctx, events.DurableDelivery{
		Data: data, PublishedAt: now,
	}); err != nil {
		t.Fatalf("processDelivery: %v", err)
	}
	if called {
		t.Fatal("expired delivery called provider")
	}
	stored, err := chattoCore.NotificationOccurrences().Get(ctx, occurrence.GetRecipientId(), occurrence.GetId())
	if err != nil || stored.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED {
		t.Fatalf("expired alert state = (%v, %v), want silenced", stored, err)
	}
}

func TestNotificationAlertQueueDoesNotExtendOrShortenDeadlineFromPublishTime(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	now := time.Now().UTC()
	chattoCore.NotificationOccurrences().now = func() time.Time { return now }
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "republished-alert-author", "Republished Alert Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "republished-alert-recipient", "Republished Alert Recipient", "password")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "republished-alert-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.Id, recipient.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "still-live target", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	providerCalls := 0
	chattoCore.SetNotificationAlertHandler(func(context.Context, *corev1.NotificationOccurrence) error {
		providerCalls++
		return nil
	})
	occurrence, _, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   recipient.Id,
		SourceEventID: posted.Id,
		SourceCreated: now.Add(-time.Minute),
		ActorID:       author.Id,
		Target:        newNotificationRoomMessageTarget(room.Id, posted.Id),
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		}},
		SkipReadLookup: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	wantDeadline := occurrence.GetSourceCreatedAt().AsTime().UTC().Add(notificationAlertDeliveryTTL)
	if got := NotificationAlertDeadline(occurrence); !got.Equal(wantDeadline) {
		t.Fatalf("alert deadline = %v, want source-time deadline %v", got, wantDeadline)
	}
	job := &corev1.NotificationAlertJob{
		RecipientId: occurrence.GetRecipientId(), SourceEventId: occurrence.GetSourceEventId(), NotificationId: occurrence.GetId(),
		AlertExpiresAt: occurrence.GetAlertExpiresAt(),
	}
	data, err := proto.Marshal(job)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// A restored or redelivered queue record may have an old publish timestamp.
	// That transport metadata must not shorten a still-live source deadline.
	if err := chattoCore.notificationAlertDelivery.processDelivery(ctx, events.DurableDelivery{
		Data: data, PublishedAt: now.Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("processDelivery: %v", err)
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls = %d, want 1 while source deadline remains live", providerCalls)
	}
	stored, err := chattoCore.NotificationOccurrences().Get(ctx, occurrence.GetRecipientId(), occurrence.GetId())
	if err != nil || stored.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_DELIVERED {
		t.Fatalf("alert state = (%v, %v), want delivered", stored, err)
	}
}

func TestNotificationAlertQueueSilencesWorkWhenPushIsDisabled(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	alice, err := chattoCore.CreateUser(ctx, SystemActorID, "disabled-alert-alice", "Disabled Alert Alice", "password")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	bob, err := chattoCore.CreateUser(ctx, SystemActorID, "disabled-alert-bob", "Disabled Alert Bob", "password")
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	room, _, err := chattoCore.FindOrCreateDM(ctx, alice.Id, []string{bob.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindDM, room.Id, bob.Id, "disabled push", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("WaitCurrent: %v", err)
	}
	occurrence, err := chattoCore.NotificationOccurrences().Get(ctx, alice.Id, notificationOccurrenceID(alice.Id, posted.GetId()))
	if err != nil {
		t.Fatalf("Get occurrence: %v", err)
	}
	job := &corev1.NotificationAlertJob{
		RecipientId: occurrence.GetRecipientId(), SourceEventId: occurrence.GetSourceEventId(), NotificationId: occurrence.GetId(),
	}
	data, err := proto.Marshal(job)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := chattoCore.notificationAlertDelivery.processDelivery(ctx, events.DurableDelivery{
		Data: data, PublishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("processDelivery: %v", err)
	}
	stored, err := chattoCore.NotificationOccurrences().Get(ctx, occurrence.GetRecipientId(), occurrence.GetId())
	if err != nil || stored.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED {
		t.Fatalf("disabled alert state = (%v, %v), want silenced", stored, err)
	}
}

func TestNotificationAlertQueueRechecksCurrentPolicyBeforeProvider(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	alice, err := chattoCore.CreateUser(ctx, SystemActorID, "policy-alert-alice", "Policy Alert Alice", "password")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	bob, err := chattoCore.CreateUser(ctx, SystemActorID, "policy-alert-bob", "Policy Alert Bob", "password")
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	room, _, err := chattoCore.FindOrCreateDM(ctx, alice.Id, []string{bob.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindDM, room.Id, bob.Id, "policy downgrade", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	occurrence, err := chattoCore.NotificationOccurrences().Get(ctx, alice.Id, notificationOccurrenceID(alice.Id, posted.GetId()))
	if err != nil {
		t.Fatalf("Get occurrence: %v", err)
	}
	if _, err := chattoCore.NotificationPolicy().SetServerNotificationIntensity(
		ctx,
		alice.Id,
		corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MESSAGE,
		corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
	); err != nil {
		t.Fatalf("SetServerNotificationIntensity: %v", err)
	}
	called := false
	chattoCore.SetNotificationAlertHandler(func(context.Context, *corev1.NotificationOccurrence) error {
		called = true
		return nil
	})
	job := &corev1.NotificationAlertJob{
		RecipientId: occurrence.GetRecipientId(), SourceEventId: occurrence.GetSourceEventId(), NotificationId: occurrence.GetId(),
	}
	data, err := proto.Marshal(job)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := chattoCore.notificationAlertDelivery.processDelivery(ctx, events.DurableDelivery{
		Data: data, PublishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("processDelivery: %v", err)
	}
	if called {
		t.Fatal("policy-downgraded delivery called provider")
	}
	stored, err := chattoCore.NotificationOccurrences().Get(ctx, occurrence.GetRecipientId(), occurrence.GetId())
	if err != nil || stored.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED {
		t.Fatalf("downgraded alert state = (%v, %v), want silenced", stored, err)
	}
}

func TestNotificationAlertDeliveryRetriesTransientProviderFailure(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	alice, err := chattoCore.CreateUser(ctx, SystemActorID, "retry-alert-alice", "Retry Alert Alice", "password")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	bob, err := chattoCore.CreateUser(ctx, SystemActorID, "retry-alert-bob", "Retry Alert Bob", "password")
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	room, _, err := chattoCore.FindOrCreateDM(ctx, alice.Id, []string{bob.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindDM, room.Id, bob.Id, "retry provider", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	occurrence, err := chattoCore.NotificationOccurrences().Get(ctx, alice.Id, notificationOccurrenceID(alice.Id, posted.GetId()))
	if err != nil {
		t.Fatalf("Get occurrence: %v", err)
	}
	job := &corev1.NotificationAlertJob{
		RecipientId: occurrence.GetRecipientId(), SourceEventId: occurrence.GetSourceEventId(), NotificationId: occurrence.GetId(),
	}
	data, err := proto.Marshal(job)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	providerErr := errors.New("provider temporarily unavailable")
	attempts := 0
	chattoCore.SetNotificationAlertHandler(func(context.Context, *corev1.NotificationOccurrence) error {
		attempts++
		if attempts == 1 {
			return providerErr
		}
		return nil
	})
	delivery := events.DurableDelivery{Data: data, PublishedAt: time.Now().UTC()}
	if err := chattoCore.notificationAlertDelivery.processDelivery(ctx, delivery); !errors.Is(err, providerErr) {
		t.Fatalf("first processDelivery error = %v, want provider error", err)
	}
	pending, err := chattoCore.NotificationOccurrences().Get(ctx, occurrence.GetRecipientId(), occurrence.GetId())
	if err != nil || pending.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING {
		t.Fatalf("occurrence after transient failure = (%+v, %v), want pending", pending, err)
	}
	if err := chattoCore.notificationAlertDelivery.processDelivery(ctx, delivery); err != nil {
		t.Fatalf("retry processDelivery: %v", err)
	}
	delivered, err := chattoCore.NotificationOccurrences().Get(ctx, occurrence.GetRecipientId(), occurrence.GetId())
	if err != nil || delivered.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_DELIVERED {
		t.Fatalf("occurrence after retry = (%+v, %v), want delivered", delivered, err)
	}
}

func TestNotificationAlertDeliveryPersistsTransportSuppression(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	alice, err := chattoCore.CreateUser(ctx, SystemActorID, "suppressed-alert-alice", "Suppressed Alert Alice", "password")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	bob, err := chattoCore.CreateUser(ctx, SystemActorID, "suppressed-alert-bob", "Suppressed Alert Bob", "password")
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	room, _, err := chattoCore.FindOrCreateDM(ctx, alice.Id, []string{bob.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindDM, room.Id, bob.Id, "suppressed provider", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	occurrence, err := chattoCore.NotificationOccurrences().Get(ctx, alice.Id, notificationOccurrenceID(alice.Id, posted.GetId()))
	if err != nil {
		t.Fatalf("Get occurrence: %v", err)
	}
	chattoCore.SetNotificationAlertHandler(func(context.Context, *corev1.NotificationOccurrence) error {
		return ErrNotificationAlertSuppressed
	})
	job := &corev1.NotificationAlertJob{
		RecipientId: occurrence.GetRecipientId(), SourceEventId: occurrence.GetSourceEventId(), NotificationId: occurrence.GetId(),
	}
	data, err := proto.Marshal(job)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := chattoCore.notificationAlertDelivery.processDelivery(ctx, events.DurableDelivery{
		Data: data, PublishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("processDelivery: %v", err)
	}
	silenced, err := chattoCore.NotificationOccurrences().Get(ctx, occurrence.GetRecipientId(), occurrence.GetId())
	if err != nil || silenced.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED {
		t.Fatalf("occurrence after suppression = (%+v, %v), want silenced", silenced, err)
	}
}
