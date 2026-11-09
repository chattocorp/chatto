// Package testmeans provides a file-backed means command fake for isolated
// journal tests. The installed CLI has separate integration coverage.
package testmeans

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Command implements show, append and update with optimistic content hashes.
func Command(_ context.Context, input []byte, args ...string) ([]byte, error) {
	if len(args) < 5 {
		return nil, errors.New("bad test command")
	}
	files, err := filepath.Glob(filepath.Join(args[1], "*.md"))
	if err != nil {
		return nil, err
	}
	id := args[len(args)-1]
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		parts := strings.SplitN(string(b), "\n---\n", 2)
		if len(parts) != 2 {
			continue
		}
		var fm struct{ ID string }
		if json.Unmarshal([]byte(strings.TrimPrefix(parts[0], "---\n")), &fm) != nil || fm.ID != id {
			continue
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(b))
		body := parts[1]
		if args[3] != "show" {
			var patch struct{ Hash, Body string }
			if json.Unmarshal(input, &patch) != nil || patch.Hash != hash {
				return nil, errors.New("stale test hash")
			}
			switch args[3] {
			case "append":
				body += patch.Body
			case "update":
				body = patch.Body
			default:
				return nil, errors.New("unexpected test command")
			}
			b = []byte(parts[0] + "\n---\n" + body)
			if err := os.WriteFile(path, b, 0600); err != nil {
				return nil, err
			}
			hash = fmt.Sprintf("%x", sha256.Sum256(b))
		}
		return json.Marshal(map[string]any{"id": id, "hash": hash, "body": body, "frontmatter": json.RawMessage(strings.TrimPrefix(parts[0], "---\n"))})
	}
	return nil, errors.New("missing test document")
}
