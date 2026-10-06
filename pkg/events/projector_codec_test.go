package events_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	. "hmans.de/chatto/pkg/events"
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

type valueCodecTestProjection struct{}

func (valueCodecTestProjection) Subjects() []string {
	return []string{"evt.codec.value.created"}
}

func (valueCodecTestProjection) Apply(codecTestEvent, uint64) error {
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

func (*codecTestProjection) SnapshotContractID() string { return "codec-test-v1" }

type mismatchedCodecSnapshotSource struct{}

func (mismatchedCodecSnapshotSource) LoadProjectionSnapshot(context.Context, ProjectionSnapshotLoadRequest) (ProjectionSnapshot, error) {
	return ProjectionSnapshot{
		ContractID:     "wrong-contract",
		StreamName:     "WRONG_STREAM",
		StreamIdentity: "wrong-stream",
		Components: []ProjectionSnapshotComponent{{
			Key: SnapshotStatePartKey, ContractID: "codec-test-v1",
			Parts: []ProjectionSnapshotPart{{Key: SnapshotStatePartKey, Payload: []byte("must-not-restore")}},
		}},
	}, nil
}

type requestBoundCodecSnapshotSource struct{}

func (requestBoundCodecSnapshotSource) LoadProjectionSnapshot(_ context.Context, request ProjectionSnapshotLoadRequest) (ProjectionSnapshot, error) {
	return ProjectionSnapshot{
		ContractID:     request.ContractID,
		StreamName:     request.StreamName,
		StreamIdentity: request.StreamIdentity,
	}, nil
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
	must(NewDecodedProjectionHandle(nil, nil, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger()}))
}

func TestDecodedProjectorRejectsTypedNilProjection(t *testing.T) {
	var projection *nilSafeCodecTestProjection
	defer func() {
		if got := recover(); got != "events: projector requires a non-nil projection" {
			t.Fatalf("NewDecodedProjector panic = %v, want nil projection guard", got)
		}
	}()
	must(NewDecodedProjector(nil, nil, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger()}))
}

func TestDecodedProjectorRejectsValueProjection(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewDecodedProjector accepted a value projection")
		}
	}()

	must(NewDecodedProjector(nil, nil, valueCodecTestProjection{}, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger()}))
}

func TestDecodedProjectorAllowsNilLogger(t *testing.T) {
	js, stream := setupTestStream(t)
	projection := &nilSafeCodecTestProjection{}
	projector := must(NewDecodedProjector(
		js,
		stream,
		projection,
		func([]byte) (DecodedEvent[codecTestEvent], error) {
			return DecodedEvent[codecTestEvent]{Event: codecTestEvent{name: "event"}, ID: "event"}, nil
		},
		ProjectorOptions{},
	))

	stop := runConsumerProjector(t, projector)
	defer stop()
	waitFor(t, 2*time.Second, func() bool {
		return projector.Status().StartupComplete
	})
}

func TestProjectorRejectsRepeatedRun(t *testing.T) {
	js, stream := setupTestStream(t)
	projector := must(NewDecodedProjector(
		js,
		stream,
		&nilSafeCodecTestProjection{},
		func([]byte) (DecodedEvent[codecTestEvent], error) {
			return DecodedEvent[codecTestEvent]{Event: codecTestEvent{name: "event"}, ID: "event"}, nil
		},
		ProjectorOptions{Logger: testLogger()},
	))
	stop := runConsumerProjector(t, projector)
	waitFor(t, 2*time.Second, func() bool { return projector.Status().StartupComplete })
	if err := projector.Run(context.Background()); !errors.Is(err, ErrProjectorAlreadyStarted) {
		t.Fatalf("repeated projector Run error = %v, want ErrProjectorAlreadyStarted", err)
	}
	stop()
}

