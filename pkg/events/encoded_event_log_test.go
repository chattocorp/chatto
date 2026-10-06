package events_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	. "hmans.de/chatto/pkg/events"
)

func TestEncodedEventLogPreservesOpaqueRecord(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	subject := "evt.compatibility.record.created"
	data := []byte{0x00, 0xff, 0x10, 0x80, 0x01}

	seq, err := eventLog.AppendAt(ctx, subject, EncodedRecord{ID: "opaque-1", Data: data}, ExpectSubjectSeq(0))
	if err != nil {
		t.Fatalf("AppendAt: %v", err)
	}
	stored, err := stream.GetMsg(ctx, seq)
	if err != nil {
		t.Fatalf("GetMsg: %v", err)
	}
	if !bytes.Equal(stored.Data, data) {
		t.Fatalf("stored data = %x, want %x", stored.Data, data)
	}
	if got := stored.Header.Get(jetstream.MsgIDHeader); got != "opaque-1" {
		t.Fatalf("Nats-Msg-Id = %q, want opaque-1", got)
	}

	page, err := eventLog.SubjectRecordsAfterPage(ctx, subject, 0, 10, 0)
	if err != nil {
		t.Fatalf("SubjectRecordsAfterPage: %v", err)
	}
	records := page.Records
	if len(records) != 1 || page.LastSequence != seq || page.More {
		t.Fatalf("records=%d lastSeq=%d more=%v, want 1, %d, and false", len(records), page.LastSequence, page.More, seq)
	}
	if records[0].Subject != subject || records[0].Sequence != seq || records[0].ID != "opaque-1" || !bytes.Equal(records[0].Data, data) {
		t.Fatalf("record = %+v, want subject=%q sequence=%d data=%x", records[0], subject, seq, data)
	}
	records[0].Data[0] ^= 0xff
	storedAgain, err := stream.GetMsg(ctx, seq)
	if err != nil {
		t.Fatalf("GetMsg after caller mutation: %v", err)
	}
	if !bytes.Equal(storedAgain.Data, data) {
		t.Fatal("mutating returned record changed durable data")
	}
}

func TestEncodedEventLogConcurrentStreamAndSubjectPositions(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	const subject = "evt.compatibility.concurrent.created"
	if _, err := eventLog.AppendEventually(ctx, subject, EncodedRecord{ID: "concurrent-1", Data: []byte("data")}); err != nil {
		t.Fatalf("AppendEventually: %v", err)
	}

	var workers sync.WaitGroup
	errorsByWorker := make(chan error, 2)
	workers.Add(2)
	go func() {
		defer workers.Done()
		for range 100 {
			if _, err := eventLog.LastStreamSeq(ctx); err != nil {
				errorsByWorker <- err
				return
			}
		}
	}()
	go func() {
		defer workers.Done()
		for range 100 {
			if _, err := eventLog.LastSubjectSeq(ctx, subject); err != nil {
				errorsByWorker <- err
				return
			}
		}
	}()
	workers.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		t.Fatalf("concurrent position read: %v", err)
	}
}

func TestEncodedEventLogAppliesPerRecordTTL(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)

	seq, err := eventLog.AppendAt(ctx, "evt.compatibility.expiring", EncodedRecord{
		ID:   "expiring-1",
		Data: []byte("temporary"),
		TTL:  time.Second,
	}, ExpectSubjectSeq(0))
	if err != nil {
		t.Fatalf("AppendAt: %v", err)
	}
	waitFor(t, 3*time.Second, func() bool {
		_, err := stream.GetMsg(ctx, seq)
		return errors.Is(err, jetstream.ErrMsgNotFound)
	})
}

func TestEncodedEventLogRejectsNegativePerRecordTTL(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	_, err := eventLog.AppendEventually(testContext(t), "evt.compatibility.expiring", EncodedRecord{
		ID:  "invalid-expiry",
		TTL: -time.Second,
	})
	if !errors.Is(err, ErrInvalidEncodedRecord) {
		t.Fatalf("negative TTL error = %v, want ErrInvalidEncodedRecord", err)
	}
}

