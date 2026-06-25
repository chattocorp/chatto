package connectapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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
	serverPath, serverHandler := apiv1connect.NewServerServiceHandler(&serverService{api: a}, options...)
	messagePath, messageHandler := apiv1connect.NewMessageServiceHandler(&messageService{api: a}, options...)
	prefsPath, prefsHandler := apiv1connect.NewNotificationPreferencesServiceHandler(&notificationPreferencesService{api: a}, options...)
	readStatePath, readStateHandler := apiv1connect.NewReadStateServiceHandler(&readStateService{api: a}, options...)
	timelinePath, timelineHandler := apiv1connect.NewRoomTimelineServiceHandler(&roomTimelineService{api: a}, options...)
	userStatusPath, userStatusHandler := apiv1connect.NewUserStatusServiceHandler(&userStatusService{api: a}, options...)
	threadPath, threadHandler := apiv1connect.NewThreadServiceHandler(&threadService{api: a}, options...)
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

func (a *API) requireRoomMember(ctx context.Context, roomID string) (*corev1.User, *corev1.Room, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(roomID) == "" {
		return nil, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("room_id is required"))
	}

	room, err := a.core.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, nil, connectError(err)
	}
	kind := core.KindOfRoom(room)
	ok, err := a.core.RoomMembershipExists(ctx, kind, user.Id, room.Id)
	if err != nil {
		return nil, nil, connectError(err)
	}
	if !ok {
		return nil, nil, connectError(core.ErrNotRoomMember)
	}
	return user, room, nil
}

func (a *API) requireThreadRoot(ctx context.Context, room *corev1.Room, threadRootEventID string) (*corev1.Event, error) {
	if strings.TrimSpace(threadRootEventID) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("thread_root_event_id is required"))
	}
	kind := core.KindOfRoom(room)
	event, err := a.core.GetRoomEventByEventID(ctx, kind, room.Id, threadRootEventID)
	if err != nil {
		return nil, connectError(err)
	}
	if event == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("thread root event not found"))
	}
	message := event.GetMessagePosted()
	if message == nil || message.GetInThread() != "" || message.GetEchoOfEventId() != "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("thread_root_event_id must identify a root message"))
	}
	return event, nil
}

func (a *API) roomReadAnchor(ctx context.Context, room *corev1.Room, eventID string) (eventIDOut string, ts time.Time, found bool, err error) {
	if strings.TrimSpace(eventID) == "" {
		return "", time.Time{}, false, nil
	}
	kind := core.KindOfRoom(room)
	event, err := a.core.GetRoomEventByEventID(ctx, kind, room.Id, eventID)
	if err != nil {
		return "", time.Time{}, false, connectError(err)
	}
	if event == nil {
		return "", time.Time{}, false, connect.NewError(connect.CodeNotFound, errors.New("up_to_event_id event not found"))
	}
	message := event.GetMessagePosted()
	if message == nil || message.GetInThread() != "" || message.GetEchoOfEventId() != "" {
		return "", time.Time{}, false, connect.NewError(connect.CodeInvalidArgument, errors.New("up_to_event_id must identify a root message in the room timeline"))
	}
	if createdAt := event.GetCreatedAt(); createdAt != nil {
		return event.Id, createdAt.AsTime(), true, nil
	}
	return event.Id, time.Time{}, true, nil
}

func (a *API) threadReadAnchor(ctx context.Context, room *corev1.Room, threadRootEventID string, eventID string) (string, error) {
	if strings.TrimSpace(eventID) == "" {
		return "", nil
	}
	kind := core.KindOfRoom(room)
	event, err := a.core.GetRoomEventByEventID(ctx, kind, room.Id, eventID)
	if err != nil {
		return "", connectError(err)
	}
	if event == nil {
		return "", nil
	}
	message := event.GetMessagePosted()
	if message == nil {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("up_to_event_id must identify a message in the thread"))
	}
	if event.Id == threadRootEventID {
		if message.GetInThread() == "" && message.GetEchoOfEventId() == "" {
			return event.Id, nil
		}
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("up_to_event_id must identify a message in the thread"))
	}
	if message.GetInThread() != threadRootEventID {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("up_to_event_id must identify a message in the thread"))
	}
	return event.Id, nil
}
