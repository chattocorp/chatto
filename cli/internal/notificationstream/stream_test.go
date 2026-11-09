package notificationstream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/chattocorp/chatto/pkg/events"
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
		wg.Go(func() {
			<-start
			id := fmt.Sprintf("NE-%d", i)
			_, err := publisher.AppendEventually(ctx, &notificationv1.NotificationEvent{
				Id: id, RecipientId: "U1", NotificationId: id, OccurredAt: timestamppb.New(now), ExpiresAt: timestamppb.New(now.Add(time.Hour)),
				Event: &notificationv1.NotificationEvent_Read{Read: &notificationv1.NotificationRead{}},
			})
			errs <- err
		})
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
	// Lifecycle facts publish without an OCC guard, so NATS stores no guard
	// headers with them.
	for _, position := range positions {
		stored, err := stream.GetMsg(ctx, position.Seq)
		if err != nil {
			t.Fatal(err)
		}
		for key := range stored.Header {
			if strings.HasPrefix(key, "Nats-Expected-") {
				t.Fatalf("lifecycle fact at seq %d stores guard header %s", position.Seq, key)
			}
		}
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

func TestPublisherGuardsSourceAcrossConcurrentPublishers(t *testing.T) {
	ns, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "NOTIFICATIONS_SOURCE_TEST", Subjects: Subjects(), Storage: jetstream.FileStorage,
		Duplicates: FanoutDedupeWindow, AllowMsgTTL: true, AllowAtomicPublish: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	signal := func(id, recipient, source string) *notificationv1.NotificationEvent {
		return &notificationv1.NotificationEvent{
			Id: id, RecipientId: recipient, NotificationId: id,
			OccurredAt: timestamppb.New(now), ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			Event: &notificationv1.NotificationEvent_Signalled{Signalled: &notificationv1.NotificationSignalled{SourceEventId: source}},
		}
	}
	// Separate connections and publisher instances share only the broker guard.
	nc2, err := nats.Connect(nats.DefaultURL, nats.InProcessServer(ns))
	if err != nil {
		t.Fatal(err)
	}
	defer nc2.Close()
	js2, err := jetstream.New(nc2)
	if err != nil {
		t.Fatal(err)
	}
	publishers := []*Publisher{NewPublisher(js, stream, time.Hour, nil), NewPublisher(js2, stream, time.Hour, nil)}
	type outcome struct {
		results []AppendResult
		err     error
	}
	outcomes := make(chan outcome, 2)
	start := make(chan struct{})
	for i, publisher := range publishers {
		go func() {
			<-start
			results, err := publisher.AppendBatchEventuallyResults(ctx, []*notificationv1.NotificationEvent{signal(fmt.Sprintf("reason-%d", i), "U1", "E1")})
			outcomes <- outcome{results, err}
		}()
	}
	close(start)
	committed := 0
	var sequence uint64
	for range publishers {
		result := <-outcomes
		if result.err != nil || len(result.results) != 1 {
			t.Fatalf("publish = %+v", result)
		}
		if result.results[0].Committed {
			committed++
		}
		if sequence != 0 && sequence != result.results[0].Position.Seq {
			t.Fatal("duplicate did not reuse stored source sequence")
		}
		sequence = result.results[0].Position.Seq
	}
	if committed != 1 {
		t.Fatalf("committed = %d, want 1", committed)
	}
	// A new publisher models a backend restart. Different source and recipient
	// coordinates must remain independent of the still-active duplicate window.
	restarted := NewPublisher(js2, stream, time.Hour, nil)
	for _, tc := range []struct {
		id, user, source string
		created          bool
	}{
		{"reason-after-restart", "U1", "E1", false},
		{"distinct-reply", "U1", "E2", true},
		{"distinct-recipient", "U2", "E1", true},
	} {
		results, err := restarted.AppendBatchEventuallyResults(ctx, []*notificationv1.NotificationEvent{signal(tc.id, tc.user, tc.source)})
		if err != nil || len(results) != 1 || results[0].Committed != tc.created {
			t.Fatalf("%s: results=%v err=%v", tc.id, results, err)
		}
	}
	info, err := stream.Info(ctx)
	if err != nil || info.State.Msgs != 3 {
		t.Fatalf("stored signals = %v, %v; want 3", info, err)
	}
}
