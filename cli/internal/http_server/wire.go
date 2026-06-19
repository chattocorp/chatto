package http_server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/events"
	graphauth "hmans.de/chatto/internal/graph/auth"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

const (
	wireProtocolVersion = "chatto-wire-v1"
	wireMaxFrameBytes   = 1 << 20
	wireWriteTimeout    = 10 * time.Second
	wireHelloTimeout    = 10 * time.Second
)

const (
	wireMethodGetViewer               = "chatto.api.v1.ChattoApiService/GetViewer"
	wireMethodGetCurrentUser          = "chatto.api.v1.ChattoApiService/GetCurrentUser"
	wireMethodGetServerSettings       = "chatto.api.v1.ChattoApiService/GetAuthenticatedServerSettings"
	wireMethodGetAccountDeletion      = "chatto.api.v1.ChattoApiService/GetAccountDeletionStatus"
	wireMethodRequestAccountDeletion  = "chatto.api.v1.ChattoApiService/RequestAccountDeletion"
	wireMethodDeleteMyAccount         = "chatto.api.v1.ChattoApiService/DeleteMyAccount"
	wireMethodGetEditableServerConfig = "chatto.api.v1.ChattoApiService/GetServerSettings"
	wireMethodUpdateServerConfig      = "chatto.api.v1.ChattoApiService/UpdateServerSettings"
	wireMethodGetAdminSecurityConfig  = "chatto.api.v1.ChattoApiService/GetAdminSecurityConfig"
	wireMethodUpdateBlockedUsernames  = "chatto.api.v1.ChattoApiService/UpdateBlockedUsernames"
	wireMethodGetAdminSystemInfo      = "chatto.api.v1.ChattoApiService/GetAdminSystemInfo"
	wireMethodListAdminEventLog       = "chatto.api.v1.ChattoApiService/ListAdminEventLog"
	wireMethodGetAdminEventLogEntry   = "chatto.api.v1.ChattoApiService/GetAdminEventLogEntry"
	wireMethodListAdminMembers        = "chatto.api.v1.ChattoApiService/ListAdminMembers"
	wireMethodGetAdminMember          = "chatto.api.v1.ChattoApiService/GetAdminMember"
	wireMethodAdminUpdateUser         = "chatto.api.v1.ChattoApiService/AdminUpdateUser"
	wireMethodAdminClearCooldown      = "chatto.api.v1.ChattoApiService/AdminClearUsernameCooldown"
	wireMethodAssignMemberRole        = "chatto.api.v1.ChattoApiService/AssignMemberRole"
	wireMethodRevokeMemberRole        = "chatto.api.v1.ChattoApiService/RevokeMemberRole"
	wireMethodGetAdminRoleCaps        = "chatto.api.v1.ChattoApiService/GetAdminRoleCapabilities"
	wireMethodGetAdminRole            = "chatto.api.v1.ChattoApiService/GetAdminRole"
	wireMethodCreateAdminRole         = "chatto.api.v1.ChattoApiService/CreateAdminRole"
	wireMethodUpdateAdminRole         = "chatto.api.v1.ChattoApiService/UpdateAdminRole"
	wireMethodDeleteAdminRole         = "chatto.api.v1.ChattoApiService/DeleteAdminRole"
	wireMethodGetRoleTierMatrix       = "chatto.api.v1.ChattoApiService/GetRolePermissionTierMatrix"
	wireMethodGetRoleMatrix           = "chatto.api.v1.ChattoApiService/GetRolePermissionMatrix"
	wireMethodGetUserMatrix           = "chatto.api.v1.ChattoApiService/GetUserPermissionMatrix"
	wireMethodSetRolePermission       = "chatto.api.v1.ChattoApiService/SetRolePermissionState"
	wireMethodSetUserPermission       = "chatto.api.v1.ChattoApiService/SetUserPermissionState"
	wireMethodGetProfileSettings      = "chatto.api.v1.ChattoApiService/GetProfileSettings"
	wireMethodUpdateProfile           = "chatto.api.v1.ChattoApiService/UpdateProfile"
	wireMethodGetUserSettings         = "chatto.api.v1.ChattoApiService/GetUserSettings"
	wireMethodUpdateUserSettings      = "chatto.api.v1.ChattoApiService/UpdateUserSettings"
	wireMethodSetServerNotification   = "chatto.api.v1.ChattoApiService/SetServerNotificationLevel"
	wireMethodSetRoomNotification     = "chatto.api.v1.ChattoApiService/SetRoomNotificationLevel"
	wireMethodSubscribeToPush         = "chatto.api.v1.ChattoApiService/SubscribeToPush"
	wireMethodUnsubscribeFromPush     = "chatto.api.v1.ChattoApiService/UnsubscribeFromPush"
	wireMethodListNotifications       = "chatto.api.v1.ChattoApiService/ListNotifications"
	wireMethodHasNotifications        = "chatto.api.v1.ChattoApiService/HasNotifications"
	wireMethodDismissNotification     = "chatto.api.v1.ChattoApiService/DismissNotification"
	wireMethodDismissAllNotifications = "chatto.api.v1.ChattoApiService/DismissAllNotifications"
	wireMethodListMyRooms             = "chatto.api.v1.ChattoApiService/ListMyRooms"
	wireMethodGetRoom                 = "chatto.api.v1.ChattoApiService/GetRoom"
	wireMethodGetRoomMembers          = "chatto.api.v1.ChattoApiService/GetRoomMembers"
	wireMethodGetRoomDirectory        = "chatto.api.v1.ChattoApiService/GetRoomDirectory"
	wireMethodSearchMembers           = "chatto.api.v1.ChattoApiService/SearchMembers"
	wireMethodStartDM                 = "chatto.api.v1.ChattoApiService/StartDM"
	wireMethodCreateRoom              = "chatto.api.v1.ChattoApiService/CreateRoom"
	wireMethodGetAdminRoomLayout      = "chatto.api.v1.ChattoApiService/GetAdminRoomLayout"
	wireMethodCreateAdminRoomGroup    = "chatto.api.v1.ChattoApiService/CreateAdminRoomGroup"
	wireMethodUpdateAdminRoomGroup    = "chatto.api.v1.ChattoApiService/UpdateAdminRoomGroup"
	wireMethodDeleteAdminRoomGroup    = "chatto.api.v1.ChattoApiService/DeleteAdminRoomGroup"
	wireMethodReorderAdminRoomGroups  = "chatto.api.v1.ChattoApiService/ReorderAdminRoomGroups"
	wireMethodMoveAdminRoomToGroup    = "chatto.api.v1.ChattoApiService/MoveAdminRoomToGroup"
	wireMethodReorderAdminRooms       = "chatto.api.v1.ChattoApiService/ReorderAdminRoomsInGroup"
	wireMethodUpdateAdminRoom         = "chatto.api.v1.ChattoApiService/UpdateAdminRoom"
	wireMethodArchiveAdminRoom        = "chatto.api.v1.ChattoApiService/ArchiveAdminRoom"
	wireMethodUnarchiveAdminRoom      = "chatto.api.v1.ChattoApiService/UnarchiveAdminRoom"
	wireMethodJoinRoom                = "chatto.api.v1.ChattoApiService/JoinRoom"
	wireMethodLeaveRoom               = "chatto.api.v1.ChattoApiService/LeaveRoom"
	wireMethodJoinGroup               = "chatto.api.v1.ChattoApiService/JoinGroup"
	wireMethodBanRoomMember           = "chatto.api.v1.ChattoApiService/BanRoomMember"
	wireMethodListRoomBans            = "chatto.api.v1.ChattoApiService/ListRoomBans"
	wireMethodUnbanRoomMember         = "chatto.api.v1.ChattoApiService/UnbanRoomMember"
	wireMethodGetRoomEvent            = "chatto.api.v1.ChattoApiService/GetRoomEvent"
	wireMethodGetRoomTimeline         = "chatto.api.v1.ChattoApiService/GetRoomTimeline"
	wireMethodGetRoomTimelineAfter    = "chatto.api.v1.ChattoApiService/GetRoomTimelineAfter"
	wireMethodGetRoomTimelineAround   = "chatto.api.v1.ChattoApiService/GetRoomTimelineAround"
	wireMethodGetThreadEvents         = "chatto.api.v1.ChattoApiService/GetThreadEvents"
	wireMethodGetThreadEventsAround   = "chatto.api.v1.ChattoApiService/GetThreadEventsAround"
	wireMethodListMyFollowedThreads   = "chatto.api.v1.ChattoApiService/ListMyFollowedThreads"
	wireMethodGetLinkPreview          = "chatto.api.v1.ChattoApiService/GetLinkPreview"
	wireMethodPostMessage             = "chatto.api.v1.ChattoApiService/PostMessage"
	wireMethodUpdateMessage           = "chatto.api.v1.ChattoApiService/UpdateMessage"
	wireMethodDeleteMessage           = "chatto.api.v1.ChattoApiService/DeleteMessage"
	wireMethodDeleteAttachment        = "chatto.api.v1.ChattoApiService/DeleteAttachment"
	wireMethodDeleteLinkPreview       = "chatto.api.v1.ChattoApiService/DeleteLinkPreview"
	wireMethodAddReaction             = "chatto.api.v1.ChattoApiService/AddReaction"
	wireMethodRemoveReaction          = "chatto.api.v1.ChattoApiService/RemoveReaction"
	wireMethodFollowThread            = "chatto.api.v1.ChattoApiService/FollowThread"
	wireMethodUnfollowThread          = "chatto.api.v1.ChattoApiService/UnfollowThread"
	wireMethodMarkRoomAsRead          = "chatto.api.v1.ChattoApiService/MarkRoomAsRead"
	wireMethodMarkThreadAsRead        = "chatto.api.v1.ChattoApiService/MarkThreadAsRead"
	wireMethodSendTypingIndicator     = "chatto.api.v1.ChattoApiService/SendTypingIndicator"
	wireMethodUpdateMyPresence        = "chatto.api.v1.ChattoApiService/UpdateMyPresence"
	wireMethodListActiveCalls         = "chatto.api.v1.ChattoApiService/ListActiveCalls"
	wireMethodGetCallParticipants     = "chatto.api.v1.ChattoApiService/GetCallParticipants"
	wireMethodJoinVoiceCall           = "chatto.api.v1.ChattoApiService/JoinVoiceCall"
	wireMethodLeaveVoiceCall          = "chatto.api.v1.ChattoApiService/LeaveVoiceCall"
	wireMethodGetVoiceCallToken       = "chatto.api.v1.ChattoApiService/GetVoiceCallToken"
)