func TestProjectorRejectsMismatchedSnapshotBinding(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	if _, err := eventLog.Append(ctx, "evt.codec.binding.created", EncodedRecord{ID: "one", Data: []byte("one:alpha")}); err != nil {
		t.Fatal(err)
	}

	projection := &codecTestProjection{subject: "evt.codec.binding.created"}
	identity := "codec-stream"
	projector := must(NewDecodedProjector(js, stream, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger(), Snapshots: &SnapshotOptions{Key: "codec", Source: mismatchedCodecSnapshotSource{}, ResolveStreamIdentity: func(*jetstream.StreamInfo) (string, error) {
		return identity, nil
	}}}))

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()
	waitFor(t, 2*time.Second, func() bool { return projector.Status().StartupComplete })
	if events, _ := projection.applied(); !slices.Equal(events, []string{"alpha"}) {
		t.Fatalf("events = %v, want cold replay after binding rejection", events)
	}
	if projector.Status().SnapshotRestored {
		t.Fatal("mismatched snapshot reported as restored")
	}
}

func TestProjectorFailsWhenSnapshotStreamChangesDuringLoad(t *testing.T) {
	js, stream := setupTestStream(t)
	projection := &codecTestProjection{subject: "evt.codec.binding.changed"}
	identityCalls := 0
	projector := must(NewDecodedProjector(js, stream, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger(), Snapshots: &SnapshotOptions{Key: "codec", Source: requestBoundCodecSnapshotSource{}, ResolveStreamIdentity: func(*jetstream.StreamInfo) (string, error) {
		identityCalls++
		if identityCalls >= 3 {
			return "changed-stream", nil
		}
		return "codec-stream", nil
	}}}))

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	errCh := make(chan error, 1)
	go func() { errCh <- projector.Run(runCtx) }()
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "stream identity changed while loading") {
			t.Fatalf("Run error = %v, want stream identity change failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("projector did not fail after snapshot stream identity changed")
	}
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
	handle := must(NewDecodedProjectionHandle(js, stream, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger()}))
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
	snapshot, err := projector.CaptureSnapshot(context.Background())
	if err != nil {
		t.Fatalf("capture decoded projection snapshot: %v", err)
	}
	if snapshot.CutoffSequence != wantSequences[len(wantSequences)-1] || len(snapshot.Components) != 1 ||
		len(snapshot.Components[0].Parts) != 1 || string(snapshot.Components[0].Parts[0].Payload) != "alpha,beta,gamma" {
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
	projector := must(NewDecodedProjector(js, stream, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger()}))
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
	projector := must(NewDecodedProjector(
		js,
		stream,
		projection,
		func([]byte) (DecodedEvent[codecTestEvent], error) {
			return DecodedEvent[codecTestEvent]{}, decodeErr
		},
		ProjectorOptions{Logger: testLogger()},
	))
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

type capturedSnapshotSource struct {
	snapshot ProjectionSnapshot
	requests []ProjectionSnapshotLoadRequest
}

func (s *capturedSnapshotSource) LoadProjectionSnapshot(_ context.Context, request ProjectionSnapshotLoadRequest) (ProjectionSnapshot, error) {
	s.requests = append(s.requests, request)
	return s.snapshot, nil
}

