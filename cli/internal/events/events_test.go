package events

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

// ============================================================================
// Test Setup
// ============================================================================

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func testLogger() Logger {
	return log.New(io.Discard)
}

// setupTestStream spins up an embedded NATS server with JetStream, creates
// a stream with the EVT shape (subjects "server.evt.>"), and returns
// the wired-up bits plus a cleanup-registered teardown.
func setupTestStream(t *testing.T) (jetstream.JetStream, jetstream.Stream) {
	t.Helper()

	_, nc := testutil.StartNATS(t)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}

	ctx := testContext(t)
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:               "EVT_TEST",
		Subjects:           []string{SubjectRoot + ">"},
		Storage:            jetstream.FileStorage,
		AllowAtomicPublish: true, // exercise AppendBatch in tests
	})
	if err != nil {
		t.Fatalf("create test stream: %v", err)
	}

	return js, stream
}

// makeEvent constructs a minimal event with a UserJoinedRoom payload so
// validateEvent passes. The room_id field is what tests typically assert on.
func makeEvent(roomID, userID string) *corev1.Event {
	return &corev1.Event{
		Id:        "EVT-" + roomID + "-" + userID,
		ActorId:   userID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{
				RoomId: roomID,
			},
		},
	}
}

func makeMessagePostedEvent(roomID, userID string) *corev1.Event {
	return &corev1.Event{
		Id:        "EVT-msg-" + roomID + "-" + userID,
		ActorId:   userID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				RoomId: roomID,
			},
		},
	}
}

// ============================================================================
// Publisher
// ============================================================================

func TestPublisher_Append_HappyPath(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	subject := RoomAggregate("R1").Subject(EventUserJoinedRoom)

	seq1, err := pub.Append(ctx, subject, makeEvent("R1", "U1"))
	if err != nil {
		t.Fatalf("first Append: %v", err)
	}
	if seq1 == 0 {
		t.Errorf("expected non-zero seq, got 0")
	}

	seq2, err := pub.Append(ctx, subject, makeEvent("R1", "U2"))
	if err != nil {
		t.Fatalf("second Append: %v", err)
	}
	if seq2 <= seq1 {
		t.Errorf("expected seq2 > seq1, got seq1=%d seq2=%d", seq1, seq2)
	}
}

func TestPublisher_Append_SetsNATSMsgID(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	event := makeEvent("R1", "U1")
	seq, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), event)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	msg, err := stream.GetMsg(ctx, seq)
	if err != nil {
		t.Fatalf("GetMsg: %v", err)
	}
	if got := msg.Header.Get(jetstream.MsgIDHeader); got != event.Id {
		t.Errorf("Nats-Msg-Id = %q, want %q", got, event.Id)
	}
}

func TestPublisher_Append_DuplicateEventIDSuppressesSecondAppend(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	subject := RoomAggregate("R1").Subject(EventUserJoinedRoom)
	event := makeEvent("R1", "U1")

	seq1, err := pub.Append(ctx, subject, event)
	if err != nil {
		t.Fatalf("first Append: %v", err)
	}

	seq2, err := pub.Append(ctx, subject, event)
	if err != nil {
		t.Fatalf("duplicate Append: %v", err)
	}
	if seq2 != seq1 {
		t.Fatalf("duplicate Append seq = %d, want original seq %d", seq2, seq1)
	}

	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatalf("stream Info: %v", err)
	}
	if info.State.Msgs != 1 {
		t.Errorf("stream messages = %d, want 1", info.State.Msgs)
	}
}

func TestPublisher_Append_RejectsInvalidEvent(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	tests := []struct {
		name  string
		event *corev1.Event
	}{
		{"nil event", nil},
		{"empty wrapper", &corev1.Event{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), tc.event)
			if !errors.Is(err, ErrInvalidEvent) {
				t.Errorf("want ErrInvalidEvent, got %v", err)
			}
		})
	}
}

func TestPublisher_AppendEventually_ConcurrentWrites(t *testing.T) {
	// Multiple goroutines append to the same subject. Each should succeed
	// (AppendEventually retries on OCC conflict); the final per-subject
	// seq should equal the number of writes.
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	subject := RoomAggregate("R1").Subject(EventUserJoinedRoom)
	const writers = 10

	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := pub.AppendEventually(ctx, subject, makeEvent("R1", "U"+itoa(i)))
			if err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent Append: %v", err)
	}

	// Verify the last seq matches the number of writes.
	msg, err := stream.GetLastMsgForSubject(ctx, subject)
	if err != nil {
		t.Fatalf("GetLastMsgForSubject: %v", err)
	}
	if msg.Sequence != writers {
		t.Errorf("want last seq %d, got %d", writers, msg.Sequence)
	}
}

func TestPublisher_AppendAt_ConflictReturnsTypedError(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	subject := RoomAggregate("R1").Subject(EventUserJoinedRoom)

	// Place one event so the subject's current last seq is non-zero.
	if _, err := pub.Append(ctx, subject, makeEvent("R1", "U1")); err != nil {
		t.Fatalf("seed Append: %v", err)
	}

	// AppendAt with expectedSeq=0 must fail with ErrConflict.
	_, err := pub.AppendAt(ctx, subject, makeEvent("R1", "U2"), 0)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestPublisher_AppendAt_DeterministicSequence(t *testing.T) {
	// Simulates a migration: a series of AppendAt calls threading the
	// returned stream seq forward as the next call's expected seq.
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	subject := RoomAggregate("R1").Subject(EventUserJoinedRoom)
	const count = 5

	var expectedSeq uint64 // 0 = no prior message
	for i := 0; i < count; i++ {
		seq, err := pub.AppendAt(ctx, subject, makeEvent("R1", "U"+itoa(i)), expectedSeq)
		if err != nil {
			t.Fatalf("AppendAt[%d]: %v", i, err)
		}
		if seq == 0 {
			t.Errorf("AppendAt[%d] returned zero seq", i)
		}
		expectedSeq = seq
	}

	// A second run starting at expectedSeq=0 must conflict on the first
	// call (migration replayability: re-running no-ops on already-emitted
	// subjects).
	_, err := pub.AppendAt(ctx, subject, makeEvent("R1", "Ureplay"), 0)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("want ErrConflict on replay, got %v", err)
	}
}

