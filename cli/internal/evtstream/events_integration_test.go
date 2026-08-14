package evtstream_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
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

	. "hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
	. "hmans.de/chatto/pkg/events"
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

func testStreamIdentity(t *testing.T, stream jetstream.Stream) string {
	t.Helper()
	if stream == nil {
		t.Fatal("test stream is nil")
	}
	return "test-application/stream-incarnation-1"
}

func fixedStreamIdentity(identity string) StreamIdentityResolver {
	return func(*jetstream.StreamInfo) (string, error) {
		return identity, nil
	}
}

func createdStreamIdentity(info *jetstream.StreamInfo) (string, error) {
	if info == nil || info.Created.IsZero() {
		return "", errors.New("stream creation time is unavailable")
	}
	return fmt.Sprintf("test-application/created/%d", info.Created.UnixNano()), nil
}

func testCreatedStreamIdentity(t *testing.T, ctx context.Context, stream jetstream.Stream) string {
	t.Helper()
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := createdStreamIdentity(info)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func recreateTestStream(t *testing.T, ctx context.Context, js jetstream.JetStream) jetstream.Stream {
	t.Helper()
	if err := js.DeleteStream(ctx, "EVT_TEST"); err != nil {
		t.Fatalf("delete original stream: %v", err)
	}
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:               "EVT_TEST",
		Subjects:           []string{SubjectRoot + ">"},
		Storage:            jetstream.FileStorage,
		AllowAtomicPublish: true,
	})
	if err != nil {
		t.Fatalf("recreate stream: %v", err)
	}
	return stream
}

func appendRecreatedStreamEvent(t *testing.T, ctx context.Context, js jetstream.JetStream, stream jetstream.Stream) {
	t.Helper()
	pub := NewPublisher(js, stream, testLogger())
	if _, err := pub.AppendEventually(ctx, RoomAggregate("R-recreated").Subject(EventUserJoinedRoom), makeEvent("R-recreated", "U1")); err != nil {
		t.Fatal(err)
	}
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
		wantExpectedSeq := ""
		if entries[i].HasOCC {
			wantExpectedSeq = fmt.Sprintf("%d", entries[i].ExpectedSeq)
		}
		if got := msg.Header.Get(jetstream.ExpectedLastSubjSeqHeader); got != wantExpectedSeq {
			t.Errorf("batch msg %d expected-last-subject-sequence = %q, want %q", i, got, wantExpectedSeq)
		}
		wantData, err := proto.Marshal(entries[i].Event)
		if err != nil {
			t.Fatalf("proto.Marshal batch entry %d: %v", i, err)
		}
		if !slices.Equal(msg.Data, wantData) {
			t.Errorf("batch msg %d data changed:\n got %x\nwant %x", i, msg.Data, wantData)
		}
	}

	// Each subject's last seq must match what we published.
	for i, e := range entries {
		got, err := pub.LastSubjectSeq(ctx, e.Subject)
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
	gotA, _ := pub.LastSubjectSeq(ctx, GroupAggregate("GA").Subject(EventUserJoinedRoom))
	if gotA != seqA {
		t.Errorf("GA last seq = %d, want %d (unchanged)", gotA, seqA)
	}
	gotB, _ := pub.LastSubjectSeq(ctx, GroupAggregate("GB").Subject(EventUserJoinedRoom))
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
	mu                sync.Mutex
	events            []*corev1.Event
	seqs              []uint64
	subs              []string
	replayCompletions int
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

func (p *trackingProjection) CompleteStartupReplay() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.replayCompletions++
}

func (p *trackingProjection) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

func (p *trackingProjection) ReplayCompletions() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.replayCompletions
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

type countingSubjectsProjection struct {
	*trackingProjection
	subjectCalls int
}

type minimalProjection struct {
	mu      sync.Mutex
	count   int
	subject string
}

func (p *minimalProjection) Subjects() []string { return []string{p.subject} }

func (p *minimalProjection) Apply(*corev1.Event, uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.count++
	return nil
}

func (p *minimalProjection) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}

type checkpointTrackingProjection struct {
	*trackingProjection
	contractID             string
	checkpoint             uint64
	expectedStreamIdentity string
	restoreErr             error
	resetErr               error
	request                ProjectionCheckpointRequest
	resets                 int
}

type startupBatchTrackingProjection struct {
	*trackingProjection
	batchSize int
	batchErr  error
	batchMu   sync.Mutex
	batches   [][]uint64
	liveCalls int
}

func newStartupBatchTrackingProjection(batchSize int, subject string) *startupBatchTrackingProjection {
	return &startupBatchTrackingProjection{
		trackingProjection: newTrackingProjection(subject),
		batchSize:          batchSize,
	}
}

func (p *startupBatchTrackingProjection) StartupBatchSize() int { return p.batchSize }

func (p *startupBatchTrackingProjection) ApplyStartupBatch(items []SequencedEvent) error {
	if p.batchErr != nil {
		return p.batchErr
	}
	seqs := make([]uint64, 0, len(items))
	for _, item := range items {
		if err := p.trackingProjection.Apply(item.Event, item.Sequence); err != nil {
			return err
		}
		seqs = append(seqs, item.Sequence)
	}
	p.batchMu.Lock()
	p.batches = append(p.batches, seqs)
	p.batchMu.Unlock()
	return nil
}

func (p *startupBatchTrackingProjection) Apply(event *corev1.Event, seq uint64) error {
	p.batchMu.Lock()
	p.liveCalls++
	p.batchMu.Unlock()
	return p.trackingProjection.Apply(event, seq)
}

func (p *startupBatchTrackingProjection) BatchSequences() [][]uint64 {
	p.batchMu.Lock()
	defer p.batchMu.Unlock()
	result := make([][]uint64, len(p.batches))
	for i := range p.batches {
		result[i] = append([]uint64(nil), p.batches[i]...)
	}
	return result
}

func (p *startupBatchTrackingProjection) LiveCalls() int {
	p.batchMu.Lock()
	defer p.batchMu.Unlock()
	return p.liveCalls
}

func newCheckpointTrackingProjection(subject string) *checkpointTrackingProjection {
	return &checkpointTrackingProjection{
		trackingProjection: newTrackingProjection(subject),
		contractID:         "checkpoint-v1",
	}
}

