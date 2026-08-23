package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNotificationOccurrenceLifecycleUsesStreamFacts(t *testing.T) {
	chattoCore, _ := newTestCore(t)
	chattoCore.SetNotificationAlertHandler(func(context.Context, *corev1.NotificationOccurrence) error {
		return errors.New("hold alert pending for lifecycle assertions")
	})
	startCoreServices(t, chattoCore)
	ctx := testContext(t)
	model := chattoCore.NotificationOccurrences()
	now := time.Now().UTC().Truncate(time.Millisecond)
	input := CreateNotificationOccurrenceInput{
		RecipientID:   "U-notification-recipient",
		SourceEventID: "E-notification-source",
		SourceCreated: now,
		ActorID:       "U-notification-actor",
		Signal: testNotificationSignal(
			notificationTestSignalDirectMention,
			"R-notification-room",
			"E-notification-source",
		),
		Mode:           corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
		AttentionLevel: corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		SkipReadLookup: true,
	}

	created, wasCreated, err := model.Create(ctx, input)
	if err != nil || !wasCreated || created == nil {
		t.Fatalf("Create = (%+v, %v, %v), want new occurrence", created, wasCreated, err)
	}
	wantID := notificationOccurrenceID(input.RecipientID, input.SourceEventID, "direct_mention_received")
	if created.GetId() != wantID || created.GetNotificationStreamSequence() == 0 {
		t.Fatalf("created occurrence = %+v, want deterministic ID and stream sequence", created)
	}
	if created.GetAlertExpiresAt() == nil || !NotificationAlertPending(created) {
		t.Fatalf("created delivery state = %+v, want pending alert", created)
	}
	storedSignal, err := chattoCore.storage.notificationStream.GetMsg(ctx, created.GetNotificationStreamSequence())
	if err != nil {
		t.Fatalf("read stored signal: %v", err)
	}
	var signalEvent corev1.NotificationEvent
	if err := proto.Unmarshal(storedSignal.Data, &signalEvent); err != nil {
		t.Fatalf("decode stored signal: %v", err)
	}
	if got := signalEvent.GetSignalled(); signalEvent.GetNotificationId() != wantID || got.GetSourceEventId() != input.SourceEventID || got.GetSignal() == nil {
		t.Fatalf("stored immutable signal = %+v", got)
	}
	duplicate, wasCreated, err := model.Create(ctx, input)
	if err != nil || wasCreated || duplicate.GetId() != wantID {
		t.Fatalf("duplicate Create = (%+v, %v, %v)", duplicate, wasCreated, err)
	}
	read, err := model.MarkRead(ctx, input.RecipientID, wantID)
	if err != nil || !read.GetRead() || read.AlertDelivered == nil || read.GetAlertDelivered() {
		t.Fatalf("MarkRead = (%+v, %v)", read, err)
	}
	deleted, err := model.Delete(ctx, input.RecipientID, wantID)
	if err != nil || !deleted {
		t.Fatalf("Delete = (%v, %v)", deleted, err)
	}
	if _, err := model.Get(ctx, input.RecipientID, wantID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v, want not found", err)
	}
	if _, err := chattoCore.storage.notificationStream.GetMsg(ctx, created.GetNotificationStreamSequence()); !notificationSignalAlreadyAbsent(err) {
		t.Fatalf("rich signal after delete = %v, want securely deleted", err)
	}
	if recreated, wasCreated, err := model.Create(ctx, input); err != nil || wasCreated || recreated != nil {
		t.Fatalf("Create after tombstone = (%+v, %v, %v), want suppressed", recreated, wasCreated, err)
	}
	model.cleanupDismissedSignals(ctx, input.SourceCreated.Add(notificationTTL+notificationPhysicalCleanupGrace+time.Second))
	model.cleanedMu.Lock()
	cleanedCount := len(model.cleaned)
	model.cleanedMu.Unlock()
	if cleanedCount != 0 {
		t.Fatalf("expired secure-delete results = %d, want none", cleanedCount)
	}
}

func TestNotificationIdentitySeparatesSignalKinds(t *testing.T) {
	recipientID, sourceID := "U1", "E1"
	mention := notificationOccurrenceID(recipientID, sourceID, "direct_mention_received")
	reply := notificationOccurrenceID(recipientID, sourceID, "reply_received")
	if mention == reply {
		t.Fatalf("different signal kinds shared ID %q", mention)
	}
}

