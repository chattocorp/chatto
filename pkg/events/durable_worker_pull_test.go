package events_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/chattocorp/chatto/pkg/events"
)

// These tests check the pull rule of DurableWorker.Run: a pull is long only
// after a pull expired empty and while no delivery is active. They observe
// each pull request on its JetStream API subject, so the order of pulls is a
// fact of the test, not an assumption about timing.

const pullTestFetchMaxWait = 50 * time.Millisecond

func TestDurableWorkerIdlePullIsLong(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "pull-idle", time.Minute)
	pulls := recordDurableWorkerPulls(t, js, "pull-idle", nil)
	worker := newPullTestWorker(t, consumer, 2, func(context.Context, events.DurableDelivery) error {
		t.Error("idle worker unexpectedly received a delivery")
		return nil
	})
	cancel, runErr := startPullTestWorker(t, ctx, worker)

	if first := nextDurableWorkerPull(t, pulls); first.long() || first.Heartbeat != 0 || first.Batch != 1 {
		t.Fatalf("first pull = %+v, want a short pull without heartbeat", first)
	}
	// FetchContext gives the server the remaining context lifetime minus at
	// most one second, so a 30-second idle pull expires after almost 29 seconds.
	second := nextDurableWorkerPull(t, pulls)
	if second.Expires <= 28*time.Second || second.Expires > 29*time.Second ||
		second.Heartbeat <= 9*time.Second || second.Heartbeat > 10*time.Second || second.Batch != 1 {
		t.Fatalf("pull after an empty pull = %+v, want about 29s expiry, 10s heartbeat, batch 1", second)
	}
	stopPullTestWorker(t, cancel, runErr)
	select {
	case pull := <-pulls:
		t.Fatalf("idle worker sent another pull before its idle pull expired: %+v", pull)
	default:
	}
}

// TestDurableWorkerBacklogUsesOnlyShortPulls checks that a backlog never
// selects a long pull. Only an idle worker uses long pulls (see Run).
func TestDurableWorkerBacklogUsesOnlyShortPulls(t *testing.T) {
	for _, maxConcurrent := range []int{1, 4} {
		t.Run(fmt.Sprintf("max concurrent %d", maxConcurrent), func(t *testing.T) {
			js, stream := setupTestStream(t)
			ctx := testContext(t)
			const backlog = 50
			for i := range backlog {
				if _, err := js.Publish(ctx, "evt.worker.backlog", fmt.Appendf(nil, "%d", i)); err != nil {
					t.Fatalf("publish: %v", err)
				}
			}
			consumer := createDurableWorkerTestConsumer(t, ctx, stream, "pull-backlog", time.Minute)
			var handled atomic.Int32
			pulls := recordDurableWorkerPulls(t, js, "pull-backlog", nil)
			worker := newPullTestWorker(t, consumer, maxConcurrent, func(context.Context, events.DurableDelivery) error {
				handled.Add(1)
				return nil
			})
			cancel, runErr := startPullTestWorker(t, ctx, worker)

			before := waitForLongDurableWorkerPull(t, pulls)
			if got := handled.Load(); got != backlog {
				t.Fatalf("handled = %d before the first long pull, want %d", got, backlog)
			}
			if len(before) <= backlog {
				t.Fatalf("pulls before the first long pull = %d, want more than %d", len(before), backlog)
			}
			for i, pull := range before {
				if pull.long() || pull.Heartbeat != 0 {
					t.Fatalf("pull %d during the backlog = %+v, want a short pull without heartbeat", i, pull)
				}
			}
			stopPullTestWorker(t, cancel, runErr)
		})
	}
}

