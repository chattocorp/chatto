// Package interchange reads the portable history contract without contacting
// source platforms. Validation errors identify record positions, never content.
//
//go:generate go run ./internal/genschemas
package interchange

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
)

// Initial directory-import limits bound memory on both sides of the operator
// socket. Larger archives must fail before writes, never be truncated.
const (
	MaxArchiveBytes = 64 << 20
	MaxRecordBytes  = 1 << 20
	MaxRecords      = 20000
)

// Archive retains original metadata bytes and verified local asset bytes. Its
// digest binds a retry to the exact directory contents, independent of location.
type Archive struct {
	Metadata map[string][]byte `json:"metadata"`
	Files    map[string][]byte `json:"files"`
	Manifest Manifest          `json:"-"`
	Users    []User            `json:"-"`
	Rooms    []Room            `json:"-"`
	Assets   []Asset           `json:"-"`
	Events   []Event           `json:"-"`
}

// Manifest describes the capture and declared losses. The destination importer
// defines the trust policy for source identities and email claims.
type Manifest struct {
	ExportID    string     `json:"export_id"`
	CapturedAt  *string    `json:"captured_at"`
	Consistency string     `json:"consistency"`
	History     string     `json:"history"`
	Omissions   []Omission `json:"omissions"`
}

// Omission identifies a category of source data that the archive cannot retain.
type Omission struct {
	Category string `json:"category"`
}

// User is a source identity, not a destination account or credential.
type User struct {
	ID          string   `json:"id"`
	Username    *string  `json:"username"`
	DisplayName string   `json:"display_name"`
	Kind        string   `json:"kind"`
	Status      string   `json:"status"`
	Emails      []string `json:"emails"`
}

// Room holds authoritative final channel metadata.
type Room struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Topic      string `json:"topic"`
	Purpose    string `json:"purpose"`
	Visibility string `json:"visibility"`
	Archived   bool   `json:"archived"`
}

// Asset declares verified local bytes or an explicit unavailable file.
type Asset struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	MediaType string `json:"media_type"`
	Storage   struct {
		Status string `json:"status"`
		Path   string `json:"path"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
		Reason string `json:"reason"`
	} `json:"storage"`
}

// Content distinguishes absent readable content from a known deletion.
type Content struct {
	Status string `json:"status"`
	Body   struct {
		Format string `json:"format"`
		Text   string `json:"text"`
	} `json:"body"`
	AssetIDs []string `json:"asset_ids"`
	Reason   string   `json:"reason"`
}

// Reaction is one Unicode sequence and its final set of source reactors.
type Reaction struct {
	Emoji   string   `json:"emoji"`
	UserIDs []string `json:"user_ids"`
}

// EventData is decoded only after the discriminated schema has checked its
// exact fields. Zero values do not substitute for missing required JSON fields.
type EventData struct {
	MessageID string     `json:"message_id"`
	AuthorID  string     `json:"author_id"`
	ThreadID  *string    `json:"thread_id"`
	ReplyTo   *string    `json:"reply_to"`
	Content   Content    `json:"content"`
	CreatedAt *string    `json:"created_at"`
	EditedAt  *string    `json:"edited_at"`
	DeletedAt *string    `json:"deleted_at"`
	Reactions []Reaction `json:"reactions"`
	UserID    string     `json:"user_id"`
	UserIDs   []string   `json:"user_ids"`
	Emoji     string     `json:"emoji"`
	RootID    string     `json:"root_id"`
	Value     string     `json:"value"`
}

// Event is a historical transition or final snapshot in authoritative file order.
type Event struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	RoomID     string    `json:"room_id"`
	ActorID    *string   `json:"actor_id"`
	OccurredAt *string   `json:"occurred_at"`
	Data       EventData `json:"data"`
}

var names = map[string]string{"manifest.json": "manifest", "users.jsonl": "user", "rooms.jsonl": "room", "assets.jsonl": "asset", "events.jsonl": "event"}
var resolved = sync.OnceValues(func() (map[string]*jsonschema.Resolved, error) {
	schemas := map[string]*jsonschema.Schema{}
	for name, raw := range schemaJSON {
		// The contract uses JSON Schema's ECMA Unicode escape syntax. The Go
		// validator uses RE2, which spells the same code point as \x{FFFF}.
		var tree any
		if err := json.Unmarshal([]byte(raw), &tree); err != nil {
			return nil, err
		}
		adaptPatterns(tree)
		encoded, err := json.Marshal(tree)
		if err != nil {
			return nil, err
		}
		var s jsonschema.Schema
		if err := json.Unmarshal(encoded, &s); err != nil {
			return nil, err
		}
		schemas[name] = &s
	}
	result := map[string]*jsonschema.Resolved{}
	for name, s := range schemas {
		r, err := s.Resolve(&jsonschema.ResolveOptions{Loader: func(u *url.URL) (*jsonschema.Schema, error) {
			if u.String() != "https://chatto.chat/schema/interchange/v1/common.schema.json" {
				return nil, errors.New("unknown schema reference")
			}
			return schemas["common"], nil
		}})
		if err != nil {
			return nil, err
		}
		result[name] = r
	}
	return result, nil
})

func adaptPatterns(v any) {
	switch value := v.(type) {
	case map[string]any:
		for key, child := range value {
			if key == "pattern" {
				value[key] = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`).ReplaceAllString(child.(string), `\x{$1}`)
			} else {
				adaptPatterns(child)
			}
		}
	case []any:
		for _, child := range value {
			adaptPatterns(child)
		}
	}
}

