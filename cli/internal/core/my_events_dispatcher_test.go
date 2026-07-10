package core

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestMyEventsDispatcherUsesConstantSharedSubscriptions(t *testing.T) {
	core, nc := setupTestCore(t)

	select {
	case <-core.myEventsModel.dispatchReady:
	case <-time.After(5 * time.Second):
		t.Fatal("live event dispatcher did not become ready")
	}

	baselineNATSSubscriptions := nc.NumSubscriptions()
	core.PresenceHub.mu.Lock()
	baselinePresenceSubscriptions := len(core.PresenceHub.subscribers)
	core.PresenceHub.mu.Unlock()
	if baselinePresenceSubscriptions != 1 {
		t.Fatalf("PresenceHub subscriptions = %d, want one shared dispatcher subscription", baselinePresenceSubscriptions)
	}

	const streamCount = 3
	streamCancels := make([]context.CancelFunc, 0, streamCount)
	streamChannels := make([]<-chan EventEnvelope, 0, streamCount)
	for i := 0; i < streamCount; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		streamCancels = append(streamCancels, cancel)
		stream, err := core.StreamMyEventsWithOptions(ctx, "dispatcher-user", StreamMyEventsOptions{})
		if err != nil {
			t.Fatalf("StreamMyEventsWithOptions %d: %v", i, err)
		}
		streamChannels = append(streamChannels, stream)
	}

	if got := nc.NumSubscriptions(); got != baselineNATSSubscriptions {
		t.Fatalf("NATS subscriptions after %d streams = %d, want baseline %d", streamCount, got, baselineNATSSubscriptions)
	}
	core.PresenceHub.mu.Lock()
	gotPresenceSubscriptions := len(core.PresenceHub.subscribers)
	core.PresenceHub.mu.Unlock()
	if gotPresenceSubscriptions != baselinePresenceSubscriptions {
		t.Fatalf("PresenceHub subscriptions after %d streams = %d, want baseline %d", streamCount, gotPresenceSubscriptions, baselinePresenceSubscriptions)
	}

	core.myEventsModel.dispatchMu.Lock()
	gotDispatchSubscriptions := len(core.myEventsModel.dispatchSubscribers)
	core.myEventsModel.dispatchMu.Unlock()
	if gotDispatchSubscriptions != streamCount {
		t.Fatalf("dispatcher subscriptions = %d, want %d", gotDispatchSubscriptions, streamCount)
	}

	for _, cancel := range streamCancels {
		cancel()
	}
	for i, stream := range streamChannels {
		select {
		case _, ok := <-stream:
			if ok {
				t.Fatalf("stream %d emitted an event while closing", i)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("stream %d did not close", i)
		}
	}

	core.myEventsModel.dispatchMu.Lock()
	remainingDispatchSubscriptions := len(core.myEventsModel.dispatchSubscribers)
	core.myEventsModel.dispatchMu.Unlock()
	if remainingDispatchSubscriptions != 0 {
		t.Fatalf("dispatcher subscriptions after disconnect = %d, want 0", remainingDispatchSubscriptions)
	}
}

func TestMyEventsDispatcherSharesPreparedEvent(t *testing.T) {
	model := readyTestMyEventsDispatcher()
	sub1 := subscribeTestLiveDispatch(t, model)
	sub2 := subscribeTestLiveDispatch(t, model)

	prepared := &preparedLiveEvent{
		kind:     preparedPresence,
		envelope: NewHeartbeatEventEnvelope("event-id", timestamppb.Now()),
	}
	model.broadcastPreparedLiveEvent(prepared)

	if got := receivePreparedLiveEvent(t, sub1); got != prepared {
		t.Fatal("subscriber 1 did not receive the shared prepared event pointer")
	}
	if got := receivePreparedLiveEvent(t, sub2); got != prepared {
		t.Fatal("subscriber 2 did not receive the shared prepared event pointer")
	}
}

func TestMyEventsDispatcherDisconnectsOnlyOverflowingSubscriber(t *testing.T) {
	model := readyTestMyEventsDispatcher()
	slow := subscribeTestLiveDispatch(t, model)
	fast := subscribeTestLiveDispatch(t, model)

	for i := 0; i < liveDispatchQueueSize; i++ {
		model.broadcastPreparedLiveEvent(&preparedLiveEvent{
			kind:     preparedPresence,
			envelope: NewHeartbeatEventEnvelope("queued-event", timestamppb.Now()),
		})
		<-fast.C
	}

	overflow := &preparedLiveEvent{
		kind:     preparedPresence,
		envelope: NewHeartbeatEventEnvelope("overflow-event", timestamppb.Now()),
	}
	model.broadcastPreparedLiveEvent(overflow)

	select {
	case <-slow.Done:
	case <-time.After(time.Second):
		t.Fatal("slow subscriber was not disconnected")
	}
	if slow.reason != liveDispatchSubscriberSlow {
		t.Fatalf("slow subscriber reason = %d, want %d", slow.reason, liveDispatchSubscriberSlow)
	}
	select {
	case <-fast.Done:
		t.Fatal("fast subscriber was disconnected with slow subscriber")
	default:
	}
	if got := receivePreparedLiveEvent(t, fast); got != overflow {
		t.Fatal("fast subscriber did not receive overflow event")
	}
	if got := model.Metrics().SlowDisconnects; got != 1 {
		t.Fatalf("slow disconnect metric = %d, want 1", got)
	}
}

func TestMyEventsDispatcherSharedGapResetsCurrentSubscribers(t *testing.T) {
	model := readyTestMyEventsDispatcher()
	sub1 := subscribeTestLiveDispatch(t, model)
	sub2 := subscribeTestLiveDispatch(t, model)

	if got := model.resetDispatchSubscribers(liveDispatchSourceGap, true); got != 2 {
		t.Fatalf("reset subscriber count = %d, want 2", got)
	}
	for i, sub := range []*liveDispatchSubscription{sub1, sub2} {
		select {
		case <-sub.Done:
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d was not reset", i)
		}
		if sub.reason != liveDispatchSourceGap {
			t.Fatalf("subscriber %d reason = %d, want %d", i, sub.reason, liveDispatchSourceGap)
		}
	}
	if got := model.Metrics().SlowDisconnects; got != 2 {
		t.Fatalf("slow disconnect metric = %d, want 2", got)
	}

	replacement := subscribeTestLiveDispatch(t, model)
	select {
	case <-replacement.Done:
		t.Fatal("replacement subscriber started closed")
	default:
	}
}

func readyTestMyEventsDispatcher() *MyEventsModel {
	model := NewMyEventsModel(&ChattoCore{})
	model.finishDispatchStartup(nil)
	return model
}

func subscribeTestLiveDispatch(t *testing.T, model *MyEventsModel) *liveDispatchSubscription {
	t.Helper()
	sub, err := model.subscribeToLiveDispatch(context.Background())
	if err != nil {
		t.Fatalf("subscribeToLiveDispatch: %v", err)
	}
	t.Cleanup(func() { model.unsubscribeFromLiveDispatch(sub) })
	return sub
}

func receivePreparedLiveEvent(t *testing.T, sub *liveDispatchSubscription) *preparedLiveEvent {
	t.Helper()
	select {
	case event := <-sub.C:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prepared event")
		return nil
	}
}
