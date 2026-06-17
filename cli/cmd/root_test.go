package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRootHelpShowsBannerAndNoResetCommand(t *testing.T) {
	originalVersion := Version
	t.Cleanup(func() {
		SetVersion(originalVersion)
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
	})

	SetVersion("9.8.7-test")

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)

	if err := rootCmd.Help(); err != nil {
		t.Fatalf("render root help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"Chatto is a self-hostable chat server for teams and communities.",
		"Version: 9.8.7-test | Self-hosting docs: https://docs.chatto.run",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}

	if strings.Contains(help, "\n  reset ") {
		t.Fatalf("root help should not list reset command:\n%s", help)
	}
}
