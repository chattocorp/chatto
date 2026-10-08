package events

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type failedStartupSnapshotSource struct{ err error }

func (s failedStartupSnapshotSource) LoadProjectionSnapshot(context.Context, ProjectionSnapshotLoadRequest) (ProjectionSnapshot, error) {
	return ProjectionSnapshot{}, s.err
}

type startupSnapshotProjection struct{ startupCompletionProjection }

func (*startupSnapshotProjection) SnapshotContractID() string { return "test-v1" }
func (*startupSnapshotProjection) Snapshot() ([]byte, error)  { return nil, nil }
func (*startupSnapshotProjection) Restore([]byte) error       { return nil }

func TestSnapshotAbsenceLogsAtDebugAndFailuresWarn(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		level string
	}{
		{"absent", fmt.Errorf("load: %w", ErrProjectionSnapshotNotFound), "DEBUG"},
		{"failed", errors.New("storage unavailable"), "WARN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			js := must(jetstream.New(startTestNATS(t)))
			stream := must(js.CreateStream(t.Context(), jetstream.StreamConfig{Name: "TEST", Subjects: []string{"evt.>"}}))
			var output bytes.Buffer
			p := must(NewDecodedProjector(js, stream, &startupSnapshotProjection{}, func([]byte) (DecodedEvent[struct{}], error) { return DecodedEvent[struct{}]{}, nil }, ProjectorOptions{
				Logger:    slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})),
				Snapshots: &SnapshotOptions{Key: "test", Source: failedStartupSnapshotSource{err: tc.err}, ResolveStreamIdentity: func(*jetstream.StreamInfo) (string, error) { return "test-stream", nil }},
			}))
			if err := p.restoreForRun(t.Context(), 0); err != nil {
				t.Fatal(err)
			}
			var record map[string]any
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if record["level"] != tc.level {
				t.Fatalf("restore log = %v, want %s", record, tc.level)
			}
		})
	}
}

func TestProjectorStartupLogSummary(t *testing.T) {
	for _, restore := range []string{"none", "snapshot", "checkpoint"} {
		t.Run(restore, func(t *testing.T) {
			var output bytes.Buffer
			p := &Projector{
				proj:    &startupCompletionProjection{},
				logger:  slog.New(slog.NewJSONHandler(&output, nil)),
				started: true, startupCh: make(chan struct{}),
				startupStartedAt:   time.Now().Add(-time.Second),
				snapshotRestored:   restore == "snapshot",
				checkpointRestored: restore == "checkpoint",
			}
			p.maybeCompleteStartup(time.Now())
			var record map[string]any
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if record["projection"] != "events.startupCompletionProjection" || record["restore"] != restore {
				t.Fatalf("unexpected summary: %v", record)
			}
			for _, field := range []string{"subjects", "last_seq", "target_seq", "messages_per_second"} {
				if _, ok := record[field]; ok {
					t.Errorf("INFO summary contains %s", field)
				}
			}
			output.Reset()
			p.logger = slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
			p.logStartupComplete(startupSummary{projectionKey: "named", restore: restore})
			if !strings.Contains(output.String(), "Projection replay details") || !strings.Contains(output.String(), "subjects") {
				t.Fatalf("missing DEBUG replay details: %s", output.String())
			}
		})
	}
}

type startupCompletionProjection struct {
	completions int
}

func (*startupCompletionProjection) Subjects() []string {
	return []string{"evt.test.created"}
}

func (*startupCompletionProjection) Apply(struct{}, uint64) error {
	return nil
}

func (p *startupCompletionProjection) CompleteStartupReplay() {
	p.completions++
}

type blockingStartupCompletionProjection struct {
	started chan struct{}
	release chan struct{}
}

func (*blockingStartupCompletionProjection) Subjects() []string {
	return []string{"evt.test.created"}
}

func (*blockingStartupCompletionProjection) Apply(struct{}, uint64) error {
	return nil
}

func (p *blockingStartupCompletionProjection) CompleteStartupReplay() {
	close(p.started)
	<-p.release
}