var errWireInvalidArgument = errors.New("invalid wire request")

var wireMethods = []string{
	"/" + wireMethodGetViewer,
	"/" + wireMethodGetCurrentUser,
	"/" + wireMethodGetServerSettings,
	"/" + wireMethodGetAccountDeletion,
	"/" + wireMethodRequestAccountDeletion,
	"/" + wireMethodDeleteMyAccount,
	"/" + wireMethodGetEditableServerConfig,
	"/" + wireMethodUpdateServerConfig,
	"/" + wireMethodGetAdminSecurityConfig,
	"/" + wireMethodUpdateBlockedUsernames,
	"/" + wireMethodGetAdminSystemInfo,
	"/" + wireMethodListAdminEventLog,
	"/" + wireMethodGetAdminEventLogEntry,
	"/" + wireMethodListAdminMembers,
	"/" + wireMethodGetAdminMember,
	"/" + wireMethodAdminUpdateUser,
	"/" + wireMethodAdminClearCooldown,
	"/" + wireMethodAssignMemberRole,
	"/" + wireMethodRevokeMemberRole,
	"/" + wireMethodGetAdminRoleCaps,
	"/" + wireMethodGetAdminRole,
	"/" + wireMethodCreateAdminRole,
	"/" + wireMethodUpdateAdminRole,
	"/" + wireMethodDeleteAdminRole,
	"/" + wireMethodGetRoleTierMatrix,
	"/" + wireMethodGetRoleMatrix,
	"/" + wireMethodGetUserMatrix,
	"/" + wireMethodSetRolePermission,
	"/" + wireMethodSetUserPermission,
	"/" + wireMethodGetProfileSettings,
	"/" + wireMethodUpdateProfile,
	"/" + wireMethodGetUserSettings,
	"/" + wireMethodUpdateUserSettings,
	"/" + wireMethodSetServerNotification,
	"/" + wireMethodSetRoomNotification,
	"/" + wireMethodSubscribeToPush,
	"/" + wireMethodUnsubscribeFromPush,
	"/" + wireMethodListNotifications,
	"/" + wireMethodHasNotifications,
	"/" + wireMethodDismissNotification,
	"/" + wireMethodDismissAllNotifications,
	"/" + wireMethodListMyRooms,
	"/" + wireMethodGetRoom,
	"/" + wireMethodGetRoomMembers,
	"/" + wireMethodGetRoomDirectory,
	"/" + wireMethodSearchMembers,
	"/" + wireMethodStartDM,
	"/" + wireMethodCreateRoom,
	"/" + wireMethodGetAdminRoomLayout,
	"/" + wireMethodCreateAdminRoomGroup,
	"/" + wireMethodUpdateAdminRoomGroup,
	"/" + wireMethodDeleteAdminRoomGroup,
	"/" + wireMethodReorderAdminRoomGroups,
	"/" + wireMethodMoveAdminRoomToGroup,
	"/" + wireMethodReorderAdminRooms,
	"/" + wireMethodUpdateAdminRoom,
	"/" + wireMethodArchiveAdminRoom,
	"/" + wireMethodUnarchiveAdminRoom,
	"/" + wireMethodJoinRoom,
	"/" + wireMethodLeaveRoom,
	"/" + wireMethodJoinGroup,
	"/" + wireMethodBanRoomMember,
	"/" + wireMethodListRoomBans,
	"/" + wireMethodUnbanRoomMember,
	"/" + wireMethodGetRoomEvent,
	"/" + wireMethodGetRoomTimeline,
	"/" + wireMethodGetRoomTimelineAfter,
	"/" + wireMethodGetRoomTimelineAround,
	"/" + wireMethodGetThreadEvents,
	"/" + wireMethodGetThreadEventsAround,
	"/" + wireMethodListMyFollowedThreads,
	"/" + wireMethodGetLinkPreview,
	"/" + wireMethodPostMessage,
	"/" + wireMethodUpdateMessage,
	"/" + wireMethodDeleteMessage,
	"/" + wireMethodDeleteAttachment,
	"/" + wireMethodDeleteLinkPreview,
	"/" + wireMethodAddReaction,
	"/" + wireMethodRemoveReaction,
	"/" + wireMethodFollowThread,
	"/" + wireMethodUnfollowThread,
	"/" + wireMethodMarkRoomAsRead,
	"/" + wireMethodMarkThreadAsRead,
	"/" + wireMethodSendTypingIndicator,
	"/" + wireMethodUpdateMyPresence,
	"/" + wireMethodListActiveCalls,
	"/" + wireMethodGetCallParticipants,
	"/" + wireMethodJoinVoiceCall,
	"/" + wireMethodLeaveVoiceCall,
	"/" + wireMethodGetVoiceCallToken,
}

