// Package interchange reads the portable history contract without contacting
// source platforms. Validation errors identify record positions, never content.
//
//go:generate go run ./internal/genschemas
package interchange

import (
	"bufio"
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

// MaxRecordBytes bounds one JSON record, not the archive or the file payloads.
const MaxRecordBytes = 1 << 20

// Archive holds catalogs and a root-relative source. Event bodies and attachment
// bytes are streamed. Reference indexes grow with record count, not payload size.
type Archive struct {
	root       *os.Root
	sums       map[string]string
	sizes      map[string]int64
	digest     string
	recordSums map[string][][32]byte
	Manifest   Manifest
	Users      []User
	Rooms      []Room
	Assets     []Asset
	EventCount int
}

// Close releases the directory handle.
func (a *Archive) Close() error { return a.root.Close() }

// Digest binds progress to the exact source bytes.
func (a *Archive) Digest() string { return a.digest }

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

// Reaction is one Unicode sequence or reaction name and its final source reactors.
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

// ReadDirectory verifies the entire archive before any destination write.
// It does not fetch URLs or create files in the source directory.
func ReadDirectory(dir string) (*Archive, error) {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return nil, errors.New("import: source must be a directory, not a link")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, errors.New("import: cannot open source directory")
	}
	a := &Archive{root: root, sums: map[string]string{}, sizes: map[string]int64{}, recordSums: map[string][][32]byte{}}
	success := false
	defer func() {
		if !success {
			_ = root.Close()
		}
	}()
	digest := sha256.New()
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
		if _, ok := names[name]; !ok && (path.Dir(name) != "files" || strings.ContainsAny(path.Base(name), "\\:")) {
			return errors.New("import: unexpected file")
		}
		f, err := a.open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		size, err := io.Copy(h, f)
		if err != nil {
			return errors.New("import: cannot read source file")
		}
		sum := hex.EncodeToString(h.Sum(nil))
		a.sums[name] = sum
		a.sizes[name] = size
		fmt.Fprintf(digest, "%d:%s:%d:%s\n", len(name), name, size, sum)
		return nil
	})
	if err != nil {
		return nil, err
	}
	a.digest = hex.EncodeToString(digest.Sum(nil))
	for _, name := range []string{"manifest.json", "users.jsonl", "rooms.jsonl", "assets.jsonl"} {
		err = a.records(name, func(line []byte) error {
			switch name {
			case "manifest.json":
				return json.Unmarshal(line, &a.Manifest)
			case "users.jsonl":
				var v User
				if err := json.Unmarshal(line, &v); err != nil {
					return err
				}
				a.Users = append(a.Users, v)
			case "rooms.jsonl":
				var v Room
				if err := json.Unmarshal(line, &v); err != nil {
					return err
				}
				a.Rooms = append(a.Rooms, v)
			case "assets.jsonl":
				var v Asset
				if err := json.Unmarshal(line, &v); err != nil {
					return err
				}
				a.Assets = append(a.Assets, v)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if err = a.validateReferences(); err != nil {
		return nil, err
	}
	success = true
	return a, nil
}

// open forbids symlinks, hard links, special files, and escapes from the root.
func (a *Archive) open(name string) (*os.File, error) {
	current, err := a.root.Lstat(name)
	if err != nil || !current.Mode().IsRegular() {
		return nil, errors.New("import: links and special files are forbidden")
	}
	f, err := a.root.Open(name)
	if err != nil {
		return nil, errors.New("import: cannot open source file")
	}
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || !os.SameFile(stat, current) {
		f.Close()
		return nil, errors.New("import: source changed while reading")
	}
	if !singleLink(stat) {
		f.Close()
		return nil, errors.New("import: hard links are forbidden")
	}
	return f, nil
}

// OpenAsset opens verified bytes for chunked upload. The upload hash checks for
// changes after preflight; the server must reject bytes with another digest.
func (a *Archive) OpenAsset(asset Asset) (*os.File, error) { return a.open(asset.Storage.Path) }

func (a *Archive) records(name string, visit func([]byte) error) error {
	validators, err := resolved()
	if err != nil {
		return errors.New("import: embedded schema error")
	}
	expected, ok := a.sums[name]
	if !ok {
		return errors.New("import: metadata file missing")
	}
	f, err := a.open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	hash := sha256.New()
	reader := bufio.NewReaderSize(io.TeeReader(f, hash), MaxRecordBytes+1)
	index := 0
	previous, checked := a.recordSums[name]
	var sums [][32]byte
	for {
		var line []byte
		var readErr error
		if name == "manifest.json" {
			line, readErr = io.ReadAll(io.LimitReader(reader, MaxRecordBytes+1))
		} else {
			line, readErr = reader.ReadSlice('\n')
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}
		index++
		if len(line) > MaxRecordBytes || readErr == bufio.ErrBufferFull {
			return fmt.Errorf("import: %s record %d exceeds size limit", name, index)
		}
		if name != "manifest.json" && (len(line) == 0 || line[len(line)-1] != '\n') {
			return fmt.Errorf("import: %s must end in a newline", name)
		}
		if readErr != nil && readErr != io.EOF {
			return errors.New("import: cannot read metadata")
		}
		sum := sha256.Sum256(line)
		if checked {
			if index > len(previous) || sum != previous[index-1] {
				return errors.New("import: metadata changed since validation")
			}
		} else {
			value, err := strictJSON(line)
			if err != nil || validators[names[name]].Validate(value) != nil {
				return fmt.Errorf("import: %s record %d violates format v1", name, index)
			}
			sums = append(sums, sum)
		}
		if err = visit(line); err != nil {
			return fmt.Errorf("%s record %d: %w", name, index, err)
		}
		if name == "manifest.json" {
			break
		}
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return errors.New("import: metadata changed since validation")
	}
	if !checked {
		a.recordSums[name] = sums
	}
	return nil
}

// VisitEvents reads validated records in their authoritative file order.
func (a *Archive) VisitEvents(visit func(Event) error) error {
	return a.records("events.jsonl", func(line []byte) error {
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return err
		}
		return visit(e)
	})
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
