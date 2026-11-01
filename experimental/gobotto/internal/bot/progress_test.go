package bot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

type progressClient struct {
	*fakeClient
	sent []string
	err  error
}

func (f *progressClient) Reply(_ context.Context, room, root, target, body, _ string) error {
	if room != "room" || root != "root" || target != "trigger" {
		panic("progress escaped target thread")
	}
	f.sent = append(f.sent, body)
	return f.err
}

func TestProgressThrottleCancellationAndUncertainDelivery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &conversation{key: threadKey{"room", "root"}}
		f := &progressClient{fakeClient: newFake()}
		p := newProgress(c, f, []chatto.MessageEvent{{Message: chatto.Message{ID: "trigger"}}})
		tool := p.tool()
		out, _ := tool.Execute(context.Background(), json.RawMessage(`{"text":"I'll check the relevant code."}`))
		if !strings.Contains(out, `"delivered":true`) || len(f.sent) != 1 {
			t.Fatal(out, f.sent)
		}
		tool.Execute(context.Background(), json.RawMessage(`{"text":"Again"}`))
		if len(f.sent) != 1 {
			t.Fatal("update not throttled")
		}
		time.Sleep(time.Minute)
		p.send(context.Background(), "Still investigating; I found a candidate.")
		if len(f.sent) != 2 {
			t.Fatal("missing activity update")
		}
		time.Sleep(time.Minute)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		p.send(ctx, "Must not send")
		if len(f.sent) != 2 {
			t.Fatal("sent after cancellation")
		}
		f.err = errors.New("uncertain")
		out = p.send(context.Background(), "New finding")
		if !strings.Contains(out, "uncertain") || len(f.sent) != 3 {
			t.Fatal(out)
		}
		time.Sleep(time.Minute)
		p.send(context.Background(), "Retry")
		if len(f.sent) != 3 {
			t.Fatal("retried uncertain progress")
		}
	})
}
