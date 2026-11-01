package subagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (files, string) {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return files{root}, dir
}

func write(t *testing.T, name, text string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReadBoundaryAndExclusions(t *testing.T) {
	f, dir := fixture(t)
	out := filepath.Join(t.TempDir(), "outside.txt")
	write(t, out, "outside-secret")
	write(t, filepath.Join(dir, "source.go"), "package example\n// needle\n")
	write(t, filepath.Join(dir, ".env"), "private-secret")
	for name, target := range map[string]string{"escape": out, "alias": ".env", "safe-alias": "source.go"} {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{out, "../outside.txt", "sub/../../outside.txt", "escape", "alias", "safe-alias", ".env", "/etc/passwd", "sub\\file"} {
		if data, err := f.read(name); err == nil {
			t.Fatalf("read forbidden path %q: %q", name, data)
		}
	}
	for _, tool := range f.tools() {
		args := map[string]any{"path": "escape"}
		if tool.Name == "search_files" {
			args["query"] = "secret"
		}
		raw, _ := json.Marshal(args)
		result, err := tool.Execute(context.Background(), raw)
		if err != nil || !strings.Contains(result, `"status":"rejected"`) || strings.Contains(result, "outside-secret") {
			t.Fatal(tool.Name, result, err)
		}
	}
	text, err := f.lines(fileArgs{Path: "source.go", Start: 2})
	if err != nil || !strings.Contains(text, "2: // needle") {
		t.Fatal(text, err)
	}
	text, err = f.search(context.Background(), fileArgs{Path: ".", Query: "secret"})
	if err != nil || strings.Contains(text, "secret") {
		t.Fatal("search exposed excluded data", text, err)
	}
	text, err = f.search(context.Background(), fileArgs{Path: ".", Query: "needle"})
	if err != nil || !strings.Contains(text, "source.go:2:") {
		t.Fatal(text, err)
	}
}

func TestReadsRemainConfinedAfterSymlinkSwap(t *testing.T) {
	f, dir := fixture(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "file"), "outside-secret")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "sub", "file"), "inside")
	if err := f.check("sub/file"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "sub"), filepath.Join(dir, "original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "sub")); err != nil {
		t.Fatal(err)
	}
	// Even after a successful precheck, the actual Root open must reject escape.
	if file, err := f.root.Open("sub/file"); err == nil {
		file.Close()
		t.Fatal("root escaped after path swap")
	}
}

func TestReadAndSearchLimits(t *testing.T) {
	f, dir := fixture(t)
	write(t, filepath.Join(dir, "large"), strings.Repeat("x", maxFileBytes+1))
	write(t, filepath.Join(dir, "binary"), "x\x00y")
	for _, name := range []string{"large", "binary"} {
		if _, err := f.read(name); err == nil {
			t.Fatal("accepted", name)
		}
	}
	write(t, filepath.Join(dir, "many"), strings.Repeat("needle\n", 300))
	text, _ := f.lines(fileArgs{Path: "many"})
	if !strings.Contains(text, `"next_line":201`) || !strings.Contains(text, `"truncated":true`) {
		t.Fatal(text)
	}
	text, _ = f.search(context.Background(), fileArgs{Path: ".", Query: "needle"})
	if !strings.Contains(text, `"truncated":true`) {
		t.Fatal(text)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tool := range f.tools() {
		text, _ := tool.Execute(ctx, json.RawMessage(`{"path":"many"}`))
		if !strings.Contains(text, "cancelled") {
			t.Fatal(text)
		}
	}
}
