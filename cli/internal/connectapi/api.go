package connectapi

import (
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/pb/chatto/app/v1/appv1connect"
)

// Prefix is the HTTP mount point for Chatto's ConnectRPC public API.
const Prefix = "/api/connect"

// MaxRequestMessageBytes caps individual inbound protobuf messages. ConnectRPC
// defaults to unlimited reads, so keep this explicit for every public handler.
const MaxRequestMessageBytes = 1 << 20 // 1 MiB

// AuthPolicy describes whether the HTTP edge should require authentication
// before forwarding a request to a generated Connect handler.
type AuthPolicy string

const (
	AuthPolicyPublic            AuthPolicy = "public"
	AuthPolicyAuthenticatedUser AuthPolicy = "authenticated_user"
)

// Handler is one generated Connect service handler, its generated service path,
// and the auth policy the HTTP server must enforce before serving it.
type Handler struct {
	ServicePath string
	Handler     http.Handler
	AuthPolicy  AuthPolicy
}

// API owns Chatto's ConnectRPC service implementations. It deliberately has no
// dependency on the Gin HTTP server so API methods stay transport-package local.
type API struct {
	core    *core.ChattoCore
	config  config.ChattoConfig
	version string
}

func New(core *core.ChattoCore, config config.ChattoConfig, version string) *API {
	return &API{core: core, config: config, version: version}
}

// HandlerOptions returns the common Connect handler options used for Chatto's
// public API. HTTP middleware that writes Connect errors should use the same
// options so errors are encoded consistently with the generated handlers.
func HandlerOptions() []connect.HandlerOption {
	return handlerOptionsWithReadMax(MaxRequestMessageBytes)
}

func handlerOptionsWithReadMax(readMaxBytes int) []connect.HandlerOption {
	return []connect.HandlerOption{
		connect.WithReadMaxBytes(readMaxBytes),
		connect.WithInterceptors(validate.NewInterceptor()),
	}
}