// ============================================================================
// AppendBatch (atomic multi-aggregate publishes)
// ============================================================================

// TestPublisher_AppendBatch_LandsContiguouslyAtomic verifies the
// happy path: N entries get N contiguous stream sequences, and the
// returned slice reflects publication order (commit ack's seq is
// the LAST entry's seq).
func TestPublisher_AppendBatch_LandsContiguouslyAtomic(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	// Seed an unrelated subject so the batch lands at a non-trivial offset.
	if _, err := pub.Append(ctx, RoomAggregate("WARMUP").Subject(EventUserJoinedRoom), makeEvent("WARMUP", "U")); err != nil {
		t.Fatalf("warmup: %v", err)
	}

	entries := []BatchEntry{
		{Subject: GroupAggregate("GA").Subject(EventUserJoinedRoom), Event: makeEvent("RA", "U1"), HasOCC: true, ExpectedSeq: 0},
		{Subject: GroupAggregate("GB").Subject(EventUserJoinedRoom), Event: makeEvent("RB", "U2")},
		{Subject: GroupAggregate("GC").Subject(EventUserJoinedRoom), Event: makeEvent("RC", "U3")},
	}

	seqs, err := pub.AppendBatch(ctx, entries)
	if err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}
	if len(seqs) != 3 {
		t.Fatalf("len(seqs) = %d, want 3", len(seqs))
	}
	if seqs[1] != seqs[0]+1 || seqs[2] != seqs[1]+1 {
		t.Errorf("seqs not contiguous: %v", seqs)
	}
	for i, seq := range seqs {
		msg, err := stream.GetMsg(ctx, seq)
		if err != nil {
			t.Fatalf("GetMsg[%d]: %v", i, err)
		}
		if got := msg.Header.Get(jetstream.MsgIDHeader); got != entries[i].Event.GetId() {
			t.Errorf("batch msg %d Nats-Msg-Id = %q, want %q", i, got, entries[i].Event.GetId())
		}
	}

	// Each subject's last seq must match what we published.
	for i, e := range entries {
		got, err := pub.lastSubjectSeq(ctx, e.Subject)
		if err != nil {
			t.Fatalf("lastSubjectSeq(%s): %v", e.Subject, err)
		}
		if got != seqs[i] {
			t.Errorf("subject %s last seq = %d, want %d", e.Subject, got, seqs[i])
		}
	}
}

func TestPublisher_AppendBatch_RejectsUnguardedBatch(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())

	entries := []BatchEntry{
		{Subject: GroupAggregate("GA").Subject(EventUserJoinedRoom), Event: makeEvent("RA", "U1")},
		{Subject: GroupAggregate("GB").Subject(EventUserJoinedRoom), Event: makeEvent("RB", "U2")},
	}

	_, err := pub.AppendBatch(testContext(t), entries)
	if !errors.Is(err, ErrMissingOCC) {
		t.Fatalf("want ErrMissingOCC, got %v", err)
	}
}

// TestPublisher_AppendBatch_OCCFailureRejectsEntireBatch verifies
// that a per-entry OCC mismatch causes the batch to be rejected and
// no entries land on the stream.
func TestPublisher_AppendBatch_OCCFailureRejectsEntireBatch(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	// Make subject GA non-empty so an "expect seq 0" OCC must fail.
	seqA, err := pub.Append(ctx, GroupAggregate("GA").Subject(EventUserJoinedRoom), makeEvent("RA", "Useed"))
	if err != nil {
		t.Fatalf("seed GA: %v", err)
	}

	entries := []BatchEntry{
		// GB has no events yet — expect seq 0 passes.
		{Subject: GroupAggregate("GB").Subject(EventUserJoinedRoom), Event: makeEvent("RB", "U"), HasOCC: true, ExpectedSeq: 0},
		// GA already has seqA — expecting 0 must fail.
		{Subject: GroupAggregate("GA").Subject(EventUserJoinedRoom), Event: makeEvent("RA", "U"), HasOCC: true, ExpectedSeq: 0},
	}

	_, err = pub.AppendBatch(ctx, entries)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict on OCC mismatch, got %v", err)
	}

	// Neither subject should have advanced past its pre-batch state.
	gotA, _ := pub.lastSubjectSeq(ctx, GroupAggregate("GA").Subject(EventUserJoinedRoom))
	if gotA != seqA {
		t.Errorf("GA last seq = %d, want %d (unchanged)", gotA, seqA)
	}
	gotB, _ := pub.lastSubjectSeq(ctx, GroupAggregate("GB").Subject(EventUserJoinedRoom))
	if gotB != 0 {
		t.Errorf("GB last seq = %d, want 0 (no events)", gotB)
	}
}

// TestPublisher_AppendBatch_EmptyIsNoOp verifies the degenerate
// case — callers shouldn't need to guard against passing an empty
// slice.
func TestPublisher_AppendBatch_EmptyIsNoOp(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())

	seqs, err := pub.AppendBatch(testContext(t), nil)
	if err != nil {
		t.Errorf("AppendBatch(nil): %v", err)
	}
	if len(seqs) != 0 {
		t.Errorf("seqs = %v, want empty", seqs)
	}
}

// ============================================================================
// Projector
// ============================================================================

// trackingProjection records every Apply call so tests can assert on the
// observed event stream.
type trackingProjection struct {
	mu     sync.Mutex
	events []*corev1.Event
	seqs   []uint64
	subs   []string
}

func newTrackingProjection(subs ...string) *trackingProjection {
	return &trackingProjection{subs: subs}
}

func (p *trackingProjection) Subjects() []string { return p.subs }

func (p *trackingProjection) Apply(e *corev1.Event, seq uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, e)
	p.seqs = append(p.seqs, seq)
	return nil
}

func (p *trackingProjection) Snapshot() ([]byte, error) { return nil, nil }
func (p *trackingProjection) Restore(_ []byte) error    { return nil }

