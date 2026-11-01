package bot

import (
	"context"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/subagent"
)

func TestChildUsageRecoveredOnceFromIndependentCheckpoint(t *testing.T) {
	s, err := OpenStore(t.TempDir(), "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := &conversation{id: 1, key: threadKey{"room", "root"}, provider: "provider", store: s}
	if err := s.save(c, false); err != nil {
		t.Fatal(err)
	}
	cost := 0.02
	r := &subagent.Run{ID: "child", Status: "running", Usage: []subagent.RunUsage{{Turn: 1, Usage: agent.Usage{Input: 100, Output: 20, Cost: &cost}}}}
	if err := s.saveChildren(c.humanID, map[string]*subagent.Run{r.ID: r}); err != nil {
		t.Fatal(err)
	}
	applied := 0
	for range 2 {
		loaded, err := s.load()
		if err != nil {
			t.Fatal(err)
		}
		c = loaded[0]
		c.store = s
		c.jobs = subagent.NewBackground(context.Background(), nil, c.childRuns, func(runs map[string]*subagent.Run) error { return s.saveChildren(c.humanID, runs) }, nil)
		c.accountUsage = func(u agent.Usage) {
			applied++
			c.usage.Input += u.Input
			c.usage.Output += u.Output
			c.usage.Cost += *u.Cost
			c.storeErr = s.save(c, false)
		}
		if err := c.syncChildren(); err != nil {
			t.Fatal(err)
		}
		c.jobs.Close()
		if applied != 1 || c.usage.Input != 100 || c.usage.Cost != cost {
			t.Fatal("lost or duplicate usage", applied, c.usage)
		}
	}
}