func TestNotificationCreateManyCommitsFanoutAsOneBatch(t *testing.T) {
	chattoCore, _ := newTestCore(t)
	startCoreServices(t, chattoCore)
	ctx := testContext(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	inputs := make([]CreateNotificationOccurrenceInput, 3)
	for i := range inputs {
		recipientID := fmt.Sprintf("U-batch-%d", i)
		inputs[i] = CreateNotificationOccurrenceInput{
			RecipientID: recipientID, SourceEventID: "E-batch-source", SourceCreated: now, ActorID: "U-actor",
			Signal: testNotificationSignal(notificationTestSignalAll, "R-batch", "E-batch-source"),
			Mode:   corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT, AttentionLevel: corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
			SkipReadLookup: true,
		}
	}
	if err := chattoCore.NotificationOccurrences().CreateMany(ctx, inputs); err != nil {
		t.Fatalf("CreateMany: %v", err)
	}
	var firstSequence uint64
	for i, input := range inputs {
		id := notificationOccurrenceID(input.RecipientID, input.SourceEventID, "all_mention_received")
		occurrence, err := chattoCore.NotificationOccurrences().Get(ctx, input.RecipientID, id)
		if err != nil {
			t.Fatalf("Get recipient %d: %v", i, err)
		}
		if i == 0 {
			firstSequence = occurrence.GetNotificationStreamSequence()
		}
		if got := occurrence.GetNotificationStreamSequence(); got != firstSequence+uint64(i) {
			t.Fatalf("recipient %d stream sequence = %d, want %d", i, got, firstSequence+uint64(i))
		}
	}
}

func TestNotificationCreateRetryReconcilesExistingOccurrenceWithReadBoundary(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	poster, err := chattoCore.CreateUser(ctx, SystemActorID, "retry-read-poster", "Retry Read Poster", "password")
	if err != nil {
		t.Fatalf("CreateUser poster: %v", err)
	}
	reader, err := chattoCore.CreateUser(ctx, SystemActorID, "retry-read-reader", "Retry Read Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, poster.Id, KindChannel, "", "retry-read-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{poster.Id, reader.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, poster.Id, "covered source", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	entry, ok := chattoCore.roomModel.timelineEntry(posted.GetId())
	if !ok {
		t.Fatal("posted message missing from timeline")
	}
	input := CreateNotificationOccurrenceInput{
		RecipientID: reader.Id, SourceEventID: posted.GetId(), SourceCreated: posted.GetCreatedAt().AsTime(), ActorID: poster.Id,
		Signal: testNotificationSignal(notificationTestSignalDirectMention, room.Id, posted.GetId()),
		Mode:   corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT, AttentionLevel: corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		SourceStreamSequence: entry.StreamSeq, SkipReadLookup: true,
	}
	occurrence, created, err := chattoCore.NotificationOccurrences().Create(ctx, input)
	if err != nil || !created || occurrence.GetRead() {
		t.Fatalf("initial Create = (%+v, %v, %v), want new unread occurrence", occurrence, created, err)
	}

	// Model the crash window where read-boundary repair already consumed this
	// scope before the committed signal reached the projection. The materializer
	// retry must reconcile the existing occurrence without another watcher wake.
	scope := notificationReadBoundaryScope{userID: reader.Id, roomID: room.Id}
	key := notificationReadBoundaryKey(reader.Id, room.Id, "")
	index := chattoCore.notificationBoundaries
	index.mu.Lock()
	index.read[key] = notificationReadBoundaryEntry{
		boundary: notificationReadBoundary{targetSequence: entry.StreamSeq, observedSequence: entry.StreamSeq},
		revision: 1,
	}
	delete(index.readDirty, scope)
	index.mu.Unlock()

	input.SkipReadLookup = false
	retried, created, err := chattoCore.NotificationOccurrences().Create(ctx, input)
	if err != nil || created || !retried.GetRead() {
		t.Fatalf("retry Create = (%+v, %v, %v), want existing read occurrence", retried, created, err)
	}
	stored, err := chattoCore.NotificationOccurrences().Get(ctx, reader.Id, occurrence.GetId())
	if err != nil || !stored.GetRead() {
		t.Fatalf("stored retry occurrence = (%+v, %v), want read", stored, err)
	}
	visible, err := chattoCore.NotificationOccurrences().VisibleOccurrences(ctx, reader.Id, []*corev1.NotificationOccurrence{stored})
	if err != nil || len(visible) != 1 {
		t.Fatalf("visible occurrence before message.read denial = (%d, %v), want (1, nil)", len(visible), err)
	}
	if err := chattoCore.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageRead); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	visible, err = chattoCore.NotificationOccurrences().VisibleOccurrences(ctx, reader.Id, []*corev1.NotificationOccurrence{stored})
	if err != nil || len(visible) != 0 {
		t.Fatalf("visible occurrence after message.read denial = (%d, %v), want (0, nil)", len(visible), err)
	}
}

func TestConcurrentNotificationRemovalCountsOneCommit(t *testing.T) {
	chattoCore, _ := newTestCore(t)
	startCoreServices(t, chattoCore)
	ctx := testContext(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	occurrence, created, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID: "U-delete-race", SourceEventID: "E-delete-race", SourceCreated: now, ActorID: "U-actor",
		Signal: testNotificationSignal(notificationTestSignalDirectMention, "R-delete-race", "E-delete-race"),
		Mode:   corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT, AttentionLevel: corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		SkipReadLookup: true,
	})
	if err != nil || !created {
		t.Fatalf("Create = (%+v, %v, %v), want new occurrence", occurrence, created, err)
	}

	start := make(chan struct{})
	results := make(chan int, 2)
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			count, deleteErr := chattoCore.NotificationOccurrences().deleteOccurrences(ctx, []*corev1.NotificationOccurrence{occurrence})
			results <- count
			errs <- deleteErr
		}()
	}
	ready.Wait()
	close(start)
	deleted := <-results + <-results
	for range 2 {
		if deleteErr := <-errs; deleteErr != nil {
			t.Fatalf("concurrent delete: %v", deleteErr)
		}
	}
	if deleted != 1 {
		t.Fatalf("combined concurrent delete count = %d, want 1", deleted)
	}
}

