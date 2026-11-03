package brainview

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
)

func TestWatchAtomicSavesAppDiscoveryAndShutdown(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "apps"), 0700); err != nil {
		t.Fatal(err)
	}
	fixed := t.TempDir()
	b := New(config.Memory{Directory: root, Collections: []config.Collection{{Name: "external", Path: fixed}}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes, err := b.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	changed := func() {
		t.Helper()
		select {
		case err, ok := <-changes:
			if !ok || err != nil {
				t.Fatal("watch failed", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("missing change")
		}
	}
	write := func(path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(fixed, "note.md"))
	changed()
	write(filepath.Join(fixed, ".replacement"))
	if err := os.Rename(filepath.Join(fixed, ".replacement"), filepath.Join(fixed, "note.md")); err != nil {
		t.Fatal(err)
	}
	changed()
	app := filepath.Join(root, "apps", "new-app")
	if err := os.Mkdir(app, 0700); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(app, "MEANS.md"))
	changed()
	write(filepath.Join(app, "note.md"))
	changed()
	if err := os.RemoveAll(app); err != nil {
		t.Fatal(err)
	}
	changed()
	cancel()
	select {
	case _, ok := <-changes:
		if ok {
			t.Fatal("watch did not close")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watcher survived cancellation")
	}
}

// Collections are watched independently of the catalog root, including built-ins
// created after startup and fixed bindings outside the managed directory.
func TestWatchLateCollections(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "brains")
	fixed := filepath.Join(t.TempDir(), "fixed")
	b := New(config.Memory{Directory: root, Collections: []config.Collection{{Name: "external", Path: fixed}}})
	ctx, cancel := context.WithCancel(context.Background())
	changes, err := b.Watch(ctx)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() {
		cancel()
		deadline := time.After(5 * time.Second)
		for {
			select {
			case _, ok := <-changes:
				if !ok {
					return
				}
			case <-deadline:
				t.Fatal("watcher survived cancellation")
			}
		}
	}()
	changed := func() {
		t.Helper()
		select {
		case err := <-changes:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("missing notification")
		}
	}
	for _, path := range []string{root, filepath.Join(root, "memory"), fixed} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		// Invalid configuration still establishes a collection boundary.
		if err := os.WriteFile(filepath.Join(path, "MEANS.md"), []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
		changed()
		if err := os.WriteFile(filepath.Join(path, "note.md"), []byte("note"), 0600); err != nil {
			t.Fatal(err)
		}
		changed()
	}
}