func (p *checkpointTrackingProjection) CheckpointContractID() string { return p.contractID }
func (*checkpointTrackingProjection) SnapshotContractID() string     { return "snapshot-v1" }

func (p *checkpointTrackingProjection) RestoreCheckpoint(_ context.Context, request ProjectionCheckpointRequest) (ProjectionCheckpoint, error) {
	p.request = request
	if p.expectedStreamIdentity != "" && request.StreamIdentity != p.expectedStreamIdentity {
		return ProjectionCheckpoint{}, fmt.Errorf("%w: stream identity changed", ErrProjectionCheckpointInvalid)
	}
	return ProjectionCheckpoint{CutoffSequence: p.checkpoint}, p.restoreErr
}

func (p *checkpointTrackingProjection) ResetCheckpoint(_ context.Context, request ProjectionCheckpointRequest) error {
	p.request = request
	p.resets++
	p.checkpoint = 0
	return p.resetErr
}

func newCountingSubjectsProjection(subs ...string) *countingSubjectsProjection {
	return &countingSubjectsProjection{
		trackingProjection: newTrackingProjection(subs...),
	}
}

func (p *countingSubjectsProjection) Subjects() []string {
	p.subjectCalls++
	return p.trackingProjection.Subjects()
}

type blockingProjection struct {
	*trackingProjection
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

type snapshotTrackingProjection struct {
	*trackingProjection
	restored   []byte
	restoreErr error
	snapshot   []byte
	contractID string
}

type snapshotReplayTrackingProjection struct {
	*snapshotTrackingProjection
	replay []string
}

func (p *snapshotReplayTrackingProjection) ReplaySubjects() []string { return p.replay }

func newSnapshotTrackingProjection(subs ...string) *snapshotTrackingProjection {
	return &snapshotTrackingProjection{trackingProjection: newTrackingProjection(subs...), snapshot: []byte("captured"), contractID: "tracking-v1"}
}

func (p *snapshotTrackingProjection) SnapshotContractID() string { return p.contractID }
func (p *snapshotTrackingProjection) Snapshot() ([]byte, error) {
	return append([]byte(nil), p.snapshot...), nil
}
func (p *snapshotTrackingProjection) Restore(data []byte) error {
	if len(data) > 0 && p.restoreErr != nil {
		return p.restoreErr
	}
	p.restored = append([]byte(nil), data...)
	return nil
}

type staticSnapshotSource struct {
	snapshot ProjectionSnapshot
	err      error
	request  ProjectionSnapshotLoadRequest
}

type identityBoundSnapshotSource struct {
	streamIdentity string
	snapshot       ProjectionSnapshot
	request        ProjectionSnapshotLoadRequest
}

type blockingSnapshotSource struct {
	canceled chan struct{}
}

type gatedSnapshotSource struct {
	started  chan struct{}
	release  chan struct{}
	snapshot ProjectionSnapshot
}

func completeTestSnapshot(snapshot ProjectionSnapshot, request ProjectionSnapshotLoadRequest) ProjectionSnapshot {
	if snapshot.ContractID == "" {
		snapshot.ContractID = request.ContractID
	}
	if snapshot.StreamName == "" {
		snapshot.StreamName = request.StreamName
	}
	if snapshot.StreamIdentity == "" {
		snapshot.StreamIdentity = request.StreamIdentity
	}
	return snapshot
}

func (s *gatedSnapshotSource) LoadProjectionSnapshot(ctx context.Context, request ProjectionSnapshotLoadRequest) (ProjectionSnapshot, error) {
	close(s.started)
	select {
	case <-s.release:
		return completeTestSnapshot(s.snapshot, request), nil
	case <-ctx.Done():
		return ProjectionSnapshot{}, ctx.Err()
	}
}

func (s *blockingSnapshotSource) LoadProjectionSnapshot(ctx context.Context, _ ProjectionSnapshotLoadRequest) (ProjectionSnapshot, error) {
	<-ctx.Done()
	close(s.canceled)
	return ProjectionSnapshot{}, ctx.Err()
}

func (s *staticSnapshotSource) LoadProjectionSnapshot(_ context.Context, request ProjectionSnapshotLoadRequest) (ProjectionSnapshot, error) {
	s.request = request
	return completeTestSnapshot(s.snapshot, request), s.err
}

func (s *identityBoundSnapshotSource) LoadProjectionSnapshot(_ context.Context, request ProjectionSnapshotLoadRequest) (ProjectionSnapshot, error) {
	s.request = request
	if request.StreamIdentity != s.streamIdentity {
		return ProjectionSnapshot{}, errors.New("snapshot stream identity changed")
	}
	return completeTestSnapshot(s.snapshot, request), nil
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
	if got := proj.ReplayCompletions(); got != 1 {
		t.Errorf("startup replay completions = %d, want 1", got)
	}
}

func TestProjectorSkipsBroadReplaySubjectsBeforeDecoding(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	if _, err := js.Publish(ctx, ConfigAggregate().Subject(EventServerNameChanged), []byte("not protobuf")); err != nil {
		t.Fatalf("publish malformed unrelated event: %v", err)
	}
	pub := NewPublisher(js, stream, testLogger())
	if _, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), makeEvent("R1", "U1")); err != nil {
		t.Fatalf("publish logical event: %v", err)
	}

	projection := newReplayTrackingProjection(
		[]string{RoomEventTypeFilter(EventUserJoinedRoom)},
		[]string{EventSubjectFilter()},
	)
	projector := NewProjector(js, stream, projection, testLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	waitFor(t, 2*time.Second, func() bool { return projector.Status().StartupComplete })
	if err := projector.Err(); err != nil {
		t.Fatalf("broad physical replay decoded unrelated payload: %v", err)
	}
	if got := projection.Count(); got != 1 {
		t.Fatalf("Apply count = %d, want 1 logical event", got)
	}
}

func TestProjectorRunsProjectionWithoutSnapshotMethods(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	subject := RoomAggregate("R-minimal").Subject(EventUserJoinedRoom)
	if _, err := pub.AppendEventually(ctx, subject, makeEvent("R-minimal", "U1")); err != nil {
		t.Fatalf("AppendEventually: %v", err)
	}

	projection := &minimalProjection{subject: subject}
	projector := NewProjector(js, stream, projection, testLogger())
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete
	})
	if got := projection.Count(); got != 1 {
		t.Fatalf("Apply count = %d, want 1", got)
	}
	if _, err := projector.CaptureSnapshot(context.Background()); err == nil {
		t.Fatal("CaptureSnapshot succeeded for projection without snapshot methods")
	}
}