func TestSubjectRecordsAfterPageBoundsRecordsAndBytes(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	subject := "evt.compatibility.page.created"
	for i := range 5 {
		if _, err := eventLog.AppendEventually(ctx, subject, EncodedRecord{
			ID:   "page-" + strconv.Itoa(i),
			Data: []byte("data"),
		}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := eventLog.SubjectRecordsAfterPage(ctx, subject, 0, 2, 100)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Records) != 2 || !first.More || first.LastSequence == 0 {
		t.Fatalf("first page = %+v, want two records and more", first)
	}
	second, err := eventLog.SubjectRecordsAfterPage(ctx, subject, first.LastSequence, 2, 100)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Records) != 2 || !second.More || second.LastSequence <= first.LastSequence {
		t.Fatalf("second page = %+v, want two records and more", second)
	}
	third, err := eventLog.SubjectRecordsAfterPage(ctx, subject, second.LastSequence, 2, 100)
	if err != nil {
		t.Fatalf("third page: %v", err)
	}
	if len(third.Records) != 1 || third.More || third.LastSequence <= second.LastSequence {
		t.Fatalf("third page = %+v, want final record", third)
	}

	shortPage, err := eventLog.SubjectRecordsAfterPage(ctx, subject, 0, 2, 5)
	if err != nil {
		t.Fatalf("byte-bounded page: %v", err)
	}
	if len(shortPage.Records) != 1 || !shortPage.More || shortPage.LastSequence != first.Records[0].Sequence {
		t.Fatalf("byte-bounded page = %+v, want first record only and more", shortPage)
	}
	if _, err := eventLog.SubjectRecordsAfterPage(ctx, subject, 0, 2, 3); !errors.Is(err, ErrInvalidSubjectReadLimit) {
		t.Fatalf("oversized record error = %v, want ErrInvalidSubjectReadLimit", err)
	}
	bytePage, err := eventLog.SubjectRecordsAfterPage(ctx, subject, 0, 1, 4)
	if err != nil {
		t.Fatalf("single-record byte page: %v", err)
	}
	if len(bytePage.Records) != 1 || !bytePage.More {
		t.Fatalf("byte page = %+v, want one record and more", bytePage)
	}
}

func TestSubjectRecordsAfterPageSplitsHistoryAtByteBudget(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	subject := "evt.compatibility.page.bytes"
	payload := bytes.Repeat([]byte("x"), 100)
	var want []uint64
	for i := range 10 {
		sequence, err := eventLog.AppendEventually(ctx, subject, EncodedRecord{
			ID:   "bytes-" + strconv.Itoa(i),
			Data: payload,
		})
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, sequence)
	}

	for _, maxBytes := range []int{100, 350, 1 << 20} {
		t.Run(strconv.Itoa(maxBytes), func(t *testing.T) {
			var got []uint64
			var afterSeq uint64
			for pages := 0; ; pages++ {
				if pages > len(want) {
					t.Fatalf("read %d pages without reaching the end; got %v", pages, got)
				}
				page, err := eventLog.SubjectRecordsAfterPage(ctx, subject, afterSeq, 500, maxBytes)
				if err != nil {
					t.Fatalf("page after %d: %v", afterSeq, err)
				}
				if len(page.Records) == 0 {
					t.Fatalf("page after %d is empty; more = %v", afterSeq, page.More)
				}
				pageBytes := 0
				for _, record := range page.Records {
					pageBytes += len(record.Data)
					got = append(got, record.Sequence)
				}
				if pageBytes > maxBytes {
					t.Fatalf("page holds %d payload bytes, want at most %d", pageBytes, maxBytes)
				}
				if !page.More {
					break
				}
				afterSeq = page.LastSequence
			}
			if !slices.Equal(got, want) {
				t.Fatalf("sequences = %v, want %v without gaps", got, want)
			}
		})
	}
}

func TestSubjectRecordsAfterPageEndsBeforeOversizedRecord(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	subject := "evt.compatibility.page.oversized"
	var sequences []uint64
	for i, data := range [][]byte{[]byte("small"), []byte("small"), bytes.Repeat([]byte("x"), 100)} {
		sequence, err := eventLog.AppendEventually(ctx, subject, EncodedRecord{ID: "oversized-" + strconv.Itoa(i), Data: data})
		if err != nil {
			t.Fatal(err)
		}
		sequences = append(sequences, sequence)
	}

	page, err := eventLog.SubjectRecordsAfterPage(ctx, subject, 0, 500, 50)
	if err != nil {
		t.Fatalf("page before oversized record: %v", err)
	}
	if len(page.Records) != 2 || !page.More || page.LastSequence != sequences[1] {
		t.Fatalf("page = %+v, want both small records and more", page)
	}
	if _, err := eventLog.SubjectRecordsAfterPage(ctx, subject, page.LastSequence, 500, 50); !errors.Is(err, ErrInvalidSubjectReadLimit) {
		t.Fatalf("oversized first record error = %v, want ErrInvalidSubjectReadLimit", err)
	}
}

