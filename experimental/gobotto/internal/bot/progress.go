package bot

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

// progress has the same single owner as the conversation. Notifications are
// synchronous, thread-bound, checkpointed before sending, and never retried on
// uncertain delivery. No background timer can post after cancellation.
type progress struct {
	c         *conversation
	client    Client
	target    string
	last      time.Time
	uncertain bool
}

func newProgress(c *conversation, client Client, batch []chatto.MessageEvent) *progress {
	p := &progress{c: c, client: client}
	if len(batch) > 0 {
		p.target = batch[len(batch)-1].Message.ID
	}
	return p
}

func (p *progress) send(ctx context.Context, body string) string {
	if p.uncertain {
		return `{"status":"rejected","error":"Earlier progress delivery was uncertain; do not retry it."}`
	}
	if !p.last.IsZero() && time.Since(p.last) < time.Minute {
		return `{"status":"rejected","error":"A progress update was posted recently. Continue work; updates are limited to once per minute."}`
	}
	p.c.editCheck.RLock()
	defer p.c.editCheck.RUnlock()
	if ctx.Err() != nil || p.target == "" {
		return `{"status":"rejected","error":"Turn stopped; update was not sent."}`
	}
	index := p.c.step("progress", "Progress update")
	if err := p.c.store.save(p.c, true); err != nil {
		p.c.storeErr = err
		p.c.finishStep(index, "failed", "Checkpoint failed; update was not sent")
		return `{"status":"failed","error":"Checkpoint failed; update was not sent."}`
	}
	p.last = time.Now()
	err := p.client.Reply(ctx, p.c.key.room, p.c.key.root, p.target, body, "")
	if err != nil {
		p.uncertain = true
		p.c.finishStep(index, "uncertain", "Progress delivery failed or is uncertain; no automatic retry")
	} else {
		p.c.finishStep(index, "complete", body)
	}
	if saveErr := p.c.store.save(p.c, true); saveErr != nil {
		p.c.storeErr = saveErr
	}
	if err != nil {
		return `{"status":"failed","error":"Progress delivery failed or is uncertain. Do not retry this message."}`
	}
	return `{"status":"complete","delivered":true}`
}

func (p *progress) tool() agent.Tool {
	return agent.Tool{Name: "post_update", Description: "Post a brief announcement or meaningful progress update in the current thread without ending your turn. Use before substantial work. At most one update per minute; do not retry uncertain delivery. This is not your final answer.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string", "minLength": 1, "maxLength": 1000}}, "required": []string{"text"}, "additionalProperties": false}, Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
		var args struct {
			Text string `json:"text"`
		}
		d := json.NewDecoder(strings.NewReader(string(raw)))
		d.DisallowUnknownFields()
		if d.Decode(&args) != nil || d.Decode(new(any)) != io.EOF || strings.TrimSpace(args.Text) == "" || len([]rune(args.Text)) > 1000 {
			return `{"status":"rejected","error":"Provide a brief text update of 1–1000 characters."}`, nil
		}
		return p.send(ctx, args.Text), nil
	}}
}
