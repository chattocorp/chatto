package events_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	. "hmans.de/chatto/pkg/events"
)

func lifecycleProjector(js jetstream.JetStream, stream jetstream.Stream, decodeErr error) *Projector {
	return NewDecodedProjector(js, stream, &codecTestProjection{subject: "evt.lifecycle.>"},
		func(data []byte) (DecodedEvent[codecTestEvent], error) {
			return DecodedEvent[codecTestEvent]{Event: codecTestEvent{name: string(data)}, ID: string(data)}, decodeErr
		}, testLogger())
}

func lifecycleStream(t *testing.T) (jetstream.JetStream, jetstream.Stream) {
	t.Helper()
	js, err := jetstream.New(startTestNATS(t))
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.CreateStream(testContext(t), jetstream.StreamConfig{Name: "LIFECYCLE", Subjects: []string{"evt.lifecycle.>"}})
	if err != nil {
		t.Fatal(err)
	}
	return js, stream
}

func runLifecycleProjector(t *testing.T, p *Projector) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(testContext(t))
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	if err := p.WaitForStartup(ctx); err != nil {
		t.Fatal(err)
	}
	return cancel, done
}

func awaitLifecycleExit(t *testing.T, done <-chan error, want error) {
	t.Helper()
	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("Run error = %v, want %v", err, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("projector did not stop")
	}
}