func TestSubjectRecordsAfterPageRejectsUnboundedLimits(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	for name, limits := range map[string][2]int{
		"zero records":   {0, 1},
		"negative bytes": {1, -1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := eventLog.SubjectRecordsAfterPage(ctx, "evt.compatibility.page.invalid", 0, limits[0], limits[1]); !errors.Is(err, ErrInvalidSubjectReadLimit) {
				t.Fatalf("error = %v, want ErrInvalidSubjectReadLimit", err)
			}
		})
	}
}

func TestEncodedEventLogAppendAtUsesWildcardTail(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	filter := "evt.compatibility.*"

	firstSeq, err := eventLog.AppendAt(ctx, "evt.compatibility.first", EncodedRecord{ID: "first", Data: []byte("first")}, ExpectFilterSeq(filter, 0))
	if err != nil {
		t.Fatalf("first AppendAt: %v", err)
	}
	if _, err := eventLog.AppendAt(ctx, "evt.compatibility.second", EncodedRecord{ID: "second", Data: []byte("second")}, ExpectFilterSeq(filter, 0)); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale wildcard append error = %v, want ErrConflict", err)
	}
	secondSeq, err := eventLog.AppendAt(ctx, "evt.compatibility.second", EncodedRecord{ID: "second", Data: []byte("second")}, ExpectFilterSeq(filter, firstSeq))
	if err != nil {
		t.Fatalf("current wildcard AppendAt: %v", err)
	}
	stored, err := stream.GetMsg(ctx, secondSeq)
	if err != nil {
		t.Fatalf("GetMsg: %v", err)
	}
	if got, want := stored.Header.Get(jetstream.ExpectedLastSubjSeqHeader), strconv.FormatUint(firstSeq, 10); got != want {
		t.Fatalf("expected-last-subject-sequence = %q, want %q", got, want)
	}
	if got := stored.Header.Get(jetstream.ExpectedLastSubjSeqSubjHeader); got != filter {
		t.Fatalf("expected-last-subject filter = %q, want %q", got, filter)
	}
}

func TestEncodedEventLogAtomicBatchPreservesBytesAndOrder(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	entries := []EncodedBatchEntry{
		{
			Subject: "evt.compatibility.batch.first",
			Record:  EncodedRecord{ID: "batch-first", Data: []byte{0x00, 0x01}},
			Expect:  ExpectSubjectSeq(0),
		},
		{
			Subject: "evt.compatibility.batch.second",
			Record:  EncodedRecord{ID: "batch-second", Data: []byte{0xfe, 0xff}, TTL: time.Second},
		},
	}

	seqs, err := eventLog.AppendBatch(ctx, entries)
	if err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}
	if len(seqs) != len(entries) || seqs[1] != seqs[0]+1 {
		t.Fatalf("batch sequences = %v, want two contiguous entries", seqs)
	}
	for i, seq := range seqs {
		stored, err := stream.GetMsg(ctx, seq)
		if err != nil {
			t.Fatalf("GetMsg(%d): %v", seq, err)
		}
		if stored.Subject != entries[i].Subject || !bytes.Equal(stored.Data, entries[i].Record.Data) {
			t.Fatalf("stored batch entry %d = subject %q data %x", i, stored.Subject, stored.Data)
		}
		if got := stored.Header.Get(jetstream.MsgIDHeader); got != entries[i].Record.ID {
			t.Fatalf("entry %d Nats-Msg-Id = %q, want %q", i, got, entries[i].Record.ID)
		}
	}
	entries[0].Expect = ExpectSubjectSeq(seqs[0])
	if _, err := eventLog.AppendBatch(ctx, entries); !errors.Is(err, ErrDuplicateBatchMessageID) {
		t.Fatalf("idempotent batch retry error = %v, want ErrDuplicateBatchMessageID", err)
	}
	waitFor(t, 3*time.Second, func() bool {
		_, err := stream.GetMsg(ctx, seqs[1])
		return errors.Is(err, jetstream.ErrMsgNotFound)
	})
}

