package events_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/chattocorp/chatto/pkg/events"
)

type recordingDurableWorkerLogger struct {
	mu       sync.Mutex
	warnings []string
	errors   []string
}

func (*recordingDurableWorkerLogger) Enabled(context.Context, slog.Level) bool { return true }

func (l *recordingDurableWorkerLogger) Handle(_ context.Context, record slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch record.Level {
	case slog.LevelWarn:
		l.warnings = append(l.warnings, record.Message)
	case slog.LevelError:
		l.errors = append(l.errors, record.Message)
	}
	return nil
}

func (l *recordingDurableWorkerLogger) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *recordingDurableWorkerLogger) WithGroup(string) slog.Handler      { return l }

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

func TestDurableWorkerRejectsRepeatedRun(t *testing.T) {
	_, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "worker-single-run", time.Second)
	worker, err := events.NewDurableWorker(consumer, func(context.Context, events.DurableDelivery) error {
		return nil
	}, events.DurableWorkerOptions{MaxConcurrent: 1, FetchMaxWait: time.Second, Logger: testLogger()})
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
	if err := worker.Run(context.Background()); !errors.Is(err, events.ErrDurableWorkerAlreadyStarted) {
		t.Fatalf("repeated worker Run error = %v, want ErrDurableWorkerAlreadyStarted", err)
	}
	cancel()
	if err := <-runErr; err != nil {
		t.Fatalf("first worker Run: %v", err)
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
	}, events.DurableWorkerOptions{MaxConcurrent: 1, FetchMaxWait: 20 * time.Millisecond, Logger: slog.New(logger)})
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
	}, events.DurableWorkerOptions{MaxConcurrent: 2, Logger: slog.New(logger)})
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

func TestDurableWorkerIdlePullIsLong(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "worker-idle-long", time.Second)
	pulls := recordDurableWorkerPulls(t, js, "worker-idle-long")
	worker, err := events.NewDurableWorker(consumer, func(context.Context, events.DurableDelivery) error {
		t.Error("idle worker unexpectedly received a delivery")
		return nil
	}, events.DurableWorkerOptions{MaxConcurrent: 2, FetchMaxWait: 20 * time.Millisecond, Logger: testLogger()})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}
	// The test context has a short deadline that would shorten the pull.
	workerCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	t.Cleanup(cancel)
	runErr := make(chan error, 1)
	go func() { runErr <- worker.Run(workerCtx) }()

	pull := nextDurableWorkerPull(t, pulls)
	// FetchContext gives the server the remaining context lifetime minus at
	// most one second, so a 30-second idle pull expires after almost 29 seconds.
	if pull.Expires <= 28*time.Second || pull.Expires > 29*time.Second ||
		pull.Heartbeat <= 9*time.Second || pull.Heartbeat > 10*time.Second || pull.Batch != 1 {
		t.Fatalf("idle pull = %+v, want about 29s expiry, 10s heartbeat, batch 1", pull)
	}
	cancel()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case pull := <-pulls:
		t.Fatalf("idle worker sent another pull before its idle pull expired: %+v", pull)
	default:
	}
}

// TestDurableWorkerIdlePullHonorsMaxRequestExpires checks that the idle pull
// stays within a consumer's MaxRequestExpires. JetStream rejects a longer pull,
// so the worker would otherwise never receive work.
func TestDurableWorkerIdlePullHonorsMaxRequestExpires(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:           "worker-max-expires",
		AckPolicy:         jetstream.AckExplicitPolicy,
		AckWait:           time.Minute,
		FilterSubject:     "evt.worker.>",
		MaxRequestExpires: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("create worker consumer: %v", err)
	}
	pulls := recordDurableWorkerPulls(t, js, "worker-max-expires")
	handled := make(chan string, 1)
	worker, err := events.NewDurableWorker(consumer, func(_ context.Context, delivery events.DurableDelivery) error {
		handled <- string(delivery.Data)
		return nil
	}, events.DurableWorkerOptions{MaxConcurrent: 1, Logger: testLogger()})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}
	// The test context has a short deadline that would shorten the pull.
	workerCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	t.Cleanup(cancel)
	runErr := make(chan error, 1)
	go func() { runErr <- worker.Run(workerCtx) }()

	if pull := nextDurableWorkerPull(t, pulls); pull.Expires <= time.Second || pull.Expires > 5*time.Second {
		t.Fatalf("idle pull = %+v, want an expiry within MaxRequestExpires", pull)
	}
	if _, err := js.Publish(ctx, "evt.worker.max-expires", []byte("work")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case got := <-handled:
		if got != "work" {
			t.Fatalf("handled = %q", got)
		}
	case <-ctx.Done():
		t.Fatal("worker did not receive work through its idle pull")
	}
	cancel()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// TestDurableWorkerHandsOffPromptlyAfterIdlePull checks that the long idle pull