func TestDurableWorkerIdlePullHonorsMaxRequestExpires(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer, err := stream.CreateOrUpdateConsumer(ctx, pullTestConsumerConfig("pull-max-expires", 5*time.Second))
	if err != nil {
		t.Fatalf("create worker consumer: %v", err)
	}
	pulls := recordDurableWorkerPulls(t, js, "pull-max-expires", nil)
	handled := make(chan string, 1)
	worker := newPullTestWorker(t, consumer, 1, func(_ context.Context, delivery events.DurableDelivery) error {
		handled <- string(delivery.Data)
		return nil
	})
	cancel, runErr := startPullTestWorker(t, ctx, worker)

	waitForIdleDurableWorkerPull(t, pulls, time.Second, 5*time.Second)
	publishAndWaitForHandling(t, ctx, js, "evt.worker.max-expires", handled)
	stopPullTestWorker(t, cancel, runErr)
}

// TestDurableWorkerIdlePullFollowsChangedMaxRequestExpires checks that a
// consumer limit that changes after construction does not stop delivery.
// JetStream rejects the long pull; the worker falls back to a short pull and
// takes the new limit from the rejection for its next long pull. The test
// also reads the consumer handle while the worker runs. With nats.go before
// v1.55.0, the race detector finds a worker that refreshes the handle's cached
// information. Later versions synchronize the cache, so the race detector
// cannot find it.
func TestDurableWorkerIdlePullFollowsChangedMaxRequestExpires(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer, err := stream.CreateOrUpdateConsumer(ctx, pullTestConsumerConfig("pull-changed-expires", 0))
	if err != nil {
		t.Fatalf("create worker consumer: %v", err)
	}
	pulls := recordDurableWorkerPulls(t, js, "pull-changed-expires", nil)
	handled := make(chan string, 1)
	logger := &recordingDurableWorkerLogger{}
	worker, err := events.NewDurableWorker(consumer, func(_ context.Context, delivery events.DurableDelivery) error {
		handled <- string(delivery.Data)
		return nil
	}, events.DurableWorkerOptions{
		MaxConcurrent:   1,
		FetchMaxWait:    pullTestFetchMaxWait,
		FetchRetryDelay: time.Millisecond,
		Logger:          slog.New(logger),
	})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}
	// The worker read the consumer without a limit. Change it now.
	if _, err := stream.CreateOrUpdateConsumer(ctx, pullTestConsumerConfig("pull-changed-expires", 2*time.Second)); err != nil {
		t.Fatalf("update worker consumer: %v", err)
	}
	cancel, runErr := startPullTestWorker(t, ctx, worker)
	stopInfo := make(chan struct{})
	infoDone := make(chan struct{})
	go func() {
		defer close(infoDone)
		for {
			select {
			case <-stopInfo:
				return
			default:
			}
			if _, err := consumer.Info(ctx); err != nil {
				t.Errorf("consumer info: %v", err)
				return
			}
			_ = consumer.CachedInfo()
		}
	}()
	defer func() {
		close(stopInfo)
		<-infoDone
	}()

	if pull := nextDurableWorkerPull(t, pulls); pull.long() {
		t.Fatalf("first pull = %+v, want a short pull", pull)
	}
	if pull := nextDurableWorkerPull(t, pulls); pull.Expires <= 10*time.Second {
		t.Fatalf("second pull = %+v, want the long pull from the stale limit", pull)
	}
	if pull := nextDurableWorkerPull(t, pulls); pull.Expires >= time.Second {
		t.Fatalf("pull after the rejected long pull = %+v, want a short pull", pull)
	}
	if pull := nextDurableWorkerPull(t, pulls); pull.Expires <= time.Second || pull.Expires > 2*time.Second {
		t.Fatalf("next idle pull = %+v, want an expiry within the new 2s limit", pull)
	}
	if !logger.containsWarning("Durable work fetch failed; retrying") {
		t.Fatal("rejected long pull was not logged")
	}
	publishAndWaitForHandling(t, ctx, js, "evt.worker.changed-expires", handled)
	stopPullTestWorker(t, cancel, runErr)
}

