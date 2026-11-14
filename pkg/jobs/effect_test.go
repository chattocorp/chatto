package jobs

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"github.com/chattocorp/chatto/pkg/events"
	"github.com/chattocorp/chatto/pkg/jobs/internal/jobpb"
)

// testEffect returns a queue, its job consumer, and a source stream with an
// effect consumer that captures every source subject.
func testEffect(t *testing.T, handle events.DurableDeliveryHandler, options EffectOptions) (*Effect, jetstream.Consumer, jetstream.Stream) {
	t.Helper()
	q, jobsConsumer := testQueue(t)
	ctx := t.Context()
	source := must(q.js.CreateStream(ctx, jetstream.StreamConfig{Name: "SOURCE", Subjects: []string{"source.>"}, Storage: jetstream.MemoryStorage}))
	sourceConsumer := must(source.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{Durable: "effect", AckPolicy: jetstream.AckExplicitPolicy, MaxDeliver: -1}))
	effect := must(NewEffect(q, "work", source, sourceConsumer, handle, options))
	return effect, jobsConsumer, source
}

func TestEffectRunsHandlerOnceForAcceptedSourceMessages(t *testing.T) {
	var handled []string
	done := make(chan struct{}, 4)
	effect, jobsConsumer, source := testEffect(t, func(_ context.Context, d events.DurableDelivery) error {
		handled = append(handled, d.Subject+"="+string(d.Data))
		done <- struct{}{}
		return nil
	}, EffectOptions{MaxAttempts: 3, RetryDelay: time.Millisecond, Accept: func(d events.DurableDelivery) (bool, error) {
		return d.Subject == "source.wanted", nil
	}})
	ctx := t.Context()
	must(effect.queue.js.Publish(ctx, "source.ignored", []byte("a")))
	must(effect.queue.js.Publish(ctx, "source.wanted", []byte("b")))
	sourceCtx, stopSource := context.WithCancel(ctx)
	sourceDone := make(chan error, 1)
	go func() { sourceDone <- effect.RunSource(sourceCtx) }()
	t.Cleanup(func() { stopSource(); <-sourceDone })
	runQueue(t, effect.queue, jobsConsumer, map[string]Handler{effect.Type(): effect.Handle}, 1)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("effect did not run")
	}
	jobs, _, err := effect.queue.List(ctx, "", 10)
	noError(t, err)
	if len(jobs) != 1 {
		t.Fatalf("jobs = %+v, want one job for the accepted message", jobs)
	}
	awaitStatus(t, effect.queue, jobs[0].ID, Succeeded)
	if len(handled) != 1 || handled[0] != "source.wanted=b" {
		t.Fatalf("handled = %v", handled)
	}
	info := must(source.Info(ctx))
	if info.State.Msgs != 2 {
		t.Fatalf("source messages = %d", info.State.Msgs)
	}
}

func TestEffectHandleClassifiesSourceAndHandlerOutcomes(t *testing.T) {
	var result error
	var seen events.DurableDelivery
	effect, _, _ := testEffect(t, func(_ context.Context, d events.DurableDelivery) error {
		seen = d
		return result
	}, EffectOptions{MaxAttempts: 3, RetryDelay: time.Millisecond})
	ctx := t.Context()
	ack := must(effect.queue.js.Publish(ctx, "source.one", []byte("payload")))
	digest := sha256.Sum256([]byte("payload"))
	job := func(subject string, sequence uint64, digest []byte) Job {
		payload := must(proto.Marshal(&jobpb.EffectSource{Subject: subject, Sequence: sequence, DataSha256: digest}))
		return Job{ID: "j", Payload: payload, Attempts: 2}
	}
	valid := job("source.one", ack.Sequence, digest[:])
	tests := []struct {
		name       string
		job        Job
		mayExpire  bool
		handlerErr error
		category   string // Empty means success; "retry" means a plain error.
	}{
		{name: "success", job: valid},
		{name: "retryable", job: valid, handlerErr: errors.New("temporary"), category: "retry"},
		{name: "terminated", job: valid, handlerErr: events.TerminateDelivery("bad", nil), category: "invalid_source"},
		{name: "invalid payload", job: Job{Payload: []byte{0xff}}, category: "invalid_payload"},
		{name: "subject mismatch", job: job("source.other", ack.Sequence, digest[:]), category: "source_mismatch"},
		{name: "replaced source", job: job("source.one", ack.Sequence, make([]byte, sha256.Size)), category: "source_mismatch"},
		{name: "missing source", job: job("source.one", ack.Sequence+1000, digest[:]), category: "source_missing"},
		{name: "expired source", job: job("source.one", ack.Sequence+1000, digest[:]), mayExpire: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result = tt.handlerErr
			effect.options.SourceMayExpire = tt.mayExpire
			err := effect.Handle(ctx, tt.job)
			switch tt.category {
			case "":
				noError(t, err)
			case "retry":
				if err == nil || errors.As(err, new(*Failure)) {
					t.Fatalf("Handle = %v, want a plain retryable error", err)
				}
			default:
				failure, ok := errors.AsType[*Failure](err)
				if !ok || failure.Category != tt.category || !failure.Permanent {
					t.Fatalf("Handle = %v, want permanent %s", err, tt.category)
				}
			}
		})
	}
	if seen.Subject != "source.one" || seen.StreamSequence != ack.Sequence || seen.NumDelivered != 2 || string(seen.Data) != "payload" {
		t.Fatalf("handler delivery = %+v", seen)
	}
}
