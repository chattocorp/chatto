package core

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/jobqueue"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	statev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// runWithEffectJobs runs one effect source with a job pool for every effect
// job type. It lets tests drive a model without a booted core.
func runWithEffectJobs(ctx context.Context, c *ChattoCore, source func(context.Context) error) error {
	handlers := map[string]jobqueue.Handler{}
	for kind, job := range c.effectJobs {
		handlers[kind] = job.perform
	}
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return source(ctx) })
	g.Go(func() error { return c.jobs.Run(ctx, c.jobsConsumer, handlers, 4) })
	return g.Wait()
}

// jobsOfType lists the current records of one job type.
func jobsOfType(t testing.TB, ctx context.Context, c *ChattoCore, kind string) []*statev1.JobRecord {
	t.Helper()
	all, _, err := c.jobs.List(ctx, "", 200)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	var out []*statev1.JobRecord
	for _, job := range all {
		if job.Type == kind {
			out = append(out, job)
		}
	}
	return out
}

func TestEffectJobPerformClassifiesSourceAndHandlerOutcomes(t *testing.T) {
	core, _ := newTestCore(t)
	ctx := testContext(t)
	subject := evtstream.RoomAggregate("R-effect-job").Subject(evtstream.EventCallEnded)
	event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_VoiceCallEnded{VoiceCallEnded: &evtv1.CallEndedEvent{RoomId: "R-effect-job", CallId: "C-effect-job"}}})
	seq, err := core.EventPublisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("append source fact: %v", err)
	}
	source, err := core.storage.serverEvtStream.GetMsg(ctx, seq)
	if err != nil {
		t.Fatalf("read source fact: %v", err)
	}
	digest := sha256.Sum256(source.Data)
	recordWithDigest := func(subject string, seq uint64, digest []byte) *statev1.JobRecord {
		payload, err := proto.Marshal(&statev1.EventEffectJob{Subject: subject, Sequence: seq, DataSha256: digest})
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		return &statev1.JobRecord{Payload: payload, Attempts: 2}
	}
	record := func(subject string, seq uint64) *statev1.JobRecord {
		return recordWithDigest(subject, seq, digest[:])
	}
	var seen events.DurableDelivery
	tests := []struct {
		name     string
		job      *statev1.JobRecord
		mayExp   bool
		handle   error
		category string // Empty means success.
	}{
		{name: "success", job: record(subject, seq)},
		{name: "retryable", job: record(subject, seq), handle: errors.New("temporary"), category: "attempt_failed"},
		{name: "terminated", job: record(subject, seq), handle: events.TerminateDelivery("bad", nil), category: "invalid_source"},
		{name: "invalid payload", job: &statev1.JobRecord{Payload: []byte{0xff}}, category: "invalid_payload"},
		{name: "subject mismatch", job: record("evt.room.other.call_ended", seq), category: "source_mismatch"},
		{name: "replaced source", job: recordWithDigest(subject, seq, make([]byte, sha256.Size)), category: "source_mismatch"},
		{name: "missing source", job: record(subject, seq+1000), category: "source_missing"},
		{name: "expired source", job: record(subject, seq+1000), mayExp: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &effectJob{core: core, stream: core.storage.serverEvtStream, options: effectJobOptions{SourceMayExpire: tt.mayExp}, handle: func(_ context.Context, d events.DurableDelivery) error {
				seen = d
				return tt.handle
			}}
			err := j.perform(ctx, tt.job)
			if tt.category == "" {
				if err != nil {
					t.Fatalf("perform = %v, want success", err)
				}
				return
			}
			if tt.category == "attempt_failed" {
				if err == nil || errors.As(err, new(*jobqueue.Failure)) {
					t.Fatalf("perform = %v, want plain retryable error", err)
				}
				return
			}
			failure, ok := errors.AsType[*jobqueue.Failure](err)
			if !ok || failure.Category != tt.category || !failure.Permanent {
				t.Fatalf("perform = %v, want permanent %s", err, tt.category)
			}
		})
	}
	if seen.Subject != subject || seen.StreamSequence != seq || seen.NumDelivered != 2 || len(seen.Data) == 0 {
		t.Fatalf("handler delivery = %+v, want reloaded source message", seen)
	}
}
