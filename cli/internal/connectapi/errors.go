package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/auth"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func requireAuth(ctx context.Context) (*corev1.User, error) {
	user := auth.ForContext(ctx)
	if user == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return user, nil
}

func connectError(err error) error {
	if err == nil {
		return nil
	}
	if connect.CodeOf(err) != connect.CodeUnknown {
		return err
	}
	if errors.Is(err, core.ErrNotAuthenticated) {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	if errors.Is(err, core.ErrPermissionDenied) || errors.Is(err, core.ErrNotRoomMember) {
		return connect.NewError(connect.CodePermissionDenied, err)
	}
	if errors.Is(err, core.ErrCustomStatusEmojiRequired) ||
		errors.Is(err, core.ErrCustomStatusTextRequired) ||
		errors.Is(err, core.ErrCustomStatusEmojiTooLong) ||
		errors.Is(err, core.ErrCustomStatusTextTooLong) ||
		errors.Is(err, core.ErrCustomStatusExpiryInPast) ||
		errors.Is(err, core.ErrInvalidArgument) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, core.ErrNotFound) ||
		errors.Is(err, core.ErrMessageNotFound) ||
		errors.Is(err, jetstream.ErrKeyNotFound) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if errors.Is(err, core.ErrMessageTooLong) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, core.ErrRoomArchived) {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewError(connect.CodeInternal, errors.New("internal server error"))
}

func invalidArgument(message string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(message))
}