func (a *API) Handlers() []Handler {
	options := HandlerOptions()
	uploadOptions := options
	messageUploadOptions := options
	if a.core != nil {
		uploadOptions = handlerOptionsWithReadMax(uploadRequestMaxBytes(a.core.AssetsConfig().MaxUploadSize))
		messageUploadOptions = handlerOptionsWithReadMax(messageUploadRequestMaxBytes(a.core.AssetsConfig().MaxUploadSize))
	}

	accountPath, accountHandler := appv1connect.NewAccountServiceHandler(&accountService{api: a}, uploadOptions...)
	attachmentPath, attachmentHandler := appv1connect.NewAttachmentServiceHandler(&attachmentService{api: a}, options...)
	adminDiagnosticsPath, adminDiagnosticsHandler := appv1connect.NewAdminDiagnosticsServiceHandler(&adminDiagnosticsService{api: a}, options...)
	adminEventLogPath, adminEventLogHandler := appv1connect.NewAdminEventLogServiceHandler(&adminEventLogService{api: a}, options...)
	adminUserManagementPath, adminUserManagementHandler := appv1connect.NewAdminUserManagementServiceHandler(&adminUserManagementService{api: a}, options...)
	serverPath, serverHandler := appv1connect.NewServerServiceHandler(&serverService{api: a}, options...)
	serverStatePath, serverStateHandler := appv1connect.NewServerStateServiceHandler(&serverStateService{api: a}, uploadOptions...)
	viewerPath, viewerHandler := appv1connect.NewViewerServiceHandler(&viewerService{api: a}, options...)
	presencePath, presenceHandler := appv1connect.NewPresenceServiceHandler(&presenceService{api: a}, options...)
	permissionPath, permissionHandler := appv1connect.NewPermissionServiceHandler(&permissionService{api: a}, options...)
	linkPreviewPath, linkPreviewHandler := appv1connect.NewLinkPreviewServiceHandler(&linkPreviewService{api: a}, options...)
	messagePath, messageHandler := appv1connect.NewMessageServiceHandler(&messageService{api: a}, messageUploadOptions...)
	memberDirectoryPath, memberDirectoryHandler := appv1connect.NewMemberDirectoryServiceHandler(&memberDirectoryService{api: a}, options...)
	notificationPath, notificationHandler := appv1connect.NewNotificationServiceHandler(&notificationService{api: a}, options...)
	prefsPath, prefsHandler := appv1connect.NewNotificationPreferencesServiceHandler(&notificationPreferencesService{api: a}, options...)
	pushPath, pushHandler := appv1connect.NewPushNotificationServiceHandler(&pushNotificationService{api: a}, options...)
	readStatePath, readStateHandler := appv1connect.NewReadStateServiceHandler(&readStateService{api: a}, options...)
	reactionPath, reactionHandler := appv1connect.NewReactionServiceHandler(&reactionService{api: a}, options...)
	rolePath, roleHandler := appv1connect.NewRoleServiceHandler(&roleService{api: a}, options...)
	timelinePath, timelineHandler := appv1connect.NewRoomTimelineServiceHandler(&roomTimelineService{api: a}, options...)
	roomPath, roomHandler := appv1connect.NewRoomServiceHandler(&roomService{api: a}, options...)
	roomDirectoryPath, roomDirectoryHandler := appv1connect.NewRoomDirectoryServiceHandler(&roomDirectoryService{api: a}, options...)
	adminRoomLayoutPath, adminRoomLayoutHandler := appv1connect.NewAdminRoomLayoutServiceHandler(&adminRoomLayoutService{api: a}, options...)
	userStatusPath, userStatusHandler := appv1connect.NewUserStatusServiceHandler(&userStatusService{api: a}, options...)
	threadPath, threadHandler := appv1connect.NewThreadServiceHandler(&threadService{api: a}, options...)
	userPath, userHandler := appv1connect.NewUserServiceHandler(&userService{api: a}, options...)
	voicePath, voiceHandler := appv1connect.NewVoiceCallServiceHandler(&voiceCallService{api: a}, options...)
	return []Handler{
		{ServicePath: accountPath, Handler: accountHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: attachmentPath, Handler: attachmentHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminDiagnosticsPath, Handler: adminDiagnosticsHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminEventLogPath, Handler: adminEventLogHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminRoomLayoutPath, Handler: adminRoomLayoutHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminUserManagementPath, Handler: adminUserManagementHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: linkPreviewPath, Handler: linkPreviewHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: messagePath, Handler: messageHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: memberDirectoryPath, Handler: memberDirectoryHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: notificationPath, Handler: notificationHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: serverPath, Handler: serverHandler, AuthPolicy: AuthPolicyPublic},
		{ServicePath: serverStatePath, Handler: serverStateHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: viewerPath, Handler: viewerHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: presencePath, Handler: presenceHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: permissionPath, Handler: permissionHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: prefsPath, Handler: prefsHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: pushPath, Handler: pushHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: readStatePath, Handler: readStateHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: reactionPath, Handler: reactionHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: rolePath, Handler: roleHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: timelinePath, Handler: timelineHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: roomPath, Handler: roomHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: roomDirectoryPath, Handler: roomDirectoryHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: userStatusPath, Handler: userStatusHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: userPath, Handler: userHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: threadPath, Handler: threadHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: voicePath, Handler: voiceHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
	}
}

func uploadRequestMaxBytes(maxUploadSize int64) int {
	const protobufOverhead = 64 * 1024
	maxInt := int(^uint(0) >> 1)
	if maxUploadSize <= 0 {
		return MaxRequestMessageBytes
	}
	if maxUploadSize > int64(maxInt-protobufOverhead) {
		return maxInt
	}
	return int(maxUploadSize) + protobufOverhead
}

func messageUploadRequestMaxBytes(maxUploadSize int64) int {
	const (
		protobufOverhead      = 256 * 1024
		maxAttachmentBatchLen = 10
	)
	maxInt := int(^uint(0) >> 1)
	if maxUploadSize <= 0 {
		return MaxRequestMessageBytes
	}
	maxPayload := int64(maxInt - protobufOverhead)
	if maxUploadSize > maxPayload/maxAttachmentBatchLen {
		return maxInt
	}
	return int(maxUploadSize)*maxAttachmentBatchLen + protobufOverhead
}
