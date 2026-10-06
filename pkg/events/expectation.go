package events

import (
	"fmt"
	"strconv"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type subjectGuardKind uint8

const (
	noSubjectGuard subjectGuardKind = iota
	ownSubjectGuard
	filterGuard
)

// Expectation is an optimistic-concurrency guard for one write. JetStream
// commits the write only when each guarded tail still has its expected last
// sequence; otherwise the write fails with ErrConflict. An expected sequence of
// zero means that no message exists yet.
//
// An Expectation has a subject guard (ExpectSubjectSeq or ExpectFilterSeq), a
// stream guard (ExpectStreamSeq), or both (AndStreamSeq). The zero value is no
// guard.
type Expectation struct {
	subject   subjectGuardKind
	filter    string
	seq       uint64
	stream    bool
	streamSeq uint64
}

// ExpectSubjectSeq guards a write with the last sequence of the record's own
// subject.
func ExpectSubjectSeq(seq uint64) Expectation {
	return Expectation{subject: ownSubjectGuard, seq: seq}
}

// ExpectFilterSeq guards a write with the last sequence of all messages that
// match filter, an exact subject or a wildcard subject filter. Use an
// aggregate's all-events filter to serialize writes with that aggregate.
func ExpectFilterSeq(filter string, seq uint64) Expectation {
	return Expectation{subject: filterGuard, filter: filter, seq: seq}
}

// ExpectStreamSeq guards a write with the last sequence of the complete
// stream. Any intervening event in the stream causes a conflict.
func ExpectStreamSeq(seq uint64) Expectation {
	return Expectation{stream: true, streamSeq: seq}
}

// AndStreamSeq returns e with an additional stream guard. JetStream then checks
// both guards.
func (e Expectation) AndStreamSeq(seq uint64) Expectation {
	e.stream = true
	e.streamSeq = seq
	return e
}

// IsZero reports whether the Expectation is no guard.
func (e Expectation) IsZero() bool {
	return e.subject == noSubjectGuard && !e.stream
}

// validate rejects a zero guard and a filter guard without a filter.
func (e Expectation) validate() error {
	if e.IsZero() {
		return ErrMissingOCC
	}
	if e.subject == filterGuard && e.filter == "" {
		return fmt.Errorf("%w: filter is empty", ErrInvalidBatchOCC)
	}
	return nil
}

// publishOpts returns the JetStream publish options for a single-record write.
func (e Expectation) publishOpts() []jetstream.PublishOpt {
	var opts []jetstream.PublishOpt
	switch e.subject {
	case ownSubjectGuard:
		opts = append(opts, jetstream.WithExpectLastSequencePerSubject(e.seq))
	case filterGuard:
		opts = append(opts, jetstream.WithExpectLastSequenceForSubject(e.seq, e.filter))
	}
	if e.stream {
		opts = append(opts, jetstream.WithExpectLastSequence(e.streamSeq))
	}
	return opts
}

// setHeaders adds the guard headers to a batch message. They are the headers
// that the single-record publish options send.
func (e Expectation) setHeaders(hdr nats.Header) {
	switch e.subject {
	case ownSubjectGuard:
		hdr.Set(jetstream.ExpectedLastSubjSeqHeader, strconv.FormatUint(e.seq, 10))
	case filterGuard:
		hdr.Set(jetstream.ExpectedLastSubjSeqHeader, strconv.FormatUint(e.seq, 10))
		hdr.Set(jetstream.ExpectedLastSubjSeqSubjHeader, e.filter)
	}
	if e.stream {
		hdr.Set(jetstream.ExpectedLastSeqHeader, strconv.FormatUint(e.streamSeq, 10))
	}
}

// conflict returns the ErrConflict error for this guard on subject. It names
// the guard when there is one, because JetStream does not report which of
// two guards failed.
func (e Expectation) conflict(subject string) error {
	switch {
	case e.subject != noSubjectGuard && e.stream:
		return fmt.Errorf("batch entry OCC guards: %w", ErrConflict)
	case e.stream:
		return fmt.Errorf("stream at expected seq %d: %w", e.streamSeq, ErrConflict)
	case e.subject == filterGuard:
		return fmt.Errorf("filter %s at expected seq %d: %w", e.filter, e.seq, ErrConflict)
	default:
		return fmt.Errorf("%s at expected seq %d: %w", subject, e.seq, ErrConflict)
	}
}

// guards returns the number of guards in e.
func (e Expectation) guards() int {
	n := 0
	if e.subject != noSubjectGuard {
		n++
	}
	if e.stream {
		n++
	}
	return n
}