type wireConn struct {
	server *HTTPServer
	conn   *websocket.Conn
	out    chan *wirev1.ServerFrame

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	user     *corev1.User
	hello    bool
	requests map[string]context.CancelFunc
}

func (s *HTTPServer) setupWireAPI(allowedOrigins []string) {
	upgrader := websocket.Upgrader{
		EnableCompression: s.config.Webserver.WebSocketCompressionEnabled(),
		CheckOrigin: func(r *http.Request) bool {
			return s.checkWireWebSocketOrigin(r, allowedOrigins)
		},
	}

	s.router.GET("/api/wire", func(c *gin.Context) {
		s.requestContextWithAuditMetadata(c)
		authenticatedRequest := s.injectUserIntoContext(c)

		conn, err := upgrader.Upgrade(c.Writer, authenticatedRequest, nil)
		if err != nil {
			s.logger.Warn("Wire WebSocket upgrade failed", "error", err)
			return
		}

		ctx, cancel := context.WithCancel(authenticatedRequest.Context())
		wc := &wireConn{
			server:   s,
			conn:     conn,
			out:      make(chan *wirev1.ServerFrame, 256),
			ctx:      ctx,
			cancel:   cancel,
			user:     graphauth.ForContext(authenticatedRequest.Context()),
			requests: make(map[string]context.CancelFunc),
		}
		wc.run()
	})
}

func (s *HTTPServer) checkWireWebSocketOrigin(r *http.Request, allowedOrigins []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if s.matchOrigin(origin, allowedOrigins) != originNotAllowed {
		return true
	}

	host := r.Host
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host = forwarded
	}
	if parsedOrigin, err := url.Parse(origin); err == nil && strings.EqualFold(parsedOrigin.Host, host) {
		return true
	}

	s.logger.Warn("Wire WebSocket connection rejected: origin mismatch",
		"origin", origin, "host", host, "allowed", allowedOrigins)
	return false
}

func (c *wireConn) run() {
	defer c.cancel()
	defer c.conn.Close()

	c.conn.SetReadLimit(wireMaxFrameBytes)
	_ = c.conn.SetReadDeadline(time.Now().Add(wireHelloTimeout))

	var writerDone sync.WaitGroup
	writerDone.Add(1)
	go func() {
		defer writerDone.Done()
		c.writeLoop()
	}()

	c.readLoop()
	c.cancel()
	_ = c.conn.Close()
	c.cancelInflight()
	writerDone.Wait()
}

func (c *wireConn) writeLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case frame, ok := <-c.out:
			if !ok {
				return
			}
			data, err := proto.Marshal(frame)
			if err != nil {
				c.server.logger.Warn("Failed to marshal wire frame", "error", err)
				return
			}
			if err := c.conn.SetWriteDeadline(time.Now().Add(wireWriteTimeout)); err != nil {
				return
			}
			if err := c.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		}
	}
}