func (p *trackingProjection) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

type replayTrackingProjection struct {
	*trackingProjection
	replay []string
}

func newReplayTrackingProjection(subjects []string, replay []string) *replayTrackingProjection {
	return &replayTrackingProjection{
		trackingProjection: newTrackingProjection(subjects...),
		replay:             replay,
	}
}

func (p *replayTrackingProjection) ReplaySubjects() []string { return p.replay }

type blockingProjection struct {
	*trackingProjection
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingProjection(subs ...string) *blockingProjection {
	return &blockingProjection{
		trackingProjection: newTrackingProjection(subs...),
		entered:            make(chan struct{}),
		release:            make(chan struct{}),
	}
}

func (p *blockingProjection) Apply(e *corev1.Event, seq uint64) error {
	p.once.Do(func() { close(p.entered) })
	<-p.release
	return p.trackingProjection.Apply(e, seq)
}

func waitForProjectorStarted(t *testing.T, projector *Projector) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !projector.Started() {
		if time.Now().After(deadline) {
			t.Fatal("projector did not start")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestProjector_AppliesEventsInOrder(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())

	// Seed three events before the projector starts.
	ctx := testContext(t)
	for i := 0; i < 3; i++ {
		if _, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), makeEvent("R1", "U"+itoa(i))); err != nil {
			t.Fatalf("seed Append: %v", err)
		}
	}

	proj := newTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	// Wait for the projection to catch up to the three seeded events.
	waitFor(t, 2*time.Second, func() bool { return proj.Count() == 3 })

	// LastSeq should equal the stream's last sequence for our subject.
	msg, err := stream.GetLastMsgForSubject(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom))
	if err != nil {
		t.Fatalf("GetLastMsgForSubject: %v", err)
	}
	if got := projector.LastSeq(); got != msg.Sequence {
		t.Errorf("LastSeq=%d, want %d", got, msg.Sequence)
	}
}

func TestRunProjectors_SharedConsumerAppliesEventsToAllProjectors(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())

	ctx := testContext(t)
	for i := 0; i < 3; i++ {
		if _, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), makeEvent("R1", "U"+itoa(i))); err != nil {
			t.Fatalf("seed Append: %v", err)
		}
	}

	projA := newTrackingProjection(RoomSubjectFilter())
	projB := newTrackingProjection(RoomSubjectFilter())
	projectorA := NewProjector(js, stream, projA, testLogger())
	projectorB := NewProjector(js, stream, projB, testLogger())

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = RunProjectors(runCtx, projectorA, projectorB) }()

	waitFor(t, 2*time.Second, func() bool {
		return projA.Count() == 3 && projB.Count() == 3
	})

	statusA := projectorA.Status()
	statusB := projectorB.Status()
	if !statusA.StartupComplete || !statusB.StartupComplete {
		t.Fatalf("startup complete = %v/%v, want both true", statusA.StartupComplete, statusB.StartupComplete)
	}
	if statusA.StartupMessages != 3 || statusB.StartupMessages != 3 {
		t.Fatalf("startup messages = %d/%d, want 3/3", statusA.StartupMessages, statusB.StartupMessages)
	}
	if statusA.LastSeq != statusB.LastSeq {
		t.Fatalf("last seq mismatch = %d/%d", statusA.LastSeq, statusB.LastSeq)
	}
}

func TestRunProjectors_SharedReplaySkipsNonLogicalSubjects(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())

	ctx := testContext(t)
	joined := makeEvent("R1", "U1")
	joinedSeq, err := pub.Append(ctx, RoomAggregate("R1").SubjectFor(joined), joined)
	if err != nil {
		t.Fatalf("Append joined: %v", err)
	}
	posted := makeMessagePostedEvent("R1", "U2")
	postedSeq, err := pub.Append(ctx, RoomAggregate("R1").SubjectFor(posted), posted)
	if err != nil {
		t.Fatalf("Append posted: %v", err)
	}

	broad := newTrackingProjection(RoomSubjectFilter())
	focused := newReplayTrackingProjection(
		[]string{RoomEventTypeFilter(EventUserJoinedRoom)},
		[]string{RoomSubjectFilter()},
	)
	broadProjector := NewProjector(js, stream, broad, testLogger())
	focusedProjector := NewProjector(js, stream, focused, testLogger())

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = RunProjectors(runCtx, broadProjector, focusedProjector) }()

	waitFor(t, 2*time.Second, func() bool {
		return broad.Count() == 2 && focused.Count() == 1 && focusedProjector.LastSeq() == postedSeq
	})

	focused.mu.Lock()
	gotSeq := focused.seqs[0]
	focused.mu.Unlock()
	if gotSeq != joinedSeq {
		t.Fatalf("focused seq = %d, want joined seq %d", gotSeq, joinedSeq)
	}
	status := focusedProjector.Status()
	if status.StartupMessages != 1 {
		t.Fatalf("focused startup messages = %d, want 1", status.StartupMessages)
	}
	if status.LastSeq != postedSeq {
		t.Fatalf("focused last seq = %d, want skipped replay tail seq %d", status.LastSeq, postedSeq)
	}
}

func TestRunProjectors_RejectsMismatchedSubjects(t *testing.T) {
	js, stream := setupTestStream(t)

	projectorA := NewProjector(js, stream, newTrackingProjection(RoomSubjectFilter()), testLogger())
	projectorB := NewProjector(js, stream, newTrackingProjection(UserSubjectFilter()), testLogger())

	err := RunProjectors(testContext(t), projectorA, projectorB)
	if err == nil || !strings.Contains(err.Error(), "identical replay subjects") {
		t.Fatalf("RunProjectors error = %v, want identical replay subjects error", err)
	}
}

