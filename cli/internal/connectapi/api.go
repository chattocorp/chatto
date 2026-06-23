package connectapi

import (
	"net/http"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
)

// Prefix is the HTTP mount point for Chatto's ConnectRPC public API.
const Prefix = "/api/connect"

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
	serverPath, serverHandler := apiv1connect.NewServerServiceHandler(&serverService{api: a})
	prefsPath, prefsHandler := apiv1connect.NewNotificationPreferencesServiceHandler(&notificationPreferencesService{api: a})
	timelinePath, timelineHandler := apiv1connect.NewRoomTimelineServiceHandler(&roomTimelineService{api: a})
	return []Handler{
		{ServicePath: serverPath, Handler: serverHandler},
		{ServicePath: prefsPath, Handler: prefsHandler},
		{ServicePath: timelinePath, Handler: timelineHandler},
	}
}