// TestDurableWorkerRetriesShortAfterHeartbeatFailure checks that a long pull
// that fails, here with a missed heartbeat, is not repeated. The next pull is
// short, and the worker returns to a long pull only after that pull is empty.
func TestDurableWorkerRetriesShortAfterHeartbeatFailure(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "pull-heartbeat", time.Minute)
	pulls := recordDurableWorkerPulls(t, js, "pull-heartbeat", nil)
	var calls atomic.Int32
	controlled := &controlledConsumer{Consumer: consumer, next: func(next func() (jetstream.Msg, error)) (jetstream.Msg, error) {
		// The second call follows an empty first pull, so it is the first long
		// pull. Fail it without sending it.
		if calls.Add(1) == 2 {
			return nil, jetstream.ErrNoHeartbeat
		}
		return next()
	}}
	logger := &recordingDurableWorkerLogger{}
	worker, err := events.NewDurableWorker(controlled, func(context.Context, events.DurableDelivery) error {
		t.Error("idle worker unexpectedly received a delivery")
		return nil
	}, events.DurableWorkerOptions{
		MaxConcurrent:   1,
		FetchMaxWait:    pullTestFetchMaxWait,
		FetchRetryDelay: time.Millisecond,
		Logger:          slog.New(logger),
	})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}
	cancel, runErr := startPullTestWorker(t, ctx, worker)

	if pull := nextDurableWorkerPull(t, pulls); pull.long() {
		t.Fatalf("first pull = %+v, want a short pull", pull)
	}
	if pull := nextDurableWorkerPull(t, pulls); pull.long() {
		t.Fatalf("pull after the failed long pull = %+v, want a short pull", pull)
	}
	if pull := nextDurableWorkerPull(t, pulls); !pull.long() {
		t.Fatalf("pull after an empty short pull = %+v, want a long pull", pull)
	}
	if !logger.containsWarning("Durable work fetch failed; retrying") {
		t.Fatal("heartbeat failure was not logged")
	}
	stopPullTestWorker(t, cancel, runErr)
}

// TestDurableWorkerLongPullWaitsForAllHandlers checks the handoff invariant
// with several handlers that finish in different orders: the worker never
// starts a long pull while a handler is active.
func TestDurableWorkerLongPullWaitsForAllHandlers(t *testing.T) {
	for _, order := range [][]int{{0, 1, 2}, {2, 0, 1}, {1, 2, 0}} {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			js, stream := setupTestStream(t)
			ctx := testContext(t)
			releases := make([]chan struct{}, 3)
			for i := range releases {
				releases[i] = make(chan struct{})
				if _, err := js.Publish(ctx, "evt.worker.order", fmt.Appendf(nil, "%d", i)); err != nil {
					t.Fatalf("publish: %v", err)
				}
			}
			consumer := createDurableWorkerTestConsumer(t, ctx, stream, "pull-order", time.Minute)
			// Each handler leaves this count before it returns, so before its
			// slot is free. The count cannot increase after the three deliveries,
			// so a long pull with a positive count breaks the invariant.
			var activeHandlers atomic.Int32
			activeHandlers.Store(3)
			pulls := recordDurableWorkerPulls(t, js, "pull-order", func(pull durableWorkerPull) {
				if pull.long() && activeHandlers.Load() != 0 {
					t.Errorf("long pull %+v while %d handlers are active", pull, activeHandlers.Load())
				}
			})
			started := make(chan struct{}, 3)
			worker := newPullTestWorker(t, consumer, 3, func(_ context.Context, delivery events.DurableDelivery) error {
				var index int
				if _, err := fmt.Sscan(string(delivery.Data), &index); err != nil {
					t.Errorf("payload %q: %v", delivery.Data, err)
					return nil
				}
				started <- struct{}{}
				<-releases[index]
				activeHandlers.Add(-1)
				return nil
			})
			cancel, runErr := startPullTestWorker(t, ctx, worker)
			for range 3 {
				<-started
			}
			// The three pulls that delivered the work. All slots are now full, so
			// the worker sends no other pull until a handler finishes.
			for range 3 {
				if pull := nextDurableWorkerPull(t, pulls); pull.long() {
					t.Fatalf("delivering pull = %+v, want a short pull", pull)
				}
			}

			for i, index := range order {
				close(releases[index])
				if i == len(order)-1 {
					break
				}
				// The freed slot starts a pull while other handlers are active.
				if pull := nextDurableWorkerPull(t, pulls); pull.long() {
					t.Fatalf("pull after release %d = %+v, want a short pull", i, pull)
				}
			}
			for _, pull := range waitForLongDurableWorkerPull(t, pulls) {
				if pull.long() {
					t.Fatalf("unexpected long pull %+v", pull)
				}
			}
			stopPullTestWorker(t, cancel, runErr)
		})
	}
}

