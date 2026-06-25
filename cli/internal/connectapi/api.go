package connectapi

import (
	"net/http"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
)

// Prefix is the HTTP mount point for Chatto's ConnectRPC public API.
const Prefix = "/api/connect"

// MaxRequestMessageBytes caps individual inbound protobuf messages. ConnectRPC
// defaults to unlimited reads, so keep this explicit for every public handler.
const MaxRequestMessageBytes = 1 << 20 // 1 MiB

// Handler is one generated Connect service handler and its generated service
// path. The HTTP server owns the actual route mounting and auth injection.
type Handler struct {
	ServicePath string
	Handler     http.Handler
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

func (a *API) Handlers() []Handler {
	options := []connect.HandlerOption{
		connect.WithReadMaxBytes(MaxRequestMessageBytes),
	}
	privateOptions := append([]connect.HandlerOption{}, options...)
	privateOptions = append(privateOptions, connect.WithInterceptors(requireAuthedUnaryInterceptor()))

	serverPath, serverHandler := apiv1connect.NewServerServiceHandler(&serverService{api: a}, options...)
	messagePath, messageHandler := apiv1connect.NewMessageServiceHandler(&messageService{api: a}, privateOptions...)
	prefsPath, prefsHandler := apiv1connect.NewNotificationPreferencesServiceHandler(&notificationPreferencesService{api: a}, privateOptions...)
	readStatePath, readStateHandler := apiv1connect.NewReadStateServiceHandler(&readStateService{api: a}, privateOptions...)
	timelinePath, timelineHandler := apiv1connect.NewRoomTimelineServiceHandler(&roomTimelineService{api: a}, privateOptions...)
	userStatusPath, userStatusHandler := apiv1connect.NewUserStatusServiceHandler(&userStatusService{api: a}, privateOptions...)
	threadPath, threadHandler := apiv1connect.NewThreadServiceHandler(&threadService{api: a}, privateOptions...)
	return []Handler{
		{ServicePath: messagePath, Handler: messageHandler},
		{ServicePath: serverPath, Handler: serverHandler},
		{ServicePath: prefsPath, Handler: prefsHandler},
		{ServicePath: readStatePath, Handler: readStateHandler},
		{ServicePath: timelinePath, Handler: timelineHandler},
		{ServicePath: userStatusPath, Handler: userStatusHandler},
		{ServicePath: threadPath, Handler: threadHandler},
	}
}
