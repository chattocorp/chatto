package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
)

func TestSchedulesReservedFromGenericBrainTools(t *testing.T) {
	m := &Manager{directory: t.TempDir(), run: func(context.Context, string, []string, []byte) ([]byte, error) {
		t.Fatal("reserved schedule reached means")
		return nil, nil
	}}
	for _, operation := range []string{"create", "archive", "schema_set", "instructions_set"} {
		out, err := toolCall(t, m.Tools(true), "memory_brain_"+operation, `{"name":"schedules"}`)
		if err != nil || !strings.Contains(out, "error") {
			t.Fatal("reserved lifecycle accepted", operation, out, err)
		}
	}
	for _, operation := range []string{"info", "list", "search", "read", "create", "update"} {
		args := `{"collection":"schedules"}`
		if operation == "search" {
			args = `{"collection":"schedules","query":""}`
		}
		out, err := toolCall(t, m.Tools(true), "memory_"+operation, args)
		if err != nil || !strings.Contains(out, "error") {
			t.Fatal("reserved collection accepted", operation, out, err)
		}
	}
}

func TestConversationsReservedFromGenericToolsAndAliases(t *testing.T) {
	m := &Manager{directory: t.TempDir(), run: func(context.Context, string, []string, []byte) ([]byte, error) {
		t.Fatal("journal reached means")
		return nil, nil
	}}
	for _, op := range []string{"create", "archive", "schema_set", "instructions_set"} {
		out, err := toolCall(t, m.Tools(true), "memory_brain_"+op, `{"name":"conversations"}`)
		if err != nil || !strings.Contains(out, "reserved") {
			t.Fatal("journal lifecycle exposed", out, err)
		}
	}
	for _, op := range []string{"info", "list", "read", "create", "update"} {
		out, err := toolCall(t, m.Tools(true), "memory_"+op, `{"collection":"conversations"}`)
		if err != nil || !strings.Contains(out, "error") {
			t.Fatal("journal collection exposed", out, err)
		}
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "means"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	brains := filepath.Join(root, "brains")
	reserved := filepath.Join(brains, "conversations")
	if err := os.MkdirAll(reserved, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reserved, "MEANS.md"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(reserved, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{reserved, alias} {
		if _, err := New([]config.Collection{{Name: "innocent", Path: path}}, brains); err == nil || !strings.Contains(err.Error(), "conversations is reserved") {
			t.Fatal("journal alias allowed", err)
		}
	}
}

func TestScheduleBindingAliasesRejected(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "means"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	brains := filepath.Join(root, "brains")
	schedules := filepath.Join(brains, "schedules")
	if err := os.MkdirAll(schedules, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(schedules, "MEANS.md"), []byte("private schedule definition"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(schedules, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{schedules, alias} {
		_, err := New([]config.Collection{{Name: "innocent_name", Path: path, Writable: true}}, brains)
		if err == nil || !strings.Contains(err.Error(), "schedules is reserved") {
			t.Fatal("schedule alias not blocked", err)
		}
	}
	t.Chdir(root)
	if _, err := New([]config.Collection{{Name: "alias", Path: "alias"}}, brains); err == nil || !strings.Contains(err.Error(), "schedules is reserved") {
		t.Fatal("relative schedule alias not blocked", err)
	}
	if err := os.Mkdir("ordinary", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("ordinary", "MEANS.md"), []byte("ordinary definition"), 0600); err != nil {
		t.Fatal(err)
	}
	// Ordinary bindings pass validation without consulting the CLI stub.
	if _, err := New([]config.Collection{{Name: "ordinary", Path: "ordinary"}}, brains); err != nil {
		t.Fatal("valid relative binding did not reach initialization", err)
	}
}