func lifecycleConsumers(t *testing.T, stream jetstream.Stream) map[string]*jetstream.ConsumerInfo {
	t.Helper()
	result := make(map[string]*jetstream.ConsumerInfo)
	list := stream.ListConsumers(testContext(t))
	for info := range list.Info() {
		result[info.Name] = info
	}
	if err := list.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProjectorConsumerIdentityRecoveryAndReplicaIsolation(t *testing.T) {
	js, stream := lifecycleStream(t)
	ctx := testContext(t)
	// A similarly named durable resource must survive projection cleanup.
	const durable = "projection-content-durable"
	if _, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{Durable: durable, AckPolicy: jetstream.AckExplicitPolicy}); err != nil {
		t.Fatal(err)
	}
	first := lifecycleProjector(js, stream, nil)
	if err := first.ConfigureConsumerIdentity("content", "Content read model"); err != nil {
		t.Fatal(err)
	}
	stopFirst, firstDone := runLifecycleProjector(t, first)
	var firstName string
	for name, info := range lifecycleConsumers(t, stream) {
		if name == durable {
			continue
		}
		firstName = name
		if !strings.HasPrefix(name, "projection-content-") || info.Config.Durable != "" || info.Config.InactiveThreshold != 5*time.Minute || info.Config.Metadata["projection_description"] != "Content read model" {
			t.Fatalf("unexpected projection consumer: %#v", info.Config)
		}
	}
	if firstName == "" {
		t.Fatal("missing first projection consumer")
	}
	if err := first.ConfigureConsumerIdentity("changed", "Changed"); !errors.Is(err, ErrProjectorAlreadyStarted) {
		t.Fatalf("identity mutation after Run = %v", err)
	}
	second := lifecycleProjector(js, stream, nil)
	if err := second.ConfigureConsumerIdentity("content", "Content read model"); err != nil {
		t.Fatal(err)
	}
	stopSecond, secondDone := runLifecycleProjector(t, second)
	before := lifecycleConsumers(t, stream)
	if len(before) != 3 {
		t.Fatalf("replicas share a consumer: got %d consumers, want 3", len(before))
	}
	var secondName string
	for name := range before {
		if name != durable && name != firstName {
			secondName = name
		}
	}
	// Force SDK recovery, then prove both projections still receive events.
	if err := stream.DeleteConsumer(ctx, firstName); err != nil {
		t.Fatal(err)
	}
	ack, err := js.Publish(ctx, "evt.lifecycle.created", []byte("after-reset"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []*Projector{first, second} {
		if err := p.WaitFor(ctx, SubjectPosition("evt.lifecycle.created", ack.Sequence)); err != nil {
			t.Fatal(err)
		}
	}
	after := lifecycleConsumers(t, stream)
	if len(after) != 3 || after[firstName] != nil {
		t.Fatalf("unexpected consumers after recovery: %v", after)
	}
	for name, info := range after {
		if name != durable && (info.Config.Metadata["projection_name"] != "content" || !strings.HasPrefix(name, "projection-content-")) {
			t.Fatalf("identity lost on recovery: %#v", info.Config)
		}
	}
	stopFirst()
	awaitLifecycleExit(t, firstDone, context.Canceled)
	after = lifecycleConsumers(t, stream)
	if len(after) != 2 || after[secondName] == nil || after[durable] == nil {
		t.Fatalf("cleanup did not isolate the current consumer: %v", after)
	}
	stopSecond()
	awaitLifecycleExit(t, secondDone, context.Canceled)
	after = lifecycleConsumers(t, stream)
	if len(after) != 1 || after[durable] == nil {
		t.Fatalf("cleanup left ephemeral consumers or removed durable state: %v", after)
	}
}

func TestProjectorConsumerCleanupOnProjectionFailure(t *testing.T) {
	js, stream := lifecycleStream(t)
	failure := errors.New("invalid projection event")
	p := lifecycleProjector(js, stream, failure)
	_, done := runLifecycleProjector(t, p)
	if _, err := js.Publish(testContext(t), "evt.lifecycle.created", []byte("invalid")); err != nil {
		t.Fatal(err)
	}
	awaitLifecycleExit(t, done, failure)
	if remaining := lifecycleConsumers(t, stream); len(remaining) != 0 {
		t.Fatalf("failed projector left consumers: %v", remaining)
	}
}

// unavailableCleanupStream simulates a broker that does not answer deletion.
// Consumer creation and delivery still cross the real JetStream boundary.
type unavailableCleanupStream struct {
	jetstream.Stream
	cleanupErr chan error
}

func (s unavailableCleanupStream) DeleteConsumer(ctx context.Context, _ string) error {
	<-ctx.Done()
	s.cleanupErr <- ctx.Err()
	return ctx.Err()
}

func TestProjectorConsumerCleanupHasIndependentDeadline(t *testing.T) {
	js, stream := lifecycleStream(t)
	wrapper := unavailableCleanupStream{Stream: stream, cleanupErr: make(chan error, 1)}
	p := lifecycleProjector(js, wrapper, nil)
	stop, done := runLifecycleProjector(t, p)
	stop()
	awaitLifecycleExit(t, done, context.Canceled)
	if err := <-wrapper.cleanupErr; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cleanup reused canceled Run context: %v", err)
	}
	if remaining := lifecycleConsumers(t, stream); len(remaining) != 1 {
		t.Fatalf("expected consumer retained for inactivity expiry: %v", remaining)
	}
}

func TestProjectorConsumerIdentityValidation(t *testing.T) {
	p := lifecycleProjector(nil, nil, nil)
	for _, name := range []string{"", "has.space", "has space", "has/slash", "has*wildcard", "has>filter", "ümlaut", strings.Repeat("a", 65)} {
		if err := p.ConfigureConsumerIdentity(name, "Read model"); err == nil {
			t.Fatalf("accepted invalid name %q", name)
		}
	}
	if err := p.ConfigureConsumerIdentity("content_v2-TEST", "Read model"); err != nil {
		t.Fatal(err)
	}
}

// failedConsumeStream creates a real consumer but fails the local pull setup.
type failedConsumeStream struct {
	jetstream.Stream
	failure error
}

type failedConsumeConsumer struct {
	jetstream.Consumer
	failure error
}

func (s failedConsumeStream) OrderedConsumer(ctx context.Context, cfg jetstream.OrderedConsumerConfig) (jetstream.Consumer, error) {
	consumer, err := s.Stream.OrderedConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return failedConsumeConsumer{Consumer: consumer, failure: s.failure}, nil
}

func (c failedConsumeConsumer) Consume(jetstream.MessageHandler, ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	return nil, c.failure
}

func TestProjectorConsumerCleanupOnConsumeSetupFailure(t *testing.T) {
	js, stream := lifecycleStream(t)
	failure := errors.New("pull setup failed")
	p := lifecycleProjector(js, failedConsumeStream{Stream: stream, failure: failure}, nil)
	if err := p.Run(testContext(t)); !errors.Is(err, failure) {
		t.Fatalf("Run error = %v, want setup failure", err)
	}
	if remaining := lifecycleConsumers(t, stream); len(remaining) != 0 {
		t.Fatalf("setup failure left consumers: %v", remaining)
	}
}
