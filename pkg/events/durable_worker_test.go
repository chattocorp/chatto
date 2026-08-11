package events_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/pkg/events"
)

type recordingDurableWorkerLogger struct {
	mu       sync.Mutex
	warnings []string
	errors   []string
}

func (*recordingDurableWorkerLogger) Debug(interface{}, ...interface{}) {}
func (*recordingDurableWorkerLogger) Info(interface{}, ...interface{})  {}
func (l *recordingDurableWorkerLogger) Warn(message interface{}, _ ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnings = append(l.warnings, fmt.Sprint(message))
}
func (l *recordingDurableWorkerLogger) Error(message interface{}, _ ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, fmt.Sprint(message))
}

func (l *recordingDurableWorkerLogger) containsWarning(message string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Contains(l.warnings, message)
}

func (l *recordingDurableWorkerLogger) containsError(message string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Contains(l.errors, message)
}

func TestDurableWorkerProcessesOpaqueDeliveriesAndAcknowledges(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "worker-ack", time.Second)
	for _, payload := range []string{"first", "second"} {
		if _, err := js.Publish(ctx, "evt.worker.ack", []byte(payload)); err != nil {
			t.Fatalf("publish %s: %v", payload, err)
		}
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	handledCh := make(chan struct{}, 2)
	var mu sync.Mutex
	var handled []string
	worker, err := events.NewDurableWorker(consumer, func(_ context.Context, delivery events.DurableDelivery) error {
		if delivery.Subject != "evt.worker.ack" || delivery.StreamSequence == 0 || delivery.PublishedAt.IsZero() || delivery.NumDelivered == 0 {
			t.Errorf("delivery metadata = %+v", delivery)
		}
		mu.Lock()
		handled = append(handled, string(delivery.Data))
		mu.Unlock()
		handledCh <- struct{}{}
		return nil
	}, events.DurableWorkerOptions{MaxConcurrent: 2, Logger: testLogger()})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}
	runErr := make(chan error, 1)
	go func() { runErr <- worker.Run(workerCtx) }()
	for range 2 {
		<-handledCh
	}
	waitForDurableWorkerConsumerSettled(t, ctx, consumer)
	cancel()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	slices.Sort(handled)
	if !slices.Equal(handled, []string{"first", "second"}) {
		t.Fatalf("handled = %v", handled)
	}
}

func TestDurableWorkerRetriesFailedDelivery(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "worker-retry", time.Second)
	if _, err := js.Publish(ctx, "evt.worker.retry", []byte("retry")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	completed := make(chan struct{})
	var mu sync.Mutex
	var deliveries []uint64
	logger := &recordingDurableWorkerLogger{}
	worker, err := events.NewDurableWorker(consumer, func(_ context.Context, delivery events.DurableDelivery) error {
		mu.Lock()
		deliveries = append(deliveries, delivery.NumDelivered)
		attempt := len(deliveries)
		mu.Unlock()
		if attempt == 1 {
			return events.RetryDeliveryAfter(errors.New("temporarily unavailable"), 10*time.Millisecond)
		}
		close(completed)
		return nil
	}, events.DurableWorkerOptions{MaxConcurrent: 1, FetchMaxWait: 20 * time.Millisecond, Logger: logger})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}
	runErr := make(chan error, 1)
	go func() { runErr <- worker.Run(workerCtx) }()
	<-completed
	waitForDurableWorkerConsumerSettled(t, ctx, consumer)
	cancel()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(deliveries) != 2 || deliveries[0] != 1 || deliveries[1] < 2 {
		t.Fatalf("delivery attempts = %v, want first and redelivery", deliveries)
	}
	if !logger.containsWarning("Durable delivery failed; retrying") {
		t.Fatal("retryable handler failure was not logged")
	}
}

