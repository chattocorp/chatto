package events

import (
	"context"
	"errors"
	"fmt"
	mrand "math/rand"
	"strings"
	"time"
)

// ErrInvalidMutationBoundary marks a missing or malformed mutation boundary.
var ErrInvalidMutationBoundary = errors.New("invalid mutation boundary")

// ErrInvalidMutationDecision marks a missing decision callback.
var ErrInvalidMutationDecision = errors.New("invalid mutation decision")

type mutationBoundaryKind uint8

const (
	mutationBoundaryInvalid mutationBoundaryKind = iota
	mutationBoundarySubject
	mutationBoundaryStream
)

// MutationBoundary selects the durable state that must remain unchanged
// between a mutation decision and its atomic commit.
//
// Construct boundaries with AtSubject or AtStreamTail. The zero value is
// invalid so callers cannot accidentally publish without choosing a scope.
type MutationBoundary struct {
	kind          mutationBoundaryKind
	subjectFilter string
}

// AtSubject fences a mutation against the current tail of an exact subject or
// wildcard subject filter. Use an aggregate's all-events filter to serialize
// mutations only with that aggregate.
func AtSubject(subjectOrFilter string) MutationBoundary {
	return MutationBoundary{
		kind:          mutationBoundarySubject,
		subjectFilter: subjectOrFilter,
	}
}

// AtStreamTail fences a mutation against the whole stream. Any intervening
// event conflicts, causing the decision callback to run again against a fresh
// stream tail. Use it only when the mutation's invariant genuinely spans the
// complete stream and contention with unrelated events is acceptable.
func AtStreamTail() MutationBoundary {
	return MutationBoundary{kind: mutationBoundaryStream}
}

// MutationAttempt describes one invocation of a mutation decision callback.
// ExpectedSequence is the subject/filter tail or stream tail captured before
// the callback began.
type MutationAttempt struct {
	Number           int
	ExpectedSequence uint64
}

// EncodedMutationEntry is one opaque event selected by a mutation decision.
// OCC headers are applied by ExecuteMutation from the chosen boundary.
type EncodedMutationEntry struct {
	Subject string
	Record  EncodedRecord
}

// MutationResult reports whether a mutation committed and how much OCC
// contention it encountered. An empty decision is a successful no-op and
// returns Committed false.
type MutationResult struct {
	Sequences []uint64
	Attempts  int
	Conflicts int
	Committed bool
}

const maxMutationAttempts = 5

// ExecuteMutation repeatedly captures boundary, invokes decide, and
// atomically commits the returned entries with OCC against that same boundary.
// A conflict reruns decide; other errors return immediately. Returning no
// entries represents a successful no-op.
//
// Event identifiers must describe the logical operation and remain stable
// across callback invocations. Applications remain responsible for waiting
// until their projections cover the captured facts before making a decision.
// Decisions containing multiple records require AllowAtomicPublish on the
// bound JetStream stream; single-record decisions use an ordinary OCC publish.
func (l *EncodedEventLog) ExecuteMutation(
	ctx context.Context,
	boundary MutationBoundary,
	decide func(context.Context, MutationAttempt) ([]EncodedMutationEntry, error),
) (MutationResult, error) {
	if err := validateMutationBoundary(boundary); err != nil {
		return MutationResult{}, err
	}
	if decide == nil {
		return MutationResult{}, fmt.Errorf("%w: callback is nil", ErrInvalidMutationDecision)
	}

	result := MutationResult{}
	var lastErr error
	for attempt := 1; attempt <= maxMutationAttempts; attempt++ {
		expectedSeq, err := l.mutationBoundarySeq(ctx, boundary)
		if err != nil {
			return result, err
		}
		result.Attempts = attempt

		entries, err := decide(ctx, MutationAttempt{
			Number:           attempt,
			ExpectedSequence: expectedSeq,
		})
		if err != nil {
			return result, err
		}
		if len(entries) == 0 {
			return result, nil
		}

		seqs, err := l.publishMutation(ctx, boundary, expectedSeq, entries)
		if err == nil {
			result.Sequences = seqs
			result.Committed = true
			return result, nil
		}
		if !errors.Is(err, ErrConflict) {
			return result, err
		}

		result.Conflicts++
		lastErr = err
		if l.logger != nil {
			l.logger.Debug("mutation OCC conflict, re-evaluating",
				"boundary", boundary.description(),
				"expected_seq", expectedSeq,
				"attempt", attempt,
				"max_attempts", maxMutationAttempts)
		}
		if attempt == maxMutationAttempts {
			break
		}

		baseDelay := time.Duration(1<<(attempt-1)) * time.Millisecond
		jitter := time.Duration(mrand.Int63n(int64(5 * time.Millisecond)))
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(baseDelay + jitter):
		}
	}
	return result, fmt.Errorf("execute mutation after %d attempts: %w", maxMutationAttempts, lastErr)
}

func (l *EncodedEventLog) publishMutation(
	ctx context.Context,
	boundary MutationBoundary,
	expectedSeq uint64,
	entries []EncodedMutationEntry,
) ([]uint64, error) {
	if len(entries) == 1 {
		if err := validateEncodedRecord(entries[0].Record); err != nil {
			return nil, err
		}
		var (
			seq uint64
			err error
		)
		switch boundary.kind {
		case mutationBoundarySubject:
			seq, err = l.publishAt(ctx, entries[0].Subject, entries[0].Record, expectedSeq, boundary.subjectFilter)
		case mutationBoundaryStream:
			seq, err = l.publishAtStreamTail(ctx, entries[0].Subject, entries[0].Record, expectedSeq)
		}
		if err != nil {
			return nil, err
		}
		return []uint64{seq}, nil
	}

	batch := make([]EncodedBatchEntry, len(entries))
	for i, entry := range entries {
		batch[i] = EncodedBatchEntry{Subject: entry.Subject, Record: entry.Record}
	}
	switch boundary.kind {
	case mutationBoundarySubject:
		batch[0].HasOCC = true
		batch[0].ExpectedSeq = expectedSeq
		batch[0].FilterSubject = boundary.subjectFilter
	case mutationBoundaryStream:
		batch[0].HasStreamOCC = true
		batch[0].ExpectedStreamSeq = expectedSeq
	}
	return l.AppendBatch(ctx, batch)
}

func (l *EncodedEventLog) mutationBoundarySeq(ctx context.Context, boundary MutationBoundary) (uint64, error) {
	switch boundary.kind {
	case mutationBoundarySubject:
		return l.LastSubjectSeq(ctx, boundary.subjectFilter)
	case mutationBoundaryStream:
		return l.LastStreamSeq(ctx)
	default:
		return 0, ErrInvalidMutationBoundary
	}
}

func validateMutationBoundary(boundary MutationBoundary) error {
	switch boundary.kind {
	case mutationBoundarySubject:
		if strings.TrimSpace(boundary.subjectFilter) == "" {
			return fmt.Errorf("%w: subject or filter is empty", ErrInvalidMutationBoundary)
		}
		return nil
	case mutationBoundaryStream:
		return nil
	default:
		return fmt.Errorf("%w: use AtSubject or AtStreamTail", ErrInvalidMutationBoundary)
	}
}

func (b MutationBoundary) description() string {
	if b.kind == mutationBoundaryStream {
		return "stream"
	}
	return "subject " + b.subjectFilter
}