func TestProjectorBatchesOnlyCapturedStartupReplay(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	subject := RoomAggregate("R-batch").Subject(EventUserJoinedRoom)
	var seqs []uint64
	for i := range 5 {
		seq, err := pub.AppendEventually(ctx, subject, makeEvent("R-batch", "U"+itoa(i)))
		if err != nil {
			t.Fatalf("AppendEventually %d: %v", i, err)
		}
		seqs = append(seqs, seq)
	}

	projection := newStartupBatchTrackingProjection(2, subject)
	projector := NewProjector(js, stream, projection, testLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool { return projector.Status().StartupComplete })

	wantBatches := [][]uint64{{seqs[0], seqs[1]}, {seqs[2], seqs[3]}, {seqs[4]}}
	if got := projection.BatchSequences(); !reflect.DeepEqual(got, wantBatches) {
		t.Fatalf("startup batches = %v, want %v", got, wantBatches)
	}
	if got := projection.LiveCalls(); got != 0 {
		t.Fatalf("live Apply calls during startup = %d, want 0", got)
	}
	if got := projector.Status().StartupMessages; got != 5 {
		t.Fatalf("startup messages = %d, want 5", got)
	}

	liveSeq, err := pub.AppendEventually(ctx, subject, makeEvent("R-batch", "U-live"))
	if err != nil {
		t.Fatalf("append live event: %v", err)
	}
	if err := projector.WaitFor(ctx, SubjectPosition(subject, liveSeq)); err != nil {
		t.Fatalf("wait for live event: %v", err)
	}
	if got := projection.LiveCalls(); got != 1 {
		t.Fatalf("live Apply calls after startup = %d, want 1", got)
	}
	if got := projection.BatchSequences(); !reflect.DeepEqual(got, wantBatches) {
		t.Fatalf("live event changed startup batches: %v", got)
	}
}

func TestProjectorStartupBatchFailureStartsAtFirstUncommittedSequence(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	subject := RoomAggregate("R-batch-fail").Subject(EventUserJoinedRoom)
	firstSeq, err := pub.AppendEventually(ctx, subject, makeEvent("R-batch-fail", "U1"))
	if err != nil {
		t.Fatalf("append first event: %v", err)
	}
	if _, err := pub.AppendEventually(ctx, subject, makeEvent("R-batch-fail", "U2")); err != nil {
		t.Fatalf("append second event: %v", err)
	}

	applyErr := errors.New("batch apply failed")
	projection := newStartupBatchTrackingProjection(2, subject)
	projection.batchErr = applyErr
	projector := NewProjector(js, stream, projection, testLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	errCh := make(chan error, 1)
	go func() { errCh <- projector.Run(runCtx) }()

	select {
	case err := <-errCh:
		if !errors.Is(err, applyErr) {
			t.Fatalf("Run error = %v, want batch error", err)
		}
	case <-ctx.Done():
		t.Fatal("projector did not fail after startup batch error")
	}
	status := projector.Status()
	if status.FailedSeq != firstSeq || status.LastSeq >= firstSeq {
		t.Fatalf("failed startup batch status = %+v, want failure at %d before advancement", status, firstSeq)
	}
}

func TestProjectorDecodeFailureIncludesPendingStartupBatch(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	subject := RoomAggregate("R-batch-decode").Subject(EventUserJoinedRoom)
	firstSeq, err := pub.AppendEventually(ctx, subject, makeEvent("R-batch-decode", "U1"))
	if err != nil {
		t.Fatalf("append first event: %v", err)
	}
	if _, err := js.Publish(ctx, subject, []byte{0xff}, jetstream.WithExpectLastSequencePerSubject(firstSeq), jetstream.WithMsgID("bad-batch-protobuf")); err != nil {
		t.Fatalf("publish malformed event: %v", err)
	}

	projection := newStartupBatchTrackingProjection(3, subject)
	projector := NewProjector(js, stream, projection, testLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	errCh := make(chan error, 1)
	go func() { errCh <- projector.Run(runCtx) }()

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrProjectionFailed) {
			t.Fatalf("Run error = %v, want projection failure", err)
		}
	case <-ctx.Done():
		t.Fatal("projector did not fail after malformed batched event")
	}
	status := projector.Status()
	if status.FailedSeq != firstSeq || status.LastSeq >= firstSeq || projection.Count() != 0 {
		t.Fatalf("decode failure status = %+v count=%d, want pending batch uncommitted from %d", status, projection.Count(), firstSeq)
	}
}

func TestProjectorRestoresLocalCheckpointAndReplaysTail(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	subject := RoomAggregate("R-checkpoint").Subject(EventUserJoinedRoom)
	var seqs []uint64
	for _, userID := range []string{"U1", "U2", "U3"} {
		seq, err := pub.AppendEventually(ctx, subject, makeEvent("R-checkpoint", userID))
		if err != nil {
			t.Fatalf("AppendEventually %s: %v", userID, err)
		}
		seqs = append(seqs, seq)
	}

	projection := newCheckpointTrackingProjection(subject)
	projection.checkpoint = seqs[1]
	projector := NewProjector(js, stream, projection, testLogger())
	if err := projector.ConfigureCheckpoint("search", fixedStreamIdentity(testStreamIdentity(t, stream))); err != nil {
		t.Fatalf("ConfigureCheckpoint: %v", err)
	}
	// Configuration captures the contract before the projection can change it.
	projection.contractID = "checkpoint-v2"

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete
	})

	if got := projection.Count(); got != 1 {
		t.Fatalf("tail Apply count = %d, want 1", got)
	}
	status := projector.Status()
	if !status.CheckpointRestored || status.CheckpointCutoffSeq != seqs[1] || status.CheckpointContractID != "checkpoint-v1" {
		t.Fatalf("checkpoint status = %+v", status)
	}
	if status.SnapshotRestored {
		t.Fatalf("checkpoint restore reported as snapshot restore: %+v", status)
	}
	if projection.request.ProjectionKey != "search" || projection.request.ContractID != "checkpoint-v1" {
		t.Fatalf("checkpoint request = %+v", projection.request)
	}
	if projection.request.StreamIdentity != testStreamIdentity(t, stream) || projection.request.FirstSequence != seqs[0] || projection.request.LastSequence != seqs[2] {
		t.Fatalf("checkpoint stream request = %+v", projection.request)
	}
}

