package events_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	. "hmans.de/chatto/pkg/events"
)

// largeBatchProjection keeps every startup event in one pending batch until
// the projector flushes it.
type largeBatchProjection struct {
	codecTestBatchProjection
}

func (*largeBatchProjection) StartupBatchSize() int { return 10 }

// failingBatchProjection rejects every startup batch.
type failingBatchProjection struct {
	largeBatchProjection
}

var errStartupBatchRejected = errors.New("startup batch rejected")

func (*failingBatchProjection) ApplyStartupBatch([]SequencedEventOf[codecTestEvent]) error {
	return errStartupBatchRejected
}

// startupGate is a snapshot source that blocks until the test releases it and
// then reports no snapshot. The projector loads snapshots after it captures
// its startup target and before it creates the consumer, so the test can
// change the stream between those two points.
type startupGate struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	// maxCutoff is the startup target from the load request. Read it only
	// after entered closes.
	maxCutoff uint64
}

func newStartupGate() *startupGate {
	return &startupGate{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *startupGate) LoadProjectionSnapshot(_ context.Context, request ProjectionSnapshotLoadRequest) (ProjectionSnapshot, error) {
	g.once.Do(func() {
		g.maxCutoff = request.MaxCutoff
		close(g.entered)
		<-g.release
	})
	return ProjectionSnapshot{}, errors.New("no snapshot")
}

func (g *startupGate) newProjector(t *testing.T, js jetstream.JetStream, stream jetstream.Stream, projection EventProjection[codecTestEvent]) *Projector {
	t.Helper()
	projector := NewDecodedProjector(js, stream, projection, decodeCodecTestEvent, testLogger())
	if err := projector.ConfigureSnapshots("startup-target", g, func(*jetstream.StreamInfo) (string, error) {
		return "stream-identity", nil
	}); err != nil {
		t.Fatalf("configure snapshot gate: %v", err)
	}
	return projector
}

type startupTargetFixture struct {
	stream    jetstream.Stream
	eventLog  *EncodedEventLog
	subject   string
	sequences []uint64
}

func newStartupTargetFixture(t *testing.T, subject string) (*startupTargetFixture, jetstream.JetStream) {
	t.Helper()
	js, stream := setupTestStream(t)
	fixture := &startupTargetFixture{
		stream:   stream,
		eventLog: NewEncodedEventLog(js, stream, testLogger()),
		subject:  subject,
	}
	for _, record := range []EncodedRecord{
		{ID: "one", Data: []byte("one:alpha")},
		{ID: "two", Data: []byte("two:beta")},
		{ID: "three", Data: []byte("three:gamma")},
	} {
		fixture.sequences = append(fixture.sequences, fixture.append(t, record))
	}
	return fixture, js
}

func (f *startupTargetFixture) append(t *testing.T, record EncodedRecord) uint64 {
	t.Helper()
	sequence, err := f.eventLog.AppendEventually(testContext(t), f.subject, record)
	if err != nil {
		t.Fatalf("append %s: %v", record.ID, err)
	}
	return sequence
}

// runWithDeletedTarget starts projector, waits until the startup target is
// fixed, deletes that target, calls beforeRelease, and then lets replay go on.
func (f *startupTargetFixture) runWithDeletedTarget(t *testing.T, projector *Projector, gate *startupGate, beforeRelease func()) {
	t.Helper()
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("projector did not start replay")
	}
	target := f.sequences[len(f.sequences)-1]
	if gate.maxCutoff != target {
		close(gate.release)
		t.Fatalf("startup target = %d, want %d", gate.maxCutoff, target)
	}
	if err := f.stream.DeleteMsg(testContext(t), target); err != nil {
		close(gate.release)
		t.Fatalf("delete startup target: %v", err)
	}
	if beforeRelease != nil {
		beforeRelease()
	}
	close(gate.release)
}

func waitForStartup(t *testing.T, projector *Projector) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := projector.WaitForStartup(ctx); err != nil {
		t.Fatalf("WaitForStartup: %v (status %+v)", err, projector.Status())
	}
}