func (c *wireConn) readLoop() {
	for {
		messageType, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.BinaryMessage {
			c.sendError("", "", wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "wire frames must be binary protobuf messages", false)
			continue
		}

		var frame wirev1.ClientFrame
		if err := proto.Unmarshal(data, &frame); err != nil {
			c.sendError("", "", wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid protobuf frame", false)
			continue
		}

		switch kind := frame.GetKind().(type) {
		case *wirev1.ClientFrame_Hello:
			c.handleHello(frame.GetFrameId(), kind.Hello)
		case *wirev1.ClientFrame_Request:
			c.handleRequestFrame(frame.GetFrameId(), kind.Request)
		case *wirev1.ClientFrame_Cancel:
			c.handleCancel(kind.Cancel)
		case *wirev1.ClientFrame_Ack:
			// Event acknowledgements are intentionally advisory in the prototype.
		default:
			c.sendError(frame.GetFrameId(), "", wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "frame kind is required", false)
		}
	}
}

func (c *wireConn) handleHello(frameID string, hello *wirev1.ClientHello) {
	if hello == nil {
		c.sendError(frameID, "", wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "hello is required", false)
		return
	}

	c.mu.Lock()
	if c.hello {
		c.mu.Unlock()
		c.sendError(frameID, "", wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "hello already received", false)
		return
	}
	c.mu.Unlock()

	user, err := c.authenticateHello(hello)
	if err != nil {
		c.sendError(frameID, "", wirev1.ErrorCode_ERROR_CODE_UNAUTHENTICATED, "authentication required", false)
		return
	}

	_ = c.conn.SetReadDeadline(time.Time{})

	c.mu.Lock()
	c.user = user
	c.hello = true
	c.mu.Unlock()

	c.send(&wirev1.ServerFrame{
		FrameId: frameID,
		Kind: &wirev1.ServerFrame_Hello{Hello: &wirev1.ServerHello{
			ProtocolVersion: wireProtocolVersion,
			ServerVersion:   c.server.version,
			Methods:         append([]string(nil), wireMethods...),
			Features:        []string{"binary-protobuf", "requests", "my-events"},
		}},
	})

	events, err := c.server.core.StreamMyEvents(c.ctx, user.GetId())
	if err != nil {
		c.sendError(frameID, "", wirev1.ErrorCode_ERROR_CODE_INTERNAL, "failed to subscribe to events", true)
		return
	}
	go c.forwardEvents(events)
}

func (c *wireConn) authenticateHello(hello *wirev1.ClientHello) (*corev1.User, error) {
	c.mu.Lock()
	user := c.user
	c.mu.Unlock()
	if user != nil {
		return user, nil
	}

	token := strings.TrimSpace(hello.GetBearerToken())
	if token == "" {
		return nil, core.ErrNotAuthenticated
	}
	userID, err := c.server.core.ValidateAuthToken(c.ctx, token)
	if err != nil {
		return nil, err
	}
	return c.server.core.GetUser(c.ctx, userID)
}

func (c *wireConn) handleRequestFrame(frameID string, req *wirev1.Request) {
	if req == nil {
		c.sendError(frameID, "", wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "request is required", false)
		return
	}
	user := c.currentUser()
	if user == nil {
		c.sendError(frameID, req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_UNAUTHENTICATED, "authentication required", false)
		return
	}
	if req.GetRequestId() == "" {
		c.sendError(frameID, "", wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "request_id is required", false)
		return
	}

	reqCtx, cancel := context.WithCancel(c.ctx)
	c.mu.Lock()
	if _, exists := c.requests[req.GetRequestId()]; exists {
		c.mu.Unlock()
		cancel()
		c.sendError(frameID, req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "request_id is already in flight", false)
		return
	}
	c.requests[req.GetRequestId()] = cancel
	c.mu.Unlock()

	go func() {
		defer func() {
			c.mu.Lock()
			delete(c.requests, req.GetRequestId())
			c.mu.Unlock()
			cancel()
		}()

		resp, wireErr := c.handleRequest(reqCtx, user, req)
		if wireErr != nil {
			c.send(&wirev1.ServerFrame{
				FrameId: frameID,
				Kind:    &wirev1.ServerFrame_Error{Error: wireErr},
			})
			return
		}
		data, err := proto.Marshal(resp)
		if err != nil {
			c.sendError(frameID, req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INTERNAL, "failed to marshal response", false)
			return
		}
		c.send(&wirev1.ServerFrame{
			FrameId: frameID,
			Kind: &wirev1.ServerFrame_Response{Response: &wirev1.Response{
				RequestId: req.GetRequestId(),
				Body:      data,
			}},
		})
	}()
}

func (c *wireConn) handleRequest(ctx context.Context, user *corev1.User, req *wirev1.Request) (proto.Message, *wirev1.WireError) {
	method := normalizeWireMethod(req.GetMethod())
	switch method {
	case wireMethodGetViewer:
		var body apiv1.GetViewerRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetViewerRequest", false)
		}
		return c.handleWireGetViewer(ctx, user, req.GetRequestId())

	case wireMethodGetCurrentUser:
		var body apiv1.GetCurrentUserRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetCurrentUserRequest", false)
		}
		return c.handleWireGetCurrentUser(ctx, user, req.GetRequestId())

	case wireMethodGetServerSettings:
		var body apiv1.GetAuthenticatedServerSettingsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetAuthenticatedServerSettingsRequest", false)
		}
		return c.handleWireGetAuthenticatedServerSettings(ctx, req.GetRequestId())

	case wireMethodGetAccountDeletion:
		var body apiv1.GetAccountDeletionStatusRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetAccountDeletionStatusRequest", false)
		}
		return c.handleWireGetAccountDeletionStatus(ctx, user.GetId(), req.GetRequestId())

	case wireMethodRequestAccountDeletion:
		var body apiv1.RequestAccountDeletionRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid RequestAccountDeletionRequest", false)
		}
		return c.handleWireRequestAccountDeletion(ctx, user.GetId(), req.GetRequestId())

	case wireMethodDeleteMyAccount:
		var body apiv1.DeleteMyAccountRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid DeleteMyAccountRequest", false)
		}
		return c.handleWireDeleteMyAccount(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetEditableServerConfig:
		var body apiv1.GetServerSettingsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetServerSettingsRequest", false)
		}
		return c.handleWireGetServerSettings(ctx, user.GetId(), req.GetRequestId())

	case wireMethodUpdateServerConfig:
		var body apiv1.UpdateServerSettingsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UpdateServerSettingsRequest", false)
		}
		return c.handleWireUpdateServerSettings(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetAdminSecurityConfig:
		var body apiv1.GetAdminSecurityConfigRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetAdminSecurityConfigRequest", false)
		}
		return c.handleWireGetAdminSecurityConfig(ctx, user.GetId(), req.GetRequestId())

	case wireMethodUpdateBlockedUsernames:
		var body apiv1.UpdateBlockedUsernamesRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UpdateBlockedUsernamesRequest", false)
		}
		return c.handleWireUpdateBlockedUsernames(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetAdminSystemInfo:
		var body apiv1.GetAdminSystemInfoRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetAdminSystemInfoRequest", false)
		}
		return c.handleWireGetAdminSystemInfo(ctx, user.GetId(), req.GetRequestId())

	case wireMethodListAdminEventLog:
		var body apiv1.ListAdminEventLogRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid ListAdminEventLogRequest", false)
		}
		return c.handleWireListAdminEventLog(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetAdminEventLogEntry:
		var body apiv1.GetAdminEventLogEntryRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetAdminEventLogEntryRequest", false)
		}
		return c.handleWireGetAdminEventLogEntry(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodListAdminMembers:
		var body apiv1.ListAdminMembersRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid ListAdminMembersRequest", false)
		}
		return c.handleWireListAdminMembers(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetAdminMember:
		var body apiv1.GetAdminMemberRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetAdminMemberRequest", false)
		}
		return c.handleWireGetAdminMember(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodAdminUpdateUser:
		var body apiv1.AdminUpdateUserRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid AdminUpdateUserRequest", false)
		}
		return c.handleWireAdminUpdateUser(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodAdminClearCooldown:
		var body apiv1.AdminClearUsernameCooldownRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid AdminClearUsernameCooldownRequest", false)
		}
		return c.handleWireAdminClearUsernameCooldown(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodAssignMemberRole:
		var body apiv1.AssignMemberRoleRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid AssignMemberRoleRequest", false)
		}
		return c.handleWireAssignMemberRole(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodRevokeMemberRole:
		var body apiv1.RevokeMemberRoleRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid RevokeMemberRoleRequest", false)
		}
		return c.handleWireRevokeMemberRole(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetAdminRoleCaps:
		var body apiv1.GetAdminRoleCapabilitiesRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetAdminRoleCapabilitiesRequest", false)
		}
		return c.handleWireGetAdminRoleCapabilities(ctx, user.GetId(), req.GetRequestId())

	case wireMethodGetAdminRole:
		var body apiv1.GetAdminRoleRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetAdminRoleRequest", false)
		}
		return c.handleWireGetAdminRole(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodCreateAdminRole:
		var body apiv1.CreateAdminRoleRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid CreateAdminRoleRequest", false)
		}
		return c.handleWireCreateAdminRole(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodUpdateAdminRole:
		var body apiv1.UpdateAdminRoleRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UpdateAdminRoleRequest", false)
		}
		return c.handleWireUpdateAdminRole(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodDeleteAdminRole:
		var body apiv1.DeleteAdminRoleRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid DeleteAdminRoleRequest", false)
		}
		return c.handleWireDeleteAdminRole(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetRoleTierMatrix:
		var body apiv1.GetRolePermissionTierMatrixRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetRolePermissionTierMatrixRequest", false)
		}
		return c.handleWireGetRolePermissionTierMatrix(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetRoleMatrix:
		var body apiv1.GetRolePermissionMatrixRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetRolePermissionMatrixRequest", false)
		}
		return c.handleWireGetRolePermissionMatrix(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetUserMatrix:
		var body apiv1.GetUserPermissionMatrixRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetUserPermissionMatrixRequest", false)
		}
		return c.handleWireGetUserPermissionMatrix(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodSetRolePermission:
		var body apiv1.SetRolePermissionStateRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid SetRolePermissionStateRequest", false)
		}
		return c.handleWireSetRolePermissionState(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodSetUserPermission:
		var body apiv1.SetUserPermissionStateRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid SetUserPermissionStateRequest", false)
		}
		return c.handleWireSetUserPermissionState(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetProfileSettings:
		var body apiv1.GetProfileSettingsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetProfileSettingsRequest", false)
		}
		return c.handleWireGetProfileSettings(ctx, user.GetId(), req.GetRequestId())

	case wireMethodUpdateProfile:
		var body apiv1.UpdateProfileRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UpdateProfileRequest", false)
		}
		return c.handleWireUpdateProfile(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetUserSettings:
		var body apiv1.GetUserSettingsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetUserSettingsRequest", false)
		}
		return c.handleWireGetUserSettings(ctx, user.GetId(), req.GetRequestId())

	case wireMethodUpdateUserSettings:
		var body apiv1.UpdateUserSettingsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UpdateUserSettingsRequest", false)
		}
		return c.handleWireUpdateUserSettings(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodSetServerNotification:
		var body apiv1.SetServerNotificationLevelRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid SetServerNotificationLevelRequest", false)
		}
		return c.handleWireSetServerNotificationLevel(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodSetRoomNotification:
		var body apiv1.SetRoomNotificationLevelRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid SetRoomNotificationLevelRequest", false)
		}
		return c.handleWireSetRoomNotificationLevel(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodSubscribeToPush:
		var body apiv1.SubscribeToPushRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid SubscribeToPushRequest", false)
		}
		return c.handleWireSubscribeToPush(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodUnsubscribeFromPush:
		var body apiv1.UnsubscribeFromPushRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UnsubscribeFromPushRequest", false)
		}
		return c.handleWireUnsubscribeFromPush(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodListNotifications:
		var body apiv1.ListNotificationsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid ListNotificationsRequest", false)
		}
		return c.handleWireListNotifications(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodHasNotifications:
		var body apiv1.HasNotificationsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid HasNotificationsRequest", false)
		}
		return c.handleWireHasNotifications(ctx, user.GetId(), req.GetRequestId())

	case wireMethodDismissNotification:
		var body apiv1.DismissNotificationRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid DismissNotificationRequest", false)
		}
		return c.handleWireDismissNotification(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodDismissAllNotifications:
		var body apiv1.DismissAllNotificationsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid DismissAllNotificationsRequest", false)
		}
		return c.handleWireDismissAllNotifications(ctx, user.GetId(), req.GetRequestId())

	case wireMethodListMyRooms:
		var body apiv1.ListMyRoomsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid ListMyRoomsRequest", false)
		}
		return c.handleWireListMyRooms(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetRoom:
		var body apiv1.GetRoomRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetRoomRequest", false)
		}
		return c.handleWireGetRoom(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetRoomMembers:
		var body apiv1.GetRoomMembersRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetRoomMembersRequest", false)
		}
		return c.handleWireGetRoomMembers(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetRoomDirectory:
		var body apiv1.GetRoomDirectoryRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetRoomDirectoryRequest", false)
		}
		return c.handleWireGetRoomDirectory(ctx, user.GetId(), req.GetRequestId())

	case wireMethodSearchMembers:
		var body apiv1.SearchMembersRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid SearchMembersRequest", false)
		}
		return c.handleWireSearchMembers(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodStartDM:
		var body apiv1.StartDMRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid StartDMRequest", false)
		}
		return c.handleWireStartDM(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodCreateRoom:
		var body apiv1.CreateRoomRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid CreateRoomRequest", false)
		}
		return c.handleWireCreateRoom(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetAdminRoomLayout:
		var body apiv1.GetAdminRoomLayoutRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetAdminRoomLayoutRequest", false)
		}
		return c.handleWireGetAdminRoomLayout(ctx, user.GetId(), req.GetRequestId())

	case wireMethodCreateAdminRoomGroup:
		var body apiv1.CreateAdminRoomGroupRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid CreateAdminRoomGroupRequest", false)
		}
		return c.handleWireCreateAdminRoomGroup(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodUpdateAdminRoomGroup:
		var body apiv1.UpdateAdminRoomGroupRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UpdateAdminRoomGroupRequest", false)
		}
		return c.handleWireUpdateAdminRoomGroup(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodDeleteAdminRoomGroup:
		var body apiv1.DeleteAdminRoomGroupRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid DeleteAdminRoomGroupRequest", false)
		}
		return c.handleWireDeleteAdminRoomGroup(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodReorderAdminRoomGroups:
		var body apiv1.ReorderAdminRoomGroupsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid ReorderAdminRoomGroupsRequest", false)
		}
		return c.handleWireReorderAdminRoomGroups(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodMoveAdminRoomToGroup:
		var body apiv1.MoveAdminRoomToGroupRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid MoveAdminRoomToGroupRequest", false)
		}
		return c.handleWireMoveAdminRoomToGroup(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodReorderAdminRooms:
		var body apiv1.ReorderAdminRoomsInGroupRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid ReorderAdminRoomsInGroupRequest", false)
		}
		return c.handleWireReorderAdminRoomsInGroup(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodUpdateAdminRoom:
		var body apiv1.UpdateAdminRoomRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UpdateAdminRoomRequest", false)
		}
		return c.handleWireUpdateAdminRoom(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodArchiveAdminRoom:
		var body apiv1.ArchiveAdminRoomRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid ArchiveAdminRoomRequest", false)
		}
		return c.handleWireArchiveAdminRoom(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodUnarchiveAdminRoom:
		var body apiv1.UnarchiveAdminRoomRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UnarchiveAdminRoomRequest", false)
		}
		return c.handleWireUnarchiveAdminRoom(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodJoinRoom:
		var body apiv1.JoinRoomRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid JoinRoomRequest", false)
		}
		return c.handleWireJoinRoom(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodLeaveRoom:
		var body apiv1.LeaveRoomRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid LeaveRoomRequest", false)
		}
		return c.handleWireLeaveRoom(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodJoinGroup:
		var body apiv1.JoinGroupRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid JoinGroupRequest", false)
		}
		return c.handleWireJoinGroup(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodBanRoomMember:
		var body apiv1.BanRoomMemberRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid BanRoomMemberRequest", false)
		}
		return c.handleWireBanRoomMember(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodListRoomBans:
		var body apiv1.ListRoomBansRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid ListRoomBansRequest", false)
		}
		return c.handleWireListRoomBans(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodUnbanRoomMember:
		var body apiv1.UnbanRoomMemberRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UnbanRoomMemberRequest", false)
		}
		return c.handleWireUnbanRoomMember(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetRoomEvent:
		var body apiv1.GetRoomEventRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetRoomEventRequest", false)
		}
		_, kind, err := c.authorizedRoom(ctx, user.GetId(), body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		event, err := c.server.core.GetRoomEventByEventID(ctx, kind, body.GetRoomId(), body.GetEventId())
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		if event == nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), core.ErrMessageNotFound)
		}
		seq, err := c.server.core.GetEventSequence(ctx, kind, body.GetRoomId(), event.GetId())
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		view, err := c.roomEventView(ctx, user.GetId(), kind, &core.RoomEvent{Event: event, Sequence: seq})
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		return &apiv1.GetRoomEventResponse{Event: view}, nil

	case wireMethodGetRoomTimeline:
		var body apiv1.GetRoomTimelineRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetRoomTimelineRequest", false)
		}
		room, kind, err := c.authorizedRoom(ctx, user.GetId(), body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		_ = room
		var before *uint64
		if body.GetBeforeSequence() > 0 {
			seq := body.GetBeforeSequence()
			before = &seq
		}
		result, err := c.server.core.GetRoomEvents(ctx, kind, body.GetRoomId(), int(body.GetLimit()), before)
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		resp := timelineResponse(result)
		page, err := c.roomEventsPage(ctx, user.GetId(), kind, result)
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		resp.EventViews = page.Events
		return resp, nil

	case wireMethodGetRoomTimelineAfter:
		var body apiv1.GetRoomTimelineAfterRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetRoomTimelineAfterRequest", false)
		}
		_, kind, err := c.authorizedRoom(ctx, user.GetId(), body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		result, err := c.server.core.GetRoomEventsAfter(ctx, kind, body.GetRoomId(), body.GetAfterSequence(), int(body.GetLimit()))
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		page, err := c.roomEventsPage(ctx, user.GetId(), kind, result)
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		return &apiv1.GetRoomTimelineAfterResponse{Page: page}, nil

	case wireMethodGetRoomTimelineAround:
		var body apiv1.GetRoomTimelineAroundRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetRoomTimelineAroundRequest", false)
		}
		_, kind, err := c.authorizedRoom(ctx, user.GetId(), body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		result, err := c.server.core.GetRoomEventsAround(ctx, kind, body.GetRoomId(), body.GetEventId(), int(body.GetLimit()))
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		page, err := c.roomEventsAroundPage(ctx, user.GetId(), kind, result)
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		return &apiv1.GetRoomTimelineAroundResponse{Page: page}, nil

	case wireMethodGetThreadEvents:
		var body apiv1.GetThreadEventsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetThreadEventsRequest", false)
		}
		_, kind, err := c.authorizedRoom(ctx, user.GetId(), body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		root, err := c.threadRootEventView(ctx, user.GetId(), kind, body.GetRoomId(), body.GetThreadRootEventId())
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		var before, after *uint64
		if body.GetBeforeSequence() > 0 {
			seq := body.GetBeforeSequence()
			before = &seq
		}
		if body.GetAfterSequence() > 0 {
			seq := body.GetAfterSequence()
			after = &seq
		}
		result, err := c.server.core.GetThreadReplyEvents(ctx, kind, body.GetRoomId(), body.GetThreadRootEventId(), int(body.GetLimit()), before, after)
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		page, err := c.roomEventsPage(ctx, user.GetId(), kind, result)
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		return &apiv1.GetThreadEventsResponse{RootEvent: root, Replies: page}, nil

	case wireMethodGetThreadEventsAround:
		var body apiv1.GetThreadEventsAroundRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetThreadEventsAroundRequest", false)
		}
		_, kind, err := c.authorizedRoom(ctx, user.GetId(), body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		root, err := c.threadRootEventView(ctx, user.GetId(), kind, body.GetRoomId(), body.GetThreadRootEventId())
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		result, err := c.server.core.GetThreadReplyEventsAround(ctx, kind, body.GetRoomId(), body.GetThreadRootEventId(), body.GetAnchorEventId(), int(body.GetLimit()))
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		page, err := c.roomEventsPage(ctx, user.GetId(), kind, result)
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		return &apiv1.GetThreadEventsAroundResponse{RootEvent: root, Replies: page}, nil

	case wireMethodListMyFollowedThreads:
		var body apiv1.ListMyFollowedThreadsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid ListMyFollowedThreadsRequest", false)
		}
		return c.handleWireListMyFollowedThreads(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetLinkPreview:
		var body apiv1.GetLinkPreviewRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetLinkPreviewRequest", false)
		}
		return c.handleWireGetLinkPreview(ctx, req.GetRequestId(), &body)

	case wireMethodPostMessage:
		var body apiv1.PostMessageRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid PostMessageRequest", false)
		}
		return c.handleWirePostMessage(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodUpdateMessage:
		var body apiv1.UpdateMessageRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UpdateMessageRequest", false)
		}
		return c.handleWireUpdateMessage(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodDeleteMessage:
		var body apiv1.DeleteMessageRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid DeleteMessageRequest", false)
		}
		return c.handleWireDeleteMessage(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodDeleteAttachment:
		var body apiv1.DeleteAttachmentRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid DeleteAttachmentRequest", false)
		}
		return c.handleWireDeleteAttachment(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodDeleteLinkPreview:
		var body apiv1.DeleteLinkPreviewRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid DeleteLinkPreviewRequest", false)
		}
		return c.handleWireDeleteLinkPreview(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodAddReaction:
		var body apiv1.AddReactionRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid AddReactionRequest", false)
		}
		return c.handleWireAddReaction(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodRemoveReaction:
		var body apiv1.RemoveReactionRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid RemoveReactionRequest", false)
		}
		return c.handleWireRemoveReaction(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodFollowThread:
		var body apiv1.FollowThreadRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid FollowThreadRequest", false)
		}
		return c.handleWireFollowThread(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodUnfollowThread:
		var body apiv1.UnfollowThreadRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UnfollowThreadRequest", false)
		}
		return c.handleWireUnfollowThread(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodMarkRoomAsRead:
		var body apiv1.MarkRoomAsReadRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid MarkRoomAsReadRequest", false)
		}
		return c.handleWireMarkRoomAsRead(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodMarkThreadAsRead:
		var body apiv1.MarkThreadAsReadRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid MarkThreadAsReadRequest", false)
		}
		return c.handleWireMarkThreadAsRead(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodSendTypingIndicator:
		var body apiv1.SendTypingIndicatorRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid SendTypingIndicatorRequest", false)
		}
		_, kind, err := c.authorizedRoom(ctx, user.GetId(), body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		var threadRoot *string
		if body.GetThreadRootEventId() != "" {
			value := body.GetThreadRootEventId()
			threadRoot = &value
		}
		if err := c.server.core.PublishTypingIndicator(ctx, user.GetId(), kind, body.GetRoomId(), threadRoot); err != nil {
			return nil, c.errorFromRequestErr(req.GetRequestId(), err)
		}
		return &apiv1.SendTypingIndicatorResponse{}, nil

	case wireMethodUpdateMyPresence:
		var body apiv1.UpdateMyPresenceRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid UpdateMyPresenceRequest", false)
		}
		return c.handleWireUpdateMyPresence(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodListActiveCalls:
		var body apiv1.ListActiveCallsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid ListActiveCallsRequest", false)
		}
		return c.handleWireListActiveCalls(ctx, user.GetId(), req.GetRequestId())

	case wireMethodGetCallParticipants:
		var body apiv1.GetCallParticipantsRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetCallParticipantsRequest", false)
		}
		return c.handleWireGetCallParticipants(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodJoinVoiceCall:
		var body apiv1.JoinVoiceCallRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid JoinVoiceCallRequest", false)
		}
		return c.handleWireJoinVoiceCall(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodLeaveVoiceCall:
		var body apiv1.LeaveVoiceCallRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid LeaveVoiceCallRequest", false)
		}
		return c.handleWireLeaveVoiceCall(ctx, user.GetId(), req.GetRequestId(), &body)

	case wireMethodGetVoiceCallToken:
		var body apiv1.GetVoiceCallTokenRequest
		if err := proto.Unmarshal(req.GetBody(), &body); err != nil {
			return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid GetVoiceCallTokenRequest", false)
		}
		return c.handleWireGetVoiceCallToken(ctx, user.GetId(), req.GetRequestId(), &body)

	default:
		return nil, wireError(req.GetRequestId(), wirev1.ErrorCode_ERROR_CODE_UNIMPLEMENTED, "unknown method", false)
	}
}

