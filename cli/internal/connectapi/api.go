package connectapi

import (
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
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
	return []connect.HandlerOption{
		connect.WithReadMaxBytes(MaxRequestMessageBytes),
		connect.WithInterceptors(validate.NewInterceptor()),
	}
}

func (a *API) Handlers() []Handler {
	options := HandlerOptions()

	serverPath, serverHandler := apiv1connect.NewServerServiceHandler(&serverService{api: a}, options...)
	presencePath, presenceHandler := apiv1connect.NewPresenceServiceHandler(&presenceService{api: a}, options...)
	messagePath, messageHandler := apiv1connect.NewMessageServiceHandler(&messageService{api: a}, options...)
	prefsPath, prefsHandler := apiv1connect.NewNotificationPreferencesServiceHandler(&notificationPreferencesService{api: a}, options...)
	readStatePath, readStateHandler := apiv1connect.NewReadStateServiceHandler(&readStateService{api: a}, options...)
	reactionPath, reactionHandler := apiv1connect.NewReactionServiceHandler(&reactionService{api: a}, options...)
	timelinePath, timelineHandler := apiv1connect.NewRoomTimelineServiceHandler(&roomTimelineService{api: a}, options...)
	roomPath, roomHandler := apiv1connect.NewRoomServiceHandler(&roomService{api: a}, options...)
	userStatusPath, userStatusHandler := apiv1connect.NewUserStatusServiceHandler(&userStatusService{api: a}, options...)
	threadPath, threadHandler := apiv1connect.NewThreadServiceHandler(&threadService{api: a}, options...)
	return []Handler{
		{ServicePath: messagePath, Handler: messageHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: serverPath, Handler: serverHandler, AuthPolicy: AuthPolicyPublic},
		{ServicePath: presencePath, Handler: presenceHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: prefsPath, Handler: prefsHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: readStatePath, Handler: readStateHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: reactionPath, Handler: reactionHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: timelinePath, Handler: timelineHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: roomPath, Handler: roomHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: userStatusPath, Handler: userStatusHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
		{ServicePath: threadPath, Handler: threadHandler, AuthPolicy: AuthPolicyAuthenticatedUser},
	}
}
