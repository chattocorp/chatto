package chatto

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/wire"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

func sendFrame(t *testing.T, c *websocket.Conn, f *wire.Frame) {
	t.Helper()
	data, err := proto.Marshal(f)
	if err != nil {
		t.Error(err)
		return
	}
	if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Error(err)
	}
}

func TestResumeUsesHeartbeatCursorAndCheckpointsBeforeDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var connections atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer c.Close()
		_, data, err := c.ReadMessage()
		if err != nil {
			t.Error(err)
			return
		}
		var sub wire.Subscribe
		if proto.Unmarshal(data, &sub) != nil {
			t.Error("bad subscribe")
			return
		}
		n := connections.Add(1)
		if n == 1 {
			if sub.GetResumeCursor() != "saved" {
				t.Error("initial cursor missing")
			}
			e := pingEvent()
			e.Cursor = proto.String("event-cursor")
			sendFrame(t, c, &wire.Frame{Frame: &wire.Frame_Event{Event: e}})
			sendFrame(t, c, &wire.Frame{Frame: &wire.Frame_Event{Event: e}})
			sendFrame(t, c, &wire.Frame{Frame: &wire.Frame_CaughtUp{CaughtUp: &wire.CaughtUp{Cursor: "caught", Recovery: 1}}})
			sendFrame(t, c, &wire.Frame{Frame: &wire.Frame_Heartbeat{Heartbeat: &wire.Heartbeat{Cursor: "fresh-heartbeat"}}})
			return
		}
		if sub.GetResumeCursor() != "fresh-heartbeat" {
			t.Error("heartbeat not used to resume")
		}
		sendFrame(t, c, &wire.Frame{Frame: &wire.Frame_CaughtUp{CaughtUp: &wire.CaughtUp{Cursor: "reconnected", Recovery: 1}}})
		_, _, _ = c.ReadMessage()
	}))
	defer s.Close()
	c, _ := New(s.URL, "token")
	checkpointed := false
	readyCount := 0
	deliveries := 0
	err := c.ObserveResumable(ctx, "bot", RecoveryOptions{Cursor: "saved", Checkpoint: func(cursor string, e *MessageEvent) error {
		if e != nil {
			if cursor != "event-cursor" {
				t.Error("wrong event cursor")
			}
			checkpointed = true
		}
		return nil
	}}, func() {
		readyCount++
		if readyCount == 2 {
			cancel()
		}
	}, func(_ context.Context, e MessageEvent) error {
		if !checkpointed || e.ID != "ping" {
			t.Error("delivery preceded checkpoint")
		}
		deliveries++
		return nil
	})
	if !errors.Is(err, context.Canceled) || deliveries != 1 || connections.Load() != 2 {
		t.Fatalf("deliveries=%d connections=%d err=%v", deliveries, connections.Load(), err)
	}
}

func TestRecoveryFallbackAndStorageFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "gap", true: "storage failure"}[fail], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var connections atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer c.Close()
				connections.Add(1)
				if _, _, err = c.ReadMessage(); err != nil {
					return
				}
				sendFrame(t, c, &wire.Frame{Frame: &wire.Frame_CaughtUp{CaughtUp: &wire.CaughtUp{Cursor: "new-boundary", Recovery: 3}}})
				_, _, _ = c.ReadMessage()
			}))
			defer s.Close()
			c, _ := New(s.URL, "token")
			saved, handled := false, false
			storageErr := errors.New("storage failed")
			err := c.ObserveResumable(ctx, "bot", RecoveryOptions{Cursor: "expired", Checkpoint: func(cursor string, e *MessageEvent) error {
				if cursor != "new-boundary" || e == nil || !e.RecoveryGap {
					t.Error("fallback not checkpointed")
				}
				if fail {
					return storageErr
				}
				saved = true
				return nil
			}}, func() {
				if !handled {
					t.Error("ready before gap handling")
				}
				cancel()
			}, func(_ context.Context, e MessageEvent) error {
				if !saved || !e.RecoveryGap {
					t.Error("gap not durable")
				}
				handled = true
				return nil
			})
			if fail && (!errors.Is(err, storageErr) || handled || connections.Load() != 1) {
				t.Fatal("storage failure retried or delivered")
			}
			if !fail && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestTerminalCloseDoesNotReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var count atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		count.Add(1)
		if _, _, err = c.ReadMessage(); err != nil {
			return
		}
		sendFrame(t, c, &wire.Frame{Frame: &wire.Frame_Close{Close: &wire.Close{Code: 4, Reconnect: false}}})
	}))
	defer s.Close()
	c, _ := New(s.URL, "token")
	if err := c.ObserveResumable(ctx, "bot", RecoveryOptions{}, nil, func(context.Context, MessageEvent) error { return nil }); err == nil || errors.Is(err, context.DeadlineExceeded) || count.Load() != 1 {
		t.Fatalf("terminal close retried: %v", err)
	}
}

func TestResyncClearsCursorAndHonorsRetryHint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		if _, _, err = c.ReadMessage(); err != nil {
			return
		}
		sendFrame(t, c, &wire.Frame{Frame: &wire.Frame_Close{Close: &wire.Close{Code: 6, Reconnect: true, RetryAfter: durationpb.New(7 * time.Second)}}})
	}))
	defer s.Close()
	c, _ := New(s.URL, "token")
	saved, handled := false, false
	retry, _, err := c.observeConnection(ctx, "bot", "stale", true, func(cursor string, event *MessageEvent) error {
		if cursor != "" || event == nil || !event.RecoveryGap {
			t.Error("resync did not clear cursor durably")
		}
		saved = true
		return nil
	}, nil, func(context.Context, MessageEvent) error {
		if !saved {
			t.Error("gap preceded checkpoint")
		}
		handled = true
		return nil
	})
	var hint reconnectHint
	if !retry || !handled || !errors.As(err, &hint) || hint.delay != 7*time.Second {
		t.Fatalf("incorrect resync handling: %v", err)
	}
}