// A single-payload projection is stored as one component with one part, and
// a captured snapshot restores into a new projector without replaying the
// events through its cutoff.
func TestSinglePayloadSnapshotRoundTrip(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	const subject = "evt.codec.roundtrip.created"
	for _, record := range []EncodedRecord{{ID: "one", Data: []byte("one:alpha")}, {ID: "two", Data: []byte("two:beta")}} {
		if _, err := eventLog.AppendEventually(ctx, subject, record); err != nil {
			t.Fatal(err)
		}
	}
	resolveIdentity := func(*jetstream.StreamInfo) (string, error) { return "codec-stream", nil }

	first := &codecTestProjection{subject: subject}
	firstProjector := must(NewDecodedProjector(js, stream, first, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger(), Snapshots: &SnapshotOptions{Key: "codec", Source: &capturedSnapshotSource{}, ResolveStreamIdentity: resolveIdentity}}))
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go func() { _ = firstProjector.Run(runCtx) }()
	if err := firstProjector.WaitForStartup(ctx); err != nil {
		t.Fatal(err)
	}
	captured, err := firstProjector.CaptureSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if captured.ContractID != "codec-test-v1" || captured.StreamIdentity != "codec-stream" || len(captured.Components) != 1 {
		t.Fatalf("captured snapshot = %+v", captured)
	}
	component := captured.Components[0]
	if component.Key != SnapshotStatePartKey || component.ContractID != "codec-test-v1" ||
		len(component.Parts) != 1 || component.Parts[0].Key != SnapshotStatePartKey {
		t.Fatalf("captured component = %+v, want one state part", component)
	}

	source := &capturedSnapshotSource{snapshot: captured}
	second := &codecTestProjection{subject: subject}
	secondProjector := must(NewDecodedProjector(js, stream, second, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger(), Snapshots: &SnapshotOptions{Key: "codec", Source: source, ResolveStreamIdentity: resolveIdentity}}))
	go func() { _ = secondProjector.Run(runCtx) }()
	if err := secondProjector.WaitForStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if status := secondProjector.Status(); !status.SnapshotRestored || status.SnapshotCutoffSeq != captured.CutoffSequence {
		t.Fatalf("status = %+v, want snapshot restored at %d", status, captured.CutoffSequence)
	}
	if events, sequences := second.applied(); !slices.Equal(events, []string{"alpha", "beta"}) || len(sequences) != 0 {
		t.Fatalf("restored events = %v, applied sequences = %v; want restored state without replay", events, sequences)
	}
	if len(source.requests) != 1 || !slices.Equal(source.requests[0].Components, []ProjectionSnapshotComponentContract{{
		Key: SnapshotStatePartKey, ContractID: "codec-test-v1", MaxParts: 1,
	}}) {
		t.Fatalf("load requests = %+v, want one state component contract", source.requests)
	}
}

// A snapshot whose components do not match a single-payload projection is
// rejected, and the projector replays the event log instead.
func TestSinglePayloadSnapshotRejectsComponentMismatch(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	const subject = "evt.codec.mismatch.created"
	seq, err := eventLog.Append(ctx, subject, EncodedRecord{ID: "one", Data: []byte("one:alpha")})
	if err != nil {
		t.Fatal(err)
	}
	statePart := []ProjectionSnapshotPart{{Key: SnapshotStatePartKey, Payload: []byte("must-not-restore")}}
	for name, components := range map[string][]ProjectionSnapshotComponent{
		"two components": {
			{Key: SnapshotStatePartKey, ContractID: "codec-test-v1", Parts: statePart},
			{Key: "other", ContractID: "codec-test-v1", Parts: statePart},
		},
		"wrong component contract": {{Key: SnapshotStatePartKey, ContractID: "other-v1", Parts: statePart}},
		"two parts":                {{Key: SnapshotStatePartKey, ContractID: "codec-test-v1", Parts: append(slices.Clone(statePart), ProjectionSnapshotPart{Key: "extra"})}},
	} {
		t.Run(name, func(t *testing.T) {
			projection := &codecTestProjection{subject: subject}
			source := &capturedSnapshotSource{}
			projector := must(NewDecodedProjector(js, stream, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger(), Snapshots: &SnapshotOptions{Key: "codec", Source: source, ResolveStreamIdentity: func(info *jetstream.StreamInfo) (string, error) {
				return "codec-stream", nil
			}}}))
			source.snapshot = ProjectionSnapshot{
				ContractID: "codec-test-v1", StreamName: stream.CachedInfo().Config.Name,
				StreamIdentity: "codec-stream", CutoffSequence: seq, Components: components,
			}
			runCtx, cancel := context.WithCancel(ctx)
			t.Cleanup(cancel)
			go func() { _ = projector.Run(runCtx) }()
			if err := projector.WaitForStartup(ctx); err != nil {
				t.Fatal(err)
			}
			if projector.Status().SnapshotRestored {
				t.Fatal("mismatched snapshot reported as restored")
			}
			if events, _ := projection.applied(); !slices.Equal(events, []string{"alpha"}) {
				t.Fatalf("events = %v, want cold replay", events)
			}
		})
	}
}

type subjectPreparedProjection struct {
	mu       sync.Mutex
	subjects []string
	prepares int
}

func (*subjectPreparedProjection) Subjects() []string { return []string{"evt.codec.prepared.>"} }

func (p *subjectPreparedProjection) Prepare(codecTestEvent, uint64) (PreparedMutation, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prepares++
	return nil, nil
}

