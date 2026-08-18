// Package jetstreamutil contains shared helpers for interpreting JetStream
// behavior consistently across Chatto's storage call sites.
package jetstreamutil

import (
	"errors"

	"github.com/nats-io/nats.go/jetstream"
)

// IsSequenceConflict reports whether err is a JetStream optimistic-concurrency
// conflict caused by an expected-last-sequence mismatch.
func IsSequenceConflict(err error) bool {
	if errors.Is(err, jetstream.ErrKeyExists) ||
		errors.Is(err, jetstream.ErrKeyRevisionMismatch) {
		return true
	}

	var apiErr *jetstream.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence ||
		apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequenceConstant
}
