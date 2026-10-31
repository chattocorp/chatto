package bot

import (
	"context"
	"encoding/json"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestEachTurnGetsCurrentUTCTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &conversation{key: threadKey{"room", "root"}}
		var system string
		c.complete = func(_ context.Context, messages []agent.Message) (agent.Message, error) {
			var content string
			if err := json.Unmarshal(messages[len(messages)-1]["content"], &content); err != nil {
				t.Fatal(err)
			}
			var batch struct {
				CurrentTime string `json:"currentTime"`
			}
			if err := json.Unmarshal([]byte(content), &batch); err != nil {
				t.Fatal(err)
			}
			if batch.CurrentTime != time.Now().UTC().Format(time.RFC3339) {
				t.Fatal("current turn did not get UTC clock")
			}
			if system != "" && system != string(messages[0]["content"]) {
				t.Fatal("clock changed cached system prefix")
			}
			system = string(messages[0]["content"])
			return answer(), nil
		}
		f := newFake()
		if err := respond(context.Background(), f, c, []chatto.MessageEvent{input("first", "today?", true)}, nil); err != nil {
			t.Fatal(err)
		}
		time.Sleep(24 * time.Hour)
		c.model = agent.Restore(c.model.Snapshot())
		if err := respond(context.Background(), f, c, []chatto.MessageEvent{input("second", "today?", true)}, nil); err != nil {
			t.Fatal(err)
		}
	})
}