func (p *subjectPreparedProjection) PrepareSubject(_ codecTestEvent, subject string, _ uint64) (PreparedMutation, error) {
	return PreparedMutationFunc(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.subjects = append(p.subjects, subject)
	}), nil
}

// A prepared projection that implements SubjectEventReducer receives each
// delivered record's subject, and the projector never calls Prepare.
func TestPreparedProjectorUsesSubjectEventReducer(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	for _, subject := range []string{"evt.codec.prepared.first", "evt.codec.prepared.second"} {
		if _, err := eventLog.Append(ctx, subject, EncodedRecord{ID: subject, Data: []byte("id:" + subject)}); err != nil {
			t.Fatal(err)
		}
	}
	projection := &subjectPreparedProjection{}
	projector := must(NewDecodedPreparedProjector(js, stream, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger()}))
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()
	if err := projector.WaitForStartup(ctx); err != nil {
		t.Fatal(err)
	}
	projection.mu.Lock()
	defer projection.mu.Unlock()
	if !slices.Equal(projection.subjects, []string{"evt.codec.prepared.first", "evt.codec.prepared.second"}) || projection.prepares != 0 {
		t.Fatalf("PrepareSubject subjects = %v, Prepare calls = %d", projection.subjects, projection.prepares)
	}
}

type resetRecordingProjection struct {
	codecTestProjection
	restores [][]byte
}

func (p *resetRecordingProjection) Restore(snapshot []byte) error {
	p.restores = append(p.restores, snapshot)
	return p.codecTestProjection.Restore(snapshot)
}

// A projection with snapshot state starts a cold replay from its canonical
// empty state, also when snapshots are not configured.
func TestProjectorResetsSnapshotStateBeforeColdReplay(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	projection := &resetRecordingProjection{codecTestProjection: codecTestProjection{subject: "evt.codec.reset.created"}}
	projector := must(NewDecodedProjector(js, stream, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger()}))
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()
	if err := projector.WaitForStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if len(projection.restores) != 1 || projection.restores[0] != nil {
		t.Fatalf("Restore calls = %q, want one Restore(nil)", projection.restores)
	}
}

// Invalid projector options fail construction instead of a later Run.
func TestProjectorRejectsInvalidOptions(t *testing.T) {
	js, stream := setupTestStream(t)
	resolve := func(*jetstream.StreamInfo) (string, error) { return "codec-stream", nil }
	snapshots := &SnapshotOptions{Key: "codec", Source: &capturedSnapshotSource{}, ResolveStreamIdentity: resolve}
	for name, test := range map[string]struct {
		projection EventProjection[codecTestEvent]
		opts       ProjectorOptions
	}{
		"snapshots and checkpoint": {&codecTestProjection{subject: "evt.codec.options"}, ProjectorOptions{
			Snapshots: snapshots, Checkpoint: &CheckpointOptions{Key: "codec", ResolveStreamIdentity: resolve},
		}},
		"snapshot without key": {&codecTestProjection{subject: "evt.codec.options"}, ProjectorOptions{
			Snapshots: &SnapshotOptions{Source: &capturedSnapshotSource{}, ResolveStreamIdentity: resolve},
		}},
		"snapshot without source": {&codecTestProjection{subject: "evt.codec.options"}, ProjectorOptions{
			Snapshots: &SnapshotOptions{Key: "codec", ResolveStreamIdentity: resolve},
		}},
		"empty snapshot stream identity": {&codecTestProjection{subject: "evt.codec.options"}, ProjectorOptions{
			Snapshots: &SnapshotOptions{Key: "codec", Source: &capturedSnapshotSource{}, ResolveStreamIdentity: func(*jetstream.StreamInfo) (string, error) {
				return "", nil
			}},
		}},
		"snapshots without snapshot support": {&nilSafeCodecTestProjection{}, ProjectorOptions{Snapshots: snapshots}},
		"checkpoint without checkpoint support": {&codecTestProjection{subject: "evt.codec.options"}, ProjectorOptions{
			Checkpoint: &CheckpointOptions{Key: "codec", ResolveStreamIdentity: resolve},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewDecodedProjector(js, stream, test.projection, decodeCodecTestEvent, test.opts); err == nil {
				t.Fatal("NewDecodedProjector accepted invalid options")
			}
		})
	}
}
