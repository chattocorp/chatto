// Package meansstore adapts Gobotto's bounded collection requests to the means
// Go library. It never starts a subprocess or loads machine-local CLI settings.
package meansstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/hmans/means/collection"
)

// Runner preserves the existing internal request format and fault-injection
// boundary. Arguments are host-generated, not a general CLI interface.
type Runner struct {
	Embeddings *collection.EmbeddingConfig
	Limit      int
}

// Run loads a fresh definition for each operation so external MEANS.md changes
// are visible. Errors are fixed strings because library errors can include data.
// A result-size error after a mutation is uncertain and must never cause replay.
func (r Runner) Run(ctx context.Context, dir string, args []string, input []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	result, err := r.execute(ctx, dir, args, input)
	if err != nil {
		return nil, errors.New("means collection operation failed")
	}
	var out boundedJSON
	out.limit = r.Limit
	if err := json.NewEncoder(&out).Encode(result); err != nil {
		return nil, errors.New("means result exceeds limit or cannot be encoded")
	}
	return out.Bytes(), nil
}

type boundedJSON struct {
	bytes.Buffer
	limit int
}

func (b *boundedJSON) Write(p []byte) (int, error) {
	if b.limit <= 0 || b.Len()+len(p) > b.limit {
		return 0, errors.New("result limit")
	}
	return b.Buffer.Write(p)
}

func (r Runner) execute(ctx context.Context, dir string, args []string, input []byte) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var positional []string
	filter := map[string]any{}
	var dates []collection.DateTimeFilter
	limit, offset := 0, 0
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if arg == "--json" {
			continue
		}
		if !strings.HasPrefix(arg, "--") {
			positional = append(positional, arg)
			continue
		}
		if i+1 == len(args) {
			return nil, errors.New("missing argument")
		}
		i++
		value := args[i]
		switch arg {
		case "--path":
			if value != dir {
				return nil, errors.New("collection path mismatch")
			}
		case "--input":
			if value != "-" {
				return nil, errors.New("unsupported input")
			}
		case "--limit", "--offset":
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return nil, errors.New("invalid page")
			}
			if arg == "--limit" {
				limit = n
			} else {
				offset = n
			}
		case "--filter":
			key, value, ok := strings.Cut(value, "=")
			if !ok || key == "" {
				return nil, errors.New("invalid filter")
			}
			var decoded any
			d := json.NewDecoder(strings.NewReader(value))
			d.UseNumber()
			if err := d.Decode(&decoded); err != nil {
				return nil, err
			}
			filter[key] = decoded
		case "--after", "--before", "--on-or-after", "--on-or-before":
			key, value, ok := strings.Cut(value, "=")
			if !ok {
				return nil, errors.New("invalid datetime")
			}
			dates = append(dates, collection.DateTimeFilter{Field: key, Op: strings.ReplaceAll(strings.TrimPrefix(arg, "--"), "-", "_"), Value: value})
		default:
			return nil, errors.New("unsupported argument")
		}
	}
	if len(positional) == 0 {
		return nil, errors.New("missing operation")
	}
	op := positional[0]
	if op == "init" {
		_, err := collection.InitConfig(dir, input)
		return map[string]string{"path": dir}, err
	}
	if op == "schema" && len(positional) == 2 && positional[1] == "set" {
		return collection.SetSchema(ctx, dir, input, false)
	}
	c, err := collection.Load(dir)
	if err != nil {
		return nil, err
	}
	c = c.WithEmbeddings(r.Embeddings)
	var payload struct {
		Hash         string         `json:"hash"`
		Body         *string        `json:"body"`
		Instructions string         `json:"instructions"`
		Frontmatter  map[string]any `json:"frontmatter"`
	}
	if len(input) > 0 {
		d := json.NewDecoder(bytes.NewReader(input))
		d.UseNumber()
		if err := d.Decode(&payload); err != nil {
			return nil, err
		}
	}
	switch op {
	case "info":
		return struct {
			*collection.Description
			Schema json.RawMessage `json:"schema"`
		}{c.Describe(), c.Schema()}, nil
	case "list":
		return c.List(ctx, collection.ListOptions{Filter: filter, DateTime: dates, Limit: limit, Offset: offset})
	case "search":
		query := ""
		if len(positional) == 2 {
			query = positional[1]
		} else if len(positional) != 1 {
			return nil, errors.New("invalid query")
		}
		return c.Search(ctx, collection.SearchOptions{Query: query, Filter: filter, DateTime: dates, Limit: limit})
	case "show":
		if len(positional) != 2 {
			return nil, errors.New("missing id")
		}
		return c.Read(ctx, positional[1])
	case "create":
		title, _ := payload.Frontmatter["title"].(string)
		data, _ := payload.Frontmatter["data"].(map[string]any)
		if payload.Body == nil {
			return nil, errors.New("missing body")
		}
		return c.Create(ctx, collection.CreateOptions{Title: title, Data: data, Body: *payload.Body})
	case "update", "append":
		if len(positional) != 2 || payload.Hash == "" {
			return nil, errors.New("missing id or hash")
		}
		if op == "append" {
			if payload.Body == nil {
				return nil, errors.New("missing body")
			}
			return c.Append(ctx, positional[1], payload.Hash, *payload.Body)
		}
		return c.Update(ctx, positional[1], payload.Hash, collection.Patch{Frontmatter: payload.Frontmatter, Body: payload.Body})
	case "instructions":
		if len(positional) != 2 || positional[1] != "set" {
			return nil, errors.New("unsupported operation")
		}
		return c.SetInstructions(ctx, payload.Hash, payload.Instructions)
	default:
		return nil, errors.New("unsupported operation")
	}
}
