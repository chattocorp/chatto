package subagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
)

func TestPublicBufferIsBoundedAndDetached(t *testing.T) {
	r := &Run{ID: "child", Status: "running"}
	for n := 0; n < 100; n++ {
		r.note("assistant", strings.Repeat("界", 3000))
	}
	bytes := 0
	for _, note := range r.Updates {
		bytes += len(note.Text)
	}
	if len(r.Updates) > 32 || bytes > 64<<10 {
		t.Fatal(len(r.Updates), bytes)
	}
	b := NewBackground(context.Background(), nil, map[string]*Run{r.ID: r}, nil, nil)
	defer b.Close()
	view := b.Context()
	notes := view[0]["recent_updates"].([]UpdateNote)
	if len(notes) > 4 {
		t.Fatal("too much default disclosure")
	}
	notes[0].Text = "changed snapshot"
	if b.Context()[0]["recent_updates"].([]UpdateNote)[0].Text == "changed snapshot" {
		t.Fatal("snapshot aliases live buffer")
	}
}

func TestBackgroundOutlivesTurnAndCancelsWithConversation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started, stopped := make(chan struct{}), make(chan struct{})
		m, err := Open(map[string]config.Subagent{"code": {Dir: t.TempDir()}}, func(_, _ string, _ func(agent.Usage)) agent.Complete {
			return func(ctx context.Context, _ []agent.Message) (agent.Message, error) {
				close(started)
				<-ctx.Done()
				close(stopped)
				return nil, ctx.Err()
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		b := NewBackground(context.Background(), m, nil, nil, nil)
		defer b.Close()
		turn, cancel := context.WithCancel(context.Background())
		raw := json.RawMessage(`{"agent":"code","task":"Inspect"}`)
		out, err := b.Tools(0, 1, "trigger", nil)[0].Execute(turn, raw)
		if err != nil || !strings.Contains(out, `"status":"running"`) {
			t.Fatal(out, err)
		}
		<-started
		cancel()
		synctest.Wait()
		select {
		case <-stopped:
			t.Fatal("parent turn cancellation stopped background task")
		default:
		}
		duplicate, _ := b.Tools(0, 2, "status", nil)[0].Execute(context.Background(), raw)
		if !strings.Contains(duplicate, "no duplicate") {
			t.Fatal(duplicate)
		}
		b.Close()
		<-stopped
		runs, _, done, err := b.Drain()
		if err != nil || len(done) != 1 || len(runs) != 1 {
			t.Fatal(err, len(done), len(runs))
		}
		for _, r := range runs {
			if r.Status != "partial" || r.Reason != "cancelled" {
				t.Fatal(r.Status, r.Reason)
			}
		}
	})
}
