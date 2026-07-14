package events

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func TestSequenceConflictErrorTranslatesWrongLastSequenceVariants(t *testing.T) {
	tests := []struct {
		name string
		code jetstream.ErrorCode
		want bool
	}{
		{name: "detailed form", code: jetstream.ErrorCode(10071), want: true},
		{name: "constant form", code: jetstream.ErrorCode(10164), want: true},
		{name: "unrelated", code: jetstream.JSErrCodeJetStreamNotEnabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sequenceConflictError(
				&jetstream.APIError{Code: 400, ErrorCode: tt.code},
				"evt.room.R1.message_sent",
				42,
			)
			if got := errors.Is(err, ErrConflict); got != tt.want {
				t.Fatalf("errors.Is(sequenceConflictError, ErrConflict) = %v, want %v (err=%v)", got, tt.want, err)
			}
		})
	}
}

func TestDecodeBatchAckTranslatesConstantWrongLastSequence(t *testing.T) {
	msg := &nats.Msg{Data: []byte(`{"error":{"code":400,"err_code":10164,"description":"wrong last sequence"}}`)}
	_, err := decodeBatchAck(msg, BatchEntry{Subject: "evt.room.R1.message_sent", ExpectedSeq: 42})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("decodeBatchAck error = %v, want ErrConflict", err)
	}
}
