package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
)

func TestListingValidationAndReadPrerequisite(t *testing.T) {
	m := &Manager{collections: []config.Collection{{Name: "notes", Path: "/fixed", Writable: true}}}
	var got []string
	m.run = func(_ context.Context, _ string, args []string, _ []byte) ([]byte, error) {
		if args[3] == "info" {
			return []byte(`{"schema":{}}`), nil
		}
		got = args
		return []byte(`{"documents":[{"id":"abcde"}],"diagnostics":[],"has_more":true}`), nil
	}
	tools := m.Tools(false)
	for _, raw := range []string{`{"collection":"notes"}`, `{"collection":"unknown"}`, `{"collection":"notes","offset":-1}`, `{"collection":"notes","offset":null}`, `{"collection":"notes","limit":null}`, `{"collection":"notes","limit":0}`, `{"collection":"notes","limit":51}`, `{"collection":"notes","offset":1.5}`, `{"collection":"notes","filters":null}`, `{"collection":"notes","query":"x"}`} {
		out, err := toolCall(t, tools, "memory_list", raw)
		if err != nil || !strings.Contains(out, "rejected") || got != nil {
			t.Fatal("unsafe listing", raw, out, err)
		}
	}
	toolCall(t, tools, "memory_info", `{"collection":"notes"}`)
	out, err := toolCall(t, tools, "memory_list", `{"collection":"notes","offset":20,"limit":5,"filters":{"tag":"true"},"after":{"at":"2026-11-01T00:00:00Z"}}`)
	want := []string{"--path", "/fixed", "--json", "list", "--offset", "20", "--limit", "5", "--filter", `tag="true"`, "--after", "at=2026-11-01T00:00:00Z"}
	if err != nil || !reflect.DeepEqual(got, want) || !strings.Contains(out, `"next_offset":21`) {
		t.Fatal("incorrect listing", got, out, err)
	}
	got = nil
	out, err = toolCall(t, tools, "memory_update", `{"collection":"notes","id":"abcde","hash":"invented","title":"Changed","body":"","data":{}}`)
	if err != nil || !strings.Contains(out, "memory_read") || got != nil {
		t.Fatal("listing authorized a write", out, err)
	}
	out, _ = toolCall(t, m.Tools(false), "memory_list", `{"collection":"notes","offset":21}`)
	if !strings.Contains(out, "memory_info") {
		t.Fatal("next turn reused guidance", out)
	}
	toolCall(t, tools, "memory_list", `{"collection":"notes"}`)
	if !reflect.DeepEqual(got, []string{"--path", "/fixed", "--json", "list", "--offset", "0", "--limit", "20"}) {
		t.Fatal("incorrect defaults", got)
	}
	got = nil
	m.collections = nil
	out, _ = toolCall(t, tools, "memory_list", `{"collection":"notes","offset":21}`)
	if !strings.Contains(out, "Unknown configured collection") || got != nil {
		t.Fatal("removed collection remained accessible", out)
	}
}

func TestListingPageRejectsInvalidContinuation(t *testing.T) {
	for _, raw := range []string{`{}`, `{"documents":[],"diagnostics":[],"has_more":true}`, `{"documents":[{},{}],"diagnostics":[],"has_more":false}`, `{"documents":[],"diagnostics":[]}`} {
		if _, err := listingPage([]byte(raw), 0, 1); err == nil {
			t.Fatal("invalid page accepted", raw)
		}
	}
	out, err := listingPage([]byte(`{"documents":[],"diagnostics":[{"message":"invalid document"}],"has_more":false}`), 50, 20)
	if err != nil || strings.Contains(out, "next_offset") || !strings.Contains(out, "invalid document") {
		t.Fatal("lost diagnostics or invented continuation", out, err)
	}
}

func TestInstalledMeansPaginatedListing(t *testing.T) {
	binary, err := exec.LookPath("means")
	if err != nil {
		t.Skip("means is not installed")
	}
	help, err := exec.Command(binary, "list", "--help").Output()
	if err != nil || !strings.Contains(string(help), "--offset") {
		t.Skip("means lacks list --offset")
	}
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	path := filepath.Join(root, "records")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	definition := "---\ntitle: Records\ndescription: Synthetic listing\nschema: {}\n---\nList these records.\n"
	if err := os.WriteFile(filepath.Join(path, "MEANS.md"), []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 53; i++ {
		body := fmt.Sprintf("---\nid: d%04d\ntitle: Record %d\ndata:\n  selected: %t\n---\nBody.\n", i, i, i%2 == 0)
		if err := os.WriteFile(filepath.Join(path, fmt.Sprintf("%03d.md", i)), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := New([]config.Collection{{Name: "records", Path: path}})
	if err != nil {
		t.Fatal(err)
	}
	tools := m.Tools(false)
	if out, err := toolCall(t, tools, "memory_info", `{"collection":"records"}`); err != nil || strings.Contains(out, "rejected") {
		t.Fatal(out, err)
	}
	for _, filtered := range []bool{false, true} {
		offset, seen := 0, map[string]bool{}
		for page := 0; ; page++ {
			if page > 3 {
				t.Fatal("listing did not terminate")
			}
			args := map[string]any{"collection": "records", "offset": offset}
			if filtered {
				args["filters"] = map[string]any{"selected": true}
			}
			raw, _ := json.Marshal(args)
			out, err := toolCall(t, tools, "memory_list", string(raw))
			var result struct {
				Documents []struct {
					ID string `json:"id"`
				}
				HasMore     bool `json:"has_more"`
				Next        *int `json:"next_offset"`
				Diagnostics []any
			}
			if err != nil || json.Unmarshal([]byte(out), &result) != nil || len(result.Diagnostics) != 0 || len(result.Documents) == 0 {
				t.Fatal("bad page", out, err)
			}
			for _, doc := range result.Documents {
				if doc.ID == "" || seen[doc.ID] {
					t.Fatal("missing or duplicate ID", out)
				}
				seen[doc.ID] = true
			}
			if !result.HasMore {
				if result.Next != nil {
					t.Fatal("terminal cursor")
				}
				break
			}
			if result.Next == nil || *result.Next != offset+len(result.Documents) {
				t.Fatal("bad continuation", out)
			}
			offset = *result.Next
		}
		want := 53
		if filtered {
			want = 27
		}
		if len(seen) != want {
			t.Fatal("incomplete traversal", len(seen), want)
		}
	}
	for _, offset := range []int{53, 100} {
		out, err := toolCall(t, tools, "memory_list", fmt.Sprintf(`{"collection":"records","offset":%d}`, offset))
		if err != nil || !strings.Contains(out, `"documents":[]`) || strings.Contains(out, "next_offset") {
			t.Fatal("past-end page", out, err)
		}
	}
	// Offsets are explicitly live, not snapshot cursors. Removing a preceding
	// filename shifts the first record on the next page.
	if err := os.Remove(filepath.Join(path, "000.md")); err != nil {
		t.Fatal(err)
	}
	out, err := toolCall(t, tools, "memory_list", `{"collection":"records","offset":20,"limit":1}`)
	if err != nil || !strings.Contains(out, `"id":"d0021"`) {
		t.Fatal("unexpected live offset semantics", out, err)
	}
	out, err = toolCall(t, tools, "memory_list", `{"collection":"records","filters":{"missing":true}}`)
	if err != nil || !strings.Contains(out, `"documents":[]`) || strings.Contains(out, "next_offset") {
		t.Fatal("empty filtered listing", out, err)
	}
}
