package bot

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
	"github.com/gorilla/websocket"
)

func TestRespondReadsBoundThreadAndPostsAnswer(t *testing.T) {
	reads, posts, steps := 0, 0, 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if input["roomId"] != "room" || input["threadRootEventId"] != "root" {
			t.Error("destination changed")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/GetThreadEvents") {
			reads++
			_, _ = w.Write([]byte(`{"page":{"events":[{"messagePosted":{"message":{"body":"context"}}}],"hasOlder":true}}`))
		} else if strings.HasSuffix(r.URL.Path, "/CreateMessage") {
			posts++
			key, _ := input["idempotencyKey"].(string)
			if input["body"] != "answer" || input["inReplyTo"] != "trigger" || !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(key) {
				t.Error("invalid post")
			}
			_, _ = w.Write([]byte(`{"message":{"id":"posted"}}`))
		} else {
			t.Error("unexpected route")
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	c, err := chatto.New(s.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	complete := func(_ context.Context, messages []agent.Message) (agent.Message, error) {
		steps++
		var m agent.Message
		if steps == 1 {
			if !strings.Contains(string(messages[2]["content"]), "question") || !strings.Contains(string(messages[1]["content"]), "context") {
				t.Error("missing trigger")
			}
			_ = json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"read_thread","arguments":"{}"}}]}`), &m)
		} else {
			var content string
			_ = json.Unmarshal(messages[4]["content"], &content)
			if !strings.Contains(content, `"olderOmitted":true`) || !strings.Contains(content, "context") {
				t.Error("missing thread result")
			}
			_ = json.Unmarshal([]byte(`{"role":"assistant","content":"answer"}`), &m)
		}
		return m, nil
	}
	var logs []string
	state := &conversation{key: threadKey{"room", "root"}, complete: complete}
	if err := respond(context.Background(), c, state, []chatto.MessageEvent{{Message: chatto.Message{ID: "trigger", Body: "question"}, Addressed: true}}, func(line string) { logs = append(logs, line) }); err != nil {
		t.Fatal(err)
	}
	if reads != 2 || posts != 1 || steps != 2 {
		t.Fatalf("reads=%d posts=%d steps=%d", reads, posts, steps)
	}
	joined := strings.Join(logs, "\n")
	if strings.Contains(joined, "Model request") || !strings.Contains(joined, "Reply posted") || strings.Contains(joined, "question") || strings.Contains(joined, "answer") {
		t.Fatalf("unexpected operational logs: %s", joined)
	}
}

func TestRealtimeMessageReadsThreadAndReactsWithoutPosting(t *testing.T) {
	// Public protocol v4 frames: event "ping", actor "human", room "room".
	// The channel frame mentions "bot"; the DM frame has no mention. Keep these
	// byte fixtures independent of the SDK's private generated protobuf types.
	for name, frameHex := range map[string]string{
		"channel": "0a300a0470696e671a0568756d616ec201200a04726f6f6d3a0912050a03626f743001420b7468616e6b732040626f744801",
		"dm":      "0a200a0470696e671a0568756d616ec201100a04726f6f6d42067468616e6b734802",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := hex.DecodeString(frameHex)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			reads, reactions := 0, 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/realtime" {
					conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
					if err != nil {
						t.Error(err)
						return
					}
					defer conn.Close()
					if _, _, err := conn.ReadMessage(); err != nil {
						t.Error(err)
						return
					}
					// A top-level ping must use its own event ID as the thread root.
					if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
						t.Error(err)
						return
					}
					_, _, _ = conn.ReadMessage()
					return
				}
				var input map[string]any
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if input["roomId"] != "room" {
					t.Error("wrong room")
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/GetThreadEvents"):
					reads++
					if input["threadRootEventId"] != "ping" {
						t.Error("wrong root")
					}
					_, _ = w.Write([]byte(`{"page":{"events":[{"messagePosted":{"message":{"body":"thread context"}}}]}}`))
				case strings.HasSuffix(r.URL.Path, "/AddReaction"):
					reactions++
					if input["messageEventId"] != "ping" || input["emoji"] != "heart" {
						t.Error("wrong reaction target")
					}
					_, _ = w.Write([]byte(`{"added":true}`))
				default:
					t.Error("unexpected request (including a text reply)")
					w.WriteHeader(400)
				}
			}))
			defer s.Close()
			c, err := chatto.New(s.URL, "token")
			if err != nil {
				t.Fatal(err)
			}
			complete := func(_ context.Context, messages []agent.Message) (agent.Message, error) {
				if !strings.Contains(string(messages[1]["content"]), "thread context") {
					t.Error("model lacks thread context")
				}
				var message agent.Message
				err := json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"reaction","type":"function","function":{"name":"react_to_ping","arguments":"{\"emoji\":\"heart\"}"}}]}`), &message)
				return message, err
			}
			err = c.Pings(ctx, "bot", nil, func(ctx context.Context, m chatto.Message) error {
				root := m.ThreadRootID()
				state := &conversation{key: threadKey{m.RoomID, root}, complete: complete}
				if err := respond(ctx, c, state, []chatto.MessageEvent{{Message: m, Addressed: true}}, nil); err != nil {
					return err
				}
				cancel()
				return nil
			})
			if !errors.Is(err, context.Canceled) || reads != 1 || reactions != 1 {
				t.Fatalf("reads=%d reactions=%d err=%v", reads, reactions, err)
			}
		})
	}
}
