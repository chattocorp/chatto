package connectapi

import (
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/pb/chatto/admin/v1/adminv1connect"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
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

	accountPath, accountHandler := apiv1connect.NewAccountServiceHandler(&accountService{api: a}, uploadOptions...)
	attachmentPath, attachmentHandler := apiv1connect.NewAttachmentServiceHandler(&attachmentService{api: a}, options...)
	adminDiagnosticsPath, adminDiagnosticsHandler := adminv1connect.NewAdminDiagnosticsServiceHandler(&adminDiagnosticsService{api: a}, options...)
	adminEventLogPath, adminEventLogHandler := adminv1connect.NewAdminEventLogServiceHandler(&adminEventLogService{api: a}, options...)
	adminMemberPath, adminMemberHandler := adminv1connect.NewAdminMemberServiceHandler(&adminUserManagementService{api: a}, options...)
	adminServerPath, adminServerHandler := adminv1connect.NewAdminServerServiceHandler(&serverService{api: a}, uploadOptions...)
	serverDiscoveryPath, serverDiscoveryHandler := apiv1connect.NewServerDiscoveryServiceHandler(&serverDiscoveryService{api: a}, options...)
	serverPath, serverHandler := apiv1connect.NewServerServiceHandler(&serverService{api: a}, options...)
	viewerPath, viewerHandler := apiv1connect.NewViewerServiceHandler(&viewerService{api: a}, options...)
	permissionPath, permissionHandler := adminv1connect.NewAdminPermissionServiceHandler(&permissionService{api: a}, options...)
	linkPreviewPath, linkPreviewHandler := apiv1connect.NewLinkPreviewServiceHandler(&linkPreviewService{api: a}, options...)
	messagePath, messageHandler := apiv1connect.NewMessageServiceHandler(&messageService{api: a}, messageUploadOptions...)
	memberDirectoryPath, memberDirectoryHandler := apiv1connect.NewMemberDirectoryServiceHandler(&memberDirectoryService{api: a}, options...)
	notificationPath, notificationHandler := apiv1connect.NewNotificationServiceHandler(&notificationService{api: a}, options...)
	prefsPath, prefsHandler := apiv1connect.NewNotificationPreferencesServiceHandler(&notificationPreferencesService{api: a}, options...)
	pushPath, pushHandler := apiv1connect.NewPushNotificationServiceHandler(&pushNotificationService{api: a}, options...)
	readStatePath, readStateHandler := apiv1connect.NewReadStateServiceHandler(&readStateService{api: a}, options...)
	reactionPath, reactionHandler := apiv1connect.NewReactionServiceHandler(&reactionService{api: a}, options...)
	rolePath, roleHandler := adminv1connect.NewAdminRoleServiceHandler(&roleService{api: a}, options...)
	timelinePath, timelineHandler := apiv1connect.NewRoomTimelineServiceHandler(&roomTimelineService{api: a}, options...)
	roomPath, roomHandler := apiv1connect.NewRoomServiceHandler(&roomService{api: a}, options...)
	roomDirectoryPath, roomDirectoryHandler := apiv1connect.NewRoomDirectoryServiceHandler(&roomDirectoryService{api: a}, options...)
	adminRoomLayoutPath, adminRoomLayoutHandler := adminv1connect.NewAdminRoomLayoutServiceHandler(&adminRoomLayoutService{api: a}, options...)
	threadPath, threadHandler := apiv1connect.NewThreadServiceHandler(&threadService{api: a}, options...)
	userPath, userHandler := apiv1connect.NewUserDirectoryServiceHandler(&userService{api: a}, options...)
	voicePath, voiceHandler := apiv1connect.NewVoiceCallServiceHandler(&voiceCallService{api: a}, options...)
	handlers := []Handler{
		{ServicePath: accountPath, Handler: accountHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: attachmentPath, Handler: attachmentHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminDiagnosticsPath, Handler: adminDiagnosticsHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminEventLogPath, Handler: adminEventLogHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminServerPath, Handler: adminServerHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminRoomLayoutPath, Handler: adminRoomLayoutHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminMemberPath, Handler: adminMemberHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: linkPreviewPath, Handler: linkPreviewHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: messagePath, Handler: messageHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: memberDirectoryPath, Handler: memberDirectoryHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: notificationPath, Handler: notificationHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: serverDiscoveryPath, Handler: serverDiscoveryHandler, AuthPolicy: AuthPolicyPublic},
		{ServicePath: serverPath, Handler: serverHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: viewerPath, Handler: viewerHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: permissionPath, Handler: permissionHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: prefsPath, Handler: prefsHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: pushPath, Handler: pushHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: readStatePath, Handler: readStateHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: reactionPath, Handler: reactionHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: rolePath, Handler: roleHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: timelinePath, Handler: timelineHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: roomPath, Handler: roomHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: roomDirectoryPath, Handler: roomDirectoryHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: userPath, Handler: userHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: threadPath, Handler: threadHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: voicePath, Handler: voiceHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
	}
	return append(handlers, reflectionHandlers(options)...)
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