// TestDurableWorkerLastHandlerFinishesDuringShortPull checks the transition
// from active back to idle: the last handler finishes while a short pull is
// open, and the worker then returns to a long pull.
func TestDurableWorkerLastHandlerFinishesDuringShortPull(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "pull-last-handler", time.Minute)
	// secondPullSent closes when the server receives the second pull, which
	// starts while the handler is active.
	secondPullSent := make(chan struct{})
	var observed atomic.Int32
	pulls := recordDurableWorkerPulls(t, js, "pull-last-handler", func(durableWorkerPull) {
		if observed.Add(1) == 2 {
			close(secondPullSent)
		}
	})
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var calls atomic.Int32
	controlled := &controlledConsumer{Consumer: consumer, next: func(next func() (jetstream.Msg, error)) (jetstream.Msg, error) {
		if calls.Add(1) != 2 {
			return next()
		}
		// Send the second pull, let the handler finish while that pull is
		// open, and only then let Run see the result of the pull.
		result := make(chan pullResult, 1)
		go func() {
			msg, err := next()
			result <- pullResult{msg: msg, err: err}
		}()
		<-secondPullSent
		close(release)
		<-finished
		got := <-result
		return got.msg, got.err
	}}
	worker := newPullTestWorker(t, controlled, 2, func(context.Context, events.DurableDelivery) error {
		close(started)
		<-release
		close(finished)
		return nil
	})
	if _, err := js.Publish(ctx, "evt.worker.last-handler", []byte("work")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	cancel, runErr := startPullTestWorker(t, ctx, worker)
	<-started

	if pull := nextDurableWorkerPull(t, pulls); pull.long() {
		t.Fatalf("first pull = %+v, want a short pull", pull)
	}
	if pull := nextDurableWorkerPull(t, pulls); pull.long() {
		t.Fatalf("pull while the handler is active = %+v, want a short pull", pull)
	}
	waitForLongDurableWorkerPull(t, pulls)
	waitForDurableWorkerConsumerSettled(t, ctx, consumer)
	stopPullTestWorker(t, cancel, runErr)
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
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "pull-handoff", time.Minute)
	pulls := recordDurableWorkerPulls(t, js, "pull-handoff", nil)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	worker := newPullTestWorker(t, consumer, 2, func(context.Context, events.DurableDelivery) error {
		if calls.Add(1) != 1 {
			t.Error("stopping worker received its handoff again")
			return nil
		}
		close(started)
		<-release
		return nil
	})
	cancel, runErr := startPullTestWorker(t, ctx, worker)

	waitForLongDurableWorkerPull(t, pulls)
	if _, err := js.Publish(ctx, "evt.worker.handoff", []byte("blocked")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	<-started
	// Wait for the next pull, so that the open pull at cancellation is the
	// pull that started while the handler was active.
	if pull := nextDurableWorkerPull(t, pulls); pull.long() {
		t.Fatalf("pull with an active delivery = %+v, want a short pull", pull)
	}
	cancel()
	expectPromptRedelivery(t, ctx, consumer, "blocked")
	close(release)
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// TestDurableWorkerCancellationWhenLongPullDelivers checks cancellation that
// arrives together with a delivery from a long pull. The worker must not start
// the handler and must give the delivery back at once.
func TestDurableWorkerCancellationWhenLongPullDelivers(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "pull-cancel-delivery", time.Minute)
	pulls := recordDurableWorkerPulls(t, js, "pull-cancel-delivery", nil)
	var cancelWorker context.CancelFunc
	cancelReady := make(chan struct{})
	controlled := &controlledConsumer{Consumer: consumer, next: func(next func() (jetstream.Msg, error)) (jetstream.Msg, error) {
		msg, err := next()
		if msg != nil {
			<-cancelReady
			cancelWorker()
		}
		return msg, err
	}}
	worker := newPullTestWorker(t, controlled, 1, func(context.Context, events.DurableDelivery) error {
		t.Error("worker started a handler after cancellation")
		return nil
	})
	cancel, runErr := startPullTestWorker(t, ctx, worker)
	cancelWorker = cancel
	close(cancelReady)

	waitForLongDurableWorkerPull(t, pulls)
	if _, err := js.Publish(ctx, "evt.worker.cancel-delivery", []byte("late")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}
	expectPromptRedelivery(t, ctx, consumer, "late")
}

