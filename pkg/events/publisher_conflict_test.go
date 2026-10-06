package events

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func TestIsSequenceConflictRecognizesWrongLastSequenceVariants(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nats.go key exists sentinel",
			err:  jetstream.ErrKeyExists,
			want: true,
		},
		{
			name: "wrapped nats.go key exists sentinel",
			err:  fmt.Errorf("publish: %w", jetstream.ErrKeyExists),
			want: true,
		},
		{
			name: "nats.go revision mismatch sentinel",
			err:  jetstream.ErrKeyRevisionMismatch,
			want: true,
		},
		{
			name: "detailed wrong last sequence",
			err:  &jetstream.APIError{Code: 400, ErrorCode: jetstream.JSErrCodeStreamWrongLastSequence},
			want: true,
		},
		{
			name: "constant wrong last sequence",
			err:  &jetstream.APIError{Code: 400, ErrorCode: jetstream.JSErrCodeStreamWrongLastSequenceConstant},
			want: true,
		},
		{
			name: "wrapped constant wrong last sequence",
			err: fmt.Errorf(
				"publish: %w",
				&jetstream.APIError{Code: 400, ErrorCode: jetstream.JSErrCodeStreamWrongLastSequenceConstant},
			),
			want: true,
		},
		{
			name: "unrelated API error",
			err: &jetstream.APIError{
				Code:      503,
				ErrorCode: jetstream.JSErrCodeJetStreamNotEnabled,
			},
		},
		{name: "unrelated error", err: errors.New("boom")},
		{name: "nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSequenceConflict(tt.err); got != tt.want {
				t.Fatalf("isSequenceConflict(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDecodeBatchAckTranslatesWrongLastSequenceVariants(t *testing.T) {
	for _, code := range []jetstream.ErrorCode{
		jetstream.JSErrCodeStreamWrongLastSequence,
		jetstream.JSErrCodeStreamWrongLastSequenceConstant,
	} {
		t.Run(fmt.Sprintf("error code %d", code), func(t *testing.T) {
			msg := &nats.Msg{Data: fmt.Appendf(nil,
				`{"error":{"code":400,"err_code":%d,"description":"wrong last sequence"}}`,
				code,
			)}
			_, err := decodeBatchAck(msg, entryConflict(EncodedBatchEntry{
				Subject: "evt.room.R1.message_sent",
				Expect:  ExpectSubjectSeq(42),
			}))
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("decodeBatchAck error = %v, want ErrConflict", err)
			}
		})
	}
}

func TestDecodeBatchAckPreservesUnrelatedServerErrors(t *testing.T) {
	msg := &nats.Msg{Data: fmt.Appendf(nil,
		`{"error":{"code":503,"err_code":%d,"description":"JetStream unavailable"}}`,
		jetstream.JSErrCodeJetStreamNotEnabled,
	)}
	_, err := decodeBatchAck(msg, entryConflict(EncodedBatchEntry{
		Subject: "evt.room.R1.message_sent",
		Expect:  ExpectSubjectSeq(42),
	}))
	if err == nil {
		t.Fatal("decodeBatchAck error = nil, want server error")
	}
	if errors.Is(err, ErrConflict) {
		t.Fatalf("decodeBatchAck error = %v, unexpectedly wraps ErrConflict", err)
	}
}

func TestDecodeBatchAckReportsStreamTailExpectation(t *testing.T) {
	msg := &nats.Msg{Data: []byte(`{"error":{"code":400,"err_code":10071,"description":"wrong last sequence"}}`)}
	_, err := decodeBatchAck(msg, entryConflict(EncodedBatchEntry{
		Subject: "evt.room.R1.reaction_added",
		Expect:  ExpectStreamSeq(42),
	}))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("decodeBatchAck error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "stream at expected seq 42") {
		t.Fatalf("decodeBatchAck error = %q, want stream expectation", err)
	}
}

func TestBatchConflictNamesTheOnlyGuard(t *testing.T) {
	guarded := EncodedBatchEntry{Subject: "evt.room.R1.reaction_added", Expect: ExpectFilterSeq("evt.room.R1.>", 17)}
	unguarded := EncodedBatchEntry{Subject: "evt.room.R1.reaction_removed"}

	err := batchConflict([]EncodedBatchEntry{unguarded, guarded}, 1)
	if !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "filter evt.room.R1.> at expected seq 17") {
		t.Fatalf("one-guard batch conflict = %v, want the filter guard", err)
	}
	err = batchConflict([]EncodedBatchEntry{guarded, guarded}, 2)
	if !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "atomic batch OCC guards") {
		t.Fatalf("two-guard batch conflict = %v, want the batch guard context", err)
	}
}

// The batch headers of each guard are the headers that the single-record
// publish options send. NATS stores them with the message.
func TestBatchGuardHeaders(t *testing.T) {
	for name, test := range map[string]struct {
		expect Expectation
		want   map[string]string
	}{
		"none":    {Expectation{}, map[string]string{}},
		"subject": {ExpectSubjectSeq(7), map[string]string{"Nats-Expected-Last-Subject-Sequence": "7"}},
		"filter": {ExpectFilterSeq("evt.room.R1.>", 7), map[string]string{
			"Nats-Expected-Last-Subject-Sequence":         "7",
			"Nats-Expected-Last-Subject-Sequence-Subject": "evt.room.R1.>",
		}},
		"stream": {ExpectStreamSeq(7), map[string]string{"Nats-Expected-Last-Sequence": "7"}},
	} {
		t.Run(name, func(t *testing.T) {
			msg := buildEncodedBatchMsg(EncodedBatchEntry{
				Subject: "evt.room.R1.created", Record: EncodedRecord{ID: "id"}, Expect: test.expect,
			}, "batch", 1, false)
			got := map[string]string{}
			for key := range msg.Header {
				if strings.HasPrefix(key, "Nats-Expected-") {
					got[key] = msg.Header.Get(key)
				}
			}
			if fmt.Sprint(got) != fmt.Sprint(test.want) {
				t.Fatalf("guard headers = %v, want %v", got, test.want)
			}
		})
	}
}
