// Package jetstreamutil contains shared helpers for interpreting JetStream
// behavior that is not yet consistently exposed by nats.go.
package jetstreamutil

import (
	"errors"

	"github.com/nats-io/nats.go/jetstream"
)

// constantWrongLastSequenceErrorCode is returned by newer NATS Server versions
// for a wrong expected last sequence without sequence details. nats.go v1.52.0
// only exposes and maps the older detailed form (10071) to ErrKeyExists, so keep
// the constant-form code centralized here until the client library classifies it.
const constantWrongLastSequenceErrorCode jetstream.ErrorCode = 10164

// IsSequenceConflict reports whether err is a JetStream optimistic-concurrency
// conflict caused by an expected-last-sequence mismatch.
func IsSequenceConflict(err error) bool {
	if errors.Is(err, jetstream.ErrKeyExists) {
		return true
	}

	var apiErr *jetstream.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence ||
		apiErr.ErrorCode == constantWrongLastSequenceErrorCode
}