// does not extend the shutdown handoff delay. The delivery arrives through the
// long pull. While its handler is active, the worker starts only short pulls, so
// cancellation hands the delivery off after FetchMaxWait plus a margin, not after
// the idle pull lifetime.
func TestDurableWorkerHandsOffPromptlyAfterIdlePull(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	// The ack wait is longer than the test deadline. Only the handoff can
	// cause the redelivery.
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "worker-idle-handoff", time.Minute)
	pulls := recordDurableWorkerPulls(t, js, "worker-idle-handoff")

	var calls sync.Map
	started := make(chan struct{})
	release := make(chan struct{})
	worker, err := events.NewDurableWorker(consumer, func(_ context.Context, delivery events.DurableDelivery) error {
		if _, loaded := calls.LoadOrStore(delivery.StreamSequence, true); loaded {
			t.Errorf("stopping worker received stream sequence %d again", delivery.StreamSequence)
			return nil
		}
		close(started)
		<-release
		return nil
	}, events.DurableWorkerOptions{
		MaxConcurrent:     2,
		FetchMaxWait:      200 * time.Millisecond,
		HeartbeatInterval: time.Minute,
		Logger:            testLogger(),
	})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}
	// The test context has a short deadline that would shorten the pull.
	workerCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	t.Cleanup(cancel)
	runErr := make(chan error, 1)
	go func() { runErr <- worker.Run(workerCtx) }()

	if pull := nextDurableWorkerPull(t, pulls); pull.Expires < 10*time.Second {
		t.Fatalf("first pull of an idle worker = %+v, want a long pull", pull)
	}
	if _, err := js.Publish(ctx, "evt.worker.idle-handoff", []byte("blocked")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	<-started
	// Every pull after the delivery starts while the handler is active. Wait
	// for one so that the outstanding pull at cancellation is a short pull.
	if pull := nextDurableWorkerPull(t, pulls); pull.Expires >= time.Second {
		t.Fatalf("pull with an active delivery = %+v, want a short pull", pull)
	}
	cancel()

	handoffStarted := time.Now()
	redelivery := fetchDurableWorkerTestMessage(t, consumer)
	if elapsed := time.Since(handoffStarted); elapsed >= 5*time.Second {
		t.Fatalf("handoff redelivery took %v", elapsed)
	}
	if got := string(redelivery.Data()); got != "blocked" {
		t.Fatalf("redelivery data = %q", got)
	}
	metadata, err := redelivery.Metadata()
	if err != nil {
		t.Fatalf("redelivery metadata: %v", err)
	}
	if metadata.NumDelivered != 2 {
		t.Fatalf("redelivery attempt = %d, want 2", metadata.NumDelivered)
	}
	if err := redelivery.DoubleAck(ctx); err != nil {
		t.Fatalf("acknowledge redelivery: %v", err)
	}
	close(release)
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

type durableWorkerPull struct {
	Batch     int           `json:"batch"`
	Expires   time.Duration `json:"expires"`
	Heartbeat time.Duration `json:"idle_heartbeat"`
}

// recordDurableWorkerPulls observes the pull requests that clients send for
// one consumer of the test stream.
func recordDurableWorkerPulls(t *testing.T, js jetstream.JetStream, consumerName string) <-chan durableWorkerPull {
	t.Helper()
	pulls := make(chan durableWorkerPull, 64)
	subscription, err := js.Conn().Subscribe("$JS.API.CONSUMER.MSG.NEXT.EVENTS_TEST."+consumerName, func(msg *nats.Msg) {
		var pull durableWorkerPull
		if err := json.Unmarshal(msg.Data, &pull); err != nil {
			t.Errorf("decode pull request %q: %v", msg.Data, err)
			return
		}
		pulls <- pull
	})
	if err != nil {
		t.Fatalf("subscribe to pull requests: %v", err)
	}
	if err := js.Conn().Flush(); err != nil {
		t.Fatalf("flush pull request subscription: %v", err)
	}
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	return pulls
}

func nextDurableWorkerPull(t *testing.T, pulls <-chan durableWorkerPull) durableWorkerPull {
	t.Helper()
	select {
	case pull := <-pulls:
		return pull
	case <-time.After(5 * time.Second):
		t.Fatal("worker sent no pull request")
		return durableWorkerPull{}
	}
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

func TestTerminationReason(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		reason string
		ok     bool
	}{
		{name: "nil", err: nil},
		{name: "plain", err: errors.New("temporary")},
		{name: "retry", err: events.RetryDeliveryAfter(errors.New("later"), time.Second)},
		{name: "terminated", err: events.TerminateDelivery("invalid input", errors.New("bad")), reason: "invalid input", ok: true},
		{name: "wrapped", err: fmt.Errorf("context: %w", events.TerminateDelivery("invalid input", nil)), reason: "invalid input", ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, ok := events.TerminationReason(tt.err)
			if reason != tt.reason || ok != tt.ok {
				t.Fatalf("TerminationReason = %q, %v; want %q, %v", reason, ok, tt.reason, tt.ok)
			}
		})
	}
}