func TestDurableWorkerTerminatesPoisonDeliveryAndContinues(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "worker-term", time.Second)
	for _, payload := range []string{"poison", "valid"} {
		if _, err := js.Publish(ctx, "evt.worker.term", []byte(payload)); err != nil {
			t.Fatalf("publish %s: %v", payload, err)
		}
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	handledCh := make(chan struct{}, 2)
	var mu sync.Mutex
	counts := map[string]int{}
	logger := &recordingDurableWorkerLogger{}
	worker, err := events.NewDurableWorker(consumer, func(_ context.Context, delivery events.DurableDelivery) error {
		payload := string(delivery.Data)
		mu.Lock()
		counts[payload]++
		mu.Unlock()
		handledCh <- struct{}{}
		if payload == "poison" {
			return events.TerminateDelivery("unsupported test payload", errors.New("poison input"))
		}
		return nil
	}, events.DurableWorkerOptions{MaxConcurrent: 2, Logger: logger})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}
	runErr := make(chan error, 1)
	go func() { runErr <- worker.Run(workerCtx) }()
	for range 2 {
		<-handledCh
	}
	waitForDurableWorkerConsumerSettled(t, ctx, consumer)
	cancel()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if counts["poison"] != 1 || counts["valid"] != 1 {
		t.Fatalf("handled counts = %v", counts)
	}
	if !logger.containsError("Durable delivery terminated") {
		t.Fatal("terminated delivery was not logged")
	}
}

func TestDurableWorkerReturnsWhenConsumerIsDeleted(t *testing.T) {
	_, stream := setupTestStream(t)
	ctx := testContext(t)
	const consumerName = "worker-deleted"
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, consumerName, time.Second)
	worker, err := events.NewDurableWorker(consumer, func(context.Context, events.DurableDelivery) error {
		t.Fatal("deleted consumer unexpectedly delivered work")
		return nil
	}, events.DurableWorkerOptions{
		MaxConcurrent: 1,
		FetchMaxWait:  time.Second,
		Logger:        testLogger(),
	})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- worker.Run(workerCtx) }()
	waitFor(t, time.Second, func() bool {
		info, err := consumer.Info(ctx)
		return err == nil && info.NumWaiting == 1
	})
	if err := stream.DeleteConsumer(ctx, consumerName); err != nil {
		t.Fatalf("delete consumer: %v", err)
	}

	select {
	case err := <-runErr:
		if !errors.Is(err, jetstream.ErrConsumerDeleted) && !errors.Is(err, jetstream.ErrConsumerNotFound) {
			t.Fatalf("Run error = %v, want deleted consumer error", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker kept retrying after its consumer was deleted")
	}
}

func TestDurableWorkerHeartbeatsLongRunningDelivery(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "worker-heartbeat", 40*time.Millisecond)
	if _, err := js.Publish(ctx, "evt.worker.heartbeat", []byte("slow")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	completed := make(chan struct{})
	var calls int
	var mu sync.Mutex
	worker, err := events.NewDurableWorker(consumer, func(_ context.Context, _ events.DurableDelivery) error {
		mu.Lock()
		calls++
		mu.Unlock()
		time.Sleep(120 * time.Millisecond)
		close(completed)
		return nil
	}, events.DurableWorkerOptions{
		MaxConcurrent:     1,
		FetchMaxWait:      20 * time.Millisecond,
		HeartbeatInterval: 10 * time.Millisecond,
		Logger:            testLogger(),
	})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}
	runErr := make(chan error, 1)
	go func() { runErr <- worker.Run(workerCtx) }()
	<-completed
	waitForDurableWorkerConsumerSettled(t, ctx, consumer)
	cancel()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestDurableWorkerCancellationHandsOffBlockedHandler(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "worker-cancel", 30*time.Second)
	if _, err := js.Publish(ctx, "evt.worker.cancel", []byte("blocked")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	started := make(chan struct{})
	release := make(chan struct{})
	worker, err := events.NewDurableWorker(consumer, func(context.Context, events.DurableDelivery) error {
		close(started)
		<-release
		return nil
	}, events.DurableWorkerOptions{
		MaxConcurrent:     2,
		FetchMaxWait:      20 * time.Millisecond,
		HeartbeatInterval: 10 * time.Millisecond,
		Logger:            testLogger(),
	})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}
	runErr := make(chan error, 1)
	go func() { runErr <- worker.Run(workerCtx) }()
	<-started
	waitFor(t, time.Second, func() bool {
		info, err := consumer.Info(ctx)
		return err == nil && info.NumWaiting == 1
	})
	cancel()
	select {
	case err := <-runErr:
		t.Fatalf("worker abandoned blocked handler with result %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	redelivery := fetchDurableWorkerTestMessage(t, consumer)
	if got := string(redelivery.Data()); got != "blocked" {
		t.Fatalf("redelivery data = %q", got)
	}
	if err := redelivery.DoubleAck(ctx); err != nil {
		t.Fatalf("acknowledge redelivery: %v", err)
	}
	close(release)
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after canceled handler returned")
	}
}

func TestDurableWorkerCancellationReleasesIdleFetch(t *testing.T) {
	_, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "worker-idle-cancel", time.Second)
	worker, err := events.NewDurableWorker(consumer, func(context.Context, events.DurableDelivery) error {
		t.Fatal("idle worker unexpectedly received a delivery")
		return nil
	}, events.DurableWorkerOptions{
		MaxConcurrent: 1,
		FetchMaxWait:  time.Minute,
		Logger:        testLogger(),
	})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	runErr := make(chan error, 1)
	go func() { runErr <- worker.Run(workerCtx) }()
	waitFor(t, time.Second, func() bool {
		info, err := consumer.Info(ctx)
		return err == nil && info.NumWaiting == 1
	})
	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("idle worker did not stop after cancellation")
	}
	waitFor(t, time.Second, func() bool {
		info, err := consumer.Info(ctx)
		return err == nil && info.NumWaiting == 0
	})
}

