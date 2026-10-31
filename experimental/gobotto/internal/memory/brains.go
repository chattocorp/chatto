package memory

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
)

var brainName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// brainArguments separates model-authored content from the MEANS.md envelope.
// JSON is valid YAML, so marshaling the frontmatter also escapes multiline text
// and delimiter-like content without another serializer dependency.
type brainArguments struct {
	Name         string          `json:"name"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Schema       json.RawMessage `json:"schema"`
	Instructions string          `json:"instructions"`
}

func (a brainArguments) definition() ([]byte, error) {
	var schema any
	if strings.TrimSpace(a.Title) == "" || strings.TrimSpace(a.Description) == "" || strings.TrimSpace(a.Instructions) == "" {
		return nil, errors.New("title, description, and instructions must be nonempty strings")
	}
	if json.Unmarshal(a.Schema, &schema) != nil {
		return nil, errors.New("schema must be a JSON Schema object or boolean")
	}
	switch schema.(type) {
	case map[string]any, bool:
	default:
		return nil, errors.New("schema must be a JSON Schema object or boolean")
	}
	header, err := json.Marshal(map[string]any{"title": a.Title, "description": a.Description, "schema": a.Schema})
	if err != nil {
		return nil, errors.New("invalid brain metadata")
	}
	return []byte("---\n" + string(header) + "\n---\n" + a.Instructions + "\n"), nil
}

// creationRejected is used only when no collection files were created. This is
// safe for the agent to correct; uncertain mutations must return an error instead.
func creationRejected(reason string) (string, error) {
	data, _ := json.Marshal(map[string]any{"status": "rejected", "error": reason, "retryable": true, "mutated": false})
	return string(data), nil
}

// bindings discovers immediate child collections on every call. Hidden staging
// and archive directories, symlinks, and incomplete collections are not exposed.
func (m *Manager) bindings() ([]config.Collection, error) {
	bindings := append([]config.Collection(nil), m.collections...)
	if m.directory == "" {
		return bindings, nil
	}
	entries, err := os.ReadDir(m.directory)
	if err != nil {
		return nil, errors.New("cannot discover brains")
	}
	names := map[string]bool{}
	for _, c := range bindings {
		names[c.Name] = true
	}
	for _, entry := range entries {
		if !entry.IsDir() || !brainName.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(m.directory, entry.Name())
		info, err := os.Lstat(filepath.Join(path, "MEANS.md"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return nil, errors.New("invalid brain configuration")
		}
		if names[entry.Name()] {
			return nil, errors.New("duplicate brain binding name")
		}
		bindings = append(bindings, config.Collection{Name: entry.Name(), Path: path, Writable: true})
	}
	return bindings, nil
}

// Catalog supplies current collection metadata to every turn, including fresh
// and restored sessions. Full guidance is loaded separately with memory_info.
func (m *Manager) Catalog(ctx context.Context) (string, error) {
	if m == nil {
		return "[]", nil
	}
	bindings, err := m.bindings()
	if err != nil {
		return "", err
	}
	type entry struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Writable    bool   `json:"writable"`
	}
	entries := []entry{}
	for _, c := range bindings {
		out, err := m.run(ctx, c.Path, []string{"--path", c.Path, "--json", "info"}, nil)
		if err != nil {
			return "", err
		}
		var metadata entry
		if json.Unmarshal(out, &metadata) != nil {
			return "", errors.New("invalid collection metadata")
		}
		metadata.Name, metadata.Writable = c.Name, c.Writable
		entries = append(entries, metadata)
	}
	data, _ := json.Marshal(entries)
	if len(data) > resultLimit {
		return "", errors.New("collection catalog exceeds limit")
	}
	return string(data), nil
}

// lifecycleTools exposes owner-only creation and reversible destruction. The
// manager is used by Gobotto's single turn worker; receipts deduplicate retries
// within that turn. No tool accepts filesystem paths or deletes directory trees.
func (m *Manager) lifecycleTools(ownerOnly bool, uncertain *bool, changed func()) []agent.Tool {
	if !ownerOnly || m.directory == "" {
		return nil
	}
	receipts := map[string]string{}
	touched := map[string]bool{}
	var result []agent.Tool
	for _, op := range []string{"create", "archive"} {
		properties := map[string]any{"name": map[string]any{"type": "string", "description": "Brain name: lowercase letter followed by letters, digits, underscores or hyphens; up to 64 characters"}}
		required := []string{"name"}
		description := "Archive a managed brain on an explicit owner request. Removes it from discovery and use, retaining its files for manual recovery. Fixed bindings cannot be archived."
		if op == "create" {
			properties["title"] = map[string]any{"type": "string", "description": "Human-readable brain title"}
			properties["description"] = map[string]any{"type": "string", "description": "Short purpose summary included in every session's catalog"}
			properties["schema"] = map[string]any{"anyOf": []any{map[string]any{"type": "object"}, map[string]any{"type": "boolean"}}, "description": "JSON Schema for document data objects, not the document envelope. External references are not supported."}
			properties["instructions"] = map[string]any{"type": "string", "description": "Markdown guidance defining purpose, read/write audience, and workflow. Supply only the body; Gobotto generates MEANS.md frontmatter."}
			required = append(required, "title", "description", "schema", "instructions")
			description = "Create a means brain on an explicit owner request. Define its schema and prompt rules for who may read/write and when. It becomes discoverable immediately, including in future sessions."
		}
		result = append(result, agent.Tool{Name: "memory_brain_" + op, Description: description, Parameters: map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}, Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
			if !ownerOnly {
				return feedback("Only owner-only batches may manage brains")
			}
			if len(raw) > 32<<10 {
				return feedback("Brain arguments exceed 32 KiB")
			}
			var a brainArguments
			var fields map[string]json.RawMessage
			if json.Unmarshal(raw, &fields) != nil || fields == nil {
				return creationRejected("Expected a JSON object; correct the arguments")
			}
			for key := range fields {
				if _, ok := properties[key]; !ok {
					return creationRejected("Unexpected argument; use the fields declared by this tool")
				}
			}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&a) != nil || !brainName.MatchString(a.Name) {
				return creationRejected("Invalid argument types or brain name; correct the arguments")
			}
			if decoder.Decode(new(any)) != io.EOF {
				return feedback("Expected one JSON object")
			}
			var definition []byte
			if op == "create" {
				var err error
				definition, err = a.definition()
				if err != nil {
					return creationRejected(err.Error())
				}
			}
			canonical, _ := json.Marshal(a)
			key := op + string(canonical)
			if receipt, ok := receipts[key]; ok {
				return receipt, nil
			}
			if touched[a.Name] {
				return feedback("This brain already had a lifecycle change this turn; use a new request for another change")
			}
			if *uncertain {
				return feedback("Previous memory operation uncertain; inspect the catalog before a new request")
			}
			for _, c := range m.collections {
				if c.Name == a.Name {
					return feedback("Name belongs to a fixed collection")
				}
			}
			path := filepath.Join(m.directory, a.Name)
			for _, c := range m.collections {
				resolved, err := filepath.EvalSymlinks(c.Path)
				if err == nil && resolved == path {
					return feedback("Brain path belongs to a fixed collection")
				}
			}
			if op == "create" {
				// Reserve exclusively: never overwrite an existing collection or link.
				if err := os.Mkdir(path, 0700); err != nil {
					return feedback("Brain name already exists or cannot be created")
				}
				_, err := m.run(ctx, path, []string{"--path", path, "--json", "init", "--input", "-"}, definition)
				if err != nil {
					// A successful empty-directory removal proves there are no created
					// collection files. Partial writes remain uncertain and cannot retry.
					if os.Remove(path) == nil {
						return creationRejected("Means could not initialize the brain and created no collection files. Check schema keyword types and references; correct invalid input before retrying.")
					}
					*uncertain = true
					return "", errors.New("brain creation failed or is uncertain")
				}
			} else {
				info, err := os.Lstat(path)
				if err != nil || !info.IsDir() {
					return feedback("Managed brain does not exist")
				}
				configInfo, err := os.Lstat(filepath.Join(path, "MEANS.md"))
				if err != nil || !configInfo.Mode().IsRegular() {
					return feedback("Not a valid managed brain")
				}
				archive := filepath.Join(m.directory, ".archive")
				if err := os.Mkdir(archive, 0700); err != nil && !os.IsExist(err) {
					return "", errors.New("cannot create archive")
				}
				info, err = os.Lstat(archive)
				if err != nil || !info.IsDir() {
					return "", errors.New("invalid archive directory")
				}
				if ctx.Err() != nil {
					return "", ctx.Err()
				}
				if os.Rename(path, filepath.Join(archive, a.Name+"-"+rand.Text())) != nil {
					*uncertain = true
					return "", errors.New("brain archive failed")
				}
			}
			changed()
			touched[a.Name] = true
			out, _ := json.Marshal(map[string]string{"name": a.Name, "operation": op, "status": "complete"})
			receipts[key] = string(out)
			return string(out), nil
		}})
	}
	return result
}
