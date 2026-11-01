package subagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
)

func TestInstructionScopeAndRefresh(t *testing.T) {
	f, dir := fixture(t)
	for _, sub := range []string{"one/deep", "two"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, text := range map[string]string{
		"AGENTS.md": "root guidance", "one/AGENTS.md": "one guidance",
		"one/deep/AGENTS.md": "deep guidance", "two/AGENTS.md": "two guidance",
		"one/deep/code.go": "package example", "two/code.go": "package other",
	} {
		write(t, filepath.Join(dir, name), text)
	}
	i := &instructions{files: f, seen: map[string][32]byte{}}
	root := i.load(".")
	if len(root) != 1 || root[0].Text != "root guidance" {
		t.Fatal(root)
	}
	read := i.tools()[1]
	out, err := read.Execute(context.Background(), json.RawMessage(`{"path":"one/deep/code.go"}`))
	if err != nil || !strings.Contains(out, "one guidance") || !strings.Contains(out, "deep guidance") || strings.Contains(out, "root guidance") || strings.Contains(out, "two guidance") {
		t.Fatal(out, err)
	}
	out, _ = read.Execute(context.Background(), json.RawMessage(`{"path":"one/deep/code.go"}`))
	if strings.Contains(out, "repository_instructions") {
		t.Fatal("unchanged guidance repeated", out)
	}
	write(t, filepath.Join(dir, "one", "AGENTS.md"), "updated guidance")
	out, _ = read.Execute(context.Background(), json.RawMessage(`{"path":"one/deep/code.go"}`))
	if !strings.Contains(out, "updated guidance") {
		t.Fatal(out)
	}
	out, _ = read.Execute(context.Background(), json.RawMessage(`{"path":"two/code.go"}`))
	if !strings.Contains(out, "two guidance") || strings.Contains(out, "updated guidance") {
		t.Fatal("sibling scope leaked", out)
	}
	if err := os.Remove(filepath.Join(dir, "one", "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	removed := i.load("one")
	if len(removed) != 1 || !strings.Contains(removed[0].Error, "no longer exists") {
		t.Fatal(removed)
	}
}

func TestInstructionsRespectReadBoundary(t *testing.T) {
	f, dir := fixture(t)
	outside := filepath.Join(t.TempDir(), "AGENTS.md")
	write(t, outside, "outside secret")
	if err := os.Symlink(outside, filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	i := &instructions{files: f, seen: map[string][32]byte{}}
	got := i.load(".")
	if len(got) != 1 || got[0].Error == "" || got[0].Text != "" {
		t.Fatal("symlink accepted", got)
	}
	if got := i.load(".."); len(got) != 0 {
		t.Fatal("parent traversal accepted", got)
	}
	if err := os.Remove(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "AGENTS.md"), strings.Repeat("x", (128<<10)+1))
	got = i.load(".")
	if len(got) != 1 || got[0].Error == "" || got[0].Text != "" {
		t.Fatal("oversized guidance accepted", got)
	}
}

func TestInstructionsAcceptLargeRepositoryGuide(t *testing.T) {
	f, dir := fixture(t)
	text := strings.Repeat("Repository guidance.\n", 2000)
	write(t, filepath.Join(dir, "AGENTS.md"), text)
	i := &instructions{files: f, seen: map[string][32]byte{}}
	got := i.load(".")
	if len(got) != 1 || got[0].Error != "" || got[0].Text != text {
		t.Fatal("guide above previous 32 KiB limit was not loaded")
	}
}

func TestDelegationInstructionOptOut(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "AGENTS.md"), "ROOT_SENTINEL")
		cfg := config.Subagent{Dir: dir}
		if !enabled {
			cfg.AgentsMD = &enabled
		}
		m, err := Open(map[string]config.Subagent{"code": cfg}, func(_, _ string, _ func(agent.Usage)) agent.Complete {
			return func(_ context.Context, messages []agent.Message) (agent.Message, error) {
				encoded, _ := json.Marshal(messages)
				if strings.Contains(string(encoded), "ROOT_SENTINEL") != enabled {
					t.Fatal("root guidance enable/disable mismatch")
				}
				return message(t, `{"role":"assistant","content":"Done"}`), nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		out, err := m.Tools(0, nil, nil)[0].Execute(context.Background(), json.RawMessage(`{"agent":"code","task":"Investigate"}`))
		m.Close()
		if err != nil || !strings.Contains(out, `"status":"complete"`) {
			t.Fatal(out, err)
		}
	}
}