func TestEncodedEventLogRejectsMissingRecordIDAndUnguardedBatch(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)

	if _, err := eventLog.AppendAt(ctx, "evt.compatibility.invalid", EncodedRecord{Data: []byte("data")}, ExpectSubjectSeq(0)); !errors.Is(err, ErrInvalidEncodedRecord) {
		t.Fatalf("missing ID error = %v, want ErrInvalidEncodedRecord", err)
	}
	if _, err := eventLog.AppendBatch(ctx, []EncodedBatchEntry{{
		Subject: "evt.compatibility.unguarded",
		Record:  EncodedRecord{ID: "unguarded", Data: []byte("data")},
	}}); !errors.Is(err, ErrMissingOCC) {
		t.Fatalf("unguarded batch error = %v, want ErrMissingOCC", err)
	}
	if _, err := eventLog.AppendBatch(ctx, []EncodedBatchEntry{
		{
			Subject: "evt.compatibility.first",
			Record:  EncodedRecord{ID: "first", Data: []byte("first")},
		},
		{
			Subject: "evt.compatibility.second",
			Record:  EncodedRecord{ID: "second", Data: []byte("second")},
			Expect:  ExpectStreamSeq(0),
		},
	}); !errors.Is(err, ErrInvalidOCC) {
		t.Fatalf("misplaced stream OCC error = %v, want ErrInvalidOCC", err)
	}
}

