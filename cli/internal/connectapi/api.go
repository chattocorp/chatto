package connectapi

import (
	"context"
	"math"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/pb/chatto/admin/v1/adminv1connect"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	"hmans.de/chatto/internal/pb/chatto/auth/v1/authv1connect"
	"hmans.de/chatto/internal/pb/chatto/discovery/v1/discoveryv1connect"
	"hmans.de/chatto/internal/pb/chatto/operator/v1/operatorv1connect"
	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
)

// Prefix is the HTTP mount point for Chatto's ConnectRPC public API.
const Prefix = "/api/connect"

// MaxRequestMessageBytes caps individual inbound protobuf messages. ConnectRPC
// defaults to unlimited reads, so keep this explicit for every public handler.
const MaxRequestMessageBytes = 1 << 20 // 1 MiB

const maxConnectAPIHydrationConcurrency = 16

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
	core           *core.ChattoCore
	config         config.ChattoConfig
	version        string
	searchProvider MessageSearchProviderClient
}

// MessageSearchProviderClient calls the trusted provider contract used behind
// the authorized public MessageSearchService.
type MessageSearchProviderClient interface {
	Query(context.Context, *searchv1.QueryRequest) (*searchv1.QueryResponse, error)
	GetStatus(context.Context) (*searchv1.GetStatusResponse, error)
}

// APIOption supplies an optional transport dependency to the public API.
type APIOption func(*API)

// WithMessageSearchProviderClient connects the public MessageSearchService to
// the trusted NATS provider boundary.
func WithMessageSearchProviderClient(client MessageSearchProviderClient) APIOption {
	return func(api *API) { api.searchProvider = client }
}

func New(core *core.ChattoCore, config config.ChattoConfig, version string, options ...APIOption) *API {
	api := &API{core: core, config: config, version: version}
	for _, option := range options {
		if option != nil {
			option(api)
		}
	}
	return api
}

// HandlerOptions returns the common Connect handler options used for Chatto's
// public API. HTTP middleware that writes Connect errors should use the same
// options so errors are encoded consistently with the generated handlers.
func HandlerOptions() []connect.HandlerOption {
	return HandlerOptionsForWebserver(config.WebserverConfig{})
}

// HandlerOptionsForWebserver returns the common Connect handler options with
// the webserver's response-compression policy applied.
func HandlerOptionsForWebserver(webserver config.WebserverConfig) []connect.HandlerOption {
	return handlerOptionsWithReadMax(MaxRequestMessageBytes, webserver)
}

func handlerOptionsWithReadMax(readMaxBytes int, webserver config.WebserverConfig) []connect.HandlerOption {
	compressionMinBytes := webserver.APICompressionMinBytesOrDefault()
	if !webserver.APICompressionEnabled() {
		// Keep gzip request decoding available while making response compression
		// unreachable for any message representable by Connect's int-sized buffer.
		compressionMinBytes = math.MaxInt
	}
	return []connect.HandlerOption{
		connect.WithReadMaxBytes(readMaxBytes),
		connect.WithCompressMinBytes(compressionMinBytes),
		connect.WithInterceptors(
			internalErrorLoggingInterceptor(),
			dekRequestCacheInterceptor(),
			validate.NewInterceptor(),
		),
	}
}

