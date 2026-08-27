package events_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	. "hmans.de/chatto/pkg/events"
)

func TestExecuteSingleRecordMutationWithoutAtomicPublish(t *testing.T) {
	connection := startTestNATS(t)
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}
	ctx := testContext(t)
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     "MUTATION_NO_ATOMIC",
		Subjects: []string{"evt.>"},
		Storage:  jetstream.FileStorage,
	})
	if err != nil {
		t.Fatalf("create stream without atomic publish: %v", err)
	}
	eventLog := NewEncodedEventLog(js, stream, testLogger())

	result, err := eventLog.ExecuteMutation(ctx, AtStreamTail(), func(context.Context, MutationAttempt) ([]EncodedMutationEntry, error) {
		return []EncodedMutationEntry{{
			Subject: "evt.account.A.changed",
			Record:  EncodedRecord{ID: "single-no-atomic", Data: []byte("mutation")},
		}}, nil
	})
	if err != nil {
		t.Fatalf("ExecuteMutation: %v", err)
	}
	if !result.Committed || len(result.Sequences) != 1 {
		t.Fatalf("result = %+v, want one committed record", result)
	}
}

func TestExecuteMutationSubjectBoundaryIgnoresUnrelatedEvents(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)

	result, err := eventLog.ExecuteMutation(ctx, AtSubject("evt.account.A.>"), func(_ context.Context, attempt MutationAttempt) ([]EncodedMutationEntry, error) {
		if attempt.Number != 1 || attempt.ExpectedSequence != 0 {
			t.Fatalf("attempt = %+v, want first attempt at sequence 0", attempt)
		}
		if _, err := eventLog.AppendAt(ctx, "evt.account.B.changed", EncodedRecord{ID: "unrelated", Data: []byte("unrelated")}, 0); err != nil {
			t.Fatalf("append unrelated event: %v", err)
		}
		return []EncodedMutationEntry{{
			Subject: "evt.account.A.changed",
			Record:  EncodedRecord{ID: "subject-mutation", Data: []byte("mutation")},
		}}, nil
	})
	if err != nil {
		t.Fatalf("ExecuteMutation: %v", err)
	}
	if !result.Committed || result.Attempts != 1 || result.Conflicts != 0 {
		t.Fatalf("result = %+v, want one-attempt commit", result)
	}
	if len(result.Sequences) != 1 || result.Sequences[0] != 2 {
		t.Fatalf("sequences = %v, want [2]", result.Sequences)
	}
}

func TestExecuteMutationSubjectBoundaryReevaluatesAfterMatchingEvent(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	filter := "evt.account.A.>"

	result, err := eventLog.ExecuteMutation(ctx, AtSubject(filter), func(_ context.Context, attempt MutationAttempt) ([]EncodedMutationEntry, error) {
		if attempt.Number == 1 {
			if _, err := eventLog.AppendAtFilter(ctx, "evt.account.A.changed", EncodedRecord{ID: "competing", Data: []byte("competing")}, filter, 0); err != nil {
				t.Fatalf("append competing event: %v", err)
			}
		}
		return []EncodedMutationEntry{{
			Subject: "evt.account.A.changed",
			Record:  EncodedRecord{ID: "subject-retry", Data: []byte("mutation")},
		}}, nil
	})
	if err != nil {
		t.Fatalf("ExecuteMutation: %v", err)
	}
	if !result.Committed || result.Attempts != 2 || result.Conflicts != 1 {
		t.Fatalf("result = %+v, want commit after one conflict", result)
	}
}

func TestExecuteMutationStreamBoundaryReevaluatesAfterAnyEvent(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)

	result, err := eventLog.ExecuteMutation(ctx, AtStreamTail(), func(_ context.Context, attempt MutationAttempt) ([]EncodedMutationEntry, error) {
		if attempt.Number == 1 {
			if attempt.ExpectedSequence != 0 {
				t.Fatalf("first expected sequence = %d, want 0", attempt.ExpectedSequence)
			}
			if _, err := eventLog.AppendAt(ctx, "evt.authorization.changed", EncodedRecord{ID: "revocation", Data: []byte("revoked")}, 0); err != nil {
				t.Fatalf("append intervening event: %v", err)
			}
		} else if attempt.Number == 2 && attempt.ExpectedSequence != 1 {
			t.Fatalf("second expected sequence = %d, want 1", attempt.ExpectedSequence)
		}
		return []EncodedMutationEntry{{
			Subject: "evt.room.R1.reaction_added",
			Record:  EncodedRecord{ID: "reaction", Data: []byte("reaction")},
		}}, nil
	})
	if err != nil {
		t.Fatalf("ExecuteMutation: %v", err)
	}
	if !result.Committed || result.Attempts != 2 || result.Conflicts != 1 {
		t.Fatalf("result = %+v, want commit after one conflict", result)
	}
	if len(result.Sequences) != 1 || result.Sequences[0] != 2 {
		t.Fatalf("sequences = %v, want [2]", result.Sequences)
	}
}

