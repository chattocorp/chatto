package events

import (
	"errors"

	"github.com/nats-io/nats.go/jetstream"
)

func isSequenceConflict(err error) bool {
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
