package events

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func TestSequenceConflictErrorTranslatesWrongLastSequenceVariants(t *testing.T) {
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
			name: "detailed wrong last sequence",
			err:  &jetstream.APIError{Code: 400, ErrorCode: jetstream.ErrorCode(10071)},
			want: true,
		},
		{
			name: "constant wrong last sequence",
			err:  &jetstream.APIError{Code: 400, ErrorCode: jetstream.ErrorCode(10164)},
			want: true,
		},
		{
			name: "wrapped constant wrong last sequence",
			err: fmt.Errorf(
				"publish: %w",
				&jetstream.APIError{Code: 400, ErrorCode: jetstream.ErrorCode(10164)},
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
			err := sequenceConflictError(
				tt.err,
				"evt.room.R1.message_sent",
				42,
			)
			if got := errors.Is(err, ErrConflict); got != tt.want {
				t.Fatalf("errors.Is(sequenceConflictError, ErrConflict) = %v, want %v (err=%v)", got, tt.want, err)
			}
		})
	}
}

func TestDecodeBatchAckTranslatesWrongLastSequenceVariants(t *testing.T) {
	for _, code := range []jetstream.ErrorCode{
		jetstream.ErrorCode(10071),
		jetstream.ErrorCode(10164),
	} {
		t.Run(fmt.Sprintf("error code %d", code), func(t *testing.T) {
			msg := &nats.Msg{Data: []byte(fmt.Sprintf(
				`{"error":{"code":400,"err_code":%d,"description":"wrong last sequence"}}`,
				code,
			))}
			_, err := decodeBatchAck(msg, EncodedBatchEntry{
				Subject:     "evt.room.R1.message_sent",
				ExpectedSeq: 42,
			})
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("decodeBatchAck error = %v, want ErrConflict", err)
			}
		})
	}
}

func TestDecodeBatchAckPreservesUnrelatedServerErrors(t *testing.T) {
	msg := &nats.Msg{Data: []byte(fmt.Sprintf(
		`{"error":{"code":503,"err_code":%d,"description":"JetStream unavailable"}}`,
		jetstream.JSErrCodeJetStreamNotEnabled,
	))}
	_, err := decodeBatchAck(msg, EncodedBatchEntry{
		Subject:     "evt.room.R1.message_sent",
		ExpectedSeq: 42,
	})
	if err == nil {
		t.Fatal("decodeBatchAck error = nil, want server error")
	}
	if errors.Is(err, ErrConflict) {
		t.Fatalf("decodeBatchAck error = %v, unexpectedly wraps ErrConflict", err)
	}
}

func TestDecodeBatchAckReportsStreamTailExpectation(t *testing.T) {
	msg := &nats.Msg{Data: []byte(`{"error":{"code":400,"err_code":10071,"description":"wrong last sequence"}}`)}
	_, err := decodeBatchAck(msg, EncodedBatchEntry{
		Subject:           "evt.room.R1.reaction_added",
		ExpectedStreamSeq: 42,
		HasStreamOCC:      true,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("decodeBatchAck error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "stream at expected seq 42") {
		t.Fatalf("decodeBatchAck error = %q, want stream expectation", err)
	}
}

func TestDecodeBatchAckReportsAmbiguousDualGuardExpectation(t *testing.T) {
	msg := &nats.Msg{Data: []byte(`{"error":{"code":400,"err_code":10071,"description":"wrong last sequence"}}`)}
	entry := EncodedBatchEntry{
		Subject:           "evt.room.R1.reaction_added",
		ExpectedSeq:       17,
		FilterSubject:     "evt.room.R1.>",
		HasOCC:            true,
		ExpectedStreamSeq: 42,
		HasStreamOCC:      true,
	}

	_, err := decodeBatchAck(msg, entry)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("decodeBatchAck error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "batch entry OCC guards") {
		t.Fatalf("decodeBatchAck error = %q, want ambiguous entry guard context", err)
	}

	_, err = decodeBatchAckWithExpectation(msg, batchConflictExpectation([]EncodedBatchEntry{entry}))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("batch conflict error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "atomic batch OCC guards") {
		t.Fatalf("batch conflict error = %q, want ambiguous batch guard context", err)
	}
}
