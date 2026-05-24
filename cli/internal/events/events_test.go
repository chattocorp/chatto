package events

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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
// a stream with the SERVER_EVT shape (subjects "server.evt.>"), and returns
// the wired-up bits plus a cleanup-registered teardown.
func setupTestStream(t *testing.T) (jetstream.JetStream, jetstream.Stream) {
	t.Helper()

	opts := &server.Options{
		JetStream: true,
		Port:      -1,
		StoreDir:  t.TempDir(),
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("create NATS server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server not ready")
	}

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("connect NATS: %v", err)
	}
	t.Cleanup(func() {
		nc.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
	})

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}

	ctx := testContext(t)
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     "SERVER_EVT_TEST",
		Subjects: []string{SubjectRoot + ">"},
		Storage:  jetstream.FileStorage,
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

// ============================================================================
// Publisher
// ============================================================================

func TestPublisher_Append_HappyPath(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	subject := RoomAggregate("R1").Subject()

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
			_, err := pub.Append(ctx, RoomAggregate("R1").Subject(), tc.event)
			if !errors.Is(err, ErrInvalidEvent) {
				t.Errorf("want ErrInvalidEvent, got %v", err)
			}
		})
	}
}

func TestPublisher_Append_ConcurrentWrites(t *testing.T) {
	// Multiple goroutines append to the same subject. Each should succeed
	// (Append retries on OCC conflict); the final per-subject seq should
	// equal the number of writes.
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	subject := RoomAggregate("R1").Subject()
	const writers = 10

	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := pub.Append(ctx, subject, makeEvent("R1", "U"+itoa(i)))
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

	subject := RoomAggregate("R1").Subject()

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

	subject := RoomAggregate("R1").Subject()
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

func TestProjector_AppliesEventsInOrder(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())

	// Seed three events before the projector starts.
	ctx := testContext(t)
	for i := 0; i < 3; i++ {
		if _, err := pub.Append(ctx, RoomAggregate("R1").Subject(), makeEvent("R1", "U"+itoa(i))); err != nil {
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
	msg, err := stream.GetLastMsgForSubject(ctx, RoomAggregate("R1").Subject())
	if err != nil {
		t.Fatalf("GetLastMsgForSubject: %v", err)
	}
	if got := projector.LastSeq(); got != msg.Sequence {
		t.Errorf("LastSeq=%d, want %d", got, msg.Sequence)
	}
}

func TestProjector_WaitForSeq_AlreadyReached(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)

	if _, err := pub.Append(ctx, RoomAggregate("R1").Subject(), makeEvent("R1", "U1")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	proj := newTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	waitFor(t, 2*time.Second, func() bool { return projector.LastSeq() > 0 })

	// WaitForSeq for a seq we've already reached returns immediately.
	deadline, cancelDeadline := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancelDeadline()
	if err := projector.WaitForSeq(deadline, projector.LastSeq()); err != nil {
		t.Errorf("WaitForSeq for already-reached seq: %v", err)
	}
}

func TestProjector_WaitForSeq_UnblocksOnApply(t *testing.T) {
	js, stream := setupTestStream(t)
	pub := NewPublisher(js, stream, testLogger())

	proj := newTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, proj, testLogger())

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	// Publish, capture seq, then WaitForSeq must return without timing out.
	ctx := testContext(t)
	seq, err := pub.Append(ctx, RoomAggregate("R1").Subject(), makeEvent("R1", "U1"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	deadline, cancelDeadline := context.WithTimeout(ctx, 2*time.Second)
	defer cancelDeadline()
	if err := projector.WaitForSeq(deadline, seq); err != nil {
		t.Fatalf("WaitForSeq: %v", err)
	}
	if got := projector.LastSeq(); got < seq {
		t.Errorf("LastSeq=%d, want >= %d", got, seq)
	}
}

func TestProjector_WaitForSeq_HonoursContextCancel(t *testing.T) {
	js, stream := setupTestStream(t)
	proj := newTrackingProjection(RoomSubjectFilter())
	projector := NewProjector(js, stream, proj, testLogger())

	// Projector is not running, so LastSeq stays at 0 — WaitForSeq blocks.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := projector.WaitForSeq(ctx, 999)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want DeadlineExceeded, got %v", err)
	}
}

// ============================================================================
// Subject helpers
// ============================================================================

func TestSubjectHelpers(t *testing.T) {
	t.Run("RoomAggregate Subject", func(t *testing.T) {
		got := RoomAggregate("ROOM123").Subject()
		want := "evt.room.ROOM123"
		if got != want {
			t.Errorf("RoomAggregate.Subject: got %q, want %q", got, want)
		}
	})

	t.Run("RoomSubjectFilter", func(t *testing.T) {
		got := RoomSubjectFilter()
		want := "evt.room.>"
		if got != want {
			t.Errorf("RoomSubjectFilter: got %q, want %q", got, want)
		}
	})

	t.Run("ParseRoomSubject", func(t *testing.T) {
		cases := []struct {
			subject string
			wantID  string
			wantOK  bool
		}{
			{"evt.room.ROOM123", "ROOM123", true},
			{"live.evt.room.ROOM123", "ROOM123", true},
			{"evt.user.U1", "", false},
			{"evt.room.", "", false},
			{"evt.room.ROOM.extra", "", false},
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