func (a *API) Handlers() []Handler {
	options := HandlerOptionsForWebserver(a.config.Webserver)
	uploadOptions := options
	assetUploadOptions := options
	if a.core != nil {
		uploadOptions = handlerOptionsWithReadMax(uploadRequestMaxBytes(a.core.AssetsConfig().MaxUploadSize), a.config.Webserver)
		assetUploadOptions = handlerOptionsWithReadMax(assetUploadRequestMaxBytes(), a.config.Webserver)
	}

	accountPath, accountHandler := apiv1connect.NewMyAccountServiceHandler(&accountService{api: a}, uploadOptions...)
	assetPath, assetHandler := apiv1connect.NewAssetServiceHandler(&assetService{api: a}, options...)
	assetUploadPath, assetUploadHandler := apiv1connect.NewAssetUploadServiceHandler(&assetUploadService{api: a}, assetUploadOptions...)
	adminDiagnosticsPath, adminDiagnosticsHandler := adminv1connect.NewAdminDiagnosticsServiceHandler(&adminDiagnosticsService{api: a}, options...)
	adminEventLogPath, adminEventLogHandler := adminv1connect.NewAdminEventLogServiceHandler(&adminEventLogService{api: a}, options...)
	adminMemberPath, adminMemberHandler := adminv1connect.NewAdminUserServiceHandler(&adminUserManagementService{api: a}, options...)
	adminServerPath, adminServerHandler := adminv1connect.NewAdminServerServiceHandler(&serverService{api: a}, uploadOptions...)
	serverDiscoveryPath, serverDiscoveryHandler := discoveryv1connect.NewServerDiscoveryServiceHandler(&serverDiscoveryService{api: a}, options...)
	serverPath, serverHandler := apiv1connect.NewServerServiceHandler(&serverService{api: a}, options...)
	userPath, userHandler := apiv1connect.NewUserServiceHandler(&userService{api: a}, options...)
	viewerPath, viewerHandler := apiv1connect.NewViewerServiceHandler(&viewerService{api: a}, options...)
	externalAuthPath, externalAuthHandler := authv1connect.NewExternalIdentityAuthServiceHandler(&externalIdentityAuthService{api: a}, options...)
	permissionPath, permissionHandler := adminv1connect.NewAdminPermissionServiceHandler(&permissionService{api: a}, options...)
	messagePath, messageHandler := apiv1connect.NewMessageServiceHandler(&messageService{api: a}, options...)
	messageSearchPath, messageSearchHandler := apiv1connect.NewMessageSearchServiceHandler(&messageSearchService{api: a}, options...)
	notificationPath, notificationHandler := apiv1connect.NewNotificationServiceHandler(&notificationService{api: a}, options...)
	prefsPath, prefsHandler := apiv1connect.NewNotificationPreferencesServiceHandler(&notificationPreferencesService{api: a}, options...)
	pushPath, pushHandler := apiv1connect.NewPushNotificationServiceHandler(&pushNotificationService{api: a}, options...)
	adminRolePath, adminRoleHandler := adminv1connect.NewAdminRoleServiceHandler(&roleService{api: a}, options...)
	rolePath, roleHandler := apiv1connect.NewRoleServiceHandler(&publicRoleService{api: a}, options...)
	roomPath, roomHandler := apiv1connect.NewRoomServiceHandler(&roomService{api: a}, options...)
	roomDirectoryPath, roomDirectoryHandler := apiv1connect.NewRoomDirectoryServiceHandler(&roomDirectoryService{api: a}, options...)
	adminRoomLayoutPath, adminRoomLayoutHandler := adminv1connect.NewAdminRoomLayoutServiceHandler(&adminRoomLayoutService{api: a}, options...)
	threadPath, threadHandler := apiv1connect.NewThreadServiceHandler(&threadService{api: a}, options...)
	voicePath, voiceHandler := apiv1connect.NewVoiceCallServiceHandler(&voiceCallService{api: a}, options...)
	handlers := []Handler{
		{ServicePath: accountPath, Handler: accountHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: assetPath, Handler: assetHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: assetUploadPath, Handler: assetUploadHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminDiagnosticsPath, Handler: adminDiagnosticsHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminEventLogPath, Handler: adminEventLogHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminServerPath, Handler: adminServerHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminRoomLayoutPath, Handler: adminRoomLayoutHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminMemberPath, Handler: adminMemberHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: externalAuthPath, Handler: externalAuthHandler, AuthPolicy: AuthPolicyPublic},
		{ServicePath: messagePath, Handler: messageHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: messageSearchPath, Handler: messageSearchHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: notificationPath, Handler: notificationHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: serverDiscoveryPath, Handler: serverDiscoveryHandler, AuthPolicy: AuthPolicyPublic},
		{ServicePath: serverPath, Handler: serverHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: userPath, Handler: userHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: viewerPath, Handler: viewerHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: permissionPath, Handler: permissionHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: prefsPath, Handler: prefsHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: pushPath, Handler: pushHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: adminRolePath, Handler: adminRoleHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: rolePath, Handler: roleHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: roomPath, Handler: roomHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: roomDirectoryPath, Handler: roomDirectoryHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: threadPath, Handler: threadHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: voicePath, Handler: voiceHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
	}
	return append(handlers, reflectionHandlers(options)...)
}

// OperatorHandlers returns the local, root-equivalent operator API surface.
// These handlers must only be mounted on the operator Unix socket.
func (a *API) OperatorHandlers() []Handler {
	options := HandlerOptionsForWebserver(a.config.Webserver)
	userPath, userHandler := operatorv1connect.NewOperatorUserServiceHandler(&operatorUserService{api: a}, options...)
	return []Handler{
		{ServicePath: userPath, Handler: userHandler, AuthPolicy: AuthPolicyPublic},
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

func assetUploadRequestMaxBytes() int {
	const protobufOverhead = 64 * 1024
	return defaultAssetUploadChunkSizeForConnect() + protobufOverhead
}

func defaultAssetUploadChunkSizeForConnect() int {
	return 512 * 1024
}