func TestRunProjectors_ContinuesFanoutAfterProjectionFailure(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	subject := RoomAggregate("R1").Subject(EventUserJoinedRoom)
	seq, err := pub.Append(ctx, subject, makeEvent("R1", "U1"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	applyErr := errors.New("apply failed")
	failing := &failingProjection{
		trackingProjection: newTrackingProjection(RoomSubjectFilter()),
		err:                applyErr,
	}
	healthy := newTrackingProjection(RoomSubjectFilter())
	failingProjector := NewProjector(js, stream, failing, testLogger())
	healthyProjector := NewProjector(js, stream, healthy, testLogger())

	runCtx, cancelRun := context.WithCancel(context.Background())
	t.Cleanup(cancelRun)
	errCh := make(chan error, 1)
	go func() { errCh <- RunProjectors(runCtx, failingProjector, healthyProjector) }()

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrProjectionFailed) {
			t.Fatalf("RunProjectors error = %v, want ErrProjectionFailed", err)
		}
		if !errors.Is(err, applyErr) {
			t.Fatalf("RunProjectors error = %v, want wrapped apply error", err)
		}
	case <-ctx.Done():
		t.Fatal("RunProjectors did not return after projection failure")
	}

	if got := healthy.Count(); got != 1 {
		t.Fatalf("healthy projection Apply count = %d, want 1", got)
	}
	if got := healthyProjector.LastSeq(); got != seq {
		t.Fatalf("healthy projector LastSeq = %d, want %d", got, seq)
	}
	if err := healthyProjector.WaitFor(ctx, SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("healthy projector WaitFor failed after shared failure: %v", err)
	}
}