func TestProjectorRestoresLocalCheckpointBeyondFilteredTail(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	subject := RoomAggregate("R-checkpoint-filtered-tail").Subject(EventUserJoinedRoom)
	matchingSeq, err := pub.AppendEventually(ctx, subject, makeEvent("R-checkpoint-filtered-tail", "U1"))
	if err != nil {
		t.Fatalf("append matching event: %v", err)
	}
	unrelatedSubject := RoomAggregate("R-checkpoint-unrelated").Subject(EventMessagePosted)
	unrelatedSeq, err := pub.AppendEventually(ctx, unrelatedSubject, makeEvent("R-checkpoint-unrelated", "U2"))
	if err != nil {
		t.Fatalf("append unrelated event: %v", err)
	}

	projection := newCheckpointTrackingProjection(subject)
	projection.checkpoint = unrelatedSeq
	projector := NewProjector(js, stream, projection, testLogger())
	if err := projector.ConfigureCheckpoint("search", fixedStreamIdentity(testStreamIdentity(t, stream))); err != nil {
		t.Fatalf("ConfigureCheckpoint: %v", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete
	})

	if projection.resets != 0 || projection.Count() != 0 {
		t.Fatalf("resets/count = %d/%d, want 0/0", projection.resets, projection.Count())
	}
	status := projector.Status()
	if !status.CheckpointRestored || status.CheckpointCutoffSeq != unrelatedSeq {
		t.Fatalf("checkpoint status = %+v, want restored cutoff %d", status, unrelatedSeq)
	}
	if status.StartupTargetSeq != matchingSeq || status.LastSeq != unrelatedSeq {
		t.Fatalf("checkpoint status = %+v, want filtered target %d behind restored cutoff %d", status, matchingSeq, unrelatedSeq)
	}
}

func TestProjectorResetsInvalidLocalCheckpoint(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	subject := RoomAggregate("R-checkpoint-reset").Subject(EventUserJoinedRoom)
	for _, userID := range []string{"U1", "U2"} {
		if _, err := pub.AppendEventually(ctx, subject, makeEvent("R-checkpoint-reset", userID)); err != nil {
			t.Fatalf("AppendEventually %s: %v", userID, err)
		}
	}

	projection := newCheckpointTrackingProjection(subject)
	projection.restoreErr = fmt.Errorf("%w: contract mismatch", ErrProjectionCheckpointInvalid)
	projector := NewProjector(js, stream, projection, testLogger())
	if err := projector.ConfigureCheckpoint("search", fixedStreamIdentity(testStreamIdentity(t, stream))); err != nil {
		t.Fatalf("ConfigureCheckpoint: %v", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete
	})
	if projection.resets != 1 || projection.Count() != 2 {
		t.Fatalf("resets/count = %d/%d, want 1/2", projection.resets, projection.Count())
	}
	if projector.Status().CheckpointRestored {
		t.Fatal("invalid checkpoint reported as restored")
	}
}

func TestProjectorDoesNotResetCheckpointOnOperationalRestoreFailure(t *testing.T) {
	js, stream := setupTestStream(t)
	projection := newCheckpointTrackingProjection(RoomSubjectFilter())
	projection.restoreErr = errors.New("local volume unavailable")
	projector := NewProjector(js, stream, projection, testLogger())
	if err := projector.ConfigureCheckpoint("search", fixedStreamIdentity(testStreamIdentity(t, stream))); err != nil {
		t.Fatalf("ConfigureCheckpoint: %v", err)
	}

	err := projector.Run(testContext(t))
	if err == nil || !strings.Contains(err.Error(), "local volume unavailable") {
		t.Fatalf("Run error = %v, want local volume failure", err)
	}
	if projection.resets != 0 {
		t.Fatalf("checkpoint resets = %d, want 0", projection.resets)
	}
	status := projector.Status()
	if !status.Failed || status.Err == nil {
		t.Fatalf("restore failure status = %+v, want failed projector", status)
	}
}

func TestProjectorResetsFutureLocalCheckpoint(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	subject := RoomAggregate("R-checkpoint-future").Subject(EventUserJoinedRoom)
	seq, err := pub.AppendEventually(ctx, subject, makeEvent("R-checkpoint-future", "U1"))
	if err != nil {
		t.Fatalf("AppendEventually: %v", err)
	}

	projection := newCheckpointTrackingProjection(subject)
	projection.checkpoint = seq + 1
	projector := NewProjector(js, stream, projection, testLogger())
	if err := projector.ConfigureCheckpoint("search", fixedStreamIdentity(testStreamIdentity(t, stream))); err != nil {
		t.Fatalf("ConfigureCheckpoint: %v", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete
	})
	if projection.resets != 1 || projection.Count() != 1 {
		t.Fatalf("resets/count = %d/%d, want 1/1", projection.resets, projection.Count())
	}
}

func TestProjectorResetsCheckpointBehindRetainedEVT(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	subject := RoomAggregate("R-checkpoint-retention-gap").Subject(EventUserJoinedRoom)
	var seqs []uint64
	for _, userID := range []string{"U1", "U2", "U3"} {
		seq, err := pub.AppendEventually(ctx, subject, makeEvent("R-checkpoint-retention-gap", userID))
		if err != nil {
			t.Fatalf("AppendEventually %s: %v", userID, err)
		}
		seqs = append(seqs, seq)
	}
	if err := stream.Purge(ctx, jetstream.WithPurgeSequence(seqs[2])); err != nil {
		t.Fatalf("purge EVT before sequence %d: %v", seqs[2], err)
	}

	projection := newCheckpointTrackingProjection(subject)
	projection.checkpoint = seqs[0]
	projector := NewProjector(js, stream, projection, testLogger())
	if err := projector.ConfigureCheckpoint("search", fixedStreamIdentity(testStreamIdentity(t, stream))); err != nil {
		t.Fatalf("ConfigureCheckpoint: %v", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete
	})

	if projection.resets != 1 || projection.Count() != 1 {
		t.Fatalf("resets/count = %d/%d, want reset plus one retained event", projection.resets, projection.Count())
	}
	if projector.Status().CheckpointRestored {
		t.Fatal("retention-gapped checkpoint reported as restored")
	}
}

