package events_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"hmans.de/chatto/pkg/events"
)

const ledgerSubject = "evt.ledger.account-1.entry_recorded"

type ledgerEntry struct {
	ID    string `json:"id"`
	Delta int    `json:"delta"`
}

type ledgerAdapter struct {
	log *events.EncodedEventLog
}

func (a ledgerAdapter) appendAt(
	ctx context.Context,
	entry ledgerEntry,
	expectedSequence uint64,
) (events.StreamPosition, error) {
	data, err := json.Marshal(entry)
	if err != nil {
		return events.StreamPosition{}, err
	}
	sequence, err := a.log.AppendAt(
		ctx,
		ledgerSubject,
		events.EncodedRecord{ID: entry.ID, Data: data},
		expectedSequence,
	)
	if err != nil {
		return events.StreamPosition{}, err
	}
	return events.SubjectPosition(ledgerSubject, sequence), nil
}

func decodeLedgerEntry(data []byte) (events.DecodedEvent[ledgerEntry], error) {
	var entry ledgerEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return events.DecodedEvent[ledgerEntry]{}, err
	}
	return events.DecodedEvent[ledgerEntry]{
		Event: entry,
		ID:    entry.ID,
	}, nil
}

type ledgerBalanceProjection struct {
	events.MemoryProjection
	balance   int
	entryIDs  []string
	sequences []uint64
}

func (*ledgerBalanceProjection) Subjects() []string {
	return []string{ledgerSubject}
}

func (p *ledgerBalanceProjection) Apply(entry ledgerEntry, sequence uint64) error {
	p.Lock()
	defer p.Unlock()
	p.balance += entry.Delta
	p.entryIDs = append(p.entryIDs, entry.ID)
	p.sequences = append(p.sequences, sequence)
	return nil
}

func (p *ledgerBalanceProjection) state() (int, []string, []uint64) {
	p.RLock()
	defer p.RUnlock()
	return p.balance, slices.Clone(p.entryIDs), slices.Clone(p.sequences)
}

func runConsumerProjector(t *testing.T, projector *events.Projector) func() {
	t.Helper()
	runContext, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- projector.Run(runContext)
	}()

	var once sync.Once
	stop := func() {
		t.Helper()
		once.Do(func() {
			cancel()
			select {
			case err := <-errCh:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("projector shutdown error = %v, want context cancellation", err)
				}
			case <-time.After(2 * time.Second):
				t.Error("projector did not stop after context cancellation")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

func TestFrameworkSupportsAnExternalJSONConsumer(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := events.NewEncodedEventLog(js, stream, testLogger())
	ledger := ledgerAdapter{log: eventLog}
	ctx := testContext(t)

	liveProjection := &ledgerBalanceProjection{}
	liveHandle := events.NewDecodedProjectionHandle(
		js,
		stream,
		liveProjection,
		decodeLedgerEntry,
		testLogger(),
	)
	stopLiveProjector := runConsumerProjector(t, liveHandle.Projector())
	waitFor(t, 2*time.Second, func() bool {
		return liveHandle.Projector().Status().StartupComplete
	})

	firstPosition, err := ledger.appendAt(ctx, ledgerEntry{ID: "entry-1", Delta: 7}, 0)
	if err != nil {
		t.Fatalf("append first ledger entry: %v", err)
	}
	if err := liveHandle.Projector().WaitFor(ctx, firstPosition); err != nil {
		t.Fatalf("wait for first ledger entry: %v", err)
	}
	if balance, entryIDs, sequences := liveProjection.state(); balance != 7 ||
		!slices.Equal(entryIDs, []string{"entry-1"}) ||
		!slices.Equal(sequences, []uint64{firstPosition.Seq}) {
		t.Fatalf(
			"state after first entry = balance %d, IDs %v, sequences %v",
			balance,
			entryIDs,
			sequences,
		)
	}

	secondPosition, err := ledger.appendAt(
		ctx,
		ledgerEntry{ID: "entry-2", Delta: 5},
		firstPosition.Seq,
	)
	if err != nil {
		t.Fatalf("append second ledger entry: %v", err)
	}
	if err := liveHandle.Projector().WaitFor(ctx, secondPosition); err != nil {
		t.Fatalf("wait for second ledger entry: %v", err)
	}

	if _, err := ledger.appendAt(
		ctx,
		ledgerEntry{ID: "entry-stale", Delta: 100},
		firstPosition.Seq,
	); !errors.Is(err, events.ErrConflict) {
		t.Fatalf("stale ledger append error = %v, want events.ErrConflict", err)
	}
	tail, err := eventLog.LastSubjectPosition(ctx, ledgerSubject)
	if err != nil {
		t.Fatalf("read ledger tail: %v", err)
	}
	if tail != secondPosition {
		t.Fatalf("ledger tail after stale append = %+v, want %+v", tail, secondPosition)
	}

	wantBalance, wantEntryIDs, wantSequences := liveProjection.state()
	if wantBalance != 12 ||
		!slices.Equal(wantEntryIDs, []string{"entry-1", "entry-2"}) ||
		!slices.Equal(wantSequences, []uint64{firstPosition.Seq, secondPosition.Seq}) {
		t.Fatalf(
			"live state = balance %d, IDs %v, sequences %v",
			wantBalance,
			wantEntryIDs,
			wantSequences,
		)
	}
	stopLiveProjector()

	replayedProjection := &ledgerBalanceProjection{}
	replayedHandle := events.NewDecodedProjectionHandle(
		js,
		stream,
		replayedProjection,
		decodeLedgerEntry,
		testLogger(),
	)
	runConsumerProjector(t, replayedHandle.Projector())
	if err := replayedHandle.Projector().WaitFor(ctx, secondPosition); err != nil {
		t.Fatalf("wait for replayed ledger history: %v", err)
	}

	gotBalance, gotEntryIDs, gotSequences := replayedProjection.state()
	if gotBalance != wantBalance ||
		!slices.Equal(gotEntryIDs, wantEntryIDs) ||
		!slices.Equal(gotSequences, wantSequences) {
		t.Fatalf(
			"replayed state = balance %d, IDs %v, sequences %v; want balance %d, IDs %v, sequences %v",
			gotBalance,
			gotEntryIDs,
			gotSequences,
			wantBalance,
			wantEntryIDs,
			wantSequences,
		)
	}
}
