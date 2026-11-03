package brainview

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/hmans/means/collection"
)

func TestModifiedOrderAppliesBeforePaginationAndRefreshes(t *testing.T) {
	dir := t.TempDir()
	if _, err := collection.Init(dir, nil); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, PageSize+1)
	for i := range paths {
		// Deliberately unrelated filenames exercise stored-ID lookup.
		paths[i] = filepath.Join(dir, fmt.Sprintf("file-%03d.md", i))
		body := fmt.Sprintf("---\nid: doc-%02d\ntitle: Document\ndata: {}\n---\nBody", i)
		if err := os.WriteFile(paths[i], []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		at := time.Unix(int64(1000+i), 0)
		if err := os.Chtimes(paths[i], at, at); err != nil {
			t.Fatal(err)
		}
	}
	b := New(config.Memory{Collections: []config.Collection{{Name: "test", Path: dir}}})
	page, err := b.List(context.Background(), "apps/test", 0)
	if err != nil || len(page.Entries) != PageSize || page.Entries[0].ID != "doc-50" || !page.More {
		t.Fatal("newest not first across pages", err)
	}
	last, err := b.List(context.Background(), "apps/test", PageSize)
	if err != nil || len(last.Entries) != 1 || last.Entries[0].ID != "doc-00" || last.More {
		t.Fatal("oldest not last", err)
	}
	updated := time.Unix(2000, 0)
	for _, i := range []int{0, 1} {
		if err := os.Chtimes(paths[i], updated, updated); err != nil {
			t.Fatal(err)
		}
	}
	page, err = b.List(context.Background(), "apps/test", 0)
	if err != nil || page.Entries[0].ID != "doc-00" || page.Entries[1].ID != "doc-01" {
		t.Fatal("refresh or stable tie-break failed", err)
	}
}

func TestTimestampScanRespectsCollectionBoundaries(t *testing.T) {
	dir := t.TempDir()
	for _, folder := range []string{".", "nested"} {
		if _, err := collection.Init(filepath.Join(dir, folder), nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"normal.md", ".hidden.md", "nested/child.md"} {
		data := fmt.Sprintf("---\nid: %q\ntitle: Test\ndata: {}\n---\nBody", name)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "nested/child.md"), filepath.Join(dir, "alias.md")); err != nil {
		t.Fatal(err)
	}
	times, err := modificationTimes(context.Background(), dir)
	if err != nil || len(times) != 1 || times["normal.md"].IsZero() {
		t.Fatal("timestamp scan crossed boundary", times, err)
	}
}
