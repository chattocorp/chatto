package notificationstream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/pkg/events"
)

func TestPublisherStoresCanonicalEventWithPhysicalTTL(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:        "NOTIFICATIONS_TEST",
		Subjects:    Subjects(),
		Storage:     jetstream.FileStorage,
		AllowMsgTTL: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	event := &notificationv1.NotificationEvent{
		Id:             "NE1",
		RecipientId:    "U1",
		NotificationId: "N1",
		OccurredAt:     timestamppb.New(now),
		ExpiresAt:      timestamppb.New(now.Add(time.Hour)),
		Event:          &notificationv1.NotificationEvent_Read{Read: &notificationv1.NotificationRead{}},
	}
	publisher := NewPublisher(js, stream, time.Hour, nil)
	publisher.now = func() time.Time { return now }
	position, err := publisher.AppendEventually(ctx, event)
	if err != nil {
		t.Fatalf("AppendEventually: %v", err)
	}
	if position.SubjectFilter != ReadSubject || position.Seq == 0 {
		t.Fatalf("position = %+v, want read subject and non-zero sequence", position)
	}
	stored, err := stream.GetMsg(ctx, position.Seq)
	if err != nil {
		t.Fatal(err)
	}
	want, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Subject != ReadSubject || !bytes.Equal(stored.Data, want) {
		t.Fatalf("stored record = subject %q data %x", stored.Subject, stored.Data)
	}
	if got := stored.Header.Get("Nats-TTL"); got != (2 * time.Hour).String() {
		t.Fatalf("Nats-TTL = %q, want %q", got, (2 * time.Hour).String())
	}
}

func TestPublisherRejectsExpiredAndIncompleteEvents(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	publisher := &Publisher{retentionGrace: time.Hour, now: func() time.Time { return now }}
	ctx := context.Background()

	if _, err := publisher.AppendEventually(ctx, &notificationv1.NotificationEvent{}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("incomplete event error = %v, want ErrInvalidEvent", err)
	}
	expired := &notificationv1.NotificationEvent{
		Id: "NE1", RecipientId: "U1", NotificationId: "N1", OccurredAt: timestamppb.New(now.Add(-2 * time.Hour)), ExpiresAt: timestamppb.New(now.Add(-time.Hour)),
		Event: &notificationv1.NotificationEvent_Read{Read: &notificationv1.NotificationRead{}},
	}
	if _, err := publisher.AppendEventually(ctx, expired); !errors.Is(err, ErrExpiredEvent) {
		t.Fatalf("expired event error = %v, want ErrExpiredEvent", err)
	}
}

func TestNotificationSubjectsAreCompleteAndCallerOwned(t *testing.T) {
	want := []string{SignalledSubject, ReadSubject, RemovedSubject, AlertResolvedSubject}
	got := Subjects()
	if len(got) != len(want) {
		t.Fatalf("Subjects() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Subjects() = %v, want %v", got, want)
		}
	}
	got[0] = "mutated"
	if Subjects()[0] != SignalledSubject {
		t.Fatal("caller mutation changed the notification subject contract")
	}
}

func TestPublisherSurvivesSharedSubjectContention(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "NOTIFICATIONS_CONTENTION_TEST", Subjects: Subjects(), Storage: jetstream.MemoryStorage, AllowMsgTTL: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher := NewPublisher(js, stream, time.Hour, nil)
	now := time.Now().UTC()
	const writers = 32
	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			id := fmt.Sprintf("NE-%d", i)
			_, err := publisher.AppendEventually(ctx, &notificationv1.NotificationEvent{
				Id: id, RecipientId: "U1", NotificationId: id, OccurredAt: timestamppb.New(now), ExpiresAt: timestamppb.New(now.Add(time.Hour)),
				Event: &notificationv1.NotificationEvent_Read{Read: &notificationv1.NotificationRead{}},
			})
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("contended append: %v", err)
		}
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.State.Msgs; got != writers {
		t.Fatalf("stored messages = %d, want %d", got, writers)
	}
}