func TestNotificationProjectionExpiresOccurrencesAndTombstones(t *testing.T) {
	p := NewNotificationProjection()
	now := time.Now().UTC().Truncate(time.Millisecond)
	p.now = func() time.Time { return now }
	occurrence := &corev1.NotificationOccurrence{
		Id:              "N1",
		RecipientId:     "U1",
		SourceEventId:   "E1",
		SourceCreatedAt: timestamp(now.Add(-time.Hour)),
		Signal:          testNotificationSignal(notificationTestSignalReply, "R1", "E1"),
		ExpiresAt:       timestamp(now.Add(time.Minute)),
	}
	if err := p.Apply(notificationSignalledEvent("NE1", occurrence, now.Add(time.Minute)), 7); err != nil {
		t.Fatalf("Apply signal: %v", err)
	}
	if got, ok := p.occurrence("U1", "N1", now); !ok || got.GetNotificationStreamSequence() != 7 {
		t.Fatalf("projected occurrence = (%+v, %v)", got, ok)
	}
	scope := notificationReadBoundaryScope{userID: "U1", roomID: "R1"}
	if got := p.scopeOccurrences(scope, now); len(got) != 1 || got[0].GetId() != "N1" {
		t.Fatalf("scope occurrences = %+v, want N1", got)
	}
	if err := p.Apply(&corev1.NotificationEvent{
		Id: "NE2", RecipientId: "U1", NotificationId: "N1", OccurredAt: timestamp(now), ExpiresAt: timestamp(now.Add(time.Minute)),
		Event: &corev1.NotificationEvent_Removed{Removed: &corev1.NotificationRemoved{SignalStreamSequence: 7}},
	}, 8); err != nil {
		t.Fatalf("Apply dismissal: %v", err)
	}
	if _, ok := p.occurrence("U1", "N1", now); ok {
		t.Fatal("dismissed occurrence remained visible")
	}
	ref := notificationOccurrenceRef{recipientID: "U1", notificationID: "N1"}
	states := p.occurrenceStates([]notificationOccurrenceRef{
		ref,
		{recipientID: "U2", notificationID: "N1"},
		{recipientID: "U1", notificationID: "missing"},
	}, now)
	if state := states[ref]; state.occurrence != nil || !state.tombstoned {
		t.Fatalf("dismissed occurrence state = %+v, want tombstone only", state)
	}
	if state := states[notificationOccurrenceRef{recipientID: "U2", notificationID: "N1"}]; state.occurrence != nil || state.tombstoned {
		t.Fatalf("cross-recipient occurrence state = %+v, want empty", state)
	}
	if got := p.scopeOccurrences(scope, now); len(got) != 0 {
		t.Fatalf("dismissed scope occurrences = %+v, want none", got)
	}
	if got := p.pendingPhysicalDeletes(now)["N1"].signalSequence; got != 7 {
		t.Fatalf("pending physical delete sequence = %d, want 7", got)
	}
	now = now.Add(2 * time.Minute)
	if p.occurrenceStates([]notificationOccurrenceRef{ref}, now)[ref].tombstoned {
		t.Fatal("application-expired tombstone still suppressed semantic state")
	}
	if got := p.pendingPhysicalDeletes(now)["N1"].signalSequence; got != 7 {
		t.Fatalf("cleanup-grace tombstone sequence = %d, want 7", got)
	}
	now = now.Add(notificationPhysicalCleanupGrace)
	if got := p.pendingPhysicalDeletes(now); len(got) != 0 {
		t.Fatalf("physically expired tombstones = %+v, want none", got)
	}
}