func (c *wireConn) handleCancel(cancel *wirev1.CancelRequest) {
	if cancel == nil || cancel.GetRequestId() == "" {
		return
	}
	c.mu.Lock()
	cancelFunc := c.requests[cancel.GetRequestId()]
	c.mu.Unlock()
	if cancelFunc != nil {
		cancelFunc()
	}
}

func (c *wireConn) forwardEvents(events <-chan core.EventEnvelope) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			streamEvent := c.streamEvent(event)
			if streamEvent == nil {
				continue
			}
			if !c.send(&wirev1.ServerFrame{
				FrameId: "",
				Kind:    &wirev1.ServerFrame_Event{Event: streamEvent},
			}) {
				return
			}
		}
	}
}

func (c *wireConn) streamEvent(event core.EventEnvelope) *wirev1.StreamEvent {
	if event == nil {
		return nil
	}
	streamEvent := &wirev1.StreamEvent{
		EventId:     event.ID(),
		EventType:   wireEventType(event),
		Invalidates: wireInvalidationHints(event),
	}
	switch {
	case event.EVTEvent() != nil:
		streamEvent.Payload = &wirev1.StreamEvent_DurableEvent{DurableEvent: cloneEvent(event.EVTEvent())}
	case event.LiveEvent() != nil:
		streamEvent.Payload = &wirev1.StreamEvent_LiveEvent{LiveEvent: cloneLiveEvent(event.LiveEvent())}
	case event.HeartbeatEvent() != nil:
		streamEvent.Payload = &wirev1.StreamEvent_Heartbeat{Heartbeat: &corev1.HeartbeatEvent{}}
	}
	return streamEvent
}

