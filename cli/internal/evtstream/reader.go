package evtstream

import (
	"context"
	"fmt"
	"sync"

	"github.com/nats-io/nats.go/jetstream"
)

const exactReadConcurrency = 16

type exactStreamReader interface {
	GetMsg(context.Context, uint64, ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error)
}

// Reader loads complete Chatto events at exact EVT stream sequences. It is a
// read boundary for derived indexes that retain sequences instead of event
// payloads.
type Reader struct {
	stream exactStreamReader
}

// NewReader constructs an exact-sequence reader for the EVT stream.
func NewReader(stream jetstream.Stream) *Reader {
	return &Reader{stream: stream}
}

// EventAt loads and decodes one EVT record through a leader-backed stream
// read. The returned subject and sequence are authoritative broker metadata.
func (r *Reader) EventAt(ctx context.Context, sequence uint64) (*SubjectEvent, error) {
	if r == nil || r.stream == nil {
		return nil, fmt.Errorf("EVT reader is unavailable")
	}
	if sequence == 0 {
		return nil, fmt.Errorf("EVT sequence must be positive")
	}
	msg, err := r.stream.GetMsg(ctx, sequence)
	if err != nil {
		return nil, fmt.Errorf("read EVT sequence %d: %w", sequence, err)
	}
	if msg == nil {
		return nil, fmt.Errorf("read EVT sequence %d: broker returned no record", sequence)
	}
	if msg.Sequence != sequence {
		return nil, fmt.Errorf("read EVT sequence %d: broker returned sequence %d", sequence, msg.Sequence)
	}
	event, err := decodeEventData(msg.Data)
	if err != nil {
		return nil, fmt.Errorf("decode EVT sequence %d: %w", sequence, err)
	}
	if err := validateEvent(event); err != nil {
		return nil, fmt.Errorf("validate EVT sequence %d: %w", sequence, err)
	}
	return &SubjectEvent{Subject: msg.Subject, Sequence: msg.Sequence, Event: event}, nil
}

// EventsAt loads exact EVT sequences with bounded concurrency. Duplicate
// sequences are read once, while the returned slice preserves caller order and
// duplicate positions. The first failure cancels outstanding reads.
func (r *Reader) EventsAt(ctx context.Context, sequences []uint64) ([]*SubjectEvent, error) {
	if len(sequences) == 0 {
		return nil, nil
	}

	unique := make([]uint64, 0, len(sequences))
	uniqueIndex := make(map[uint64]int, len(sequences))
	for _, sequence := range sequences {
		if sequence == 0 {
			return nil, fmt.Errorf("EVT sequence must be positive")
		}
		if _, exists := uniqueIndex[sequence]; exists {
			continue
		}
		uniqueIndex[sequence] = len(unique)
		unique = append(unique, sequence)
	}

	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan uint64, len(unique))
	for _, sequence := range unique {
		jobs <- sequence
	}
	close(jobs)

	loaded := make([]*SubjectEvent, len(unique))
	var firstErr error
	var errOnce sync.Once
	workerCount := min(exactReadConcurrency, len(unique))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for sequence := range jobs {
				if readCtx.Err() != nil {
					return
				}
				record, err := r.EventAt(readCtx, sequence)
				if err != nil {
					errOnce.Do(func() {
						firstErr = err
						cancel()
					})
					return
				}
				loaded[uniqueIndex[sequence]] = record
			}
		}()
	}
	workers.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result := make([]*SubjectEvent, len(sequences))
	for i, sequence := range sequences {
		result[i] = loaded[uniqueIndex[sequence]]
	}
	return result, nil
}
