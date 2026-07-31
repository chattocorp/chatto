package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"hmans.de/authling"
)

func TestRunVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"version"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("run(version) exit code = %d, want 0", exitCode)
	}
	if got, want := stdout.String(), fmt.Sprintf("authling version %s\n", authling.Version); got != want {
		t.Errorf("run(version) stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("run(version) stderr = %q, want empty", got)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"wat"}, &stdout, &stderr)

	if exitCode != 2 {
		t.Fatalf("run(wat) exit code = %d, want 2", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("run(wat) stdout = %q, want empty", got)
	}
	if got, want := stderr.String(), "unknown command: wat\n"; got != want {
		t.Errorf("run(wat) stderr = %q, want %q", got, want)
	}
}

func TestRunHelpListsRun(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"help"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("run(help) exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "run") {
		t.Fatalf("run(help) stdout = %q, want run command", stdout.String())
	}
}

func TestRunCommandRejectsMissingExplicitConfig(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	path := filepath.Join(t.TempDir(), "missing.toml")

	exitCode := runContext(
		context.Background(),
		[]string{"run", "--config", path},
		&stdout,
		&stderr,
	)

	if exitCode != 1 {
		t.Fatalf("run(run) exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), "load configuration") {
		t.Fatalf("run(run) stderr = %q, want configuration error", stderr.String())
	}
}

func TestRunCommandHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"run", "--help"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("run(run --help) exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stderr.String(), "-config") {
		t.Fatalf("run(run --help) stderr = %q, want config flag", stderr.String())
	}
}