func TestExecuteMutationStreamBoundaryRerunsAuthorizationDecision(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	errDenied := errors.New("permission denied")
	authorized := true

	result, err := eventLog.ExecuteMutation(ctx, AtStreamTail(), func(_ context.Context, attempt MutationAttempt) ([]EncodedMutationEntry, error) {
		if !authorized {
			return nil, errDenied
		}
		if attempt.Number == 1 {
			if _, err := eventLog.AppendAt(ctx, "evt.authorization.changed", EncodedRecord{ID: "deny", Data: []byte("deny")}, 0); err != nil {
				t.Fatalf("append authorization change: %v", err)
			}
			authorized = false
		}
		return []EncodedMutationEntry{{
			Subject: "evt.room.R1.reaction_added",
			Record:  EncodedRecord{ID: "denied-reaction", Data: []byte("reaction")},
		}}, nil
	})
	if !errors.Is(err, errDenied) {
		t.Fatalf("ExecuteMutation error = %v, want permission denied", err)
	}
	if result.Committed || result.Attempts != 2 || result.Conflicts != 1 {
		t.Fatalf("result = %+v, want denial after one conflict", result)
	}
	if _, err := stream.GetLastMsgForSubject(ctx, "evt.room.R1.reaction_added"); err == nil {
		t.Fatal("denied mutation was committed")
	}
}

func TestExecuteMutationEmptyDecisionIsNoop(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)

	result, err := eventLog.ExecuteMutation(ctx, AtStreamTail(), func(context.Context, MutationAttempt) ([]EncodedMutationEntry, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("ExecuteMutation: %v", err)
	}
	if result.Committed || result.Attempts != 1 || result.Conflicts != 0 || len(result.Sequences) != 0 {
		t.Fatalf("result = %+v, want one-attempt no-op", result)
	}
}

func TestExecuteMutationReportsDuplicateSingleRecordAsNotCommitted(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	entry := EncodedMutationEntry{
		Subject: "evt.account.A.changed",
		Record:  EncodedRecord{ID: "same-logical-event", Data: []byte("mutation")},
	}

	first, err := eventLog.ExecuteMutation(ctx, AtSubject(entry.Subject), func(context.Context, MutationAttempt) ([]EncodedMutationEntry, error) {
		return []EncodedMutationEntry{entry}, nil
	})
	if err != nil || !first.Committed {
		t.Fatalf("first mutation = %+v, %v, want committed", first, err)
	}
	second, err := eventLog.ExecuteMutation(ctx, AtSubject(entry.Subject), func(context.Context, MutationAttempt) ([]EncodedMutationEntry, error) {
		return []EncodedMutationEntry{entry}, nil
	})
	if err != nil {
		t.Fatalf("duplicate mutation: %v", err)
	}
	if second.Committed || len(second.Sequences) != 1 || second.Sequences[0] != first.Sequences[0] {
		t.Fatalf("duplicate mutation = %+v, want original sequence and committed=false", second)
	}
}

func TestExecuteMutationRejectsInvalidBoundary(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)

	_, err := eventLog.ExecuteMutation(ctx, MutationBoundary{}, func(context.Context, MutationAttempt) ([]EncodedMutationEntry, error) {
		return nil, nil
	})
	if !errors.Is(err, ErrInvalidMutationBoundary) {
		t.Fatalf("ExecuteMutation error = %v, want ErrInvalidMutationBoundary", err)
	}
}

func TestExecuteMutationRejectsNilDecision(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)

	_, err := eventLog.ExecuteMutation(ctx, AtStreamTail(), nil)
	if !errors.Is(err, ErrInvalidMutationDecision) {
		t.Fatalf("ExecuteMutation error = %v, want ErrInvalidMutationDecision", err)
	}
}
