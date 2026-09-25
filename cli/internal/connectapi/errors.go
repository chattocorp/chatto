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
	"hmans.de/chatto/pkg/events"
)

var (
	errorLogEmailRE       = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	errorLogTokenRE       = regexp.MustCompile(`cht_[A-Za-z0-9]{2}[A-Za-z0-9_.-]+`)
	errorLogInviteLinkRE  = regexp.MustCompile(`(/invite/)[A-Za-z0-9_-]+`)
	errorLogURLQueryRE    = regexp.MustCompile(`(https?://[^\s?]+)\?[^ \t\n\r]+`)
	errorLogQueryParamRE  = regexp.MustCompile(`(?i)\b(token|code|password|email|login|redirect|subject)=([^ \t\n\r&]+)`)
	errorLogControlCharRE = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
)

// connectErrorCodes maps core and framework errors to Connect status codes.
// The first matching row wins, so keep more specific errors before broader
// ones. Errors that match no row become CodeInternal with a generic message.
var connectErrorCodes = []struct {
	code connect.Code
	errs []error
}{
	{connect.CodeFailedPrecondition, []error{core.ErrSetupUnavailable, core.ErrSetupRequired}},
	{connect.CodeCanceled, []error{context.Canceled}},
	{connect.CodeDeadlineExceeded, []error{context.DeadlineExceeded}},
	{connect.CodeAborted, []error{events.ErrConflict, core.ErrNeighborRevisionChanged}},
	{connect.CodeUnauthenticated, []error{core.ErrNotAuthenticated}},
	{connect.CodePermissionDenied, []error{
		core.ErrPermissionDenied,
		core.ErrNotRoomMember,
		core.ErrNotMessageAuthor,
	}},
	{connect.CodeFailedPrecondition, []error{
		core.ErrHumanAccountRequired,
		core.ErrPrivilegedModeUnavailable,
		core.ErrBotOwnerPermissionCeiling,
		core.ErrNeighborMatchesServerOrigin,
	}},
	{connect.CodeAlreadyExists, []error{
		core.ErrRoomNameExists,
		core.ErrLoginAlreadyTaken,
		core.ErrEmailAlreadyVerified,
		core.ErrExternalIdentityAlreadyClaimed,
		core.ErrRoleAlreadyExists,
		core.ErrNeighborAlreadyExists,
	}},
	{connect.CodeInvalidArgument, []error{
		core.ErrCustomStatusEmojiRequired,
		core.ErrCustomStatusEmojiInvalid,
		core.ErrCustomStatusTextRequired,
		core.ErrCustomStatusEmojiTooLong,
		core.ErrCustomStatusTextTooLong,
		core.ErrCustomStatusExpiryInPast,
		core.ErrCannotRemoveDMRoomMember,
		core.ErrExternalIdentityFlowWrongKind,
		core.ErrExternalIdentityFlowUserBound,
		core.ErrCurrentPasswordRequired,
		core.ErrCurrentPasswordInvalid,
		core.ErrLoginTooShort,
		core.ErrLoginTooLong,
		core.ErrLoginInvalidCharacter,
		core.ErrUsernameBlocked,
		core.ErrDisplayNameTooLong,
		core.ErrDisplayNameInvalidCharacter,
		core.ErrDisplayNameInvalidStart,
		core.ErrPasswordTooShort,
		core.ErrPasswordTooLong,
		core.ErrImplicitRole,
		core.ErrRoomGroupNameEmpty,
		core.ErrSidebarLinkLabelEmpty,
		core.ErrSidebarLinkURLInvalid,
		core.ErrInvalidRoleName,
		core.ErrInvalidPermission,
		core.ErrInvitationInvalid,
		core.ErrInvalidArgument,
	}},
	{connect.CodeNotFound, []error{
		core.ErrNotFound,
		core.ErrExternalIdentityNotFound,
		core.ErrExternalIdentityFlowNotFound,
		core.ErrExternalIdentityFlowExpired,
		core.ErrRoleNotFound,
		core.ErrRoomGroupNotFound,
		core.ErrSidebarLinkNotFound,
		core.ErrSidebarItemNotFound,
		core.ErrMessageNotFound,
		core.ErrMessageAttachmentNotFound,
		core.ErrMessageLinkPreviewNotFound,
		core.ErrNeighborNotFound,
		jetstream.ErrKeyNotFound,
	}},
	{connect.CodeInvalidArgument, []error{core.ErrMessageTooLong}},
	{connect.CodeResourceExhausted, []error{
		core.ErrLimitExceeded,
		core.ErrReactionLimitExceeded,
		core.ErrPushSubscriptionLimitReached,
		core.ErrSlowModeActive,
		core.ErrNeighborLimitReached,
	}},
	{connect.CodeFailedPrecondition, []error{
		core.ErrRoomArchived,
		core.ErrRoomThreadingPolicy,
		core.ErrEditWindowExpired,
		core.ErrFreshAuthRequired,
		core.ErrPasswordAlreadySet,
		core.ErrLoginChangeCooldown,
		core.ErrAdminCannotSetOwnPassword,
		core.ErrCannotLeaveDMConversation,
		core.ErrCannotLeaveUniversalRoom,
		core.ErrCannotRevokeSelfAdmin,
		core.ErrExternalIdentityLastMethod,
		core.ErrCannotDeleteSystemRole,
		core.ErrRoomGroupHasRooms,
		core.ErrRoomGroupOrderMismatch,
		core.ErrRoomMoveSourceChanged,
		core.ErrAssetNotAttachable,
		core.ErrSidebarLinkSourceChanged,
		core.ErrSidebarItemPlacement,
	}},
}