func TestEncodedEventLogReportsAmbiguousMultiGuardConflict(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)

	if _, err := eventLog.AppendAt(ctx, "evt.compatibility.guarded.second", EncodedRecord{ID: "seed-second", Data: []byte("seed")}, ExpectSubjectSeq(0)); err != nil {
		t.Fatalf("seed second subject: %v", err)
	}
	_, err := eventLog.AppendBatch(ctx, []EncodedBatchEntry{
		{
			Subject: "evt.compatibility.guarded.first",
			Record:  EncodedRecord{ID: "guarded-first", Data: []byte("first")},
			Expect:  ExpectSubjectSeq(0),
		},
		{
			Subject: "evt.compatibility.guarded.second",
			Record:  EncodedRecord{ID: "guarded-second", Data: []byte("second")},
			Expect:  ExpectSubjectSeq(0),
		},
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("AppendBatch error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "atomic batch OCC guards") {
		t.Fatalf("AppendBatch error = %q, want ambiguous batch guard context", err)
	}
}

func TestEncodedEventLogReportsAmbiguousDualGuardConflict(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	subject := "evt.compatibility.dual-guard"

	streamSeq, err := eventLog.AppendAt(ctx, subject, EncodedRecord{ID: "dual-guard-seed", Data: []byte("seed")}, ExpectSubjectSeq(0))
	if err != nil {
		t.Fatalf("seed guarded subject: %v", err)
	}

	_, err = eventLog.AppendBatch(ctx, []EncodedBatchEntry{
		{
			Subject: subject,
			Record:  EncodedRecord{ID: "dual-guard-first", Data: []byte("first")},
			Expect:  ExpectSubjectSeq(0).AndStreamSeq(streamSeq),
		},
		{
			Subject: "evt.compatibility.dual-guard.second",
			Record:  EncodedRecord{ID: "dual-guard-second", Data: []byte("second")},
		},
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("AppendBatch error = %v, want ErrConflict", err)
	}
	if strings.Contains(err.Error(), "stream at expected seq") {
		t.Fatalf("AppendBatch error = %q, falsely reports the current stream guard", err)
	}
	if !strings.Contains(err.Error(), "OCC guards") {
		t.Fatalf("AppendBatch error = %q, want ambiguous guard context", err)
	}
}

// NATS stores the Nats-Expected-* headers with each message. Each guard kind
// must keep sending exactly its headers.
func TestAppendAtStoresGuardHeaders(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	streamSeq, err := eventLog.LastStreamSeq(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		subject string
		expect  func(streamSeq uint64) Expectation
		want    map[string]string
	}{
		{"subject", "evt.guards.subject.created", func(uint64) Expectation { return ExpectSubjectSeq(0) }, map[string]string{
			"Nats-Expected-Last-Subject-Sequence": "0",
		}},
		{"filter", "evt.guards.filter.created", func(uint64) Expectation { return ExpectFilterSeq("evt.guards.filter.>", 0) }, map[string]string{
			"Nats-Expected-Last-Subject-Sequence":         "0",
			"Nats-Expected-Last-Subject-Sequence-Subject": "evt.guards.filter.>",
		}},
		{"stream", "evt.guards.stream.created", ExpectStreamSeq, nil},
		{"subject and stream", "evt.guards.dual.created", func(seq uint64) Expectation { return ExpectSubjectSeq(0).AndStreamSeq(seq) }, map[string]string{
			"Nats-Expected-Last-Subject-Sequence": "0",
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			seq, err := eventLog.AppendAt(ctx, test.subject, EncodedRecord{ID: "guard-" + test.name, Data: []byte("x")}, test.expect(streamSeq))
			if err != nil {
				t.Fatalf("AppendAt: %v", err)
			}
			want := map[string]string{}
			maps.Copy(want, test.want)
			if strings.Contains(test.name, "stream") {
				want["Nats-Expected-Last-Sequence"] = strconv.FormatUint(streamSeq, 10)
			}
			if got := storedGuardHeaders(t, stream, seq); !maps.Equal(got, want) {
				t.Fatalf("stored guard headers = %v, want %v", got, want)
			}
			streamSeq = seq
		})
	}
}

// A subject mutation boundary on the records' own subject sends the shorter
// own-subject header, for a single record and for the guarded first batch
// record. A wildcard boundary keeps the filter header.
func TestExecuteMutationStoresSubjectBoundaryHeaders(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	const subject = "evt.guards.mutation.created"
	var subjectTail uint64
	for _, records := range [][]string{{"single"}, {"batch-first", "batch-second"}} {
		result, err := eventLog.ExecuteMutation(ctx, AtSubject(subject), func(_ context.Context, attempt MutationAttempt) ([]EncodedMutationEntry, error) {
			entries := make([]EncodedMutationEntry, len(records))
			for i, id := range records {
				entries[i] = EncodedMutationEntry{Subject: subject, Record: EncodedRecord{ID: id, Data: []byte(id)}}
			}
			return entries, nil
		})
		if err != nil {
			t.Fatalf("ExecuteMutation %v: %v", records, err)
		}
		want := map[string]string{"Nats-Expected-Last-Subject-Sequence": strconv.FormatUint(subjectTail, 10)}
		if got := storedGuardHeaders(t, stream, result.Sequences[0]); !maps.Equal(got, want) {
			t.Fatalf("records %v: stored guard headers = %v, want %v", records, got, want)
		}
		if len(records) > 1 {
			if got := storedGuardHeaders(t, stream, result.Sequences[1]); len(got) != 0 {
				t.Fatalf("unguarded batch record stored guard headers %v", got)
			}
		}
		subjectTail = result.Sequences[len(result.Sequences)-1]
	}

	result, err := eventLog.ExecuteMutation(ctx, AtSubject("evt.guards.mutation.>"), func(context.Context, MutationAttempt) ([]EncodedMutationEntry, error) {
		return []EncodedMutationEntry{{Subject: subject, Record: EncodedRecord{ID: "wildcard", Data: []byte("w")}}}, nil
	})
	if err != nil {
		t.Fatalf("ExecuteMutation wildcard: %v", err)
	}
	want := map[string]string{
		"Nats-Expected-Last-Subject-Sequence":         strconv.FormatUint(subjectTail, 10),
		"Nats-Expected-Last-Subject-Sequence-Subject": "evt.guards.mutation.>",
	}
	if got := storedGuardHeaders(t, stream, result.Sequences[0]); !maps.Equal(got, want) {
		t.Fatalf("wildcard boundary stored guard headers = %v, want %v", got, want)
	}
}

// AppendEventually publishes without a guard: concurrent writers on one
// subject never conflict, the stored record has no guard headers, and a retry
// with the same ID returns the stored sequence.
func TestAppendEventuallyPublishesWithoutGuard(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	const subject = "evt.unguarded.single.created"

	const writers = 20
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			_, err := eventLog.AppendEventually(ctx, subject, EncodedRecord{ID: fmt.Sprintf("unguarded-%d", i), Data: []byte("x")})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent AppendEventually: %v", err)
		}
	}

	seq, err := eventLog.AppendEventually(ctx, subject, EncodedRecord{ID: "unguarded-retry", Data: []byte("x")})
	if err != nil {
		t.Fatal(err)
	}
	if got := storedGuardHeaders(t, stream, seq); len(got) != 0 {
		t.Fatalf("unguarded record stored guard headers %v", got)
	}
	retried, err := eventLog.AppendEventually(ctx, subject, EncodedRecord{ID: "unguarded-retry", Data: []byte("x")})
	if err != nil || retried != seq {
		t.Fatalf("retry = %d, %v; want stored sequence %d", retried, err, seq)
	}
}