func TestProjectorResolvesCheckpointIdentityWithFreshStreamBounds(t *testing.T) {
	js, originalStream := setupTestStream(t)
	ctx := testContext(t)
	originalIdentity := testCreatedStreamIdentity(t, ctx, originalStream)

	projection := newCheckpointTrackingProjection(RoomSubjectFilter())
	projection.checkpoint = 1
	projection.expectedStreamIdentity = originalIdentity
	projector := NewProjector(js, originalStream, projection, testLogger())
	if err := projector.ConfigureCheckpoint("search", createdStreamIdentity); err != nil {
		t.Fatal(err)
	}

	recreatedStream := recreateTestStream(t, ctx, js)
	recreatedIdentity := testCreatedStreamIdentity(t, ctx, recreatedStream)
	if recreatedIdentity == originalIdentity {
		t.Fatalf("recreated identity = original identity %q", originalIdentity)
	}
	appendRecreatedStreamEvent(t, ctx, js, recreatedStream)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete
	})

	if projection.request.StreamIdentity != recreatedIdentity {
		t.Fatalf("checkpoint request identity = %q, want recreated %q", projection.request.StreamIdentity, recreatedIdentity)
	}
	if projection.resets != 1 || projection.Count() != 1 {
		t.Fatalf("checkpoint resets/count = %d/%d, want 1/1", projection.resets, projection.Count())
	}
}

func TestProjectorResolvesSnapshotIdentityFromRecreatedStream(t *testing.T) {
	js, originalStream := setupTestStream(t)
	ctx := testContext(t)
	originalIdentity := testCreatedStreamIdentity(t, ctx, originalStream)

	projection := newSnapshotTrackingProjection(RoomSubjectFilter())
	source := &identityBoundSnapshotSource{
		streamIdentity: originalIdentity,
		snapshot: ProjectionSnapshot{
			GenerationID:   "old-generation",
			CutoffSequence: 1,
			Payload:        []byte("old-state"),
		},
	}
	projector := NewProjector(js, originalStream, projection, testLogger())
	if err := projector.ConfigureSnapshots("tracking", source, createdStreamIdentity); err != nil {
		t.Fatal(err)
	}

	recreatedStream := recreateTestStream(t, ctx, js)
	recreatedIdentity := testCreatedStreamIdentity(t, ctx, recreatedStream)
	if recreatedIdentity == originalIdentity {
		t.Fatalf("recreated identity = original identity %q", originalIdentity)
	}
	appendRecreatedStreamEvent(t, ctx, js, recreatedStream)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete
	})

	if source.request.StreamIdentity != recreatedIdentity {
		t.Fatalf("snapshot request identity = %q, want recreated %q", source.request.StreamIdentity, recreatedIdentity)
	}
	if projector.Status().SnapshotRestored || projection.Count() != 1 || len(projection.restored) != 0 {
		t.Fatalf("snapshot status/count/restored = %+v/%d/%q, want cold replay of recreated stream", projector.Status(), projection.Count(), projection.restored)
	}
	captured, err := projector.CaptureSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if captured.StreamIdentity != recreatedIdentity {
		t.Fatalf("captured snapshot identity = %q, want recreated %q", captured.StreamIdentity, recreatedIdentity)
	}
	recreateTestStream(t, ctx, js)
	if _, err := projector.CaptureSnapshot(ctx); err == nil || !strings.Contains(err.Error(), "stream identity changed") {
		t.Fatalf("CaptureSnapshot after another recreation error = %v, want stream identity change", err)
	}
}

func TestProjectorSnapshotPublicationRecoversAfterTransientIdentityFailure(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	pub := NewPublisher(js, stream, testLogger())
	if _, err := pub.AppendEventually(ctx, RoomAggregate("R-transient").Subject(EventUserJoinedRoom), makeEvent("R-transient", "U1")); err != nil {
		t.Fatal(err)
	}

	resolverCalls := 0
	resolveIdentity := func(info *jetstream.StreamInfo) (string, error) {
		resolverCalls++
		if resolverCalls == 2 {
			return "", errors.New("transient stream info failure")
		}
		return createdStreamIdentity(info)
	}
	projection := newSnapshotTrackingProjection(RoomSubjectFilter())
	source := &staticSnapshotSource{}
	projector := NewProjector(js, stream, projection, testLogger())
	if err := projector.ConfigureSnapshots("tracking", source, resolveIdentity); err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete
	})
	if projection.Count() != 1 || source.request.StreamIdentity != "" {
		t.Fatalf("cold replay count/request = %d/%+v, want 1 and no snapshot request", projection.Count(), source.request)
	}

	captured, err := projector.CaptureSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantIdentity := testCreatedStreamIdentity(t, ctx, stream)
	if captured.StreamIdentity != wantIdentity {
		t.Fatalf("captured identity = %q, want configured fallback %q", captured.StreamIdentity, wantIdentity)
	}
}

func TestProjectorSnapshotIdentityLookupDoesNotHoldApplyBarrier(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	publisher := NewPublisher(js, stream, testLogger())
	identityLookupStarted := make(chan struct{})
	releaseIdentityLookup := make(chan struct{})
	resolverCalls := 0
	resolveIdentity := func(info *jetstream.StreamInfo) (string, error) {
		resolverCalls++
		if resolverCalls == 3 {
			close(identityLookupStarted)
			<-releaseIdentityLookup
		}
		return createdStreamIdentity(info)
	}

	projection := newSnapshotTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, projection, testLogger())
	if err := projector.ConfigureSnapshots("tracking", &staticSnapshotSource{err: errors.New("snapshot unavailable")}, resolveIdentity); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete
	})

	captureDone := make(chan error, 1)
	go func() {
		_, err := projector.CaptureSnapshot(ctx)
		captureDone <- err
	}()
	select {
	case <-identityLookupStarted:
	case <-ctx.Done():
		t.Fatal("snapshot identity lookup did not start")
	}

	sequence, err := publisher.Append(
		ctx,
		RoomAggregate("R1").Subject(EventUserJoinedRoom),
		makeEvent("R1", "U2"),
	)
	if err != nil {
		t.Fatal(err)
	}
	applied := make(chan error, 1)
	go func() {
		applied <- projector.WaitFor(
			ctx,
			SubjectPosition(RoomAggregate("R1").Subject(EventUserJoinedRoom), sequence),
		)
	}()
	select {
	case err := <-applied:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("snapshot identity lookup held the projection apply barrier")
	}
	close(releaseIdentityLookup)
	if err := <-captureDone; err != nil {
		t.Fatal(err)
	}
}