func (c *wireConn) currentUser() *corev1.User {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hello {
		return nil
	}
	return c.user
}

func (c *wireConn) authorizedRoom(ctx context.Context, userID, roomID string) (*corev1.Room, core.RoomKind, error) {
	if roomID == "" {
		return nil, "", fmt.Errorf("%w: room_id is required", errWireInvalidArgument)
	}
	room, err := c.server.core.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, "", err
	}
	kind := core.KindOfRoom(room)
	member, err := c.server.core.RoomMembershipExists(ctx, kind, userID, roomID)
	if err != nil {
		return nil, "", err
	}
	if !member {
		return nil, "", core.ErrPermissionDenied
	}
	return room, kind, nil
}

func (c *wireConn) sendError(frameID, requestID string, code wirev1.ErrorCode, message string, retryable bool) bool {
	return c.send(&wirev1.ServerFrame{
		FrameId: frameID,
		Kind:    &wirev1.ServerFrame_Error{Error: wireError(requestID, code, message, retryable)},
	})
}

func (c *wireConn) send(frame *wirev1.ServerFrame) bool {
	select {
	case <-c.ctx.Done():
		return false
	case c.out <- frame:
		return true
	}
}

func (c *wireConn) cancelInflight() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, cancel := range c.requests {
		cancel()
	}
	clear(c.requests)
}

