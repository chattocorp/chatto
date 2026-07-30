package events

import (
	"errors"

	"github.com/nats-io/nats.go/jetstream"
)

// constantWrongLastSequenceErrorCode is returned by newer NATS Server
// versions for a wrong expected last sequence without sequence details.
// nats.go v1.52.0 only exposes and maps the older detailed form (10071) to
// ErrKeyExists, so retain the constant-form classification here until the
// client library provides one.
const constantWrongLastSequenceErrorCode jetstream.ErrorCode = 10164

func isSequenceConflict(err error) bool {
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