func TestProjectorRejectsCompetingRestoreAuthorities(t *testing.T) {
	js, stream := setupTestStream(t)
	source := &staticSnapshotSource{}
	identity := testStreamIdentity(t, stream)

	checkpointFirst := NewProjector(js, stream, newCheckpointTrackingProjection(RoomSubjectFilter()), testLogger())
	if err := checkpointFirst.ConfigureCheckpoint("search", fixedStreamIdentity(identity)); err != nil {
		t.Fatalf("ConfigureCheckpoint: %v", err)
	}
	if err := checkpointFirst.ConfigureSnapshots("search", source, fixedStreamIdentity(identity)); err == nil {
		t.Fatal("ConfigureSnapshots succeeded after ConfigureCheckpoint")
	}

	snapshotFirst := NewProjector(js, stream, newCheckpointTrackingProjection(RoomSubjectFilter()), testLogger())
	if err := snapshotFirst.ConfigureSnapshots("search", source, fixedStreamIdentity(identity)); err != nil {
		t.Fatalf("ConfigureSnapshots: %v", err)
	}
	if err := snapshotFirst.ConfigureCheckpoint("search", fixedStreamIdentity(identity)); err == nil {
		t.Fatal("ConfigureCheckpoint succeeded after ConfigureSnapshots")
	}
}

func TestProjectorRequiresStreamIdentityForPersistence(t *testing.T) {
	js, stream := setupTestStream(t)

	checkpoint := NewProjector(js, stream, newCheckpointTrackingProjection(RoomSubjectFilter()), testLogger())
	if err := checkpoint.ConfigureCheckpoint("search", nil); err == nil {
		t.Fatal("ConfigureCheckpoint accepted a nil stream identity resolver")
	}

	snapshot := NewProjector(js, stream, newSnapshotTrackingProjection(RoomSubjectFilter()), testLogger())
	if err := snapshot.ConfigureSnapshots("tracking", &staticSnapshotSource{}, nil); err == nil {
		t.Fatal("ConfigureSnapshots accepted a nil stream identity resolver")
	}
}

func TestProjectorsRestoreAndReplayIndependently(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	var seqs []uint64
	for i := 0; i < 3; i++ {
		seq, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), makeEvent("R1", "U"+itoa(i)))
		if err != nil {
			t.Fatal(err)
		}
		seqs = append(seqs, seq)
	}

	restoredProjection := newSnapshotTrackingProjection(RoomSubjectFilter())
	coldProjection := newTrackingProjection(RoomSubjectFilter())
	restoredProjector := NewProjector(js, stream, restoredProjection, testLogger())
	coldProjector := NewProjector(js, stream, coldProjection, testLogger())
	createdAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	source := &staticSnapshotSource{snapshot: ProjectionSnapshot{GenerationID: "generation", CutoffSequence: seqs[1], CreatedAt: createdAt, Payload: []byte("restored")}}
	if err := restoredProjector.ConfigureSnapshots("tracking", source, fixedStreamIdentity(testStreamIdentity(t, stream))); err != nil {
		t.Fatal(err)
	}
	// Configuration captures the contract once so restore and publication cannot
	// diverge if projection wiring changes later.
	restoredProjection.contractID = "tracking-v2"
	if got := restoredProjector.SnapshotContractID(); got != "tracking-v1" {
		t.Fatalf("configured snapshot contract = %q, want tracking-v1", got)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = restoredProjector.Run(runCtx) }()
	go func() { _ = coldProjector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool {
		return restoredProjector.Status().StartupComplete && coldProjector.Status().StartupComplete
	})
	if got := restoredProjection.Count(); got != 1 {
		t.Fatalf("restored Apply count = %d, want 1", got)
	}
	if got := coldProjection.Count(); got != 3 {
		t.Fatalf("cold Apply count = %d, want 3", got)
	}
	status := restoredProjector.Status()
	if !status.SnapshotRestored || status.SnapshotCutoffSeq != seqs[1] || status.StartupMessages != 1 || status.LastSeq != seqs[2] {
		t.Fatalf("restored status = %#v", status)
	}
	if status.LatestSnapshotSeq != seqs[1] || !status.LatestSnapshotAt.Equal(createdAt) {
		t.Fatalf("latest snapshot status = %#v", status)
	}
	if source.request.StreamName != "EVT_TEST" || source.request.StreamIdentity != testStreamIdentity(t, stream) || source.request.MaxCutoff != seqs[2] || source.request.ContractID != "tracking-v1" {
		t.Fatalf("snapshot load request = %#v", source.request)
	}
}

func TestProjectorsStartAfterTheirOwnSnapshotCutoffs(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	malformedAck, err := js.Publish(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), []byte("not protobuf"))
	if err != nil {
		t.Fatal(err)
	}
	malformedSeq := malformedAck.Sequence
	pub := NewPublisher(js, stream, testLogger())
	lastSeq, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), makeEvent("R1", "tail"))
	if err != nil {
		t.Fatal(err)
	}

	firstProjection := newSnapshotTrackingProjection(RoomSubjectFilter())
	secondProjection := newSnapshotTrackingProjection(RoomSubjectFilter())
	first := NewProjector(js, stream, firstProjection, testLogger())
	second := NewProjector(js, stream, secondProjection, testLogger())
	for projector, cutoff := range map[*Projector]uint64{first: malformedSeq, second: lastSeq} {
		source := &staticSnapshotSource{snapshot: ProjectionSnapshot{GenerationID: "generation", CutoffSequence: cutoff, Payload: []byte("restored")}}
		if err := projector.ConfigureSnapshots("tracking", source, fixedStreamIdentity(testStreamIdentity(t, stream))); err != nil {
			t.Fatal(err)
		}
	}

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = first.Run(runCtx) }()
	go func() { _ = second.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool {
		return first.Status().StartupComplete && second.Status().StartupComplete
	})
	for name, projector := range map[string]*Projector{"first": first, "second": second} {
		status := projector.Status()
		wantMessages := uint64(1)
		if projector == second {
			wantMessages = 0
		}
		if status.Failed || !status.SnapshotRestored || status.LastSeq != lastSeq || status.StartupMessages != wantMessages {
			t.Fatalf("%s status = %#v", name, status)
		}
	}
}