func (c *wireConn) errorFromRequestErr(requestID string, err error) *wirev1.WireError {
	var mentionErr *core.MentionConfirmationRequiredError
	var stringLengthErr *core.StringLengthError
	switch {
	case errors.Is(err, context.Canceled):
		return wireError(requestID, wirev1.ErrorCode_ERROR_CODE_CANCELLED, "request cancelled", false)
	case errors.Is(err, core.ErrNotAuthenticated):
		return wireError(requestID, wirev1.ErrorCode_ERROR_CODE_UNAUTHENTICATED, "authentication required", false)
	case errors.Is(err, core.ErrPermissionDenied), errors.Is(err, core.ErrNotRoomMember), errors.Is(err, core.ErrNotMessageAuthor):
		return wireError(requestID, wirev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "permission denied", false)
	case errors.Is(err, core.ErrNotFound), errors.Is(err, core.ErrMessageNotFound), errors.Is(err, core.ErrRoomGroupNotFound), errors.Is(err, jetstream.ErrKeyNotFound):
		return wireError(requestID, wirev1.ErrorCode_ERROR_CODE_NOT_FOUND, "not found", false)
	case errors.Is(err, errWireInvalidArgument), strings.Contains(err.Error(), "invalid timezone"), strings.Contains(err.Error(), "room name"), strings.Contains(err.Error(), "room description"), strings.Contains(err.Error(), "ban reason"), strings.Contains(err.Error(), "ban expiry"), strings.Contains(err.Error(), "unban reason"), errors.Is(err, core.ErrMessageTooLong), errors.Is(err, core.ErrRoomArchived), errors.Is(err, core.ErrCannotLeaveDMConversation), errors.Is(err, core.ErrCannotBanDMRoomMember), errors.Is(err, core.ErrDisplayNameTooLong), errors.Is(err, core.ErrDisplayNameInvalidCharacter), errors.Is(err, core.ErrDisplayNameInvalidStart), errors.Is(err, core.ErrLoginTooShort), errors.Is(err, core.ErrLoginTooLong), errors.Is(err, core.ErrLoginInvalidCharacter), errors.Is(err, core.ErrLoginChangeCooldown), errors.Is(err, core.ErrLoginAlreadyTaken), errors.Is(err, core.ErrUsernameBlocked), errors.Is(err, core.ErrTokenNotFound), errors.Is(err, core.ErrTokenExpired), errors.Is(err, core.ErrRoomNameExists), errors.Is(err, core.ErrRoomGroupHasRooms), errors.Is(err, core.ErrRoomGroupNameEmpty), errors.Is(err, core.ErrRoomGroupOrderMismatch), errors.As(err, &mentionErr), errors.As(err, &stringLengthErr):
		return wireError(requestID, wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, err.Error(), false)
	default:
		c.server.logger.Warn("Wire request failed", "error", err)
		return wireError(requestID, wirev1.ErrorCode_ERROR_CODE_INTERNAL, "internal error", true)
	}
}

func wireError(requestID string, code wirev1.ErrorCode, message string, retryable bool) *wirev1.WireError {
	return &wirev1.WireError{
		RequestId: requestID,
		Code:      code,
		Message:   message,
		Retryable: retryable,
	}
}

func normalizeWireMethod(method string) string {
	return strings.TrimPrefix(strings.TrimSpace(method), "/")
}

func roomKindFromProto(kind corev1.RoomKind) core.RoomKind {
	if kind == corev1.RoomKind_ROOM_KIND_DM {
		return core.KindDM
	}
	return core.KindChannel
}

func timelineResponse(result *core.RoomEventsResult) *apiv1.GetRoomTimelineResponse {
	resp := &apiv1.GetRoomTimelineResponse{}
	if result == nil {
		return resp
	}
	resp.HasOlder = result.HasOlder
	resp.HasNewer = result.HasNewer
	resp.StartSequence = result.StartCursorSeq
	resp.EndSequence = result.EndCursorSeq
	resp.Events = make([]*apiv1.TimelineEvent, 0, len(result.Events))
	for _, event := range result.Events {
		if event == nil || event.Event == nil {
			continue
		}
		resp.Events = append(resp.Events, &apiv1.TimelineEvent{
			Event:    cloneEvent(event.Event),
			Sequence: event.Sequence,
		})
	}
	return resp
}

