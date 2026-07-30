package events

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

type codecTestEvent struct {
	name string
}

type nilSafeCodecTestProjection struct{}

func (*nilSafeCodecTestProjection) Subjects() []string {
	return []string{"evt.codec.nil.created"}
}

func (*nilSafeCodecTestProjection) Apply(codecTestEvent, uint64) error {
	return nil
}

type codecTestProjection struct {
	mu        sync.Mutex
	subject   string
	events    []string
	sequences []uint64
}

func (p *codecTestProjection) Subjects() []string {
	return []string{p.subject}
}

func (p *codecTestProjection) Apply(event codecTestEvent, sequence uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event.name)
	p.sequences = append(p.sequences, sequence)
	return nil
}

func (p *codecTestProjection) applied() ([]string, []uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.events), slices.Clone(p.sequences)
}

func (p *codecTestProjection) Snapshot() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return []byte(strings.Join(p.events, ",")), nil
}

func (p *codecTestProjection) Restore(snapshot []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = nil
	p.sequences = nil
	if len(snapshot) > 0 {
		p.events = strings.Split(string(snapshot), ",")
	}
	return nil
}

type codecTestBatchProjection struct {
	codecTestProjection
	batches [][]uint64
}

func (*codecTestBatchProjection) StartupBatchSize() int {
	return 2
}

func (p *codecTestBatchProjection) ApplyStartupBatch(items []SequencedEventOf[codecTestEvent]) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	batch := make([]uint64, 0, len(items))
	for _, item := range items {
		p.events = append(p.events, item.Event.name)
		p.sequences = append(p.sequences, item.Sequence)
		batch = append(batch, item.Sequence)
	}
	p.batches = append(p.batches, batch)
	return nil
}

func (p *codecTestBatchProjection) state() ([]string, [][]uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	batches := make([][]uint64, len(p.batches))
	for i := range p.batches {
		batches[i] = slices.Clone(p.batches[i])
	}
	return slices.Clone(p.events), batches
}

func decodeCodecTestEvent(data []byte) (DecodedEvent[codecTestEvent], error) {
	id, name, ok := strings.Cut(string(data), ":")
	if !ok {
		return DecodedEvent[codecTestEvent]{}, fmt.Errorf("invalid test event")
	}
	return DecodedEvent[codecTestEvent]{
		Event: codecTestEvent{name: name},
		ID:    id,
	}, nil
}

func TestDecodedProjectionHandleRejectsNilProjection(t *testing.T) {
	var projection *codecTestProjection
	defer func() {
		if recover() == nil {
			t.Fatal("NewDecodedProjectionHandle accepted a nil projection")
		}
	}()
	NewDecodedProjectionHandle(nil, nil, projection, decodeCodecTestEvent, testLogger())
}

func TestDecodedProjectorRejectsTypedNilProjection(t *testing.T) {
	var projection *nilSafeCodecTestProjection
	defer func() {
		if got := recover(); got != "events: projector requires a non-nil projection" {
			t.Fatalf("NewDecodedProjector panic = %v, want nil projection guard", got)
		}
	}()
	NewDecodedProjector(nil, nil, projection, decodeCodecTestEvent, testLogger())
}

func TestDecodedProjectorReplaysApplicationCodecInOrder(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	subject := "evt.codec.A.created"

	var wantSequences []uint64
	for _, record := range []EncodedRecord{
		{ID: "one", Data: []byte("one:alpha")},
		{ID: "two", Data: []byte("two:beta")},
		{ID: "three", Data: []byte("three:gamma")},
	} {
		sequence, err := eventLog.AppendEventually(ctx, subject, record)
		if err != nil {
			t.Fatalf("append encoded record: %v", err)
		}
		wantSequences = append(wantSequences, sequence)
	}

	projection := &codecTestProjection{subject: subject}
	handle := NewDecodedProjectionHandle(js, stream, projection, decodeCodecTestEvent, testLogger())
	if handle.Projection() != projection {
		t.Fatal("decoded projection handle did not retain the projection")
	}
	projector := handle.Projector()
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	waitFor(t, 2*time.Second, func() bool { return projector.Status().StartupComplete })
	events, sequences := projection.applied()
	if !slices.Equal(events, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("events = %v, want ordered decoded values", events)
	}
	if !slices.Equal(sequences, wantSequences) {
		t.Fatalf("sequences = %v, want %v", sequences, wantSequences)
	}
	snapshot, err := projector.CaptureSnapshot()
	if err != nil {
		t.Fatalf("capture decoded projection snapshot: %v", err)
	}
	if snapshot.CutoffSequence != wantSequences[len(wantSequences)-1] || string(snapshot.Payload) != "alpha,beta,gamma" {
		t.Fatalf("snapshot = %+v, want codec-neutral state through final sequence", snapshot)
	}
}

