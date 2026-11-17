package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// blockingStallProjection applies events immediately, except an event with
// the data "block", which waits until release is closed.
type blockingStallProjection struct {
	release chan struct{}
}

func (*blockingStallProjection) Subjects() []string { return []string{"evt.stall.>"} }
func (p *blockingStallProjection) Apply(event string, _ uint64) error {
	if event == "block" {
		<-p.release
	}
	return nil
}

func waitForProjectorStatus(t *testing.T, projector *Projector, condition func(ProjectorStatus) bool) ProjectorStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		status := projector.Status()
		if condition(status) {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("projector status did not reach the expected state: %#v", status)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestProjectorReportsBlockedCommitAfterStartupAsStalled(t *testing.T) {
	connection := startTestNATS(t)
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "EVENTS_STALL_TEST", Subjects: []string{"evt.stall.>"}})
	if err != nil {
		t.Fatal(err)
	}
	eventLog := NewEncodedEventLog(js, stream, nil)
	appendEvent := func(data string) {
		t.Helper()
		if _, err := eventLog.AppendEventually(ctx, "evt.stall.item", EncodedRecord{ID: data, Data: []byte(data)}); err != nil {
			t.Fatal(err)
		}
	}
	appendEvent("first")

	const stallAfter = 100 * time.Millisecond
	projection := &blockingStallProjection{release: make(chan struct{})}
	projector := must(NewDecodedProjector(js, stream, projection, func(data []byte) (DecodedEvent[string], error) {
		return DecodedEvent[string]{Event: string(data), ID: string(data)}, nil
	}, ProjectorOptions{StallAfter: stallAfter}))
	projector.sourceCheckInterval = 10 * time.Millisecond
	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- projector.Run(runCtx) }()
	t.Cleanup(func() {
		stop()
		<-done
	})

	status := waitForProjectorStatus(t, projector, func(s ProjectorStatus) bool { return s.StartupComplete })
	if status.LastCommitAt.IsZero() || status.ObservedSeq != status.LastSeq {
		t.Fatalf("caught-up status = %#v", status)
	}

	// An idle, caught-up projector stays healthy without new writes.
	time.Sleep(3 * stallAfter)
	if status := projector.Status(); status.Stalled || status.PendingFor != 0 || status.CurrentOperation != ProjectorIdle {
		t.Fatalf("idle projector status = %#v", status)
	}

	appendEvent("block")
	status = waitForProjectorStatus(t, projector, func(s ProjectorStatus) bool { return s.Stalled })
	if status.CurrentOperation != ProjectorApplying || status.CurrentOperationDuration < stallAfter || status.ObservedSeq <= status.LastSeq {
		t.Fatalf("stalled status = %#v", status)
	}

	close(projection.release)
	status = waitForProjectorStatus(t, projector, func(s ProjectorStatus) bool { return s.LastSeq == s.ObservedSeq })
	if status.Stalled || status.PendingFor != 0 || status.LastCommitDuration < stallAfter {
		t.Fatalf("recovered status = %#v", status)
	}
}

// statusProjector returns a running projector for status tests that need no
// stream.
func statusProjector(stallAfter time.Duration) *Projector {
	return &Projector{started: true, stallAfter: stallAfter, failedCh: make(chan struct{})}
}

func TestProjectorStallNeedsPendingWork(t *testing.T) {
	const stallAfter = time.Minute
	p := statusProjector(stallAfter)
	p.advance(10)
	p.observeSource(10)
	if status := p.statusAt(time.Now().Add(time.Hour)); status.Stalled || status.PendingFor != 0 {
		t.Fatalf("caught-up projector status = %#v", status)
	}

	// The source check finds work that the consumer has not delivered.
	p.observeSource(12)
	if status := p.statusAt(time.Now().Add(stallAfter + time.Second)); !status.Stalled || status.ObservedSeq != 12 {
		t.Fatalf("undelivered work status = %#v", status)
	}

	// A deleted newest event lowers the source sequence and ends the stall.
	p.observeSource(10)
	if status := p.statusAt(time.Now().Add(time.Hour)); status.Stalled || status.PendingFor != 0 {
		t.Fatalf("status after source deletion = %#v", status)
	}
}

func TestProjectorAdvancingReplayIsNotStalled(t *testing.T) {
	const stallAfter = time.Minute
	p := statusProjector(stallAfter)
	p.observeSource(100)
	for seq := uint64(1); seq < 100; seq++ {
		p.advance(seq)
		// Each commit restarts the interval, so only a full interval without
		// progress after the latest commit is a stall.
		if status := p.statusAt(time.Now().Add(stallAfter - time.Second)); status.Stalled {
			t.Fatalf("advancing replay stalled at seq %d: %#v", seq, status)
		}
	}
	if status := p.statusAt(time.Now().Add(stallAfter + time.Second)); !status.Stalled {
		t.Fatalf("replay without progress status = %#v", status)
	}
	p.advance(100)
	if status := p.statusAt(time.Now().Add(time.Hour)); status.Stalled {
		t.Fatalf("completed replay status = %#v", status)
	}
}

func TestProjectorFailedOrStoppedIsNotStalled(t *testing.T) {
	p := statusProjector(time.Minute)
	p.observeSource(5)
	p.fail(1, errors.New("broken"))
	if status := p.statusAt(time.Now().Add(time.Hour)); status.Stalled || status.Err == nil {
		t.Fatalf("failed projector status = %#v", status)
	}
	notStarted := &Projector{stallAfter: time.Minute}
	notStarted.observeSource(5)
	if status := notStarted.statusAt(time.Now().Add(time.Hour)); status.Stalled {
		t.Fatalf("projector before Run status = %#v", status)
	}
}

func TestProjectorRejectsNegativeStallThreshold(t *testing.T) {
	connection := startTestNATS(t)
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.CreateStream(context.Background(), jetstream.StreamConfig{Name: "EVENTS_STALL_OPTIONS", Subjects: []string{"evt.stall.>"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewDecodedProjector(js, stream, &blockingStallProjection{}, func(data []byte) (DecodedEvent[string], error) {
		return DecodedEvent[string]{Event: string(data)}, nil
	}, ProjectorOptions{StallAfter: -time.Second})
	if err == nil {
		t.Fatal("NewDecodedProjector accepted a negative stall threshold")
	}
}
