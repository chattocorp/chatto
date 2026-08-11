package events_test

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	. "hmans.de/chatto/pkg/events"
)

func TestEncodedEventLogPreservesOpaqueRecord(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	subject := "evt.compatibility.record.created"
	data := []byte{0x00, 0xff, 0x10, 0x80, 0x01}

	seq, err := eventLog.AppendAt(ctx, subject, EncodedRecord{ID: "opaque-1", Data: data}, 0)
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

	records, lastSeq, err := eventLog.SubjectRecordsAfter(ctx, subject, 0)
	if err != nil {
		t.Fatalf("SubjectRecordsAfter: %v", err)
	}
	if len(records) != 1 || lastSeq != seq {
		t.Fatalf("records=%d lastSeq=%d, want 1 and %d", len(records), lastSeq, seq)
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

func TestSubjectRecordsAfterPageBoundsRecordsAndBytes(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	subject := "evt.compatibility.page.created"
	for i := 0; i < 5; i++ {
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

	if _, err := eventLog.SubjectRecordsAfterPage(ctx, subject, 0, 2, 5); !errors.Is(err, ErrInvalidSubjectReadLimit) {
		t.Fatalf("byte-bounded page error = %v, want ErrInvalidSubjectReadLimit", err)
	}
	bytePage, err := eventLog.SubjectRecordsAfterPage(ctx, subject, 0, 1, 4)
	if err != nil {
		t.Fatalf("single-record byte page: %v", err)
	}
	if len(bytePage.Records) != 1 || !bytePage.More {
		t.Fatalf("byte page = %+v, want one record and more", bytePage)
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

func TestEncodedEventLogAppendAtFilterUsesWildcardTail(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	filter := "evt.compatibility.*"

	firstSeq, err := eventLog.AppendAtFilter(
		ctx,
		"evt.compatibility.first",
		EncodedRecord{ID: "first", Data: []byte("first")},
		filter,
		0,
	)
	if err != nil {
		t.Fatalf("first AppendAtFilter: %v", err)
	}
	if _, err := eventLog.AppendAtFilter(
		ctx,
		"evt.compatibility.second",
		EncodedRecord{ID: "second", Data: []byte("second")},
		filter,
		0,
	); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale wildcard append error = %v, want ErrConflict", err)
	}
	secondSeq, err := eventLog.AppendAtFilter(
		ctx,
		"evt.compatibility.second",
		EncodedRecord{ID: "second", Data: []byte("second")},
		filter,
		firstSeq,
	)
	if err != nil {
		t.Fatalf("current wildcard AppendAtFilter: %v", err)
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
			Subject:     "evt.compatibility.batch.first",
			Record:      EncodedRecord{ID: "batch-first", Data: []byte{0x00, 0x01}},
			HasOCC:      true,
			ExpectedSeq: 0,
		},
		{
			Subject: "evt.compatibility.batch.second",
			Record:  EncodedRecord{ID: "batch-second", Data: []byte{0xfe, 0xff}},
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
}

func TestEncodedEventLogRejectsMissingRecordIDAndUnguardedBatch(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)

	if _, err := eventLog.AppendAt(ctx, "evt.compatibility.invalid", EncodedRecord{Data: []byte("data")}, 0); !errors.Is(err, ErrInvalidEncodedRecord) {
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
			Subject:           "evt.compatibility.second",
			Record:            EncodedRecord{ID: "second", Data: []byte("second")},
			HasStreamOCC:      true,
			ExpectedStreamSeq: 0,
		},
	}); !errors.Is(err, ErrInvalidBatchOCC) {
		t.Fatalf("misplaced stream OCC error = %v, want ErrInvalidBatchOCC", err)
	}
}

func TestEncodedEventLogReportsAmbiguousMultiGuardConflict(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)

	if _, err := eventLog.AppendAt(ctx, "evt.compatibility.guarded.second", EncodedRecord{ID: "seed-second", Data: []byte("seed")}, 0); err != nil {
		t.Fatalf("seed second subject: %v", err)
	}
	_, err := eventLog.AppendBatch(ctx, []EncodedBatchEntry{
		{
			Subject:     "evt.compatibility.guarded.first",
			Record:      EncodedRecord{ID: "guarded-first", Data: []byte("first")},
			HasOCC:      true,
			ExpectedSeq: 0,
		},
		{
			Subject:     "evt.compatibility.guarded.second",
			Record:      EncodedRecord{ID: "guarded-second", Data: []byte("second")},
			HasOCC:      true,
			ExpectedSeq: 0,
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

	streamSeq, err := eventLog.AppendAt(ctx, subject, EncodedRecord{ID: "dual-guard-seed", Data: []byte("seed")}, 0)
	if err != nil {
		t.Fatalf("seed guarded subject: %v", err)
	}

	_, err = eventLog.AppendBatch(ctx, []EncodedBatchEntry{
		{
			Subject:           subject,
			Record:            EncodedRecord{ID: "dual-guard-first", Data: []byte("first")},
			HasOCC:            true,
			ExpectedSeq:       0,
			HasStreamOCC:      true,
			ExpectedStreamSeq: streamSeq,
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