func TestProjectorCompletesStartupReplayOnceAcrossReentry(t *testing.T) {
	projection := &startupCompletionProjection{}
	projector := must(NewDecodedProjector(
		stubJetStream{},
		stubStream{},
		projection,
		func([]byte) (DecodedEvent[struct{}], error) {
			return DecodedEvent[struct{}]{Event: struct{}{}, ID: "test"}, nil
		},
		ProjectorOptions{},
	))
	projector.started = true

	projector.maybeCompleteStartup(time.Now())
	projector.maybeCompleteStartup(time.Now())

	if projection.completions != 1 {
		t.Fatalf("startup replay completions = %d, want 1", projection.completions)
	}
	if err := projector.WaitForStartup(t.Context()); err != nil {
		t.Fatalf("wait for completed startup: %v", err)
	}
}

func TestProjectorWaitForStartupHonorsContext(t *testing.T) {
	projector := must(NewDecodedProjector(
		stubJetStream{},
		stubStream{},
		&startupCompletionProjection{},
		func([]byte) (DecodedEvent[struct{}], error) {
			return DecodedEvent[struct{}]{Event: struct{}{}, ID: "test"}, nil
		},
		ProjectorOptions{},
	))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := projector.WaitForStartup(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait for startup error = %v, want context cancellation", err)
	}
}

func TestProjectorWaitForStartupReturnsProjectionFailure(t *testing.T) {
	projector := must(NewDecodedProjector(
		stubJetStream{},
		stubStream{},
		&startupCompletionProjection{},
		func([]byte) (DecodedEvent[struct{}], error) {
			return DecodedEvent[struct{}]{Event: struct{}{}, ID: "test"}, nil
		},
		ProjectorOptions{},
	))
	projector.fail(0, errors.New("decode failed"))

	if err := projector.WaitForStartup(t.Context()); !errors.Is(err, ErrProjectionFailed) {
		t.Fatalf("wait for startup error = %v, want ErrProjectionFailed", err)
	}
}

func TestProjectorWaitForStartupIncludesCompletionHook(t *testing.T) {
	projection := &blockingStartupCompletionProjection{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	projector := must(NewDecodedProjector(
		stubJetStream{},
		stubStream{},
		projection,
		func([]byte) (DecodedEvent[struct{}], error) {
			return DecodedEvent[struct{}]{Event: struct{}{}, ID: "test"}, nil
		},
		ProjectorOptions{},
	))
	projector.started = true
	startupFinished := make(chan struct{})
	go func() {
		projector.maybeCompleteStartup(time.Now())
		close(startupFinished)
	}()
	<-projection.started

	waitContext, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := projector.WaitForStartup(waitContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait during completion hook error = %v, want deadline exceeded", err)
	}
	if projector.Status().StartupComplete {
		t.Fatal("status reports startup complete while the completion hook runs")
	}

	close(projection.release)
	<-startupFinished
	if err := projector.WaitForStartup(t.Context()); err != nil {
		t.Fatalf("wait after completion hook: %v", err)
	}
	if !projector.Status().StartupComplete {
		t.Fatal("status does not report startup complete after the completion hook")
	}
}

// An event applied after startup ends must see a completed replay hook.
// Completion therefore must mark startup ended and run the hook in one apply
// barrier, so a concurrent apply cannot observe one without the other.
func TestProjectorCompletesStartupInsideApplyBarrier(t *testing.T) {
	projection := &startupCompletionProjection{}
	projector := must(NewDecodedProjector(
		stubJetStream{},
		stubStream{},
		projection,
		func([]byte) (DecodedEvent[struct{}], error) {
			return DecodedEvent[struct{}]{Event: struct{}{}, ID: "test"}, nil
		},
		ProjectorOptions{},
	))
	projector.started = true

	// Hold the barrier as an in-progress apply does.
	projector.applyMu.Lock()
	completed := make(chan struct{})
	go func() {
		projector.maybeCompleteStartup(time.Now())
		close(completed)
	}()
	deadline := time.Now().Add(50 * time.Millisecond)
	for time.Now().Before(deadline) {
		if projector.Status().StartupComplete {
			projector.applyMu.Unlock()
			t.Fatal("startup completed outside the apply barrier, before the replay hook ran")
		}
		time.Sleep(time.Millisecond)
	}
	projector.applyMu.Unlock()
	<-completed

	if projection.completions != 1 {
		t.Fatalf("startup replay completions = %d, want 1", projection.completions)
	}
	if !projector.Status().StartupComplete {
		t.Fatal("startup is not complete")
	}
}