func TestDurableWorkerReplenishesConcurrencyAroundBlockedDelivery(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "worker-replenish", time.Second)
	for _, payload := range []string{"blocked", "second", "third"} {
		if _, err := js.Publish(ctx, "evt.worker.replenish", []byte(payload)); err != nil {
			t.Fatalf("publish %s: %v", payload, err)
		}
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	release := make(chan struct{})
	started := make(chan string, 3)
	worker, err := events.NewDurableWorker(consumer, func(_ context.Context, delivery events.DurableDelivery) error {
		payload := string(delivery.Data)
		started <- payload
		if payload == "blocked" {
			<-release
		}
		return nil
	}, events.DurableWorkerOptions{MaxConcurrent: 2, FetchMaxWait: 20 * time.Millisecond, Logger: testLogger()})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}
	runErr := make(chan error, 1)
	go func() { runErr <- worker.Run(workerCtx) }()

	seen := map[string]bool{}
	for len(seen) < 3 {
		select {
		case payload := <-started:
			seen[payload] = true
		case <-time.After(time.Second):
			t.Fatalf("later work did not replenish free concurrency; started = %v", seen)
		}
	}
	close(release)
	waitForDurableWorkerConsumerSettled(t, ctx, consumer)
	cancel()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func waitForDurableWorkerConsumerSettled(t *testing.T, ctx context.Context, consumer jetstream.Consumer) {
	t.Helper()
	waitFor(t, 5*time.Second, func() bool {
		info, err := consumer.Info(ctx)
		return err == nil && info.NumPending == 0 && info.NumAckPending == 0
	})
}

func fetchDurableWorkerTestMessage(t *testing.T, consumer jetstream.Consumer) jetstream.Msg {
	t.Helper()
	batch, err := consumer.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
	if err != nil {
		t.Fatalf("fetch delivery: %v", err)
	}
	for msg := range batch.Messages() {
		return msg
	}
	if err := batch.Error(); err != nil {
		t.Fatalf("receive delivery: %v", err)
	}
	t.Fatal("consumer returned no delivery")
	return nil
}

func createDurableWorkerTestConsumer(t *testing.T, ctx context.Context, stream jetstream.Stream, name string, ackWait time.Duration) jetstream.Consumer {
	t.Helper()
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          name,
		Durable:       name,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       ackWait,
		MaxDeliver:    -1,
		FilterSubject: "evt.worker.>",
		ReplayPolicy:  jetstream.ReplayInstantPolicy,
		MaxAckPending: 8,
	})
	if err != nil {
		t.Fatalf("create worker consumer: %v", err)
	}
	return consumer
}