func TestDecodedProjectorPreservesGenericStartupBatching(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	subject := "evt.codec.B.created"

	var wantSequences []uint64
	for _, record := range []EncodedRecord{
		{ID: "one", Data: []byte("one:alpha")},
		{ID: "two", Data: []byte("two:beta")},
		{ID: "three", Data: []byte("three:gamma")},
	} {
		sequence, err := eventLog.AppendEventually(ctx, subject, record)
		if err != nil {
			t.Fatalf("append encoded record: %v", err)
		}
		wantSequences = append(wantSequences, sequence)
	}

	projection := &codecTestBatchProjection{
		codecTestProjection: codecTestProjection{subject: subject},
	}
	projector := NewDecodedProjector(js, stream, projection, decodeCodecTestEvent, testLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	waitFor(t, 2*time.Second, func() bool { return projector.Status().StartupComplete })
	events, batches := projection.state()
	if !slices.Equal(events, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("events = %v, want ordered decoded values", events)
	}
	wantBatches := [][]uint64{wantSequences[:2], wantSequences[2:]}
	if !slices.EqualFunc(batches, wantBatches, slices.Equal[[]uint64]) {
		t.Fatalf("batches = %v, want %v", batches, wantBatches)
	}
}

func TestDecodedProjectorReportsApplicationDecodeFailure(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	subject := "evt.codec.C.created"
	sequence, err := eventLog.Append(ctx, subject, EncodedRecord{ID: "bad", Data: []byte("invalid")})
	if err != nil {
		t.Fatalf("append invalid application record: %v", err)
	}

	decodeErr := errors.New("application codec rejected record")
	projection := &codecTestProjection{subject: subject}
	projector := NewDecodedProjector(
		js,
		stream,
		projection,
		func([]byte) (DecodedEvent[codecTestEvent], error) {
			return DecodedEvent[codecTestEvent]{}, decodeErr
		},
		testLogger(),
	)
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	errCh := make(chan error, 1)
	go func() { errCh <- projector.Run(runCtx) }()

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrProjectionFailed) || !errors.Is(err, decodeErr) {
			t.Fatalf("Run error = %v, want projection and decoder errors", err)
		}
	case <-ctx.Done():
		t.Fatal("projector did not fail after application decode error")
	}
	if status := projector.Status(); status.FailedSeq != sequence || status.LastSeq >= sequence {
		t.Fatalf("decode failure status = %+v, want failure at %d before advancement", status, sequence)
	}
}

func TestChattoProjectorReplaysExistingProtobufBytes(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	event := makeEvent("R-existing-codec", "U-existing-codec")
	data, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal existing Chatto event: %v", err)
	}
	subject := RoomAggregate("R-existing-codec").Subject(EventUserJoinedRoom)
	sequence, err := eventLog.Append(ctx, subject, EncodedRecord{ID: event.GetId(), Data: data})
	if err != nil {
		t.Fatalf("seed existing Chatto bytes: %v", err)
	}

	projection := newTrackingProjection(subject)
	projector := NewProjector(js, stream, projection, testLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()

	if err := projector.WaitFor(ctx, SubjectPosition(subject, sequence)); err != nil {
		t.Fatalf("wait for existing Chatto event: %v", err)
	}
	if projection.Count() != 1 {
		t.Fatalf("projection count = %d, want existing event applied", projection.Count())
	}
}
