// Package subagent runs configured read-only workers with isolated context.
package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
)

const maxFileBytes = 1 << 20

// files never opens an agent path outside root. os.Root enforces containment at
// open time, including concurrent symlink changes. Hidden/runtime paths and
// symlinks are additionally excluded from ordinary repository investigation.
type files struct{ root *os.Root }

func allowed(name string) bool {
	if !fs.ValidPath(name) || strings.Contains(name, "\\") {
		return false
	}
	if name == "." {
		return true
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
		switch part {
		case "node_modules", "vendor", "brains":
			return false
		}
	}
	return true
}

func (f files) check(name string) error {
	if !allowed(name) {
		return errors.New("path not allowed")
	}
	current := ""
	for _, part := range strings.Split(name, "/") {
		current = path.Join(current, part)
		info, err := f.root.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("path unavailable or symlink")
		}
	}
	return nil
}

func (f files) read(name string) ([]byte, error) {
	if err := f.check(name); err != nil {
		return nil, err
	}
	info, err := f.root.Stat(name)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if info.Size() > maxFileBytes {
		return nil, errors.New("file larger than 1 MiB")
	}
	file, err := f.root.Open(name)
	if err != nil {
		return nil, errors.New("cannot open file")
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil || len(data) > maxFileBytes || !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return data, errors.New("file unreadable, binary, or larger than 1 MiB")
	}
	return data, nil
}

func result(value any) string { data, _ := json.Marshal(value); return string(data) }
func rejected(message string) (string, error) {
	return result(map[string]any{"status": "rejected", "error": message}), nil
}

type fileArgs struct {
	Path  string `json:"path"`
	Start int    `json:"start_line,omitempty"`
	Query string `json:"query,omitempty"`
}

func (f files) tools() []agent.Tool {
	var tools []agent.Tool
	for _, name := range []string{"list_files", "read_file", "search_files"} {
		properties := map[string]any{"path": map[string]any{"type": "string", "description": "Relative path within dir; use . for the root"}}
		required := []string{"path"}
		description := "List immediate entries in a directory (up to 200). Hidden/runtime paths and symlinks are omitted."
		if name == "read_file" {
			description = "Read up to 200 numbered lines of a UTF-8 file, at most 1 MiB. Results are bounded; use start_line to continue."
			properties["start_line"] = map[string]any{"type": "integer", "minimum": 1}
		}
		if name == "search_files" {
			description = "Search a file or subtree for literal, case-sensitive text. Returns numbered matches with paths. Bounded scans report truncation."
			properties["query"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 500}
			required = append(required, "query")
		}
		tools = append(tools, agent.Tool{Name: name, Description: description, Parameters: map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}, Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args fileArgs
			d := json.NewDecoder(strings.NewReader(string(raw)))
			d.DisallowUnknownFields()
			if d.Decode(&args) != nil || d.Decode(new(any)) != io.EOF || args.Path == "" || args.Start < 0 || (name != "read_file" && args.Start != 0) || (name != "search_files" && args.Query != "") {
				return rejected("Invalid file arguments")
			}
			if err := ctx.Err(); err != nil {
				return rejected("Investigation cancelled")
			}
			if f.check(args.Path) != nil {
				return rejected("Path unavailable or excluded; use a relative path within dir without symlinks")
			}
			switch name {
			case "list_files":
				return f.list(args.Path)
			case "read_file":
				return f.lines(args)
			default:
				return f.search(ctx, args)
			}
		}})
		tools[len(tools)-1].ReadOnly = true
	}
	return tools
}

func (f files) list(name string) (string, error) {
	info, err := f.root.Stat(name)
	if err != nil || !info.IsDir() {
		return rejected("Not a directory")
	}
	dir, err := f.root.Open(name)
	if err != nil {
		return rejected("Cannot list directory")
	}
	defer dir.Close()
	entries, err := dir.ReadDir(1000)
	if err != nil && err != io.EOF {
		return rejected("Cannot list directory")
	}
	names := []string{}
	for _, entry := range entries {
		if !allowed(path.Join(name, entry.Name())) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		label := entry.Name()
		if entry.IsDir() {
			label += "/"
		}
		names = append(names, label)
		if len(names) == 200 {
			break
		}
	}
	return result(map[string]any{"entries": names, "truncated": len(entries) == 1000 || len(names) == 200}), nil
}

func (f files) lines(args fileArgs) (string, error) {
	data, err := f.read(args.Path)
	if err != nil {
		return rejected(err.Error())
	}
	lines := strings.Split(string(data), "\n")
	start := max(1, args.Start)
	var out strings.Builder
	next := start
	for ; next <= len(lines) && next < start+200; next++ {
		line := fmt.Sprintf("%d: %s\n", next, lines[next-1])
		if out.Len()+len(line) > 24000 {
			if out.Len() == 0 {
				return rejected("Line exceeds read limit")
			}
			break
		}
		out.WriteString(line)
	}
	return result(map[string]any{"path": args.Path, "text": out.String(), "next_line": next, "truncated": next <= len(lines)}), nil
}

func (f files) search(ctx context.Context, args fileArgs) (string, error) {
	if args.Query == "" || len(args.Query) > 500 {
		return rejected("Provide a literal query of 1–500 bytes")
	}
	matches := []string{}
	visited, bytesRead, outputBytes, skipped := 0, 0, 0, 0
	truncated := false
	err := fs.WalkDir(f.root.FS(), args.Path, func(name string, entry fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		visited++
		if visited > 5000 || bytesRead >= 8<<20 || len(matches) >= 100 || outputBytes >= 24000 {
			truncated = true
			return fs.SkipAll
		}
		if err != nil {
			skipped++
			return nil
		}
		if !allowed(name) || entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		data, err := f.read(name)
		bytesRead += len(data)
		if err != nil {
			skipped++
			return nil
		}
		for line, text := range strings.Split(string(data), "\n") {
			if strings.Contains(text, args.Query) {
				if len(text) > 1000 {
					text = string([]rune(text)[:min(250, len([]rune(text)))]) + "…"
				}
				match := fmt.Sprintf("%s:%d: %s", name, line+1, text)
				matches = append(matches, match)
				outputBytes += len(match)
				if len(matches) >= 100 || outputBytes >= 24000 {
					truncated = true
					return fs.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		return rejected("Search interrupted or unavailable")
	}
	return result(map[string]any{"matches": matches, "truncated": truncated, "skipped_files": skipped}), nil
}
