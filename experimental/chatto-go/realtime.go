package chatto

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/wire"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// Pings opens a live-only realtime subscription authenticated by the API token.
// It delivers direct messages and server-verified mentions of the viewer,
// including role and broadcast mentions. Echoes and the viewer's own messages
// cannot trigger a session. The server-reported room kind determines DM intake.
// Callbacks run in order. Their errors stop consumption, as do socket failures.
// There is no replay or automatic reconnect yet; restarting starts at live time.
func (c *Client) Pings(ctx context.Context, viewer string, ready func(), handle func(context.Context, Message) error) error {
	return c.ObserveMessages(ctx, viewer, ready, func(ctx context.Context, event MessageEvent) error {
		if event.Addressed {
			return handle(ctx, event.Message)
		}
		return nil
	})
}

// MessageEvent supplies context independently from whether a message addresses
// the viewer. Invalidations require reconciliation of affected cached history.
// Such events have no Message. A missing target requires room-wide reconciliation.
type MessageEvent struct {
	// ID identifies the source event, including history invalidations.
	ID string
	// RecoveryGap means replay was unavailable. Refresh retained thread context.
	RecoveryGap     bool
	Message         Message
	Addressed       bool
	InvalidatedRoom string
	// InvalidatedMessage narrows an edit/retraction to a source message. Consumers
	// may ignore it for conversations whose retained and pending context omit it.
	InvalidatedMessage string
	// InvalidationReason describes the event without exposing message content.
	InvalidationReason InvalidationReason
	// InvalidateAll covers account deletion and viewer permission changes.
	InvalidateAll bool
}

// InvalidationReason identifies why cached context may no longer be valid.
type InvalidationReason string

const (
	MessageEdited            InvalidationReason = "message_edited"
	MessageRetracted         InvalidationReason = "message_retracted"
	RoomDeleted              InvalidationReason = "room_deleted"
	ViewerLeftRoom           InvalidationReason = "viewer_left_room"
	ViewerPermissionsChanged InvalidationReason = "viewer_permissions_changed"
	AccountDeleted           InvalidationReason = "account_deleted"
)

// ObserveMessages delivers visible messages, including unaddressed context, and
// history invalidations. Self-posts and echoes are omitted. Callbacks must return
// promptly; callers should queue model work rather than block the socket reader.
// Subscription lifetime and live-only recovery match Pings.
func (c *Client) ObserveMessages(ctx context.Context, viewer string, ready func(), handle func(context.Context, MessageEvent) error) error {
	origin := strings.TrimSuffix(c.endpoint, "/api/connect/chatto.api.v1.")
	endpoint := "ws" + strings.TrimPrefix(origin, "http") + "/api/realtime"
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, response, err := dialer.DialContext(ctx, endpoint, nil)
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		return errors.New("connect Chatto realtime")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	conn.SetReadLimit(4 << 20)
	data, err := proto.Marshal(&wire.Subscribe{ProtocolVersion: 4, BearerToken: &c.token, InitialState: 1})
	if err != nil {
		return errors.New("encode realtime subscription")
	}
	if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return errors.New("set realtime write deadline")
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return errors.New("send realtime subscription")
	}
	// Bounded duplicate protection for a single connection. No cursor is persisted.
	seen := make(map[string]bool)
	var order []string
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := conn.SetReadDeadline(time.Now().Add(90 * time.Second)); err != nil {
			return errors.New("set realtime read deadline")
		}
		kind, data, err := conn.ReadMessage()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return errors.New("Chatto realtime disconnected; restart to reconnect")
		}
		if kind != websocket.BinaryMessage {
			return errors.New("unexpected realtime message type")
		}
		var frame wire.Frame
		if proto.Unmarshal(data, &frame) != nil {
			return errors.New("invalid realtime frame")
		}
		if frame.GetClose() != nil {
			return errors.New("Chatto closed the realtime subscription")
		}
		if frame.GetCaughtUp() != nil && ready != nil {
			ready()
			ready = nil
		}
		event := frame.GetEvent()
		if event == nil || event.Id == "" {
			continue
		}
		update, ok := messageEvent(event, viewer)
		if !ok || seen[event.Id] {
			continue
		}
		seen[event.Id] = true
		order = append(order, event.Id)
		if len(order) > 1024 {
			delete(seen, order[0])
			order = order[1:]
		}
		if err := handle(ctx, update); err != nil {
			return err
		}
	}
}

func ping(event *wire.Event, viewer string) (Message, bool) {
	update, ok := messageEvent(event, viewer)
	return update.Message, ok && update.Addressed
}

func messageEvent(event *wire.Event, viewer string) (MessageEvent, bool) {
	update, ok := decodeMessageEvent(event, viewer)
	if event != nil {
		update.ID = event.Id
	}
	return update, ok
}

func decodeMessageEvent(event *wire.Event, viewer string) (MessageEvent, bool) {
	if event == nil || event.Id == "" {
		return MessageEvent{}, false
	}
	if event.GetViewerPermissionsChanged() != nil {
		return MessageEvent{InvalidateAll: true, InvalidationReason: ViewerPermissionsChanged}, true
	}
	if event.GetUserAccountDeleted() != nil {
		return MessageEvent{InvalidateAll: true, InvalidationReason: AccountDeleted}, true
	}
	if room := event.GetRoomDeleted(); room != nil {
		return MessageEvent{InvalidatedRoom: room.RoomId, InvalidationReason: RoomDeleted}, room.RoomId != ""
	}
	if room := event.GetUserLeftRoom(); room != nil && event.GetActorId() == viewer {
		return MessageEvent{InvalidatedRoom: room.RoomId, InvalidationReason: ViewerLeftRoom}, room.RoomId != ""
	}
	if edited := event.GetMessageEdited(); edited != nil {
		return MessageEvent{InvalidatedRoom: edited.RoomId, InvalidatedMessage: edited.MessageEventId, InvalidationReason: MessageEdited}, edited.RoomId != ""
	}
	if deleted := event.GetMessageRetracted(); deleted != nil {
		return MessageEvent{InvalidatedRoom: deleted.RoomId, InvalidatedMessage: deleted.MessageEventId, InvalidationReason: MessageRetracted}, deleted.RoomId != ""
	}
	if event == nil || event.Id == "" || event.GetActorId() == "" || event.GetActorId() == viewer {
		return MessageEvent{}, false
	}
	posted := event.GetMessagePosted()
	if posted == nil || posted.RoomId == "" || posted.EchoOfEventId != "" || posted.BodyPlaintext == nil {
		return MessageEvent{}, false
	}
	addressed := posted.RoomKind == wire.RoomKind_ROOM_KIND_DM
	for _, mention := range posted.Mentions {
		if !mention.IncludesViewer {
			continue
		}
		switch cause := mention.Cause.(type) {
		case *wire.Mention_Direct:
			addressed = addressed || cause.Direct.GetUserId() == viewer
		case *wire.Mention_Role, *wire.Mention_Here, *wire.Mention_All:
			addressed = true
		}
	}
	return MessageEvent{Message: Message{ID: event.Id, RoomID: posted.RoomId, ActorID: event.GetActorId(), Body: posted.GetBodyPlaintext(), ThreadRootEventID: posted.ThreadRootEventId}, Addressed: addressed}, true
}
