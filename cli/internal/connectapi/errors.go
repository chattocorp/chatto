package connectapi

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"connectrpc.com/connect"
	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/core"
)

var (
	errorLogEmailRE       = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	errorLogTokenRE       = regexp.MustCompile(`cht_[A-Za-z0-9]{2}[A-Za-z0-9_-]+`)
	errorLogURLQueryRE    = regexp.MustCompile(`(https?://[^\s?]+)\?[^ \t\n\r]+`)
	errorLogQueryParamRE  = regexp.MustCompile(`(?i)\b(token|code|password|email|login|redirect|subject)=([^ \t\n\r&]+)`)
	errorLogControlCharRE = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
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
	if errors.Is(err, core.ErrLoginAlreadyTaken) ||
		errors.Is(err, core.ErrEmailAlreadyVerified) ||
		errors.Is(err, core.ErrExternalIdentityAlreadyClaimed) ||
		errors.Is(err, core.ErrRoleAlreadyExists) {
		return connect.NewError(connect.CodeAlreadyExists, err)
	}
	if errors.Is(err, core.ErrCustomStatusEmojiRequired) ||
		errors.Is(err, core.ErrCustomStatusTextRequired) ||
		errors.Is(err, core.ErrCustomStatusEmojiTooLong) ||
		errors.Is(err, core.ErrCustomStatusTextTooLong) ||
		errors.Is(err, core.ErrCustomStatusExpiryInPast) ||
		errors.Is(err, core.ErrCannotBanDMRoomMember) ||
		errors.Is(err, core.ErrExternalIdentityFlowWrongKind) ||
		errors.Is(err, core.ErrExternalIdentityFlowUserBound) ||
		errors.Is(err, core.ErrCurrentPasswordRequired) ||
		errors.Is(err, core.ErrCurrentPasswordInvalid) ||
		errors.Is(err, core.ErrLoginTooShort) ||
		errors.Is(err, core.ErrLoginTooLong) ||
		errors.Is(err, core.ErrLoginInvalidCharacter) ||
		errors.Is(err, core.ErrUsernameBlocked) ||
		errors.Is(err, core.ErrDisplayNameTooLong) ||
		errors.Is(err, core.ErrDisplayNameInvalidCharacter) ||
		errors.Is(err, core.ErrDisplayNameInvalidStart) ||
		errors.Is(err, core.ErrPasswordTooShort) ||
		errors.Is(err, core.ErrPasswordTooLong) ||
		errors.Is(err, core.ErrImplicitRole) ||
		errors.Is(err, core.ErrRoomGroupNameEmpty) ||
		errors.Is(err, core.ErrSidebarLinkLabelEmpty) ||
		errors.Is(err, core.ErrSidebarLinkURLInvalid) ||
		errors.Is(err, core.ErrInvalidRoleName) ||
		errors.Is(err, core.ErrInvalidArgument) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, core.ErrNotFound) ||
		errors.Is(err, core.ErrExternalIdentityNotFound) ||
		errors.Is(err, core.ErrExternalIdentityFlowNotFound) ||
		errors.Is(err, core.ErrExternalIdentityFlowExpired) ||
		errors.Is(err, core.ErrRoleNotFound) ||
		errors.Is(err, core.ErrRoomGroupNotFound) ||
		errors.Is(err, core.ErrSidebarLinkNotFound) ||
		errors.Is(err, core.ErrMessageNotFound) ||
		errors.Is(err, core.ErrMessageAttachmentNotFound) ||
		errors.Is(err, core.ErrMessageLinkPreviewNotFound) ||
		errors.Is(err, core.ErrRoleNotFound) ||
		errors.Is(err, jetstream.ErrKeyNotFound) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if errors.Is(err, core.ErrMessageTooLong) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, core.ErrLimitExceeded) {
		return connect.NewError(connect.CodeResourceExhausted, err)
	}
	if errors.Is(err, core.ErrRoomArchived) ||
		errors.Is(err, core.ErrEditWindowExpired) ||
		errors.Is(err, core.ErrLimitExceeded) ||
		errors.Is(err, core.ErrFreshAuthRequired) ||
		errors.Is(err, core.ErrPasswordAlreadySet) ||
		errors.Is(err, core.ErrAdminCannotSetOwnPassword) ||
		errors.Is(err, core.ErrCannotLeaveDMConversation) ||
		errors.Is(err, core.ErrCannotLeaveUniversalRoom) ||
		errors.Is(err, core.ErrCannotRevokeSelfAdmin) ||
		errors.Is(err, core.ErrExternalIdentityLastMethod) ||
		errors.Is(err, core.ErrCannotDeleteSystemRole) ||
		errors.Is(err, core.ErrRoomGroupHasRooms) ||
		errors.Is(err, core.ErrRoomGroupOrderMismatch) ||
		errors.Is(err, core.ErrRoomMoveSourceChanged) ||
		errors.Is(err, core.ErrSidebarLinkSourceChanged) {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connectInternalError(err)
}

func invalidArgument(message string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(message))
}

func connectInternalError(err error) error {
	logInternalConnectError(err)
	return connect.NewError(connect.CodeInternal, loggedInternalClientError{})
}

type loggedInternalClientError struct{}

func (loggedInternalClientError) Error() string {
	return "internal server error"
}

func internalErrorLoggingInterceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			res, err := next(ctx, req)
			if err != nil && connect.CodeOf(err) == connect.CodeInternal && !internalErrorAlreadyLogged(err) {
				logInternalConnectError(err, "procedure", req.Spec().Procedure)
			}
			return res, err
		}
	})
}

func internalErrorAlreadyLogged(err error) bool {
	var logged loggedInternalClientError
	return errors.As(err, &logged)
}

func logInternalConnectError(err error, attrs ...any) {
	attrs = append(attrs,
		"error", safeInternalErrorForLog(err),
		"error_type", fmt.Sprintf("%T", err),
		"root_error_type", rootErrorType(err))
	log.Error("Connect API internal error", attrs...)
}

func safeInternalErrorForLog(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	message = errorLogURLQueryRE.ReplaceAllString(message, "$1?[redacted]")
	message = errorLogQueryParamRE.ReplaceAllString(message, "$1=[redacted]")
	message = errorLogEmailRE.ReplaceAllString(message, "[redacted-email]")
	message = errorLogTokenRE.ReplaceAllString(message, "[redacted-token]")
	message = errorLogControlCharRE.ReplaceAllString(message, "?")
	const maxInternalErrorLogLength = 2048
	if len(message) > maxInternalErrorLogLength {
		message = message[:maxInternalErrorLogLength] + "...[truncated]"
	}
	return message
}

func rootErrorType(err error) string {
	for {
		if unwrapper, ok := err.(interface{ Unwrap() error }); ok {
			unwrapped := unwrapper.Unwrap()
			if unwrapped != nil {
				err = unwrapped
				continue
			}
		}
		return fmt.Sprintf("%T", err)
	}
}
