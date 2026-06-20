package http_server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/auth"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	clientLiveProtocol              = "chatto.client-live-protobuf.v1"
	clientLiveTicketTTL             = time.Minute
	clientLiveWriteTimeout          = 10 * time.Second
	clientLiveReadLimitBytes        = 1 << 20
	clientLivePingInterval          = 25 * time.Second
	clientLiveMaxConcurrentRequests = 8

	clientLiveCapabilityEvents        = "live.events.v1"
	clientLiveCapabilityRequests      = "live.requests.v1"
	clientLiveCapabilityRoomHistory   = "history.room_events.v1"
	clientLiveCapabilityThreadHistory = "history.thread_events.v1"
)

type clientLiveTicketClaims struct {
	UserID    string `json:"uid"`
	Origin    string `json:"origin,omitempty"`
	ExpiresAt int64  `json:"exp"`
	Nonce     string `json:"nonce"`
}

type clientLiveTokenResponse struct {
	URL       string    `json:"url"`
	Ticket    string    `json:"ticket"`
	Protocol  string    `json:"protocol"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (s *HTTPServer) setupLiveRoutes(allowedOrigins []string) {
	s.router.POST("/api/live-token", s.handleClientLiveToken(allowedOrigins))
	s.router.GET("/api/live", s.handleClientLiveWebSocket(allowedOrigins))
}

func (s *HTTPServer) handleClientLiveToken(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.originAllowedForRequest(c.Request, allowedOrigins) {
			c.JSON(http.StatusForbidden, gin.H{"error": "origin not allowed"})
			return
		}

		c.Request = s.injectUserIntoContext(c)
		user := auth.ForContext(c.Request.Context())
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		expiresAt := time.Now().Add(clientLiveTicketTTL).UTC()
		ticket, err := s.signClientLiveTicket(clientLiveTicketClaims{
			UserID:    user.Id,
			Origin:    c.GetHeader("Origin"),
			ExpiresAt: expiresAt.Unix(),
			Nonce:     randomTokenNonce(),
		})
		if err != nil {
			s.logger.Warn("Failed to sign live ticket", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create live ticket"})
			return
		}

		c.JSON(http.StatusOK, clientLiveTokenResponse{
			URL:       s.clientLivePublicURL(c),
			Ticket:    ticket,
			Protocol:  clientLiveProtocol,
			ExpiresAt: expiresAt,
		})
	}
}

func (s *HTTPServer) handleClientLiveWebSocket(allowedOrigins []string) gin.HandlerFunc {
	upgrader := websocket.Upgrader{
		EnableCompression: s.config.Webserver.WebSocketCompressionEnabled(),
		CheckOrigin: func(r *http.Request) bool {
			return s.originAllowedForRequest(r, allowedOrigins)
		},
	}

	return func(c *gin.Context) {
		ticket := c.Query("ticket")
		if ticket == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ticket is required"})
			return
		}

		claims, err := s.validateClientLiveTicket(ticket)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired ticket"})
			return
		}
		if claims.Origin != "" && !strings.EqualFold(claims.Origin, c.GetHeader("Origin")) {
			c.JSON(http.StatusForbidden, gin.H{"error": "origin mismatch"})
			return
		}

		user, err := s.core.GetUser(c.Request.Context(), claims.UserID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authenticated user no longer exists"})
			return
		}

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		closeMetric := s.metrics.openClientLiveSocket()
		defer closeMetric()

		sessionCtx, cancel := context.WithCancel(auth.WithUser(c.Request.Context(), user))
		defer cancel()

		session := newClientLiveSession(s, conn, user.Id, cancel)
		session.run(sessionCtx)
	}
}

type clientLiveSession struct {
	server *HTTPServer
	conn   *websocket.Conn
	userID string
	cancel context.CancelFunc

	send chan *corev1.ClientLiveServerFrame
	reqs chan struct{}

	seqMu sync.Mutex
	seq   uint64
}

func newClientLiveSession(server *HTTPServer, conn *websocket.Conn, userID string, cancel context.CancelFunc) *clientLiveSession {
	return &clientLiveSession{
		server: server,
		conn:   conn,
		userID: userID,
		cancel: cancel,
		send:   make(chan *corev1.ClientLiveServerFrame, 64),
		reqs:   make(chan struct{}, clientLiveMaxConcurrentRequests),
	}
}

func (s *clientLiveSession) run(ctx context.Context) {
	stream, err := s.server.core.StreamMyEvents(ctx, s.userID)
	if err != nil {
		_ = s.writeFrame(&corev1.ClientLiveServerFrame{
			Payload: &corev1.ClientLiveServerFrame_Error{
				Error: &corev1.ClientLiveError{Code: "stream_failed", Message: "failed to start live stream", Fatal: true},
			},
		})
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.readLoop(ctx)
	}()
	go func() {
		defer wg.Done()
		s.writeLoop(ctx)
	}()

	s.enqueue(&corev1.ClientLiveServerFrame{
		Payload: &corev1.ClientLiveServerFrame_Hello{
			Hello: &corev1.ClientLiveHello{
				Protocol:      clientLiveProtocol,
				ServerVersion: s.server.version,
				Capabilities: []string{
					"events",
					"liveEvents",
					"requests",
					clientLiveCapabilityEvents,
					clientLiveCapabilityRequests,
					clientLiveCapabilityRoomHistory,
					clientLiveCapabilityThreadHistory,
				},
			},
		},
	})

	pingTicker := time.NewTicker(clientLivePingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.cancel()
			_ = s.conn.Close()
			wg.Wait()
			return
		case <-pingTicker.C:
			s.enqueue(&corev1.ClientLiveServerFrame{
				Payload: &corev1.ClientLiveServerFrame_Ping{
					Ping: &corev1.ClientLivePing{Nonce: randomTokenNonce()},
				},
			})
		case event, ok := <-stream:
			if !ok {
				s.cancel()
				_ = s.conn.Close()
				wg.Wait()
				return
			}
			frame, err := s.clientLiveFrameForEvent(ctx, event)
			if err != nil {
				s.server.logger.Warn("Failed to encode client live event", "error", err)
				s.enqueue(&corev1.ClientLiveServerFrame{
					Payload: &corev1.ClientLiveServerFrame_Error{
						Error: &corev1.ClientLiveError{Code: "encode_failed", Message: "failed to encode live event"},
					},
				})
				continue
			}
			s.enqueue(frame)
			if core.EventSessionTerminated(event) != nil {
				s.cancel()
				_ = s.conn.Close()
				wg.Wait()
				return
			}
		}
	}
}

func (s *clientLiveSession) readLoop(ctx context.Context) {
	s.conn.SetReadLimit(clientLiveReadLimitBytes)
	for {
		messageType, payload, err := s.conn.ReadMessage()
		if err != nil {
			s.cancel()
			return
		}
		if messageType != websocket.BinaryMessage {
			s.enqueueError(0, "invalid_frame", "client live frames must be binary protobuf messages", false)
			continue
		}

		var frame corev1.ClientLiveClientFrame
		if err := proto.Unmarshal(payload, &frame); err != nil {
			s.enqueueError(0, "invalid_frame", "invalid client live frame", false)
			continue
		}
		s.handleClientFrame(ctx, &frame)
	}
}

func (s *clientLiveSession) handleClientFrame(ctx context.Context, frame *corev1.ClientLiveClientFrame) {
	switch payload := frame.Payload.(type) {
	case *corev1.ClientLiveClientFrame_Hello:
		// The server sends its hello immediately; client hello is accepted for
		// future capability negotiation and ignored in this first pass.
	case *corev1.ClientLiveClientFrame_Ack:
		// Reserved for future resumability/backpressure.
	case *corev1.ClientLiveClientFrame_Pong:
		// Application-level ping/pong is advisory; Gorilla's control frames
		// still handle transport liveness.
	case *corev1.ClientLiveClientFrame_Request:
		s.dispatchClientRequest(ctx, frame.GetRequestId(), payload.Request)
	default:
		s.enqueueError(frame.GetRequestId(), "invalid_request", "client frame has no payload", false)
	}
}

func (s *clientLiveSession) dispatchClientRequest(ctx context.Context, requestID uint64, req *corev1.ClientLiveRequest) {
	if req == nil {
		s.enqueueError(requestID, "invalid_request", "missing request payload", false)
		return
	}
	select {
	case s.reqs <- struct{}{}:
		go func() {
			defer func() { <-s.reqs }()
			s.handleClientRequest(ctx, requestID, req)
		}()
	default:
		s.enqueueError(requestID, "too_many_requests", "too many concurrent live requests", false)
	}
}

func (s *clientLiveSession) handleClientRequest(ctx context.Context, requestID uint64, req *corev1.ClientLiveRequest) {
	started := time.Now()
	requestType := clientLiveMetricRequestType(req.GetType())
	outcome := "ok"
	defer func() {
		s.server.metrics.recordClientLiveRequest(requestType, outcome, time.Since(started))
	}()

	if handled, historyOutcome := s.handleHistoryRequest(ctx, requestID, req); handled {
		outcome = historyOutcome
		return
	}
	switch req.GetType() {
	case "ping":
		s.enqueue(&corev1.ClientLiveServerFrame{
			RequestId: requestID,
			Payload: &corev1.ClientLiveServerFrame_Response{
				Response: &corev1.ClientLiveResponse{Type: "pong", Payload: req.GetPayload()},
			},
		})
	default:
		outcome = "unknown_request"
		s.enqueueError(requestID, "unknown_request", "unknown live request type", false)
	}
}

func clientLiveMetricRequestType(requestType string) string {
	switch requestType {
	case clientLiveRequestRoomEvents,
		clientLiveRequestRoomEventsAround,
		clientLiveRequestRoomEvent,
		clientLiveRequestThreadEvents,
		clientLiveRequestThreadEventsAround,
		"ping":
		return requestType
	default:
		return "unknown"
	}
}

func (s *clientLiveSession) writeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-s.send:
			if !ok {
				return
			}
			if err := s.writeFrame(frame); err != nil {
				s.cancel()
				return
			}
		}
	}
}

func (s *clientLiveSession) enqueue(frame *corev1.ClientLiveServerFrame) {
	if frame == nil {
		return
	}
	s.seqMu.Lock()
	defer s.seqMu.Unlock()
	s.seq++
	frame.DeliverySequence = s.seq
	select {
	case s.send <- frame:
	default:
		s.cancel()
	}
}

func (s *clientLiveSession) enqueueError(requestID uint64, code, message string, fatal bool) {
	s.server.metrics.recordClientLiveError(code)
	s.enqueue(&corev1.ClientLiveServerFrame{
		RequestId: requestID,
		Payload: &corev1.ClientLiveServerFrame_Error{
			Error: &corev1.ClientLiveError{Code: code, Message: message, Fatal: fatal},
		},
	})
	if fatal {
		s.cancel()
	}
}

func (s *clientLiveSession) writeFrame(frame *corev1.ClientLiveServerFrame) error {
	payload, err := proto.Marshal(frame)
	if err != nil {
		return err
	}
	if err := s.conn.SetWriteDeadline(time.Now().Add(clientLiveWriteTimeout)); err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, payload)
}

func (s *clientLiveSession) clientLiveFrameForEvent(ctx context.Context, event core.EventEnvelope) (*corev1.ClientLiveServerFrame, error) {
	if event == nil {
		return nil, errors.New("nil event envelope")
	}
	frame := &corev1.ClientLiveServerFrame{
		Id:             event.ID(),
		CreatedAt:      event.CreatedAt(),
		ActorId:        event.ActorID(),
		StreamSequence: event.DeliverySeq(),
	}
	switch {
	case event.EVTEvent() != nil:
		live, err := s.clientLiveEventForEnvelope(ctx, event)
		if err != nil {
			return nil, err
		}
		frame.Payload = &corev1.ClientLiveServerFrame_LiveEvent{LiveEvent: live}
	case event.LiveEvent() != nil:
		frame.Payload = &corev1.ClientLiveServerFrame_LiveEvent{LiveEvent: event.LiveEvent()}
	case event.HeartbeatEvent() != nil:
		frame.Payload = &corev1.ClientLiveServerFrame_Heartbeat{Heartbeat: event.HeartbeatEvent()}
	default:
		return nil, errors.New("event envelope has no payload")
	}
	return frame, nil
}

func (s *HTTPServer) signClientLiveTicket(claims clientLiveTicketClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := s.signClientLiveTicketPayload(encodedPayload)
	return encodedPayload + "." + signature, nil
}

func (s *HTTPServer) validateClientLiveTicket(ticket string) (clientLiveTicketClaims, error) {
	payload, signature, ok := strings.Cut(ticket, ".")
	if !ok || payload == "" || signature == "" {
		return clientLiveTicketClaims{}, errors.New("malformed ticket")
	}
	expected := s.signClientLiveTicketPayload(payload)
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return clientLiveTicketClaims{}, errors.New("invalid signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return clientLiveTicketClaims{}, err
	}
	var claims clientLiveTicketClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return clientLiveTicketClaims{}, err
	}
	if claims.UserID == "" || claims.ExpiresAt <= time.Now().Unix() {
		return clientLiveTicketClaims{}, errors.New("expired ticket")
	}
	return claims, nil
}

func (s *HTTPServer) signClientLiveTicketPayload(payload string) string {
	mac := hmac.New(sha256.New, []byte(s.config.Webserver.CookieSigningSecret))
	mac.Write([]byte(clientLiveProtocol))
	mac.Write([]byte{0})
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *HTTPServer) clientLivePublicURL(c *gin.Context) string {
	base := s.config.Webserver.URL
	if base == "" {
		scheme := "http"
		if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else if c.Request.TLS != nil {
			scheme = "https"
		}
		base = scheme + "://" + c.Request.Host
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "/api/live"
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = "/api/live"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (s *HTTPServer) originAllowedForRequest(r *http.Request, allowedOrigins []string) bool {
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
	if parsedOrigin, err := url.Parse(origin); err == nil {
		if strings.EqualFold(parsedOrigin.Host, host) {
			return true
		}
	}
	return false
}

func randomTokenNonce() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

func clientLiveTokenURL() string {
	return "/api/live-token"
}

func clientLiveDiscoveryProtocol() string {
	return clientLiveProtocol
}