// connectError converts err to the Connect error that clients receive. It
// returns existing Connect errors unchanged, including not-modified errors,
// so repeated calls are safe. errorMappingInterceptor applies it to every
// handler error.
func connectError(err error) error {
	if err == nil {
		return nil
	}
	var existing *connect.Error
	if errors.As(err, &existing) {
		return err
	}
	for _, row := range connectErrorCodes {
		for _, target := range row.errs {
			if errors.Is(err, target) {
				return connect.NewError(row.code, err)
			}
		}
	}
	return connectInternalError(err)
}

// errorCode returns the Connect code that a client receives for err.
func errorCode(err error) connect.Code {
	return connect.CodeOf(connectError(err))
}

// errorMappingInterceptor converts every unary handler error with connectError.
// Chatto has no streaming RPCs; add a streaming conversion before adding one.
// Handlers can therefore return core errors directly. Install it inside
// internalErrorLoggingInterceptor so that the logger sees mapped errors.
func errorMappingInterceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			res, err := next(ctx, req)
			if err != nil {
				return nil, connectError(err)
			}
			return res, nil
		}
	})
}

func invalidArgument(message string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(message))
}

func connectInternalError(err error) error {
	return connect.NewError(connect.CodeInternal, internalClientError{cause: err})
}

type internalClientError struct {
	cause error
}

func (internalClientError) Error() string {
	return "internal server error"
}

func internalErrorLoggingInterceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			res, err := next(ctx, req)
			if err != nil && connect.CodeOf(err) == connect.CodeInternal {
				logInternalConnectError(internalServerCause(err), "procedure", req.Spec().Procedure)
			}
			return res, err
		}
	})
}

func internalServerCause(err error) error {
	var internal internalClientError
	if errors.As(err, &internal) && internal.cause != nil {
		return internal.cause
	}
	return err
}

// LogSafeError returns err as text that is safe to log. It unwraps a hidden
// internal cause and redacts email addresses, tokens, invite links, and query
// values. Use it when code outside the Connect handlers logs API errors.
func LogSafeError(err error) string {
	return safeInternalErrorForLog(internalServerCause(err))
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
	message = errorLogInviteLinkRE.ReplaceAllString(message, "$1[redacted]")
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