func cloneUser(user *corev1.User) *corev1.User {
	if user == nil {
		return nil
	}
	return proto.Clone(user).(*corev1.User)
}

func cloneUsers(users []*corev1.User) []*corev1.User {
	cloned := make([]*corev1.User, 0, len(users))
	for _, user := range users {
		if user != nil {
			cloned = append(cloned, cloneUser(user))
		}
	}
	return cloned
}

func cloneRoom(room *corev1.Room) *corev1.Room {
	if room == nil {
		return nil
	}
	return proto.Clone(room).(*corev1.Room)
}

func cloneRooms(rooms []*corev1.Room) []*corev1.Room {
	cloned := make([]*corev1.Room, 0, len(rooms))
	for _, room := range rooms {
		if room != nil {
			cloned = append(cloned, proto.Clone(room).(*corev1.Room))
		}
	}
	return cloned
}

func cloneRoomGroup(group *corev1.RoomGroup) *corev1.RoomGroup {
	if group == nil {
		return nil
	}
	return proto.Clone(group).(*corev1.RoomGroup)
}

func cloneRoomGroups(groups []*corev1.RoomGroup) []*corev1.RoomGroup {
	cloned := make([]*corev1.RoomGroup, 0, len(groups))
	for _, group := range groups {
		if group != nil {
			cloned = append(cloned, cloneRoomGroup(group))
		}
	}
	return cloned
}

func cloneEvent(event *corev1.Event) *corev1.Event {
	if event == nil {
		return nil
	}
	return proto.Clone(event).(*corev1.Event)
}

func cloneLiveEvent(event *corev1.LiveEvent) *corev1.LiveEvent {
	if event == nil {
		return nil
	}
	return proto.Clone(event).(*corev1.LiveEvent)
}

func wireEventType(event core.EventEnvelope) string {
	switch {
	case event == nil:
		return ""
	case event.EVTEvent() != nil:
		return events.EventTypeOf(event.EVTEvent())
	case event.LiveEvent() != nil:
		return protoOneofFieldName(event.LiveEvent().ProtoReflect(), "event")
	case event.HeartbeatEvent() != nil:
		return "heartbeat"
	default:
		return ""
	}
}

func protoOneofFieldName(msg protoreflect.Message, oneofName protoreflect.Name) string {
	oneof := msg.Descriptor().Oneofs().ByName(oneofName)
	if oneof == nil {
		return ""
	}
	field := msg.WhichOneof(oneof)
	if field == nil {
		return ""
	}
	return string(field.Name())
}

func wireInvalidationHints(event core.EventEnvelope) []*wirev1.InvalidationHint {
	if event == nil {
		return nil
	}
	if roomID := wireRoomIDOfEnvelope(event); roomID != "" {
		return []*wirev1.InvalidationHint{
			{Kind: wirev1.InvalidationKind_INVALIDATION_KIND_ROOM, Id: roomID},
			{Kind: wirev1.InvalidationKind_INVALIDATION_KIND_ROOM_TIMELINE, Id: roomID},
		}
	}
	if live := event.LiveEvent(); live != nil {
		switch e := live.GetEvent().(type) {
		case *corev1.LiveEvent_UserProfileUpdated:
			return []*wirev1.InvalidationHint{{Kind: wirev1.InvalidationKind_INVALIDATION_KIND_USER, Id: e.UserProfileUpdated.GetUserId()}}
		case *corev1.LiveEvent_ServerUpdated, *corev1.LiveEvent_RoomGroupsUpdated:
			return []*wirev1.InvalidationHint{{Kind: wirev1.InvalidationKind_INVALIDATION_KIND_SERVER, Id: "server"}}
		case *corev1.LiveEvent_ServerUserPreferencesUpdated,
			*corev1.LiveEvent_NotificationLevelChanged,
			*corev1.LiveEvent_ThreadFollowChanged,
			*corev1.LiveEvent_NotificationCreated,
			*corev1.LiveEvent_NotificationDismissed,
			*corev1.LiveEvent_RoomMarkedAsRead,
			*corev1.LiveEvent_MentionStatusCleared:
			return []*wirev1.InvalidationHint{{Kind: wirev1.InvalidationKind_INVALIDATION_KIND_VIEWER, Id: live.GetActorId()}}
		}
	}
	return nil
}

func wireRoomIDOfEnvelope(event core.EventEnvelope) string {
	if event == nil {
		return ""
	}
	if evt := event.EVTEvent(); evt != nil {
		return wireRoomIDOfEvent(evt)
	}
	if live := event.LiveEvent(); live != nil {
		switch e := live.GetEvent().(type) {
		case *corev1.LiveEvent_UserTyping:
			return e.UserTyping.GetRoomId()
		case *corev1.LiveEvent_CallParticipantJoined:
			return e.CallParticipantJoined.GetRoomId()
		case *corev1.LiveEvent_CallParticipantLeft:
			return e.CallParticipantLeft.GetRoomId()
		}
	}
	return ""
}

func wireRoomIDOfEvent(event *corev1.Event) string {
	if event == nil {
		return ""
	}
	switch e := event.GetEvent().(type) {
	case *corev1.Event_RoomCreated:
		return e.RoomCreated.GetRoomId()
	case *corev1.Event_RoomUpdated:
		return e.RoomUpdated.GetRoomId()
	case *corev1.Event_RoomDeleted:
		return e.RoomDeleted.GetRoomId()
	case *corev1.Event_RoomArchived:
		return e.RoomArchived.GetRoomId()
	case *corev1.Event_RoomUnarchived:
		return e.RoomUnarchived.GetRoomId()
	case *corev1.Event_UserJoinedRoom:
		return e.UserJoinedRoom.GetRoomId()
	case *corev1.Event_UserLeftRoom:
		return e.UserLeftRoom.GetRoomId()
	case *corev1.Event_RoomMemberBanned:
		return e.RoomMemberBanned.GetRoomId()
	case *corev1.Event_RoomMemberUnbanned:
		return e.RoomMemberUnbanned.GetRoomId()
	case *corev1.Event_MessagePosted:
		return e.MessagePosted.GetRoomId()
	case *corev1.Event_MessageEdited:
		return e.MessageEdited.GetRoomId()
	case *corev1.Event_MessageRetracted:
		return e.MessageRetracted.GetRoomId()
	case *corev1.Event_ThreadCreated:
		return e.ThreadCreated.GetRoomId()
	case *corev1.Event_ReactionAdded:
		return e.ReactionAdded.GetRoomId()
	case *corev1.Event_ReactionRemoved:
		return e.ReactionRemoved.GetRoomId()
	case *corev1.Event_VoiceCallParticipantJoined:
		return e.VoiceCallParticipantJoined.GetRoomId()
	case *corev1.Event_VoiceCallParticipantLeft:
		return e.VoiceCallParticipantLeft.GetRoomId()
	case *corev1.Event_VoiceCallStarted:
		return e.VoiceCallStarted.GetRoomId()
	case *corev1.Event_VoiceCallEnded:
		return e.VoiceCallEnded.GetRoomId()
	}
	return ""
}
