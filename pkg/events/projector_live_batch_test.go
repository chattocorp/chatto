package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/chattocorp/chatto/pkg/events"
)

type liveBatchProjection struct {
	codecTestBatchProjection
	size    int
	delay   time.Duration
	fail    bool
	entered chan struct{}
	release chan struct{}
}

func (p *liveBatchProjection) LiveBatchSize() int               { return p.size }
func (p *liveBatchProjection) LiveBatchMaxDelay() time.Duration { return p.delay }
func (p *liveBatchProjection) ApplyLiveBatch(items []SequencedEventOf[codecTestEvent]) error {
	if p.entered != nil {
		close(p.entered)
		<-p.release
	}
	if p.fail {
		return errors.New("commit failed")
	}
	return p.ApplyStartupBatch(items)
}

func TestLiveBatchCommitBarrier(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			js, stream := setupTestStream(t)
			projection := &liveBatchProjection{size: 2, delay: time.Hour, fail: fail, entered: make(chan struct{}), release: make(chan struct{})}
			t.Cleanup(func() {
				select {
				case <-projection.release:
				default:
					close(projection.release)
				}
			})
			projection.subject = "evt.codec.live.created"
			projector := must(NewDecodedProjector(js, stream, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger()}))
			ctx, cancel := context.WithCancel(testContext(t))
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- projector.Run(ctx) }()
			if err := projector.WaitForStartup(ctx); err != nil {
				t.Fatal(err)
			}
			log := NewEncodedEventLog(js, stream, testLogger())
			first, err := log.AppendEventually(ctx, projection.subject, EncodedRecord{ID: "one", Data: []byte("one:alpha")})
			if err != nil {
				t.Fatal(err)
			}
			last, err := log.AppendEventually(ctx, projection.subject, EncodedRecord{ID: "two", Data: []byte("two:beta")})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-projection.entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if projector.LastSeq() >= first {
				t.Error("advanced before durable commit")
			}
			close(projection.release)
			err = projector.WaitFor(ctx, StreamPosition{Seq: last, SubjectFilter: projection.subject})
			if fail {
				if !errors.Is(err, ErrProjectionFailed) {
					t.Fatalf("want failed waiter, got %v", err)
				}
				if projector.LastSeq() >= first {
					t.Fatal("failed batch advanced")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			cancel()
			<-done
		})
	}
}

func TestLiveBatchFlushesSparseTraffic(t *testing.T) {
	js, stream := setupTestStream(t)
	projection := &liveBatchProjection{size: 256, delay: 10 * time.Millisecond}
	projection.subject = "evt.codec.sparse.created"
	projector := must(NewDecodedProjector(js, stream, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger()}))
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- projector.Run(ctx) }()
	if err := projector.WaitForStartup(ctx); err != nil {
		t.Fatal(err)
	}
	seq, err := NewEncodedEventLog(js, stream, testLogger()).AppendEventually(ctx, projection.subject, EncodedRecord{ID: "one", Data: []byte("one:alpha")})
	if err != nil {
		t.Fatal(err)
	}
	if err := projector.WaitFor(ctx, StreamPosition{Seq: seq, SubjectFilter: projection.subject}); err != nil {
		t.Fatal(err)
	}
	values, batches := projection.state()
	if len(values) != 1 || len(batches) != 1 || len(batches[0]) != 1 {
		t.Fatalf("values=%v batches=%v", values, batches)
	}
	cancel()
	<-done
}

func TestLiveBatchDecodeFailureDiscardsPending(t *testing.T) {
	js, stream := setupTestStream(t)
	projection := &liveBatchProjection{size: 256, delay: time.Hour}
	projection.subject = "evt.codec.badlive.created"
	projector := must(NewDecodedProjector(js, stream, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger()}))
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- projector.Run(ctx) }()
	if err := projector.WaitForStartup(ctx); err != nil {
		t.Fatal(err)
	}
	log := NewEncodedEventLog(js, stream, testLogger())
	first, err := log.AppendEventually(ctx, projection.subject, EncodedRecord{ID: "one", Data: []byte("one:alpha")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = log.AppendEventually(ctx, projection.subject, EncodedRecord{ID: "bad", Data: []byte("invalid")})
	if err != nil {
		t.Fatal(err)
	}
	err = projector.WaitFor(ctx, StreamPosition{Seq: first, SubjectFilter: projection.subject})
	if !errors.Is(err, ErrProjectionFailed) {
		t.Fatalf("pending waiter: %v", err)
	}
	values, _ := projection.state()
	if len(values) != 0 {
		t.Fatal("committed pending batch after decode failure")
	}
	if projector.LastSeq() >= first {
		t.Fatal("advanced past pending batch")
	}
	cancel()
	<-done
}

func TestLiveBatchFollowsStartupReplay(t *testing.T) {
	js, stream := setupTestStream(t)
	projection := &liveBatchProjection{size: 256, delay: 10 * time.Millisecond}
	projection.subject = "evt.codec.transition.created"
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	log := NewEncodedEventLog(js, stream, testLogger())
	for _, record := range []EncodedRecord{{ID: "one", Data: []byte("one:alpha")}, {ID: "two", Data: []byte("two:beta")}, {ID: "three", Data: []byte("three:gamma")}} {
		if _, err := log.AppendEventually(ctx, projection.subject, record); err != nil {
			t.Fatal(err)
		}
	}
	projector := must(NewDecodedProjector(js, stream, projection, decodeCodecTestEvent, ProjectorOptions{Logger: testLogger()}))
	done := make(chan error, 1)
	go func() { done <- projector.Run(ctx) }()
	if err := projector.WaitForStartup(ctx); err != nil {
		t.Fatal(err)
	}
	seq, err := log.AppendEventually(ctx, projection.subject, EncodedRecord{ID: "four", Data: []byte("four:delta")})
	if err != nil {
		t.Fatal(err)
	}
	if err := projector.WaitFor(ctx, StreamPosition{Seq: seq, SubjectFilter: projection.subject}); err != nil {
		t.Fatal(err)
	}
	values, batches := projection.state()
	if len(values) != 4 || values[3] != "delta" || len(batches) != 3 || len(batches[0]) != 2 || len(batches[1]) != 1 || len(batches[2]) != 1 {
		t.Fatalf("values=%v batches=%v", values, batches)
	}
	if projector.Status().StartupMessages != 3 {
		t.Fatal("live event counted as startup replay")
	}
	cancel()
	<-done
}
