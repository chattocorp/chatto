package events_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/pkg/events"
)

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
	}, events.DurableWorkerOptions{MaxConcurrent: 1, FetchMaxWait: 20 * time.Millisecond, Logger: testLogger()})
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
	if counts["poison"] != 1 || counts["valid"] != 1 {
		t.Fatalf("handled counts = %v", counts)
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

func waitForDurableWorkerConsumerSettled(t *testing.T, ctx context.Context, consumer jetstream.Consumer) {
	t.Helper()
	waitFor(t, 5*time.Second, func() bool {
		info, err := consumer.Info(ctx)
		return err == nil && info.NumPending == 0 && info.NumAckPending == 0
	})
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
