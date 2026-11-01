package chatto

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/wire"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// RecoveryOptions binds a subscription to the caller's durable inbox. Checkpoint
// must atomically retain a non-nil event and the supplied cursor before returning.
// A nil event advances only the cursor. Empty cursors clear the resume position.
// Callback errors are fatal; they never cause automatic retries. Callbacks run
// serially. Status is operational text and contains no server-provided content.
type RecoveryOptions struct {
	Cursor     string
	Checkpoint func(cursor string, event *MessageEvent) error
	Status     func(string)
}

type reconnectHint struct{ delay time.Duration }

// ObserveTimeline observes message posts and invalidations with reconnect and
// replay, including the viewer's own posts, echoes, and non-text messages.
// This is an invalidation feed, not a complete resource projection: hydrate
// changed timelines with Timeline. Body is only the realtime plaintext preview.
// Room directory, reaction and profile changes still require resource refreshes.
// Existing bot observers retain their self/echo suppression.
func (c *Client) ObserveTimeline(ctx context.Context, viewer string, options RecoveryOptions, ready func(), handle func(context.Context, MessageEvent) error) error {
	copy := *c
	copy.allMessages = true
	return copy.ObserveResumable(ctx, viewer, options, ready, handle)
}

func (reconnectHint) Error() string { return "Chatto requested reconnect" }

// ObserveResumable reconnects with bounded exponential backoff and jitter. It
// resumes from the last accepted cursor, including heartbeat refreshes. Replay
// fallback emits a durable RecoveryGap before live events. The caller must drain
// its saved inbox before calling this method. Handle must not perform model work.
func (c *Client) ObserveResumable(ctx context.Context, viewer string, options RecoveryOptions, ready func(), handle func(context.Context, MessageEvent) error) error {
	cursor := options.Cursor
	hadConnection := cursor != ""
	checkpoint := func(next string, event *MessageEvent) error {
		if options.Checkpoint != nil {
			if err := options.Checkpoint(next, event); err != nil {
				return err
			}
		}
		cursor = next
		return nil
	}
	delay := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		retry, connected, err := c.observeConnection(ctx, viewer, cursor, hadConnection || cursor != "", checkpoint, ready, handle)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !retry {
			return err
		}
		if connected {
			delay = time.Second
			hadConnection = true
		}
		if options.Status != nil {
			options.Status("Disconnected; reconnecting")
		}
		wait := delay + time.Duration(rand.Int64N(int64(delay/2)+1))
		var hint reconnectHint
		if errors.As(err, &hint) {
			wait = max(wait, hint.delay)
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		delay = min(30*time.Second, delay*2)
	}
}

// observeConnection distinguishes transport failures from callback/protocol
// failures. Only the former and explicitly retryable server closes reconnect.
func (c *Client) observeConnection(ctx context.Context, viewer, cursor string, hadConnection bool, checkpoint func(string, *MessageEvent) error, ready func(), handle func(context.Context, MessageEvent) error) (retry, connected bool, result error) {
	origin := strings.TrimSuffix(c.endpoint, "/api/connect/chatto.api.v1.")
	endpoint := "ws" + strings.TrimPrefix(origin, "http") + "/api/realtime"
	conn, response, err := (&websocket.Dialer{HandshakeTimeout: 10 * time.Second}).DialContext(ctx, endpoint, nil)
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		if response != nil && response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != http.StatusTooManyRequests {
			return false, false, errors.New("Chatto realtime connection rejected")
		}
		return true, false, errors.New("connect Chatto realtime")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	conn.SetReadLimit(4 << 20)
	sub := &wire.Subscribe{ProtocolVersion: 4, BearerToken: &c.token, InitialState: 1}
	if cursor != "" {
		sub.ResumeCursor = &cursor
	}
	data, _ := proto.Marshal(sub)
	if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return true, false, errors.New("set realtime write deadline")
	}
	if conn.WriteMessage(websocket.BinaryMessage, data) != nil {
		return true, false, errors.New("send realtime subscription")
	}
	seen := map[string]bool{}
	var order []string
	caughtUp := false
	for {
		if err := conn.SetReadDeadline(time.Now().Add(90 * time.Second)); err != nil {
			return true, connected, errors.New("set realtime read deadline")
		}
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return true, connected, errors.New("Chatto realtime disconnected")
		}
		if kind != websocket.BinaryMessage {
			return false, connected, errors.New("unexpected realtime message type")
		}
		var frame wire.Frame
		if proto.Unmarshal(data, &frame) != nil {
			return false, connected, errors.New("invalid realtime frame")
		}
		if close := frame.GetClose(); close != nil {
			if close.Code == 6 && close.Reconnect {
				gap := MessageEvent{RecoveryGap: true}
				if err := checkpoint("", &gap); err != nil {
					return false, connected, err
				}
				if err := handle(ctx, gap); err != nil {
					return false, connected, err
				}
			}
			if close.Reconnect {
				wait := time.Duration(0)
				if close.RetryAfter != nil && close.RetryAfter.CheckValid() == nil {
					wait = max(0, close.RetryAfter.AsDuration())
				}
				return true, connected, reconnectHint{delay: wait}
			}
			return false, connected, errors.New("Chatto closed the realtime subscription")
		}
		if caught := frame.GetCaughtUp(); caught != nil {
			if caughtUp || (caught.Recovery != 1 && caught.Recovery != 3) {
				return false, connected, errors.New("unexpected realtime recovery result")
			}
			var gap *MessageEvent
			if hadConnection && caught.Recovery != 1 {
				gap = &MessageEvent{RecoveryGap: true}
			}
			if err := checkpoint(caught.Cursor, gap); err != nil {
				return false, connected, err
			}
			if gap != nil {
				if err := handle(ctx, *gap); err != nil {
					return false, connected, err
				}
			}
			cursor = caught.Cursor
			caughtUp = true
			connected = true
			if ready != nil {
				ready()
			}
			continue
		}
		if heartbeat := frame.GetHeartbeat(); heartbeat != nil {
			if heartbeat.Cursor != "" {
				if err := checkpoint(heartbeat.Cursor, nil); err != nil {
					return false, connected, err
				}
				cursor = heartbeat.Cursor
			}
			continue
		}
		event := frame.GetEvent()
		if event == nil {
			continue
		}
		next := cursor
		if event.GetCursor() != "" {
			next = event.GetCursor()
		}
		update, ok := decodeVisibleEvent(event, viewer, c.allMessages)
		update.ID = event.Id
		var accepted *MessageEvent
		if ok && !seen[event.Id] {
			accepted = &update
		}
		if err := checkpoint(next, accepted); err != nil {
			return false, connected, err
		}
		cursor = next
		if accepted != nil {
			if err := handle(ctx, update); err != nil {
				return false, connected, err
			}
			seen[event.Id] = true
			order = append(order, event.Id)
			if len(order) > 1024 {
				delete(seen, order[0])
				order = order[1:]
			}
		}
	}
}
