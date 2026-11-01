package subagent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"strings"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
)

// instructions belongs to one child conversation. It discloses root-to-leaf
// guidance once per content version, without changing earlier model messages.
// Instruction reads use the same rooted filesystem and exclusions as source reads.
type instructions struct {
	files files
	seen  map[string][32]byte
}

type repositoryInstruction struct {
	Path  string `json:"path"`
	Scope string `json:"scope"`
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

func (i *instructions) load(dir string) []repositoryInstruction {
	if !allowed(dir) {
		return nil
	}
	dirs := []string{"."}
	if dir != "." {
		current := ""
		for _, part := range strings.Split(dir, "/") {
			current = path.Join(current, part)
			dirs = append(dirs, current)
		}
	}
	var out []repositoryInstruction
	bytes := 0
	for _, scope := range dirs {
		name := path.Join(scope, "AGENTS.md")
		entry := repositoryInstruction{Path: name, Scope: scope}
		info, err := i.files.root.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			if _, existed := i.seen[name]; !existed {
				continue
			}
			entry.Error = "Instruction file no longer exists; previous guidance from this file no longer applies."
		} else if err != nil || info.Size() > 128<<10 {
			entry.Error = "Instruction file unavailable or exceeds 128 KiB; guidance was not loaded."
		} else if data, readErr := i.files.read(name); readErr != nil || len(data) > 128<<10 {
			entry.Error = "Instruction file unavailable, excluded, or exceeds 128 KiB; guidance was not loaded."
		} else {
			entry.Text = string(data)
		}
		hash := sha256.Sum256([]byte(entry.Text + "\x00" + entry.Error))
		if previous, ok := i.seen[name]; ok && previous == hash {
			continue
		}
		bytes += len(entry.Text)
		if bytes > 256<<10 {
			out = append(out, repositoryInstruction{Path: name, Scope: scope, Error: "Instruction disclosure exceeds 256 KiB; read applicable AGENTS.md files before drawing conclusions."})
			break
		}
		i.seen[name] = hash
		out = append(out, entry)
	}
	return out
}

// tools attaches scoped guidance only to successful file operations. Search
// does not load every matched subtree: the investigator must read matched files
// before relying on snippets, which loads each file's applicable instructions.
func (i *instructions) tools() []agent.Tool {
	tools := i.files.tools()
	for n := range tools {
		execute := tools[n].Execute
		tools[n].Execute = func(ctx context.Context, raw json.RawMessage) (string, error) {
			out, err := execute(ctx, raw)
			if err != nil {
				return out, err
			}
			var payload map[string]any
			if json.Unmarshal([]byte(out), &payload) != nil || payload["status"] == "rejected" {
				return out, nil
			}
			var args fileArgs
			if json.Unmarshal(raw, &args) != nil {
				return out, nil
			}
			info, err := i.files.root.Stat(args.Path)
			if err != nil {
				return out, nil
			}
			dir := args.Path
			if !info.IsDir() {
				dir = path.Dir(dir)
			}
			if guidance := i.load(dir); len(guidance) > 0 {
				payload["repository_instructions"] = guidance
				return result(payload), nil
			}
			return out, nil
		}
	}
	return tools
}