func TestNotificationProjectionColdReplayRetainsExpiredDismissalCleanupCoordinate(t *testing.T) {
	p := NewNotificationProjection()
	now := time.Now().UTC().Truncate(time.Millisecond)
	p.now = func() time.Time { return now }
	expiresAt := now.Add(-time.Minute)
	occurrence := &corev1.NotificationOccurrence{
		Id: "N-expired", RecipientId: "U1", SourceEventId: "E1", SourceCreatedAt: timestamp(now.Add(-notificationTTL)),
		Signal: testNotificationSignal(notificationTestSignalReply, "R1", "E1"), ExpiresAt: timestamp(expiresAt),
	}
	if err := p.Apply(notificationSignalledEvent("signal-expired", occurrence, expiresAt), 7); err != nil {
		t.Fatalf("Apply expired signal: %v", err)
	}
	if _, visible := p.occurrence("U1", occurrence.GetId(), now); visible {
		t.Fatal("application-expired signal became visible during cold replay")
	}
	if err := p.Apply(&corev1.NotificationEvent{
		Id: "remove-expired", RecipientId: "U1", NotificationId: occurrence.GetId(), OccurredAt: timestamp(now), ExpiresAt: timestamp(expiresAt),
		Event: &corev1.NotificationEvent_Removed{Removed: &corev1.NotificationRemoved{SignalStreamSequence: 7}},
	}, 8); err != nil {
		t.Fatalf("Apply expired removal: %v", err)
	}
	if got := p.pendingPhysicalDeletes(now)[occurrence.GetId()].signalSequence; got != 7 {
		t.Fatalf("cold-replayed secure-delete sequence = %d, want 7", got)
	}
}

func TestNotificationProjectionKeepsFirstAlertResolution(t *testing.T) {
	p := NewNotificationProjection()
	now := time.Now().UTC().Truncate(time.Millisecond)
	p.now = func() time.Time { return now }
	occurrence := &corev1.NotificationOccurrence{
		Id: "N1", RecipientId: "U1", SourceEventId: "E1", SourceCreatedAt: timestamp(now),
		Signal:    testNotificationSignal(notificationTestSignalDirectMention, "R1", "E1"),
		ExpiresAt: timestamp(now.Add(time.Hour)), AlertExpiresAt: timestamp(now.Add(time.Minute)),
	}
	if err := p.Apply(notificationSignalledEvent("signal", occurrence, now.Add(time.Hour)), 1); err != nil {
		t.Fatal(err)
	}
	resolved := func(id string, delivered bool) *corev1.NotificationEvent {
		return &corev1.NotificationEvent{
			Id: id, RecipientId: "U1", NotificationId: "N1", OccurredAt: timestamp(now), ExpiresAt: timestamp(now.Add(time.Hour)),
			Event: &corev1.NotificationEvent_AlertResolved{AlertResolved: &corev1.NotificationAlertResolved{Delivered: delivered}},
		}
	}
	if err := p.Apply(resolved("first", true), 2); err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(resolved("late-contradiction", false), 3); err != nil {
		t.Fatal(err)
	}
	got, ok := p.occurrence("U1", "N1", now)
	if !ok || got.AlertDelivered == nil || !got.GetAlertDelivered() {
		t.Fatalf("terminal alert state = %+v, want first Delivered outcome", got)
	}
}

func notificationSignalledEvent(id string, occurrence *corev1.NotificationOccurrence, expires time.Time) *corev1.NotificationEvent {
	return &corev1.NotificationEvent{
		Id: id, RecipientId: occurrence.GetRecipientId(), NotificationId: occurrence.GetId(), OccurredAt: occurrence.GetSourceCreatedAt(), ExpiresAt: timestamp(expires),
		Event: &corev1.NotificationEvent_Signalled{Signalled: &corev1.NotificationSignalled{
			SourceEventId:        occurrence.GetSourceEventId(),
			SourceCreatedAt:      occurrence.GetSourceCreatedAt(),
			ActorId:              occurrence.GetActorId(),
			Signal:               occurrence.GetSignal(),
			InitiallyRead:        occurrence.GetRead(),
			SourceStreamSequence: occurrence.GetSourceStreamSequence(),
			AttentionLevel:       occurrence.GetAttentionLevel(),
			AlertExpiresAt:       occurrence.GetAlertExpiresAt(),
		}},
	}
}

func TestUnsupportedNotificationSignalDetection(t *testing.T) {
	if !NotificationOccurrenceHasUnsupportedSignal(&corev1.NotificationOccurrence{Signal: testUnsupportedNotificationSignal()}) {
		t.Fatal("future signal was not detected")
	}
	if NotificationOccurrenceHasUnsupportedSignal(&corev1.NotificationOccurrence{Signal: &corev1.NotificationSignal{}}) {
		t.Fatal("empty signal was treated as an unknown future signal")
	}
}