func TestProjector_StatusReportsStartupDuration(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	subject := RoomAggregate("R1").Subject(EventUserJoinedRoom)
	seq, err := pub.Append(ctx, subject, makeEvent("R1", "U1"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	proj := newBlockingProjection(RoomSubjectFilter())
	releaseProjection := func() {
		select {
		case <-proj.release:
		default:
			close(proj.release)
		}
	}
	t.Cleanup(releaseProjection)

	projector := NewProjector(js, stream, proj, testLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	select {
	case <-proj.entered:
	case <-ctx.Done():
		t.Fatal("projection Apply did not start")
	}
	time.Sleep(20 * time.Millisecond)
	inProgress := projector.Status()
	if inProgress.StartupComplete {
		t.Fatal("StartupComplete = true before initial replay finished")
	}
	if inProgress.StartupDuration <= 0 {
		t.Fatalf("StartupDuration while in progress = %s, want positive elapsed duration", inProgress.StartupDuration)
	}

	releaseProjection()

	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete
	})

	status := projector.Status()
	if status.StartupTargetSeq != seq {
		t.Fatalf("StartupTargetSeq = %d, want %d", status.StartupTargetSeq, seq)
	}
	if status.StartupDuration < 10*time.Millisecond {
		t.Fatalf("StartupDuration = %s, want at least 10ms", status.StartupDuration)
	}
}

func TestProjector_WaitFor_AlreadyReached(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	subject := RoomAggregate("R1").Subject(EventUserJoinedRoom)
	seq, err := pub.Append(ctx, subject, makeEvent("R1", "U1"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	proj := newTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	waitFor(t, 2*time.Second, func() bool { return projector.LastSeq() > 0 })

	// WaitFor for a seq we've already reached returns immediately.
	deadline, cancelDeadline := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancelDeadline()
	if err := projector.WaitFor(deadline, SubjectPosition(subject, seq)); err != nil {
		t.Errorf("WaitFor for already-reached seq: %v", err)
	}
}

func TestProjector_WaitFor_UnblocksOnApply(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())

	proj := newTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	// Publish, capture subject + seq, then WaitFor must return without timing out.
	ctx := testContext(t)
	subject := RoomAggregate("R1").Subject(EventUserJoinedRoom)
	seq, err := pub.Append(ctx, subject, makeEvent("R1", "U1"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	deadline, cancelDeadline := context.WithTimeout(ctx, 2*time.Second)
	defer cancelDeadline()
	if err := projector.WaitFor(deadline, SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if got := projector.LastSeq(); got < seq {
		t.Errorf("LastSeq=%d, want >= %d", got, seq)
	}
}

func TestProjector_WaitFor_HonoursContextCancel(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	proj := newBlockingProjection(RoomSubjectFilter())
	t.Cleanup(func() { close(proj.release) })
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancelRun := context.WithCancel(context.Background())
	t.Cleanup(cancelRun)
	go func() { _ = projector.Run(runCtx) }()
	waitForProjectorStarted(t, projector)

	ctx := testContext(t)
	subject := RoomAggregate("R1").Subject(EventUserJoinedRoom)
	seq, err := pub.Append(ctx, subject, makeEvent("R1", "U1"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	select {
	case <-proj.entered:
	case <-ctx.Done():
		t.Fatal("projection Apply did not start")
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := projector.WaitFor(waitCtx, SubjectPosition(subject, seq)); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want DeadlineExceeded, got %v", err)
	}
}

func TestProjector_WaitForRejectsUnconsumedSubject(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	proj := newTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	userSubject := UserAggregate("U1").Subject(EventUserAccountCreated)
	userSeq, err := pub.Append(ctx, userSubject, makeEvent("R1", "U1"))
	if err != nil {
		t.Fatalf("user Append: %v", err)
	}
	roomSubject := RoomAggregate("R1").Subject(EventUserJoinedRoom)
	roomSeq, err := pub.Append(ctx, roomSubject, makeEvent("R1", "U2"))
	if err != nil {
		t.Fatalf("room Append: %v", err)
	}
	if err := projector.WaitFor(ctx, SubjectPosition(roomSubject, roomSeq)); err != nil {
		t.Fatalf("warm WaitFor: %v", err)
	}
	if got := projector.LastSeq(); got <= userSeq {
		t.Fatalf("test setup expected projector LastSeq beyond user seq; got %d <= %d", got, userSeq)
	}

	err = projector.WaitFor(ctx, SubjectPosition(userSubject, userSeq))
	if !errors.Is(err, ErrProjectionSubjectNotConsumed) {
		t.Fatalf("want ErrProjectionSubjectNotConsumed, got %v", err)
	}
}

func TestProjector_WaitForRejectsSequenceSubjectMismatch(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	proj := newTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	userSubject := UserAggregate("U1").Subject(EventUserAccountCreated)
	userSeq, err := pub.Append(ctx, userSubject, makeEvent("R1", "U1"))
	if err != nil {
		t.Fatalf("user Append: %v", err)
	}
	roomSubject := RoomAggregate("R1").Subject(EventUserJoinedRoom)

	err = projector.WaitFor(ctx, SubjectPosition(roomSubject, userSeq))
	if !errors.Is(err, ErrProjectionSequenceSubjectMismatch) {
		t.Fatalf("want ErrProjectionSequenceSubjectMismatch, got %v", err)
	}
}

func TestProjector_WaitForAcceptsSubjectFilter(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	proj := newTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	subject := RoomAggregate("R1").Subject(EventUserJoinedRoom)
	seq, err := pub.Append(ctx, subject, makeEvent("R1", "U1"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	if err := projector.WaitFor(ctx, SubjectPosition(RoomSubjectFilter(), seq)); err != nil {
		t.Fatalf("WaitFor with wildcard filter: %v", err)
	}
}

type failingProjection struct {
	*trackingProjection
	err error
}

func (p *failingProjection) Apply(_ *corev1.Event, _ uint64) error {
	return p.err
}

func TestProjector_WaitFor_ReturnsProjectionError(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	applyErr := errors.New("apply failed")
	proj := &failingProjection{
		trackingProjection: newTrackingProjection(RoomSubjectFilter()),
		err:                applyErr,
	}
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancelRun := context.WithCancel(context.Background())
	t.Cleanup(cancelRun)
	go func() { _ = projector.Run(runCtx) }()
	waitForProjectorStarted(t, projector)

	ctx := testContext(t)
	seq, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), makeEvent("R1", "U1"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	err = projector.WaitFor(ctx, SubjectPosition(RoomAggregate("R1").Subject(EventUserJoinedRoom), seq))
	if !errors.Is(err, ErrProjectionFailed) {
		t.Fatalf("want ErrProjectionFailed, got %v", err)
	}
	if !errors.Is(err, applyErr) {
		t.Fatalf("want wrapped apply error, got %v", err)
	}
	if got := projector.LastSeq(); got >= seq {
		t.Fatalf("LastSeq=%d, want less than failed seq %d", got, seq)
	}

	status := projector.Status()
	if !status.Failed {
		t.Fatal("Status.Failed = false, want true")
	}
	if status.FailedSeq != seq {
		t.Fatalf("Status.FailedSeq = %d, want %d", status.FailedSeq, seq)
	}
	if !errors.Is(status.Err, applyErr) {
		t.Fatalf("Status.Err = %v, want wrapped apply error", status.Err)
	}
}

func TestProjector_RunReturnsProjectionError(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	applyErr := errors.New("apply failed")
	proj := &failingProjection{
		trackingProjection: newTrackingProjection(RoomSubjectFilter()),
		err:                applyErr,
	}
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancelRun := context.WithCancel(context.Background())
	t.Cleanup(cancelRun)
	errCh := make(chan error, 1)
	go func() { errCh <- projector.Run(runCtx) }()
	waitForProjectorStarted(t, projector)

	ctx := testContext(t)
	seq, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), makeEvent("R1", "U1"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrProjectionFailed) {
			t.Fatalf("want ErrProjectionFailed, got %v", err)
		}
		if !errors.Is(err, applyErr) {
			t.Fatalf("want wrapped apply error, got %v", err)
		}
	case <-ctx.Done():
		t.Fatal("projector Run did not return after projection failure")
	}

	status := projector.Status()
	if !status.Failed {
		t.Fatal("Status.Failed = false, want true")
	}
	if status.FailedSeq != seq {
		t.Fatalf("Status.FailedSeq = %d, want %d", status.FailedSeq, seq)
	}
}

func TestProjector_RunFailsOnUnmarshalableEvent(t *testing.T) {
	js, stream := setupTestStream(t)
	proj := newTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancelRun := context.WithCancel(context.Background())
	t.Cleanup(cancelRun)
	errCh := make(chan error, 1)
	go func() { errCh <- projector.Run(runCtx) }()
	waitForProjectorStarted(t, projector)

	ctx := testContext(t)
	subject := RoomAggregate("R1").Subject(EventUserJoinedRoom)
	ack, err := js.Publish(ctx, subject, []byte{0xff},
		jetstream.WithExpectLastSequencePerSubject(0),
		jetstream.WithMsgID("bad-protobuf"),
	)
	if err != nil {
		t.Fatalf("raw Publish: %v", err)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrProjectionFailed) {
			t.Fatalf("want ErrProjectionFailed, got %v", err)
		}
	case <-ctx.Done():
		t.Fatal("projector Run did not return after decode failure")
	}

	status := projector.Status()
	if !status.Failed {
		t.Fatal("Status.Failed = false, want true")
	}
	if status.FailedSeq != ack.Sequence {
		t.Fatalf("Status.FailedSeq = %d, want %d", status.FailedSeq, ack.Sequence)
	}
	if got := projector.LastSeq(); got >= ack.Sequence {
		t.Fatalf("LastSeq=%d, want less than failed seq %d", got, ack.Sequence)
	}
}

// ============================================================================
// Subject helpers
// ============================================================================

func TestSubjectHelpers(t *testing.T) {
	t.Run("RoomAggregate Subject", func(t *testing.T) {
		got := RoomAggregate("ROOM123").Subject(EventUserJoinedRoom)
		want := "evt.room.ROOM123.user_joined"
		if got != want {
			t.Errorf("RoomAggregate.Subject: got %q, want %q", got, want)
		}
	})

	t.Run("AllEventsFilter", func(t *testing.T) {
		got := RoomAggregate("ROOM123").AllEventsFilter()
		want := "evt.room.ROOM123.>"
		if got != want {
			t.Errorf("AllEventsFilter: got %q, want %q", got, want)
		}
	})

	t.Run("SubjectFor derives event type", func(t *testing.T) {
		event := makeEvent("ROOM123", "U1")
		got := RoomAggregate("ROOM123").SubjectFor(event)
		want := "evt.room.ROOM123.user_joined"
		if got != want {
			t.Errorf("SubjectFor: got %q, want %q", got, want)
		}
	})

	t.Run("RoomAggregate call subject", func(t *testing.T) {
		got := RoomAggregate("ROOM123").Subject(EventCallParticipantJoined)
		want := "evt.room.ROOM123.call_joined"
		if got != want {
			t.Errorf("RoomAggregate.Subject(call): got %q, want %q", got, want)
		}
	})

	t.Run("RoomSubjectFilter", func(t *testing.T) {
		got := RoomSubjectFilter()
		want := "evt.room.>"
		if got != want {
			t.Errorf("RoomSubjectFilter: got %q, want %q", got, want)
		}
	})

	t.Run("EventSubjectFilter", func(t *testing.T) {
		got := EventSubjectFilter()
		want := "evt.>"
		if got != want {
			t.Errorf("EventSubjectFilter: got %q, want %q", got, want)
		}
	})

	t.Run("RoomEventTypeFilter", func(t *testing.T) {
		got := RoomEventTypeFilter(EventUserJoinedRoom)
		want := "evt.room.*.user_joined"
		if got != want {
			t.Errorf("RoomEventTypeFilter: got %q, want %q", got, want)
		}
	})

	t.Run("AggregateEventTypeFilter", func(t *testing.T) {
		got := AggregateEventTypeFilter(AggregateUser, EventUserDEKGenerated)
		want := "evt.user.*.dek_generated"
		if got != want {
			t.Errorf("AggregateEventTypeFilter: got %q, want %q", got, want)
		}
	})

	t.Run("ConfigEventTypeFilter", func(t *testing.T) {
		got := ConfigEventTypeFilter(EventServerNameChanged)
		want := "evt.config.*.server_name_changed"
		if got != want {
			t.Errorf("ConfigEventTypeFilter: got %q, want %q", got, want)
		}
	})

	t.Run("UserEventTypeFilter", func(t *testing.T) {
		got := UserEventTypeFilter(EventUserKeyShredded)
		want := "evt.user.*.user_key_shredded"
		if got != want {
			t.Errorf("UserEventTypeFilter: got %q, want %q", got, want)
		}
	})

	t.Run("ParseRoomSubject", func(t *testing.T) {
		cases := []struct {
			subject string
			wantID  string
			wantOK  bool
		}{
			{"evt.room.ROOM123.user_joined", "ROOM123", true},
			{"evt.room.ROOM123.call_joined", "ROOM123", true},
			{"live.evt.room.ROOM123.user_joined", "ROOM123", true},
			{"live.evt.room.ROOM123.call_left", "ROOM123", true},
			{"evt.user.U1.user_deleted", "", false},
			{"evt.room.", "", false},
			{"evt.room.ROOM123", "", false}, // missing event-type segment
			{"unrelated.subject", "", false},
			{"", "", false},
		}
		for _, c := range cases {
			id, ok := ParseRoomSubject(c.subject)
			if id != c.wantID || ok != c.wantOK {
				t.Errorf("ParseRoomSubject(%q) = (%q, %v), want (%q, %v)",
					c.subject, id, ok, c.wantID, c.wantOK)
			}
		}
	})

}

func TestSubjectMatchesFilter(t *testing.T) {
	cases := []struct {
		filter  string
		subject string
		want    bool
	}{
		{"evt.room.>", "evt.room.R1.user_joined", true},
		{"evt.room.*.user_joined", "evt.room.R1.user_joined", true},
		{"evt.room.*.user_joined", "evt.room.R1.message_posted", false},
		{"evt.room.*.user_joined", "evt.room.R1.extra.user_joined", false},
		{"evt.room.R1.user_joined", "evt.room.R1.user_joined", true},
		{"evt.room.R1.user_joined", "evt.room.R2.user_joined", false},
		{"evt.room.>", "evt.room", false},
		{">", "evt.room.R1.user_joined", true},
		{"", "evt.room.R1.user_joined", false},
		{"evt.room.>", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.filter+" matches "+tc.subject, func(t *testing.T) {
			if got := subjectMatchesFilter(tc.filter, tc.subject); got != tc.want {
				t.Fatalf("subjectMatchesFilter(%q, %q) = %v, want %v", tc.filter, tc.subject, got, tc.want)
			}
		})
	}
}

// ============================================================================
// Message events (issue #597 phase 1 — wire format lockdown)
// ============================================================================

// TestEventTypeOf_MessageEvents locks in the subject-token mapping for the
// durable message and shred event variants. These tokens become part of NATS
// subjects (evt.room.{R}.message_*) and persist on disk — once shipped,
// renaming requires a stream migration.
func TestEventTypeOf_MessageEvents(t *testing.T) {
	cases := []struct {
		name  string
		event *corev1.Event
		want  string
	}{
		{
			name: "MessagePosted",
			event: &corev1.Event{
				Event: &corev1.Event_MessagePosted{
					MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"},
				},
			},
			want: EventMessagePosted,
		},
		{
			name: "MessageEdited",
			event: &corev1.Event{
				Event: &corev1.Event_MessageEdited{
					MessageEdited: &corev1.MessageEditedEvent{RoomId: "R1", EventId: "M1"},
				},
			},
			want: EventMessageEdited,
		},
		{
			name: "MessageRetracted",
			event: &corev1.Event{
				Event: &corev1.Event_MessageRetracted{
					MessageRetracted: &corev1.MessageRetractedEvent{RoomId: "R1", EventId: "M1"},
				},
			},
			want: EventMessageRetracted,
		},
		{
			name: "ThreadCreated",
			event: &corev1.Event{
				Event: &corev1.Event_ThreadCreated{
					ThreadCreated: &corev1.ThreadCreatedEvent{RoomId: "R1", ThreadRootEventId: "M1"},
				},
			},
			want: EventThreadCreated,
		},
		{
			name: "CallStarted",
			event: &corev1.Event{
				Event: &corev1.Event_VoiceCallStarted{
					VoiceCallStarted: &corev1.CallStartedEvent{RoomId: "R1", CallId: "C1"},
				},
			},
			want: EventCallStarted,
		},
		{
			name: "CallParticipantJoined",
			event: &corev1.Event{
				Event: &corev1.Event_VoiceCallParticipantJoined{
					VoiceCallParticipantJoined: &corev1.CallParticipantJoinedEvent{RoomId: "R1", CallId: "C1"},
				},
			},
			want: EventCallParticipantJoined,
		},
		{
			name: "CallParticipantLeft",
			event: &corev1.Event{
				Event: &corev1.Event_VoiceCallParticipantLeft{
					VoiceCallParticipantLeft: &corev1.CallParticipantLeftEvent{RoomId: "R1", CallId: "C1"},
				},
			},
			want: EventCallParticipantLeft,
		},
		{
			name: "CallEnded",
			event: &corev1.Event{
				Event: &corev1.Event_VoiceCallEnded{
					VoiceCallEnded: &corev1.CallEndedEvent{RoomId: "R1", CallId: "C1"},
				},
			},
			want: EventCallEnded,
		},
		{
			name: "UserKeyShredded",
			event: &corev1.Event{
				Event: &corev1.Event_UserKeyShredded{
					UserKeyShredded: &corev1.UserKeyShreddedEvent{UserId: "U1"},
				},
			},
			want: EventUserKeyShredded,
		},
		{
			name: "UserDEKGenerated",
			event: &corev1.Event{
				Event: &corev1.Event_UserDekGenerated{
					UserDekGenerated: &corev1.UserDEKGeneratedEvent{UserId: "U1", Epoch: 1, Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY},
				},
			},
			want: EventUserDEKGenerated,
		},
		{
			name: "RegistrationVerificationCodeIssued",
			event: &corev1.Event{
				Event: &corev1.Event_RegistrationVerificationCodeIssued{
					RegistrationVerificationCodeIssued: &corev1.RegistrationVerificationCodeIssuedEvent{EmailHash: "hash"},
				},
			},
			want: EventRegistrationVerificationCodeIssued,
		},
		{
			name: "EmailVerificationCodeIssued",
			event: &corev1.Event{
				Event: &corev1.Event_EmailVerificationCodeIssued{
					EmailVerificationCodeIssued: &corev1.EmailVerificationCodeIssuedEvent{UserId: "U1", EmailHash: "hash"},
				},
			},
			want: EventEmailVerificationCodeIssued,
		},
		{
			name: "PasswordResetLinkIssued",
			event: &corev1.Event{
				Event: &corev1.Event_PasswordResetLinkIssued{
					PasswordResetLinkIssued: &corev1.PasswordResetLinkIssuedEvent{UserId: "U1", EmailHash: "hash"},
				},
			},
			want: EventPasswordResetLinkIssued,
		},
		{
			name: "AccountDeletionConfirmationIssued",
			event: &corev1.Event{
				Event: &corev1.Event_AccountDeletionConfirmationIssued{
					AccountDeletionConfirmationIssued: &corev1.AccountDeletionConfirmationIssuedEvent{UserId: "U1"},
				},
			},
			want: EventAccountDeletionConfirmationIssued,
		},
		{
			name: "PasswordResetCompleted",
			event: &corev1.Event{
				Event: &corev1.Event_PasswordResetCompleted{
					PasswordResetCompleted: &corev1.PasswordResetCompletedEvent{UserId: "U1"},
				},
			},
			want: EventPasswordResetCompleted,
		},
		{
			name: "LoginSucceeded",
			event: &corev1.Event{
				Event: &corev1.Event_LoginSucceeded{
					LoginSucceeded: &corev1.LoginSucceededEvent{UserId: "U1"},
				},
			},
			want: EventLoginSucceeded,
		},
		{
			name: "LoginFailed",
			event: &corev1.Event{
				Event: &corev1.Event_LoginFailed{
					LoginFailed: &corev1.LoginFailedEvent{IdentifierHash: "hash"},
				},
			},
			want: EventLoginFailed,
		},
		{
			name: "LogoutSucceeded",
			event: &corev1.Event{
				Event: &corev1.Event_LogoutSucceeded{
					LogoutSucceeded: &corev1.LogoutSucceededEvent{UserId: "U1"},
				},
			},
			want: EventLogoutSucceeded,
		},
		{
			name: "AuthCodeIssued",
			event: &corev1.Event{
				Event: &corev1.Event_AuthCodeIssued{
					AuthCodeIssued: &corev1.AuthCodeIssuedEvent{UserId: "U1", RedirectUriHash: "hash"},
				},
			},
			want: EventAuthCodeIssued,
		},
		{
			name: "AuthCodeExchangeSucceeded",
			event: &corev1.Event{
				Event: &corev1.Event_AuthCodeExchangeSucceeded{
					AuthCodeExchangeSucceeded: &corev1.AuthCodeExchangeSucceededEvent{UserId: "U1", RedirectUriHash: "hash"},
				},
			},
			want: EventAuthCodeExchangeSucceeded,
		},
		{
			name: "AuthCodeExchangeFailed",
			event: &corev1.Event{
				Event: &corev1.Event_AuthCodeExchangeFailed{
					AuthCodeExchangeFailed: &corev1.AuthCodeExchangeFailedEvent{UserId: "U1", RedirectUriHash: "hash", Reason: "invalid_verifier"},
				},
			},
			want: EventAuthCodeExchangeFailed,
		},
		{
			name: "BearerTokenIssued",
			event: &corev1.Event{
				Event: &corev1.Event_BearerTokenIssued{
					BearerTokenIssued: &corev1.BearerTokenIssuedEvent{UserId: "U1", Source: "password_login"},
				},
			},
			want: EventBearerTokenIssued,
		},
		{
			name: "BearerTokenRevoked",
			event: &corev1.Event{
				Event: &corev1.Event_BearerTokenRevoked{
					BearerTokenRevoked: &corev1.BearerTokenRevokedEvent{UserId: "U1", Reason: "logout"},
				},
			},
			want: EventBearerTokenRevoked,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EventTypeOf(c.event); got != c.want {
				t.Errorf("EventTypeOf = %q, want %q", got, c.want)
			}
			agg := RoomAggregate("ROOM123")
			if c.want == EventUserKeyShredded || c.want == EventUserDEKGenerated {
				agg = UserAggregate("U1")
			}
			if c.want == EventRegistrationVerificationCodeIssued {
				agg = AuthAggregate()
			}
			if c.want == EventEmailVerificationCodeIssued ||
				c.want == EventPasswordResetLinkIssued ||
				c.want == EventAccountDeletionConfirmationIssued ||
				c.want == EventPasswordResetCompleted ||
				c.want == EventLoginSucceeded ||
				c.want == EventLogoutSucceeded ||
				c.want == EventAuthCodeIssued ||
				c.want == EventAuthCodeExchangeSucceeded ||
				c.want == EventAuthCodeExchangeFailed ||
				c.want == EventBearerTokenIssued ||
				c.want == EventBearerTokenRevoked {
				agg = UserAggregate("U1")
			}
			if c.want == EventLoginFailed {
				agg = AuthAggregate()
			}
			subject := agg.SubjectFor(c.event)
			wantSubject := agg.Subject(c.want)
			if subject != wantSubject {
				t.Errorf("SubjectFor = %q, want %q", subject, wantSubject)
			}
		})
	}
}

func TestAuthAggregate_Subject(t *testing.T) {
	got := AuthAggregate().Subject(EventRegistrationVerificationCodeIssued)
	want := "evt.auth.server.registration_verification_code_issued"
	if got != want {
		t.Fatalf("AuthAggregate subject = %q, want %q", got, want)
	}
	if AuthSubjectFilter() != "evt.auth.>" {
		t.Fatalf("AuthSubjectFilter = %q", AuthSubjectFilter())
	}
}

func TestMessagePostedEvent_RemovedLegacyMessageBodyIDRoundTripsUnknown(t *testing.T) {
	var legacyBytes []byte
	legacyBytes = protowire.AppendTag(legacyBytes, 2, protowire.BytesType)
	legacyBytes = protowire.AppendString(legacyBytes, "R1")
	legacyBytes = protowire.AppendTag(legacyBytes, 3, protowire.BytesType)
	legacyBytes = protowire.AppendString(legacyBytes, "U1.M1")

	var decoded corev1.MessagePostedEvent
	if err := proto.Unmarshal(legacyBytes, &decoded); err != nil {
		t.Fatalf("unmarshal legacy under new schema: %v", err)
	}

	if decoded.GetRoomId() != "R1" {
		t.Errorf("RoomId = %q, want R1", decoded.GetRoomId())
	}
	if got := decoded.ProtoReflect().GetUnknown(); len(got) == 0 {
		t.Fatal("expected legacy message_body_id to remain in unknown fields")
	}
}

func TestEventOneofDurableFieldNumberPolicy(t *testing.T) {
	allowedHighDurableTags := map[protoreflect.Name]protoreflect.FieldNumber{
		"reaction_added":   1050,
		"reaction_removed": 1051,
	}

	desc := (&corev1.Event{}).ProtoReflect().Descriptor()
	oneof := desc.Oneofs().ByName("event")
	if oneof == nil {
		t.Fatal("Event oneof not found")
	}

	fields := oneof.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		number := field.Number()
		if number < 1000 {
			continue
		}
		if allowed, ok := allowedHighDurableTags[field.Name()]; ok && number == allowed {
			continue
		}
		t.Errorf("Event.%s uses field number %d; durable Event variants must stay below 1000 except reaction_added=1050/reaction_removed=1051", field.Name(), number)
	}
}

func TestRemovedEventShapeFieldsRemainReserved(t *testing.T) {
	eventDesc := (&corev1.Event{}).ProtoReflect().Descriptor()
	if !eventDesc.ReservedRanges().Has(9001) {
		t.Error("Event tag 9001 must stay reserved for removed sequence_id")
	}
	if !eventDesc.ReservedNames().Has("sequence_id") {
		t.Error("Event name sequence_id must stay reserved")
	}

	postedDesc := (&corev1.MessagePostedEvent{}).ProtoReflect().Descriptor()
	if !postedDesc.ReservedRanges().Has(3) {
		t.Error("MessagePostedEvent tag 3 must stay reserved for removed message_body_id")
	}
	if !postedDesc.ReservedRanges().Has(9) {
		t.Error("MessagePostedEvent tag 9 must stay reserved for removed body")
	}
	if !postedDesc.ReservedNames().Has("message_body_id") {
		t.Error("MessagePostedEvent name message_body_id must stay reserved")
	}
	if !postedDesc.ReservedNames().Has("body") {
		t.Error("MessagePostedEvent name body must stay reserved")
	}
	if postedDesc.Fields().ByName("message_body_id") != nil {
		t.Error("MessagePostedEvent must not reintroduce message_body_id")
	}
	if postedDesc.Fields().ByName("body") != nil {
		t.Error("MessagePostedEvent must not reintroduce body")
	}
	if postedDesc.Fields().ByName("event_id") != nil {
		t.Error("MessagePostedEvent must not reintroduce event_id")
	}

	editedDesc := (&corev1.MessageEditedEvent{}).ProtoReflect().Descriptor()
	if !editedDesc.ReservedRanges().Has(3) {
		t.Error("MessageEditedEvent tag 3 must stay reserved for removed body")
	}
	if !editedDesc.ReservedNames().Has("body") {
		t.Error("MessageEditedEvent name body must stay reserved")
	}
	if editedDesc.Fields().ByName("body") != nil {
		t.Error("MessageEditedEvent must not reintroduce body")
	}

	updatedDesc := (&corev1.MessageUpdatedEvent{}).ProtoReflect().Descriptor()
	if !updatedDesc.ReservedRanges().Has(3) {
		t.Error("MessageUpdatedEvent tag 3 must stay reserved for removed sequence_id")
	}
	if !updatedDesc.ReservedNames().Has("sequence_id") {
		t.Error("MessageUpdatedEvent name sequence_id must stay reserved")
	}
}

// ============================================================================
// Helpers
// ============================================================================

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

// itoa is a tiny helper so the tests don't need strconv just for short IDs.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	negative := i < 0
	if negative {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
