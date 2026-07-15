package jetstreamutil

import (
	"errors"
	"fmt"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

func TestIsSequenceConflict(t *testing.T) {
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
			err:  fmt.Errorf("publish presence: %w", &jetstream.APIError{Code: 400, ErrorCode: jetstream.ErrorCode(10164)}),
			want: true,
		},
		{
			name: "unrelated API error",
			err:  &jetstream.APIError{Code: 503, ErrorCode: jetstream.JSErrCodeJetStreamNotEnabled},
		},
		{
			name: "unrelated error",
			err:  errors.New("boom"),
		},
		{
			name: "nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSequenceConflict(tt.err); got != tt.want {
				t.Fatalf("IsSequenceConflict(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
