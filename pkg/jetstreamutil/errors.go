// Package jetstreamutil provides application-neutral helpers for NATS
// JetStream: key-value reads through the stream leader, retried resource
// provisioning, and consistent classification of OCC conflicts.
package jetstreamutil

import (
	"errors"

	"github.com/nats-io/nats.go/jetstream"
)

// IsSequenceConflict reports whether err is a JetStream optimistic-concurrency
// conflict caused by an expected-last-sequence mismatch. pkg/events keeps a
// private copy so that it does not depend on this module; change both
// together.
func IsSequenceConflict(err error) bool {
	return errors.Is(err, jetstream.ErrKeyExists) ||
		errors.Is(err, jetstream.ErrKeyRevisionMismatch) ||
		isWrongLastSequence(err)
}

// isWrongLastSequence reports an expected-last-sequence conflict. JetStream
// uses a different error code for streams with more than one replica.
func isWrongLastSequence(err error) bool {
	var apiErr *jetstream.APIError
	return errors.As(err, &apiErr) &&
		(apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence ||
			apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequenceConstant)
}