// ReadDirectory reads a bounded immutable view. Root-relative opens prevent
// traversal outside the chosen directory; all links and special files fail.
// It never creates progress files or fetches URLs.
func ReadDirectory(dir string) (*Archive, error) {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return nil, errors.New("import: source must be a directory, not a link")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, errors.New("import: cannot open source directory")
	}
	defer root.Close()
	a := &Archive{Metadata: map[string][]byte{}, Files: map[string][]byte{}}
	var total int64
	fileCount := 0
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("import: cannot inspect source directory")
		}
		if entry.IsDir() {
			if name != "." && name != "files" {
				return errors.New("import: unexpected directory")
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.New("import: links and special files are forbidden")
		}
		fileCount++
		if fileCount > MaxRecords+len(names) {
			return errors.New("import: file count limit exceeded")
		}
		if _, ok := names[name]; !ok && (path.Dir(name) != "files" || strings.ContainsAny(path.Base(name), "\\:")) {
			return errors.New("import: unexpected file")
		}
		f, err := root.Open(name)
		if err != nil {
			return errors.New("import: cannot read source file")
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || !stat.Mode().IsRegular() {
			return errors.New("import: invalid source file")
		}
		if !singleLink(stat) {
			return errors.New("import: hard links are forbidden")
		}
		current, err := root.Lstat(name)
		if err != nil || !current.Mode().IsRegular() || !os.SameFile(stat, current) {
			return errors.New("import: source changed while reading")
		}
		data, err := io.ReadAll(io.LimitReader(f, MaxArchiveBytes-total+1))
		if err != nil {
			return errors.New("import: cannot read source file")
		}
		total += int64(len(data))
		if total > MaxArchiveBytes {
			return errors.New("import: archive exceeds 64 MiB limit")
		}
		if _, ok := names[name]; ok {
			a.Metadata[name] = data
		} else {
			a.Files[name] = data
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return a, nil
}

// Validate rechecks untrusted transport input, not only local directory reads.
// Decoded views are replaced on each call so retries cannot retain stale state.
func (a *Archive) Validate() error {
	validators, err := resolved()
	if err != nil {
		return errors.New("import: embedded schema error")
	}
	if len(a.Metadata) != len(names) {
		return errors.New("import: five metadata files are required")
	}
	a.Manifest = Manifest{}
	a.Users = nil
	a.Rooms = nil
	a.Assets = nil
	a.Events = nil
	total, count := 0, 0
	for _, data := range a.Files {
		total += len(data)
	}
	for _, data := range a.Metadata {
		total += len(data)
	}
	if total > MaxArchiveBytes {
		return errors.New("import: archive exceeds 64 MiB limit")
	}
	// Fixed order gives stable, content-free diagnostics.
	for _, name := range []string{"manifest.json", "users.jsonl", "rooms.jsonl", "assets.jsonl", "events.jsonl"} {
		raw, exists := a.Metadata[name]
		if !exists {
			return errors.New("import: metadata file missing")
		}
		lines := [][]byte{raw}
		if name != "manifest.json" {
			if len(raw) == 0 {
				continue
			}
			if raw[len(raw)-1] != '\n' {
				return fmt.Errorf("import: %s must end in a newline", name)
			}
			// Check before Split allocates one slice per line. A small file of
			// newlines must not expand into an unbounded slice allocation.
			if bytes.Count(raw, []byte{'\n'}) > MaxRecords-count {
				return errors.New("import: record limit exceeded")
			}
			lines = bytes.Split(raw[:len(raw)-1], []byte{'\n'})
		}
		for index, line := range lines {
			count++
			if count > MaxRecords {
				return errors.New("import: record limit exceeded")
			}
			if len(line) > MaxRecordBytes {
				return fmt.Errorf("import: %s record %d exceeds size limit", name, index+1)
			}
			value, err := strictJSON(line)
			if err != nil || validators[names[name]].Validate(value) != nil {
				return fmt.Errorf("import: %s record %d violates format v1", name, index+1)
			}
			switch name {
			case "manifest.json":
				err = json.Unmarshal(line, &a.Manifest)
			case "users.jsonl":
				var v User
				err = json.Unmarshal(line, &v)
				a.Users = append(a.Users, v)
			case "rooms.jsonl":
				var v Room
				err = json.Unmarshal(line, &v)
				a.Rooms = append(a.Rooms, v)
			case "assets.jsonl":
				var v Asset
				err = json.Unmarshal(line, &v)
				a.Assets = append(a.Assets, v)
			case "events.jsonl":
				var v Event
				err = json.Unmarshal(line, &v)
				a.Events = append(a.Events, v)
			}
			if err != nil {
				return fmt.Errorf("import: %s record %d has invalid values", name, index+1)
			}
		}
	}
	return a.validateReferences()
}

// Digest covers filenames and exact bytes, including asset hashes and all source
// metadata. It contains no plaintext and is independent of map iteration order.
func (a *Archive) Digest() string {
	b, _ := json.Marshal(struct{ Metadata, Files map[string][]byte }{a.Metadata, a.Files})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// strictJSON rejects duplicates and malformed Unicode before encoding/json can
// replace it. Schema errors must never echo the offending value to an operator.
func strictJSON(raw []byte) (any, error) {
	bad := errors.New("invalid JSON")
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return nil, bad
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		i++
		for ; i < len(raw) && raw[i] != '"'; i++ {
			if raw[i] != '\\' {
				continue
			}
			i++
			if raw[i] != 'u' {
				continue
			}
			n, _ := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
			i += 4
			if n >= 0xdc00 && n <= 0xdfff {
				return nil, bad
			}
			if n >= 0xd800 && n <= 0xdbff {
				if i+6 >= len(raw) || string(raw[i+1:i+3]) != "\\u" {
					return nil, bad
				}
				low, _ := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
				if low < 0xdc00 || low > 0xdfff {
					return nil, bad
				}
				i += 6
			}
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		if depth > 64 {
			return nil, bad
		}
		t, err := d.Token()
		if err != nil {
			return nil, bad
		}
		switch t {
		case json.Delim('{'):
			m := map[string]any{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return nil, bad
				}
				k, ok := key.(string)
				if !ok {
					return nil, bad
				}
				if _, ok = m[k]; ok {
					return nil, bad
				}
				v, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				m[k] = v
			}
			_, err = d.Token()
			return m, err
		case json.Delim('['):
			a := []any{}
			for d.More() {
				v, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			}
			_, err = d.Token()
			return a, err
		default:
			return t, nil
		}
	}
	return read(0)
}
