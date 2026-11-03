// Package brainview provides a read-only, offline collection browser for the
// local operator. It is never exposed as an agent tool or added to model context.
package brainview

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/hmans/means/collection"
	"go.yaml.in/yaml/v3"
)

// PageSize bounds each visible collection page after sorting by modification time.
const PageSize = 50
const maxText = 256 << 10

// Brain identifies a configured or managed collection without exposing paths.
type Brain struct{ Key, Name, Kind string }

// Entry is a lightweight list row; bodies are loaded only after selection.
type Entry struct{ ID, Title string }

// Document contains bounded text for local inspection, never agent context.
type Document struct{ Title, Body, Metadata string }

// Page includes collection instructions and schema even when there are no documents.
type Page struct {
	Definition Document
	Entries    []Entry
	More       bool
	Skipped    int
}

// Source returns detached snapshots and never changes collections or calls a
// provider. Implementations must be safe for reads alongside the bot's writes.
type Source interface {
	Catalog(context.Context) ([]Brain, error)
	List(context.Context, string, int) (Page, error)
	Read(context.Context, string, string) (Document, error)
}

// Browser uses immutable operator settings, not the means CLI configuration.
type Browser struct {
	root  string
	fixed []config.Collection
}

// New copies collection bindings without opening files or enabling embeddings.
func New(settings config.Memory) *Browser {
	return &Browser{root: settings.Directory, fixed: append([]config.Collection(nil), settings.Collections...)}
}

var appName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var errRead = errors.New("Unable to read brain; refresh to retry")

func (b *Browser) bindings(ctx context.Context) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	paths := map[string]string{}
	if b.root != "" {
		for _, name := range []string{"memory", "conversations", "schedules"} {
			paths[name] = filepath.Join(b.root, name)
		}
		entries, err := os.ReadDir(filepath.Join(b.root, "apps"))
		if err != nil && !os.IsNotExist(err) {
			return nil, errRead
		}
		for _, entry := range entries {
			if !entry.IsDir() || !appName.MatchString(entry.Name()) {
				continue
			}
			path := filepath.Join(b.root, "apps", entry.Name())
			info, err := os.Lstat(filepath.Join(path, "MEANS.md"))
			if err == nil && info.Mode().IsRegular() {
				paths["apps/"+entry.Name()] = path
			}
		}
	}
	for _, c := range b.fixed {
		key := "apps/" + c.Name
		if _, duplicate := paths[key]; duplicate {
			return nil, errRead
		}
		paths[key] = c.Path
	}
	if len(paths) > 256 {
		return nil, errors.New("Brain catalog exceeds 256 collections")
	}
	return paths, nil
}

func (b *Browser) Catalog(ctx context.Context) ([]Brain, error) {
	paths, err := b.bindings(ctx)
	if err != nil {
		return nil, err
	}
	var out []Brain
	for _, key := range []string{"memory", "conversations", "schedules"} {
		if _, ok := paths[key]; ok {
			out = append(out, Brain{key, strings.ToUpper(key[:1]) + key[1:], "Built-in"})
		}
	}
	var keys []string
	for key := range paths {
		if strings.HasPrefix(key, "apps/") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, Brain{key, strings.TrimPrefix(key, "apps/"), "App"})
	}
	return out, nil
}

func (b *Browser) load(ctx context.Context, key string) (*collection.Collection, error) {
	paths, err := b.bindings(ctx)
	if err != nil {
		return nil, err
	}
	path, ok := paths[key]
	if !ok {
		return nil, errRead
	}
	c, err := collection.Load(path)
	if err != nil {
		return nil, errRead
	}
	return c.WithEmbeddings(nil), nil
}

func bounded(text string) string {
	if len(text) <= maxText {
		return text
	}
	// Preserve UTF-8 when cutting a large private payload.
	end := maxText
	for end > 0 && text[end]&0xc0 == 0x80 {
		end--
	}
	return text[:end] + "\n[display truncated at 256 KiB]"
}

func titleLabel(text string) string {
	const limit = 160
	for i, count := 0, 0; i < len(text); {
		if count == limit {
			return text[:i] + "…"
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
		count++
	}
	return text
}

func (b *Browser) List(ctx context.Context, key string, offset int) (Page, error) {
	if offset < 0 {
		return Page{}, errRead
	}
	c, err := b.load(ctx, key)
	if err != nil {
		return Page{}, err
	}
	result, err := c.List(ctx, collection.ListOptions{})
	if err != nil {
		return Page{}, errRead
	}
	times, err := modificationTimes(ctx, c.Path())
	if err != nil {
		return Page{}, err
	}
	sort.Slice(result.Documents, func(i, j int) bool {
		a, b := result.Documents[i].ID, result.Documents[j].ID
		if times[a].Equal(times[b]) {
			return a < b
		}
		return times[a].After(times[b])
	})
	start := min(offset, len(result.Documents))
	end := start + min(PageSize, len(result.Documents)-start)
	result.HasMore = end < len(result.Documents)
	result.Documents = result.Documents[start:end]
	d := c.Describe()
	metadata, err := metadataYAML(map[string]any{"description": d.Description, "schema": json.RawMessage(c.Schema())})
	if err != nil {
		return Page{}, errRead
	}
	page := Page{Definition: Document{Title: titleLabel(d.Title), Body: bounded(d.Instructions), Metadata: metadata}, More: result.HasMore, Skipped: len(result.Diagnostics)}
	for _, doc := range result.Documents {
		title, _ := doc.Frontmatter["title"].(string)
		if title == "" {
			title = doc.ID
		}
		if key == "conversations" {
			// Journals share a generic title; keep their existing short stored ID
			// visible so the operator can distinguish room and thread notebooks.
			kind := "Room"
			if data, ok := doc.Frontmatter["data"].(map[string]any); ok {
				if scope, ok := data["scope"].(map[string]any); ok {
					if thread, _ := scope["thread"].(string); thread != "" {
						kind = "Thread"
					}
				}
			}
			title = kind + " · " + doc.ID
		}
		page.Entries = append(page.Entries, Entry{doc.ID, titleLabel(title)})
	}
	return page, nil
}

func (b *Browser) Read(ctx context.Context, key, id string) (Document, error) {
	c, err := b.load(ctx, key)
	if err != nil {
		return Document{}, err
	}
	doc, err := c.Read(ctx, id)
	if err != nil {
		return Document{}, errRead
	}
	title, _ := doc.Frontmatter["title"].(string)
	if title == "" {
		title = doc.ID
	}
	metadata, err := metadataYAML(doc.Frontmatter)
	if err != nil {
		return Document{}, errRead
	}
	return Document{titleLabel(title), bounded(doc.Body), metadata}, nil
}

// metadataYAML converts JSON-compatible values through YAML nodes to retain
// exact json.Number values and scalar types without a float64 round trip.
func metadataYAML(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return "", err
	}
	var blockStyle func(*yaml.Node)
	blockStyle = func(n *yaml.Node) {
		n.Style = 0
		for _, child := range n.Content {
			blockStyle(child)
		}
	}
	blockStyle(&node)
	var out strings.Builder
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&node); err != nil {
		return "", err
	}
	return bounded(out.String()), nil
}