func TestProjectorConfiguresRestoredConsumerAfterItsCutoff(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	var seqs []uint64
	for i := 0; i < 3; i++ {
		seq, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), makeEvent("R1", "U"+itoa(i)))
		if err != nil {
			t.Fatal(err)
		}
		seqs = append(seqs, seq)
	}

	projection := newSnapshotTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, projection, testLogger())
	source := &staticSnapshotSource{snapshot: ProjectionSnapshot{
		GenerationID:   "generation",
		CutoffSequence: seqs[1],
		Payload:        []byte("restored"),
	}}
	if err := projector.ConfigureSnapshots("tracking", source, fixedStreamIdentity(testStreamIdentity(t, stream))); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool { return projector.Status().StartupComplete })

	var info *jetstream.ConsumerInfo
	waitFor(t, 2*time.Second, func() bool {
		lister := stream.ListConsumers(ctx)
		for candidate := range lister.Info() {
			info = candidate
			break
		}
		return lister.Err() == nil && info != nil
	})
	if info.Config.DeliverPolicy != jetstream.DeliverByStartSequencePolicy {
		t.Fatalf("consumer deliver policy = %v, want start sequence", info.Config.DeliverPolicy)
	}
	if info.Config.OptStartSeq != seqs[1]+1 {
		t.Fatalf("consumer start sequence = %d, want %d", info.Config.OptStartSeq, seqs[1]+1)
	}
}

func TestProjectorRestoreReleasesWaiterRegisteredInFlight(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := context.Background()
	pub := NewPublisher(js, stream, testLogger())
	seq, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), makeEvent("R1", "joined"))
	if err != nil {
		t.Fatal(err)
	}
	projection := newSnapshotTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, projection, testLogger())
	source := &gatedSnapshotSource{started: make(chan struct{}), release: make(chan struct{}), snapshot: ProjectionSnapshot{GenerationID: "generation", CutoffSequence: seq, Payload: []byte("restored")}}
	if err := projector.ConfigureSnapshots("tracking", source, fixedStreamIdentity(testStreamIdentity(t, stream))); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()
	select {
	case <-source.started:
	case <-time.After(time.Second):
		t.Fatal("snapshot load did not start")
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- projector.WaitForCurrent(ctx) }()
	close(source.release)
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("restore did not release sequence waiter")
	}
}

func TestProjectorSnapshotCutoffTracksItsLogicalEvents(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := context.Background()
	pub := NewPublisher(js, stream, testLogger())
	joined := makeEvent("R1", "joined")
	joinedSeq, err := pub.Append(ctx, RoomAggregate("R1").SubjectFor(joined), joined)
	if err != nil {
		t.Fatal(err)
	}
	posted := makeMessagePostedEvent("R1", "poster")
	if _, err := pub.Append(ctx, RoomAggregate("R1").SubjectFor(posted), posted); err != nil {
		t.Fatal(err)
	}
	projection := &snapshotReplayTrackingProjection{
		snapshotTrackingProjection: &snapshotTrackingProjection{
			trackingProjection: newTrackingProjection(RoomEventTypeFilter(EventUserJoinedRoom)),
			snapshot:           []byte("captured"),
		},
		replay: []string{RoomSubjectFilter()},
	}
	projector := NewProjector(js, stream, projection, testLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, time.Second, func() bool { return projector.Status().StartupComplete })
	if got := projector.LastSeq(); got != joinedSeq {
		t.Fatalf("projection replay watermark = %d, want last logical event %d", got, joinedSeq)
	}
	captured, err := projector.CaptureSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if captured.CutoffSequence != joinedSeq {
		t.Fatalf("snapshot cutoff = %d, want %d", captured.CutoffSequence, joinedSeq)
	}
}

func TestProjectorRejectsFutureSnapshotAndFallsBackAfterRestoreFailure(t *testing.T) {
	for _, test := range []struct {
		name        string
		cutoffDelta uint64
		restoreErr  error
	}{
		{name: "future cutoff", cutoffDelta: 1},
		{name: "restore failure", restoreErr: errors.New("invalid snapshot payload")},
	} {
		t.Run(test.name, func(t *testing.T) {
			js, stream := setupTestStream(t)
			pub := NewPublisher(js, stream, testLogger())
			ctx := testContext(t)
			seq, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), makeEvent("R1", "U1"))
			if err != nil {
				t.Fatal(err)
			}
			projection := newSnapshotTrackingProjection(RoomSubjectFilter())
			projection.restoreErr = test.restoreErr
			projector := NewProjector(js, stream, projection, testLogger())
			source := &staticSnapshotSource{snapshot: ProjectionSnapshot{GenerationID: "generation", CutoffSequence: seq + test.cutoffDelta, Payload: []byte("bad")}}
			if err := projector.ConfigureSnapshots("tracking", source, fixedStreamIdentity(testStreamIdentity(t, stream))); err != nil {
				t.Fatal(err)
			}
			runCtx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			go func() { _ = projector.Run(runCtx) }()
			waitFor(t, 2*time.Second, func() bool { return projector.Status().StartupComplete })
			if got := projection.Count(); got != 1 {
				t.Fatalf("fallback Apply count = %d, want 1", got)
			}
			if projector.Status().SnapshotRestored {
				t.Fatal("invalid snapshot reported as restored")
			}
		})
	}
}

