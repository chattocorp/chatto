package memory

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
)

func TestSearchFiltersPreserveTypesAndArgumentBoundaries(t *testing.T) {
	m := &Manager{collections: []config.Collection{{Name: "notes", Path: "/fixed"}}}
	var got []string
	m.run = func(_ context.Context, _ string, args []string, _ []byte) ([]byte, error) {
		if args[3] == "info" {
			return []byte(`{"schema":{}}`), nil
		}
		got = args
		return []byte(`{"documents":[],"has_more":false,"diagnostics":[]}`), nil
	}
	tools := m.Tools(false)
	toolCall(t, tools, "memory_info", `{"collection":"notes"}`)
	_, err := toolCall(t, tools, "memory_search", `{"collection":"notes","query":"--path=/outside","filters":{"user_id":"true","number":9007199254740993,"nested":{"name":"$(command)"},"tags":["a","b"],"empty":null},"after":{"created":"2026-11-01T00:00:00Z"},"on_or_before":{"created":"2026-11-02T01:00:00+01:00"}}`)
	want := []string{"--path", "/fixed", "--json", "search", "--limit", "20", "--filter", "empty=null", "--filter", `nested={"name":"$(command)"}`, "--filter", "number=9007199254740993", "--filter", `tags=["a","b"]`, "--filter", `user_id="true"`, "--after", "created=2026-11-01T00:00:00Z", "--on-or-before", "created=2026-11-02T01:00:00+01:00", "--", "--path=/outside"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("filter encoding changed types or argument boundaries", err, got)
	}
}

func TestRejectInvalidSearchFiltersBeforeCLI(t *testing.T) {
	m := &Manager{run: func(context.Context, string, []string, []byte) ([]byte, error) {
		t.Fatal("invalid filter reached CLI")
		return nil, nil
	}}
	for _, filter := range []string{
		`"filters":null`, `"filters":[]`, `"filters":{"bad=key":"x"}`, `"filters":{"":"x"}`, `"filters":{"bad\u0000key":"x"}`,
		`"after":{"created":"2026-11-01"}`, `"before":{"created":"2026-11-01T00:00:00"}`, `"on_or_after":{"created":null}`, `"on_or_before":null`,
	} {
		out, err := toolCall(t, m.Tools(false), "memory_search", `{"collection":"unknown","query":"",`+filter+`}`)
		if err != nil || !strings.Contains(out, "error") || strings.Contains(out, "Unknown configured collection") {
			t.Fatal("invalid filter not rejected early", filter, err, out)
		}
	}
}

func TestInstalledMeansStructuredSearch(t *testing.T) {
	if _, err := exec.LookPath("means"); err != nil {
		t.Skip("means is not installed")
	}
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	if err := os.MkdirAll(filepath.Join(root, "config", "means"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "means", "config.yaml"), []byte("embeddings: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(nil, filepath.Join(root, "brains"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := toolCall(t, m.Tools(true), "memory_brain_create", `{"name":"records","title":"Records","description":"Synthetic search records","schema":{},"instructions":"Search structured records."}`)
	if err != nil || !strings.Contains(out, "complete") {
		t.Fatal("create app", err, out)
	}
	tools := m.Tools(false)
	toolCall(t, tools, "memory_info", `{"collection":"records"}`)
	for _, record := range []struct{ title, data string }{
		{"first", `{"kind":"user_fact","user_id":"alice","status":"open","priority":"high","at":"2026-11-01T12:00:00Z","count":2,"active":true,"tags":["a","b"],"meta":{"team":"blue","extra":1},"nullable":null,"literal":"true"}`},
		{"second", `{"kind":"user_fact","user_id":"bob","status":"open","priority":"low","at":"2026-11-02T12:00:00Z","count":"2","tags":["b","a"],"meta":{"team":"red"},"literal":true}`},
		{"third", `{"kind":"general_fact","status":"done","priority":"high","at":"2026-11-03T12:00:00Z"}`},
	} {
		out, err := toolCall(t, tools, "memory_create", `{"collection":"records","title":"`+record.title+`","body":"A project note","data":`+record.data+`}`)
		if err != nil || !strings.Contains(out, `"id"`) {
			t.Fatal("create record", err, out)
		}
	}
	for _, tc := range []struct {
		name, query, filters string
		want                 []string
	}{
		{"user", "", `"filters":{"kind":"user_fact","user_id":"alice"}`, []string{"first"}},
		{"app", "project", `"filters":{"status":"open","priority":"high"}`, []string{"first"}},
		{"number", "", `"filters":{"count":2}`, []string{"first"}},
		{"boolean", "", `"filters":{"active":true}`, []string{"first"}},
		{"object", "", `"filters":{"meta":{"team":"blue"}}`, []string{"first"}},
		{"array", "", `"filters":{"tags":["a","b"]}`, []string{"first"}},
		{"null", "", `"filters":{"nullable":null}`, []string{"first"}},
		{"string", "", `"filters":{"literal":"true"}`, []string{"first"}},
		{"exclusive", "", `"after":{"at":"2026-11-01T12:00:00Z"},"before":{"at":"2026-11-03T12:00:00Z"}`, []string{"second"}},
		{"inclusive", "", `"on_or_after":{"at":"2026-11-01T12:00:00Z"},"on_or_before":{"at":"2026-11-01T12:00:00Z"}`, []string{"first"}},
		{"text excludes", "missing", `"filters":{"user_id":"alice"}`, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query, _ := json.Marshal(tc.query)
			out, err := toolCall(t, tools, "memory_search", `{"collection":"records","query":`+string(query)+`,`+tc.filters+`}`)
			var result struct {
				Documents   []struct{ Frontmatter struct{ Title string } }
				HasMore     bool `json:"has_more"`
				Diagnostics []any
			}
			if err != nil || json.Unmarshal([]byte(out), &result) != nil || result.HasMore || len(result.Diagnostics) > 0 {
				t.Fatal("search failed", err, out)
			}
			got := []string{}
			for _, doc := range result.Documents {
				got = append(got, doc.Frontmatter.Title)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatal("wrong matches", got, tc.want)
			}
		})
	}
}