func TestProjectorCompletesStartupWhenTargetIsDeleted(t *testing.T) {
	fixture, js := newStartupTargetFixture(t, "evt.startup.deleted.plain")
	projection := &codecTestProjection{subject: fixture.subject}
	gate := newStartupGate()
	projector := gate.newProjector(t, js, fixture.stream, projection)
	projector.SetStartupReconcileIntervalForTest(10 * time.Millisecond)

	fixture.runWithDeletedTarget(t, projector, gate, nil)
	waitForStartup(t, projector)

	events, sequences := projection.applied()
	if !slices.Equal(events, []string{"alpha", "beta"}) || !slices.Equal(sequences, fixture.sequences[:2]) {
		t.Fatalf("applied %v at %v, want retained startup events", events, sequences)
	}
	if status := projector.Status(); status.LastSeq != fixture.sequences[1] || status.StartupTargetSeq != fixture.sequences[1] {
		t.Fatalf("status = %+v, want startup target lowered to last retained event", status)
	}
}

func TestProjectorFlushesStartupBatchWhenTargetIsDeleted(t *testing.T) {
	fixture, js := newStartupTargetFixture(t, "evt.startup.deleted.batch")
	projection := &largeBatchProjection{
		codecTestBatchProjection: codecTestBatchProjection{
			codecTestProjection: codecTestProjection{subject: fixture.subject},
		},
	}
	gate := newStartupGate()
	projector := gate.newProjector(t, js, fixture.stream, projection)
	projector.SetStartupReconcileIntervalForTest(10 * time.Millisecond)

	fixture.runWithDeletedTarget(t, projector, gate, nil)
	waitForStartup(t, projector)

	events, batches := projection.state()
	if !slices.Equal(events, []string{"alpha", "beta"}) {
		t.Fatalf("events = %v, want retained startup events", events)
	}
	wantBatches := [][]uint64{fixture.sequences[:2]}
	if !slices.EqualFunc(batches, wantBatches, slices.Equal[[]uint64]) {
		t.Fatalf("batches = %v, want %v", batches, wantBatches)
	}
	if status := projector.Status(); status.LastSeq != fixture.sequences[1] {
		t.Fatalf("status = %+v, want last retained startup event applied", status)
	}
}

func TestProjectorAppliesPendingStartupBatchBeforeLaterEvent(t *testing.T) {
	fixture, js := newStartupTargetFixture(t, "evt.startup.deleted.later")
	projection := &largeBatchProjection{
		codecTestBatchProjection: codecTestBatchProjection{
			codecTestProjection: codecTestProjection{subject: fixture.subject},
		},
	}
	gate := newStartupGate()
	projector := gate.newProjector(t, js, fixture.stream, projection)
	// Keep reconciliation out of this test: the later event alone must flush
	// the pending batch in stream order.
	projector.SetStartupReconcileIntervalForTest(time.Hour)

	var laterSeq uint64
	fixture.runWithDeletedTarget(t, projector, gate, func() {
		laterSeq = fixture.append(t, EncodedRecord{ID: "four", Data: []byte("four:delta")})
	})
	waitForStartup(t, projector)
	waitFor(t, 3*time.Second, func() bool { return projector.LastSeq() >= laterSeq })

	events, sequences := projection.applied()
	if !slices.Equal(events, []string{"alpha", "beta", "delta"}) {
		t.Fatalf("events = %v, want pending batch before later event", events)
	}
	wantSequences := append(slices.Clone(fixture.sequences[:2]), laterSeq)
	if !slices.Equal(sequences, wantSequences) {
		t.Fatalf("sequences = %v, want %v", sequences, wantSequences)
	}
}

func TestProjectorFailsWhenStartupBatchFlushFailsAfterTargetDeletion(t *testing.T) {
	fixture, js := newStartupTargetFixture(t, "evt.startup.deleted.failing")
	projection := &failingBatchProjection{}
	projection.subject = fixture.subject
	gate := newStartupGate()
	projector := gate.newProjector(t, js, fixture.stream, projection)
	projector.SetStartupReconcileIntervalForTest(10 * time.Millisecond)

	fixture.runWithDeletedTarget(t, projector, gate, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := projector.WaitForStartup(ctx); !errors.Is(err, ErrProjectionFailed) || !errors.Is(err, errStartupBatchRejected) {
		t.Fatalf("WaitForStartup error = %v, want failed startup batch", err)
	}
	if status := projector.Status(); status.FailedSeq != fixture.sequences[0] || status.LastSeq != 0 || status.StartupComplete {
		t.Fatalf("status = %+v, want failure at first batched event", status)
	}
}
