package connectapi

import (
	"errors"

	"connectrpc.com/connect"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/core"
)

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
	if errors.Is(err, core.ErrPermissionDenied) ||
		errors.Is(err, core.ErrNotRoomMember) ||
		errors.Is(err, core.ErrNotMessageAuthor) {
		return connect.NewError(connect.CodePermissionDenied, err)
	}
	if errors.Is(err, core.ErrRoomNameExists) {
		return connect.NewError(connect.CodeAlreadyExists, err)
	}
	if errors.Is(err, core.ErrRoleAlreadyExists) {
		return connect.NewError(connect.CodeAlreadyExists, err)
	}
	if errors.Is(err, core.ErrCustomStatusEmojiRequired) ||
		errors.Is(err, core.ErrCustomStatusTextRequired) ||
		errors.Is(err, core.ErrCustomStatusEmojiTooLong) ||
		errors.Is(err, core.ErrCustomStatusTextTooLong) ||
		errors.Is(err, core.ErrCustomStatusExpiryInPast) ||
		errors.Is(err, core.ErrCannotBanDMRoomMember) ||
		errors.Is(err, core.ErrRoomGroupNameEmpty) ||
		errors.Is(err, core.ErrSidebarLinkLabelEmpty) ||
		errors.Is(err, core.ErrSidebarLinkURLInvalid) ||
		errors.Is(err, core.ErrInvalidRoleName) ||
		errors.Is(err, core.ErrInvalidArgument) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, core.ErrNotFound) ||
		errors.Is(err, core.ErrRoleNotFound) ||
		errors.Is(err, core.ErrRoomGroupNotFound) ||
		errors.Is(err, core.ErrSidebarLinkNotFound) ||
		errors.Is(err, core.ErrMessageNotFound) ||
		errors.Is(err, core.ErrMessageAttachmentNotFound) ||
		errors.Is(err, core.ErrMessageLinkPreviewNotFound) ||
		errors.Is(err, jetstream.ErrKeyNotFound) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if errors.Is(err, core.ErrMessageTooLong) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, core.ErrRoomArchived) ||
		errors.Is(err, core.ErrEditWindowExpired) ||
		errors.Is(err, core.ErrCannotLeaveDMConversation) ||
		errors.Is(err, core.ErrCannotLeaveUniversalRoom) ||
		errors.Is(err, core.ErrCannotRevokeSelfAdmin) ||
		errors.Is(err, core.ErrCannotDeleteSystemRole) ||
		errors.Is(err, core.ErrRoomGroupHasRooms) ||
		errors.Is(err, core.ErrRoomGroupOrderMismatch) ||
		errors.Is(err, core.ErrRoomMoveSourceChanged) ||
		errors.Is(err, core.ErrSidebarLinkSourceChanged) {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewError(connect.CodeInternal, errors.New("internal server error"))
}

func invalidArgument(message string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(message))
}
