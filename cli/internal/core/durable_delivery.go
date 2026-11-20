package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

func decodeDurableCoreDelivery(delivery events.DurableDelivery) (*evtv1.Event, error) {
	var event evtv1.Event
	if err := proto.Unmarshal(delivery.Data, &event); err != nil {
		return nil, events.TerminateDelivery("invalid Chatto event envelope", err)
	}
	if event.GetEvent() == nil {
		return nil, events.TerminateDelivery("empty Chatto event envelope", fmt.Errorf("missing event payload"))
	}
	return &event, nil
}

// secureDeleteStreamMessage accepts an unsuccessful deletion only after an
// exact read confirms the message is already absent.
func secureDeleteStreamMessage(ctx context.Context, stream jetstream.Stream, sequence uint64) error {
	deleteErr := stream.SecureDeleteMsg(ctx, sequence)
	if deleteErr == nil {
		return nil
	}
	if _, lookupErr := stream.GetMsg(ctx, sequence); streamMessageAlreadyAbsent(lookupErr) {
		return nil
	} else if lookupErr != nil {
		return errors.Join(deleteErr, fmt.Errorf("confirm stream message absence: %w", lookupErr))
	}
	return deleteErr
}

// NATS does not currently expose these server API error codes as Go constants.
const (
	jetStreamMessageNotFoundErrorCode  = 10057
	jetStreamSequenceNotFoundErrorCode = 10043
)

func streamMessageAlreadyAbsent(err error) bool {
	if errors.Is(err, jetstream.ErrMsgNotFound) {
		return true
	}
	var jsErr jetstream.JetStreamError
	if !errors.As(err, &jsErr) || jsErr.APIError() == nil {
		return false
	}
	switch jsErr.APIError().ErrorCode {
	case jetStreamMessageNotFoundErrorCode, jetStreamSequenceNotFoundErrorCode:
		return true
	default:
		return false
	}
}