func TestProjectorCaptureWaitsForApplyBarrier(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	seq, err := pub.Append(ctx, RoomAggregate("R1").Subject(EventUserJoinedRoom), makeEvent("R1", "U1"))
	if err != nil {
		t.Fatal(err)
	}
	base := newBlockingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, &structSnapshotBlockingProjection{blockingProjection: base}, testLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()
	select {
	case <-base.entered:
	case <-ctx.Done():
		t.Fatal("Apply did not enter")
	}
	capturedCh := make(chan ProjectionSnapshot, 1)
	go func() { captured, _ := projector.CaptureSnapshot(context.Background()); capturedCh <- captured }()
	select {
	case <-capturedCh:
		t.Fatal("CaptureSnapshot crossed an in-progress Apply")
	case <-time.After(20 * time.Millisecond):
	}
	close(base.release)
	select {
	case captured := <-capturedCh:
		if captured.CutoffSequence != seq || string(captured.Payload) != "captured" {
			t.Fatalf("captured = %#v", captured)
		}
	case <-ctx.Done():
		t.Fatal("CaptureSnapshot did not complete")
	}
}

type structSnapshotBlockingProjection struct{ blockingProjection *blockingProjection }

func (p structSnapshotBlockingProjection) Subjects() []string { return p.blockingProjection.Subjects() }
func (p structSnapshotBlockingProjection) Apply(e *corev1.Event, seq uint64) error {
	return p.blockingProjection.Apply(e, seq)
}
func (structSnapshotBlockingProjection) Snapshot() ([]byte, error) { return []byte("captured"), nil }
func (structSnapshotBlockingProjection) Restore([]byte) error      { return nil }

func TestProjector_CompletesEmptyStartupReplayOnce(t *testing.T) {
	js, stream := setupTestStream(t)
	proj := newTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete && proj.ReplayCompletions() == 1
	})
	if got := proj.ReplayCompletions(); got != 1 {
		t.Fatalf("startup replay completions = %d, want 1", got)
	}
}

func TestProjectorsConsumeTheSameEventsIndependently(t *testing.T) {
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
	go func() { _ = projectorA.Run(runCtx) }()
	go func() { _ = projectorB.Run(runCtx) }()

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
	if gotA, gotB := projA.ReplayCompletions(), projB.ReplayCompletions(); gotA != 1 || gotB != 1 {
		t.Fatalf("startup replay completions = %d/%d, want 1/1", gotA, gotB)
	}
}

func TestProjectorBroadReplayFilterSkipsNonLogicalSubjects(t *testing.T) {
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
	go func() { _ = broadProjector.Run(runCtx) }()
	go func() { _ = focusedProjector.Run(runCtx) }()

	waitFor(t, 2*time.Second, func() bool {
		return broad.Count() == 2 && focused.Count() == 1 && broadProjector.LastSeq() == postedSeq
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
	if status.LastSeq != joinedSeq {
		t.Fatalf("focused replay watermark = %d, want logical event seq %d", status.LastSeq, joinedSeq)
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

	t.Run("OAuthClientAggregate hashes the client ID", func(t *testing.T) {
		const clientID = "https://remote.example/oauth/client-metadata.json"
		event := &corev1.Event{Event: &corev1.Event_OauthClientAuthorizationRecorded{
			OauthClientAuthorizationRecorded: &corev1.OAuthClientAuthorizationRecordedEvent{ClientId: clientID},
		}}
		got := OAuthClientAggregate(clientID).SubjectFor(event)
		want := "evt.oauth_client.5e155b7992d0702f7826580094fee3aff9d6bfb6e94d4ddd25da630c5b0e6035.authorization_recorded"
		if got != want {
			t.Errorf("OAuthClientAggregate.Subject: got %q, want %q", got, want)
		}
		if strings.Contains(got, clientID) {
			t.Errorf("OAuthClientAggregate.Subject leaked client ID: %q", got)
		}
		if OAuthClientSubjectFilter() != "evt.oauth_client.>" {
			t.Errorf("OAuthClientSubjectFilter = %q", OAuthClientSubjectFilter())
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
			name: "RoomSlowModeChanged",
			event: &corev1.Event{
				Event: &corev1.Event_RoomSlowModeChanged{
					RoomSlowModeChanged: &corev1.RoomSlowModeChangedEvent{RoomId: "R1", SlowModeSeconds: 30},
				},
			},
			want: EventRoomSlowModeChanged,
		},
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
			name: "MessagePinned",
			event: &corev1.Event{
				Event: &corev1.Event_MessagePinned{
					MessagePinned: &corev1.MessagePinnedEvent{RoomId: "R1", MessageEventId: "M1"},
				},
			},
			want: EventMessagePinned,
		},
		{
			name: "MessageUnpinned",
			event: &corev1.Event{
				Event: &corev1.Event_MessageUnpinned{
					MessageUnpinned: &corev1.MessageUnpinnedEvent{RoomId: "R1", MessageEventId: "M1"},
				},
			},
			want: EventMessageUnpinned,
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
			name: "ThreadFollowed",
			event: &corev1.Event{
				Event: &corev1.Event_ThreadFollowed{
					ThreadFollowed: &corev1.ThreadFollowedEvent{RoomId: "R1", ThreadRootEventId: "M1", UserId: "U1"},
				},
			},
			want: EventThreadFollowed,
		},
		{
			name: "ThreadUnfollowed",
			event: &corev1.Event{
				Event: &corev1.Event_ThreadUnfollowed{
					ThreadUnfollowed: &corev1.ThreadUnfollowedEvent{RoomId: "R1", ThreadRootEventId: "M1", UserId: "U1"},
				},
			},
			want: EventThreadUnfollowed,
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
			name: "UserKeyShreddingRequested",
			event: &corev1.Event{
				Event: &corev1.Event_UserKeyShreddingRequested{
					UserKeyShreddingRequested: &corev1.UserKeyShreddingRequestedEvent{UserId: "U1"},
				},
			},
			want: EventUserKeyShreddingRequested,
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
			if c.want == EventUserKeyShreddingRequested || c.want == EventUserKeyShredded || c.want == EventUserDEKGenerated {
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

func TestEventTypeOf_AssetAttached(t *testing.T) {
	event := &corev1.Event{Event: &corev1.Event_AssetAttached{
		AssetAttached: &corev1.AssetAttachedEvent{AssetId: "A1", RoomId: "R1", MessageEventId: "M1", UserId: "U1"},
	}}
	if got := EventTypeOf(event); got != EventAssetAttached {
		t.Fatalf("EventTypeOf(asset attached) = %q, want %q", got, EventAssetAttached)
	}
	if got := AssetAggregate("A1").SubjectFor(event); got != "evt.asset.A1.asset_attached" {
		t.Fatalf("asset attachment subject = %q", got)
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