func TestPublisherAppendsLifecycleBatchAtomically(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "NOTIFICATIONS_BATCH_TEST", Subjects: Subjects(), Storage: jetstream.MemoryStorage, AllowMsgTTL: true, AllowAtomicPublish: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher := NewPublisher(js, stream, time.Hour, nil)
	now := time.Now().UTC()
	eventsToAppend := make([]*notificationv1.NotificationEvent, 3)
	for i := range eventsToAppend {
		id := fmt.Sprintf("NE-batch-%d", i)
		eventsToAppend[i] = &notificationv1.NotificationEvent{
			Id: id, RecipientId: "U1", NotificationId: id, OccurredAt: timestamppb.New(now), ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			Event: &notificationv1.NotificationEvent_Read{Read: &notificationv1.NotificationRead{}},
		}
	}

	results, err := publisher.AppendBatchEventuallyResults(ctx, eventsToAppend)
	if err != nil {
		t.Fatalf("AppendBatchEventuallyResults: %v", err)
	}
	positions := make([]events.StreamPosition, len(results))
	for i, result := range results {
		positions[i] = result.Position
		if !result.Committed {
			t.Fatalf("initial batch result %d = %+v, want committed", i, result)
		}
	}
	if len(positions) != 3 || positions[1].Seq != positions[0].Seq+1 || positions[2].Seq != positions[1].Seq+1 {
		t.Fatalf("batch positions = %+v, want three adjacent records", positions)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 3 {
		t.Fatalf("stored batch messages = %d, want 3", info.State.Msgs)
	}
	retry, err := publisher.AppendBatchEventuallyResults(ctx, eventsToAppend)
	if err != nil {
		t.Fatalf("idempotent AppendBatchEventually retry: %v", err)
	}
	for i, result := range retry {
		if result.Committed || result.Position != positions[i] {
			t.Fatalf("retry result %d = %+v, want original position and committed=false", i, result)
		}
	}
	info, err = stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 3 {
		t.Fatalf("stored messages after idempotent retry = %d, want 3", info.State.Msgs)
	}
}

func TestAppendNotificationEventsIndividuallyReturnsCommittedPrefixOnFailure(t *testing.T) {
	wantErr := errors.New("later append failed")
	eventsToAppend := []*notificationv1.NotificationEvent{{Id: "one"}, {Id: "two"}, {Id: "three"}}
	calls := 0
	results, err := appendNotificationEventsIndividually(eventsToAppend, func(event *notificationv1.NotificationEvent) (AppendResult, error) {
		calls++
		if event.GetId() == "three" {
			return AppendResult{}, wantErr
		}
		return AppendResult{Position: events.SubjectPosition(ReadSubject, uint64(calls)), Committed: true}, nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if calls != 3 {
		t.Fatalf("append calls = %d, want 3", calls)
	}
	if len(results) != 2 || results[0].Position.Seq != 1 || results[1].Position.Seq != 2 {
		t.Fatalf("partial results = %+v, want committed two-entry prefix", results)
	}
}

func TestAlertResolutionEventIDMakesTerminalOutcomeSingleWinner(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "NOTIFICATIONS_ALERT_RESOLUTION_TEST", Subjects: Subjects(), Storage: jetstream.MemoryStorage, AllowMsgTTL: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher := NewPublisher(js, stream, time.Hour, nil)
	now := time.Now().UTC()
	resolve := func(delivered bool) *notificationv1.NotificationEvent {
		return &notificationv1.NotificationEvent{
			Id: "NE-alert-resolved", RecipientId: "U1", NotificationId: "N1", OccurredAt: timestamppb.New(now), ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			Event: &notificationv1.NotificationEvent_AlertResolved{AlertResolved: &notificationv1.NotificationAlertResolved{Delivered: delivered}},
		}
	}
	first, err := publisher.AppendEventually(ctx, resolve(true))
	if err != nil {
		t.Fatal(err)
	}
	second, err := publisher.AppendEventually(ctx, resolve(false))
	if err != nil {
		t.Fatal(err)
	}
	if first.Seq != second.Seq {
		t.Fatalf("competing resolutions stored at sequences %d and %d", first.Seq, second.Seq)
	}
	stored, err := stream.GetMsg(ctx, first.Seq)
	if err != nil {
		t.Fatal(err)
	}
	var event notificationv1.NotificationEvent
	if err := proto.Unmarshal(stored.Data, &event); err != nil {
		t.Fatal(err)
	}
	if !event.GetAlertResolved().GetDelivered() {
		t.Fatal("duplicate resolution replaced the first terminal outcome")
	}
}