// TestDurableWorkerCancellationAtHandlerStart checks cancellation that arrives
// when the handler starts. The handler reports success only after the
// cancellation; the worker must still hand the delivery off and not
// acknowledge it.
func TestDurableWorkerCancellationAtHandlerStart(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	consumer := createDurableWorkerTestConsumer(t, ctx, stream, "pull-cancel-start", time.Minute)
	pulls := recordDurableWorkerPulls(t, js, "pull-cancel-start", nil)
	started := make(chan struct{})
	worker := newPullTestWorker(t, consumer, 1, func(handlerCtx context.Context, _ events.DurableDelivery) error {
		close(started)
		<-handlerCtx.Done()
		return nil
	})
	cancel, runErr := startPullTestWorker(t, ctx, worker)

	waitForLongDurableWorkerPull(t, pulls)
	if _, err := js.Publish(ctx, "evt.worker.cancel-start", []byte("starting")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	<-started
	cancel()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}
	expectPromptRedelivery(t, ctx, consumer, "starting")
}

type durableWorkerPull struct {
	Batch     int           `json:"batch"`
	Expires   time.Duration `json:"expires"`
	Heartbeat time.Duration `json:"idle_heartbeat"`
}

// long reports a long idle pull. Short pulls in these tests last
// pullTestFetchMaxWait.
func (p durableWorkerPull) long() bool { return p.Expires >= time.Second }

type pullResult struct {
	msg jetstream.Msg
	err error
}

// controlledConsumer lets a test act around each Next call of a worker. next
// receives the real call and decides when to make it.
type controlledConsumer struct {
	jetstream.Consumer
	next func(next func() (jetstream.Msg, error)) (jetstream.Msg, error)
}

func (c *controlledConsumer) Next(opts ...jetstream.FetchOpt) (jetstream.Msg, error) {
	return c.next(func() (jetstream.Msg, error) { return c.Consumer.Next(opts...) })
}

func pullTestConsumerConfig(name string, maxRequestExpires time.Duration) jetstream.ConsumerConfig {
	return jetstream.ConsumerConfig{
		Durable:           name,
		AckPolicy:         jetstream.AckExplicitPolicy,
		AckWait:           time.Minute,
		FilterSubject:     "evt.worker.>",
		MaxAckPending:     8,
		MaxRequestExpires: maxRequestExpires,
	}
}