// An unguarded mutation runs its decision once and commits without guard
// headers. A single record reports a duplicate ID as not committed; several
// records commit as one atomic batch. AppendBatch still rejects an unguarded
// batch.
func TestExecuteMutationUnguarded(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	const subject = "evt.unguarded.mutation.created"
	mutate := func(ids ...string) (MutationResult, error) {
		decisions := 0
		result, err := eventLog.ExecuteMutation(ctx, Unguarded(), func(context.Context, MutationAttempt) ([]EncodedMutationEntry, error) {
			decisions++
			entries := make([]EncodedMutationEntry, len(ids))
			for i, id := range ids {
				entries[i] = EncodedMutationEntry{Subject: subject, Record: EncodedRecord{ID: id, Data: []byte(id)}}
			}
			return entries, nil
		})
		if decisions != 1 {
			t.Fatalf("decision ran %d times, want 1", decisions)
		}
		return result, err
	}

	single, err := mutate("single")
	if err != nil || !single.Committed || len(single.Sequences) != 1 {
		t.Fatalf("single = %+v, %v; want one committed record", single, err)
	}
	duplicate, err := mutate("single")
	if err != nil || duplicate.Committed || duplicate.Sequences[0] != single.Sequences[0] {
		t.Fatalf("duplicate = %+v, %v; want the stored sequence, not committed", duplicate, err)
	}
	batch, err := mutate("batch-first", "batch-second")
	if err != nil || !batch.Committed || len(batch.Sequences) != 2 || batch.Sequences[1] != batch.Sequences[0]+1 {
		t.Fatalf("batch = %+v, %v; want two adjacent committed records", batch, err)
	}
	for _, seq := range append(single.Sequences, batch.Sequences...) {
		if got := storedGuardHeaders(t, stream, seq); len(got) != 0 {
			t.Fatalf("unguarded mutation seq %d stored guard headers %v", seq, got)
		}
	}
	if _, err := eventLog.AppendBatch(ctx, []EncodedBatchEntry{
		{Subject: subject, Record: EncodedRecord{ID: "direct-first"}},
		{Subject: subject, Record: EncodedRecord{ID: "direct-second"}},
	}); !errors.Is(err, ErrMissingOCC) {
		t.Fatalf("AppendBatch without a guard = %v, want ErrMissingOCC", err)
	}
}

func storedGuardHeaders(t *testing.T, stream jetstream.Stream, seq uint64) map[string]string {
	t.Helper()
	stored, err := stream.GetMsg(testContext(t), seq)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for key := range stored.Header {
		if strings.HasPrefix(key, "Nats-Expected-") {
			got[key] = stored.Header.Get(key)
		}
	}
	return got
}

func TestAppendAtRejectsMissingGuard(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	if _, err := eventLog.AppendAt(testContext(t), "evt.guards.none", EncodedRecord{ID: "none"}, Expectation{}); !errors.Is(err, ErrMissingOCC) {
		t.Fatalf("AppendAt without a guard = %v, want ErrMissingOCC", err)
	}
	if _, err := eventLog.AppendAt(testContext(t), "evt.guards.none", EncodedRecord{ID: "none"}, ExpectFilterSeq("", 0)); !errors.Is(err, ErrInvalidOCC) {
		t.Fatalf("AppendAt with an empty filter = %v, want ErrInvalidOCC", err)
	}
}