func newPullTestWorker(t *testing.T, consumer jetstream.Consumer, maxConcurrent int, handle events.DurableDeliveryHandler) *events.DurableWorker {
	t.Helper()
	worker, err := events.NewDurableWorker(consumer, handle, events.DurableWorkerOptions{
		MaxConcurrent:     maxConcurrent,
		FetchMaxWait:      pullTestFetchMaxWait,
		FetchRetryDelay:   time.Millisecond,
		HeartbeatInterval: time.Minute,
		Logger:            testLogger(),
	})
	if err != nil {
		t.Fatalf("NewDurableWorker: %v", err)
	}
	return worker
}

// startPullTestWorker runs the worker without the test deadline, which would
// shorten its pulls.
func startPullTestWorker(t *testing.T, ctx context.Context, worker *events.DurableWorker) (context.CancelFunc, <-chan error) {
	t.Helper()
	workerCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	t.Cleanup(cancel)
	runErr := make(chan error, 1)
	go func() { runErr <- worker.Run(workerCtx) }()
	return cancel, runErr
}

func stopPullTestWorker(t *testing.T, cancel context.CancelFunc, runErr <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
}

// recordDurableWorkerPulls observes the pull requests that clients send for
// one consumer of the test stream. check, when set, runs as each request is
// observed.
func recordDurableWorkerPulls(t *testing.T, js jetstream.JetStream, consumerName string, check func(durableWorkerPull)) <-chan durableWorkerPull {
	t.Helper()
	pulls := make(chan durableWorkerPull, 1024)
	subscription, err := js.Conn().Subscribe("$JS.API.CONSUMER.MSG.NEXT.EVENTS_TEST."+consumerName, func(msg *nats.Msg) {
		var pull durableWorkerPull
		if err := json.Unmarshal(msg.Data, &pull); err != nil {
			t.Errorf("decode pull request %q: %v", msg.Data, err)
			return
		}
		if check != nil {
			check(pull)
		}
		select {
		case pulls <- pull:
		default:
			t.Error("too many pull requests to record")
		}
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

// waitForLongDurableWorkerPull reads pull requests until a long one and
// returns the pulls before it.
func waitForLongDurableWorkerPull(t *testing.T, pulls <-chan durableWorkerPull) []durableWorkerPull {
	t.Helper()
	var before []durableWorkerPull
	for {
		pull := nextDurableWorkerPull(t, pulls)
		if pull.long() {
			return before
		}
		before = append(before, pull)
	}
}

// waitForIdleDurableWorkerPull reads pull requests until a pull longer than
// FetchMaxWait and checks that its expiry is within (minimum, maximum].
func waitForIdleDurableWorkerPull(t *testing.T, pulls <-chan durableWorkerPull, minimum, maximum time.Duration) {
	t.Helper()
	for {
		pull := nextDurableWorkerPull(t, pulls)
		if pull.Expires <= pullTestFetchMaxWait {
			continue
		}
		if pull.Expires <= minimum || pull.Expires > maximum {
			t.Fatalf("idle pull = %+v, want an expiry in (%v, %v]", pull, minimum, maximum)
		}
		return
	}
}

func publishAndWaitForHandling(t *testing.T, ctx context.Context, js jetstream.JetStream, subject string, handled <-chan string) {
	t.Helper()
	if _, err := js.Publish(ctx, subject, []byte("work")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case got := <-handled:
		if got != "work" {
			t.Fatalf("handled = %q", got)
		}
	case <-ctx.Done():
		t.Fatal("worker did not receive work")
	}
}

// expectPromptRedelivery fetches the delivery that a stopped worker handed
// off. The consumer's ack wait is one minute, so only a handoff can deliver it
// within the fetch wait. It must be the second attempt.
func expectPromptRedelivery(t *testing.T, ctx context.Context, consumer jetstream.Consumer, want string) {
	t.Helper()
	redelivery := fetchDurableWorkerTestMessage(t, consumer)
	if got := string(redelivery.Data()); got != want {
		t.Fatalf("redelivery data = %q, want %q", got, want)
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
}
